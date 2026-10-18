package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/netsafe"
)

// ErrNotFound means the source answered that there is no picture (a remembered miss).
var ErrNotFound = errors.New("images: no picture at the source")

// Fetch limits.
const (
	fetchTimeout = 10 * time.Second
	maxRedirects = 3
	maxHTMLBytes = 512 << 10
	maxJSONBytes = 64 << 10
)

// Fetcher downloads pictures from the internet through the SSRF guard.
type Fetcher struct {
	Dev    bool         // development instance: http and loopback targets allowed
	Client *http.Client // guarded, no redirects; nil = netsafe.Client
	// Base URLs, overridable in tests.
	GravatarBase string // default https://gravatar.com/avatar/
	// LogoURL, when set, returns the logo provider URL for a domain (LOGO_DEV_TOKEN).
	LogoURL func(domain string) string
	// SiteBase maps a domain to its home page URL; default https://<domain>/.
	SiteBase  func(domain string) string
	UserAgent string
}

func (f *Fetcher) client() *http.Client {
	if f.Client == nil {
		f.Client = netsafe.Client(f.Dev, fetchTimeout)
	}
	return f.Client
}

func (f *Fetcher) guard() netsafe.Guard { return netsafe.Guard{Dev: f.Dev} }

// get fetches raw with at most maxRedirects hops, each validated, and returns the body
// (at most max bytes) when its type is one of types (prefix match, "" = any).
func (f *Fetcher) get(ctx context.Context, raw string, max int64, types []string, header http.Header) ([]byte, string, *url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	cur := raw
	for hop := 0; ; hop++ {
		u, err := f.guard().ValidateURL(ctx, cur)
		if err != nil {
			return nil, "", nil, fmt.Errorf("images: refused url: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, "", nil, err
		}
		for k, v := range header {
			req.Header[k] = v
		}
		ua := f.UserAgent
		if ua == "" {
			ua = "opendesk-images/1 (+https://opendesk.sh)"
		}
		req.Header.Set("User-Agent", ua)
		res, err := f.client().Do(req)
		if err != nil {
			return nil, "", nil, err
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			loc := res.Header.Get("Location")
			res.Body.Close()
			if loc == "" || hop >= maxRedirects {
				return nil, "", nil, errors.New("images: too many redirects")
			}
			next, err := req.URL.Parse(loc)
			if err != nil {
				return nil, "", nil, err
			}
			cur = next.String()
			continue
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone {
			return nil, "", nil, ErrNotFound
		}
		if res.StatusCode != http.StatusOK {
			return nil, "", nil, fmt.Errorf("images: status %d", res.StatusCode)
		}
		ct, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
		ct = strings.ToLower(ct)
		if len(types) > 0 && !hasPrefixAny(ct, types) {
			return nil, "", nil, ErrUnsupported
		}
		if res.ContentLength > max {
			return nil, "", nil, ErrTooLarge
		}
		b, err := ReadLimited(res.Body, max)
		return b, ct, req.URL, err
	}
}

func hasPrefixAny(s string, ps []string) bool {
	for _, p := range ps {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// imageTypes are the content types accepted for picture downloads. The bytes are
// sniffed and decoded regardless; the header only filters out pages and documents.
var imageTypes = []string{"image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/x-icon",
	"image/vnd.microsoft.icon", "image/ico", "application/octet-stream"}

// Image downloads a picture URL and processes it.
func (f *Fetcher) Image(ctx context.Context, raw string, fit Fit) (Processed, error) {
	return f.image(ctx, raw, fit, nil, MinSide)
}

func (f *Fetcher) image(ctx context.Context, raw string, fit Fit, header http.Header, minSide int) (Processed, error) {
	b, _, _, err := f.get(ctx, raw, MaxBytes, imageTypes, header)
	if err != nil {
		return Processed{}, err
	}
	if ico := icoPNG(b); ico != nil {
		b = ico
	}
	return processMin(b, fit, minSide)
}

func processMin(b []byte, fit Fit, minSide int) (Processed, error) {
	img, err := Decode(b)
	if err != nil {
		return Processed{}, err
	}
	if r := img.Bounds(); r.Dx() < minSide || r.Dy() < minSide {
		return Processed{}, ErrDimensions
	}
	return Render(img, fit)
}

// Icon processes a logo-like picture fitted on a square: an https URL downloaded like
// Image, or a base64 data: URI of a PNG, JPEG, GIF or WebP image. Icons under 32 px a
// side are refused, like site icons.
func (f *Fetcher) Icon(ctx context.Context, raw string) (Processed, error) {
	if b, ok, err := DataImage(raw); ok {
		if err != nil {
			return Processed{}, err
		}
		return processMin(b, Contain, minIconSide)
	}
	return f.image(ctx, raw, Contain, nil, minIconSide)
}

var dataImage = regexp.MustCompile(`(?i)^data:image/(png|jpeg|jpg|gif|webp);base64,`)

// DataImage decodes a base64 data: URI of a PNG, JPEG, GIF or WebP picture (the bytes
// are still sniffed and decoded by Process). ok is false when raw is not a data: URI;
// other media types, SVG among them, are ErrUnsupported.
func DataImage(raw string) (b []byte, ok bool, err error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 5 || !strings.EqualFold(raw[:5], "data:") {
		return nil, false, nil
	}
	head := dataImage.FindString(raw)
	if head == "" {
		return nil, true, ErrUnsupported
	}
	enc := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, raw[len(head):])
	if base64.StdEncoding.DecodedLen(len(enc)) > MaxBytes+3 {
		return nil, true, ErrTooLarge
	}
	if b, err = base64.StdEncoding.DecodeString(enc); err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(enc); err != nil {
			return nil, true, ErrUnsupported
		}
	}
	if len(b) > MaxBytes {
		return nil, true, ErrTooLarge
	}
	return b, true, nil
}

// icoPNG returns the largest PNG image embedded in an ICO file, or nil. Bitmap entries
// are not decoded.
func icoPNG(b []byte) []byte {
	if len(b) < 6 || binary.LittleEndian.Uint16(b[0:2]) != 0 || binary.LittleEndian.Uint16(b[2:4]) != 1 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(b[4:6]))
	var best []byte
	bestSide := -1
	for i := 0; i < n && i < 64; i++ {
		e := 6 + i*16
		if e+16 > len(b) {
			return best
		}
		side := int(b[e])
		if side == 0 {
			side = 256
		}
		size := int(binary.LittleEndian.Uint32(b[e+8 : e+12]))
		off := int(binary.LittleEndian.Uint32(b[e+12 : e+16]))
		if off < 0 || size <= 0 || off+size > len(b) || off+size < off {
			continue
		}
		data := b[off : off+size]
		if Sniff(data) == FormatPNG && side > bestSide {
			best, bestSide = data, side
		}
	}
	if best == nil {
		return []byte{} // an ICO without PNG entries: decoding fails as unsupported
	}
	return bytes.Clone(best)
}

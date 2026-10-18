package integrations

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/images"
)

func pngOf(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestServerIconsParsed(t *testing.T) {
	srv := httptest.NewServer(&fakeMCP{icons: []any{
		map[string]any{"src": "https://example.com/a.png", "mimeType": "image/png", "sizes": []string{"48x48", "96x96"}, "theme": "light"},
		map[string]any{"src": "https://example.com/b.png", "sizes": "128x128"}, // a string, read as a list
		map[string]any{"src": ""},
		"not an icon",
	}})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := newSession(http.DefaultClient, srv.URL+"/mcp", "")
	if err := s.open(ctx); err != nil {
		t.Fatal(err)
	}
	got := s.Icons()
	if len(got) != 2 || got[0].Src != "https://example.com/a.png" || got[0].Theme != "light" || !slices.Equal(got[0].Sizes, []string{"48x48", "96x96"}) ||
		largestSide(got[0].Sizes) != 96 || !slices.Equal(got[1].Sizes, []string{"128x128"}) {
		t.Fatalf("icons = %+v", got)
	}
	// The protocol version is still read beside the icons.
	if s.version != protocolVersion {
		t.Fatalf("version = %q", s.version)
	}
}

func TestLegacyServerHasNoIcons(t *testing.T) {
	l := &legacyMCP{streams: map[string]chan []byte{}}
	l.icons = []any{map[string]any{"src": "https://example.com/a.png"}}
	srv := httptest.NewServer(l)
	defer srv.Close()
	s := newSession(http.DefaultClient, srv.URL+"/sse", "")
	if err := s.open(context.Background()); err != nil || !s.isLegacy() || len(s.Icons()) != 0 {
		t.Fatalf("legacy = %v icons = %v err = %v", s.isLegacy(), s.Icons(), err)
	}
}

func TestIconCandidates(t *testing.T) {
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	icons := []ServerIcon{
		{Src: "https://example.com/logo.svg"},                                    // SVG by name
		{Src: "https://example.com/logo", MimeType: "image/svg+xml"},             // SVG by type
		{Src: "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="},                      // SVG data
		{Src: "https://cdn.other.net/logo.png", Sizes: iconSizes{"64x64"}},       // another site
		{Src: "http://example.com/logo.png", Sizes: iconSizes{"64x64"}},          // not https
		{Src: "https://user:pw@example.com/logo.png", Sizes: iconSizes{"64x64"}}, // credentials
		{Src: "https://example.com/dark.png", Sizes: iconSizes{"96x96"}, Theme: "dark"},
		{Src: "https://static.example.com/big.png", Sizes: iconSizes{"512x512"}, Theme: "light"},
		{Src: "https://mcp.example.com/64.png", Sizes: iconSizes{"64x64"}},
		{Src: data, Sizes: iconSizes{"32x32"}},
		{Src: "https://example.com/any.png", Sizes: iconSizes{"any"}},
	}
	got := iconCandidates("https://mcp.example.com/mcp", icons, false)
	want := []string{"https://mcp.example.com/64.png", "https://static.example.com/big.png", "https://example.com/any.png"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	// Without the light ones, the dark icon comes after a small one of any theme.
	got = iconCandidates("https://mcp.example.com/mcp", []ServerIcon{icons[6], icons[9]}, false)
	if !slices.Equal(got, []string{data, "https://example.com/dark.png"}) {
		t.Fatalf("got %v", got)
	}
	// Development instances also load icons from a loopback http server.
	got = iconCandidates("http://127.0.0.1:8080/mcp", []ServerIcon{{Src: "http://127.0.0.1:8080/icon.png"}, {Src: "http://127.0.0.2/icon.png"}}, true)
	if !slices.Equal(got, []string{"http://127.0.0.1:8080/icon.png"}) {
		t.Fatalf("dev got %v", got)
	}
	if got := iconCandidates("http://127.0.0.1:8080/mcp", []ServerIcon{{Src: "http://127.0.0.1:8080/icon.png"}}, false); len(got) != 0 {
		t.Fatalf("production loaded http: %v", got)
	}
}

func TestSiteDomain(t *testing.T) {
	cases := map[string]string{
		"https://mcp.example.com/mcp":     "example.com",
		"https://api.acme.co.uk/v1/mcp":   "acme.co.uk",
		"https://tools.my-app.vercel.app": "my-app.vercel.app",
		"http://127.0.0.1:8080/mcp":       "",
		"http://localhost:8080/mcp":       "",
	}
	for in, want := range cases {
		if got := siteDomain(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
	if !sameSite("mcp.example.com", "cdn.example.com") || sameSite("example.com", "example.net") || sameSite("a.vercel.app", "b.vercel.app") {
		t.Fatal("sameSite")
	}
}

func TestIconDownloadsAdvertisedAndDataIcons(t *testing.T) {
	big := pngOf(t, 96)
	var origin string
	mux := http.NewServeMux()
	mcp := &fakeMCP{bearer: "secret"}
	mux.Handle("/mcp", mcp)
	var sawAuth bool
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = sawAuth || r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	origin = srv.URL
	mcp.icons = []any{
		map[string]any{"src": origin + "/icon.png", "sizes": []string{"96x96"}},
		map[string]any{"src": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngOf(t, 64)), "sizes": []string{"64x64"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := newSession(http.DefaultClient, origin+"/mcp", "secret")
	if err := s.open(ctx); err != nil {
		t.Fatal(err)
	}
	srcs := iconCandidates(origin+"/mcp", s.Icons(), true)
	if len(srcs) != 2 {
		t.Fatalf("candidates = %v", srcs)
	}
	f := &images.Fetcher{Dev: true}
	for _, src := range srcs {
		p, err := f.Icon(ctx, src)
		if err != nil || len(p.Large) == 0 || len(p.Small) == 0 || images.Sniff(p.Large) != images.FormatPNG {
			t.Fatalf("%.40s: %v", src, err)
		}
	}
	if sawAuth {
		t.Fatal("the icon request carried credentials")
	}
	// Too small, and not a picture: refused.
	if _, err := f.Icon(ctx, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngOf(t, 16))); !images.IsMiss(err) {
		t.Fatalf("small icon: %v", err)
	}
	if _, err := f.Icon(ctx, "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="); !images.IsMiss(err) {
		t.Fatalf("svg icon: %v", err)
	}
}

package hd

import (
	"regexp"
	"strings"

	"github.com/aymerick/douceur/css"
	"github.com/aymerick/douceur/parser"
	"golang.org/x/net/html"
)

// Email CSS: designed mail (receipts, newsletters, notifications) is laid out with
// inline styles and <style> sheets. Both are kept, reduced to the layout and type
// properties below; values may only call the functions below, url() loads only
// http(s) and data: images, and at-rules other than @media are dropped. Clients
// render a styled body in a frame of its own, so its CSS never reaches the app.

var emailCSSProps = func() map[string]bool {
	props := map[string]bool{}
	for _, p := range strings.Fields(`
		background background-color background-image background-position background-repeat background-size
		border border-collapse border-color border-radius border-spacing border-style border-width
		border-top border-right border-bottom border-left
		border-top-color border-right-color border-bottom-color border-left-color
		border-top-style border-right-style border-bottom-style border-left-style
		border-top-width border-right-width border-bottom-width border-left-width
		border-top-left-radius border-top-right-radius border-bottom-left-radius border-bottom-right-radius
		box-shadow box-sizing caption-side clear color direction display empty-cells float
		font font-family font-size font-style font-variant font-weight
		height max-height min-height width max-width min-width
		letter-spacing line-height list-style list-style-position list-style-type
		margin margin-top margin-right margin-bottom margin-left
		padding padding-top padding-right padding-bottom padding-left
		opacity outline overflow overflow-x overflow-y overflow-wrap word-wrap word-break word-spacing
		table-layout text-align text-decoration text-decoration-color text-decoration-line text-decoration-style
		text-indent text-overflow text-size-adjust text-transform vertical-align visibility white-space
		align-items align-self justify-content flex flex-basis flex-direction flex-grow flex-shrink flex-wrap gap
		object-fit object-position`) {
		props[p] = true
	}
	return props
}()

var (
	cssVendorPrefix = regexp.MustCompile(`^(-webkit-|-moz-|-ms-|-o-|mso-)`)
	cssFunc         = regexp.MustCompile(`([a-z-]+)\s*\(`)
	cssURL          = regexp.MustCompile(`url\s*\(\s*(['"]?)([^'"()\s]*)(['"]?)\s*\)`)
	cssSafeURL      = regexp.MustCompile(`^(https?://[^\s"'<>()\\]+|data:image/(png|gif|jpe?g|webp);base64,[a-z0-9+/=]+)$`)
	cssMediaQuery   = regexp.MustCompile(`^[a-z0-9\s(),:.\-]+$`)
	cssSelector     = regexp.MustCompile(`^[^<>\\{}@;]+$`)
	cssFuncs        = map[string]bool{"rgb": true, "rgba": true, "hsl": true, "hsla": true, "calc": true, "min": true, "max": true, "clamp": true,
		"linear-gradient": true, "radial-gradient": true, "repeating-linear-gradient": true, "url": true}
)

// emailCSSProp reports a property email CSS keeps, vendor prefixes aside.
func emailCSSProp(prop string) bool {
	return emailCSSProps[cssVendorPrefix.ReplaceAllString(strings.ToLower(strings.TrimSpace(prop)), "")]
}

// emailCSSValue reports a declaration value email CSS keeps. Inline values arrive with
// their CSS escapes decoded, and empty when an escape does not decode, so an empty
// value is refused.
func emailCSSValue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || strings.ContainsAny(v, "\\<>{};") || strings.Contains(v, "javascript:") || strings.Contains(v, "/*") {
		return false
	}
	calls := 0
	for _, m := range cssFunc.FindAllStringSubmatch(v, -1) {
		if !cssFuncs[m[1]] {
			return false
		}
		if m[1] == "url" {
			calls++
		}
	}
	urls := cssURL.FindAllStringSubmatch(v, -1)
	if len(urls) != calls {
		return false
	}
	for _, u := range urls {
		if u[1] != u[3] || !cssSafeURL.MatchString(u[2]) {
			return false
		}
	}
	return true
}

// dropEscapedStyles removes style attributes that use CSS escapes. Values are checked
// decoded; stored CSS has no escapes, so every url() in it is plain to see, and clients
// that hold back remote images find them all.
func dropEscapedStyles(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			attrs := t.Attr[:0]
			for _, a := range t.Attr {
				if a.Key != "style" || !strings.Contains(a.Val, `\`) {
					attrs = append(attrs, a)
				}
			}
			t.Attr = attrs
			b.WriteString(t.String())
		default:
			b.Write(z.Raw())
		}
	}
}

// emailStyleSheets returns the cleaned <style> sheets of an email, joined.
func emailStyleSheets(raw string) string {
	if !strings.Contains(strings.ToLower(raw), "<style") {
		return ""
	}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(raw))
	inStyle := false
	for b.Len() < maxEmailCSS {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken:
			name, _ := z.TagName()
			inStyle = string(name) == "style"
		case html.TextToken:
			if inStyle {
				b.WriteString(cleanStyleSheet(string(z.Text())))
			}
		default:
			inStyle = false
		}
	}
	return b.String()
}

const maxEmailCSS = 256 << 10

// cleanStyleSheet rebuilds a style sheet from its allowed rules; anything it cannot
// parse is dropped.
func cleanStyleSheet(src string) string {
	sheet, err := parser.Parse(src)
	if err != nil {
		return ""
	}
	var b strings.Builder
	writeRules(&b, sheet.Rules)
	return b.String()
}

func writeRules(b *strings.Builder, rules []*css.Rule) {
	for _, r := range rules {
		switch {
		case r.Kind == css.QualifiedRule:
			sels := make([]string, 0, len(r.Selectors))
			for _, s := range r.Selectors {
				if s = strings.TrimSpace(s); s != "" && cssSelector.MatchString(s) {
					sels = append(sels, s)
				}
			}
			decls := cleanDeclarations(r.Declarations)
			if len(sels) == 0 || decls == "" {
				continue
			}
			b.WriteString(strings.Join(sels, ","))
			b.WriteString("{" + decls + "}")
		case strings.EqualFold(r.Name, "@media") && cssMediaQuery.MatchString(strings.ToLower(r.Prelude)):
			var inner strings.Builder
			writeRules(&inner, r.Rules)
			if inner.Len() > 0 {
				b.WriteString("@media " + strings.TrimSpace(r.Prelude) + "{" + inner.String() + "}")
			}
		}
	}
}

func cleanDeclarations(decls []*css.Declaration) string {
	var out []string
	for _, d := range decls {
		if !emailCSSProp(d.Property) || !emailCSSValue(d.Value) {
			continue
		}
		s := strings.ToLower(strings.TrimSpace(d.Property)) + ":" + strings.TrimSpace(d.Value)
		if d.Important {
			s += " !important"
		}
		out = append(out, s)
	}
	return strings.Join(out, ";")
}

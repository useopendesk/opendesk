package templates

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var skipContent = []string{"style", "script", "head", "title", "noscript", "iframe", "object", "embed", "applet", "svg", "math", "template"}

// cssProps are the inline style properties the design allowlist keeps.
var cssProps = []string{
	"color", "background-color", "background", "font", "font-family", "font-size", "font-weight", "font-style",
	"line-height", "letter-spacing", "text-align", "text-decoration", "text-transform", "vertical-align", "white-space",
	"width", "max-width", "min-width", "height", "max-height", "display",
	"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
	"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"border", "border-top", "border-right", "border-bottom", "border-left", "border-color", "border-width", "border-style",
	"border-radius", "border-collapse", "border-spacing", "opacity",
}

var cssValue = regexp.MustCompile(`^[a-z0-9#%.,()\s'"+\-/!_]*$`)

// safeCSS accepts plain values: colours, lengths, keywords and font names. Nothing that
// loads or runs anything.
func safeCSS(v string) bool {
	v = strings.ToLower(v)
	if !cssValue.MatchString(v) {
		return false
	}
	for _, bad := range []string{"url(", "expression", "javascript", "/*", "image(", "var(", "attr(", "behavior", "binding"} {
		if strings.Contains(v, bad) {
			return false
		}
	}
	return true
}

var (
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$`)
	dimension = regexp.MustCompile(`^\d{1,4}(%|px)?$`)
	alignment = regexp.MustCompile(`(?i)^(left|right|center|justify)$`)
	valign    = regexp.MustCompile(`(?i)^(top|middle|bottom|baseline)$`)
	smallInt  = regexp.MustCompile(`^\d{1,2}$`)
)

// newDesignPolicy is the allowlist of signatures, auto-replies, footers, layouts and CSAT
// emails: email HTML with tables, inline styles of safe properties and the attributes
// email layouts use. relative keeps relative URLs, which is how template tags in links
// survive saving (protect); the policy emails go out with drops them.
func newDesignPolicy(relative bool) *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.SkipElementsContent(skipContent...)
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowRelativeURLs(relative)
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowDataURIImages()
	p.AllowElements("center", "font")
	p.AllowStyles(cssProps...).MatchingHandler(safeCSS).Globally()
	p.AllowAttrs("align").Matching(alignment).OnElements("p", "div", "td", "th", "table", "img", "tr", "h1", "h2", "h3", "h4", "center")
	p.AllowAttrs("valign").Matching(valign).OnElements("td", "th", "tr")
	p.AllowAttrs("width", "height").Matching(dimension).OnElements("img", "table", "td", "th", "col")
	p.AllowAttrs("bgcolor").Matching(hexColor).OnElements("table", "td", "th", "tr", "body", "div")
	p.AllowAttrs("color").Matching(hexColor).OnElements("font")
	p.AllowAttrs("cellpadding", "cellspacing", "border").Matching(smallInt).OnElements("table", "img")
	p.AllowAttrs("role").Matching(regexp.MustCompile(`^presentation$`)).OnElements("table")
	return p
}

var (
	designSave = newDesignPolicy(true)
	designSend = newDesignPolicy(false)
	textPolicy = bluemonday.StrictPolicy().SkipElementsContent(skipContent...)
	blockBreak = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6]|/blockquote|/pre|/table)\s*/?>`)
	placeRe    = regexp.MustCompile(`zqtpl(\d+)qz`)
)

// Protect runs sanitize over src with its template tags kept: each tag becomes a
// placeholder that is valid text, CSS value and URL, and comes back after. Tags go back
// as typed in text and escaped inside attributes.
func Protect(src string, sanitize func(string) string) string {
	if !HasTags(src) {
		return sanitize(src)
	}
	var tags []string
	var b strings.Builder
	for _, tk := range lex(src) {
		if tk.kind == 't' {
			b.WriteString(strings.ReplaceAll(tk.raw, "zqtpl", "zq tpl"))
			continue
		}
		b.WriteString("zqtpl" + strconv.Itoa(len(tags)) + "qz")
		tags = append(tags, tk.raw)
	}
	clean := sanitize(b.String())
	var out strings.Builder
	inTag := false
	last := 0
	for _, m := range placeRe.FindAllStringSubmatchIndex(clean, -1) {
		seg := clean[last:m[0]]
		for _, c := range seg {
			switch c {
			case '<':
				inTag = true
			case '>':
				inTag = false
			}
		}
		out.WriteString(seg)
		n, _ := strconv.Atoi(clean[m[2]:m[3]])
		if n < len(tags) {
			tag := html.UnescapeString(tags[n])
			if inTag {
				tag = html.EscapeString(tag)
			} else {
				tag = strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(tag)
			}
			out.WriteString(tag)
		}
		last = m[1]
	}
	out.WriteString(clean[last:])
	return out.String()
}

// SanitizeDesign cleans a signature, auto-reply, footer, layout or CSAT email for storage,
// keeping its template tags.
func SanitizeDesign(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return strings.TrimSpace(Protect(raw, designSave.Sanitize))
}

// SanitizeForSend cleans filled design HTML before it goes out: no relative URLs left.
func SanitizeForSend(filled string) string {
	return strings.TrimSpace(designSend.Sanitize(filled))
}

// TextOf flattens HTML to text with line structure kept.
func TextOf(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	withBreaks := blockBreak.ReplaceAllString(raw, "\n$0")
	lines := strings.Split(html.UnescapeString(textPolicy.Sanitize(withBreaks)), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, ln := range lines {
		ln = strings.Join(strings.Fields(strings.ReplaceAll(ln, " ", " ")), " ")
		if ln == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ValidColor reports a #rrggbb colour.
func ValidColor(s string) bool { return len(s) == 7 && hexColor.MatchString(s) }

package email

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
)

// sanitizeHTML is the only way email HTML reaches the database (hd.SanitizeEmailHTML).
func sanitizeHTML(s string) string { return hd.SanitizeEmailHTML(s) }

// sanitizeSettingsHTML cleans signatures and auto-replies written by admins.
func sanitizeSettingsHTML(s string) string { return hd.SanitizeSignature(s) }

// htmlToText renders HTML as plain text: block elements become line breaks, list
// items get a dash, whitespace collapses outside <pre>.
func htmlToText(s string) string {
	doc := parseFragment(s)
	if doc == nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *xhtml.Node, pre bool)
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	walk = func(n *xhtml.Node, pre bool) {
		switch n.Type {
		case xhtml.TextNode:
			t := n.Data
			if !pre {
				t = strings.Join(strings.Fields(strings.ReplaceAll(t, "\u00a0", " ")), " ")
				if t == "" {
					if strings.TrimSpace(n.Data) == "" && len(n.Data) > 0 && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") && !strings.HasSuffix(b.String(), " ") {
						b.WriteByte(' ')
					}
					return
				}
				if (unicode.IsSpace(rune(n.Data[0])) || strings.HasPrefix(n.Data, "\u00a0")) && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") && !strings.HasSuffix(b.String(), " ") {
					b.WriteByte(' ')
				}
				b.WriteString(t)
				if last := n.Data[len(n.Data)-1]; last == ' ' || last == '\n' || last == '\t' {
					b.WriteByte(' ')
				}
				return
			}
			b.WriteString(t)
			return
		case xhtml.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Head, atom.Title:
				return
			case atom.Br:
				b.WriteByte('\n')
				return
			case atom.Img:
				if alt := attr(n, "alt"); alt != "" {
					b.WriteString("[" + alt + "]")
				}
				return
			}
		}
		block := n.Type == xhtml.ElementNode && blockAtoms[n.DataAtom]
		if block {
			newline()
			if n.DataAtom == atom.Li {
				b.WriteString("- ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, pre || n.DataAtom == atom.Pre)
		}
		if block {
			newline()
			if n.DataAtom == atom.P || n.DataAtom == atom.Blockquote {
				b.WriteByte('\n')
			}
		}
	}
	walk(doc, false)
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.TrimLeft(l, " "), " \t")
	}
	return strings.TrimSpace(collapseBlankLines(strings.Join(lines, "\n")))
}

var blockAtoms = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Li: true, atom.Ul: true, atom.Ol: true, atom.Tr: true, atom.Table: true,
	atom.Blockquote: true, atom.Pre: true, atom.Hr: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.H5: true, atom.H6: true, atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Center: true, atom.Address: true, atom.Dl: true, atom.Dt: true, atom.Dd: true, atom.Figure: true,
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return blankRuns.ReplaceAllString(s, "\n\n")
}

// textToHTML turns a plain-text body into simple paragraphs.
func textToHTML(t string) string {
	t = strings.TrimSpace(strings.ReplaceAll(t, "\r\n", "\n"))
	if t == "" {
		return ""
	}
	var b strings.Builder
	for _, para := range strings.Split(t, "\n\n") {
		para = strings.Trim(para, "\n")
		if para == "" {
			continue
		}
		b.WriteString("<p>")
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				b.WriteString("<br>")
			}
			b.WriteString(html.EscapeString(line))
		}
		b.WriteString("</p>")
	}
	return b.String()
}

// parseFragment parses an email body (often a full document) into a tree.
func parseFragment(s string) *xhtml.Node {
	doc, err := xhtml.Parse(strings.NewReader(s))
	if err != nil {
		return nil
	}
	return doc
}

func render(n *xhtml.Node) string {
	body := findFirst(n, func(x *xhtml.Node) bool { return x.Type == xhtml.ElementNode && x.DataAtom == atom.Body })
	var buf bytes.Buffer
	if body == nil {
		_ = xhtml.Render(&buf, n)
		return buf.String()
	}
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		_ = xhtml.Render(&buf, c)
	}
	return buf.String()
}

func findFirst(n *xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
	if n == nil {
		return nil
	}
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findFirst(c, match); f != nil {
			return f
		}
	}
	return nil
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *xhtml.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if strings.EqualFold(c, class) {
			return true
		}
	}
	return false
}

// rewriteCID points <img src="cid:..."> at stored attachments. urls maps a bare
// content id (no angle brackets, lower case) to its attachment URL.
func rewriteCID(doc *xhtml.Node, urls map[string]string) (used map[string]bool) {
	used = map[string]bool{}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Img {
			for i, a := range n.Attr {
				if !strings.EqualFold(a.Key, "src") {
					continue
				}
				v := strings.TrimSpace(a.Val)
				if len(v) > 4 && strings.EqualFold(v[:4], "cid:") {
					cid := normalizeCID(v[4:])
					if u, ok := urls[cid]; ok {
						n.Attr[i].Val = u
						used[cid] = true
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return used
}

func normalizeCID(s string) string {
	s = strings.TrimSpace(s)
	if u, err := urlUnescape(s); err == nil {
		s = u
	}
	return strings.ToLower(strings.Trim(s, "<>"))
}

func urlUnescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, lo := unhex(s[i+1]), unhex(s[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}

func unhex(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	}
	return -1
}

package mailer

import (
	"html/template"
	"strings"
	"unicode"
)

// Email is one platform email before rendering: every opendesk email goes through Render,
// so they share one look. All text is plain and escaped on output; URLs must be ones the
// server built (only http and https links are kept).
type Email struct {
	// Subject is also the HTML document title.
	Subject string
	// Preheader is the preview line mail clients show after the subject.
	Preheader string
	// Eyebrow is a small label above the heading, such as the workspace name.
	Eyebrow string
	Heading string
	Intro   []string
	Ticket  *TicketCard
	Facts   []Fact
	// Summary is a short line above the list, such as "Since 14:05: 2 assigned, 1 mention".
	Summary string
	// Sections are lists of notifications under a small title; Items is one untitled list.
	Sections []Section
	Items    []Item
	// More follows the items, such as "And 3 more in opendesk".
	More   string
	Button *Button
	// Outro paragraphs follow the button in a smaller type.
	Outro  []string
	Footer Footer
}

// TicketCard shows the ticket an email is about.
type TicketCard struct {
	Ref     string // "#1095"
	Subject string
	Meta    string // "Dana Lee"
	Snippet string
}

// Fact is a label and value row in a light box.
type Fact struct{ Label, Value string }

// Section is a titled list of notifications. Urgent sections lead and their title is red.
type Section struct {
	Title  string
	Urgent bool
	Items  []Item
}

// Item is one entry of a list of notifications.
type Item struct {
	Title   string // "Maya Chen assigned you"
	Ref     string // "#1095"
	Subject string
	Snippet string
	Time    string
	URL     string
	// Tone is the dot's colour, a hex value such as "#3B82F6".
	Tone string
}

// Button is the one call to action.
type Button struct{ Label, URL string }

// Link is a footer link.
type Link struct{ Label, URL string }

// Footer says why the email came and how to change that.
type Footer struct {
	Reason  string
	Links   []Link
	Contact string // an address for questions, shown when set
	// Legal identifies the company that sends the email, where the law asks for it; empty
	// leaves the line out.
	Legal string
}

// Render returns the HTML and plain-text bodies of e.
func Render(e Email) (html, text string) {
	if len(e.Items) > 0 {
		e.Sections = append([]Section{{Items: e.Items}}, e.Sections...)
		e.Items = nil
	}
	var b strings.Builder
	if err := layoutTmpl.Execute(&b, e); err != nil {
		// The template is fixed and its data plain values; this does not happen.
		panic(err)
	}
	return b.String(), renderText(e)
}

// SafeURL keeps http and https URLs and drops anything else.
func SafeURL(u string) string {
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		return u
	}
	return ""
}

// HeaderSafe flattens s for a header: control characters become spaces.
func HeaderSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func renderText(e Email) string {
	var t strings.Builder
	line := func(s string) { t.WriteString(s + "\n") }
	if e.Eyebrow != "" {
		line(e.Eyebrow)
	}
	line(e.Heading)
	line("")
	for _, p := range e.Intro {
		line(p)
		line("")
	}
	if c := e.Ticket; c != nil {
		line(strings.TrimSpace(c.Ref + " " + c.Subject))
		if c.Meta != "" {
			line(c.Meta)
		}
		if c.Snippet != "" {
			line("> " + c.Snippet)
		}
		line("")
	}
	for _, f := range e.Facts {
		line(f.Label + ": " + f.Value)
	}
	if len(e.Facts) > 0 {
		line("")
	}
	if e.Summary != "" {
		line(e.Summary)
		line("")
	}
	for _, sec := range e.Sections {
		if sec.Title != "" {
			line(strings.ToUpper(sec.Title))
			line("")
		}
		renderItems(line, sec.Items)
	}
	if e.More != "" {
		line(e.More)
		line("")
	}
	if e.Button != nil {
		if u := SafeURL(e.Button.URL); u != "" {
			line(e.Button.Label + ": " + u)
			line("")
		}
	}
	for _, p := range e.Outro {
		line(p)
		line("")
	}
	line("--")
	if e.Footer.Reason != "" {
		line(e.Footer.Reason)
	}
	for _, l := range e.Footer.Links {
		if u := SafeURL(l.URL); u != "" {
			line(l.Label + ": " + u)
		}
	}
	if e.Footer.Contact != "" {
		line("Questions? Write to " + e.Footer.Contact + ".")
	}
	if e.Footer.Legal != "" {
		line(e.Footer.Legal)
	}
	line("opendesk")
	return t.String()
}

func renderItems(line func(string), items []Item) {
	for _, it := range items {
		head := "- " + it.Title
		if it.Time != "" {
			head += " (" + it.Time + ")"
		}
		line(head)
		if ref := strings.TrimSpace(it.Ref + " " + it.Subject); ref != "" {
			line("  " + ref)
		}
		if it.Snippet != "" {
			line("  > " + it.Snippet)
		}
		if u := SafeURL(it.URL); u != "" {
			line("  " + u)
		}
		line("")
	}
}

var msoParts = map[string]string{
	"head":  `<!--[if mso]><noscript><xml><o:OfficeDocumentSettings><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript><![endif]-->`,
	"open":  `<!--[if mso]><table role="presentation" width="560" align="center" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`,
	"close": `<!--[if mso]></td></tr></table><![endif]-->`,
}

const fontStack = `Geist,-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif`
const monoStack = `'Geist Mono',ui-monospace,SFMono-Regular,Menlo,Consolas,monospace`

var layoutTmpl = template.Must(template.New("layout").Funcs(template.FuncMap{
	"url":   func(u string) template.URL { return template.URL(SafeURL(u)) },
	"font":  func() template.CSS { return template.CSS("font-family:" + fontStack) },
	"mono":  func() template.CSS { return template.CSS("font-family:" + monoStack) },
	"last":  func(i int, list []Item) bool { return i == len(list)-1 },
	"last2": func(i int, list []Section) bool { return i == len(list)-1 },
	// Outlook's conditional comments; html/template drops comments written in the template.
	"mso": func(part string) template.HTML { return template.HTML(msoParts[part]) },
	"tone": func(s string) template.CSS {
		if len(s) == 7 && s[0] == '#' {
			return template.CSS("background:" + s)
		}
		return template.CSS("background:#8A8A8A")
	},
}).Parse(`<!doctype html>
<html lang="en" xmlns="http://www.w3.org/1999/xhtml" xmlns:o="urn:schemas-microsoft-com:office:office">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="x-apple-disable-message-reformatting">
<meta name="format-detection" content="telephone=no,address=no,email=no,date=no">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>{{.Subject}}</title>
{{mso "head"}}
<style>
:root{color-scheme:light dark;supported-color-schemes:light dark}
body{margin:0;padding:0;width:100%;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%}
table{border-collapse:collapse}
a{text-decoration:none}
.od-link{text-decoration:underline !important}
@media (max-width:620px){
.od-outer{padding:20px 12px 28px !important}
.od-card{padding:28px 22px 26px !important}
.od-h1{font-size:21px !important}
.od-time{display:none !important}
}
@media (prefers-color-scheme:dark){
.od-bg{background:#0A0A0A !important}
.od-card{background:#171717 !important;border-color:#262626 !important}
.od-ink{color:#F4F4F4 !important}
.od-body{color:#C7C7C7 !important}
.od-muted{color:#8A8A8A !important}
.od-box{background:#1F1F1F !important;border-color:#2E2E2E !important}
.od-rule{border-color:#262626 !important}
.od-btn{background:#F4F4F4 !important}
.od-btn a{color:#0A0A0A !important}
.od-mark{background:#F4F4F4 !important}
.od-quote{border-color:#3A3A3A !important}
.od-urgent{color:#F87171 !important}
}
[data-ogsc] .od-bg{background:#0A0A0A !important}
[data-ogsc] .od-card{background:#171717 !important;border-color:#262626 !important}
[data-ogsc] .od-ink{color:#F4F4F4 !important}
[data-ogsc] .od-body{color:#C7C7C7 !important}
[data-ogsc] .od-box{background:#1F1F1F !important;border-color:#2E2E2E !important}
[data-ogsc] .od-btn{background:#F4F4F4 !important}
[data-ogsc] .od-btn a{color:#0A0A0A !important}
[data-ogsc] .od-mark{background:#F4F4F4 !important}
</style>
</head>
<body class="od-bg" style="margin:0;padding:0;background:#F4F4F4;{{font}}">
{{if .Preheader}}<div style="display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all">{{.Preheader}}&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;</div>{{end}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="od-bg" bgcolor="#F4F4F4" style="background:#F4F4F4">
<tr><td align="center" class="od-outer" style="padding:36px 16px 40px">
{{mso "open"}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;margin:0 auto">
<tr><td style="padding:0 4px 18px">
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td valign="middle" style="padding:0 9px 0 0">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="border-collapse:separate"><tr>
<td class="od-mark" width="9" height="9" bgcolor="#0A0A0A" style="width:9px;height:9px;background:#0A0A0A;border-radius:2px;font-size:0;line-height:0">&nbsp;</td>
<td width="2" style="width:2px;font-size:0;line-height:0">&nbsp;</td>
<td class="od-mark" width="9" height="9" bgcolor="#0A0A0A" style="width:9px;height:9px;background:#0A0A0A;border-radius:50%;font-size:0;line-height:0">&nbsp;</td>
</tr><tr><td colspan="3" height="2" style="height:2px;font-size:0;line-height:0">&nbsp;</td></tr><tr>
<td class="od-mark" width="9" height="9" bgcolor="#0A0A0A" style="width:9px;height:9px;background:#0A0A0A;border-radius:2px;font-size:0;line-height:0">&nbsp;</td>
<td width="2" style="width:2px;font-size:0;line-height:0">&nbsp;</td>
<td class="od-mark" width="9" height="9" bgcolor="#0A0A0A" style="width:9px;height:9px;background:#0A0A0A;border-radius:2px;font-size:0;line-height:0">&nbsp;</td>
</tr></table>
</td>
<td valign="middle" class="od-ink" style="{{font}};font-size:16px;line-height:18px;font-weight:600;letter-spacing:-0.03em;color:#0A0A0A">opendesk</td>
</tr></table>
</td></tr>
<tr><td class="od-card" bgcolor="#FFFFFF" style="background:#FFFFFF;border:1px solid #E4E4E4;border-radius:10px;padding:36px 36px 32px">
{{if .Eyebrow}}<p class="od-muted" style="margin:0 0 10px;{{mono}};font-size:11px;line-height:16px;letter-spacing:0.06em;text-transform:uppercase;color:#8A8A8A">{{.Eyebrow}}</p>{{end}}
<h1 class="od-ink od-h1" style="margin:0 0 14px;{{font}};font-size:22px;line-height:30px;font-weight:600;letter-spacing:-0.02em;color:#0A0A0A">{{.Heading}}</h1>
{{range .Intro}}<p class="od-body" style="margin:0 0 14px;{{font}};font-size:15px;line-height:24px;color:#404040">{{.}}</p>
{{end}}
{{with .Ticket}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:6px 0 20px;border-collapse:separate">
<tr><td class="od-box" bgcolor="#FAFAFA" style="background:#FAFAFA;border:1px solid #E4E4E4;border-radius:8px;padding:16px 18px">
<p class="od-ink" style="margin:0;{{font}};font-size:15px;line-height:22px;font-weight:600;color:#0A0A0A">{{if .Ref}}<span class="od-muted" style="{{mono}};font-size:13px;font-weight:500;color:#8A8A8A">{{.Ref}}</span>&nbsp;&nbsp;{{end}}{{.Subject}}</p>
{{if .Meta}}<p class="od-muted" style="margin:4px 0 0;{{font}};font-size:13px;line-height:20px;color:#8A8A8A">{{.Meta}}</p>{{end}}
{{if .Snippet}}<p class="od-body od-quote" style="margin:12px 0 0;padding:0 0 0 12px;border-left:2px solid #E4E4E4;{{font}};font-size:14px;line-height:22px;color:#525252">{{.Snippet}}</p>{{end}}
</td></tr></table>
{{end}}
{{if .Facts}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:6px 0 20px;border-collapse:separate">
<tr><td class="od-box" bgcolor="#FAFAFA" style="background:#FAFAFA;border:1px solid #E4E4E4;border-radius:8px;padding:6px 18px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
{{range $i, $f := .Facts}}<tr>
<td class="od-muted{{if $i}} od-rule{{end}}" style="padding:10px 12px 10px 0;{{if $i}}border-top:1px solid #ECECEC;{{end}}{{font}};font-size:13px;line-height:20px;color:#8A8A8A;white-space:nowrap" valign="top">{{$f.Label}}</td>
<td class="od-ink{{if $i}} od-rule{{end}}" align="right" style="padding:10px 0;{{if $i}}border-top:1px solid #ECECEC;{{end}}{{font}};font-size:14px;line-height:20px;font-weight:500;color:#0A0A0A" valign="top">{{$f.Value}}</td>
</tr>{{end}}
</table>
</td></tr></table>
{{end}}
{{if .Summary}}<p class="od-muted" style="margin:2px 0 6px;{{font}};font-size:13px;line-height:20px;color:#737373">{{.Summary}}</p>
{{end}}
{{range $si, $sec := .Sections}}{{if $sec.Title}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:{{if $si}}18px{{else}}14px{{end}} 0 0"><tr>
<td class="{{if $sec.Urgent}}od-urgent{{else}}od-muted{{end}}" style="padding:0 0 2px;{{mono}};font-size:11px;line-height:16px;letter-spacing:0.06em;text-transform:uppercase;color:{{if $sec.Urgent}}#DC2626{{else}}#8A8A8A{{end}}">{{$sec.Title}}</td>
</tr></table>
{{end}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:{{if $sec.Title}}0{{else}}4px{{end}} 0 {{if last2 $si $.Sections}}14px{{else}}0{{end}}">
{{range $i, $it := $sec.Items}}<tr>
<td width="18" valign="top" class="od-rule" style="width:18px;padding:14px 0 {{if last $i $sec.Items}}6px{{else}}14px{{end}};{{if $i}}border-top:1px solid #ECECEC;{{end}}">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="border-collapse:separate;margin-top:7px"><tr><td width="8" height="8" style="width:8px;height:8px;{{tone $it.Tone}};border-radius:4px;font-size:0;line-height:0">&nbsp;</td></tr></table>
</td>
<td valign="top" class="od-rule" style="padding:14px 0 {{if last $i $sec.Items}}6px{{else}}14px{{end}};{{if $i}}border-top:1px solid #ECECEC;{{end}}">
<p style="margin:0;{{font}};font-size:15px;line-height:22px;font-weight:600"><a class="od-ink" href="{{url $it.URL}}" style="color:#0A0A0A;text-decoration:none">{{$it.Title}}</a></p>
{{if or $it.Ref $it.Subject}}<p class="od-body" style="margin:2px 0 0;{{font}};font-size:14px;line-height:21px;color:#404040">{{if $it.Ref}}<span class="od-muted" style="{{mono}};font-size:12px;color:#8A8A8A">{{$it.Ref}}</span>&nbsp;&nbsp;{{end}}{{$it.Subject}}</p>{{end}}
{{if $it.Snippet}}<p class="od-muted od-quote" style="margin:8px 0 0;padding:0 0 0 10px;border-left:2px solid #E4E4E4;{{font}};font-size:13px;line-height:20px;color:#737373">{{$it.Snippet}}</p>{{end}}
</td>
<td valign="top" align="right" class="od-muted od-time od-rule" style="padding:16px 0 0 12px;{{if $i}}border-top:1px solid #ECECEC;{{end}}{{font}};font-size:12px;line-height:20px;color:#8A8A8A;white-space:nowrap">{{$it.Time}}</td>
</tr>{{end}}
</table>
{{end}}
{{if .More}}<p class="od-muted" style="margin:4px 0 18px;{{font}};font-size:13px;line-height:20px;color:#8A8A8A">{{.More}}</p>{{end}}
{{with .Button}}{{if url .URL}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:10px 0 6px;border-collapse:separate">
<tr><td class="od-btn" bgcolor="#0A0A0A" style="background:#0A0A0A;border-radius:7px;padding:11px 20px;mso-padding-alt:11px 20px">
<a href="{{url .URL}}" style="display:inline-block;{{font}};font-size:14px;line-height:20px;font-weight:600;color:#FFFFFF;text-decoration:none">{{.Label}}</a>
</td></tr></table>{{end}}{{end}}
{{range .Outro}}<p class="od-muted" style="margin:18px 0 0;{{font}};font-size:13px;line-height:20px;color:#8A8A8A">{{.}}</p>
{{end}}
</td></tr>
<tr><td style="padding:22px 6px 0">
{{if .Footer.Reason}}<p class="od-muted" style="margin:0 0 8px;{{font}};font-size:12px;line-height:19px;color:#8A8A8A">{{.Footer.Reason}}</p>{{end}}
{{if .Footer.Links}}<p class="od-muted" style="margin:0 0 8px;{{font}};font-size:12px;line-height:19px;color:#8A8A8A">{{range $i, $l := .Footer.Links}}{{if $i}}&nbsp;&nbsp;&middot;&nbsp;&nbsp;{{end}}<a class="od-muted od-link" href="{{url $l.URL}}" style="color:#8A8A8A;text-decoration:underline">{{$l.Label}}</a>{{end}}</p>{{end}}
{{if .Footer.Contact}}<p class="od-muted" style="margin:0 0 8px;{{font}};font-size:12px;line-height:19px;color:#8A8A8A">Questions? Write to <a class="od-muted od-link" href="mailto:{{.Footer.Contact}}" style="color:#8A8A8A;text-decoration:underline">{{.Footer.Contact}}</a>.</p>{{end}}
{{if .Footer.Legal}}<p class="od-muted" style="margin:0 0 8px;{{font}};font-size:11px;line-height:17px;color:#A3A3A3">{{.Footer.Legal}}</p>{{end}}
<p class="od-muted" style="margin:14px 0 0;{{font}};font-size:12px;line-height:19px;font-weight:600;letter-spacing:-0.02em;color:#A3A3A3">opendesk</p>
</td></tr>
</table>
{{mso "close"}}
</td></tr></table>
</body>
</html>
`))

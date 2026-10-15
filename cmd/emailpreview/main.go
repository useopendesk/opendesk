// Command emailpreview renders every email opendesk sends with sample data, one .html and
// one .txt per email, plus an index.html gallery:
//
//	go run ./cmd/emailpreview -out /tmp/opendesk-emails
//
// It needs no database, server or configuration and sends nothing.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/useopendesk/opendesk/internal/emails"
)

func main() {
	out := flag.String("out", "opendesk-emails", "directory to write the previews to")
	app := flag.String("app-url", "https://app.opendesk.sh", "APP_URL the links point to")
	api := flag.String("api-url", "https://api.opendesk.sh", "API_URL the unsubscribe header points to")
	cloud := flag.Bool("cloud", true, "render as opendesk Cloud (footers name the contact address)")
	legal := flag.String("legal", "", "the company line under the footer, as EMAIL_LEGAL_* renders it")
	flag.Parse()

	site := emails.Site{AppURL: *app, APIURL: *api, Legal: *legal}
	if *cloud {
		site.Contact = emails.Contact
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	now := time.Date(2026, 10, 13, 9, 41, 0, 0, time.UTC)
	previews := emails.Previews(site, now)
	type card struct {
		Name, Title, Subject string
		Headers              map[string]string
	}
	var cards []card
	for _, p := range previews {
		if err := os.WriteFile(filepath.Join(*out, p.Name+".html"), []byte(p.Mail.HTML), 0o644); err != nil {
			fail(err)
		}
		if err := os.WriteFile(filepath.Join(*out, p.Name+".txt"), []byte("Subject: "+p.Mail.Subject+"\n\n"+p.Mail.Text), 0o644); err != nil {
			fail(err)
		}
		cards = append(cards, card{Name: p.Name, Title: p.Title, Subject: p.Mail.Subject, Headers: p.Mail.Headers})
	}
	f, err := os.Create(filepath.Join(*out, "index.html"))
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := indexTmpl.Execute(f, cards); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %d emails to %s\n", len(previews), filepath.Join(*out, "index.html"))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "emailpreview:", err)
	os.Exit(1)
}

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>opendesk emails</title>
<style>
:root{color-scheme:light dark}
body{margin:0;padding:32px;background:#F4F4F4;color:#0A0A0A;font:14px/1.5 Geist,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
h1{margin:0 0 4px;font-size:22px;letter-spacing:-0.02em}
p.lead{margin:0 0 24px;color:#525252}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(420px,1fr));gap:20px}
.card{background:#FFF;border:1px solid #E4E4E4;border-radius:10px;overflow:hidden}
.card header{padding:12px 14px;border-bottom:1px solid #E4E4E4}
.card h2{margin:0;font-size:14px}
.card .subj{color:#525252;font-size:13px}
.card .meta{color:#8A8A8A;font:12px/1.5 'Geist Mono',ui-monospace,monospace;word-break:break-all}
.card a{color:inherit}
iframe{display:block;width:100%;height:560px;border:0;background:#F4F4F4}
@media (prefers-color-scheme:dark){body{background:#0A0A0A;color:#F4F4F4}.card{background:#171717;border-color:#262626}.card header{border-color:#262626}p.lead,.card .subj{color:#A3A3A3}}
</style></head><body>
<h1>opendesk emails</h1>
<p class="lead">Every email opendesk sends, rendered with sample data. Open one to see it full size; the .txt next to it is the plain-text part.</p>
<div class="grid">
{{range .}}<section class="card"><header><h2><a href="{{.Name}}.html">{{.Title}}</a></h2><div class="subj">{{.Subject}}</div>
{{range $k, $v := .Headers}}<div class="meta">{{$k}}: {{$v}}</div>{{end}}<div class="meta"><a href="{{.Name}}.txt">plain text</a></div></header>
<iframe src="{{.Name}}.html" loading="lazy" title="{{.Title}}"></iframe></section>
{{end}}</div></body></html>
`))

package templates

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
)

// LayoutOptions are a brand's email layout (contracts/templates.md "Email design").
type LayoutOptions struct {
	Mode          string `json:"mode" enum:"simple,custom" required:"false"`
	Style         string `json:"style" enum:"plain,branded" required:"false"`
	Logo          bool   `json:"logo" required:"false"`
	Accent        string `json:"accent" required:"false" doc:"#rrggbb, or empty for the brand colour"`
	Font          string `json:"font" enum:"system,sans,serif" required:"false"`
	FooterHTML    string `json:"footer_html" maxLength:"16384" required:"false"`
	History       string `json:"history" enum:"none,last,all" required:"false"`
	HistoryHeader string `json:"history_header" maxLength:"200" required:"false"`
}

// DefaultLayout is the layout of a brand that has not changed it: reads like a personal
// email and quotes the previous message.
func DefaultLayout() LayoutOptions {
	return LayoutOptions{Mode: "simple", Style: "plain", Font: "system", History: "last"}
}

func (o LayoutOptions) normalized() LayoutOptions {
	d := DefaultLayout()
	if o.Mode != "custom" {
		o.Mode = d.Mode
	}
	if o.Style != "branded" {
		o.Style = d.Style
	}
	if _, ok := fonts[o.Font]; !ok {
		o.Font = d.Font
	}
	switch o.History {
	case "none", "last", "all":
	default:
		o.History = d.History
	}
	if !ValidColor(o.Accent) {
		o.Accent = ""
	}
	return o
}

// Design is a brand's email design: its layout and its CSAT email.
type Design struct {
	Layout      LayoutOptions
	CustomHTML  string
	CSATSubject string
	CSATHTML    string
}

// DefaultCSATSubject and DefaultCSATHTML are the satisfaction email of a brand that has
// not changed it.
const (
	DefaultCSATSubject = "How did we do?"
	DefaultCSATHTML    = `<p>Hi {{ contact.first_name | default: "there" }},</p>` +
		`<p>Your request #{{ ticket.number }} "{{ ticket.subject }}" was marked solved. How did {{ team.name }} do? Rate it from 1 (very poor) to 5 (great).</p>` +
		`{{ survey }}` +
		`<p style="font-size: 12px; color: #8A8A8A">One click records your answer. You can add a comment on the next page.</p>`
	// DefaultCustomLayout is where a custom layout starts.
	DefaultCustomLayout = `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color: #F4F4F5">` +
		`<tr><td align="center" style="padding: 28px 12px">` +
		`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width: 600px; background-color: #FFFFFF; border-radius: 10px">` +
		`<tr><td style="padding: 24px 32px">{{ content }}{{ signature }}{{ history }}</td></tr></table>` +
		`<p style="font-size: 12px; color: #8A8A8A">{{ brand.name }}</p>` +
		`</td></tr></table>`
)

// DefaultDesign is the design of a brand without saved templates.
func DefaultDesign() Design {
	return Design{Layout: DefaultLayout(), CSATSubject: DefaultCSATSubject, CSATHTML: DefaultCSATHTML}
}

// LoadDesign reads a brand's saved templates over the defaults.
func LoadDesign(ctx context.Context, q db.Querier, wid, brandID uuid.UUID) (Design, error) {
	d := DefaultDesign()
	rows, err := q.Query(ctx, `SELECT kind, subject, body_html, options FROM email_templates WHERE workspace_id = $1 AND brand_id = $2`, wid, brandID)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, subject, body string
		var opts []byte
		if err := rows.Scan(&kind, &subject, &body, &opts); err != nil {
			return d, err
		}
		switch kind {
		case KindLayout:
			o := DefaultLayout()
			_ = json.Unmarshal(opts, &o)
			d.Layout, d.CustomHTML = o.normalized(), body
		case KindCSAT:
			if strings.TrimSpace(subject) != "" {
				d.CSATSubject = subject
			}
			if strings.TrimSpace(body) != "" {
				d.CSATHTML = body
			}
		}
	}
	return d, rows.Err()
}

// EmailTemplate is a brand's template as the API shows it.
type EmailTemplate struct {
	BrandID   uuid.UUID     `json:"brand_id"`
	Kind      string        `json:"kind" enum:"layout,csat"`
	Subject   string        `json:"subject"`
	BodyHTML  string        `json:"body_html"`
	Options   LayoutOptions `json:"options"`
	IsDefault bool          `json:"is_default"`
	UpdatedAt *time.Time    `json:"updated_at"`
}

// LoadTemplates returns a brand's layout and CSAT templates, the defaults filled in.
func LoadTemplates(ctx context.Context, q db.Querier, wid, brandID uuid.UUID) (layout, csat EmailTemplate, err error) {
	layout = EmailTemplate{BrandID: brandID, Kind: KindLayout, BodyHTML: DefaultCustomLayout, Options: DefaultLayout(), IsDefault: true}
	csat = EmailTemplate{BrandID: brandID, Kind: KindCSAT, Subject: DefaultCSATSubject, BodyHTML: DefaultCSATHTML, Options: DefaultLayout(), IsDefault: true}
	rows, err := q.Query(ctx, `SELECT kind, subject, body_html, options, updated_at FROM email_templates WHERE workspace_id = $1 AND brand_id = $2`, wid, brandID)
	if err != nil {
		return layout, csat, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EmailTemplate, error) {
		var t EmailTemplate
		var opts []byte
		var at time.Time
		err := r.Scan(&t.Kind, &t.Subject, &t.BodyHTML, &opts, &at)
		t.Options = DefaultLayout()
		_ = json.Unmarshal(opts, &t.Options)
		t.Options = t.Options.normalized()
		t.UpdatedAt = &at
		return t, err
	})
	if err != nil {
		return layout, csat, err
	}
	for _, t := range list {
		t.BrandID = brandID
		switch t.Kind {
		case KindLayout:
			if strings.TrimSpace(t.BodyHTML) == "" {
				t.BodyHTML = DefaultCustomLayout
			}
			layout = t
		case KindCSAT:
			if strings.TrimSpace(t.Subject) == "" {
				t.Subject = DefaultCSATSubject
			}
			if strings.TrimSpace(t.BodyHTML) == "" {
				t.BodyHTML = DefaultCSATHTML
			}
			t.Options = DefaultLayout()
			csat = t
		}
	}
	return layout, csat, nil
}

// CleanLayout validates and sanitizes layout options and a custom layout. fields are the
// workspace's custom field variables.
func CleanLayout(o LayoutOptions, custom string, fields map[string]bool) (LayoutOptions, string, []Problem) {
	var probs []Problem
	if o.Accent != "" && !ValidColor(o.Accent) {
		probs = append(probs, Problem{Message: "The accent must be a colour like #2563EB, or empty for the brand colour."})
	}
	o = o.normalized()
	if utf8.RuneCountInString(o.HistoryHeader) > 200 {
		probs = append(probs, Problem{Message: "The quote line must be 200 characters or less."})
	}
	o.HistoryHeader = strings.Join(strings.Fields(o.HistoryHeader), " ")
	probs = append(probs, Check(o.HistoryHeader, KindHistoryHeader, nil)...)
	if len(o.FooterHTML) > 16<<10 {
		probs = append(probs, Problem{Message: "The footer must be 16 KB or less."})
	}
	o.FooterHTML = SanitizeDesign(o.FooterHTML)
	probs = append(probs, Check(o.FooterHTML, KindSignature, fields)...)
	if len(custom) > 128<<10 {
		probs = append(probs, Problem{Message: "The layout must be 128 KB or less."})
	}
	custom = SanitizeDesign(custom)
	if o.Mode == "custom" {
		probs = append(probs, Check(custom, KindLayout, fields)...)
	}
	return o, custom, probs
}

// FieldVariables are the workspace's custom field variable names.
func FieldVariables(ctx context.Context, q db.Querier, wid uuid.UUID) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT entity, key FROM custom_fields WHERE workspace_id = $1`, wid)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var e, k string
		err := r.Scan(&e, &k)
		out[e+".fields."+k] = true
		return struct{}{}, err
	})
	return out, err
}

package templates

import (
	"strings"
	"testing"
	"time"
)

func richValues() *Values {
	v := vals(map[string]string{
		"contact.first_name": "Jane", "contact.name": "jane cooper", "ticket.priority": "Urgent", "ticket.number": "1042",
		"ticket.subject": "Re: Refund for a double charge", "agent.phone": "", "agent.title": "Support lead",
		"ticket.created_date": "18 October 2026", "contact.fields.seats": "12",
	})
	v.Lists = map[string][]string{"ticket.labels": {"Billing", "Refund", "VIP"}, "contact.labels": {}}
	v.Location = time.UTC
	return v
}

func TestLanguage(t *testing.T) {
	v := richValues()
	cases := map[string]string{
		// elsif, and, or, not, comparisons
		`{% if agent.phone %}p{% elsif agent.title %}t{% else %}n{% endif %}`:                   "t",
		`{% if ticket.priority == "urgent" and contact.first_name %}yes{% endif %}`:             "yes",
		`{% if agent.phone or agent.title %}yes{% endif %}`:                                     "yes",
		`{% if not agent.phone %}none{% endif %}`:                                               "none",
		`{% if ticket.number > 1000 %}big{% endif %}{% if ticket.number >= 1043 %}x{% endif %}`: "big",
		`{% if contact.fields.seats <= 12 and contact.fields.seats > 11 %}ok{% endif %}`:        "ok",
		`{% if ticket.labels contains "vip" %}vip{% endif %}`:                                   "vip",
		`{% if ticket.subject contains "refund" %}r{% endif %}`:                                 "r",
		`{% if contact.labels == empty %}no labels{% endif %}`:                                  "no labels",
		`{% if agent.phone != blank %}x{% else %}blank{% endif %}`:                              "blank",
		`{% if (agent.phone or agent.title) and contact.first_name %}g{% endif %}`:              "g",
		// case
		`{% case ticket.priority %}{% when "Low", "Normal" %}calm{% when "Urgent" %}fast{% else %}?{% endcase %}`: "fast",
		`{% case ticket.priority %}{% when "Low" %}calm{% else %}other{% endcase %}`:                              "other",
		// loops
		`{% for l in ticket.labels %}{{ forloop.index }}.{{ l }}{% unless forloop.last %}, {% endunless %}{% endfor %}`: "1.Billing, 2.Refund, 3.VIP",
		`{% for l in ticket.labels limit: 2 reversed %}{{ l }} {% endfor %}`:                                            "Refund Billing ",
		`{% for l in ticket.labels offset: 1 %}{{ l }}{% endfor %}`:                                                     "RefundVIP",
		`{% for l in contact.labels %}{{ l }}{% else %}none{% endfor %}`:                                                "none",
		`{% for i in (1..3) %}{{ i }}{% endfor %}`:                                                                      "123",
		`{% for l in ticket.labels %}{% for i in (1..2) %}{{ l | slice: 0, 1 }}{% endfor %}{% endfor %}`:                "BBRRVV",
		// assign, capture, comment, raw
		`{% assign who = contact.first_name | default: "there" | upcase %}Hi {{ who }}`:         "Hi JANE",
		`{% capture line %}{{ agent.title }} ({{ ticket.number }}){% endcapture %}[{{ line }}]`: "[Support lead (1042)]",
		`a{% comment %}{{ secret }} hidden{% endcomment %}b`:                                    "ab",
		`{% raw %}{{ contact.first_name }}{% endraw %}`:                                         "{{ contact.first_name }}",
		// filters
		`{{ contact.name | titlecase }}`:                                                    "Jane Cooper",
		`{{ ticket.subject | remove: "Re: " | truncate: 12 }}`:                              "Refund for…",
		`{{ ticket.subject | truncatewords: 2, "..." }}`:                                    "Re: Refund...",
		`{{ ticket.subject | replace: "double", "duplicate" }}`:                             "Re: Refund for a duplicate charge",
		`{{ ticket.number | plus: 8 | times: 2 | divided_by: 4 }}`:                          "525",
		`{{ contact.fields.seats | divided_by: 5 | round: 1 }}`:                             "2.4",
		`{{ ticket.labels | join: " / " }}`:                                                 "Billing / Refund / VIP",
		`{{ ticket.labels }}`:                                                               "Billing, Refund, VIP",
		`{{ ticket.labels | size }} {{ ticket.labels | last }}`:                             "3 VIP",
		`{{ "b,a,b" | split: "," | uniq | sort | join }}`:                                   "a, b",
		`{{ ticket.created_date | date: "%A %e %b %Y" }}`:                                   "Sunday 18 Oct 2026",
		`{{ contact.name | slice: 0, 4 }} {{ ticket.number | prepend: "#" | append: "!" }}`: "jane #1042!",
		`{{ "a b" | url_encode }}`:                                                          "a+b",
		// unknown variables keep the whole block as written
		`{% if mystery %}x{% endif %}`:                   `{% if mystery %}x{% endif %}`,
		`{% for x in mystery.list %}{{ x }}{% endfor %}`: `{% for x in mystery.list %}{{ x }}{% endfor %}`,
	}
	for src, want := range cases {
		if got := Render(src, v, Text); got != want {
			t.Errorf("Render(%q)\n got %q\nwant %q", src, got, want)
		}
	}
}

func TestLanguageEscapesAndLimits(t *testing.T) {
	v := richValues()
	v.Lists["ticket.labels"] = []string{"<b>x</b>"}
	if got := Render(`{% for l in ticket.labels %}{{ l }}{% endfor %}`, v, HTML); got != "&lt;b&gt;x&lt;/b&gt;" {
		t.Fatalf("loop items escaped: %q", got)
	}
	// A loop never runs away.
	got := Render(`{% for i in (1..100000) %}{% for j in (1..100000) %}x{% endfor %}{% endfor %}`, v, Text)
	if len(got) > maxIterations+maxLoop {
		t.Fatalf("loops ran %d times", len(got))
	}
	// Nesting has a ceiling.
	deep := strings.Repeat("{% if contact.first_name %}", 40) + "x" + strings.Repeat("{% endif %}", 40)
	if p := Parse(deep).Problems(); len(p) == 0 {
		t.Fatal("deep nesting not reported")
	}
}

func TestCheckLocals(t *testing.T) {
	src := `{% assign g = contact.first_name %}{{ g }}{% for l in ticket.labels %}{{ l }}{{ forloop.index }}{% endfor %}{% capture c %}x{% endcapture %}{{ c }}`
	if p := Check(src, KindReply, nil); len(p) != 0 {
		t.Fatalf("locals reported: %+v", p)
	}
	// A loop name is only known inside its loop.
	if p := Check(`{% for l in ticket.labels %}{% endfor %}{{ l }}`, KindReply, nil); len(p) != 1 {
		t.Fatalf("loop name outside its loop: %+v", p)
	}
	bad := []string{
		`{{ ticket.subject | truncate }}`,
		`{{ ticket.subject | default }}`,
		`{% case %}{% endcase %}`,
		`{% for in ticket.labels %}{% endfor %}`,
		`{% assign = 1 %}`,
		`{% if ticket.number > %}{% endif %}`,
		`{% when "x" %}`,
		`{% loop %}`,
	}
	for _, src := range bad {
		if p := Check(src, KindReply, nil); len(p) == 0 {
			t.Errorf("no problem for %q", src)
		}
	}
}

func TestDidYouMean(t *testing.T) {
	for src, want := range map[string]string{
		"{{ contact.frist_name }}":           "Did you mean contact.first_name?",
		"{{ ticket.subject | truncat: 10 }}": "Did you mean truncate?",
		"{% iff contact.name %}{% endif %}":  "Did you mean if?",
		"{{ contact.fields.plann }}":         "Did you mean contact.fields.plan?",
	} {
		probs := Check(src, KindReply, map[string]bool{"contact.fields.plan": true})
		found := false
		for _, p := range probs {
			found = found || strings.Contains(p.Message, want)
		}
		if !found {
			t.Errorf("%s: %+v", src, probs)
		}
	}
}

package helpdomains

import (
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/config"
)

func TestCleanHostname(t *testing.T) {
	cfg := &config.Config{Env: "production", AppURL: "https://app.opendesk.sh", APIURL: "https://api.opendesk.sh",
		GatewayURL: "wss://gateway.opendesk.sh/socket", SiteURL: "https://opendesk.sh", HelpDomain: "opendeskhelp.com",
		HelpCNAMETarget: "custom.opendeskhelp.com"}
	ok := map[string]string{
		"help.acme.com":                 "help.acme.com",
		"  Help.ACME.com. ":             "help.acme.com",
		"https://help.acme.com/":        "help.acme.com",
		"https://help.acme.com/a/reset": "help.acme.com",
		"help.acme.com/path?x=1":        "help.acme.com",
		"support.acme.co.uk":            "support.acme.co.uk",
		"docs.help.acme.io":             "docs.help.acme.io",
		"hilfe.münchen.de":              "hilfe.xn--mnchen-3ya.de",
	}
	for in, want := range ok {
		got, err := CleanHostname(cfg, in)
		if err != nil || got != want {
			t.Errorf("CleanHostname(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", "acme.com", "acme.co.uk", "localhost", "help.acme.com:8443", "10.0.0.1", "[::1]",
		"help_desk.acme.com", "-help.acme.com", "acme.opendeskhelp.com", "opendeskhelp.com", "custom.opendeskhelp.com",
		"app.opendesk.sh", "api.opendesk.sh", "help.local", "a..b.com", "https://"}
	for _, in := range bad {
		if got, err := CleanHostname(cfg, in); err == nil {
			t.Errorf("CleanHostname(%q) = %q, want an error", in, got)
		}
	}
}

func TestNextCheck(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		age  time.Duration
		o    outcome
		want time.Duration
	}{
		{0, outcome{status: "pending", problem: problemTXT}, 2 * time.Minute},
		{10 * time.Minute, outcome{status: "pending", problem: problemCertPending}, time.Minute},
		{2 * time.Hour, outcome{status: "pending", problem: problemCertPending}, 15 * time.Minute},
		{30 * time.Hour, outcome{status: "pending", problem: problemCNAME}, time.Hour},
		{100 * time.Hour, outcome{status: "active"}, 6 * time.Hour},
	}
	for _, c := range cases {
		if got := nextCheck(now, now.Add(-c.age), c.o).Sub(now); got != c.want {
			t.Errorf("nextCheck(age %v, %+v) = %v, want %v", c.age, c.o, got, c.want)
		}
	}
}

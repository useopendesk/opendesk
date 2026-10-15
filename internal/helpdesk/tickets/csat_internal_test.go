package tickets

import (
	"strings"
	"testing"
)

func TestCSATEmailLook(t *testing.T) {
	cases := []struct {
		look csatLook
		want string
	}{
		{csatLook{team: "Billing", brand: "Margin", workspace: "Margin Inc"}, "How did Billing do?"},
		{csatLook{brand: "Margin", workspace: "Margin Inc"}, "How did Margin do?"},
		{csatLook{brand: "  ", workspace: "Margin Inc"}, "How did Margin Inc do?"},
		{csatLook{}, "How did we do?"},
	}
	for _, c := range cases {
		m := csatEmail("https://app.example.com/", "tok", c.look, 7, "Refund")
		if !strings.Contains(m.html, c.want) || !strings.HasPrefix(m.text, c.want) {
			t.Fatalf("%+v: %q", c.look, m.text)
		}
	}

	m := csatEmail("https://app.example.com", "tok", csatLook{team: "<b>Ops</b>", brand: "Margin", color: "#2563EB", logoURL: "https://api.example.com/v1/images/abc"}, 7, "Refund")
	if !strings.Contains(m.html, "How did &lt;b&gt;Ops&lt;/b&gt; do?") {
		t.Fatalf("team name not escaped: %s", m.html)
	}
	if strings.Count(m.html, "border:2px solid #2563EB") != 5 || !strings.Contains(m.html, `<img src="https://api.example.com/v1/images/abc" alt="Margin"`) {
		t.Fatalf("brand colour or logo missing: %s", m.html)
	}
	for n, label := range []string{"Very poor", "Poor", "Okay", "Good", "Great"} {
		if !strings.Contains(m.text, label) || !strings.Contains(m.html, `title="`+label+`"`) {
			t.Fatalf("score %d label %q missing", n+1, label)
		}
	}
	if !strings.Contains(m.text, "https://app.example.com/csat/tok?score=1") {
		t.Fatalf("link: %s", m.text)
	}

	// A colour that is not #rrggbb never reaches the markup.
	m = csatEmail("https://app.example.com", "tok", csatLook{brand: "Margin", color: "red;background:url(x)"}, 7, "Refund")
	if strings.Contains(m.html, "url(x)") || strings.Count(m.html, "border:2px solid #0A0A0A") != 5 || strings.Contains(m.html, "<img") {
		t.Fatalf("unsafe colour or stray logo: %s", m.html)
	}
}

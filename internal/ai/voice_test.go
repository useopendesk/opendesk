package ai

import "testing"

func TestTidyReply(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"em dash joins", "The fix — which shipped today — works now.", "The fix, which shipped today, works now."},
		{"unspaced em dash", "It's quick—two minutes at most.", "It's quick, two minutes at most."},
		{"spaced en dash and double hyphen", "Open Settings – then Billing -- and pick a plan.", "Open Settings, then Billing, and pick a plan."},
		{"range stays", "We're open 9–5, Monday to Friday, 2025–2026.", "We're open 9–5, Monday to Friday, 2025–2026."},
		{"bullets stay", "Try this:\n\n- Sign out\n- Sign back in — it resets the session", "Try this:\n\n- Sign out\n- Sign back in, it resets the session"},
		{"horizontal rule stays", "One\n\n---\n\nTwo", "One\n\n---\n\nTwo"},
		{"dash before punctuation", "Here's the link —.", "Here's the link."},
		{"code stays", "Run `a — b` then:\n```\nx -- y\n```\nDone — thanks.", "Run `a — b` then:\n```\nx -- y\n```\nDone, thanks."},
		{"cyrillic keeps dashes", "Москва — столица.", "Москва — столица."},
		{"stock opener line", "Hi Jane,\n\nThank you for reaching out!\n\nYour refund went out today.", "Hi Jane,\n\nYour refund went out today."},
		{"stock sentences inline", "Hi Jane,\n\nI hope this email finds you well. Your refund went out today. I hope this helps!", "Hi Jane,\n\nYour refund went out today."},
		{"two stock sentences", "Thanks for reaching out! Great question! The limit is 10.", "The limit is 10."},
		{"stock closer", "The limit is 10.\n\nDon’t hesitate to reach out if you have any other questions.\n\nBest,\nOllie", "The limit is 10.\n\nBest,\nOllie"},
		{"longer sentence kept", "Thank you for reaching out about the refund. It went out today.", "Thank you for reaching out about the refund. It went out today."},
		{"all stock kept", "Thanks for reaching out!", "Thanks for reaching out!"},
	}
	for _, c := range cases {
		if got := TidyReply(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestTidyHTML(t *testing.T) {
	in := `<p>Reset it — it takes a minute.</p><pre><code>a — b</code></pre><p><a href="https://x.test/a—b">Guide</a> — read it.</p>`
	want := `<p>Reset it, it takes a minute.</p><pre><code>a — b</code></pre><p><a href="https://x.test/a—b">Guide</a>, read it.</p>`
	if got := tidyHTML(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := tidyHTML(`<p>Pick one — then open <a href="/x">the guide</a>.</p>`); got != `<p>Pick one, then open <a href="/x">the guide</a>.</p>` {
		t.Errorf("space before a tag: %q", got)
	}
	if got := TidyReply("<p>Done — thanks.</p>"); got != "<p>Done, thanks.</p>" {
		t.Errorf("TidyReply on HTML: %q", got)
	}
}

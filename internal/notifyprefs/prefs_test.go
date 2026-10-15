package notifyprefs

import (
	"testing"
	"time"
)

func TestEmailDue(t *testing.T) {
	created := time.Date(2026, 10, 13, 10, 58, 30, 0, time.UTC)
	cases := []struct {
		s    Settings
		want string
	}{
		{Settings{Cadence: CadenceInstant}, "2026-10-13T11:03:30Z"},
		{Settings{Cadence: CadenceHourly}, "2026-10-13T11:00:00Z"},
		{Settings{Cadence: CadenceDaily, DigestHour: 9, Timezone: "UTC"}, "2026-10-14T09:00:00Z"},
		{Settings{Cadence: CadenceDaily, DigestHour: 13, Timezone: "UTC"}, "2026-10-13T13:00:00Z"},
		// 10:58 UTC is 12:58 in Budapest (CEST): today's 13:00 there is 11:00 UTC.
		{Settings{Cadence: CadenceDaily, DigestHour: 13, Timezone: "Europe/Budapest"}, "2026-10-13T11:00:00Z"},
		{Settings{Cadence: CadenceDaily, DigestHour: 9, Timezone: "Nowhere/Unknown"}, "2026-10-14T09:00:00Z"},
	}
	for _, c := range cases {
		got := c.s.EmailDue(created)
		if got == nil || got.Format(time.RFC3339) != c.want {
			t.Errorf("%+v: got %v, want %s", c.s, got, c.want)
		}
	}
	if (Settings{Cadence: CadenceOff}).EmailDue(created) != nil {
		t.Error("off schedules an email")
	}
}

func TestDefaultsAndCatalogue(t *testing.T) {
	seen := map[string]bool{}
	for _, ty := range Types {
		if seen[ty.Type] {
			t.Fatalf("duplicate %s", ty.Type)
		}
		seen[ty.Type] = true
		if (ty.Group == GroupAIBilling) != ty.Admins {
			t.Errorf("%s: billing types are for admins", ty.Type)
		}
	}
	if d := Defaults("assigned"); !d.Email || !d.Push || !d.Desktop {
		t.Errorf("assigned: %+v", d)
	}
	if d := Defaults("team_joined"); d.Email || d.Push {
		t.Errorf("team_joined is quiet: %+v", d)
	}
	if d := Defaults("something_new"); d.Email {
		t.Error("unknown types are not emailed")
	}
	if !ValidTimezone("America/New_York") || ValidTimezone("Local") || ValidTimezone("") || ValidTimezone("Mars/Base") {
		t.Error("ValidTimezone")
	}
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	if (Settings{PausedUntil: &past}).Paused(now) || !(Settings{PausedUntil: &future}).Paused(now) {
		t.Error("Paused")
	}
}

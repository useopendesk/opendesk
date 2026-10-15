package notifyprefs

import (
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/config"
)

var t0 = time.Date(2026, 10, 13, 14, 0, 0, 0, time.UTC)

func strict() Pacing { return Pacing(config.NotifyEmailDefaults(true)) }

func wait(created time.Time, p Pacing, urgent bool) Waiting {
	return Waiting{Created: created, Due: created.Add(p.Delay), Urgent: urgent}
}

func ptr(t time.Time) *time.Time { return &t }

// The first email after a quiet spell goes at the delay; while activity continues each
// email waits a longer cooldown (15m, 30m, 1h, then 1h again); a quiet spell starts over.
func TestCooldownLadder(t *testing.T) {
	p := strict()
	d := p.Decide(t0, CadenceInstant, []Waiting{wait(t0, p, false)}, History{})
	if !d.At.Equal(t0.Add(5*time.Minute)) || d.Step != 0 || d.Why != "due" {
		t.Fatalf("first: %+v", d)
	}
	h := History{}
	sent := t0.Add(5 * time.Minute)
	want := []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour, time.Hour}
	for i, gap := range append([]time.Duration{0}, want...) {
		if i > 0 {
			// Something new right after the last email waits for its cooldown.
			d = p.Decide(sent, CadenceInstant, []Waiting{wait(sent.Add(time.Second), p, false)}, h)
			if !d.At.Equal(sent.Add(gap)) || d.Why != "cooldown" {
				t.Fatalf("email %d: want +%s, got %+v", i+1, gap, d)
			}
			sent = d.At
		}
		h.LastEmailed, h.Step, h.NextAllowed = ptr(sent), d.Step, ptr(sent.Add(p.Cooldown(d.Step)))
		h.Sent = nil // caps are tested on their own
	}
	// Two quiet hours later the next one is a first email again.
	later := sent.Add(3 * time.Hour)
	d = p.Decide(later, CadenceInstant, []Waiting{wait(later, p, false)}, h)
	if !d.At.Equal(later.Add(p.Delay)) || d.Step != 0 {
		t.Fatalf("after quiet: %+v", d)
	}
}

// An urgent notification may cut a cooldown short, never sooner than the urgent gap after
// the last email, and only so many times a day.
func TestUrgentCutsCooldownShort(t *testing.T) {
	p := strict()
	last := t0
	h := History{LastEmailed: ptr(last), Step: 2, NextAllowed: ptr(last.Add(time.Hour))}
	n := last.Add(time.Minute)
	d := p.Decide(n, CadenceInstant, []Waiting{wait(n, p, false), wait(n, p, true)}, h)
	if !d.Urgent || !d.At.Equal(last.Add(p.UrgentGap)) || d.Why != "urgent" {
		t.Fatalf("urgent: %+v", d)
	}
	// Not twice within the urgent gap.
	h.LastUrgent = ptr(last.Add(p.UrgentGap))
	d = p.Decide(n, CadenceInstant, []Waiting{wait(n, p, true)}, h)
	if !d.At.Equal(h.LastUrgent.Add(p.UrgentGap)) {
		t.Fatalf("urgent gap: %+v", d)
	}
	// Past the daily allowance urgent ones wait like the rest.
	h.UrgentSent = p.UrgentPerDay
	d = p.Decide(n, CadenceInstant, []Waiting{wait(n, p, true)}, h)
	if d.Urgent || !d.At.Equal(last.Add(time.Hour)) {
		t.Fatalf("urgent cap: %+v", d)
	}
	// A normal one never cuts it short.
	h.UrgentSent = 0
	if d = p.Decide(n, CadenceInstant, []Waiting{wait(n, p, false)}, h); d.Urgent || d.Why != "cooldown" {
		t.Fatalf("normal: %+v", d)
	}
}

// The hourly and daily caps hold every email, digests too, until a place frees up.
func TestCapsRollOver(t *testing.T) {
	p := strict()
	now := t0
	var h History
	for i := range p.PerHour {
		h.Sent = append(h.Sent, now.Add(-time.Duration(50-i)*time.Minute))
	}
	d := p.Decide(now, CadenceHourly, []Waiting{{Created: now.Add(-time.Hour), Due: now}}, h)
	if d.Why != "hour_cap" || !d.At.Equal(h.Sent[0].Add(time.Hour)) {
		t.Fatalf("hour cap: %+v", d)
	}
	h.Sent = nil
	for i := range p.PerDay {
		h.Sent = append(h.Sent, now.Add(-23*time.Hour+time.Duration(i)*time.Minute))
	}
	d = p.Decide(now, CadenceHourly, []Waiting{{Created: now.Add(-time.Hour), Due: now}}, h)
	if d.Why != "day_cap" || !d.At.Equal(h.Sent[0].Add(24*time.Hour)) {
		t.Fatalf("day cap: %+v", d)
	}
}

// Nothing goes while the person is active in the app; it waits until they have been away
// for the active window.
func TestActiveHolds(t *testing.T) {
	p := strict()
	n := t0.Add(-10 * time.Minute)
	h := History{ActiveAt: ptr(t0.Add(-time.Minute))}
	d := p.Decide(t0, CadenceInstant, []Waiting{wait(n, p, true)}, h)
	if d.Why != "active" || !d.At.Equal(t0.Add(-time.Minute).Add(p.ActiveWindow)) {
		t.Fatalf("active: %+v", d)
	}
	h.ActiveAt = ptr(t0.Add(-time.Hour))
	if d = p.Decide(t0, CadenceInstant, []Waiting{wait(n, p, true)}, h); d.Why == "active" || d.At.After(t0) {
		t.Fatalf("away: %+v", d)
	}
}

func TestDefaultsValid(t *testing.T) {
	for _, strict := range []bool{true, false} {
		if err := config.NotifyEmailDefaults(strict).Validate(); err != nil {
			t.Errorf("defaults (strict %v): %v", strict, err)
		}
	}
}

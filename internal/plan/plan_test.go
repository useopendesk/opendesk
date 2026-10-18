package plan

import "testing"

// TestOverrides: an override replaces the plan's limit, 0 lifts it, nil keeps it.
func TestOverrides(t *testing.T) {
	five, zero, ai := 5, 0, Micro(25_000_000)
	e := Entitlements{Plan: Free, MaxMailboxes: FreeMaxMailboxes, MaxMembers: FreeMaxMembers, MonthlyUSD: FreeMonthlyUSD}
	Overrides{MaxMailboxes: &five, MaxMembers: &zero, MonthlyUSD: &ai}.apply(&e)
	if e.MaxMailboxes != 5 || e.MaxMembers != 0 || e.MonthlyUSD != ai {
		t.Fatalf("applied: %+v", e)
	}
	e = Entitlements{Plan: Pro, MonthlyUSD: ProUSDPerSeat}
	Overrides{}.apply(&e)
	if e.MaxMailboxes != 0 || e.MonthlyUSD != ProUSDPerSeat {
		t.Fatalf("no overrides: %+v", e)
	}
}

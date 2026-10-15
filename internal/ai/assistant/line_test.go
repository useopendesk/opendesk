package assistant

import (
	"strings"
	"testing"
)

func TestTicketLineNamesTeamBrandAndAddresses(t *testing.T) {
	team := "team1"
	tk := ticket{Number: 7, Subject: "Refund", Status: "open", Priority: "normal", Channel: "email", TeamID: &team,
		ReceivedAt: "billing@acme.test", teamName: "Billing", brandName: "Acme EU", replyFrom: "billing@acme.test"}
	line := tk.line()
	for _, want := range []string{"team: Billing", "brand: Acme EU", "written to: billing@acme.test", "replies from: billing@acme.test"} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q lacks %q", line, want)
		}
	}
	tk.teamName = ""
	if line := tk.line(); !strings.Contains(line, "team id: team1") {
		t.Fatalf("%q", line)
	}
}

func TestPatchCarriesTeam(t *testing.T) {
	b := patchArgs{TeamSet: true, TeamID: "t1"}.body()
	if b["team_id"] != "t1" {
		t.Fatalf("%v", b)
	}
	b = patchArgs{TeamSet: true}.body()
	if v, ok := b["team_id"]; !ok || v != nil {
		t.Fatalf("%v", b)
	}
	cur := "t0"
	ch := patchArgs{TeamSet: true, TeamID: "t1", TeamName: "Billing"}.diff(&ticket{TeamID: &cur, teamName: "Support"})
	if len(ch) != 1 || ch[0].Field != "Team" || ch[0].From != "Support" || ch[0].To != "Billing" {
		t.Fatalf("%+v", ch)
	}
}

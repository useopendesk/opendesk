package billing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/useopendesk/opendesk/internal/plan"
)

// A call on the workspace's own AI key is recorded at $0, leaves the balance
// alone even when it is empty, and the usage report counts it as "own_key".
func TestSpendOnOwnKey(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	ctx := context.Background()
	svc := plan.New(true, 1.5)
	if err := plan.Ensure(ctx, e.d.DB.Pool, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE ai_balances SET included_used_micro_usd = 500000 WHERE workspace_id = $1`, wid); err != nil {
		t.Fatal(err)
	}
	charged, err := svc.Spend(ctx, e.d.DB.Pool, wid, plan.Usage{Feature: "draft_reply", Model: "anthropic/claude-sonnet-5.5", CostUSD: 0.01, OwnKey: true, InputTokens: 100})
	if err != nil || charged != 0 {
		t.Fatalf("own key charged %d, %v", charged, err)
	}
	if n := e.scalar(`SELECT included_used_micro_usd FROM ai_balances WHERE workspace_id = $1`, wid).(int64); n != 500000 {
		t.Fatalf("balance moved: %d", n)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE ai_balances SET purchased_micro_usd = 50000 WHERE workspace_id = $1`, wid); err != nil {
		t.Fatal(err)
	}
	if charged, err := svc.Spend(ctx, e.d.DB.Pool, wid, plan.Usage{Feature: "triage", Model: "jev"}); err != nil || charged != 10000 {
		t.Fatalf("jev charged %d, %v", charged, err)
	}
	r := owner.do("GET", "/v1/billing/usage", nil)
	expect(t, r, 200, "")
	var rep struct {
		TotalUSD    float64 `json:"total_usd"`
		TotalCalls  int     `json:"total_calls"`
		OwnKeyCalls int     `json:"own_key_calls"`
		ByModel     []struct {
			Model  string  `json:"model"`
			Source string  `json:"source"`
			USD    float64 `json:"usd"`
			Calls  int     `json:"calls"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(r.Raw, &rep); err != nil || rep.TotalUSD != 0.01 || rep.TotalCalls != 2 || rep.OwnKeyCalls != 1 || len(rep.ByModel) != 2 {
		t.Fatalf("usage: %s", r.Raw)
	}
	for _, m := range rep.ByModel {
		if (m.Model == "jev") != (m.Source == "instance") || (m.Source == "own_key" && m.USD != 0) {
			t.Fatalf("by_model: %s", r.Raw)
		}
	}
}

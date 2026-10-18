package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/billing"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/plan"
)

// proWorkspace walks an owner through Checkout and the webhook to an active Pro plan.
func proWorkspace(t *testing.T, e *env) (*client, uuid.UUID, *fakeSub) {
	t.Helper()
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	r := owner.do("POST", "/v1/billing/checkout", map[string]any{"interval": "month"})
	expect(t, r, 200, "")
	cus := e.scalar(`SELECT stripe_customer_id FROM subscriptions WHERE workspace_id = $1`, wid).(string)
	sub := e.fake.addSub(cus, wid.String(), 1)
	expect(t, e.event("evt_co_"+wid.String(), "checkout.session.completed", map[string]any{
		"id": "cs_" + wid.String(), "object": "checkout.session", "mode": "subscription", "payment_status": "paid",
		"customer": cus, "subscription": sub.ID, "client_reference_id": wid.String(), "metadata": map[string]any{"workspace_id": wid.String()},
	}), 200, "")
	return owner, wid, sub
}

func TestCheckoutLiftsLimits(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")

	b := owner.do("GET", "/v1/billing", nil)
	expect(t, b, 200, "")
	if b.str("plan") != "free" || b.num("limits", "max_mailboxes") != 1 || b.num("limits", "max_members") != 1 ||
		b.str("status") != "none" || b.num("ai", "monthly_usd") != 0.5 || b.num("prices", "pro_month", "unit_amount") != 1900 {
		t.Fatalf("free billing: %s", b.Raw)
	}

	// Free: one mailbox, one member (the owner).
	expect(t, owner.addMailbox("one@acme.test"), 201, "")
	expect(t, owner.addMailbox("two@acme.test"), 402, "plan_limit")
	expect(t, owner.invite("agent@acme.test", "agent"), 402, "plan_limit")

	r := owner.do("POST", "/v1/billing/checkout", map[string]any{"interval": "month"})
	expect(t, r, 200, "")
	if r.str("url") == "" {
		t.Fatal("no checkout url")
	}
	f := e.fake.lastSession()
	for k, want := range map[string]string{
		"mode": "subscription", "line_items[0][price]": "price_pro", "line_items[0][quantity]": "1",
		"client_reference_id": wid.String(), "metadata[workspace_id]": wid.String(),
		"subscription_data[metadata][workspace_id]": wid.String(), "automatic_tax[enabled]": "",
		"billing_address_collection": "required", "name_collection[business][enabled]": "true",
		"name_collection[business][optional]": "false",
		"customer_update[address]":            "auto", "customer_update[name]": "auto", "tax_id_collection[enabled]": "true", "allow_promotion_codes": "true",
		"adaptive_pricing[enabled]": "true", "success_url": "http://app.test/settings/billing?checkout=success",
		"cancel_url": "http://app.test/settings/billing?checkout=canceled",
	} {
		if got := f.Get(k); got != want {
			t.Errorf("checkout %s = %q, want %q", k, got, want)
		}
	}
	cus := e.scalar(`SELECT stripe_customer_id FROM subscriptions WHERE workspace_id = $1`, wid).(string)
	if f.Get("customer") != cus {
		t.Fatalf("checkout customer %q, stored %q", f.Get("customer"), cus)
	}
	cf := e.fake.customers[cus]
	if cf.Get("metadata[workspace_id]") != wid.String() || cf.Get("name") != "Acme" || cf.Get("email") != owner.email {
		t.Fatalf("customer form: %v", cf)
	}
	// A second checkout reuses the customer.
	expect(t, owner.do("POST", "/v1/billing/checkout", nil), 200, "")
	if len(e.fake.customers) != 1 {
		t.Fatalf("customers created: %d", len(e.fake.customers))
	}

	// Stripe creates the subscription and reports the session.
	sub := e.fake.addSub(cus, wid.String(), 1)
	expect(t, e.event("evt_sub_created", "customer.subscription.created", sub.json()), 200, "")
	expect(t, e.event("evt_checkout", "checkout.session.completed", map[string]any{"id": "cs_1", "object": "checkout.session",
		"mode": "subscription", "payment_status": "paid", "customer": cus, "subscription": sub.ID, "client_reference_id": wid.String(),
		"metadata": map[string]any{"workspace_id": wid.String()}}), 200, "")

	b = owner.do("GET", "/v1/billing", nil)
	if b.str("plan") != "pro" || b.str("status") != "active" || b.num("seats") != 1 || b.num("limits", "max_mailboxes") != 0 ||
		b.num("ai", "monthly_usd") != 10 || b.str("current_period_end") == "" || b.str("interval") != "month" {
		t.Fatalf("pro billing: %s", b.Raw)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.subscribed'`, wid); n != 1 {
		t.Fatalf("billing.subscribed audit rows: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'billing.updated'`, wid); n < 1 {
		t.Fatal("no billing.updated event")
	}
	// Limits lifted.
	expect(t, owner.addMailbox("two@acme.test"), 201, "")
	expect(t, owner.invite("agent@acme.test", "agent"), 201, "")
	// Already on Pro: checkout is refused, the portal opens.
	expect(t, owner.do("POST", "/v1/billing/checkout", nil), 409, "already_subscribed")
	p := owner.do("POST", "/v1/billing/portal", nil)
	expect(t, p, 200, "")
	if p.str("url") == "" {
		t.Fatal("no portal url")
	}
}

func TestSeatSyncAfterInviteAccepted(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, sub := proWorkspace(t, e)

	agent := e.signup("agent")
	expect(t, owner.invite(agent.email, "agent"), 201, "")
	expect(t, agent.do("POST", "/v1/invitations/accept", map[string]any{"token": e.inviteToken(agent.email)}), 200, "")

	if n := e.count(`SELECT count(*) FROM river_job WHERE kind = 'billing.sync_seats' AND args->>'workspace_id' = $1
		AND state = 'scheduled' AND scheduled_at > now()`, wid.String()); n != 1 {
		t.Fatalf("debounced sync jobs: %d", n)
	}
	b := owner.do("GET", "/v1/billing", nil)
	if b.num("billed_seats") != 2 || b.num("seats") != 1 {
		t.Fatalf("before sync: %s", b.Raw)
	}
	if err := billing.RunSyncSeats(context.Background(), e.d, wid); err != nil {
		t.Fatal(err)
	}
	if q := e.fake.sub(sub.ID).Quantity; q != 2 {
		t.Fatalf("stripe quantity %d, want 2", q)
	}
	if f := e.fake.itemForms[len(e.fake.itemForms)-1]; f.Get("proration_behavior") != "create_prorations" {
		t.Fatalf("proration: %v", f)
	}
	if b := owner.do("GET", "/v1/billing", nil); b.num("seats") != 2 || b.num("ai", "monthly_usd") != 20 {
		t.Fatalf("after sync: %s", b.Raw)
	}

	// Viewers are free: no Stripe update.
	viewer := e.signup("viewer")
	expect(t, owner.invite(viewer.email, "viewer"), 201, "")
	expect(t, viewer.do("POST", "/v1/invitations/accept", map[string]any{"token": e.inviteToken(viewer.email)}), 200, "")
	updates := len(e.fake.itemForms)
	if err := billing.RunSyncSeats(context.Background(), e.d, wid); err != nil {
		t.Fatal(err)
	}
	if len(e.fake.itemForms) != updates {
		t.Fatal("a viewer changed the Stripe quantity")
	}

	// Removing the agent brings the quantity back down; the hourly reconcile queues it too.
	agentID := e.scalar(`SELECT id FROM users WHERE email = $1`, agent.email)
	expect(t, owner.do("DELETE", "/v1/members/"+uuidString(agentID), nil), 204, "")
	if err := billing.RunReconcile(context.Background(), e.d); err != nil {
		t.Fatal(err)
	}
	if err := billing.RunSyncSeats(context.Background(), e.d, wid); err != nil {
		t.Fatal(err)
	}
	if q := e.fake.sub(sub.ID).Quantity; q != 1 {
		t.Fatalf("stripe quantity after removal %d, want 1", q)
	}
}

func uuidString(v any) string {
	switch id := v.(type) {
	case [16]byte:
		return uuid.UUID(id).String()
	case string:
		return id
	}
	return ""
}

func TestCancelFallsBackToFree(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, sub := proWorkspace(t, e)
	expect(t, owner.addMailbox("one@acme.test"), 201, "")
	expect(t, owner.addMailbox("two@acme.test"), 201, "")

	// past_due keeps Pro while Stripe retries; the failed payment emails owners and admins.
	e.fake.setStatus(sub.ID, "past_due")
	expect(t, e.event("evt_inv_failed", "invoice.payment_failed", map[string]any{"id": "in_1", "object": "invoice", "customer": sub.Customer,
		"parent": map[string]any{"type": "subscription_details", "subscription_details": map[string]any{"subscription": sub.ID,
			"metadata": map[string]any{"workspace_id": wid.String()}}}}), 200, "")
	if b := owner.do("GET", "/v1/billing", nil); b.str("plan") != "pro" || b.str("status") != "past_due" {
		t.Fatalf("past_due: %s", b.Raw)
	}
	if n := e.count(`SELECT count(*) FROM river_job WHERE kind = 'mail.system' AND args->>'to' = $1 AND args->>'subject' LIKE 'Payment failed%'`,
		owner.email); n != 1 {
		t.Fatalf("payment failed emails: %d", n)
	}

	e.fake.setStatus(sub.ID, "canceled")
	expect(t, e.event("evt_sub_deleted", "customer.subscription.deleted", sub.json()), 200, "")
	b := owner.do("GET", "/v1/billing", nil)
	if b.str("plan") != "free" || b.str("status") != "canceled" || b.num("limits", "max_mailboxes") != 1 || b.num("usage", "mailboxes") != 2 {
		t.Fatalf("after cancel: %s", b.Raw)
	}
	// Nothing is deleted; creating more is refused until within the free limits.
	expect(t, owner.addMailbox("three@acme.test"), 402, "plan_limit")
	expect(t, owner.invite("x@acme.test", "agent"), 402, "plan_limit")
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.canceled'`, wid); n != 1 {
		t.Fatalf("billing.canceled audit rows: %d", n)
	}
	// A late update for the old subscription does not revive it.
	expect(t, e.event("evt_sub_late", "customer.subscription.updated", sub.json()), 200, "")
	if b := owner.do("GET", "/v1/billing", nil); b.str("plan") != "free" {
		t.Fatalf("late update: %s", b.Raw)
	}
}

func TestTopUpGrantIsIdempotent(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"amount_usd": 4.99}), 422, "")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"amount_usd": 5000.01}), 422, "")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{}), 422, "")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"top_ups": 21}), 422, "")
	// Any amount, priced on the top-up product dollar for dollar.
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"amount_usd": 37.5}), 200, "")
	f := e.fake.lastSession()
	for k, want := range map[string]string{"mode": "payment", "line_items[0][price_data][product]": "prod_ai_topup", "line_items[0][price_data][unit_amount]": "3750",
		"line_items[0][price_data][currency]": "usd", "line_items[0][quantity]": "1", "billing_address_collection": "required", "tax_id_collection[enabled]": "true",
		"automatic_tax[enabled]": "", "metadata[workspace_id]": wid.String(), "metadata[micro_usd]": "37500000", "metadata[kind]": "top_up",
		"invoice_creation[enabled]": "true", "invoice_creation[invoice_data][metadata][kind]": "top_up",
		"payment_intent_data[setup_future_usage]": "off_session"} {
		if got := f.Get(k); got != want {
			t.Errorf("top-up checkout %s = %q, want %q", k, got, want)
		}
	}
	// The older body still buys $10 each.
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"top_ups": 3}), 200, "")
	if f := e.fake.lastSession(); f.Get("metadata[micro_usd]") != "30000000" || f.Get("line_items[0][price_data][unit_amount]") != "3000" {
		t.Fatalf("top-up checkout by count: %v", f)
	}
	cus := e.scalar(`SELECT stripe_customer_id FROM subscriptions WHERE workspace_id = $1`, wid).(string)
	session := func(status string) map[string]any {
		return map[string]any{"id": "cs_topup_" + wid.String(), "object": "checkout.session", "mode": "payment", "payment_status": status,
			"customer": cus, "client_reference_id": wid.String(), "metadata": map[string]any{"workspace_id": wid.String(), "kind": "top_up", "top_ups": "3"}}
	}
	// Not paid yet (async method): nothing granted.
	expect(t, e.event("evt_cr_unpaid", "checkout.session.completed", session("unpaid")), 200, "")
	if n := e.scalar(`SELECT purchased_micro_usd FROM ai_balances WHERE workspace_id = $1`, wid).(int64); n != 0 {
		t.Fatalf("granted before payment: %d", n)
	}
	expect(t, e.event("evt_cr_paid", "checkout.session.async_payment_succeeded", session("paid")), 200, "")
	// Replays of the event, and another event for the same session, grant nothing more.
	expect(t, e.event("evt_cr_paid", "checkout.session.async_payment_succeeded", session("paid")), 200, "")
	expect(t, e.event("evt_cr_paid_2", "checkout.session.completed", session("paid")), 200, "")
	if n := e.scalar(`SELECT purchased_micro_usd FROM ai_balances WHERE workspace_id = $1`, wid).(int64); n != int64(3*plan.TopUpUSD) {
		t.Fatalf("purchased balance %d, want %d", n, 3*plan.TopUpUSD)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.ai_top_up_purchased'`, wid); n != 1 {
		t.Fatalf("top-up audit rows: %d", n)
	}
	b := owner.do("GET", "/v1/billing", nil)
	if b.num("ai", "purchased_usd") != 30 || b.num("ai", "remaining_usd") != 30.5 || b.num("prices", "ai_top_up", "usd") != 10 {
		t.Fatalf("ai balance: %s", b.Raw)
	}
}

func TestWebhookRefusesBadSignatures(t *testing.T) {
	e := newEnv(t, true)
	payload := eventPayload("evt_forged", "checkout.session.completed", map[string]any{"id": "cs_x", "object": "checkout.session"})
	expect(t, e.postSigned(payload, "whsec_wrong", time.Now()), 400, "invalid_signature")
	expect(t, e.postSigned(payload, whSecret, time.Now().Add(-10*time.Minute)), 400, "invalid_signature")
	r := e.send(mustReq(t, e.srv.URL+billing.WebhookPath, payload))
	expect(t, r, 400, "invalid_signature")
	if n := e.count(`SELECT count(*) FROM stripe_events WHERE id = 'evt_forged'`); n != 0 {
		t.Fatal("a forged event was recorded")
	}
	// Bodies over 1MB are refused before verification.
	big := make([]byte, 1<<20+10)
	expect(t, e.postSigned(big, whSecret, time.Now()), 413, "payload_too_large")
	// Unhandled types are acknowledged and recorded once.
	expect(t, e.event("evt_other", "customer.created", map[string]any{"id": "cus_x", "object": "customer"}), 200, "")
	if n := e.count(`SELECT count(*) FROM stripe_events WHERE id = $1`, "evt_other_"+e.run); n != 1 {
		t.Fatal("unhandled event not recorded")
	}
}

func TestSpendConcurrency(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	ctx := context.Background()
	svc := plan.New(true, 1.5)
	// Free allowance $0.50; leave $0.10. Calls without a reported cost charge the
	// feature's typical cost (draft_reply: $0.03); concurrent spends never charge more
	// than what is left, and the balance never goes below zero.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE ai_balances SET included_used_micro_usd = 400000 WHERE workspace_id = $1`, wid); err != nil {
		t.Fatal(err)
	}
	spend := func(u plan.Usage) (charged plan.Micro) {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				m, err := svc.Spend(ctx, e.d.DB.Pool, wid, u)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					t.Errorf("spend: %v", err)
				}
				charged += m
			})
		}
		wg.Wait()
		return
	}
	if charged := spend(plan.Usage{Feature: "draft_reply", Model: "test"}); charged != 100000 {
		t.Fatalf("charged %d, want 100000", charged)
	}
	c, err := svc.Balance(ctx, e.d.DB.Pool, wid)
	if err != nil || c.IncludedUsed != 500000 || c.Remaining != 0 {
		t.Fatalf("balance %+v %v", c, err)
	}
	if err := svc.Reserve(ctx, e.d.DB.Pool, wid, plan.Typical["draft_reply"]); !errors.Is(err, plan.ErrNoBalance) {
		t.Fatalf("reserve: %v", err)
	}
	// Metered: $0.002 at 1.5x is $0.003, taken from purchased top-ups after the allowance.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE ai_balances SET purchased_micro_usd = 100000 WHERE workspace_id = $1`, wid); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reserve(ctx, e.d.DB.Pool, wid, plan.Typical["draft_reply"]); err != nil {
		t.Fatalf("reserve with purchased top-ups: %v", err)
	}
	// An estimate above the balance, or below the floor with an empty balance, is refused.
	if err := svc.Reserve(ctx, e.d.DB.Pool, wid, 100001); !errors.Is(err, plan.ErrNoBalance) {
		t.Fatalf("reserve above the balance: %v", err)
	}
	if charged := spend(plan.Usage{Feature: "draft_reply", Model: "test", CostUSD: 0.002}); charged != 60000 {
		t.Fatalf("metered charged %d, want 60000", charged)
	}
	if n := e.scalar(`SELECT purchased_micro_usd FROM ai_balances WHERE workspace_id = $1`, wid).(int64); n != 40000 {
		t.Fatalf("purchased balance %d, want 40000", n)
	}
	if n := e.count(`SELECT count(*) FROM ai_usage WHERE workspace_id = $1`, wid); n != 40 {
		t.Fatalf("usage rows %d, want 40", n)
	}
	// Usage report: $0.10 + $0.06 over 40 calls.
	r := owner.do("GET", "/v1/billing/usage", nil)
	expect(t, r, 200, "")
	if r.num("total_usd") != 0.16 || r.num("total_calls") != 40 {
		t.Fatalf("usage: %s", r.Raw)
	}
	// A YYYY-MM-DD "to" includes that day; each day carries its per-feature split.
	today := time.Now().UTC().Format(time.DateOnly)
	r = owner.do("GET", "/v1/billing/usage?from="+today+"&to="+today, nil)
	expect(t, r, 200, "")
	var rep struct {
		TotalUSD float64 `json:"total_usd"`
		ByDay    []struct {
			Date      string             `json:"date"`
			USD       float64            `json:"usd"`
			ByFeature map[string]float64 `json:"by_feature"`
		} `json:"by_day"`
		ByModel []struct {
			Model  string  `json:"model"`
			USD    float64 `json:"usd"`
			Calls  int     `json:"calls"`
			Source string  `json:"source"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(r.Raw, &rep); err != nil || rep.TotalUSD != 0.16 || len(rep.ByDay) != 1 ||
		rep.ByDay[0].Date != today || rep.ByDay[0].USD != 0.16 || rep.ByDay[0].ByFeature["draft_reply"] != 0.16 ||
		len(rep.ByModel) != 1 || rep.ByModel[0].Model != "test" || rep.ByModel[0].Calls != 40 || rep.ByModel[0].Source != "instance" {
		t.Fatalf("usage today: %s", r.Raw)
	}
}

func TestFromCost(t *testing.T) {
	for _, c := range []struct {
		usd, markup float64
		want        plan.Micro
	}{{0, 1.5, 0}, {0.01, 1, 10000}, {0.002, 1.5, 3000}, {0.0000001, 1.5, 1}, {0.0123456, 2, 24692}, {0.0195, 1, 19500}} {
		if got := plan.FromCost(c.usd, c.markup); got != c.want {
			t.Errorf("FromCost(%v, %v) = %d, want %d", c.usd, c.markup, got, c.want)
		}
	}
}

func TestWorkspaceDeletionCancelsSubscription(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, sub := proWorkspace(t, e)
	expect(t, owner.do("DELETE", "/v1/workspace", map[string]any{"confirm_name": "Acme"}), 204, "")
	var a jobs.CancelSubscriptionArgs
	raw := e.scalar(`SELECT args FROM river_job WHERE kind = 'billing.cancel_subscription' AND args->>'workspace_id' = $1`, wid.String())
	if err := jsonInto(raw, &a); err != nil || a.SubscriptionID != sub.ID || a.CustomerID != sub.Customer {
		t.Fatalf("cancel args %+v %v", a, err)
	}
	if err := billing.RunCancel(context.Background(), e.d, a); err != nil {
		t.Fatal(err)
	}
	if st := e.fake.sub(sub.ID).Status; st != "canceled" {
		t.Fatalf("subscription status %s", st)
	}
	// The cancellation webhook for a deleted workspace is acknowledged.
	expect(t, e.event("evt_deleted_ws", "customer.subscription.deleted", e.fake.subs[sub.ID].json()), 200, "")
}

func TestOnlyOwnersAndAdminsOpenStripe(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, _ := proWorkspace(t, e)
	agent := e.signup("agent")
	expect(t, owner.invite(agent.email, "agent"), 201, "")
	expect(t, agent.do("POST", "/v1/invitations/accept", map[string]any{"token": e.inviteToken(agent.email)}), 200, "")
	agent.workspace = wid.String()
	expect(t, agent.do("GET", "/v1/billing", nil), 200, "")
	expect(t, agent.do("POST", "/v1/billing/portal", nil), 403, "forbidden")
	expect(t, agent.do("POST", "/v1/billing/top-up/checkout", map[string]any{"top_ups": 1}), 403, "forbidden")
	expect(t, agent.do("GET", "/v1/billing/usage", nil), 403, "forbidden")
	expect(t, agent.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 10}), 403, "forbidden")
	expect(t, agent.do("GET", "/v1/billing/invoices", nil), 403, "forbidden")
	// The workspace comes from the session's workspace header, never the body.
	other := e.signup("other")
	other.newWorkspace("Other")
	expect(t, other.do("POST", "/v1/billing/top-up/checkout", map[string]any{"top_ups": 1, "workspace_id": wid.String()}), 422, "validation_failed")
	expect(t, other.do("POST", "/v1/billing/top-up/checkout", map[string]any{"top_ups": 1}), 200, "")
	if got := e.fake.lastSession().Get("metadata[workspace_id]"); got != other.workspace {
		t.Fatalf("checkout for %s, want %s", got, other.workspace)
	}
	if r := e.send(mustReq(t, e.srv.URL+"/v1/billing/portal", nil)); r.Status != 401 {
		t.Fatalf("anonymous portal: %d", r.Status)
	}
}

func TestBillingDisabled(t *testing.T) {
	e := newEnv(t, false)
	owner := e.signup("owner")
	owner.newWorkspace("Self")
	b := owner.do("GET", "/v1/billing", nil)
	expect(t, b, 200, "")
	if b.str("plan") != "self_hosted" || b.Body["billing_enabled"] != false || b.num("limits", "max_mailboxes") != 0 ||
		b.Body["ai"].(map[string]any)["unlimited"] != true || b.Body["prices"].(map[string]any)["pro_month"] != nil {
		t.Fatalf("self-hosted billing: %s", b.Raw)
	}
	expect(t, owner.addMailbox("one@self.test"), 201, "")
	expect(t, owner.addMailbox("two@self.test"), 201, "")
	expect(t, owner.invite("a@self.test", "agent"), 201, "")
	expect(t, owner.invite("b@self.test", "admin"), 201, "")
	for _, c := range []struct{ method, path string }{{"POST", "/v1/billing/checkout"}, {"POST", "/v1/billing/portal"},
		{"POST", "/v1/billing/top-up/checkout"}} {
		var body any
		if c.path == "/v1/billing/top-up/checkout" {
			body = map[string]any{"top_ups": 1}
		}
		expect(t, owner.do(c.method, c.path, body), 404, "billing_disabled")
	}
	expect(t, e.postSigned([]byte(`{}`), whSecret, time.Now()), 404, "billing_disabled")
	expect(t, owner.do("GET", "/v1/billing/usage", nil), 200, "")
	// Members change without seat sync jobs.
	wid := owner.workspace
	if n := e.count(`SELECT count(*) FROM river_job WHERE kind LIKE 'billing.%' AND args->>'workspace_id' = $1`, wid); n != 0 {
		t.Fatalf("billing jobs on a self-hosted instance: %d", n)
	}
}

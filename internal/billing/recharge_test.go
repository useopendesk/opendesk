package billing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"

	"github.com/useopendesk/opendesk/internal/billing"
	"github.com/useopendesk/opendesk/internal/plan"
)

// setBalance leaves exactly `usage` of purchased top-ups and no monthly allowance.
func (e *env) setBalance(wid uuid.UUID, usage plan.Micro) {
	e.t.Helper()
	ent, err := e.d.Plan.Entitlements(context.Background(), e.d.DB.Pool, wid)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(context.Background(), `UPDATE ai_balances SET included_used_micro_usd = $2, purchased_micro_usd = $3,
		period_start = date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' WHERE workspace_id = $1`,
		wid, ent.MonthlyUSD, usage); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) spend(wid uuid.UUID, feature string, usd float64) {
	e.t.Helper()
	if _, err := e.d.Plan.Spend(context.Background(), e.d.DB.Pool, wid, plan.Usage{Feature: feature, Model: "test", CostUSD: usd}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) rechargeJobs(wid uuid.UUID) int64 {
	return e.count(`SELECT count(*) FROM river_job WHERE kind = 'billing.auto_recharge' AND args->>'workspace_id' = $1`, wid.String())
}

func (e *env) notices(wid uuid.UUID, typ string) int64 {
	return e.count(`SELECT count(*) FROM notifications WHERE workspace_id = $1 AND type = $2`, wid, typ)
}

func (e *env) grants(wid uuid.UUID) int64 {
	return e.count(`SELECT count(*) FROM ai_balance_grants WHERE workspace_id = $1`, wid)
}

// recharge runs the auto-recharge job once and reports whether it asked to be tried later.
func (e *env) recharge(wid uuid.UUID) (snoozed bool) {
	e.t.Helper()
	err := billing.RunAutoRecharge(context.Background(), e.d, wid)
	var snooze *rivertype.JobSnoozeError
	if errors.As(err, &snooze) {
		return true
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return false
}

func customerOf(e *env, wid uuid.UUID) string {
	return e.scalar(`SELECT stripe_customer_id FROM subscriptions WHERE workspace_id = $1`, wid).(string)
}

func TestAutoRechargeSettingsNeedAPaymentMethod(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")

	b := owner.do("GET", "/v1/billing", nil)
	expect(t, b, 200, "")
	if b.num("auto_recharge", "threshold_usd") != 5 || b.num("auto_recharge", "amount_usd") != 25 || b.num("auto_recharge", "monthly_limit_usd") != 250 ||
		b.Body["auto_recharge"].(map[string]any)["enabled"] != false || b.num("auto_recharge", "low_at_usd") != 0.1 || b.Body["payment_method"] != nil {
		t.Fatalf("defaults: %s", b.Raw)
	}
	// One recharge must lift the balance above the threshold, and fit the monthly limit.
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"threshold_usd": 50, "amount_usd": 20}), 422, "recharge_too_small")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"threshold_usd": 25}), 422, "recharge_too_small")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"amount_usd": 100, "monthly_limit_usd": 50}), 422, "monthly_limit_too_small")
	// No Stripe customer yet, then a customer without a saved card.
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true}), 409, "payment_method_required")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"amount_usd": 10}), 200, "")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true}), 409, "payment_method_required")

	// A setup Checkout saves a card and makes it the default.
	r := owner.do("POST", "/v1/billing/payment-method", nil)
	expect(t, r, 200, "")
	f := e.fake.lastSession()
	cus := customerOf(e, wid)
	if f.Get("mode") != "setup" || f.Get("currency") != "usd" || f.Get("customer") != cus ||
		f.Get("success_url") != "http://app.test/settings/billing?payment_method=success" {
		t.Fatalf("setup checkout form: %v", f)
	}
	pm := e.fake.addPM(cus)
	e.fake.mu.Lock()
	e.fake.setupPMs["seti_1"] = pm
	e.fake.mu.Unlock()
	expect(t, e.event("evt_setup", "checkout.session.completed", map[string]any{"id": "cs_setup", "object": "checkout.session", "mode": "setup",
		"status": "complete", "customer": cus, "setup_intent": "seti_1", "metadata": map[string]any{"workspace_id": wid.String(), "kind": "payment_method"}}), 200, "")
	e.fake.mu.Lock()
	def := e.fake.defaultPM[cus]
	e.fake.mu.Unlock()
	if def != pm {
		t.Fatalf("default payment method %q, want %q", def, pm)
	}
	// The completed Checkout queued a refresh of the card the app shows.
	if n := e.count(`SELECT count(*) FROM river_job WHERE kind = 'billing.sync_payment_method' AND args->>'workspace_id' = $1`, wid.String()); n != 1 {
		t.Fatalf("payment method sync jobs: %d", n)
	}
	if err := billing.RunSyncPaymentMethod(context.Background(), e.d, wid); err != nil {
		t.Fatal(err)
	}
	b = owner.do("GET", "/v1/billing", nil)
	if b.str("payment_method", "brand") != "visa" || b.str("payment_method", "last4") != "4242" || b.num("payment_method", "exp_month") != 8 ||
		b.num("payment_method", "exp_year") != 2030 {
		t.Fatalf("payment method: %s", b.Raw)
	}

	// Validation and roles.
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"amount_usd": 4}), 422, "")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"amount_usd": 5001}), 422, "")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"threshold_usd": 0}), 422, "")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"threshold_usd": 1000.01}), 422, "")
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"monthly_limit_usd": 50001}), 422, "")

	r = owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true, "threshold_usd": 0.204, "amount_usd": 30, "monthly_limit_usd": 60})
	expect(t, r, 200, "")
	if r.Body["auto_recharge"].(map[string]any)["enabled"] != true || r.num("auto_recharge", "threshold_usd") != 0.2 || r.num("auto_recharge", "amount_usd") != 30 ||
		r.num("auto_recharge", "monthly_limit_usd") != 60 || r.num("auto_recharge", "low_at_usd") != 0.2 {
		t.Fatalf("updated: %s", r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.auto_recharge_updated'`, wid); n != 1 {
		t.Fatalf("audit rows: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'billing.updated'`, wid); n < 1 {
		t.Fatal("no billing.updated event")
	}
	// The free allowance ($0.50) is above the threshold: nothing to recharge yet.
	if n := e.rechargeJobs(wid); n != 0 {
		t.Fatalf("recharge jobs: %d", n)
	}
}

func TestAutoRechargeOnThreshold(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, _ := proWorkspace(t, e)
	cus := customerOf(e, wid)
	pm := e.fake.addPM(cus)
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true, "threshold_usd": 5, "amount_usd": 20, "monthly_limit_usd": 40}), 200, "")
	if n := e.rechargeJobs(wid); n != 0 {
		t.Fatalf("recharge jobs at $10: %d", n)
	}

	// $4 at 1.5x is $6: $10 -> $4, below $5. One job, however many calls.
	e.spend(wid, "draft_reply", 4)
	e.spend(wid, "summarize", 0)
	if n := e.rechargeJobs(wid); n != 1 {
		t.Fatalf("recharge jobs: %d", n)
	}
	if e.recharge(wid) {
		t.Fatal("snoozed")
	}
	// One invoice with one line on the top-up product, paid off-session with the saved card.
	invs, items, pays := e.fake.forms("invoice"), e.fake.forms("item"), e.fake.forms("pay")
	if len(invs) != 1 || len(items) != 1 || len(pays) != 1 {
		t.Fatalf("invoices %d, items %d, payments %d", len(invs), len(items), len(pays))
	}
	for k, want := range map[string]string{"customer": cus, "currency": "usd", "collection_method": "charge_automatically", "auto_advance": "false",
		"pending_invoice_items_behavior": "exclude", "metadata[workspace_id]": wid.String(), "metadata[kind]": "auto_recharge", "metadata[micro_usd]": "20000000"} {
		if got := invs[0].Get(k); got != want {
			t.Errorf("invoice %s = %q, want %q", k, got, want)
		}
	}
	if items[0].Get("price_data[product]") != "prod_ai_topup" || items[0].Get("price_data[unit_amount]") != "2000" || items[0].Get("quantity") != "1" {
		t.Errorf("invoice item: %v", items[0])
	}
	if pays[0].Get("off_session") != "true" || pays[0].Get("payment_method") != pm {
		t.Errorf("invoice pay: %v", pays[0])
	}
	inv := e.fake.lastInvoice()
	if n := e.count(`SELECT count(*) FROM ai_balance_grants WHERE workspace_id = $1 AND external_id = $2 AND source = 'stripe_auto_recharge'`, wid, inv); n != 1 {
		t.Fatalf("grants: %d", n)
	}
	b := owner.do("GET", "/v1/billing", nil)
	if b.num("ai", "purchased_usd") != 20 || b.num("ai", "remaining_usd") != 23.99 || b.num("auto_recharge", "recharges_this_month") != 1 ||
		b.num("auto_recharge", "charged_this_month_usd") != 20 || b.num("auto_recharge", "last_recharge", "usd") != 20 ||
		b.num("auto_recharge", "last_recharge", "amount") != 2000 {
		t.Fatalf("after recharge: %s", b.Raw)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.ai_recharged'`, wid); n != 1 {
		t.Fatalf("recharge audit rows: %d", n)
	}

	// invoice.paid for the same invoice grants nothing more.
	expect(t, e.event("evt_inv_ok", "invoice.paid", e.fake.invoice(inv)), 200, "")
	expect(t, e.event("evt_inv_ok_again", "invoice.paid", e.fake.invoice(inv)), 200, "")
	if n := e.grants(wid); n != 1 {
		t.Fatalf("grants after webhook: %d", n)
	}
	// A retried job above the threshold does nothing.
	e.recharge(wid)
	if len(e.fake.forms("invoice")) != 1 {
		t.Fatal("charged above the threshold")
	}

	// Second recharge of the month, then the monthly limit ($40) holds.
	e.setBalance(wid, 1_000_000)
	e.recharge(wid)
	e.setBalance(wid, 1_000_000)
	e.recharge(wid)
	if n := len(e.fake.forms("invoice")); n != 2 {
		t.Fatalf("invoices past the monthly limit: %d", n)
	}
	// Raising the limit to $50 leaves room for $10: the next recharge adds only that.
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"monthly_limit_usd": 50}), 200, "")
	e.recharge(wid)
	if items := e.fake.forms("item"); len(items) != 3 || items[2].Get("price_data[unit_amount]") != "1000" {
		t.Fatalf("partial recharge: %d items", len(items))
	}
	// At the limit, spending below the threshold queues nothing and, at the end, tells admins.
	if _, err := e.d.DB.Pool.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'billing.auto_recharge' AND args->>'workspace_id' = $1`, wid.String()); err != nil {
		t.Fatal(err)
	}
	e.setBalance(wid, 35_000)      // $0.035
	e.spend(wid, "draft_reply", 0) // typical $0.03: $0.005 left, below $0.01
	if n := e.rechargeJobs(wid); n != 0 {
		t.Fatalf("recharge jobs past the limit: %d", n)
	}
	if n := e.notices(wid, billing.NotifyBalanceOut); n != 1 {
		t.Fatalf("out-of-balance notices: %d", n)
	}

	// The invoices are listed in the app, newest first.
	r := owner.do("GET", "/v1/billing/invoices", nil)
	expect(t, r, 200, "")
	list := r.Body["data"].([]any)
	if len(list) != 3 {
		t.Fatalf("invoices: %s", r.Raw)
	}
	first := list[0].(map[string]any)
	if first["kind"] != "auto_recharge" || first["status"] != "paid" || first["total"].(float64) != 1000 || first["url"] == "" || first["pdf_url"] == "" {
		t.Fatalf("invoice: %v", first)
	}
}

func TestAutoRechargeFailureTurnsItOff(t *testing.T) {
	for _, tc := range []struct{ mode, code string }{{"decline", "card_declined"}, {"auth", "authentication_required"}} {
		t.Run(tc.mode, func(t *testing.T) {
			e := newEnv(t, true)
			owner, wid, _ := proWorkspace(t, e)
			e.fake.addPM(customerOf(e, wid))
			expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true}), 200, "")
			e.fake.setPayMode(tc.mode)
			e.setBalance(wid, 500_000)
			if e.recharge(wid) {
				t.Fatal("a hard decline was snoozed")
			}
			b := owner.do("GET", "/v1/billing", nil)
			ar := b.Body["auto_recharge"].(map[string]any)
			if ar["enabled"] != false || b.str("auto_recharge", "failure", "code") != tc.code || b.num("ai", "purchased_usd") != 0.5 {
				t.Fatalf("after failure: %s", b.Raw)
			}
			if n := e.notices(wid, billing.NotifyAutoRechargeFailed); n != 1 {
				t.Fatalf("failure notices: %d", n)
			}
			if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.auto_recharge_failed'`, wid); n != 1 {
				t.Fatalf("failure audit rows: %d", n)
			}
			// The email follows the owner's notification preferences: on by default, it waits
			// to be sent unless the notice is read first.
			if n := e.count(`SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id
				WHERE n.workspace_id = $1 AND n.type = $2 AND u.email = $3 AND n.email_due_at IS NOT NULL`, wid, billing.NotifyAutoRechargeFailed, owner.email); n != 1 {
				t.Fatalf("failure emails waiting: %d", n)
			}
			if st := e.scalar(`SELECT status FROM ai_auto_recharges WHERE workspace_id = $1`, wid).(string); st != "failed" {
				t.Fatalf("charge status %q", st)
			}
			// The unpaid invoice is voided, so nobody pays it later, and is not listed.
			inv := e.fake.lastInvoice()
			if st := e.fake.invoice(inv)["status"]; st != "void" {
				t.Fatalf("invoice status %v", st)
			}
			if r := owner.do("GET", "/v1/billing/invoices", nil); len(r.Body["data"].([]any)) != 0 {
				t.Fatalf("invoices: %s", r.Raw)
			}
			// Off now: nothing else is charged. Stripe's invoice.payment_failed changes nothing
			// and is not the subscription's payment-failed email.
			e.recharge(wid)
			if n := len(e.fake.forms("pay")); n != 1 {
				t.Fatalf("payments: %d", n)
			}
			expect(t, e.event("evt_inv_fail", "invoice.payment_failed", e.fake.invoice(inv)), 200, "")
			if n := e.notices(wid, billing.NotifyAutoRechargeFailed); n != 1 {
				t.Fatalf("failure notices after webhook: %d", n)
			}
			if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.payment_failed'`, wid); n != 0 {
				t.Fatalf("subscription payment failures: %d", n)
			}
			// Turning it back on clears the failure.
			e.fake.setPayMode("")
			r := owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true})
			expect(t, r, 200, "")
			if r.Body["auto_recharge"].(map[string]any)["failure"] != nil {
				t.Fatalf("failure kept: %s", r.Raw)
			}
			// Still below the threshold, so turning it on queued a recharge.
			if n := e.rechargeJobs(wid); n != 1 {
				t.Fatalf("recharge jobs after turning on: %d", n)
			}
		})
	}
}

func TestAutoRechargeRetriesASoftDecline(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, _ := proWorkspace(t, e)
	e.fake.addPM(customerOf(e, wid))
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true}), 200, "")
	e.fake.setPayMode("soft")
	e.setBalance(wid, 500_000)

	// Insufficient funds: still on, tried again later on the same invoice.
	if !e.recharge(wid) {
		t.Fatal("a soft decline was not snoozed")
	}
	b := owner.do("GET", "/v1/billing", nil)
	if b.Body["auto_recharge"].(map[string]any)["enabled"] != true || b.str("auto_recharge", "retry", "code") != "card_declined" ||
		b.str("auto_recharge", "retry", "at") == "" || b.num("auto_recharge", "recharges_this_month") != 1 {
		t.Fatalf("after a soft decline: %s", b.Raw)
	}
	if n := e.notices(wid, billing.NotifyAutoRechargeFailed); n != 0 {
		t.Fatalf("failure notices while retrying: %d", n)
	}
	e.fake.setPayMode("")
	if e.recharge(wid) {
		t.Fatal("snoozed after the card went through")
	}
	if n := len(e.fake.forms("invoice")); n != 1 {
		t.Fatalf("invoices: %d", n)
	}
	if n := len(e.fake.forms("pay")); n != 2 {
		t.Fatalf("payments: %d", n)
	}
	b = owner.do("GET", "/v1/billing", nil)
	if b.num("ai", "purchased_usd") != 25.5 || b.Body["auto_recharge"].(map[string]any)["retry"] != nil {
		t.Fatalf("after the retry: %s", b.Raw)
	}

	// Declined on every try: after the last one, auto-recharge turns off.
	e.fake.setPayMode("soft")
	e.setBalance(wid, 500_000)
	for i := 0; i < 3; i++ {
		if !e.recharge(wid) {
			t.Fatalf("try %d was not snoozed", i+1)
		}
	}
	if e.recharge(wid) {
		t.Fatal("the last try was snoozed")
	}
	b = owner.do("GET", "/v1/billing", nil)
	if b.Body["auto_recharge"].(map[string]any)["enabled"] != false || b.str("auto_recharge", "failure", "code") != "card_declined" {
		t.Fatalf("after the last try: %s", b.Raw)
	}
	if st := e.fake.invoice(e.fake.lastInvoice())["status"]; st != "void" {
		t.Fatalf("invoice status %v", st)
	}
}

func TestAutoRechargeSettlesByWebhook(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, _ := proWorkspace(t, e)
	cus := customerOf(e, wid)
	e.fake.addPM(cus)
	expect(t, owner.do("PATCH", "/v1/billing/auto-recharge", map[string]any{"enabled": true}), 200, "")
	e.fake.setPayMode("open")
	e.setBalance(wid, 100_000)
	e.recharge(wid)
	if n := e.grants(wid); n != 0 {
		t.Fatalf("granted while processing: %d", n)
	}
	// A pending charge counts against the limit and is not charged twice.
	if b := owner.do("GET", "/v1/billing", nil); b.num("auto_recharge", "recharges_this_month") != 1 || b.num("auto_recharge", "charged_this_month_usd") != 25 {
		t.Fatalf("pending recharge not counted: %s", b.Raw)
	}
	inv := e.fake.invoice(e.fake.lastInvoice())
	inv["status"] = "paid"
	expect(t, e.event("evt_inv_processing_ok", "invoice.paid", inv), 200, "")
	if b := owner.do("GET", "/v1/billing", nil); b.num("ai", "purchased_usd") != 25.1 || b.num("auto_recharge", "last_recharge", "usd") != 25 {
		t.Fatalf("after webhook: %s", b.Raw)
	}
	// An invoice from a top-up Checkout is granted with its session, not here.
	expect(t, e.event("evt_inv_checkout", "invoice.paid", map[string]any{"id": "in_checkout", "object": "invoice", "status": "paid",
		"total": 1000, "currency": "usd", "customer": cus, "metadata": map[string]any{"workspace_id": wid.String(), "kind": "top_up"}}), 200, "")
	if n := e.grants(wid); n != 1 {
		t.Fatalf("grants after a checkout invoice: %d", n)
	}
}

func TestTopUpWithTheSavedCard(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	expect(t, owner.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 50}), 409, "payment_method_required")
	expect(t, owner.do("POST", "/v1/billing/top-up/checkout", map[string]any{"amount_usd": 10}), 200, "")
	pm := e.fake.addPM(customerOf(e, wid))
	expect(t, owner.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 4.99}), 422, "")
	expect(t, owner.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 5000.01}), 422, "")

	r := owner.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 42.5})
	expect(t, r, 200, "")
	if r.str("status") != "paid" || r.num("usd") != 42.5 || r.num("billing", "ai", "purchased_usd") != 42.5 {
		t.Fatalf("top-up: %s", r.Raw)
	}
	pays := e.fake.forms("pay")
	if len(pays) != 1 || pays[0].Get("off_session") != "false" || pays[0].Get("payment_method") != pm {
		t.Fatalf("payments: %v", pays)
	}
	if f := e.fake.forms("invoice")[0]; f.Get("metadata[kind]") != "top_up" || f.Get("metadata[micro_usd]") != "42500000" {
		t.Fatalf("invoice form: %v", f)
	}
	if items := e.fake.forms("item"); items[0].Get("price_data[unit_amount]") != "4250" {
		t.Fatalf("invoice item: %v", items[0])
	}
	inv := e.fake.lastInvoice()
	if n := e.count(`SELECT count(*) FROM ai_balance_grants WHERE workspace_id = $1 AND external_id = $2 AND source = 'stripe_top_up'`, wid, inv); n != 1 {
		t.Fatalf("grants: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'billing.ai_top_up_purchased'`, wid); n != 1 {
		t.Fatalf("audit rows: %d", n)
	}
	// Top-ups do not count toward auto-recharge's monthly limit.
	if b := owner.do("GET", "/v1/billing", nil); b.num("auto_recharge", "charged_this_month_usd") != 0 {
		t.Fatalf("top-up counted as a recharge: %s", b.Raw)
	}
	expect(t, e.event("evt_topup_paid", "invoice.paid", e.fake.invoice(inv)), 200, "")
	if n := e.grants(wid); n != 1 {
		t.Fatalf("grants after webhook: %d", n)
	}

	// Declined, or the bank wants the buyer to confirm: nothing is charged or granted.
	for mode, code := range map[string]string{"decline": "card_declined", "auth": "authentication_required"} {
		e.fake.setPayMode(mode)
		expect(t, owner.do("POST", "/v1/billing/top-up", map[string]any{"amount_usd": 10}), 402, code)
		if st := e.fake.invoice(e.fake.lastInvoice())["status"]; st != "void" {
			t.Fatalf("%s: invoice status %v", mode, st)
		}
	}
	if n := e.grants(wid); n != 1 {
		t.Fatalf("grants after declines: %d", n)
	}

}

func TestPaymentMethodEventsRefreshTheCard(t *testing.T) {
	e := newEnv(t, true)
	owner, wid, _ := proWorkspace(t, e)
	cus := customerOf(e, wid)
	pm := e.fake.addPM(cus)
	expect(t, e.event("evt_pm_attached", "payment_method.attached", map[string]any{"id": pm, "object": "payment_method", "type": "card",
		"customer": cus}), 200, "")
	expect(t, e.event("evt_cus_updated", "customer.updated", map[string]any{"id": cus, "object": "customer",
		"metadata": map[string]any{"workspace_id": wid.String()}}), 200, "")
	if n := e.count(`SELECT count(*) FROM river_job WHERE kind = 'billing.sync_payment_method' AND args->>'workspace_id' = $1`, wid.String()); n != 1 {
		t.Fatalf("payment method sync jobs: %d", n)
	}
	if err := billing.RunSyncPaymentMethod(context.Background(), e.d, wid); err != nil {
		t.Fatal(err)
	}
	if b := owner.do("GET", "/v1/billing", nil); b.str("payment_method", "last4") != "4242" {
		t.Fatalf("payment method: %s", b.Raw)
	}
	// The card became the customer's default.
	e.fake.mu.Lock()
	def := e.fake.defaultPM[cus]
	e.fake.mu.Unlock()
	if def != pm {
		t.Fatalf("default payment method %q", def)
	}
}

func TestRunningLowNotices(t *testing.T) {
	e := newEnv(t, true)
	owner := e.signup("owner")
	wid := owner.newWorkspace("Acme")
	// Free: $0.50, so low is below $0.10 (a fifth of the allowance) and out below $0.01.
	e.spend(wid, "draft_reply", 0.2) // $0.30: $0.20 left
	if n := e.notices(wid, billing.NotifyBalanceLow); n != 0 {
		t.Fatalf("low notices at $0.20: %d", n)
	}
	e.spend(wid, "draft_reply", 0.08) // $0.12: $0.08 left
	e.spend(wid, "summarize", 0)      // typical $0.01: $0.07 left
	if n := e.notices(wid, billing.NotifyBalanceLow); n != 1 {
		t.Fatalf("low notices: %d", n)
	}
	e.spend(wid, "draft_reply", 0.04) // $0.06: $0.01 left, not out yet
	if n := e.notices(wid, billing.NotifyBalanceOut); n != 0 {
		t.Fatalf("out notices at $0.01: %d", n)
	}
	e.spend(wid, "summarize", 0) // typical $0.01: $0 left
	if n := e.notices(wid, billing.NotifyBalanceOut); n != 1 {
		t.Fatalf("out notices: %d", n)
	}
	if n := e.notices(wid, billing.NotifyBalanceLow); n != 1 {
		t.Fatalf("low notices after out: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE n.workspace_id = $1 AND n.type = $2 AND u.email = $3 AND n.email_due_at IS NOT NULL`, wid, billing.NotifyBalanceOut, owner.email); n != 1 {
		t.Fatalf("out emails waiting: %d", n)
	}
	if v := e.scalar(`SELECT data->>'remaining_usd' FROM notifications WHERE workspace_id = $1 AND type = $2`, wid, billing.NotifyBalanceOut).(string); v != "0" {
		t.Fatalf("out notice remaining_usd %q", v)
	}
	// A purchase re-arms the notices.
	cs := map[string]any{"id": "cs_rearm_" + wid.String(), "object": "checkout.session", "mode": "payment", "payment_status": "paid",
		"client_reference_id": wid.String(), "metadata": map[string]any{"workspace_id": wid.String(), "kind": "top_up", "micro_usd": "10000000"}}
	expect(t, e.event("evt_rearm", "checkout.session.completed", cs), 200, "")
	if v := e.scalar(`SELECT out_notified_at IS NULL AND low_notified_at IS NULL FROM ai_auto_recharge WHERE workspace_id = $1`, wid).(bool); !v {
		t.Fatal("notices not re-armed after a purchase")
	}
}

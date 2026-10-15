package billing

// Auto-recharge, top-ups paid with the saved payment method, and running-low handling for
// the AI balance (contracts/billing.md "AI top-ups" and "Auto-recharge").
//
// Every charge on the saved payment method is a Stripe invoice that billing creates,
// finalizes and pays, so the customer has an invoice for each one. Its AI usage is granted
// once per invoice, by whichever sees it paid first: the request or worker that paid it,
// or the invoice.paid webhook. When a charged AI call takes a workspace below its
// threshold, a unique River job recharges it; a soft decline is tried again a few times
// before auto-recharge turns itself off. With auto-recharge off (or at its monthly limit),
// owners and admins are told once when the balance runs low and once when it runs out.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stripe/stripe-go/v87"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/plan"
)

// Amounts and bounds (the ai_auto_recharge columns). AI usage is bought dollar for dollar.
const (
	DefaultRechargeThreshold = plan.Micro(5_000_000)      // $5
	DefaultRechargeAmount    = plan.Micro(25_000_000)     // $25
	DefaultRechargeLimit     = plan.Micro(250_000_000)    // $250 a month
	MinRechargeThreshold     = plan.Micro(10_000)         // $0.01
	MaxRechargeThreshold     = plan.Micro(1_000_000_000)  // $1,000
	MaxRechargeLimit         = plan.Micro(50_000_000_000) // $50,000 a month
	// MinTopUp and MaxTopUp bound one purchase or recharge.
	MinTopUp = plan.Micro(5_000_000)     // $5
	MaxTopUp = plan.Micro(5_000_000_000) // $5,000
)

// retryAfter spaces the tries of a recharge the bank declined for now (insufficient funds,
// the issuer unavailable): three more over about a day, then auto-recharge turns off.
var retryAfter = []time.Duration{time.Hour, 6 * time.Hour, 18 * time.Hour}

// OutBelow is the balance under which a workspace is out of AI balance: no metered call
// can start (plan.MinBalance), and the widget agent goes silent.
const OutBelow = plan.MinBalance

// Notification types sent to owners and admins.
const (
	NotifyBalanceLow         = "ai_balance_low"
	NotifyBalanceOut         = "ai_balance_out"
	NotifyAutoRechargeFailed = "auto_recharge_failed"
)

// Charge kinds (ai_auto_recharges.kind and the Stripe metadata kind) and grant sources.
const (
	topUpKind      = "top_up"
	rechargeKind   = "auto_recharge"
	topUpSource    = "stripe_top_up"
	rechargeSource = "stripe_auto_recharge"
)

// Why a charge failed (RechargeFailure.code).
const (
	failCardDeclined         = "card_declined"
	failAuthenticationNeeded = "authentication_required"
	failNoPaymentMethod      = "no_payment_method"
	failPaymentFailed        = "payment_failed"
	failUnavailable          = "unavailable"
)

const (
	errPaymentMethodRequired  = "payment_method_required"
	paymentMethodRequiredText = "Add a payment method first."
)

func errTopUpUnavailable() *httpx.Error {
	return httpx.E(422, "top_up_unavailable", "AI top-ups are not available.")
}

// usdText formats an amount for people: "$10", "$2.50", "$0.004".
func usdText(m plan.Micro) string {
	switch {
	case m%1_000_000 == 0:
		return fmt.Sprintf("$%d", m/1_000_000)
	case m%10_000 == 0:
		return fmt.Sprintf("$%.2f", m.USD())
	}
	return "$" + strconv.FormatFloat(m.USD(), 'f', -1, 64)
}

// cents rounds US dollars people typed to cents.
func cents(usd float64) plan.Micro {
	return plan.FromUSD(math.Round(usd*100) / 100)
}

// topUpAmount checks the AI usage one purchase or recharge adds.
func topUpAmount(usd float64, field string) (plan.Micro, error) {
	m := cents(usd)
	if m < MinTopUp || m > MaxTopUp {
		return 0, httpx.Validation(field + " must be from 5 to 5000.")
	}
	return m, nil
}

// amountFor is what usd of AI usage costs, in the top-up price's currency and minor unit:
// the price (STRIPE_PRICE_AI_TOPUP) is the cost of plan.TopUpUSD.
func amountFor(usd plan.Micro, p *Price) int64 {
	return int64(math.Round(float64(usd) * float64(p.UnitAmount) / float64(plan.TopUpUSD)))
}

// AutoRecharge is Billing.auto_recharge: the settings and what happened last.
type AutoRecharge struct {
	Enabled             bool             `json:"enabled"`
	ThresholdUSD        float64          `json:"threshold_usd" doc:"Recharge when the AI balance falls below this many US dollars"`
	AmountUSD           float64          `json:"amount_usd" doc:"AI usage each recharge adds, in US dollars (charged dollar for dollar)"`
	MonthlyLimitUSD     float64          `json:"monthly_limit_usd" doc:"At most this many US dollars are recharged per calendar month (UTC)"`
	ChargedThisMonthUSD float64          `json:"charged_this_month_usd" doc:"Recharged this month, paid or still being paid"`
	RechargesThisMonth  int              `json:"recharges_this_month"`
	LowAtUSD            float64          `json:"low_at_usd" doc:"An AI balance below this many US dollars reads as running low"`
	LastRecharge        *Recharge        `json:"last_recharge"`
	Retry               *RechargeRetry   `json:"retry" doc:"A recharge the bank declined for now, tried again at this time"`
	Failure             *RechargeFailure `json:"failure" doc:"Why auto-recharge turned itself off; null once it is back on"`
}

type Recharge struct {
	At       time.Time `json:"at"`
	USD      float64   `json:"usd" doc:"AI usage added, in US dollars"`
	Amount   int64     `json:"amount" doc:"What was charged, in the currency's minor unit"`
	Currency string    `json:"currency"`
}

type RechargeRetry struct {
	At   time.Time `json:"at"`
	Code string    `json:"code" enum:"card_declined"`
}

type RechargeFailure struct {
	Code string    `json:"code" enum:"card_declined,authentication_required,no_payment_method,payment_failed,unavailable"`
	At   time.Time `json:"at"`
}

// rechargeSettings is one ai_auto_recharge row (or the defaults).
type rechargeSettings struct {
	Enabled                  bool
	Threshold, Amount, Limit plan.Micro
	FailureCode              *string
	FailedAt                 *time.Time
	LowNotified, OutNotified *time.Time
}

func defaultSettings() rechargeSettings {
	return rechargeSettings{Threshold: DefaultRechargeThreshold, Amount: DefaultRechargeAmount, Limit: DefaultRechargeLimit}
}

func readSettings(ctx context.Context, q db.Querier, wid uuid.UUID, forUpdate bool) (rechargeSettings, error) {
	st := defaultSettings()
	sql := `SELECT enabled, threshold_micro_usd, amount_micro_usd, monthly_limit_micro_usd, failure_code, failed_at, low_notified_at, out_notified_at
		FROM ai_auto_recharge WHERE workspace_id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	err := q.QueryRow(ctx, sql, wid).Scan(&st.Enabled, &st.Threshold, &st.Amount, &st.Limit, &st.FailureCode, &st.FailedAt,
		&st.LowNotified, &st.OutNotified)
	if db.IsNotFound(err) {
		return defaultSettings(), nil
	}
	return st, err
}

func ensureSettings(ctx context.Context, q db.Querier, wid uuid.UUID) error {
	_, err := q.Exec(ctx, `INSERT INTO ai_auto_recharge (workspace_id) VALUES ($1) ON CONFLICT DO NOTHING`, wid)
	return err
}

func monthStart(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// rechargedThisMonth is what auto-recharge charged or may still charge this month: the
// AI usage and the number of recharges.
func rechargedThisMonth(ctx context.Context, q db.Querier, wid uuid.UUID) (plan.Micro, int, error) {
	var sum plan.Micro
	var n int
	err := q.QueryRow(ctx, `SELECT COALESCE(sum(micro_usd), 0)::bigint, count(*)::int FROM ai_auto_recharges
		WHERE workspace_id = $1 AND kind = 'auto_recharge' AND status IN ('pending', 'succeeded') AND created_at >= $2`,
		wid, monthStart(time.Now())).Scan(&sum, &n)
	return sum, n, err
}

// LowAt is the balance under which a workspace is running low: the auto-recharge
// threshold, but never more than a fifth of the monthly allowance (at least $0.05), so
// a small allowance does not read as low from its first call.
func LowAt(threshold, monthly plan.Micro) plan.Micro {
	if threshold <= 0 {
		threshold = DefaultRechargeThreshold
	}
	return min(threshold, max(monthly/5, lowAtFloor))
}

// lowAtFloor is the smallest running-low line.
const lowAtFloor = plan.Micro(50_000) // $0.05

func loadAutoRecharge(ctx context.Context, q db.Querier, wid uuid.UUID, monthly plan.Micro) (*AutoRecharge, error) {
	st, err := readSettings(ctx, q, wid, false)
	if err != nil {
		return nil, err
	}
	ar := &AutoRecharge{Enabled: st.Enabled, ThresholdUSD: st.Threshold.USD(), AmountUSD: st.Amount.USD(), MonthlyLimitUSD: st.Limit.USD(),
		LowAtUSD: LowAt(st.Threshold, monthly).USD()}
	if st.Enabled {
		ar.LowAtUSD = st.Threshold.USD()
	}
	charged, n, err := rechargedThisMonth(ctx, q, wid)
	if err != nil {
		return nil, err
	}
	ar.ChargedThisMonthUSD, ar.RechargesThisMonth = charged.USD(), n
	var r Recharge
	var usage plan.Micro
	err = q.QueryRow(ctx, `SELECT updated_at, micro_usd, amount, currency FROM ai_auto_recharges
		WHERE workspace_id = $1 AND kind = 'auto_recharge' AND status = 'succeeded' ORDER BY updated_at DESC LIMIT 1`, wid).
		Scan(&r.At, &usage, &r.Amount, &r.Currency)
	switch {
	case err == nil:
		r.USD = usage.USD()
		ar.LastRecharge = &r
	case !db.IsNotFound(err):
		return nil, err
	}
	if st.Enabled {
		var at time.Time
		var code string
		err = q.QueryRow(ctx, `SELECT retry_at, COALESCE(failure_code, 'card_declined') FROM ai_auto_recharges
			WHERE workspace_id = $1 AND kind = 'auto_recharge' AND status = 'pending' AND retry_at IS NOT NULL
			ORDER BY created_at DESC LIMIT 1`, wid).Scan(&at, &code)
		switch {
		case err == nil:
			ar.Retry = &RechargeRetry{At: at, Code: code}
		case !db.IsNotFound(err):
			return nil, err
		}
	}
	if !st.Enabled && st.FailureCode != nil && st.FailedAt != nil {
		ar.Failure = &RechargeFailure{Code: *st.FailureCode, At: *st.FailedAt}
	}
	return ar, nil
}

func rechargeOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: jobs.QueueDefault, MaxAttempts: 6,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: uniqueStates}}
}

// ─── API ───────────────────────────────────────────────────────────────────

// TopUpResult is the POST /v1/billing/top-up body.
type TopUpResult struct {
	// Status is "paid" (the AI usage is in the balance) or "processing" (the payment is
	// still being confirmed; billing.updated follows when it lands).
	Status  string  `json:"status" enum:"paid,processing"`
	USD     float64 `json:"usd" doc:"AI usage bought, in US dollars"`
	Billing Billing `json:"billing"`
}

func (s *svc) registerRechargeAPI(api huma.API) {
	tags := []string{"Billing"}

	huma.Register(api, huma.Operation{OperationID: "update-billing-auto-recharge", Method: http.MethodPatch, Path: "/v1/billing/auto-recharge",
		Summary: "Change the AI balance auto-recharge settings", Tags: tags},
		func(ctx context.Context, in *struct {
			Body struct {
				Enabled         *bool    `json:"enabled,omitempty"`
				ThresholdUSD    *float64 `json:"threshold_usd,omitempty" minimum:"0.01" maximum:"1000" doc:"Recharge when the AI balance falls below this many US dollars (rounded to cents)"`
				AmountUSD       *float64 `json:"amount_usd,omitempty" minimum:"5" maximum:"5000" doc:"AI usage each recharge adds, in US dollars (rounded to cents)"`
				MonthlyLimitUSD *float64 `json:"monthly_limit_usd,omitempty" minimum:"5" maximum:"50000" doc:"Most recharged per calendar month (UTC), in US dollars"`
			}
		}) (*struct{ Body Billing }, error) {
			if !s.enabled() {
				return nil, errDisabled()
			}
			p, err := requireBillingAdmin(ctx)
			if err != nil {
				return nil, err
			}
			wid := p.WorkspaceID
			if !s.d.Limiter.Allow("billing:auto_recharge:"+wid.String(), 120, time.Hour) {
				return nil, httpx.RateLimited()
			}
			body := in.Body
			cur, err := readSettings(ctx, s.d.DB.Pool, wid, false)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if body.Enabled != nil && *body.Enabled && !cur.Enabled {
				if s.d.Cfg.StripePriceTopUp == "" {
					return nil, errTopUpUnavailable()
				}
				pm, err := s.savedPaymentMethod(ctx, wid)
				if err != nil {
					return nil, s.wrapStripe(err)
				}
				if pm == "" {
					return nil, httpx.E(409, errPaymentMethodRequired, paymentMethodRequiredText)
				}
			}
			prices := s.pricesFor(ctx)
			var out Billing
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := plan.Ensure(ctx, tx, wid); err != nil {
					return err
				}
				if err := ensureSettings(ctx, tx, wid); err != nil {
					return err
				}
				st, err := readSettings(ctx, tx, wid, true)
				if err != nil {
					return err
				}
				next := st
				if body.Enabled != nil {
					next.Enabled = *body.Enabled
				}
				if body.ThresholdUSD != nil {
					// Cents: the threshold is a line people set, not a metered amount.
					if next.Threshold = cents(*body.ThresholdUSD); next.Threshold < MinRechargeThreshold || next.Threshold > MaxRechargeThreshold {
						return httpx.Validation("threshold_usd must be from 0.01 to 1000.")
					}
				}
				if body.AmountUSD != nil {
					if next.Amount, err = topUpAmount(*body.AmountUSD, "amount_usd"); err != nil {
						return err
					}
				}
				if body.MonthlyLimitUSD != nil {
					if next.Limit = cents(*body.MonthlyLimitUSD); next.Limit < MinTopUp || next.Limit > MaxRechargeLimit {
						return httpx.Validation("monthly_limit_usd must be from 5 to 50000.")
					}
				}
				// One recharge must lift the balance back above the threshold, or every
				// charged call after it would buy again until the monthly limit.
				if next.Amount <= next.Threshold {
					return httpx.E(422, "recharge_too_small", "Each recharge must bring your AI balance back above the threshold. Recharge more or lower the threshold.")
				}
				if next.Limit < next.Amount {
					return httpx.E(422, "monthly_limit_too_small", "The monthly limit must cover at least one recharge.")
				}
				changes := map[string]any{}
				if next.Enabled != st.Enabled {
					changes["enabled"] = next.Enabled
				}
				if next.Threshold != st.Threshold {
					changes["threshold_usd"] = next.Threshold.USD()
				}
				if next.Amount != st.Amount {
					changes["amount_usd"] = next.Amount.USD()
				}
				if next.Limit != st.Limit {
					changes["monthly_limit_usd"] = next.Limit.USD()
				}
				if len(changes) > 0 {
					// Turning it on clears the last failure.
					if _, err := tx.Exec(ctx, `UPDATE ai_auto_recharge SET enabled = $2, threshold_micro_usd = $3, amount_micro_usd = $4,
						monthly_limit_micro_usd = $5,
						failure_code = CASE WHEN $2 AND NOT enabled THEN NULL ELSE failure_code END,
						failed_at = CASE WHEN $2 AND NOT enabled THEN NULL ELSE failed_at END,
						updated_at = now() WHERE workspace_id = $1`, wid, next.Enabled, next.Threshold, next.Amount, next.Limit); err != nil {
						return err
					}
					if err := audit.Log(ctx, tx, p, wid, "billing.auto_recharge_updated", "workspace", &wid, changes); err != nil {
						return err
					}
				}
				if next.Enabled {
					// Already below the threshold: recharge now.
					c, err := s.d.Plan.Balance(ctx, tx, wid)
					if err != nil {
						return err
					}
					if !c.Unlimited && c.Remaining < next.Threshold {
						if _, err := s.d.Jobs.InsertTx(ctx, tx, jobs.AutoRechargeArgs{WorkspaceID: wid}, rechargeOpts()); err != nil {
							return err
						}
					}
				}
				if len(changes) > 0 {
					if err := s.publish(ctx, tx, wid, prices); err != nil {
						return err
					}
				}
				out, err = s.load(ctx, tx, wid, prices)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body Billing }{out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-top-up", Method: http.MethodPost, Path: "/v1/billing/top-up",
		Summary: "Buy AI usage with the saved payment method", Tags: tags},
		func(ctx context.Context, in *struct {
			Body struct {
				AmountUSD float64 `json:"amount_usd" minimum:"5" maximum:"5000" doc:"AI usage to buy, in US dollars (rounded to cents), charged dollar for dollar"`
			}
		}) (*struct{ Body TopUpResult }, error) {
			if !s.enabled() {
				return nil, errDisabled()
			}
			p, err := requireBillingAdmin(ctx)
			if err != nil {
				return nil, err
			}
			wid := p.WorkspaceID
			usd, err := topUpAmount(in.Body.AmountUSD, "amount_usd")
			if err != nil {
				return nil, err
			}
			if s.d.Cfg.StripePriceTopUp == "" {
				return nil, errTopUpUnavailable()
			}
			if !s.d.Limiter.Allow("billing:top_up:"+wid.String(), 20, time.Hour) {
				return nil, httpx.RateLimited()
			}
			price := s.pricesFor(ctx).AITopUp
			if price == nil || price.UnitAmount <= 0 {
				return nil, httpx.E(http.StatusBadGateway, "billing_unavailable", "The billing provider is unavailable. Try again in a moment.")
			}
			cus, err := s.customerID(ctx, s.d.DB.Pool, wid)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			pm := ""
			if cus != "" {
				if pm, err = s.paymentMethod(ctx, cus); err != nil {
					return nil, s.wrapStripe(err)
				}
			}
			if pm == "" {
				return nil, httpx.E(409, errPaymentMethodRequired, paymentMethodRequiredText)
			}
			c, err := newCharge(ctx, s.d.DB.Pool, wid, topUpKind, cus, pm, usd, price)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			// The buyer is here, so the bank may ask them to confirm (off_session false).
			out, err := s.pay(ctx, wid, c, price, false)
			if err != nil {
				return nil, s.wrapStripe(err)
			}
			if out.declined != "" {
				s.abandon(ctx, c, out, out.declined)
				if out.declined == failAuthenticationNeeded {
					return nil, httpx.E(http.StatusPaymentRequired, failAuthenticationNeeded,
						"Your bank wants to confirm this payment. Continue in Stripe Checkout to confirm it.")
				}
				return nil, httpx.E(http.StatusPaymentRequired, failCardDeclined, "The card was declined, so nothing was charged. Try another card.")
			}
			res := TopUpResult{Status: "processing", USD: usd.USD()}
			if out.paid {
				res.Status = "paid"
				if err := s.grantPaid(ctx, wid, c.ID, out.invoice.ID, nil); err != nil {
					return nil, httpx.Wrap(err)
				}
			}
			if res.Billing, err = s.load(ctx, s.d.DB.Pool, wid, s.pricesFor(ctx)); err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body TopUpResult }{res}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-billing-payment-method", Method: http.MethodPost, Path: "/v1/billing/payment-method",
		Summary: "Start a Stripe Checkout that saves a payment method", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*urlBody, error) {
			if !s.enabled() {
				return nil, errDisabled()
			}
			p, err := requireBillingAdmin(ctx)
			if err != nil {
				return nil, err
			}
			if !s.d.Limiter.Allow("billing:checkout:"+p.WorkspaceID.String(), 30, time.Hour) {
				return nil, httpx.RateLimited()
			}
			cus, err := s.customer(ctx, p.WorkspaceID)
			if err != nil {
				return nil, s.wrapStripe(err)
			}
			currency := "usd"
			if pack := s.pricesFor(ctx).AITopUp; pack != nil && pack.Currency != "" {
				currency = pack.Currency
			}
			wid := p.WorkspaceID.String()
			meta := map[string]string{"workspace_id": wid, "kind": "payment_method"}
			sess, err := s.sc.V1CheckoutSessions.Create(ctx, &stripe.CheckoutSessionCreateParams{
				Mode:              stripe.String(string(stripe.CheckoutSessionModeSetup)),
				Customer:          stripe.String(cus),
				ClientReferenceID: stripe.String(wid),
				Currency:          stripe.String(currency),
				SuccessURL:        stripe.String(s.appURL("payment_method", "success")),
				CancelURL:         stripe.String(s.appURL("payment_method", "canceled")),
				Metadata:          meta,
				SetupIntentData:   &stripe.CheckoutSessionCreateSetupIntentDataParams{Metadata: meta},
			})
			if err != nil {
				return nil, s.wrapStripe(err)
			}
			return newURL(sess.URL), nil
		})
}

// customerID is the workspace's Stripe customer, or "" before any purchase.
func (s *svc) customerID(ctx context.Context, q db.Querier, wid uuid.UUID) (string, error) {
	var cus *string
	err := q.QueryRow(ctx, `SELECT stripe_customer_id FROM subscriptions WHERE workspace_id = $1`, wid).Scan(&cus)
	if db.IsNotFound(err) || cus == nil {
		return "", nil
	}
	return *cus, err
}

// savedPaymentMethod is the payment method charges for wid use, or "" when none is saved.
func (s *svc) savedPaymentMethod(ctx context.Context, wid uuid.UUID) (string, error) {
	cus, err := s.customerID(ctx, s.d.DB.Pool, wid)
	if err != nil || cus == "" {
		return "", err
	}
	return s.paymentMethod(ctx, cus)
}

// paymentMethod is the payment method a charge uses: the customer's default, else the
// most recently saved one. "" means none is saved.
func (s *svc) paymentMethod(ctx context.Context, cus string) (string, error) {
	c, err := s.sc.V1Customers.Retrieve(ctx, cus, nil)
	if isMissing(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if c.Deleted {
		return "", nil
	}
	if c.InvoiceSettings != nil && c.InvoiceSettings.DefaultPaymentMethod != nil && c.InvoiceSettings.DefaultPaymentMethod.ID != "" {
		return c.InvoiceSettings.DefaultPaymentMethod.ID, nil
	}
	params := &stripe.CustomerListPaymentMethodsParams{Customer: stripe.String(cus)}
	params.Limit = stripe.Int64(1)
	params.Single = true
	for pm, err := range s.sc.V1Customers.ListPaymentMethods(ctx, params).All(ctx) {
		if err != nil {
			if isMissing(err) {
				return "", nil
			}
			return "", err
		}
		return pm.ID, nil
	}
	return "", nil
}

// ensureDefaultPM makes a saved payment method the customer's default, so the portal
// and charges use it: pm when given (a setup Checkout), else the most recent one when the
// customer has no default yet. Best effort: failures are logged.
func (s *svc) ensureDefaultPM(ctx context.Context, cus, pm string) {
	if cus == "" {
		return
	}
	if pm == "" {
		c, err := s.sc.V1Customers.Retrieve(ctx, cus, nil)
		if err != nil {
			if !isMissing(err) {
				s.d.Log.Warn("stripe customer", "err", stripeErr(err))
			}
			return
		}
		if c.Deleted || (c.InvoiceSettings != nil && c.InvoiceSettings.DefaultPaymentMethod != nil) {
			return
		}
		if pm, err = s.paymentMethod(ctx, cus); err != nil || pm == "" {
			if err != nil {
				s.d.Log.Warn("stripe payment methods", "err", stripeErr(err))
			}
			return
		}
	}
	if _, err := s.sc.V1Customers.Update(ctx, cus, &stripe.CustomerUpdateParams{
		InvoiceSettings: &stripe.CustomerUpdateInvoiceSettingsParams{DefaultPaymentMethod: stripe.String(pm)}}); err != nil {
		s.d.Log.Warn("stripe default payment method", "err", stripeErr(err))
	}
}

// ─── Spend hook ────────────────────────────────────────────────────────────

// onSpend runs after every charged AI call (plan.Service.OnSpend). It never fails the
// call: problems are logged.
func (s *svc) onSpend(ctx context.Context, wid uuid.UUID, before, after plan.Micro) {
	if !s.enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.afterSpend(ctx, wid, before, after); err != nil {
		s.d.Log.Warn("ai balance after spend", "workspace_id", wid, "err", err)
	}
}

func (s *svc) afterSpend(ctx context.Context, wid uuid.UUID, before, after plan.Micro) error {
	pool := s.d.DB.Pool
	st, err := readSettings(ctx, pool, wid, false)
	if err != nil {
		return err
	}
	threshold := st.Threshold
	// LowAt never exceeds the threshold, so nothing below happens above it.
	if after >= max(threshold, OutBelow) {
		return nil
	}
	if st.Enabled && after < threshold {
		charged, _, err := rechargedThisMonth(ctx, pool, wid)
		if err != nil {
			return err
		}
		if st.Limit-charged >= MinTopUp {
			if s.d.Jobs == nil {
				return nil
			}
			_, err := s.d.Jobs.Insert(ctx, jobs.AutoRechargeArgs{WorkspaceID: wid}, rechargeOpts())
			return err
		}
		// At its monthly limit, auto-recharge waits for next month: the notices below apply.
	}
	if before >= OutBelow && after < OutBelow {
		return s.notifyOnce(ctx, wid, NotifyBalanceOut, after)
	}
	e, err := s.d.Plan.Entitlements(ctx, pool, wid)
	if err != nil {
		return err
	}
	low := LowAt(st.Threshold, e.MonthlyUSD)
	if st.Enabled {
		low = threshold
	}
	if before >= low && after < low && after >= OutBelow {
		return s.notifyOnce(ctx, wid, NotifyBalanceLow, after)
	}
	return nil
}

// notifyOnce sends a running-low or out-of-balance notice to owners and admins, once
// per period: a grant or a new month re-arms it.
func (s *svc) notifyOnce(ctx context.Context, wid uuid.UUID, typ string, remaining plan.Micro) error {
	col := "low_notified_at"
	if typ == NotifyBalanceOut {
		col = "out_notified_at"
	}
	prices := s.pricesFor(ctx)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureSettings(ctx, tx, wid); err != nil {
			return err
		}
		st, err := readSettings(ctx, tx, wid, true)
		if err != nil {
			return err
		}
		sent := st.LowNotified
		if typ == NotifyBalanceOut {
			sent = st.OutNotified
		}
		if sent != nil && !sent.Before(monthStart(time.Now())) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE ai_auto_recharge SET `+col+` = now() WHERE workspace_id = $1`, wid); err != nil {
			return err
		}
		// Owners and admins hear of it in the app and, by their notification preferences,
		// by email (internal/emails has the email's words).
		if err := s.notifyAdmins(ctx, tx, wid, typ, map[string]any{"remaining_usd": remaining.USD()}); err != nil {
			return err
		}
		// Spends publish nothing; this lets the app's running-low banner appear live.
		return s.publish(ctx, tx, wid, prices)
	})
}

// notifyAdmins sends a notification to every owner and admin. Its email follows each
// one's notification preferences (contracts/api.md "Notification preferences and emails").
func (s *svc) notifyAdmins(ctx context.Context, tx pgx.Tx, wid uuid.UUID, typ string, data map[string]any) error {
	rows, err := tx.Query(ctx, `SELECT user_id FROM members WHERE workspace_id = $1 AND role IN ('owner', 'admin')`, wid)
	if err != nil {
		return err
	}
	admins, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, uid := range admins {
		d := make(map[string]any, len(data))
		for k, v := range data {
			d[k] = v
		}
		if err := hd.Notify(ctx, tx, wid, uid, typ, nil, d); err != nil {
			return err
		}
	}
	return nil
}

// ─── Charges on the saved payment method ───────────────────────────────────

// charge is one ai_auto_recharges row: an auto-recharge or a top-up paid in the app.
type charge struct {
	ID              uuid.UUID
	Kind            string
	InvoiceID       *string
	CustomerID      string
	PaymentMethodID string
	USD             plan.Micro
	Amount          int64
	Currency        string
	Tries           int
}

func newCharge(ctx context.Context, q db.Querier, wid uuid.UUID, kind, cus, pm string, usd plan.Micro, price *Price) (*charge, error) {
	c := &charge{Kind: kind, CustomerID: cus, PaymentMethodID: pm, USD: usd, Amount: amountFor(usd, price), Currency: price.Currency}
	err := q.QueryRow(ctx, `INSERT INTO ai_auto_recharges (workspace_id, kind, customer_id, payment_method_id, micro_usd, amount, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, wid, kind, cus, pm, usd, c.Amount, c.Currency).Scan(&c.ID)
	return c, err
}

// pendingRecharge is the auto-recharge still being paid (or waiting for its next try).
func pendingRecharge(ctx context.Context, q db.Querier, wid uuid.UUID) (*charge, error) {
	var c charge
	err := q.QueryRow(ctx, `SELECT id, kind, invoice_id, customer_id, payment_method_id, micro_usd, amount, currency, tries
		FROM ai_auto_recharges WHERE workspace_id = $1 AND kind = 'auto_recharge' AND status = 'pending' ORDER BY created_at DESC LIMIT 1`, wid).
		Scan(&c.ID, &c.Kind, &c.InvoiceID, &c.CustomerID, &c.PaymentMethodID, &c.USD, &c.Amount, &c.Currency, &c.Tries)
	if db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// chargeText is the charge's line on its invoice.
func chargeText(c *charge) string {
	if c.Kind == rechargeKind {
		return "AI usage auto-recharge, " + usdText(c.USD)
	}
	return "AI usage top-up, " + usdText(c.USD)
}

// payOutcome is how one try of a charge ended.
type payOutcome struct {
	invoice  *stripe.Invoice
	paid     bool
	declined string // the failure code when the payment was refused
	soft     bool   // the bank may accept it on a later try
}

// pay creates the charge's invoice (or picks it up again), finalizes it and pays it with
// the saved payment method. Every Stripe request has an idempotency key derived from the
// charge, so a retried job never creates or pays anything twice.
func (s *svc) pay(ctx context.Context, wid uuid.UUID, c *charge, price *Price, offSession bool) (payOutcome, error) {
	pool := s.d.DB.Pool
	key := "od-charge-" + c.ID.String()
	var inv *stripe.Invoice
	var err error
	if c.InvoiceID != nil {
		if inv, err = s.sc.V1Invoices.Retrieve(ctx, *c.InvoiceID, nil); err != nil {
			return payOutcome{}, err
		}
	} else {
		meta := map[string]string{"workspace_id": wid.String(), "kind": c.Kind, "charge_id": c.ID.String(),
			"micro_usd": strconv.FormatInt(int64(c.USD), 10)}
		params := &stripe.InvoiceCreateParams{
			Customer:                    stripe.String(c.CustomerID),
			Currency:                    stripe.String(c.Currency),
			CollectionMethod:            stripe.String(string(stripe.InvoiceCollectionMethodChargeAutomatically)),
			AutoAdvance:                 stripe.Bool(false),
			PendingInvoiceItemsBehavior: stripe.String("exclude"),
			Description:                 stripe.String(chargeText(c)),
			Metadata:                    meta,
		}
		params.IdempotencyKey = stripe.String(key + "-invoice")
		if inv, err = s.sc.V1Invoices.Create(ctx, params); err != nil {
			return s.refused(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE ai_auto_recharges SET invoice_id = $2, updated_at = now() WHERE id = $1`, c.ID, inv.ID); err != nil {
			return payOutcome{}, err
		}
		c.InvoiceID = &inv.ID
	}
	switch inv.Status {
	case stripe.InvoiceStatusPaid:
		return payOutcome{invoice: inv, paid: true}, nil
	case stripe.InvoiceStatusVoid, stripe.InvoiceStatusUncollectible:
		return payOutcome{invoice: inv, declined: failPaymentFailed}, nil
	case stripe.InvoiceStatusDraft:
		if inv.Total == 0 {
			item := &stripe.InvoiceItemCreateParams{Customer: stripe.String(c.CustomerID), Invoice: stripe.String(inv.ID),
				Description: stripe.String(chargeText(c))}
			if price != nil && price.Product != "" {
				item.PriceData = &stripe.InvoiceItemCreatePriceDataParams{Currency: stripe.String(c.Currency), Product: stripe.String(price.Product),
					UnitAmount: stripe.Int64(c.Amount), TaxBehavior: stripe.String("exclusive")}
				item.Quantity = stripe.Int64(1)
			} else {
				item.Amount, item.Currency = stripe.Int64(c.Amount), stripe.String(c.Currency)
			}
			item.IdempotencyKey = stripe.String(key + "-item")
			if _, err := s.sc.V1InvoiceItems.Create(ctx, item); err != nil {
				return s.refused(err)
			}
		}
		fin := &stripe.InvoiceFinalizeInvoiceParams{AutoAdvance: stripe.Bool(false)}
		fin.IdempotencyKey = stripe.String(key + "-finalize")
		if inv, err = s.sc.V1Invoices.FinalizeInvoice(ctx, inv.ID, fin); err != nil {
			return s.refused(err)
		}
		if inv.Status == stripe.InvoiceStatusPaid {
			return payOutcome{invoice: inv, paid: true}, nil
		}
	}
	c.Tries++
	if _, err := pool.Exec(ctx, `UPDATE ai_auto_recharges SET tries = $2, updated_at = now() WHERE id = $1`, c.ID, c.Tries); err != nil {
		return payOutcome{}, err
	}
	params := &stripe.InvoicePayParams{OffSession: stripe.Bool(offSession), PaymentMethod: stripe.String(c.PaymentMethodID)}
	params.IdempotencyKey = stripe.String(fmt.Sprintf("%s-pay-%d", key, c.Tries))
	paid, err := s.sc.V1Invoices.Pay(ctx, inv.ID, params)
	if err != nil {
		out, err := s.refused(err)
		out.invoice = inv
		return out, err
	}
	return payOutcome{invoice: paid, paid: paid.Status == stripe.InvoiceStatusPaid}, nil
}

// softDeclines are the bank's answers that may change on a later try.
var softDeclines = map[stripe.DeclineCode]bool{
	stripe.DeclineCodeInsufficientFunds: true, stripe.DeclineCodeTryAgainLater: true, "issuer_not_available": true,
	"processing_error": true, "reenter_transaction": true, "approve_with_id": true,
}

// refused turns a Stripe error into a declined outcome when the payment itself was
// refused. Any other error is returned, so the request fails or the job retries.
func (s *svc) refused(err error) (payOutcome, error) {
	var se *stripe.Error
	if !errors.As(err, &se) {
		return payOutcome{}, err
	}
	switch {
	case se.Code == stripe.ErrorCodeAuthenticationRequired || se.Code == stripe.ErrorCodeInvoicePaymentIntentRequiresAction ||
		se.DeclineCode == stripe.DeclineCodeAuthenticationRequired:
		return payOutcome{declined: failAuthenticationNeeded}, nil
	case se.Type == stripe.ErrorTypeCard:
		return payOutcome{declined: failCardDeclined, soft: softDeclines[se.DeclineCode] || se.Code == stripe.ErrorCodeProcessingError}, nil
	case se.Type == stripe.ErrorTypeInvalidRequest && se.HTTPStatusCode < 500:
		// The customer or payment method can no longer be charged (detached, deleted).
		s.d.Log.Warn("stripe charge refused", "err", stripeErr(err))
		return payOutcome{declined: failPaymentFailed}, nil
	}
	return payOutcome{}, err
}

// abandon voids a charge's unpaid invoice and marks it failed.
func (s *svc) abandon(ctx context.Context, c *charge, out payOutcome, code string) {
	s.voidInvoice(ctx, out.invoice)
	if _, err := s.d.DB.Pool.Exec(ctx, `UPDATE ai_auto_recharges SET status = 'failed', failure_code = $2, retry_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'pending'`, c.ID, code); err != nil {
		s.d.Log.Warn("mark charge failed", "err", err)
	}
}

// voidInvoice removes an unpaid invoice, so nobody pays it later. Best effort.
func (s *svc) voidInvoice(ctx context.Context, inv *stripe.Invoice) {
	if inv == nil {
		return
	}
	var err error
	switch inv.Status {
	case stripe.InvoiceStatusDraft:
		_, err = s.sc.V1Invoices.Delete(ctx, inv.ID, nil)
	case stripe.InvoiceStatusOpen:
		_, err = s.sc.V1Invoices.VoidInvoice(ctx, inv.ID, nil)
	}
	if err != nil {
		s.d.Log.Warn("void invoice", "err", stripeErr(err))
	}
}

// grantPaid grants a paid charge's AI usage and publishes billing.updated.
func (s *svc) grantPaid(ctx context.Context, wid, chargeID uuid.UUID, invoiceID string, meta map[string]string) error {
	prices := s.pricesFor(ctx)
	var caps []capture
	err := s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if caps, err = s.grantChargeTx(ctx, tx, wid, chargeID, invoiceID, meta); err != nil || len(caps) == 0 {
			return err
		}
		return s.publish(ctx, tx, wid, prices)
	})
	if err == nil {
		for _, c := range caps {
			s.d.Analytics.Capture(c.wid.String(), c.wid.String(), c.event, c.props)
		}
	}
	return err
}

// grantChargeTx grants a charge's AI usage once per invoice and marks the charge paid.
// meta (the invoice metadata) stands in when the charge row is gone. It returns the
// analytics event to send, or nothing when the invoice was already granted.
func (s *svc) grantChargeTx(ctx context.Context, tx pgx.Tx, wid, chargeID uuid.UUID, invoiceID string, meta map[string]string) ([]capture, error) {
	var kind string
	var usage plan.Micro
	err := tx.QueryRow(ctx, `UPDATE ai_auto_recharges SET status = 'succeeded', invoice_id = $3, failure_code = NULL, retry_at = NULL, updated_at = now()
		WHERE id = $1 AND workspace_id = $2 RETURNING kind, micro_usd`, chargeID, wid, invoiceID).Scan(&kind, &usage)
	if db.IsNotFound(err) {
		n, _ := strconv.ParseInt(meta["micro_usd"], 10, 64)
		kind, usage, err = meta["kind"], plan.Micro(n), nil
	}
	if err != nil {
		return nil, err
	}
	if (kind != rechargeKind && kind != topUpKind) || usage < MinTopUp || usage > MaxTopUp {
		return nil, fmt.Errorf("invoice %s: invalid charge metadata", invoiceID)
	}
	source, action, event := topUpSource, "billing.ai_top_up_purchased", "ai_top_up_purchased"
	if kind == rechargeKind {
		source, action, event = rechargeSource, "billing.ai_recharged", "ai_recharged"
	}
	granted, err := plan.Grant(ctx, tx, wid, usage, source, invoiceID)
	if err != nil || !granted {
		return nil, err
	}
	if err := rearmNotices(ctx, tx, wid); err != nil {
		return nil, err
	}
	data := map[string]any{"usd": usage.USD()}
	if err := audit.Log(ctx, tx, nil, wid, action, "workspace", &wid, data); err != nil {
		return nil, err
	}
	return []capture{{wid, event, data}}, nil
}

// rearmNotices lets the running-low notices fire again after AI usage was added.
func rearmNotices(ctx context.Context, tx pgx.Tx, wid uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE ai_auto_recharge SET low_notified_at = NULL, out_notified_at = NULL WHERE workspace_id = $1`, wid)
	return err
}

// ─── The recharge ──────────────────────────────────────────────────────────

// autoRechargeWorker recharges one workspace off-session.
type autoRechargeWorker struct {
	river.WorkerDefaults[jobs.AutoRechargeArgs]
	s *svc
}

func (w *autoRechargeWorker) Timeout(*river.Job[jobs.AutoRechargeArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *autoRechargeWorker) Work(ctx context.Context, job *river.Job[jobs.AutoRechargeArgs]) error {
	s, wid := w.s, job.Args.WorkspaceID
	if !s.enabled() {
		return nil
	}
	pool := s.d.DB.Pool
	c, err := pendingRecharge(ctx, pool, wid)
	if err != nil {
		return err
	}
	st, err := readSettings(ctx, pool, wid, false)
	if err != nil {
		return err
	}
	if !st.Enabled {
		if c != nil {
			// Turned off before the charge went through: nothing more is charged.
			var inv *stripe.Invoice
			if c.InvoiceID != nil {
				inv, _ = s.sc.V1Invoices.Retrieve(ctx, *c.InvoiceID, nil)
			}
			if inv != nil && inv.Status == stripe.InvoiceStatusPaid {
				return s.grantPaid(ctx, wid, c.ID, inv.ID, nil)
			}
			s.abandon(ctx, c, payOutcome{invoice: inv}, failPaymentFailed)
		}
		return nil
	}
	if c == nil {
		if c, err = s.newRecharge(ctx, wid, st); err != nil || c == nil {
			return err
		}
	} else if pm, err := s.paymentMethod(ctx, c.CustomerID); err == nil && pm != "" && pm != c.PaymentMethodID {
		// The card changed while a declined recharge waited: the next try uses the new one.
		if _, err := pool.Exec(ctx, `UPDATE ai_auto_recharges SET payment_method_id = $2 WHERE id = $1`, c.ID, pm); err != nil {
			return err
		}
		c.PaymentMethodID = pm
	}
	out, err := s.pay(ctx, wid, c, s.pricesFor(ctx).AITopUp, true)
	if err != nil {
		return fmt.Errorf("auto-recharge: %s", stripeErr(err))
	}
	switch {
	case out.paid:
		return s.grantPaid(ctx, wid, c.ID, out.invoice.ID, nil)
	case out.declined != "" && out.soft && c.Tries <= len(retryAfter):
		wait := retryAfter[c.Tries-1]
		s.d.Log.Info("auto-recharge declined for now", "workspace_id", wid, "tries", c.Tries)
		prices := s.pricesFor(ctx)
		if err := s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE ai_auto_recharges SET retry_at = now() + $2::interval, failure_code = $3, updated_at = now()
				WHERE id = $1`, c.ID, fmt.Sprintf("%d seconds", int(wait.Seconds())), out.declined); err != nil {
				return err
			}
			return s.publish(ctx, tx, wid, prices)
		}); err != nil {
			return err
		}
		return river.JobSnooze(wait)
	case out.declined != "":
		s.d.Log.Info("auto-recharge declined", "workspace_id", wid, "code", out.declined)
		s.voidInvoice(ctx, out.invoice)
		return s.fail(ctx, wid, &c.ID, out.declined)
	}
	// Open but not paid yet: invoice.paid finishes it.
	return nil
}

// newRecharge records the next recharge, or returns nil when none is due. Near the monthly
// limit a recharge adds only what the limit has left (at least MinTopUp). A missing
// payment method or price turns auto-recharge off.
func (s *svc) newRecharge(ctx context.Context, wid uuid.UUID, st rechargeSettings) (*charge, error) {
	pool := s.d.DB.Pool
	bal, err := s.d.Plan.Balance(ctx, pool, wid)
	if err != nil {
		return nil, err
	}
	if bal.Unlimited || bal.Remaining >= st.Threshold {
		return nil, nil
	}
	charged, _, err := rechargedThisMonth(ctx, pool, wid)
	if err != nil {
		return nil, err
	}
	usd := min(st.Amount, st.Limit-charged)
	if usd < MinTopUp {
		return nil, nil
	}
	if s.d.Cfg.StripePriceTopUp == "" {
		return nil, s.fail(ctx, wid, nil, failUnavailable)
	}
	price := s.pricesFor(ctx).AITopUp
	if price == nil || price.UnitAmount <= 0 {
		return nil, errors.New("ai top-up price unavailable")
	}
	cus, err := s.customerID(ctx, pool, wid)
	if err != nil {
		return nil, err
	}
	pm := ""
	if cus != "" {
		if pm, err = s.paymentMethod(ctx, cus); err != nil {
			return nil, fmt.Errorf("payment method: %s", stripeErr(err))
		}
	}
	if pm == "" {
		return nil, s.fail(ctx, wid, nil, failNoPaymentMethod)
	}
	return newCharge(ctx, pool, wid, rechargeKind, cus, pm, usd, price)
}

// fail turns auto-recharge off after a recharge could not be paid, tells owners and
// admins, and records why.
func (s *svc) fail(ctx context.Context, wid uuid.UUID, chargeID *uuid.UUID, code string) error {
	prices := s.pricesFor(ctx)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		return s.failTx(ctx, tx, wid, chargeID, code, prices)
	})
}

func (s *svc) failTx(ctx context.Context, tx pgx.Tx, wid uuid.UUID, chargeID *uuid.UUID, code string, prices Prices) error {
	if chargeID != nil {
		if _, err := tx.Exec(ctx, `UPDATE ai_auto_recharges SET status = 'failed', failure_code = $2, retry_at = NULL, updated_at = now()
			WHERE id = $1 AND status = 'pending'`, *chargeID, code); err != nil {
			return err
		}
	}
	if err := ensureSettings(ctx, tx, wid); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE ai_auto_recharge SET enabled = false, failure_code = $2, failed_at = now(), updated_at = now()
		WHERE workspace_id = $1 AND enabled`, wid, code)
	if err != nil || tag.RowsAffected() == 0 {
		// Already off: someone turned it off, or an earlier failure did.
		return err
	}
	if err := audit.Log(ctx, tx, nil, wid, "billing.auto_recharge_failed", "workspace", &wid, map[string]any{"reason": code}); err != nil {
		return err
	}
	if err := s.notifyAdmins(ctx, tx, wid, NotifyAutoRechargeFailed, map[string]any{"reason": code}); err != nil {
		return err
	}
	return s.publish(ctx, tx, wid, prices)
}

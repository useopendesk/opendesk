package billing

// The saved payment method as the app shows it, and the workspace's invoices
// (contracts/billing.md "Payment method and invoices"). Stripe keeps the card; billing
// keeps only its brand, last four digits and expiry, refreshed whenever Stripe reports a
// change (a Checkout, the portal, the card network's automatic updates).

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stripe/stripe-go/v87"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// PaymentMethod is Billing.payment_method: what charges use, as people recognise it.
type PaymentMethod struct {
	Brand    string `json:"brand" doc:"The card brand (visa, mastercard, amex, ...) or the payment method type (link, sepa_debit, ...)"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month,omitempty"`
	ExpYear  int    `json:"exp_year,omitempty"`
	Wallet   string `json:"wallet,omitempty" doc:"apple_pay, google_pay, link, ... when the card is in a wallet"`
}

func loadPaymentMethod(ctx context.Context, q db.Querier, wid uuid.UUID) (*PaymentMethod, error) {
	var brand, last4, wallet *string
	var month, year *int
	err := q.QueryRow(ctx, `SELECT pm_brand, pm_last4, pm_exp_month, pm_exp_year, pm_wallet FROM subscriptions WHERE workspace_id = $1`, wid).
		Scan(&brand, &last4, &month, &year, &wallet)
	if db.IsNotFound(err) || (err == nil && brand == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pm := &PaymentMethod{Brand: *brand}
	if last4 != nil {
		pm.Last4 = *last4
	}
	if month != nil && year != nil {
		pm.ExpMonth, pm.ExpYear = *month, *year
	}
	if wallet != nil {
		pm.Wallet = *wallet
	}
	return pm, nil
}

func describePaymentMethod(pm *stripe.PaymentMethod) *PaymentMethod {
	if pm == nil {
		return nil
	}
	out := &PaymentMethod{Brand: string(pm.Type)}
	switch {
	case pm.Card != nil:
		out.Brand, out.Last4, out.ExpMonth, out.ExpYear = string(pm.Card.Brand), pm.Card.Last4, int(pm.Card.ExpMonth), int(pm.Card.ExpYear)
		if pm.Card.Wallet != nil {
			out.Wallet = string(pm.Card.Wallet.Type)
		}
	case pm.SEPADebit != nil:
		out.Last4 = pm.SEPADebit.Last4
	case pm.USBankAccount != nil:
		out.Last4 = pm.USBankAccount.Last4
	}
	return out
}

func paymentMethodSyncOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: jobs.QueueDefault, MaxAttempts: 8, ScheduledAt: time.Now().Add(2 * time.Second),
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: uniqueStates}}
}

// enqueuePaymentMethodSync queues a refresh of wid's saved payment method in tx.
func enqueuePaymentMethodSync(ctx context.Context, d *deps.Deps, tx pgx.Tx, wid uuid.UUID) error {
	if d.Jobs == nil {
		return nil
	}
	_, err := d.Jobs.InsertTx(ctx, tx, jobs.SyncPaymentMethodArgs{WorkspaceID: wid}, paymentMethodSyncOpts())
	return err
}

// syncPaymentMethodWorker reads the payment method charges use from Stripe and stores what
// the app shows of it.
type syncPaymentMethodWorker struct {
	river.WorkerDefaults[jobs.SyncPaymentMethodArgs]
	s *svc
}

func (w *syncPaymentMethodWorker) Work(ctx context.Context, job *river.Job[jobs.SyncPaymentMethodArgs]) error {
	s, wid := w.s, job.Args.WorkspaceID
	if !s.enabled() {
		return nil
	}
	cus, err := s.customerID(ctx, s.d.DB.Pool, wid)
	if err != nil {
		return err
	}
	var next *PaymentMethod
	if cus != "" {
		s.ensureDefaultPM(ctx, cus, "")
		id, err := s.paymentMethod(ctx, cus)
		if err != nil {
			return err
		}
		if id != "" {
			pm, err := s.sc.V1PaymentMethods.Retrieve(ctx, id, nil)
			if err != nil && !isMissing(err) {
				return err
			}
			next = describePaymentMethod(pm)
		}
	}
	prices := s.pricesFor(ctx)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := loadPaymentMethod(ctx, tx, wid)
		if err != nil {
			return err
		}
		if (cur == nil && next == nil) || (cur != nil && next != nil && *cur == *next) {
			return nil
		}
		var brand, last4, wallet *string
		var month, year *int
		if next != nil {
			brand, last4 = &next.Brand, &next.Last4
			if next.ExpYear > 0 {
				month, year = &next.ExpMonth, &next.ExpYear
			}
			if next.Wallet != "" {
				wallet = &next.Wallet
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET pm_brand = $2, pm_last4 = $3, pm_exp_month = $4, pm_exp_year = $5, pm_wallet = $6,
			updated_at = now() WHERE workspace_id = $1`, wid, brand, last4, month, year, wallet); err != nil {
			return err
		}
		return s.publish(ctx, tx, wid, prices)
	})
}

// Invoice is one entry of GET /v1/billing/invoices.
type Invoice struct {
	ID          string    `json:"id"`
	Number      string    `json:"number"`
	Created     time.Time `json:"created"`
	Total       int64     `json:"total" doc:"In the currency's minor unit"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status" enum:"paid,open,uncollectible"`
	Kind        string    `json:"kind" enum:"subscription,top_up,auto_recharge,other"`
	Description string    `json:"description"`
	URL         string    `json:"url" doc:"Stripe's page for the invoice, to view, download or pay it"`
	PDFURL      string    `json:"pdf_url"`
}

// invoiceLimit is how many recent invoices the app lists; the portal has them all.
const invoiceLimit = 24

func (s *svc) registerPaymentsAPI(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-billing-invoices", Method: http.MethodGet, Path: "/v1/billing/invoices",
		Summary: "List the workspace's recent invoices", Tags: []string{"Billing"}},
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Data []Invoice `json:"data"`
			}
		}, error) {
			if !s.enabled() {
				return nil, errDisabled()
			}
			p, err := requireBillingAdmin(ctx)
			if err != nil {
				return nil, err
			}
			if !s.d.Limiter.Allow("billing:invoices:"+p.WorkspaceID.String(), 240, time.Hour) {
				return nil, httpx.RateLimited()
			}
			out := &struct {
				Body struct {
					Data []Invoice `json:"data"`
				}
			}{}
			out.Body.Data = []Invoice{}
			cus, err := s.customerID(ctx, s.d.DB.Pool, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if cus == "" {
				return out, nil
			}
			params := &stripe.InvoiceListParams{Customer: stripe.String(cus)}
			params.Limit = stripe.Int64(invoiceLimit)
			params.Single = true
			for inv, err := range s.sc.V1Invoices.List(ctx, params).All(ctx) {
				if err != nil {
					if isMissing(err) {
						return out, nil
					}
					return nil, s.wrapStripe(err)
				}
				if v, ok := invoiceOf(inv); ok {
					out.Body.Data = append(out.Body.Data, v)
				}
			}
			return out, nil
		})
}

// invoiceOf is the app's view of a Stripe invoice. Drafts and voided invoices (a declined
// charge) are left out.
func invoiceOf(inv *stripe.Invoice) (Invoice, bool) {
	switch inv.Status {
	case stripe.InvoiceStatusPaid, stripe.InvoiceStatusOpen, stripe.InvoiceStatusUncollectible:
	default:
		return Invoice{}, false
	}
	v := Invoice{ID: inv.ID, Number: inv.Number, Created: time.Unix(inv.Created, 0).UTC(), Total: inv.Total, Currency: string(inv.Currency),
		Status: string(inv.Status), Kind: "other", Description: inv.Description, URL: inv.HostedInvoiceURL, PDFURL: inv.InvoicePDF}
	switch {
	case inv.Parent != nil && inv.Parent.SubscriptionDetails != nil:
		v.Kind = "subscription"
	case inv.Metadata["kind"] == rechargeKind:
		v.Kind = rechargeKind
	case inv.Metadata["kind"] == topUpKind:
		v.Kind = topUpKind
	}
	return v, true
}

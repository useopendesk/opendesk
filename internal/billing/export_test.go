package billing

import (
	"context"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Test hooks for the external billing_test package.

func SetStripeBackend(url string) { stripeBackendURL = url }

func RunSyncSeats(ctx context.Context, d *deps.Deps, wid uuid.UUID) error {
	return (&syncSeatsWorker{s: newSvc(d)}).Work(ctx, &river.Job[jobs.SyncSeatsArgs]{Args: jobs.SyncSeatsArgs{WorkspaceID: wid}})
}

func RunCancel(ctx context.Context, d *deps.Deps, a jobs.CancelSubscriptionArgs) error {
	return (&cancelWorker{s: newSvc(d)}).Work(ctx, &river.Job[jobs.CancelSubscriptionArgs]{Args: a})
}

func RunReconcile(ctx context.Context, d *deps.Deps) error {
	return (&reconcileWorker{s: newSvc(d)}).Work(ctx, &river.Job[jobs.ReconcileBillingArgs]{})
}

func RunAutoRecharge(ctx context.Context, d *deps.Deps, wid uuid.UUID) error {
	return (&autoRechargeWorker{s: newSvc(d)}).Work(ctx, &river.Job[jobs.AutoRechargeArgs]{Args: jobs.AutoRechargeArgs{WorkspaceID: wid}})
}

func RunSyncPaymentMethod(ctx context.Context, d *deps.Deps, wid uuid.UUID) error {
	return (&syncPaymentMethodWorker{s: newSvc(d)}).Work(ctx, &river.Job[jobs.SyncPaymentMethodArgs]{Args: jobs.SyncPaymentMethodArgs{WorkspaceID: wid}})
}

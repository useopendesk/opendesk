package helpdomains

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// sweepBatch bounds the checks queued per sweep.
const sweepBatch = 200

// RegisterWorkers adds the domain workers.
func RegisterWorkers(d *deps.Deps, w *river.Workers) {
	s := New(d)
	river.AddWorker(w, &checkWorker{s: s})
	river.AddWorker(w, &sweepWorker{d: d})
	river.AddWorker(w, &releaseWorker{s: s})
	river.AddWorker(w, &reconcileWorker{s: s})
}

// Periodic sweeps for due checks every minute and reconciles Cloudflare daily.
func Periodic(d *deps.Deps) []*river.PeriodicJob {
	out := []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return jobs.HelpDomainSweepArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true})}
	if d.Cfg.CloudflareSaaS() {
		out = append(out, river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return jobs.HelpDomainReconcileArgs{}, nil }, nil))
	}
	return out
}

// enqueueCheck queues a check of a domain in the write's transaction.
func enqueueCheck(ctx context.Context, d *deps.Deps, tx pgx.Tx, id uuid.UUID) error {
	_, err := d.Jobs.InsertTx(ctx, tx, jobs.HelpDomainCheckArgs{DomainID: id}, &river.InsertOpts{MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: time.Minute}})
	return err
}

type checkWorker struct {
	river.WorkerDefaults[jobs.HelpDomainCheckArgs]
	s *Service
}

func (w *checkWorker) Timeout(*river.Job[jobs.HelpDomainCheckArgs]) time.Duration { return time.Minute }

func (w *checkWorker) Work(ctx context.Context, job *river.Job[jobs.HelpDomainCheckArgs]) error {
	return w.s.Check(ctx, job.Args.DomainID, false)
}

type sweepWorker struct {
	river.WorkerDefaults[jobs.HelpDomainSweepArgs]
	d *deps.Deps
}

// Work queues the checks that are due, pushing each one's next check out meanwhile so a
// slow check is not queued twice.
func (w *sweepWorker) Work(ctx context.Context, _ *river.Job[jobs.HelpDomainSweepArgs]) error {
	return w.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE help_domains SET next_check_at = now() + interval '10 minutes'
			WHERE id IN (SELECT id FROM help_domains WHERE status <> 'failed' AND next_check_at <= now()
				ORDER BY next_check_at LIMIT $1 FOR UPDATE SKIP LOCKED) RETURNING id`, sweepBatch)
		if err != nil {
			return err
		}
		due, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		for _, id := range due {
			if err := enqueueCheck(ctx, w.d, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

type releaseWorker struct {
	river.WorkerDefaults[jobs.HelpDomainReleaseArgs]
	s *Service
}

func (w *releaseWorker) Work(ctx context.Context, job *river.Job[jobs.HelpDomainReleaseArgs]) error {
	return w.s.Release(ctx, job.Args.ProviderID)
}

type reconcileWorker struct {
	river.WorkerDefaults[jobs.HelpDomainReconcileArgs]
	s *Service
}

func (w *reconcileWorker) Timeout(*river.Job[jobs.HelpDomainReconcileArgs]) time.Duration {
	return 10 * time.Minute
}

func (w *reconcileWorker) Work(ctx context.Context, _ *river.Job[jobs.HelpDomainReconcileArgs]) error {
	return w.s.Reconcile(ctx)
}

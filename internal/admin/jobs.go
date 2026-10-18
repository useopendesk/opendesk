package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river/rivertype"

	"github.com/useopendesk/opendesk/internal/httpx"
)

type AdminJob struct {
	ID          int64          `json:"id"`
	Kind        string         `json:"kind"`
	Queue       string         `json:"queue"`
	State       string         `json:"state" enum:"available,running,retryable,scheduled,completed,cancelled,discarded,pending"`
	Attempt     int            `json:"attempt"`
	MaxAttempts int            `json:"max_attempts"`
	Args        map[string]any `json:"args"`
	LastError   *string        `json:"last_error"`
	Errors      int            `json:"errors"`
	CreatedAt   time.Time      `json:"created_at"`
	ScheduledAt time.Time      `json:"scheduled_at"`
	AttemptedAt *time.Time     `json:"attempted_at"`
	FinalizedAt *time.Time     `json:"finalized_at"`
}

type AdminJobKind struct {
	Kind         string `json:"kind"`
	Available    int    `json:"available"`
	Running      int    `json:"running"`
	Retryable    int    `json:"retryable"`
	Discarded24h int    `json:"discarded_24h"`
}

type AdminJobStats struct {
	States map[string]int `json:"states"`
	Kinds  []AdminJobKind `json:"kinds"`
}

var jobStates = []string{"available", "running", "retryable", "scheduled", "completed", "cancelled", "discarded", "pending"}

// maxErrorLen cuts a job's last error for the panel.
const maxErrorLen = 2000

// redactArgs keeps the shape of a job's args but not what could be a message, a link or
// a credential: system email jobs carry whole emails with their sign-in links.
func redactArgs(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if sensitiveKey(k) {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactArgs(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = redactArgs(val)
		}
		return out
	case string:
		if len(x) > 200 {
			return fmt.Sprintf("[%d characters]", len(x))
		}
	}
	return v
}

func sensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"html", "text", "body", "subject", "header", "token", "secret", "password", "code", "key", "credential", "content", "preview"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

const jobCols = `id, kind, queue, state, attempt, max_attempts, args,
	errors[array_length(errors, 1)]->>'error', COALESCE(array_length(errors, 1), 0), created_at, scheduled_at, attempted_at, finalized_at`

func scanJob(r pgx.Row) (AdminJob, error) {
	var j AdminJob
	var raw []byte
	err := r.Scan(&j.ID, &j.Kind, &j.Queue, &j.State, &j.Attempt, &j.MaxAttempts, &raw, &j.LastError, &j.Errors,
		&j.CreatedAt, &j.ScheduledAt, &j.AttemptedAt, &j.FinalizedAt)
	if err != nil {
		return j, err
	}
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	if args == nil {
		args = map[string]any{}
	}
	j.Args, _ = redactArgs(args).(map[string]any)
	if j.LastError != nil && len(*j.LastError) > maxErrorLen {
		cut := (*j.LastError)[:maxErrorLen]
		j.LastError = &cut
	}
	return j, nil
}

type jobPath struct {
	ID string `path:"id"`
}

func (s *svc) registerJobs(api huma.API) {
	op(s, api, huma.Operation{OperationID: "admin-list-jobs", Method: http.MethodGet, Path: "/v1/admin/jobs", Summary: "List background jobs"},
		func(ctx context.Context, in *struct {
			httpx.ListParams
			State string `query:"state" enum:"available,running,retryable,scheduled,completed,cancelled,discarded,pending," default:"retryable"`
			Kind  string `query:"kind" maxLength:"100"`
			Queue string `query:"queue" maxLength:"100"`
		}) (*struct{ Body httpx.Page[AdminJob] }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			var cur struct {
				ID int64 `json:"id"`
			}
			if err := httpx.DecodeCursor(in.Cursor, &cur); err != nil {
				return nil, err
			}
			if cur.ID == 0 {
				cur.ID = 1<<63 - 1
			}
			state := in.State
			if state == "" {
				state = "retryable"
			}
			rows, err := s.d.DB.Pool.Query(ctx, `SELECT `+jobCols+` FROM river_job
				WHERE state = $1::river_job_state AND id < $2 AND ($3 = '' OR kind = $3) AND ($4 = '' OR queue = $4)
				ORDER BY id DESC LIMIT $5`, state, cur.ID, in.Kind, in.Queue, in.Limit+1)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminJob, error) { return scanJob(r) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			page := httpx.NewPage(list, in.Limit, func(j AdminJob) any { return map[string]any{"id": j.ID} })
			return &struct{ Body httpx.Page[AdminJob] }{page}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-job-stats", Method: http.MethodGet, Path: "/v1/admin/jobs/stats", Summary: "Background jobs by state and kind"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body AdminJobStats }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			st, err := s.jobStats(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminJobStats }{st}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-retry-job", Method: http.MethodPost, Path: "/v1/admin/jobs/{id}/retry", Summary: "Run a job again now"},
		func(ctx context.Context, in *jobPath) (*struct{ Body AdminJob }, error) {
			return s.jobAction(ctx, in.ID, "retry")
		})

	op(s, api, huma.Operation{OperationID: "admin-cancel-job", Method: http.MethodPost, Path: "/v1/admin/jobs/{id}/cancel", Summary: "Cancel a job"},
		func(ctx context.Context, in *jobPath) (*struct{ Body AdminJob }, error) {
			return s.jobAction(ctx, in.ID, "cancel")
		})
}

func (s *svc) jobStats(ctx context.Context) (AdminJobStats, error) {
	st := AdminJobStats{States: map[string]int{}}
	for _, k := range jobStates {
		st.States[k] = 0
	}
	rows, err := s.d.DB.Pool.Query(ctx, `SELECT state::text, count(*) FROM river_job GROUP BY state`)
	if err != nil {
		return st, err
	}
	if err := forEach(rows, func(r pgx.CollectableRow) error {
		var k string
		var n int
		if err := r.Scan(&k, &n); err != nil {
			return err
		}
		st.States[k] = n
		return nil
	}); err != nil {
		return st, err
	}
	rows, err = s.d.DB.Pool.Query(ctx, `SELECT kind,
		count(*) FILTER (WHERE state = 'available'), count(*) FILTER (WHERE state = 'running'),
		count(*) FILTER (WHERE state = 'retryable'),
		count(*) FILTER (WHERE state = 'discarded' AND finalized_at > now() - interval '24 hours')
		FROM river_job WHERE state IN ('available', 'running', 'retryable', 'discarded')
		GROUP BY kind ORDER BY 4 DESC, 5 DESC, 2 DESC, kind LIMIT 200`)
	if err != nil {
		return st, err
	}
	st.Kinds, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminJobKind, error) {
		var k AdminJobKind
		err := r.Scan(&k.Kind, &k.Available, &k.Running, &k.Retryable, &k.Discarded24h)
		return k, err
	})
	if st.Kinds == nil {
		st.Kinds = []AdminJobKind{}
	}
	return st, err
}

func (s *svc) jobAction(ctx context.Context, rawID, action string) (*struct{ Body AdminJob }, error) {
	a, err := require(ctx, PermJobsManage)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return nil, httpx.NotFound("Job")
	}
	var job AdminJob
	err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var state string
		err := tx.QueryRow(ctx, `SELECT state::text FROM river_job WHERE id = $1 FOR UPDATE`, id).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Job")
		}
		if err != nil {
			return err
		}
		switch action {
		case "retry":
			if state != "retryable" && state != "discarded" && state != "cancelled" {
				return httpx.E(409, "conflict", "Only a retryable, discarded or cancelled job can be run again.")
			}
			_, err = s.d.Jobs.JobRetryTx(ctx, tx, id)
		case "cancel":
			if state == "completed" || state == "cancelled" || state == "discarded" {
				return httpx.E(409, "conflict", "This job has already finished.")
			}
			_, err = s.d.Jobs.JobCancelTx(ctx, tx, id)
		}
		if errors.Is(err, rivertype.ErrNotFound) {
			return httpx.NotFound("Job")
		}
		if err != nil {
			return err
		}
		verb := map[string]string{"retry": "job.retried", "cancel": "job.cancelled"}[action]
		if err := s.audit(ctx, tx, a, verb, "job", strconv.FormatInt(id, 10), map[string]any{"from": state}); err != nil {
			return err
		}
		job, err = scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM river_job WHERE id = $1`, id))
		return err
	})
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	return &struct{ Body AdminJob }{job}, nil
}

func forEach(rows pgx.Rows, fn func(pgx.CollectableRow) error) error {
	_, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) { return struct{}{}, fn(r) })
	return err
}

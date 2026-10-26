package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type JobRun struct {
	ID           uuid.UUID
	Name         string
	ScheduledFor time.Time
	StartedAt    sql.NullTime
	FinishedAt   sql.NullTime
	Status       string
	Error        string
}

var SupportedJobs = map[string]struct{}{
	"market_data": {}, "fx_rates": {}, "rankings": {}, "alerts": {}, "data_quality": {},
}

func (r *MarketData) QueueJob(ctx context.Context, name string, scheduledFor time.Time) (JobRun, error) {
	if _, ok := SupportedJobs[name]; !ok {
		return JobRun{}, fmt.Errorf("unsupported job %q", name)
	}
	if scheduledFor.IsZero() {
		return JobRun{}, fmt.Errorf("scheduled time is required")
	}
	run := JobRun{ID: uuid.New(), Name: name, ScheduledFor: scheduledFor.UTC().Truncate(time.Microsecond), Status: "queued"}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO job_runs (id, job_name, scheduled_for, status)
		VALUES ($1, $2, $3, 'queued')
		ON CONFLICT (job_name, scheduled_for) DO NOTHING`,
		run.ID, name, run.ScheduledFor,
	)
	if err != nil {
		return JobRun{}, fmt.Errorf("queue job run: %w", err)
	}
	err = r.db.QueryRowContext(ctx, `
		SELECT id, job_name, scheduled_for, status, started_at, finished_at, error_message
		FROM job_runs WHERE job_name = $1 AND scheduled_for = $2`,
		name, run.ScheduledFor,
	).Scan(&run.ID, &run.Name, &run.ScheduledFor, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Error)
	if err != nil {
		return JobRun{}, fmt.Errorf("read queued job: %w", err)
	}
	return run, nil
}

func (r *MarketData) ListJobs(ctx context.Context, limit, offset int) ([]JobRun, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM job_runs").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count job runs: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, job_name, scheduled_for, status, started_at, finished_at, error_message
		FROM job_runs
		ORDER BY scheduled_for DESC, id DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list job runs: %w", err)
	}
	defer rows.Close()
	runs := make([]JobRun, 0)
	for rows.Next() {
		var run JobRun
		if err := rows.Scan(&run.ID, &run.Name, &run.ScheduledFor, &run.Status,
			&run.StartedAt, &run.FinishedAt, &run.Error); err != nil {
			return nil, 0, fmt.Errorf("scan job run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read job runs: %w", err)
	}
	return runs, total, nil
}

func (r *MarketData) ClaimJob(ctx context.Context, owner string, lease time.Duration) (JobRun, bool, error) {
	if owner == "" || lease <= 0 {
		return JobRun{}, false, fmt.Errorf("job owner and positive lease duration are required")
	}
	var run JobRun
	err := r.db.QueryRowContext(ctx, `
		WITH next_job AS (
			SELECT id
			FROM job_runs
			WHERE (status = 'queued' AND scheduled_for <= now())
			   OR (status = 'running' AND (lease_expires_at IS NULL OR lease_expires_at < now()))
			ORDER BY scheduled_for, requested_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE job_runs AS j
		SET status = 'running', started_at = now(), finished_at = NULL,
		    error_message = '', lock_owner = $1,
		    lease_expires_at = now() + ($2::bigint * interval '1 millisecond')
		FROM next_job
		WHERE j.id = next_job.id
		RETURNING j.id, j.job_name, j.scheduled_for, j.status, j.started_at, j.finished_at, j.error_message`,
		owner, lease.Milliseconds(),
	).Scan(&run.ID, &run.Name, &run.ScheduledFor, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Error)
	if err == sql.ErrNoRows {
		return JobRun{}, false, nil
	}
	if err != nil {
		return JobRun{}, false, fmt.Errorf("claim next job: %w", err)
	}
	return run, true, nil
}

func (r *MarketData) FinishJob(ctx context.Context, id uuid.UUID, owner string, runErr error) error {
	status, message := "completed", ""
	if runErr != nil {
		status, message = "failed", runErr.Error()
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE job_runs
		SET status = $3, finished_at = now(), error_message = $4,
		    lock_owner = '', lease_expires_at = NULL
		WHERE id = $1 AND lock_owner = $2 AND status = 'running'`,
		id, owner, status, message)
	if err != nil {
		return fmt.Errorf("finish job run: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read job completion result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("job %s lease was lost before completion", id)
	}
	return nil
}

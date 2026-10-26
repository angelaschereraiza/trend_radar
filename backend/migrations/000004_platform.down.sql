DROP INDEX IF EXISTS alerts_daily_idempotency_idx;
DROP INDEX IF EXISTS job_runs_recent_idx;
DROP INDEX IF EXISTS job_runs_queue_idx;
DROP INDEX IF EXISTS backtest_runs_created_at_idx;

ALTER TABLE alerts DROP COLUMN IF EXISTS generated_date;
ALTER TABLE job_runs DROP COLUMN IF EXISTS requested_at;
ALTER TABLE job_runs DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE backtest_runs
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS warnings_json,
    DROP COLUMN IF EXISTS summary_json;

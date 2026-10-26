ALTER TABLE backtest_runs
    ADD COLUMN summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN warnings_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

ALTER TABLE job_runs
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN requested_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE alerts
    ADD COLUMN generated_date DATE NOT NULL DEFAULT CURRENT_DATE;

CREATE INDEX backtest_runs_created_at_idx ON backtest_runs(created_at DESC);
CREATE INDEX job_runs_queue_idx ON job_runs(status, scheduled_for);
CREATE INDEX job_runs_recent_idx ON job_runs(job_name, scheduled_for DESC);
CREATE UNIQUE INDEX alerts_daily_idempotency_idx
    ON alerts(stock_id, strategy_id, alert_type, generated_date);

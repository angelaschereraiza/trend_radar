CREATE TABLE indicators (
    stock_id UUID NOT NULL REFERENCES stocks(id),
    calculation_date DATE NOT NULL,
    indicator_type TEXT NOT NULL,
    period INTEGER NOT NULL CHECK (period > 0),
    value DOUBLE PRECISION,
    calculation_version TEXT NOT NULL,
    PRIMARY KEY (stock_id, calculation_date, indicator_type, period, calculation_version)
);

CREATE TABLE strategy_definitions (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    parameters_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

CREATE TABLE stock_scores (
    stock_id UUID NOT NULL REFERENCES stocks(id),
    strategy_id UUID NOT NULL REFERENCES strategy_definitions(id),
    calculation_date DATE NOT NULL,
    score DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 100),
    component_scores_json JSONB NOT NULL,
    calculation_version TEXT NOT NULL,
    PRIMARY KEY (stock_id, strategy_id, calculation_date, calculation_version)
);

CREATE TABLE factor_correlations (
    strategy_id UUID NOT NULL REFERENCES strategy_definitions(id),
    calculation_date DATE NOT NULL,
    factor_a TEXT NOT NULL,
    factor_b TEXT NOT NULL,
    correlation DOUBLE PRECISION NOT NULL CHECK (correlation >= -1 AND correlation <= 1),
    sample_size INTEGER NOT NULL CHECK (sample_size >= 0),
    PRIMARY KEY (strategy_id, calculation_date, factor_a, factor_b)
);

CREATE TABLE backtest_runs (
    id UUID PRIMARY KEY,
    strategy_id UUID NOT NULL REFERENCES strategy_definitions(id),
    universe_id UUID NOT NULL REFERENCES universes(id),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    configuration_json JSONB NOT NULL,
    data_snapshot_id UUID NOT NULL REFERENCES data_snapshots(id),
    code_version TEXT NOT NULL,
    dataset_role TEXT NOT NULL CHECK (dataset_role IN ('development', 'walk_forward', 'holdout', 'forward')),
    survivorship_biased BOOLEAN NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    CHECK (end_date >= start_date)
);

CREATE TABLE backtest_orders (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    stock_id UUID NOT NULL REFERENCES stocks(id),
    decision_date DATE NOT NULL,
    intended_execution_date DATE NOT NULL,
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    requested_quantity NUMERIC(20, 8) NOT NULL CHECK (requested_quantity > 0),
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('filled', 'partially_filled', 'rejected')),
    reject_reason TEXT NOT NULL DEFAULT ''
);

CREATE TABLE backtest_fills (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES backtest_orders(id),
    execution_date DATE NOT NULL,
    quantity NUMERIC(20, 8) NOT NULL CHECK (quantity > 0),
    price_usd NUMERIC(20, 8) NOT NULL CHECK (price_usd > 0),
    fx_rate_usd_chf NUMERIC(20, 8) NOT NULL CHECK (fx_rate_usd_chf > 0),
    gross_value_chf NUMERIC(20, 4) NOT NULL,
    commission_chf NUMERIC(20, 4) NOT NULL,
    fx_cost_chf NUMERIC(20, 4) NOT NULL,
    stamp_duty_chf NUMERIC(20, 4) NOT NULL,
    slippage_chf NUMERIC(20, 4) NOT NULL
);

CREATE TABLE backtest_cash_ledger (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    booking_date DATE NOT NULL,
    entry_type TEXT NOT NULL,
    amount_chf NUMERIC(20, 4) NOT NULL,
    related_fill_id UUID REFERENCES backtest_fills(id),
    balance_after_chf NUMERIC(20, 4) NOT NULL
);

CREATE TABLE backtest_positions (
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    stock_id UUID NOT NULL REFERENCES stocks(id),
    entry_date DATE NOT NULL,
    exit_date DATE,
    entry_price NUMERIC(20, 8) NOT NULL,
    exit_price NUMERIC(20, 8),
    quantity NUMERIC(20, 8) NOT NULL,
    fees NUMERIC(20, 4) NOT NULL,
    realized_pnl NUMERIC(20, 4),
    PRIMARY KEY (run_id, stock_id, entry_date)
);

CREATE TABLE backtest_equity (
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    trading_date DATE NOT NULL,
    portfolio_value_chf NUMERIC(20, 4) NOT NULL,
    cash_balance_chf NUMERIC(20, 4) NOT NULL,
    invested_value_chf NUMERIC(20, 4) NOT NULL,
    benchmark_value_chf NUMERIC(20, 4) NOT NULL,
    PRIMARY KEY (run_id, trading_date)
);

CREATE TABLE experiment_trials (
    id UUID PRIMARY KEY,
    strategy_name TEXT NOT NULL,
    strategy_version TEXT NOT NULL,
    parameters_hash CHAR(64) NOT NULL,
    parameters_json JSONB NOT NULL,
    dataset_role TEXT NOT NULL,
    data_snapshot_id UUID NOT NULL REFERENCES data_snapshots(id),
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    primary_metric_value DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE holdout_access_log (
    id UUID PRIMARY KEY,
    strategy_name TEXT NOT NULL,
    strategy_version TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES backtest_runs(id),
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    note TEXT NOT NULL DEFAULT ''
);

CREATE TABLE job_runs (
    id UUID PRIMARY KEY,
    job_name TEXT NOT NULL,
    scheduled_for TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    error_message TEXT NOT NULL DEFAULT '',
    lock_owner TEXT NOT NULL DEFAULT '',
    UNIQUE (job_name, scheduled_for)
);

CREATE TABLE alerts (
    id UUID PRIMARY KEY,
    stock_id UUID NOT NULL REFERENCES stocks(id),
    strategy_id UUID NOT NULL REFERENCES strategy_definitions(id),
    alert_type TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ
);

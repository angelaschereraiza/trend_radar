CREATE TABLE stocks (
    id UUID PRIMARY KEY,
    symbol TEXT NOT NULL,
    exchange TEXT NOT NULL,
    name TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    sector TEXT NOT NULL DEFAULT '',
    industry TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exchange, symbol)
);

CREATE TABLE symbol_history (
    stock_id UUID NOT NULL REFERENCES stocks(id),
    symbol TEXT NOT NULL,
    valid_from DATE NOT NULL,
    valid_to DATE,
    PRIMARY KEY (stock_id, symbol, valid_from),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE UNIQUE INDEX symbol_history_current_symbol_idx
    ON symbol_history(symbol) WHERE valid_to IS NULL;

CREATE TABLE stock_prices (
    stock_id UUID NOT NULL REFERENCES stocks(id),
    trading_date DATE NOT NULL,
    open NUMERIC(20, 8) NOT NULL CHECK (open > 0),
    high NUMERIC(20, 8) NOT NULL CHECK (high > 0),
    low NUMERIC(20, 8) NOT NULL CHECK (low > 0),
    close NUMERIC(20, 8) NOT NULL CHECK (close > 0),
    adjusted_close NUMERIC(20, 8) NOT NULL CHECK (adjusted_close > 0),
    volume BIGINT NOT NULL CHECK (volume >= 0),
    currency CHAR(3) NOT NULL,
    provider TEXT NOT NULL,
    data_version TEXT NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stock_id, trading_date, provider, data_version),
    CHECK (high >= low AND high >= open AND high >= close AND low <= open AND low <= close)
);

CREATE INDEX stock_prices_history_idx ON stock_prices(stock_id, trading_date);

CREATE TABLE fx_rates (
    base_currency CHAR(3) NOT NULL,
    quote_currency CHAR(3) NOT NULL,
    rate_date DATE NOT NULL,
    rate NUMERIC(20, 8) NOT NULL CHECK (rate > 0),
    provider TEXT NOT NULL,
    data_version TEXT NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (base_currency, quote_currency, rate_date, provider, data_version)
);

CREATE TABLE corporate_actions (
    id UUID PRIMARY KEY,
    stock_id UUID NOT NULL REFERENCES stocks(id),
    action_type TEXT NOT NULL CHECK (action_type IN ('split', 'dividend', 'delisting', 'symbol_change', 'merger')),
    effective_date DATE NOT NULL,
    amount NUMERIC(20, 8) NOT NULL DEFAULT 0,
    ratio NUMERIC(20, 8) NOT NULL DEFAULT 0,
    delisting_reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX corporate_actions_idempotency_idx
    ON corporate_actions(stock_id, action_type, effective_date, amount, ratio, source);

CREATE TABLE universes (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    definition_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

CREATE TABLE market_universe_memberships (
    universe_id UUID NOT NULL REFERENCES universes(id),
    stock_id UUID NOT NULL REFERENCES stocks(id),
    valid_from DATE NOT NULL,
    valid_to DATE,
    source TEXT NOT NULL,
    PRIMARY KEY (universe_id, stock_id, valid_from),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX universe_memberships_history_idx
    ON market_universe_memberships(universe_id, valid_from, valid_to);

CREATE TABLE data_snapshots (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    description TEXT NOT NULL,
    content_hash CHAR(64) NOT NULL,
    row_counts_json JSONB NOT NULL,
    date_range_start DATE,
    date_range_end DATE,
    survivorship_biased BOOLEAN NOT NULL,
    provider_summary_json JSONB NOT NULL
);

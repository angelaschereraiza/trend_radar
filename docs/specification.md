# TrendRadar – Software Specification

**Repository:** `trend_radar`
**Language:** English for source code, documentation, API contracts and tests
**Implementation language:** Go (backend, analytics, backtesting) and TypeScript (frontend). No Python.

## 1. Project Overview

TrendRadar is a data-driven stock market analysis platform designed to identify emerging investment opportunities over medium-term investment horizons of approximately 1 to 6 months.

The application collects historical and current market data, calculates technical indicators, ranks stocks based on configurable strategies and evaluates those strategies using historical backtesting under realistic cost assumptions for a small private investor.

The system must prioritize data accuracy, reproducibility, testability and maintainability.

TrendRadar is a research tool. It does not provide investment advice, and no output of the system is a recommendation.

### 1.1 Investment Constraint: Maximum 5,000 CHF

This constraint is a core design parameter, not a cosmetic limit.

- The **total capital** managed by the system (backtest, paper trading, rankings with position sizing) must never exceed **5,000 CHF**.
- The ceiling is defined as a compile-time constant (`domain.MaxCapitalCHF = 5000`). Configuration may lower the value but never raise it. Raising it requires a code change and a specification change.
- No additional deposits during a simulation in MVP. Capital grows or shrinks only through profit and loss. Profits are reinvested; the 5,000 CHF cap applies to contributed capital, not to portfolio value.
- Any configuration, API request or CLI parameter with `initial_capital` greater than 5,000 CHF is rejected with a validation error.
- The base currency is **CHF**. All portfolio values, fees, equity curves and performance metrics are reported in CHF.

Consequences that must be reflected in the implementation:

1. **Position size is small.** With 5,000 CHF and 10 positions, each position is about 500 CHF. Fixed minimum commissions, FX spreads and stamp duty are a large share of each trade and must be modelled (Section 9.4).
2. **Whole shares by default.** Stocks whose price exceeds the per-position budget are not tradeable and must be excluded from the point-in-time eligible universe (unless fractional shares are explicitly enabled).
3. **Turnover is expensive.** The strategy must include turnover-reducing rules (rank buffer, minimum trade size, see Section 8).
4. **Liquidity is rarely a constraint at this size**, but a liquidity filter is still required to avoid untradeable or manipulated securities.
5. **Rankings display affordability.** Every ranking entry shows whether one position (at the configured position budget) can be bought in whole shares.

### Primary objectives

1. Collect and persist historical and current stock market data.
2. Identify stocks showing positive momentum and relative strength.
3. Calculate transparent, reproducible investment scores.
4. Provide interactive visualizations and stock rankings.
5. Backtest investment strategies against historical data under realistic small-account costs.
6. Compare strategies against passive market benchmarks in CHF.
7. Detect potential investment opportunities and generate alerts.
8. Support future integration of fundamental analysis, news sentiment and machine learning.

### Out of scope for MVP

- Automatic trading or brokerage order execution.
- High-frequency or intraday trading.
- Cryptocurrency trading.
- Predicting exact future stock prices.
- Portfolio optimization using machine learning.
- Multi-user authentication and subscription management.
- AI-generated buy or sell recommendations.
- Capital above 5,000 CHF.
- Tax reporting or tax optimisation (taxes are approximated only where stated in Section 9.4).
- Portfolios denominated in currencies other than CHF, and securities not quoted in USD.
- Python-based or other non-Go analytics components.

The architecture must allow these features to be introduced later without major restructuring.

---

## 2. Technology Stack

### Backend and analytics (Go only)

- Go, current stable version.
- Standard `net/http` (including the Go 1.22+ `ServeMux` routing) for REST endpoints.
- `database/sql` with `github.com/jackc/pgx/v5/stdlib` (pgx PostgreSQL driver). Use parameterized PostgreSQL queries (`$1`, `$2`, etc.).
- SQL migrations using golang-migrate or equivalent.
- Structured JSON logging using `log/slog`.
- OpenAPI 3.1, **spec-first**. Server interfaces and request/response types are generated from `contracts/openapi.yaml` (for example with oapi-codegen using the standard `net/http` target). Generated code is committed or generated in CI, but never edited by hand.
- Exact decimal arithmetic (for example `shopspring/decimal`) or fixed-point integers for all money, price and portfolio accounting. `float64` is permitted for indicators, scores and statistics, with documented tolerances.
- Optional numerical helpers from `gonum` for statistics (regression, distributions). Prefer the standard library where sufficient.
- Property-based testing with `pgregory.net/rapid` (or `testing/quick`) and Go native fuzzing for parsers.

Avoid unnecessary frameworks and abstractions. Do not use cgo-based or non-Go analytics runtimes.

### Frontend

- Vue 3 with Composition API.
- TypeScript in strict mode.
- Vite.
- Vue Router.
- Pinia for shared application state.
- PrimeVue for UI components.
- Apache ECharts with vue-echarts for visualizations.
- Vitest and Vue Test Utils.
- Playwright for a small number of end-to-end tests.
- API types generated from `contracts/openapi.yaml`.

### Database

- PostgreSQL (current supported stable major version).
- UTC event timestamps stored as `TIMESTAMPTZ` (microsecond precision); configure application/database connections to UTC and always use timezone-aware values.
- JSON document columns (`*_json`, including definitions, configurations, parameters and component scores) use `JSONB`.
- Exchange trading days stored as `DATE` (never converted through UTC).
- Appropriate PostgreSQL B-tree composite indexes, unique constraints and foreign keys. Use `ON CONFLICT` for idempotent upserts, with explicit conflict/version handling.
- Versioned PostgreSQL database migrations. Use PostgreSQL-compatible SQL, not MySQL/MariaDB syntax (`AUTO_INCREMENT`, backticks, `ON DUPLICATE KEY UPDATE`, `GET_LOCK`).
- No TimescaleDB dependency, no second database engine. Use native PostgreSQL indexes and partitioning only when justified by measured workloads.

### Analytics execution model

All analytics run as Go batch jobs or CLI processes (`cmd/worker`, `cmd/backtest`), not as a permanently running separate service.

For backtests, the engine loads the required price, FX and universe data for the simulated date range **once** into in-memory columnar structures (slices indexed by stock and trading day) and does not query the database inside the simulation loop.

### Infrastructure

- Podman Compose.
- Environment-based configuration.
- Health checks.
- Persistent database storage.
- GitHub Actions CI.

---

## 3. Repository Structure

```text
trend_radar/
├── backend/
│   ├── cmd/
│   │   ├── api/            # REST API server
│   │   ├── collector/      # market data and FX import CLI
│   │   ├── worker/         # scheduled jobs: indicators, rankings, alerts, quality checks
│   │   └── backtest/       # backtest and validation CLI
│   ├── internal/
│   │   ├── config/
│   │   ├── domain/         # core types, constants (MaxCapitalCHF), validation
│   │   ├── calendar/       # exchange trading calendar (NYSE), rebalance dates
│   │   ├── marketdata/     # provider interface, importer, validation
│   │   ├── fx/             # FX rates and USD→CHF conversion
│   │   ├── indicators/     # pure indicator functions
│   │   ├── scoring/        # factor normalisation and score computation
│   │   ├── strategy/       # selection, target weights, exit rules
│   │   ├── backtest/       # engine, execution model, portfolio ledger, cost model
│   │   ├── metrics/        # performance metrics
│   │   ├── stats/          # bootstrap, Deflated Sharpe Ratio, trial registry logic
│   │   ├── validation/     # bias checks, data splits, walk-forward
│   │   ├── alerts/
│   │   ├── repository/     # PostgreSQL access
│   │   ├── scheduler/
│   │   └── api/            # HTTP handlers (generated interfaces + implementation)
│   ├── migrations/
│   ├── testdata/           # fixtures, golden datasets
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   ├── views/
│   │   ├── stores/
│   │   ├── services/
│   │   └── types/
│   ├── e2e/
│   └── package.json
├── contracts/
│   └── openapi.yaml
├── deployments/
│   └── compose.yaml
├── docs/
├── .github/workflows/
├── .env.example
└── README.md
```

Use domain-oriented packages. Do not introduce a generic repository or service abstraction unless it provides a concrete benefit.

**Package dependency rules**

- `indicators`, `scoring`, `strategy`, `metrics`, `stats`, `calendar` and `backtest` are pure: no database access, no HTTP, no wall-clock access (time is always passed in).
- `repository` and `api` depend on domain packages, never the other way round.
- Only `internal/indicators` and `internal/scoring` implement indicator and scoring formulas. No other package may re-implement them.

---

## 4. Data Strategy

Data quality is the primary risk of this project. A backtest on flawed data produces misleading results regardless of code quality.

### 4.1 Provider Requirements

A market data provider is acceptable for backtesting only if it supplies:

1. Daily OHLCV for US-listed equities, including delisted securities.
2. Both raw and corporate-action-adjusted prices, or raw prices plus a complete corporate action history.
3. Split and dividend history.
4. A usable license for personal research use.
5. Either historical index membership (for example S&P 500 constituents over time) or enough data to reconstruct a point-in-time universe by rule.

The provider decision is recorded in `docs/data-sources.md` including cost, coverage, licensing and known gaps. **Until a provider with delisted-security coverage is chosen, results must be labelled "survivorship-biased" in every report, API response and UI view that shows backtest results.** This flag is a stored property of the data snapshot.

### 4.2 Universe Definition

- The default research universe is defined by rule before any result is evaluated (for example: US common stocks, constituents of a defined index at each date, minimum price, minimum average dollar volume).
- The universe definition is versioned and stored. Changing it creates a new version and invalidates comparisons with earlier runs.
- If only current index members are available, this is a documented bias and sets the `survivorship_biased` flag.

### 4.3 Delisting Policy

Delisting treatment is a fixed, documented policy, applied identically in all runs:

| Delisting reason | Treatment |
|---|---|
| Acquisition for cash | Position closed at the deal price on the effective date (cash credited). |
| Acquisition for stock | Converted into acquirer shares by the documented ratio, otherwise closed at last available price. |
| Bankruptcy / liquidation | Position closed at last available price multiplied by a configurable recovery factor (default 0.0 = total loss unless proven otherwise by provider data). |
| Unknown | Closed at last available price with a configurable haircut (default 30%). |

If the provider does not supply delisting reasons, the "Unknown" row applies and the run is flagged.

### 4.4 Data Snapshots

Reproducibility requires a precise definition of "the data used":

**data_snapshots**

- id
- created_at
- description
- content_hash (SHA-256 over canonically ordered price bars, corporate actions, FX rates and universe memberships included in the snapshot)
- row_counts_json
- date_range_start
- date_range_end
- survivorship_biased (boolean)
- provider_summary_json

A backtest run references exactly one snapshot. Imports after the snapshot creation never alter a snapshot (snapshots are logical: they record the identifying hash and the import cut-off `imported_at <= snapshot.created_at`; data corrections create a new version, see 5.2).

### 4.5 Currency Data

Prices are in USD. The portfolio is in CHF. The system therefore requires historical **USD/CHF daily FX rates** (source and licensing documented). Missing FX rates are never invented; the last known rate may be used only within a configurable maximum gap (default 3 calendar days), otherwise the day is invalid for execution.

---

## 5. Market Data Collection

### 5.1 Data Provider

Implement a provider-independent market data interface.

```go
type MarketDataProvider interface {
    GetSymbols(ctx context.Context) ([]Symbol, error)
    GetHistoricalPrices(
        ctx context.Context,
        symbol string,
        from time.Time,
        to time.Time,
    ) ([]PriceBar, error)
    GetCorporateActions(
        ctx context.Context,
        symbol string,
        from time.Time,
        to time.Time,
    ) ([]CorporateAction, error)
}

type FXProvider interface {
    GetDailyRates(
        ctx context.Context,
        base string,
        quote string,
        from time.Time,
        to time.Time,
    ) ([]FXRate, error)
}
```

The initial implementation supports one configurable provider that meets the requirements of Section 4.1.

Do not assume that free provider tiers contain complete historical coverage.

Provider credentials must be loaded from environment variables.

A deterministic fixture provider (market data and FX) must be available for tests and local development.

### 5.2 Historical Data

Collect daily OHLCV data:

- Open
- High
- Low
- Close
- Volume
- Trading date
- Currency
- Source/provider
- Adjustment metadata

Preserve the distinction between raw and corporate-action-adjusted prices.

The system must support splits, dividends and delistings when computing total returns.

Do not mix adjusted and unadjusted price series within the same calculation. Indicators use one explicitly selected series type (default: adjusted close for indicators, raw open for execution, with share quantities and cash adjusted through explicit corporate-action events in the portfolio ledger).

When a provider re-adjusts historical prices (for example after a new split), the new values are stored as a **new data version** rather than overwriting. Conflicts are reported (see below).

### 5.3 Collection Requirements

- Initial historical import.
- Incremental daily updates.
- Configurable stock universe.
- API rate limiting.
- Retry with exponential backoff for transient errors.
- Idempotent imports.
- Duplicate detection.
- Data validation.
- Logging of failed imports.
- Recovery after interrupted imports.

Data ingestion must not silently overwrite conflicting historical values.

### Unit Tests: Market Data

| Test | Expected result |
|---|---|
| Valid OHLCV response | Correct domain objects |
| Empty response | Explicit empty result |
| Invalid JSON | Descriptive error |
| Missing required fields | Validation error |
| Negative price | Rejected |
| Negative volume | Rejected |
| High below low | Rejected |
| Duplicate price bar | No duplicate persistence |
| HTTP 429 | Rate-limit handling |
| HTTP 500 | Retry according to policy |
| HTTP 401 | Authentication error, no pointless retry |
| Network timeout | Controlled failure |
| Context cancellation | Processing stops |
| Repeated import | Idempotent result |
| Conflicting historical value | Conflict reported, no silent overwrite |
| Stock split | Correct adjustment handling |
| Missing FX rate | Day marked invalid, no invented rate |
| FX gap within tolerance | Last known rate used and flagged |
| FX gap beyond tolerance | Day invalid for execution |

Use `httptest` servers. Unit tests must never require a real market data API.

Fuzz tests are required for all provider response parsers.

---

## 6. Trading Calendar

A dedicated `calendar` package provides the exchange trading calendar. It is used by indicators (lookback in trading sessions), the strategy (rebalance dates), the backtest engine (execution dates) and the scheduler.

Requirements:

- NYSE trading days, holidays and early closes for the full supported date range, from a documented static source checked into the repository (`testdata`/embedded) and verified in tests.
- Function set: `IsTradingDay`, `NextTradingDay`, `PreviousTradingDay`, `AddTradingDays`, `LastTradingDayOfMonth`, `TradingDaysBetween`.
- Trading days are `civil dates` (year, month, day) in the exchange timezone `America/New_York`. They are never derived by converting UTC timestamps.
- Scheduler jobs convert between Europe/Zurich, UTC and America/New_York explicitly and are tested across DST transitions (including the weeks where US and European DST differ).

### Unit Tests: Calendar

- Known holidays (for example New Year, Independence Day, Thanksgiving, Christmas) are non-trading days.
- Observed holidays when a holiday falls on a weekend.
- Early-close days are trading days.
- `LastTradingDayOfMonth` for months ending on weekends and holidays.
- `AddTradingDays` forward and backward across holidays.
- DST transition handling for scheduler conversions.
- Dates outside the supported range return an explicit error.

---

## 7. Database Design

### 7.1 Core Tables

**stocks**

- id
- symbol
- exchange
- name
- currency
- sector
- industry
- active
- created_at
- updated_at

A symbol alone must not be treated as a permanent security identifier. Historical symbol changes and reused tickers must be supported through stable internal IDs and a **symbol_history** table (`stock_id`, `symbol`, `valid_from`, `valid_to`).

**stock_prices**

- stock_id
- trading_date (`DATE`)
- open
- high
- low
- close
- adjusted_close
- volume
- provider
- data_version
- imported_at (UTC)

Unique constraint on stock, date, provider and data_version.

Index on `(stock_id, trading_date)`.

Additional indexes must be justified using actual query plans.

**fx_rates**

- base_currency
- quote_currency
- rate_date (`DATE`)
- rate
- provider
- imported_at

**corporate_actions**

- id
- stock_id
- action_type (split, dividend, delisting, symbol_change, merger)
- effective_date
- amount
- ratio
- delisting_reason
- source

**market_universe_memberships**

- universe_id
- stock_id
- valid_from
- valid_to
- source

Membership must be historically reconstructable.

**universes**

- id
- name
- version
- definition_json
- created_at

**data_snapshots** (see Section 4.4)

**indicators**

- stock_id
- calculation_date
- indicator_type
- period
- value
- calculation_version

**strategy_definitions**

- id
- name
- version
- parameters_json
- created_at

**stock_scores**

- stock_id
- strategy_id
- calculation_date
- score
- component_scores_json
- calculation_version

**backtest_runs**

- id
- strategy_id
- universe_id
- start_date
- end_date
- configuration_json
- data_snapshot_id
- code_version
- dataset_role (development, walk_forward, holdout, forward)
- survivorship_biased
- status
- created_at
- finished_at

**backtest_orders**

- id
- run_id
- stock_id
- decision_date
- intended_execution_date
- side (buy, sell)
- requested_quantity
- reason (rebalance_entry, rebalance_exit, max_holding, delisting, corporate_action)
- status (filled, partially_filled, rejected)
- reject_reason

**backtest_fills**

- id
- order_id
- execution_date
- quantity
- price_usd
- fx_rate_usd_chf
- gross_value_chf
- commission_chf
- fx_cost_chf
- stamp_duty_chf
- slippage_chf

**backtest_cash_ledger**

- id
- run_id
- booking_date
- entry_type (initial_capital, buy, sell, commission, fx_cost, stamp_duty, dividend_gross, withholding_tax, delisting_proceeds, interest)
- amount_chf
- related_fill_id
- balance_after_chf

**backtest_positions** (derived read model, rebuilt from the ledger and fills)

- run_id
- stock_id
- entry_date
- exit_date
- entry_price
- exit_price
- quantity
- fees
- realized_pnl

**backtest_equity**

- run_id
- trading_date
- portfolio_value_chf
- cash_balance_chf
- invested_value_chf
- benchmark_value_chf

**experiment_trials**

- id
- strategy_name
- strategy_version
- parameters_hash
- parameters_json
- dataset_role
- data_snapshot_id
- run_id
- primary_metric_value
- created_at

Every backtest run on development or walk-forward data creates a trial row. This table is the basis for the multiple-testing correction (Section 11.4).

**holdout_access_log**

- id
- strategy_name
- strategy_version
- run_id
- accessed_at
- note

**job_runs**

- id
- job_name
- scheduled_for
- started_at
- finished_at
- status
- error_message
- lock_owner

**alerts**

- id
- stock_id
- strategy_id
- alert_type
- message
- created_at
- acknowledged_at

### 7.2 Database Requirements

- All migrations must have defined upgrade and rollback behavior.
- Foreign key integrity must be enforced.
- Financial values must use suitable decimal precision where exact representation is required (for example PostgreSQL `NUMERIC(20,8)` for prices and `NUMERIC(20,4)` for CHF amounts; precision documented per column).
- Date-only market observations are stored as PostgreSQL `DATE` and never converted into arbitrary UTC trading days. Event timestamps use `TIMESTAMPTZ` and are normalized to UTC at the application boundary.
- The ledger tables are append-only in application code.
- Schema must support reproducible backtesting.

### Integration Tests

- Apply migrations to an empty PostgreSQL instance.
- Verify foreign keys.
- Verify uniqueness constraints.
- Verify rollback behavior.
- Verify transaction rollback on failure.
- Verify historical queries.
- Verify repeated imports.
- Verify index usage with `EXPLAIN (ANALYZE, BUFFERS)` for representative queries; assert correctness independently of planner-dependent plan choices.
- Verify that `DATE` columns round-trip without timezone shifts.
- Verify `TIMESTAMPTZ` round-trips and UTC normalization across connection time zones.
- Verify `JSONB` persistence and decoding, `NUMERIC` precision, `INSERT ... ON CONFLICT` idempotency, and atomic job-lock acquisition/recovery.

---

## 8. Technical Indicators

Implement the following indicators as pure, independently testable Go functions in `internal/indicators`. Input is a slice of observations sorted by date; functions return a result series aligned with the input and explicit "not available" markers for insufficient history (no zero-fill, no NaN leaking into scores without handling).

### 8.1 Simple Moving Average

Support:

- SMA20
- SMA50
- SMA200

Formula:

`SMA(n) = sum(last n closing prices) / n`

### 8.2 Exponential Moving Average

Support:

- EMA20
- EMA50

Initialisation convention: the first EMA value equals the SMA of the first `n` observations; `alpha = 2 / (n + 1)`. This convention is documented and applied consistently.

### 8.3 Momentum

Support:

- 1-month return
- 3-month return
- 6-month return
- 12-month return
- 12-1 momentum, excluding the most recent month

Explicit trading-day lookback conventions: 21, 63, 126 and 252 trading sessions. 12-1 momentum is `Price(t-21) / Price(t-252) - 1`.

### 8.4 Relative Strength

Compare stock performance against a benchmark over identical periods.

`RelativeReturn(n) = StockReturn(n) - BenchmarkReturn(n)`

The benchmark series for relative strength is the benchmark in its native currency (USD) so that relative strength is not distorted by FX movements. The date alignment rule (identical trading sessions) is enforced and tested.

### 8.5 Volatility

Annualized standard deviation of daily log returns, using the sample standard deviation (n-1 denominator) and annualization factor `sqrt(252)`. Default window: 126 sessions (configurable).

### 8.6 Relative Volume

`RelativeVolume = Volume(t) / mean(Volume(t-n .. t-1))`, default `n = 50`. The current day is excluded from the average. A zero average volume yields "not available".

### 8.7 RSI

RSI14 using Wilder's smoothing, documented initialisation (simple average of the first 14 gains and losses). Output is bounded to [0, 100]. A zero average loss yields 100; zero average gain and loss yields 50 (documented convention).

### 8.8 Trend Quality

Trend quality measures how smooth and persistent an uptrend is. Definition (default window 126 sessions):

1. Fit a linear regression of `ln(adjusted_close)` on the session index.
2. `slope_annualized = slope * 252` (log-return per year).
3. `r_squared` = coefficient of determination of the fit.
4. `TrendQuality = slope_annualized * r_squared` (a steep but erratic trend is penalised).

Trend quality is reported in raw form and normalised in scoring.

### 8.9 Volatility-Adjusted Momentum

`VolAdjMomentum = Momentum12-1 / Volatility(126)`. Volatility of zero yields "not available".

### Unit Tests: Indicators

Every indicator must have tests covering:

- Known reference datasets with manually verified expected values.
- Empty input.
- Insufficient history.
- Constant prices.
- Increasing prices.
- Decreasing prices.
- Missing observations.
- Duplicate dates.
- Unsorted input.
- Zero volume.
- Numerical precision.
- Non-mutation of input data.
- Deterministic output.

Additional requirements:

- SMA must return the expected arithmetic mean.
- Momentum must correctly calculate positive and negative returns.
- Relative strength must compare matching date ranges.
- RSI must remain between 0 and 100.
- Volatility must be zero for constant returns.
- Trend quality must be (near) zero for constant prices and must be penalised for noisy paths with the same endpoints.
- Future data must never influence historical indicator values (appending data leaves earlier values unchanged; tested as a property).

Use numerical tolerance assertions where floating-point arithmetic is involved. Critical invariants are additionally covered by property-based tests.

---

## 9. Investment Scoring

### 9.1 Objective

Generate a transparent ranking of stocks based on measurable market indicators.

Initial scoring factors:

| Factor | Raw input | Weight |
|---|---|---:|
| Momentum | 12-1 momentum | 30% |
| Relative strength | 6-month relative return vs benchmark | 20% |
| Trend quality | Trend quality (Section 8.8) | 20% |
| Relative volume | Relative volume (Section 8.6) | 15% |
| Volatility-adjusted momentum | Section 8.9 | 15% |

These weights are initial experimental parameters, not validated investment recommendations.

**Correlation note.** Momentum, relative strength and volatility-adjusted momentum are highly correlated, so the weights do not imply equivalent diversification. Every scoring run must produce a **factor correlation matrix** (cross-sectional rank correlation per date, averaged over time) that is stored and shown in documentation and the UI. Weight changes are experiments and are recorded as trials (Section 11.4).

### 9.2 Scoring Rules

- Each factor is normalised **cross-sectionally** against the eligible universe on the same date using **percentile rank** (0–100) after winsorising the raw values at the 1st and 99th percentile. Ties receive the average rank.
- Combined score = weighted sum of factor percentile ranks; range 0 to 100.
- Higher scores represent stronger ranking signals.
- Missing data is handled explicitly: a stock missing any required factor is **not scored** and is listed as "excluded: missing data" with the reason. No imputation in MVP.
- No future observations may be used.
- Scores must be reproducible from stored inputs and configuration.
- Every score must expose its component contributions.
- Deterministic tie-breaking: equal scores are ordered by higher 12-1 momentum, then by symbol/stock id ascending.

Do not interpret a score of 90 as a 90% probability of positive returns.

### Unit Tests: Scoring

- All minimum inputs produce a valid minimum score.
- All maximum inputs produce a valid maximum score.
- Increasing a positively weighted factor cannot decrease the score when other inputs are fixed (property test; note: percentile ranks are cross-sectional, so the property is tested on rank-preserving transformations).
- Weights must sum to 1.
- Invalid weights must be rejected.
- Missing required indicators must be handled consistently (stock excluded, reason reported).
- Equal inputs produce equal scores.
- Ranking must be deterministic.
- Scores must stay within 0–100.
- Winsorising reduces the influence of extreme outliers.
- Historical scores must not change when future observations are appended.
- Normalisation uses only data of the scoring date (no cross-date leakage).

---

## 10. Investment Strategy

### 10.1 Initial Strategy

Implement a configurable momentum-based strategy. Parameters reflect the 5,000 CHF constraint.

Default configuration:

```yaml
strategy:
  name: momentum_v1
  base_currency: CHF
  initial_capital_chf: 5000          # hard ceiling: 5000
  benchmark: SPY
  rebalance_frequency: monthly       # signal on last trading day of month
  max_positions: 10
  weighting: equal
  rank_buffer: 15                    # held position kept while rank <= 15
  max_holding_months: 3              # forced exit after 3 months
  min_trade_value_chf: 150           # smaller rebalance trades are skipped
  fractional_shares: false
  eligibility:
    min_history_sessions: 252
    min_price_usd: 5
    min_avg_dollar_volume_usd: 5000000
    avg_dollar_volume_window: 50
    max_position_budget_share: 1.0   # price must fit into position budget (whole shares)
  costs:
    commission_bps: 5
    commission_min_chf: 1.00         # PLACEHOLDER: replace with the real broker schedule
    fx_spread_bps: 25                # PLACEHOLDER per currency conversion
    stamp_duty_bps: 30               # Swiss turnover tax, foreign securities, per side; verify
    slippage_bps: 10
  dividends:
    withholding_tax_rate: 0.15       # US withholding with valid W-8BEN; verify
```

These values are hypotheses and must be configurable (except the capital ceiling). The cost defaults are placeholders and must be replaced with the actual fee schedule of the intended broker before results are interpreted. This document does not constitute tax or legal advice; the stamp duty and withholding tax values must be verified by the user.

### 10.2 Stock Selection

At each rebalance date (signal on the last trading day of the month, after the close):

1. Determine the historically eligible stock universe (point-in-time membership).
2. Exclude stocks with insufficient historical data.
3. Apply eligibility filters: minimum price, minimum average dollar volume, and **affordability** (one share, converted to CHF at the decision-date FX rate, must not exceed the position budget `portfolio_value / max_positions`, unless fractional shares are enabled).
4. Calculate indicators using only information available at that time.
5. Score and rank eligible stocks (Section 9).
6. Determine the target portfolio (Section 10.3).
7. Generate target weights.
8. Create orders executed at the next permitted execution timestamp (Section 11.2).

### 10.3 Exit and Rebalancing Rules (fixed for MVP)

Rules are evaluated in this order at each rebalance:

1. **Forced exit (max holding):** a position held for at least `max_holding_months` is sold, regardless of rank. It may be re-bought in a later rebalance if it qualifies again.
2. **Rank exit:** a held position whose rank is worse than `rank_buffer` (or that is no longer eligible, or has no valid score) is sold.
3. **Keep:** a held position within `rank_buffer` is kept even if outside the top `max_positions` (rank buffer reduces turnover and cost drag).
4. **Entry:** free slots (up to `max_positions`) are filled with the highest-ranked eligible stocks not already held.
5. **Sizing:** target weight is `1 / max_positions` per slot (equal weighting). Unfilled slots remain in cash.
6. **Trade filter:** rebalancing trades smaller than `min_trade_value_chf` are skipped. Existing positions are not resized for drift unless the deviation exceeds the threshold. New entries are always sized to the target slot value.
7. **Delisting:** handled by the delisting policy (Section 4.3), independent of rank.

There are no stop-loss rules in MVP.

### 10.4 Portfolio Rules

- Maximum number of positions.
- Configurable equal weighting.
- No leverage in MVP.
- No short selling in MVP.
- No fractional shares unless explicitly enabled.
- No spending more cash than available (cash includes the estimated costs of the order; an order that would overdraw cash is reduced by whole shares or rejected).
- Configurable fees, FX costs, stamp duty and slippage.
- Explicit treatment of dividends (including withholding tax), splits and delistings.
- Explicit treatment of cash when insufficient stocks qualify. Cash earns no interest unless a cash rate is configured (default 0).
- Deterministic handling of simultaneous signals (sorted by score, then tie-breaking rules; sells are processed before buys on the same day).
- Total capital never exceeds `domain.MaxCapitalCHF` (Section 1.1).

### Unit Tests: Strategy

- Correct top-N selection.
- Insufficient eligible stocks (remaining slots stay in cash).
- Correct equal weighting.
- Maximum position limit.
- No negative cash from purchases.
- No duplicate positions.
- Correct rebalance dates.
- Correct holding period handling (forced exit after `max_holding_months`).
- Rank buffer keeps positions within the buffer and exits positions beyond it.
- Minimum trade value skips small trades.
- Affordability filter excludes stocks that do not fit into the position budget.
- Fractional shares enabled changes affordability behaviour.
- Correct cash preservation.
- Correct handling of delisted stocks.
- Correct treatment of missing trading days.
- Deterministic tie-breaking.
- No trading using unavailable future data.
- `initial_capital_chf > 5000` is rejected.

---

## 11. Backtesting Engine

This is the most critical component.

### 11.1 Requirements

The engine must simulate historical portfolio performance using only information available at each simulated decision point.

Support:

- Configurable initial capital (maximum 5,000 CHF).
- Historical stock universe.
- Strategy selection.
- Configurable date range.
- Transaction fees, FX costs, stamp duty and slippage.
- Corporate actions.
- Cash balance.
- Position tracking through an order / fill / cash-ledger model.
- Benchmark comparison in CHF.
- Portfolio equity history.
- Trade history.
- Reproducible simulation runs.

Portfolio accounting uses exact decimal or fixed-point arithmetic. Every cash movement is a ledger entry. Portfolio value on any date is derivable from the ledger, fills and prices; equity curves are checked against that derivation in tests.

### 11.2 Execution Model

Use a documented daily-bar execution model.

Default:

- Signals calculated after the market close of the decision date.
- Orders executed no earlier than the next trading session.
- Execution price based on the next available opening price.
- Fees, FX costs, stamp duty and slippage applied to executed orders.
- Prices are converted USD → CHF using the FX rate of the execution date (documented convention; the same rate source is used consistently).
- No assumption that an order can execute at a price that was unavailable when the decision was made.
- If the execution day has no valid price or FX rate for a stock, the order is not executed on that day; it is rejected or deferred according to the configured policy (default: rejected and logged). A price is never invented.

The engine must reject any strategy attempting to access future data. This is enforced structurally: strategies receive a read-only `MarketView` that can only return data with date ≤ decision date.

### 11.3 Performance Metrics

Calculate (all in CHF):

- Total return
- CAGR
- Annualized volatility
- Sharpe ratio
- Sortino ratio
- Maximum drawdown
- Calmar ratio
- Win rate
- Average trade return
- Portfolio turnover
- Total fees (split into commission, FX cost, stamp duty, slippage)
- Total withholding tax
- Benchmark return
- Excess return
- Exposure over time
- Fee drag (total costs as a percentage of average portfolio value, per year)

Metric definitions must be documented, including annualization assumptions (252 sessions), the risk-free rate (configurable CHF reference rate; default 0 if no series is available, documented explicitly) and treatment of days without positions.

### 11.4 Cost Model for a Small Account

For each fill, in CHF:

```
gross_value      = quantity * execution_price_usd * fx_rate
slippage         = gross_value * slippage_bps / 10000      (applied adversely to the price)
commission       = max(commission_min_chf, gross_value * commission_bps / 10000)
fx_cost          = gross_value * fx_spread_bps / 10000
stamp_duty       = gross_value * stamp_duty_bps / 10000
total_cost       = slippage + commission + fx_cost + stamp_duty
```

- The same cost model is applied to buys and sells.
- Dividends are credited net of the configured withholding tax; withholding tax is a separate ledger entry.
- Reports must show the break-even gross return needed to cover costs, so the effect of small position sizes is visible.
- The cost model is a pure function and has its own table-driven tests.

### 11.5 Backtesting Unit Tests

| Scenario | Expected result |
|---|---|
| Constant prices, zero costs | Zero gross return |
| Increasing prices | Correct positive return |
| Decreasing prices | Correct negative return |
| Buy and hold | Matches independent reference calculation |
| Transaction fees | Reduce portfolio value |
| Minimum commission on small order | Minimum applied |
| FX spread and stamp duty | Applied on every buy and sell |
| Slippage | Reduces performance under configured model |
| No trades | Capital remains in cash |
| Insufficient cash | Order rejected or reduced |
| Price above position budget | Not purchased (whole shares) |
| Stock split | Share quantity and price adjusted consistently |
| Dividend | Cash credited net of withholding tax |
| Delisting | Position resolved according to documented policy |
| Missing price | No invented execution |
| Missing FX rate | No execution |
| Multiple positions | Correct aggregate portfolio value |
| Drawdown | Correct peak-to-trough calculation |
| Same input and configuration | Identical results (including ledger) |
| Future data appended | Earlier decisions remain unchanged |
| Incorrect historical universe | Validation failure |
| `initial_capital_chf > 5000` | Validation failure |
| Ledger consistency | Sum of ledger entries equals cash balance on every date |

Create a small synthetic dataset (a few stocks, a few months, simple prices and FX rate) for which every trade, cash movement and final portfolio value in CHF can be calculated manually.

This dataset must serve as a **golden reference test** and must be committed under `backend/testdata/golden/` together with the manual calculation in a document next to it.

---

## 12. Backtesting Validation

Historical profitability alone is not sufficient.

### 12.1 Data Splitting

Support separate datasets for:

- Strategy development.
- Walk-forward validation.
- Final untouched out-of-sample testing (holdout).
- Forward paper trading.

Recommended initial split (configurable, subject to actual data availability; 12-month momentum requires a one-year warm-up before the first signal):

| Role | Period | Rationale |
|---|---|---|
| Development + walk-forward | 2004-01-01 to 2017-12-31 | Contains the 2008 financial crisis, the 2009–2017 recovery and several drawdowns |
| Final holdout | 2018-01-01 to 2025-12-31 | Contains Q4 2018, the 2020 crash, the 2022 bear market and different regimes |
| Forward paper trading | 2026-01-01 onward | Real-time, out of sample |

If data does not reach back to 2004, the development period is shortened and the limitation is documented in the report. A development period that consists only of one bull market must be flagged in every report.

The final holdout must not be used for repeated parameter optimization. Enforcement:

- Every run with `dataset_role = holdout` writes a row to `holdout_access_log`.
- The `backtest` CLI and API require an explicit confirmation flag for holdout runs.
- A second holdout run for the same strategy name and version is reported with a prominent "holdout already used" warning in the result and stored in the report.

### 12.2 Walk-Forward Validation

- Rolling or expanding training windows with strictly later test windows (configurable lengths; default: 5-year training, 1-year test, 1-year step).
- Parameters are chosen only on the training window; the test window is evaluated once.
- Results of all test windows are concatenated into one out-of-sample equity curve.

### 12.3 Bias Prevention

Explicitly test for:

**Look-ahead bias**

Only historically available data may influence decisions.

**Survivorship bias**

Historical universe membership must include securities that were later removed or delisted. Runs on data without this property carry the `survivorship_biased` flag.

**Selection bias**

The universe definition must be fixed before evaluating results.

**Overfitting**

Track tested parameter combinations in `experiment_trials` and avoid selecting strategies solely by the highest historical return.

**Data leakage**

Normalisation, feature calculation and any parameter fitting must respect chronological boundaries.

### 12.4 Statistical Significance (mandatory)

Implemented in `internal/stats` as pure, seeded and deterministic functions:

- **Bootstrap confidence intervals** (stationary bootstrap on daily returns, fixed seed, configurable block length) for Sharpe ratio, CAGR and excess return versus benchmark.
- **Deflated Sharpe Ratio** using the number of recorded trials from `experiment_trials`, accounting for skewness, kurtosis and sample length.
- Every performance report states the number of trials considered and the deflated result. A strategy is not labelled "promising" if the bootstrap confidence interval of the out-of-sample excess return includes zero.
- Optional later extension: White's Reality Check or SPA test (not required for MVP).

### 12.5 Benchmark Comparison

Compare each strategy against (all in CHF, same date range and return conventions):

1. **SPY** converted to CHF (adjusted close as total-return proxy). If an S&P 500 total-return index series is available, it is used as an additional reference. The distinction between SPY and the index is documented.
2. Passive buy-and-hold: equal-weight purchase of the same number of positions at the start, no rebalancing, same cost model.
3. A simple momentum baseline: 12-1 momentum top-N, equal weight, monthly, with the same costs.
4. Cash / CHF risk-free reference.

### 12.6 Validation Tests

- Changing future data cannot change past decisions.
- Training data never contains validation observations.
- Historical universe membership matches the simulated date.
- Fees, FX costs, stamp duty and slippage are included.
- Benchmark returns use the same date range and CHF conversion.
- Currency conversions are consistent (same FX source and convention for portfolio and benchmark).
- Every backtest stores its configuration, data snapshot and code version.
- Holdout runs are logged and repeated holdout use is flagged.
- Bootstrap results are deterministic for a fixed seed.
- Deflated Sharpe Ratio decreases when the trial count increases (all else equal).

---

## 13. REST API

Base path: `/api/v1`

The API is defined **spec-first** in `contracts/openapi.yaml`. Handler interfaces and DTOs are generated; handlers contain only mapping and orchestration.

### Stocks

`GET /stocks`

Supports pagination, sorting and filtering.

`GET /stocks/{id}`

Returns stock metadata.

`GET /stocks/{id}/prices`

Returns historical OHLCV data.

`GET /stocks/{id}/indicators`

Returns calculated indicators.

### Rankings

`GET /rankings`

Query parameters:

- strategy
- date
- limit
- minimum_score
- sector
- affordable_only (boolean; only stocks that fit into one position budget)

Each entry includes score, component contributions, rank, and an `affordable` flag with the position budget used.

### Strategies

`GET /strategies`

`POST /strategies`

`GET /strategies/{id}`

Requests with `initial_capital_chf > 5000` return HTTP 400.

### Backtesting

`POST /backtests`

Starts a backtest (asynchronous; returns the run id).

`GET /backtests/{id}`

Returns execution status and summary, including data snapshot, `survivorship_biased` flag, dataset role and trial count.

`GET /backtests/{id}/equity`

Returns portfolio and benchmark equity series (CHF).

`GET /backtests/{id}/trades`

Returns simulated orders and fills, including cost breakdown.

`GET /backtests/{id}/metrics`

Returns performance metrics, confidence intervals and the Deflated Sharpe Ratio.

### Alerts

`GET /alerts`

`PATCH /alerts/{id}`

Acknowledges an alert.

### System

`GET /health/live`

`GET /health/ready`

`GET /jobs`

Returns recent job runs and status (for the dashboard's data collection status).

### API Requirements

- JSON request and response bodies.
- Consistent error responses.
- Input validation.
- Pagination.
- Stable identifiers.
- Appropriate HTTP status codes.
- Request timeouts.
- Context cancellation.
- OpenAPI documentation.

For MVP, bind the API to localhost by default. Do not expose it publicly without authentication and transport security.

### API Tests

- Valid request returns expected response.
- Invalid parameters return HTTP 400.
- Unknown resource returns HTTP 404.
- Unsupported method returns HTTP 405.
- Internal errors do not leak sensitive information.
- Pagination works correctly.
- Sorting is deterministic.
- Date filters work correctly.
- Empty results are valid.
- Request cancellation is respected.
- Response payloads conform to OpenAPI schemas.
- Capital above the ceiling is rejected.
- The generated code is up to date with `openapi.yaml` (CI check).

---

## 14. Frontend

### 14.1 Dashboard

Display:

- Market overview.
- Top-ranked stocks (with affordability).
- Recent signal changes.
- Strongest and weakest momentum.
- Latest data collection status.
- Recent backtest results (with survivorship and holdout warnings).

### 14.2 Stock Explorer

Features:

- Search by symbol or company name.
- Filter by sector.
- Filter "affordable only".
- Sort by score, momentum or volatility.
- View indicator values.
- Open detailed stock analysis.

### 14.3 Stock Details

Display:

- Company information.
- Historical price chart.
- SMA20, SMA50 and SMA200.
- Momentum metrics.
- Relative strength.
- Volume chart.
- Historical score development.
- Explanation of score components.
- Price in USD and CHF equivalent.

### 14.4 Strategy Configuration

Allow configuration of:

- Momentum lookback.
- Maximum positions.
- Rank buffer.
- Rebalancing frequency.
- Holding period.
- Transaction costs (commission, minimum commission, FX spread, stamp duty, slippage).
- Date range and dataset role.
- Initial capital (input limited to 5,000 CHF; validation message shown).

### 14.5 Backtesting Dashboard

Display:

- Portfolio equity curve (CHF).
- Benchmark equity curve (CHF).
- Drawdown chart.
- Performance metrics including confidence intervals and Deflated Sharpe Ratio.
- Cost breakdown and break-even gross return.
- Trade history.
- Portfolio allocations.
- Configuration used, data snapshot and code version.
- Warnings: survivorship-biased data, holdout already used, development period lacks a bear market.

### 14.6 Alerts

Display stocks whose scores or indicators cross configurable thresholds.

No real-time push notification system is required for MVP.

### Frontend Unit Tests

Use Vitest and Vue Test Utils.

Test:

- Correct rendering of stock rankings.
- Sorting and filtering.
- Pagination.
- Loading states.
- Empty states.
- API error handling.
- Chart data transformation.
- Score formatting.
- CHF and USD formatting.
- Date formatting.
- Strategy form validation (including the capital ceiling).
- Store updates.
- Backtest status display and warning banners.
- Alert acknowledgement.

Mock all HTTP requests in unit tests.

Add a small number of browser-level end-to-end tests (Playwright) for the most important workflows: viewing rankings, configuring and running a backtest, viewing results.

---

## 15. Scheduling

The application must support scheduled tasks for:

- Daily market data and FX collection.
- Indicator calculation.
- Stock ranking.
- Alert generation.
- Data quality checks.

Requirements:

- Jobs must be idempotent.
- Failed jobs must be logged.
- Job status must be persisted (`job_runs`).
- Concurrent duplicate execution must be prevented using PostgreSQL-native coordination (prefer a persisted `job_runs` lease with an atomic `INSERT ... ON CONFLICT` / conditional `UPDATE ... RETURNING`, or transaction-scoped `pg_try_advisory_xact_lock` for short transactions). Locks must have defined recovery after crashes; session-level advisory locks must not be used without explicit connection ownership and release handling.
- Jobs must support manual execution.
- Market holidays and trading calendars must be respected (Section 6).
- Scheduling is time-zone aware (Europe/Zurich host, America/New_York exchange, UTC storage).

Do not assume that every weekday is a trading day.

### Scheduler Tests

- Correct job scheduling.
- Duplicate execution prevention.
- Failed job recovery.
- Job cancellation.
- Holiday handling.
- Timezone and DST handling.
- Repeated execution produces consistent results.

---

## 16. Configuration

All environment-specific settings must be configurable.

Example:

```env
APP_ENV=development
API_HOST=127.0.0.1
API_PORT=8080

DB_HOST=localhost
DB_PORT=5432
DB_NAME=trend_radar
DB_USER=trend_radar
DB_PASSWORD=

MARKET_DATA_PROVIDER=
MARKET_DATA_API_KEY=
FX_PROVIDER=
FX_API_KEY=

BASE_CURRENCY=CHF
MAX_CAPITAL_CHF=5000        # may be lowered, never raised above the compiled ceiling
ALERT_SCORE_THRESHOLD=80    # score threshold for crossing alerts; 0-100

LOG_LEVEL=info
```

Never commit real credentials.

Provide `.env.example`.

Validate configuration during startup and return descriptive errors for missing required settings. `MAX_CAPITAL_CHF` greater than the compiled ceiling is a startup error.

---

## 17. Testing Strategy

All tests are written in Go (backend and analytics) or TypeScript (frontend).

### Backend and analytics

Use Go's standard `testing` package.

Requirements:

- Table-driven tests where appropriate.
- Dependency injection for external systems.
- No real HTTP requests in unit tests.
- No database dependency in pure unit tests.
- Deterministic fixture datasets in `backend/testdata/`.
- Numerical reference tests for indicators and metrics.
- Property-based tests (rapid or `testing/quick`) for critical invariants: no look-ahead, score bounds, ledger consistency, cash never negative, capital ceiling.
- Go fuzz tests for provider parsers.
- Golden-reference backtests.
- Explicit data leakage tests.
- Integration tests against PostgreSQL (build tag `integration`).
- Race detection in CI.

Commands:

```bash
go test ./...
go test -race ./...
go vet ./...
go test -cover ./...
go test -tags=integration ./...
```

### Frontend

Use Vitest.

Commands:

```bash
npm run test
npm run typecheck
npm run lint
npm run build
npm run e2e
```

### Integration Testing

Use a disposable PostgreSQL container.

Test the complete workflow:

1. Import fixture market data and FX rates.
2. Persist price history.
3. Calculate indicators.
4. Generate stock rankings.
5. Run a backtest.
6. Persist backtest results (orders, fills, ledger, equity).
7. Retrieve results through the REST API.

### Coverage Targets

Aim for:

- At least 80% meaningful coverage for backend domain logic.
- At least 90% coverage for critical financial calculations (indicators, scoring, metrics, costs).
- At least 90% coverage for backtesting execution and accounting logic.

Coverage is not a substitute for testing edge cases, numerical correctness and historical data integrity. Golden tests and property tests on the accounting and no-look-ahead invariants take priority over raw coverage numbers.

---

## 18. CI/CD

Configure GitHub Actions.

The pipeline must:

1. Run Go formatting checks (`gofmt`).
2. Run Go linting (`golangci-lint` or `staticcheck`).
3. Run Go unit tests.
4. Run Go race detection.
5. Run `go vet`.
6. Verify generated OpenAPI code is up to date.
7. Run frontend linting.
8. Run TypeScript checks.
9. Run frontend unit tests.
10. Build all applications.
11. Run PostgreSQL integration tests (service container).
12. Run frontend end-to-end tests.

Pull requests must fail when required checks fail.

No deployment to production is required for MVP.

---

## 19. Documentation

Create:

**README.md**

- Project description.
- Architecture overview.
- Requirements.
- Local setup.
- Configuration.
- Development commands.
- Test commands.
- Known limitations.
- Disclaimer: research tool, no investment advice, 5,000 CHF limit.

**docs/architecture.md**

- Component responsibilities.
- Data flow.
- Module boundaries.
- Database design.

**docs/strategy.md**

- Indicator definitions.
- Scoring formulas and normalisation.
- Investment strategy rules and exit rules.
- Risk assumptions.
- Factor correlation notes.

**docs/backtesting.md**

- Execution assumptions.
- Cost model for small accounts.
- Performance metrics.
- Historical data requirements.
- Bias prevention.
- Statistical validation (bootstrap, Deflated Sharpe Ratio, trial registry).
- Known limitations.

**docs/data-sources.md**

- Provider interfaces and the chosen provider.
- Data coverage and known gaps.
- Licensing restrictions.
- Adjustment policies.
- Delisting policy.
- FX data source.

**docs/swiss-investor-notes.md**

- Currency treatment (USD/CHF).
- Stamp duty and withholding tax assumptions and how to change them.
- Broker fee schedule placeholders to be filled in.
- Statement that taxes are approximated and not tax advice.

---

## 20. Implementation Milestones

Milestones are ordered so that the research core is complete and validated before the user-facing layers are built. **The full scope of Version 1.0 is retained; only the order changed.**

### Milestone 1: Project Foundation

Implement:

- Repository structure.
- Go backend skeleton.
- Vue frontend skeleton.
- PostgreSQL container.
- Configuration management (including capital ceiling).
- Database migrations.
- Health endpoints.
- OpenAPI skeleton and code generation.
- Basic CI pipeline.

**Acceptance criteria:** All components start locally and all initial tests pass.

### Milestone 2: Calendar, Market Data and FX

Implement:

- Trading calendar.
- Market data and FX provider interfaces.
- Historical OHLCV and corporate action import.
- Database persistence, data versions and conflict reporting.
- Data validation.
- Fixture provider.
- Stock REST endpoints.
- Data snapshots with content hash.

**Acceptance criteria:** Historical data can be imported, retrieved and reimported without duplication; snapshots are reproducible.

### Milestone 3: Indicators and Rankings

Implement:

- Go indicator library (all indicators of Section 8).
- Scoring engine with percentile normalisation.
- Factor correlation report.
- Ranking persistence.
- Ranking REST endpoints.
- Affordability flag.

**Acceptance criteria:** Rankings are deterministic and verified against reference datasets.

### Milestone 4: Strategy and Backtesting

Implement:

- Strategy selection and exit rules.
- Historical simulation engine.
- Order / fill / cash-ledger accounting.
- Trade execution model.
- Cost model (commission, FX, stamp duty, slippage) and dividend withholding.
- Performance metrics.
- Benchmark comparison in CHF.
- Golden-reference tests.

**Acceptance criteria:** Backtest results match independently calculated expected values.

### Milestone 5: Validation and Statistics

Implement:

- Dataset splitting and holdout access log.
- Walk-forward testing.
- Historical universe handling.
- Data leakage and look-ahead checks.
- Trial registry, bootstrap confidence intervals, Deflated Sharpe Ratio.
- Baseline strategies for comparison.
- Documented performance report.

**Acceptance criteria:** The system can run a reproducible, historically valid backtest and produce a documented performance report that includes statistical uncertainty and bias flags.

*Research checkpoint:* after this milestone the results are reviewed. This is a decision point for the user, not a removal of later milestones.

### Milestone 6: API Completion

Implement:

- Strategy endpoints.
- Backtest endpoints (start, status, equity, trades, metrics).
- Asynchronous backtest execution.
- API tests and schema conformance tests.

**Acceptance criteria:** All results are retrievable through the REST API and conform to the OpenAPI contract.

### Milestone 7: Frontend

Implement:

- Dashboard.
- Stock Explorer.
- Stock Details.
- Strategy configuration.
- Backtesting visualizations including warnings and statistics.
- End-to-end tests.

**Acceptance criteria:** Core workflows function using persisted backend data.

### Milestone 8: Scheduler, Alerts and Hardening

Implement:

- Scheduler with persisted job status and locking.
- Alerts and alert endpoints.
- Data quality checks.
- Integration tests of the complete workflow.
- Documentation.

**Acceptance criteria:** Scheduled jobs run idempotently across trading days and holidays, alerts are generated and acknowledgeable, and the documented local setup works from a clean checkout.

---

## 21. Engineering Rules for GitHub Copilot

Follow these rules throughout implementation:

1. Implement one milestone at a time.
2. Do not implement future milestones prematurely.
3. Write tests alongside production code.
4. Use Go for all backend and analytics code. Do not introduce Python or other analytics runtimes.
5. Prefer standard libraries and well-maintained dependencies.
6. Avoid unnecessary frameworks and architectural complexity.
7. Keep business logic separate from HTTP handlers and database access.
8. Use interfaces where external dependencies or meaningful alternative implementations exist.
9. Do not introduce microservices, Kubernetes, Kafka or message brokers for MVP.
10. Do not introduce an ORM unless there is a demonstrated benefit.
11. Use database migrations for every schema change.
12. Never hardcode API keys or credentials.
13. Never silently ignore invalid market data.
14. Never fabricate missing market prices or FX rates.
15. Never use future market information in historical decisions.
16. Make all financial calculations deterministic and independently testable.
17. Use exact decimal or fixed-point arithmetic for money; never `float64` for ledger balances.
18. Implement each indicator and scoring formula in exactly one place.
19. Never allow capital above `domain.MaxCapitalCHF` (5,000 CHF); never raise this constant without a specification change.
20. Document important financial assumptions.
21. Avoid unnecessary comments that merely repeat what the code does.
22. Use clear naming and idiomatic Go and TypeScript.
23. Do not edit generated code by hand.
24. Run the relevant tests after each implementation step.
25. Do not claim tests passed unless they were actually executed.
26. Do not claim a strategy is profitable based only on historical performance.
27. Always show survivorship, holdout and statistical-uncertainty warnings where results are displayed.
28. Do not add automatic order execution without an explicit future requirement.

## 22. Definition of Done

The MVP is complete when:

- Historical stock data and USD/CHF FX rates can be imported and updated.
- Market data is validated and persisted in PostgreSQL.
- Technical indicators can be calculated correctly in Go.
- Stocks can be ranked using a configurable strategy, including affordability for a 5,000 CHF portfolio.
- The frontend displays rankings and stock charts.
- Historical backtests can be executed in CHF with realistic small-account costs.
- Portfolio accounting and execution assumptions are verified (golden reference, ledger consistency).
- Backtests can be compared against a passive benchmark in CHF.
- Look-ahead bias and survivorship bias are addressed or explicitly flagged.
- Results are reproducible and carry confidence intervals and a trial-adjusted Sharpe ratio.
- Holdout usage is logged and enforced.
- Scheduler and alerts work across trading calendars.
- All required unit and integration tests pass.
- The application can be started using a documented local setup.
- No capital above 5,000 CHF can be configured.
- No real-money trading functionality is enabled.

**Primary success criterion:** TrendRadar must provide a reliable research environment for evaluating whether medium-term stock market signals offer measurable predictive value for a small private investor after realistic trading costs, currency effects, taxes (as approximated) and risks.

---

## 23. Open Decisions (to be resolved before or during Milestone 2)

| Decision | Why it matters |
|---|---|
| Market data provider with delisted securities and corporate actions | Determines whether the survivorship flag can be removed |
| USD/CHF FX data source | Required for CHF-based results |
| Actual broker fee schedule (commission, minimum, FX spread) | Cost defaults are placeholders; with 5,000 CHF the results are very sensitive to them |
| Whether the broker allows fractional shares | Changes affordability and universe size significantly |
| Stamp duty applicability for the chosen broker | A 0.30% per-side cost materially affects a monthly-rebalanced strategy |
| Universe definition (for example S&P 500 point-in-time) | Must be fixed before evaluating any result |
| CHF risk-free reference series (for example SARON) | Needed for Sharpe and Sortino |
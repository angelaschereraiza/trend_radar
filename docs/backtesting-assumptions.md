# Backtesting and validation assumptions

The in-memory engine accepts chronologically ordered daily market observations. Signals must have a decision date strictly before the execution date. Trades execute at the supplied next-session open and use that session's USD/CHF rate. Exact decimal arithmetic is used for cash, fills, and ledger entries; indicators and statistical estimates use floating point.

## Costs and corporate actions

- Commission minimum, FX spread, stamp duty, and slippage are charged separately on every fill. Slippage is represented as an explicit adverse cost rather than changing the quoted fill price.
- Dividends are supplied as a USD-per-share amount on the effective date. The engine credits gross proceeds, posts withholding tax as a separate negative cash-ledger entry, and records the configured withholding amount.
- A split multiplies held quantity by its positive ratio without changing cash or cost basis.
- A cash delisting with a positive amount uses that amount as the USD close-out price. Without an amount, the engine applies the configured recovery rate to the effective-date close: 0% for bankruptcy and 70% for unknown reasons by default. A zero recovery removes the position without inventing proceeds.
- Stock-for-stock mergers are rejected because the current market-day input does not identify the acquiring security. They must not be treated as cash proceeds.
- The default costs and tax rates remain research placeholders and must be verified against a chosen broker and the investor's tax treatment.

The engine's golden test is documented in [the golden reference README](../backend/testdata/golden/README.md).

## Baselines and statistical reporting

`RunMomentumBaseline` ranks eligible candidates by their supplied 12-1 momentum value, uses the same position rules and costs, and rebalances on the input schedule. `RunBuyAndHoldBaseline` makes the initial selection once and disables later rebalances. The engine also tracks the configured passive benchmark in CHF.

`BuildPerformanceReport` combines portfolio metrics with seeded stationary-bootstrap intervals and a Deflated Sharpe Ratio. The default bootstrap configuration is 2,000 replicates, block length 20, seed 42, and 95% confidence. Reports include trial count, dataset role, baseline metrics, survivorship and repeated-holdout warnings. Development results are exploratory and are never labelled promising; an out-of-sample result is labelled promising only if the bootstrap interval for excess return is strictly above zero. A development-period benchmark drawdown below 20% is flagged as a possible single-regime limitation.

`EvaluateWalkForward` passes each fit callback only the fold's training observations and each evaluation callback only the later test observations. It rejects overlapping test periods and links daily strategy and benchmark returns into one out-of-sample equity curve. A fit callback must derive parameters only from its supplied training slice; evaluation receives only the fitted value and test slice.

`RunResearch` requires point-in-time membership for every simulated date and filters candidates against each date's members before simulation. The report compares the strategy against the simulated SPY benchmark, buy-and-hold, momentum, and a CHF cash curve compounded at the configured annual risk-free rate.

Dataset split and walk-forward helpers prevent overlapping training and test windows. Holdout runs require explicit confirmation and a persistent research registry; a holdout access is serialized and logged before the simulation. With a registry, the run's existing `backtest_runs` row must match the supplied snapshot, universe, and dataset role. Membership is then loaded from that fixed universe for each simulated date, and the snapshot row provides the survivorship flag. The tested backtest, metric, bootstrap, and universe configuration is hashed and stored with the run; Sharpe is its primary trial metric. The report uses the persisted trial count after adding the current run. The caller must create the `backtest_runs` row first because trial and holdout records reference that run id. Historical universe membership can be loaded as symbols or stock ids as of the simulated date.

## Integration boundary

The research workflow is wired to the REST backtest endpoints. The current project has no selected production market-data/FX provider or point-in-time delisted-security dataset. Any run based on the existing fixture data must remain marked survivorship-biased; provider and broker assumptions are not inferred by this implementation.

## API, jobs, and frontend workflow

`POST /api/v1/backtests` selects the newest snapshot covering the requested interval and its compatible universe, persists a queued `backtest_runs` record, and executes in a bounded background slot. The API recovers queued/running records as interrupted on startup rather than presenting stale work as active. Completed runs persist equity, orders, fills, cash ledger, primary metrics, the statistical report, and warnings for the status/equity/trades/metrics endpoints. A run requires SPY, exact-date USD/CHF rates, USD security prices, a matching snapshot/universe, and at least 253 aligned observations before its first rebalance; missing inputs fail the run rather than being carried forward or fabricated. Signal ranks are calculated from observations through the prior market session and rebalance monthly on the first available session of each month.

The scheduler consumes persisted `job_runs` with `FOR UPDATE SKIP LOCKED` claims and expiring leases. It schedules after the NYSE close, avoids weekends and NYSE holidays, and stores UTC timestamps. Rankings, threshold-crossing alerts, and data-quality checks are executable jobs. The default alert score threshold is 80 and can be changed with `ALERT_SCORE_THRESHOLD` (0–100). Market-data and FX collection jobs remain failed with an explicit provider-not-configured error; the fixture importer remains a manual development-only workflow. Run the worker with `go run ./cmd/worker --schedule` to process jobs and accept manual `POST /api/v1/jobs/{jobName}/run` requests.

The frontend exposes the dashboard, stock explorer, stock details with price/volume and moving-average charts, strategy creation, backtest status/results, and alert acknowledgement. Stock-detail CHF conversion is intentionally omitted until a historical FX-rate display endpoint is available; the stored USD/CHF series is used by backtests.

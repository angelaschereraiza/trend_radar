# Backtesting and validation

Backtests are deterministic research simulations, not predictions or investment advice. The engine executes signals using the next session's supplied open and that session's USD/CHF rate. A signal date must precede its execution date. Missing required prices, FX rates, or benchmark observations cause a run to fail; the engine does not carry prices forward or invent data.

Cash, fills, and ledger amounts use decimal arithmetic. The portfolio is long-only, unleveraged, and subject to the CHF 5,000 capital ceiling. Orders account for estimated trading costs before spending available cash. The engine records orders, fills, cost components, cash-ledger entries, positions, and daily equity.

## Costs and corporate actions

Default cost parameters are research placeholders: 5 bps commission with CHF 1 minimum, 25 bps FX spread, 30 bps stamp duty, 10 bps slippage, and 15% dividend withholding. Each cost is charged per fill. The actual broker schedule, stamp-duty applicability, and personal tax treatment have not been verified.

Dividends are credited gross and withholding is recorded as a separate ledger deduction. Splits adjust held quantity without changing cash or cost basis. Cash delistings use a supplied positive amount when available; otherwise the configured recovery rate applies to the effective-date close (0% for bankruptcy and 70% for an unknown reason by default). Stock-for-stock mergers are rejected because the input does not identify the acquiring security.

See [detailed backtesting assumptions](./backtesting-assumptions.md) and [Swiss investor notes](./swiss-investor-notes.md) for additional integration limits and caveats.

## Benchmarks and metrics

Results include portfolio and benchmark equity in CHF and standard performance metrics. The research report may also compare buy-and-hold, momentum, and a CHF cash curve compounded at the configured risk-free rate. The CHF risk-free series itself has not been selected; a configured annual rate is an assumption, not sourced market data.

Statistical reporting uses a seeded stationary bootstrap (default 2,000 replicates, block length 20, seed 42, 95% confidence) and a Deflated Sharpe Ratio adjusted for the recorded trial count. The primary trial metric is Sharpe. A holdout run requires explicit confirmation and a persistent registry entry; each access is recorded before simulation. Repeated holdout access and survivorship-bias warnings must remain visible with results.

Walk-forward evaluation keeps each training fold earlier than its test fold, rejects overlapping test periods, and combines daily test returns into an out-of-sample equity curve. Fixture-backed performance remains exploratory and survivorship-biased even when its simulation is reproducible.

## Reproducibility and known gaps

Backtest runs retain the selected snapshot, universe, strategy/configuration, dataset role, code version, trial count, and warnings. Snapshot content hashes provide an identity for canonicalized imported data; they do not prove the source data is complete or unbiased.

The current manual Yahoo Finance import is an unofficial source and its small current-symbol universe is survivorship-biased; it is not a licensed production feed or point-in-time delisted-security dataset. The default cost and tax assumptions are unverified. These gaps prevent interpreting imported-data results as evidence of real-world profitability. CHF-denominated stocks such as Nestlé are deliberately excluded from the USD-only ranking and backtest calculations.

For execution details, test references, and the API workflow, see [backtesting assumptions](./backtesting-assumptions.md). The simulator is not connected to a brokerage and cannot place orders.

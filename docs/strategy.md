# Strategy and scoring

## Current ranking factors

The ranking pipeline calculates five factors from aligned stock and SPY observations:

| Factor | Definition | Default weight |
|---|---|---:|
| 12-1 momentum | Return from 252 sessions ago to 21 sessions ago | 30% |
| Relative strength | Stock return minus SPY return over 126 sessions | 20% |
| Trend quality | Annualized log-price regression slope multiplied by its R-squared over 126 sessions | 20% |
| Relative volume | Current volume divided by the mean of the preceding 50 sessions | 15% |
| Volatility-adjusted momentum | 12-1 momentum divided by 126-session annualized volatility | 15% |

Relative strength compares native-currency returns over exactly aligned sessions. Indicator lookbacks use trading sessions, not calendar days. Values without sufficient valid history are unavailable; a stock missing any required ranking factor is excluded rather than assigned a default value.

## Cross-sectional score

For each factor independently, the ranking code winsorizes valid values to the 1st and 99th percentiles, then converts them to percentile ranks on a 0–100 scale. Ties receive their average rank. The final score is the weighted sum of the five percentile ranks. Default weights sum to 1 and can be changed only while remaining finite, non-negative, and summing to 1.

Results are ordered by score descending, then momentum contribution descending, then symbol and identifier for deterministic tie-breaking. Factor correlation reports are calculated separately; correlated factors should be considered when interpreting a composite score.

Indicator conventions are specified in [the software specification](./specification.md#8-technical-indicators) and implemented in `backend/internal/indicators`. In particular, 12-1 momentum excludes the most recent 21 sessions; volatility uses sample standard deviation of daily log returns annualized by `sqrt(252)`; and relative volume excludes the current session from its average.

## Portfolio selection

The initial strategy is long-only, unleveraged and equal-weighted, with a default of 10 positions. At a monthly rebalance, it retains held securities ranked within a buffer of 15, exits ineligible or lower-ranked holdings, then fills free slots in rank order. A holding is forcibly sold after three calendar months. There are no stop-loss rules.

The target budget per position is initial portfolio capital divided by the maximum position count. With whole shares, a candidate is affordable only if one share converted to CHF at the supplied FX rate fits that budget. The configuration also sets a CHF 150 minimum trade value; fractional shares are disabled by default.

The full strategy selection order, eligibility criteria, and sizing assumptions are defined in [the strategy section of the specification](./specification.md#10-investment-strategy). The current implementation does not make broker orders.

## Interpretation

Scores are relative to the securities and observations available on the ranking date. They are not forecasts, recommendations, or measures of expected return. They depend on data quality, universe construction, factor definitions, and the selected weights. Fixture universes are survivorship-biased; do not treat their historical results as representative of an investable point-in-time universe.

package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/indicators"
	"github.com/angelaschereraiza/trend_radar/backend/internal/scoring"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

var ErrInsufficientBacktestHistory = errors.New("insufficient backtest history")

type historicalBar struct {
	stockID  string
	symbol   string
	sector   string
	date     time.Time
	open     decimal.Decimal
	close    decimal.Decimal
	adjusted decimal.Decimal
	volume   int64
	currency string
}

func (r *MarketData) LoadBacktestDays(ctx context.Context, start, end time.Time) ([]backtest.MarketDay, error) {
	lookbackStart := start.AddDate(-2, 0, 0)
	rows, err := r.db.QueryContext(ctx, `
		SELECT stock_id, symbol, sector, trading_date, open, close, adjusted_close, volume, currency
		FROM (
			SELECT DISTINCT ON (p.stock_id, p.trading_date)
			       p.stock_id, s.symbol, s.sector, p.trading_date, p.open, p.close,
			       p.adjusted_close, p.volume, p.currency, p.imported_at, p.data_version
			FROM stock_prices p
			JOIN stocks s ON s.id = p.stock_id
			WHERE p.trading_date >= $1 AND p.trading_date <= $2
			  AND s.currency = 'USD' AND p.currency = 'USD'
			ORDER BY p.stock_id, p.trading_date, p.imported_at DESC, p.data_version DESC
		) latest
		ORDER BY trading_date, symbol`, lookbackStart, end)
	if err != nil {
		return nil, fmt.Errorf("query backtest prices: %w", err)
	}
	defer rows.Close()

	series := make(map[string][]historicalBar)
	barsByDate := make(map[string]map[string]historicalBar)
	var benchmarkDates []time.Time
	for rows.Next() {
		var bar historicalBar
		if err := rows.Scan(&bar.stockID, &bar.symbol, &bar.sector, &bar.date,
			&bar.open, &bar.close, &bar.adjusted, &bar.volume, &bar.currency); err != nil {
			return nil, fmt.Errorf("scan backtest price: %w", err)
		}
		if bar.currency != "USD" {
			return nil, fmt.Errorf("backtest contains unsupported %s-denominated price for %s", bar.currency, bar.symbol)
		}
		dateKey := bar.date.Format("2006-01-02")
		series[bar.symbol] = append(series[bar.symbol], bar)
		if barsByDate[dateKey] == nil {
			barsByDate[dateKey] = make(map[string]historicalBar)
		}
		barsByDate[dateKey][bar.symbol] = bar
		if bar.symbol == "SPY" && !bar.date.Before(start) && !bar.date.After(end) {
			benchmarkDates = append(benchmarkDates, bar.date)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read backtest prices: %w", err)
	}
	if len(benchmarkDates) == 0 {
		return nil, fmt.Errorf("no SPY benchmark prices in requested date range")
	}
	sort.Slice(benchmarkDates, func(i, j int) bool { return benchmarkDates[i].Before(benchmarkDates[j]) })

	fxRates, err := r.loadDailyFX(ctx, start, end)
	if err != nil {
		return nil, err
	}
	actions, err := r.loadCorporateActions(ctx, start, end)
	if err != nil {
		return nil, err
	}
	benchmark := series["SPY"]
	sort.Slice(benchmark, func(i, j int) bool { return benchmark[i].date.Before(benchmark[j].date) })
	benchmarkByDate := make(map[string]historicalBar, len(benchmark))
	for _, bar := range benchmark {
		benchmarkByDate[bar.date.Format("2006-01-02")] = bar
	}

	days := make([]backtest.MarketDay, 0, len(benchmarkDates))
	lastRebalanceMonth := time.Month(0)
	for index, date := range benchmarkDates {
		dateKey := date.Format("2006-01-02")
		rate, ok := fxRates[dateKey]
		if !ok || !rate.IsPositive() {
			return nil, fmt.Errorf("USD/CHF rate missing for backtest date %s", dateKey)
		}
		benchmarkBar, ok := benchmarkByDate[dateKey]
		if !ok || !benchmarkBar.open.IsPositive() || !benchmarkBar.close.IsPositive() {
			return nil, fmt.Errorf("SPY open/close price missing for backtest date %s", dateKey)
		}
		day := backtest.MarketDay{
			Date: date, USDCHF: rate, OpenPricesUSD: make(map[string]decimal.Decimal),
			ClosePricesUSD:   make(map[string]decimal.Decimal),
			BenchmarkOpenUSD: benchmarkBar.open, BenchmarkCloseUSD: benchmarkBar.close,
		}
		for symbol, bar := range barsByDate[dateKey] {
			day.OpenPricesUSD[symbol] = bar.open
			day.ClosePricesUSD[symbol] = bar.close
		}
		day.CorporateActions = actions[dateKey]
		if index == 0 || date.Month() != lastRebalanceMonth {
			decisionDate, err := previousHistoryDate(benchmarkDates, benchmark, date, index)
			if err != nil {
				return nil, err
			}
			rankings, err := historicalRankings(series, benchmarkByDate, decisionDate)
			if err != nil {
				return nil, fmt.Errorf("calculate historical rankings as of %s: %w", decisionDate.Format("2006-01-02"), err)
			}
			day.Rebalance = true
			day.DecisionDate = decisionDate
			day.RankingsAsOfDate = decisionDate
			day.Rankings = rankings
			lastRebalanceMonth = date.Month()
		}
		days = append(days, day)
	}
	return days, nil
}

func previousHistoryDate(dates []time.Time, benchmark []historicalBar, date time.Time, index int) (time.Time, error) {
	if index > 0 {
		return dates[index-1], nil
	}
	for i := len(benchmark) - 1; i >= 0; i-- {
		if benchmark[i].date.Before(date) {
			return benchmark[i].date, nil
		}
	}
	return time.Time{}, fmt.Errorf("no prior benchmark session for initial rebalance on %s", date.Format("2006-01-02"))
}

func historicalRankings(series map[string][]historicalBar, benchmarkByDate map[string]historicalBar, asOf time.Time) ([]strategy.Candidate, error) {
	benchmarkObservations := make([]indicators.Observation, 0, len(benchmarkByDate))
	for _, bar := range benchmarkByDate {
		if !bar.date.After(asOf) {
			benchmarkObservations = append(benchmarkObservations, indicators.Observation{
				Date: bar.date, Close: bar.adjusted.InexactFloat64(), Volume: float64(bar.volume),
			})
		}
	}
	sort.Slice(benchmarkObservations, func(i, j int) bool {
		return benchmarkObservations[i].Date.Before(benchmarkObservations[j].Date)
	})
	if len(benchmarkObservations) < 253 {
		return nil, fmt.Errorf("%w: benchmark has fewer than 253 aligned observations by %s",
			ErrInsufficientBacktestHistory, asOf.Format("2006-01-02"))
	}
	inputs := make([]scoring.StockInput, 0, len(series)-1)
	factorsBySymbol := make(map[string]scoring.Factors)
	priceBySymbol := make(map[string]historicalBar)
	for symbol, bars := range series {
		if symbol == "SPY" {
			continue
		}
		observations := make([]indicators.Observation, 0, len(bars))
		alignedBenchmark := make([]indicators.Observation, 0, len(bars))
		var latest historicalBar
		var averageDollarVolume float64
		for _, bar := range bars {
			if bar.date.After(asOf) {
				break
			}
			benchmarkBar, aligned := benchmarkByDate[bar.date.Format("2006-01-02")]
			if !aligned {
				continue
			}
			observations = append(observations, indicators.Observation{
				Date: bar.date, Close: bar.adjusted.InexactFloat64(), Volume: float64(bar.volume),
			})
			alignedBenchmark = append(alignedBenchmark, indicators.Observation{
				Date: benchmarkBar.date, Close: benchmarkBar.adjusted.InexactFloat64(), Volume: float64(benchmarkBar.volume),
			})
			latest = bar
		}
		if len(observations) < 253 || !latest.date.Equal(asOf) || latest.close.LessThan(decimal.NewFromInt(5)) {
			continue
		}
		for i := len(observations) - 50; i < len(observations); i++ {
			averageDollarVolume += observations[i].Close * observations[i].Volume
		}
		if averageDollarVolume/50 < 5_000_000 {
			continue
		}
		factors, err := scoring.CalculateFactors(observations, alignedBenchmark)
		if err != nil {
			return nil, fmt.Errorf("calculate factors for %s: %w", symbol, err)
		}
		inputs = append(inputs, scoring.StockInput{ID: latest.stockID, Symbol: symbol, Factors: factors})
		factorsBySymbol[symbol] = factors
		priceBySymbol[symbol] = latest
	}
	ranked, err := scoring.Rank(inputs, scoring.DefaultWeights())
	if err != nil {
		return nil, err
	}
	candidates := make([]strategy.Candidate, 0, len(ranked))
	for _, item := range ranked {
		if item.Excluded != "" {
			continue
		}
		bar := priceBySymbol[item.Symbol]
		candidates = append(candidates, strategy.Candidate{
			Symbol: item.Symbol, Rank: item.Rank, Score: item.Score,
			Momentum: factorsBySymbol[item.Symbol].Momentum, Eligible: true, PriceUSD: bar.close,
		})
	}
	return candidates, nil
}

func (r *MarketData) loadDailyFX(ctx context.Context, start, end time.Time) (map[string]decimal.Decimal, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT rate_date, rate FROM (
			SELECT DISTINCT ON (rate_date) rate_date, rate, imported_at, data_version
			FROM fx_rates
			WHERE base_currency = 'USD' AND quote_currency = 'CHF'
			  AND rate_date >= $1 AND rate_date <= $2
			ORDER BY rate_date, imported_at DESC, data_version DESC
		) latest ORDER BY rate_date`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query backtest FX rates: %w", err)
	}
	defer rows.Close()
	rates := make(map[string]decimal.Decimal)
	for rows.Next() {
		var date time.Time
		var rate decimal.Decimal
		if err := rows.Scan(&date, &rate); err != nil {
			return nil, fmt.Errorf("scan backtest FX rate: %w", err)
		}
		rates[date.Format("2006-01-02")] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read backtest FX rates: %w", err)
	}
	return rates, nil
}

func (r *MarketData) loadCorporateActions(ctx context.Context, start, end time.Time) (map[string][]backtest.CorporateAction, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT symbol, action_type, delisting_reason, amount, ratio, effective_date
		FROM (
			SELECT DISTINCT ON (ca.stock_id, ca.action_type, ca.effective_date)
			       s.symbol, ca.action_type, ca.delisting_reason, ca.amount, ca.ratio,
			       ca.effective_date, ca.imported_at, ca.id
			FROM corporate_actions ca
			JOIN stocks s ON s.id = ca.stock_id
			WHERE ca.effective_date >= $1 AND ca.effective_date <= $2
			ORDER BY ca.stock_id, ca.action_type, ca.effective_date, ca.imported_at DESC, ca.id DESC
		) latest
		ORDER BY effective_date, id`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query backtest corporate actions: %w", err)
	}
	defer rows.Close()
	actions := make(map[string][]backtest.CorporateAction)
	for rows.Next() {
		var action backtest.CorporateAction
		var date time.Time
		if err := rows.Scan(&action.Symbol, &action.Type, &action.Reason, &action.Amount, &action.Ratio, &date); err != nil {
			return nil, fmt.Errorf("scan backtest corporate action: %w", err)
		}
		key := date.Format("2006-01-02")
		actions[key] = append(actions[key], action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read backtest corporate actions: %w", err)
	}
	return actions, nil
}

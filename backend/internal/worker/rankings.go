package worker

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/indicators"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
	"github.com/angelaschereraiza/trend_radar/backend/internal/scoring"
)

type RankingRun struct {
	Date         time.Time
	Entries      []repository.RankingEntry
	Correlations []scoring.Correlation
	Excluded     map[string]string
}

func CalculateLatestRanking(history []repository.PriceHistory, fxRates map[string]string, capitalCHF int) (RankingRun, error) {
	if len(history) == 0 {
		return RankingRun{}, fmt.Errorf("no price history is available")
	}
	series := make(map[string][]repository.PriceHistory)
	metadata := make(map[string]repository.PriceHistory)
	for _, item := range history {
		series[item.Symbol] = append(series[item.Symbol], item)
		metadata[item.Symbol] = item
	}
	benchmark, ok := series["SPY"]
	if !ok || len(benchmark) == 0 {
		return RankingRun{}, fmt.Errorf("benchmark SPY is missing from imported history")
	}
	sort.Slice(benchmark, func(i, j int) bool { return benchmark[i].Date.Before(benchmark[j].Date) })
	asOf := benchmark[len(benchmark)-1].Date
	rateText, ok := fxRates[asOf.Format("2006-01-02")]
	if !ok {
		return RankingRun{}, fmt.Errorf("USD/CHF rate missing for latest date %s", asOf.Format("2006-01-02"))
	}
	fxRate, err := decimal.NewFromString(rateText)
	if err != nil || !fxRate.IsPositive() {
		return RankingRun{}, fmt.Errorf("invalid USD/CHF rate for %s", asOf.Format("2006-01-02"))
	}
	benchmarkByDate := make(map[string]repository.PriceHistory, len(benchmark))
	for _, item := range benchmark {
		benchmarkByDate[item.Date.Format("2006-01-02")] = item
	}

	inputs := make([]scoring.StockInput, 0, len(series))
	factorsBySymbol := make(map[string]scoring.Factors)
	metadataBySymbol := make(map[string]repository.PriceHistory)
	excluded := make(map[string]string)
	for symbol, prices := range series {
		if symbol == "SPY" {
			continue
		}
		sort.Slice(prices, func(i, j int) bool { return prices[i].Date.Before(prices[j].Date) })
		if len(prices) < 253 {
			excluded[symbol] = "insufficient history: 12-1 momentum requires 253 aligned observations"
			continue
		}
		stockObservations := make([]indicators.Observation, 0, len(prices))
		benchmarkObservations := make([]indicators.Observation, 0, len(prices))
		var latest repository.PriceHistory
		for _, price := range prices {
			benchmarkPrice, aligned := benchmarkByDate[price.Date.Format("2006-01-02")]
			if !aligned {
				continue
			}
			stockObservations = append(stockObservations, indicators.Observation{
				Date: price.Date, Close: price.AdjustedClose.InexactFloat64(), Volume: float64(price.Volume),
			})
			benchmarkObservations = append(benchmarkObservations, indicators.Observation{
				Date: benchmarkPrice.Date, Close: benchmarkPrice.AdjustedClose.InexactFloat64(), Volume: float64(benchmarkPrice.Volume),
			})
			latest = price
		}
		if len(stockObservations) < 253 || !latest.Date.Equal(asOf) {
			excluded[symbol] = "insufficient aligned history through scoring date"
			continue
		}
		factors, err := scoring.CalculateFactors(stockObservations, benchmarkObservations)
		if err != nil {
			excluded[symbol] = err.Error()
			continue
		}
		if latest.Close.LessThan(decimal.NewFromInt(5)) {
			excluded[symbol] = "price below USD 5 eligibility threshold"
			continue
		}
		var dollarVolume float64
		for i := len(stockObservations) - 50; i < len(stockObservations); i++ {
			dollarVolume += stockObservations[i].Close * stockObservations[i].Volume
		}
		if dollarVolume/50 < 5_000_000 {
			excluded[symbol] = "average dollar volume below USD 5 million"
			continue
		}
		inputs = append(inputs, scoring.StockInput{
			ID: latest.StockID.String(), Symbol: symbol, Factors: factors,
		})
		factorsBySymbol[symbol] = factors
		metadataBySymbol[symbol] = metadata[symbol]
	}
	ranked, err := scoring.Rank(inputs, scoring.DefaultWeights())
	if err != nil {
		return RankingRun{}, err
	}
	positionBudget := decimal.NewFromInt(int64(capitalCHF)).Div(decimal.NewFromInt(10))
	entries := make([]repository.RankingEntry, 0, len(ranked))
	correlationInput := make([]scoring.DatedFactors, 0, len(ranked))
	for _, result := range ranked {
		if result.Excluded != "" {
			excluded[result.Symbol] = result.Excluded
			continue
		}
		stock := metadataBySymbol[result.Symbol]
		affordable := stock.Close.Mul(fxRate).LessThanOrEqual(positionBudget)
		entries = append(entries, repository.RankingEntry{
			StockID: stock.StockID, Symbol: result.Symbol, Sector: stock.Sector,
			Score: result.Score, Rank: result.Rank, Components: result.Components,
			Affordable: affordable,
		})
		correlationInput = append(correlationInput, scoring.DatedFactors{
			Date: asOf, Symbol: result.Symbol, Values: factorsBySymbol[result.Symbol],
		})
	}
	return RankingRun{
		Date: asOf, Entries: entries,
		Correlations: scoring.FactorCorrelations(correlationInput),
		Excluded:     excluded,
	}, nil
}

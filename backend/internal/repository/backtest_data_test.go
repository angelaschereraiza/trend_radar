package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/marketdata"
)

func TestFixtureSupportsDefaultBacktestFirstSignal(t *testing.T) {
	batch := marketdata.NewFixtureProvider().Batch()
	stockIDs := make(map[string]string, len(batch.Symbols))
	for _, symbol := range batch.Symbols {
		stockIDs[symbol.Symbol] = symbol.ID
	}
	asOf := time.Date(2023, time.November, 30, 0, 0, 0, 0, time.UTC)
	series := make(map[string][]historicalBar)
	benchmarkByDate := make(map[string]historicalBar)
	for _, price := range batch.Prices {
		if price.TradingDate.After(asOf) {
			continue
		}
		bar := historicalBar{
			stockID: stockIDs[price.Symbol], symbol: price.Symbol,
			date: price.TradingDate, close: price.Close, adjusted: price.AdjustedClose,
			volume: price.Volume, currency: price.Currency,
		}
		series[price.Symbol] = append(series[price.Symbol], bar)
		if price.Symbol == "SPY" {
			benchmarkByDate[price.TradingDate.Format("2006-01-02")] = bar
		}
	}

	candidates, err := historicalRankings(series, benchmarkByDate, asOf)
	if err != nil {
		t.Fatalf("historicalRankings() for default first signal: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("eligible fixture candidates = %d, want 3", len(candidates))
	}
}

func TestHistoricalRankingsIdentifiesInsufficientBenchmarkHistory(t *testing.T) {
	start := time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC)
	benchmarkByDate := make(map[string]historicalBar, 252)
	for i := 0; i < 252; i++ {
		date := start.AddDate(0, 0, i)
		bar := historicalBar{
			symbol: "SPY", date: date,
			adjusted: decimal.NewFromInt(400), volume: 1_000_000,
		}
		benchmarkByDate[date.Format("2006-01-02")] = bar
	}

	_, err := historicalRankings(nil, benchmarkByDate, start.AddDate(0, 0, 251))
	if !errors.Is(err, ErrInsufficientBacktestHistory) {
		t.Fatalf("historicalRankings() error = %v, want insufficient-history error", err)
	}
}

func TestHistoricalRankingsUseOnlyObservationsAsOfDecisionDate(t *testing.T) {
	start := time.Date(2022, time.January, 3, 0, 0, 0, 0, time.UTC)
	series := map[string][]historicalBar{}
	benchmarkByDate := map[string]historicalBar{}
	for i := 0; i < 270; i++ {
		date := start.AddDate(0, 0, i)
		dateKey := date.Format("2006-01-02")
		benchmark := historicalBar{
			stockID: "benchmark", symbol: "SPY", date: date,
			close:    decimal.NewFromInt(int64(100 + i)),
			adjusted: decimal.NewFromInt(int64(100 + i)),
			volume:   1_000_000, currency: "USD",
		}
		benchmarkByDate[dateKey] = benchmark
		series["SPY"] = append(series["SPY"], benchmark)
		for symbolIndex, symbol := range []string{"AAA", "BBB"} {
			value := int64(50 + symbolIndex*30 + i*(symbolIndex+1))
			bar := historicalBar{
				stockID: fmt.Sprintf("stock-%d", symbolIndex), symbol: symbol, date: date,
				close: decimal.NewFromInt(value), adjusted: decimal.NewFromInt(value),
				volume: 1_000_000, currency: "USD",
			}
			series[symbol] = append(series[symbol], bar)
		}
	}
	asOf := series["SPY"][258].date
	first, err := historicalRankings(series, benchmarkByDate, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("ranked candidates = %d, want 2", len(first))
	}

	series["AAA"][269].close = decimal.NewFromInt(1_000_000)
	series["AAA"][269].adjusted = decimal.NewFromInt(1_000_000)
	benchmarkByDate[series["SPY"][269].date.Format("2006-01-02")] = historicalBar{
		symbol: "SPY", date: series["SPY"][269].date,
		adjusted: decimal.NewFromInt(1_000_000), volume: 1_000_000,
	}
	second, err := historicalRankings(series, benchmarkByDate, asOf)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("future observation changed ranking at index %d: before=%+v after=%+v", i, first[i], second[i])
		}
	}
}

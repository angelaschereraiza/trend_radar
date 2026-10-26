package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

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

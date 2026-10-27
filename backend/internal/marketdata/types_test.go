package marketdata

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestFixtureProviderDeterministicAndValid(t *testing.T) {
	first := NewFixtureProvider().Batch()
	second := NewFixtureProvider().Batch()
	if err := ValidateBatch(first); err != nil {
		t.Fatalf("fixture batch invalid: %v", err)
	}
	a, err := BuildSnapshot(first, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildSnapshot(second, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentHash != b.ContentHash || !a.SurvivorshipBiased {
		t.Fatalf("fixture snapshots differ or missing bias flag: %+v %+v", a, b)
	}
}

func TestFixtureProvidesBacktestWarmupWithoutChangingExistingPriceSeries(t *testing.T) {
	batch := NewFixtureProvider().Batch()
	dataStart := time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC)
	firstSignalDate := time.Date(2023, time.December, 1, 0, 0, 0, 0, time.UTC)
	benchmarkHistory := 0
	var firstFixturePrice *PriceBar
	for i := range batch.Prices {
		bar := &batch.Prices[i]
		if bar.Symbol != "SPY" {
			continue
		}
		if bar.TradingDate.Before(firstSignalDate) {
			benchmarkHistory++
		}
		if bar.TradingDate.Equal(dataStart) {
			firstFixturePrice = bar
		}
	}
	if benchmarkHistory < 253 {
		t.Fatalf("benchmark warmup has %d sessions before the first signal, want at least 253", benchmarkHistory)
	}
	if firstFixturePrice == nil || !firstFixturePrice.Close.Equal(decimal.NewFromInt(400)) {
		t.Fatalf("first original SPY close = %v, want 400", firstFixturePrice)
	}
}

func TestSnapshotOrderingDoesNotMutateInput(t *testing.T) {
	batch := NewFixtureProvider().Batch()
	originalFirst := batch.Prices[0]
	batch.Prices[0], batch.Prices[1] = batch.Prices[1], batch.Prices[0]
	before := batch.Prices[0]
	if _, err := BuildSnapshot(batch, false); err != nil {
		t.Fatal(err)
	}
	if batch.Prices[0] != before {
		t.Fatal("snapshot canonicalization mutated input")
	}
	if originalFirst == batch.Prices[0] {
		t.Fatal("test did not reorder fixture rows")
	}
}

func TestInvalidOHLCRejected(t *testing.T) {
	batch := NewFixtureProvider().Batch()
	batch.Prices[0].High = decimal.NewFromInt(1)
	if err := ValidateBatch(batch); err == nil {
		t.Fatal("invalid high/low values accepted")
	}
}

func TestDuplicatePriceBarRejected(t *testing.T) {
	batch := NewFixtureProvider().Batch()
	batch.Prices = append(batch.Prices, batch.Prices[0])
	if err := ValidateBatch(batch); err == nil {
		t.Fatal("duplicate price bar accepted")
	}
}

func TestFixtureProviderHonorsContextAndRange(t *testing.T) {
	provider := NewFixtureProvider()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.GetSymbols(ctx); err == nil {
		t.Fatal("provider ignored canceled context")
	}
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := from
	bars, err := provider.GetHistoricalPrices(context.Background(), "FIXA", from, to)
	if err != nil || len(bars) != 1 {
		t.Fatalf("GetHistoricalPrices = %d bars, %v; want 1", len(bars), err)
	}
}

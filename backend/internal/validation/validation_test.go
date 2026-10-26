package validation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestWalkForwardWindowsAreChronologicalAndDisjoint(t *testing.T) {
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2011, 12, 31, 0, 0, 0, 0, time.UTC)
	windows, err := WalkForwardWindows(start, end, 5, 1, 1)
	if err != nil || len(windows) != 7 {
		t.Fatalf("windows = %d, %v; want 7", len(windows), err)
	}
	for _, window := range windows {
		if !window.TestStart.After(window.TrainEnd) {
			t.Fatalf("overlapping window: %+v", window)
		}
	}
}

func TestMarketViewRejectsFuturePrices(t *testing.T) {
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := NewMarketView(asOf, map[string][]Price{
		"AAA": {{Date: asOf.AddDate(0, 0, 1), Close: decimal.NewFromInt(10)}},
	})
	if err == nil {
		t.Fatal("MarketView accepted future data")
	}
}

func TestMarketViewOnlyReturnsPricesAsOfDecision(t *testing.T) {
	asOf := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	prices := map[string][]Price{
		"AAA": {
			{Date: asOf.AddDate(0, 0, -1), Close: decimal.NewFromInt(10)},
			{Date: asOf, Close: decimal.NewFromInt(12)},
		},
	}
	view, err := NewMarketView(asOf, prices)
	if err != nil {
		t.Fatal(err)
	}
	price, ok := view.PriceOnOrBefore("AAA", asOf)
	if !ok || !price.Close.Equal(decimal.NewFromInt(12)) {
		t.Fatalf("price = %+v, %v", price, ok)
	}
	if _, ok := view.PriceOnOrBefore("AAA", asOf.AddDate(0, 0, 1)); ok {
		t.Fatal("MarketView returned a price for a future query")
	}
}

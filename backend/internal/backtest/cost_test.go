package backtest

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateCostsIncludesMinimumCommission(t *testing.T) {
	cost, err := CalculateCosts(decimal.NewFromInt(100), DefaultCosts())
	if err != nil {
		t.Fatal(err)
	}
	if !cost.Commission.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("commission = %s, want CHF 1", cost.Commission)
	}
	if !cost.StampDuty.Equal(decimal.RequireFromString("0.30")) ||
		!cost.FXCost.Equal(decimal.RequireFromString("0.25")) ||
		!cost.Slippage.Equal(decimal.RequireFromString("0.10")) {
		t.Fatalf("unexpected cost breakdown: %+v", cost)
	}
}

func TestBreakEvenIsRoundTripCostRatio(t *testing.T) {
	costs := DefaultCosts()
	result, err := costs.BreakEvenGrossReturn(decimal.NewFromInt(1000))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(decimal.RequireFromString("0.015")) {
		t.Fatalf("break-even = %s, want 0.015", result)
	}
}

package backtest

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func TestMomentumAndBuyAndHoldBaselines(t *testing.T) {
	days := testDays()
	days[0].Rankings[0].Momentum = 0.1
	days[1].Rebalance = true
	days[1].DecisionDate = days[0].Date
	days[1].RankingsAsOfDate = days[0].Date
	days[1].Rankings = []strategy.Candidate{
		{Symbol: "AAA", Rank: 1, Momentum: 0.05, Eligible: true},
		{Symbol: "BBB", Rank: 2, Momentum: 0.20, Eligible: true},
	}
	days[1].OpenPricesUSD["BBB"] = decimal.NewFromInt(10)
	days[1].ClosePricesUSD["BBB"] = decimal.NewFromInt(10)
	config := testConfig()

	momentum, err := RunMomentumBaseline(days, config)
	if err != nil {
		t.Fatal(err)
	}
	buyAndHold, err := RunBuyAndHoldBaseline(days, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(momentum.Fills) != 3 || momentum.Positions["BBB"].Quantity.IsZero() {
		t.Fatalf("momentum baseline did not rotate into the strongest candidate: fills=%+v positions=%+v",
			momentum.Fills, momentum.Positions)
	}
	if len(buyAndHold.Fills) != 1 || buyAndHold.Positions["AAA"].Quantity.IsZero() {
		t.Fatalf("buy-and-hold baseline was rebalanced: fills=%+v positions=%+v",
			buyAndHold.Fills, buyAndHold.Positions)
	}
}

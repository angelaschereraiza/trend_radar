package api

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestStrategyConfigPreservesDecimalCapital(t *testing.T) {
	config, err := strategyConfig(StrategyConfig{
		InitialCapitalChf: "1234.5678", MaxPositions: 10, RankBuffer: 15,
		MaxHoldingMonths: 3, MinTradeValueChf: "150.25",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !config.InitialCapitalCHF.Equal(decimal.RequireFromString("1234.5678")) {
		t.Fatalf("capital = %s, want 1234.5678", config.InitialCapitalCHF)
	}
	if !config.MinTradeValueCHF.Equal(decimal.RequireFromString("150.25")) {
		t.Fatalf("minimum trade = %s, want 150.25", config.MinTradeValueCHF)
	}
}

func TestDecodeStoredStrategyConfigSupportsLegacyRankingWeights(t *testing.T) {
	config, err := decodeStoredStrategyConfig([]byte(`{"Momentum":0.3,"RelativeStrength":0.2}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.InitialCapitalChf != "5000" || config.MaxPositions != 10 || config.RankBuffer != 15 {
		t.Fatalf("legacy defaults = %+v", config)
	}
}

func TestDecodeStoredStrategyConfigRejectsUnknownShape(t *testing.T) {
	_, err := decodeStoredStrategyConfig([]byte(`{"unexpected":true}`))
	if err == nil {
		t.Fatal("invalid persisted strategy configuration was accepted")
	}
}

func TestBacktestCostsValidateNonNegativeRates(t *testing.T) {
	parameters := StrategyConfig{Costs: &BacktestCostConfig{
		CommissionBps: "5", MinCommissionChf: "1", FxSpreadBps: "25",
		StampDutyBps: "30", SlippageBps: "10", WithholdingRate: "0.15",
	}}
	if _, err := backtestCosts(parameters); err != nil {
		t.Fatalf("backtestCosts() error = %v", err)
	}
	parameters.Costs.WithholdingRate = "1.1"
	if _, err := backtestCosts(parameters); err == nil {
		t.Fatal("backtestCosts() accepted withholding above 100%")
	}
}

package strategy

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestSelectUsesRankBufferAndAffordability(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialCapitalCHF = decimal.NewFromInt(1000)
	cfg.MaxPositions = 2
	cfg.RankBuffer = 3
	candidates := []Candidate{
		{Symbol: "A", Rank: 1, Eligible: true, PriceUSD: decimal.NewFromInt(10), FXRate: decimal.NewFromInt(1)},
		{Symbol: "B", Rank: 2, Eligible: true, PriceUSD: decimal.NewFromInt(600), FXRate: decimal.NewFromInt(1)},
		{Symbol: "C", Rank: 3, Eligible: true, PriceUSD: decimal.NewFromInt(10), FXRate: decimal.NewFromInt(1)},
		{Symbol: "D", Rank: 4, Eligible: true, PriceUSD: decimal.NewFromInt(10), FXRate: decimal.NewFromInt(1)},
	}
	held := map[string]time.Time{"D": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	targets, exits, err := Select(candidates, held, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 1 || exits[0].Symbol != "D" {
		t.Fatalf("exits = %+v, want D", exits)
	}
	if len(targets) != 2 || targets[0].Symbol != "A" || targets[1].Symbol != "C" {
		t.Fatalf("targets = %+v, want affordable A and C", targets)
	}
}

func TestConfigRejectsCapitalOverCeiling(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialCapitalCHF = decimal.NewFromInt(5001)
	if err := cfg.Validate(); err == nil {
		t.Fatal("configuration above capital ceiling accepted")
	}
}

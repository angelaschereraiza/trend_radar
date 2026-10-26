package validation

import (
	"testing"
	"time"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func TestApplyPointInTimeUniverseFiltersEachDateIndependently(t *testing.T) {
	firstDate := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	secondDate := firstDate.AddDate(0, 0, 1)
	days := []backtest.MarketDay{
		{
			Date: firstDate,
			Rankings: []strategy.Candidate{
				{Symbol: "AAA", Rank: 1, Eligible: true},
				{Symbol: "BBB", Rank: 2, Eligible: true},
			},
		},
		{
			Date: secondDate,
			Rankings: []strategy.Candidate{
				{Symbol: "AAA", Rank: 1, Eligible: true},
				{Symbol: "BBB", Rank: 2, Eligible: true},
			},
		},
	}
	filtered, excluded, err := ApplyPointInTimeUniverse(days, map[string][]string{
		MembershipDateKey(firstDate):  {"AAA"},
		MembershipDateKey(secondDate): {"BBB"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered[0].Rankings) != 1 || filtered[0].Rankings[0].Symbol != "AAA" ||
		len(filtered[1].Rankings) != 1 || filtered[1].Rankings[0].Symbol != "BBB" {
		t.Fatalf("membership was not applied independently by date: %+v", filtered)
	}
	if len(excluded[MembershipDateKey(firstDate)]) != 1 || excluded[MembershipDateKey(firstDate)][0] != "BBB" {
		t.Fatalf("excluded universe symbols were not recorded: %+v", excluded)
	}
	if len(days[0].Rankings) != 2 {
		t.Fatal("filter mutated its input market days")
	}
}

func TestApplyPointInTimeUniverseRejectsMissingMembership(t *testing.T) {
	day := backtest.MarketDay{Date: time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)}
	if _, _, err := ApplyPointInTimeUniverse([]backtest.MarketDay{day}, map[string][]string{}); err == nil {
		t.Fatal("missing point-in-time membership accepted")
	}
}

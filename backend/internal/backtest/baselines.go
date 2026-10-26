package backtest

import (
	"errors"
	"math"
	"sort"

	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func RunMomentumBaseline(days []MarketDay, config Config) (Result, error) {
	baselineDays := cloneMarketDays(days)
	for dayIndex := range baselineDays {
		if !baselineDays[dayIndex].Rebalance {
			continue
		}
		candidates := baselineDays[dayIndex].Rankings
		for _, candidate := range candidates {
			if candidate.Eligible && (math.IsNaN(candidate.Momentum) || math.IsInf(candidate.Momentum, 0)) {
				return Result{}, errors.New("eligible momentum baseline candidates require finite momentum")
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Eligible != candidates[j].Eligible {
				return candidates[i].Eligible
			}
			if candidates[i].Momentum != candidates[j].Momentum {
				return candidates[i].Momentum > candidates[j].Momentum
			}
			return candidates[i].Symbol < candidates[j].Symbol
		})
		rank := 0
		for i := range candidates {
			if candidates[i].Eligible {
				rank++
				candidates[i].Rank = rank
			} else {
				candidates[i].Rank = len(candidates) + 1
			}
		}
		baselineDays[dayIndex].Rankings = candidates
	}
	return Run(baselineDays, config)
}

func RunBuyAndHoldBaseline(days []MarketDay, config Config) (Result, error) {
	baselineDays := cloneMarketDays(days)
	firstRebalance := -1
	for i := range baselineDays {
		if baselineDays[i].Rebalance {
			firstRebalance = i
			break
		}
	}
	if firstRebalance < 0 {
		return Result{}, errors.New("buy-and-hold baseline requires an initial rebalance signal")
	}
	for i := range baselineDays {
		if i == firstRebalance {
			continue
		}
		baselineDays[i].Rebalance = false
		baselineDays[i].Rankings = nil
	}
	return Run(baselineDays, config)
}

func cloneMarketDays(days []MarketDay) []MarketDay {
	cloned := append([]MarketDay(nil), days...)
	for i := range cloned {
		cloned[i].Rankings = append([]strategy.Candidate(nil), days[i].Rankings...)
		cloned[i].CorporateActions = append([]CorporateAction(nil), days[i].CorporateActions...)
	}
	return cloned
}

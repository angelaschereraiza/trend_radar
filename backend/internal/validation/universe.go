package validation

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
)

type UniverseExclusions map[string][]string

func ApplyPointInTimeUniverse(
	days []backtest.MarketDay,
	membership map[string][]string,
) ([]backtest.MarketDay, UniverseExclusions, error) {
	if len(days) == 0 || len(membership) == 0 {
		return nil, nil, errors.New("market days and point-in-time universe membership are required")
	}
	filtered := make([]backtest.MarketDay, len(days))
	excluded := make(UniverseExclusions)
	for dayIndex, day := range days {
		if day.Date.IsZero() || (dayIndex > 0 && !day.Date.After(days[dayIndex-1].Date)) {
			return nil, nil, fmt.Errorf("market days must be strictly ascending at index %d", dayIndex)
		}
		dateKey := day.Date.Format("2006-01-02")
		members, exists := membership[dateKey]
		if !exists || len(members) == 0 {
			return nil, nil, fmt.Errorf("point-in-time universe membership is missing for %s", dateKey)
		}
		memberSet := make(map[string]struct{}, len(members))
		for _, symbol := range members {
			if symbol == "" {
				return nil, nil, fmt.Errorf("point-in-time universe has an empty symbol on %s", dateKey)
			}
			if _, duplicate := memberSet[symbol]; duplicate {
				return nil, nil, fmt.Errorf("duplicate point-in-time universe symbol %q on %s", symbol, dateKey)
			}
			memberSet[symbol] = struct{}{}
		}
		filtered[dayIndex] = cloneMarketDay(day)
		rankings := filtered[dayIndex].Rankings[:0]
		for _, candidate := range filtered[dayIndex].Rankings {
			if _, member := memberSet[candidate.Symbol]; member {
				rankings = append(rankings, candidate)
				continue
			}
			excluded[dateKey] = append(excluded[dateKey], candidate.Symbol)
		}
		filtered[dayIndex].Rankings = rankings
		sort.Strings(excluded[dateKey])
	}
	return filtered, excluded, nil
}

func MembershipDateKey(date time.Time) string {
	return date.Format("2006-01-02")
}

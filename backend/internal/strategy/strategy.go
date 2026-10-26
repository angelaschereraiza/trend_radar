package strategy

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/domain"
)

type Config struct {
	InitialCapitalCHF decimal.Decimal
	MaxPositions      int
	RankBuffer        int
	MaxHoldingMonths  int
	MinTradeValueCHF  decimal.Decimal
	FractionalShares  bool
}

type Candidate struct {
	Symbol   string
	Rank     int
	Score    float64
	Momentum float64
	Eligible bool
	PriceUSD decimal.Decimal
	FXRate   decimal.Decimal
}

func DefaultConfig() Config {
	return Config{
		InitialCapitalCHF: decimal.NewFromInt(domain.MaxCapitalCHF),
		MaxPositions:      10,
		RankBuffer:        15,
		MaxHoldingMonths:  3,
		MinTradeValueCHF:  decimal.NewFromInt(150),
	}
}

func (c Config) Validate() error {
	if !c.InitialCapitalCHF.IsPositive() || c.InitialCapitalCHF.GreaterThan(decimal.NewFromInt(domain.MaxCapitalCHF)) {
		return fmt.Errorf("initial capital must be greater than zero and no more than %d CHF", domain.MaxCapitalCHF)
	}
	if c.MaxPositions < 1 || c.MaxPositions > 100 {
		return errors.New("max positions must be between 1 and 100")
	}
	if c.RankBuffer < c.MaxPositions {
		return errors.New("rank buffer must be at least max positions")
	}
	if c.MaxHoldingMonths < 1 || c.MinTradeValueCHF.IsNegative() {
		return errors.New("holding months must be positive and minimum trade value non-negative")
	}
	return nil
}

func Select(candidates []Candidate, held map[string]time.Time, at time.Time, config Config) ([]Candidate, []Candidate, error) {
	if err := config.Validate(); err != nil {
		return nil, nil, err
	}
	bySymbol := make(map[string]Candidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.Symbol == "" || candidate.Rank < 1 {
			return nil, nil, errors.New("candidates require a symbol and positive rank")
		}
		if _, duplicate := bySymbol[candidate.Symbol]; duplicate {
			return nil, nil, fmt.Errorf("duplicate candidate %q", candidate.Symbol)
		}
		bySymbol[candidate.Symbol] = candidate
	}
	exits := make([]Candidate, 0)
	kept := make(map[string]struct{}, len(held))
	for symbol, entryDate := range held {
		candidate, ok := bySymbol[symbol]
		if !ok || !candidate.Eligible || candidate.Rank > config.RankBuffer ||
			monthsBetween(entryDate, at) >= config.MaxHoldingMonths {
			exits = append(exits, candidate)
			continue
		}
		kept[symbol] = struct{}{}
	}
	sort.Slice(exits, func(i, j int) bool { return exits[i].Symbol < exits[j].Symbol })
	targets := make([]Candidate, 0, config.MaxPositions-len(kept))
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
	for _, candidate := range candidates {
		if len(kept)+len(targets) >= config.MaxPositions {
			break
		}
		if _, alreadyHeld := kept[candidate.Symbol]; alreadyHeld || !candidate.Eligible {
			continue
		}
		if !candidate.PriceUSD.IsPositive() || !candidate.FXRate.IsPositive() {
			continue
		}
		positionBudget := config.InitialCapitalCHF.Div(decimal.NewFromInt(int64(config.MaxPositions)))
		shareValue := candidate.PriceUSD.Mul(candidate.FXRate)
		if !config.FractionalShares && shareValue.GreaterThan(positionBudget) {
			continue
		}
		targets = append(targets, candidate)
	}
	return targets, exits, nil
}

func monthsBetween(start, end time.Time) int {
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
}

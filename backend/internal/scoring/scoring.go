package scoring

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

type Factors struct {
	Momentum                   float64
	RelativeStrength           float64
	TrendQuality               float64
	RelativeVolume             float64
	VolatilityAdjustedMomentum float64
}

type StockInput struct {
	ID      string
	Symbol  string
	Factors Factors
}

type Weights struct {
	Momentum                   float64
	RelativeStrength           float64
	TrendQuality               float64
	RelativeVolume             float64
	VolatilityAdjustedMomentum float64
}

type ComponentScores struct {
	Momentum                   float64 `json:"momentum"`
	RelativeStrength           float64 `json:"relative_strength"`
	TrendQuality               float64 `json:"trend_quality"`
	RelativeVolume             float64 `json:"relative_volume"`
	VolatilityAdjustedMomentum float64 `json:"volatility_adjusted_momentum"`
}

type Result struct {
	ID         string          `json:"id"`
	Symbol     string          `json:"symbol"`
	Score      float64         `json:"score"`
	Rank       int             `json:"rank"`
	Components ComponentScores `json:"components"`
	Excluded   string          `json:"excluded,omitempty"`
}

type rankedValue struct {
	index int
	value float64
}

func DefaultWeights() Weights {
	return Weights{
		Momentum: 0.30, RelativeStrength: 0.20, TrendQuality: 0.20,
		RelativeVolume: 0.15, VolatilityAdjustedMomentum: 0.15,
	}
}

func (w Weights) Validate() error {
	values := []float64{w.Momentum, w.RelativeStrength, w.TrendQuality, w.RelativeVolume, w.VolatilityAdjustedMomentum}
	total := 0.0
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("weights must be finite and non-negative")
		}
		total += value
	}
	if math.Abs(total-1) > 1e-9 {
		return fmt.Errorf("weights must sum to 1 (got %.12g)", total)
	}
	return nil
}

func Rank(stocks []StockInput, weights Weights) ([]Result, error) {
	if err := weights.Validate(); err != nil {
		return nil, err
	}
	results := make([]Result, len(stocks))
	factorVectors := make([][]float64, 5)
	for i := range factorVectors {
		factorVectors[i] = make([]float64, len(stocks))
	}
	valid := make([]bool, len(stocks))
	for i, stock := range stocks {
		results[i] = Result{ID: stock.ID, Symbol: stock.Symbol}
		values := factorValues(stock.Factors)
		valid[i] = true
		for f, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				valid[i] = false
			}
			factorVectors[f][i] = value
		}
		if !valid[i] {
			results[i].Excluded = "missing data: one or more required factors are unavailable"
		}
	}

	scores := make([][]float64, 5)
	for f := range factorVectors {
		values := make([]float64, 0, len(stocks))
		indices := make([]int, 0, len(stocks))
		for i, value := range factorVectors[f] {
			if valid[i] {
				values = append(values, value)
				indices = append(indices, i)
			}
		}
		ranks := percentileRanks(winsorize(values))
		scores[f] = make([]float64, len(stocks))
		for i, originalIndex := range indices {
			scores[f][originalIndex] = ranks[i]
		}
	}

	weightValues := []float64{weights.Momentum, weights.RelativeStrength, weights.TrendQuality, weights.RelativeVolume, weights.VolatilityAdjustedMomentum}
	for i := range stocks {
		if !valid[i] {
			continue
		}
		components := ComponentScores{
			Momentum: scores[0][i], RelativeStrength: scores[1][i],
			TrendQuality: scores[2][i], RelativeVolume: scores[3][i],
			VolatilityAdjustedMomentum: scores[4][i],
		}
		contributions := []float64{
			components.Momentum, components.RelativeStrength, components.TrendQuality,
			components.RelativeVolume, components.VolatilityAdjustedMomentum,
		}
		for f, contribution := range contributions {
			results[i].Score += contribution * weightValues[f]
		}
		results[i].Components = components
	}

	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if (left.Excluded == "") != (right.Excluded == "") {
			return left.Excluded == ""
		}
		if left.Excluded != "" {
			if left.Symbol != right.Symbol {
				return left.Symbol < right.Symbol
			}
			return left.ID < right.ID
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Components.Momentum != right.Components.Momentum {
			return left.Components.Momentum > right.Components.Momentum
		}
		if left.Symbol != right.Symbol {
			return left.Symbol < right.Symbol
		}
		return left.ID < right.ID
	})
	rank := 0
	for i := range results {
		if results[i].Excluded == "" {
			rank++
			results[i].Rank = rank
		}
	}
	return results, nil
}

func winsorize(values []float64) []float64 {
	if len(values) < 2 {
		return append([]float64(nil), values...)
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	lower, upper := quantile(sorted, 0.01), quantile(sorted, 0.99)
	output := make([]float64, len(values))
	for i, value := range values {
		output[i] = math.Max(lower, math.Min(value, upper))
	}
	return output
}

func quantile(sorted []float64, p float64) float64 {
	position := p * float64(len(sorted)-1)
	lower, upper := int(math.Floor(position)), int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	fraction := position - float64(lower)
	return sorted[lower]*(1-fraction) + sorted[upper]*fraction
}

func percentileRanks(values []float64) []float64 {
	ranks := make([]rankedValue, len(values))
	for i, value := range values {
		ranks[i] = rankedValue{index: i, value: value}
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].value == ranks[j].value {
			return ranks[i].index < ranks[j].index
		}
		return ranks[i].value < ranks[j].value
	})
	if len(ranks) == 1 {
		return []float64{50}
	}
	output := make([]float64, len(ranks))
	for start := 0; start < len(ranks); {
		end := start + 1
		for end < len(ranks) && ranks[end].value == ranks[start].value {
			end++
		}
		averageRank := float64(start+end-1) / 2
		percentile := averageRank / float64(len(ranks)-1) * 100
		for i := start; i < end; i++ {
			output[ranks[i].index] = percentile
		}
		start = end
	}
	return output
}

func factorValues(f Factors) []float64 {
	return []float64{
		f.Momentum, f.RelativeStrength, f.TrendQuality,
		f.RelativeVolume, f.VolatilityAdjustedMomentum,
	}
}

package scoring

import (
	"math"
	"sort"
	"time"
)

type DatedFactors struct {
	Date   time.Time
	Symbol string
	Values Factors
}

type Correlation struct {
	FactorA string  `json:"factor_a"`
	FactorB string  `json:"factor_b"`
	Value   float64 `json:"value"`
	Samples int     `json:"samples"`
}

var factorNames = []string{
	"momentum", "relative_strength", "trend_quality", "relative_volume", "volatility_adjusted_momentum",
}

func FactorCorrelations(data []DatedFactors) []Correlation {
	byDate := make(map[string][]DatedFactors)
	for _, observation := range data {
		byDate[observation.Date.Format("2006-01-02")] = append(byDate[observation.Date.Format("2006-01-02")], observation)
	}
	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	type aggregate struct {
		total   float64
		samples int
	}
	aggregates := make(map[[2]int]aggregate)
	for _, date := range dates {
		rows := byDate[date]
		vectors := make([][]float64, len(factorNames))
		for factor := range vectors {
			vectors[factor] = make([]float64, 0, len(rows))
		}
		for _, row := range rows {
			values := factorValues(row.Values)
			valid := true
			for _, value := range values {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			for factor, value := range values {
				vectors[factor] = append(vectors[factor], value)
			}
		}
		ranks := make([][]float64, len(vectors))
		for factor := range vectors {
			ranks[factor] = ordinalRanks(vectors[factor])
		}
		for left := range factorNames {
			for right := left; right < len(factorNames); right++ {
				correlation, ok := pearson(ranks[left], ranks[right])
				if !ok {
					continue
				}
				key := [2]int{left, right}
				value := aggregates[key]
				value.total += correlation
				value.samples++
				aggregates[key] = value
			}
		}
	}
	result := make([]Correlation, 0, len(factorNames)*(len(factorNames)+1)/2)
	for left := range factorNames {
		for right := left; right < len(factorNames); right++ {
			value := aggregates[[2]int{left, right}]
			average := 0.0
			if value.samples > 0 {
				average = value.total / float64(value.samples)
			}
			result = append(result, Correlation{
				FactorA: factorNames[left], FactorB: factorNames[right],
				Value: average, Samples: value.samples,
			})
		}
	}
	return result
}

func ordinalRanks(values []float64) []float64 {
	items := make([]rankedValue, len(values))
	for i, value := range values {
		items[i] = rankedValue{index: i, value: value}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].value == items[j].value {
			return items[i].index < items[j].index
		}
		return items[i].value < items[j].value
	})
	result := make([]float64, len(values))
	for start := 0; start < len(items); {
		end := start + 1
		for end < len(items) && items[end].value == items[start].value {
			end++
		}
		rank := float64(start+end-1) / 2
		for i := start; i < end; i++ {
			result[items[i].index] = rank
		}
		start = end
	}
	return result
}

func pearson(left, right []float64) (float64, bool) {
	if len(left) != len(right) || len(left) < 2 {
		return 0, false
	}
	var meanLeft, meanRight float64
	for i := range left {
		meanLeft += left[i]
		meanRight += right[i]
	}
	meanLeft /= float64(len(left))
	meanRight /= float64(len(right))
	var covariance, varianceLeft, varianceRight float64
	for i := range left {
		x, y := left[i]-meanLeft, right[i]-meanRight
		covariance += x * y
		varianceLeft += x * x
		varianceRight += y * y
	}
	if varianceLeft == 0 || varianceRight == 0 {
		return 0, false
	}
	return covariance / math.Sqrt(varianceLeft*varianceRight), true
}

package scoring

import (
	"math"
	"testing"
	"time"

	"github.com/angelaschereraiza/trend_radar/backend/internal/indicators"
)

func TestRankDeterministicAndBounded(t *testing.T) {
	inputs := []StockInput{
		{ID: "3", Symbol: "CCC", Factors: Factors{3, 3, 3, 3, 3}},
		{ID: "1", Symbol: "AAA", Factors: Factors{1, 1, 1, 1, 1}},
		{ID: "2", Symbol: "BBB", Factors: Factors{2, 2, 2, 2, 2}},
	}
	results, err := Rank(inputs, DefaultWeights())
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Symbol != "CCC" || results[0].Rank != 1 || results[0].Score != 100 {
		t.Fatalf("top result = %+v", results[0])
	}
	for _, result := range results {
		if result.Score < 0 || result.Score > 100 {
			t.Fatalf("score outside [0, 100]: %v", result.Score)
		}
	}
}

func TestFactorCorrelationsUseDailyCrossSectionalRanks(t *testing.T) {
	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	rows := make([]DatedFactors, 3)
	for i := range rows {
		value := float64(i)
		rows[i] = DatedFactors{
			Date: date, Symbol: string(rune('A' + i)),
			Values: Factors{value, value * 10, value + 3, value * 2, value - 1},
		}
	}
	matrix := FactorCorrelations(rows)
	if len(matrix) != 15 {
		t.Fatalf("correlation pairs = %d, want 15", len(matrix))
	}
	if matrix[1].Samples != 1 || math.Abs(matrix[1].Value-1) > 1e-12 {
		t.Fatalf("momentum correlation = %+v, want 1.0 with one sample", matrix[1])
	}
}

func TestCalculateFactorsProducesMomentumInputs(t *testing.T) {
	stock := make([]indicators.Observation, 260)
	benchmark := make([]indicators.Observation, 260)
	for i := range stock {
		date := time.Date(2023, 1, 2+i, 0, 0, 0, 0, time.UTC)
		stock[i] = indicators.Observation{Date: date, Close: 100 * math.Pow(1.004, float64(i)), Volume: 1_000_000}
		benchmark[i] = indicators.Observation{Date: date, Close: 100 * math.Pow(1.002, float64(i)), Volume: 1_000_000}
	}
	factors, err := CalculateFactors(stock, benchmark)
	if err != nil {
		t.Fatal(err)
	}
	if factors.Momentum <= 0 || factors.RelativeStrength <= 0 || factors.VolatilityAdjustedMomentum <= 0 {
		t.Fatalf("unexpected factors: %+v", factors)
	}
}

func TestMissingFactorIsExplicitlyExcluded(t *testing.T) {
	inputs := []StockInput{
		{ID: "1", Symbol: "AAA", Factors: Factors{Momentum: math.NaN()}},
		{ID: "2", Symbol: "BBB", Factors: Factors{1, 1, 1, 1, 1}},
	}
	results, err := Rank(inputs, DefaultWeights())
	if err != nil {
		t.Fatal(err)
	}
	if results[1].Symbol != "AAA" || results[1].Excluded == "" || results[1].Rank != 0 {
		t.Fatalf("missing factor result = %+v", results[1])
	}
}

func TestEqualFactorsUseStableSymbolTieBreak(t *testing.T) {
	inputs := []StockInput{
		{ID: "2", Symbol: "BBB", Factors: Factors{1, 1, 1, 1, 1}},
		{ID: "1", Symbol: "AAA", Factors: Factors{1, 1, 1, 1, 1}},
	}
	results, err := Rank(inputs, DefaultWeights())
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Symbol != "AAA" || results[1].Symbol != "BBB" {
		t.Fatalf("results = %+v", results)
	}
}

func TestRejectInvalidWeights(t *testing.T) {
	weights := DefaultWeights()
	weights.Momentum = 0.8
	if _, err := Rank(nil, weights); err == nil {
		t.Fatal("Rank accepted weights that do not sum to one")
	}
}

func TestOneStockGetsNeutralPercentile(t *testing.T) {
	input := StockInput{ID: "1", Symbol: "AAA", Factors: Factors{1, 1, 1, 1, 1}}
	results, err := Rank([]StockInput{input}, DefaultWeights())
	if err != nil || results[0].Score != 50 {
		t.Fatalf("single-stock score = %+v, %v; want 50", results, err)
	}
}

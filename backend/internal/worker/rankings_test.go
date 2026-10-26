package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
)

func TestCalculateLatestRanking(t *testing.T) {
	start := time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC)
	history := make([]repository.PriceHistory, 0, 4*260)
	symbols := []struct {
		symbol string
		id     string
		growth float64
	}{
		{"SPY", "00000000-0000-4000-8000-000000000004", 1.002},
		{"AAA", "00000000-0000-4000-8000-000000000001", 1.004},
		{"BBB", "00000000-0000-4000-8000-000000000002", 1.010},
		{"CCC", "00000000-0000-4000-8000-000000000003", 1.006},
	}
	for day := 0; day < 260; day++ {
		date := start.AddDate(0, 0, day)
		for _, symbol := range symbols {
			price := 100 * pow(symbol.growth, day)
			history = append(history, repository.PriceHistory{
				StockID: uuid.MustParse(symbol.id), Symbol: symbol.symbol,
				Sector: "Test", Date: date, Close: decimal.NewFromFloat(price),
				AdjustedClose: decimal.NewFromFloat(price), Volume: 1_000_000,
			})
		}
	}
	lastDate := start.AddDate(0, 0, 259)
	run, err := CalculateLatestRanking(history, map[string]string{
		lastDate.Format("2006-01-02"): "0.88",
	}, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Entries) != 3 || len(run.Correlations) != 15 {
		t.Fatalf("ranking result = %d entries, %d correlations", len(run.Entries), len(run.Correlations))
	}
	for _, entry := range run.Entries {
		if entry.Score < 0 || entry.Score > 100 {
			t.Fatalf("score outside range: %v", entry.Score)
		}
	}
	if run.Date.Format("2006-01-02") != lastDate.Format("2006-01-02") {
		t.Fatalf("ranking date %s differs from latest fixture %s", run.Date, lastDate)
	}
}

func pow(base float64, exponent int) float64 {
	result := 1.0
	for i := 0; i < exponent; i++ {
		result *= base
	}
	return result
}

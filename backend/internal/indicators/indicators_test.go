package indicators

import (
	"math"
	"testing"
	"time"
)

func observations(prices, volumes []float64) []Observation {
	data := make([]Observation, len(prices))
	for i, price := range prices {
		data[i] = Observation{Date: time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC), Close: price}
		if i < len(volumes) {
			data[i].Volume = volumes[i]
		}
	}
	return data
}

func TestSMAEMAAndMomentum(t *testing.T) {
	data := observations([]float64{1, 2, 3, 4}, nil)
	sma, err := SMA(data, 3)
	if err != nil || !sma[2].Available || sma[2].Number != 2 {
		t.Fatalf("SMA = %+v, %v", sma, err)
	}
	ema, err := EMA(data, 3)
	if err != nil || ema[2].Number != 2 || ema[3].Number != 3 {
		t.Fatalf("EMA = %+v, %v", ema, err)
	}
	momentum, err := Momentum(data, 2)
	if err != nil || math.Abs(momentum[3].Number-1) > 1e-12 {
		t.Fatalf("Momentum = %+v, %v", momentum, err)
	}
}

func TestRSIAndRelativeVolume(t *testing.T) {
	up := observations([]float64{1, 2, 3, 4}, []float64{10, 10, 10, 20})
	rsi, err := RSI(up, 2)
	if err != nil || rsi[2].Number != 100 {
		t.Fatalf("RSI = %+v, %v", rsi, err)
	}
	volume, err := RelativeVolume(up, 3)
	if err != nil || volume[3].Number != 2 {
		t.Fatalf("RelativeVolume = %+v, %v", volume, err)
	}
}

func TestVolatilityAndTrendQuality(t *testing.T) {
	constant := observations([]float64{2, 2, 2, 2, 2}, nil)
	volatility, err := Volatility(constant, 3)
	if err != nil || !volatility[3].Available || volatility[3].Number != 0 {
		t.Fatalf("Volatility = %+v, %v", volatility, err)
	}
	trend, err := TrendQuality(observations([]float64{1, 2, 4, 8}, nil), 4)
	if err != nil || !trend[3].Available || trend[3].Number <= 0 {
		t.Fatalf("TrendQuality = %+v, %v", trend, err)
	}
}

func TestRelativeReturnRequiresAlignedDates(t *testing.T) {
	stock := observations([]float64{10, 11}, nil)
	benchmark := observations([]float64{10, 10}, nil)
	benchmark[1].Date = benchmark[1].Date.AddDate(0, 0, 1)
	if _, err := RelativeReturn(stock, benchmark, 1); err == nil {
		t.Fatal("RelativeReturn accepted misaligned dates")
	}
}

func TestInvalidOrderingRejected(t *testing.T) {
	data := observations([]float64{1, 2}, nil)
	data[0], data[1] = data[1], data[0]
	if _, err := SMA(data, 1); err == nil {
		t.Fatal("SMA accepted unsorted observations")
	}
}

func TestHistoricalValuesUnaffectedByFutureObservation(t *testing.T) {
	data := observations([]float64{10, 11, 12, 13}, nil)
	before, err := Momentum(data, 2)
	if err != nil {
		t.Fatal(err)
	}
	extended := append(append([]Observation(nil), data...), Observation{
		Date: data[len(data)-1].Date.AddDate(0, 0, 1), Close: 10000,
	})
	after, err := Momentum(extended, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("future data changed historical index %d", i)
		}
	}
}

func TestMomentumExcludingRecentMonth(t *testing.T) {
	data := make([]Observation, 253)
	for i := range data {
		price := 100.0
		if i == 231 {
			price = 110
		}
		if i == 252 {
			price = 200
		}
		data[i] = Observation{
			Date: time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC), Close: price,
		}
	}
	values, err := MomentumExcludingRecent(data, 252, 21)
	if err != nil || !values[252].Available || math.Abs(values[252].Number-0.1) > 1e-12 {
		t.Fatalf("12-1 momentum = %+v, %v; want 0.1", values[252], err)
	}
}

package scoring

import (
	"errors"
	"math"

	"github.com/angelaschereraiza/trend_radar/backend/internal/indicators"
)

func CalculateFactors(stock, benchmark []indicators.Observation) (Factors, error) {
	if len(stock) == 0 || len(benchmark) == 0 {
		return Factors{}, errors.New("stock and benchmark observations are required")
	}
	if len(stock) != len(benchmark) {
		return Factors{}, errors.New("stock and benchmark observations must align")
	}
	for i := range stock {
		if !stock[i].Date.Equal(benchmark[i].Date) {
			return Factors{}, errors.New("stock and benchmark dates must align")
		}
	}
	momentum, err := indicators.MomentumExcludingRecent(stock, 252, 21)
	if err != nil {
		return Factors{}, err
	}
	relative, err := indicators.RelativeReturn(stock, benchmark, 126)
	if err != nil {
		return Factors{}, err
	}
	trend, err := indicators.TrendQuality(stock, 126)
	if err != nil {
		return Factors{}, err
	}
	volume, err := indicators.RelativeVolume(stock, 50)
	if err != nil {
		return Factors{}, err
	}
	volatility, err := indicators.Volatility(stock, 126)
	if err != nil {
		return Factors{}, err
	}
	last := len(stock) - 1
	factorValues := []indicators.Value{momentum[last], relative[last], trend[last], volume[last], volatility[last]}
	for _, value := range factorValues {
		if !value.Available {
			return Factors{}, errors.New("insufficient or missing indicator history")
		}
	}
	volAdjMomentum := math.NaN()
	if volatility[last].Number > 0 {
		volAdjMomentum = momentum[last].Number / volatility[last].Number
	}
	return Factors{
		Momentum:                   momentum[last].Number,
		RelativeStrength:           relative[last].Number,
		TrendQuality:               trend[last].Number,
		RelativeVolume:             volume[last].Number,
		VolatilityAdjustedMomentum: volAdjMomentum,
	}, nil
}

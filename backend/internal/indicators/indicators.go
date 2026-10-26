package indicators

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type Observation struct {
	Date   time.Time
	Close  float64
	Volume float64
}

type Value struct {
	Number    float64
	Available bool
}

func validate(data []Observation) error {
	for i, item := range data {
		if item.Date.IsZero() {
			return fmt.Errorf("observation %d has no date", i)
		}
		if math.IsNaN(item.Close) || math.IsInf(item.Close, 0) || item.Close < 0 {
			return fmt.Errorf("observation %d has invalid close", i)
		}
		if math.IsNaN(item.Volume) || math.IsInf(item.Volume, 0) || item.Volume < 0 {
			return fmt.Errorf("observation %d has invalid volume", i)
		}
		if i > 0 && !item.Date.After(data[i-1].Date) {
			return errors.New("observations must have unique dates in ascending order")
		}
	}
	return nil
}

func SMA(data []Observation, period int) ([]Value, error) {
	if period <= 0 {
		return nil, errors.New("period must be positive")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	var sum float64
	validCount := 0
	for i, item := range data {
		sum += item.Close
		if item.Close > 0 {
			validCount++
		}
		if i >= period {
			sum -= data[i-period].Close
			if data[i-period].Close > 0 {
				validCount--
			}
		}
		if i+1 >= period && validCount == period {
			result[i] = Value{Number: sum / float64(period), Available: true}
		}
	}
	return result, nil
}

func EMA(data []Observation, period int) ([]Value, error) {
	if period <= 0 {
		return nil, errors.New("period must be positive")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	alpha := 2 / float64(period+1)
	runStart, ema := 0, 0.0
	for i := range data {
		if data[i].Close <= 0 {
			runStart, ema = i+1, 0
			continue
		}
		if i-runStart+1 < period {
			continue
		}
		if i-runStart+1 == period {
			for j := runStart; j <= i; j++ {
				ema += data[j].Close
			}
			ema /= float64(period)
			result[i] = Value{Number: ema, Available: true}
			continue
		}
		ema = alpha*data[i].Close + (1-alpha)*ema
		result[i] = Value{Number: ema, Available: true}
	}
	return result, nil
}

func Momentum(data []Observation, lookback int) ([]Value, error) {
	if lookback <= 0 {
		return nil, errors.New("lookback must be positive")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	for i := lookback; i < len(data); i++ {
		start, end := data[i-lookback].Close, data[i].Close
		validWindow := true
		for j := i - lookback; j <= i; j++ {
			if data[j].Close <= 0 {
				validWindow = false
				break
			}
		}
		if start > 0 && validWindow {
			result[i] = Value{Number: end/start - 1, Available: true}
		}
	}
	return result, nil
}

func MomentumExcludingRecent(data []Observation, lookback, excludedRecent int) ([]Value, error) {
	if lookback <= 0 || excludedRecent < 0 || excludedRecent >= lookback {
		return nil, errors.New("lookback must be positive and excluded recent period must be smaller than lookback")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	for i := lookback; i < len(data); i++ {
		startIndex, endIndex := i-lookback, i-excludedRecent
		valid := true
		for j := startIndex; j <= i; j++ {
			if data[j].Close <= 0 {
				valid = false
				break
			}
		}
		if valid {
			result[i] = Value{
				Number: data[endIndex].Close/data[startIndex].Close - 1, Available: true,
			}
		}
	}
	return result, nil
}

func RelativeReturn(stock, benchmark []Observation, lookback int) ([]Value, error) {
	if len(stock) != len(benchmark) {
		return nil, errors.New("stock and benchmark dates must align")
	}
	for i := range stock {
		if !stock[i].Date.Equal(benchmark[i].Date) {
			return nil, fmt.Errorf("date mismatch at index %d", i)
		}
	}
	stockReturn, err := Momentum(stock, lookback)
	if err != nil {
		return nil, err
	}
	benchmarkReturn, err := Momentum(benchmark, lookback)
	if err != nil {
		return nil, err
	}
	for i := range stockReturn {
		if stockReturn[i].Available && benchmarkReturn[i].Available {
			stockReturn[i].Number -= benchmarkReturn[i].Number
		} else {
			stockReturn[i] = Value{}
		}
	}
	return stockReturn, nil
}

func Volatility(data []Observation, window int) ([]Value, error) {
	if window <= 1 {
		return nil, errors.New("volatility window must be greater than one")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	returns := make([]float64, len(data))
	valid := make([]bool, len(data))
	for i := 1; i < len(data); i++ {
		if data[i-1].Close > 0 && data[i].Close > 0 {
			returns[i] = math.Log(data[i].Close / data[i-1].Close)
			valid[i] = true
		}
	}
	for i := window; i < len(data); i++ {
		mean := 0.0
		ok := true
		for j := i - window + 1; j <= i; j++ {
			if !valid[j] {
				ok = false
				break
			}
			mean += returns[j]
		}
		if !ok {
			continue
		}
		mean /= float64(window)
		var sumSquares float64
		for j := i - window + 1; j <= i; j++ {
			delta := returns[j] - mean
			sumSquares += delta * delta
		}
		result[i] = Value{Number: math.Sqrt(sumSquares/float64(window-1)) * math.Sqrt(252), Available: true}
	}
	return result, nil
}

func RelativeVolume(data []Observation, window int) ([]Value, error) {
	if window <= 0 {
		return nil, errors.New("volume window must be positive")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	for i := window; i < len(data); i++ {
		var total float64
		for j := i - window; j < i; j++ {
			total += data[j].Volume
		}
		average := total / float64(window)
		if average > 0 {
			result[i] = Value{Number: data[i].Volume / average, Available: true}
		}
	}
	return result, nil
}

func RSI(data []Observation, period int) ([]Value, error) {
	if period <= 0 {
		return nil, errors.New("period must be positive")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	var avgGain, avgLoss, seedGain, seedLoss float64
	seedCount := 0
	for i := 1; i < len(data); i++ {
		if data[i-1].Close <= 0 || data[i].Close <= 0 {
			avgGain, avgLoss, seedGain, seedLoss, seedCount = 0, 0, 0, 0, 0
			continue
		}
		gain, loss := change(data[i-1].Close, data[i].Close)
		if seedCount < period {
			seedGain += gain
			seedLoss += loss
			seedCount++
			if seedCount == period {
				avgGain = seedGain / float64(period)
				avgLoss = seedLoss / float64(period)
				result[i] = Value{Number: rsiValue(avgGain, avgLoss), Available: true}
			}
			continue
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		result[i] = Value{Number: rsiValue(avgGain, avgLoss), Available: true}
	}
	return result, nil
}

func TrendQuality(data []Observation, window int) ([]Value, error) {
	if window < 2 {
		return nil, errors.New("trend-quality window must be at least two")
	}
	if err := validate(data); err != nil {
		return nil, err
	}
	result := make([]Value, len(data))
	for end := window - 1; end < len(data); end++ {
		logs := make([]float64, window)
		valid := true
		for j := 0; j < window; j++ {
			price := data[end-window+1+j].Close
			if price <= 0 {
				valid = false
				break
			}
			logs[j] = math.Log(price)
		}
		if !valid {
			continue
		}
		var meanY float64
		for _, y := range logs {
			meanY += y
		}
		meanY /= float64(window)
		meanX := float64(window-1) / 2
		var covariance, varianceX, varianceY float64
		for i, y := range logs {
			dx, dy := float64(i)-meanX, y-meanY
			covariance += dx * dy
			varianceX += dx * dx
			varianceY += dy * dy
		}
		if varianceX == 0 {
			continue
		}
		slope := covariance / varianceX
		rSquared := 0.0
		if varianceY > 0 {
			rSquared = covariance * covariance / (varianceX * varianceY)
		}
		result[end] = Value{Number: slope * 252 * rSquared, Available: true}
	}
	return result, nil
}

func VolatilityAdjustedMomentum(data []Observation, momentumLookback, volatilityWindow int) ([]Value, error) {
	momentum, err := Momentum(data, momentumLookback)
	if err != nil {
		return nil, err
	}
	volatility, err := Volatility(data, volatilityWindow)
	if err != nil {
		return nil, err
	}
	for i := range momentum {
		if !momentum[i].Available || !volatility[i].Available || volatility[i].Number == 0 {
			momentum[i] = Value{}
			continue
		}
		momentum[i].Number /= volatility[i].Number
	}
	return momentum, nil
}

func change(previous, current float64) (gain, loss float64) {
	delta := current - previous
	if delta > 0 {
		return delta, 0
	}
	return 0, -delta
}

func rsiValue(avgGain, avgLoss float64) float64 {
	if avgGain == 0 && avgLoss == 0 {
		return 50
	}
	if avgLoss == 0 {
		return 100
	}
	return 100 - 100/(1+avgGain/avgLoss)
}

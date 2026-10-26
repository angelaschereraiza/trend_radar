package stats

import (
	"errors"
	"math"
	"math/rand"
	"sort"
)

type BootstrapConfig struct {
	Replicates      int
	BlockLength     int
	Seed            int64
	ConfidenceLevel float64
	RiskFreeAnnual  float64
}

type Interval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

type ConfidenceIntervals struct {
	Sharpe       Interval `json:"sharpe"`
	CAGR         Interval `json:"cagr"`
	ExcessReturn Interval `json:"excess_return"`
	Replicates   int      `json:"replicates"`
	Seed         int64    `json:"seed"`
}

type PerformanceSample struct {
	Sharpe       float64
	CAGR         float64
	ExcessReturn float64
}

func StationaryBootstrap(strategyReturns, benchmarkReturns []float64, config BootstrapConfig) (ConfidenceIntervals, error) {
	if len(strategyReturns) == 0 || len(strategyReturns) != len(benchmarkReturns) {
		return ConfidenceIntervals{}, errors.New("strategy and benchmark returns must be non-empty and aligned")
	}
	if config.Replicates < 1 || config.BlockLength < 1 {
		return ConfidenceIntervals{}, errors.New("replicates and block length must be positive")
	}
	if config.ConfidenceLevel <= 0 || config.ConfidenceLevel >= 1 {
		return ConfidenceIntervals{}, errors.New("confidence level must be between zero and one")
	}
	for i := range strategyReturns {
		if !finite(strategyReturns[i]) || !finite(benchmarkReturns[i]) ||
			strategyReturns[i] <= -1 || benchmarkReturns[i] <= -1 {
			return ConfidenceIntervals{}, errors.New("returns must be finite and greater than -100%")
		}
	}
	random := rand.New(rand.NewSource(config.Seed))
	sharpeSamples := make([]float64, config.Replicates)
	cagrSamples := make([]float64, config.Replicates)
	excessSamples := make([]float64, config.Replicates)
	for replicate := 0; replicate < config.Replicates; replicate++ {
		sampleStrategy, sampleBenchmark := sampleStationary(strategyReturns, benchmarkReturns, config.BlockLength, random)
		metrics := sampleMetrics(sampleStrategy, sampleBenchmark, config.RiskFreeAnnual)
		sharpeSamples[replicate] = metrics.Sharpe
		cagrSamples[replicate] = metrics.CAGR
		excessSamples[replicate] = metrics.ExcessReturn
	}
	alpha := (1 - config.ConfidenceLevel) / 2
	return ConfidenceIntervals{
		Sharpe:       Interval{Lower: quantile(sharpeSamples, alpha), Upper: quantile(sharpeSamples, 1-alpha)},
		CAGR:         Interval{Lower: quantile(cagrSamples, alpha), Upper: quantile(cagrSamples, 1-alpha)},
		ExcessReturn: Interval{Lower: quantile(excessSamples, alpha), Upper: quantile(excessSamples, 1-alpha)},
		Replicates:   config.Replicates, Seed: config.Seed,
	}, nil
}

func sampleStationary(strategyReturns, benchmarkReturns []float64, blockLength int, random *rand.Rand) ([]float64, []float64) {
	n := len(strategyReturns)
	strategySample := make([]float64, n)
	benchmarkSample := make([]float64, n)
	index := random.Intn(n)
	restartProbability := 1 / float64(blockLength)
	for i := 0; i < n; i++ {
		if i > 0 && random.Float64() < restartProbability {
			index = random.Intn(n)
		} else if i > 0 {
			index = (index + 1) % n
		}
		strategySample[i] = strategyReturns[index]
		benchmarkSample[i] = benchmarkReturns[index]
	}
	return strategySample, benchmarkSample
}

func sampleMetrics(strategyReturns, benchmarkReturns []float64, riskFreeAnnual float64) PerformanceSample {
	meanReturn := average(strategyReturns)
	stdDev := standardDeviation(strategyReturns)
	sharpe := 0.0
	if stdDev > 0 {
		sharpe = (meanReturn - riskFreeAnnual/252) / stdDev * math.Sqrt(252)
	}
	strategyGrowth, benchmarkGrowth := 1.0, 1.0
	for i := range strategyReturns {
		strategyGrowth *= 1 + strategyReturns[i]
		benchmarkGrowth *= 1 + benchmarkReturns[i]
	}
	strategyCAGR := math.Pow(strategyGrowth, 252/float64(len(strategyReturns))) - 1
	benchmarkCAGR := math.Pow(benchmarkGrowth, 252/float64(len(benchmarkReturns))) - 1
	return PerformanceSample{Sharpe: sharpe, CAGR: strategyCAGR, ExcessReturn: strategyCAGR - benchmarkCAGR}
}

func quantile(values []float64, p float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	position := p * float64(len(sorted)-1)
	lower, upper := int(math.Floor(position)), int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	fraction := position - float64(lower)
	return sorted[lower]*(1-fraction) + sorted[upper]*fraction
}

func average(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func standardDeviation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := average(values)
	var sum float64
	for _, value := range values {
		delta := value - mean
		sum += delta * delta
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

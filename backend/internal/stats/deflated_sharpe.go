package stats

import (
	"errors"
	"math"
)

func DeflatedSharpeRatio(returns []float64, trialCount int) (float64, error) {
	if len(returns) < 2 || trialCount < 1 {
		return 0, errors.New("at least two returns and one trial are required")
	}
	for _, value := range returns {
		if !finite(value) || value <= -1 {
			return 0, errors.New("returns must be finite and greater than -100%")
		}
	}
	mean := average(returns)
	stdDev := standardDeviation(returns)
	if stdDev == 0 {
		return 0.5, nil
	}
	sharpe := mean / stdDev
	var third, fourth float64
	for _, value := range returns {
		standardized := (value - mean) / stdDev
		third += standardized * standardized * standardized
		fourth += standardized * standardized * standardized * standardized
	}
	skewness := third / float64(len(returns))
	excessKurtosis := fourth/float64(len(returns)) - 3
	expectedMaximum := 0.0
	if trialCount > 1 {
		expectedMaximum = 0.5772156649015329*normalQuantile(1-1/float64(trialCount)) +
			(1-0.5772156649015329)*normalQuantile(1-1/(float64(trialCount)*math.E))
	}
	variance := 1 - skewness*sharpe + (excessKurtosis+2)*sharpe*sharpe/4
	if variance <= 0 {
		return 0, errors.New("deflated Sharpe variance is not positive")
	}
	z := (sharpe - expectedMaximum) * math.Sqrt(float64(len(returns)-1)) / math.Sqrt(variance)
	return 0.5 * (1 + math.Erf(z/math.Sqrt2)), nil
}

func normalQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	const (
		a1    = -3.969683028665376e+01
		a2    = 2.209460984245205e+02
		a3    = -2.759285104469687e+02
		a4    = 1.383577518672690e+02
		a5    = -3.066479806614716e+01
		a6    = 2.506628277459239e+00
		b1    = -5.447609879822406e+01
		b2    = 1.615858368580409e+02
		b3    = -1.556989798598866e+02
		b4    = 6.680131188771972e+01
		b5    = -1.328068155288572e+01
		c1    = -7.784894002430293e-03
		c2    = -3.223964580411365e-01
		c3    = -2.400758277161838e+00
		c4    = -2.549732539343734e+00
		c5    = 4.374664141464968e+00
		c6    = 2.938163982698783e+00
		d1    = 7.784695709041462e-03
		d2    = 3.224671290700398e-01
		d3    = 2.445134137142996e+00
		d4    = 3.754408661907416e+00
		plow  = 0.02425
		phigh = 1 - plow
	)
	if p < plow {
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	if p > phigh {
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	q := p - 0.5
	r := q * q
	return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
		(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
}

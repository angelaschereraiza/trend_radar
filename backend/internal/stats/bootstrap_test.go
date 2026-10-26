package stats

import (
	"reflect"
	"testing"
)

func TestStationaryBootstrapIsSeeded(t *testing.T) {
	strategy := []float64{0.01, -0.002, 0.004, 0.006, -0.001}
	benchmark := []float64{0.002, 0.001, 0.003, -0.001, 0.001}
	config := BootstrapConfig{Replicates: 200, BlockLength: 2, Seed: 42, ConfidenceLevel: 0.95}
	first, err := StationaryBootstrap(strategy, benchmark, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StationaryBootstrap(strategy, benchmark, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("bootstrap results differ for fixed seed: %+v %+v", first, second)
	}
}

func TestDeflatedSharpeRatio(t *testing.T) {
	returns := make([]float64, 500)
	for i := range returns {
		returns[i] = 0.001
		if i%2 == 0 {
			returns[i] += 0.001
		} else {
			returns[i] -= 0.001
		}
	}

	singleTrial, err := DeflatedSharpeRatio(returns, 1)
	if err != nil {
		t.Fatal(err)
	}
	manyTrials, err := DeflatedSharpeRatio(returns, 100)
	if err != nil {
		t.Fatal(err)
	}
	if singleTrial < 0 || singleTrial > 1 || manyTrials >= singleTrial {
		t.Fatalf("DSR should be bounded and decrease with trials: single=%v many=%v", singleTrial, manyTrials)
	}
}

func TestDeflatedSharpeRejectsTotalLossReturn(t *testing.T) {
	if _, err := DeflatedSharpeRatio([]float64{0.01, -1}, 1); err == nil {
		t.Fatal("DeflatedSharpeRatio accepted a return at or below -100%")
	}
}

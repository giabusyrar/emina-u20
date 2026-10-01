package benchmark_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
)

func TestAgreeKnownAnswer(t *testing.T) {
	// ours - theirs = [0, 1, 1, 2]: bias 1, sample SD sqrt(2/3), MAE 1;
	// Pearson r of ours vs theirs = 2 / sqrt(5).
	a := benchmark.Agree([]float64{1, 2, 3, 4}, []float64{1, 1, 2, 2})
	sd := math.Sqrt(2.0 / 3.0)
	assert.Equal(t, 4, a.N)
	require.NotNil(t, a.Bias)
	assert.InDelta(t, 1.0, *a.Bias, 1e-12)
	assert.InDelta(t, sd, *a.SDDiff, 1e-12)
	assert.InDelta(t, 1-benchmark.LoAZ*sd, *a.LoALow, 1e-12)
	assert.InDelta(t, 1+benchmark.LoAZ*sd, *a.LoAHigh, 1e-12)
	assert.InDelta(t, 1.0, *a.MAE, 1e-12)
	assert.InDelta(t, 2/math.Sqrt(5), *a.PearsonR, 1e-12)
}

func TestAgreeUndefinedStatisticsAreNil(t *testing.T) {
	none := benchmark.Agree(nil, nil)
	assert.Equal(t, 0, none.N)
	assert.Nil(t, none.Bias)
	assert.Nil(t, none.MAE)
	assert.Nil(t, none.SDDiff)

	one := benchmark.Agree([]float64{2}, []float64{1})
	assert.InDelta(t, 1.0, *one.Bias, 1e-12)
	assert.InDelta(t, 1.0, *one.MAE, 1e-12)
	assert.Nil(t, one.SDDiff, "a sample SD needs two pairs")
	assert.Nil(t, one.LoALow)
	assert.Nil(t, one.LoAHigh)

	two := benchmark.Agree([]float64{1, 2}, []float64{1, 3})
	assert.NotNil(t, two.SDDiff)
	assert.Nil(t, two.PearsonR, "with two pairs r is always +-1")

	flat := benchmark.Agree([]float64{1, 1, 1}, []float64{1, 2, 3})
	assert.Nil(t, flat.PearsonR, "no variance on one side: r undefined")
	assert.InDelta(t, 0.0, *benchmark.Agree([]float64{1, 2, 3}, []float64{1, 2, 3}).SDDiff, 1e-12)
}

func TestAgreeRefusesBadInput(t *testing.T) {
	assert.Panics(t, func() { benchmark.Agree([]float64{1}, []float64{1, 2}) })
	assert.Panics(t, func() { benchmark.Agree([]float64{math.NaN()}, []float64{1}) })
	assert.Panics(t, func() { benchmark.Agree([]float64{1}, []float64{math.Inf(1)}) })
}

func TestLoAZIsTheNormal975thPercentile(t *testing.T) {
	// Phi(z) = 0.975 <=> erf(z / sqrt 2) = 0.95.
	assert.InDelta(t, 0.95, math.Erf(benchmark.LoAZ/math.Sqrt2), 1e-12)
}

func TestAgreeConstantSeriesPearsonNil(t *testing.T) {
	// When one side is constant, r is undefined even if sums round-trip.
	// Regression: floating-point sxx/syy checks miss constant series that don't
	// produce exactly zero, like [0.1, 0.1, 0.1].
	a := benchmark.Agree([]float64{1, 2, 3}, []float64{0.1, 0.1, 0.1})
	assert.Nil(t, a.PearsonR, "constant theirs: r undefined")

	b := benchmark.Agree([]float64{0.1, 0.1, 0.1}, []float64{1, 2, 3})
	assert.Nil(t, b.PearsonR, "constant ours: r undefined")
}

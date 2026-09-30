package benchmark_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
)

// The defaults are the Open literals the user confirmed; each is defined
// once, here, and every run records the values it used.
func TestDefaultParams(t *testing.T) {
	assert.Equal(t, benchmark.Params{
		HeldOutFraction:    0.2,
		DedupMaxDistance:   6,
		MinClassCount:      10,
		ECEBins:            10,
		Seed:               1,
		MinConsensusRaters: 2,
	}, benchmark.DefaultParams())
}

func TestParamsValidate(t *testing.T) {
	require.NoError(t, benchmark.DefaultParams().Validate())
	// Both ends of dedupMaxDistance are legal: 0 is exact match only.
	edge := benchmark.DefaultParams()
	edge.DedupMaxDistance = 0
	require.NoError(t, edge.Validate())
	edge.DedupMaxDistance = benchmark.DHashBits
	require.NoError(t, edge.Validate())
	// Two is the smallest class count a sample standard deviation allows.
	edge.MinClassCount = 2
	require.NoError(t, edge.Validate())

	for name, tc := range map[string]struct {
		mutate func(*benchmark.Params)
		want   string
	}{
		"held-out zero":     {func(p *benchmark.Params) { p.HeldOutFraction = 0 }, "HeldOutFraction must be in (0,1), got 0"},
		"held-out one":      {func(p *benchmark.Params) { p.HeldOutFraction = 1 }, "HeldOutFraction must be in (0,1), got 1"},
		"held-out negative": {func(p *benchmark.Params) { p.HeldOutFraction = -0.1 }, "HeldOutFraction must be in (0,1), got -0.1"},
		"held-out NaN":      {func(p *benchmark.Params) { p.HeldOutFraction = math.NaN() }, "HeldOutFraction must be in (0,1), got NaN"},
		"ece bins":          {func(p *benchmark.Params) { p.ECEBins = 0 }, "ECEBins must be at least 1, got 0"},
		"min class count":   {func(p *benchmark.Params) { p.MinClassCount = 0 }, "MinClassCount must be at least 2 (a sample standard deviation needs n-1 >= 1), got 0"},
		"min class count 1": {func(p *benchmark.Params) { p.MinClassCount = 1 }, "MinClassCount must be at least 2 (a sample standard deviation needs n-1 >= 1), got 1"},
		"min raters":        {func(p *benchmark.Params) { p.MinConsensusRaters = -2 }, "MinConsensusRaters must be at least 1, got -2"},
		"dedup negative":    {func(p *benchmark.Params) { p.DedupMaxDistance = -1 }, "DedupMaxDistance must be in 0..64, got -1"},
		"dedup too big":     {func(p *benchmark.Params) { p.DedupMaxDistance = 65 }, "DedupMaxDistance must be in 0..64, got 65"},
	} {
		t.Run(name, func(t *testing.T) {
			p := benchmark.DefaultParams()
			tc.mutate(&p)
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

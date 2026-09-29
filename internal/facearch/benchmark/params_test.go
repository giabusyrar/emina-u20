package benchmark_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

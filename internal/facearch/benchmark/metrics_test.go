package benchmark_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
)

func repeat(label string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = label
	}
	return out
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Cohen's kappa on the textbook 2x2 table: 20 yes/yes, 5 yes/no, 10 no/yes,
// 15 no/no. po = 0.7, pe = 0.5*0.6 + 0.5*0.4 = 0.5, kappa = 0.4.
func TestCohenKappaKnownAnswer(t *testing.T) {
	a := concat(repeat("yes", 25), repeat("no", 25))
	b := concat(repeat("yes", 20), repeat("no", 5), repeat("yes", 10), repeat("no", 15))
	k := benchmark.CohenKappa(a, b)
	require.NotNil(t, k)
	assert.InDelta(t, 0.4, *k, 1e-12)
	assert.InDelta(t, 0.7, *benchmark.Agreement(a, b), 1e-12)

	same := benchmark.CohenKappa([]string{"x", "y", "x"}, []string{"x", "y", "x"})
	assert.InDelta(t, 1.0, *same, 1e-12)
	assert.Nil(t, benchmark.CohenKappa([]string{"x", "x"}, []string{"x", "x"}), "pe = 1: kappa undefined")
	assert.Nil(t, benchmark.CohenKappa(nil, nil))
}

// truth A A B B, pred A B B B: F1(A) = 2/3, F1(B) = 4/5, macro 11/15.
func TestMacroF1KnownAnswer(t *testing.T) {
	f := benchmark.MacroF1([]string{"A", "A", "B", "B"}, []string{"A", "B", "B", "B"})
	require.NotNil(t, f)
	assert.InDelta(t, 11.0/15.0, *f, 1e-12)
	// A label only we use counts with F1 0: truth A A, pred A C -> (2/3 + 0) / 2.
	f = benchmark.MacroF1([]string{"A", "A"}, []string{"A", "C"})
	assert.InDelta(t, 1.0/3.0, *f, 1e-12)
	assert.Nil(t, benchmark.MacroF1(nil, nil))
}

// Four answers at 0.95 with three right (gap 0.20), four at 0.25 with one
// right (gap 0): ECE = 4/8 * 0.20 + 4/8 * 0 = 0.10.
func TestECEKnownAnswer(t *testing.T) {
	conf := []float64{0.95, 0.95, 0.95, 0.95, 0.25, 0.25, 0.25, 0.25}
	correct := []bool{true, true, true, false, true, false, false, false}
	e := benchmark.ECE(conf, correct, 10)
	require.NotNil(t, e)
	assert.InDelta(t, 0.10, *e, 1e-12)
	// A confidence of exactly 1 lands in the last bin, not past it.
	e = benchmark.ECE([]float64{1, 1}, []bool{true, true}, 10)
	assert.InDelta(t, 0.0, *e, 1e-12)
	assert.Nil(t, benchmark.ECE(nil, nil, 10))
}

func TestConfusion(t *testing.T) {
	c := benchmark.NewConfusion([]string{"Oval", "Oval", "Round"}, []string{"Oval", "Round", "Square"})
	assert.Equal(t, []string{"Oval", "Round", "Square"}, c.Labels)
	assert.Equal(t, [][]int{{1, 1, 0}, {0, 0, 1}, {0, 0, 0}}, c.Counts)
}

func score(x float64) *float64 { return &x }

func TestCompareTop2AndScores(t *testing.T) {
	pairs := []benchmark.Pair{
		{Truth: "Oval", Pred: "Oval", Top2: []string{"Oval", "Oblong"}, Score: score(0.55)},
		{Truth: "Oblong", Pred: "Oval", Top2: []string{"Oval", "Oblong"}, Score: score(0.52)},
		{Truth: "Round", Pred: "Oval", Top2: []string{"Oval"}, Score: score(0.9)},
		{Truth: "Round", Pred: "Round", Top2: []string{"Round"}, Score: score(0.8)},
	}
	m := benchmark.Compare(pairs, 10)
	assert.Equal(t, 4, m.N)
	assert.InDelta(t, 0.50, *m.ExactAgreement, 1e-12)
	assert.InDelta(t, 0.75, *m.Top2Agreement, 1e-12, "the blend's secondary counts for top-2")
	require.NotNil(t, m.ECE)
	assert.Equal(t, 10, m.ECEBins)

	traitPairs := []benchmark.Pair{{Truth: "Broad", Pred: "Average", Top2: []string{"Average", "Broad"}}}
	m = benchmark.Compare(traitPairs, 10)
	assert.Nil(t, m.ECE, "traits have no scores")
	assert.InDelta(t, 1.0, *m.Top2Agreement, 1e-12, "a trait's boundary alternative counts for top-2")

	m = benchmark.Compare(nil, 10)
	assert.Equal(t, 0, m.N)
	assert.Nil(t, m.ExactAgreement)
	assert.Nil(t, m.Top2Agreement)
	assert.Nil(t, m.Kappa)
}

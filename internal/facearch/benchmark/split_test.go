package benchmark_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/benchmark"
)

func subjects(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("subject-%02d", i)
	}
	return out
}

func TestSplitSubjectsIsDeterministicAndDisjoint(t *testing.T) {
	s := subjects(20)
	a, err := benchmark.SplitSubjects(s, 0.2, 7)
	require.NoError(t, err)
	assert.Len(t, a.HeldOut, 4)
	assert.Len(t, a.Train, 16)
	train := map[string]bool{}
	for _, x := range a.Train {
		train[x] = true
	}
	for _, x := range a.HeldOut {
		assert.False(t, train[x], "%s on both sides", x)
	}

	// Order and repeats (several images per subject) change nothing.
	rev := append([]string{}, s...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	b, err := benchmark.SplitSubjects(append(rev, s...), 0.2, 7)
	require.NoError(t, err)
	assert.Equal(t, a, b)

	// Pinned: the ranking is SHA-256 based, so this holds on every Go
	// version (checked independently with Python's hashlib).
	assert.Equal(t, []string{"subject-04", "subject-06", "subject-15", "subject-16"}, a.HeldOut)
	other := mustSplit(t, s, 0.2, 8)
	assert.NotEqual(t, a.HeldOut, other.HeldOut, "another seed, another split")
}

func mustSplit(t *testing.T, s []string, f float64, seed uint64) benchmark.Split {
	t.Helper()
	sp, err := benchmark.SplitSubjects(s, f, seed)
	require.NoError(t, err)
	return sp
}

func TestSplitSubjectsBounds(t *testing.T) {
	sp := mustSplit(t, subjects(3), 0.01, 1)
	assert.Len(t, sp.HeldOut, 1, "at least one subject is held out")
	sp = mustSplit(t, subjects(3), 0.99, 1)
	assert.Len(t, sp.Train, 1, "at least one subject trains")

	_, err := benchmark.SplitSubjects([]string{"a", "a"}, 0.2, 1)
	assert.ErrorContains(t, err, "need at least 2 subjects")
	for _, f := range []float64{0, 1, -0.5} {
		_, err = benchmark.SplitSubjects(subjects(5), f, 1)
		assert.ErrorContains(t, err, "held-out fraction must be in (0, 1)")
	}
}

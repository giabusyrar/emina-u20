package classify_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func f(x float64) *float64 { return &x }

// obs builds a response: a float value is usable, "occluded" is occluded.
func obs(values map[string]any) mc.Index {
	r := &mc.Response{}
	for k, v := range values {
		switch v := v.(type) {
		case float64:
			r.Measurements = append(r.Measurements, mc.Measurement{Key: k, Value: mc.Value{Scalar: f(v)}, Visibility: mc.VisibilityUnknown})
		case string:
			r.Measurements = append(r.Measurements, mc.Measurement{Key: k, Visibility: v})
		}
	}
	return r.Index()
}

func term(c, s, w float64) profile.PrototypeTerm {
	return profile.PrototypeTerm{Centre: c, Spread: s, Weight: w}
}

// Two classes on one measurement: A at 0, B at 2, spread 1.
func twoClass() profile.ClassifierSpec {
	return profile.ClassifierSpec{
		Key: "shape", BlendMargin: 0.6, MinObservedWeight: 0.5,
		Classes: []profile.ClassSpec{
			{Label: "A", Requires: []string{"m"}, Prototype: map[string]profile.PrototypeTerm{"m": term(0, 1, 1)}},
			{Label: "B", Requires: []string{"m"}, Prototype: map[string]profile.PrototypeTerm{"m": term(2, 1, 1)}},
		},
	}
}

func TestSingleWhenTopReachesMargin(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 0.0}))
	// log scores 0 and -2: A = 1 / (1 + e^-2).
	want := 1 / (1 + math.Exp(-2))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Empty(t, r.Secondary)
	assert.InDelta(t, want, r.Scores["A"], 1e-12)
	assert.InDelta(t, 1-want, r.Scores["B"], 1e-12)
	assert.Empty(t, r.NotAssessable)
	require.NotNil(t, r.ObservedWeightShare)
	assert.Equal(t, 1.0, *r.ObservedWeightShare)
}

func TestBlendBelowMarginTieKeepsSpecOrder(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 1.0}))
	assert.Equal(t, classify.StatusBlend, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Equal(t, "B", r.Secondary)
	assert.InDelta(t, 0.5, r.Scores["A"], 1e-12)
}

func TestWeightsAndSpreadsEnterTheFormula(t *testing.T) {
	spec := twoClass()
	spec.Classes[0].Prototype["m"] = term(0, 2, 3) // z = 1/2, w = 3 -> log = -3/8
	spec.Classes[1].Prototype["m"] = term(2, 1, 1) // z = -1, w = 1 -> log = -1/2
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 1.0}))
	a, b := math.Exp(-3.0/8), math.Exp(-0.5)
	assert.InDelta(t, a/(a+b), r.Scores["A"], 1e-12)
}

func TestFarOffFaceDoesNotUnderflow(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 1e4}))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "B", r.Primary)
	assert.InDelta(t, 1.0, r.Scores["A"]+r.Scores["B"], 1e-12)
}

func TestRequiresOccludedMakesClassNotAssessable(t *testing.T) {
	spec := twoClass()
	spec.Classes = append(spec.Classes, profile.ClassSpec{
		Label: "C", Requires: []string{"hidden"},
		Prototype: map[string]profile.PrototypeTerm{"m": term(1, 1, 1), "hidden": term(0, 1, 1)},
	})
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0, "hidden": mc.VisibilityOccluded}))
	assert.Equal(t, []string{"C"}, r.NotAssessable)
	assert.NotContains(t, r.Scores, "C")
	assert.InDelta(t, 1.0, r.Scores["A"]+r.Scores["B"], 1e-12)
	assert.Equal(t, []string{"hidden"}, r.MissingMeasurements)
	assert.InDelta(t, 3.0/4.0, *r.ObservedWeightShare, 1e-12)
}

func TestNonRequiredMissingTermIsSkipped(t *testing.T) {
	spec := twoClass()
	spec.Classes[0].Prototype["extra"] = term(5, 1, 1)
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0}))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Equal(t, []string{"extra"}, r.MissingMeasurements)
}

func TestClassWithNoUsableTermIsNotAssessable(t *testing.T) {
	spec := twoClass()
	spec.Classes[1].Requires = nil
	spec.Classes[1].Prototype = map[string]profile.PrototypeTerm{"gone": term(0, 1, 1)}
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0}))
	assert.Equal(t, []string{"B"}, r.NotAssessable)
	assert.Equal(t, classify.StatusInsufficientEvidence, r.Status, "fewer than two assessable classes")
	assert.Empty(t, r.Primary)
	assert.Equal(t, map[string]float64{"A": 1}, r.Scores, "assessable scores are still reported")
}

func TestInsufficientWhenObservedShareBelowMinimum(t *testing.T) {
	spec := twoClass()
	for i := range spec.Classes {
		spec.Classes[i].Requires = nil
		spec.Classes[i].Prototype["n"] = term(0, 1, 3)
	}
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0, "n": mc.VisibilityOccluded}))
	assert.InDelta(t, 0.25, *r.ObservedWeightShare, 1e-12)
	assert.Equal(t, classify.StatusInsufficientEvidence, r.Status)
	assert.Equal(t, []string{"n"}, r.MissingMeasurements)
	assert.Len(t, r.Scores, 2)
}

func TestNotSupportedAndListValuesAreNotUsable(t *testing.T) {
	r := &mc.Response{Measurements: []mc.Measurement{
		{Key: "m", Visibility: mc.VisibilityNotSupported},
	}}
	res := classify.NewPrototype(twoClass()).Classify(r.Index())
	assert.Equal(t, classify.StatusInsufficientEvidence, res.Status)
	assert.Equal(t, []string{"A", "B"}, res.NotAssessable)

	r = &mc.Response{Measurements: []mc.Measurement{
		{Key: "m", Value: mc.Value{List: []float64{1, 2}}, Visibility: mc.VisibilityUnknown},
	}}
	res = classify.NewPrototype(twoClass()).Classify(r.Index())
	assert.Equal(t, []string{"A", "B"}, res.NotAssessable)
}

func TestUnavailable(t *testing.T) {
	r := classify.Unavailable("catalogue drift")
	assert.Equal(t, classify.StatusUnavailable, r.Status)
	assert.Equal(t, "catalogue drift", r.Reason)
	assert.NotNil(t, r.Scores)
	assert.NotNil(t, r.NotAssessable)
}

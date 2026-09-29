package traits_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

func f(x float64) *float64 { return &x }

func m(key string, value, lo, hi float64) mc.Measurement {
	return mc.Measurement{
		Key: key, Visibility: mc.VisibilityUnknown,
		Value: mc.Value{Scalar: f(value)},
		Band:  &[2]mc.Value{{Scalar: f(lo)}, {Scalar: f(hi)}},
	}
}

func index(ms ...mc.Measurement) mc.Index {
	return (&mc.Response{Measurements: ms}).Index()
}

func cond(key, op string, v float64) profile.Condition {
	return profile.Condition{Measurement: key, Op: op, Value: v}
}

// noseWidth-shaped trait: Broad above 1.1, Narrow below 0.9, else Average.
func width() profile.TraitSpec {
	return profile.TraitSpec{
		Key: "noseWidth", Default: "Average",
		Rules: []profile.RuleSpec{
			{Label: "Broad", When: []profile.Condition{cond("w", ">", 1.1)}},
			{Label: "Narrow", When: []profile.Condition{cond("w", "<", 0.9)}},
		},
	}
}

func TestFirstMatchWinsAndDefault(t *testing.T) {
	assert.Equal(t, "Broad", traits.Evaluate(width(), index(m("w", 1.3, 1.25, 1.35))).Label)
	assert.Equal(t, "Narrow", traits.Evaluate(width(), index(m("w", 0.8, 0.75, 0.85))).Label)
	r := traits.Evaluate(width(), index(m("w", 1.0, 0.95, 1.05)))
	assert.Equal(t, traits.StatusAssessed, r.Status)
	assert.Equal(t, "Average", r.Label)
	assert.False(t, r.BoundaryUncertain)
	assert.Empty(t, r.Alternative)
}

func TestRuleOrderMatters(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "First", When: []profile.Condition{cond("w", ">", 0)}},
		{Label: "Second", When: []profile.Condition{cond("w", ">", 1)}},
	}}
	assert.Equal(t, "First", traits.Evaluate(spec, index(m("w", 2, 1.9, 2.1))).Label)
}

func TestAllConditionsMustHold(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "Both", When: []profile.Condition{cond("a", ">", 1), cond("b", "<", 0)}},
	}}
	assert.Equal(t, "D", traits.Evaluate(spec, index(m("a", 2, 1.9, 2.1), m("b", 1, 0.9, 1.1))).Label)
	assert.Equal(t, "Both", traits.Evaluate(spec, index(m("a", 2, 1.9, 2.1), m("b", -1, -1.1, -0.9))).Label)
}

func TestBandStraddlingThresholdGivesAlternative(t *testing.T) {
	r := traits.Evaluate(width(), index(m("w", 1.15, 1.0, 1.3)))
	assert.Equal(t, "Broad", r.Label)
	assert.True(t, r.BoundaryUncertain)
	assert.Equal(t, "Average", r.Alternative)

	r = traits.Evaluate(width(), index(m("w", 1.0, 0.85, 1.05)))
	assert.Equal(t, "Average", r.Label)
	assert.True(t, r.BoundaryUncertain)
	assert.Equal(t, "Narrow", r.Alternative)
}

func TestStraddleThatCannotChangeTheLabelIsIgnored(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "A", When: []profile.Condition{cond("a", ">", 1)}},
		{Label: "B", When: []profile.Condition{cond("b", ">", 0), cond("c", ">", 5)}},
	}}
	// a is clearly true; b straddles but rule B is never reached.
	r := traits.Evaluate(spec, index(m("a", 3, 2.9, 3.1), m("b", 0.1, -0.1, 0.2), m("c", 9, 8, 10)))
	assert.Equal(t, "A", r.Label)
	assert.False(t, r.BoundaryUncertain)
}

// The worker sends "band": null when bands are disabled or too few perturbed
// samples were finite. That is no uncertainty information: the trait is
// assessed from the value, never flagged, and nothing panics.
func TestNullBandMeansNoUncertaintyInformation(t *testing.T) {
	r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityObserved, Value: mc.Value{Scalar: f(1.1)}}))
	assert.Equal(t, traits.StatusAssessed, r.Status)
	assert.Equal(t, "Average", r.Label)
	assert.False(t, r.BoundaryUncertain)
	assert.Empty(t, r.Alternative)

	r = traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityUnknown, Value: mc.Value{Scalar: f(1.3)}}))
	assert.Equal(t, "Broad", r.Label)
	assert.False(t, r.BoundaryUncertain)
}

// The worker's "degenerate geometry" (value null, visibility unknown) is
// not usable: the trait cannot be assessed.
func TestDegenerateGeometryIsNotAssessable(t *testing.T) {
	reason := "degenerate geometry"
	r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityUnknown, Reason: &reason}))
	assert.Equal(t, traits.StatusNotAssessable, r.Status)
	assert.Equal(t, []string{"w"}, r.MissingMeasurements)
}

func TestOccludedOrUnsupportedConditionIsNotAssessable(t *testing.T) {
	for _, vis := range []string{mc.VisibilityOccluded, mc.VisibilityNotSupported} {
		r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: vis}))
		assert.Equal(t, traits.StatusNotAssessable, r.Status, vis)
		assert.Empty(t, r.Label)
		assert.Equal(t, []string{"w"}, r.MissingMeasurements)
		assert.Equal(t, "requires w", r.Reason)
	}
}

func TestLaterRuleOnHiddenMeasurementStillBlocks(t *testing.T) {
	spec := width()
	spec.Rules = append(spec.Rules, profile.RuleSpec{Label: "X", When: []profile.Condition{cond("hidden", ">", 0)}})
	r := traits.Evaluate(spec, index(m("w", 1.3, 1.25, 1.35), mc.Measurement{Key: "hidden", Visibility: mc.VisibilityOccluded}))
	assert.Equal(t, traits.StatusNotAssessable, r.Status)
}

func TestUnavailable(t *testing.T) {
	r := traits.Unavailable("drift")
	assert.Equal(t, traits.StatusUnavailable, r.Status)
	assert.Equal(t, "drift", r.Reason)
}

package guidance_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

func shape(status, primary, secondary string) guidance.Evidence {
	return guidance.Evidence{
		Classifications: map[string]classify.Result{"faceShape": {Status: status, Primary: primary, Secondary: secondary}},
		Traits:          map[string]traits.Result{},
	}
}

func TestMatch_Classifier(t *testing.T) {
	round := profile.GuidanceRule{ID: "r", When: map[string][]string{"faceShape": {"Round"}}}
	cases := []struct {
		name string
		ev   guidance.Evidence
		want guidance.Grade
	}{
		{"single primary", shape(classify.StatusSingle, "Round", ""), guidance.MatchPrimary},
		{"single other", shape(classify.StatusSingle, "Oval", ""), guidance.NoMatch},
		{"blend primary", shape(classify.StatusBlend, "Round", "Oval"), guidance.MatchPrimary},
		{"blend secondary", shape(classify.StatusBlend, "Oval", "Round"), guidance.MatchSecondary},
		{"blend neither", shape(classify.StatusBlend, "Oval", "Heart"), guidance.NoMatch},
		{"single ignores a stale secondary", shape(classify.StatusSingle, "Oval", "Round"), guidance.NoMatch},
		{"insufficient evidence", shape(classify.StatusInsufficientEvidence, "", ""), guidance.NoMatch},
		{"insufficient evidence with a stray primary", shape(classify.StatusInsufficientEvidence, "Round", ""), guidance.NoMatch},
		{"unavailable", shape(classify.StatusUnavailable, "Round", ""), guidance.NoMatch},
		{"no result", guidance.Evidence{}, guidance.NoMatch},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, guidance.Match(round, tc.ev), tc.name)
	}
}

func TestMatch_Trait(t *testing.T) {
	r := profile.GuidanceRule{ID: "r", When: map[string][]string{"eyebrowShape": {"Straight"}}}
	ev := func(res traits.Result) guidance.Evidence {
		return guidance.Evidence{Traits: map[string]traits.Result{"eyebrowShape": res}}
	}
	assert.Equal(t, guidance.MatchPrimary, guidance.Match(r, ev(traits.Result{Status: traits.StatusAssessed, Label: "Straight"})))
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusAssessed, Label: "Rounded", BoundaryUncertain: true, Alternative: "Straight"})),
		"the alternative label is not the label")
	assert.Equal(t, guidance.MatchSecondary, guidance.Match(r, ev(traits.Result{Status: traits.StatusAssessed, Label: "Straight", BoundaryUncertain: true, Alternative: "Rounded"})),
		"an uncertain label matches at secondary grade")
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusNotAssessable})))
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusUnavailable, Label: "Straight"})))
}

func TestMatch_AllEntriesMustMatch_WeakestGradeWins(t *testing.T) {
	r := profile.GuidanceRule{ID: "r", When: map[string][]string{"faceShape": {"Round"}, "eyebrowShape": {"Straight"}}}
	ev := shape(classify.StatusBlend, "Oval", "Round")
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Straight"}
	assert.Equal(t, guidance.MatchSecondary, guidance.Match(r, ev))
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Rounded"}
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev))
	assert.Equal(t, guidance.NoMatch, guidance.Match(profile.GuidanceRule{ID: "empty"}, ev), "a rule with no When never matches")
}

func TestMatch_UncertainTraitWeakensRule(t *testing.T) {
	r := profile.GuidanceRule{ID: "r", When: map[string][]string{"faceShape": {"Round"}, "eyebrowShape": {"Straight"}}}
	ev := shape(classify.StatusSingle, "Round", "")
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Straight", BoundaryUncertain: true, Alternative: "Rounded"}
	assert.Equal(t, guidance.MatchSecondary, guidance.Match(r, ev))
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Straight"}
	assert.Equal(t, guidance.MatchPrimary, guidance.Match(r, ev))
}

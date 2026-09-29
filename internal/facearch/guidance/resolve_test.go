package guidance_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// testGuidance's numbers are test inputs, not proposed values.
const testGuidance = `{
  "templates": {
    "tip":  {"role": "highlight", "points": [{"index": 1}, {"lerp": [1, 2, 0.5]}, {"index": 2, "offsetIod": [0, 0.5]}]},
    "jaw":  {"role": "contour",   "points": [{"index": 172}, {"index": 136}, {"lerp": [150, 1, 0.2]}]},
    "brow": {"role": "brow",      "points": [{"index": 70}, {"index": 63}, {"index": 105}]}
  },
  "rules": [
    {"id": "a", "when": {"faceShape": ["Oval"]}, "apply": [{"template": "tip", "intensity": 0.5}, {"template": "jaw", "intensity": 0.6}], "textKeys": ["k.a"]},
    {"id": "b", "when": {"faceShape": ["Round"]}, "apply": [{"template": "brow", "intensity": 0.3}, {"template": "tip", "intensity": 0.9}], "textKeys": ["k.b"], "placeholder": true}
  ]
}`

func testSpec(t *testing.T) *profile.Guidance {
	t.Helper()
	var g profile.Guidance
	require.NoError(t, json.Unmarshal([]byte(testGuidance), &g))
	return &g
}

func fixture() ([][]float64, map[string]string) {
	m := facearchtest.FixtureFace()
	return m.Landmarks, m.Regions
}

func templates(res *guidance.Result) (placed []string, dropped []string) {
	for _, r := range res.Regions {
		placed = append(placed, r.RuleID+":"+r.Template)
	}
	for _, d := range res.DroppedTemplates {
		dropped = append(dropped, d.RuleID+":"+d.Template+": "+d.Reason)
	}
	return placed, dropped
}

// On the recorded face (jaw out of frame, so "occluded"), a jaw-anchored
// template is dropped with the reason and the rest is placed in pixels.
func TestResolve_FixtureSingle(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	placed, dropped := templates(res)
	assert.Equal(t, []string{"a:tip"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)

	tip := res.Regions[0]
	assert.Equal(t, profile.RoleHighlight, tip.Role)
	assert.Equal(t, 0.5, tip.Intensity)
	assert.Equal(t, []string{"k.a"}, tip.TextKeys)
	assert.False(t, tip.Placeholder)
	require.Len(t, tip.Polygon, 3)
	// An index point is the landmark itself.
	assert.InDelta(t, l[1][0], tip.Polygon[0][0], 1e-9)
	assert.InDelta(t, l[1][1], tip.Polygon[0][1], 1e-9)
	// A lerp is the same point in pixels as in the frame (affine map).
	assert.InDelta(t, (l[1][0]+l[2][0])/2, tip.Polygon[1][0], 1e-9)
	assert.InDelta(t, (l[1][1]+l[2][1])/2, tip.Polygon[1][1], 1e-9)
	// offsetIod [0, 0.5] moves half an IOD down the face, perpendicular to
	// the eye line (the fixture's roll is about -1.3 degrees).
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	dx, dy := tip.Polygon[2][0]-l[2][0], tip.Polygon[2][1]-l[2][1]
	assert.InDelta(t, 0.5*f.IodPx(), math.Hypot(dx, dy), 1e-9)
	c := f.ToCanonical(tip.Polygon[2])
	c2 := f.ToCanonical([2]float64{l[2][0], l[2][1]})
	assert.InDelta(t, c2[0], c[0], 1e-9)
	assert.InDelta(t, c2[1]+0.5, c[1], 1e-9)
}

// Primary-matched rules apply before secondary-matched ones; each template
// is placed once, by the first rule that applies it.
func TestResolve_BlendOrderAndDedup(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusBlend, "Oval", "Round"), l, regions)
	placed, dropped := templates(res)
	assert.Equal(t, []string{"a:tip", "b:brow"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)

	res = guidance.Resolve(testSpec(t), shape(classify.StatusBlend, "Round", "Oval"), l, regions)
	placed, dropped = templates(res)
	assert.Equal(t, []string{"b:brow", "b:tip"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)
	assert.Equal(t, 0.9, res.Regions[1].Intensity)
	assert.True(t, res.Regions[1].Placeholder, "a placeholder rule's regions say so")
}

func TestResolve_NothingMatches(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusInsufficientEvidence, "", ""), l, regions)
	b, err := json.Marshal(res)
	require.NoError(t, err)
	assert.JSONEq(t, `{"regions": [], "droppedTemplates": []}`, string(b))
}

func TestResolve_EyesOccludedDropsEverything(t *testing.T) {
	l, regions := fixture()
	regions["eyes"] = "occluded"
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	placed, dropped := templates(res)
	assert.Empty(t, placed)
	assert.Equal(t, []string{
		"a:tip: canonical frame requires region eyes",
		"a:jaw: canonical frame requires region eyes",
	}, dropped)
}

func TestResolve_BadLandmarksAndRegions(t *testing.T) {
	_, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), nil, regions)
	_, dropped := templates(res)
	assert.Equal(t, []string{"a:tip: the ai-worker returned no landmarks", "a:jaw: the ai-worker returned no landmarks"}, dropped)

	l, _ := fixture()
	res = guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l[:100], regions)
	_, dropped = templates(res)
	assert.Equal(t, "a:tip: landmarks unusable: want 478 landmarks, got 100", dropped[0])

	l, regions = fixture()
	delete(regions, "nose")
	res = guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	_, dropped = templates(res)
	assert.Equal(t, "a:tip: region nose not reported by the ai-worker (anchor landmark 1)", dropped[0])
}

func TestResolve_UndefinedTemplateIsDroppedNotFatal(t *testing.T) {
	l, regions := fixture()
	g := testSpec(t)
	g.Rules[0].Apply = append(g.Rules[0].Apply, profile.Application{Template: "gone", Intensity: 0.5})
	res := guidance.Resolve(g, shape(classify.StatusSingle, "Oval", ""), l, regions)
	_, dropped := templates(res)
	assert.Contains(t, dropped, "a:gone: template is not defined")
}

func TestPlace_RejectsMissingLandmark(t *testing.T) {
	l, _ := fixture()
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	big := 900
	_, err = guidance.Place(profile.Template{Role: "contour", Points: []profile.Point{{Index: &big}}}, f, l)
	assert.ErrorContains(t, err, "landmark 900 is not available")
}

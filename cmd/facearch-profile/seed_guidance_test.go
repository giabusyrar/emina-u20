package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

var faceShapes = []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}

// sourcedRoles is what Perfect Corp's page gives per shape (contour and
// highlight for Round, Oval, Square and Heart; Round has no highlight). The
// seed ships only these; every other shape and role gets no guidance.
var sourcedRoles = map[string]map[string]bool{
	"Round":  {"contour": true},
	"Oval":   {"contour": true, "highlight": true},
	"Square": {"contour": true, "highlight": true},
	"Heart":  {"contour": true, "highlight": true},
}

// The seed ships sourced placements only: no rule is a placeholder and no
// cited source says "placeholder" except for the intensities, which every
// rule's source flags. Each rule's text key and templates share one role.
func TestSeedGuidanceIsSourcedOnly(t *testing.T) {
	_, d := loadSeed(t)
	g := d.Guidance
	require.NotNil(t, g)
	sources := map[string]profile.Source{}
	for _, s := range d.Calibration.Sources {
		sources[s.For] = s
	}
	roles := map[string]map[string]bool{}
	for _, r := range g.Rules {
		src, ok := sources["guidance.rules."+r.ID]
		require.True(t, ok, r.ID)
		assert.False(t, r.Placeholder, "rule %s: the seed ships sourced placements only", r.ID)
		assert.False(t, strings.HasPrefix(src.Cite, "placeholder"), r.ID)
		assert.Contains(t, src.Note, "is a placeholder pending benchmark", "rule %s: its intensity is flagged", r.ID)
		require.Len(t, r.TextKeys, 1, r.ID)
		parts := strings.Split(r.TextKeys[0], ".")
		require.Len(t, parts, 4, r.TextKeys[0])
		assert.Equal(t, []string{"facearch", "guidance"}, parts[:2], r.TextKeys[0])
		role := parts[3]
		for _, a := range r.Apply {
			assert.Equal(t, role, g.Templates[a.Template].Role, "rule %s template %s", r.ID, a.Template)
		}
		for _, shape := range r.When["faceShape"] {
			assert.Equal(t, strings.ToLower(shape), parts[2], r.ID)
			if roles[shape] == nil {
				roles[shape] = map[string]bool{}
			}
			roles[shape][role] = true
		}
	}
	assert.Equal(t, sourcedRoles, roles)
	for name, tpl := range g.Templates {
		src, ok := sources["guidance.templates."+name]
		require.True(t, ok, name)
		assert.False(t, strings.HasPrefix(src.Cite, "placeholder"), name)
		assert.Contains(t, []string{profile.RoleContour, profile.RoleHighlight}, tpl.Role, name)
	}
}

// A face classified into a shape the source does not cover gets no
// regions and nothing dropped: honest absence, not a guess. A Round face
// gets contour only.
func TestSeedNoGuidanceForUncoveredShapes(t *testing.T) {
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	m := facearchtest.FixtureFace()
	for _, shape := range faceShapes {
		ev := guidance.Evidence{
			Classifications: map[string]classify.Result{"faceShape": {Status: classify.StatusSingle, Primary: shape}},
			Traits:          map[string]traits.Result{},
		}
		res := guidance.Resolve(d.Guidance, ev, m.Landmarks, m.Regions)
		got := map[string]bool{}
		for _, r := range res.Regions {
			got[r.Role] = true
		}
		for _, x := range res.DroppedTemplates {
			got[d.Guidance.Templates[x.Template].Role] = true
		}
		want := sourcedRoles[shape]
		if want == nil {
			assert.Empty(t, res.Regions, shape)
			assert.Empty(t, res.DroppedTemplates, shape)
			continue
		}
		assert.Equal(t, want, got, shape)
	}
}

// On the recorded face the face shape is insufficient_evidence (chin out of
// frame), so no rule matches: guidance is present and empty, never guessed.
func TestSeedGuidanceOnFixtureFace(t *testing.T) {
	p, d := loadSeed(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	require.NotNil(t, out.Guidance)
	assert.Empty(t, out.Guidance.Regions)
	assert.Empty(t, out.Guidance.DroppedTemplates)
}

// The synthetic in-frame measurements of TestSeedClassifiesAnInFrameFace
// (blend Oval / Round) on the fixture's landmarks, whose jaw region is still
// reported occluded. Oval's rules apply first, then Round's through the
// secondary; jaw-anchored templates are dropped with the reason.
func TestSeedGuidanceOnSyntheticBlend(t *testing.T) {
	p, d := loadSeed(t)
	m := facearchtest.FixtureFace()
	set := map[string]float64{"width_length_ratio": 0.8521, "jaw_to_mid_width_ratio": 0.8326, "chin_taper": 0.4534}
	for i := range m.Measurements {
		if v, ok := set[m.Measurements[i].Key]; ok {
			v := v
			m.Measurements[i].Value = measureclient.Value{Scalar: &v}
			m.Measurements[i].Visibility = measureclient.VisibilityUnknown
			m.Measurements[i].Band = nil
		}
	}
	out := facearch.Interpret(p, d, m)
	require.NotNil(t, out.Guidance)
	fs := out.Classifications["faceShape"]
	require.Equal(t, classify.StatusBlend, fs.Status)
	require.Equal(t, "Oval", fs.Primary)
	require.Equal(t, "Round", fs.Secondary)

	var placed []string
	for _, r := range out.Guidance.Regions {
		placed = append(placed, r.RuleID+":"+r.Template)
		assert.False(t, r.Placeholder, r.RuleID)
	}
	assert.Equal(t, []string{
		"oval-contour:contour_forehead_side_r", "oval-contour:contour_forehead_side_l",
		"oval-contour:contour_cheek_hollow_r", "oval-contour:contour_cheek_hollow_l",
		"oval-highlight:highlight_forehead_centre", "oval-highlight:highlight_nose_bridge",
	}, placed, "Round adds only its jaw contour, and that is dropped")
	assert.Equal(t, []guidance.Dropped{
		{Template: "highlight_chin", RuleID: "oval-highlight", Reason: "region jaw is occluded (anchor landmark 152)"},
		{Template: "contour_jaw_r", RuleID: "round-contour", Reason: "region jaw is occluded (anchor landmark 58)"},
		{Template: "contour_jaw_l", RuleID: "round-contour", Reason: "region jaw is occluded (anchor landmark 288)"},
	}, out.Guidance.DroppedTemplates)
}

// allTemplates is the seed's guidance with one rule applying every
// template, so each can be resolved on the fixture.
func allTemplates(t *testing.T) (*profile.Guidance, guidance.Evidence) {
	t.Helper()
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	rule := profile.GuidanceRule{ID: "all", When: map[string][]string{"faceShape": {"Oval"}}, TextKeys: []string{"k"}}
	for _, name := range d.Guidance.TemplateNames() {
		rule.Apply = append(rule.Apply, profile.Application{Template: name, Intensity: 1})
	}
	g := &profile.Guidance{Templates: d.Guidance.Templates, Rules: []profile.GuidanceRule{rule}}
	ev := guidance.Evidence{
		Classifications: map[string]classify.Result{"faceShape": {Status: classify.StatusSingle, Primary: "Oval"}},
		Traits:          map[string]traits.Result{},
	}
	return g, ev
}

// The fixture's chin is out of frame (jaw "occluded"): exactly the
// templates with a jaw anchor are dropped, and the rest are placed.
func TestSeedJawAnchoredTemplatesDropOnFixture(t *testing.T) {
	g, ev := allTemplates(t)
	m := facearchtest.FixtureFace()
	res := guidance.Resolve(g, ev, m.Landmarks, m.Regions)
	var dropped []string
	for _, x := range res.DroppedTemplates {
		dropped = append(dropped, x.Template)
		assert.Contains(t, x.Reason, "region jaw is occluded")
	}
	assert.Equal(t, []string{"contour_jaw_l", "contour_jaw_r", "highlight_chin"}, dropped)
	assert.Len(t, res.Regions, len(g.Templates)-3)
	for name, tpl := range g.Templates {
		usesJaw := false
		for _, pt := range tpl.Points {
			for _, i := range pt.Anchors() {
				for _, r := range measureclient.RegionsOf(i) {
					usesJaw = usesJaw || r == "jaw"
				}
			}
		}
		assert.Equal(t, usesJaw, contains(dropped, name), name)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Every seed template lands where its name says, on the recorded face, in
// the canonical frame (+x toward the subject's left, +y down, IOD units;
// subject's right = _r = negative x). All regions are treated as visible
// here so the jaw templates can be placed on the landmarker's extrapolated
// chin. These are sanity bounds on uncalibrated geometry, not accuracy.
func TestSeedTemplatesLandWhereTheirNamesSay(t *testing.T) {
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	l := facearchtest.FixtureFace().Landmarks
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	lm := func(i int) [2]float64 { return f.ToCanonical([2]float64{l[i][0], l[i][1]}) }
	verts := func(name string) [][2]float64 {
		tpl, ok := d.Guidance.Templates[name]
		require.True(t, ok, name)
		poly, err := guidance.Place(tpl, f, l)
		require.NoError(t, err, name)
		out := make([][2]float64, len(poly))
		for i, p := range poly {
			out[i] = f.ToCanonical(p)
		}
		return out
	}
	abs := func(x float64) float64 {
		if x < 0 {
			return -x
		}
		return x
	}
	each := func(name string, ok func(v [2]float64) bool, what string) {
		for i, v := range verts(name) {
			assert.True(t, ok(v), "%s vertex %d at (%.3f, %.3f): %s", name, i, v[0], v[1], what)
		}
	}
	centroidY := func(name string) float64 {
		var y float64
		vs := verts(name)
		for _, v := range vs {
			y += v[1]
		}
		return y / float64(len(vs))
	}

	for _, side := range []struct {
		s                                  string
		sign                               float64
		oval, alar, mouth, browEnd, browIn int
		endo                               int
	}{
		{"r", -1, 234, 98, 61, 70, 107, 133},
		{"l", 1, 454, 327, 291, 300, 336, 362},
	} {
		onSide := func(v [2]float64) bool { return v[0]*side.sign > 0 }

		hollow := "contour_cheek_hollow_" + side.s
		each(hollow, onSide, "on its own side")
		each(hollow, func(v [2]float64) bool { return v[1] > lm(side.oval)[1] }, "below the cheekbone line (the oval extreme's height)")
		each(hollow, func(v [2]float64) bool { return v[1] < lm(side.mouth)[1] }, "above the mouth corner")
		each(hollow, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.alar)[0]) }, "outside the nose (lateral to the alar base)")

		cheekbone := "highlight_cheekbone_" + side.s
		each(cheekbone, onSide, "on its own side")
		each(cheekbone, func(v [2]float64) bool { return v[1] > 0 && v[1] < lm(1)[1] }, "between the eye line and the nose tip")
		each(cheekbone, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.endo)[0]) }, "lateral to the inner eye corner")
		assert.Less(t, centroidY(cheekbone), centroidY(hollow), "the cheekbone highlight sits above the hollow contour")

		forehead := "contour_forehead_side_" + side.s
		each(forehead, onSide, "on its own side")
		each(forehead, func(v [2]float64) bool { return v[1] < lm(side.browEnd)[1] }, "above the brow tail")
		each(forehead, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.browIn)[0]) }, "lateral to the brow head")

		jaw := "contour_jaw_" + side.s
		each(jaw, onSide, "on its own side")
		each(jaw, func(v [2]float64) bool { return v[1] > lm(1)[1] }, "below the nose tip")
		each(jaw, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.mouth)[0]) }, "lateral to the mouth corner")

	}

	each("highlight_forehead_centre", func(v [2]float64) bool {
		return v[1] < lm(9)[1] && abs(v[0]) < abs(lm(107)[0]) && abs(v[0]) < abs(lm(336)[0])
	}, "above the brows, between the brow heads")
	each("highlight_nose_bridge", func(v [2]float64) bool {
		return v[1] >= lm(168)[1]-1e-9 && v[1] < lm(1)[1] && abs(v[0]) < abs(lm(98)[0])
	}, "on the nose between 168 and the tip, inside the alar width")
	each("highlight_chin", func(v [2]float64) bool {
		return v[1] > lm(17)[1] && abs(v[0]) < abs(lm(61)[0])
	}, "below the lower lip, inside the mouth width")
}

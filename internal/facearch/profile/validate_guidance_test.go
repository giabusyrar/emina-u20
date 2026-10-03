package profile_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
)

func TestValidate_GuidanceValid(t *testing.T) {
	assert.NoError(t, profile.Validate(withGuidance(t), facearchtest.Catalogue()))
	assert.NoError(t, profile.Validate(withGuidance(t), nil))
}

func TestValidate_GuidanceRules(t *testing.T) {
	idx := func(i int) *int { return &i }
	cases := map[string]struct {
		mutate func(g *profile.Guidance)
		want   string
	}{
		"unknown role": {func(g *profile.Guidance) {
			tp := g.Templates["cheek_r"]
			tp.Role = "bronzer"
			g.Templates["cheek_r"] = tp
		},
			`role: must be one of contour, highlight, blush, brow, got "bronzer"`},
		"too few points": {func(g *profile.Guidance) {
			tp := g.Templates["cheek_r"]
			tp.Points = tp.Points[:2]
			g.Templates["cheek_r"] = tp
		},
			"a polygon needs at least 3 points, got 2"},
		"index above 477": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(478) },
			"points[0].index: landmark 478 is outside 0-477"},
		"negative index": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(-1) },
			"landmark -1 is outside 0-477"},
		"lerp a above 477": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.A = 478 },
			"points[1].lerp[0]: landmark 478 is outside 0-477"},
		"lerp b negative": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.B = -3 },
			"points[1].lerp[1]: landmark -3 is outside 0-477"},
		"index in no region": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(50) },
			"landmark 50 is in no ai-worker region"},
		"lerp end in no region": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.B = 205 },
			"points[1].lerp[1]: landmark 205 is in no ai-worker region"},
		"lerp t above 1": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.T = 1.2 },
			"points[1].lerp[2]: t must be in [0, 1], got 1.2"},
		"lerp t negative": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.T = -0.1 },
			"t must be in [0, 1], got -0.1"},
		"lerp same ends": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.B = 234 },
			"landmarks a and b are both 234"},
		"index and lerp": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Index = idx(234) },
			"points[1]: has both index and lerp"},
		"neither": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = nil },
			"points[0]: needs index or lerp"},
		"offset not finite": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[2].OffsetIod[1] = math.Inf(1) },
			"points[2].offsetIod: must be finite"},
		"no rules": {func(g *profile.Guidance) { g.Rules = nil }, "guidance.rules: needs at least 1 rule"},
		"duplicate rule id": {func(g *profile.Guidance) { g.Rules = append(g.Rules, g.Rules[0]) },
			`guidance.rules[1].id: duplicate rule "long-contour"`},
		"unknown when key": {func(g *profile.Guidance) { g.Rules[0].When["lipShape"] = []string{"Full"} },
			`when.lipShape: no classifier or trait "lipShape"`},
		"unknown class label": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = []string{"Oval"} },
			`"faceShape" cannot produce label "Oval" (labels: Long, Wide)`},
		"unknown trait label": {func(g *profile.Guidance) { g.Rules[0].When["noseWidth"] = []string{"Hooked"} },
			`"noseWidth" cannot produce label "Hooked" (labels: Average, Broad)`},
		"empty labels": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = nil },
			"when.faceShape: needs at least 1 label"},
		"empty when": {func(g *profile.Guidance) { g.Rules[0].When = nil },
			"when: needs at least 1 classifier or trait"},
		"duplicate label": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = []string{"Long", "Long"} },
			`duplicate label "Long"`},
		"undefined template": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Template = "chin" },
			`apply[0].template: no template "chin"`},
		"template twice": {func(g *profile.Guidance) { g.Rules[0].Apply = append(g.Rules[0].Apply, g.Rules[0].Apply[0]) },
			`template "cheek_r" applied twice`},
		"zero intensity": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Intensity = 0 },
			"apply[0].intensity: must be in (0, 1], got 0"},
		"intensity above 1": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Intensity = 1.5 },
			"must be in (0, 1], got 1.5"},
		"no text keys": {func(g *profile.Guidance) { g.Rules[0].TextKeys = nil },
			"textKeys: needs at least 1 i18n key"},
		"dotted template name": {func(g *profile.Guidance) {
			g.Templates["cheek.r"] = g.Templates["cheek_r"]
		},
			`guidance.templates.cheek.r: template name must not contain "."`},
		"dotted rule id": {func(g *profile.Guidance) { g.Rules[0].ID = "long.contour" },
			`guidance.rules[0].id: rule id must not contain "."`},
		"prose text key": {func(g *profile.Guidance) { g.Rules[0].TextKeys = []string{"Contour under the cheekbones"} },
			"must be a non-empty i18n key without whitespace"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := withGuidance(t)
			tc.mutate(d.Guidance)
			msg := problems(t, profile.Validate(d, facearchtest.Catalogue()))
			assert.Contains(t, msg, tc.want)
		})
	}
}

func TestValidate_GuidanceKeyBothClassifierAndTrait(t *testing.T) {
	d := withGuidance(t)
	d.Traits = append(d.Traits, profile.TraitSpec{Key: "faceShape", Default: "Long",
		Rules: []profile.RuleSpec{{Label: "Wide", When: []profile.Condition{{Measurement: "width_length_ratio", Op: ">", Value: 1}}}}})
	d.Calibration.Sources = append(d.Calibration.Sources, profile.Source{Cite: "test", For: "traits.faceShape"})
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, `when.faceShape: "faceShape" is both a classifier and a trait`)
}

// Decision 13, extended: an uncalibrated document cites every guidance
// template and rule.
func TestValidate_GuidanceSourceCoverage(t *testing.T) {
	d := withGuidance(t)
	d.Calibration.Sources = d.Calibration.Sources[:5]
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "no source cited for guidance.templates.cheek_r")
	assert.Contains(t, msg, "no source cited for guidance.rules.long-contour")

	d.Calibration.Sources = append(d.Calibration.Sources, profile.Source{Cite: "vocabulary only", For: "guidance"})
	msg = problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "no source cited for guidance.templates.cheek_r", "a parent path covers nothing")

	runID, target := "run-1", "perfectcorp"
	d.Calibration.Status, d.Calibration.BenchmarkRunID, d.Calibration.Target = profile.CalibrationCalibrated, &runID, &target
	assert.NoError(t, profile.Validate(d, nil), "calibrated profiles are not subject to coverage")
}

func TestLoad_ChecksGuidanceStructure(t *testing.T) {
	d := withGuidance(t)
	d.Guidance.Templates["cheek_r"].Points[0].Index = nil
	b, err := json.Marshal(d)
	require.NoError(t, err)
	_, err = profile.Load(&profile.FaceArchitectureProfile{Document: string(b)})
	assert.ErrorContains(t, err, "needs index or lerp")
}

// The worker region table core-engine validates anchors against is recorded
// for one catalogue version; a guidance section for another is refused.
func TestValidate_GuidanceCatalogueVersion(t *testing.T) {
	d := withGuidance(t)
	d.CatalogueVersion = "fa-measure/2"
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "guidance: the ai-worker region table is recorded for fa-measure/1, document targets fa-measure/2")

	d.Guidance = nil
	err := profile.Validate(d, nil)
	if err != nil {
		assert.NotContains(t, err.Error(), "region table", "no guidance section, nothing to refuse")
	}
}

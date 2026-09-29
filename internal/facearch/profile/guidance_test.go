package profile_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// guidanceJSON is a small valid guidance section for validDoc. Its numbers
// are test inputs, not proposed values.
const guidanceJSON = `{
  "templates": {
    "cheek_r": {"role": "contour", "points": [
      {"index": 234}, {"lerp": [234, 61, 0.5]}, {"lerp": [93, 61, 0.35], "offsetIod": [0.0, -0.05]}]}
  },
  "rules": [
    {"id": "long-contour", "when": {"faceShape": ["Long"], "noseWidth": ["Broad", "Average"]},
     "apply": [{"template": "cheek_r", "intensity": 0.5}],
     "textKeys": ["facearch.guidance.long.contour"], "placeholder": true}
  ]
}`

// withGuidance returns validDoc with guidanceJSON and the sources it needs.
func withGuidance(t *testing.T) *profile.Document {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validDoc), &raw))
	var g any
	require.NoError(t, json.Unmarshal([]byte(guidanceJSON), &g))
	raw["guidance"] = g
	cal := raw["calibration"].(map[string]any)
	cal["sources"] = append(cal["sources"].([]any),
		map[string]any{"cite": "test", "for": "guidance.templates.cheek_r"},
		map[string]any{"cite": "test", "for": "guidance.rules.long-contour"})
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	d, err := profile.Parse(b)
	require.NoError(t, err)
	return d
}

func TestParse_Guidance(t *testing.T) {
	d := withGuidance(t)
	tpl := d.Guidance.Templates["cheek_r"]
	assert.Equal(t, profile.RoleContour, tpl.Role)
	require.Len(t, tpl.Points, 3)
	assert.Equal(t, 234, *tpl.Points[0].Index)
	assert.Nil(t, tpl.Points[0].Lerp)
	assert.Equal(t, profile.Lerp{A: 93, B: 61, T: 0.35}, *tpl.Points[2].Lerp)
	assert.Equal(t, [2]float64{0, -0.05}, *tpl.Points[2].OffsetIod)
	assert.Equal(t, []int{93, 61}, tpl.Points[2].Anchors())
	assert.Equal(t, []int{234}, tpl.Points[0].Anchors())
	r := d.Guidance.Rules[0]
	assert.Equal(t, []string{"faceShape", "noseWidth"}, r.WhenKeys())
	assert.True(t, r.Placeholder)

	b, err := json.Marshal(d.Guidance)
	require.NoError(t, err)
	assert.JSONEq(t, guidanceJSON, string(b), "guidance round-trips")
}

func TestParse_GuidanceIsStrict(t *testing.T) {
	for name, bad := range map[string]string{
		"misspelt offset":   strings.Replace(guidanceJSON, `"offsetIod"`, `"offset"`, 1),
		"fractional index":  strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234.5, 61, 0.5]`, 1),
		"exponent index":    strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[2e2, 61, 0.5]`, 1),
		"two-element lerp":  strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234, 61]`, 1),
		"string t":          strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234, 61, "half"]`, 1),
		"misspelt rule key": strings.Replace(guidanceJSON, `"textKeys"`, `"textKey"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, guidanceJSON, bad)
			doc := strings.Replace(validDoc, `"traits": [{`, `"guidance": `+bad+`, "traits": [{`, 1)
			_, err := profile.Parse([]byte(doc))
			assert.Error(t, err)
		})
	}
}

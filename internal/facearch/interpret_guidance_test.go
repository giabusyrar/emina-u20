package facearch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
)

// testGuidance's numbers are test inputs, not proposed values. The trait
// rule matches the fixture (noseWidth is Broad there); the classifier rule
// cannot (faceShape is insufficient_evidence: the chin is out of frame).
const testGuidance = `{
  "templates": {
    "nose": {"role": "highlight", "points": [{"index": 168}, {"lerp": [6, 1, 0.5]}, {"index": 1}]},
    "jaw":  {"role": "contour",   "points": [{"index": 172}, {"index": 136}, {"index": 150}]}
  },
  "rules": [
    {"id": "broad-nose", "when": {"noseWidth": ["Broad"]}, "apply": [{"template": "nose", "intensity": 0.4}], "textKeys": ["k.nose"]},
    {"id": "long-jaw", "when": {"faceShape": ["Long"]}, "apply": [{"template": "jaw", "intensity": 0.6}], "textKeys": ["k.jaw"]}
  ]
}`

func TestInterpret_Guidance(t *testing.T) {
	p, d := testProfile(t)
	var g profile.Guidance
	require.NoError(t, json.Unmarshal([]byte(testGuidance), &g))
	d.Guidance = &g

	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	require.NotNil(t, out.Guidance)
	require.Len(t, out.Guidance.Regions, 1)
	r := out.Guidance.Regions[0]
	assert.Equal(t, "nose", r.Template)
	assert.Equal(t, "broad-nose", r.RuleID)
	assert.Len(t, r.Polygon, 3)
	assert.Empty(t, out.Guidance.DroppedTemplates, "long-jaw never matched, so nothing was dropped")

	b, err := json.Marshal(out)
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &body))
	var gj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body["guidance"], &gj))
	assert.Contains(t, gj, "regions")
	assert.JSONEq(t, `[]`, string(gj["droppedTemplates"]))
}

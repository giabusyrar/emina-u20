package profile_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// validDoc is a small, complete document. Its numbers are test inputs, not
// proposed values.
const validDoc = `{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {
    "status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide.prototype.width_length_ratio"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]
  },
  "measurementsShown": ["width_length_ratio", "fifths"],
  "classifiers": [{
    "key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5,
    "classes": [
      {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
      {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}
    ]
  }],
  "traits": [{
    "key": "noseWidth",
    "rules": [{"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]}],
    "default": "Average"
  }]
}`

func parseValid(t *testing.T) *profile.Document {
	t.Helper()
	d, err := profile.Parse([]byte(validDoc))
	require.NoError(t, err)
	return d
}

func TestNormalizeStatus(t *testing.T) {
	for in, want := range map[string]string{"": "ACTIVE", "active": "ACTIVE", " Inactive ": "INACTIVE"} {
		assert.Equal(t, want, profile.NormalizeStatus(in), in)
	}
	assert.Equal(t, "face_architecture_profiles", profile.FaceArchitectureProfile{}.TableName())
}

func TestParse_StrictAndGuidanceOpaque(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, "faceShape", d.Classifiers[0].Key)
	assert.Nil(t, d.Guidance)

	_, err := profile.Parse([]byte(`{"schemaVersion": 1, "blendMarign": 0.6}`))
	assert.Error(t, err, "unknown fields are rejected")

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validDoc), &raw))
	raw["guidance"] = map[string]any{"templates": map[string]any{"x": map[string]any{"role": "contour"}}}
	b, _ := json.Marshal(raw)
	d, err = profile.Parse(b)
	require.NoError(t, err)
	assert.JSONEq(t, `{"templates":{"x":{"role":"contour"}}}`, string(d.Guidance))
}

func TestConditionHolds(t *testing.T) {
	c := profile.Condition{Measurement: "m", Op: ">", Value: 1}
	assert.True(t, c.Holds(1.5))
	assert.False(t, c.Holds(1))
	c.Op = ">="
	assert.True(t, c.Holds(1))
	c.Op = "<"
	assert.True(t, c.Holds(0.5))
	c.Op = "<="
	assert.True(t, c.Holds(1))
	c.Op = "=="
	assert.False(t, c.Holds(1))
}

func TestKeys(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, []string{"width_length_ratio"}, d.Classifiers[0].Keys())
	assert.Equal(t, []string{"alar_width_to_intercanthal_ratio"}, d.Traits[0].Keys())
}

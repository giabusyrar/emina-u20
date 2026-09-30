package facearch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// testDoc's numbers are test inputs, not proposed values.
const testDoc = `{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]},
  "measurementsShown": ["eye_width", "fifths", "gone_key"],
  "classifiers": [{
    "key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5,
    "classes": [
      {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
      {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}
    ]
  }],
  "traits": [{
    "key": "noseWidth",
    "rules": [
      {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]},
      {"label": "Narrow", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": "<", "value": 0.9}]}
    ],
    "default": "Average"
  }]
}`

func testProfile(t *testing.T) (*profile.FaceArchitectureProfile, *profile.Document) {
	t.Helper()
	p := &profile.FaceArchitectureProfile{ID: "p-1", Code: "test", Version: 3, Document: testDoc}
	d, err := profile.Load(p)
	require.NoError(t, err)
	return p, d
}

func TestInterpret_FixtureFace(t *testing.T) {
	p, d := testProfile(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())

	keys := []string{}
	for _, m := range out.Measurements {
		keys = append(keys, m.Key)
	}
	assert.Equal(t, []string{"fifths", "eye_width"}, keys, "worker order, shown keys only")
	assert.Equal(t, []string{"gone_key"}, out.MeasurementsMissing)

	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status, "the fixture's chin is out of frame")
	assert.Equal(t, []string{"Long", "Wide"}, fs.NotAssessable)
	assert.Equal(t, []string{"width_length_ratio"}, fs.MissingMeasurements)

	nw := out.Traits["noseWidth"]
	assert.Equal(t, traits.StatusAssessed, nw.Status)
	assert.Equal(t, "Broad", nw.Label) // 1.1773, band 0.9439..1.3768
	assert.True(t, nw.BoundaryUncertain)
	assert.Equal(t, "Average", nw.Alternative)

	assert.Equal(t, "occluded", out.Regions["jaw"])
	assert.Equal(t, "fa-measure/1", out.Provenance.CatalogueVersion)
	assert.Equal(t, facearch.ProfileRef{ID: "p-1", Code: "test", Version: 3, CatalogueVersion: "fa-measure/1"}, out.Provenance.Profile)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	assert.Equal(t, "face_landmarker", out.Provenance.Model.Name)
}

func TestInterpret_JSONShape(t *testing.T) {
	p, d := testProfile(t)
	b, err := json.Marshal(facearch.Interpret(p, d, facearchtest.FixtureFace()))
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &body))
	assert.Equal(t, "null", string(body["guidance"]), "a profile without a guidance section answers null")
	for _, k := range []string{"quality", "regions", "measurements", "classifications", "traits", "provenance"} {
		assert.Contains(t, body, k)
	}
	assert.JSONEq(t, `{"status":"uncalibrated"}`, string(mustField(t, body["provenance"], "calibration")))
}

func mustField(t *testing.T, raw json.RawMessage, key string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	return m[key]
}

func TestInterpret_CatalogueVersionDriftMarksEverythingUnavailable(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	m.CatalogueVersion = "fa-measure/2"
	out := facearch.Interpret(p, d, m)
	assert.Equal(t, classify.StatusUnavailable, out.Classifications["faceShape"].Status)
	assert.Contains(t, out.Classifications["faceShape"].Reason, "profile targets fa-measure/1, ai-worker measured with fa-measure/2")
	assert.Equal(t, traits.StatusUnavailable, out.Traits["noseWidth"].Status)
	assert.Equal(t, "fa-measure/2", out.Provenance.CatalogueVersion)
}

func TestInterpret_KeyDriftIsPerClassifierOrTrait(t *testing.T) {
	p, d := testProfile(t)
	d.Traits = append(d.Traits,
		profile.TraitSpec{Key: "lid", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "eyelid_type", Op: ">", Value: 0}}}}},
		profile.TraitSpec{Key: "gone", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "removed_key", Op: ">", Value: 0}}}}},
		profile.TraitSpec{Key: "list", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "fifths", Op: ">", Value: 0}}}}},
	)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	assert.Equal(t, traits.StatusAssessed, out.Traits["noseWidth"].Status)
	assert.Contains(t, out.Traits["lid"].Reason, "eyelid_type is not supported")
	assert.Contains(t, out.Traits["gone"].Reason, "removed_key is not in fa-measure/1")
	assert.Contains(t, out.Traits["list"].Reason, "fifths is list-valued")
}

func TestInterpret_LandmarksPassThrough(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	require.Len(t, m.Landmarks, measureclient.NLandmarks)
	out := facearch.Interpret(p, d, m)
	require.Len(t, out.Landmarks, measureclient.NLandmarks)
	for i, pt := range m.Landmarks {
		assert.Equal(t, [2]float64{pt[0], pt[1]}, out.Landmarks[i], "landmark %d unchanged, same index", i)
	}
}

func TestInterpret_LandmarksTrimmedToXY(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	m.Landmarks[7] = append([]float64{m.Landmarks[7][0], m.Landmarks[7][1]}, -0.25) // a z the worker may add
	out := facearch.Interpret(p, d, m)
	assert.Equal(t, [2]float64{m.Landmarks[7][0], m.Landmarks[7][1]}, out.Landmarks[7])
}

func TestInterpret_LandmarksNullWhenAbsentOrUnusable(t *testing.T) {
	p, d := testProfile(t)
	nan := math.NaN()
	for name, mutate := range map[string]func(m *measureclient.Response){
		"none":        func(m *measureclient.Response) { m.Landmarks = nil },
		"empty":       func(m *measureclient.Response) { m.Landmarks = [][]float64{} },
		"wrong count": func(m *measureclient.Response) { m.Landmarks = m.Landmarks[:10] },
		"short point": func(m *measureclient.Response) { m.Landmarks[3] = []float64{1} },
		"not finite":  func(m *measureclient.Response) { m.Landmarks[3] = []float64{nan, 2} },
	} {
		m := facearchtest.FixtureFace()
		mutate(m)
		out := facearch.Interpret(p, d, m)
		assert.Nil(t, out.Landmarks, name)
		b, err := json.Marshal(out)
		require.NoError(t, err, name)
		var body map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &body))
		assert.Equal(t, "null", string(body["landmarks"]), "%s: null, never an empty or partial array", name)
	}
}

func TestInterpret_ResultsListTheirSpecMeasurements(t *testing.T) {
	p, d := testProfile(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	fs := out.Classifications["faceShape"]
	assert.Equal(t, d.Classifiers[0].Keys(), fs.Measurements)
	assert.Equal(t, []string{"width_length_ratio"}, fs.Measurements)
	assert.Subset(t, fs.Measurements, fs.MissingMeasurements, "missing is a subset")
	assert.Equal(t, []string{"alar_width_to_intercanthal_ratio"}, out.Traits["noseWidth"].Measurements)

	// Unavailable through drift still names what the spec reads.
	m := facearchtest.FixtureFace()
	m.CatalogueVersion = "fa-measure/2"
	drifted := facearch.Interpret(p, d, m)
	assert.Equal(t, []string{"width_length_ratio"}, drifted.Classifications["faceShape"].Measurements)
	assert.Equal(t, []string{"alar_width_to_intercanthal_ratio"}, drifted.Traits["noseWidth"].Measurements)
}

func TestUnavailableWithoutSpecListsNoMeasurements(t *testing.T) {
	for name, v := range map[string]any{"classify": classify.Unavailable("x"), "traits": traits.Unavailable("x")} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		assert.Equal(t, "[]", string(mustField(t, b, "measurements")), name)
	}
}

// With the eyes occluded the worker withholds every supported measurement
// (the canonical frame is built from the eye corners). That is occlusion,
// not drift: classifiers lack evidence and traits are not assessable.
func TestInterpret_EyesOccludedWithholdsEverything(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	m.Regions["eyes"] = "occluded"
	reason := "canonical frame requires region eyes"
	for i := range m.Measurements {
		if m.Measurements[i].Visibility == measureclient.VisibilityNotSupported {
			continue
		}
		m.Measurements[i].Value = measureclient.Value{}
		m.Measurements[i].Band = nil
		m.Measurements[i].Visibility = measureclient.VisibilityOccluded
		m.Measurements[i].Reason = &reason
	}
	out := facearch.Interpret(p, d, m)
	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status)
	assert.Equal(t, []string{"Long", "Wide"}, fs.NotAssessable)
	nw := out.Traits["noseWidth"]
	assert.Equal(t, traits.StatusNotAssessable, nw.Status)
	assert.Equal(t, "requires alar_width_to_intercanthal_ratio", nw.Reason)
	for _, meas := range out.Measurements {
		assert.True(t, meas.Value.IsNull(), meas.Key)
	}
}

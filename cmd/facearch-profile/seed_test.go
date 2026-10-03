package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/classify"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/traits"
)

const seedPath = "seed/seagull-default-uncalibrated.json"

func TestValidateSeed(t *testing.T) {
	code, out, errOut := runCLI(testDeps(nil), "validate", seedPath)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "valid against fa-measure/1 (1 classifiers, 10 traits, 11 guidance templates, 7 guidance rules, calibration uncalibrated)")
}

func TestUpsertSeedAsGlobalDefault(t *testing.T) {
	store := profile.NewMemoryStore()
	code, _, errOut := runCLI(testDeps(store), "upsert", seedPath, "--brand", "*", "--app", "*", "--code", "seagull-default-uncalibrated")
	require.Equal(t, 0, code, errOut)
	p, err := store.Active(context.Background(), "any-brand", "any-app")
	require.NoError(t, err)
	assert.Equal(t, "seagull-default-uncalibrated", p.Code)
}

func loadSeed(t *testing.T) (*profile.FaceArchitectureProfile, *profile.Document) {
	t.Helper()
	raw, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	p := &profile.FaceArchitectureProfile{ID: "seed", Code: "seagull-default-uncalibrated", Version: 1, Document: string(raw)}
	d, err := profile.Load(p)
	require.NoError(t, err)
	require.NoError(t, profile.Validate(d, facearchtest.Catalogue()))
	return p, d
}

// The seed is uncalibrated and says so.
func TestSeedIsLabelledUncalibrated(t *testing.T) {
	_, d := loadSeed(t)
	assert.Equal(t, profile.CalibrationUncalibrated, d.Calibration.Status)
	assert.Nil(t, d.Calibration.BenchmarkRunID)
	fs := d.Classifiers[0]
	assert.Equal(t, "faceShape", fs.Key)
	labels := []string{}
	for _, c := range fs.Classes {
		labels = append(labels, c.Label)
	}
	assert.Equal(t, []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}, labels)
	keys := []string{}
	for _, tr := range d.Traits {
		keys = append(keys, tr.Key)
	}
	assert.Equal(t, []string{"eyeSize", "eyeAngle", "eyeDistance", "eyeShape", "eyebrowShape",
		"eyebrowDistance", "eyebrowShortness", "lipShape", "noseWidth", "noseLength"}, keys)
}

// On the recorded fixture face (chin out of frame) the seed withholds what
// it cannot see and flags thresholds inside the noise band. Expected labels
// follow from the fixture values in facearchtest and the seed thresholds;
// they check the mechanism, not the seed's accuracy.
func TestSeedOnFixtureFace(t *testing.T) {
	p, d := loadSeed(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())

	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status)
	assert.Equal(t, []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}, fs.NotAssessable)
	assert.Equal(t, []string{"chin_taper", "jaw_to_mid_width_ratio", "width_length_ratio"}, fs.MissingMeasurements)
	assert.InDelta(t, 0.25, *fs.ObservedWeightShare, 1e-12)

	want := map[string]traits.Result{
		"eyeSize":          {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Big"},
		"eyeAngle":         {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Downturned"},
		"eyeDistance":      {Status: "assessed", Label: "Wide-Set", BoundaryUncertain: true, Alternative: "Average"},
		"eyeShape":         {Status: "assessed", Label: "Almond", BoundaryUncertain: true, Alternative: "Round"},
		"eyebrowShape":     {Status: "assessed", Label: "Soft Angled", BoundaryUncertain: true, Alternative: "Hard Angled"},
		"eyebrowDistance":  {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Far-Apart"},
		"eyebrowShortness": {Status: "assessed", Label: "Normal"},
		"lipShape":         {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Downturned"},
		"noseWidth":        {Status: "assessed", Label: "Broad", BoundaryUncertain: true, Alternative: "Average"},
		"noseLength":       {Status: "not_assessable", MissingMeasurements: []string{"nose_length_ratio"}, Reason: "requires nose_length_ratio"},
	}
	// Every result names the keys its spec in the seed reads.
	for _, spec := range d.Traits {
		w := want[spec.Key]
		w.Measurements = spec.Keys()
		want[spec.Key] = w
	}
	assert.Equal(t, want, out.Traits)
	assert.Equal(t, []string{"chin_taper", "jaw_to_mid_width_ratio", "upper_to_mid_width_ratio", "width_length_ratio"}, fs.Measurements)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	require.Len(t, out.Measurements, 15, "occluded measurements are shown with value null")
	var withheld []string
	for _, m := range out.Measurements {
		if m.Value.IsNull() {
			withheld = append(withheld, m.Key)
		}
	}
	assert.Equal(t, []string{"width_length_ratio", "jaw_to_mid_width_ratio", "thirds_lower", "nose_length_ratio", "asymmetry_total"}, withheld)
}

// A synthetic in-frame face: the fixture's face with the three jaw-dependent
// measurements set to the values the landmarker extrapolated for the
// cropped chin. Synthetic input, not a measurement of anyone.
func TestSeedClassifiesAnInFrameFace(t *testing.T) {
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
	fs := facearch.Interpret(p, d, m).Classifications["faceShape"]
	assert.Empty(t, fs.NotAssessable)
	assert.Len(t, fs.Scores, 8)
	var sum float64
	for _, s := range fs.Scores {
		sum += s
	}
	assert.InDelta(t, 1.0, sum, 1e-9)
	assert.Equal(t, classify.StatusBlend, fs.Status, "eight wide prototypes: no class reaches 0.6")
	assert.Equal(t, "Oval", fs.Primary)
	assert.Equal(t, "Round", fs.Secondary)
}

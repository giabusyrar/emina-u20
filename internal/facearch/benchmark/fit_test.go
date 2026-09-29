package benchmark_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// example decodes a synthetic measure body, as the harness does with the cache.
func example(t *testing.T, values map[string]any, labels map[string]string) benchmark.Example {
	t.Helper()
	var r mc.Response
	require.NoError(t, json.Unmarshal(measureBody(values), &r))
	return benchmark.Example{Index: r.Index(), Labels: labels}
}

// Training set with known answers: Long faces at width_length_ratio 0.58,
// 0.59, 0.60, 0.61, 0.62 (mean 0.60, sample SD sqrt(0.00025)), Wide faces
// at 0.88 ... 0.92 (mean 0.90). noseWidth: Narrow at alar 0.80/0.82/0.84,
// Average 0.95/1.00/1.05, Broad 1.20/1.22/1.25.
func knownTrainingSet(t *testing.T) []benchmark.Example {
	var out []benchmark.Example
	offsets := []float64{-0.02, -0.01, 0, 0.01, 0.02}
	for _, cls := range []struct {
		label  string
		centre float64
	}{{"Long", 0.60}, {"Wide", 0.90}} {
		for _, o := range offsets {
			out = append(out, example(t, map[string]any{"width_length_ratio": cls.centre + o}, map[string]string{"faceShape": cls.label}))
		}
	}
	for label, xs := range map[string][]float64{"Narrow": {0.80, 0.82, 0.84}, "Average": {0.95, 1.00, 1.05}, "Broad": {1.20, 1.22, 1.25}} {
		for _, x := range xs {
			out = append(out, example(t, map[string]any{"alar_width_to_intercanthal_ratio": x}, map[string]string{"noseWidth": label}))
		}
	}
	// No target label: ignored by every fit.
	out = append(out, example(t, map[string]any{"width_length_ratio": 5.0, "alar_width_to_intercanthal_ratio": 5.0}, map[string]string{}))
	return out
}

func TestFitProfileKnownCentresAndThresholds(t *testing.T) {
	fit, err := benchmark.FitProfile(testDocument(t), knownTrainingSet(t), 3, "fb-test", "perfectcorp")
	require.NoError(t, err)
	classes := fit.Doc.Classifiers[0].Classes
	sd := math.Sqrt(0.00025)
	assert.InDelta(t, 0.60, classes[0].Prototype["width_length_ratio"].Centre, 1e-12)
	assert.InDelta(t, sd, classes[0].Prototype["width_length_ratio"].Spread, 1e-12)
	assert.InDelta(t, 0.90, classes[1].Prototype["width_length_ratio"].Centre, 1e-12)
	assert.InDelta(t, sd, classes[1].Prototype["width_length_ratio"].Spread, 1e-12)
	assert.Equal(t, 1.0, classes[0].Prototype["width_length_ratio"].Weight, "weights are kept")
	assert.Equal(t, 0.6, fit.Doc.Classifiers[0].BlendMargin, "blend margin is kept")

	rules := fit.Doc.Traits[0].Rules
	assert.InDelta(t, (1.05+1.20)/2, rules[0].When[0].Value, 1e-12, "Broad: midpoint between the top Average and the bottom Broad")
	assert.InDelta(t, (0.84+0.95)/2, rules[1].When[0].Value, 1e-12, "Narrow: midpoint between the top Narrow and the bottom Average")
	assert.Equal(t, ">", rules[0].When[0].Op)
	assert.Equal(t, "Average", fit.Doc.Traits[0].Default)

	assert.Equal(t, []string{"classifiers.faceShape.classes.Long", "classifiers.faceShape.classes.Wide", "traits.noseWidth"}, fit.Owners)
	var paths []string
	for _, s := range fit.Sources {
		assert.Equal(t, "face-benchmark run fb-test", s.Cite)
		paths = append(paths, s.For)
	}
	assert.Equal(t, []string{
		"classifiers.faceShape.classes.Long.prototype.width_length_ratio",
		"classifiers.faceShape.classes.Wide.prototype.width_length_ratio",
		"traits.noseWidth.rules.0.when.0",
		"traits.noseWidth.rules.1.when.0",
	}, paths)
	assert.Contains(t, fit.Sources[0].Note, "over 5 training images labelled Long by perfectcorp")

	base := testDocument(t)
	assert.Equal(t, 0.6, base.Classifiers[0].Classes[0].Prototype["width_length_ratio"].Centre, "the base document is not modified")
}

func TestFitProfileRefusesToMixFittedAndUnfittedNumbers(t *testing.T) {
	_, err := benchmark.FitProfile(testDocument(t), knownTrainingSet(t), 4, "fb-test", "perfectcorp")
	var fe *benchmark.FitError
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, []string{
		"traits.noseWidth: 3 assessable training images labelled Average, need 4",
		"traits.noseWidth: 3 assessable training images labelled Broad, need 4",
		"traits.noseWidth: 3 assessable training images labelled Narrow, need 4",
	}, fe.Problems)

	flat := []benchmark.Example{}
	for i := 0; i < 3; i++ {
		flat = append(flat,
			example(t, map[string]any{"width_length_ratio": 0.7}, map[string]string{"faceShape": "Long"}),
			example(t, map[string]any{"width_length_ratio": 0.8 + float64(i)/100}, map[string]string{"faceShape": "Wide"}))
	}
	_, err = benchmark.FitProfile(testDocument(t), flat, 3, "fb-test", "perfectcorp")
	require.ErrorAs(t, err, &fe)
	assert.Contains(t, fe.Problems, "classifiers.faceShape.classes.Long.prototype.width_length_ratio: the 3 training values have no spread")
	assert.Contains(t, fe.Problems, "traits.noseWidth: 0 assessable training images labelled Average, need 3")

	_, err = benchmark.FitProfile(testDocument(t), flat, 1, "fb-test", "perfectcorp")
	assert.ErrorContains(t, err, "minimum class count must be at least 2")
}

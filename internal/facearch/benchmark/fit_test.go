package benchmark_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/benchmark"
	mc "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
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

	var paths []string
	for _, s := range fit.Sources {
		assert.Equal(t, "face-benchmark run fb-test", s.Cite)
		paths = append(paths, s.For)
	}
	assert.Equal(t, []string{
		"classifiers.faceShape.classes.Long.prototype.width_length_ratio.centre",
		"classifiers.faceShape.classes.Long.prototype.width_length_ratio.spread",
		"classifiers.faceShape.classes.Wide.prototype.width_length_ratio.centre",
		"classifiers.faceShape.classes.Wide.prototype.width_length_ratio.spread",
		"traits.noseWidth.rules.0.when.0.value",
		"traits.noseWidth.rules.1.when.0.value",
	}, paths)
	assert.Equal(t, paths, fit.Owners, "owners are exactly the fitted numbers")
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

func TestFitProfileOwnersAreOnlyFittedNumbers(t *testing.T) {
	base := testDocument(t)
	fit, err := benchmark.FitProfile(base, knownTrainingSet(t), 3, "fb-test", "perfectcorp")
	require.NoError(t, err)
	for _, kept := range []string{
		"classifiers.faceShape.classes.Long",
		"classifiers.faceShape.classes.Long.prototype.width_length_ratio.weight",
		"classifiers.faceShape.classes.Long.requires",
		"classifiers.faceShape.blendMargin",
		"traits.noseWidth",
		"traits.noseWidth.rules",
		"traits.noseWidth.rules.0.op",
		"traits.noseWidth.default",
	} {
		for _, o := range fit.Owners {
			assert.NotEqual(t, kept, o, "a base source for %s must survive", kept)
			assert.False(t, strings.HasPrefix(kept, o+"."), "a base source for %s is beneath owner %s and would be dropped", kept, o)
		}
	}
	sourced := map[string]int{}
	for _, s := range fit.Sources {
		sourced[s.For]++
	}
	for _, o := range fit.Owners {
		assert.Equal(t, 1, sourced[o], "%s has its own new source", o)
	}
	assert.Len(t, fit.Sources, len(fit.Owners))
}

func nosePoints(t *testing.T, narrow, average, broad []float64) []benchmark.Example {
	var out []benchmark.Example
	for label, xs := range map[string][]float64{"Narrow": narrow, "Average": average, "Broad": broad} {
		for _, x := range xs {
			out = append(out, example(t, map[string]any{"alar_width_to_intercanthal_ratio": x}, map[string]string{"noseWidth": label}))
		}
	}
	return out
}

func TestFitProfileThresholdTieBreak(t *testing.T) {
	train := nosePoints(t, []float64{0.25, 0.5}, []float64{1, 1.5, 3}, []float64{2.5, 3.5, 4})
	for _, x := range []float64{0.5, 0.6, 0.7, 0.8, 0.9, 1.0} {
		label := "Long"
		if x > 0.75 {
			label = "Wide"
		}
		train = append(train, example(t, map[string]any{"width_length_ratio": x}, map[string]string{"faceShape": label}))
	}
	for _, tc := range []struct{ base, want float64 }{{2.625, 2}, {2.1, 2}, {3.2, 3.25}} {
		doc := testDocument(t)
		doc.Traits[0].Rules[0].When[0].Value = tc.base
		fit, err := benchmark.FitProfile(doc, train, 2, "fb-test", "perfectcorp")
		require.NoError(t, err)
		assert.InDelta(t, tc.want, fit.Doc.Traits[0].Rules[0].When[0].Value, 1e-12, "base %v", tc.base)
	}
}

func TestFitProfileIdenticalTraitValuesAreRefused(t *testing.T) {
	train := nosePoints(t, []float64{1, 1}, []float64{1, 1}, []float64{1, 1})
	_, err := benchmark.FitProfile(testDocument(t), train, 2, "fb-test", "perfectcorp")
	var fe *benchmark.FitError
	require.ErrorAs(t, err, &fe)
	assert.Contains(t, fe.Problems, "traits.noseWidth.rules.0.when.0: alar_width_to_intercanthal_ratio has fewer than 2 distinct training values")
}

func TestFitProfileAsymmetricClass(t *testing.T) {
	var train []benchmark.Example
	for _, x := range []float64{0.50, 0.52, 0.60, 0.70} {
		train = append(train, example(t, map[string]any{"width_length_ratio": x}, map[string]string{"faceShape": "Long"}))
	}
	for _, x := range []float64{0.88, 0.90, 0.92, 0.94} {
		train = append(train, example(t, map[string]any{"width_length_ratio": x}, map[string]string{"faceShape": "Wide"}))
	}
	train = append(train, nosePoints(t, []float64{0.8, 0.82}, []float64{1, 1.05}, []float64{1.2, 1.25})...)
	doc := testDocument(t)
	require.NotEqual(t, 0.58, doc.Classifiers[0].Classes[0].Prototype["width_length_ratio"].Centre)
	fit, err := benchmark.FitProfile(doc, train, 2, "fb-test", "perfectcorp")
	require.NoError(t, err)
	term := fit.Doc.Classifiers[0].Classes[0].Prototype["width_length_ratio"]
	assert.InDelta(t, 0.58, term.Centre, 1e-12)
	assert.InDelta(t, 0.0909212113132390, term.Spread, 1e-12)
}

func TestFitProfileUnanswerableLabelsAreReported(t *testing.T) {
	train := knownTrainingSet(t)
	train = append(train,
		example(t, map[string]any{"width_length_ratio": 0.7}, map[string]string{"faceShape": "Oval"}),
		example(t, map[string]any{"alar_width_to_intercanthal_ratio": 1.0}, map[string]string{"noseWidth": "Huge"}))
	_, err := benchmark.FitProfile(testDocument(t), train, 3, "fb-test", "perfectcorp")
	var fe *benchmark.FitError
	require.ErrorAs(t, err, &fe)
	assert.Contains(t, fe.Problems, `classifiers.faceShape: 1 training images labelled "Oval", which the profile cannot answer`)
	assert.Contains(t, fe.Problems, `traits.noseWidth: 1 training images labelled "Huge", which the profile cannot answer`)
}

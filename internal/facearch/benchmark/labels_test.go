package benchmark_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func testDocument(t *testing.T) *profile.Document {
	t.Helper()
	d, err := profile.Parse([]byte(testDoc))
	require.NoError(t, err)
	return d
}

func readLabelMap(t *testing.T, body string) *benchmark.LabelMap {
	t.Helper()
	m, err := benchmark.ReadLabelMap(writeFile(t, t.TempDir(), "label-map.json", body))
	require.NoError(t, err)
	return m
}

func TestLabelMapResolve(t *testing.T) {
	targets, err := readLabelMap(t, testLabelMap).Resolve(testDocument(t))
	require.NoError(t, err)
	assert.Equal(t, map[string]benchmark.Target{
		"faceShape": {Kind: benchmark.KindClassifier, Key: "faceShape"},
		"noseWidth": {Kind: benchmark.KindTrait, Key: "noseWidth"},
	}, targets)
}

func TestLabelMapResolveRejects(t *testing.T) {
	m := readLabelMap(t, `{"attributes": {
	  "faceShape": {"ours": "faceShape", "values": {"perfectcorp": {"Oval": "Oval"}, "vendorX": {}}},
	  "shape2": {"ours": "faceShape", "values": {}},
	  "eyelid": {"ours": "eyelid", "values": {}},
	  "noseWidth": {"ours": "noseWidth", "values": {"perfectcorp": {"Wide": "Wide"}}}
	}}`)
	_, err := m.Resolve(testDocument(t))
	require.Error(t, err)
	for _, want := range []string{
		`faceShape: perfectcorp value "Oval" maps to "Oval", which classifier faceShape cannot answer`,
		`faceShape: unknown source family "vendorX"`,
		`shape2: ours "faceShape" is already mapped by faceShape`,
		`eyelid: ours "eyelid" is not a classifier or trait of the profile`,
		`noseWidth: perfectcorp value "Wide" maps to "Wide", which trait noseWidth cannot answer`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestReadLabelMapIsStrict(t *testing.T) {
	_, err := benchmark.ReadLabelMap(writeFile(t, t.TempDir(), "m.json", `{"attributes": {"a": {"ours": "x", "valeus": {}}}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "valeus"`)
	_, err = benchmark.ReadLabelMap(writeFile(t, t.TempDir(), "m.json", `{"attributes": {}}`))
	assert.ErrorContains(t, err, "no attributes")
}

func results(rows ...[4]string) []benchmark.ResultRow {
	out := make([]benchmark.ResultRow, len(rows))
	for i, r := range rows {
		out[i] = benchmark.ResultRow{Line: i + 2, ImageID: r[0], Source: r[1], Attribute: r[2], Value: r[3]}
	}
	return out
}

func TestBuildLabels(t *testing.T) {
	m := readLabelMap(t, testLabelMap)
	targets, err := m.Resolve(testDocument(t))
	require.NoError(t, err)
	ls, err := benchmark.BuildLabels(results(
		[4]string{"a", "perfectcorp", "faceShape", "Oblong"},
		[4]string{"b", "perfectcorp", "faceShape", "Unknown"},
		[4]string{"a", "rater:r1", "faceShape", "long"},
		[4]string{"a", "rater:r2", "faceShape", "long"},
		[4]string{"a", "rater:r3", "faceShape", "wide"},
		[4]string{"b", "rater:r1", "faceShape", "long"},
		[4]string{"b", "rater:r2", "faceShape", "wide"},
		[4]string{"c", "rater:r1", "faceShape", "wide"},
		[4]string{"a", "wardah", "faceBalance", "Curved Thin"},
	), m, targets, 2)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"a": "Long"}, ls.Labels["faceShape"]["perfectcorp"])
	assert.Equal(t, 1, ls.Abstained["faceShape"]["perfectcorp"], "Unknown maps to null: abstained, not compared")
	// a: 2 of 3 raters say long -> consensus Long. b: 1-1 -> none.
	// c: one rater is below the 2-rater minimum -> none.
	assert.Equal(t, map[string]string{"a": "Long"}, ls.Labels["faceShape"][benchmark.TargetRaterConsensus])
	assert.Equal(t, 2, ls.NoConsensus["faceShape"])
	assert.Equal(t, map[string]int{"faceBalance": 1}, ls.Unmapped)
	assert.Equal(t, []string{"perfectcorp", "rater-consensus", "rater:r1", "rater:r2", "rater:r3"}, ls.Sources("faceShape"))
	assert.Equal(t, map[string]map[string]string{"faceShape": {"a": "Long"}}, ls.ForTarget("perfectcorp"))
}

func TestBuildLabelsRejectsUnknownVendorValue(t *testing.T) {
	m := readLabelMap(t, testLabelMap)
	targets, err := m.Resolve(testDocument(t))
	require.NoError(t, err)
	_, err = benchmark.BuildLabels(results([4]string{"a", "perfectcorp", "faceShape", "Oval"}), m, targets, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `results line 2: perfectcorp value "Oval" for faceShape is not in the label map`)
}

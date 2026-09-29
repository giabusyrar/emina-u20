package benchmark_test

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func loadTestProfile(t *testing.T) *benchmark.ProfileFile {
	t.Helper()
	p, err := benchmark.LoadProfileFile(writeFile(t, t.TempDir(), "candidate.json", testDoc), facearchtest.Catalogue())
	require.NoError(t, err)
	return p
}

func decode(t *testing.T, body []byte) *mc.Response {
	t.Helper()
	var r mc.Response
	require.NoError(t, json.Unmarshal(body, &r))
	return &r
}

func TestLoadProfileFile(t *testing.T) {
	p := loadTestProfile(t)
	assert.Equal(t, "candidate", p.Entity.Code)
	assert.Len(t, p.SHA256, 64)
	assert.Equal(t, "file:"+p.SHA256[:12], p.Entity.ID)
	assert.Equal(t, "faceShape", p.Doc.Classifiers[0].Key)

	_, err := benchmark.LoadProfileFile(writeFile(t, t.TempDir(), "bad.json",
		strings.Replace(testDoc, `"blendMargin": 0.6`, `"blendMargin": 0.4`, 1)), facearchtest.Catalogue())
	assert.ErrorContains(t, err, "classifiers[0].blendMargin: must be in (0.5, 1]")
}

// Answers are facearch.Interpret's, reduced to what the metrics compare.
func TestAnswers(t *testing.T) {
	p := loadTestProfile(t)
	shape := benchmark.Target{Kind: benchmark.KindClassifier, Key: "faceShape"}
	nose := benchmark.Target{Kind: benchmark.KindTrait, Key: "noseWidth"}

	a := benchmark.Answers(p.Entity, p.Doc, decode(t, measureBody(map[string]any{"width_length_ratio": 0.6, "alar_width_to_intercanthal_ratio": 0.8})))
	assert.Equal(t, "single", a[shape].Status)
	assert.Equal(t, "Long", a[shape].Label)
	assert.Equal(t, []string{"Long"}, a[shape].Top2)
	assert.InDelta(t, 1/(1+math.Exp(-4.5)), *a[shape].Score, 1e-12)
	assert.Equal(t, benchmark.Answer{Status: "assessed", Label: "Narrow", Top2: []string{"Narrow"}}, a[nose])

	// Equidistant: a blend, primary by class order; the band straddles 1.1.
	a = benchmark.Answers(p.Entity, p.Doc, decode(t, measureBody(map[string]any{
		"width_length_ratio": 0.75, "alar_width_to_intercanthal_ratio": [3]float64{1.05, 1.0, 1.15}})))
	assert.Equal(t, "blend", a[shape].Status)
	assert.Equal(t, []string{"Long", "Wide"}, a[shape].Top2)
	assert.InDelta(t, 0.5, *a[shape].Score, 1e-12)
	assert.Equal(t, []string{"Average", "Broad"}, a[nose].Top2, "the boundary alternative joins top-2")

	a = benchmark.Answers(p.Entity, p.Doc, decode(t, measureBody(map[string]any{"width_length_ratio": "occluded", "alar_width_to_intercanthal_ratio": 1.0})))
	assert.Equal(t, "insufficient_evidence", a[shape].Status)
	assert.False(t, a[shape].Answered())
	assert.Nil(t, a[shape].Score)
	assert.True(t, a[nose].Answered())
}

func TestReduceClassificationPanicsOnBrokenInvariant(t *testing.T) {
	for name, scores := range map[string]map[string]float64{
		"no scores":    {},
		"nan":          {"Long": math.NaN()},
		"out of range": {"Long": 1.5},
	} {
		t.Run(name, func(t *testing.T) {
			assert.PanicsWithValue(t,
				"classify returned single for faceShape with no finite primary score in [0,1]",
				func() {
					benchmark.ReduceClassification("faceShape", classify.Result{Status: classify.StatusSingle, Primary: "Long", Scores: scores})
				})
		})
	}
	a := benchmark.ReduceClassification("faceShape", classify.Result{Status: classify.StatusInsufficientEvidence, Scores: map[string]float64{}})
	assert.Equal(t, benchmark.Answer{Status: "insufficient_evidence"}, a)
}

func TestLoadItemsErrorsNameTheImageNotThePath(t *testing.T) {
	dir := t.TempDir()
	cache, err := benchmark.NewCache(filepath.Join(dir, "cache"), testModel, "fa-measure/1")
	require.NoError(t, err)
	missing := filepath.Join(dir, "secret-subject-folder", "gone.jpg")
	_, err = benchmark.LoadItems([]benchmark.ManifestRow{{ImageID: "img7", ImagePath: missing}}, cache)
	require.Error(t, err)
	assert.Equal(t, "image img7: file not found", err.Error())
	assert.NotContains(t, err.Error(), "secret-subject-folder")
	assert.NotContains(t, err.Error(), "gone.jpg")
}

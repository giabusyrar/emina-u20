package benchmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
)

func TestNewCacheChecksKey(t *testing.T) {
	_, err := benchmark.NewCache(t.TempDir(), "not-a-sha", "fa-measure/1")
	assert.ErrorContains(t, err, "64 lowercase hex digits")
	_, err = benchmark.NewCache(t.TempDir(), testModel, " ")
	assert.ErrorContains(t, err, "catalogue version is required")
	_, err = benchmark.NewCache("", testModel, "fa-measure/1")
	assert.ErrorContains(t, err, "cache directory is required")
}

func TestCacheRoundTripsRecordsByKey(t *testing.T) {
	dir := t.TempDir()
	c, err := benchmark.NewCache(dir, testModel, "fa-measure/1")
	require.NoError(t, err)
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, ok, err := c.Get(sha)
	require.NoError(t, err)
	assert.False(t, ok)

	body := measureBody(map[string]any{"width_length_ratio": 0.75})
	require.NoError(t, c.Put(benchmark.Record{ImageSHA256: sha, ModelSHA256: testModel, CatalogueVersion: "fa-measure/1", Status: 200, Body: body}))
	assert.FileExists(t, filepath.Join(dir, testModel, "fa-measure_1", sha+".json"))

	rec, ok, err := c.Get(sha)
	require.NoError(t, err)
	require.True(t, ok)
	assert.JSONEq(t, string(body), string(rec.Body))
	assert.Equal(t, benchmark.OutcomeMeasured, rec.Outcome())
	assert.True(t, rec.Final())
	resp, err := rec.Response()
	require.NoError(t, err)
	assert.Equal(t, 0.75, *resp.Measurements[0].Value.Scalar)

	// Same image, other catalogue: a different key, so nothing is found.
	other, err := benchmark.NewCache(dir, testModel, "fa-measure/2")
	require.NoError(t, err)
	_, ok, err = other.Get(sha)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.ErrorContains(t, other.Put(*rec), "does not belong")
}

func TestRecordOutcomes(t *testing.T) {
	for status, tc := range map[string]struct {
		status  int
		body    []byte
		outcome string
		final   bool
	}{
		"gate":       {422, gateBody, benchmark.OutcomeGateRejected, true},
		"face count": {422, faceCountBody, benchmark.OutcomeFaceCount, true},
		"validation": {422, []byte(`{"detail":[{"loc":["body","image"]}]}`), benchmark.OutcomeRejectedOther, true},
		"unreadable": {400, []byte(`{"detail":{"code":"unreadable_image"}}`), benchmark.OutcomeUnreadable, true},
		"no model":   {503, noModelBody, benchmark.OutcomeUnavailable, false},
	} {
		t.Run(status, func(t *testing.T) {
			r := benchmark.Record{Status: tc.status, Body: tc.body}
			assert.Equal(t, tc.outcome, r.Outcome())
			assert.Equal(t, tc.final, r.Final())
			_, err := r.Response()
			assert.ErrorContains(t, err, "has no measurements")
		})
	}
}

func TestRecordResponseChecksModelAndCatalogue(t *testing.T) {
	body := measureBody(map[string]any{"width_length_ratio": 0.75})
	r := benchmark.Record{ImageSHA256: "x", ModelSHA256: testModel, CatalogueVersion: "fa-measure/2", Status: 200, Body: body}
	_, err := r.Response()
	assert.ErrorContains(t, err, "cached body is catalogue fa-measure/1, record says fa-measure/2")
	r = benchmark.Record{ImageSHA256: "x", ModelSHA256: "other", CatalogueVersion: "fa-measure/1", Status: 200, Body: body}
	_, err = r.Response()
	assert.ErrorContains(t, err, "cached body is model")
}

func TestPutCatalogueRefusesSilentDefinitionChange(t *testing.T) {
	c, err := benchmark.NewCache(t.TempDir(), testModel, "fa-measure/1")
	require.NoError(t, err)
	require.NoError(t, c.PutCatalogue(facearchtest.CatalogueJSON()))
	require.NoError(t, c.PutCatalogue(facearchtest.CatalogueJSON()), "the same catalogue again is fine")
	cat, err := c.Catalogue()
	require.NoError(t, err)
	assert.Equal(t, facearchtest.Catalogue(), cat)

	changed := []byte(`{"catalogueVersion": "fa-measure/1", "measurements": []}`)
	assert.ErrorContains(t, c.PutCatalogue(changed), "changed without a version bump")
	assert.ErrorContains(t, c.PutCatalogue([]byte(`{"catalogueVersion": "fa-measure/9", "measurements": []}`)), "cache is for fa-measure/1")

	empty, err := benchmark.NewCache(t.TempDir(), testModel, "fa-measure/1")
	require.NoError(t, err)
	_, err = empty.Catalogue()
	assert.ErrorIs(t, err, os.ErrNotExist)
}

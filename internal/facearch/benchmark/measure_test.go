package benchmark_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
)

// fakeWorker answers by image bytes and records the calls it gets.
type fakeWorker struct {
	answers map[string]answer
	calls   []string
	catalog []byte
}

type answer struct {
	status int
	body   []byte
}

func (f *fakeWorker) Catalogue(context.Context) ([]byte, error) { return f.catalog, nil }

func (f *fakeWorker) Measure(_ context.Context, image []byte, _ string) (int, []byte, error) {
	f.calls = append(f.calls, string(image))
	if strings.Contains(string(image), "unreachable") {
		return 0, nil, errors.New("connection refused")
	}
	a := f.answers[string(image)]
	return a.status, a.body, nil
}

func (f *fakeWorker) answer(image string, status int, body []byte) {
	if f.answers == nil {
		f.answers = map[string]answer{}
	}
	f.answers[image] = answer{status, body}
}

func TestMeasureCachesEveryAnswerAndSkipsFinalOnes(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"a.jpg": "img-a", "b.jpg": "img-b", "c.jpg": "img-c", "d.jpg": "img-d-unreachable", "e.jpg": "img-a"} {
		writeFile(t, dir, name, content)
	}
	manifest := writeFile(t, dir, "manifest.csv", "image_id,image_path,subject_id,consent_ref,hijab,notes\n"+
		"a,a.jpg,s1,c,false,\nb,b.jpg,s2,c,true,\nc,c.jpg,s3,c,false,\nd,d.jpg,s4,c,false,\ne,e.jpg,s1,c,false,copy of a\n")
	rows, err := benchmark.ReadManifest(manifest)
	require.NoError(t, err)

	w := &fakeWorker{catalog: facearchtest.CatalogueJSON()}
	w.answer("img-a", 200, measureBody(map[string]any{"width_length_ratio": 0.7}))
	w.answer("img-b", 422, gateBody)
	w.answer("img-c", 503, noModelBody)
	var log bytes.Buffer
	sum, err := benchmark.Measure(context.Background(), rows, w, filepath.Join(dir, "cache"), testModel, &log)
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", sum.CatalogueVersion)
	assert.Equal(t, 5, sum.Images)
	assert.Equal(t, 1, sum.Cached, "e has a's bytes: same SHA-256, already cached")
	assert.Equal(t, 1, sum.Failed)
	assert.Equal(t, map[string]int{benchmark.OutcomeMeasured: 1, benchmark.OutcomeGateRejected: 1, benchmark.OutcomeUnavailable: 1}, sum.Recorded)
	assert.Contains(t, log.String(), "d: connection refused")
	assert.NotContains(t, log.String(), "d.jpg", "progress names images by id, not path")

	// Second run: 200 and 422 are final; the 503 and the unreachable image are retried.
	w.calls = nil
	w.answer("img-c", 200, measureBody(map[string]any{"width_length_ratio": 0.9}))
	sum, err = benchmark.Measure(context.Background(), rows, w, filepath.Join(dir, "cache"), testModel, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, []string{"img-c", "img-d-unreachable"}, w.calls)
	assert.Equal(t, 3, sum.Cached)
	assert.Equal(t, map[string]int{benchmark.OutcomeMeasured: 1}, sum.Recorded)

	cache, err := benchmark.NewCache(filepath.Join(dir, "cache"), testModel, "fa-measure/1")
	require.NoError(t, err)
	items, err := benchmark.LoadItems(rows, cache)
	require.NoError(t, err)
	var outcomes []string
	for _, it := range items {
		outcomes = append(outcomes, it.Outcome)
	}
	assert.Equal(t, []string{"measured", "gate_rejected", "measured", "not_measured", "measured"}, outcomes)
}

func TestMeasureStopsOnAnotherModel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", "img-a")
	rows, err := benchmark.ReadManifest(writeFile(t, dir, "manifest.csv", "image_id,image_path,subject_id,consent_ref,hijab,notes\na,a.jpg,s1,c,false,\n"))
	require.NoError(t, err)
	w := &fakeWorker{catalog: facearchtest.CatalogueJSON()}
	w.answer("img-a", 200, facearchtest.FixtureFaceJSON()) // recorded with the real model's manifest sha
	_, err = benchmark.Measure(context.Background(), rows, w, filepath.Join(dir, "cache"), testModel, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected "+testModel+"; nothing was recorded")
}

func TestMeasureRefusesNonJSONBodies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", "img-a")
	rows, err := benchmark.ReadManifest(writeFile(t, dir, "manifest.csv", "image_id,image_path,subject_id,consent_ref,hijab,notes\na,a.jpg,s1,c,false,\n"))
	require.NoError(t, err)
	w := &fakeWorker{catalog: facearchtest.CatalogueJSON()}
	w.answer("img-a", 503, []byte("<html>bad gateway</html>"))
	sum, err := benchmark.Measure(context.Background(), rows, w, filepath.Join(dir, "cache"), testModel, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.Failed)
	assert.Empty(t, sum.Recorded)
}

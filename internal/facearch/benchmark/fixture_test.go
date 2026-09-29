package benchmark_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
)

// img is one synthetic image: its file holds placeholder bytes (never
// decoded), and its cache record the given status and body. Status 0
// means no cache record.
type img struct {
	id, subject string
	hijab       bool
	status      int
	body        []byte
}

// fixture writes a manifest and a cache for imgs into a temp dir.
type fixture struct {
	dir, manifest string
	cache         *benchmark.Cache
}

func newFixture(t *testing.T, imgs []img) *fixture {
	t.Helper()
	dir := t.TempDir()
	cache, err := benchmark.NewCache(filepath.Join(dir, "cache"), testModel, "fa-measure/1")
	require.NoError(t, err)
	require.NoError(t, cache.PutCatalogue(facearchtest.CatalogueJSON()))
	lines := []string{"image_id,image_path,subject_id,consent_ref,hijab,notes"}
	for _, im := range imgs {
		name := im.id + ".jpg"
		writeFile(t, dir, name, "placeholder bytes of "+im.id)
		lines = append(lines, fmt.Sprintf("%s,%s,%s,consent-%s,%t,", im.id, name, im.subject, im.id, im.hijab))
		if im.status == 0 {
			continue
		}
		sha, err := benchmark.FileSHA256(filepath.Join(dir, name))
		require.NoError(t, err)
		require.NoError(t, cache.Put(benchmark.Record{ImageSHA256: sha, ModelSHA256: testModel,
			CatalogueVersion: "fa-measure/1", Status: im.status, Body: im.body}))
	}
	return &fixture{dir: dir, manifest: writeFile(t, dir, "manifest.csv", strings.Join(lines, "\n")+"\n"), cache: cache}
}

func (f *fixture) items(t *testing.T) []benchmark.Item {
	t.Helper()
	rows, err := benchmark.ReadManifest(f.manifest)
	require.NoError(t, err)
	items, err := benchmark.LoadItems(rows, f.cache)
	require.NoError(t, err)
	return items
}

func (f *fixture) profile(t *testing.T, doc string) *benchmark.ProfileFile {
	t.Helper()
	p, err := benchmark.LoadProfileFile(writeFile(t, f.dir, "profile.json", doc), facearchtest.Catalogue())
	require.NoError(t, err)
	return p
}

func (f *fixture) labels(t *testing.T, p *benchmark.ProfileFile, results string) *benchmark.LabelSet {
	t.Helper()
	rows, err := benchmark.ReadManifest(f.manifest)
	require.NoError(t, err)
	rr, err := benchmark.ReadResults(writeFile(t, f.dir, "results.csv", results), rows)
	require.NoError(t, err)
	lm, err := benchmark.ReadLabelMap(writeFile(t, f.dir, "label-map.json", testLabelMap))
	require.NoError(t, err)
	targets, err := lm.Resolve(p.Doc)
	require.NoError(t, err)
	ls, err := benchmark.BuildLabels(rr, lm, targets, benchmark.DefaultMinConsensusRaters)
	require.NoError(t, err)
	return ls
}

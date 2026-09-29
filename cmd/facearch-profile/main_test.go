package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// cliDoc is a small valid document; its numbers are test inputs.
const cliDoc = `{
  "schemaVersion": 1, "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"}
    ]},
  "measurementsShown": ["width_length_ratio"],
  "classifiers": [{"key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5, "classes": [
    {"label": "Long", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
    {"label": "Wide", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}]}],
  "traits": []
}`

func writeDoc(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func testDeps(store *profile.MemoryStore) deps {
	return deps{
		catalogue: func(context.Context) (*measureclient.Catalogue, error) { return facearchtest.Catalogue(), nil },
		store: func(context.Context) (profile.Store, func(), error) {
			return store, func() {}, nil
		},
	}
}

func runCLI(d deps, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestValidate(t *testing.T) {
	code, out, errOut := runCLI(testDeps(nil), "validate", writeDoc(t, cliDoc))
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "valid against fa-measure/1 (1 classifiers, 0 traits, 0 guidance templates, 0 guidance rules, calibration uncalibrated)")
}

func TestValidateRejectsBadDocument(t *testing.T) {
	bad := writeDoc(t, strings.Replace(cliDoc, `"blendMargin": 0.6`, `"blendMargin": 0.4`, 1))
	code, _, errOut := runCLI(testDeps(nil), "validate", bad)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "classifiers[0].blendMargin: must be in (0.5, 1]")
}

func TestValidateReportsCatalogueFailure(t *testing.T) {
	d := testDeps(nil)
	d.catalogue = func(context.Context) (*measureclient.Catalogue, error) {
		return nil, errors.New("face-measure unavailable: connection refused")
	}
	code, _, errOut := runCLI(d, "validate", writeDoc(t, cliDoc))
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "catalogue: face-measure unavailable")
}

func TestUpsert(t *testing.T) {
	store := profile.NewMemoryStore()
	path := writeDoc(t, cliDoc)
	code, out, errOut := runCLI(testDeps(store), "upsert", path, "--brand", "wardah", "--app", "*", "--code", "wardah-v1", "--title", "Wardah")
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "stored wardah-v1 version 1 for brand wardah app *")

	p, err := store.Active(context.Background(), "wardah", "app-1")
	require.NoError(t, err)
	assert.Equal(t, "Wardah", p.Title)
	assert.JSONEq(t, cliDoc, p.Document)

	code, out, _ = runCLI(testDeps(store), "upsert", path, "--brand", "wardah", "--app", "*", "--code", "wardah-v1")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "version 2")
}

func TestUpsertInvalidDocumentWritesNothing(t *testing.T) {
	store := profile.NewMemoryStore()
	code, _, _ := runCLI(testDeps(store), "upsert", writeDoc(t, `{"schemaVersion": 1}`), "--brand", "*", "--app", "*", "--code", "x")
	assert.Equal(t, 1, code)
	_, err := store.Active(context.Background(), "*", "*")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)
}

func TestUsage(t *testing.T) {
	path := writeDoc(t, cliDoc)
	for _, args := range [][]string{
		{},
		{"validate"},
		{"validate", path, "extra"},
		{"frobnicate", path},
		{"upsert", path, "--brand", "*", "--app", "*"},
		{"upsert", "--brand", "*", path},
	} {
		code, _, errOut := runCLI(testDeps(profile.NewMemoryStore()), args...)
		assert.Equal(t, 2, code, "%v", args)
		assert.Contains(t, errOut, "usage:", "%v", args)
	}
}

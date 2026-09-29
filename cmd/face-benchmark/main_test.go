package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

var model = strings.Repeat("cd", 32)

// body is a synthetic /face-measure 200 body (JSON text, decoded like a
// real one).
func body(wlr, alar float64) []byte {
	m := func(k string, v float64) string {
		return fmt.Sprintf(`{"key":%q,"value":%v,"unit":"ratio","band":null,"visibility":"observed","proxy":false,"landmarks":[],"reason":null}`, k, v)
	}
	return []byte(fmt.Sprintf(`{"catalogueVersion":"fa-measure/1","model":{"name":"face_landmarker","version":"test","manifestSha256":%q},`+
		`"quality":{"rollDeg":0,"yawDeg":0,"pitchDeg":0,"iodPx":100,"gatesPassed":[],"warnings":[],"regionDeltaE":{}},"regions":{},`+
		`"measurements":[%s,%s],"landmarks":null}`, model, m("width_length_ratio", wlr), m("alar_width_to_intercanthal_ratio", alar)))
}

// worker answers by image bytes.
type worker struct {
	bodies  map[string][]byte
	rejects map[string][]byte // 422 bodies by image bytes
	calls   int
}

func (w *worker) Catalogue(context.Context) ([]byte, error) { return facearchtest.CatalogueJSON(), nil }

func (w *worker) Measure(_ context.Context, image []byte, _ string) (int, []byte, error) {
	w.calls++
	if rb, ok := w.rejects[string(image)]; ok {
		return 422, rb, nil
	}
	b, ok := w.bodies[string(image)]
	if !ok {
		return 0, nil, errors.New("connection refused")
	}
	return 200, b, nil
}

// cliDoc and cliLabelMap: numbers are test inputs.
const cliDoc = `{
  "schemaVersion": 1, "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]},
  "measurementsShown": ["width_length_ratio"],
  "classifiers": [{"key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5, "classes": [
    {"label": "Long", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
    {"label": "Wide", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}]}],
  "traits": [{"key": "noseWidth", "rules": [
    {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]}],
    "default": "Average"}]
}`

const cliLabelMap = `{"attributes": {
  "faceShape": {"ours": "faceShape", "values": {"perfectcorp": {"Oblong": "Long", "Round": "Wide"}}},
  "noseWidth": {"ours": "noseWidth", "values": {"perfectcorp": {"Broad": "Broad", "Average": "Average"}}}
}}`

type files struct {
	dir, manifest, results, labelMap, profile, cache string
	w                                                *worker
}

// setup writes 20 subjects' images (placeholder bytes), their labels and a
// worker that knows all of them.
func setup(t *testing.T) files {
	t.Helper()
	dir := t.TempDir()
	w := &worker{bodies: map[string][]byte{}}
	man := []string{"image_id,image_path,subject_id,consent_ref,hijab,notes"}
	res := []string{"image_id,source,attribute,value"}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("img-%02d", i)
		content := "bytes of " + id
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".jpg"), []byte(content), 0o600))
		shape, wlr := "Oblong", 0.58+float64(i%4)/100
		if i%2 == 1 {
			shape, wlr = "Round", 0.88+float64(i%4)/100
		}
		nose, alar := "Average", 0.9+float64(i%5)/100
		if i%3 == 0 {
			nose, alar = "Broad", 1.3+float64(i%5)/100
		}
		w.bodies[content] = body(wlr, alar)
		man = append(man, fmt.Sprintf("%s,%s.jpg,subject-%02d,consent-%d,%t,", id, id, i, i, i%2 == 0))
		res = append(res, fmt.Sprintf("%s,perfectcorp,faceShape,%s", id, shape), fmt.Sprintf("%s,perfectcorp,noseWidth,%s", id, nose))
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}
	return files{
		dir:      dir,
		manifest: write("manifest.csv", strings.Join(man, "\n")+"\n"),
		results:  write("results.csv", strings.Join(res, "\n")+"\n"),
		labelMap: write("label-map.json", cliLabelMap),
		profile:  write("profile.json", cliDoc),
		cache:    filepath.Join(dir, "cache"),
		w:        w,
	}
}

func (f files) deps() deps {
	return deps{
		worker: func() (benchmark.Worker, string, error) { return f.w, "http://ai-worker.test", nil },
		dhash: func(path, _ string) (uint64, error) {
			// Distinct, far-apart hashes: no near-duplicates in this set.
			var h uint64 = 1469598103934665603
			for _, c := range filepath.Base(path) {
				h = (h ^ uint64(c)) * 1099511628211
			}
			return h, nil
		},
	}
}

func runCLI(d deps, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestMeasureEvaluateCalibrate(t *testing.T) {
	f := setup(t)
	common := []string{"--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model}

	code, out, errOut := runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "measuring 20 images with the ai-worker at http://ai-worker.test (nothing is sent anywhere else)")
	assert.Contains(t, out, "catalogue fa-measure/1: 20 images, 0 already cached, 0 failed, recorded measured 20")
	assert.Equal(t, 20, f.w.calls)

	code, out, _ = runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "20 already cached")
	assert.Equal(t, 20, f.w.calls, "cached images are not sent again")

	labelled := append(common, "--results", f.results, "--label-map", f.labelMap)
	reportDir := filepath.Join(f.dir, "report")
	code, out, errOut = runCLI(f.deps(), append(append([]string{"evaluate"}, labelled...), "--profile", f.profile, "--out", reportDir)...)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "wrote report.json and report.md")
	assert.FileExists(t, filepath.Join(reportDir, "report.json"))
	assert.FileExists(t, filepath.Join(reportDir, "report.md"))
	assert.NotContains(t, out+errOut, ".jpg", "no image paths in the output")

	outProfile := filepath.Join(f.dir, "calibrated.json")
	calArgs := append(append([]string{"calibrate"}, labelled...), "--profile", f.profile, "--target", "perfectcorp",
		"--out", outProfile, "--min-class-count", "2")
	code, out, errOut = runCLI(f.deps(), calArgs...)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "calibrated, run fb-")
	assert.Contains(t, out, "split 16/4 subjects")
	assert.Contains(t, out, "facearch-profile upsert "+outProfile)
	raw, err := os.ReadFile(outProfile)
	require.NoError(t, err)
	doc, err := profile.Parse(raw)
	require.NoError(t, err)
	require.NoError(t, profile.Validate(doc, facearchtest.Catalogue()))
	assert.Equal(t, profile.CalibrationCalibrated, doc.Calibration.Status)

	// Same inputs, same run id and profile.
	code, _, _ = runCLI(f.deps(), calArgs...)
	require.Equal(t, 0, code)
	again, err := os.ReadFile(outProfile)
	require.NoError(t, err)
	assert.Equal(t, string(raw), string(again))

	// The calibrated profile can itself be evaluated.
	code, _, errOut = runCLI(f.deps(), append(append([]string{"evaluate"}, labelled...), "--profile", outProfile, "--out", reportDir)...)
	require.Equal(t, 0, code, errOut)
}

func TestMeasureExitsOneWhenImagesFail(t *testing.T) {
	f := setup(t)
	delete(f.w.bodies, "bytes of img-03")
	code, _, errOut := runCLI(f.deps(), "measure", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "img-03: connection refused")
	assert.Contains(t, errOut, "run measure again")
}

func TestMeasureRedactsWorkerURLCredentials(t *testing.T) {
	f := setup(t)
	d := f.deps()
	d.worker = func() (benchmark.Worker, string, error) { return f.w, "http://user:secret@ai-worker.test", nil }
	code, out, errOut := runCLI(d, "measure", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model)
	require.Equal(t, 0, code, errOut)
	assert.NotContains(t, out+errOut, "secret")
	assert.Contains(t, out, "ai-worker.test")
}

func TestEvaluateNeedsMeasurements(t *testing.T) {
	f := setup(t)
	code, _, errOut := runCLI(f.deps(), "evaluate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model,
		"--results", f.results, "--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "run measure first")
}

func TestCalibrateReportsWhatCannotBeFitted(t *testing.T) {
	f := setup(t)
	common := []string{"--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model}
	code, _, _ := runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, code)
	out := filepath.Join(f.dir, "calibrated.json")
	code, _, errOut := runCLI(f.deps(), append(append([]string{"calibrate"}, common...), "--results", f.results, "--label-map", f.labelMap,
		"--profile", f.profile, "--target", "perfectcorp", "--out", out)...)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "cannot calibrate:")
	assert.Contains(t, errOut, "need 10", "the default minimum class count")
	assert.NoFileExists(t, out)
}

func TestCalibratePrintsCrossSubjectDuplicatesByImageID(t *testing.T) {
	f := setup(t)
	common := []string{"--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model}
	code, _, _ := runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, code)
	d := f.deps()
	// img-00 and img-01 belong to different subjects but hash the same.
	d.dhash = func(path, id string) (uint64, error) {
		if id == "img-01" {
			id = "img-00"
		}
		return f.deps().dhash(id+".jpg", id)
	}
	out := filepath.Join(f.dir, "calibrated.json")
	code, stdout, errOut := runCLI(d, append(append([]string{"calibrate"}, common...), "--results", f.results, "--label-map", f.labelMap,
		"--profile", f.profile, "--target", "perfectcorp", "--out", out, "--min-class-count", "2")...)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, stdout, "img-01")
	assert.Contains(t, stdout, "img-00")
	assert.Contains(t, stdout, "different subjects")
	assert.NotContains(t, stdout+errOut, ".jpg", "image ids only, no image paths")
}

func TestHashFailureNamesImageID(t *testing.T) {
	f := setup(t)
	common := []string{"--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model}
	code, _, _ := runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, code)
	d := f.deps()
	d.dhash = func(_, id string) (uint64, error) { return 0, fmt.Errorf("image %s: cannot decode image", id) }
	code, _, errOut := runCLI(d, append(append([]string{"calibrate"}, common...), "--results", f.results, "--label-map", f.labelMap,
		"--profile", f.profile, "--target", "perfectcorp", "--out", filepath.Join(f.dir, "c.json"))...)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "image img-00: cannot decode image")
	assert.NotContains(t, errOut, ".jpg")
}

func TestUsage(t *testing.T) {
	f := setup(t)
	for _, args := range [][]string{
		{},
		{"frobnicate"},
		{"measure"},
		{"measure", "--manifest", f.manifest, "--cache", f.cache},
		{"measure", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "extra"},
		{"evaluate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results},
		{"evaluate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--ece-bins", "0"},
		{"evaluate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--min-consensus-raters", "0"},
		{"calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir},
		{"calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--target", "vendorX"},
		{"calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--target", "perfectcorp", "--held-out-fraction", "1.5"},
		{"calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--target", "perfectcorp", "--dedup-max-distance", "65"},
		{"calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model, "--results", f.results,
			"--label-map", f.labelMap, "--profile", f.profile, "--out", f.dir, "--target", "perfectcorp", "--min-class-count", "0"},
	} {
		code, _, errOut := runCLI(f.deps(), args...)
		assert.Equal(t, 2, code, "%v", args)
		assert.True(t, strings.Contains(errOut, "usage:") || strings.Contains(errOut, "target must be"), "%v: %s", args, errOut)
	}
	assert.Equal(t, 0, f.w.calls, "usage errors do no work")
}

// rejectWith makes the worker answer 422 with the given detail code for one
// image's bytes.
func (w *worker) rejectWith(content, code string) {
	w.rejects[content] = []byte(fmt.Sprintf(`{"detail":{"code":%q}}`, code))
	delete(w.bodies, content)
}

func calibrateRunID(t *testing.T, code string) string {
	t.Helper()
	f := setup(t)
	f.w.rejects = map[string][]byte{}
	f.w.rejectWith("bytes of img-03", code)
	common := []string{"--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model}
	exit, _, errOut := runCLI(f.deps(), append([]string{"measure"}, common...)...)
	require.Equal(t, 0, exit, errOut)
	exit, out, errOut := runCLI(f.deps(), append(append([]string{"calibrate"}, common...), "--results", f.results, "--label-map", f.labelMap,
		"--profile", f.profile, "--target", "perfectcorp", "--out", filepath.Join(f.dir, "c.json"), "--min-class-count", "2")...)
	require.Equal(t, 0, exit, errOut)
	i := strings.Index(out, "run fb-")
	require.GreaterOrEqual(t, i, 0, out)
	return out[i : i+len("run fb-")+16]
}

func TestRunIDChangesWhenAnUnmeasuredImageOutcomeChanges(t *testing.T) {
	assert.Equal(t, calibrateRunID(t, "face_count"), calibrateRunID(t, "face_count"))
	assert.NotEqual(t, calibrateRunID(t, "face_count"), calibrateRunID(t, "quality_gates_failed"))
}

func TestCalibrateRefusesToOverwriteTheBaseProfile(t *testing.T) {
	f := setup(t)
	before, err := os.ReadFile(f.profile)
	require.NoError(t, err)
	code, _, errOut := runCLI(f.deps(), "calibrate", "--manifest", f.manifest, "--cache", f.cache, "--model-sha256", model,
		"--results", f.results, "--label-map", f.labelMap, "--profile", f.profile, "--target", "perfectcorp",
		"--out", filepath.Join(f.dir, "sub", "..", "profile.json"))
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "same file as --profile")
	after, err := os.ReadFile(f.profile)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

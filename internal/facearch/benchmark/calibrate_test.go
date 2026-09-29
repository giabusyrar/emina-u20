package benchmark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// calibrationSet is 40 subjects, one image each, alternating Long and
// Wide with a spread of values around 0.6 and 0.9, and noseWidth labels
// cycling Narrow / Average / Broad over matching alar ratios. Image 39
// is a near-duplicate of image 1 (hash one bit away) and is removed.
func calibrationSet(t *testing.T) (*fixture, map[string]uint64, string) {
	t.Helper()
	var imgs []img
	hashes := map[string]uint64{}
	var results strings.Builder
	results.WriteString("image_id,source,attribute,value\n")
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("img-%03d", i)
		shape, wlr := "Oblong", 0.6+float64(i%5-2)/100
		if i%2 == 1 {
			shape, wlr = "Round", 0.9+float64(i%5-2)/100
		}
		nose := []string{"Narrow", "Average", "Broad"}[i%3]
		alar := map[string]float64{"Narrow": 0.8, "Average": 1.0, "Broad": 1.2}[nose] + float64(i%7)/100
		imgs = append(imgs, img{id: id, subject: fmt.Sprintf("s%02d", i), hijab: i%4 == 0, status: 200,
			body: measureBody(map[string]any{"width_length_ratio": wlr, "alar_width_to_intercanthal_ratio": alar})})
		hashes[id] = spreadHash(i)
		fmt.Fprintf(&results, "%s,perfectcorp,faceShape,%s\n%s,perfectcorp,noseWidth,%s\n", id, shape, id, nose)
	}
	hashes["img-039"] = hashes["img-001"] ^ 1 // one bit from img-001
	// A gate-rejected image takes no part.
	imgs = append(imgs, img{id: "img-gate", subject: "s99", status: 422, body: gateBody})
	return newFixture(t, imgs), hashes, results.String()
}

// spreadHash gives image i a hash far (in Hamming distance) from the
// others: multiplication by the 64-bit golden-ratio constant scatters bits.
func spreadHash(i int) uint64 { return uint64(i+1) * 0x9E3779B97F4A7C15 }

func calibrateFixture(t *testing.T, params benchmark.Params) (*benchmark.CalibrateResult, error) {
	t.Helper()
	f, hashes, res := calibrationSet(t)
	base := f.profile(t, testDoc)
	return benchmark.Calibrate(benchmark.CalibrateInput{
		Items: f.items(t), Hashes: hashes, Base: base, Labels: f.labels(t, base, res),
		Target: "perfectcorp", Catalogue: facearchtest.Catalogue(), Params: params, RunID: "fb-0123456789abcdef",
	})
}

func testParams() benchmark.Params {
	p := benchmark.DefaultParams()
	p.MinClassCount = 3
	return p
}

func TestCalibrateWritesAValidCalibratedProfile(t *testing.T) {
	res, err := calibrateFixture(t, testParams())
	require.NoError(t, err)

	doc, err := profile.Parse(res.JSON)
	require.NoError(t, err)
	require.NoError(t, profile.Validate(doc, facearchtest.Catalogue()))
	assert.Equal(t, profile.CalibrationCalibrated, doc.Calibration.Status)
	assert.Equal(t, "fb-0123456789abcdef", *doc.Calibration.BenchmarkRunID)
	assert.Equal(t, "perfectcorp", *doc.Calibration.Target)

	// Every number is cited: the calibrated document would even pass the
	// uncalibrated source-coverage rule (Plan 2 Decision 13).
	doc.Calibration.Status = profile.CalibrationUncalibrated
	assert.NoError(t, profile.Validate(doc, facearchtest.Catalogue()))

	var fors []string
	for _, s := range res.Doc.Calibration.Sources {
		fors = append(fors, s.For)
	}
	assert.Contains(t, fors, "classifiers", "the vocabulary source is kept")
	assert.Contains(t, fors, "classifiers.faceShape.blendMargin", "unfitted values keep their source")
	// Fit owners are the fitted numbers only, so a class- or trait-level
	// base source (which also covers unfitted weights, requires, rule order
	// and labels) is kept beside the new per-number sources.
	assert.Contains(t, fors, "classifiers.faceShape.classes.Long", "class-level base source is kept")
	assert.Contains(t, fors, "traits.noseWidth.rules", "trait-level base source is kept")
	assert.Contains(t, fors, "classifiers.faceShape.classes.Long.prototype.width_length_ratio.centre")
	assert.Contains(t, fors, "classifiers.faceShape.classes.Long.prototype.width_length_ratio.spread")
	assert.Contains(t, fors, "traits.noseWidth.rules.0.when.0.value")

	var held benchmark.HeldOutMetrics
	require.NoError(t, json.Unmarshal(doc.Calibration.HeldOutMetrics, &held))
	assert.Equal(t, res.HeldOut, held)
	assert.Equal(t, "perfectcorp", held.Target)
	assert.Equal(t, testModel, held.ModelSHA256)
	assert.Equal(t, testParams(), held.Params)
	assert.Equal(t, benchmark.DedupSummary{Considered: 40, Removed: 1, CrossSubjectRemoved: 1}, held.Dedup)
	assert.Equal(t, 39, held.Split.TrainSubjects+held.Split.HeldOutSubjects)
	assert.Equal(t, 8, held.Split.HeldOutSubjects, "round(0.2 x 39)")
	assert.Equal(t, 39, held.Split.TrainImages+held.Split.HeldOutImages)
	fitted, base := held.Fitted["classifier:faceShape"], held.Base["classifier:faceShape"]
	assert.Equal(t, held.Split.HeldOutImages, fitted.Metrics.N)
	assert.Equal(t, held.Split.HeldOutImages, fitted.HeldOutLabelled)
	assert.Equal(t, fitted.Metrics.N, fitted.Answered)
	assert.Equal(t, 0, fitted.NoAnswer)
	assert.InDelta(t, 1.0, *fitted.Metrics.ExactAgreement, 1e-12, "the classes are well separated")
	assert.Equal(t, base.HeldOutLabelled, fitted.HeldOutLabelled, "base and fitted are scored on the same held-out images")
	assert.Equal(t, base.Metrics.N, fitted.Metrics.N)
	require.Contains(t, held.Fitted, "trait:noseWidth")
	assert.NotNil(t, held.Regressions)
	assert.Empty(t, held.Regressions, "fitted matches base on well-separated data")
	assert.Contains(t, string(res.Doc.Calibration.HeldOutMetrics), `"regressions":[]`)

	// Base sources above a fitted number say what the run superseded;
	// the others are unchanged.
	notes := map[string]string{}
	for _, s := range res.Doc.Calibration.Sources {
		notes[s.For] = s.Note
	}
	prefix := benchmark.SupersededPrefix(held.BaseProfileSHA, "fb-0123456789abcdef")
	assert.True(t, strings.HasPrefix(notes["classifiers.faceShape.classes.Long"], prefix))
	assert.True(t, strings.HasPrefix(notes["traits.noseWidth.rules"], prefix))
	assert.Equal(t, "", notes["classifiers.faceShape.blendMargin"], "not an ancestor of a fitted number: unchanged")

	// The cross-subject near-duplicate is reported for the operator (a
	// likely labelling error), by image id, never in the written profile.
	require.Len(t, res.CrossSubjectDuplicates, 1)
	d := res.CrossSubjectDuplicates[0]
	assert.Equal(t, "img-039", d.ImageID)
	assert.Equal(t, "img-001", d.KeptID)
	assert.Equal(t, 1, d.Distance)
	assert.True(t, d.CrossSubject)
	for _, s := range []string{"img-0", "img-gate", ".jpg", "consent-"} {
		assert.NotContains(t, string(res.JSON), s, "the written profile carries no image ids or paths")
	}
}

func TestCalibrateIsReproducible(t *testing.T) {
	a, err := calibrateFixture(t, testParams())
	require.NoError(t, err)
	b, err := calibrateFixture(t, testParams())
	require.NoError(t, err)
	assert.Equal(t, string(a.JSON), string(b.JSON))
}

func TestCalibrateRefusals(t *testing.T) {
	p := testParams()
	p.MinClassCount = 50
	_, err := calibrateFixture(t, p)
	var fe *benchmark.FitError
	assert.ErrorAs(t, err, &fe)

	p = testParams()
	p.HeldOutFraction = 0
	_, err = calibrateFixture(t, p)
	assert.ErrorContains(t, err, "invalid params")

	f, hashes, res := calibrationSet(t)
	base := f.profile(t, testDoc)
	in := benchmark.CalibrateInput{Items: f.items(t), Hashes: hashes, Base: base, Labels: f.labels(t, base, res),
		Target: "wardah", Catalogue: facearchtest.Catalogue(), Params: testParams(), RunID: "fb-x"}
	_, err = benchmark.Calibrate(in)
	assert.ErrorContains(t, err, "target wardah labelled no attribute")
	in.Target = "someone"
	_, err = benchmark.Calibrate(in)
	assert.ErrorContains(t, err, "target must be")
	assert.ErrorContains(t, err, benchmark.TargetRaterConsensus)
	in.Target = "perfectcorp"

	mismatch := in
	mismatch.Params.MinConsensusRaters = 3
	_, err = benchmark.Calibrate(mismatch)
	assert.ErrorContains(t, err, "MinConsensusRaters")

	noRun := in
	noRun.RunID = ""
	_, err = benchmark.Calibrate(noRun)
	assert.ErrorContains(t, err, "run id")

	delete(in.Hashes, "img-005")
	_, err = benchmark.Calibrate(in)
	assert.ErrorContains(t, err, "img-005: no perceptual hash")
	assert.NotContains(t, err.Error(), f.dir, "errors never carry image paths")
}

// Held-out images never reach the fit: the held-out subjects' measurements
// are set far outside the training range, so any leak would move the
// fitted centres and thresholds. The fitted centre must equal the mean of
// the training values alone, and every threshold must lie inside the
// training range.
func TestCalibrateFitsOnTrainingSubjectsOnly(t *testing.T) {
	p := testParams()
	const n = 30
	var subjects []string
	for i := 0; i < n; i++ {
		subjects = append(subjects, fmt.Sprintf("s%02d", i))
	}
	split, err := benchmark.SplitSubjects(subjects, p.HeldOutFraction, p.Seed)
	require.NoError(t, err)
	heldOut := map[string]bool{}
	for _, s := range split.HeldOut {
		heldOut[s] = true
	}
	require.NotEmpty(t, heldOut)

	const leak = 50.0 // far outside every training value
	var imgs []img
	hashes := map[string]uint64{}
	var results strings.Builder
	results.WriteString("image_id,source,attribute,value\n")
	var trainLong, allLong []float64
	maxTrainAlar := 0.0
	for i := 0; i < n; i++ {
		id, subject := fmt.Sprintf("img-%03d", i), subjects[i]
		shape, wlr := "Oblong", 0.6+float64(i%5-2)/100
		if i%2 == 1 {
			shape, wlr = "Round", 0.9+float64(i%5-2)/100
		}
		nose := []string{"Narrow", "Average", "Broad"}[i%3]
		alar := map[string]float64{"Narrow": 0.8, "Average": 1.0, "Broad": 1.2}[nose] + float64(i%7)/100
		if heldOut[subject] {
			wlr, alar = wlr+leak, alar+leak
		} else {
			maxTrainAlar = max(maxTrainAlar, alar)
			if shape == "Oblong" {
				trainLong = append(trainLong, wlr)
			}
		}
		if shape == "Oblong" {
			allLong = append(allLong, wlr)
		}
		imgs = append(imgs, img{id: id, subject: subject, status: 200,
			body: measureBody(map[string]any{"width_length_ratio": wlr, "alar_width_to_intercanthal_ratio": alar})})
		hashes[id] = spreadHash(i)
		fmt.Fprintf(&results, "%s,perfectcorp,faceShape,%s\n%s,perfectcorp,noseWidth,%s\n", id, shape, id, nose)
	}
	f := newFixture(t, imgs)
	base := f.profile(t, testDoc)
	res, err := benchmark.Calibrate(benchmark.CalibrateInput{Items: f.items(t), Hashes: hashes, Base: base,
		Labels: f.labels(t, base, results.String()), Target: "perfectcorp", Catalogue: facearchtest.Catalogue(),
		Params: p, RunID: "fb-leak"})
	require.NoError(t, err)

	mean := func(xs []float64) float64 {
		s := 0.0
		for _, x := range xs {
			s += x
		}
		return s / float64(len(xs))
	}
	var long profile.ClassSpec
	for _, cl := range res.Doc.Classifiers[0].Classes {
		if cl.Label == "Long" {
			long = cl
		}
	}
	centre := long.Prototype["width_length_ratio"].Centre
	assert.InDelta(t, mean(trainLong), centre, 1e-12, "the centre is the training mean")
	assert.Greater(t, mean(allLong)-centre, 1.0, "a leak would have pulled the centre up")
	for _, r := range res.Doc.Traits[0].Rules {
		for _, c := range r.When {
			assert.LessOrEqual(t, c.Value, maxTrainAlar, "thresholds lie inside the training range")
		}
	}
	assert.Equal(t, len(split.HeldOut), res.HeldOut.Split.HeldOutSubjects)
	assert.Equal(t, len(split.HeldOut), res.HeldOut.Split.HeldOutImages)
	assert.Equal(t, n-len(split.HeldOut), res.HeldOut.Split.TrainImages)
	for _, s := range res.Doc.Calibration.Sources {
		if s.For == "classifiers.faceShape.classes.Long.prototype.width_length_ratio.centre" {
			assert.Contains(t, s.Note, fmt.Sprintf("over %d training images", len(trainLong)))
		}
	}
}

// The shipped seed (Plan 2 + Plan 3: eight classes, ten traits, guidance)
// calibrates into a valid profile whose guidance section is unchanged.
func TestCalibrateTheShippedSeed(t *testing.T) {
	seedPath := "../../../cmd/facearch-profile/seed/seagull-default-uncalibrated.json"
	raw, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	seed, err := profile.Parse(raw)
	require.NoError(t, err)
	cat := facearchtest.Catalogue()

	var scalars []string
	for _, e := range cat.Measurements {
		if s, err := cat.Shape(e.Key); err == nil && s == "scalar" && e.Supported {
			scalars = append(scalars, e.Key)
		}
	}
	answers := map[string][]string{}
	for _, c := range seed.Classifiers[0].Classes {
		answers["faceShape"] = append(answers["faceShape"], c.Label)
	}
	for _, tr := range seed.Traits {
		set := map[string]bool{tr.Default: true}
		answers[tr.Key] = append(answers[tr.Key], tr.Default)
		for _, r := range tr.Rules {
			if !set[r.Label] {
				set[r.Label] = true
				answers[tr.Key] = append(answers[tr.Key], r.Label)
			}
		}
	}
	var imgs []img
	hashes := map[string]uint64{}
	var results strings.Builder
	results.WriteString("image_id,source,attribute,value\n")
	labelMap := map[string]any{}
	for key, labels := range answers {
		vocab := map[string]string{}
		for _, l := range labels {
			vocab[l] = l
		}
		labelMap[key] = map[string]any{"ours": key, "values": map[string]any{"perfectcorp": vocab}}
	}
	for i := 0; i < 120; i++ {
		id := fmt.Sprintf("img-%03d", i)
		values := map[string]any{}
		for k, key := range scalars {
			// Deterministic, spread, distinct per image.
			values[key] = 0.5 + float64((i*37+k*11)%97)/97
		}
		imgs = append(imgs, img{id: id, subject: fmt.Sprintf("s%03d", i), status: 200, body: measureBody(values)})
		hashes[id] = spreadHash(i)
		for key, labels := range answers {
			fmt.Fprintf(&results, "%s,perfectcorp,%s,%s\n", id, key, labels[i%len(labels)])
		}
	}
	f := newFixture(t, imgs)
	base := f.profile(t, string(raw))
	rows, err := benchmark.ReadManifest(f.manifest)
	require.NoError(t, err)
	rr, err := benchmark.ReadResults(writeFile(t, f.dir, "results.csv", results.String()), rows)
	require.NoError(t, err)
	lmRaw, err := json.Marshal(map[string]any{"attributes": labelMap})
	require.NoError(t, err)
	lm, err := benchmark.ReadLabelMap(writeFile(t, f.dir, "label-map.json", string(lmRaw)))
	require.NoError(t, err)
	targets, err := lm.Resolve(base.Doc)
	require.NoError(t, err)
	ls, err := benchmark.BuildLabels(rr, lm, targets, 2)
	require.NoError(t, err)

	p := benchmark.DefaultParams()
	p.MinClassCount = 2
	res, err := benchmark.Calibrate(benchmark.CalibrateInput{Items: f.items(t), Hashes: hashes, Base: base, Labels: ls,
		Target: "perfectcorp", Catalogue: cat, Params: p, RunID: "fb-seed"})
	require.NoError(t, err)
	doc, err := profile.Parse(res.JSON)
	require.NoError(t, err)
	require.NoError(t, profile.Validate(doc, cat))
	wantGuidance, err := json.Marshal(seed.Guidance)
	require.NoError(t, err)
	gotGuidance, err := json.Marshal(doc.Guidance)
	require.NoError(t, err)
	assert.JSONEq(t, string(wantGuidance), string(gotGuidance))
	assert.NotEqual(t, seed.Classifiers[0].Classes[0].Prototype, doc.Classifiers[0].Classes[0].Prototype)

	// Seed notes that quote replaced numbers are marked superseded when
	// their path is above a fitted number; others keep their note as-is.
	seedNotes := map[string]string{}
	for _, s := range seed.Calibration.Sources {
		seedNotes[s.For] = s.Note
	}
	prefix := benchmark.SupersededPrefix(base.SHA256, "fb-seed")
	got := map[string]string{}
	for _, s := range doc.Calibration.Sources {
		if _, ok := seedNotes[s.For]; ok {
			got[s.For] = s.Note
		}
	}
	for _, p := range []string{
		"classifiers.faceShape.classes.Triangle.prototype",
		"classifiers.faceShape.classes.Oval",
		"traits.noseWidth.rules",
		"traits.eyeShape.rules",
		// Names a rule by label: an ancestor of that rule's fitted values.
		"traits.lipShape.rules.Wide",
	} {
		require.Contains(t, seedNotes, p)
		assert.Equal(t, prefix+seedNotes[p], got[p], p)
	}
	for _, p := range []string{
		"classifiers.faceShape.blendMargin",
		"classifiers.faceShape.minObservedWeight",
		"guidance.rules.round-contour",
	} {
		require.Contains(t, seedNotes, p)
		assert.Equal(t, seedNotes[p], got[p], p)
	}
}

// Calibrating an already calibrated profile replaces the superseded
// prefix instead of stacking a second one.
func TestCalibrateRecalibrationKeepsOnePrefix(t *testing.T) {
	f, hashes, results := calibrationSet(t)
	cat := facearchtest.Catalogue()
	run := func(base *benchmark.ProfileFile, runID string) *benchmark.CalibrateResult {
		res, err := benchmark.Calibrate(benchmark.CalibrateInput{Items: f.items(t), Hashes: hashes, Base: base,
			Labels: f.labels(t, base, results), Target: "perfectcorp", Catalogue: cat, Params: testParams(), RunID: runID})
		require.NoError(t, err)
		return res
	}
	first := run(f.profile(t, testDoc), "fb-first")
	second := f.profile(t, string(first.JSON))
	res := run(second, "fb-second")

	want := benchmark.SupersededPrefix(second.SHA256, "fb-second")
	checked := 0
	for _, s := range res.Doc.Calibration.Sources {
		n := strings.Count(s.Note, "superseded by face-benchmark run")
		switch s.For {
		case "classifiers.faceShape.classes.Long", "traits.noseWidth.rules":
			assert.Equal(t, want, s.Note, s.For)
			checked++
		}
		assert.LessOrEqual(t, n, 1, s.For)
		assert.NotContains(t, s.Note, "fb-first", s.For)
	}
	assert.Equal(t, 2, checked)
}

func heldOutItems(ids ...string) []benchmark.Item {
	var out []benchmark.Item
	for _, id := range ids {
		out = append(out, benchmark.Item{Row: benchmark.ManifestRow{ImageID: id, SubjectID: "s-" + id}, Outcome: benchmark.OutcomeMeasured})
	}
	return out
}

func heldOutAnswer(label string) benchmark.Answer {
	s := 0.9
	return benchmark.Answer{Status: "single", Label: label, Top2: []string{label}, Score: &s}
}

var (
	faceShape    = benchmark.Target{Kind: benchmark.KindClassifier, Key: "faceShape"}
	heldOutTruth = map[string]string{"a": "Long", "b": "Wide", "c": "Long", "d": "Wide", "e": "Long", "f": "Wide"}
)

func allCorrect() map[string]map[benchmark.Target]benchmark.Answer {
	out := map[string]map[benchmark.Target]benchmark.Answer{}
	for id, l := range heldOutTruth {
		out[id] = map[benchmark.Target]benchmark.Answer{faceShape: heldOutAnswer(l)}
	}
	return out
}

// A fitted profile that abstains where the base answers is scored on
// fewer images; the counts make that visible instead of hiding it.
func TestCompareHeldOutCountsAbstentions(t *testing.T) {
	items := heldOutItems("a", "b", "c", "d", "e", "f", "unlabelled")
	base := allCorrect()
	fitted := allCorrect()
	fitted["d"][faceShape] = benchmark.Answer{Status: "insufficient_evidence"}

	b := benchmark.CompareHeldOut(items, base, faceShape, heldOutTruth, 10)
	f := benchmark.CompareHeldOut(items, fitted, faceShape, heldOutTruth, 10)
	assert.Equal(t, benchmark.HeldOutScore{HeldOutLabelled: 6, Answered: 6, NoAnswer: 0, Metrics: b.Metrics}, b)
	assert.Equal(t, benchmark.HeldOutScore{HeldOutLabelled: 6, Answered: 5, NoAnswer: 1, Metrics: f.Metrics}, f)
	assert.Equal(t, 5, f.Metrics.N)
}

func TestRegressions(t *testing.T) {
	items := heldOutItems("a", "b", "c", "d", "e", "f")
	base := map[string]benchmark.HeldOutScore{"classifier:faceShape": benchmark.CompareHeldOut(items, allCorrect(), faceShape, heldOutTruth, 10)}

	same := map[string]benchmark.HeldOutScore{"classifier:faceShape": benchmark.CompareHeldOut(items, allCorrect(), faceShape, heldOutTruth, 10)}
	none := benchmark.Regressions(same, base)
	assert.NotNil(t, none)
	assert.Empty(t, none)

	worse := allCorrect()
	worse["d"][faceShape] = heldOutAnswer("Long")
	regressed := map[string]benchmark.HeldOutScore{"classifier:faceShape": benchmark.CompareHeldOut(items, worse, faceShape, heldOutTruth, 10)}
	got := benchmark.Regressions(regressed, base)
	require.Len(t, got, 2)
	assert.Equal(t, "classifier:faceShape", got[0].Attribute)
	assert.Equal(t, "macroF1", got[0].Metric)
	assert.Equal(t, "kappa", got[1].Metric)
	assert.Less(t, got[0].Fitted, got[0].Base)
	assert.Less(t, got[1].Fitted, got[1].Base)

	// An undefined (nil) metric is not compared.
	empty := map[string]benchmark.HeldOutScore{"classifier:faceShape": benchmark.CompareHeldOut(items, map[string]map[benchmark.Target]benchmark.Answer{}, faceShape, heldOutTruth, 10)}
	assert.Empty(t, benchmark.Regressions(empty, base))
}

package benchmark_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// evalImages: with testDoc (Long 0.6 / Wide 0.9, spread 0.1; noseWidth
// Broad > 1.1, Narrow < 0.9):
//
//	img-001 no hijab  wlr 0.60 alar 0.8  -> Long (single), Narrow
//	img-002 no hijab  wlr 0.90 alar 1.2  -> Wide (single), Broad
//	img-003 hijab     wlr 0.75 alar 1.0  -> blend Long/Wide (tie, class order), Average
//	img-004 hijab     wlr occluded       -> insufficient_evidence, Average
//	img-005 hijab     422 quality gates
//	img-006 no hijab  never measured
func evalImages() []img {
	return []img{
		{id: "img-001", subject: "s1", status: 200, body: measureBody(map[string]any{"width_length_ratio": 0.60, "alar_width_to_intercanthal_ratio": 0.8})},
		{id: "img-002", subject: "s2", status: 200, body: measureBody(map[string]any{"width_length_ratio": 0.90, "alar_width_to_intercanthal_ratio": 1.2})},
		{id: "img-003", subject: "s3", hijab: true, status: 200, body: measureBody(map[string]any{"width_length_ratio": 0.75, "alar_width_to_intercanthal_ratio": 1.0})},
		{id: "img-004", subject: "s4", hijab: true, status: 200, body: measureBody(map[string]any{"width_length_ratio": "occluded", "alar_width_to_intercanthal_ratio": 1.0})},
		{id: "img-005", subject: "s5", hijab: true, status: 422, body: gateBody},
		{id: "img-006", subject: "s6"},
	}
}

const evalResults = `image_id,source,attribute,value
img-001,perfectcorp,faceShape,Oblong
img-002,perfectcorp,faceShape,Round
img-003,perfectcorp,faceShape,Round
img-004,perfectcorp,faceShape,Oblong
img-005,perfectcorp,faceShape,Round
img-006,perfectcorp,faceShape,Unknown
img-001,perfectcorp,noseWidth,Narrow
img-002,perfectcorp,noseWidth,Average
img-003,perfectcorp,noseWidth,Average
img-004,perfectcorp,noseWidth,Broad
img-001,rater:r1,faceShape,long
img-002,rater:r1,faceShape,wide
img-003,rater:r1,faceShape,wide
img-001,rater:r2,faceShape,long
img-002,rater:r2,faceShape,long
img-003,rater:r2,faceShape,wide
img-001,wardah,faceBalance,Curved Thin
`

func evaluateFixtureWith(t *testing.T, params benchmark.Params) (*fixture, *benchmark.Report, error) {
	t.Helper()
	f := newFixture(t, evalImages())
	p := f.profile(t, testDoc)
	r, err := benchmark.Evaluate(benchmark.EvaluateInput{
		Items: f.items(t), Profile: p, Labels: f.labels(t, p, evalResults),
		ModelSHA256: testModel, CatalogueVersion: "fa-measure/1", Params: params,
	})
	return f, r, err
}

func evaluateFixture(t *testing.T) *benchmark.Report {
	t.Helper()
	_, r, err := evaluateFixtureWith(t, benchmark.DefaultParams())
	require.NoError(t, err)
	return r
}

func TestEvaluateRecordsAndValidatesParams(t *testing.T) {
	p := benchmark.DefaultParams()
	p.ECEBins, p.Seed = 5, 42
	_, r, err := evaluateFixtureWith(t, p)
	require.NoError(t, err)
	assert.Equal(t, p, r.Params, "the params actually used are recorded")
	assert.Equal(t, 5, r.Agreement[0].Slices[benchmark.SliceAll].ECEBins)

	bad := benchmark.DefaultParams()
	bad.ECEBins = 0
	_, r, err = evaluateFixtureWith(t, bad)
	require.Error(t, err)
	assert.Nil(t, r)
	assert.Contains(t, err.Error(), "ECEBins")
}

func TestEvaluateReportIsJSONEncodable(t *testing.T) {
	r := evaluateFixture(t)
	raw, err := json.Marshal(r)
	require.NoError(t, err, "no NaN or Inf anywhere in the report")
	assert.NotContains(t, string(raw), "NaN")
	// Undefined metrics stay null, never 0: perfectcorp noseWidth has no
	// scores, so ECE is null.
	assert.Contains(t, string(raw), `"ece":null`)
}

func row(t *testing.T, r *benchmark.Report, attr, source string) benchmark.AgreementRow {
	t.Helper()
	for _, a := range r.Agreement {
		if a.Attribute == attr && a.Source == source {
			return a
		}
	}
	t.Fatalf("no agreement row for %s / %s", attr, source)
	return benchmark.AgreementRow{}
}

func TestEvaluateAgreement(t *testing.T) {
	r := evaluateFixture(t)
	assert.Equal(t, 6, r.Images)
	assert.Equal(t, 6, r.Subjects)
	assert.Equal(t, "uncalibrated", r.Profile.CalibrationStatus)

	fs := row(t, r, "faceShape", "perfectcorp")
	assert.Equal(t, benchmark.Target{Kind: "classifier", Key: "faceShape"}, fs.Target)
	assert.Equal(t, 5, fs.Labelled)
	assert.Equal(t, 1, fs.NotMeasured, "img-005 was gate-rejected")
	assert.Equal(t, 1, fs.OursNoAnswer, "img-004 is insufficient_evidence")
	assert.Equal(t, 1, fs.Abstained, "Unknown maps to null")
	all := fs.Slices[benchmark.SliceAll]
	assert.Equal(t, 3, all.N)
	assert.InDelta(t, 2.0/3.0, *all.ExactAgreement, 1e-12)
	assert.InDelta(t, 1.0, *all.Top2Agreement, 1e-12, "img-003's blend holds Wide")
	// truth Long Wide Wide, ours Long Wide Long: po 2/3, pe 4/9, kappa 0.4.
	assert.InDelta(t, 0.4, *all.Kappa, 1e-12)
	require.NotNil(t, all.ECE)
	assert.Equal(t, benchmark.Confusion{Labels: []string{"Long", "Wide"}, Counts: [][]int{{1, 0}, {1, 1}}}, all.Confusion)
	assert.Equal(t, 1, fs.Slices[benchmark.SliceHijab].N)
	assert.InDelta(t, 0.0, *fs.Slices[benchmark.SliceHijab].ExactAgreement, 1e-12)
	assert.InDelta(t, 1.0, *fs.Slices[benchmark.SliceNoHijab].ExactAgreement, 1e-12)

	nw := row(t, r, "noseWidth", "perfectcorp")
	m := nw.Slices[benchmark.SliceAll]
	assert.Equal(t, 4, m.N)
	assert.InDelta(t, 0.5, *m.ExactAgreement, 1e-12)
	assert.Nil(t, m.ECE, "traits have no scores")

	cons := row(t, r, "faceShape", benchmark.TargetRaterConsensus)
	assert.Equal(t, 2, cons.Labelled, "img-002's raters split 1-1")
	assert.Equal(t, map[string]int{"faceShape": 1}, r.NoConsensus)
	assert.Equal(t, map[string]int{"faceBalance": 1}, r.UnmappedAttributes)
}

func TestEvaluateSourceVsSource(t *testing.T) {
	r := evaluateFixture(t)
	type pair struct{ a, b string }
	type stat struct {
		n     int
		exact float64
		kappa float64
	}
	got := map[pair]stat{}
	for _, p := range r.SourceAgreement {
		assert.Equal(t, "faceShape", p.Attribute)
		require.NotNil(t, p.ExactAgreement)
		require.NotNil(t, p.Kappa)
		got[pair{p.SourceA, p.SourceB}] = stat{p.N, *p.ExactAgreement, *p.Kappa}
	}
	// N is the images both sources labelled, not the images we answered.
	// perfectcorp vs r1: Long Wide Wide both, kappa 1. r1 vs r2: po 2/3,
	// pe 4/9, kappa 0.4. consensus holds img-001 and img-003 only.
	want := map[pair]stat{
		{"perfectcorp", "rater-consensus"}: {2, 1, 1},
		{"perfectcorp", "rater:r1"}:        {3, 1, 1},
		{"perfectcorp", "rater:r2"}:        {3, 2.0 / 3.0, 0.4},
		{"rater:r1", "rater:r2"}:           {3, 2.0 / 3.0, 0.4},
	}
	require.Len(t, got, len(want), "the consensus is never compared with the raters it is built from")
	for k, w := range want {
		g, ok := got[k]
		require.True(t, ok, k)
		assert.Equal(t, w.n, g.n, k)
		assert.InDelta(t, w.exact, g.exact, 1e-12, k)
		assert.InDelta(t, w.kappa, g.kappa, 1e-12, k)
	}
}

func TestEvaluateRefusesLabelsBuiltWithOtherConsensusRule(t *testing.T) {
	p := benchmark.DefaultParams()
	p.MinConsensusRaters = 3 // labels below are built with the default, 2
	_, r, err := evaluateFixtureWith(t, p)
	require.Error(t, err)
	assert.Nil(t, r)
	assert.Contains(t, err.Error(), "3")
	assert.Contains(t, err.Error(), "2")
	assert.Contains(t, err.Error(), "MinConsensusRaters")
}

func TestEvaluateRefusesLabelsForAnotherProfile(t *testing.T) {
	f := newFixture(t, evalImages())
	p := f.profile(t, testDoc)
	ls := f.labels(t, p, evalResults)
	assert.Equal(t, benchmark.DefaultMinConsensusRaters, ls.MinConsensusRaters)
	for _, tgt := range []benchmark.Target{{Kind: benchmark.KindClassifier, Key: "jawShape"}, {Kind: benchmark.KindTrait, Key: "lipFullness"}} {
		ls.Targets = map[string]benchmark.Target{"other": tgt}
		r, err := benchmark.Evaluate(benchmark.EvaluateInput{Items: f.items(t), Profile: p, Labels: ls,
			ModelSHA256: testModel, CatalogueVersion: "fa-measure/1", Params: benchmark.DefaultParams()})
		require.Error(t, err)
		assert.Nil(t, r)
		assert.Contains(t, err.Error(), tgt.Key)
	}
}

func TestEvaluateCoverageByHijab(t *testing.T) {
	r := evaluateFixture(t)
	all, hijab, bare := r.Coverage[benchmark.SliceAll], r.Coverage[benchmark.SliceHijab], r.Coverage[benchmark.SliceNoHijab]
	assert.Equal(t, 6, all.Images)
	assert.Equal(t, 4, all.Outcomes[benchmark.OutcomeMeasured])
	assert.Equal(t, 1, all.Outcomes[benchmark.OutcomeGateRejected])
	assert.Equal(t, 1, all.Outcomes[benchmark.OutcomeNotMeasured])
	assert.InDelta(t, 0.2, *all.GateRejectionRate, 1e-12, "1 of the 5 images the worker answered")
	assert.InDelta(t, 1.0/3.0, *hijab.GateRejectionRate, 1e-12)
	assert.InDelta(t, 0.0, *bare.GateRejectionRate, 1e-12)

	fs := all.Answers["classifier:faceShape"]
	assert.Equal(t, 4, fs.Measured)
	assert.Equal(t, map[string]int{"single": 2, "blend": 1, "insufficient_evidence": 1}, fs.Statuses)
	assert.InDelta(t, 0.25, *fs.NoAnswerRate, 1e-12)
	assert.InDelta(t, 0.5, *hijab.Answers["classifier:faceShape"].NoAnswerRate, 1e-12)
	assert.InDelta(t, 0.0, *bare.Answers["classifier:faceShape"].NoAnswerRate, 1e-12)
	assert.Equal(t, map[string]int{"assessed": 4}, all.Answers["trait:noseWidth"].Statuses)
}

func TestWriteReportHoldsNoPathsOrIDs(t *testing.T) {
	f, r, err := evaluateFixtureWith(t, benchmark.DefaultParams())
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, benchmark.WriteReport(dir, r))
	leaks := []string{"img-00", ".jpg", "consent-", "s1", string(filepath.Separator) + "cache"}
	for _, it := range f.items(t) {
		leaks = append(leaks, it.Row.ImageID, it.Row.ImagePath, filepath.ToSlash(it.Row.ImagePath), it.Row.SubjectID, it.Row.ConsentRef)
	}
	leaks = append(leaks, f.dir, filepath.ToSlash(f.dir))
	for _, name := range []string{benchmark.ReportJSONName, benchmark.ReportMarkdownName} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		for _, leak := range leaks {
			assert.NotContains(t, string(raw), leak, name)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, benchmark.ReportJSONName))
	require.NoError(t, err)
	var back benchmark.Report
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, r.Agreement[0].Slices[benchmark.SliceAll].N, back.Agreement[0].Slices[benchmark.SliceAll].N)

	md, err := os.ReadFile(filepath.Join(dir, benchmark.ReportMarkdownName))
	require.NoError(t, err)
	for _, want := range []string{
		"# Face benchmark report",
		"calibration **uncalibrated**",
		"Vendor labels are not ground truth",
		"source-vs-source",
		"every image both sources labelled",
		"agreement between two sources is the practical ceiling for our agreement with either",
		"ECE bins: 10",
		"held-out fraction",
		"| hijab | 3 | 2 | 1 | 0 | 0 | 0 | 0 | 0 | 33.3% |",
		"| faceShape | perfectcorp | all | 3 | 66.7% | 100.0% |",
		"### faceShape vs perfectcorp",
		"| faceShape | rater:r1 | rater:r2 | 3 | 66.7% |",
		"Not compared (attribute not in the label map), rows: faceBalance 1;",
	} {
		assert.Contains(t, string(md), want)
	}
}

// A calibrated profile scored on the image set it was fitted on is scored
// in-sample; the report must say so, and say where the out-of-sample
// figures are.
func TestReportMarksCalibratedProfilesInSample(t *testing.T) {
	_, r, err := evaluateFixtureWith(t, benchmark.DefaultParams())
	require.NoError(t, err)
	assert.NotContains(t, r.Markdown(), "in-sample", "an uncalibrated profile was fitted on nothing")

	r.Profile.CalibrationStatus = profile.CalibrationCalibrated
	md := r.Markdown()
	assert.Contains(t, md, "calibration **calibrated**")
	assert.Contains(t, md, "**Figures on the image set this profile was calibrated on are in-sample.**")
	assert.Contains(t, md, "only calibrate's `heldOutMetrics` are out-of-sample")
}

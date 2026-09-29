package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// RunID derives a run id from everything that shapes a calibrated profile,
// so the same inputs always give the same id and profile.
func RunID(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		sum := sha256.Sum256(p)
		h.Write(sum[:])
	}
	return "fb-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// DedupSummary counts what near-duplicate removal did.
type DedupSummary struct {
	Considered int `json:"considered"`
	Removed    int `json:"removed"`
	// CrossSubjectRemoved counts removed images whose kept near-duplicate
	// belongs to a different subject_id: the same photo under two subjects
	// is most likely a labelling error. The pairs themselves (by image id)
	// are in CalibrateResult.CrossSubjectDuplicates, never in the profile.
	CrossSubjectRemoved int `json:"crossSubjectRemoved"`
}

// SplitSummary counts the subject split.
type SplitSummary struct {
	TrainSubjects   int `json:"trainSubjects"`
	HeldOutSubjects int `json:"heldOutSubjects"`
	TrainImages     int `json:"trainImages"`
	HeldOutImages   int `json:"heldOutImages"`
}

// HeldOutMetrics is written into calibration.heldOutMetrics. It carries
// counts and metrics only: no image ids, no image paths, no per-image rows.
type HeldOutMetrics struct {
	Target           string       `json:"target"`
	ModelSHA256      string       `json:"modelSha256"`
	CatalogueVersion string       `json:"catalogueVersion"`
	BaseProfileSHA   string       `json:"baseProfileSha256"`
	Params           Params       `json:"params"`
	Dedup            DedupSummary `json:"dedup"`
	Split            SplitSummary `json:"split"`
	// Fitted and Base are the calibrated and the base profile on the same
	// held-out images, against the target, keyed "classifier:<key>" or
	// "trait:<key>".
	Fitted map[string]Metrics `json:"fitted"`
	Base   map[string]Metrics `json:"base"`
}

// CalibrateInput is everything calibrate needs, already loaded.
type CalibrateInput struct {
	Items []Item
	// Hashes holds the dHash of every measured item, by image id.
	Hashes    map[string]uint64
	Base      *ProfileFile
	Labels    *LabelSet
	Target    string
	Catalogue *measureclient.Catalogue
	Params    Params
	RunID     string
}

// CalibrateResult is the calibrated document and how it was obtained.
type CalibrateResult struct {
	Doc     *profile.Document
	JSON    []byte
	HeldOut HeldOutMetrics
	// CrossSubjectDuplicates lists the removed near-duplicates whose kept
	// image has a different subject_id (likely labelling errors), by image
	// id, for the operator to review. It is not written into the profile.
	CrossSubjectDuplicates []Duplicate
}

// ValidTarget reports whether t can be a calibration target.
func ValidTarget(t string) bool { return t == TargetRaterConsensus || ValidSource(t) }

// Calibrate removes near-duplicates, splits by subject, fits the base
// profile on the training subjects only, scores fitted and base profiles
// on the held-out subjects, and returns a calibrated document that passes
// profile.Validate against the catalogue.
func Calibrate(in CalibrateInput) (*CalibrateResult, error) {
	if err := in.Params.Validate(); err != nil {
		return nil, fmt.Errorf("calibrate: invalid params: %w", err)
	}
	if in.Base == nil || in.Labels == nil || in.Catalogue == nil {
		return nil, errors.New("calibrate: base profile, labels and catalogue are required")
	}
	if in.RunID == "" {
		return nil, errors.New("calibrate: run id is required")
	}
	if in.Labels.MinConsensusRaters != in.Params.MinConsensusRaters {
		return nil, fmt.Errorf("calibrate: labels were built with MinConsensusRaters %d but Params.MinConsensusRaters is %d",
			in.Labels.MinConsensusRaters, in.Params.MinConsensusRaters)
	}
	if !ValidTarget(in.Target) {
		return nil, fmt.Errorf("target must be perfectcorp, wardah, makeover, rater:<id> or %s, got %q", TargetRaterConsensus, in.Target)
	}
	have := map[Target]bool{}
	for _, c := range in.Base.Doc.Classifiers {
		have[Target{Kind: KindClassifier, Key: c.Key}] = true
	}
	for _, t := range in.Base.Doc.Traits {
		have[Target{Kind: KindTrait, Key: t.Key}] = true
	}
	for _, attr := range sortedKeys(in.Labels.Targets) {
		if t := in.Labels.Targets[attr]; !have[t] {
			return nil, fmt.Errorf("calibrate: labels for %q target %s %q, which the base profile does not have (labels resolved against another profile?)", attr, t.Kind, t.Key)
		}
	}
	labels := in.Labels.ForTarget(in.Target)
	if len(labels) == 0 {
		return nil, fmt.Errorf("target %s labelled no attribute of the label map", in.Target)
	}

	var hashed []HashedImage
	byID := map[string]Item{}
	for _, it := range in.Items {
		if it.Outcome != OutcomeMeasured {
			continue
		}
		h, ok := in.Hashes[it.Row.ImageID]
		if !ok {
			return nil, fmt.Errorf("%s: no perceptual hash", it.Row.ImageID)
		}
		hashed = append(hashed, HashedImage{ImageID: it.Row.ImageID, SubjectID: it.Row.SubjectID, Hash: h})
		byID[it.Row.ImageID] = it
	}
	dd := Dedup(hashed, in.Params.DedupMaxDistance)
	held := HeldOutMetrics{
		Target: in.Target, CatalogueVersion: in.Catalogue.CatalogueVersion, BaseProfileSHA: in.Base.SHA256,
		Params: in.Params, Dedup: DedupSummary{Considered: len(hashed), Removed: len(dd.Removed)},
	}
	var cross []Duplicate
	for _, d := range dd.Removed {
		if d.CrossSubject {
			held.Dedup.CrossSubjectRemoved++
			cross = append(cross, d)
		}
	}
	var subjects []string
	for _, id := range dd.Kept {
		subjects = append(subjects, byID[id].Row.SubjectID)
	}
	split, err := SplitSubjects(subjects, in.Params.HeldOutFraction, in.Params.Seed)
	if err != nil {
		return nil, err
	}
	heldOut := map[string]bool{}
	for _, s := range split.HeldOut {
		heldOut[s] = true
	}
	held.Split = SplitSummary{TrainSubjects: len(split.Train), HeldOutSubjects: len(split.HeldOut)}

	// Only training-subject images become Examples: held-out images go to
	// test and nowhere else, so FitProfile never sees them.
	var train []Example
	var test []Item
	for _, id := range dd.Kept {
		it := byID[id]
		if held.ModelSHA256 == "" {
			held.ModelSHA256 = it.Response.Model.ManifestSha256
		}
		if heldOut[it.Row.SubjectID] {
			test = append(test, it)
			held.Split.HeldOutImages++
			continue
		}
		ex := Example{Index: it.Response.Index(), Labels: map[string]string{}}
		for key, byImage := range labels {
			if l, ok := byImage[id]; ok {
				ex.Labels[key] = l
			}
		}
		train = append(train, ex)
		held.Split.TrainImages++
	}

	fit, err := FitProfile(in.Base.Doc, train, in.Params.MinClassCount, in.RunID, in.Target)
	if err != nil {
		return nil, err
	}
	fitted := &ProfileFile{Entity: &profile.FaceArchitectureProfile{ID: "run:" + in.RunID, Code: in.RunID}, Doc: fit.Doc}
	held.Fitted = heldOutMetrics(test, fitted, labels, in.Params.ECEBins)
	held.Base = heldOutMetrics(test, in.Base, labels, in.Params.ECEBins)

	metrics, err := json.Marshal(held)
	if err != nil {
		return nil, err
	}
	doc := fit.Doc
	runID, target := in.RunID, in.Target
	doc.Calibration = profile.Calibration{
		Status:         profile.CalibrationCalibrated,
		BenchmarkRunID: &runID,
		Target:         &target,
		HeldOutMetrics: metrics,
		Sources:        append(keptSources(in.Base.Doc.Calibration.Sources, fit.Owners), fit.Sources...),
	}
	if err := profile.Validate(doc, in.Catalogue); err != nil {
		return nil, fmt.Errorf("calibrated profile does not validate: %w", err)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return &CalibrateResult{Doc: doc, JSON: append(out, '\n'), HeldOut: held, CrossSubjectDuplicates: cross}, nil
}

// keptSources drops a base source only when it describes a fitted number
// itself (its For equals an owner) or something under one (owner + ".");
// owners are leaf paths (prototype centre/spread, trait threshold value).
// Everything else is kept: vocabulary, class- and trait-level sources
// (which also cover unfitted weights, requires, rule order and labels),
// blendMargin, minObservedWeight and guidance.
func keptSources(base []profile.Source, owners []string) []profile.Source {
	out := []profile.Source{}
	for _, s := range base {
		refitted := false
		for _, o := range owners {
			if s.For == o || strings.HasPrefix(s.For, o+".") {
				refitted = true
				break
			}
		}
		if !refitted {
			out = append(out, s)
		}
	}
	return out
}

// heldOutMetrics scores a profile on the held-out items against the
// target labels (our key -> image id -> label), over all held-out images.
func heldOutMetrics(test []Item, p *ProfileFile, labels map[string]map[string]string, bins int) map[string]Metrics {
	answers := map[string]map[Target]Answer{}
	for _, it := range test {
		answers[it.Row.ImageID] = Answers(p.Entity, p.Doc, it.Response)
	}
	out := map[string]Metrics{}
	for _, c := range p.Doc.Classifiers {
		t := Target{Kind: KindClassifier, Key: c.Key}
		out[answerKey(t)] = Compare(PairsFor(test, answers, t, labels[c.Key], SliceAll), bins)
	}
	for _, tr := range p.Doc.Traits {
		t := Target{Kind: KindTrait, Key: tr.Key}
		out[answerKey(t)] = Compare(PairsFor(test, answers, t, labels[tr.Key], SliceAll), bins)
	}
	return out
}

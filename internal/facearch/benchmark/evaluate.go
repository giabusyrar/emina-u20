package benchmark

import (
	"errors"
	"fmt"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// Slices every coverage and agreement figure is split by.
const (
	SliceAll     = "all"
	SliceHijab   = "hijab"
	SliceNoHijab = "no_hijab"
)

// SliceNames lists the slices in report order.
var SliceNames = []string{SliceAll, SliceHijab, SliceNoHijab}

func inSlice(slice string, row ManifestRow) bool {
	switch slice {
	case SliceHijab:
		return row.Hijab
	case SliceNoHijab:
		return !row.Hijab
	}
	return true
}

// ProfileInfo identifies the scored profile file.
type ProfileInfo struct {
	Code              string  `json:"code"`
	SHA256            string  `json:"sha256"`
	CalibrationStatus string  `json:"calibrationStatus"`
	BenchmarkRunID    *string `json:"benchmarkRunId"`
	Target            *string `json:"target"`
}

// AnswerCoverage counts one classifier's or trait's statuses over the
// measured images of a slice.
type AnswerCoverage struct {
	Measured int            `json:"measured"`
	Statuses map[string]int `json:"statuses"`
	// NoAnswerRate is the share of measured images with no label
	// (insufficient_evidence, not_assessable or unavailable).
	NoAnswerRate *float64 `json:"noAnswerRate"`
}

// SliceCoverage is what happened to the images of one slice.
type SliceCoverage struct {
	Images   int            `json:"images"`
	Outcomes map[string]int `json:"outcomes"`
	// GateRejectionRate is gate_rejected over images the worker answered
	// (every outcome except not_measured and unavailable).
	GateRejectionRate *float64 `json:"gateRejectionRate"`
	// Answers is keyed "classifier:<key>" or "trait:<key>".
	Answers map[string]AnswerCoverage `json:"answers"`
}

// AgreementRow compares our answers with one source on one attribute.
type AgreementRow struct {
	Attribute string `json:"attribute"`
	Source    string `json:"source"`
	Target    Target `json:"target"`
	// Slices holds the metrics per slice (all, hijab, no_hijab).
	Slices map[string]Metrics `json:"slices"`
	// Labelled is how many images the source gave a label for.
	Labelled int `json:"labelled"`
	// NotMeasured: labelled images without a measurement (any outcome
	// other than measured). OursNoAnswer: measured, but no label from us.
	NotMeasured  int `json:"notMeasured"`
	OursNoAnswer int `json:"oursNoAnswer"`
	// Abstained: rows the label map sends to null (the source's "Unknown").
	Abstained int `json:"abstained"`
}

// SourcePairRow is how two sources agree with each other on one attribute:
// the practical ceiling for agreement with either of them.
type SourcePairRow struct {
	Attribute      string   `json:"attribute"`
	SourceA        string   `json:"sourceA"`
	SourceB        string   `json:"sourceB"`
	N              int      `json:"n"`
	ExactAgreement *float64 `json:"exactAgreement"`
	Kappa          *float64 `json:"kappa"`
}

// Report is report.json. It holds counts and metrics only: no image paths,
// no image ids, no per-image rows.
type Report struct {
	Profile          ProfileInfo `json:"profile"`
	ModelSHA256      string      `json:"modelSha256"`
	CatalogueVersion string      `json:"catalogueVersion"`
	// Params are the tunables this run actually used, after Validate.
	Params             Params                   `json:"params"`
	Images             int                      `json:"images"`
	Subjects           int                      `json:"subjects"`
	Coverage           map[string]SliceCoverage `json:"coverage"`
	Agreement          []AgreementRow           `json:"agreement"`
	SourceAgreement    []SourcePairRow          `json:"sourceAgreement"`
	NoConsensus        map[string]int           `json:"noConsensus"`
	UnmappedAttributes map[string]int           `json:"unmappedAttributes"`
}

// EvaluateInput is everything evaluate needs, already loaded.
type EvaluateInput struct {
	Items            []Item
	Profile          *ProfileFile
	Labels           *LabelSet
	ModelSHA256      string
	CatalogueVersion string
	// Params are validated by Evaluate. ECEBins and MinConsensusRaters are
	// the ones it uses; the labels must have been built with the same
	// MinConsensusRaters.
	Params Params
}

func answerKey(t Target) string { return t.Kind + ":" + t.Key }

// Evaluate scores the profile against every source. It fails when the
// params are invalid or the profile or labels are missing; the report
// records the params it used.
func Evaluate(in EvaluateInput) (*Report, error) {
	if err := in.Params.Validate(); err != nil {
		return nil, fmt.Errorf("evaluate: invalid params: %w", err)
	}
	if in.Profile == nil || in.Labels == nil {
		return nil, errors.New("evaluate: profile and labels are required")
	}
	doc := in.Profile.Doc
	answers := map[string]map[Target]Answer{}
	subjects := map[string]bool{}
	for _, it := range in.Items {
		subjects[it.Row.SubjectID] = true
		if it.Outcome == OutcomeMeasured {
			answers[it.Row.ImageID] = Answers(in.Profile.Entity, doc, it.Response)
		}
	}
	rep := &Report{
		Profile: ProfileInfo{
			Code:              in.Profile.Entity.Code,
			SHA256:            in.Profile.SHA256,
			CalibrationStatus: doc.Calibration.Status,
			BenchmarkRunID:    doc.Calibration.BenchmarkRunID,
			Target:            doc.Calibration.Target,
		},
		ModelSHA256:        in.ModelSHA256,
		CatalogueVersion:   in.CatalogueVersion,
		Params:             in.Params,
		Images:             len(in.Items),
		Subjects:           len(subjects),
		Coverage:           map[string]SliceCoverage{},
		Agreement:          []AgreementRow{},
		SourceAgreement:    []SourcePairRow{},
		NoConsensus:        in.Labels.NoConsensus,
		UnmappedAttributes: in.Labels.Unmapped,
	}

	var targets []Target
	for _, c := range doc.Classifiers {
		targets = append(targets, Target{Kind: KindClassifier, Key: c.Key})
	}
	for _, t := range doc.Traits {
		targets = append(targets, Target{Kind: KindTrait, Key: t.Key})
	}
	for _, slice := range SliceNames {
		cov := SliceCoverage{Outcomes: map[string]int{}, Answers: map[string]AnswerCoverage{}}
		for _, o := range Outcomes {
			cov.Outcomes[o] = 0
		}
		for _, t := range targets {
			cov.Answers[answerKey(t)] = AnswerCoverage{Statuses: map[string]int{}}
		}
		for _, it := range in.Items {
			if !inSlice(slice, it.Row) {
				continue
			}
			cov.Images++
			cov.Outcomes[it.Outcome]++
			if it.Outcome != OutcomeMeasured {
				continue
			}
			for t, a := range answers[it.Row.ImageID] {
				ac := cov.Answers[answerKey(t)]
				ac.Measured++
				ac.Statuses[a.Status]++
				cov.Answers[answerKey(t)] = ac
			}
		}
		if answered := cov.Images - cov.Outcomes[OutcomeNotMeasured] - cov.Outcomes[OutcomeUnavailable]; answered > 0 {
			cov.GateRejectionRate = ptr(float64(cov.Outcomes[OutcomeGateRejected]) / float64(answered))
		}
		for k, ac := range cov.Answers {
			if ac.Measured > 0 {
				none := 0
				for status, n := range ac.Statuses {
					switch status {
					case classify.StatusSingle, classify.StatusBlend, traits.StatusAssessed:
					default:
						none += n
					}
				}
				ac.NoAnswerRate = ptr(float64(none) / float64(ac.Measured))
			}
			cov.Answers[k] = ac
		}
		rep.Coverage[slice] = cov
	}

	byID := map[string]Item{}
	for _, it := range in.Items {
		byID[it.Row.ImageID] = it
	}
	for _, attr := range sortedKeys(in.Labels.Targets) {
		t := in.Labels.Targets[attr]
		sources := in.Labels.Sources(attr)
		for _, src := range sources {
			labels := in.Labels.Labels[attr][src]
			row := AgreementRow{Attribute: attr, Source: src, Target: t, Slices: map[string]Metrics{},
				Labelled: len(labels), Abstained: in.Labels.Abstained[attr][src]}
			for _, img := range sortedKeys(labels) {
				it := byID[img]
				if it.Outcome != OutcomeMeasured {
					row.NotMeasured++
				} else if !answers[img][t].Answered() {
					row.OursNoAnswer++
				}
			}
			for _, slice := range SliceNames {
				row.Slices[slice] = Compare(PairsFor(in.Items, answers, t, labels, slice), in.Params.ECEBins)
			}
			rep.Agreement = append(rep.Agreement, row)
		}
		for i := 0; i < len(sources); i++ {
			for j := i + 1; j < len(sources); j++ {
				a, b := sources[i], sources[j]
				if derivedFrom(a, b) || derivedFrom(b, a) {
					continue
				}
				var la, lb []string
				for _, img := range sortedKeys(in.Labels.Labels[attr][a]) {
					if l, ok := in.Labels.Labels[attr][b][img]; ok {
						la = append(la, in.Labels.Labels[attr][a][img])
						lb = append(lb, l)
					}
				}
				if len(la) == 0 {
					continue
				}
				rep.SourceAgreement = append(rep.SourceAgreement, SourcePairRow{Attribute: attr, SourceA: a, SourceB: b,
					N: len(la), ExactAgreement: Agreement(la, lb), Kappa: CohenKappa(la, lb)})
			}
		}
	}
	return rep, nil
}

// derivedFrom reports whether source a is computed from b: the rater
// consensus is not an independent opinion of any rater.
func derivedFrom(a, b string) bool {
	return a == TargetRaterConsensus && strings.HasPrefix(b, RaterPrefix)
}

// PairsFor pairs a source's labels with our answers for target t over the
// measured items of a slice, in manifest order. Images we gave no label
// are left out (they are counted as oursNoAnswer, not as disagreement).
func PairsFor(items []Item, answers map[string]map[Target]Answer, t Target, labels map[string]string, slice string) []Pair {
	var pairs []Pair
	for _, it := range items {
		if it.Outcome != OutcomeMeasured || !inSlice(slice, it.Row) {
			continue
		}
		truth, ok := labels[it.Row.ImageID]
		if !ok {
			continue
		}
		a := answers[it.Row.ImageID][t]
		if !a.Answered() {
			continue
		}
		pairs = append(pairs, Pair{Truth: truth, Pred: a.Label, Top2: a.Top2, Score: a.Score})
	}
	return pairs
}

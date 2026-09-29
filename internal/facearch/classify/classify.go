// Package classify turns measurements into class scores (face shape, and any
// other prototype classifier a profile defines).
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Classifiers (prototype scoring)".
package classify

import (
	"math"
	"sort"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Result statuses.
const (
	// StatusSingle: the top class's normalised score reached blendMargin.
	StatusSingle = "single"
	// StatusBlend: it did not; primary and secondary are the top two.
	StatusBlend = "blend"
	// StatusInsufficientEvidence: too little of the prototype weight was
	// observed, or fewer than two classes could be scored.
	StatusInsufficientEvidence = "insufficient_evidence"
	// StatusUnavailable: the profile no longer matches the ai-worker's
	// catalogue (set by the caller, see Unavailable).
	StatusUnavailable = "unavailable"
)

// Result is one classifier's answer. Scores always holds every assessable
// class's normalised score (they sum to 1), whatever the status.
type Result struct {
	Status              string             `json:"status"`
	Primary             string             `json:"primary,omitempty"`
	Secondary           string             `json:"secondary,omitempty"`
	Scores              map[string]float64 `json:"scores"`
	NotAssessable       []string           `json:"notAssessable"`
	MissingMeasurements []string           `json:"missingMeasurements,omitempty"`
	ObservedWeightShare *float64           `json:"observedWeightShare,omitempty"`
	Reason              string             `json:"reason,omitempty"`
}

// Classifier scores one face. PrototypeClassifier is the v1 implementation;
// a learned classifier can implement the same interface later.
type Classifier interface {
	Classify(obs measureclient.Index) Result
}

// Unavailable is the result for a classifier the caller cannot run.
func Unavailable(reason string) Result {
	return Result{Status: StatusUnavailable, Scores: map[string]float64{}, NotAssessable: []string{}, Reason: reason}
}

// PrototypeClassifier scores each class by distance to its prototype:
//
//	score_c = exp(-1/2 * sum_m w_m * ((x_m - centre_m) / spread_m)^2)
//
// over the class's usable measurements, normalised across assessable
// classes. "Usable" means the worker returned a float: visibility
// "observed" or "unknown" (v1 feature regions are always "unknown").
//
// A class is not assessable when a "requires" measurement is not usable, or
// when none of its prototype measurements is usable (a score over nothing
// would be exp(0) = 1, a fabricated certainty).
type PrototypeClassifier struct {
	spec profile.ClassifierSpec
}

// NewPrototype returns a classifier for a validated spec.
func NewPrototype(spec profile.ClassifierSpec) *PrototypeClassifier {
	return &PrototypeClassifier{spec: spec}
}

type scored struct {
	label string
	order int
	log   float64
}

func (p *PrototypeClassifier) Classify(obs measureclient.Index) Result {
	res := Result{Scores: map[string]float64{}, NotAssessable: []string{}}
	missing := map[string]bool{}
	var totalW, observedW float64
	var ok []scored

	for i, cl := range p.spec.Classes {
		assessable := true
		for _, k := range cl.Requires {
			if !obs.Observe(k).Usable() {
				missing[k] = true
				assessable = false
			}
		}
		var logScore float64
		used := 0
		for _, k := range cl.SortedKeys() {
			term := cl.Prototype[k]
			totalW += term.Weight
			o := obs.Observe(k)
			if !o.Usable() {
				missing[k] = true
				continue
			}
			observedW += term.Weight
			z := (o.Value - term.Centre) / term.Spread
			logScore -= 0.5 * term.Weight * z * z
			used++
		}
		if !assessable || used == 0 {
			res.NotAssessable = append(res.NotAssessable, cl.Label)
			continue
		}
		ok = append(ok, scored{label: cl.Label, order: i, log: logScore})
	}

	// Normalise in log space so far-off faces do not underflow to 0/0.
	if len(ok) > 0 {
		maxLog := math.Inf(-1)
		for _, s := range ok {
			maxLog = math.Max(maxLog, s.log)
		}
		var sum float64
		for _, s := range ok {
			sum += math.Exp(s.log - maxLog)
		}
		for _, s := range ok {
			res.Scores[s.label] = math.Exp(s.log-maxLog) / sum
		}
	}
	if len(missing) > 0 {
		res.MissingMeasurements = make([]string, 0, len(missing))
		for k := range missing {
			res.MissingMeasurements = append(res.MissingMeasurements, k)
		}
		sort.Strings(res.MissingMeasurements)
	}
	share := 0.0
	if totalW > 0 {
		share = observedW / totalW
	}
	res.ObservedWeightShare = &share

	if share < p.spec.MinObservedWeight || len(ok) < 2 {
		res.Status = StatusInsufficientEvidence
		return res
	}
	sort.SliceStable(ok, func(i, j int) bool {
		si, sj := res.Scores[ok[i].label], res.Scores[ok[j].label]
		if si != sj {
			return si > sj
		}
		return ok[i].order < ok[j].order
	})
	res.Primary = ok[0].label
	if res.Scores[ok[0].label] < p.spec.BlendMargin {
		res.Status = StatusBlend
		res.Secondary = ok[1].label
		return res
	}
	res.Status = StatusSingle
	return res
}

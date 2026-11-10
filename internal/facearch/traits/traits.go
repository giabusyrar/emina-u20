// Package traits evaluates a profile's ordered trait rule lists.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Traits".
package traits

import (
	"sort"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
)

// Result statuses.
const (
	StatusAssessed      = "assessed"
	StatusNotAssessable = "not_assessable"
	StatusUnavailable   = "unavailable"
)

// Result is one trait's answer.
type Result struct {
	Status            string `json:"status"`
	Label             string `json:"label,omitempty"`
	BoundaryUncertain bool   `json:"boundaryUncertain"`
	Alternative       string `json:"alternative,omitempty"`
	// Measurements lists every measurement key the trait's spec reads
	// (profile.TraitSpec.Keys), observed or not; MissingMeasurements is the
	// subset that was missing. Set by the caller that holds the spec.
	Measurements        []string `json:"measurements"`
	MissingMeasurements []string `json:"missingMeasurements,omitempty"`
	Reason              string   `json:"reason,omitempty"`
}

// Unavailable is the result for a trait the caller cannot run.
func Unavailable(reason string) Result {
	return Result{Status: StatusUnavailable, Measurements: []string{}, Reason: reason}
}

// Evaluate runs the rules in order; the first whose conditions all hold
// wins, otherwise the default label.
//
// Any condition, in any rule, on a measurement that is not usable (occluded,
// not supported, null) makes the trait not_assessable: the default is only
// meaningful when every earlier rule could be tested.
//
// A condition whose noise band straddles its threshold (lo <= value <= hi)
// could have gone the other way. Conditions are tried in rule order; the
// first one whose flip changes the winning label sets boundaryUncertain and
// alternative to that label. The worker's bands are value-centred and
// conservative (independent noise at the model card's worst regional
// error), so boundaryUncertain is expected often. A measurement without a
// band (null: bands disabled or too few finite samples) carries no
// uncertainty information, so its conditions never straddle.
//
// Conditions are flipped one at a time; a label that would change only if
// two conditions of one rule flipped together is not detected, so
// boundaryUncertain: false does not guarantee the label is robust.
func Evaluate(spec profile.TraitSpec, obs measureclient.Index) Result {
	keys := spec.Keys()
	values := make(map[string]measureclient.Observation, len(keys))
	var missing []string
	for _, k := range keys {
		o := obs.Observe(k)
		if !o.Usable() {
			missing = append(missing, k)
			continue
		}
		values[k] = o
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Result{
			Status:              StatusNotAssessable,
			MissingMeasurements: missing,
			Reason:              "requires " + strings.Join(missing, ", "),
		}
	}

	truth := make([][]bool, len(spec.Rules))
	for i, r := range spec.Rules {
		truth[i] = make([]bool, len(r.When))
		for j, c := range r.When {
			truth[i][j] = c.Holds(values[c.Measurement].Value)
		}
	}
	res := Result{Status: StatusAssessed, Label: pick(spec, truth)}

	for i, r := range spec.Rules {
		for j, c := range r.When {
			band := values[c.Measurement].Band
			if band == nil || !(band[0] <= c.Value && c.Value <= band[1]) {
				continue
			}
			truth[i][j] = !truth[i][j]
			alt := pick(spec, truth)
			truth[i][j] = !truth[i][j]
			if alt != res.Label {
				res.BoundaryUncertain = true
				res.Alternative = alt
				return res
			}
		}
	}
	return res
}

func pick(spec profile.TraitSpec, truth [][]bool) string {
	for i, r := range spec.Rules {
		all := true
		for j := range r.When {
			if !truth[i][j] {
				all = false
				break
			}
		}
		if all {
			return r.Label
		}
	}
	return spec.Default
}

package guidance

import (
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/classify"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/traits"
)

// Evidence is what rules match on: the response's classifications and
// traits, keyed as in the profile.
type Evidence struct {
	Classifications map[string]classify.Result
	Traits          map[string]traits.Result
}

// Grade says how a rule matched.
type Grade int

const (
	// NoMatch: some When entry did not match.
	NoMatch Grade = iota
	// MatchSecondary: every entry matched, at least one only through a
	// blend's secondary class.
	MatchSecondary
	// MatchPrimary: every entry matched on a primary class or a trait label.
	MatchPrimary
)

// Match grades rule r against the evidence. A classifier entry matches when
// the result is single or blend and its primary is listed (MatchPrimary),
// or the result is a blend and its secondary is listed (MatchSecondary). A
// trait entry matches when the trait is assessed and its label is listed:
// at MatchPrimary when the label is certain, at MatchSecondary when the
// trait is BoundaryUncertain (the label could flip under noise, as a blend's
// secondary could be the primary). A trait's Alternative never matches.
// Every other status (insufficient_evidence, not_assessable, unavailable),
// and a key with no result, never matches. The rule's grade is its weakest
// entry's.
func Match(r profile.GuidanceRule, ev Evidence) Grade {
	if len(r.When) == 0 {
		return NoMatch
	}
	grade := MatchPrimary
	for _, key := range r.WhenKeys() {
		g := matchKey(key, r.When[key], ev)
		if g < grade {
			grade = g
		}
	}
	return grade
}

func matchKey(key string, labels []string, ev Evidence) Grade {
	if res, ok := ev.Classifications[key]; ok {
		switch res.Status {
		case classify.StatusSingle, classify.StatusBlend:
			if contains(labels, res.Primary) {
				return MatchPrimary
			}
			if res.Status == classify.StatusBlend && contains(labels, res.Secondary) {
				return MatchSecondary
			}
		}
		return NoMatch
	}
	if res, ok := ev.Traits[key]; ok && res.Status == traits.StatusAssessed && contains(labels, res.Label) {
		if res.BoundaryUncertain {
			return MatchSecondary
		}
		return MatchPrimary
	}
	return NoMatch
}

func contains(list []string, s string) bool {
	if s == "" {
		return false
	}
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

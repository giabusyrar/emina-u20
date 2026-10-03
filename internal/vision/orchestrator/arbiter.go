package orchestrator

import (
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/workers"
)

type ZoneArbiter struct{}

func NewZoneArbiter() *ZoneArbiter {
	return &ZoneArbiter{}
}

// facingAngle is the camera angle that faces each zone: a cheek is seen best
// from its own side, every other zone from the front. A fact about the face,
// not a tunable.
func facingAngle(zoneCode string) domain.ImageAngle {
	switch zoneCode {
	case domain.ZoneLeftCheekInner, domain.ZoneLeftCheekOuter:
		return domain.AngleLeft
	case domain.ZoneRightCheekInner, domain.ZoneRightCheekOuter:
		return domain.AngleRight
	default:
		return domain.AngleFront
	}
}

// preference ranks a candidate view of a zone: visible beats out of frame,
// then the facing angle, then FRONT. No measured quality score exists, so the
// choice is this fixed order; equal ranks keep the earlier image.
func preference(candidate workers.SegmentedZone) int {
	if !candidate.IsVisible {
		return 0
	}
	switch candidate.SourceAngle {
	case facingAngle(candidate.ZoneCode):
		return 3
	case domain.AngleFront:
		return 2
	default:
		return 1
	}
}

// Arbitrate keeps one candidate per zone code: the most preferred view.
func (a *ZoneArbiter) Arbitrate(candidates []workers.SegmentedZone) []workers.SegmentedZone {
	bestByZone := make(map[string]workers.SegmentedZone)
	var order []string

	for _, cand := range candidates {
		current, exists := bestByZone[cand.ZoneCode]
		if !exists {
			order = append(order, cand.ZoneCode)
		}
		if !exists || preference(cand) > preference(current) {
			bestByZone[cand.ZoneCode] = cand
		}
	}

	canonicalOrder := []string{
		domain.ZoneForehead,
		domain.ZoneTZoneNose,
		domain.ZoneLeftCheekInner,
		domain.ZoneLeftCheekOuter,
		domain.ZoneRightCheekInner,
		domain.ZoneRightCheekOuter,
		domain.ZonePeriorbitalEyelids,
		domain.ZoneChinJawline,
	}
	canonical := make(map[string]bool, len(canonicalOrder))
	var winners []workers.SegmentedZone
	for _, code := range canonicalOrder {
		canonical[code] = true
		if winner, ok := bestByZone[code]; ok {
			winners = append(winners, winner)
		}
	}
	// Non-standard zone codes follow, in the order they were first seen.
	for _, code := range order {
		if !canonical[code] {
			winners = append(winners, bestByZone[code])
		}
	}
	return winners
}

package orchestrator

import (
	"math"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/workers"
)

type ZoneArbiter struct{}

func NewZoneArbiter() *ZoneArbiter {
	return &ZoneArbiter{}
}

// CalculateQualityScore computes the effective score considering base confidence, angle affinity, and occlusion.
func (a *ZoneArbiter) CalculateQualityScore(candidate workers.SegmentedZone) float64 {
	score := candidate.Confidence

	// Angle affinity bonus
	angleBonus := 0.0
	switch candidate.ZoneCode {
	case domain.ZoneLeftCheekInner, domain.ZoneLeftCheekOuter:
		if candidate.SourceAngle == domain.AngleLeft {
			angleBonus = 0.10
		}
	case domain.ZoneRightCheekInner, domain.ZoneRightCheekOuter:
		if candidate.SourceAngle == domain.AngleRight {
			angleBonus = 0.10
		}
	case domain.ZoneForehead, domain.ZoneTZoneNose, domain.ZonePeriorbitalEyelids, domain.ZoneChinJawline:
		if candidate.SourceAngle == domain.AngleFront {
			angleBonus = 0.10
		}
	}
	score += angleBonus

	// Occlusion multiplier
	occlusionMultiplier := 1.0
	if candidate.IsOccluded {
		occlusionMultiplier = 0.5
	}

	return score * occlusionMultiplier
}

// Arbitrate evaluates all candidate zones and returns the single highest quality candidate per canonical ZoneCode.
func (a *ZoneArbiter) Arbitrate(candidates []workers.SegmentedZone) []workers.SegmentedZone {
	bestByZone := make(map[string]workers.SegmentedZone)
	bestScoreByZone := make(map[string]float64)

	for _, cand := range candidates {
		score := a.CalculateQualityScore(cand)
		currentBestScore, exists := bestScoreByZone[cand.ZoneCode]

		if !exists || score > currentBestScore || (math.Abs(score-currentBestScore) < 0.001 && cand.SourceAngle == domain.AngleFront) {
			bestByZone[cand.ZoneCode] = cand
			bestScoreByZone[cand.ZoneCode] = score
		}
	}

	// Canonical order preservation
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

	var winners []workers.SegmentedZone
	for _, code := range canonicalOrder {
		if winner, ok := bestByZone[code]; ok {
			winners = append(winners, winner)
		}
	}

	// Any non-standard zone codes that might exist
	for code, winner := range bestByZone {
		found := false
		for _, c := range canonicalOrder {
			if c == code {
				found = true
				break
			}
		}
		if !found {
			winners = append(winners, winner)
		}
	}

	return winners
}

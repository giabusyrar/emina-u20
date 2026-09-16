package orchestrator_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/orchestrator"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/workers"
	"github.com/stretchr/testify/assert"
)

func TestZoneArbiter_Arbitrate_SingleAngle(t *testing.T) {
	arbiter := orchestrator.NewZoneArbiter()

	candidates := []workers.SegmentedZone{
		{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, Confidence: 0.90, IsOccluded: false},
		{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleFront, Confidence: 0.85, IsOccluded: false},
	}

	winners := arbiter.Arbitrate(candidates)
	assert.Len(t, winners, 2)
}

func TestZoneArbiter_Arbitrate_MultiAngleSelection(t *testing.T) {
	arbiter := orchestrator.NewZoneArbiter()

	candidates := []workers.SegmentedZone{
		// Left cheek outer from front vs left angle
		{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleFront, Confidence: 0.85, IsOccluded: false},
		{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleLeft, Confidence: 0.85, IsOccluded: false},
		// Forehead occluded in front vs clear in side
		{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, Confidence: 0.90, IsOccluded: true},
		{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleLeft, Confidence: 0.80, IsOccluded: false},
	}

	winners := arbiter.Arbitrate(candidates)
	assert.Len(t, winners, 2)

	winnerMap := make(map[string]workers.SegmentedZone)
	for _, w := range winners {
		winnerMap[w.ZoneCode] = w
	}

	// Left cheek should favor AngleLeft due to angle affinity bonus
	assert.Equal(t, domain.AngleLeft, winnerMap[domain.ZoneLeftCheekOuter].SourceAngle)

	// Forehead should favor AngleLeft because Frontal is occluded (0.90 * 0.5 = 0.45 < 0.80)
	assert.Equal(t, domain.AngleLeft, winnerMap[domain.ZoneForehead].SourceAngle)
}

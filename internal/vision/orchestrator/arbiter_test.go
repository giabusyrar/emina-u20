package orchestrator_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	"github.com/experience-gateway/apps/core-engine/internal/vision/workers"
	"github.com/stretchr/testify/assert"
)

func winners(candidates ...workers.SegmentedZone) map[string]workers.SegmentedZone {
	out := make(map[string]workers.SegmentedZone)
	for _, w := range orchestrator.NewZoneArbiter().Arbitrate(candidates) {
		out[w.ZoneCode] = w
	}
	return out
}

func TestZoneArbiter_OneWinnerPerZone(t *testing.T) {
	got := orchestrator.NewZoneArbiter().Arbitrate([]workers.SegmentedZone{
		{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, IsVisible: true},
		{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleFront, IsVisible: true},
	})
	assert.Len(t, got, 2)
}

func TestZoneArbiter_PrefersTheAngleFacingTheZone(t *testing.T) {
	got := winners(
		workers.SegmentedZone{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleFront, IsVisible: true},
		workers.SegmentedZone{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleLeft, IsVisible: true},
		workers.SegmentedZone{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleLeft, IsVisible: true},
		workers.SegmentedZone{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, IsVisible: true},
	)
	assert.Equal(t, domain.AngleLeft, got[domain.ZoneLeftCheekOuter].SourceAngle)
	assert.Equal(t, domain.AngleFront, got[domain.ZoneForehead].SourceAngle)
}

func TestZoneArbiter_FrontBeatsTheOppositeSide(t *testing.T) {
	got := winners(
		workers.SegmentedZone{ZoneCode: domain.ZoneRightCheekInner, SourceAngle: domain.AngleLeft, IsVisible: true},
		workers.SegmentedZone{ZoneCode: domain.ZoneRightCheekInner, SourceAngle: domain.AngleFront, IsVisible: true},
	)
	assert.Equal(t, domain.AngleFront, got[domain.ZoneRightCheekInner].SourceAngle)
}

func TestZoneArbiter_VisibleBeatsOutOfFrame(t *testing.T) {
	got := winners(
		workers.SegmentedZone{ZoneCode: domain.ZoneRightCheekOuter, SourceAngle: domain.AngleRight, IsVisible: false},
		workers.SegmentedZone{ZoneCode: domain.ZoneRightCheekOuter, SourceAngle: domain.AngleLeft, IsVisible: true},
	)
	assert.Equal(t, domain.AngleLeft, got[domain.ZoneRightCheekOuter].SourceAngle)
}

func TestZoneArbiter_OutOfFrameEverywhereIsKeptAsNotVisible(t *testing.T) {
	got := winners(workers.SegmentedZone{ZoneCode: domain.ZoneRightCheekOuter, SourceAngle: domain.AngleLeft, IsVisible: false})
	assert.False(t, got[domain.ZoneRightCheekOuter].IsVisible)
}

func TestZoneArbiter_EqualPreferenceKeepsTheEarlierImage(t *testing.T) {
	got := winners(
		workers.SegmentedZone{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, IsVisible: true, SourceImageIndex: 0},
		workers.SegmentedZone{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, IsVisible: true, SourceImageIndex: 1},
	)
	assert.Equal(t, 0, got[domain.ZoneForehead].SourceImageIndex)
}

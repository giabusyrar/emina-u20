package workers

import (
	"image"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
)

type OcclusionCheckResult struct {
	IsOccluded bool
	Confidence float64
	Warning    *domain.WarningItem
}

type OcclusionWorker struct{}

func NewOcclusionWorker() *OcclusionWorker {
	return &OcclusionWorker{}
}

func (o *OcclusionWorker) CheckOcclusion(zone *SegmentedZone, img image.Image) OcclusionCheckResult {
	return OcclusionCheckResult{
		IsOccluded: false,
		Confidence: 0.95,
		Warning:    nil,
	}
}

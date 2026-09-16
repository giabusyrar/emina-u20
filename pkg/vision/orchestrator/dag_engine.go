package orchestrator

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/inference"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/workers"
)

// Segmenter locates facial zones and head pose in one image.
type Segmenter interface {
	SegmentWithPose(declaredAngle domain.ImageAngle, img image.Image) ([]workers.SegmentedZone, domain.ImageAngle, float64, float64, float64, error)
}

// OcclusionChecker decides whether a segmented zone is blocked.
type OcclusionChecker interface {
	CheckOcclusion(zone *workers.SegmentedZone, img image.Image) workers.OcclusionCheckResult
}

// PlanResolver turns requested dimension/skin-condition codes into a capability plan.
type PlanResolver interface {
	Resolve(ctx context.Context, dimensions, skinConditions []string) (*catalog.Plan, error)
}

type VisionOrchestrator struct {
	segmenter    Segmenter
	occlusion    OcclusionChecker
	arbiter      *ZoneArbiter
	resolver     PlanResolver
	capabilities inference.CapabilityClient
}

func NewVisionOrchestrator(
	segmenter Segmenter,
	occlusion OcclusionChecker,
	resolver PlanResolver,
	capabilities inference.CapabilityClient,
) *VisionOrchestrator {
	return &VisionOrchestrator{
		segmenter:    segmenter,
		occlusion:    occlusion,
		arbiter:      NewZoneArbiter(),
		resolver:     resolver,
		capabilities: capabilities,
	}
}

func (o *VisionOrchestrator) Execute(ctx context.Context, req *domain.AnalyzeVisionRequest) (*domain.VisionAnalysisResponse, error) {
	start := time.Now()

	// Phase 0: Resolve requested codes against Reference Data
	plan, err := o.resolver.Resolve(ctx, req.Dimensions, req.SkinConditions)
	if err != nil {
		return nil, err
	}

	// Enforce 30-second context timeout constraint
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	captureMode := "SINGLE_ANGLE"
	if len(req.Images) > 1 {
		captureMode = "MULTI_ANGLE"
	}

	var (
		muCandidates      sync.Mutex
		providedAngles    []domain.ImageAngle
		processedImages   []domain.ProcessedImageMeta
		allCandidateZones []workers.SegmentedZone
		warnings          []domain.WarningItem
		decodedImages     = make([]image.Image, len(req.Images))
	)

	// Phase 1: Parallel Segmentation & Occlusion DAG Node
	segGroup, _ := errgroup.WithContext(execCtx)

	for idx, imgInput := range req.Images {
		imageIndex := idx
		input := imgInput

		segGroup.Go(func() error {
			img, _, err := image.Decode(bytes.NewReader(input.Data))
			if err != nil {
				muCandidates.Lock()
				warnings = append(warnings, domain.WarningItem{
					Code:    "ERR_IMAGE_DECODE",
					Message: "Failed to decode input image: " + input.FileName,
				})
				muCandidates.Unlock()
				return nil
			}

			zones, detectedAngle, yaw, pitch, roll, err := o.segmenter.SegmentWithPose(input.Angle, img)
			if err != nil {
				return nil
			}

			var imgZones []workers.SegmentedZone
			var imgWarnings []domain.WarningItem

			for _, z := range zones {
				occ := o.occlusion.CheckOcclusion(&z, img)
				z.IsOccluded = occ.IsOccluded
				z.Confidence = occ.Confidence
				z.SourceImageIndex = imageIndex
				if occ.Warning != nil {
					imgWarnings = append(imgWarnings, *occ.Warning)
				}
				imgZones = append(imgZones, z)
			}

			muCandidates.Lock()
			defer muCandidates.Unlock()
			decodedImages[imageIndex] = img
			providedAngles = append(providedAngles, detectedAngle)
			processedImages = append(processedImages, domain.ProcessedImageMeta{
				Angle:              detectedAngle,
				FileName:           input.FileName,
				LandmarkDetected:   true,
				LandmarkConfidence: 0.95,
				Yaw:                yaw,
				Pitch:              pitch,
				Roll:               roll,
			})
			allCandidateZones = append(allCandidateZones, imgZones...)
			warnings = append(warnings, imgWarnings...)
			return nil
		})
	}

	_ = segGroup.Wait()

	// Phase 2: In-DAG Zone Arbiter (Prune to at most 8 canonical winning zones)
	winningZones := o.arbiter.Arbitrate(allCandidateZones)

	// Phase 3: Crop each usable winning zone from its own source image
	zoneBreakdownItems := make([]domain.ZoneBreakdownItem, len(winningZones))
	itemIndexByZone := make(map[string]int, len(winningZones))
	var crops []inference.ZoneCrop

	for i, zone := range winningZones {
		zoneBreakdownItems[i] = domain.ZoneBreakdownItem{
			ZoneCode:    zone.ZoneCode,
			ZoneName:    zone.ZoneName,
			SourceAngle: zone.SourceAngle,
			Confidence:  zone.Confidence,
			IsOccluded:  zone.IsOccluded,
			BoundingBox: zone.BoundingBox,
			Polygon:     zone.Polygon,
			Metrics:     emptyZoneMetrics(),
		}
		// The arbiter keeps at most one winner per zone code.
		itemIndexByZone[zone.ZoneCode] = i

		if zone.IsOccluded {
			continue
		}
		cropped, ok := cropZone(decodedImages[zone.SourceImageIndex], zone.BoundingBox)
		if !ok {
			continue
		}
		crops = append(crops, inference.ZoneCrop{ZoneCode: zone.ZoneCode, SourceAngle: zone.SourceAngle, JPEG: cropped})
	}

	// Phase 4: One batched capability dispatch to the vision AI worker
	scoredCapabilities := make(map[string]bool)
	executedCount := 0
	var unsupportedCapabilities []string

	if len(plan.Capabilities) > 0 && len(crops) > 0 {
		result, err := o.capabilities.DispatchZones(execCtx, plan.Capabilities, crops)
		if err != nil {
			return nil, err
		}
		unsupportedCapabilities = result.UnsupportedCapabilities

		for _, zoneScores := range result.Zones {
			i, ok := itemIndexByZone[zoneScores.ZoneCode]
			if !ok {
				continue
			}
			health := make(map[string]float64, len(zoneScores.Telemetry))
			for capability, severity := range zoneScores.Telemetry {
				health[capability] = SeverityToHealth(severity)
				scoredCapabilities[capability] = true
				executedCount++
			}
			zoneBreakdownItems[i].Metrics = RollUpZone(plan, health, zoneBreakdownItems[i].Confidence)
		}
	}

	warnings = append(warnings, capabilityWarnings(plan, unsupportedCapabilities, scoredCapabilities)...)

	// Phase 5: Aggregation Rollup
	globalAgg := AggregateScores(zoneBreakdownItems)

	executedModels := []string{}
	for _, capability := range plan.Capabilities {
		if scoredCapabilities[capability] {
			executedModels = append(executedModels, capability)
		}
	}

	coverageCompleteness := "PARTIAL_ANGLE"
	if len(providedAngles) >= 3 {
		coverageCompleteness = "FULL_FACIAL_SWEEP"
	}

	return &domain.VisionAnalysisResponse{
		AnalysisID:    uuid.New().String(),
		BrandID:       req.BrandID,
		ApplicationID: req.ApplicationID,
		Status:        "SUCCESS",
		CaptureContext: domain.CaptureContext{
			CaptureMode:          captureMode,
			InputCount:           len(req.Images),
			ProvidedAngles:       providedAngles,
			CoverageCompleteness: coverageCompleteness,
			ProcessedImages:      processedImages,
		},
		GlobalAggregation: globalAgg,
		ZoneBreakdown:     zoneBreakdownItems,
		Warnings:          warnings,
		ExecutionMetrics: domain.ExecutionMetrics{
			TotalLatencyMs:      time.Since(start).Milliseconds(),
			ExecutedModelsCount: executedCount,
			SkippedModelsCount:  len(plan.Capabilities) - len(executedModels),
			ExecutedModels:      executedModels,
		},
	}, nil
}

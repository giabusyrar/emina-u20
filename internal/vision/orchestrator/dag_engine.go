package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/catalog"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/inference"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/workers"
)

// Segmenter locates facial zones and head pose in one image.
type Segmenter interface {
	SegmentWithPose(declaredAngle domain.ImageAngle, img image.Image, raw []byte) ([]workers.SegmentedZone, domain.ImageAngle, float64, float64, float64, error)
}

// PlanResolver turns requested dimension/skin-condition codes into a capability plan.
type PlanResolver interface {
	Resolve(ctx context.Context, dimensions, skinConditions []string) (*catalog.Plan, error)
}

type VisionOrchestrator struct {
	segmenter    Segmenter
	arbiter      *ZoneArbiter
	resolver     PlanResolver
	capabilities inference.CapabilityClient
	execTimeout  time.Duration
}

func NewVisionOrchestrator(
	segmenter Segmenter,
	resolver PlanResolver,
	capabilities inference.CapabilityClient,
	execTimeout time.Duration,
) *VisionOrchestrator {
	return &VisionOrchestrator{
		segmenter:    segmenter,
		arbiter:      NewZoneArbiter(),
		resolver:     resolver,
		capabilities: capabilities,
		execTimeout:  execTimeout,
	}
}

func (o *VisionOrchestrator) Execute(ctx context.Context, req *domain.AnalyzeVisionRequest) (*domain.VisionAnalysisResponse, error) {
	start := time.Now()

	// Phase 0: Resolve requested codes against Reference Data
	plan, err := o.resolver.Resolve(ctx, req.Dimensions, req.SkinConditions)
	if err != nil {
		return nil, err
	}

	// Bound the whole pipeline (VISION_DAG_TIMEOUT).
	execCtx, cancel := context.WithTimeout(ctx, o.execTimeout)
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

	// Phase 1: Parallel segmentation DAG node
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
			// Turn a phone photo stored sideways upright once, here, so skin
			// and the crops below use the same upright pixel grid. An upright
			// upload still goes to skin unchanged; a rotated one is sent as
			// the upright pixels (raw nil: the segmenter encodes img).
			raw := input.Data
			if orientation := exifOrientation(input.Data); orientation != orientationUpright {
				img = applyOrientation(img, orientation)
				raw = nil
			}

			zones, detectedAngle, yaw, pitch, roll, err := o.segmenter.SegmentWithPose(input.Angle, img, raw)
			if errors.Is(err, workers.ErrNoFace) {
				// This image has no measurable face; the others still count.
				muCandidates.Lock()
				warnings = append(warnings, domain.WarningItem{
					Code:    "NO_FACE_DETECTED",
					Message: fmt.Sprintf("No face found in %s: %v", input.FileName, err),
				})
				muCandidates.Unlock()
				return nil
			}
			if err != nil {
				// No substitute zones exist: fail the analysis.
				return err
			}

			imgZones := make([]workers.SegmentedZone, 0, len(zones))
			for _, z := range zones {
				z.SourceImageIndex = imageIndex
				imgZones = append(imgZones, z)
			}

			muCandidates.Lock()
			defer muCandidates.Unlock()
			decodedImages[imageIndex] = img
			providedAngles = append(providedAngles, detectedAngle)
			processedImages = append(processedImages, domain.ProcessedImageMeta{
				Angle:            detectedAngle,
				FileName:         input.FileName,
				LandmarkDetected: true,
				Yaw:              yaw,
				Pitch:            pitch,
				Roll:             roll,
			})
			allCandidateZones = append(allCandidateZones, imgZones...)
			return nil
		})
	}

	if err := segGroup.Wait(); err != nil {
		return nil, err
	}

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
			IsVisible:   zone.IsVisible,
			BoundingBox: zone.BoundingBox,
			Polygon:     zone.Polygon,
			Metrics:     emptyZoneMetrics(),
		}
		// The arbiter keeps at most one winner per zone code.
		itemIndexByZone[zone.ZoneCode] = i

		if !zone.IsVisible {
			continue
		}
		cropped, ok := cropZone(decodedImages[zone.SourceImageIndex], zone.BoundingBox)
		if !ok {
			continue
		}
		crops = append(crops, inference.ZoneCrop{ZoneCode: zone.ZoneCode, SourceAngle: zone.SourceAngle, Image: cropped})
	}

	// Phase 4: One batched capability dispatch to the vision AI worker
	scoredCapabilities := make(map[string]bool)
	var unsupportedCapabilities []string
	var unavailableCapabilities map[string]string

	if len(plan.Capabilities) > 0 && len(crops) > 0 {
		result, err := o.capabilities.DispatchZones(execCtx, plan.Capabilities, crops)
		if err != nil {
			return nil, err
		}
		unsupportedCapabilities = result.UnsupportedCapabilities
		unavailableCapabilities = result.UnavailableCapabilities

		for _, zoneScores := range result.Zones {
			i, ok := itemIndexByZone[zoneScores.ZoneCode]
			if !ok {
				continue
			}
			health := make(map[string]float64, len(zoneScores.Telemetry))
			for capability, severity := range zoneScores.Telemetry {
				health[capability] = SeverityToHealth(severity)
				scoredCapabilities[capability] = true
			}
			zoneBreakdownItems[i].Metrics = RollUpZone(plan, health)
		}
	}

	warnings = append(warnings, capabilityWarnings(plan, unsupportedCapabilities, unavailableCapabilities, scoredCapabilities)...)

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
			ExecutedModelsCount: len(executedModels),
			SkippedModelsCount:  len(plan.Capabilities) - len(executedModels),
			ExecutedModels:      executedModels,
		},
	}, nil
}

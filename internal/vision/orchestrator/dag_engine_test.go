package orchestrator_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/inference"
	"github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	"github.com/experience-gateway/apps/core-engine/internal/vision/workers"
)

var (
	foreheadBox  = domain.BoundingBox{MinX: 0.25, MinY: 0.25, MaxX: 0.75, MaxY: 0.5}   // 50x25 on a 100px image
	tinyNoseBox  = domain.BoundingBox{MinX: 0.5, MinY: 0.25, MaxX: 0.625, MaxY: 0.5}   // 13px wide: below the 16px minimum
	leftCheekBox = domain.BoundingBox{MinX: 0.125, MinY: 0.5, MaxX: 0.375, MaxY: 0.75} // 26x25 on a 100px image
)

func createTestPng(size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for x := 0; x < size; x++ {
		for y := 0; y < size; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 180, B: 170, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// fakeSegmenter returns the zones configured for an image's width, so tests
// can tell input images apart.
type fakeSegmenter struct {
	mu           sync.Mutex
	calls        int
	zonesByWidth map[int][]workers.SegmentedZone
	errByWidth   map[int]error
}

func (f *fakeSegmenter) SegmentWithPose(declared domain.ImageAngle, img image.Image) ([]workers.SegmentedZone, domain.ImageAngle, float64, float64, float64, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if err := f.errByWidth[img.Bounds().Dx()]; err != nil {
		return nil, declared, 0, 0, 0, err
	}
	zones := make([]workers.SegmentedZone, 0)
	for _, zone := range f.zonesByWidth[img.Bounds().Dx()] {
		zone.SourceAngle = declared
		zones = append(zones, zone)
	}
	return zones, declared, 0, 0, 0, nil
}

type fakeResolver struct {
	plan *catalog.Plan
	err  error
}

func (f fakeResolver) Resolve(ctx context.Context, dimensions, skinConditions []string) (*catalog.Plan, error) {
	return f.plan, f.err
}

type fakeCapabilityClient struct {
	mu           sync.Mutex
	calls        int
	capabilities []string
	crops        []inference.ZoneCrop
	result       *inference.DispatchResult
	err          error
}

func (f *fakeCapabilityClient) DispatchZones(ctx context.Context, capabilities []string, crops []inference.ZoneCrop) (*inference.DispatchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.capabilities = capabilities
	f.crops = crops
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return &inference.DispatchResult{}, nil
	}
	return f.result, nil
}

func testPlan() *catalog.Plan {
	return &catalog.Plan{
		Conditions: []catalog.Condition{
			{Code: "concern_acne_papule", DimensionCode: "acne", Capabilities: []string{"acne_lesion_classifier"}},
			{Code: "concern_melasma", DimensionCode: "pigmentation", Capabilities: []string{"hyperpigmentation_net"}},
		},
		Dimensions:   []string{"acne"},
		Capabilities: []string{"acne_lesion_classifier", "hyperpigmentation_net"},
	}
}

func request(images ...domain.RawImageInput) *domain.AnalyzeVisionRequest {
	return &domain.AnalyzeVisionRequest{BrandID: "wardah", ApplicationID: "skinverse", Images: images}
}

func findZone(t *testing.T, resp *domain.VisionAnalysisResponse, code string) domain.ZoneBreakdownItem {
	t.Helper()
	for _, zone := range resp.ZoneBreakdown {
		if zone.ZoneCode == code {
			return zone
		}
	}
	t.Fatalf("zone %s missing from breakdown", code)
	return domain.ZoneBreakdownItem{}
}

func warningCodes(resp *domain.VisionAnalysisResponse) []string {
	var codes []string
	for _, warning := range resp.Warnings {
		codes = append(codes, warning.Code)
	}
	return codes
}

func cropSize(t *testing.T, crop inference.ZoneCrop) (int, int) {
	t.Helper()
	decoded, err := jpeg.Decode(bytes.NewReader(crop.JPEG))
	require.NoError(t, err)
	return decoded.Bounds().Dx(), decoded.Bounds().Dy()
}

func TestExecute_ScoresArbiterWinnersPerZone(t *testing.T) {
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {
			{ZoneCode: domain.ZoneForehead, ZoneName: "Forehead", BoundingBox: foreheadBox, IsVisible: true},
			{ZoneCode: domain.ZoneTZoneNose, ZoneName: "T-Zone & Nose", BoundingBox: tinyNoseBox, IsVisible: true},
			{ZoneCode: domain.ZoneLeftCheekOuter, ZoneName: "Left Cheek (Outer)", BoundingBox: leftCheekBox, IsVisible: false},
		},
	}}
	client := &fakeCapabilityClient{result: &inference.DispatchResult{
		Zones: []inference.ZoneScores{
			{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, Telemetry: map[string]float64{"acne_lesion_classifier": 30}},
		},
		UnsupportedCapabilities: []string{"hyperpigmentation_net"},
	}}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))
	require.NoError(t, err)

	require.Equal(t, 1, client.calls)
	assert.Equal(t, testPlan().Capabilities, client.capabilities)
	require.Len(t, client.crops, 1, "tiny and out-of-frame zones are not cropped")
	assert.Equal(t, domain.ZoneForehead, client.crops[0].ZoneCode)
	width, height := cropSize(t, client.crops[0])
	assert.Equal(t, 50, width)
	assert.Equal(t, 25, height)

	require.Len(t, resp.ZoneBreakdown, 3)
	forehead := findZone(t, resp, domain.ZoneForehead)
	assert.InDelta(t, 70, forehead.Metrics.SkinConditions["concern_acne_papule"].Score, 1e-9, "severity 30 becomes health 70")
	assert.InDelta(t, 70, forehead.Metrics.Dimensions["acne"].Score, 1e-9)
	assert.NotContains(t, forehead.Metrics.Dimensions, "pigmentation")
	assert.NotContains(t, forehead.Metrics.SkinConditions, "concern_melasma")

	assert.Empty(t, findZone(t, resp, domain.ZoneTZoneNose).Metrics.SkinConditions)
	cheek := findZone(t, resp, domain.ZoneLeftCheekOuter)
	assert.False(t, cheek.IsVisible)
	assert.Empty(t, cheek.Metrics.SkinConditions)

	require.NotNil(t, resp.GlobalAggregation.OverallSkinHealthScore)
	assert.InDelta(t, 70, *resp.GlobalAggregation.OverallSkinHealthScore, 1e-9)
	assert.Equal(t, []string{"acne_lesion_classifier"}, resp.ExecutionMetrics.ExecutedModels)
	assert.Equal(t, 1, resp.ExecutionMetrics.ExecutedModelsCount)
	assert.Equal(t, 1, resp.ExecutionMetrics.SkippedModelsCount)

	require.Contains(t, warningCodes(resp), "UNSUPPORTED_CAPABILITY")
	for _, warning := range resp.Warnings {
		if warning.Code == "UNSUPPORTED_CAPABILITY" {
			assert.Contains(t, warning.Message, "hyperpigmentation_net")
			assert.Contains(t, warning.Message, "concern_melasma")
		}
	}
}

func TestExecute_CropsFromTheWinningCandidatesOwnImage(t *testing.T) {
	// The 100px image is a LEFT view and the 200px image a FRONT view; the
	// arbiter must pick the FRONT forehead (the angle facing it) and the crop
	// must come from that image.
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {{ZoneCode: domain.ZoneForehead, BoundingBox: foreheadBox, IsVisible: true}},
		200: {{ZoneCode: domain.ZoneForehead, BoundingBox: foreheadBox, IsVisible: true}},
	}}
	client := &fakeCapabilityClient{}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleLeft, FileName: "left.png", Data: createTestPng(100)},
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(200)},
	))
	require.NoError(t, err)

	assert.Equal(t, "MULTI_ANGLE", resp.CaptureContext.CaptureMode)
	require.Len(t, resp.ZoneBreakdown, 1)
	assert.Equal(t, domain.AngleFront, resp.ZoneBreakdown[0].SourceAngle)
	require.Len(t, client.crops, 1)
	width, height := cropSize(t, client.crops[0])
	assert.Equal(t, 100, width)
	assert.Equal(t, 50, height)
}

func TestExecute_NoUsableCropsSkipsDispatchAndWarns(t *testing.T) {
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {{ZoneCode: domain.ZoneTZoneNose, BoundingBox: tinyNoseBox, IsVisible: true}},
	}}
	client := &fakeCapabilityClient{}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))
	require.NoError(t, err)

	assert.Equal(t, 0, client.calls)
	assert.Equal(t, []string{"NO_APPLICABLE_ZONE", "NO_APPLICABLE_ZONE"}, warningCodes(resp))
	assert.Empty(t, resp.ExecutionMetrics.ExecutedModels)
	assert.Equal(t, 2, resp.ExecutionMetrics.SkippedModelsCount)
	assert.Nil(t, resp.GlobalAggregation.OverallSkinHealthScore, "nothing scored: no overall score")
}

func TestExecute_ResolverErrorStopsBeforeSegmentation(t *testing.T) {
	segmenter := &fakeSegmenter{}
	client := &fakeCapabilityClient{}
	invalid := &catalog.ValidationError{Items: []domain.ValidationErrorItem{{Field: "dimensions", Value: "pigment"}}}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{err: invalid}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))

	assert.Nil(t, resp)
	var validationErr *catalog.ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, 0, segmenter.calls)
	assert.Equal(t, 0, client.calls)
}

func TestExecutedModelsCountDoesNotDoubleCountZones(t *testing.T) {
	// One capability scored on three zones is one executed model, not three.
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {
			{ZoneCode: domain.ZoneForehead, ZoneName: "Forehead", BoundingBox: foreheadBox, IsVisible: true},
			{ZoneCode: domain.ZoneLeftCheekOuter, ZoneName: "Left Cheek (Outer)", BoundingBox: leftCheekBox, IsVisible: true},
			{ZoneCode: domain.ZoneChinJawline, ZoneName: "Chin & Jawline", BoundingBox: foreheadBox, IsVisible: true},
		},
	}}
	client := &fakeCapabilityClient{result: &inference.DispatchResult{
		Zones: []inference.ZoneScores{
			{ZoneCode: domain.ZoneForehead, Telemetry: map[string]float64{"acne": 20}},
			{ZoneCode: domain.ZoneLeftCheekOuter, Telemetry: map[string]float64{"acne": 30}},
			{ZoneCode: domain.ZoneChinJawline, Telemetry: map[string]float64{"acne": 40}},
		},
	}}
	plan := &catalog.Plan{Capabilities: []string{"acne"}}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: plan}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))
	require.NoError(t, err)

	assert.Equal(t, 1, resp.ExecutionMetrics.ExecutedModelsCount)
	assert.Equal(t, 0, resp.ExecutionMetrics.SkippedModelsCount)
}

func TestExecute_DispatchFailureIsReturned(t *testing.T) {
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {{ZoneCode: domain.ZoneForehead, BoundingBox: foreheadBox, IsVisible: true}},
	}}
	client := &fakeCapabilityClient{err: fmt.Errorf("%w: connection refused", inference.ErrWorkerUnavailable)}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))

	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, inference.ErrWorkerUnavailable))
}

func TestExecute_UnloadableCapabilityIsReportedAsUnavailableNotUnuploaded(t *testing.T) {
	segmenter := &fakeSegmenter{zonesByWidth: map[int][]workers.SegmentedZone{
		100: {{ZoneCode: domain.ZoneForehead, BoundingBox: foreheadBox, IsVisible: true}},
	}}
	// dispatch-zones lists an unloadable capability in both fields.
	client := &fakeCapabilityClient{result: &inference.DispatchResult{
		Zones: []inference.ZoneScores{
			{ZoneCode: domain.ZoneForehead, SourceAngle: domain.AngleFront, Telemetry: map[string]float64{"acne_lesion_classifier": 30}},
		},
		UnsupportedCapabilities: []string{"hyperpigmentation_net"},
		UnavailableCapabilities: map[string]string{"hyperpigmentation_net": "RuntimeError: S3 unreachable"},
	}}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))
	require.NoError(t, err)

	codes := warningCodes(resp)
	assert.Contains(t, codes, "CAPABILITY_UNAVAILABLE")
	assert.NotContains(t, codes, "UNSUPPORTED_CAPABILITY")
	for _, warning := range resp.Warnings {
		if warning.Code == "CAPABILITY_UNAVAILABLE" {
			assert.Equal(t, `Model for capability "hyperpigmentation_net" could not be loaded: RuntimeError: S3 unreachable`, warning.Message)
		}
	}
}

func TestExecute_SegmentationUnavailableFailsTheAnalysis(t *testing.T) {
	segmenter := &fakeSegmenter{errByWidth: map[int]error{
		100: fmt.Errorf("%w: skin worker returned 503", workers.ErrSegmentationUnavailable),
	}}
	client := &fakeCapabilityClient{}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	_, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
	))
	assert.ErrorIs(t, err, workers.ErrSegmentationUnavailable)
	assert.Equal(t, 0, client.calls)
}

func TestExecute_NoFaceSkipsOnlyThatImage(t *testing.T) {
	segmenter := &fakeSegmenter{
		zonesByWidth: map[int][]workers.SegmentedZone{
			100: {{ZoneCode: domain.ZoneForehead, BoundingBox: foreheadBox, IsVisible: true}},
		},
		errByWidth: map[int]error{80: fmt.Errorf("%w: no landmarks", workers.ErrNoFace)},
	}
	client := &fakeCapabilityClient{}
	orch := orchestrator.NewVisionOrchestrator(segmenter, fakeResolver{plan: testPlan()}, client, 30*time.Second)

	resp, err := orch.Execute(context.Background(), request(
		domain.RawImageInput{Angle: domain.AngleFront, FileName: "front.png", Data: createTestPng(100)},
		domain.RawImageInput{Angle: domain.AngleLeft, FileName: "left.png", Data: createTestPng(80)},
	))
	require.NoError(t, err)
	assert.Contains(t, warningCodes(resp), "NO_FACE_DETECTED")
	assert.Len(t, resp.CaptureContext.ProcessedImages, 1)
	assert.Nil(t, resp.CaptureContext.ProcessedImages[0].LandmarkConfidence)
}

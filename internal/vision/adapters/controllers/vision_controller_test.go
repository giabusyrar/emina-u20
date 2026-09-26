package controllers_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/vision/adapters/controllers"
	"github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/inference"
	"github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	"github.com/experience-gateway/apps/core-engine/internal/vision/workers"
)

type stubSegmenter struct{}

func (stubSegmenter) SegmentWithPose(declared domain.ImageAngle, img image.Image) ([]workers.SegmentedZone, domain.ImageAngle, float64, float64, float64, error) {
	angle := declared
	if angle == "" {
		angle = domain.AngleFront
	}
	return []workers.SegmentedZone{{
		ZoneCode:    domain.ZoneForehead,
		ZoneName:    "Forehead",
		SourceAngle: angle,
		BoundingBox: domain.BoundingBox{MinX: 0.1, MinY: 0.1, MaxX: 0.9, MaxY: 0.9},
		Confidence:  0.9,
	}}, angle, 0, 0, 0, nil
}

type stubResolver struct {
	plan *catalog.Plan
	err  error
}

func (s stubResolver) Resolve(ctx context.Context, dimensions, skinConditions []string) (*catalog.Plan, error) {
	return s.plan, s.err
}

// stubCapabilities scores every crop with severity 20 (health 80).
type stubCapabilities struct {
	err error
}

func (s stubCapabilities) DispatchZones(ctx context.Context, capabilities []string, crops []inference.ZoneCrop) (*inference.DispatchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	zones := make([]inference.ZoneScores, len(crops))
	for i, crop := range crops {
		zones[i] = inference.ZoneScores{
			ZoneCode:    crop.ZoneCode,
			SourceAngle: crop.SourceAngle,
			Telemetry:   map[string]float64{"acne_lesion_classifier": 20},
		}
	}
	return &inference.DispatchResult{Zones: zones}, nil
}

type stubCatalog struct {
	snapshot *catalog.Snapshot
	err      error
}

func (s stubCatalog) Snapshot(ctx context.Context) (*catalog.Snapshot, error) {
	return s.snapshot, s.err
}

var acnePlan = &catalog.Plan{
	Conditions:   []catalog.Condition{{Code: "concern_acne_papule", DimensionCode: "acne", Capabilities: []string{"acne_lesion_classifier"}}},
	Dimensions:   []string{"acne"},
	Capabilities: []string{"acne_lesion_classifier"},
}

func newController(resolver orchestrator.PlanResolver, capabilities inference.CapabilityClient) *controllers.VisionController {
	orch := orchestrator.NewVisionOrchestrator(stubSegmenter{}, workers.NewOcclusionWorker(), resolver, capabilities, 30*time.Second)
	return controllers.NewVisionController(orch, stubCatalog{snapshot: &catalog.Snapshot{}}, nil)
}

type upload struct {
	field string
	name  string
}

func pngBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for x := 0; x < 50; x++ {
		for y := 0; y < 50; y++ {
			img.Set(x, y, color.RGBA{R: 210, G: 190, B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func analyzeRequest(t *testing.T, fields map[string]string, uploads ...upload) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, u := range uploads {
		part, err := writer.CreateFormFile(u.field, u.name)
		require.NoError(t, err)
		_, _ = part.Write(pngBytes())
	}
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vision/analyze", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	return req
}

func serveAnalyze(t *testing.T, ctrl *controllers.VisionController, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, ctrl.AnalyzeImage(echo.New().NewContext(req, rec)))
	return rec
}

func TestVisionController_NoImages_HTTP400(t *testing.T) {
	ctrl := newController(stubResolver{plan: acnePlan}, stubCapabilities{})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t, map[string]string{"dimensions": "acne"}))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "BAD_REQUEST")
	assert.Contains(t, rec.Body.String(), "images")
}

func TestVisionController_UnknownCode_HTTP400(t *testing.T) {
	invalid := &catalog.ValidationError{Items: []domain.ValidationErrorItem{
		{Field: "dimensions", Value: "wrinkles", Reason: "unrecognized dimension. Supported: [acne]"},
	}}
	ctrl := newController(stubResolver{err: invalid}, stubCapabilities{})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t, map[string]string{"dimensions": "wrinkles"}, upload{"image_front", "front.png"}))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "BAD_REQUEST")
	assert.Contains(t, rec.Body.String(), `"value":"wrinkles"`)
	assert.Contains(t, rec.Body.String(), "Supported: [acne]")
}

func TestVisionController_ReferenceDataUnavailable_HTTP503(t *testing.T) {
	ctrl := newController(stubResolver{err: fmt.Errorf("%w: connection refused", catalog.ErrCatalogUnavailable)}, stubCapabilities{})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t, nil, upload{"image_front", "front.png"}))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "REFERENCE_DATA_UNAVAILABLE")
}

func TestVisionController_VisionWorkerUnavailable_HTTP502(t *testing.T) {
	ctrl := newController(stubResolver{plan: acnePlan}, stubCapabilities{err: fmt.Errorf("%w: timeout", inference.ErrWorkerUnavailable)})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t, nil, upload{"image_front", "front.png"}))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "VISION_WORKER_UNAVAILABLE")
}

func TestVisionController_Success(t *testing.T) {
	ctrl := newController(stubResolver{plan: acnePlan}, stubCapabilities{})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t,
		map[string]string{"dimensions": "acne", "skinConditions": "concern_acne_papule"},
		upload{"image_front", "front.png"},
	))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "SINGLE_ANGLE")
	assert.Contains(t, rec.Body.String(), "ZONE_FOREHEAD")
	assert.Contains(t, rec.Body.String(), "concern_acne_papule")
	assert.Contains(t, rec.Body.String(), `"overallSkinHealthScore":80`)
}

func TestVisionController_UnlabelledMultiImage(t *testing.T) {
	ctrl := newController(stubResolver{plan: acnePlan}, stubCapabilities{})

	rec := serveAnalyze(t, ctrl, analyzeRequest(t,
		map[string]string{"dimensions": "acne"},
		upload{"images", "photo_1.png"},
		upload{"images", "photo_2.png"},
	))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "MULTI_ANGLE")
	assert.Contains(t, rec.Body.String(), "photo_1.png")
	assert.Contains(t, rec.Body.String(), "photo_2.png")
}

func serveRegistry(t *testing.T, referenceCatalog catalog.Catalog) *httptest.ResponseRecorder {
	t.Helper()
	orch := orchestrator.NewVisionOrchestrator(stubSegmenter{}, workers.NewOcclusionWorker(), stubResolver{plan: acnePlan}, stubCapabilities{}, 30*time.Second)
	ctrl := controllers.NewVisionController(orch, referenceCatalog, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/vision/registry", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, ctrl.GetRegistry(echo.New().NewContext(req, rec)))
	return rec
}

func TestVisionController_GetRegistry_ServesReferenceData(t *testing.T) {
	papule := catalog.Condition{
		Code:          "concern_acne_papule",
		Name:          "Acne Papule",
		Description:   "Raised lesion",
		DimensionCode: "acne",
		Capabilities:  []string{"acne_lesion_classifier"},
	}
	snapshot := &catalog.Snapshot{
		Dimensions: []catalog.Dimension{{Code: "acne", Name: "Acne & Blemishes", Conditions: []catalog.Condition{papule}}},
		Conditions: []catalog.Condition{papule},
	}

	rec := serveRegistry(t, stubCatalog{snapshot: snapshot})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{
		"dimensions": [{"code":"acne","name":"Acne & Blemishes","description":"","skinConditions":["concern_acne_papule"]}],
		"skinConditions": [{"code":"concern_acne_papule","name":"Acne Papule","description":"Raised lesion","dimensionCode":"acne","capabilities":["acne_lesion_classifier"]}]
	}`, rec.Body.String())
}

func TestVisionController_GetRegistry_EmptyCatalogGivesEmptyArrays(t *testing.T) {
	rec := serveRegistry(t, stubCatalog{snapshot: &catalog.Snapshot{}})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"dimensions":[],"skinConditions":[]}`, rec.Body.String())
}

func TestVisionController_GetRegistry_ReferenceDataUnavailable_HTTP503(t *testing.T) {
	rec := serveRegistry(t, stubCatalog{err: fmt.Errorf("%w: connection refused", catalog.ErrCatalogUnavailable)})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "REFERENCE_DATA_UNAVAILABLE")
}

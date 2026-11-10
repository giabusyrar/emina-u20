package workers_test

import (
	"encoding/json"
	"image"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/workers"
)

func createTestImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for x := 0; x < 50; x++ {
		for y := 0; y < 50; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 180, B: 170, A: 255})
		}
	}
	return img
}

func TestSegmenterWorker_MockAIService_Success(t *testing.T) {
	// Mock Python AI Service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/segment-and-pose", r.URL.Path)
		resp := map[string]interface{}{
			"status": "SUCCESS",
			"headPose": map[string]interface{}{
				"detectedAngle": "FRONT",
				"yaw":           -1.2,
				"pitch":         2.4,
				"roll":          0.5,
				"confidence":    0.97,
			},
			"landmarksTotal": 468,
			"zones": []map[string]interface{}{
				{
					"zoneCode":   "ZONE_FOREHEAD",
					"zoneName":   "Forehead",
					"isVisible":  true,
					"isOccluded": false,
					"confidence": 0.98,
					"boundingBox": map[string]float64{
						"minX": 0.25, "minY": 0.10, "maxX": 0.75, "maxY": 0.30,
					},
					"polygon": []map[string]float64{
						{"x": 0.25, "y": 0.12},
						{"x": 0.50, "y": 0.08},
						{"x": 0.75, "y": 0.12},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	worker := workers.NewSegmenterWorkerWithURL(server.URL, 5*time.Second)
	zones, err := worker.Segment(domain.AngleFront, createTestImage())

	assert.NoError(t, err)
	assert.NotEmpty(t, zones)
	assert.Equal(t, "ZONE_FOREHEAD", zones[0].ZoneCode)
	assert.True(t, zones[0].IsVisible)
}

func TestSegmenterWorker_UnreachableIsUnavailableNotTemplateZones(t *testing.T) {
	worker := workers.NewSegmenterWorkerWithURL("http://127.0.0.1:59999", 5*time.Second)
	zones, err := worker.Segment(domain.AngleFront, createTestImage())

	assert.ErrorIs(t, err, workers.ErrSegmentationUnavailable)
	assert.Empty(t, zones)
}

func TestSegmenterWorker_Skin503CarriesReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":{"code":"LANDMARKER_UNAVAILABLE","message":"no FaceLandmarker model path is configured"}}`))
	}))
	defer server.Close()

	_, err := workers.NewSegmenterWorkerWithURL(server.URL, 5*time.Second).Segment(domain.AngleFront, createTestImage())
	assert.ErrorIs(t, err, workers.ErrSegmentationUnavailable)
	assert.Contains(t, err.Error(), "LANDMARKER_UNAVAILABLE")
}

func TestSegmenterWorker_Skin422IsNoFace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"Failed to identify facial landmarks from image"}`))
	}))
	defer server.Close()

	_, err := workers.NewSegmenterWorkerWithURL(server.URL, 5*time.Second).Segment(domain.AngleFront, createTestImage())
	assert.ErrorIs(t, err, workers.ErrNoFace)
	assert.NotErrorIs(t, err, workers.ErrSegmentationUnavailable)
}

func TestSegmenterWorker_SendsTheOriginalUploadBytes(t *testing.T) {
	original := []byte("ÿØoriginal-jpeg-bytes")
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("image")
		if err == nil {
			received, _ = io.ReadAll(file)
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	_, _, _, _, _, _ = workers.NewSegmenterWorkerWithURL(server.URL, 5*time.Second).SegmentWithPose(domain.AngleFront, createTestImage(), original)
	assert.Equal(t, original, received, "the upload must reach skin unchanged, not re-encoded")
}

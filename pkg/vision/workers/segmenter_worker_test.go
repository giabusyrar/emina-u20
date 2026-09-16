package workers_test

import (
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/workers"
	"github.com/stretchr/testify/assert"
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

	worker := workers.NewSegmenterWorkerWithURL(server.URL)
	zones, err := worker.Segment(domain.AngleFront, createTestImage())

	assert.NoError(t, err)
	assert.NotEmpty(t, zones)
	assert.Equal(t, "ZONE_FOREHEAD", zones[0].ZoneCode)
	assert.Equal(t, 0.98, zones[0].Confidence)
}

func TestSegmenterWorker_OfflineFallback(t *testing.T) {
	// Worker pointing to unreachable port -> triggers fallback
	worker := workers.NewSegmenterWorkerWithURL("http://127.0.0.1:59999")
	zones, err := worker.Segment(domain.AngleFront, createTestImage())

	assert.NoError(t, err)
	assert.Len(t, zones, 8) // Fallback emits all 8 standard zones
	assert.Equal(t, "ZONE_FOREHEAD", zones[0].ZoneCode)
}

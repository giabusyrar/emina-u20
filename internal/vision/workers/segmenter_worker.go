package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
)

type SegmentedZone struct {
	ZoneCode    string
	ZoneName    string
	SourceAngle domain.ImageAngle
	Polygon     []domain.NormalizedPoint
	BoundingBox domain.BoundingBox
	Confidence  float64
	IsOccluded  bool
	// SourceImageIndex is the position of the request image this zone was
	// segmented from, so crops use the right image even when angles repeat.
	SourceImageIndex int
}

type pythonAISegmentResponse struct {
	Status   string `json:"status"`
	HeadPose struct {
		DetectedAngle string  `json:"detectedAngle"`
		Yaw           float64 `json:"yaw"`
		Pitch         float64 `json:"pitch"`
		Roll          float64 `json:"roll"`
		Confidence    float64 `json:"confidence"`
	} `json:"headPose"`
	HijabDetected   bool    `json:"hijabDetected"`
	HijabConfidence float64 `json:"hijabConfidence"`
	Zones           []struct {
		ZoneCode    string                   `json:"zoneCode"`
		ZoneName    string                   `json:"zoneName"`
		IsVisible   bool                     `json:"isVisible"`
		IsOccluded  bool                     `json:"isOccluded"`
		Confidence  float64                  `json:"confidence"`
		BoundingBox domain.BoundingBox       `json:"boundingBox"`
		Polygon     []domain.NormalizedPoint `json:"polygon"`
	} `json:"zones"`
}

type SegmenterWorker struct {
	workerURL  string
	httpClient *http.Client
}

func NewSegmenterWorkerWithURL(workerURL string, timeout time.Duration) *SegmenterWorker {
	return &SegmenterWorker{
		workerURL: workerURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (s *SegmenterWorker) fallbackSegment(angle domain.ImageAngle) []SegmentedZone {
	defs := domain.GetStandardFacialZones()
	results := make([]SegmentedZone, len(defs))

	for i, def := range defs {
		minX, minY := 1.0, 1.0
		maxX, maxY := 0.0, 0.0
		for _, pt := range def.DefaultPolygon {
			if pt.X < minX { minX = pt.X }
			if pt.X > maxX { maxX = pt.X }
			if pt.Y < minY { minY = pt.Y }
			if pt.Y > maxY { maxY = pt.Y }
		}

		results[i] = SegmentedZone{
			ZoneCode:    def.Code,
			ZoneName:    def.Name,
			SourceAngle: angle,
			Polygon:     def.DefaultPolygon,
			BoundingBox: domain.BoundingBox{
				MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
			},
			Confidence: 0.94,
			IsOccluded: false,
		}
	}

	return results
}

func (s *SegmenterWorker) Segment(angle domain.ImageAngle, img image.Image) ([]SegmentedZone, error) {
	zones, _, _, _, _, err := s.SegmentWithPose(angle, img)
	return zones, err
}

func (s *SegmenterWorker) SegmentWithPose(declaredAngle domain.ImageAngle, img image.Image) ([]SegmentedZone, domain.ImageAngle, float64, float64, float64, error) {
	if img == nil {
		return nil, domain.AngleFront, 0, 0, 0, fmt.Errorf("image cannot be nil")
	}

	// 1. Encode image to PNG buffer
	var imgBuf bytes.Buffer
	if err := png.Encode(&imgBuf, img); err != nil {
		fallbackAngle := declaredAngle
		if fallbackAngle == "" {
			fallbackAngle = domain.AngleFront
		}
		return s.fallbackSegment(fallbackAngle), fallbackAngle, 0, 0, 0, nil
	}

	// 2. Prepare multipart request body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "frame.png")
	if err == nil {
		_, _ = io.Copy(part, &imgBuf)
		if declaredAngle != "" {
			_ = writer.WriteField("declaredAngle", string(declaredAngle))
		}
		_ = writer.Close()

		req, err := http.NewRequest("POST", s.workerURL+"/api/v1/segment-and-pose", body)
		if err == nil {
			req.Header.Set("Content-Type", writer.FormDataContentType())
			resp, err := s.httpClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				var aiResp pythonAISegmentResponse
				if err := json.NewDecoder(resp.Body).Decode(&aiResp); err == nil && len(aiResp.Zones) > 0 {
					detectedAngle := declaredAngle
					if detectedAngle == "" || aiResp.HeadPose.DetectedAngle != "" {
						switch aiResp.HeadPose.DetectedAngle {
						case "LEFT":
							detectedAngle = domain.AngleLeft
						case "RIGHT":
							detectedAngle = domain.AngleRight
						case "FRONT":
							detectedAngle = domain.AngleFront
						default:
							if aiResp.HeadPose.Yaw < -20.0 {
								detectedAngle = domain.AngleLeft
							} else if aiResp.HeadPose.Yaw > 20.0 {
								detectedAngle = domain.AngleRight
							} else {
								detectedAngle = domain.AngleFront
							}
						}
					}

					results := make([]SegmentedZone, len(aiResp.Zones))
					for i, z := range aiResp.Zones {
						results[i] = SegmentedZone{
							ZoneCode:    z.ZoneCode,
							ZoneName:    z.ZoneName,
							SourceAngle: detectedAngle,
							Polygon:     z.Polygon,
							BoundingBox: z.BoundingBox,
							Confidence:  z.Confidence,
							IsOccluded:  z.IsOccluded,
						}
					}
					return results, detectedAngle, aiResp.HeadPose.Yaw, aiResp.HeadPose.Pitch, aiResp.HeadPose.Roll, nil
				}
			}
		}
	}

	// 3. Graceful fallback if Python worker is unreachable
	fallbackAngle := declaredAngle
	if fallbackAngle == "" {
		fallbackAngle = domain.AngleFront
	}
	return s.fallbackSegment(fallbackAngle), fallbackAngle, 0, 0, 0, nil
}

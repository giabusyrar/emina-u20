package workers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
)

// ErrSegmentationUnavailable: the skin worker could not segment the image
// (unreachable, no model loaded, or an unusable reply). There is no
// substitute: callers report it instead of using template zones.
var ErrSegmentationUnavailable = errors.New("skin segmentation unavailable")

// ErrNoFace: the skin worker measured the image and found no face.
var ErrNoFace = errors.New("no face detected")

type SegmentedZone struct {
	ZoneCode    string
	ZoneName    string
	SourceAngle domain.ImageAngle
	Polygon     []domain.NormalizedPoint
	BoundingBox domain.BoundingBox
	// IsVisible is false when the zone is out of frame for the detected
	// angle (skin's measurement). No per-zone confidence or occlusion is
	// measured, so none is carried.
	IsVisible bool
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
	} `json:"headPose"`
	Zones []struct {
		ZoneCode    string                   `json:"zoneCode"`
		ZoneName    string                   `json:"zoneName"`
		IsVisible   bool                     `json:"isVisible"`
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

func (s *SegmenterWorker) Segment(angle domain.ImageAngle, img image.Image) ([]SegmentedZone, error) {
	zones, _, _, _, _, err := s.SegmentWithPose(angle, img, nil)
	return zones, err
}

// raw is the upload exactly as received; when present it is sent to the skin
// worker as is, so the photo is not decoded and re-encoded on the way. Without
// it the decoded image is sent as PNG.
func (s *SegmenterWorker) SegmentWithPose(declaredAngle domain.ImageAngle, img image.Image, raw []byte) ([]SegmentedZone, domain.ImageAngle, float64, float64, float64, error) {
	if img == nil {
		return nil, domain.AngleFront, 0, 0, 0, fmt.Errorf("image cannot be nil")
	}
	aiResp, err := s.callSkin(declaredAngle, img, raw)
	if err != nil {
		return nil, declaredAngle, 0, 0, 0, err
	}

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
			IsVisible:   z.IsVisible,
		}
	}
	return results, detectedAngle, aiResp.HeadPose.Yaw, aiResp.HeadPose.Pitch, aiResp.HeadPose.Roll, nil
}

// callSkin posts the image to the skin worker's segment-and-pose. A 422 is
// ErrNoFace; every other failure is ErrSegmentationUnavailable with the
// worker's reason.
func (s *SegmenterWorker) callSkin(declaredAngle domain.ImageAngle, img image.Image, raw []byte) (*pythonAISegmentResponse, error) {
	var imgBuf bytes.Buffer
	if len(raw) > 0 {
		imgBuf.Write(raw)
	} else if err := png.Encode(&imgBuf, img); err != nil {
		return nil, fmt.Errorf("encode image for segmentation: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "frame")
	if err != nil {
		return nil, fmt.Errorf("build segmentation request: %w", err)
	}
	if _, err := io.Copy(part, &imgBuf); err != nil {
		return nil, fmt.Errorf("build segmentation request: %w", err)
	}
	if declaredAngle != "" {
		_ = writer.WriteField("declaredAngle", string(declaredAngle))
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("build segmentation request: %w", err)
	}

	req, err := http.NewRequest("POST", s.workerURL+"/api/v1/segment-and-pose", body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSegmentationUnavailable, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSegmentationUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reason := skinErrorReason(resp)
		if resp.StatusCode == http.StatusUnprocessableEntity {
			return nil, fmt.Errorf("%w: %s", ErrNoFace, reason)
		}
		return nil, fmt.Errorf("%w: skin worker returned %d: %s", ErrSegmentationUnavailable, resp.StatusCode, reason)
	}
	var aiResp pythonAISegmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, fmt.Errorf("%w: decode segment-and-pose response: %v", ErrSegmentationUnavailable, err)
	}
	if len(aiResp.Zones) == 0 {
		return nil, fmt.Errorf("%w: segment-and-pose returned no zones", ErrSegmentationUnavailable)
	}
	return &aiResp, nil
}

// skinErrorReason extracts FastAPI's "detail" (a string, or {code, message})
// from an error reply, falling back to the raw body.
func skinErrorReason(resp *http.Response) string {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var body struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(raw, &body) == nil && len(body.Detail) > 0 {
		var text string
		if json.Unmarshal(body.Detail, &text) == nil {
			return text
		}
		var coded struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body.Detail, &coded) == nil && coded.Code != "" {
			return coded.Code + ": " + coded.Message
		}
	}
	return string(raw)
}

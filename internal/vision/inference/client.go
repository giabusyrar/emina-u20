package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

const dispatchZonesPath = "/api/v1/models/dispatch-zones"

// ZoneCrop is one facial zone cut out of its source image.
type ZoneCrop struct {
	ZoneCode    string
	SourceAngle domain.ImageAngle
	Image       []byte // PNG
}

// ZoneScores holds the severity scores (0-100, 100 = worst) the worker
// produced for one zone, keyed by capability code.
type ZoneScores struct {
	ZoneCode    string
	SourceAngle domain.ImageAngle
	Telemetry   map[string]float64
}

type DispatchResult struct {
	Zones                   []ZoneScores
	UnsupportedCapabilities []string
	// UnavailableCapabilities maps a capability whose bound model could not
	// be loaded to the model server's reason. dispatch-zones also lists these
	// in UnsupportedCapabilities.
	UnavailableCapabilities map[string]string
}

// ErrWorkerUnavailable wraps every failure to get scores from the vision AI worker.
var ErrWorkerUnavailable = errors.New("vision AI worker unavailable")

// CapabilityClient runs capability models on zone crops.
type CapabilityClient interface {
	DispatchZones(ctx context.Context, capabilities []string, crops []ZoneCrop) (*DispatchResult, error)
}

type HTTPCapabilityClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPCapabilityClient(baseURL string, timeout time.Duration) *HTTPCapabilityClient {
	return &HTTPCapabilityClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

type dispatchZone struct {
	ZoneCode    string `json:"zoneCode"`
	SourceAngle string `json:"sourceAngle"`
	Image       string `json:"image"`
}

type dispatchRequest struct {
	Capabilities []string       `json:"capabilities"`
	Zones        []dispatchZone `json:"zones"`
}

type dispatchZoneResult struct {
	ZoneCode    string             `json:"zoneCode"`
	SourceAngle string             `json:"sourceAngle"`
	Telemetry   map[string]float64 `json:"telemetry"`
}

type dispatchResponse struct {
	Results                 []dispatchZoneResult `json:"results"`
	UnsupportedCapabilities []string             `json:"unsupportedCapabilities"`
	UnavailableCapabilities map[string]string    `json:"unavailableCapabilities"`
}

func (c *HTTPCapabilityClient) DispatchZones(ctx context.Context, capabilities []string, crops []ZoneCrop) (*DispatchResult, error) {
	body := dispatchRequest{Capabilities: capabilities, Zones: make([]dispatchZone, len(crops))}
	for i, crop := range crops {
		body.Zones[i] = dispatchZone{
			ZoneCode:    crop.ZoneCode,
			SourceAngle: string(crop.SourceAngle),
			Image:       base64.StdEncoding.EncodeToString(crop.Image),
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode dispatch-zones request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+dispatchZonesPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorkerUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorkerUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: dispatch-zones returned status %d", ErrWorkerUnavailable, resp.StatusCode)
	}

	var decoded dispatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: decode dispatch-zones response: %v", ErrWorkerUnavailable, err)
	}

	result := &DispatchResult{
		Zones:                   make([]ZoneScores, len(decoded.Results)),
		UnsupportedCapabilities: decoded.UnsupportedCapabilities,
		UnavailableCapabilities: decoded.UnavailableCapabilities,
	}
	for i, r := range decoded.Results {
		result.Zones[i] = ZoneScores{
			ZoneCode:    r.ZoneCode,
			SourceAngle: domain.ImageAngle(r.SourceAngle),
			Telemetry:   r.Telemetry,
		}
	}
	return result, nil
}

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const referenceTimeout = 3 * time.Second

type referenceDimension struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type referenceSkinCondition struct {
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	DimensionCode      string   `json:"dimensionCode"`
	VisionCapabilities []string `json:"visionCapabilities"`
}

// ReferenceClient reads dimensions and skin conditions from reference-service.
type ReferenceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewReferenceClient(baseURL string) *ReferenceClient {
	return &ReferenceClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: referenceTimeout},
	}
}

func (c *ReferenceClient) Fetch(ctx context.Context) (*Snapshot, error) {
	var dimensions []referenceDimension
	if err := c.getList(ctx, "/api/reference/dimensions", &dimensions); err != nil {
		return nil, err
	}
	var skinConditions []referenceSkinCondition
	if err := c.getList(ctx, "/api/reference/skin-conditions", &skinConditions); err != nil {
		return nil, err
	}
	return buildSnapshot(dimensions, skinConditions), nil
}

func (c *ReferenceClient) getList(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("GET %s: decode: %w", path, err)
	}
	if len(envelope.Data) == 0 {
		return fmt.Errorf("GET %s: response has no data field", path)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("GET %s: decode data: %w", path, err)
	}
	return nil
}

// buildSnapshot keeps skin conditions with at least one capability and
// dimensions with at least one such condition, preserving Reference order.
func buildSnapshot(dimensions []referenceDimension, skinConditions []referenceSkinCondition) *Snapshot {
	snapshot := &Snapshot{}
	conditionsByDimension := make(map[string][]Condition)

	for _, rc := range skinConditions {
		capabilities := cleanCodes(rc.VisionCapabilities)
		if len(capabilities) == 0 {
			continue
		}
		condition := Condition{
			Code:          rc.Code,
			Name:          rc.Name,
			Description:   rc.Description,
			DimensionCode: rc.DimensionCode,
			Capabilities:  capabilities,
		}
		snapshot.Conditions = append(snapshot.Conditions, condition)
		conditionsByDimension[rc.DimensionCode] = append(conditionsByDimension[rc.DimensionCode], condition)
	}

	for _, rd := range dimensions {
		conditions := conditionsByDimension[rd.Code]
		if len(conditions) == 0 {
			continue
		}
		snapshot.Dimensions = append(snapshot.Dimensions, Dimension{
			Code:        rd.Code,
			Name:        rd.Name,
			Description: rd.Description,
			Conditions:  conditions,
		})
	}

	return snapshot
}

func cleanCodes(codes []string) []string {
	seen := make(map[string]bool, len(codes))
	var cleaned []string
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		cleaned = append(cleaned, code)
	}
	return cleaned
}

// Package tryonclient asks tryon-engine to generate a shade's photorealistic
// UV texture. It implements service.ShadeExtractor.
package tryonclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
)

type Client struct {
	baseURL string
	http    *http.Client
}

// New builds a client for tryon-engine at baseURL (TRYON_ENGINE_URL). An
// empty baseURL is allowed: every call then fails with a clear error.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: http.DefaultClient}
}

// TriggerExtraction posts the shade to tryon-engine's /extract. tryon-engine
// reports back via the shade's extraction-callback route.
func (c *Client) TriggerExtraction(ctx context.Context, shade entity.Shade) error {
	if c.baseURL == "" {
		return fmt.Errorf("TRYON_ENGINE_URL is not configured")
	}

	payload, err := json.Marshal(map[string]string{
		"shade_id":            shade.ID,
		"reference_photo_url": shade.ReferencePhotoURL,
		"hex_color":           shade.HexColor,
		"region":              shade.Region,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/extract", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("tryon-engine returned status %d", resp.StatusCode)
	}
	return nil
}

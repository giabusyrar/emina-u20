// Package scoreclient forwards a validated, assembled evaluation
// payload to the Score Module (a separate service — apps/services/
// score-engine) to compute dimension_scores and resolver
// classifications (spec Section 2.2 step 6). This module never
// performs that math itself.
package scoreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/experience-gateway/apps/core-engine/pkg/form/domain"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Evaluate forwards the assembled answer_list/customer_condition/
// dimensions payload and returns the Score Module's computed
// dimension_scores.
func (c *Client) Evaluate(ctx context.Context, req domain.ScoreModuleRequest) (*domain.ScoreModuleResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/scoring/evaluate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: score module unreachable: %v", domain.ErrUpstream, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: score module returned %d: %s", domain.ErrUpstream, resp.StatusCode, string(respBody))
	}

	var out domain.ScoreModuleResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("%w: invalid score module response: %v", domain.ErrUpstream, err)
	}
	return &out, nil
}

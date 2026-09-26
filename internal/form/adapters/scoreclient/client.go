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
	"net/url"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
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

// ActiveRuleset is the subset of the Score Module's ruleset entity callers
// outside that module need — just enough to read field_mapping/
// dimension_fusion out of Schema without depending on score/entity directly.
type ActiveRuleset struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Status string `json:"status"`
	Schema string `json:"schema"`
}

// ListRulesets fetches every ruleset registered for (brand, application) —
// used to resolve which ruleset a survey belongs to via its schema's own
// form_survey_code, since a survey's code and its ruleset's code are not
// guaranteed to match (e.g. survey "skinverse_q1_q6" is scored by ruleset
// "skinverse_skin_character").
func (c *Client) ListRulesets(ctx context.Context, brandID, appID string) ([]ActiveRuleset, error) {
	reqURL := fmt.Sprintf("%s/api/scoring/rulesets?brand_id=%s&application_id=%s",
		c.BaseURL, url.QueryEscape(brandID), url.QueryEscape(appID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

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

	var out struct {
		Success  bool            `json:"success"`
		Rulesets []ActiveRuleset `json:"rulesets"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("%w: invalid score module response: %v", domain.ErrUpstream, err)
	}
	return out.Rulesets, nil
}

// GetActiveRuleset fetches the same (brand, application, code) -> ruleset the
// Score Module's own Evaluate uses, so a caller can read its field_mapping
// before building a vision_signals payload.
func (c *Client) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*ActiveRuleset, error) {
	reqURL := fmt.Sprintf("%s/api/scoring/rulesets/active?brand_id=%s&application_id=%s&code=%s",
		c.BaseURL, url.QueryEscape(brandID), url.QueryEscape(appID), url.QueryEscape(code))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

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

	var out struct {
		Success bool          `json:"success"`
		Ruleset ActiveRuleset `json:"ruleset"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("%w: invalid score module response: %v", domain.ErrUpstream, err)
	}
	return &out.Ruleset, nil
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

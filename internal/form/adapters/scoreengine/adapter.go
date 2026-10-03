// Package scoreengine gives the Form Engine the Score Module in-process.
//
// Both engines run in the same core-engine binary; this replaces the former
// HTTP client (form/adapters/scoreclient) that called this very process over
// loopback. The two engines keep separate copies of the score request and
// response types, so the adapter converts between them through their shared
// JSON contract -- the exact contract the HTTP call used -- instead of
// hand-mapping fields that could drift.
package scoreengine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	formdomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
	scoredomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/entity"
)

// Scorer is the part of score's ScoreService the Form Engine uses.
type Scorer interface {
	Evaluate(ctx context.Context, req scoredomain.ScoreModuleRequest) (*scoredomain.ScoreModuleResponse, error)
	ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error)
	GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error)
}

// Ruleset is the subset of a score ruleset the Form Engine reads: enough to
// find a survey's ruleset and read field_mapping/dimension_fusion out of its
// Schema, without depending on score's entity package.
type Ruleset struct {
	ID     string
	Code   string
	Status string
	Schema string
}

type Adapter struct {
	scorer  Scorer
	timeout time.Duration
}

// New wraps scorer. timeout bounds each call (SCORE_CLIENT_TIMEOUT), as the
// HTTP client's timeout did.
func New(scorer Scorer, timeout time.Duration) *Adapter {
	return &Adapter{scorer: scorer, timeout: timeout}
}

// Evaluate runs the Score Module on the assembled payload.
func (a *Adapter) Evaluate(ctx context.Context, req formdomain.ScoreModuleRequest) (*formdomain.ScoreModuleResponse, error) {
	var scoreReq scoredomain.ScoreModuleRequest
	if err := convert(req, &scoreReq); err != nil {
		return nil, fmt.Errorf("%w: score request: %v", formdomain.ErrUpstream, err)
	}

	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.scorer.Evaluate(ctx, scoreReq)
	if err != nil {
		return nil, fmt.Errorf("%w: score module: %v", formdomain.ErrUpstream, err)
	}

	var out formdomain.ScoreModuleResponse
	if err := convert(resp, &out); err != nil {
		return nil, fmt.Errorf("%w: score response: %v", formdomain.ErrUpstream, err)
	}
	return &out, nil
}

// ListRulesets returns every ruleset registered for (brand, application).
func (a *Adapter) ListRulesets(ctx context.Context, brandID, appID string) ([]Ruleset, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	rulesets, err := a.scorer.ListRulesets(ctx, brandID, appID)
	if err != nil {
		return nil, fmt.Errorf("%w: score module: %v", formdomain.ErrUpstream, err)
	}
	out := make([]Ruleset, len(rulesets))
	for i, r := range rulesets {
		out[i] = toRuleset(r)
	}
	return out, nil
}

// GetActiveRuleset resolves the ruleset the Score Module's Evaluate would use.
// A missing ruleset is an error, as the former HTTP path's 404 was.
func (a *Adapter) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*Ruleset, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	ruleset, err := a.scorer.GetActiveRuleset(ctx, brandID, appID, code)
	if err != nil {
		return nil, fmt.Errorf("%w: score module: %v", formdomain.ErrUpstream, err)
	}
	if ruleset == nil {
		return nil, fmt.Errorf("%w: active ruleset not found", formdomain.ErrUpstream)
	}
	r := toRuleset(*ruleset)
	return &r, nil
}

func toRuleset(r entity.ScoreRuleset) Ruleset {
	return Ruleset{ID: r.ID, Code: r.Code, Status: r.Status, Schema: r.Schema}
}

// convert copies src into dst through their JSON representation.
func convert(src, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

package entity

import "time"

// TieringItem defines a unified tier entry
type TieringItem struct {
	MinScore  float64 `json:"minScore"`
	MaxScore  float64 `json:"maxScore"`
	ValueCode string  `json:"valueCode"`
	Label     string  `json:"label"`
	GradeName string  `json:"gradeName"`
	Severity  string  `json:"severity"` // optimal, mild, moderate, severe, critical
}

// SkinProfileItem is a dynamic diagnostic skin profile DTO
type SkinProfileItem struct {
	Code          string            `json:"code"`
	BrandID       string            `json:"brandId,omitempty"`
	ApplicationID string            `json:"applicationId,omitempty"`
	Category      string            `json:"category,omitempty"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	AxisValues    map[string]string `json:"axis_values,omitempty"`
	Traits        []string          `json:"traits,omitempty"`
	CreatedAt     time.Time         `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time        `json:"updatedAt,omitempty"`
}

// RulesetSimulationRequest is the payload for testing rulesets in Admin
// Studio. FormScores/VisionScores are HEALTH-space (100 = sehat, matching
// what the slider itself is labelled) — Simulate converts each to concern
// space before handing off to the same stage2Score fusion Evaluate uses, so
// the two can never quietly drift apart. AgeYears feeds the ruleset's
// age_over_30-mapped dimension via the exact same AgeOverThirty the real
// evaluate path uses (not a re-derived <=30 comparison), overriding any
// FormScores entry for that same dimension.
type RulesetSimulationRequest struct {
	Schema            string                 `json:"schema"`
	FormScores        map[string]float64     `json:"form_scores,omitempty"`
	VisionScores      map[string]float64     `json:"vision_scores,omitempty"`
	AgeYears          *int                   `json:"age_years,omitempty"`
	CustomerCondition map[string]bool        `json:"customer_condition,omitempty"`
	TargetPayload     map[string]interface{} `json:"target_payload,omitempty"`
}

// RulesetSimulationResponse is the response from simulation testing
type RulesetSimulationResponse struct {
	Success     bool                   `json:"success"`
	Performance string                 `json:"performance"`
	Result      map[string]interface{} `json:"result"`
	Error       string                 `json:"error,omitempty"`
}

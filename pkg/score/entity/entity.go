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

// RulesetSimulationRequest is the payload for testing rulesets in Admin Studio
type RulesetSimulationRequest struct {
	Schema            string                 `json:"schema"`
	DimensionScores   map[string]float64     `json:"dimension_scores"`
	VisionSignals     map[string]float64     `json:"vision_signals,omitempty"`
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

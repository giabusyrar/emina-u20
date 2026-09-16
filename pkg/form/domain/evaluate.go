package domain

// AnswerSubmission is one entry in the thin POST /evaluate request
// body (Section 9.2) — a raw selection/value keyed by question id.
type AnswerSubmission struct {
	QuestionID string      `json:"question_id"`
	Value      interface{} `json:"value"` // option value (string) or numeric_input value (number)
}

// EvaluateRequest is the full POST /v1/questionnaire/:code/evaluate body.
type EvaluateRequest struct {
	BrandID            string                 `json:"brand_id"`
	ApplicationID      string                 `json:"application_id"`
	Answers            []AnswerSubmission     `json:"answers"`
	CustomerConditions map[string]bool        `json:"customer_conditions,omitempty"`
	AnalysisResult     map[string]interface{} `json:"analysis_result,omitempty"`
}

// AnswerEntry is one resolved answer in the Section 7 output shape —
// self-contained, carrying its own score and the scale it was scored
// against.
type AnswerEntry struct {
	Answer   string `json:"answer"`
	Score    int    `json:"score"`
	MinScore int    `json:"min_score"`
	MaxScore int    `json:"max_score"`
}

// SkinProfileType represents a parameterized diagnostic skin profile computed dynamically from active axes
type SkinProfileType struct {
	Code        string            `json:"code"`                  // e.g. "OSPT", "DRNW", "LONGEVITY-A"
	Name        string            `json:"name"`                  // e.g. "Oily, Sensitive, Pigmented, Tight"
	Description string            `json:"description"`           // Diagnostic clinical overview
	Category    string            `json:"category,omitempty"`
	AxisValues  map[string]string `json:"axis_values,omitempty"` // dynamic map: {"OILINESS": "O", "SENSITIVITY": "S", ...}
	Traits      []string          `json:"traits,omitempty"`      // ["Oily", "Sensitive", "Pigmented", "Tight"]
}

// SkinGradingTier represents a clinical severity diagnosis for a single dimension
type SkinGradingTier struct {
	DimensionKey string  `json:"dimension_key"`
	Score        float64 `json:"score"`
	GradeName    string  `json:"grade_name"` // e.g. "Seborrheic Oily (Grade III)"
	Severity     string  `json:"severity"`   // mild, moderate, severe, critical
}

// EvaluationOutput is the unified diagnostic response shape returned by
// POST /v1/survey/:code/evaluate. Evaluation itself is stateless; persistence
// of the submission belongs to the Gateway Assessment layer.
type EvaluationOutput struct {
	Success           bool                   `json:"success"`
	Code              string                 `json:"code"`
	BrandID           string                 `json:"brand_id"`
	ApplicationID     string                 `json:"application_id"`
	CustomerID        string                 `json:"customer_id,omitempty"`
	TotalScore        float64                `json:"total_score"`
	DimensionScores   map[string]float64     `json:"dimension_scores"`
	ScoreRange        string                 `json:"score_range"`
	SeverityLevel     string                 `json:"severity_level"`
	SkinConcern       *SkinConcernFocus      `json:"skin_concern,omitempty"`
	SkinProfile       SkinProfileType        `json:"skin_profile"`
	SkinGradingTiers  []SkinGradingTier      `json:"skin_grading_tiers"`
	CustomerCondition map[string]bool        `json:"customer_condition"`
	AnswerList        []AnswerEntry          `json:"answer_list"`
	AnalysisResult    map[string]interface{} `json:"analysis_result,omitempty"`
	AppliedRules      []string               `json:"applied_rules,omitempty"`
	EvaluatedAt       string                 `json:"evaluated_at,omitempty"`
}

// ScoreModuleRequest is what the Form Module forwards to the Score
// Module after validating and assembling a submission (Section 2.2
// step 6, Section 9.2). The Score Module computes dimension_scores
// and any resolver classifications from this.
type ScoreModuleRequest struct {
	Code              string                 `json:"code"`
	BrandID           string                 `json:"brand_id"`
	ApplicationID     string                 `json:"application_id"`
	CustomerID        string                 `json:"customer_id,omitempty"`
	AnswerList        []AnswerEntry          `json:"answer_list"`
	CustomerCondition map[string]bool        `json:"customer_condition"`
	Dimensions        []DimensionRef         `json:"dimensions"`
	VisionSignals     map[string]float64     `json:"vision_signals,omitempty"`
	TargetPayload     map[string]interface{} `json:"target_payload,omitempty"`
}

// DimensionRef tells the Score Module which registered dimensions
// this questionnaire's questions roll up into and each one's ceiling
// / resolver, so it can compute totals without re-fetching the
// registry itself.
type DimensionRef struct {
	Key               string `json:"key"`
	MinScore          int    `json:"min_score,omitempty"`
	MaxScore          int    `json:"max_score"`
	CalculationMethod string `json:"calculation_method,omitempty"`
	Resolver          string `json:"resolver,omitempty"`
	Answers           []int  `json:"answers"` // the raw per-answer scores that roll up into this dimension
}

// SkinConcernFocus mirrors the Score Module's dominant-concern facet.
type SkinConcernFocus struct {
	Dimension string  `json:"dimension"`
	Label     string  `json:"label"`
	Score     float64 `json:"score"`
}

// ScoreModuleResponse is what the Score Module returns for a
// forwarded evaluation.
type ScoreModuleResponse struct {
	Success         bool               `json:"success"`
	DimensionScores map[string]float64 `json:"dimension_scores"`
	TotalScore      float64            `json:"total_score"`

	// GRAND_DESIGN §2.3.3 Skin Grading facets, derived from total_score.
	ScoreRange    string            `json:"score_range"`
	SeverityLevel string            `json:"severity_level"`
	SkinConcern   *SkinConcernFocus `json:"skin_concern,omitempty"`

	SkinProfile       SkinProfileType   `json:"skin_profile"`
	SkinGradingTiers  []SkinGradingTier `json:"skin_grading_tiers"`
	CustomerCondition map[string]bool   `json:"customer_conditions,omitempty"`
	AppliedRules      []string          `json:"applied_rules,omitempty"`
}

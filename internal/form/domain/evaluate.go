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
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Score    int    `json:"score"`
	MinScore int    `json:"min_score"`
	MaxScore int    `json:"max_score"`
}

// SkinProfileType represents a parameterized diagnostic skin profile computed dynamically from active axes
type SkinProfileType struct {
	Code        string            `json:"code"`        // e.g. "OSPT", "DRNW", "LONGEVITY-A"
	Name        string            `json:"name"`        // e.g. "Oily, Sensitive, Pigmented, Tight"
	Description string            `json:"description"` // Diagnostic clinical overview
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

// Warning explains something the caller should know about a result that
// still succeeded, e.g. that it could not be stored.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WarningAssessmentNotSaved: the evaluation succeeded but its record could
// not be written to customer_assessments.
const WarningAssessmentNotSaved = "ASSESSMENT_NOT_SAVED"

// EvaluationOutput is the unified diagnostic response shape returned by
// POST /v1/survey/:code/evaluate. Scoring itself is stateless; the HTTP
// handler stores each successful result as a customer assessment
// (internal/assessment) and reports the stored id, or a warning if storing
// failed.
type EvaluationOutput struct {
	Success            bool                   `json:"success"`
	Code               string                 `json:"code"`
	BrandID            string                 `json:"brand_id"`
	ApplicationID      string                 `json:"application_id"`
	CustomerID         string                 `json:"customer_id,omitempty"`
	TotalScore         float64                `json:"total_score"`
	DimensionScores    map[string]float64     `json:"dimension_scores"`
	ScoreRange         string                 `json:"score_range"`
	SeverityLevel      string                 `json:"severity_level"`
	SkinConcern        *SkinConcernFocus      `json:"skin_concern,omitempty"`
	SkinProfile        SkinProfileType        `json:"skin_profile"`
	SkinGradingTiers   []SkinGradingTier      `json:"skin_grading_tiers"`
	CustomerCondition  map[string]bool        `json:"customer_condition"`
	AnswerList         []AnswerEntry          `json:"answer_list"`
	AnalysisResult     map[string]interface{} `json:"analysis_result,omitempty"`
	AppliedRules       []string               `json:"applied_rules,omitempty"`
	SubClassification  map[string]interface{} `json:"sub_classification,omitempty"`
	RawDimensionScores map[string]float64     `json:"raw_dimension_scores,omitempty"`
	// Labels carries every answered question that has no dimension binding
	// at all (question or choice) — e.g. Q6's main concern, or a lifestyle
	// checklist not yet wired into a scored axis — keyed by question name,
	// so a caller can show them as plain metadata instead of digging them
	// out of answer_list (which mixes in real scored answers too). Each
	// value is a plain string for a question that can only ever have one
	// answer, or a []string for one that can carry several (checkbox,
	// tagbox, ranking, matrix).
	Labels      map[string]interface{} `json:"labels,omitempty"`
	EvaluatedAt string                 `json:"evaluated_at,omitempty"`
	// AssessmentID is the customer_assessments row this result was stored
	// as. Empty when it was not stored (see Warnings).
	AssessmentID string    `json:"assessment_id,omitempty"`
	Warnings     []Warning `json:"warnings,omitempty"`
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
	// SubClassification carries ruleset-defined refinement fields that don't
	// fit dimension_scores/axis_values (e.g. sub_classification.acne_type =
	// "Sensitive Acne", sub_classification.pore_severity) — written by a
	// custom decisionTableNode targeting sub_classification.<key>. Mirrors
	// the Score Module's own field name so json.Unmarshal actually picks it
	// up; without this field here it silently disappears at this hop even
	// though the Score Module computes and returns it correctly.
	SubClassification map[string]interface{} `json:"sub_classification,omitempty"`
	// RawDimensionScores mirrors the Score Module's own field — each
	// dimension's raw aggregate before min-max normalisation (e.g. Q1's raw
	// answer sum), needed for a hypothesis band that reads the raw total.
	RawDimensionScores map[string]float64 `json:"raw_dimension_scores,omitempty"`
}

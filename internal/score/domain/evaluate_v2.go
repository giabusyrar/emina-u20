package domain

// DimensionBreakdown is the per-dimension detail the unified evaluate
// response carries, replacing a flat dimension_scores map — it shows exactly
// where each dimension's number came from instead of leaving that implicit.
type DimensionBreakdown struct {
	// Source is "form", "vision", "blend", or "none" (registered on the
	// ruleset but no data at all was available for this particular call).
	Source      string           `json:"source"`
	FormScore   *float64         `json:"form_score"`
	VisionScore *float64         `json:"vision_score"`
	Weight      *DimensionWeight `json:"weight,omitempty"`
	FinalScore  *float64         `json:"final_score"`
	Axis        *string          `json:"axis"`
}

// SkinProfileV2 mirrors SkinProfileType but adds Complete — false when at
// least one of the ruleset's registered Baumann axes has no data this call,
// so code/name/description are known to reflect only a partial read.
type SkinProfileV2 struct {
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	Category    string            `json:"category,omitempty"`
	Description string            `json:"description"`
	Complete    bool              `json:"complete"`
	AxisValues  map[string]string `json:"axis_values,omitempty"`
}

// EvaluateV2Response is the unified evaluate endpoint's response shape.
// score_range/severity_level/skin_concern/skin_grading_tiers from the legacy
// ScoreModuleResponse are deliberately absent — Skinverse-generation rulesets
// never define a generic total-score severity layer, so those fields only
// ever misrepresented a mean-of-axes bucket as an alarming diagnosis.
type EvaluateV2Response struct {
	Success           bool                          `json:"success"`
	Code              string                        `json:"code"` // ruleset code
	BrandID           string                        `json:"brand_id"`
	ApplicationID     string                        `json:"application_id"`
	CustomerID        string                        `json:"customer_id,omitempty"`
	TotalScore        float64                       `json:"total_score"`
	Dimensions        map[string]DimensionBreakdown `json:"dimensions"`
	SkinProfile       SkinProfileV2                 `json:"skin_profile"`
	SubClassification map[string]interface{}        `json:"sub_classification,omitempty"`
	CustomerCondition map[string]bool               `json:"customer_condition"`
	AnswerList        []AnswerEntry                 `json:"answer_list,omitempty"`
	Labels            map[string]interface{}        `json:"labels,omitempty"`
	AppliedRules      []string                      `json:"applied_rules,omitempty"`
	// Warnings is always present, even as an empty array — a caller should
	// never have to distinguish "no warnings" from "this response shape
	// doesn't have that field."
	Warnings            []string `json:"warnings"`
	VisionTransactionID string   `json:"vision_transaction_id,omitempty"`
	EvaluatedAt         string   `json:"evaluated_at,omitempty"`
}

package domain

// AnswerEntry is one resolved answer passed from Form Engine
type AnswerEntry struct {
	Answer   string `json:"answer"`
	Score    int    `json:"score"`
	MinScore int    `json:"min_score"`
	MaxScore int    `json:"max_score"`
}

// DimensionRef specifies dimension rollup configurations
type DimensionRef struct {
	Key               string `json:"key"`
	MinScore          int    `json:"min_score,omitempty"`
	MaxScore          int    `json:"max_score"`
	CalculationMethod string `json:"calculation_method,omitempty"`
	Resolver          string `json:"resolver,omitempty"` // Fallback alias
	Answers           []int  `json:"answers"`
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

// SkinGradingTier represents a per-dimension grading entry: the 0-100 health
// score (100 = optimal) plus its Score Range bucket.
type SkinGradingTier struct {
	DimensionKey string  `json:"dimension_key"`
	Score        float64 `json:"score"`      // 100 = optimal, 0 = critical
	GradeName    string  `json:"grade_name"` // Score Range label: Optimal | Sedang | Perlu Perhatian Khusus
	Severity     string  `json:"severity"`   // same Score Range label, kept for backward compatibility
}

// SkinConcernFocus is the single weakest dimension (argmin of dimension scores),
// surfaced only when that dimension falls below the Optimal band.
type SkinConcernFocus struct {
	Dimension string  `json:"dimension"`
	Label     string  `json:"label"` // clinical concern name, e.g. "Minyak Berlebih"
	Score     float64 `json:"score"`
}

// ScoreModuleRequest is the canonical evaluation request payload
type ScoreModuleRequest struct {
	Code              string                 `json:"code"`
	BrandID           string                 `json:"brand_id"`
	ApplicationID     string                 `json:"application_id"`
	AnswerList        []AnswerEntry          `json:"answer_list"`
	CustomerCondition map[string]bool        `json:"customer_condition"` // Direct safety flags (is_pregnant, uses_retinol, etc.)
	Dimensions        []DimensionRef         `json:"dimensions"`
	VisionSignals     map[string]float64     `json:"vision_signals,omitempty"`
	FusionConfig      *FusionConfig          `json:"fusion_config,omitempty"`
	TargetPayload     map[string]interface{} `json:"target_payload,omitempty"`
}

// ScoreModuleResponse is the output containing calculated scores, skin profile diagnosis, and severity tiers
type ScoreModuleResponse struct {
	Success         bool               `json:"success"`
	DimensionScores map[string]float64 `json:"dimension_scores"`
	TotalScore      float64            `json:"total_score"` // 100 = optimal, 0 = critical

	// Skin Grading facets, all derived from TotalScore.
	ScoreRange    string            `json:"score_range"`    // Perlu Perhatian Khusus | Sedang | Optimal
	SeverityLevel string            `json:"severity_level"` // Sangat Parah | Parah | Sedang | Ringan | Sehat
	SkinConcern   *SkinConcernFocus `json:"skin_concern,omitempty"`

	SkinProfile       SkinProfileType   `json:"skin_profile"`
	SkinGradingTiers  []SkinGradingTier `json:"skin_grading_tiers"`
	CustomerCondition map[string]bool   `json:"customer_conditions"`
	AppliedRules      []string          `json:"applied_rules,omitempty"`

	// SubClassification carries ruleset-defined refinement fields that don't fit
	// axis_values/traits/skin_profile (e.g. sub_sensitive: "Sensitive Acne"),
	// written by a JDM decisionTableNode targeting sub_classification.<key>.
	SubClassification map[string]interface{} `json:"sub_classification,omitempty"`

	// RawDimensionScores is each dimension's aggregate BEFORE min-max
	// normalisation into the 0-100 concern figure DimensionScores carries —
	// e.g. Q1's raw answer sum. A hypothesis band (Sub Oily) needs this raw
	// total directly, not the normalised value.
	RawDimensionScores map[string]float64 `json:"raw_dimension_scores,omitempty"`
}

package domain

// CustomerProfileInput represents profile details
type CustomerProfileInput struct {
	UserID       string          `json:"user_id,omitempty"`
	AgeGroup     string          `json:"age_group,omitempty"`
	SkinToneCode string          `json:"skin_tone_code,omitempty"`
	Conditions   map[string]bool `json:"conditions,omitempty"`
}

// EnvironmentalContext provides climate/UV data
type EnvironmentalContext struct {
	UVIndex     float64 `json:"uv_index,omitempty"`
	HumidityPct float64 `json:"humidity_pct,omitempty"`
	Climate     string  `json:"climate,omitempty"`
}

// MatchingPreferences specifies texture and budget constraints
type MatchingPreferences struct {
	RoutineDepth      string   `json:"routine_depth,omitempty"`
	SplitAMPM         bool     `json:"split_am_pm,omitempty"`
	PreferredTextures []string `json:"preferred_textures,omitempty"`
	AvoidIngredients  []string `json:"avoid_ingredients,omitempty"`
	BudgetTier        string   `json:"budget_tier,omitempty"`
	MaxSteps          int      `json:"max_steps,omitempty"`
}

// MatchRequest is the canonical evaluation request payload directly ingesting Score Engine results
type MatchRequest struct {
	BrandID              string                 `json:"brand_id"`
	ApplicationID        string                 `json:"application_id"`
	StrategyID           string                 `json:"strategy_id,omitempty"`
	CustomerProfile      *CustomerProfileInput  `json:"customer_profile,omitempty"`
	CustomerConditions   map[string]bool        `json:"customer_conditions,omitempty"`
	DimensionScores      map[string]float64     `json:"dimension_scores"`
	EnvironmentalContext *EnvironmentalContext  `json:"environmental_context,omitempty"`
	Preferences          *MatchingPreferences   `json:"preferences,omitempty"`
	RawPayload           map[string]interface{} `json:"raw_payload,omitempty"`
}

// ProductMatchItem represents a recommended product
type ProductMatchItem struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Brand            string                 `json:"brand"`
	Category         string                 `json:"category"`
	Texture          string                 `json:"texture"`
	MatchScore       float64                `json:"match_score"`
	WhySelected      []string               `json:"why_selected"`
	KeyActives       []string               `json:"key_actives,omitempty"`
	EfficacyDetails  map[string]interface{} `json:"efficacy_details,omitempty"`
	ImageURL         string                 `json:"image_url,omitempty"`
}

// RegimenStep represents one step in a skincare routine
type RegimenStep struct {
	StepNumber         int                `json:"step_number"`
	StepName           string             `json:"step_name"`
	Category           string             `json:"category"`
	RecommendedTexture string             `json:"recommended_texture,omitempty"`
	PrimaryProduct     *ProductMatchItem  `json:"primary_product"`
	Alternatives       []ProductMatchItem `json:"alternatives,omitempty"`
}

// RegimenCollection groups AM and PM routines or dynamic brand-defined phases
type RegimenCollection struct {
	AMRoutine []RegimenStep            `json:"am_routine,omitempty"`
	PMRoutine []RegimenStep            `json:"pm_routine,omitempty"`
	Phases    map[string][]RegimenStep `json:"phases,omitempty"`
}

// UserPreferences is an alias for MatchingPreferences for cleaner engine interfaces
type UserPreferences = MatchingPreferences

// ClinicalConflictReport reports active ingredient interactions
type ClinicalConflictReport struct {
	ConflictsDetected    int      `json:"conflicts_detected"`
	LayeringRulesApplied []string `json:"layering_rules_applied"`
	Warnings             []string `json:"warnings,omitempty"`
}

// ProfileSummary diagnoses skin condition
type ProfileSummary struct {
	SkinType                string   `json:"skin_type"`
	PrimaryConcerns         []string `json:"primary_concerns"`
	OverallSuitabilityScore float64  `json:"overall_suitability_score"`
}

// MatchResponse is the output recommendation result
type MatchResponse struct {
	Success                bool                    `json:"success"`
	MatchID                string                  `json:"match_id"`
	BrandID                string                  `json:"brand_id"`
	ApplicationID          string                  `json:"application_id"`
	ProfileSummary         ProfileSummary          `json:"profile_summary"`
	Regimens               RegimenCollection       `json:"regimens"`
	ClinicalConflictMatrix ClinicalConflictReport  `json:"clinical_conflict_matrix"`
	EvaluatedAt            string                  `json:"evaluated_at"`
}

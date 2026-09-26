package domain

// Relationship constants
const (
	RelationshipContraindicated = "CONTRAINDICATED"
	RelationshipRecommended     = "RECOMMENDED"
	RelationshipCaution         = "CAUTION"
)

// IngredientConditionRelation links an ingredient to a customer condition
type IngredientConditionRelation struct {
	IngredientCode   string `json:"ingredient_code"`
	ConditionCode    string `json:"condition_code"`
	RelationshipType string `json:"relationship_type"` // CONTRAINDICATED, RECOMMENDED, CAUTION
	Rationale        string `json:"rationale"`
}

// IngredientDimensionEfficacy maps ingredient impact to a diagnostic dimension and severity tier
type IngredientDimensionEfficacy struct {
	IngredientCode  string  `json:"ingredient_code"`
	DimensionID     string  `json:"dimension_id"`
	SeverityTierID  string  `json:"severity_tier_id"` // e.g., SEVERITY_MILD, SEVERITY_MODERATE, SEVERITY_CRITICAL, *
	EfficacyWeight  float64 `json:"efficacy_weight"`
	ActionDirection string  `json:"action_direction"` // REDUCE, INCREASE
}

// RoutineSlot represents an individual product slot in a routine phase
type RoutineSlot struct {
	SlotID   string `json:"slot_id"`
	Category string `json:"category"`
	Required bool   `json:"required"`
	MaxItems int    `json:"max_items"`
}

// BrandRoutineBlueprint defines brand-specific routine structure and named phases
type BrandRoutineBlueprint struct {
	BrandID  string                   `json:"brand_id"`
	Paradigm string                   `json:"paradigm"`
	Phases   map[string][]RoutineSlot `json:"phases"`
}

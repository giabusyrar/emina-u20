package decision

import "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/domain"

// DefaultIngredientConditions contains clinical contraindications and recommendations
var DefaultIngredientConditions = []domain.IngredientConditionRelation{
	// Pregnancy Contraindications
	{IngredientCode: "RETINOL", ConditionCode: "is_pregnant", RelationshipType: domain.RelationshipContraindicated, Rationale: "Teratogenic risk during pregnancy"},
	{IngredientCode: "RETINYL_PALMITATE", ConditionCode: "is_pregnant", RelationshipType: domain.RelationshipContraindicated, Rationale: "Vitamin A derivative unsafe in pregnancy"},
	{IngredientCode: "TRETINOIN", ConditionCode: "is_pregnant", RelationshipType: domain.RelationshipContraindicated, Rationale: "Prescription retinoid strictly prohibited in pregnancy"},
	{IngredientCode: "HIGH_DOSE_SALICYLIC_ACID", ConditionCode: "is_pregnant", RelationshipType: domain.RelationshipContraindicated, Rationale: "High concentration BHA contraindicated in pregnancy"},
	{IngredientCode: "HYDROQUINONE", ConditionCode: "is_pregnant", RelationshipType: domain.RelationshipContraindicated, Rationale: "High systemic absorption risk in pregnancy"},

	// Sensitive Skin Contraindications
	{IngredientCode: "FRAGRANCE", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipContraindicated, Rationale: "High potential allergen and barrier irritant"},
	{IngredientCode: "ALCOHOL_DENAT", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipContraindicated, Rationale: "Dries and strips delicate skin barrier"},
	{IngredientCode: "ESSENTIAL_OILS", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipContraindicated, Rationale: "Sensitizing botanical volatile compounds"},

	// Recommendations
	{IngredientCode: "CENTELLA_ASIATICA", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipRecommended, Rationale: "Clinically proven soothing and calming active"},
	{IngredientCode: "CERAMIDE_NP", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipRecommended, Rationale: "Bio-identical lipid to restore compromised skin barrier"},
	{IngredientCode: "PANTHENOL", ConditionCode: "is_sensitive", RelationshipType: domain.RelationshipRecommended, Rationale: "Pro-vitamin B5 accelerates barrier healing"},
	{IngredientCode: "SALICYLIC_ACID", ConditionCode: "has_acne", RelationshipType: domain.RelationshipRecommended, Rationale: "Lipophilic BHA penetrates pores to clear sebum and micro-comedones"},
	{IngredientCode: "ZINC_PCA", ConditionCode: "has_acne", RelationshipType: domain.RelationshipRecommended, Rationale: "Regulates sebum production and suppresses microbial proliferation"},
}

// DefaultDimensionEfficacies maps target active ingredients to dimension needs and severity tiers
var DefaultDimensionEfficacies = []domain.IngredientDimensionEfficacy{
	// Sebum (High / Oily)
	{IngredientCode: "SALICYLIC_ACID", DimensionID: "sebum", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.95, ActionDirection: "REDUCE"},
	{IngredientCode: "ZINC_PCA", DimensionID: "sebum", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.90, ActionDirection: "REDUCE"},
	{IngredientCode: "NIACINAMIDE", DimensionID: "sebum", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.85, ActionDirection: "REDUCE"},
	{IngredientCode: "GREEN_TEA", DimensionID: "sebum", SeverityTierID: "SEVERITY_MILD", EfficacyWeight: 0.70, ActionDirection: "REDUCE"},

	// Sebum (Low / Dry)
	{IngredientCode: "SQUALANE", DimensionID: "sebum_dry", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.90, ActionDirection: "INCREASE"},
	{IngredientCode: "CERAMIDE_NP", DimensionID: "sebum_dry", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.85, ActionDirection: "INCREASE"},

	// Hydration (Low / Dehydrated)
	{IngredientCode: "HYALURONIC_ACID", DimensionID: "hydration", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.95, ActionDirection: "INCREASE"},
	{IngredientCode: "POLYGLUTAMIC_ACID", DimensionID: "hydration", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.90, ActionDirection: "INCREASE"},
	{IngredientCode: "GLYCERIN", DimensionID: "hydration", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.80, ActionDirection: "INCREASE"},
	{IngredientCode: "PANTHENOL", DimensionID: "hydration", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.85, ActionDirection: "INCREASE"},

	// Sensitivity (High / Reactive)
	{IngredientCode: "CENTELLA_ASIATICA", DimensionID: "sensitivity", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.95, ActionDirection: "REDUCE"},
	{IngredientCode: "MADECASSOSIDE", DimensionID: "sensitivity", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.90, ActionDirection: "REDUCE"},
	{IngredientCode: "ALLANTOIN", DimensionID: "sensitivity", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.80, ActionDirection: "REDUCE"},

	// Pigmentation (High / Discoloration)
	{IngredientCode: "TRANEXAMIC_ACID", DimensionID: "pigmentation", SeverityTierID: "SEVERITY_SEVERE", EfficacyWeight: 0.90, ActionDirection: "REDUCE"},
	{IngredientCode: "ALPHA_ARBUTIN", DimensionID: "pigmentation", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.85, ActionDirection: "REDUCE"},
	{IngredientCode: "VITAMIN_C", DimensionID: "pigmentation", SeverityTierID: "SEVERITY_MODERATE", EfficacyWeight: 0.85, ActionDirection: "REDUCE"},
}

// DefaultBrandBlueprints stores brand-defined routine blueprints
var DefaultBrandBlueprints = map[string]domain.BrandRoutineBlueprint{
	"wardah": {
		BrandID:  "wardah",
		Paradigm: "clinical_skincare",
		Phases: map[string][]domain.RoutineSlot{
			"morning_protection": {
				{SlotID: "cleanser", Category: "Cleanser", Required: true, MaxItems: 1},
				{SlotID: "treatment_serum", Category: "Serum", Required: false, MaxItems: 1},
				{SlotID: "moisturizer", Category: "Moisturizer", Required: true, MaxItems: 1},
				{SlotID: "sunscreen", Category: "Sunscreen", Required: true, MaxItems: 1},
			},
			"night_restoration": {
				{SlotID: "cleanser", Category: "Cleanser", Required: true, MaxItems: 1},
				{SlotID: "active_treatment", Category: "Serum", Required: true, MaxItems: 2},
				{SlotID: "night_barrier_cream", Category: "Moisturizer", Required: true, MaxItems: 1},
			},
		},
	},
	"makeover": {
		BrandID:  "makeover",
		Paradigm: "cosmetics_glam",
		Phases: map[string][]domain.RoutineSlot{
			"skin_prep": {
				{SlotID: "hydrating_toner", Category: "Toner", Required: true, MaxItems: 1},
				{SlotID: "primer_serum", Category: "Serum", Required: true, MaxItems: 1},
			},
			"base_complexion": {
				{SlotID: "cushion_foundation", Category: "Foundation", Required: true, MaxItems: 1},
				{SlotID: "setting_powder", Category: "Powder", Required: false, MaxItems: 1},
			},
			"makeup_removal": {
				{SlotID: "cleansing_oil", Category: "Cleanser", Required: true, MaxItems: 1},
			},
		},
	},
	"kahf": {
		BrandID:  "kahf",
		Paradigm: "mens_grooming",
		Phases: map[string][]domain.RoutineSlot{
			"daily_morning_defense": {
				{SlotID: "face_wash", Category: "Cleanser", Required: true, MaxItems: 1},
				{SlotID: "sunscreen_moisturizer", Category: "Sunscreen", Required: true, MaxItems: 1},
			},
			"post_shave_soothing": {
				{SlotID: "soothing_balm", Category: "Moisturizer", Required: false, MaxItems: 1},
			},
			"night_hydration": {
				{SlotID: "face_wash", Category: "Cleanser", Required: true, MaxItems: 1},
				{SlotID: "night_moisturizer", Category: "Moisturizer", Required: true, MaxItems: 1},
			},
		},
	},
	"biodef": {
		BrandID:  "biodef",
		Paradigm: "body_hygiene",
		Phases: map[string][]domain.RoutineSlot{
			"shower_cleanse": {
				{SlotID: "body_wash", Category: "Body Wash", Required: true, MaxItems: 1},
			},
			"hand_sanitizing": {
				{SlotID: "hand_sanitizer", Category: "Hand Sanitizer", Required: true, MaxItems: 1},
			},
			"body_barrier_moisture": {
				{SlotID: "body_lotion", Category: "Body Lotion", Required: true, MaxItems: 1},
			},
		},
	},
}

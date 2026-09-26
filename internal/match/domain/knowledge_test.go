package domain_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
)

func TestRelationalKnowledgeTypes(t *testing.T) {
	contra := domain.IngredientConditionRelation{
		IngredientCode:   "RETINOL",
		ConditionCode:    "IS_PREGNANT",
		RelationshipType: "CONTRAINDICATED",
		Rationale:        "Teratogenic risk during pregnancy",
	}
	if contra.RelationshipType != "CONTRAINDICATED" {
		t.Fatalf("expected CONTRAINDICATED, got %s", contra.RelationshipType)
	}

	eff := domain.IngredientDimensionEfficacy{
		IngredientCode:  "NIACINAMIDE",
		DimensionID:     "SEBUM",
		SeverityTierID:  "SEVERITY_MODERATE",
		EfficacyWeight:  0.75,
		ActionDirection: "REDUCE",
	}
	if eff.EfficacyWeight != 0.75 {
		t.Fatalf("expected 0.75, got %f", eff.EfficacyWeight)
	}

	blueprint := domain.BrandRoutineBlueprint{
		BrandID:  "wardah",
		Paradigm: "clinical_skincare",
		Phases: map[string][]domain.RoutineSlot{
			"morning_protection": {
				{SlotID: "cleanser", Category: "Cleanser", Required: true, MaxItems: 1},
				{SlotID: "sunscreen", Category: "Sunscreen", Required: true, MaxItems: 1},
			},
		},
	}
	if len(blueprint.Phases["morning_protection"]) != 2 {
		t.Fatalf("expected 2 slots in morning_protection phase, got %d", len(blueprint.Phases["morning_protection"]))
	}
}

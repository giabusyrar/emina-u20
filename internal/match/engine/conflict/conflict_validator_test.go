package conflict_test

import (
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/conflict"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/retrieval"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
)

func TestConflictValidator_DetectsActiveCollisions(t *testing.T) {
	validator := conflict.NewConflictValidator()

	phaseProducts := map[string][]retrieval.ScoredProduct{
		"night_restoration": {
			{Item: entity.ProductCatalogItem{ID: "AHA-SERUM", Ingredients: "GLYCOLIC_ACID, WATER"}},
			{Item: entity.ProductCatalogItem{ID: "RETINOL-CREAM", Ingredients: "RETINOL, CERAMIDE"}},
		},
	}

	report := validator.ValidateRoutine(phaseProducts)
	if report.ConflictsDetected == 0 {
		t.Fatalf("expected conflict detected for AHA + Retinol in same night phase")
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected warning message for AHA + Retinol collision")
	}
}

func TestConflictValidator_NoCollisionsWhenSeparated(t *testing.T) {
	validator := conflict.NewConflictValidator()

	phaseProducts := map[string][]retrieval.ScoredProduct{
		"morning_protection": {
			{Item: entity.ProductCatalogItem{ID: "VIT-C-SERUM", Ingredients: "ASCORBIC_ACID, WATER"}},
		},
		"night_restoration": {
			{Item: entity.ProductCatalogItem{ID: "RETINOL-CREAM", Ingredients: "RETINOL, CERAMIDE"}},
		},
	}

	report := validator.ValidateRoutine(phaseProducts)
	if report.ConflictsDetected != 0 {
		t.Fatalf("expected 0 conflicts when Vit C is in AM and Retinol in PM, got %d", report.ConflictsDetected)
	}
}

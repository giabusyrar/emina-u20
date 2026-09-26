package retrieval_test

import (
	"context"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/decision"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/retrieval"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
)

func TestHybridMatcher_FilterContraindicationsAndRank(t *testing.T) {
	matcher := retrieval.NewHybridMatcher()

	candidates := []entity.ProductCatalogItem{
		{
			ID:          "PROD-RETINOL-NIGHT",
			BrandID:     "wardah",
			Name:        "Night Retinol Serum",
			Category:    "Serum",
			Ingredients: "RETINOL, HYALURONIC_ACID",
			Texture:     "Fluid",
		},
		{
			ID:          "PROD-NIACINAMIDE-SERUM",
			BrandID:     "wardah",
			Name:        "Niacinamide 10% + Zinc Serum",
			Category:    "Serum",
			Ingredients: "NIACINAMIDE, ZINC_PCA, HYALURONIC_ACID",
			Texture:     "Water Gel",
		},
	}

	dec := &decision.DecisionResult{
		ProhibitedIngredients: []string{"RETINOL"},
		TargetActives:         []string{"NIACINAMIDE", "ZINC_PCA"},
	}

	slot := domain.RoutineSlot{SlotID: "serum", Category: "Serum", Required: true, MaxItems: 1}
	results := matcher.MatchSlot(context.Background(), slot, candidates, dec, domain.UserPreferences{
		PreferredTextures: []string{"water gel"},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 candidate after retinol exclusion, got %d", len(results))
	}
	if results[0].Item.ID != "PROD-NIACINAMIDE-SERUM" {
		t.Fatalf("expected PROD-NIACINAMIDE-SERUM to be selected, got %s", results[0].Item.ID)
	}
	if results[0].MatchScore <= 0 {
		t.Fatalf("expected positive match score, got %f", results[0].MatchScore)
	}
}

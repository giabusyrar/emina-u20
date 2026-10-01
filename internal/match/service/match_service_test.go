package service_test

import (
	"context"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
)

func setupTestService(t *testing.T) (*service.MatchService, *repository.MatchRepo) {
	repo := repository.NewMatchRepo(nil)
	svc := service.NewMatchService(repo, "")

	// Seed sample products
	ctx := context.Background()
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "W-GEL-CLEANSER",
		BrandID:     "wardah",
		Name:        "Wardah Hydra Rose Gel Cleanser",
		Category:    "Cleanser",
		Texture:     "Water Gel",
		Ingredients: "CENTELLA_ASIATICA, GLYCERIN",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "W-RETINOL-SERUM",
		BrandID:     "wardah",
		Name:        "Wardah Renew You Retinol Serum",
		Category:    "Serum",
		Texture:     "Fluid",
		Ingredients: "RETINOL, PEPTIDES",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "W-NIACIN-SERUM",
		BrandID:     "wardah",
		Name:        "Wardah Lightening Serum Ampoule",
		Category:    "Serum",
		Texture:     "Water Gel",
		Ingredients: "NIACINAMIDE, ZINC_PCA",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "W-MOIST-GEL",
		BrandID:     "wardah",
		Name:        "Wardah Hydra Rose Moisture Gel",
		Category:    "Moisturizer",
		Texture:     "Water Gel",
		Ingredients: "CERAMIDE_NP, HYALURONIC_ACID",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "W-UV-SHIELD",
		BrandID:     "wardah",
		Name:        "Wardah UV Shield SPF 50",
		Category:    "Sunscreen",
		Texture:     "Fluid",
		Ingredients: "ZINC_OXIDE, HYALURONIC_ACID",
	})

	// Make Over products
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "MO-PRIMER-SERUM",
		BrandID:     "makeover",
		Name:        "Make Over Powerstay Primer Serum",
		Category:    "Serum",
		Texture:     "Fluid",
		Ingredients: "HYALURONIC_ACID, NIACINAMIDE",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "MO-CUSHION",
		BrandID:     "makeover",
		Name:        "Make Over Hydrastay Cushion",
		Category:    "Foundation",
		Texture:     "Cushion",
		Ingredients: "GLYCERIN, TITANIUM_DIOXIDE",
	})
	repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
		ID:          "MO-CLEANSE-OIL",
		BrandID:     "makeover",
		Name:        "Make Over Deep Cleansing Oil",
		Category:    "Cleanser",
		Texture:     "Oil",
		Ingredients: "SQUALANE",
	})

	return svc, repo
}

func TestMatchService_Evaluate_WardahPregnancySafety(t *testing.T) {
	svc, _ := setupTestService(t)

	req := domain.MatchRequest{
		BrandID: "wardah",
		DimensionScores: map[string]float64{
			"sebum":       85.0,
			"hydration":   35.0,
			"sensitivity": 70.0,
		},
		CustomerConditions: map[string]bool{
			"is_pregnant": true,
		},
		Preferences: &domain.MatchingPreferences{
			PreferredTextures: []string{"water gel"},
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected evaluate success")
	}

	// Verify Retinol serum was excluded due to pregnancy
	for _, step := range resp.Regimens.AMRoutine {
		if step.PrimaryProduct != nil && step.PrimaryProduct.ID == "W-RETINOL-SERUM" {
			t.Fatalf("critical safety violation: W-RETINOL-SERUM recommended to pregnant user in AM routine")
		}
	}
	for _, step := range resp.Regimens.PMRoutine {
		if step.PrimaryProduct != nil && step.PrimaryProduct.ID == "W-RETINOL-SERUM" {
			t.Fatalf("critical safety violation: W-RETINOL-SERUM recommended to pregnant user in PM routine")
		}
	}

	// Verify dynamic phases are populated
	if resp.Regimens.Phases == nil || len(resp.Regimens.Phases["morning_protection"]) == 0 {
		t.Fatalf("expected morning_protection phase in Regimens.Phases")
	}
}

func TestMatchService_Evaluate_MakeOverCustomParadigm(t *testing.T) {
	svc, _ := setupTestService(t)

	req := domain.MatchRequest{
		BrandID: "makeover",
		DimensionScores: map[string]float64{
			"sebum":     50.0,
			"hydration": 40.0,
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected evaluate success")
	}

	if resp.Regimens.Phases == nil {
		t.Fatalf("expected non-nil Regimens.Phases for Make Over")
	}

	if _, ok := resp.Regimens.Phases["skin_prep"]; !ok {
		t.Fatalf("expected skin_prep phase for Make Over")
	}
	if _, ok := resp.Regimens.Phases["base_complexion"]; !ok {
		t.Fatalf("expected base_complexion phase for Make Over")
	}
}

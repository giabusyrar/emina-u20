package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
)

func BenchmarkMatchService_Evaluate(b *testing.B) {
	repo := repository.NewMatchRepo(nil)
	svc := service.NewMatchService(repo, "")

	// Seed 500 products across various categories
	ctx := context.Background()
	categories := []string{"Cleanser", "Serum", "Moisturizer", "Sunscreen", "Toner", "Foundation"}
	textures := []string{"Water Gel", "Cream", "Fluid", "Emulsion"}
	ingredientsPool := []string{
		"NIACINAMIDE, ZINC_PCA",
		"RETINOL, PEPTIDES",
		"CENTELLA_ASIATICA, GLYCERIN",
		"CERAMIDE_NP, HYALURONIC_ACID",
		"SALICYLIC_ACID, TEA_TREE_OIL",
		"TRANEXAMIC_ACID, ALPHA_ARBUTIN",
	}

	for i := 0; i < 500; i++ {
		repo.AddInMemoryProduct(ctx, entity.ProductCatalogItem{
			ID:          fmt.Sprintf("BENCH-PROD-%d", i),
			BrandID:     "wardah",
			Name:        fmt.Sprintf("Benchmark Product %d", i),
			Category:    categories[i%len(categories)],
			Texture:     textures[i%len(textures)],
			Ingredients: ingredientsPool[i%len(ingredientsPool)],
		})
	}

	req := domain.MatchRequest{
		BrandID: "wardah",
		DimensionScores: map[string]float64{
			"sebum":        82.0,
			"hydration":    35.0,
			"sensitivity":  75.0,
			"pigmentation": 50.0,
		},
		CustomerConditions: map[string]bool{
			"is_pregnant":  true,
			"is_sensitive": true,
		},
		Preferences: &domain.MatchingPreferences{
			PreferredTextures: []string{"Water Gel", "Fluid"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Evaluate(ctx, req)
		if err != nil {
			b.Fatalf("benchmark evaluate failed: %v", err)
		}
	}
}

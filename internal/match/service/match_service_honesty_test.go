package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/tryonclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/decision"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/repository"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/service"
)

// A request missing what match diagnoses from is refused, never filled in.
// Before, an empty request came back as "Seborrheic Oily & Dehydrated &
// Reactive Barrier", suitability 94.5, for brand "wardah".
func TestEvaluateRefusesIncompleteRequest(t *testing.T) {
	svc, _ := setupTestService(t)
	_, err := svc.Evaluate(context.Background(), domain.MatchRequest{})
	var verrs *domain.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("got %v, want validation errors", err)
	}
	for _, want := range []string{"brand_id", "application_id", "dimension_scores"} {
		if !strings.Contains(verrs.Error(), want) {
			t.Errorf("validation errors %q do not name %s", verrs.Error(), want)
		}
	}
}

func TestEvaluateUnknownBrand(t *testing.T) {
	svc, _ := setupTestService(t)
	_, err := svc.Evaluate(context.Background(), domain.MatchRequest{
		BrandID: "nope", ApplicationID: "app", DimensionScores: map[string]float64{"sebum": 50},
	})
	if !errors.Is(err, brand.ErrUnknown) {
		t.Fatalf("got %v, want brand.ErrUnknown", err)
	}
}

// OMG has products but no routine blueprint: refused, not given Wardah's.
func TestEvaluateBrandWithoutBlueprint(t *testing.T) {
	svc, repo := setupTestService(t)
	repo.AddInMemoryProduct(context.Background(), entity.ProductCatalogItem{ID: "O-1", BrandID: "omg", Name: "x", Category: "Moisturizer"})
	_, err := svc.Evaluate(context.Background(), domain.MatchRequest{
		BrandID: "omg", ApplicationID: "app", DimensionScores: map[string]float64{"sebum": 50},
	})
	if !errors.Is(err, decision.ErrNoBlueprint) {
		t.Fatalf("got %v, want decision.ErrNoBlueprint", err)
	}
}

// A brand with no products is refused, not served another brand's products.
func TestEvaluateBrandWithoutProducts(t *testing.T) {
	svc := service.NewMatchService(setupEmptyRepo(), tryonclient.New("")).WithBrands(testBrands)
	_, err := svc.Evaluate(context.Background(), domain.MatchRequest{
		BrandID: "wardah", ApplicationID: "app", DimensionScores: map[string]float64{"sebum": 50},
	})
	if !errors.Is(err, service.ErrNoProducts) {
		t.Fatalf("got %v, want ErrNoProducts", err)
	}
}

// The summary describes only the dimensions sent: a missing hydration no
// longer reads as 0 ("Dehydrated"), and there is no suitability number.
func TestEvaluateSummaryFromSentDimensionsOnly(t *testing.T) {
	svc, _ := setupTestService(t)
	resp, err := svc.Evaluate(context.Background(), domain.MatchRequest{
		BrandID: "wardah", ApplicationID: "app", DimensionScores: map[string]float64{"sebum": 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.ProfileSummary.SkinType; got != "Seborrheic Oily" {
		t.Errorf("skin type = %q, want only what sebum 80 says", got)
	}
	if resp.BrandID != "wardah" || resp.ApplicationID != "app" {
		t.Errorf("echoed %q/%q, want the request's brand and app", resp.BrandID, resp.ApplicationID)
	}
}

func setupEmptyRepo() *repository.MatchRepo { return repository.NewMatchRepo(nil) }

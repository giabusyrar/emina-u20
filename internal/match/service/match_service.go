package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/conflict"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/decision"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/retrieval"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
)

type MatchService struct {
	repo              *repository.MatchRepo
	decisionEngine    *decision.ClinicalDecisionEngine
	hybridMatcher     *retrieval.HybridMatcher
	conflictValidator *conflict.ConflictValidator
	tryonEngineURL    string
}

func NewMatchService(repo *repository.MatchRepo, tryonEngineURL string) *MatchService {
	return &MatchService{
		repo:              repo,
		decisionEngine:    decision.NewClinicalDecisionEngine(),
		hybridMatcher:     retrieval.NewHybridMatcher(),
		conflictValidator: conflict.NewConflictValidator(),
		tryonEngineURL:    tryonEngineURL,
	}
}

// Evaluate runs the 2-Tier Hybrid Clinical Recommendation Pipeline
func (s *MatchService) Evaluate(ctx context.Context, req domain.MatchRequest) (*domain.MatchResponse, error) {
	brandID := strings.ToLower(strings.TrimSpace(req.BrandID))
	if brandID == "" {
		brandID = "wardah"
	}
	appID := strings.TrimSpace(req.ApplicationID)
	if appID == "" {
		appID = "skin-analyzer-pro"
	}

	// 1. Ingest Customer Conditions & Dimension Scores
	conditions := make(map[string]bool)
	if req.CustomerProfile != nil && req.CustomerProfile.Conditions != nil {
		for k, v := range req.CustomerProfile.Conditions {
			conditions[k] = v
		}
	}
	if req.CustomerConditions != nil {
		for k, v := range req.CustomerConditions {
			conditions[k] = v
		}
	}

	dimScores := req.DimensionScores
	if dimScores == nil || len(dimScores) == 0 {
		dimScores = map[string]float64{
			"sebum":        75.0,
			"hydration":    40.0,
			"sensitivity":  65.0,
			"pigmentation": 30.0,
		}
	}

	// 2. Fetch Reference Product Catalog
	products, err := s.repo.ListProducts(ctx, brandID)
	if err != nil || len(products) == 0 {
		// Fallback: fetch all brand products
		products, _ = s.repo.ListProducts(ctx, "*")
	}

	// 2b. Campaign Product Group Restriction: if the brand has one or more
	// active product groups, narrow the candidate pool to only the
	// products/categories curated for their campaign(s). Brands with no
	// active groups keep the full catalog (backward compatible).
	if restricted := s.applyProductGroupRestriction(ctx, brandID, appID, products); restricted != nil {
		products = restricted
	}

	// 3. Stage 1: Tier 1 Clinical Decision & Safety Engine Evaluation
	decInput := decision.DecisionInput{
		BrandID:         brandID,
		DimensionScores: dimScores,
		Conditions:      conditions,
	}
	decResult, err := s.decisionEngine.Evaluate(ctx, decInput)
	if err != nil {
		return nil, fmt.Errorf("decision engine error: %w", err)
	}

	// 4. Stage 2: Tier 2 Hybrid Matcher - Evaluate dynamic routine blueprint phases
	blueprint := decResult.ResolvedBlueprint
	phaseSteps := make(map[string][]domain.RegimenStep)
	phaseScoredProducts := make(map[string][]retrieval.ScoredProduct)

	var prefs domain.UserPreferences
	if req.Preferences != nil {
		prefs = *req.Preferences
	}

	for phaseName, slots := range blueprint.Phases {
		var steps []domain.RegimenStep
		var phaseProds []retrieval.ScoredProduct

		for i, slot := range slots {
			scored := s.hybridMatcher.MatchSlot(ctx, slot, products, decResult, prefs)
			if len(scored) == 0 {
				continue
			}

			top := scored[0]
			phaseProds = append(phaseProds, top)

			primaryMatch := domain.ProductMatchItem{
				ID:          top.Item.ID,
				Name:        top.Item.Name,
				Brand:       top.Item.BrandID,
				Category:    top.Item.Category,
				Texture:     top.Item.Texture,
				MatchScore:  top.MatchScore,
				WhySelected: top.WhySelected,
				KeyActives:  top.KeyActives,
				ImageURL:    top.Item.ImageURL,
			}

			var alternatives []domain.ProductMatchItem
			for j := 1; j < len(scored) && j < 3; j++ {
				alt := scored[j]
				alternatives = append(alternatives, domain.ProductMatchItem{
					ID:          alt.Item.ID,
					Name:        alt.Item.Name,
					Brand:       alt.Item.BrandID,
					Category:    alt.Item.Category,
					Texture:     alt.Item.Texture,
					MatchScore:  alt.MatchScore,
					WhySelected: alt.WhySelected,
					KeyActives:  alt.KeyActives,
					ImageURL:    alt.Item.ImageURL,
				})
			}

			stepName := fmt.Sprintf("Step %d: %s", i+1, slot.Category)
			steps = append(steps, domain.RegimenStep{
				StepNumber:         i + 1,
				StepName:           stepName,
				Category:           slot.Category,
				RecommendedTexture: top.Item.Texture,
				PrimaryProduct:     &primaryMatch,
				Alternatives:       alternatives,
			})
		}

		phaseSteps[phaseName] = steps
		phaseScoredProducts[phaseName] = phaseProds
	}

	// 5. Stage 3: Clinical Conflict Validation
	conflictReport := s.conflictValidator.ValidateRoutine(phaseScoredProducts)

	// Backward-compatibility mapping for standard AM / PM regimens
	var amRoutine []domain.RegimenStep
	var pmRoutine []domain.RegimenStep

	if am, ok := phaseSteps["morning_protection"]; ok {
		amRoutine = am
	} else if am, ok := phaseSteps["daily_morning_defense"]; ok {
		amRoutine = am
	} else if am, ok := phaseSteps["am"]; ok {
		amRoutine = am
	}

	if pm, ok := phaseSteps["night_restoration"]; ok {
		pmRoutine = pm
	} else if pm, ok := phaseSteps["night_hydration"]; ok {
		pmRoutine = pm
	} else if pm, ok := phaseSteps["pm"]; ok {
		pmRoutine = pm
	}

	// Skin type summary description
	skinTypeDesc := "Normal Balanced Skin"
	if dimScores["sebum"] >= 66.0 {
		skinTypeDesc = "Seborrheic Oily"
	} else if dimScores["sebum"] <= 35.0 && dimScores["sebum"] > 0 {
		skinTypeDesc = "Alipic Dry"
	}
	if dimScores["hydration"] <= 40.0 {
		skinTypeDesc += " & Dehydrated"
	}
	if conditions["is_sensitive"] || dimScores["sensitivity"] >= 65.0 {
		skinTypeDesc += " & Reactive Barrier"
	}

	matchID := fmt.Sprintf("match-%d", time.Now().UnixNano())

	return &domain.MatchResponse{
		Success:       true,
		MatchID:       matchID,
		BrandID:       brandID,
		ApplicationID: appID,
		ProfileSummary: domain.ProfileSummary{
			SkinType:                skinTypeDesc,
			PrimaryConcerns:         decResult.DiagnosedConcerns,
			OverallSuitabilityScore: 94.5,
		},
		Regimens: domain.RegimenCollection{
			AMRoutine: amRoutine,
			PMRoutine: pmRoutine,
			Phases:    phaseSteps,
		},
		ClinicalConflictMatrix: conflictReport,
		EvaluatedAt:            time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// applyProductGroupRestriction narrows the candidate product pool to the
// union of members across the brand's active product groups (by explicit
// product ID or by category tag). Returns nil when the brand has no active
// groups, signaling the caller to keep the unrestricted catalog.
func (s *MatchService) applyProductGroupRestriction(ctx context.Context, brandID, appID string, products []entity.ProductCatalogItem) []entity.ProductCatalogItem {
	groups, err := s.repo.ListProductGroups(ctx, brandID, appID)
	if err != nil || len(groups) == 0 {
		return nil
	}

	productIDs := make(map[string]bool)
	categories := make(map[string]bool)
	hasActiveGroup := false
	for _, g := range groups {
		if !g.IsActive {
			continue
		}
		hasActiveGroup = true
		for _, id := range g.ProductIDs {
			productIDs[id] = true
		}
		for _, cat := range g.Categories {
			categories[strings.ToLower(strings.TrimSpace(cat))] = true
		}
	}
	if !hasActiveGroup {
		return nil
	}

	filtered := []entity.ProductCatalogItem{}
	for _, p := range products {
		if productIDs[p.ID] || categories[strings.ToLower(strings.TrimSpace(p.Category))] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// -------------------------------------------------------------
// Reference CRUD Delegations for Admin / API Controllers
// -------------------------------------------------------------

func (s *MatchService) ListProducts(ctx context.Context, brandID string) ([]entity.ProductCatalogItem, error) {
	return s.repo.ListProducts(ctx, brandID)
}

func (s *MatchService) CreateProduct(ctx context.Context, product entity.ProductCatalogItem) (*entity.ProductCatalogItem, error) {
	return s.repo.CreateProduct(ctx, product)
}

func (s *MatchService) UpdateProduct(ctx context.Context, product entity.ProductCatalogItem) (*entity.ProductCatalogItem, error) {
	return s.repo.UpdateProduct(ctx, product)
}

func (s *MatchService) DeleteProduct(ctx context.Context, id string) error {
	return s.repo.DeleteProduct(ctx, id)
}

func (s *MatchService) ListConflictRules(ctx context.Context, brandID, appID string) ([]entity.ConflictMatrixRule, error) {
	return s.repo.ListConflictRules(ctx, brandID, appID)
}

func (s *MatchService) CreateConflictRule(ctx context.Context, rule entity.ConflictMatrixRule) (*entity.ConflictMatrixRule, error) {
	return s.repo.CreateConflictRule(ctx, rule)
}

func (s *MatchService) UpdateConflictRule(ctx context.Context, rule entity.ConflictMatrixRule) (*entity.ConflictMatrixRule, error) {
	return s.repo.UpdateConflictRule(ctx, rule)
}

func (s *MatchService) DeleteConflictRule(ctx context.Context, id string) error {
	return s.repo.DeleteConflictRule(ctx, id)
}

func (s *MatchService) ListProductGroups(ctx context.Context, brandID, appID string) ([]entity.ProductGroup, error) {
	return s.repo.ListProductGroups(ctx, brandID, appID)
}

func (s *MatchService) CreateProductGroup(ctx context.Context, group entity.ProductGroup) (*entity.ProductGroup, error) {
	return s.repo.CreateProductGroup(ctx, group)
}

func (s *MatchService) UpdateProductGroup(ctx context.Context, group entity.ProductGroup) (*entity.ProductGroup, error) {
	return s.repo.UpdateProductGroup(ctx, group)
}

func (s *MatchService) DeleteProductGroup(ctx context.Context, id string) error {
	return s.repo.DeleteProductGroup(ctx, id)
}

func (s *MatchService) ListShades(ctx context.Context, productID string) ([]entity.Shade, error) {
	return s.repo.ListShades(ctx, productID)
}

func (s *MatchService) GetShadeByID(ctx context.Context, id string) (*entity.Shade, error) {
	return s.repo.GetShadeByID(ctx, id)
}

func (s *MatchService) CreateShade(ctx context.Context, shade entity.Shade) (*entity.Shade, error) {
	return s.repo.CreateShade(ctx, shade)
}

func (s *MatchService) UpdateShade(ctx context.Context, shade entity.Shade) (*entity.Shade, error) {
	return s.repo.UpdateShade(ctx, shade)
}

func (s *MatchService) DeleteShade(ctx context.Context, id string) error {
	return s.repo.DeleteShade(ctx, id)
}

func (s *MatchService) SaveShadeAsset(ctx context.Context, asset entity.ShadeAsset) (*entity.ShadeAsset, error) {
	return s.repo.SaveShadeAsset(ctx, asset)
}

func (s *MatchService) GetShadeAsset(ctx context.Context, shadeID string) (*entity.ShadeAsset, error) {
	return s.repo.GetShadeAsset(ctx, shadeID)
}

func (s *MatchService) GetShadeAssetByID(ctx context.Context, id string) (*entity.ShadeAsset, error) {
	return s.repo.GetShadeAssetByID(ctx, id)
}

// TriggerShadeExtraction asks tryon-engine to generate the photorealistic
// UV texture for a shade. tryon-engine reports back via the shade's
// extraction-callback route (see MatchHandler.ExtractionCallback).
func (s *MatchService) TriggerShadeExtraction(ctx context.Context, shade entity.Shade) error {
	baseURL := s.tryonEngineURL
	if baseURL == "" {
		return fmt.Errorf("TRYON_ENGINE_URL is not configured")
	}

	payload, err := json.Marshal(map[string]string{
		"shade_id":            shade.ID,
		"reference_photo_url": shade.ReferencePhotoURL,
		"hex_color":           shade.HexColor,
		"region":              shade.Region,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/extract", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("tryon-engine returned status %d", resp.StatusCode)
	}
	return nil
}

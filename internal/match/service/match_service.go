package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/conflict"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/decision"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/retrieval"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/repository"
)

type MatchService struct {
	repo              *repository.MatchRepo
	decisionEngine    *decision.ClinicalDecisionEngine
	hybridMatcher     *retrieval.HybridMatcher
	conflictValidator *conflict.ConflictValidator
	tryonEngineURL    string
	brands            BrandResolver
}

// BrandResolver turns the brand a request names (ref_brands id or code)
// into the brand row; internal/brand implements it.
type BrandResolver interface {
	Resolve(ctx context.Context, ref string) (brand.Brand, error)
}

// WithBrands sets the brand resolver Evaluate needs. Without one, Evaluate
// refuses to run rather than guess the brand.
func (s *MatchService) WithBrands(r BrandResolver) *MatchService {
	s.brands = r
	return s
}

// ErrNoProducts: the brand has no live products to recommend from. Match
// does not borrow another brand's products.
var ErrNoProducts = errors.New("brand has no products")

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
//
// Every input it diagnoses from must be in the request: brand, application
// and dimension scores are required, and a missing one is a validation
// error (domain.ValidationErrors), never a default. The brand may be its
// ref_brands id or its code.
func (s *MatchService) Evaluate(ctx context.Context, req domain.MatchRequest) (*domain.MatchResponse, error) {
	appID := strings.TrimSpace(req.ApplicationID)
	var verrs domain.ValidationErrors
	if strings.TrimSpace(req.BrandID) == "" {
		verrs.Add("brand_id is required")
	}
	if appID == "" {
		verrs.Add("application_id is required")
	}
	if len(req.DimensionScores) == 0 {
		verrs.Add("dimension_scores is required: match diagnoses from them and has no defaults")
	}
	if verrs.HasErrors() {
		return nil, &verrs
	}
	if s.brands == nil {
		return nil, fmt.Errorf("match service: no brand resolver configured")
	}
	b, err := s.brands.Resolve(ctx, req.BrandID)
	if err != nil {
		return nil, err
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

	// 2. Fetch the brand's products (ref_products is keyed by brand id).
	products, err := s.repo.ListProducts(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("listing products of brand %s: %w", b.ID, err)
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoProducts, b.ID)
	}

	// 2b. Campaign Product Group Restriction: if the brand has one or more
	// active product groups, narrow the candidate pool to only the
	// products/categories curated for their campaign(s). Brands with no
	// active groups keep the full catalog (backward compatible).
	if restricted := s.applyProductGroupRestriction(ctx, b.ID, appID, products); restricted != nil {
		products = restricted
	}

	// 3. Stage 1: Tier 1 Clinical Decision & Safety Engine Evaluation.
	// Routine blueprints are keyed by brand code.
	decInput := decision.DecisionInput{
		BrandID:         b.Code,
		DimensionScores: dimScores,
		Conditions:      conditions,
	}
	decResult, err := s.decisionEngine.Evaluate(ctx, decInput)
	if err != nil {
		return nil, fmt.Errorf("decision engine: %w", err)
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

	matchID := fmt.Sprintf("match-%d", time.Now().UnixNano())

	return &domain.MatchResponse{
		Success:       true,
		MatchID:       matchID,
		BrandID:       b.ID,
		ApplicationID: appID,
		ProfileSummary: domain.ProfileSummary{
			SkinType:        skinTypeSummary(dimScores, conditions),
			PrimaryConcerns: decResult.DiagnosedConcerns,
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
// skinTypeSummary describes the skin from the dimensions the request scored.
// A dimension the request did not send says nothing: before, a missing
// hydration read as 0 and produced "Dehydrated". With no describing
// dimension at all the summary is empty rather than "Normal Balanced Skin".
func skinTypeSummary(dims map[string]float64, conditions map[string]bool) string {
	var parts []string
	if v, ok := dims["sebum"]; ok {
		switch {
		case v >= 66.0:
			parts = append(parts, "Seborrheic Oily")
		case v <= 35.0:
			parts = append(parts, "Alipic Dry")
		default:
			parts = append(parts, "Balanced Sebum")
		}
	}
	if v, ok := dims["hydration"]; ok && v <= 40.0 {
		parts = append(parts, "Dehydrated")
	}
	if v, ok := dims["sensitivity"]; (ok && v >= 65.0) || conditions["is_sensitive"] {
		parts = append(parts, "Reactive Barrier")
	}
	return strings.Join(parts, " & ")
}

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

// ListProducts lists products, of one brand when brandID names one (its
// ref_brands id or code); "" or "*" lists every brand's.
func (s *MatchService) ListProducts(ctx context.Context, brandID string) ([]entity.ProductCatalogItem, error) {
	if brandID != "" && brandID != "*" && s.brands != nil {
		b, err := s.brands.Resolve(ctx, brandID)
		if err != nil {
			return nil, err
		}
		brandID = b.ID
	}
	return s.repo.ListProducts(ctx, brandID)
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

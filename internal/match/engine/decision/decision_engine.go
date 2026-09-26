package decision

import (
	"context"
	"strings"
	"sync"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
)

// DecisionInput is the input payload for Tier 1 evaluation
type DecisionInput struct {
	BrandID         string
	DimensionScores map[string]float64
	Conditions      map[string]bool
	RoutineOverride *domain.BrandRoutineBlueprint
}

// DecisionResult is the output from Tier 1 evaluation
type DecisionResult struct {
	DiagnosedConcerns      []string
	TargetActives          []string
	ProhibitedIngredients  []string
	RecommendedIngredients []string
	ResolvedBlueprint      domain.BrandRoutineBlueprint
}

// ClinicalDecisionEngine evaluates clinical rules, contraindications, and dynamic blueprints
type ClinicalDecisionEngine struct {
	mu                  sync.RWMutex
	conditionRules      []domain.IngredientConditionRelation
	dimensionEfficacies []domain.IngredientDimensionEfficacy
	brandBlueprints     map[string]domain.BrandRoutineBlueprint
}

// NewClinicalDecisionEngine creates a new instance initialized with default clinical knowledge
func NewClinicalDecisionEngine() *ClinicalDecisionEngine {
	blueprints := make(map[string]domain.BrandRoutineBlueprint)
	for k, v := range DefaultBrandBlueprints {
		blueprints[k] = v
	}

	return &ClinicalDecisionEngine{
		conditionRules:      append([]domain.IngredientConditionRelation{}, DefaultIngredientConditions...),
		dimensionEfficacies: append([]domain.IngredientDimensionEfficacy{}, DefaultDimensionEfficacies...),
		brandBlueprints:     blueprints,
	}
}

// RegisterConditionRule adds a new clinical condition relation
func (e *ClinicalDecisionEngine) RegisterConditionRule(rule domain.IngredientConditionRelation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.conditionRules = append(e.conditionRules, rule)
}

// RegisterDimensionEfficacy adds a new dimension efficacy mapping
func (e *ClinicalDecisionEngine) RegisterDimensionEfficacy(eff domain.IngredientDimensionEfficacy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dimensionEfficacies = append(e.dimensionEfficacies, eff)
}

// RegisterBrandBlueprint registers a brand routine blueprint
func (e *ClinicalDecisionEngine) RegisterBrandBlueprint(blueprint domain.BrandRoutineBlueprint) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.brandBlueprints[strings.ToLower(blueprint.BrandID)] = blueprint
}

// Evaluate runs Tier 1 clinical grading, safety exclusions, target active selection, and routine resolution
func (e *ClinicalDecisionEngine) Evaluate(ctx context.Context, input DecisionInput) (*DecisionResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	brandID := strings.ToLower(strings.TrimSpace(input.BrandID))
	if brandID == "" {
		brandID = "wardah"
	}

	scores := input.DimensionScores
	if scores == nil {
		scores = make(map[string]float64)
	}

	conditions := input.Conditions
	if conditions == nil {
		conditions = make(map[string]bool)
	}

	var diagnosedConcerns []string
	prohibitedSet := make(map[string]bool)
	recommendedSet := make(map[string]bool)
	targetActivesSet := make(map[string]bool)

	// 1. Skin Grading & Diagnosed Concerns
	sebum := scores["sebum"]
	hydration := scores["hydration"]
	sens := scores["sensitivity"]
	pigment := scores["pigmentation"]

	if sebum >= 66.0 {
		diagnosedConcerns = append(diagnosedConcerns, "Hyper-Seborrhea (Oily)")
	} else if sebum <= 35.0 && sebum > 0 {
		diagnosedConcerns = append(diagnosedConcerns, "Alipic Dry Skin (Lipid Deficit)")
	}

	if hydration <= 40.0 {
		diagnosedConcerns = append(diagnosedConcerns, "Transepidermal Dehydration")
	}

	if sens >= 65.0 || conditions["is_sensitive"] {
		diagnosedConcerns = append(diagnosedConcerns, "Reactive Skin Barrier")
		conditions["is_sensitive"] = true
	}

	if pigment >= 60.0 {
		diagnosedConcerns = append(diagnosedConcerns, "Hyperpigmentation & Uneven Tone")
	}

	// 2. Relational Safety & Contraindication Evaluation
	for _, rule := range e.conditionRules {
		if conditions[rule.ConditionCode] {
			switch rule.RelationshipType {
			case domain.RelationshipContraindicated:
				prohibitedSet[strings.ToUpper(rule.IngredientCode)] = true
			case domain.RelationshipRecommended:
				recommendedSet[strings.ToUpper(rule.IngredientCode)] = true
				targetActivesSet[strings.ToUpper(rule.IngredientCode)] = true
			}
		}
	}

	// 3. Relational Dimension Efficacy Resolution
	for _, eff := range e.dimensionEfficacies {
		ing := strings.ToUpper(eff.IngredientCode)
		if prohibitedSet[ing] {
			continue
		}

		switch eff.DimensionID {
		case "sebum":
			if sebum >= 66.0 && eff.ActionDirection == "REDUCE" {
				targetActivesSet[ing] = true
			}
		case "sebum_dry":
			if sebum <= 35.0 && sebum > 0 && eff.ActionDirection == "INCREASE" {
				targetActivesSet[ing] = true
			}
		case "hydration":
			if hydration <= 45.0 && eff.ActionDirection == "INCREASE" {
				targetActivesSet[ing] = true
			}
		case "sensitivity":
			if (sens >= 60.0 || conditions["is_sensitive"]) && eff.ActionDirection == "REDUCE" {
				targetActivesSet[ing] = true
			}
		case "pigmentation":
			if pigment >= 55.0 && eff.ActionDirection == "REDUCE" {
				targetActivesSet[ing] = true
			}
		}
	}

	// 4. Resolve Brand Routine Blueprint
	var resolvedBlueprint domain.BrandRoutineBlueprint
	if input.RoutineOverride != nil && len(input.RoutineOverride.Phases) > 0 {
		resolvedBlueprint = *input.RoutineOverride
	} else if bp, ok := e.brandBlueprints[brandID]; ok {
		resolvedBlueprint = bp
	} else {
		resolvedBlueprint = e.brandBlueprints["wardah"]
	}

	var prohibitedList []string
	for k := range prohibitedSet {
		prohibitedList = append(prohibitedList, k)
	}

	var recommendedList []string
	for k := range recommendedSet {
		recommendedList = append(recommendedList, k)
	}

	var targetActivesList []string
	for k := range targetActivesSet {
		targetActivesList = append(targetActivesList, k)
	}

	return &DecisionResult{
		DiagnosedConcerns:      diagnosedConcerns,
		TargetActives:          targetActivesList,
		ProhibitedIngredients:  prohibitedList,
		RecommendedIngredients: recommendedList,
		ResolvedBlueprint:      resolvedBlueprint,
	}, nil
}

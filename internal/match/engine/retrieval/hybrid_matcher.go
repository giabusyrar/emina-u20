package retrieval

import (
	"context"
	"sort"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/decision"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
)

// ScoredProduct represents a product scored and ranked for a specific slot
type ScoredProduct struct {
	Item        entity.ProductCatalogItem
	MatchScore  float64
	WhySelected []string
	KeyActives  []string
}

// HybridMatcher handles candidate filtering and multi-objective ranking
type HybridMatcher struct{}

// NewHybridMatcher creates a new HybridMatcher instance
func NewHybridMatcher() *HybridMatcher {
	return &HybridMatcher{}
}

// MatchSlot filters candidates by category & safety rules, then scores and ranks them
func (m *HybridMatcher) MatchSlot(
	ctx context.Context,
	slot domain.RoutineSlot,
	candidates []entity.ProductCatalogItem,
	dec *decision.DecisionResult,
	prefs domain.UserPreferences,
) []ScoredProduct {
	var matched []ScoredProduct

	prohibited := make(map[string]bool)
	if dec != nil {
		for _, p := range dec.ProhibitedIngredients {
			prohibited[strings.ToUpper(strings.TrimSpace(p))] = true
		}
	}

	targetActives := make(map[string]bool)
	if dec != nil {
		for _, a := range dec.TargetActives {
			targetActives[strings.ToUpper(strings.TrimSpace(a))] = true
		}
	}

	preferredTextures := make(map[string]bool)
	for _, t := range prefs.PreferredTextures {
		preferredTextures[strings.ToLower(strings.TrimSpace(t))] = true
	}

	avoidIngs := make(map[string]bool)
	for _, a := range prefs.AvoidIngredients {
		avoidIngs[strings.ToUpper(strings.TrimSpace(a))] = true
	}

	slotCat := strings.ToLower(strings.TrimSpace(slot.Category))

	for _, item := range candidates {
		// Category match check
		itemCat := strings.ToLower(strings.TrimSpace(item.Category))
		if slotCat != "" && !categoryMatches(itemCat, slotCat) {
			continue
		}

		// Parse product ingredients
		ings := parseIngredients(item.Ingredients)

		// 1. Safety Guardrail: Zero-tolerance contraindication check
		isSafe := true
		for _, ing := range ings {
			upper := strings.ToUpper(ing)
			if prohibited[upper] || avoidIngs[upper] {
				isSafe = false
				break
			}
		}
		if !isSafe {
			continue
		}

		// 2. Multi-Objective Scoring
		baseScore := 65.0
		var whySelected []string
		var keyActives []string

		// Efficacy Fit
		var matchedActivesCount int
		for _, ing := range ings {
			upper := strings.ToUpper(ing)
			if targetActives[upper] {
				matchedActivesCount++
				keyActives = append(keyActives, ing)
				whySelected = append(whySelected, "Contains clinical active "+ing)
			}
		}
		baseScore += float64(matchedActivesCount) * 10.0

		// Texture Fit
		if item.Texture != "" && preferredTextures[strings.ToLower(strings.TrimSpace(item.Texture))] {
			baseScore += 8.0
			whySelected = append(whySelected, "Matches preferred "+item.Texture+" texture")
		}

		if len(whySelected) == 0 {
			whySelected = append(whySelected, "Optimal foundational formula for "+item.Category)
		}

		if baseScore > 99.0 {
			baseScore = 99.0
		}

		matched = append(matched, ScoredProduct{
			Item:        item,
			MatchScore:  baseScore,
			WhySelected: whySelected,
			KeyActives:  keyActives,
		})
	}

	// Rank by MatchScore descending
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].MatchScore > matched[j].MatchScore
	})

	return matched
}

func categoryMatches(itemCat, slotCat string) bool {
	if itemCat == slotCat {
		return true
	}
	if strings.Contains(itemCat, slotCat) || strings.Contains(slotCat, itemCat) {
		return true
	}
	return false
}

func parseIngredients(raw string) []string {
	parts := strings.Split(raw, ",")
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

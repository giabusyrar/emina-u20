package conflict

import (
	"fmt"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/engine/retrieval"
)

type ConflictPair struct {
	ActiveA string
	ActiveB string
	Reason  string
}

// ConflictValidator validates active ingredient collisions within routine phases
type ConflictValidator struct {
	knownConflicts []ConflictPair
}

// NewConflictValidator initializes the validator with clinical conflict matrices
func NewConflictValidator() *ConflictValidator {
	return &ConflictValidator{
		knownConflicts: []ConflictPair{
			{ActiveA: "RETINOL", ActiveB: "GLYCOLIC_ACID", Reason: "Combining Retinol and AHA in the same phase causes barrier irritation and over-exfoliation."},
			{ActiveA: "RETINOL", ActiveB: "SALICYLIC_ACID", Reason: "Simultaneous Retinoid and BHA application may compromise lipid barrier."},
			{ActiveA: "RETINOL", ActiveB: "BENZOYL_PEROXIDE", Reason: "Benzoyl Peroxide oxidizes and deactivates tretinoin/retinol molecules."},
			{ActiveA: "ASCORBIC_ACID", ActiveB: "GLYCOLIC_ACID", Reason: "Dual low-pH acids in one phase can induce acute barrier erythema."},
		},
	}
}

// ValidateRoutine scans each routine phase for active collisions
func (v *ConflictValidator) ValidateRoutine(phaseProducts map[string][]retrieval.ScoredProduct) domain.ClinicalConflictReport {
	var conflictsDetected int
	var warnings []string
	var rulesApplied []string

	for phaseName, products := range phaseProducts {
		// Collect all active ingredients in this phase
		phaseActives := make(map[string]string) // upper active -> product ID
		for _, sp := range products {
			ings := strings.Split(sp.Item.Ingredients, ",")
			for _, ing := range ings {
				trimmed := strings.ToUpper(strings.TrimSpace(ing))
				if trimmed != "" {
					phaseActives[trimmed] = sp.Item.ID
				}
			}
		}

		// Check against known conflicts
		for _, rule := range v.knownConflicts {
			prodA, hasA := phaseActives[rule.ActiveA]
			prodB, hasB := phaseActives[rule.ActiveB]

			if hasA && hasB && prodA != prodB {
				conflictsDetected++
				warnMsg := fmt.Sprintf("[%s] Collision detected between %s (%s) and %s (%s): %s",
					phaseName, rule.ActiveA, prodA, rule.ActiveB, prodB, rule.Reason)
				warnings = append(warnings, warnMsg)
				rulesApplied = append(rulesApplied, fmt.Sprintf("Separate %s and %s across alternating phases", rule.ActiveA, rule.ActiveB))
			}
		}
	}

	return domain.ClinicalConflictReport{
		ConflictsDetected:    conflictsDetected,
		LayeringRulesApplied: rulesApplied,
		Warnings:             warnings,
	}
}

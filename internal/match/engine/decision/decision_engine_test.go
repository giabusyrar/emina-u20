package decision_test

import (
	"context"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/engine/decision"
)

func TestClinicalDecisionEngine_SafetyAndEfficacy(t *testing.T) {
	eng := decision.NewClinicalDecisionEngine()

	input := decision.DecisionInput{
		BrandID: "wardah",
		DimensionScores: map[string]float64{
			"sebum":       85.0, // Severe Oily
			"hydration":   30.0, // Dehydrated
			"sensitivity": 75.0, // Reactive
		},
		Conditions: map[string]bool{
			"is_pregnant":  true,
			"is_sensitive": true,
		},
	}

	res, err := eng.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Pregnancy Contraindications excluded
	hasRetinolBlacklist := false
	for _, ing := range res.ProhibitedIngredients {
		if ing == "RETINOL" {
			hasRetinolBlacklist = true
			break
		}
	}
	if !hasRetinolBlacklist {
		t.Fatalf("expected RETINOL in ProhibitedIngredients for pregnant profile")
	}

	// Verify Target Actives resolved for oily & dehydrated barrier
	if len(res.TargetActives) == 0 {
		t.Fatalf("expected resolved target actives, got empty")
	}

	// Verify Brand Routine Resolved
	if _, ok := res.ResolvedBlueprint.Phases["morning_protection"]; !ok {
		t.Fatalf("expected morning_protection phase for wardah routine")
	}
}

func TestClinicalDecisionEngine_MakeOverAndKahfParadigms(t *testing.T) {
	eng := decision.NewClinicalDecisionEngine()

	// Test Make Over
	moRes, err := eng.Evaluate(context.Background(), decision.DecisionInput{
		BrandID:         "makeover",
		DimensionScores: map[string]float64{"sebum": 50.0},
	})
	if err != nil {
		t.Fatalf("unexpected error for makeover: %v", err)
	}
	if _, ok := moRes.ResolvedBlueprint.Phases["skin_prep"]; !ok {
		t.Fatalf("expected skin_prep phase for Make Over")
	}

	// Test Kahf
	kahfRes, err := eng.Evaluate(context.Background(), decision.DecisionInput{
		BrandID:         "kahf",
		DimensionScores: map[string]float64{"sebum": 60.0},
	})
	if err != nil {
		t.Fatalf("unexpected error for kahf: %v", err)
	}
	if _, ok := kahfRes.ResolvedBlueprint.Phases["daily_morning_defense"]; !ok {
		t.Fatalf("expected daily_morning_defense phase for Kahf")
	}
}

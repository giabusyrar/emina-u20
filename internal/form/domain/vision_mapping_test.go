package domain_test

import (
	"testing"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

func TestAgeOverThirty(t *testing.T) {
	// "now" fixed at 2026-09-24 per this session's working date.
	now := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		dob  string
		want bool
	}{
		{"turns 30 today", "1996-09-24", false},        // spec: <=30 -> T, not over
		{"turned 31 today", "1995-09-24", true},        // 31 today -> over
		{"turns 30 tomorrow", "1996-09-25", false},     // still 29 -> not over
		{"28yo, birthday passed", "1998-04-12", false}, // matches the session's worked example
		{"exactly 30 but birthday not yet this year", "1996-12-25", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.AgeOverThirty(tc.dob, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("AgeOverThirty(%q) = %v, want %v", tc.dob, got, tc.want)
			}
		})
	}
}

func TestRulesetVisionSignals_InvertsHealthToConcern(t *testing.T) {
	schema := `{"field_mapping": {
		"aging": {"form": "age_over_30", "vision": "score_wrinkle"},
		"pigmentation": {"form": "pigmentation", "vision": "score_darkspot"},
		"sebum": {"form": "sebum"}
	}}`
	// Vendor is health-space (high = healthy).
	vendorFields := map[string]float64{
		"score_wrinkle":  85,
		"score_darkspot": 70,
	}

	got := domain.RulesetVisionSignals(schema, vendorFields)

	if got["aging"] != 15 {
		t.Errorf("aging = %v, want 15 (100 - 85)", got["aging"])
	}
	if got["pigmentation"] != 30 {
		t.Errorf("pigmentation = %v, want 30 (100 - 70)", got["pigmentation"])
	}
	if _, ok := got["sebum"]; ok {
		t.Errorf("sebum has no vision side and no matching vendor field — should be absent, got %v", got["sebum"])
	}
}

// TestRulesetVisionSignals_FullPathAvoidsPoreCollision reproduces the real
// bug found in staging: the vendor's response repeats "Pores" under both
// results.skin_level (1-5 severity grade) and results.skin_scoring (0-100
// health score). field_mapping must reference the full dot-path so it can
// only ever match the intended one, never the other short-name collision.
func TestRulesetVisionSignals_FullPathAvoidsPoreCollision(t *testing.T) {
	schema := `{"field_mapping": {
		"pore_severity": {"vision": "inference_result.results.skin_scoring.Pores"}
	}}`
	vendorFields := map[string]float64{
		"inference_result.results.skin_level.Pores":   3,  // 1-5 grade — must NOT be picked
		"inference_result.results.skin_scoring.Pores": 41, // 0-100 health score — the intended one
	}

	got := domain.RulesetVisionSignals(schema, vendorFields)

	if got["pore_severity"] != 59 {
		t.Errorf("pore_severity = %v, want 59 (100 - 41, the skin_scoring reading)", got["pore_severity"])
	}
}

func TestResolveRulesetCode(t *testing.T) {
	candidates := []domain.RulesetSchemaWithCode{
		{Code: "skinverse_skin_character", Status: "ACTIVE", Schema: `{"form_survey_code": "skinverse_q1_q6"}`},
		{Code: "some_other_ruleset", Status: "ACTIVE", Schema: `{}`},
	}

	if got := domain.ResolveRulesetCode(candidates, "skinverse_q1_q6"); got != "skinverse_skin_character" {
		t.Errorf("got %q, want skinverse_skin_character", got)
	}
	if got := domain.ResolveRulesetCode(candidates, "no_such_survey"); got != "no_such_survey" {
		t.Errorf("fallback: got %q, want the survey code unchanged", got)
	}
}

func TestResolveRulesetCodeIgnoresStatusCase(t *testing.T) {
	candidates := []domain.RulesetSchemaWithCode{
		{Code: "linked_ruleset", Status: "active", Schema: `{"form_survey_code": "q1"}`},
		{Code: "draft_ruleset", Status: "draft", Schema: `{"form_survey_code": "q2"}`},
	}
	if got := domain.ResolveRulesetCode(candidates, "q1"); got != "linked_ruleset" {
		t.Errorf("a lowercase 'active' ruleset must still link: got %q", got)
	}
	if got := domain.ResolveRulesetCode(candidates, "q2"); got != "q2" {
		t.Errorf("a non-active ruleset must not link: got %q", got)
	}
}

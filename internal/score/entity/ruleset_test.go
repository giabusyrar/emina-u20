package entity_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
)

func TestNormalizeRulesetStatus(t *testing.T) {
	cases := map[string]string{
		"":         entity.RulesetStatusActive,
		"ACTIVE":   "ACTIVE",
		"active":   "ACTIVE",
		" Active ": "ACTIVE",
		"draft":    "DRAFT",
	}
	for in, want := range cases {
		if got := entity.NormalizeRulesetStatus(in); got != want {
			t.Errorf("NormalizeRulesetStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

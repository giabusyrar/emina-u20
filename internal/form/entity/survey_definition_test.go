package entity_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/form/entity"
)

func TestSurveyDefinitionTableName(t *testing.T) {
	sd := entity.SurveyDefinition{}
	if sd.TableName() != "survey_definitions" {
		t.Fatalf("expected survey_definitions, got %s", sd.TableName())
	}
}

func TestNormalizeSurveyStatus(t *testing.T) {
	cases := map[string]string{
		"":         entity.SurveyStatusActive,
		"active":   "active",
		"ACTIVE":   "active",
		" Active ": "active",
		"DRAFT":    "draft",
		"archived": "archived",
	}
	for in, want := range cases {
		if got := entity.NormalizeSurveyStatus(in); got != want {
			t.Errorf("NormalizeSurveyStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

package entity_test

import (
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/entity"
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

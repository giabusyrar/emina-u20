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

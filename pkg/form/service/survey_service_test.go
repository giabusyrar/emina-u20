package service_test

import (
	"context"
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/form/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/form/service"
)

type mockSurveyProvider struct {
	survey *domain.SurveyModel
}

func TestSurveyService_Evaluate(t *testing.T) {
	schemaJSON := []byte(`{
		"title": "Wardah Quiz",
		"brand_id": "wardah",
		"application_id": "skin_app",
		"code": "wardah-quiz",
		"pages": [{
			"name": "page1",
			"elements": [{
				"type": "radiogroup",
				"name": "oil_level",
				"dimension": "OILINESS",
				"scale": {"min": 1, "max": 4},
				"choices": [
					{"value": "high", "text": "Very High", "score": 4, "condition_map": {"sebum_high": true}}
				]
			}]
		}]
	}`)

	model, err := domain.ParseSurveySchema(schemaJSON)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}
	model.BrandID = "wardah"
	model.ApplicationID = "skin_app"
	model.Code = "wardah-quiz"

	svc := service.NewSurveyService(nil, nil, nil, nil)
	ctx := context.Background()

	evalOut, err := svc.EvaluateWithModel(ctx, model, domain.SurveyEvaluateRequest{
		BrandID:       "wardah",
		ApplicationID: "skin_app",
		Data: map[string]interface{}{
			"oil_level": "high",
		},
		CustomerConditions: map[string]bool{
			"is_pregnant": true,
		},
	})
	if err != nil {
		t.Fatalf("failed to evaluate: %v", err)
	}

	if !evalOut.Success {
		t.Fatalf("expected success")
	}
	if !evalOut.CustomerCondition["is_pregnant"] || !evalOut.CustomerCondition["sebum_high"] {
		t.Fatalf("expected is_pregnant and sebum_high, got: %+v", evalOut.CustomerCondition)
	}
	if len(evalOut.AnswerList) != 1 || evalOut.AnswerList[0].Score != 4 {
		t.Fatalf("unexpected answers: %+v", evalOut.AnswerList)
	}
}

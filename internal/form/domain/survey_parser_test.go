package domain_test

import (
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

func TestParseSurveyAndExtractMetadata(t *testing.T) {
	sampleSchema := []byte(`{
		"title": "Wardah Quiz",
		"brand_id": "wardah",
		"application_id": "skin_diagnostic",
		"code": "wardah-skin-v1",
		"pages": [{
			"name": "page1",
			"elements": [
				{
					"type": "radiogroup",
					"name": "midday_shine",
					"dimension": "OILINESS",
					"scale": {"min": 1, "max": 4},
					"choices": [
						{"value": "tight", "text": "Tight", "score": 1},
						{"value": "oily", "text": "Oily", "score": 4, "condition_map": {"excess_sebum": true}}
					]
				},
				{
					"type": "checkbox",
					"name": "special_conditions",
					"choices": [
						{"value": "pregnant", "text": "Pregnant", "condition_map": {"is_pregnant": true, "avoid_retinol": true}},
						{"value": "rosacea", "text": "Rosacea", "dimension": "SENSITIVITY", "score": 4, "condition_map": {"barrier_compromised": true}}
					]
				},
				{
					"type": "text",
					"name": "water_glasses",
					"inputType": "number",
					"dimension": "HYDRATION",
					"scale": {"min": 0, "max": 20}
				}
			]
		}]
	}`)

	model, err := domain.ParseSurveySchema(sampleSchema)
	if err != nil {
		t.Fatalf("unexpected error parsing schema: %v", err)
	}

	if model.Code != "wardah-skin-v1" {
		t.Fatalf("expected code wardah-skin-v1, got %s", model.Code)
	}

	answers := map[string]interface{}{
		"midday_shine":       "oily",
		"special_conditions": []interface{}{"pregnant", "rosacea"},
		"water_glasses":      8.0,
	}

	answerList, dims, conds, _, err := domain.ExtractEvaluationData(model, answers)
	if err != nil {
		t.Fatalf("unexpected error extracting evaluation data: %v", err)
	}

	if len(answerList) < 3 {
		t.Fatalf("expected at least 3 answers, got %d", len(answerList))
	}

	// Verify dimensions
	dimMap := make(map[string][]int)
	for _, d := range dims {
		dimMap[d.Key] = d.Answers
	}

	if scores, ok := dimMap["OILINESS"]; !ok || len(scores) != 1 || scores[0] != 4 {
		t.Fatalf("unexpected OILINESS scores: %v", scores)
	}
	if scores, ok := dimMap["SENSITIVITY"]; !ok || len(scores) != 1 || scores[0] != 4 {
		t.Fatalf("unexpected SENSITIVITY scores: %v", scores)
	}
	if scores, ok := dimMap["HYDRATION"]; !ok || len(scores) != 1 || scores[0] != 8 {
		t.Fatalf("unexpected HYDRATION scores: %v", scores)
	}

	// Verify conditions
	if !conds["is_pregnant"] || !conds["avoid_retinol"] || !conds["barrier_compromised"] || !conds["excess_sebum"] {
		t.Fatalf("expected condition flags to be set, got: %+v", conds)
	}
}

func TestExtractEvaluationData_NewTypesAndCalcMethods(t *testing.T) {
	schema := []byte(`{
		"code": "types-v1",
		"calculation_methods": {"SEBUM": "average", "SENSITIVITY": "boolean_or"},
		"pages": [{"name": "p1", "elements": [
			{"type": "rating", "name": "r1", "dimension": "SEBUM", "scale": {"min": 1, "max": 5}},
			{"type": "boolean", "name": "b1", "dimension": "SENSITIVITY", "scoreTrue": 3, "scoreFalse": 0},
			{"type": "tagbox", "name": "t1", "dimension": "SEBUM", "choices": [
				{"value": "a", "text": "A", "score": 2},
				{"value": "b", "text": "B", "score": 4}
			]},
			{"type": "ranking", "name": "k1", "dimension": "PORES", "choices": [
				{"value": "x", "text": "X", "score": 1},
				{"value": "y", "text": "Y", "score": 5}
			]}
		]}]
	}`)

	model, err := domain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	answers := map[string]interface{}{
		"r1": 4.0,
		"b1": true,
		"t1": []interface{}{"a", "b"},
		"k1": []interface{}{"y", "x"},
	}

	_, dims, _, _, err := domain.ExtractEvaluationData(model, answers)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	got := map[string]domain.DimensionRef{}
	for _, d := range dims {
		got[d.Key] = d
	}

	if d := got["SEBUM"]; d.CalculationMethod != "average" || len(d.Answers) != 3 {
		t.Fatalf("SEBUM: method=%q answers=%v", d.CalculationMethod, d.Answers) // rating 4 + tagbox 2 + 4
	}
	if d := got["SENSITIVITY"]; d.CalculationMethod != "boolean_or" || len(d.Answers) != 1 || d.Answers[0] != 3 {
		t.Fatalf("SENSITIVITY: method=%q answers=%v", d.CalculationMethod, d.Answers)
	}
	if d := got["PORES"]; d.CalculationMethod != "sum" || len(d.Answers) != 2 {
		t.Fatalf("PORES (default sum): method=%q answers=%v", d.CalculationMethod, d.Answers)
	}
}

func TestExtractEvaluationData_Matrix(t *testing.T) {
	schema := []byte(`{
		"code": "matrix-v1",
		"calculation_methods": {"CONCERN": "average"},
		"pages": [{"name": "p1", "elements": [
			{"type": "matrix", "name": "m1", "dimension": "CONCERN",
			 "rows": [{"value": "dryness", "text": "Dryness"}, {"value": "oil", "text": "Oiliness"}],
			 "columns": [{"value": "low", "text": "Low", "score": 1}, {"value": "high", "text": "High", "score": 5}]}
		]}]
	}`)
	model, err := domain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	answers := map[string]interface{}{
		"m1": map[string]interface{}{"dryness": "high", "oil": "low"},
	}
	_, dims, _, _, err := domain.ExtractEvaluationData(model, answers)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(dims) != 1 || dims[0].Key != "CONCERN" || dims[0].CalculationMethod != "average" {
		t.Fatalf("unexpected dims: %+v", dims)
	}
	sum := 0
	for _, s := range dims[0].Answers {
		sum += s
	}
	if len(dims[0].Answers) != 2 || sum != 6 { // 5 + 1
		t.Fatalf("expected [5 1], got %v", dims[0].Answers)
	}
}

func TestExtractEvaluationData_TheoreticalBounds(t *testing.T) {
	// Pixie-style: a checkbox scored across the sebum spectrum (sum), three
	// 1..3 radiogroups rolled up by average, and a Yes/No boolean_or.
	schema := []byte(`{
		"code": "bounds_test",
		"pages": [{ "name": "p1", "elements": [
			{ "type": "checkbox", "name": "sebum_q", "dimension": "sebum", "choices": [
				{ "value": "a", "text": "a", "score": 2 },
				{ "value": "b", "text": "b", "score": 1 },
				{ "value": "c", "text": "c", "score": 3 },
				{ "value": "d", "text": "d", "score": -1 },
				{ "value": "e", "text": "e", "score": -2 },
				{ "value": "f", "text": "f", "score": -3 },
				{ "value": "g", "text": "g", "score": 0 }
			]},
			{ "type": "radiogroup", "name": "pig_a", "dimension": "pigmentation", "choices": [
				{ "value": "1", "text": "1", "score": 1 },
				{ "value": "2", "text": "2", "score": 2 },
				{ "value": "3", "text": "3", "score": 3 }
			]},
			{ "type": "radiogroup", "name": "pig_b", "dimension": "pigmentation", "choices": [
				{ "value": "1", "text": "1", "score": 1 },
				{ "value": "3", "text": "3", "score": 3 }
			]},
			{ "type": "radiogroup", "name": "ds_q", "dimension": "dark_spot", "choices": [
				{ "value": "yes", "text": "Yes", "score": 1 },
				{ "value": "no", "text": "No", "score": 0 }
			]}
		]}],
		"calculation_methods": { "sebum": "sum", "pigmentation": "average", "dark_spot": "boolean_or" }
	}`)
	model, err := domain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	answers := map[string]interface{}{
		"sebum_q": []interface{}{"c", "a"},
		"pig_a":   "2",
		"pig_b":   "3",
		"ds_q":    "yes",
	}
	_, dims, _, _, err := domain.ExtractEvaluationData(model, answers)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := map[string]domain.DimensionRef{}
	for _, d := range dims {
		got[d.Key] = d
	}
	// sum: negatives -1-2-3 = -6 .. positives 2+1+3 = 6
	if got["sebum"].MinScore != -6 || got["sebum"].MaxScore != 6 {
		t.Errorf("sebum bounds: want [-6, 6], got [%d, %d]", got["sebum"].MinScore, got["sebum"].MaxScore)
	}
	// average: widest single-question range is [1, 3]
	if got["pigmentation"].MinScore != 1 || got["pigmentation"].MaxScore != 3 {
		t.Errorf("pigmentation bounds: want [1, 3], got [%d, %d]", got["pigmentation"].MinScore, got["pigmentation"].MaxScore)
	}
	// boolean_or: [0, 1]
	if got["dark_spot"].MinScore != 0 || got["dark_spot"].MaxScore != 1 {
		t.Errorf("dark_spot bounds: want [0, 1], got [%d, %d]", got["dark_spot"].MinScore, got["dark_spot"].MaxScore)
	}
}

func TestValidateRequiredAnswers(t *testing.T) {
	schema := []byte(`{
		"title": "t", "code": "c",
		"pages": [{"name": "page1", "elements": [
			{"type": "checkbox", "name": "q1", "isRequired": true, "choices": [{"value": "a", "text": "A"}]},
			{"type": "radiogroup", "name": "q2", "isRequired": true, "choices": [{"value": "yes", "text": "Ya"}]},
			{"type": "checkbox", "name": "q3", "choices": [{"value": "x", "text": "X"}]}
		]}]
	}`)
	model, err := domain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name    string
		answers map[string]interface{}
		wantErr bool
	}{
		{"all required answered", map[string]interface{}{"q1": []interface{}{"a"}, "q2": "yes"}, false},
		{"q1 missing entirely", map[string]interface{}{"q2": "yes"}, true},
		{"q1 present but empty selection", map[string]interface{}{"q1": []interface{}{}, "q2": "yes"}, true},
		{"q2 present but empty string", map[string]interface{}{"q1": []interface{}{"a"}, "q2": ""}, true},
		{"q3 (not required) never blocks", map[string]interface{}{"q1": []interface{}{"a"}, "q2": "yes"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verrs := domain.ValidateRequiredAnswers(model, tc.answers)
			if verrs.HasErrors() != tc.wantErr {
				t.Errorf("HasErrors() = %v, want %v (errors: %v)", verrs.HasErrors(), tc.wantErr, verrs.Errors)
			}
		})
	}
}

package service

import (
	"context"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/score/domain"
	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
)

const testBaumannJDM = `{
  "nodes": [
    {
      "id": "input_node",
      "name": "Input Request",
      "type": "inputNode",
      "position": { "x": 50, "y": 200 }
    },
    {
      "id": "decision_table_oiliness",
      "name": "Oiliness Axis (O/D)",
      "type": "decisionTableNode",
      "content": {
        "hitPolicy": "first",
        "inputs": [
          { "id": "in_sebum", "field": "dimension_scores.sebum", "label": "Sebum Score" }
        ],
        "outputs": [
          { "id": "out_oil_code", "field": "axis_values.OILINESS", "label": "Axis Code" },
          { "id": "out_oil_grade", "field": "tiers.sebum.grade_name", "label": "Grade Name" },
          { "id": "out_oil_sev", "field": "tiers.sebum.severity", "label": "Severity" },
          { "id": "out_oil_trait", "field": "traits.sebum", "label": "Trait" }
        ],
        "rules": [
          { "_id": "r1", "in_sebum": "[0..25]", "out_oil_code": "\"O\"", "out_oil_grade": "\"Seborrheic Oily (Grade IV)\"", "out_oil_sev": "\"severe\"", "out_oil_trait": "\"Oily\"" },
          { "_id": "r2", "in_sebum": "[26..50]", "out_oil_code": "\"O\"", "out_oil_grade": "\"Mild Oily (Grade III)\"", "out_oil_sev": "\"moderate\"", "out_oil_trait": "\"Oily\"" },
          { "_id": "r3", "in_sebum": "[51..74]", "out_oil_code": "\"D\"", "out_oil_grade": "\"Balanced (Grade II)\"", "out_oil_sev": "\"optimal\"", "out_oil_trait": "\"Normal\"" },
          { "_id": "r4", "in_sebum": "[75..100]", "out_oil_code": "\"D\"", "out_oil_grade": "\"Resistant Dry (Grade I)\"", "out_oil_sev": "\"optimal\"", "out_oil_trait": "\"Dry\"" },
          { "_id": "r5", "in_sebum": "-", "out_oil_code": "\"D\"", "out_oil_grade": "\"Normal\"", "out_oil_sev": "\"optimal\"", "out_oil_trait": "\"Normal\"" }
        ]
      },
      "position": { "x": 300, "y": 80 }
    },
    {
      "id": "decision_table_sensitivity",
      "name": "Sensitivity Axis (S/R)",
      "type": "decisionTableNode",
      "content": {
        "hitPolicy": "first",
        "inputs": [
          { "id": "in_sens", "field": "dimension_scores.sensitivity", "label": "Sensitivity Score" }
        ],
        "outputs": [
          { "id": "out_sens_code", "field": "axis_values.SENSITIVITY", "label": "Axis Code" },
          { "id": "out_sens_grade", "field": "tiers.sensitivity.grade_name", "label": "Grade Name" },
          { "id": "out_sens_sev", "field": "tiers.sensitivity.severity", "label": "Severity" },
          { "id": "out_sens_trait", "field": "traits.sensitivity", "label": "Trait" }
        ],
        "rules": [
          { "_id": "s1", "in_sens": "[0..30]", "out_sens_code": "\"S\"", "out_sens_grade": "\"Highly Reactive (Grade III)\"", "out_sens_sev": "\"severe\"", "out_sens_trait": "\"Sensitive\"" },
          { "_id": "s2", "in_sens": "[31..60]", "out_sens_code": "\"S\"", "out_sens_grade": "\"Mild Sensitive (Grade II)\"", "out_sens_sev": "\"mild\"", "out_sens_trait": "\"Sensitive\"" },
          { "_id": "s3", "in_sens": "[61..100]", "out_sens_code": "\"R\"", "out_sens_grade": "\"Resistant (Grade I)\"", "out_sens_sev": "\"optimal\"", "out_sens_trait": "\"Resistant\"" },
          { "_id": "s4", "in_sens": "-", "out_sens_code": "\"R\"", "out_sens_grade": "\"Resistant\"", "out_sens_sev": "\"optimal\"", "out_sens_trait": "\"Resistant\"" }
        ]
      },
      "position": { "x": 300, "y": 200 }
    },
    {
      "id": "decision_table_pigmentation",
      "name": "Pigmentation Axis (P/N)",
      "type": "decisionTableNode",
      "content": {
        "hitPolicy": "first",
        "inputs": [
          { "id": "in_pigm", "field": "dimension_scores.pigmentation", "label": "Pigmentation Score" }
        ],
        "outputs": [
          { "id": "out_pigm_code", "field": "axis_values.PIGMENTATION", "label": "Axis Code" },
          { "id": "out_pigm_grade", "field": "tiers.pigmentation.grade_name", "label": "Grade Name" },
          { "id": "out_pigm_sev", "field": "tiers.pigmentation.severity", "label": "Severity" },
          { "id": "out_pigm_trait", "field": "traits.pigmentation", "label": "Trait" }
        ],
        "rules": [
          { "_id": "p1", "in_pigm": "[0..49]", "out_pigm_code": "\"P\"", "out_pigm_grade": "\"Hyperpigmented / Melasma Prone\"", "out_pigm_sev": "\"moderate\"", "out_pigm_trait": "\"Pigmented\"" },
          { "_id": "p2", "in_pigm": "[50..100]", "out_pigm_code": "\"N\"", "out_pigm_grade": "\"Non-Pigmented (Uniform)\"", "out_pigm_sev": "\"optimal\"", "out_pigm_trait": "\"Non-Pigmented\"" },
          { "_id": "p3", "in_pigm": "-", "out_pigm_code": "\"N\"", "out_pigm_grade": "\"Non-Pigmented\"", "out_pigm_sev": "\"optimal\"", "out_pigm_trait": "\"Non-Pigmented\"" }
        ]
      },
      "position": { "x": 300, "y": 320 }
    },
    {
      "id": "decision_table_aging",
      "name": "Aging & Elasticity Axis (W/T)",
      "type": "decisionTableNode",
      "content": {
        "hitPolicy": "first",
        "inputs": [
          { "id": "in_aging", "field": "dimension_scores.aging", "label": "Aging Score" }
        ],
        "outputs": [
          { "id": "out_aging_code", "field": "axis_values.AGING", "label": "Axis Code" },
          { "id": "out_aging_grade", "field": "tiers.aging.grade_name", "label": "Grade Name" },
          { "id": "out_aging_sev", "field": "tiers.aging.severity", "label": "Severity" },
          { "id": "out_aging_trait", "field": "traits.aging", "label": "Trait" }
        ],
        "rules": [
          { "_id": "a1", "in_aging": "[0..49]", "out_aging_code": "\"W\"", "out_aging_grade": "\"Wrinkle-Prone & Sagging\"", "out_aging_sev": "\"moderate\"", "out_aging_trait": "\"Wrinkled\"" },
          { "_id": "a2", "in_aging": "[50..100]", "out_aging_code": "\"T\"", "out_aging_grade": "\"Tight & Firm (Youthful)\"", "out_aging_sev": "\"optimal\"", "out_aging_trait": "\"Tight\"" },
          { "_id": "a3", "in_aging": "-", "out_aging_code": "\"T\"", "out_aging_grade": "\"Tight\"", "out_aging_sev": "\"optimal\"", "out_aging_trait": "\"Tight\"" }
        ]
      },
      "position": { "x": 300, "y": 440 }
    },
    {
      "id": "output_node",
      "name": "Output Diagnosis",
      "type": "outputNode",
      "position": { "x": 600, "y": 260 }
    }
  ],
  "edges": [
    { "id": "e1", "sourceId": "input_node", "targetId": "decision_table_oiliness" },
    { "id": "e2", "sourceId": "input_node", "targetId": "decision_table_sensitivity" },
    { "id": "e3", "sourceId": "input_node", "targetId": "decision_table_pigmentation" },
    { "id": "e4", "sourceId": "input_node", "targetId": "decision_table_aging" },
    { "id": "e5", "sourceId": "decision_table_oiliness", "targetId": "output_node" },
    { "id": "e6", "sourceId": "decision_table_sensitivity", "targetId": "output_node" },
    { "id": "e7", "sourceId": "decision_table_pigmentation", "targetId": "output_node" },
    { "id": "e8", "sourceId": "decision_table_aging", "targetId": "output_node" }
  ]
}`

type mockRulesetRepo struct {
	ruleset entity.ScoreRuleset
}

func (m *mockRulesetRepo) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error) {
	return &m.ruleset, nil
}

func (m *mockRulesetRepo) ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error) {
	return []entity.ScoreRuleset{m.ruleset}, nil
}

func (m *mockRulesetRepo) GetRulesetByID(ctx context.Context, id string) (*entity.ScoreRuleset, error) {
	return &m.ruleset, nil
}

func (m *mockRulesetRepo) SaveRuleset(ctx context.Context, item *entity.ScoreRuleset) error {
	m.ruleset = *item
	return nil
}

func (m *mockRulesetRepo) DeleteRuleset(ctx context.Context, id string) error {
	return nil
}

func newTestScoreService() *ScoreService {
	repo := &mockRulesetRepo{
		ruleset: entity.ScoreRuleset{
			ID:            "test-baumann",
			BrandID:       "*",
			ApplicationID: "*",
			Code:          "baumann_16_types",
			Title:         "Baumann 16 Skin Types (Zen JDM Decision Graph)",
			Status:        "ACTIVE",
			Version:       1,
			Schema:        testBaumannJDM,
		},
	}
	return NewScoreService(repo)
}

func TestScoreService_Evaluate_BaumannModel(t *testing.T) {
	svc := newTestScoreService()

	// OSPT profile test (health-oriented): raw concern sebum 80/sens 75/pigm 60/aging 30
	// -> health 20 (Oily 'O') / 25 (Sensitive 'S') / 40 (Pigmented 'P') / 70 (Tight 'T')
	req := domain.ScoreModuleRequest{
		BrandID:       "wardah",
		ApplicationID: "app_mobile_survey",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			{Key: "sebum", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{40, 40}},
			{Key: "sensitivity", MinScore: 0, MaxScore: 100, CalculationMethod: "average", Answers: []int{75}},
			{Key: "pigmentation", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{60}},
			{Key: "aging", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{30}},
		},
		CustomerCondition: map[string]bool{
			"is_pregnant": false,
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success true, got false")
	}

	if resp.SkinProfile.Code != "OSPT" {
		t.Errorf("expected SkinProfile.Code 'OSPT', got %q", resp.SkinProfile.Code)
	}

	if resp.SkinProfile.AxisValues["OILINESS"] != "O" {
		t.Errorf("expected OILINESS 'O', got %q", resp.SkinProfile.AxisValues["OILINESS"])
	}

	if resp.SkinProfile.AxisValues["SENSITIVITY"] != "S" {
		t.Errorf("expected SENSITIVITY 'S', got %q", resp.SkinProfile.AxisValues["SENSITIVITY"])
	}

	if resp.SkinProfile.AxisValues["PIGMENTATION"] != "P" {
		t.Errorf("expected PIGMENTATION 'P', got %q", resp.SkinProfile.AxisValues["PIGMENTATION"])
	}

	if resp.SkinProfile.AxisValues["AGING"] != "T" {
		t.Errorf("expected AGING 'T', got %q", resp.SkinProfile.AxisValues["AGING"])
	}

	// Verify severity tiers
	if len(resp.SkinGradingTiers) == 0 {
		t.Errorf("expected skin grading tiers, got none")
	}
}

func TestScoreService_Evaluate_MultiModalFusion(t *testing.T) {
	svc := newTestScoreService()

	// Fusion runs in concern space: form concern sebum = 40, vision concern = 90.
	// Vision weight 0.7, form weight 0.3 -> fused concern = (40*0.3)+(90*0.7) = 75,
	// then flipped to health -> 25 (Oily 'O').
	req := domain.ScoreModuleRequest{
		BrandID:       "*",
		ApplicationID: "*",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			{Key: "sebum", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{40}},
			{Key: "sensitivity", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{20}},
			{Key: "pigmentation", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{10}},
			{Key: "aging", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{10}},
		},
		VisionSignals: map[string]float64{
			"sebum": 90.0,
		},
		FusionConfig: &domain.FusionConfig{
			DefaultWeights: domain.DimensionWeight{
				FormWeight:   0.3,
				VisionWeight: 0.7,
			},
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DimensionScores["sebum"] != 25.0 {
		t.Errorf("expected fused+flipped sebum score 25.0, got %f", resp.DimensionScores["sebum"])
	}

	if resp.SkinProfile.AxisValues["OILINESS"] != "O" {
		t.Errorf("expected OILINESS 'O' after fusion, got %q", resp.SkinProfile.AxisValues["OILINESS"])
	}
}

func TestScoreService_Evaluate_MinMaxNormalization(t *testing.T) {
	svc := newTestScoreService()

	req := domain.ScoreModuleRequest{
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			// sum: raw 5 in [-10, 10] -> concern 75 -> health 100-75 = 25
			{Key: "sebum", MinScore: -10, MaxScore: 10, CalculationMethod: "sum", Answers: []int{3, 2}},
			// average: raw avg 2 in [1, 3] -> concern 50 -> health 50
			{Key: "sensitivity", MinScore: 1, MaxScore: 3, CalculationMethod: "average", Answers: []int{1, 3}},
			// boolean_or: a positive answer -> concern 100 -> health 0
			{Key: "pigmentation", MinScore: 0, MaxScore: 1, CalculationMethod: "boolean_or", Answers: []int{1}},
			// boolean_or: no positive answer -> concern 0 -> health 100
			{Key: "dark_spot", MinScore: 0, MaxScore: 1, CalculationMethod: "boolean_or", Answers: []int{0}},
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]float64{
		"sebum":        25.0,
		"sensitivity":  50.0,
		"pigmentation": 0.0,
		"dark_spot":    100.0,
	}
	for key, want := range cases {
		if got := resp.DimensionScores[key]; got != want {
			t.Errorf("%s: expected normalized %.1f, got %.1f", key, want, got)
		}
	}
}

func TestScoreService_Simulate(t *testing.T) {
	svc := newTestScoreService()

	simReq := entity.RulesetSimulationRequest{
		Schema: testBaumannJDM,
		FormScores: map[string]float64{
			"sebum":        80.0,
			"sensitivity":  70.0,
			"pigmentation": 60.0,
			"aging":        30.0,
		},
	}

	simResp, err := svc.Simulate(context.Background(), simReq)
	if err != nil {
		t.Fatalf("unexpected error during simulation: %v", err)
	}

	if !simResp.Success {
		t.Errorf("expected simulation success, got error: %s", simResp.Error)
	}

	if simResp.Result == nil {
		t.Fatalf("expected non-nil result map")
	}
}

func BenchmarkScoreService_Evaluate(b *testing.B) {
	svc := newTestScoreService()

	req := domain.ScoreModuleRequest{
		BrandID:       "wardah",
		ApplicationID: "app_mobile_survey",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			{Key: "sebum", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{40, 40}},
			{Key: "sensitivity", MinScore: 0, MaxScore: 100, CalculationMethod: "average", Answers: []int{75}},
			{Key: "pigmentation", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{60}},
			{Key: "aging", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{30}},
		},
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Evaluate(ctx, req)
	}
}

// TestScoreService_Evaluate_SkinGradingFacets verifies the health-oriented
// facets: Score Range (coarse 3), Severity Level (clinical 5) and Skin Concern
// (argmin). Uses the default bands since testBaumannJDM carries no overrides.
func TestScoreService_Evaluate_SkinGradingFacets(t *testing.T) {
	svc := newTestScoreService()

	// health: sebum 25, sensitivity 50, pigmentation 0, dark_spot 100 -> mean 43.75 -> 43.8
	req := domain.ScoreModuleRequest{
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			{Key: "sebum", MinScore: -10, MaxScore: 10, CalculationMethod: "sum", Answers: []int{3, 2}},
			{Key: "sensitivity", MinScore: 1, MaxScore: 3, CalculationMethod: "average", Answers: []int{1, 3}},
			{Key: "pigmentation", MinScore: 0, MaxScore: 1, CalculationMethod: "boolean_or", Answers: []int{1}},
			{Key: "dark_spot", MinScore: 0, MaxScore: 1, CalculationMethod: "boolean_or", Answers: []int{0}},
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.TotalScore < 43.0 || resp.TotalScore > 44.5 {
		t.Errorf("total_score: expected ~43.8, got %.1f", resp.TotalScore)
	}
	if resp.ScoreRange != "Sedang" {
		t.Errorf("score_range: expected Sedang (41-60), got %q", resp.ScoreRange)
	}
	if resp.SeverityLevel != "Sedang" {
		t.Errorf("severity_level: expected Sedang (41-60), got %q", resp.SeverityLevel)
	}
	if resp.SkinConcern == nil {
		t.Fatalf("skin_concern: expected non-nil (pigmentation health 0)")
	}
	if resp.SkinConcern.Dimension != "pigmentation" {
		t.Errorf("skin_concern.dimension: expected pigmentation (argmin), got %q", resp.SkinConcern.Dimension)
	}
	if resp.SkinConcern.Label != "Noda Gelap" {
		t.Errorf("skin_concern.label: expected Noda Gelap, got %q", resp.SkinConcern.Label)
	}
}

// TestScoreService_Evaluate_OptimalHasNoConcern verifies that an all-low
// (optimal) assessment reports no Skin Concern.
func TestScoreService_Evaluate_OptimalHasNoConcern(t *testing.T) {
	svc := newTestScoreService()

	// health: sebum concern 15 -> 85, sensitivity concern 8 -> 92, mean 88.5
	req := domain.ScoreModuleRequest{
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		Code:          "baumann_16_types",
		Dimensions: []domain.DimensionRef{
			{Key: "sebum", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{10, 5}},
			{Key: "sensitivity", MinScore: 0, MaxScore: 100, CalculationMethod: "sum", Answers: []int{8}},
		},
	}

	resp, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ScoreRange != "Optimal" {
		t.Errorf("score_range: expected Optimal, got %q (total %.1f)", resp.ScoreRange, resp.TotalScore)
	}
	if resp.SeverityLevel != "Sehat" {
		t.Errorf("severity_level: expected Sehat, got %q", resp.SeverityLevel)
	}
	if resp.SkinConcern != nil {
		t.Errorf("skin_concern: expected nil for optimal skin, got %+v", resp.SkinConcern)
	}
}

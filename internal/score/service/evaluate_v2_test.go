package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/entity"

	formdomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

// stubVisionProvider returns a fixed, incomplete flattened response — used
// to reproduce "Paradev answered, but the mapped field isn't in it" without
// a real vendor call.
type stubVisionProvider struct {
	code      string
	flattened map[string]float64
}

func (p *stubVisionProvider) Code() string { return p.code }
func (p *stubVisionProvider) Submit(ctx context.Context, sub domain.VisionSubmission) (*domain.VisionResult, error) {
	return &domain.VisionResult{Flattened: p.flattened, TransactionID: "stub-tx"}, nil
}

// stubSurveyProvider returns a fixed one-question survey binding "Q3_PN" to
// the pigmentation dimension, just enough to give EvaluateV2's form-side a
// real answer to work with in tests that don't need the full Skinverse form.
type stubSurveyProvider struct{ model *formdomain.SurveyModel }

func (p *stubSurveyProvider) GetSurvey(ctx context.Context, brandID, applicationID, code string) (*formdomain.SurveyModel, error) {
	return p.model, nil
}

func newStubSurveyProvider(t *testing.T) *stubSurveyProvider {
	t.Helper()
	schema := []byte(`{
		"title": "t", "code": "test_survey",
		"pages": [{"name": "page1", "elements": [
			{"type": "radiogroup", "name": "Q3_PN", "dimension": "pigmentation",
			 "choices": [{"value": "yes", "text": "Ya", "score": 1}, {"value": "no", "text": "Tidak", "score": 0}]}
		]}]
	}`)
	model, err := formdomain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("failed to parse stub survey schema: %v", err)
	}
	return &stubSurveyProvider{model: model}
}

// A minimal, Skinverse-shaped schema: axis_codes-driven (not the legacy
// per-dimension tiers.* style), no profile decisionTableNode at all — the
// composed-from-axis-letters fallback in composeSkinProfile is exercised
// directly. One extra node mirrors the real pore-band classifier, reading
// dimension_scores.pore_severity, to test that a non-axis dimension still
// shows up in the breakdown but never dilutes total_score.
const testSkinCharacterSchema = `{
  "nodes": [
    {
      "id": "pore-band",
      "type": "decisionTableNode",
      "content": {
        "hitPolicy": "first",
        "inputs": [{"id": "pore", "field": "dimension_scores.pore_severity"}],
        "outputs": [{"id": "pore_out", "field": "sub_classification.pore_severity"}],
        "rules": [
          {"pore": "[90..100]", "pore_out": "\"Smooth\""},
          {"pore": "[40..60]", "pore_out": "\"Visible\""},
          {"pore": "[0..10]", "pore_out": "\"Enlarged\""}
        ]
      }
    }
  ],
  "axis_codes": {
    "sebum": {"low": "O", "high": "D", "threshold": 56},
    "sensitivity": {"low": "S", "high": "R", "threshold": 50},
    "pigmentation": {"low": "P", "high": "N", "threshold": 50},
    "aging": {"low": "W", "high": "T", "threshold": 90}
  },
  "field_mapping": {
    "sebum": {"form": "sebum"},
    "sensitivity": {"form": "sensitivity"},
    "pigmentation": {"form": "pigmentation", "vision": "score_darkspot"},
    "aging": {"form": "age_over_30", "vision": "score_wrinkle"},
    "pore_severity": {"vision": "Pores"}
  },
  "dimension_fusion": {
    "pigmentation": {"form": 0.8, "vision": 0.2},
    "aging": {"form": 0.5, "vision": 0.5}
  },
  "dimension_weights": {"sebum": 1, "sensitivity": 1, "pigmentation": 1, "aging": 1},
  "vision_source_code": "test_provider",
  "form_survey_code": "test_survey"
}`

func newSkinCharacterTestService() *ScoreService {
	repo := &mockRulesetRepo{
		ruleset: entity.ScoreRuleset{
			ID:      "test-skin-character",
			Code:    "skinverse_skin_character",
			Status:  "ACTIVE",
			Version: 0,
			Schema:  testSkinCharacterSchema,
		},
	}
	return NewScoreService(repo)
}

// TestEvaluateV2_VisionFieldMissingWarning reproduces the real bug this
// session found: Paradev answers successfully, but the ruleset's
// field_mapping.vision path isn't in that particular response (a stale/wrong
// path, exactly what happened before the "data." prefix fix). That
// dimension must NOT silently look identical to "this ruleset has no vision
// side here" — it must stay source: "form" AND carry an explicit
// VISION_FIELD_MISSING warning naming the dimension and the path.
func TestEvaluateV2_VisionFieldMissingWarning(t *testing.T) {
	svc := newSkinCharacterTestService()
	stub := &stubVisionProvider{
		code: "test_provider",
		flattened: map[string]float64{
			"score_wrinkle": 90, // present -> aging should blend fine
			// "score_darkspot" and "Pores" deliberately absent.
		},
	}
	svc.WithEvaluateV2Deps(newStubSurveyProvider(t), domain.NewVisionProviderRegistry(stub))

	out, err := svc.EvaluateV2(context.Background(), EvaluateV2Input{
		RulesetCode:           "skinverse_skin_character",
		BrandID:               "wardah",
		ApplicationID:         "skinverse",
		CustomerID:            "test-vision-missing",
		Data:                  map[string]interface{}{"Q3_PN": "yes"},
		FullName:              "Test",
		PhoneNumber:           "+628123456789",
		Email:                 "t@example.com",
		DateOfBirth:           "1998-04-12",
		ConsentDataProcessing: true,
		Photo:                 strings.NewReader("fake-image-bytes"),
		PhotoFilename:         "test.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pigmentation := out.Dimensions["pigmentation"]
	if pigmentation.Source != "form" {
		t.Errorf("pigmentation.source = %q, want form (vision field was missing from vendor response)", pigmentation.Source)
	}

	foundDarkspotWarning, foundPoresWarning := false, false
	for _, w := range out.Warnings {
		if strings.Contains(w, "VISION_FIELD_MISSING") && strings.Contains(w, "pigmentation") && strings.Contains(w, "score_darkspot") {
			foundDarkspotWarning = true
		}
		if strings.Contains(w, "VISION_FIELD_MISSING") && strings.Contains(w, "pore_severity") && strings.Contains(w, "Pores") {
			foundPoresWarning = true
		}
	}
	if !foundDarkspotWarning {
		t.Errorf("expected a VISION_FIELD_MISSING warning naming pigmentation/score_darkspot, got %v", out.Warnings)
	}
	if !foundPoresWarning {
		t.Errorf("expected a VISION_FIELD_MISSING warning naming pore_severity/Pores, got %v", out.Warnings)
	}

	// aging's vision field WAS present -> no warning for it, real blend happens.
	aging := out.Dimensions["aging"]
	if aging.Source != "blend" {
		t.Errorf("aging.source = %q, want blend (its vision field was present)", aging.Source)
	}
}

// TestSyntheticDOBFromAge_ThirtyThresholdBoundary confirms the Simulate age
// slider's 30/31 boundary reuses AgeOverThirty's real "<=30 -> not over"
// semantics via the synthetic DOB, not an off-by-one from how AddDate lands
// on a birthday relative to "now".
func TestSyntheticDOBFromAge_ThirtyThresholdBoundary(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		age  int
		want bool
	}{
		{29, false},
		{30, false}, // spec: <=30 -> T, not over
		{31, true},
		{50, true},
	}
	for _, tc := range cases {
		dob := syntheticDOBFromAge(tc.age, now)
		got, err := formdomain.AgeOverThirty(dob, now)
		if err != nil {
			t.Fatalf("age %d: unexpected error: %v", tc.age, err)
		}
		if got != tc.want {
			t.Errorf("age %d (synthetic dob %s): AgeOverThirty = %v, want %v", tc.age, dob, got, tc.want)
		}
	}
}

// TestStage2Score_WorkedExample reproduces the hand-derived example from this
// session (Q1=["f"], Q2=["a"], Q3="yes", age 28, score_wrinkle=88,
// score_darkspot=85) directly at the stage2Score level, confirming the
// refactor produces the exact same numbers as the manual derivation:
// total_score 37.0, code OSPT, sebum health 36.8, aging health 94.0.
func TestStage2Score_WorkedExample(t *testing.T) {
	svc := newSkinCharacterTestService()
	ruleset := &entity.ScoreRuleset{Code: "skinverse_skin_character", Version: 0, Schema: testSkinCharacterSchema}

	out, err := svc.stage2Score(context.Background(), stage2Input{
		Ruleset: ruleset,
		FormConcern: map[string]float64{
			"sebum":        63.2, // Q1=["f"] score 2, bounds [-10,9] -> (2-(-10))/19*100
			"sensitivity":  100,  // Q2=["a"] boolean_or tripped -> max
			"pigmentation": 100,  // Q3="yes" boolean_or tripped -> max
			"aging":        0,    // age 28 <=30 -> concern 0
		},
		VisionConcern: map[string]float64{
			"aging":        12, // 100 - score_wrinkle(88)
			"pigmentation": 15, // 100 - score_darkspot(85)
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.TotalScore != 37.0 {
		t.Errorf("total_score = %v, want 37.0", out.TotalScore)
	}
	if out.SkinProfile.Code != "OSPT" {
		t.Errorf("skin_profile.code = %q, want OSPT", out.SkinProfile.Code)
	}
	if !out.SkinProfile.Complete {
		t.Errorf("skin_profile.complete = false, want true (all 4 axes resolved)")
	}

	sebum := out.Dimensions["sebum"]
	if sebum.Source != "form" || sebum.FinalScore == nil || *sebum.FinalScore != 36.8 {
		t.Errorf("sebum breakdown = %+v, want source=form final_score=36.8", sebum)
	}
	if sebum.Axis == nil || *sebum.Axis != "O" {
		t.Errorf("sebum.axis = %v, want O", sebum.Axis)
	}

	aging := out.Dimensions["aging"]
	if aging.Source != "blend" || aging.FinalScore == nil || *aging.FinalScore != 94.0 {
		t.Errorf("aging breakdown = %+v, want source=blend final_score=94.0", aging)
	}
	if aging.Weight == nil || aging.Weight.FormWeight != 0.5 || aging.Weight.VisionWeight != 0.5 {
		t.Errorf("aging.weight = %+v, want 0.5/0.5", aging.Weight)
	}
	if aging.Axis == nil || *aging.Axis != "T" {
		t.Errorf("aging.axis = %v, want T", aging.Axis)
	}
}

// TestStage2Score_NoDataDimensionIsNull confirms a dimension the ruleset
// registers (field_mapping) but that got no data at all this call comes back
// with source "none" and every score/axis field nil — not 0 and not a
// default letter — and that skin_profile.complete correctly flips to false.
func TestStage2Score_NoDataDimensionIsNull(t *testing.T) {
	svc := newSkinCharacterTestService()
	ruleset := &entity.ScoreRuleset{Code: "skinverse_skin_character", Version: 0, Schema: testSkinCharacterSchema}

	out, err := svc.stage2Score(context.Background(), stage2Input{
		Ruleset: ruleset,
		FormConcern: map[string]float64{
			"sebum":       63.2,
			"sensitivity": 100,
			// pigmentation and aging deliberately absent from both maps.
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pigmentation := out.Dimensions["pigmentation"]
	if pigmentation.Source != "none" {
		t.Errorf("pigmentation.source = %q, want none", pigmentation.Source)
	}
	if pigmentation.FormScore != nil || pigmentation.VisionScore != nil || pigmentation.FinalScore != nil || pigmentation.Axis != nil {
		t.Errorf("pigmentation with no data should be all-nil, got %+v", pigmentation)
	}

	if out.SkinProfile.Complete {
		t.Errorf("skin_profile.complete = true, want false (pigmentation/aging unresolved)")
	}
	if len(out.Warnings) < 2 {
		t.Errorf("expected a warning per unresolved dimension, got %v", out.Warnings)
	}
}

// TestStage2Score_PoreSeverityExcludedFromTotalScore confirms a non-axis
// dimension (pore_severity, only in field_mapping/sub_classification, never
// in axis_codes) still appears in the breakdown but never dilutes
// total_score, whether or not it has data this call.
func TestStage2Score_PoreSeverityExcludedFromTotalScore(t *testing.T) {
	svc := newSkinCharacterTestService()
	ruleset := &entity.ScoreRuleset{Code: "skinverse_skin_character", Version: 0, Schema: testSkinCharacterSchema}

	base := map[string]float64{"sebum": 0, "sensitivity": 0, "pigmentation": 0, "aging": 0} // all max health (concern 0)

	withoutPore, err := svc.stage2Score(context.Background(), stage2Input{Ruleset: ruleset, FormConcern: base})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	withPore, err := svc.stage2Score(context.Background(), stage2Input{
		Ruleset:       ruleset,
		FormConcern:   base,
		VisionConcern: map[string]float64{"pore_severity": 20}, // health 80 — would drag total_score down if wrongly included
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if withoutPore.TotalScore != withPore.TotalScore {
		t.Errorf("total_score changed when pore_severity got data: %v -> %v, want unchanged", withoutPore.TotalScore, withPore.TotalScore)
	}
	pore := withPore.Dimensions["pore_severity"]
	if pore.Source != "vision" || pore.Axis != nil {
		t.Errorf("pore_severity breakdown = %+v, want source=vision axis=nil", pore)
	}
}

// TestStage2Score_PoreSeverityCutoffPending confirms a pore_severity health
// score landing in the deliberately-uncovered pore-band gap (11-39 or
// 61-89 — no confirmed cutoff from Paradev yet) surfaces as an explicit
// sub_classification.pore_severity: null plus a PORE_CUTOFF_PENDING
// warning, instead of silently vanishing from sub_classification as if it
// had never been computed. A value that DOES land in a covered band must
// NOT trigger the warning.
func TestStage2Score_PoreSeverityCutoffPending(t *testing.T) {
	svc := newSkinCharacterTestService()
	ruleset := &entity.ScoreRuleset{Code: "skinverse_skin_character", Version: 0, Schema: testSkinCharacterSchema}

	// vision concern 20 -> health 80, inside the uncovered 61-89 gap.
	inGap, err := svc.stage2Score(context.Background(), stage2Input{
		Ruleset:       ruleset,
		VisionConcern: map[string]float64{"pore_severity": 20},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, has := inGap.SubClassification["pore_severity"]
	if !has {
		t.Fatalf("expected sub_classification.pore_severity to be present (as null), got absent entirely")
	}
	if val != nil {
		t.Errorf("sub_classification.pore_severity = %v, want nil", val)
	}
	foundWarning := false
	for _, w := range inGap.Warnings {
		if strings.Contains(w, "PORE_CUTOFF_PENDING") && strings.Contains(w, "80") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a PORE_CUTOFF_PENDING warning mentioning the health score, got %v", inGap.Warnings)
	}

	// vision concern 5 -> health 95, inside the covered [90..100] "Smooth" band.
	covered, err := svc.stage2Score(context.Background(), stage2Input{
		Ruleset:       ruleset,
		VisionConcern: map[string]float64{"pore_severity": 5},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if covered.SubClassification["pore_severity"] != "Smooth" {
		t.Errorf("sub_classification.pore_severity = %v, want \"Smooth\"", covered.SubClassification["pore_severity"])
	}
	for _, w := range covered.Warnings {
		if strings.Contains(w, "PORE_CUTOFF_PENDING") {
			t.Errorf("did not expect a PORE_CUTOFF_PENDING warning for a covered band, got %v", covered.Warnings)
		}
	}
}

// TestEvaluateV2_DOBMissingWarning confirms a form-only (or vision-only)
// call with no date_of_birth leaves the age-derived dimension (aging) null
// — source "none" — rather than silently treating the missing DOB as age 0
// (<=30, i.e. "not over thirty"), and names it in a DOB_MISSING warning.
func TestEvaluateV2_DOBMissingWarning(t *testing.T) {
	svc := newSkinCharacterTestService()
	svc.WithEvaluateV2Deps(newStubSurveyProvider(t), domain.NewVisionProviderRegistry())

	out, err := svc.EvaluateV2(context.Background(), EvaluateV2Input{
		RulesetCode:   "skinverse_skin_character",
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		CustomerID:    "test-dob-missing",
		Data:          map[string]interface{}{"Q3_PN": "yes"},
		// DateOfBirth deliberately omitted.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aging := out.Dimensions["aging"]
	if aging.Source != "none" {
		t.Errorf("aging.source = %q, want none (no DOB, no photo)", aging.Source)
	}
	if aging.FinalScore != nil {
		t.Errorf("aging.final_score = %v, want nil — a missing DOB must never read as age 0", *aging.FinalScore)
	}

	found := false
	for _, w := range out.Warnings {
		if strings.Contains(w, "DOB_MISSING") && strings.Contains(w, "aging") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a DOB_MISSING warning naming aging, got %v", out.Warnings)
	}
}

// TestEvaluateV2_VisionRequiredForAging confirms that even with a real
// date_of_birth, aging stays null without a photo — per the guideline the
// W/T axis is vision+DOB, and DOB alone can never resolve it — and that
// DOB_MISSING does NOT also fire, since DOB genuinely was provided.
func TestEvaluateV2_VisionRequiredForAging(t *testing.T) {
	svc := newSkinCharacterTestService()
	svc.WithEvaluateV2Deps(newStubSurveyProvider(t), domain.NewVisionProviderRegistry())

	out, err := svc.EvaluateV2(context.Background(), EvaluateV2Input{
		RulesetCode:   "skinverse_skin_character",
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		CustomerID:    "test-vision-required",
		Data:          map[string]interface{}{"Q3_PN": "yes"},
		DateOfBirth:   "1998-04-12",
		// Photo deliberately omitted.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aging := out.Dimensions["aging"]
	if aging.Source != "none" {
		t.Errorf("aging.source = %q, want none (DOB alone must not resolve W/T)", aging.Source)
	}

	foundVisionRequired, foundDOBMissing := false, false
	for _, w := range out.Warnings {
		if strings.Contains(w, "VISION_REQUIRED") && strings.Contains(w, "aging") {
			foundVisionRequired = true
		}
		if strings.Contains(w, "DOB_MISSING") {
			foundDOBMissing = true
		}
	}
	if !foundVisionRequired {
		t.Errorf("expected a VISION_REQUIRED warning naming aging, got %v", out.Warnings)
	}
	if foundDOBMissing {
		t.Errorf("did not expect DOB_MISSING — a real date_of_birth was provided, got %v", out.Warnings)
	}
}

// TestEvaluateV2_WarningsAlwaysPresent confirms warnings serializes as an
// empty array, never a bare omitted field or null, even when nothing
// triggered a warning this call.
func TestEvaluateV2_WarningsAlwaysPresent(t *testing.T) {
	svc := newSkinCharacterTestService()
	svc.WithEvaluateV2Deps(newStubSurveyProvider(t), domain.NewVisionProviderRegistry())

	out, err := svc.EvaluateV2(context.Background(), EvaluateV2Input{
		RulesetCode:   "skinverse_skin_character",
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		CustomerID:    "test-warnings-empty",
		Data:          map[string]interface{}{"Q3_PN": "yes"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Warnings == nil {
		t.Errorf("Warnings is nil, want a non-nil (possibly empty) slice so it serializes as []")
	}
}

// TestEvaluateV2_AnswerListSchemaOrderAndQuestion confirms answer_list is
// ordered by the survey's own question order (not the randomized order a Go
// map would iterate req.Data in), and each entry carries its question name.
func TestEvaluateV2_AnswerListSchemaOrderAndQuestion(t *testing.T) {
	svc := newSkinCharacterTestService()
	// A 3-question stub survey so ordering has something to get wrong.
	schema := []byte(`{
		"title": "t", "code": "test_survey",
		"pages": [{"name": "page1", "elements": [
			{"type": "radiogroup", "name": "Q1", "dimension": "sebum", "choices": [{"value": "a", "text": "A1", "score": 1}]},
			{"type": "radiogroup", "name": "Q2", "dimension": "sensitivity", "choices": [{"value": "b", "text": "B1", "score": 2}]},
			{"type": "radiogroup", "name": "Q3", "dimension": "pigmentation", "choices": [{"value": "c", "text": "C1", "score": 3}]}
		]}]
	}`)
	model, err := formdomain.ParseSurveySchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.WithEvaluateV2Deps(&stubSurveyProvider{model: model}, domain.NewVisionProviderRegistry())

	// Submitted out of schema order on purpose.
	out, err := svc.EvaluateV2(context.Background(), EvaluateV2Input{
		RulesetCode:   "skinverse_skin_character",
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		CustomerID:    "test-order",
		Data:          map[string]interface{}{"Q3": "c", "Q1": "a", "Q2": "b"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.AnswerList) != 3 {
		t.Fatalf("expected 3 answers, got %d: %+v", len(out.AnswerList), out.AnswerList)
	}
	wantOrder := []string{"Q1", "Q2", "Q3"}
	for i, want := range wantOrder {
		if out.AnswerList[i].Question != want {
			t.Errorf("answer_list[%d].question = %q, want %q (schema order)", i, out.AnswerList[i].Question, want)
		}
	}
}

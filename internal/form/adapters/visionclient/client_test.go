package visionclient

import (
	"encoding/json"
	"testing"
)

// A real Paradev staging response captured 2026-09-25 — "Pores" genuinely
// appears twice, under results.skin_level (a 1-5 severity grade) and
// results.skin_scoring (a 0-100 health score), with different values. This
// is the exact case that broke leaf-name-only flattening: pore_severity was
// silently picking up whichever "Pores" Go's randomized map iteration
// visited last, sometimes 3 (skin_level) instead of 41 (skin_scoring).
const realParadevResponseWithPoreCollision = `{
  "inference_result": {
    "baumann": {
      "baumann_type": "DSRW",
      "dimensions": {
        "oiliness": {"score_hydration": 35, "score_oiliness": 65, "skin_primary_condition": "Oily"},
        "pigmentation": {"score_darkspot": 55, "skin_primary_condition": "Non-Pigmented"},
        "sensitivity": {"score_acne": 95, "score_redness": 40, "skin_primary_condition": "Sensitive"},
        "wrinkle": {"score_elasticity": 20, "score_wrinkle": 20, "skin_primary_condition": "Wrinkle"}
      }
    },
    "results": {
      "confidence": 0.83,
      "skin_level": {"Pores": 3, "Wrinkle": 2, "Darkspot": 5},
      "skin_scoring": {"Pores": 41, "Wrinkle": 21.5, "Darkspot": 90}
    }
  }
}`

func TestFlattenNumeric_FullPathAvoidsPoreCollision(t *testing.T) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(realParadevResponseWithPoreCollision), &raw); err != nil {
		t.Fatalf("invalid fixture JSON: %v", err)
	}

	flat := flattenNumeric(raw)

	// Both "Pores" readings must survive, at distinct full-path keys.
	if v, ok := flat["inference_result.results.skin_level.Pores"]; !ok || v != 3 {
		t.Errorf("skin_level.Pores = %v, ok=%v, want 3", v, ok)
	}
	if v, ok := flat["inference_result.results.skin_scoring.Pores"]; !ok || v != 41 {
		t.Errorf("skin_scoring.Pores = %v, ok=%v, want 41", v, ok)
	}

	// A bare leaf-name key must NOT exist anymore — that was the collision.
	if _, ok := flat["Pores"]; ok {
		t.Errorf("bare leaf key \"Pores\" must not exist after switching to full-path flattening")
	}

	// score_wrinkle/score_darkspot happen to be unique in this response
	// (only "Wrinkle"/"Darkspot" without the score_ prefix are duplicated
	// under skin_level/skin_scoring), but they must still resolve at their
	// full path, not a bare leaf key.
	if v, ok := flat["inference_result.baumann.dimensions.wrinkle.score_wrinkle"]; !ok || v != 20 {
		t.Errorf("score_wrinkle = %v, ok=%v, want 20", v, ok)
	}
	if v, ok := flat["inference_result.baumann.dimensions.pigmentation.score_darkspot"]; !ok || v != 55 {
		t.Errorf("score_darkspot = %v, ok=%v, want 55", v, ok)
	}
}

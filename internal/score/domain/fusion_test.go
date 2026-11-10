package domain

import (
	"testing"
)

func TestFuseMultiModalScores(t *testing.T) {
	formSignals := map[string]float64{
		"SEBUM":       70.0,
		"SENSITIVITY": 60.0,
	}
	visionSignals := map[string]float64{
		"UV_DEFENSE":     85.0,
		"SKIN_LONGEVITY": 80.0,
		"SEBUM":          50.0,
		"SENSITIVITY":    40.0,
	}

	// Vision weight 70%, Form weight 30% for overlapping traits
	fused := FuseScores(formSignals, visionSignals, 0.70, 0.30)

	// Sebum: (50 * 0.70) + (70 * 0.30) = 35 + 21 = 56.0
	if fused["SEBUM"] != 56.0 {
		t.Errorf("expected fused SEBUM 56.0, got %f", fused["SEBUM"])
	}

	// UV_DEFENSE is vision-only: 85.0
	if fused["UV_DEFENSE"] != 85.0 {
		t.Errorf("expected fused UV_DEFENSE 85.0, got %f", fused["UV_DEFENSE"])
	}

	// SKIN_LONGEVITY is vision-only: 80.0
	if fused["SKIN_LONGEVITY"] != 80.0 {
		t.Errorf("expected fused SKIN_LONGEVITY 80.0, got %f", fused["SKIN_LONGEVITY"])
	}
}

func TestFuseScoresDynamic(t *testing.T) {
	formSignals := map[string]float64{
		"SEBUM":            80.0,
		"ACNE_LEVEL":       60.0,
		"LIFESTYLE_STRESS": 90.0,
	}
	visionSignals := map[string]float64{
		"SEBUM":         60.0,
		"ACNE_LEVEL":    40.0,
		"WRINKLE_DEPTH": 75.0,
	}

	cfg := FusionConfig{
		Strategy: "weighted_dimension",
		DefaultWeights: DimensionWeight{
			FormWeight:   0.5,
			VisionWeight: 0.5,
		},
		DimensionWeights: map[string]DimensionWeight{
			"SEBUM": {
				FormWeight:   0.3,
				VisionWeight: 0.7,
			},
			"ACNE_LEVEL": {
				FormWeight:   0.6,
				VisionWeight: 0.4,
			},
			"LIFESTYLE_STRESS": {
				FormWeight:   1.0,
				VisionWeight: 0.0,
			},
			"WRINKLE_DEPTH": {
				FormWeight:   0.0,
				VisionWeight: 1.0,
			},
		},
	}

	fused := FuseScoresDynamic(formSignals, visionSignals, cfg)

	// SEBUM: (80 * 0.3) + (60 * 0.7) = 24 + 42 = 66.0
	if fused["SEBUM"] != 66.0 {
		t.Errorf("expected SEBUM 66.0, got %f", fused["SEBUM"])
	}

	// ACNE_LEVEL: (60 * 0.6) + (40 * 0.4) = 36 + 16 = 52.0
	if fused["ACNE_LEVEL"] != 52.0 {
		t.Errorf("expected ACNE_LEVEL 52.0, got %f", fused["ACNE_LEVEL"])
	}

	// LIFESTYLE_STRESS (Form only): 90.0
	if fused["LIFESTYLE_STRESS"] != 90.0 {
		t.Errorf("expected LIFESTYLE_STRESS 90.0, got %f", fused["LIFESTYLE_STRESS"])
	}

	// WRINKLE_DEPTH (Vision only): 75.0
	if fused["WRINKLE_DEPTH"] != 75.0 {
		t.Errorf("expected WRINKLE_DEPTH 75.0, got %f", fused["WRINKLE_DEPTH"])
	}
}

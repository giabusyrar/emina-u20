package domain

import "math"

// DimensionWeight holds relative weights between form signals and vision signals for a dimension.
type DimensionWeight struct {
	FormWeight   float64 `json:"form"`
	VisionWeight float64 `json:"vision"`
}

// FusionConfig defines dynamic multi-modal scoring fusion configuration.
type FusionConfig struct {
	Strategy         string                     `json:"strategy,omitempty"` // e.g. "weighted_dimension"
	DefaultWeights   DimensionWeight            `json:"default_weights,omitempty"`
	DimensionWeights map[string]DimensionWeight `json:"dimension_weights,omitempty"`
}

// FuseScores merges subjective form questionnaire signals with objective Computer Vision signals.
func FuseScores(formSignals, visionSignals map[string]float64, visionWeight, formWeight float64) map[string]float64 {
	result := make(map[string]float64)

	// Process all traits from vision
	for key, vScore := range visionSignals {
		if fScore, exists := formSignals[key]; exists {
			// Multi-modal weighted combination
			combined := (vScore * visionWeight) + (fScore * formWeight)
			result[key] = math.Round(combined*10.0) / 10.0
		} else {
			result[key] = vScore
		}
	}

	// Any form-only traits not observed by camera (e.g. lifestyle, hydration)
	for key, fScore := range formSignals {
		if _, exists := result[key]; !exists {
			result[key] = fScore
		}
	}

	return result
}

// FuseScoresDynamic merges form and vision signals dynamically per dimension based on FusionConfig.
func FuseScoresDynamic(formSignals, visionSignals map[string]float64, cfg FusionConfig) map[string]float64 {
	result := make(map[string]float64)
	allKeys := make(map[string]bool)

	for k := range formSignals {
		allKeys[k] = true
	}
	for k := range visionSignals {
		allKeys[k] = true
	}

	defaultWeights := cfg.DefaultWeights
	if defaultWeights.FormWeight == 0 && defaultWeights.VisionWeight == 0 {
		defaultWeights = DimensionWeight{FormWeight: 0.5, VisionWeight: 0.5}
	}

	for key := range allKeys {
		fScore, hasForm := formSignals[key]
		vScore, hasVision := visionSignals[key]

		weights, hasCustom := cfg.DimensionWeights[key]
		if !hasCustom {
			weights = defaultWeights
		}

		var combined float64
		if hasForm && hasVision {
			combined = (fScore * weights.FormWeight) + (vScore * weights.VisionWeight)
		} else if hasForm {
			combined = fScore
		} else if hasVision {
			combined = vScore
		}

		// Boundary clamping guard to [0.0, 100.0]
		if combined > 100.0 {
			combined = 100.0
		}
		if combined < 0.0 {
			combined = 0.0
		}

		result[key] = math.Round(combined*10.0) / 10.0
	}

	return result
}

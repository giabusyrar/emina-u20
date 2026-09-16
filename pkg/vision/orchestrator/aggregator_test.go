package orchestrator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/vision/orchestrator"
)

func rollUpPlan() *catalog.Plan {
	return &catalog.Plan{
		Conditions: []catalog.Condition{
			{Code: "concern_acne_papule", DimensionCode: "acne", Capabilities: []string{"acne_lesion_classifier", "comedone_pore_detector"}},
			{Code: "concern_blackhead", DimensionCode: "acne", Capabilities: []string{"comedone_pore_detector"}},
			{Code: "concern_melasma", DimensionCode: "pigmentation", Capabilities: []string{"hyperpigmentation_net"}},
			{Code: "concern_redness", DimensionCode: "sensitivity", Capabilities: []string{"erythema_vascular_net"}},
		},
		Dimensions:   []string{"acne", "sensitivity"},
		Capabilities: []string{"acne_lesion_classifier", "comedone_pore_detector", "hyperpigmentation_net", "erythema_vascular_net"},
	}
}

func TestSeverityToHealth(t *testing.T) {
	cases := map[float64]float64{-5: 100, 0: 100, 30: 70, 100: 0, 140: 0}
	for severity, want := range cases {
		assert.InDelta(t, want, orchestrator.SeverityToHealth(severity), 1e-9, "severity %v", severity)
	}
}

func TestRollUpZone_AveragesOnlyScoresThatExist(t *testing.T) {
	health := map[string]float64{
		"acne_lesion_classifier": 80,
		"comedone_pore_detector": 60,
		"hyperpigmentation_net":  50,
	}

	metrics := orchestrator.RollUpZone(rollUpPlan(), health, 0.9)

	assert.InDelta(t, 70, metrics.SkinConditions["concern_acne_papule"].Score, 1e-9)
	assert.InDelta(t, 60, metrics.SkinConditions["concern_blackhead"].Score, 1e-9)
	assert.InDelta(t, 50, metrics.SkinConditions["concern_melasma"].Score, 1e-9)
	assert.Equal(t, 0.9, metrics.SkinConditions["concern_melasma"].Confidence)
	assert.NotContains(t, metrics.SkinConditions, "concern_redness")

	assert.InDelta(t, 65, metrics.Dimensions["acne"].Score, 1e-9)
	assert.Equal(t, 0.9, metrics.Dimensions["acne"].Confidence)
	assert.NotContains(t, metrics.Dimensions, "pigmentation", "pigmentation was not requested")
	assert.NotContains(t, metrics.Dimensions, "sensitivity", "sensitivity has no scored condition")
}

func TestRollUpZone_NoScoresGivesEmptyMaps(t *testing.T) {
	metrics := orchestrator.RollUpZone(rollUpPlan(), map[string]float64{}, 0.9)

	assert.NotNil(t, metrics.Dimensions)
	assert.NotNil(t, metrics.SkinConditions)
	assert.Empty(t, metrics.Dimensions)
	assert.Empty(t, metrics.SkinConditions)
}

func zoneWith(dimensions, conditions map[string]domain.MetricValue) domain.ZoneBreakdownItem {
	return domain.ZoneBreakdownItem{Metrics: domain.ZoneMetrics{Dimensions: dimensions, SkinConditions: conditions}}
}

func TestAggregateScores_MeansAcrossZonesWhereScored(t *testing.T) {
	breakdown := []domain.ZoneBreakdownItem{
		zoneWith(
			map[string]domain.MetricValue{"acne": {Score: 80, Confidence: 0.9}},
			map[string]domain.MetricValue{"concern_acne_papule": {Score: 80, Confidence: 0.9}},
		),
		zoneWith(
			map[string]domain.MetricValue{"acne": {Score: 60, Confidence: 0.7}, "sensitivity": {Score: 40, Confidence: 0.8}},
			map[string]domain.MetricValue{},
		),
		zoneWith(map[string]domain.MetricValue{}, map[string]domain.MetricValue{}),
	}

	global := orchestrator.AggregateScores(breakdown)

	assert.InDelta(t, 70, global.Dimensions["acne"].Score, 1e-9)
	assert.InDelta(t, 0.8, global.Dimensions["acne"].Confidence, 1e-9)
	assert.InDelta(t, 40, global.Dimensions["sensitivity"].Score, 1e-9)
	assert.InDelta(t, 80, global.SkinConditions["concern_acne_papule"].Score, 1e-9)
	assert.InDelta(t, 55, global.OverallSkinHealthScore, 1e-9, "mean of global dimension scores 70 and 40")
}

func TestAggregateScores_OverallFallsBackToConditions(t *testing.T) {
	breakdown := []domain.ZoneBreakdownItem{
		zoneWith(map[string]domain.MetricValue{}, map[string]domain.MetricValue{
			"concern_blackhead": {Score: 30, Confidence: 0.9},
			"concern_melasma":   {Score: 50, Confidence: 0.9},
		}),
	}

	assert.InDelta(t, 40, orchestrator.AggregateScores(breakdown).OverallSkinHealthScore, 1e-9)
}

func TestAggregateScores_NothingScoredIsZero(t *testing.T) {
	global := orchestrator.AggregateScores([]domain.ZoneBreakdownItem{zoneWith(map[string]domain.MetricValue{}, map[string]domain.MetricValue{})})

	assert.Equal(t, 0.0, global.OverallSkinHealthScore)
	assert.Empty(t, global.Dimensions)
	assert.Empty(t, global.SkinConditions)
}

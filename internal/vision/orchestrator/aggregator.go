package orchestrator

import (
	"github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
)

// SeverityToHealth converts a model severity score (100 = worst) into the
// health-oriented scale used everywhere else (100 = optimal).
func SeverityToHealth(severity float64) float64 {
	if severity < 0 {
		severity = 0
	}
	if severity > 100 {
		severity = 100
	}
	return 100 - severity
}

func emptyZoneMetrics() domain.ZoneMetrics {
	return domain.ZoneMetrics{
		Dimensions:     make(map[string]domain.MetricValue),
		SkinConditions: make(map[string]domain.MetricValue),
	}
}

// RollUpZone turns one zone's capability health scores into condition scores
// (mean of the condition's capabilities that were scored) and dimension scores
// for the plan's dimensions (mean of their scored conditions). Missing scores
// are skipped, never counted as zero.
func RollUpZone(plan *catalog.Plan, health map[string]float64, confidence float64) domain.ZoneMetrics {
	metrics := emptyZoneMetrics()
	conditionScores := make(map[string]float64, len(plan.Conditions))

	for _, condition := range plan.Conditions {
		sum, count := 0.0, 0
		for _, capability := range condition.Capabilities {
			if score, ok := health[capability]; ok {
				sum += score
				count++
			}
		}
		if count == 0 {
			continue
		}
		score := sum / float64(count)
		conditionScores[condition.Code] = score
		metrics.SkinConditions[condition.Code] = domain.MetricValue{Score: score, Confidence: confidence}
	}

	for _, dimensionCode := range plan.Dimensions {
		sum, count := 0.0, 0
		for _, condition := range plan.Conditions {
			if condition.DimensionCode != dimensionCode {
				continue
			}
			if score, ok := conditionScores[condition.Code]; ok {
				sum += score
				count++
			}
		}
		if count == 0 {
			continue
		}
		metrics.Dimensions[dimensionCode] = domain.MetricValue{Score: sum / float64(count), Confidence: confidence}
	}

	return metrics
}

type metricAccumulator struct {
	scoreSum      float64
	confidenceSum float64
	count         int
}

func (a *metricAccumulator) add(value domain.MetricValue) {
	a.scoreSum += value.Score
	a.confidenceSum += value.Confidence
	a.count++
}

func (a *metricAccumulator) mean() domain.MetricValue {
	return domain.MetricValue{
		Score:      a.scoreSum / float64(a.count),
		Confidence: a.confidenceSum / float64(a.count),
	}
}

// AggregateScores averages each dimension and condition over the zones that
// scored it. The overall score is the mean of the global dimension scores,
// falling back to the condition scores, then 0.
func AggregateScores(breakdown []domain.ZoneBreakdownItem) domain.GlobalAggregation {
	dimensionTotals := make(map[string]*metricAccumulator)
	conditionTotals := make(map[string]*metricAccumulator)

	for _, zone := range breakdown {
		for code, value := range zone.Metrics.Dimensions {
			if dimensionTotals[code] == nil {
				dimensionTotals[code] = &metricAccumulator{}
			}
			dimensionTotals[code].add(value)
		}
		for code, value := range zone.Metrics.SkinConditions {
			if conditionTotals[code] == nil {
				conditionTotals[code] = &metricAccumulator{}
			}
			conditionTotals[code].add(value)
		}
	}

	global := domain.GlobalAggregation{
		Dimensions:     make(map[string]domain.MetricValue, len(dimensionTotals)),
		SkinConditions: make(map[string]domain.MetricValue, len(conditionTotals)),
	}
	for code, totals := range dimensionTotals {
		global.Dimensions[code] = totals.mean()
	}
	for code, totals := range conditionTotals {
		global.SkinConditions[code] = totals.mean()
	}

	switch {
	case len(global.Dimensions) > 0:
		global.OverallSkinHealthScore = meanScore(global.Dimensions)
	case len(global.SkinConditions) > 0:
		global.OverallSkinHealthScore = meanScore(global.SkinConditions)
	}

	return global
}

func meanScore(values map[string]domain.MetricValue) float64 {
	sum := 0.0
	for _, value := range values {
		sum += value.Score
	}
	return sum / float64(len(values))
}

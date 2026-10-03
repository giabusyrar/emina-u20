// Package service stores each successful survey evaluation as a customer
// assessment and serves them back (per customer, and as brand history).
//
// The table, customer_assessments, keeps the columns gateway-engine created
// for it in the gateway database, where it lived before the databases were
// separated (seagull-gateway/docs/REPORT-SEAGULL-CORE-DB-SEPARATION.md).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

// Filter selects stored assessments. BrandID and ApplicationID are always
// required; CustomerID narrows to one customer. Newest first.
type Filter struct {
	BrandID       string
	ApplicationID string
	CustomerID    string
	Limit         int
}

// Store persists assessments; repository.GormStore implements it.
type Store interface {
	Save(ctx context.Context, r *entity.Record) error
	List(ctx context.Context, f Filter) ([]entity.Record, error)
}

// Recorder turns an evaluation into a Record and stores it. It implements the
// form handler's AssessmentRecorder.
type Recorder struct{ store Store }

func NewRecorder(store Store) *Recorder { return &Recorder{store: store} }

func (r *Recorder) Record(ctx context.Context, out *domain.EvaluationOutput, visionSignals map[string]float64, elapsed time.Duration) (string, error) {
	rec, err := FromEvaluation(out, visionSignals, elapsed)
	if err != nil {
		return "", err
	}
	if err := r.store.Save(ctx, rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

// FromEvaluation maps an evaluation onto a Record. Nothing is filled in that
// the evaluation did not produce: recommended_products stays NULL because a
// survey evaluation recommends no products, and skin_profile, grading tiers,
// answers and vision metrics are NULL when absent.
func FromEvaluation(out *domain.EvaluationOutput, visionSignals map[string]float64, elapsed time.Duration) (*entity.Record, error) {
	if out.CustomerID == "" {
		return nil, errors.New("evaluation has no customer_id")
	}
	dims := []byte("{}")
	if out.DimensionScores != nil {
		b, err := json.Marshal(out.DimensionScores)
		if err != nil {
			return nil, fmt.Errorf("dimension_scores: %w", err)
		}
		dims = b
	}
	rec := &entity.Record{
		CustomerID:      out.CustomerID,
		BrandID:         out.BrandID,
		ApplicationID:   out.ApplicationID,
		FormID:          out.Code,
		TotalScore:      out.TotalScore,
		SkinProfileCode: out.SkinProfile.Code,
		DimensionScores: string(dims),
		ExecutionTimeMs: elapsed.Milliseconds(),
	}
	for _, f := range []struct {
		name    string
		dst     **string
		v       any
		present bool
	}{
		{"skin_profile", &rec.SkinProfile, out.SkinProfile, out.SkinProfile.Code != ""},
		{"skin_grading_tiers", &rec.SkinGradingTiers, out.SkinGradingTiers, len(out.SkinGradingTiers) > 0},
		{"raw_answers", &rec.RawAnswers, out.AnswerList, len(out.AnswerList) > 0},
		{"vision_metrics", &rec.VisionMetrics, visionSignals, len(visionSignals) > 0},
	} {
		if !f.present {
			continue
		}
		b, err := json.Marshal(f.v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		s := string(b)
		*f.dst = &s
	}
	return rec, nil
}

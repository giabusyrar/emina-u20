package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
)

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
func FromEvaluation(out *domain.EvaluationOutput, visionSignals map[string]float64, elapsed time.Duration) (*Record, error) {
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
	rec := &Record{
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

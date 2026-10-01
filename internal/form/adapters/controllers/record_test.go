package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
)

type fakeRecorder struct {
	id  string
	err error
}

func (f fakeRecorder) Record(context.Context, *domain.EvaluationOutput, map[string]float64, time.Duration) (string, error) {
	return f.id, f.err
}

func TestRecordSetsAssessmentID(t *testing.T) {
	h := (&SurveyHandler{}).WithRecorder(fakeRecorder{id: "a-1"})
	out := &domain.EvaluationOutput{}
	h.record(context.Background(), out, nil, time.Now())
	if out.AssessmentID != "a-1" || len(out.Warnings) != 0 {
		t.Fatalf("got id %q, warnings %v", out.AssessmentID, out.Warnings)
	}
}

func TestRecordFailureWarnsInsteadOfFailing(t *testing.T) {
	h := (&SurveyHandler{}).WithRecorder(fakeRecorder{err: errors.New("db down")})
	out := &domain.EvaluationOutput{}
	h.record(context.Background(), out, nil, time.Now())
	if out.AssessmentID != "" {
		t.Fatalf("assessment id must be empty when not stored, got %q", out.AssessmentID)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != domain.WarningAssessmentNotSaved {
		t.Fatalf("want one %s warning, got %v", domain.WarningAssessmentNotSaved, out.Warnings)
	}
}

func TestRecordWithoutRecorderLeavesOutputUntouched(t *testing.T) {
	out := &domain.EvaluationOutput{}
	(&SurveyHandler{}).record(context.Background(), out, nil, time.Now())
	if out.AssessmentID != "" || out.Warnings != nil {
		t.Fatalf("output changed without a recorder: %+v", out)
	}
}

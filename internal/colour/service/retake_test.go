package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
)

func analyzeWith(t *testing.T, qc []string, enforce bool) (*AnalyzeResponse, error) {
	t.Helper()
	s, w := newSvc(t, false)
	w.qcFailed = qc
	s.cfg.Retake.Enforce = enforce
	return s.Analyze(context.Background(), []byte("x"), "a.jpg", false, true, false)
}

func TestRetakeReasonsAreQCFailedIntersectConfig(t *testing.T) {
	// wajah_kecil (face too small) is not a retake code; reasons follow config order.
	out, err := analyzeWith(t, []string{"cahaya_campuran", "wajah_kecil", "terlalu_gelap"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Retake.Required || !out.Retake.Provisional ||
		!reflect.DeepEqual(out.Retake.Reasons, []string{"terlalu_gelap", "cahaya_campuran"}) {
		t.Fatalf("retake %+v", out.Retake)
	}
	if len(out.QualityFailed) != 3 {
		t.Fatalf("qualityFailed must stay as the worker reported: %v", out.QualityFailed)
	}
}

func TestRetakeNotRequiredWithoutQCFailures(t *testing.T) {
	out, err := analyzeWith(t, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Retake.Required || out.Retake.Reasons == nil || len(out.Retake.Reasons) != 0 || !out.Retake.Provisional {
		t.Fatalf("retake %+v", out.Retake)
	}
}

func TestRetakeOtherQCCodesDoNotRequireRetake(t *testing.T) {
	out, err := analyzeWith(t, []string{"wajah_kecil"}, true)
	if err != nil || out.Retake.Required {
		t.Fatalf("err %v retake %+v", err, out.Retake)
	}
}

func TestRetakeEnforcedRefusesWithReasons(t *testing.T) {
	out, err := analyzeWith(t, []string{"terlalu_terang"}, true)
	if out != nil || !errors.Is(err, domain.ErrRetakeRequired) {
		t.Fatalf("out %v err %v", out, err)
	}
	var re *domain.RetakeRequiredError
	if !errors.As(err, &re) || !reflect.DeepEqual(re.Reasons, []string{"terlalu_terang"}) {
		t.Fatalf("err %v", err)
	}
}

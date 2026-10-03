package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/repository"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/service"
)

func TestTriggerShadeExtractionUsesConfiguredURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	svc := service.NewMatchService(repository.NewMatchRepo(nil), srv.URL)
	if err := svc.TriggerShadeExtraction(context.Background(), entity.Shade{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/extract" {
		t.Fatalf("path = %q, want /extract", gotPath)
	}
}

func TestTriggerShadeExtractionWithoutURLErrors(t *testing.T) {
	t.Setenv("TRYON_ENGINE_URL", "http://must-not-be-read")
	svc := service.NewMatchService(repository.NewMatchRepo(nil), "")
	if err := svc.TriggerShadeExtraction(context.Background(), entity.Shade{ID: "s1"}); err == nil {
		t.Fatal("want error: the service must read its URL from config, not the environment")
	}
}

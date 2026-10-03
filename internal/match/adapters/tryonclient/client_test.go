package tryonclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/tryonclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
)

func TestTriggerShadeExtractionUsesConfiguredURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := tryonclient.New(srv.URL)
	if err := c.TriggerExtraction(context.Background(), entity.Shade{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/extract" {
		t.Fatalf("path = %q, want /extract", gotPath)
	}
}

func TestTriggerShadeExtractionWithoutURLErrors(t *testing.T) {
	t.Setenv("TRYON_ENGINE_URL", "http://must-not-be-read")
	c := tryonclient.New("")
	if err := c.TriggerExtraction(context.Background(), entity.Shade{ID: "s1"}); err == nil {
		t.Fatal("want error: the client must read its URL from config, not the environment")
	}
}

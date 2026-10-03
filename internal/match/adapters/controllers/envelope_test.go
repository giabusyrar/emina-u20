package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/controllers"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/repository"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/service"
)

func TestMatchEvaluateBadJSONKeepsEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	h := controllers.NewMatchHandler(service.NewMatchService(repository.NewMatchRepo(nil), ""), nil)
	if err := h.Evaluate(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body["success"] != false {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if msg, ok := body["error"].(string); !ok || !strings.HasPrefix(msg, "Invalid JSON payload") {
		t.Fatalf("error = %v", body["error"])
	}
}

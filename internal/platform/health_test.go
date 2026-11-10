package goplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func probe(s CheckState) func(context.Context) CheckState {
	return func(context.Context) CheckState { return s }
}

func runHealth(t *testing.T, checks ...Check) (int, HealthResponse) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/health", nil), rec)
	if err := HealthHandler("svc", time.Second, checks...)(c); err != nil {
		t.Fatal(err)
	}
	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func TestHealthOKWhenRequiredUpAndOptionalDisabled(t *testing.T) {
	code, body := runHealth(t,
		Check{Name: "database", Required: true, Probe: probe(CheckUp)},
		Check{Name: "redis", Probe: probe(CheckDisabled)})
	if code != http.StatusOK || body.Status != "ok" || body.Service != "svc" ||
		body.Checks["database"] != CheckUp || body.Checks["redis"] != CheckDisabled {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestHealthOKWhenOnlyOptionalDown(t *testing.T) {
	code, body := runHealth(t,
		Check{Name: "database", Required: true, Probe: probe(CheckUp)},
		Check{Name: "redis", Probe: probe(CheckDown)})
	if code != http.StatusOK || body.Status != "ok" {
		t.Fatalf("optional down must not fail health: got %d %+v", code, body)
	}
}

func TestHealth503WhenRequiredDown(t *testing.T) {
	code, body := runHealth(t, Check{Name: "database", Required: true, Probe: probe(CheckDown)})
	if code != http.StatusServiceUnavailable || body.Status != "degraded" {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestRedisCheckStates(t *testing.T) {
	ctx := context.Background()
	if s := RedisCheck(nil, false).Probe(ctx); s != CheckDisabled {
		t.Fatalf("unconfigured: got %s", s)
	}
	if s := RedisCheck(nil, true).Probe(ctx); s != CheckDown {
		t.Fatalf("configured but not connected: got %s", s)
	}
}

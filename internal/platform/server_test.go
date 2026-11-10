package goplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestNewEchoAppliesCORSConfig(t *testing.T) {
	e := NewEcho(ServerOptions{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSAllowOrigins: []string{"*"},
		CORSAllowHeaders: []string{"Origin", "Content-Type"},
	})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Origin,Content-Type" {
		t.Fatalf("Allow-Headers = %q", got)
	}
}

func TestNewEchoSetsRequestID(t *testing.T) {
	e := NewEcho(ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CORSAllowOrigins: []string{"*"}})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Header().Get(echo.HeaderXRequestID) == "" {
		t.Fatal("want X-Request-Id on response")
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, echo.New(), "127.0.0.1:0", time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestNewEchoLogsUnregisteredRouteAsWarnNotError(t *testing.T) {
	var buf bytes.Buffer
	e := NewEcho(ServerOptions{
		Logger:           slog.New(slog.NewJSONHandler(&buf, nil)),
		CORSAllowOrigins: []string{"*"},
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-such-path", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var logged bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		if entry["msg"] != "request" {
			continue
		}
		logged = true
		if entry["level"] != "WARN" {
			t.Fatalf("level = %v, want WARN for a 404", entry["level"])
		}
	}
	if !logged {
		t.Fatal("no request log line found")
	}
}

func TestNewEchoRecoversPanicAsJSONError(t *testing.T) {
	var buf bytes.Buffer
	e := NewEcho(ServerOptions{
		Logger:           slog.New(slog.NewJSONHandler(&buf, nil)),
		CORSAllowOrigins: []string{"*"},
	})
	e.GET("/panic", func(c echo.Context) error { panic("boom") })

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Fatalf("log output missing %q: %s", "panic recovered", buf.String())
	}
}

func TestRunReturnsListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := Run(context.Background(), echo.New(), l.Addr().String(), time.Second); err == nil {
		t.Fatal("want error when the port is taken")
	}
}

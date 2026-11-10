package goplatform

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGormLoggerLogsSlowQueryAndSkipsRecordNotFound(t *testing.T) {
	var buf bytes.Buffer
	gl := NewGormLogger(slog.New(slog.NewJSONHandler(&buf, nil)), 2*time.Second)
	sql := func() (string, int64) { return "SELECT 1", 1 }

	gl.Trace(context.Background(), time.Now().Add(-3*time.Second), sql, nil)
	if !strings.Contains(buf.String(), "gorm slow query") {
		t.Fatalf("want slow-query line, got %q", buf.String())
	}

	buf.Reset()
	gl.Trace(context.Background(), time.Now(), sql, gorm.ErrRecordNotFound)
	if buf.Len() != 0 {
		t.Fatalf("record-not-found must not log, got %q", buf.String())
	}

	buf.Reset()
	gl.Trace(context.Background(), time.Now(), sql, gorm.ErrInvalidData)
	if !strings.Contains(buf.String(), "gorm query failed") {
		t.Fatalf("want error line, got %q", buf.String())
	}
}

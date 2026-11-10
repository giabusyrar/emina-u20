package goplatform

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNewLoggerWritesJSONWithService(t *testing.T) {
	var buf bytes.Buffer
	log, err := NewLogger(&buf, "svc-x", "info")
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("hidden")
	log.Info("shown", "k", "v")

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("want exactly one JSON line, got %q: %v", buf.String(), err)
	}
	if line["service"] != "svc-x" || line["msg"] != "shown" || line["k"] != "v" {
		t.Fatalf("unexpected line: %v", line)
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	if _, err := ParseLevel("loud"); err == nil {
		t.Fatal("want error for unknown level")
	}
	if _, err := ParseLevel("WARN"); err != nil {
		t.Fatalf("WARN: %v", err)
	}
}

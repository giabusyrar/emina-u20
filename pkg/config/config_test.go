package config_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/config"
)

func TestLoad_CoreEnginePortTakesPriorityOverGenericPort(t *testing.T) {
	t.Setenv("CORE_ENGINE_PORT", "8082")
	t.Setenv("PORT", "4000")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Port != "8082" {
		t.Errorf("Port = %q, want %q (CORE_ENGINE_PORT must win over generic PORT)", cfg.Port, "8082")
	}
}

func TestLoad_DefaultsPortTo8082WhenUnset(t *testing.T) {
	t.Setenv("CORE_ENGINE_PORT", "")
	t.Setenv("PORT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Port != "8082" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8082")
	}
}

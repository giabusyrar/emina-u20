package config_test

import (
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/config"
)

func TestFromEnv_FaceMeasureTimeout(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("FACE_MEASURE_TIMEOUT", "")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FaceMeasureTimeout != config.DefaultFaceMeasureTimeout || config.DefaultFaceMeasureTimeout != 20*time.Second {
		t.Errorf("FaceMeasureTimeout = %v, want 20s default", cfg.FaceMeasureTimeout)
	}

	t.Setenv("FACE_MEASURE_TIMEOUT", "45s")
	if cfg, err = config.FromEnv(); err != nil || cfg.FaceMeasureTimeout != 45*time.Second {
		t.Errorf("override: got %v, %v", cfg, err)
	}

	t.Setenv("FACE_MEASURE_TIMEOUT", "0s")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for FACE_MEASURE_TIMEOUT=0s")
	}
}

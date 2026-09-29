package config_test

import (
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/config"
)

var tunables = []string{"FORM_CACHE_TTL", "SCORE_CACHE_TTL", "MATCH_CACHE_TTL", "VISION_CATALOG_TTL",
	"SCORE_CLIENT_TIMEOUT", "PARADEV_TIMEOUT", "REFERENCE_CLIENT_TIMEOUT", "VISION_DISPATCH_TIMEOUT",
	"VISION_DAG_TIMEOUT", "SEGMENTER_TIMEOUT", "TRYON_ENGINE_URL", "CORS_ALLOW_HEADERS",
	// Shared goplatform.CommonConfig variables.
	"REDIS_URL", "LOG_LEVEL", "CORS_ALLOW_ORIGINS", "SHUTDOWN_TIMEOUT",
	"DB_SLOW_QUERY_THRESHOLD", "DEPENDENCY_PING_TIMEOUT",
	// core-engine's own port and downstream service URLs.
	"CORE_ENGINE_PORT", "PORT", "REFERENCE_SERVICE_URL", "VISION_AI_WORKER_URL",
	"PARADEV_GATEWAY_URL", "PARADEV_GATEWAY_API_KEY",
	// S3 configuration for shade textures.
	"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_REGION", "S3_PRESIGN_TTL"}

// FromEnv, not Load: Load reads the repository's .env.
func setBaseEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	for _, k := range tunables {
		t.Setenv(k, "")
	}
}

func TestFromEnv_DefaultsEqualPreStandardizationLiterals(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][2]time.Duration{
		"FormCacheTTL":           {cfg.FormCacheTTL, 15 * time.Minute},
		"ScoreCacheTTL":          {cfg.ScoreCacheTTL, 15 * time.Minute},
		"MatchCacheTTL":          {cfg.MatchCacheTTL, 15 * time.Minute},
		"VisionCatalogTTL":       {cfg.VisionCatalogTTL, time.Minute},
		"ScoreClientTimeout":     {cfg.ScoreClientTimeout, 5 * time.Second},
		"ParadevTimeout":         {cfg.ParadevTimeout, 50 * time.Second},
		"ReferenceClientTimeout": {cfg.ReferenceClientTimeout, 3 * time.Second},
		"VisionDispatchTimeout":  {cfg.VisionDispatchTimeout, 20 * time.Second},
		"VisionDAGTimeout":       {cfg.VisionDAGTimeout, 30 * time.Second},
		"SegmenterTimeout":       {cfg.SegmenterTimeout, 4 * time.Second},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %v, want %v", name, got[0], got[1])
		}
	}
	if len(cfg.CORSAllowHeaders) != 1 || cfg.CORSAllowHeaders[0] != "*" {
		t.Errorf("CORSAllowHeaders = %v, want [*]", cfg.CORSAllowHeaders)
	}
	if cfg.TryonEngineURL != "" {
		t.Errorf("TryonEngineURL = %q, want empty (no default)", cfg.TryonEngineURL)
	}
}

func TestFromEnv_Overrides(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PARADEV_TIMEOUT", "55s")
	t.Setenv("TRYON_ENGINE_URL", "http://tryon:8090")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ParadevTimeout != 55*time.Second || cfg.TryonEngineURL != "http://tryon:8090" {
		t.Fatalf("got ParadevTimeout=%v TryonEngineURL=%q", cfg.ParadevTimeout, cfg.TryonEngineURL)
	}
}

func TestFromEnv_RejectsBadDurationAndMissingDatabase(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("VISION_DAG_TIMEOUT", "0s")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for VISION_DAG_TIMEOUT=0s")
	}
	setBaseEnv(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error when DATABASE_URL is unset")
	}
}

func TestFromEnv_S3IsOptionalWithDefaults(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.S3Bucket != "" || cfg.S3AccessKeyID != "" || cfg.S3SecretAccessKey != "" || cfg.S3Endpoint != "" {
		t.Fatalf("S3 fields should be empty when unset, got %+v", cfg)
	}
	if cfg.S3Region != config.DefaultS3Region || cfg.S3PresignTTL != config.DefaultS3PresignTTL {
		t.Fatalf("S3Region=%q S3PresignTTL=%v, want defaults", cfg.S3Region, cfg.S3PresignTTL)
	}
}

func TestFromEnv_S3Values(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("S3_ENDPOINT", "https://acct.example-storage.com")
	t.Setenv("S3_BUCKET", "bkt")
	t.Setenv("S3_ACCESS_KEY_ID", "id")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	t.Setenv("S3_REGION", "ap-southeast-1")
	t.Setenv("S3_PRESIGN_TTL", "90s")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.S3Endpoint != "https://acct.example-storage.com" || cfg.S3Bucket != "bkt" ||
		cfg.S3AccessKeyID != "id" || cfg.S3SecretAccessKey != "secret" ||
		cfg.S3Region != "ap-southeast-1" || cfg.S3PresignTTL != 90*time.Second {
		t.Fatalf("got %+v", cfg)
	}
}

func TestFromEnv_S3PresignTTLInvalidIsAnError(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("S3_PRESIGN_TTL", "soon")
	if _, err := config.FromEnv(); err == nil {
		t.Fatal("want an error for S3_PRESIGN_TTL=soon")
	}
}

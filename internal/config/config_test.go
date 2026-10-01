package config_test

import (
	"os"
	"path/filepath"
	"strings"
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
	"CORE_ENGINE_PORT", "PORT", "REFERENCE_SERVICE_URL", "VISION_AI_WORKER_URL", "SKIN_WORKER_URL", "FACE_WORKER_URL",
	"MODEL_SERVER_URL", "PARADEV_GATEWAY_URL", "PARADEV_GATEWAY_API_KEY",
	// Colour engine.
	"COLOUR_WORKER_URL", "WCPA_CONFIG_PATH", "COLOUR_WORKER_TIMEOUT", "COLOUR_MAX_IMAGE_BYTES",
	"COLOUR_DEBUG_ENABLED",
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

func TestFromEnv_ColourDefaults(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ColourWorkerURL != "http://127.0.0.1:8092" || cfg.WCPAConfigPath != "apps/core-engine/config/wcpa.json" {
		t.Errorf("ColourWorkerURL=%q WCPAConfigPath=%q", cfg.ColourWorkerURL, cfg.WCPAConfigPath)
	}
	if cfg.ColourWorkerTimeout != 60*time.Second || cfg.ColourMaxImageBytes != 15<<20 || cfg.ColourDebugEnabled {
		t.Errorf("ColourWorkerTimeout=%v ColourMaxImageBytes=%d ColourDebugEnabled=%v",
			cfg.ColourWorkerTimeout, cfg.ColourMaxImageBytes, cfg.ColourDebugEnabled)
	}
	// The default config path is relative to the repository root; the file
	// must be there.
	if _, err := os.Stat(filepath.Join("..", "..", "..", "..", config.DefaultWCPAConfigPath)); err != nil {
		t.Errorf("DefaultWCPAConfigPath does not name the WCPA config: %v", err)
	}
}

func TestFromEnv_ColourOverrides(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("COLOUR_WORKER_URL", "http://worker-colour:8092")
	t.Setenv("COLOUR_WORKER_TIMEOUT", "90s")
	t.Setenv("COLOUR_MAX_IMAGE_BYTES", "1048576")
	t.Setenv("COLOUR_DEBUG_ENABLED", "true")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ColourWorkerURL != "http://worker-colour:8092" || cfg.ColourWorkerTimeout != 90*time.Second ||
		cfg.ColourMaxImageBytes != 1<<20 || !cfg.ColourDebugEnabled {
		t.Fatalf("got %+v", cfg)
	}
}

func TestFromEnv_RejectsBadColourValues(t *testing.T) {
	for key, val := range map[string]string{
		"COLOUR_MAX_IMAGE_BYTES": "15MB",
		"COLOUR_DEBUG_ENABLED":   "yes",
		"COLOUR_WORKER_TIMEOUT":  "0s",
	} {
		setBaseEnv(t)
		t.Setenv(key, val)
		if _, err := config.FromEnv(); err == nil {
			t.Errorf("want error for %s=%s", key, val)
		}
	}
	setBaseEnv(t)
	t.Setenv("COLOUR_MAX_IMAGE_BYTES", "0")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for COLOUR_MAX_IMAGE_BYTES=0")
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

func TestFromEnv_WorkerURLDefaults(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkinWorkerURL != "http://127.0.0.1:8088" || cfg.FaceWorkerURL != "http://127.0.0.1:8094" {
		t.Fatalf("SkinWorkerURL=%q FaceWorkerURL=%q", cfg.SkinWorkerURL, cfg.FaceWorkerURL)
	}
}

func TestFromEnv_WorkerURLOverrides(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SKIN_WORKER_URL", "http://worker-skin:8088")
	t.Setenv("FACE_WORKER_URL", "http://worker-face:8094")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkinWorkerURL != "http://worker-skin:8088" || cfg.FaceWorkerURL != "http://worker-face:8094" {
		t.Fatalf("SkinWorkerURL=%q FaceWorkerURL=%q", cfg.SkinWorkerURL, cfg.FaceWorkerURL)
	}
}

func TestFromEnv_RemovedVisionWorkerURLIsAnError(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("VISION_AI_WORKER_URL", "http://worker-vision:8088")
	_, err := config.FromEnv()
	if err == nil || !strings.Contains(err.Error(), "SKIN_WORKER_URL") || !strings.Contains(err.Error(), "FACE_WORKER_URL") {
		t.Fatalf("err = %v, want one naming SKIN_WORKER_URL and FACE_WORKER_URL", err)
	}
}

func TestFromEnv_ModelServerURL(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelServerURL != "http://127.0.0.1:8096" {
		t.Fatalf("ModelServerURL = %q", cfg.ModelServerURL)
	}
	t.Setenv("MODEL_SERVER_URL", "http://worker-models:8096")
	if cfg, err = config.FromEnv(); err != nil || cfg.ModelServerURL != "http://worker-models:8096" {
		t.Fatalf("override: %v %q", err, cfg.ModelServerURL)
	}
}

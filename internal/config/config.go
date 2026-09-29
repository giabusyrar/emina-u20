package config

import (
	"time"

	"github.com/experience-gateway/packages/goplatform"
)

// Defaults for core-engine's tunables. Each is the literal that sat at the
// named call site before the 2026-09-26 standardization, moved here unchanged.
const (
	DefaultFormCacheTTL     = 15 * time.Minute // cmd/server/main.go, NewSurveyCache
	DefaultScoreCacheTTL    = 15 * time.Minute // main.go, and score_cache.go's zero-value default
	DefaultMatchCacheTTL    = 15 * time.Minute // main.go, and match_cache.go's zero-value default
	DefaultVisionCatalogTTL = time.Minute      // main.go, NewCachedCatalog

	DefaultScoreClientTimeout = 5 * time.Second // was the form->score HTTP client timeout; now bounds the in-process call (form/adapters/scoreengine)
	// main.go and visionclient. Paradev's scoring call runs a synchronous
	// LLM pass; the reviewed Paradev spec asked for 45-60s.
	DefaultParadevTimeout         = 50 * time.Second
	DefaultReferenceClientTimeout = 3 * time.Second  // vision/catalog/reference_client.go
	DefaultVisionDispatchTimeout  = 20 * time.Second // vision/inference/client.go
	DefaultVisionDAGTimeout       = 30 * time.Second // vision/orchestrator/dag_engine.go
	DefaultSegmenterTimeout       = 4 * time.Second  // vision/workers/segmenter_worker.go

	// DefaultPort and the URL defaults below are the local-development
	// defaults that existed before standardization; they match the ports
	// recorded in docs/PORTS.md.
	DefaultPort                = "8082"
	DefaultReferenceServiceURL = "http://127.0.0.1:8086"
	DefaultVisionAIWorkerURL   = "http://127.0.0.1:8088"
	DefaultParadevGatewayURL   = "http://127.0.0.1:9080/paradev/scoring"
)

// DefaultFaceMeasureTimeout bounds one synchronous ai-worker face-measure
// call (facearch/measureclient): landmarker plus FACE_MEASURE_NOISE_SAMPLES
// re-measurements. Unlike the defaults above it is new, not a moved
// literal, and no latency has been measured yet: it is the same bound as
// DefaultVisionDispatchTimeout, the other synchronous ai-worker model call,
// until benchmark timings replace it.
const DefaultFaceMeasureTimeout = 20 * time.Second

// Object storage for tryon shade textures. The S3_* names are the ones
// ai-worker already reads, so one .env serves both services.
const (
	// DefaultS3Region matches ai-worker's S3_REGION default; Cloudflare R2
	// requires "auto".
	DefaultS3Region = "auto"
	// DefaultS3PresignTTL is how long a shade texture link stays valid.
	// Product policy, not a measurement: the web app fetches the shade asset
	// and loads its images immediately and never keeps the URL, so minutes
	// suffice. docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md.
	DefaultS3PresignTTL = 15 * time.Minute
)

// defaultCORSAllowHeaders is what core-engine allowed before standardization.
var defaultCORSAllowHeaders = []string{"*"}

type Config struct {
	goplatform.CommonConfig

	Port                string
	ReferenceServiceURL string
	VisionAIWorkerURL   string
	// ParadevGatewayURL/APIKey reach the Paradev Skin Analyzer vendor call
	// through our own gateway (its route already injects the vendor's own
	// secret). core-engine only needs its own gateway collection key, never
	// the vendor's, which keeps that key out of the brand-facing surface.
	ParadevGatewayURL    string
	ParadevGatewayAPIKey string
	// TryonEngineURL has no default. Shade extraction reports an error when
	// it is unset.
	TryonEngineURL string

	// S3* reach the bucket holding tryon shade textures. All optional:
	// without bucket and keys, shade asset links are unavailable (503).
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3PresignTTL      time.Duration

	FormCacheTTL     time.Duration
	ScoreCacheTTL    time.Duration
	MatchCacheTTL    time.Duration
	VisionCatalogTTL time.Duration

	ScoreClientTimeout     time.Duration
	ParadevTimeout         time.Duration
	ReferenceClientTimeout time.Duration
	VisionDispatchTimeout  time.Duration
	VisionDAGTimeout       time.Duration
	SegmenterTimeout       time.Duration
	FaceMeasureTimeout     time.Duration
}

// Load reads .env files (see goplatform.LoadDotEnv), then the environment.
func Load() (*Config, error) {
	goplatform.LoadDotEnv()
	return FromEnv()
}

// FromEnv reads the process environment only.
func FromEnv() (*Config, error) {
	common, err := goplatform.LoadCommon(goplatform.CommonDefaults{CORSAllowHeaders: defaultCORSAllowHeaders})
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		CommonConfig:         common,
		Port:                 goplatform.Port("CORE_ENGINE_PORT", "PORT", DefaultPort),
		ReferenceServiceURL:  firstOr("REFERENCE_SERVICE_URL", DefaultReferenceServiceURL),
		VisionAIWorkerURL:    firstOr("VISION_AI_WORKER_URL", DefaultVisionAIWorkerURL),
		ParadevGatewayURL:    firstOr("PARADEV_GATEWAY_URL", DefaultParadevGatewayURL),
		ParadevGatewayAPIKey: goplatform.First("PARADEV_GATEWAY_API_KEY"),
		TryonEngineURL:       goplatform.First("TRYON_ENGINE_URL"),
		S3Endpoint:           goplatform.First("S3_ENDPOINT"),
		S3Bucket:             goplatform.First("S3_BUCKET"),
		S3AccessKeyID:        goplatform.First("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:    goplatform.First("S3_SECRET_ACCESS_KEY"),
		S3Region:             firstOr("S3_REGION", DefaultS3Region),
	}

	for _, d := range []struct {
		key string
		def time.Duration
		dst *time.Duration
	}{
		{"FORM_CACHE_TTL", DefaultFormCacheTTL, &cfg.FormCacheTTL},
		{"SCORE_CACHE_TTL", DefaultScoreCacheTTL, &cfg.ScoreCacheTTL},
		{"MATCH_CACHE_TTL", DefaultMatchCacheTTL, &cfg.MatchCacheTTL},
		{"VISION_CATALOG_TTL", DefaultVisionCatalogTTL, &cfg.VisionCatalogTTL},
		{"SCORE_CLIENT_TIMEOUT", DefaultScoreClientTimeout, &cfg.ScoreClientTimeout},
		{"PARADEV_TIMEOUT", DefaultParadevTimeout, &cfg.ParadevTimeout},
		{"REFERENCE_CLIENT_TIMEOUT", DefaultReferenceClientTimeout, &cfg.ReferenceClientTimeout},
		{"VISION_DISPATCH_TIMEOUT", DefaultVisionDispatchTimeout, &cfg.VisionDispatchTimeout},
		{"VISION_DAG_TIMEOUT", DefaultVisionDAGTimeout, &cfg.VisionDAGTimeout},
		{"SEGMENTER_TIMEOUT", DefaultSegmenterTimeout, &cfg.SegmenterTimeout},
		{"FACE_MEASURE_TIMEOUT", DefaultFaceMeasureTimeout, &cfg.FaceMeasureTimeout},
		{"S3_PRESIGN_TTL", DefaultS3PresignTTL, &cfg.S3PresignTTL},
	} {
		if *d.dst, err = goplatform.Duration(d.key, d.def); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// firstOr keeps the local-development service URL defaults that existed
// before standardization. They point at this machine's ports from
// docs/PORTS.md.
func firstOr(key, def string) string {
	if v := goplatform.First(key); v != "" {
		return v
	}
	return def
}

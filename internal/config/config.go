package config

import (
	"errors"
	"fmt"
	"strconv"
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
	DefaultSkinWorkerURL       = "http://127.0.0.1:8088"
	// DefaultFaceWorkerURL is the face worker's port in docs/PORTS.md.
	DefaultFaceWorkerURL     = "http://127.0.0.1:8094"
	DefaultParadevGatewayURL = "http://127.0.0.1:9080/paradev/scoring"
	// DefaultModelServerURL is the model server's port in docs/PORTS.md.
	DefaultModelServerURL = "http://127.0.0.1:8096"
)

// DefaultFaceMeasureTimeout bounds one synchronous ai-worker face-measure
// call (facearch/measureclient): landmarker plus FACE_MEASURE_NOISE_SAMPLES
// re-measurements. Unlike the defaults above it is new, not a moved
// literal, and no latency has been measured yet: it is the same bound as
// DefaultVisionDispatchTimeout, the other synchronous ai-worker model call,
// until benchmark timings replace it.
const DefaultFaceMeasureTimeout = 20 * time.Second

// Defaults for the colour engine (internal/colour): WCPA personal colour
// analysis and photo try-on, measured and rendered by the ai-worker colour
// profile. Carried over unchanged from the feat/tryon-engine branch, except
// the worker URL, which follows that profile's port in docs/PORTS.md.
const (
	DefaultColourWorkerURL = "http://127.0.0.1:8092"
	// Relative to the repository root, where the npm scripts run core-engine;
	// the Docker image sets WCPA_CONFIG_PATH instead (Dockerfile).
	DefaultWCPAConfigPath = "apps/core-engine/config/wcpa.json"
	// Measurement and render run MediaPipe and numpy on the worker's CPU: a few
	// seconds per photo, several times that on a slow laptop.
	DefaultColourWorkerTimeout = 60 * time.Second
	// Upload cap for /analyze and /tryon; a phone photo is a few MB.
	DefaultColourMaxImageBytes int64 = 15 << 20
)

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
	// SkinWorkerURL is apps/workers/skin (segment-and-pose);
	// ModelServerURL is apps/workers/models (model registry, capability dispatch);
	// FaceWorkerURL is apps/workers/face (face-measure, face architecture).
	SkinWorkerURL  string
	ModelServerURL string
	FaceWorkerURL  string
	// ParadevGatewayURL/APIKey reach the Paradev Skin Analyzer vendor call
	// through our own gateway (its route already injects the vendor's own
	// secret). core-engine only needs its own gateway collection key, never
	// the vendor's, which keeps that key out of the brand-facing surface.
	ParadevGatewayURL    string
	ParadevGatewayAPIKey string
	// TryonEngineURL has no default. Shade extraction reports an error when
	// it is unset.
	TryonEngineURL string

	// Colour engine. ColourWorkerURL is the ai-worker colour profile;
	// WCPAConfigPath holds the WCPA rules (thresholds, centres, labels);
	// ColourDebugEnabled lets /analyze?debug=1 return the raw colorimetry.
	ColourWorkerURL     string
	WCPAConfigPath      string
	ColourWorkerTimeout time.Duration
	ColourMaxImageBytes int64
	ColourDebugEnabled  bool
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
	if goplatform.First("VISION_AI_WORKER_URL") != "" {
		return nil, errors.New("VISION_AI_WORKER_URL was removed: set SKIN_WORKER_URL (segment-and-pose, model dispatch) and FACE_WORKER_URL (face-measure) instead")
	}
	common, err := goplatform.LoadCommon(goplatform.CommonDefaults{CORSAllowHeaders: defaultCORSAllowHeaders})
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		CommonConfig:         common,
		Port:                 goplatform.Port("CORE_ENGINE_PORT", "PORT", DefaultPort),
		ReferenceServiceURL:  firstOr("REFERENCE_SERVICE_URL", DefaultReferenceServiceURL),
		SkinWorkerURL:        firstOr("SKIN_WORKER_URL", DefaultSkinWorkerURL),
		ModelServerURL:       firstOr("MODEL_SERVER_URL", DefaultModelServerURL),
		FaceWorkerURL:        firstOr("FACE_WORKER_URL", DefaultFaceWorkerURL),
		ParadevGatewayURL:    firstOr("PARADEV_GATEWAY_URL", DefaultParadevGatewayURL),
		ParadevGatewayAPIKey: goplatform.First("PARADEV_GATEWAY_API_KEY"),
		TryonEngineURL:       goplatform.First("TRYON_ENGINE_URL"),
		ColourWorkerURL:      firstOr("COLOUR_WORKER_URL", DefaultColourWorkerURL),
		WCPAConfigPath:       firstOr("WCPA_CONFIG_PATH", DefaultWCPAConfigPath),
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
		{"COLOUR_WORKER_TIMEOUT", DefaultColourWorkerTimeout, &cfg.ColourWorkerTimeout},
		{"S3_PRESIGN_TTL", DefaultS3PresignTTL, &cfg.S3PresignTTL},
	} {
		if *d.dst, err = goplatform.Duration(d.key, d.def); err != nil {
			return nil, err
		}
	}
	if cfg.ColourMaxImageBytes, err = positiveInt64("COLOUR_MAX_IMAGE_BYTES", DefaultColourMaxImageBytes); err != nil {
		return nil, err
	}
	if cfg.ColourDebugEnabled, err = boolean("COLOUR_DEBUG_ENABLED", false); err != nil {
		return nil, err
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

// positiveInt64 returns key parsed as a positive integer, or def when unset.
// Like goplatform.Duration, a malformed or non-positive value is an error,
// never a silent fallback: an upload cap of 0 would refuse every photo.
func positiveInt64(key string, def int64) (int64, error) {
	raw := goplatform.First(key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s: must be positive, got %q", key, raw)
	}
	return n, nil
}

// boolean returns key parsed by strconv.ParseBool (true/false, 1/0, t/f), or
// def when unset. A malformed value is an error, not false.
func boolean(key string, def bool) (bool, error) {
	raw := goplatform.First(key)
	if raw == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}

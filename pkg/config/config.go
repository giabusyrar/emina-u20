package config

import (
	"github.com/experience-gateway/packages/goenv"
)

type Config struct {
	Port                string
	DatabaseURL         string
	RedisURL            string
	ReferenceServiceURL string
	VisionAIWorkerURL   string
	// ParadevGatewayURL/APIKey reach the Paradev Skin Analyzer vendor call
	// through our own gateway (its route already injects the vendor's own
	// secret) — core-engine only needs its own gateway collection key, never
	// the vendor's, keeping that key out of the brand-facing surface.
	ParadevGatewayURL    string
	ParadevGatewayAPIKey string
}

func Load() (*Config, error) {
	goenv.LoadDotEnv()

	port := goenv.Port("CORE_ENGINE_PORT", "PORT", "8082")

	dbURL := goenv.First("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@127.0.0.1:5432/gateway_db?sslmode=disable"
	}

	redisURL := goenv.First("REDIS_URL")

	referenceServiceURL := goenv.First("REFERENCE_SERVICE_URL")
	if referenceServiceURL == "" {
		referenceServiceURL = "http://127.0.0.1:8086"
	}

	visionAIWorkerURL := goenv.First("VISION_AI_WORKER_URL")
	if visionAIWorkerURL == "" {
		visionAIWorkerURL = "http://127.0.0.1:8088"
	}

	paradevGatewayURL := goenv.First("PARADEV_GATEWAY_URL")
	if paradevGatewayURL == "" {
		paradevGatewayURL = "http://127.0.0.1:9080/paradev/scoring"
	}
	paradevGatewayAPIKey := goenv.First("PARADEV_GATEWAY_API_KEY")

	return &Config{
		Port:                 port,
		DatabaseURL:          dbURL,
		RedisURL:             redisURL,
		ReferenceServiceURL:  referenceServiceURL,
		VisionAIWorkerURL:    visionAIWorkerURL,
		ParadevGatewayURL:    paradevGatewayURL,
		ParadevGatewayAPIKey: paradevGatewayAPIKey,
	}, nil
}

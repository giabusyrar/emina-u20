// Package goplatform is the shared bootstrap for the Go services in
// Seagull-core: environment and .env resolution, logging, Postgres and Redis,
// the health endpoint, the HTTP server and the error envelope.
//
// The environment helpers keep the port-priority ordering fixed after three
// separate local-dev incidents where a service checked the generic PORT
// before its own named variable.
package goplatform

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LoadDotEnv searches for .env and .env.local files starting from the current
// working directory and walking upwards up to 6 levels (or filesystem root),
// loading any variable not already present in the process environment.
func LoadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}

	current := dir
	for i := 0; i < 6; i++ {
		loadEnvFile(filepath.Join(current, ".env.local"))
		loadEnvFile(filepath.Join(current, ".env"))
		loadEnvFile(filepath.Join(current, "apps", "web", ".env.local"))
		loadEnvFile(filepath.Join(current, "apps", "web", ".env"))
		loadEnvFile(filepath.Join(current, "frontend", ".env.local"))
		loadEnvFile(filepath.Join(current, "frontend", ".env"))

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// First returns the first non-empty, quote-trimmed value among keys, or "".
func First(keys ...string) string {
	for _, key := range keys {
		val := strings.Trim(strings.TrimSpace(os.Getenv(key)), "\"'")
		if val != "" {
			return val
		}
	}
	return ""
}

// Int returns key parsed as an int, or defaultVal if unset or unparseable.
func Int(key string, defaultVal int) int {
	valStr := strings.TrimSpace(os.Getenv(key))
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

// ExtractPort parses a URL string (e.g. "http://localhost:8081" or "http://127.0.0.1:8081/api") and returns its port, or "".
func ExtractPort(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err == nil && u.Port() != "" {
		return u.Port()
	}
	return ""
}

// Port resolves a service's listen port with the canonical priority:
//  1. serviceVar       — explicit, service-specific port (e.g. "CORE_ENGINE_PORT")
//  2. matching URL var — port extracted from matching service URL (e.g. CORE_ENGINE_URL, NEXT_PUBLIC_GATEWAY_ENGINE_URL -> "8081")
//  3. platformVar      — platform-injected (e.g. "PORT"), meaningful only when
//     this process is the sole occupant of its container/deployment
//  4. fallback         — hardcoded default matching docker-compose.yml
func Port(serviceVar, platformVar, fallback string) string {
	if v := First(serviceVar); v != "" {
		return v
	}
	urlVar := strings.TrimSuffix(serviceVar, "_PORT") + "_URL"
	candidates := []string{urlVar, "NEXT_PUBLIC_" + urlVar}
	if serviceVar == "GATEWAY_PORT" || serviceVar == "GATEWAY_PROXY_PORT" {
		candidates = append(candidates, "GATEWAY_PROXY_URL", "NEXT_PUBLIC_GATEWAY_PROXY_URL")
	}
	for _, cand := range candidates {
		if urlVal := First(cand); urlVal != "" {
			if p := ExtractPort(urlVal); p != "" {
				return p
			}
		}
	}
	if v := First(platformVar); v != "" {
		return v
	}
	return fallback
}

// Duration returns key parsed by time.ParseDuration, or def when unset.
// A malformed or non-positive value is an error, never a silent fallback:
// every duration configured here is a timeout or a TTL, and zero means
// "no limit" to net/http and "never expire" to Redis.
func Duration(key string, def time.Duration) (time.Duration, error) {
	raw := First(key)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: must be positive, got %q", key, raw)
	}
	return d, nil
}

// List returns key split on commas, trimmed, with empty items dropped, or
// def when unset.
func List(key string, def []string) []string {
	raw := First(key)
	if raw == "" {
		return def
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

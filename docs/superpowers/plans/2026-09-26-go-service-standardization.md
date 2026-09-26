# Go Service Standardization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Assemble `core-engine` and `reference-service` the same way, from one
shared bootstrap package. Move their packages under `internal/`, and move every
unflagged literal into config. Do not change the stack or any wire contract.

**Architecture:** `packages/goenv` is renamed `packages/goplatform` and grows
the shared pieces:
- slog logging, including a GORM logger adapter;
- Postgres and Redis open with fail-fast semantics;
- the `/health` handler;
- the Echo server with graceful shutdown;
- the error-envelope helpers.

Each service's `cmd/server/main.go` becomes config → `goplatform.Bootstrap` →
wire the engines → `goplatform.Run`. The engines move from `pkg/` to
`internal/` with no logic changes.

**Tech Stack:** Go 1.25.0, Echo v4.13.4, GORM v1.25.12 with
`gorm.io/driver/postgres` v1.5.11, go-redis v9.7.1, and the standard library
`log/slog`.

**Spec:** `docs/superpowers/specs/2026-09-26-go-service-standardization-design.md`

## Global Constraints

- Go `1.25.0` in every `go.mod` and in `go.work`.
- Module paths stay `github.com/experience-gateway/...`. The new module is
  `github.com/experience-gateway/packages/goplatform`.
- **No wire contract changes:**
  - ports, route prefixes and every route alias stay;
  - `AutoMigrate` lists and GORM models stay;
  - error bodies only **gain** fields (`success`, `error_code`), never lose or
    rename one;
  - the vision controller's error bodies are not touched.
- Every configurable default is a named constant with a comment citing where the
  literal came from. Defaults equal today's values exactly.
- Configured durations must be positive. A malformed or non-positive value
  fails startup.
- `DATABASE_URL` is required. There is no credentialed default DSN anywhere.
- Redis stays optional. An unreachable Redis at startup logs a warning and runs
  without cache.
- **Do not run `git push`.** Commit locally only (`AGENTS.md`).
- **Lanes (`docs/LANES.md`):**
  - Task 5 needs the `reference` lane.
  - Tasks 6–9 need all four core lanes (`core-form`, `core-match`,
    `core-score`, `core-vision`).
  - Task 10 needs `docs` for `README.md`, `AGENTS.md`, `docs/SPLIT.md`,
    `RAILWAY.md` and the lane map in `docs/LANES.md`.
  - Tasks 1–4 touch `packages/`, `go.work`, the two `go.mod` files, the two
    Dockerfiles and the two `pkg/config` packages, which no lane maps. Claim
    them explicitly by path.
  - Claim before editing, and release when done.
- **Build checks.** Run all commands from the repository root. Go resolves the
  modules through `go.work`, **but Docker does not**: each Dockerfile builds one
  module standalone, with only its `replace` to `packages/goplatform`. Docker is
  not installed on the development machine, so each task reproduces the
  Dockerfile build with the workspace off:
  ```bash
  (cd apps/core-engine && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/core-engine" ./cmd/server)
  ```
  A `missing go.sum entry` failure here would also fail on Railway. Fix it with
  `(cd apps/<app> && GOWORK=off go mod tidy)`. The user verifies the actual image
  builds.
- **Baseline.** Before Task 1, `go test ./apps/core-engine/... ./apps/reference-service/... ./packages/goenv/...`
  passes on commit `500b696`.

## File map

| Path | Responsibility |
|---|---|
| `packages/goplatform/env.go` | `.env` loading and typed env reads (today's goenv, plus `Duration`, `List`) |
| `packages/goplatform/common.go` | `CommonConfig`, loaded identically by both services |
| `packages/goplatform/log.go` | `ParseLevel`, `NewLogger` |
| `packages/goplatform/gormlog.go` | GORM `logger.Interface` over slog |
| `packages/goplatform/db.go` | `OpenPostgres` |
| `packages/goplatform/redis.go` | `OpenRedis` |
| `packages/goplatform/health.go` | `Check`, `HealthHandler`, `DatabaseCheck`, `RedisCheck` |
| `packages/goplatform/bootstrap.go` | `Bootstrap` → `*Deps` (logger, DB, Redis, health) |
| `packages/goplatform/server.go` | `NewEcho`, `Run` |
| `packages/goplatform/httperr.go` | `JSONError`, `JSONErrorCode`, `JSONValidationError` |
| `apps/reference-service/internal/config/` | reference-service config |
| `apps/reference-service/internal/reference/{cache,controllers,entity,repository}/` | was `pkg/adapters/{cache,controllers}`, `pkg/entity`, `pkg/repository` |
| `apps/core-engine/internal/{config,form,score,match,vision}/` | was `pkg/...` |

---

### Task 1: Rename `packages/goenv` → `packages/goplatform`

**Files:**
- Move: `packages/goenv/` → `packages/goplatform/`. `goenv.go` becomes `env.go`, and `goenv_test.go` becomes `env_test.go`.
- Modify: `packages/goplatform/go.mod`, `go.work`
- Modify: `apps/core-engine/go.mod`, `apps/reference-service/go.mod`
- Modify: `apps/core-engine/Dockerfile:10`, `apps/reference-service/Dockerfile:10`
- Modify: `apps/core-engine/pkg/config/config.go`, `apps/reference-service/pkg/config/config.go`

**Interfaces:**
- Produces: package `goplatform` exporting today's goenv API unchanged: `LoadDotEnv()`, `First(keys ...string) string`, `Int(key string, def int) int`, `ExtractPort(rawURL string) string`, `Port(serviceVar, platformVar, fallback string) string`.

- [ ] **Step 1: Move the files**

```bash
git mv packages/goenv packages/goplatform
git mv packages/goplatform/goenv.go packages/goplatform/env.go
git mv packages/goplatform/goenv_test.go packages/goplatform/env_test.go
```

- [ ] **Step 2: Rename the module and the package clause**

In `packages/goplatform/go.mod`, change line 1 to:

```
module github.com/experience-gateway/packages/goplatform
```

In `packages/goplatform/env.go`, replace the package comment and clause (lines 1–5) with:

```go
// Package goplatform is the shared bootstrap for the Go services in
// Seagull-core: environment and .env resolution, logging, Postgres and Redis,
// the health endpoint, the HTTP server and the error envelope.
//
// The environment helpers keep the port-priority ordering fixed after three
// separate local-dev incidents where a service checked the generic PORT
// before its own named variable.
package goplatform
```

In `packages/goplatform/env_test.go`, change `package goenv` to `package goplatform`.

- [ ] **Step 3: Repoint the workspace, the modules, the Dockerfiles and the imports**

```bash
sed -i 's#./packages/goenv#./packages/goplatform#' go.work
for m in apps/core-engine apps/reference-service; do
  sed -i 's#packages/goenv#packages/goplatform#g' $m/go.mod $m/Dockerfile
  sed -i -e 's#"github.com/experience-gateway/packages/goenv"#"github.com/experience-gateway/packages/goplatform"#' \
         -e 's#goenv\.#goplatform.#g' $m/pkg/config/config.go
done
grep -rn "goenv" --include='*.go' --include='go.mod' --include='go.work' --include='Dockerfile' . \
  | grep -v '^./.claude/' || echo "no goenv references left"
```

Expected: `no goenv references left`.

- [ ] **Step 4: Verify**

```bash
go build ./apps/core-engine/... ./apps/reference-service/... ./packages/goplatform/...
go test ./apps/core-engine/... ./apps/reference-service/... ./packages/goplatform/...
(cd apps/core-engine && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/core-engine" ./cmd/server)
(cd apps/reference-service && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/reference-service" ./cmd/server)
```

Expected: all pass. The package list is the same as the baseline, with `packages/goplatform` in place of `packages/goenv`.

- [ ] **Step 5: Commit**

```bash
git add -A packages/ go.work apps/core-engine/go.mod apps/reference-service/go.mod \
  apps/core-engine/Dockerfile apps/reference-service/Dockerfile \
  apps/core-engine/pkg/config/config.go apps/reference-service/pkg/config/config.go
git commit -m "refactor: rename packages/goenv to packages/goplatform

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Config primitives and the logger

**Files:**
- Modify: `packages/goplatform/env.go` (append `Duration`, `List`)
- Create: `packages/goplatform/log.go`, `packages/goplatform/common.go`
- Test: `packages/goplatform/env_test.go` (append), `packages/goplatform/log_test.go`, `packages/goplatform/common_test.go`

**Interfaces:**
- Produces:
  - `func Duration(key string, def time.Duration) (time.Duration, error)`
  - `func List(key string, def []string) []string`
  - `func ParseLevel(s string) (slog.Level, error)`
  - `func NewLogger(w io.Writer, service, level string) (*slog.Logger, error)`
  - `type CommonConfig struct { DatabaseURL, RedisURL, LogLevel string; CORSAllowOrigins, CORSAllowHeaders []string; ShutdownTimeout, DBSlowQueryThreshold, DependencyPingTimeout time.Duration }`
  - `type CommonDefaults struct { CORSAllowHeaders []string }`
  - `func LoadCommon(d CommonDefaults) (CommonConfig, error)`. It reads the process environment only. It does **not** call `LoadDotEnv`.
  - Constants `DefaultLogLevel`, `DefaultShutdownTimeout`, `DefaultDBSlowQueryThreshold`, `DefaultDependencyPingTimeout`, and the var `DefaultCORSAllowOrigins`.

- [ ] **Step 1: Write the failing tests**

Append to `packages/goplatform/env_test.go`. Add `"time"` and `"reflect"` to its imports:

```go
func TestDuration(t *testing.T) {
	t.Setenv("GP_TEST_DUR", "")
	if d, err := Duration("GP_TEST_DUR", 3*time.Second); err != nil || d != 3*time.Second {
		t.Fatalf("unset: got %v, %v; want 3s, nil", d, err)
	}

	t.Setenv("GP_TEST_DUR", "250ms")
	if d, err := Duration("GP_TEST_DUR", time.Second); err != nil || d != 250*time.Millisecond {
		t.Fatalf("set: got %v, %v; want 250ms, nil", d, err)
	}

	for _, bad := range []string{"soon", "0s", "-1s"} {
		t.Setenv("GP_TEST_DUR", bad)
		if _, err := Duration("GP_TEST_DUR", time.Second); err == nil {
			t.Errorf("%q: want error, got nil", bad)
		}
	}
}

func TestList(t *testing.T) {
	t.Setenv("GP_TEST_LIST", "")
	if got := List("GP_TEST_LIST", []string{"*"}); !reflect.DeepEqual(got, []string{"*"}) {
		t.Fatalf("unset: got %v, want [*]", got)
	}
	t.Setenv("GP_TEST_LIST", " a, b ,,c ")
	if got := List("GP_TEST_LIST", nil); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("set: got %v, want [a b c]", got)
	}
}
```

Create `packages/goplatform/log_test.go`:

```go
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
```

Create `packages/goplatform/common_test.go`:

```go
package goplatform

import (
	"reflect"
	"testing"
	"time"
)

func clearCommonEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "LOG_LEVEL", "CORS_ALLOW_ORIGINS",
		"CORS_ALLOW_HEADERS", "SHUTDOWN_TIMEOUT", "DB_SLOW_QUERY_THRESHOLD", "DEPENDENCY_PING_TIMEOUT"} {
		t.Setenv(k, "")
	}
}

func TestLoadCommonRequiresDatabaseURL(t *testing.T) {
	clearCommonEnv(t)
	if _, err := LoadCommon(CommonDefaults{}); err == nil {
		t.Fatal("want error when DATABASE_URL is unset")
	}
}

func TestLoadCommonDefaults(t *testing.T) {
	clearCommonEnv(t)
	t.Setenv("DATABASE_URL", "postgres://test")

	c, err := LoadCommon(CommonDefaults{CORSAllowHeaders: []string{"Origin"}})
	if err != nil {
		t.Fatal(err)
	}
	want := CommonConfig{
		DatabaseURL:           "postgres://test",
		LogLevel:              "info",
		CORSAllowOrigins:      []string{"*"},
		CORSAllowHeaders:      []string{"Origin"},
		ShutdownTimeout:       10 * time.Second,
		DBSlowQueryThreshold:  2 * time.Second,
		DependencyPingTimeout: 3 * time.Second,
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
}

func TestLoadCommonRejectsBadValues(t *testing.T) {
	for key, val := range map[string]string{"LOG_LEVEL": "loud", "SHUTDOWN_TIMEOUT": "0s"} {
		clearCommonEnv(t)
		t.Setenv("DATABASE_URL", "postgres://test")
		t.Setenv(key, val)
		if _, err := LoadCommon(CommonDefaults{}); err == nil {
			t.Errorf("%s=%s: want error", key, val)
		}
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./packages/goplatform/...`
Expected: FAIL to compile. `Duration`, `List`, `NewLogger`, `ParseLevel`, `LoadCommon`, `CommonConfig` and `CommonDefaults` are undefined.

- [ ] **Step 3: Implement**

Append to `packages/goplatform/env.go`. Add `"fmt"` and `"time"` to its imports:

```go
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
```

Create `packages/goplatform/log.go`:

```go
package goplatform

import (
	"fmt"
	"io"
	"log/slog"
)

// ParseLevel accepts debug, info, warn or error, in any case.
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return l, nil
}

// NewLogger returns a JSON logger at level whose every line carries service.
func NewLogger(w io.Writer, service, level string) (*slog.Logger, error) {
	l, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l})).With("service", service), nil
}
```

Create `packages/goplatform/common.go`:

```go
package goplatform

import (
	"errors"
	"time"
)

// Defaults shared by both Go services. Each is the literal that sat in
// cmd/server/main.go before the 2026-09-26 standardization, moved here
// unchanged so that no deployment has to set a new variable.
const (
	DefaultLogLevel = "info"
	// reference-service main.go, e.Shutdown timeout.
	DefaultShutdownTimeout = 10 * time.Second
	// Both main.go files, gormlogger.Config.SlowThreshold.
	DefaultDBSlowQueryThreshold = 2 * time.Second
	// Both main.go files, Redis startup ping. Now also bounds the Postgres
	// startup ping and each /health probe.
	DefaultDependencyPingTimeout = 3 * time.Second
)

// DefaultCORSAllowOrigins is what both services allowed before standardization.
var DefaultCORSAllowOrigins = []string{"*"}

// CommonConfig is the configuration every Go service here loads the same way.
type CommonConfig struct {
	DatabaseURL           string
	RedisURL              string
	LogLevel              string
	CORSAllowOrigins      []string
	CORSAllowHeaders      []string
	ShutdownTimeout       time.Duration
	DBSlowQueryThreshold  time.Duration
	DependencyPingTimeout time.Duration
}

// CommonDefaults are the per-service defaults for CommonConfig fields whose
// pre-standardization value differed between services.
type CommonDefaults struct {
	CORSAllowHeaders []string
}

// LoadCommon reads CommonConfig from the process environment. It does not
// load .env files; each service's config.Load does that first.
func LoadCommon(d CommonDefaults) (CommonConfig, error) {
	c := CommonConfig{
		DatabaseURL:      First("DATABASE_URL"),
		RedisURL:         First("REDIS_URL"),
		LogLevel:         First("LOG_LEVEL"),
		CORSAllowOrigins: List("CORS_ALLOW_ORIGINS", DefaultCORSAllowOrigins),
		CORSAllowHeaders: List("CORS_ALLOW_HEADERS", d.CORSAllowHeaders),
	}
	if c.DatabaseURL == "" {
		return CommonConfig{}, errors.New("DATABASE_URL is required")
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
	if _, err := ParseLevel(c.LogLevel); err != nil {
		return CommonConfig{}, err
	}

	var err error
	for _, d := range []struct {
		key string
		def time.Duration
		dst *time.Duration
	}{
		{"SHUTDOWN_TIMEOUT", DefaultShutdownTimeout, &c.ShutdownTimeout},
		{"DB_SLOW_QUERY_THRESHOLD", DefaultDBSlowQueryThreshold, &c.DBSlowQueryThreshold},
		{"DEPENDENCY_PING_TIMEOUT", DefaultDependencyPingTimeout, &c.DependencyPingTimeout},
	} {
		if *d.dst, err = Duration(d.key, d.def); err != nil {
			return CommonConfig{}, err
		}
	}
	return c, nil
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./packages/goplatform/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add packages/goplatform
git commit -m "feat(goplatform): typed env reads, CommonConfig and slog logger

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Postgres, Redis, health and `Bootstrap`

**Files:**
- Modify: `packages/goplatform/go.mod` (add dependencies)
- Create: `packages/goplatform/gormlog.go`, `db.go`, `redis.go`, `health.go`, `bootstrap.go`
- Test: `packages/goplatform/gormlog_test.go`, `db_test.go`, `health_test.go`

**Interfaces:**
- Consumes: `NewLogger`, `CommonConfig` (Task 2).
- Produces:
  - `func NewGormLogger(log *slog.Logger, slow time.Duration) gormlogger.Interface`
  - `type DBOptions struct { Logger *slog.Logger; SlowQueryThreshold, PingTimeout time.Duration }`
  - `func OpenPostgres(ctx context.Context, url string, o DBOptions) (*gorm.DB, error)`
  - `func OpenRedis(ctx context.Context, url string, pingTimeout time.Duration) (*redis.Client, error)`. It returns `(nil, nil)` when the URL is empty.
  - `type CheckState string` with constants `CheckUp = "up"`, `CheckDown = "down"`, `CheckDisabled = "disabled"`
  - `type Check struct { Name string; Required bool; Probe func(context.Context) CheckState }`
  - `type HealthResponse struct { Status string; Service string; Checks map[string]CheckState }`, with JSON tags `status`, `service`, `checks`
  - `func HealthHandler(service string, timeout time.Duration, checks ...Check) echo.HandlerFunc`
  - `func DatabaseCheck(db *gorm.DB) Check` (required)
  - `func RedisCheck(rdb *redis.Client, configured bool) Check` (optional)
  - `type Deps struct { Service string; Log *slog.Logger; DB *gorm.DB; Redis *redis.Client }`, with unexported fields `redisConfigured bool` and `pingTimeout time.Duration`
  - `func Bootstrap(ctx context.Context, service string, c CommonConfig) (*Deps, error)`
  - `func (d *Deps) HealthHandler() echo.HandlerFunc`
  - `func (d *Deps) Close()`

- [ ] **Step 1: Add the dependencies**

```bash
cd packages/goplatform
go get github.com/labstack/echo/v4@v4.13.4 github.com/redis/go-redis/v9@v9.7.1 \
  gorm.io/gorm@v1.25.12 gorm.io/driver/postgres@v1.5.11
cd ../..
```

Expected: `go.mod` gains those four requirements, and a `go.sum` appears. After Step 3, run `go mod tidy` in `packages/goplatform` to mark indirects correctly.

- [ ] **Step 2: Write the failing tests**

Create `packages/goplatform/gormlog_test.go`:

```go
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
```

Create `packages/goplatform/db_test.go`:

```go
package goplatform

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestOpenPostgresRequiresURL(t *testing.T) {
	if _, err := OpenPostgres(context.Background(), "", DBOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PingTimeout: time.Second}); err == nil {
		t.Fatal("want error for empty URL")
	}
}

func TestOpenPostgresFailsWhenUnreachable(t *testing.T) {
	// Port 1 on loopback refuses connections immediately.
	_, err := OpenPostgres(context.Background(),
		"postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		DBOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PingTimeout: 2 * time.Second})
	if err == nil {
		t.Fatal("want error when Postgres is unreachable")
	}
}

func TestOpenRedis(t *testing.T) {
	if c, err := OpenRedis(context.Background(), "", time.Second); c != nil || err != nil {
		t.Fatalf("empty URL: got %v, %v; want nil, nil", c, err)
	}
	if c, err := OpenRedis(context.Background(), "redis://127.0.0.1:1", time.Second); c != nil || err == nil {
		t.Fatalf("unreachable: got %v, %v; want nil, error", c, err)
	}
}
```

Create `packages/goplatform/health_test.go`:

```go
package goplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func probe(s CheckState) func(context.Context) CheckState {
	return func(context.Context) CheckState { return s }
}

func runHealth(t *testing.T, checks ...Check) (int, HealthResponse) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/health", nil), rec)
	if err := HealthHandler("svc", time.Second, checks...)(c); err != nil {
		t.Fatal(err)
	}
	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func TestHealthOKWhenRequiredUpAndOptionalDisabled(t *testing.T) {
	code, body := runHealth(t,
		Check{Name: "database", Required: true, Probe: probe(CheckUp)},
		Check{Name: "redis", Probe: probe(CheckDisabled)})
	if code != http.StatusOK || body.Status != "ok" || body.Service != "svc" ||
		body.Checks["database"] != CheckUp || body.Checks["redis"] != CheckDisabled {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestHealthOKWhenOnlyOptionalDown(t *testing.T) {
	code, body := runHealth(t,
		Check{Name: "database", Required: true, Probe: probe(CheckUp)},
		Check{Name: "redis", Probe: probe(CheckDown)})
	if code != http.StatusOK || body.Status != "ok" {
		t.Fatalf("optional down must not fail health: got %d %+v", code, body)
	}
}

func TestHealth503WhenRequiredDown(t *testing.T) {
	code, body := runHealth(t, Check{Name: "database", Required: true, Probe: probe(CheckDown)})
	if code != http.StatusServiceUnavailable || body.Status != "degraded" {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestRedisCheckStates(t *testing.T) {
	ctx := context.Background()
	if s := RedisCheck(nil, false).Probe(ctx); s != CheckDisabled {
		t.Fatalf("unconfigured: got %s", s)
	}
	if s := RedisCheck(nil, true).Probe(ctx); s != CheckDown {
		t.Fatalf("configured but not connected: got %s", s)
	}
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run: `go test ./packages/goplatform/...`
Expected: FAIL to compile, because `NewGormLogger`, `OpenPostgres`, `DBOptions`, `OpenRedis`, `HealthHandler`, `Check`, `CheckState` and `RedisCheck` are undefined.

- [ ] **Step 4: Implement**

Create `packages/goplatform/gormlog.go`:

```go
package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// gormSlog routes GORM's logging through slog. It keeps the behaviour both
// services configured before: warn level, slow queries above a threshold
// logged as warnings, and record-not-found not treated as an error.
type gormSlog struct {
	log   *slog.Logger
	slow  time.Duration
	level gormlogger.LogLevel
}

func NewGormLogger(log *slog.Logger, slow time.Duration) gormlogger.Interface {
	return &gormSlog{log: log, slow: slow, level: gormlogger.Warn}
}

func (g *gormSlog) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *g
	c.level = level
	return &c
}

func (g *gormSlog) Info(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Info {
		g.log.InfoContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Warn(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Warn {
		g.log.WarnContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Error(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Error {
		g.log.ErrorContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	switch {
	case err != nil && g.level >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		g.log.ErrorContext(ctx, "gorm query failed", "err", err, "elapsed", elapsed, "rows", rows, "sql", sql)
	case g.slow > 0 && elapsed > g.slow && g.level >= gormlogger.Warn:
		sql, rows := fc()
		g.log.WarnContext(ctx, "gorm slow query", "elapsed", elapsed, "threshold", g.slow, "rows", rows, "sql", sql)
	case g.level >= gormlogger.Info:
		sql, rows := fc()
		g.log.DebugContext(ctx, "gorm query", "elapsed", elapsed, "rows", rows, "sql", sql)
	}
}
```

Create `packages/goplatform/db.go`:

```go
package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DBOptions struct {
	Logger             *slog.Logger
	SlowQueryThreshold time.Duration
	PingTimeout        time.Duration
}

// OpenPostgres opens and pings Postgres, and returns an error if either
// fails. There is no degraded mode: a service that cannot reach its database
// must not start and report itself healthy.
func OpenPostgres(ctx context.Context, url string, o DBOptions) (*gorm.DB, error) {
	if url == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: NewGormLogger(o.Logger, o.SlowQueryThreshold),
		// GORM's own ping has no deadline. Ping below with PingTimeout instead.
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres handle: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, o.PingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}
```

Create `packages/goplatform/redis.go`:

```go
package goplatform

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// OpenRedis returns (nil, nil) when url is empty: Redis is optional, and
// every cache treats a nil client as a miss. A set but unreachable url
// returns (nil, err). The caller logs it and runs without cache, and
// /health reports redis "down" until restart.
//
// A url that does not parse as redis:// is used as a bare host:port, as
// both services did before.
func OpenRedis(ctx context.Context, url string, pingTimeout time.Duration) (*redis.Client, error) {
	if url == "" {
		return nil, nil
	}
	var client *redis.Client
	if opt, err := redis.ParseURL(url); err == nil {
		client = redis.NewClient(opt)
	} else {
		client = redis.NewClient(&redis.Options{Addr: url})
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
```

Create `packages/goplatform/health.go`:

```go
package goplatform

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type CheckState string

const (
	CheckUp       CheckState = "up"
	CheckDown     CheckState = "down"
	CheckDisabled CheckState = "disabled"
)

// Check is one dependency probed by /health. Only a Required check that is
// down turns the response into 503.
type Check struct {
	Name     string
	Required bool
	Probe    func(ctx context.Context) CheckState
}

type HealthResponse struct {
	Status  string                `json:"status"`
	Service string                `json:"service"`
	Checks  map[string]CheckState `json:"checks"`
}

// HealthHandler runs every check within timeout. It answers 200 "ok", or
// 503 "degraded" when a required check is down.
func HealthHandler(service string, timeout time.Duration, checks ...Check) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
		defer cancel()

		resp := HealthResponse{Status: "ok", Service: service, Checks: make(map[string]CheckState, len(checks))}
		code := http.StatusOK
		for _, chk := range checks {
			state := chk.Probe(ctx)
			resp.Checks[chk.Name] = state
			if chk.Required && state == CheckDown {
				resp.Status = "degraded"
				code = http.StatusServiceUnavailable
			}
		}
		return c.JSON(code, resp)
	}
}

func DatabaseCheck(db *gorm.DB) Check {
	return Check{Name: "database", Required: true, Probe: func(ctx context.Context) CheckState {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			return CheckDown
		}
		return CheckUp
	}}
}

// RedisCheck reports disabled when REDIS_URL is unset, and down when it is
// set but the client is nil (unreachable at startup) or does not answer.
func RedisCheck(rdb *redis.Client, configured bool) Check {
	return Check{Name: "redis", Probe: func(ctx context.Context) CheckState {
		switch {
		case !configured:
			return CheckDisabled
		case rdb == nil, rdb.Ping(ctx).Err() != nil:
			return CheckDown
		default:
			return CheckUp
		}
	}}
}
```

Create `packages/goplatform/bootstrap.go`:

```go
package goplatform

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Deps are the connections every Go service here starts with.
type Deps struct {
	Service string
	Log     *slog.Logger
	DB      *gorm.DB
	// Redis is nil when REDIS_URL is unset or Redis was unreachable at startup.
	Redis *redis.Client

	redisConfigured bool
	pingTimeout     time.Duration
}

// Bootstrap sets up JSON logging (and makes it slog's default), opens
// Postgres (required) and opens Redis (optional).
func Bootstrap(ctx context.Context, service string, c CommonConfig) (*Deps, error) {
	log, err := NewLogger(os.Stdout, service, c.LogLevel)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(log)

	db, err := OpenPostgres(ctx, c.DatabaseURL, DBOptions{
		Logger:             log,
		SlowQueryThreshold: c.DBSlowQueryThreshold,
		PingTimeout:        c.DependencyPingTimeout,
	})
	if err != nil {
		return nil, err
	}
	log.Info("connected to postgres")

	rdb, err := OpenRedis(ctx, c.RedisURL, c.DependencyPingTimeout)
	switch {
	case err != nil:
		log.Warn("redis unreachable at startup; cache disabled until restart", "err", err)
	case rdb == nil:
		log.Info("REDIS_URL unset; cache disabled")
	default:
		log.Info("connected to redis")
	}

	return &Deps{
		Service:         service,
		Log:             log,
		DB:              db,
		Redis:           rdb,
		redisConfigured: c.RedisURL != "",
		pingTimeout:     c.DependencyPingTimeout,
	}, nil
}

func (d *Deps) HealthHandler() echo.HandlerFunc {
	return HealthHandler(d.Service, d.pingTimeout, DatabaseCheck(d.DB), RedisCheck(d.Redis, d.redisConfigured))
}

func (d *Deps) Close() {
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if d.Redis != nil {
		_ = d.Redis.Close()
	}
}
```

- [ ] **Step 5: Tidy, then run the tests and confirm they pass**

```bash
(cd packages/goplatform && GOWORK=off go mod tidy)
# Each app builds standalone in Docker, so its go.sum must cover goplatform's deps.
(cd apps/core-engine && GOWORK=off go mod tidy)
(cd apps/reference-service && GOWORK=off go mod tidy)
go test ./packages/goplatform/...
go vet ./packages/goplatform/...
(cd apps/core-engine && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/core-engine" ./cmd/server)
(cd apps/reference-service && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/reference-service" ./cmd/server)
```

Expected: PASS, `vet` is clean, and both standalone builds succeed. The app
`go.mod` diffs should be empty or only touch `// indirect` lines, because
nothing imports the new goplatform code yet.

- [ ] **Step 6: Commit**

```bash
git add packages/goplatform apps/core-engine/go.mod apps/core-engine/go.sum   apps/reference-service/go.mod apps/reference-service/go.sum
git commit -m "feat(goplatform): fail-fast Postgres, optional Redis, /health and Bootstrap

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Echo server, graceful shutdown, error envelope

**Files:**
- Create: `packages/goplatform/server.go`, `packages/goplatform/httperr.go`
- Test: `packages/goplatform/server_test.go`, `packages/goplatform/httperr_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks except package-level imports.
- Produces:
  - `type ServerOptions struct { Logger *slog.Logger; CORSAllowOrigins, CORSAllowHeaders []string }`
  - `func NewEcho(o ServerOptions) *echo.Echo`
  - `func Run(ctx context.Context, e *echo.Echo, addr string, shutdownTimeout time.Duration) error`
  - `type ErrorBody struct { Success bool; Error string; ErrorCode string; Errors []string }`, with JSON tags `success`, `error`, `error_code,omitempty`, `errors,omitempty`
  - `func JSONError(c echo.Context, status int, msg string) error`
  - `func JSONErrorCode(c echo.Context, status int, code, msg string) error`
  - `func JSONValidationError(c echo.Context, status int, errs []string) error`

- [ ] **Step 1: Write the failing tests**

Create `packages/goplatform/httperr_test.go`:

```go
package goplatform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func body(t *testing.T, write func(echo.Context) error) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := write(c); err != nil {
		t.Fatal(err)
	}
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name  string
		write func(echo.Context) error
		code  int
		want  string
	}{
		{"plain", func(c echo.Context) error { return JSONError(c, 404, "survey not found") },
			404, `{"success":false,"error":"survey not found"}`},
		{"coded", func(c echo.Context) error { return JSONErrorCode(c, 503, "VISION_UNAVAILABLE", "down") },
			503, `{"success":false,"error":"down","error_code":"VISION_UNAVAILABLE"}`},
		{"validation", func(c echo.Context) error { return JSONValidationError(c, 422, []string{"a", "b"}) },
			422, `{"success":false,"error":"validation failed","errors":["a","b"]}`},
	}
	for _, tc := range cases {
		code, got := body(t, tc.write)
		if code != tc.code || got != tc.want {
			t.Errorf("%s: got %d %s, want %d %s", tc.name, code, got, tc.code, tc.want)
		}
	}
}
```

Create `packages/goplatform/server_test.go`:

```go
package goplatform

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestNewEchoAppliesCORSConfig(t *testing.T) {
	e := NewEcho(ServerOptions{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSAllowOrigins: []string{"*"},
		CORSAllowHeaders: []string{"Origin", "Content-Type"},
	})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Origin,Content-Type" {
		t.Fatalf("Allow-Headers = %q", got)
	}
}

func TestNewEchoSetsRequestID(t *testing.T) {
	e := NewEcho(ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CORSAllowOrigins: []string{"*"}})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Header().Get(echo.HeaderXRequestID) == "" {
		t.Fatal("want X-Request-Id on response")
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, echo.New(), "127.0.0.1:0", time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := Run(context.Background(), echo.New(), l.Addr().String(), time.Second); err == nil {
		t.Fatal("want error when the port is taken")
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./packages/goplatform/...`
Expected: FAIL to compile, because `JSONError`, `JSONErrorCode`, `JSONValidationError`, `NewEcho`, `ServerOptions` and `Run` are undefined.

- [ ] **Step 3: Implement**

Create `packages/goplatform/httperr.go`:

```go
package goplatform

import "github.com/labstack/echo/v4"

// ErrorBody is the error envelope of both Go services. It is a superset of
// the two shapes that existed before standardization ({"error"} and
// {"success":false,"error"}), so no existing client loses a field it reads.
//
// The vision controller in core-engine does not use it: there, "error" is a
// machine code, not a message.
type ErrorBody struct {
	Success   bool     `json:"success"`
	Error     string   `json:"error"`
	ErrorCode string   `json:"error_code,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// validationFailedMessage fills "error" on validation responses, which
// previously carried only "errors".
const validationFailedMessage = "validation failed"

func JSONError(c echo.Context, status int, msg string) error {
	return c.JSON(status, ErrorBody{Error: msg})
}

func JSONErrorCode(c echo.Context, status int, code, msg string) error {
	return c.JSON(status, ErrorBody{Error: msg, ErrorCode: code})
}

func JSONValidationError(c echo.Context, status int, errs []string) error {
	return c.JSON(status, ErrorBody{Error: validationFailedMessage, Errors: errs})
}
```

Create `packages/goplatform/server.go`:

```go
package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// corsAllowMethods are the methods the routes in this repository register.
// That is a property of the code, not of a deployment, so it is not
// configurable.
var corsAllowMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}

type ServerOptions struct {
	Logger           *slog.Logger
	CORSAllowOrigins []string
	CORSAllowHeaders []string
}

// NewEcho returns an Echo instance with the standard middleware, in this
// order: Recover, RequestID, request logger (slog), CORS.
func NewEcho(o ServerOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogError:     true,
		HandleError:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []any{"method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "request_id", v.RequestID}
			if v.Error != nil {
				o.Logger.Error("request", append(attrs, "err", v.Error)...)
				return nil
			}
			o.Logger.Info("request", attrs...)
			return nil
		},
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: o.CORSAllowOrigins,
		AllowMethods: corsAllowMethods,
		AllowHeaders: o.CORSAllowHeaders,
	}))
	return e
}

// Run serves until ctx is cancelled, then shuts down gracefully within
// shutdownTimeout. It returns early with the error if the server cannot
// start.
func Run(ctx context.Context, e *echo.Echo, addr string, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- e.Start(addr) }()
	slog.Info("listening", "addr", addr)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down", "timeout", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

```bash
go test ./packages/goplatform/...
go vet ./packages/goplatform/...
```

Expected: PASS. If `TestNewEchoAppliesCORSConfig` fails only on the header separator (`, ` vs `,`), match Echo v4.13.4's actual join in the assertion. Do not change the middleware.

- [ ] **Step 5: Commit**

```bash
git add packages/goplatform
git commit -m "feat(goplatform): standard Echo server, graceful Run and error envelope

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: reference-service onto `internal/` and `goplatform`

Claim the `reference` lane first.

**Files:**
- Move: `apps/reference-service/pkg/config/` → `internal/config/`
- Move: `apps/reference-service/pkg/adapters/cache/` → `internal/reference/cache/`
- Move: `apps/reference-service/pkg/adapters/controllers/` → `internal/reference/controllers/`
- Move: `apps/reference-service/pkg/entity/` → `internal/reference/entity/`
- Move: `apps/reference-service/pkg/repository/` → `internal/reference/repository/`
- Rewrite: `apps/reference-service/internal/config/config.go`, `apps/reference-service/cmd/server/main.go`
- Modify: `apps/reference-service/cmd/seed-reference-data/main.go` (config and DB open only)
- Test: `apps/reference-service/internal/config/config_test.go`

**Interfaces:**
- Consumes: `goplatform.LoadDotEnv`, `LoadCommon`, `CommonDefaults`, `CommonConfig`, `Duration`, `Port`, `Bootstrap`, `Deps`, `NewEcho`, `ServerOptions`, `Run`, `OpenPostgres`, `DBOptions`, `NewLogger`.
- Produces:
  - `config.Config{ goplatform.CommonConfig; Port string; ReferenceCacheTTL time.Duration }`
  - `config.Load() (*Config, error)`, which loads `.env` and then calls `FromEnv`
  - `config.FromEnv() (*Config, error)`
  - `config.DefaultReferenceCacheTTL`

- [ ] **Step 1: Move the packages**

```bash
cd apps/reference-service
mkdir -p internal/reference
git mv pkg/config internal/config
git mv pkg/adapters/cache internal/reference/cache
git mv pkg/adapters/controllers internal/reference/controllers
git mv pkg/entity internal/reference/entity
git mv pkg/repository internal/reference/repository
M=github.com/experience-gateway/apps/reference-service
grep -rl "$M/pkg/" --include='*.go' . | xargs sed -i \
  -e "s#$M/pkg/config#$M/internal/config#g" \
  -e "s#$M/pkg/adapters/cache#$M/internal/reference/cache#g" \
  -e "s#$M/pkg/adapters/controllers#$M/internal/reference/controllers#g" \
  -e "s#$M/pkg/entity#$M/internal/reference/entity#g" \
  -e "s#$M/pkg/repository#$M/internal/reference/repository#g"
test -z "$(find pkg -type f 2>/dev/null)" && rm -rf pkg
cd ../..
go build ./apps/reference-service/...
```

Expected: builds. `grep -rn "reference-service/pkg" apps/reference-service` prints nothing.

- [ ] **Step 2: Write the failing config tests**

Replace `apps/reference-service/internal/config/config_test.go` with:

```go
package config_test

import (
	"testing"
	"time"

	"github.com/experience-gateway/apps/reference-service/internal/config"
)

// FromEnv, not Load: Load reads the repository's .env, which would fill in
// the variables these tests clear.
func setBaseEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	for _, k := range []string{"REFERENCE_SERVICE_PORT", "REFERENCE_SERVICE_URL", "NEXT_PUBLIC_REFERENCE_SERVICE_URL",
		"PORT", "REFERENCE_CACHE_TTL", "CORS_ALLOW_HEADERS"} {
		t.Setenv(k, "")
	}
}

func TestFromEnv_ReferenceServicePortTakesPriorityOverGenericPort(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("REFERENCE_SERVICE_PORT", "8086")
	t.Setenv("PORT", "4000")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() returned error: %v", err)
	}
	if cfg.Port != "8086" {
		t.Errorf("Port = %q, want %q (REFERENCE_SERVICE_PORT must win over generic PORT)", cfg.Port, "8086")
	}
}

func TestFromEnv_Defaults(t *testing.T) {
	setBaseEnv(t)

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() returned error: %v", err)
	}
	if cfg.Port != "8086" {
		t.Errorf("Port = %q, want 8086", cfg.Port)
	}
	if cfg.ReferenceCacheTTL != 10*time.Minute {
		t.Errorf("ReferenceCacheTTL = %v, want 10m", cfg.ReferenceCacheTTL)
	}
	want := []string{"Origin", "Content-Type", "Accept", "Authorization"}
	if len(cfg.CORSAllowHeaders) != len(want) {
		t.Fatalf("CORSAllowHeaders = %v, want %v", cfg.CORSAllowHeaders, want)
	}
	for i := range want {
		if cfg.CORSAllowHeaders[i] != want[i] {
			t.Fatalf("CORSAllowHeaders = %v, want %v", cfg.CORSAllowHeaders, want)
		}
	}
}

func TestFromEnv_RequiresDatabaseURL(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := config.FromEnv(); err == nil {
		t.Fatal("want error when DATABASE_URL is unset")
	}
}

func TestFromEnv_RejectsBadCacheTTL(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("REFERENCE_CACHE_TTL", "forever")
	if _, err := config.FromEnv(); err == nil {
		t.Fatal("want error for malformed REFERENCE_CACHE_TTL")
	}
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run: `go test ./apps/reference-service/internal/config/...`
Expected: FAIL to compile, because `config.FromEnv` and `cfg.ReferenceCacheTTL` are undefined.

- [ ] **Step 4: Rewrite the config**

Replace `apps/reference-service/internal/config/config.go`:

```go
package config

import (
	"time"

	"github.com/experience-gateway/packages/goplatform"
)

// DefaultReferenceCacheTTL is the TTL cmd/server/main.go passed to
// NewReferenceCache before the 2026-09-26 standardization.
const DefaultReferenceCacheTTL = 10 * time.Minute

// defaultCORSAllowHeaders is the explicit list reference-service allowed
// before standardization (echo.HeaderOrigin, ContentType, Accept, Authorization).
var defaultCORSAllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}

type Config struct {
	goplatform.CommonConfig
	Port              string
	ReferenceCacheTTL time.Duration
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
	ttl, err := goplatform.Duration("REFERENCE_CACHE_TTL", DefaultReferenceCacheTTL)
	if err != nil {
		return nil, err
	}
	return &Config{
		CommonConfig:      common,
		Port:              goplatform.Port("REFERENCE_SERVICE_PORT", "PORT", "8086"),
		ReferenceCacheTTL: ttl,
	}, nil
}
```

- [ ] **Step 5: Run the config tests and confirm they pass**

Run: `go test ./apps/reference-service/internal/config/...`
Expected: PASS.

- [ ] **Step 6: Rewrite `cmd/server/main.go`**

Replace `apps/reference-service/cmd/server/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/reference-service/internal/config"
	"github.com/experience-gateway/apps/reference-service/internal/reference/cache"
	"github.com/experience-gateway/apps/reference-service/internal/reference/controllers"
	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
	"github.com/experience-gateway/apps/reference-service/internal/reference/repository"
	"github.com/experience-gateway/packages/goplatform"
)

const serviceName = "reference-service"

func main() {
	if err := run(); err != nil {
		slog.Error(serviceName+" exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := goplatform.Bootstrap(ctx, serviceName, cfg.CommonConfig)
	if err != nil {
		return err
	}
	defer deps.Close()

	// AutoMigrate at error-level logging, as before. Its error used to be
	// discarded; it is now logged. It is not fatal, because the database is
	// shared with Seagull-gateway (docs/SHARED-DATABASE.md).
	if err := deps.DB.Session(&gorm.Session{Logger: deps.DB.Logger.LogMode(gormlogger.Error)}).AutoMigrate(
		&entity.RefDimension{},
		&entity.RefCondition{},
		&entity.RefSeverityTierGroup{},
		&entity.RefSeverityTierItem{},
		&entity.RefBrand{},
		&entity.RefIngredient{},
		&entity.RefStatus{},
		&entity.RefApplication{},
		&entity.RefSkinCondition{},
	); err != nil {
		deps.Log.Error("automigrate failed", "err", err)
	}

	refRepo := repository.NewReferenceRepo(deps.DB, cache.NewReferenceCache(deps.Redis, cfg.ReferenceCacheTTL))
	refHandler := controllers.NewReferenceHandler(refRepo)

	e := goplatform.NewEcho(goplatform.ServerOptions{
		Logger:           deps.Log,
		CORSAllowOrigins: cfg.CORSAllowOrigins,
		CORSAllowHeaders: cfg.CORSAllowHeaders,
	})
	e.GET("/health", deps.HealthHandler())
	refHandler.RegisterRoutes(e.Group("/api/reference"))
	refHandler.RegisterRoutes(e.Group("/reference"))

	return goplatform.Run(ctx, e, ":"+cfg.Port, cfg.ShutdownTimeout)
}
```

- [ ] **Step 7: Point the seeder at the shared config and DB open**

In `apps/reference-service/cmd/seed-reference-data/main.go`:
- Delete the `gormLogger := logger.New(...)` block.
- Delete the `gorm.Open(...)` call that follows it (the lines between `cfg, err := config.Load()` handling and `log.Printf("--> Seeding %d dimensions...`).
- Put this in their place:

```go
	db, err := goplatform.OpenPostgres(context.Background(), cfg.DatabaseURL, goplatform.DBOptions{
		Logger:             slog.Default(),
		SlowQueryThreshold: cfg.DBSlowQueryThreshold,
		PingTimeout:        cfg.DependencyPingTimeout,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
```

Fix the imports:
- Add `"context"`, `"log/slog"` and `"github.com/experience-gateway/packages/goplatform"`.
- Remove `"os"`, `gorm.io/driver/postgres` and `gorm.io/gorm/logger`, if they are now unused.
- Keep `"gorm.io/gorm"` only if the file still references `gorm.`.

`go build` reports the exact unused imports; delete exactly those. The seeder's own 500ms slow threshold is intentionally replaced by `DB_SLOW_QUERY_THRESHOLD` (spec, "Rollout and risk").

- [ ] **Step 8: Verify**

```bash
go build ./apps/reference-service/...
go vet ./apps/reference-service/...
go test ./apps/reference-service/...
(cd apps/reference-service && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/reference-service" ./cmd/server)
grep -rn '"log"$' apps/reference-service/cmd/server || echo "server main no longer uses stdlib log"
```

Expected: all pass. The last line prints `server main no longer uses stdlib log`.

- [ ] **Step 9: Commit**

```bash
git add -A apps/reference-service
git commit -m "refactor(reference-service): move to internal/ and goplatform bootstrap

Fails fast without Postgres, reports dependency state on /health, logs
JSON via slog. REFERENCE_CACHE_TTL is configurable (default 10m).

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Move core-engine `pkg/` → `internal/`

This is a move only. Claim `core-form`, `core-match`, `core-score` and `core-vision` first. Hold them through Task 9.

**Files:**
- Move: `apps/core-engine/pkg/` → `apps/core-engine/internal/` (every subpackage keeps its relative path)
- Modify: every `.go` file under `apps/core-engine` that imports `apps/core-engine/pkg/...`

**Interfaces:**
- Produces: identical packages at `github.com/experience-gateway/apps/core-engine/internal/<same path>`.

- [ ] **Step 1: Move and rewrite the imports**

```bash
git mv apps/core-engine/pkg apps/core-engine/internal
grep -rl "apps/core-engine/pkg/" --include='*.go' apps/core-engine \
  | xargs sed -i 's#github.com/experience-gateway/apps/core-engine/pkg/#github.com/experience-gateway/apps/core-engine/internal/#g'
grep -rn "apps/core-engine/pkg/" --include='*.go' apps/core-engine || echo "no pkg imports left"
```

Expected: `no pkg imports left`.

- [ ] **Step 2: Verify that no logic changed**

```bash
go build ./apps/core-engine/...
go vet ./apps/core-engine/...
go test ./apps/core-engine/...
git diff --cached -M --stat | tail -3
git diff -M HEAD --stat -- apps/core-engine | grep -v "=>" | grep -v "internal/" || true
```

Expected: build, vet and test pass. The rename stat shows `pkg/... => internal/...`. The only content changes are import lines. Spot-check with `git diff -M HEAD -- apps/core-engine | grep '^[-+][^-+]' | grep -v 'core-engine/internal\|core-engine/pkg'`, which must print nothing.

- [ ] **Step 3: Commit**

```bash
git add -A apps/core-engine
git commit -m "refactor(core-engine): move pkg/ to internal/

Pure move; only import paths change.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: core-engine config owns every tunable

**Files:**
- Rewrite: `apps/core-engine/internal/config/config.go`
- Create: `apps/core-engine/internal/config/config_test.go`
- Modify these constructors, removing their internal defaults and constants:
  - `internal/form/adapters/scoreclient/client.go` (`New`)
  - `internal/form/adapters/visionclient/client.go` (`New`)
  - `internal/score/adapters/cache/score_cache.go` (`NewScoreCache`)
  - `internal/match/adapters/cache/match_cache.go` (`NewMatchCache`)
  - `internal/vision/catalog/reference_client.go` (`NewReferenceClient`, `referenceTimeout`)
  - `internal/vision/inference/client.go` (`NewHTTPCapabilityClient`, `dispatchTimeout`)
  - `internal/vision/workers/segmenter_worker.go` (`NewSegmenterWorkerWithURL`)
  - `internal/vision/orchestrator/dag_engine.go` (`NewVisionOrchestrator`, `Execute`)
  - `internal/match/service/match_service.go` (`NewMatchService`, `TriggerShadeExtraction`)
- Modify the tests calling those constructors:
  - `internal/vision/catalog/catalog_test.go`
  - `internal/vision/inference/client_test.go`
  - `internal/vision/orchestrator/dag_engine_test.go`
  - `internal/vision/adapters/controllers/vision_controller_test.go`
  - `internal/vision/workers/segmenter_worker_test.go`
  - `internal/match/service/match_service_test.go`
  - `internal/match/service/match_service_benchmark_test.go`
  - `internal/match/adapters/controllers/match_handler_test.go`
- Modify: `apps/core-engine/cmd/server/main.go` (call sites only; Task 8 rewrites it)
- Test: `apps/core-engine/internal/match/service/tryon_trigger_test.go` (new)

**Interfaces:**
- Consumes: `goplatform.LoadDotEnv`, `LoadCommon`, `CommonDefaults`, `CommonConfig`, `Duration`, `Port`, `First`.
- Produces:
  - `config.Config` embeds `goplatform.CommonConfig`, plus `Port`, `ReferenceServiceURL`, `VisionAIWorkerURL`, `ParadevGatewayURL`, `ParadevGatewayAPIKey`, `TryonEngineURL string`, plus `FormCacheTTL`, `ScoreCacheTTL`, `MatchCacheTTL`, `VisionCatalogTTL`, `ScoreClientTimeout`, `ParadevTimeout`, `ReferenceClientTimeout`, `VisionDispatchTimeout`, `VisionDAGTimeout`, `SegmenterTimeout time.Duration`.
  - `config.Load()` and `config.FromEnv()`, both `(*Config, error)`.
  - Constructor signatures:
    - `scoreclient.New(baseURL string, timeout time.Duration) *Client`
    - `catalog.NewReferenceClient(baseURL string, timeout time.Duration) *ReferenceClient`
    - `inference.NewHTTPCapabilityClient(baseURL string, timeout time.Duration) *HTTPCapabilityClient`
    - `workers.NewSegmenterWorkerWithURL(workerURL string, timeout time.Duration) *SegmenterWorker`
    - `orchestrator.NewVisionOrchestrator(segmenter Segmenter, occlusion OcclusionChecker, resolver PlanResolver, capabilities inference.CapabilityClient, execTimeout time.Duration) *VisionOrchestrator`
    - `service.NewMatchService(repo *repository.MatchRepo, tryonEngineURL string) *MatchService`
  - `visionclient.New`, `NewScoreCache` and `NewMatchCache` keep their signatures but lose their `<= 0` defaulting.

- [ ] **Step 1: Write the failing config test**

Create `apps/core-engine/internal/config/config_test.go`:

```go
package config_test

import (
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/config"
)

var tunables = []string{"FORM_CACHE_TTL", "SCORE_CACHE_TTL", "MATCH_CACHE_TTL", "VISION_CATALOG_TTL",
	"SCORE_CLIENT_TIMEOUT", "PARADEV_TIMEOUT", "REFERENCE_CLIENT_TIMEOUT", "VISION_DISPATCH_TIMEOUT",
	"VISION_DAG_TIMEOUT", "SEGMENTER_TIMEOUT", "TRYON_ENGINE_URL", "CORS_ALLOW_HEADERS"}

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
```

Create `apps/core-engine/internal/match/service/tryon_trigger_test.go`:

```go
package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
)

func TestTriggerShadeExtractionUsesConfiguredURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	svc := service.NewMatchService(repository.NewMatchRepo(nil), srv.URL)
	if err := svc.TriggerShadeExtraction(context.Background(), entity.Shade{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/extract" {
		t.Fatalf("path = %q, want /extract", gotPath)
	}
}

func TestTriggerShadeExtractionWithoutURLErrors(t *testing.T) {
	t.Setenv("TRYON_ENGINE_URL", "http://must-not-be-read")
	svc := service.NewMatchService(repository.NewMatchRepo(nil), "")
	if err := svc.TriggerShadeExtraction(context.Background(), entity.Shade{ID: "s1"}); err == nil {
		t.Fatal("want error: the service must read its URL from config, not the environment")
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./apps/core-engine/internal/config/... ./apps/core-engine/internal/match/service/...`
Expected: FAIL to compile. `config.FromEnv` and the new fields are undefined, and `NewMatchService` gets too many arguments.

- [ ] **Step 3: Rewrite the config**

Replace `apps/core-engine/internal/config/config.go`:

```go
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

	DefaultScoreClientTimeout = 5 * time.Second // form/adapters/scoreclient
	// main.go and visionclient. Paradev's scoring call runs a synchronous
	// LLM pass; the reviewed Paradev spec asked for 45-60s.
	DefaultParadevTimeout         = 50 * time.Second
	DefaultReferenceClientTimeout = 3 * time.Second  // vision/catalog/reference_client.go
	DefaultVisionDispatchTimeout  = 20 * time.Second // vision/inference/client.go
	DefaultVisionDAGTimeout       = 30 * time.Second // vision/orchestrator/dag_engine.go
	DefaultSegmenterTimeout       = 4 * time.Second  // vision/workers/segmenter_worker.go
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
		Port:                 goplatform.Port("CORE_ENGINE_PORT", "PORT", "8082"),
		ReferenceServiceURL:  firstOr("REFERENCE_SERVICE_URL", "http://127.0.0.1:8086"),
		VisionAIWorkerURL:    firstOr("VISION_AI_WORKER_URL", "http://127.0.0.1:8088"),
		ParadevGatewayURL:    firstOr("PARADEV_GATEWAY_URL", "http://127.0.0.1:9080/paradev/scoring"),
		ParadevGatewayAPIKey: goplatform.First("PARADEV_GATEWAY_API_KEY"),
		TryonEngineURL:       goplatform.First("TRYON_ENGINE_URL"),
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
```

- [ ] **Step 4: Thread the values through the constructors**

`internal/form/adapters/scoreclient/client.go`: change `New`:

```go
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: timeout},
	}
}
```

`internal/form/adapters/visionclient/client.go`: in `New`, delete the three lines `if timeout <= 0 { timeout = 50 * time.Second }`. Replace the doc comment's last sentence with: `The timeout comes from config (PARADEV_TIMEOUT).`

`internal/score/adapters/cache/score_cache.go` and `internal/match/adapters/cache/match_cache.go`: in `NewScoreCache` and `NewMatchCache`, delete `if ttl <= 0 { ttl = 15 * time.Minute }`. Config guarantees a positive TTL.

`internal/vision/catalog/reference_client.go`: delete `const referenceTimeout = 3 * time.Second` and change the constructor:

```go
func NewReferenceClient(baseURL string, timeout time.Duration) *ReferenceClient {
```

Inside it, use `&http.Client{Timeout: timeout}`. Keep the rest of the body unchanged.

`internal/vision/inference/client.go`: remove `dispatchTimeout   = 20 * time.Second` from the `const` block, leaving `dispatchZonesPath`. Change the constructor:

```go
func NewHTTPCapabilityClient(baseURL string, timeout time.Duration) *HTTPCapabilityClient {
	return &HTTPCapabilityClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}
```

`internal/vision/workers/segmenter_worker.go`:

```go
func NewSegmenterWorkerWithURL(workerURL string, timeout time.Duration) *SegmenterWorker {
	return &SegmenterWorker{
		workerURL:  workerURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}
```

`internal/vision/orchestrator/dag_engine.go`:
- Add the field `execTimeout time.Duration` to `VisionOrchestrator`.
- Add the trailing parameter `execTimeout time.Duration` to `NewVisionOrchestrator` and set `execTimeout: execTimeout`.
- In `Execute`, replace:

```go
	// Enforce 30-second context timeout constraint
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
```

with:

```go
	// Bound the whole pipeline (VISION_DAG_TIMEOUT).
	execCtx, cancel := context.WithTimeout(ctx, o.execTimeout)
```

`internal/match/service/match_service.go`:
- Add the field `tryonEngineURL string` to `MatchService`.
- Change the constructor to `func NewMatchService(repo *repository.MatchRepo, tryonEngineURL string) *MatchService` and set `tryonEngineURL: tryonEngineURL`.
- In `TriggerShadeExtraction`, replace `baseURL := os.Getenv("TRYON_ENGINE_URL")` with `baseURL := s.tryonEngineURL`.
- Remove `"os"` from the imports if nothing else uses it. `go build` will say.

- [ ] **Step 5: Update the constructor callers in the tests and `main.go`**

In the test files, pass explicit values. A test's timeout is a test fixture, not configuration:

```bash
cd apps/core-engine/internal
sed -i 's#NewReferenceClient(\([^)]*\))#NewReferenceClient(\1, 5*time.Second)#g' vision/catalog/catalog_test.go
sed -i 's#NewHTTPCapabilityClient(\([^)]*\))#NewHTTPCapabilityClient(\1, 5*time.Second)#g' vision/inference/client_test.go
sed -i 's#NewSegmenterWorkerWithURL(\([^)]*\))#NewSegmenterWorkerWithURL(\1, 5*time.Second)#g' vision/workers/segmenter_worker_test.go
sed -i 's#NewVisionOrchestrator(\(.*\))$#NewVisionOrchestrator(\1, 30*time.Second)#' \
  vision/orchestrator/dag_engine_test.go vision/adapters/controllers/vision_controller_test.go
sed -i 's#service.NewMatchService(repo)#service.NewMatchService(repo, "")#g' \
  match/service/match_service_test.go match/service/match_service_benchmark_test.go \
  match/adapters/controllers/match_handler_test.go
cd ../../..
go vet ./apps/core-engine/internal/... 2>&1 | grep -i "time\b\|undefined: time" || true
```

For each test file `go vet` reports as missing `"time"`, add `"time"` to its imports. Check `client_test.go:99`, which builds its own context deadline. Its assertion must still hold with a 5s client timeout. If it relied on the 20s constant, set that call's timeout to the value the test asserts against.

In `apps/core-engine/cmd/server/main.go` (still the old bootstrap; Task 8 replaces it), fix `config.Load()` handling first. Replace:

```go
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Core Engine: config warning: %v, using defaults", err)
	}
```

with:

```go
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Core Engine: config: %v", err)
	}
```

Then update the call sites:
- `formCache.NewSurveyCache(rdb, 15*time.Minute)` → `formCache.NewSurveyCache(rdb, cfg.FormCacheTTL)`
- `formScoreClient.New("http://127.0.0.1:" + cfg.Port)` → `formScoreClient.New("http://127.0.0.1:"+cfg.Port, cfg.ScoreClientTimeout)`
- `formVisionClient.New(cfg.ParadevGatewayURL, cfg.ParadevGatewayAPIKey, 50*time.Second)` → `...cfg.ParadevTimeout)`
- `scoreCache.NewScoreCache(rdb, 15*time.Minute)` → `scoreCache.NewScoreCache(rdb, cfg.ScoreCacheTTL)`
- `matchCache.NewMatchCache(rdb, 15*time.Minute)` → `matchCache.NewMatchCache(rdb, cfg.MatchCacheTTL)`
- `matchService.NewMatchService(mRepo)` → `matchService.NewMatchService(mRepo, cfg.TryonEngineURL)`
- `visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL)` → `visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL, cfg.ReferenceClientTimeout)`
- `time.Minute,` (the `NewCachedCatalog` TTL) → `cfg.VisionCatalogTTL,`
- `visionWorkers.NewSegmenterWorkerWithURL(cfg.VisionAIWorkerURL)` → `...(cfg.VisionAIWorkerURL, cfg.SegmenterTimeout)`
- `visionInference.NewHTTPCapabilityClient(cfg.VisionAIWorkerURL),` → `...(cfg.VisionAIWorkerURL, cfg.VisionDispatchTimeout),`
- Add `cfg.VisionDAGTimeout,` as the last argument of `visionOrchestrator.NewVisionOrchestrator(...)`.

- [ ] **Step 6: Verify, and prove the literals are gone**

```bash
go build ./apps/core-engine/...
go vet ./apps/core-engine/...
go test ./apps/core-engine/...
grep -rnE "[0-9]+ ?\* ?time\.(Second|Minute)|time\.Minute[,)]" --include='*.go' apps/core-engine \
  | grep -v "_test.go" | grep -v "internal/config/config.go" || echo "no duration literals outside config"
grep -rn 'os.Getenv' --include='*.go' apps/core-engine | grep -v _test.go || echo "no direct env reads"
```

Expected: all pass. The third command's only remaining hits outside config should be the old bootstrap's GORM slow threshold (`2 * time.Second`) and Redis ping (`3*time.Second`) in `cmd/server/main.go`, which Task 8 removes. Anything else is a missed literal: move it into config the same way. The fourth prints `no direct env reads`.

- [ ] **Step 7: Commit**

```bash
git add -A apps/core-engine
git commit -m "refactor(core-engine): config owns every timeout, TTL and TRYON_ENGINE_URL

Defaults equal the previous literals. Constructors no longer default
zero values internally, so each value is defined exactly once.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: core-engine `main.go` onto `goplatform`

**Files:**
- Rewrite: `apps/core-engine/cmd/server/main.go`

**Interfaces:**
- Consumes: `goplatform.Bootstrap`, `Deps`, `NewEcho`, `ServerOptions`, `Run` (Tasks 3–4). The `config.Config` fields and the constructor signatures come from Task 7.

- [ ] **Step 1: Rewrite `main.go`**

Replace `apps/core-engine/cmd/server/main.go`. The route tables are copied verbatim from the current file. Only the bootstrap, health and server lines change.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/packages/goplatform"

	// Form Engine
	formCache "github.com/experience-gateway/apps/core-engine/internal/form/adapters/cache"
	formControllers "github.com/experience-gateway/apps/core-engine/internal/form/adapters/controllers"
	formScoreClient "github.com/experience-gateway/apps/core-engine/internal/form/adapters/scoreclient"
	formVisionClient "github.com/experience-gateway/apps/core-engine/internal/form/adapters/visionclient"
	formEntity "github.com/experience-gateway/apps/core-engine/internal/form/entity"
	formRepo "github.com/experience-gateway/apps/core-engine/internal/form/repository"
	formService "github.com/experience-gateway/apps/core-engine/internal/form/service"

	// Score Engine
	scoreCache "github.com/experience-gateway/apps/core-engine/internal/score/adapters/cache"
	scoreControllers "github.com/experience-gateway/apps/core-engine/internal/score/adapters/controllers"
	scoreEntity "github.com/experience-gateway/apps/core-engine/internal/score/entity"
	scoreRepo "github.com/experience-gateway/apps/core-engine/internal/score/repository"
	scoreService "github.com/experience-gateway/apps/core-engine/internal/score/service"

	// Match Engine
	matchCache "github.com/experience-gateway/apps/core-engine/internal/match/adapters/cache"
	matchControllers "github.com/experience-gateway/apps/core-engine/internal/match/adapters/controllers"
	matchEntity "github.com/experience-gateway/apps/core-engine/internal/match/entity"
	matchRepo "github.com/experience-gateway/apps/core-engine/internal/match/repository"
	matchService "github.com/experience-gateway/apps/core-engine/internal/match/service"

	// Vision Engine
	visionControllers "github.com/experience-gateway/apps/core-engine/internal/vision/adapters/controllers"
	visionCatalog "github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	visionClient "github.com/experience-gateway/apps/core-engine/internal/vision/client"
	visionInference "github.com/experience-gateway/apps/core-engine/internal/vision/inference"
	visionOrchestrator "github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	visionWorkers "github.com/experience-gateway/apps/core-engine/internal/vision/workers"
)

const serviceName = "core-engine"

func main() {
	if err := run(); err != nil {
		slog.Error(serviceName+" exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := goplatform.Bootstrap(ctx, serviceName, cfg.CommonConfig)
	if err != nil {
		return err
	}
	defer deps.Close()
	db, rdb := deps.DB, deps.Redis

	// AutoMigrate at error-level logging, as before. Its error used to be
	// discarded; it is now logged. It is not fatal, because the database is
	// shared with Seagull-gateway (docs/SHARED-DATABASE.md).
	if err := db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Error)}).AutoMigrate(
		&formEntity.SurveyDefinition{},
		&scoreEntity.ScoreRuleset{},
		&matchEntity.ProductCatalogItem{},
		&matchEntity.ConflictMatrixRule{},
		&matchEntity.ProductGroup{},
		&matchEntity.Shade{},
		&matchEntity.ShadeAsset{},
	); err != nil {
		deps.Log.Error("automigrate failed", "err", err)
	}

	// Form Engine
	formC := formCache.NewSurveyCache(rdb, cfg.FormCacheTTL)
	fRepo := formRepo.NewSurveyRepo(db)
	// scoreclient appends /api/scoring/evaluate, so the base must be the host only.
	scoreClient := formScoreClient.New("http://127.0.0.1:"+cfg.Port, cfg.ScoreClientTimeout)
	paradevClient := formVisionClient.New(cfg.ParadevGatewayURL, cfg.ParadevGatewayAPIKey, cfg.ParadevTimeout)
	fService := formService.NewSurveyService(fRepo, formC, scoreClient, paradevClient)
	formHandler := formControllers.NewSurveyHandler(fService)

	// Score Engine
	scoreC := scoreCache.NewScoreCache(rdb, cfg.ScoreCacheTTL)
	sRepo := scoreRepo.NewRulesetRepo(db, scoreC)
	sService := scoreService.NewScoreService(sRepo)
	scoreHandler := scoreControllers.NewScoreHandler(sService)

	// Match Engine
	matchC := matchCache.NewMatchCache(rdb, cfg.MatchCacheTTL)
	mRepo := matchRepo.NewMatchRepo(db, matchC)
	mService := matchService.NewMatchService(mRepo, cfg.TryonEngineURL)
	matchHandler := matchControllers.NewMatchHandler(mService)

	// Vision Engine Orchestrator
	visionBridge := visionClient.NewEngineBridge(
		"http://127.0.0.1:"+cfg.Port+"/core/score-engine",
		"http://127.0.0.1:"+cfg.Port+"/core/match-engine",
	)
	visionReferenceCatalog := visionCatalog.NewCachedCatalog(
		visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL, cfg.ReferenceClientTimeout),
		cfg.VisionCatalogTTL,
	)
	visionOrch := visionOrchestrator.NewVisionOrchestrator(
		visionWorkers.NewSegmenterWorkerWithURL(cfg.VisionAIWorkerURL, cfg.SegmenterTimeout),
		visionWorkers.NewOcclusionWorker(),
		visionCatalog.NewResolver(visionReferenceCatalog),
		visionInference.NewHTTPCapabilityClient(cfg.VisionAIWorkerURL, cfg.VisionDispatchTimeout),
		cfg.VisionDAGTimeout,
	)
	visionHandler := visionControllers.NewVisionController(visionOrch, visionReferenceCatalog, visionBridge)

	e := goplatform.NewEcho(goplatform.ServerOptions{
		Logger:           deps.Log,
		CORSAllowOrigins: cfg.CORSAllowOrigins,
		CORSAllowHeaders: cfg.CORSAllowHeaders,
	})

	// One health handler for the service and every engine prefix. It reports
	// real dependency state instead of a static "active" map.
	health := deps.HealthHandler()
	e.GET("/health", health)

	// --- FORM ENGINE ---
	mountFormRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.GET("/survey", formHandler.List)
		g.GET("/survey/:code", formHandler.Get)
		g.POST("/survey", formHandler.Save)
		g.PUT("/survey/:code", formHandler.Save)
		g.POST("/evaluate", formHandler.Evaluate)
		g.POST("/survey/:code/evaluate", formHandler.Evaluate)
		g.POST("/survey/:code/evaluate-with-photos", formHandler.EvaluateWithPhotos)
	}
	mountFormRoutes(e.Group("/core/form-engine"))
	mountFormRoutes(e.Group("/api/forms"))
	mountFormRoutes(e.Group("/api/form"))
	mountFormRoutes(e.Group("/v1"))

	// --- SCORE ENGINE ---
	mountScoreRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.POST("/evaluate", scoreHandler.Evaluate)
		g.POST("/simulate", scoreHandler.Simulate)
		g.GET("/rulesets", scoreHandler.ListRulesets)
		g.GET("/rulesets/active", scoreHandler.GetActiveRuleset)
		g.GET("/rulesets/:id", scoreHandler.GetRuleset)
		g.POST("/rulesets", scoreHandler.CreateRuleset)
		g.PUT("/rulesets/:id", scoreHandler.UpdateRuleset)
		g.DELETE("/rulesets/:id", scoreHandler.DeleteRuleset)
	}
	mountScoreRoutes(e.Group("/core/score-engine"))
	mountScoreRoutes(e.Group("/api/scoring"))
	mountScoreRoutes(e.Group("/api/score"))

	// --- MATCH ENGINE ---
	mountMatchRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.POST("/evaluate", matchHandler.Evaluate)
		g.GET("/products", matchHandler.ListProducts)
		g.GET("/conflicts", matchHandler.ListConflictRules)
		g.POST("/conflicts", matchHandler.CreateConflictRule)
		g.PUT("/conflicts", matchHandler.UpdateConflictRule)
		g.DELETE("/conflicts", matchHandler.DeleteConflictRule)
		g.GET("/product-groups", matchHandler.ListProductGroups)
		g.POST("/product-groups", matchHandler.CreateProductGroup)
		g.PUT("/product-groups", matchHandler.UpdateProductGroup)
		g.DELETE("/product-groups", matchHandler.DeleteProductGroup)
		g.GET("/shades", matchHandler.ListShades)
		g.POST("/shades", matchHandler.CreateShade)
		g.PUT("/shades", matchHandler.UpdateShade)
		g.DELETE("/shades", matchHandler.DeleteShade)
		g.GET("/shade-assets/:id", matchHandler.GetShadeAsset)
		g.POST("/shades/:id/extraction-callback", matchHandler.ExtractionCallback)
	}
	mountMatchRoutes(e.Group("/core/match-engine"))
	mountMatchRoutes(e.Group("/api/matching"))
	mountMatchRoutes(e.Group("/api/match"))

	// --- VISION ENGINE ORCHESTRATION ---
	mountVisionRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.POST("/analyze-image", visionHandler.AnalyzeImage)
		g.POST("/evaluate", visionHandler.AnalyzeImage)
		g.GET("/config/:brandId/:applicationId", visionHandler.GetConfig)
		g.GET("/registry", visionHandler.GetRegistry)
	}
	mountVisionRoutes(e.Group("/core/vision-engine"))
	mountVisionRoutes(e.Group("/api/v1/vision"))
	mountVisionRoutes(e.Group("/api/vision"))

	return goplatform.Run(ctx, e, ":"+cfg.Port, cfg.ShutdownTimeout)
}
```

- [ ] **Step 2: Confirm the route table is unchanged**

```bash
git show HEAD:apps/core-engine/cmd/server/main.go | grep -oE 'g\.(GET|POST|PUT|DELETE)\("[^"]+"|e\.Group\("[^"]+"\)' | sort > /tmp/routes_before
grep -oE 'g\.(GET|POST|PUT|DELETE)\("[^"]+"|e\.Group\("[^"]+"\)' apps/core-engine/cmd/server/main.go | sort > /tmp/routes_after
diff /tmp/routes_before /tmp/routes_after && echo "routes identical"
```

Expected: `routes identical`. Use the scratchpad directory instead of `/tmp` if you are running on Windows.

- [ ] **Step 3: Verify**

```bash
go build ./apps/core-engine/...
go vet ./apps/core-engine/...
go test ./apps/core-engine/...
(cd apps/core-engine && GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o "$(mktemp -d)/core-engine" ./cmd/server)
grep -rnE "[0-9]+ ?\* ?time\.(Second|Minute)|time\.Minute[,)]" --include='*.go' apps/core-engine \
  | grep -v _test.go | grep -v internal/config/config.go || echo "no duration literals outside config"
```

Expected: all pass, and the last line prints `no duration literals outside config`.

- [ ] **Step 4: Run it**

This step needs the `runtime` lane. Postgres comes from Seagull-gateway's `docker-compose.infra.yml` (`README.md`).

- **With Postgres running:** `go run ./apps/core-engine/cmd/server`.
  - `curl -s localhost:8082/health` returns 200 with `{"status":"ok","service":"core-engine","checks":{"database":"up","redis":...}}`.
  - `curl -s localhost:8082/api/scoring/health` returns the same body.
  - `curl -s -X POST localhost:8082/api/scoring/evaluate -d '{' -H 'Content-Type: application/json'` returns 400 with an `error` field.
  - Ctrl-C logs `shutting down` and exits 0.
- **With Postgres stopped:** the same command exits non-zero, logging `ping postgres`.

If Postgres is not available in this session, say so in the task report. Do not claim the manual check passed.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/cmd/server/main.go
git commit -m "refactor(core-engine): bootstrap through goplatform

Graceful shutdown on SIGTERM, fail-fast Postgres, real /health on every
engine prefix, JSON logs. Routes unchanged.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: One error envelope (form, score, match, reference)

**Files:**
- Modify: `apps/core-engine/internal/form/adapters/controllers/errors.go`, `survey_handlers.go`
- Modify: `apps/core-engine/internal/score/adapters/controllers/score_handler.go`
- Modify: `apps/core-engine/internal/match/adapters/controllers/match_handler.go`
- Modify: `apps/reference-service/internal/reference/controllers/reference_handler.go`
- Do **not** modify: `apps/core-engine/internal/vision/adapters/controllers/vision_controller.go`
- Test: `envelope_test.go` in `apps/core-engine/internal/{form,score,match}/adapters/controllers/`

**Interfaces:**
- Consumes: `goplatform.JSONError(c, status, msg)`, `goplatform.JSONErrorCode(c, status, code, msg)`, `goplatform.JSONValidationError(c, status, errs []string)` (Task 4).

- [ ] **Step 1: Write the failing contract tests**

These tests pin backward compatibility. They are written against the new envelope, so they fail until the handlers switch over.

Create `apps/core-engine/internal/score/adapters/controllers/envelope_test.go`:

```go
package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/score/adapters/controllers"
)

// The error body must keep "success":false and "error":<string>, which
// clients read today.
func TestEvaluateBadJSONKeepsEnvelope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	h := controllers.NewScoreHandler(nil) // Bind fails before the service is touched.
	if err := h.Evaluate(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body["success"] != false {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if msg, ok := body["error"].(string); !ok || !strings.HasPrefix(msg, "Invalid JSON payload") {
		t.Fatalf("error = %v, want the existing message prefix", body["error"])
	}
	if _, has := body["errors"]; has {
		t.Fatalf("non-validation error must not carry errors: %v", body)
	}
}
```

If `NewScoreHandler` does not accept `nil` (for example, because it takes an interface and dereferences it at construction), use the same fake the existing score tests use. Grep `internal/score` for one.

Create `apps/core-engine/internal/form/adapters/controllers/envelope_test.go`:

```go
package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRespondErrValidationKeepsErrorsAndGainsError(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	verrs := &domain.ValidationErrors{}
	verrs.Add("field a is required")

	if err := respondErr(c, verrs); err != nil {
		t.Fatal(err)
	}
	body := decode(t, rec)
	if rec.Code != http.StatusUnprocessableEntity || body["success"] != false || body["error"] != "validation failed" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if errs, ok := body["errors"].([]any); !ok || len(errs) != 1 || errs[0] != "field a is required" {
		t.Fatalf("errors = %v", body["errors"])
	}
}

func TestRespondErrNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := respondErr(c, fmt.Errorf("wrap: %w", domain.ErrNotFound)); err != nil {
		t.Fatal(err)
	}
	body := decode(t, rec)
	if rec.Code != http.StatusNotFound || body["success"] != false || body["error"] != "not found" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}
```

If `domain.ValidationErrors` is a value receiver or `Add` has a different signature, adapt the construction. The file is `internal/form/domain/errors.go:21`.

Create `apps/core-engine/internal/match/adapters/controllers/envelope_test.go`:

```go
package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/match/adapters/controllers"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
)

func TestMatchEvaluateBadJSONKeepsEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	h := controllers.NewMatchHandler(service.NewMatchService(repository.NewMatchRepo(nil), ""))
	if err := h.Evaluate(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body["success"] != false {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if msg, ok := body["error"].(string); !ok || !strings.HasPrefix(msg, "Invalid JSON payload") {
		t.Fatalf("error = %v", body["error"])
	}
}
```

reference-service has no controller tests. Its envelope is covered by
goplatform's `TestErrorEnvelopes` plus the Step 4 grep, which proves every
write goes through `JSONError`.

- [ ] **Step 2: Run the tests and see which fail**

Run: `go test ./apps/core-engine/internal/{form,score,match}/adapters/controllers/...`
Expected:
- `TestRespondErrValidationKeepsErrorsAndGainsError` FAILS, because `error` is absent today.
- The score and not-found tests may already pass, since those handlers already write `success:false`. They are there to pin the contract through the rewrite.

- [ ] **Step 3: Convert the handlers**

Apply these mappings in every listed file. Keep each message string and status code exactly as they are today:

| Today | Becomes |
|---|---|
| `c.JSON(S, map[string]interface{}{"success": false, "error": X})` (single or multi-line) | `goplatform.JSONError(c, S, X)` |
| `c.JSON(S, map[string]string{"error": X})` | `goplatform.JSONError(c, S, X)` |
| `c.JSON(S, map[string]interface{}{"errors": verrs.Errors})` and the `"success": false, "errors": verrs.Errors` form in `errors.go` | `goplatform.JSONValidationError(c, S, verrs.Errors)` |
| `survey_handlers.go` `EvaluateWithPhotos`, the `"error_code": "VISION_UNAVAILABLE"` body | `goplatform.JSONErrorCode(c, http.StatusServiceUnavailable, "VISION_UNAVAILABLE", err.Error())` |

For example, `respondErr` in `internal/form/adapters/controllers/errors.go` becomes:

```go
func respondErr(c echo.Context, err error) error {
	var verrs *domain.ValidationErrors
	if errors.As(err, &verrs) {
		return goplatform.JSONValidationError(c, http.StatusUnprocessableEntity, verrs.Errors)
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return goplatform.JSONError(c, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrConflict):
		return goplatform.JSONError(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidCode):
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUpstream):
		return goplatform.JSONError(c, http.StatusBadGateway, err.Error())
	default:
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
}
```

And the first error in `ScoreHandler.Evaluate` becomes:

```go
	if err := c.Bind(&req); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
	}
```

Add `"github.com/experience-gateway/packages/goplatform"` to each file's imports.

If a body in these files carries any key other than `success`, `error`, `errors` and `error_code`, leave that call alone and list it in the task report. Folding it in would drop a field.

- [ ] **Step 4: Verify that nothing was missed and nothing was lost**

```bash
go build ./apps/core-engine/... ./apps/reference-service/...
go vet ./apps/core-engine/... ./apps/reference-service/...
go test ./apps/core-engine/... ./apps/reference-service/...
grep -nE 'map\[string\](string|interface\{\})\{\s*"(error|errors|success)"|^\s*"(success|error|errors|error_code)":' \
  apps/core-engine/internal/{form,score,match}/adapters/controllers/*.go \
  apps/reference-service/internal/reference/controllers/*.go | grep -v _test.go || echo "all converted"
git diff --stat HEAD -- apps/core-engine/internal/vision
```

Expected: tests pass, the grep prints `all converted` (or only the calls you deliberately left and reported), and the vision diff is empty.

- [ ] **Step 5: Commit**

```bash
git add -A apps/core-engine/internal apps/reference-service/internal
git commit -m "refactor: one error envelope for form, score, match and reference

{success:false, error, error_code?, errors?} is a superset of both shapes
in use, so no client loses a field. The vision controller keeps its own
{error:<CODE>, message} shape.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Documentation

Claim `docs` for `README.md`, `AGENTS.md`, `docs/SPLIT.md`, `RAILWAY.md` and the lane map in `docs/LANES.md`.

**Files:**
- Modify: `README.md`, `AGENTS.md`, `docs/SPLIT.md`, `RAILWAY.md`, `docs/LANES.md` (the "Lane map" table only, never another session's claim row)

- [ ] **Step 1: Lane map**

In `docs/LANES.md`'s "Lane map" table, replace `apps/core-engine/pkg/form/`, `pkg/match/`, `pkg/score/` and `pkg/vision/` with `apps/core-engine/internal/form/`, `internal/match/`, `internal/score/` and `internal/vision/`. Add a row:

```markdown
| `goplatform` | `packages/goplatform/` |
```

- [ ] **Step 2: README.md**

In the "What is in here" table, replace the `packages/goenv/` row with:

```markdown
| `packages/goplatform/` | — | Shared Go bootstrap: env/.env, slog, Postgres (required), Redis (optional), `/health`, Echo server with graceful shutdown, error envelope. |
```

Add this section after "Running it":

```markdown
## Go service conventions

Both Go services are assembled from `packages/goplatform` the same way:
`cmd/server/main.go` loads `internal/config`, calls `goplatform.Bootstrap`,
wires its engines and calls `goplatform.Run`.

- **`DATABASE_URL` is required.** The service exits if Postgres is unreachable
  at startup. There is no in-memory fallback.
- **`/health`** returns `{"status","service","checks":{"database","redis"}}`,
  and **503** when the database is down. Redis is optional: `disabled` or `down`
  never fails health.
- **Errors** are `{"success":false,"error":"...","error_code"?,"errors"?}`.
  The vision controller keeps its own `{"error":"<CODE>","message"}` shape.
- **Tunables** (timeouts, cache TTLs, CORS, log level) are env vars with the
  documented defaults in each service's `internal/config/config.go` and in
  `packages/goplatform/common.go`. Durations use Go syntax (`15m`, `500ms`) and
  must be positive.
- **Packages live under `internal/`.** Nothing outside a service imports them.
```

- [ ] **Step 3: AGENTS.md, SPLIT.md, RAILWAY.md**

- `AGENTS.md`: no path in it names `pkg/`. Confirm with `grep -n "pkg/\|goenv" AGENTS.md` and fix any hit.
- `docs/SPLIT.md`: under "What changed beyond moving files", append to the `packages/goenv` bullet: `Seagull-core has since renamed its copy to packages/goplatform and grown it into the shared bootstrap (2026-09-26); Seagull-gateway's copy is still goenv.`
- `RAILWAY.md`: add this section before "What still spans repositories":

```markdown
## Health checks and startup (since 2026-09-26)

Core Engine and Reference Data now **exit on startup** if Postgres is
unreachable, and `/health` returns **503** while the database is down. A
Railway health check on `/health` will therefore restart them during a
database outage instead of letting them serve on a missing database. Both
require `DATABASE_URL`; there is no longer a default.
```

- [ ] **Step 4: Verify**

```bash
grep -rn "packages/goenv\|core-engine/pkg/\|reference-service/pkg/" --include='*.md' . \
  | grep -v "docs/superpowers/" | grep -v "SPLIT.md" || echo "no stale paths"
```

Expected: `no stale paths`. `SPLIT.md` legitimately mentions goenv as history.

- [ ] **Step 5: Commit and release the lanes**

```bash
git add README.md AGENTS.md docs/SPLIT.md RAILWAY.md
git add -p docs/LANES.md   # stage only the lane-map hunk, not claim rows
git commit -m "docs: Go service conventions, goplatform, new lane paths

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Then delete this session's claim rows from `docs/LANES.md`: the `reference`, the four `core-*`, `docs`, and the path claims from Tasks 1–4. Leave that edit uncommitted, as with every claim-table edit.

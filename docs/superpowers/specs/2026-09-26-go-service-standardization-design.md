# Go service standardization

Date: 2026-09-26
Status: design, awaiting review
Scope: `apps/core-engine`, `apps/reference-service`, `packages/goenv`.

This is sub-project **A** of the Seagull-core restructure. The others are:

- **B — Python worker merge.** Fold `vision-ai-worker` and `tryon-engine` into
  one `apps/ai-worker` codebase built as two images (CPU and CUDA). It is
  deliberately sequenced **after** the plug-and-play model registry plan
  (`docs/superpowers/plans/2026-09-26-plug-and-play-model-registry.md`) lands,
  because that plan rewrites the same files.
- **C — Repo tooling.** One task runner in place of npm-as-runner plus a
  Makefile wrapper; a compose entry for every service.

Replacing GORM `AutoMigrate` with versioned migrations is **out of scope**. The
database is shared with Seagull-gateway (`docs/SHARED-DATABASE.md`), so that
needs its own cross-repo design.

## The problem

The two Go services use the same stack (Go 1.25, Echo v4, GORM, go-redis, the
`goenv` helper). But they have drifted in everything around that stack:

| | core-engine | reference-service |
|---|---|---|
| Graceful shutdown | none | SIGTERM + `e.Shutdown` |
| Middleware order | Recover → Logger | Logger → Recover |
| CORS headers | `*` | explicit list |
| `/health` | always `"healthy"`, static engine map | `"ok"` + db/redis/port |
| Layout | `pkg/<engine>/{adapters,domain,service,...}` | flat `pkg/` |
| Logging | stdlib `log` + Echo logger | same |

The DB and Redis bootstrap is copied nearly verbatim between the two `main.go`
files. core-engine uses two error JSON shapes: `form/adapters/controllers/errors.go`
writes `{"success":false,"error"}`, while the survey, score and match handlers
write `{"error"}`.

Two behaviours break the ground rules in `AGENTS.md`:

1. **A silent degraded mode.** If Postgres is unreachable, both services log
   "running with in-memory fallback" and serve requests anyway, with a `nil`
   `*gorm.DB`. core-engine's `/health` still reports `"healthy"`. The same goes
   for config: `config.Load()` errors are logged as warnings and ignored.
2. **Unflagged literals** at call sites:

   | Where | Value |
   |---|---|
   | `cmd/server/main.go` (core) | form cache TTL 15m, GORM slow threshold 2s, Redis ping 3s |
   | `cmd/server/main.go` (reference) | reference cache TTL 10m, shutdown 10s |
   | `pkg/score/adapters/cache/score_cache.go:18` | 15m |
   | `pkg/match/adapters/cache/match_cache.go:18` | 15m |
   | `pkg/form/adapters/scoreclient/client.go:29` | 5s |
   | `pkg/form/adapters/visionclient/client.go:34` | 50s (Paradev) |
   | `pkg/vision/catalog/reference_client.go:12` | 3s |
   | `pkg/vision/inference/client.go:19` | 20s |
   | `pkg/vision/orchestrator/dag_engine.go:69` | 30s |
   | `pkg/vision/workers/segmenter_worker.go:61` | 4s |
   | `cmd/server/main.go` (core) | vision catalog cache TTL `time.Minute` |
   | both `pkg/config/config.go` | default `DATABASE_URL` with embedded credentials |
   | `pkg/match/service/match_service.go:348` | `os.Getenv("TRYON_ENGINE_URL")` bypasses config |

## What this design changes

The stack does not change. Echo, GORM, go-redis and Go 1.25 all stay. What
changes is that both services are **assembled the same way**, from one shared
bootstrap package, and their Go packages move under `internal/`.

### Layout

```
packages/goplatform/          renamed from packages/goenv, grows the shared bootstrap
  env.go                      today's goenv.go, unchanged API
  log.go                      slog JSON handler; GORM logger adapter over slog
  db.go                       OpenPostgres(ctx, url, opts) (*gorm.DB, error)
  redis.go                    OpenRedis(ctx, url, opts) (*redis.Client, error)
  server.go                   NewEcho(opts) and Run(ctx, e, addr, shutdownTimeout)
  health.go                   Health handler over named Check funcs
  httperr.go                  the error envelope helpers

apps/core-engine/
  cmd/server/main.go          config → platform bootstrap → wire engines → Run
  internal/config/
  internal/form/  internal/score/  internal/match/  internal/vision/
                              today's pkg/<engine>, moved with no logic changes

apps/reference-service/
  cmd/server/main.go
  cmd/seed-reference-data/main.go
  internal/config/
  internal/reference/{cache,controllers,entity,repository}
                              today's flat pkg/, moved as one package tree
```

- **`pkg/` → `internal/`.** Go then enforces that nothing outside the module
  imports these packages. The move is mechanical (import-path rewrites only), so
  `git diff -M` shows it as renames. reference-service does **not** gain
  domain/service layers. At about 1,250 lines it does not need them.
- **`goenv` is renamed to `goplatform`, not duplicated.** It is already wired
  through `go.work` and a `replace` in each `go.mod`, so the rename changes two
  `replace` lines, two `require` lines, `go.work` and two Dockerfile `COPY`
  lines. The copy of goenv in Seagull-gateway is untouched. `docs/SPLIT.md`
  already accepts that the two copies may drift.
- **Module paths stay `github.com/experience-gateway/...`**, for the reason
  `docs/SPLIT.md` gives.

### What `goplatform` standardizes

- **Logging.** `log/slog` with a JSON handler, level from `LOG_LEVEL`. Echo
  request logging uses `middleware.RequestLoggerWithConfig` writing to slog.
  GORM's logger is an adapter over the same slog logger. Every log line carries
  `service`.
- **Middleware order:** Recover → RequestID → request logger → CORS. CORS
  origins, methods and headers come from config
  (`CORS_ALLOW_ORIGINS`, `CORS_ALLOW_HEADERS`). The mechanism is shared, and
  **each service keeps today's values as its default**: core-engine allows
  headers `*`, and reference-service keeps its explicit list. Narrowing
  core-engine's headers is a policy change for a later decision, not part of
  standardization.
- **Graceful shutdown.** `Run` starts the server, waits for SIGINT or SIGTERM,
  then calls `e.Shutdown` with a configured timeout. core-engine gains this
  behaviour.
- **Health.** `GET /health` returns

  ```json
  {"status":"ok","service":"core-engine","checks":{"database":"up","redis":"disabled"}}
  ```

  Each check is `up`, `down` or `disabled`. The status is **503** with
  `"status":"degraded"` when any required check is `down`. The per-engine
  `/<prefix>/health` routes in core-engine stay and delegate to the same
  handler.
- **Fail fast.**
  - `DATABASE_URL` becomes **required**. The credentialed default DSN is
    removed, and the root `.env` supplies it for local runs.
  - If Postgres cannot be opened and pinged at startup, the process exits
    non-zero with the error.
  - `config.Load()` errors are fatal.
  - Redis stays optional. An unset `REDIS_URL` gives `disabled`. A set but
    unreachable one gives `down` at startup, and the service still starts,
    because every cache already tolerates a `nil` client. `/health` shows the
    state.
- **Error envelope.** One helper, `goplatform.JSONError(c, status, msg)`, plus
  `JSONValidationError(c, errs)`, writes

  ```json
  {"success": false, "error": "<message>", "errors": [ ... ]}
  ```

  `errors` appears only on validation failures. This is a superset of both
  shapes in use today, so every existing client keeps reading `error`. Clients
  of `form/errors.go` keep reading `success`. No field is removed or renamed.
  `JSONErrorCode` adds `error_code` for the one handler that already sends it
  (`EvaluateWithPhotos`, `VISION_UNAVAILABLE`).

  **The vision controller is excluded.** Its bodies are
  `{"error":"<CODE>","message":"<text>"}`. There, `error` is a machine code,
  not a message, so folding it into the envelope would change what that field
  means to its clients. It keeps its shape.

  **reference-service is included.** Its `{"error":msg}` bodies gain
  `success:false`, which is additive.

### Configuration

Every literal in the table above becomes a `Config` field read through
`goplatform`, with **today's value as the documented default**. Nothing
deployed has to set a new variable.

- **core-engine.** `FORM_CACHE_TTL`, `SCORE_CACHE_TTL`, `MATCH_CACHE_TTL`,
  `SCORE_CLIENT_TIMEOUT`, `PARADEV_TIMEOUT`, `REFERENCE_CLIENT_TIMEOUT`,
  `VISION_DISPATCH_TIMEOUT`, `VISION_DAG_TIMEOUT`, `SEGMENTER_TIMEOUT`,
  `VISION_CATALOG_TTL`,
  `TRYON_ENGINE_URL`.
- **Both services.** `DB_SLOW_QUERY_THRESHOLD`, `DEPENDENCY_PING_TIMEOUT`
  (the Postgres and Redis ping at startup and in `/health`),
  `SHUTDOWN_TIMEOUT`, `CORS_ALLOW_ORIGINS`, `CORS_ALLOW_HEADERS`, `LOG_LEVEL`.
- **reference-service.** `REFERENCE_CACHE_TTL`.

Durations are parsed with `time.ParseDuration`. A malformed or non-positive
value is a startup error, not a silent fallback. (A zero TTL would mean
"never expire" to Redis.) Constructors that today default a zero value
internally (`score_cache.go`, `match_cache.go`, `visionclient`) take the value
from config instead, and the internal defaults are removed so there is exactly
one definition of each.

### What does not change

- Ports, route prefixes and **all route aliases** (`/core/score-engine`,
  `/api/scoring`, `/api/score`, `/v1`, and so on). The gateway and the web app
  depend on them.
- `AutoMigrate` lists and GORM models. The `cmd/server/main.go` paths cited by
  `docs/SHARED-DATABASE.md` stay valid.
- Railway service config and Dockerfile paths. Only the `COPY packages/goenv`
  line changes.
- The self-calls over `http://127.0.0.1:<port>`. Those are a design question of
  their own, not a standardization one.

## Sequence

Each step is one commit. `go build ./...` and `go test ./...` pass for both
modules before the next step starts. Each step also runs the Dockerfile's own
build command (`CGO_ENABLED=0 GOOS=linux go build`). Docker is not installed
on the development machine, so **the image builds are verified by the user or
by Railway**, not by the implementation.

1. **Rename** `packages/goenv` → `packages/goplatform`. Update the `replace`,
   `require`, `go.work` and Dockerfile `COPY` lines. No behaviour change.
2. **Add the bootstrap** to `goplatform`: `log.go`, `db.go`, `redis.go`,
   `server.go`, `health.go`, `httperr.go`, with unit tests. Nothing calls it
   yet.
3. **reference-service**: `pkg/` → `internal/`. Switch both `cmd/` entrypoints
   to `goplatform`. Move its literals to config. It is small and already has
   graceful shutdown, so it proves the pattern.
4. **core-engine**: `pkg/` → `internal/`. Switch `main.go` to `goplatform`.
   Move all literals and `TRYON_ENGINE_URL` into config.
5. **Error helpers**: replace per-controller `c.JSON` error writes in
   core-engine's form, score and match controllers and in reference-service's
   controller with `goplatform.JSONError` / `JSONErrorCode` /
   `JSONValidationError`. The vision controller is left alone (see above).
6. **Docs**: `README.md`, `AGENTS.md`, and the lane map in `docs/LANES.md`
   (`apps/core-engine/pkg/<engine>/` → `apps/core-engine/internal/<engine>/`).

Step 4 touches every core-engine lane (`core-form`, `core-match`, `core-score`,
`core-vision`). Claim all four together before starting it.

## Testing

- **goplatform unit tests.**
  - The health handler returns 200 when every check is `up` or `disabled`, and
    503 when a required check is `down`.
  - The error envelope is exact, and `errors` appears only for validation.
  - `Run` returns after a cancelled context, within the shutdown timeout.
  - A malformed duration makes config loading fail.
- **Contract tests in core-engine.** For one handler per engine, assert that
  an error response still has an `error` string. Form's also has
  `success: false`. These pin the backward-compatibility claim.
- **Existing tests.** All 30 existing `_test.go` files in core-engine and the
  config test in reference-service pass unchanged, apart from import paths.
- **Manual check.** Run both services against the shared Postgres. Hit
  `/health` on each and one route per alias group. Stop Postgres and confirm
  `/health` returns 503. Start a service with Postgres stopped and confirm it
  exits non-zero.

## Rollout and risk

Nothing reaches Railway until the user pushes. After that, per `RAILWAY.md`,
redeploy Reference Data first, confirm healthy, then Core Engine.

- **`/health` can now return 503.** If Railway's health check targets
  `/health`, a Postgres outage now restarts the service instead of being hidden
  behind a `nil` database. This is intended.
- **A missing `DATABASE_URL` now fails startup.** Both Railway services already
  set it. Check with `list-variables` before the first deploy.
- **The reference seeder's slow-query threshold** moves from its own 500ms
  literal to the shared `DB_SLOW_QUERY_THRESHOLD` (default 2s). It only affects
  which seeder queries are logged as slow.
- **Log format changes** from stdlib text to JSON. Anything that parses the
  Railway logs as text (ClickHouse Log ingestion, saved searches) will see a
  different shape.

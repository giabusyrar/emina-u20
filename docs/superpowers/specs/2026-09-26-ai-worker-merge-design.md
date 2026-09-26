# ai-worker: one Python codebase, two router profiles

Date: 2026-09-26
Status: implemented on branch ai-worker-merge
Scope:
- merges `apps/vision-ai-worker` and `apps/tryon-engine` into a new `apps/ai-worker`;
- `package.json`, `scripts/`, `docker-compose.yml` and root `pytest.ini`;
- docs that name the old paths: `README.md`, `RAILWAY.md`, `AGENTS.md`,
  `docs/{MODEL-REGISTRY,SHARED-DATABASE,PORTS,SPLIT,LANES}.md` and
  `.claude/launch.json`.

This is sub-project **B** of the Seagull-core restructure. A (Go services) and
C (repo tooling) are on `main`. The model registry landed on `main` in
`74124e2`, which is what unblocked this.

## The problem

Two FastAPI services live side by side and have drifted apart:

| | vision-ai-worker | tryon-engine |
|---|---|---|
| Size | ~5,700 LOC; 125 passed + 15 skipped + 6 benchmark (deselected) | ~640 LOC; 6 passed, ~8 min (downloads models) |
| Package | top-level `app` | top-level `app` (the same name) |
| Dependencies | `pyproject.toml` `[project]` + `test` extra; `requirements.txt` mirrors the runtime deps | no `[project]`; `requirements.txt` mixes pytest into the runtime deps; torch, transformers and diffusers are unconditional |
| Startup | `@app.on_event` (deprecated) | none |
| Config | hand-rolled `.env` walker + `Settings` | the same walker, copy-pasted |
| Storage | `_get_client()`, upload/download/delete/presign, local-disk fallback | `_get_client()` copied verbatim, upload only |
| Logging | `logging` | none; job failures are swallowed into the callback payload |
| Health | `GET /healthz` | `GET /health` |
| Image | **no Dockerfile** | `nvidia/cuda:12.1.0-runtime-ubuntu22.04`, Python 3.11 |
| Deployed | nowhere yet (`RAILWAY.md`: not on Railway) | nowhere yet |

Because both packages are named `app`, a root `pytest` run cannot collect them:
whichever is imported first shadows the other. `pytest.ini` documents this
workaround at length.

## Contracts this change must not break

- **core-engine → vision:**
  - `POST {VISION_AI_WORKER_URL}/api/v1/segment-and-pose`, called from
    `apps/core-engine/internal/vision/workers/segmenter_worker.go`.
  - `POST {VISION_AI_WORKER_URL}/api/v1/models/dispatch-zones`, called from
    `apps/core-engine/internal/vision/inference/client.go`.
- **Web and admin → vision:** every other `/api/v1/models/...` route stays as
  it is today. That includes the deprecated 410 routes.
- **core-engine → tryon:** `POST {TRYON_ENGINE_URL}/extract`.
- **tryon → core-engine:**
  `POST {MATCH_ENGINE_URL}/api/matching/shades/{shade_id}/extraction-callback`.
- **Ports:** vision on 8088, tryon on 8090 (`docs/PORTS.md`).
- **Env var names:** every name either service reads today keeps working.
- **Registry schema:** the tables, the advisory lock id and the migration
  behaviour in `app/db.py` move unchanged. The registry's "deploy is one-way"
  warning in `docs/MODEL-REGISTRY.md` still applies.

core-engine is **not modified**. Its URLs, env names and paths stay valid.

## Design

### Layout

```
apps/ai-worker/
  pyproject.toml          single source of dependencies (no requirements.txt)
  Dockerfile              targets: cpu, gpu
  ai_worker/
    __init__.py
    main.py               create_app(): lifespan, logging, CORS, /health, mounts profiles
    core/
      settings.py         one lazily-read Settings (union of both); every default named once
      storage.py          one S3 client + local-disk fallback (today's vision storage.py)
      logging.py          JSON-lines stdlib logging with `service` and `profile`
      health.py           /health (+ vision's /healthz alias)
    vision/               today's apps/vision-ai-worker/app/* (git mv), imports rewritten;
                          internal layout unchanged (routers/, models/, schemas/, db.py, zones.py)
      routers/segment.py  segment-and-pose + its detector singletons (moved out of main.py)
      profile.py          ROUTERS, startup(), shutdown(), health_checks()
    tryon/                today's apps/tryon-engine/app/* (git mv), imports rewritten;
                          internal layout unchanged (routers/extraction.py, services/, schemas.py)
      profile.py          ROUTERS, startup(), shutdown(), health_checks()
  tests/
    conftest.py
    core/  vision/  tryon/
```

- **Package name `ai_worker`, not `app`.** This removes the collision, so
  `pytest apps/ai-worker` collects everything in one run.
- **Moves use `git mv`** so history follows the files. Content changes happen
  in separate commits from the moves.
- **vision's `/api/v1/segment-and-pose` handler moves from `main.py` into
  `vision/router.py`.** The path, request and response are unchanged, and the
  detector singletons move with it.

### Router profiles

`AI_WORKER_PROFILES` is a comma-separated list, a subset of `vision,tryon`, read
by `core/settings.py`.

- **Unset:** the Dockerfile target chooses the default (below). For a bare
  `uvicorn` run, the default is `vision`, which matches today's
  `npm run dev:vision`.
- **Load behaviour:** `create_app()` imports each profile's router module only
  if that profile is enabled. The tryon profile imports torch at module top
  level, which today's tryon code already does. So a vision-only process never
  imports torch.
- **Missing dependency:** if a profile's dependency is not installed (tryon
  without the `tryon` extra), startup fails with one clear error that names
  the missing extra. It does not fail later with an `ImportError` on the first
  request.
- **Unknown names:** an unknown profile name is a startup error.
- **Lifespan:** the vision profile's lifespan work, `init_pool()` and
  `close_pool()`, runs only when vision is enabled.

### Service behaviour (standardized with the Go services)

- **Startup:** a `lifespan` context manager replaces `@app.on_event`.
- **Logging:** `core/logging.py` configures stdlib `logging` to write JSON lines
  with `service="ai-worker"`, `profile=<profiles>`, level, logger and message.
  - The level comes from `LOG_LEVEL`, default `INFO`, the same variable the Go
    services use. An invalid level is a startup error.
  - `tryon`'s background job logs failures at ERROR, with the exception, before
    it posts the `failed` callback.
- **Health:** `GET /health` returns
  `{"status","service","profiles","checks":{...}}`.
  - With vision enabled and `DATABASE_URL` set, `checks.database` is `up` or
    `down`. **`down` returns 503.** This matches the Go services:
    `dispatch-zones` cannot work without the registry.
  - With `DATABASE_URL` unset, `database` is `disabled`, which is not a
    failure. This preserves today's local-dev behaviour.
  - With tryon enabled, `checks.gpu` is `available` or `unavailable`. This is
    informational and never a failure.
  - `GET /healthz` remains an alias when vision is enabled, because vision's
    existing callers use it.
- **Config:** `core/settings.py` is **one shared, lazily-read `Settings`
  class**, the union of today's two classes. It uses the same env names:
  - vision: `DATABASE_URL` / `DATABASE_PUBLIC_URL` / `POSTGRES_URL`,
    `VISION_MODEL_STORAGE_DIR`, `VISION_AI_WORKER_URL`, `MODEL_*`;
  - both: `S3_*`;
  - tryon: `MATCH_ENGINE_URL`, `FACE_PARSING_MODEL_ID`;
  - plus `PORT`, and the new `LOG_LEVEL`, `AI_WORKER_PROFILES` and
    `HEALTH_DB_TIMEOUT_SECONDS`.

  Every other rule is unchanged:
  - The `DATABASE_URL` → `DATABASE_PUBLIC_URL` → `POSTGRES_URL` fallback order
    is kept.
  - Defaults keep today's values and are defined once.
  - `require_s3()` and `require_database()` keep their behaviour. The S3
    message is vision's wording.
  - The `.env` walker is kept, as one copy. `python-dotenv` becomes a declared
    dependency; vision used it without declaring it.

  *Amended from `pydantic-settings`:* that library reads env once, when the
  object is built. Vision relies on reading env on each access, which lets
  segment-and-pose run before S3 or the database is configured. Its tests set
  env at runtime (`test_db_schema.py`) and patch properties on the class
  (`test_model_validation.py`, `test_model_upload.py`). A frozen settings
  object would break both.
- **Local model storage:** the default `VISION_MODEL_STORAGE_DIR` becomes
  `apps/ai-worker/data/models`, replacing `apps/vision-ai-worker/data/models`,
  which was never created on the development machine. It is added to
  `.gitignore`.
- **Storage:** vision's `storage.py` becomes `core/storage.py` and tryon uses it.
  - Tryon's `upload_bytes` currently returns a constructed public URL, which
    vision's version does not. That behaviour moves into `core/storage.py` as
    `public_url(key)`, so tryon's callback payload is byte-identical.
  - Tryon keeps a thin `upload_bytes(key, data, content_type)` wrapper that
    calls `settings.require_s3()` first. Vision's local-disk fallback must not
    apply to tryon: tryon hands its URLs to the web app, and a local-disk write
    would return a URL that does not exist.
- **Errors:** FastAPI's `{"detail": ...}` stays the wire format. Nothing changes
  for clients.

### Dependencies (`apps/ai-worker/pyproject.toml`)

- `name = "ai-worker"`, `requires-python = ">=3.12"`.
- **`[project].dependencies`:** vision's current 11 runtime packages plus
  `python-dotenv`.
- **`[project.optional-dependencies]`:**
  - `tryon`: torch, torchvision, transformers, diffusers, accelerate, pillow,
    scipy. Lower bounds come from today's tryon `requirements.txt`, which has no
    pins, so take the versions installed on the development machine as floors.
  - `test`: pytest, pytest-asyncio, pytest-benchmark, onnx, torch and
    torchvision, the same as vision's current extra.
- **`[tool.pytest.ini_options]`:**
  - `asyncio_mode = "auto"` and `addopts = -m "not benchmark"`;
  - markers `benchmark` and `tryon`;
  - `testpaths = ["tests"]`.
- **Removed:** both `requirements.txt` files. The dependency lists live only in
  `pyproject.toml`.
- **Install:** local development uses
  `pip install -e "apps/ai-worker[test,tryon]"`. `uv` is not installed on the
  development machine, so it is not required. The `pyproject` works with
  `uv pip` unchanged if it is adopted later.

### Image (`apps/ai-worker/Dockerfile`)

- **`cpu` target:**
  - Base `python:3.12-slim`, plus the system libraries OpenCV and mediapipe
    need (`libgl1`, `libglib2.0-0`).
  - Installs `.` with no extras.
  - `ENV AI_WORKER_PROFILES=vision PORT=8088`.
  - Command: `uvicorn ai_worker.main:app --host 0.0.0.0 --port ${PORT}`,
    written in shell form so `$PORT` expands. Railway injects `PORT`.
- **`gpu` target:**
  - Base `nvidia/cuda:12.1.0-runtime-ubuntu22.04`, the same as today, with
    Python 3.12 from the deadsnakes PPA.
  - Installs `.[tryon]`.
  - `ENV AI_WORKER_PROFILES=tryon PORT=8090`.
  - Same command.
- **Build context:** the repository root, so compose and Railway use one
  Dockerfile path: `apps/ai-worker/Dockerfile`.
- Neither image contains `test` extras.

### Tests

- `apps/vision-ai-worker/tests/*` moves to `apps/ai-worker/tests/vision/`.
  `apps/tryon-engine/tests/*`, including its fixture image, moves to
  `apps/ai-worker/tests/tryon/`. Imports are rewritten from `app.` to
  `ai_worker.vision.` and `ai_worker.tryon.`.
- **Markers:**
  - `tests/tryon/` modules carry `pytest.mark.tryon`.
  - `tests/tryon/conftest.py` skips the whole directory when torch is not
    importable, with a stated reason.
  - The existing `benchmark` marker is unchanged.
- **New tests in `tests/core/`:**
  - profile selection: an unknown profile fails, vision-only never imports
    `ai_worker.tryon`, and a missing extra names the extra;
  - `/health` semantics: 503 only for vision with the database configured and
    down;
  - settings: env names and fallback order, and an invalid `LOG_LEVEL` fails;
  - `public_url` equals tryon's old URL construction.
- **Baselines that must hold after the move:**
  - vision: **125 passed, 15 skipped, 6 deselected**;
  - tryon: **6 passed**, where torch is installed.

  New core tests are counted separately.

### Scripts, compose, docs

- **`package.json`:** script names are unchanged; commands are updated.
  - `dev:vision` and `dev:tryon` call `node scripts/ai-worker.mjs dev vision` and
    `node scripts/ai-worker.mjs dev tryon`. The helper sets `AI_WORKER_PROFILES`
    and `PORT` and runs `python -m uvicorn ai_worker.main:app --app-dir apps/ai-worker --reload --port <port>`.
    Env vars are set in Node because npm uses `cmd.exe` on Windows.
  - `test:vision`: `pytest apps/ai-worker -m "not benchmark and not tryon"`.
    This covers `tests/vision` and `tests/core`. `test` and `check` keep calling
    it unchanged.
  - `test:tryon`: `pytest apps/ai-worker -m tryon`.
  - `bench:vision`: `pytest apps/ai-worker/tests/vision/test_dispatch_benchmark.py -m benchmark --benchmark-only`.
- **`pytest.ini`:** delete it. The collision it documents is gone, and
  `apps/ai-worker/pyproject.toml` declares the markers.
- **`docker-compose.yml`:**
  - Add `ai-worker-vision`: target `cpu`, port 8088, `env_file: .env`,
    `DATABASE_URL`, healthcheck on `/health` using Python's `urllib`, because
    the slim image has no `wget` or `curl`.
  - Add `ai-worker-tryon`: target `gpu`, port 8090, `profiles: ["gpu"]`, and a
    `deploy.resources.reservations.devices` GPU request.
  - `VISION_AI_WORKER_URL` defaults to `http://ai-worker-vision:8088`.
    `TRYON_ENGINE_URL` defaults to `http://ai-worker-tryon:8090`.
  - The `host.docker.internal` comment block and `extra_hosts` from part C are
    removed, because they are no longer needed.
- **Docs:**
  - Update paths in `README.md`, `RAILWAY.md`, `AGENTS.md`,
    `docs/{MODEL-REGISTRY,SHARED-DATABASE,PORTS}.md` and the lane map in
    `docs/LANES.md`: lanes `vision-ai` and `tryon` become `ai-worker-vision`
    (`apps/ai-worker/ai_worker/vision/`, `tests/vision/`), `ai-worker-tryon`
    and `ai-worker-core`.
  - `docs/SPLIT.md` gets a history line.
  - `.claude/launch.json` keeps `npm run dev:vision`, so it needs no change.
  - The core-engine Go comments that say "tryon-engine" describe a role, not a
    path, so they are left alone.

## Order of work

1. Scaffold `apps/ai-worker/pyproject.toml` and move vision with `git mv`
   (`app/` → `ai_worker/vision/`, `tests/` → `tests/vision/`). Rewrite imports
   and create a minimal `ai_worker/main.py` that serves exactly today's vision
   app. The vision baseline must hold.
2. Move tryon with `git mv` and rewrite imports. The tryon baseline must hold,
   and both suites must collect in one `pytest apps/ai-worker` run.
3. Add `core/settings.py` (one shared lazy Settings) and replace both `Settings`
   classes, keeping the env names and fallbacks, with tests.
4. Add `core/storage.py`: one client and `public_url`, with tryon switched to
   it, with tests.
5. Add `main.py` profiles, lifespan, `core/logging.py` and `core/health.py`,
   with tests. Tryon logs its job failures.
6. Dockerfile (cpu and gpu), `scripts/ai-worker.mjs`, `package.json`, compose;
   delete `pytest.ini`, both old `requirements.txt` files and the empty old
   directories.
7. Docs.

Each step ends green:
- `pytest apps/ai-worker -m "not benchmark and not tryon"` passes;
- the tryon suite passes, run after steps 2, 4, 5 and 7, because it takes
  about 8 minutes;
- `npm run check` passes.

## Out of scope

- Deploying either image anywhere; nothing is deployed today.
- Changing core-engine (URLs, env names, the vision client).
- Vision's internal registry design, API shapes and schema.
- An error-envelope change for the Python services.
- `uv` adoption and lockfiles for Python.

## Verification limits

Docker is not installed on the development machine. `docker build --target cpu`,
`docker build --target gpu` and `docker compose up` are for the user to run.
Local verification covers:
- the test suites;
- running `npm run dev:vision` and hitting `/health` and `/healthz` on port
  8088, if the runtime lane and port are free;
- starting a tryon-profile process with torch installed and hitting `/health`
  on port 8090.

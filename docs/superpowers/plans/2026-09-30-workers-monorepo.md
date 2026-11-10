# Python Workers as Separate Apps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `apps/ai-worker` (one package, profiles `vision`/`tryon`/`colour`) into `apps/workers/{common,skin,face,colour,tryon}`: one shared library plus one app, one Dockerfile and one test suite per worker.

**Architecture:** `worker_common` holds what every worker shares (base settings, S3 client, JSON logging, `/health`, the FastAPI factory, the model-mirror engine). Each worker is a package `worker_<name>` with a `worker.py` module (the old `profile.py` contract plus `SERVICE_NAME`, `REMOVED_ENV`, `mirror_needs()`), its own `settings.py`, `main.py`, tests and Dockerfile. Face-measure leaves the old `vision` profile for `worker_face`; the rest of `vision` becomes `worker_skin`. core-engine gets `SKIN_WORKER_URL` and `FACE_WORKER_URL` and refuses the removed `VISION_AI_WORKER_URL`.

**Tech Stack:** Python 3.12, FastAPI, pytest (+pytest-asyncio), setuptools; Go 1.25 (core-engine config); Docker; npm scripts (Node built-ins).

**Spec:** `docs/superpowers/specs/2026-09-30-workers-monorepo-design.md`

## Global Constraints

- Layout: `apps/workers/common` (package `worker_common`, a library, never a service), `apps/workers/skin` (`worker_skin`, :8088), `apps/workers/face` (`worker_face`, :8094), `apps/workers/colour` (`worker_colour`, :8092), `apps/workers/tryon` (`worker_tryon`, :8090, GPU).
- Public API unchanged: `/core/vision-engine/...` (including face-architecture) stays on core-engine; no gateway or Seagull-web change.
- No database schema change; `vision_models` and the registry tables keep their names.
- A worker may import `worker_common` and itself only; `worker_common` imports no worker.
- Env renames, no silent fallback: `VISION_MODEL_STORAGE_DIR`→`SKIN_MODEL_STORAGE_DIR`; skin's own `VISION_AI_WORKER_URL`→`SKIN_WORKER_PUBLIC_URL`; `VISION_FACE_LANDMARKER_PATH`→`FACE_LANDMARKER_PATH` (face); colour's `FACE_LANDMARKER_PATH` override→`COLOUR_LANDMARKER_PATH`; `AI_WORKER_PROFILES` removed. core-engine: `SKIN_WORKER_URL` (default `http://127.0.0.1:8088`), `FACE_WORKER_URL` (default `http://127.0.0.1:8094`); a set `VISION_AI_WORKER_URL` is a config error naming both.
- Unchanged names: `COLOUR_WORKER_URL`, `TRYON_ENGINE_URL`, `MATCH_ENGINE_URL`, `FACE_MEASURE_*`, `WCPA_*`, `MODEL_*`, `LASH_TEMPLATE_*`, `RENDER_PARAMS_PATH`, `FACE_PARSING_MODEL_ID`, `S3_*`, `DATABASE_URL`, `LOG_LEVEL`, `HEALTH_DB_TIMEOUT_SECONDS`.
- Files move with `git mv` (per-file history). Tests move with their code and are adapted only for new import paths and the new factory/mirror APIs, never to make a failing assertion pass.
- The passing-test count is recorded before and after every task; a changed count is explained in the report.
- Each worker's `tests` package is named `tests` and is run in its own pytest process (`pytest apps/workers/<name>`), never together with another worker's suite.
- Colour (`ai_worker/colour`, `tests/colour`) is the JEV session's lane. Task 7 starts only after that session agrees; no other task depends on it.
- AGENTS.md: never `git push`; claim lanes in `docs/LANES.md` (if the edit is refused by the permission layer, report it and continue); no hardcoded hosts/ports outside named, documented defaults; a placeholder must announce itself.

## Additions to the spec (decided while planning)

- `FACE_MODEL_DIR` (new, face worker): its model directory, default `apps/workers/face/data/models`; the image sets `/app/data/models`. Previously the landmarker default hung off the vision storage dir.
- Colour does not refuse `FACE_LANDMARKER_PATH`: it is the face worker's valid name in a shared `.env`; colour reads `COLOUR_LANDMARKER_PATH` and ignores it.
- Worker pyprojects do not declare `worker-common` (a relative `file:` requirement is not valid PEP 508); installs name both packages.
- `/health` bodies drop the `profiles` key; `service` is `worker-<name>`. Logs drop the `profile` field.

Task order: 0 → 1 → {2 anytime} → 3 → 4 → 5 → 6 → 7 (gated) → 8 → 9.

## File Map

| Path | Responsibility |
|---|---|
| `apps/workers/common/worker_common/settings.py` | `BaseSettings` (DB, S3, log level, health timeout, `_int_env`/`_float_env`), `.env` discovery, `REMOVED_ENV`, `check_removed_env()` |
| `apps/workers/common/worker_common/{logging,health,app,storage,model_mirror}.py` | JSON logs; `/health`; `create_app(worker)`; S3-only storage; generic mirror engine + CLI |
| `apps/workers/common/tests/` | tests for the above + the cross-worker import test |
| `apps/workers/<name>/worker_<name>/{worker,main,settings}.py` | worker contract, ASGI app, worker settings |
| `apps/workers/<name>/{pyproject.toml,Dockerfile,tests/}` | per-worker packaging, image, tests |
| `apps/core-engine/internal/config/config.go` (+ test), `cmd/server/main.go`, `cmd/facearch-profile/main.go`, `cmd/face-benchmark/main.go`, `internal/facearch/measureclient/client.go` | `SKIN_WORKER_URL`/`FACE_WORKER_URL` |
| `docker-compose.yml`, `scripts/worker.mjs`, `package.json`, `.gitignore`, `.dockerignore` | wiring and tooling |
| `docs/*.md`, `README.md`, `AGENTS.md` | documentation |

## Worker module contract (used by every task)

```python
# worker_<name>/worker.py
SERVICE_NAME = "worker-<name>"          # "service" in logs and /health
REMOVED_ENV: dict[str, str] = {...}     # old env name -> what replaces it (may be empty)
ROUTERS = [...]                         # FastAPI routers
async def startup() -> None: ...
async def shutdown() -> None: ...
async def health_checks() -> dict[str, tuple[str, bool]]: ...   # name -> (state, required)
def mirror_needs() -> "worker_common.model_mirror.MirrorNeeds": ...
```

```python
# worker_<name>/main.py
from worker_common.app import create_app
from worker_<name> import worker

app = create_app(worker)
```

---

### Task 0: Workspace and baseline

- [ ] **Step 1:** Create the worktree: `git -C C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull-core worktree add ../seagull-core-worktrees/workers-split -b refactor/workers-split main`. All later tasks run in `C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull-core-worktrees/workers-split` ("the worktree").
- [ ] **Step 2:** Record the baseline in the worktree and keep the output for the reports:

```bash
python -m pytest apps/ai-worker -q -p no:cacheprovider 2>&1 | tail -1
python -m pytest apps/ai-worker -m tryon -q -p no:cacheprovider 2>&1 | tail -1
python -m pytest apps/ai-worker -m colour -q -p no:cacheprovider 2>&1 | tail -1
go test ./apps/core-engine/... 2>&1 | grep -c "^ok"
```

The tryon run takes ~12 min; run it in the background. Skips caused by files absent from a fresh worktree (FaceLandmarker model, TEST_DATABASE_URL) are expected; note their count.

---

### Task 1: `worker_common`

**Files:**
- Create: `apps/workers/common/pyproject.toml`, `apps/workers/common/worker_common/{__init__,settings,logging,health,app,storage,model_mirror}.py`
- Create: `apps/workers/common/tests/{__init__,conftest,test_settings,test_app,test_storage,test_model_mirror,test_boundaries}.py`
- The old `apps/ai-worker/ai_worker/core/` stays untouched until Task 8 (profiles not yet moved still use it).

**Interfaces:**
- Produces: `worker_common.settings.{BaseSettings, settings, check_removed_env, REMOVED_ENV}`; `worker_common.app.create_app(worker: ModuleType) -> FastAPI`; `worker_common.storage.{upload_bytes, download_bytes, delete_object, presigned_download_url, _get_client}` (S3 only); `worker_common.model_mirror.{MirrorNeeds, MirrorError, sync, main, LANDMARKER_KEY, MANIFEST_KEY, HF_PREFIX}`; `worker_common.logging.configure_logging(service: str, level: str)`; `worker_common.health.health_router(service: str, check_fns)`.

- [ ] **Step 1: `pyproject.toml`**

```toml
[project]
name = "worker-common"
version = "0.1.0"
description = "Shared library for the Seagull Python workers (apps/workers/*). Not a service."
requires-python = ">=3.12"
dependencies = [
    "fastapi>=0.110.0",
    "uvicorn[standard]>=0.28.0",
    "pydantic>=2.6.0",
    "python-multipart>=0.0.9",
    "boto3>=1.34.0",
    "python-dotenv>=1.0.0",
    "httpx>=0.27.0",
]

[project.optional-dependencies]
test = ["pytest>=8.0.0", "pytest-asyncio>=0.23"]

[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[tool.setuptools.packages.find]
include = ["worker_common*"]

[tool.pytest.ini_options]
asyncio_mode = "auto"
```

Worker pyprojects do NOT declare `worker-common` as a dependency: a relative `file:` requirement is not valid PEP 508, so every install names both (`pip install -e apps/workers/common -e "apps/workers/<name>[test]"`; Dockerfiles `pip install /src/common /src/<name>`). This deviates from the spec's `worker-common @ file:../common` wording for that reason.

- [ ] **Step 2: Write the failing tests.**

`tests/__init__.py`: empty. `tests/conftest.py`: copy `apps/ai-worker/tests/conftest.py` verbatim (root-logging restore fixture), fixing its docstring's path to `worker_common/logging.py`.

`tests/test_settings.py`:

```python
import pytest

from worker_common.settings import BaseSettings, check_removed_env

ENV = ["DATABASE_URL", "DATABASE_PUBLIC_URL", "POSTGRES_URL", "S3_BUCKET", "S3_ACCESS_KEY_ID",
       "S3_SECRET_ACCESS_KEY", "S3_REGION", "S3_ENDPOINT", "LOG_LEVEL", "HEALTH_DB_TIMEOUT_SECONDS",
       "AI_WORKER_PROFILES", "OLD_NAME"]


@pytest.fixture
def env(monkeypatch):
    for name in ENV:
        monkeypatch.delenv(name, raising=False)
    return monkeypatch


def test_reads_env_lazily_on_every_access(env):
    s = BaseSettings()
    assert s.database_url == ""
    env.setenv("POSTGRES_URL", "postgres://c")
    assert s.database_url == "postgres://c"
    env.setenv("DATABASE_URL", "postgres://a")
    assert s.database_url == "postgres://a"


def test_s3_is_configured_only_with_bucket_and_keys(env):
    s = BaseSettings()
    assert not s.s3_configured and s.s3_region == "auto"
    for k, v in {"S3_BUCKET": "b", "S3_ACCESS_KEY_ID": "i", "S3_SECRET_ACCESS_KEY": "s"}.items():
        env.setenv(k, v)
    assert s.s3_configured


def test_log_level_accepts_go_spelling_and_rejects_unknown(env):
    env.setenv("LOG_LEVEL", "warn")
    assert BaseSettings().log_level == "WARNING"
    env.setenv("LOG_LEVEL", "loud")
    with pytest.raises(RuntimeError, match="LOG_LEVEL"):
        BaseSettings().log_level


def test_removed_env_names_its_replacement(env):
    check_removed_env({"OLD_NAME": "NEW_NAME"})  # unset: fine
    env.setenv("OLD_NAME", "x")
    with pytest.raises(RuntimeError, match="OLD_NAME.*NEW_NAME"):
        check_removed_env({"OLD_NAME": "NEW_NAME"})


def test_ai_worker_profiles_is_removed_everywhere(env):
    env.setenv("AI_WORKER_PROFILES", "vision")
    with pytest.raises(RuntimeError, match="AI_WORKER_PROFILES"):
        check_removed_env({})
```

`tests/test_app.py`:

```python
import json
import logging
import types

import pytest
from fastapi import APIRouter
from fastapi.testclient import TestClient

from worker_common.app import create_app


def fake_worker(events=None, checks=None, removed=None):
    events = events if events is not None else []
    router = APIRouter()

    @router.get("/ping")
    async def ping():
        return {"pong": True}

    async def startup():
        events.append("start")

    async def shutdown():
        events.append("stop")

    async def health_checks():
        return checks or {}

    return types.SimpleNamespace(SERVICE_NAME="worker-fake", REMOVED_ENV=removed or {}, ROUTERS=[router],
                                 startup=startup, shutdown=shutdown, health_checks=health_checks)


def test_mounts_the_worker_routes_and_health():
    paths = set(create_app(fake_worker()).openapi()["paths"])
    assert {"/ping", "/health"} <= paths


def test_health_reports_service_and_checks():
    res = TestClient(create_app(fake_worker(checks={"db": ("up", True), "gpu": ("unavailable", False)}))).get("/health")
    assert res.status_code == 200
    assert res.json() == {"status": "ok", "service": "worker-fake", "checks": {"db": "up", "gpu": "unavailable"}}


def test_health_503_when_a_required_check_is_down():
    res = TestClient(create_app(fake_worker(checks={"db": ("down", True)}))).get("/health")
    assert res.status_code == 503 and res.json()["status"] == "degraded"


def test_lifespan_starts_and_stops_the_worker_once():
    events = []
    with TestClient(create_app(fake_worker(events))):
        assert events == ["start"]
    assert events == ["start", "stop"]


def test_removed_env_stops_the_app(monkeypatch):
    monkeypatch.setenv("OLD_NAME", "x")
    with pytest.raises(RuntimeError, match="OLD_NAME"):
        create_app(fake_worker(removed={"OLD_NAME": "NEW_NAME"}))


def test_logs_are_json_lines_with_service(capsys):
    create_app(fake_worker())
    logging.getLogger("worker.test").warning("hello %s", "world")
    line = json.loads(capsys.readouterr().out.strip().splitlines()[-1])
    assert line["service"] == "worker-fake" and line["msg"] == "hello world" and "profile" not in line


def test_uvicorn_logs_are_json_lines(capsys):
    import logging.config

    from uvicorn.config import LOGGING_CONFIG

    logging.config.dictConfig(LOGGING_CONFIG)
    create_app(fake_worker())
    logging.getLogger("uvicorn.access").info("GET /health 200")
    line = json.loads(capsys.readouterr().out.strip().splitlines()[-1])
    assert line["logger"] == "uvicorn.access" and line["service"] == "worker-fake"
```

`tests/test_storage.py`: move the S3 parts of `apps/ai-worker/tests/core/test_storage.py` — the `_FakeS3` class, the `s3` fixture (import `from worker_common import storage`), `test_upload_bytes_passes_content_type_when_given`, `test_upload_bytes_without_content_type_is_unchanged_for_vision` renamed `test_upload_bytes_without_content_type`. Add:

```python
async def test_upload_requires_s3(monkeypatch):
    for k in ("S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"):
        monkeypatch.delenv(k, raising=False)
    monkeypatch.setattr(storage, "_client", None)
    with pytest.raises(RuntimeError, match="S3 is not configured"):
        await storage.upload_bytes("k", b"x")
```

The tryon-specific storage tests (`test_tryon_upload_*`, `test_core_storage_has_no_public_url`) move in Task 6; the local-fallback behaviour moves with skin in Task 4.

`tests/test_model_mirror.py`: move `apps/ai-worker/tests/core/test_model_mirror.py`, with imports from `worker_common.model_mirror`, and replace every `sync(s3, "b", <profiles>, hf_cache=X)` call using this mapping (`paths` fixture paths stay as they are):

| old profiles argument | new `MirrorNeeds` argument |
|---|---|
| `["vision"]` | `MirrorNeeds(landmarker_paths=(face_path,), landmarker_pins=PINS, pin_source="face manifests")` |
| `["colour"]` | `MirrorNeeds(landmarker_paths=(colour_path,))` |
| `["vision", "colour"]` | `MirrorNeeds(landmarker_paths=(face_path, colour_path), landmarker_pins=PINS, pin_source="face manifests")` |
| `["tryon"]` | `MirrorNeeds(hf_cache=X)` |
| `["vision", "tryon"]` | `MirrorNeeds(landmarker_paths=(face_path,), landmarker_pins=PINS, hf_cache=X)` |

where `face_path`/`colour_path` are the paths the old test read from `settings.face_landmarker_path`/`settings.colour_face_landmarker_path` (set them explicitly in the `paths` fixture instead of via env), and `PINS = frozenset({<the sha256 the fixture's landmarker bytes hash to>})` for tests that expect acceptance, or a set without it for `test_vision_refuses_a_landmarker_its_manifests_do_not_pin` (rename: `test_pinned_needs_refuse_an_unpinned_landmarker`). `test_by_default_vision_and_colour_share_one_file` is dropped (defaults now live in the workers; Task 3/7 cover them) — say so in the report. `test_the_real_vision_pin_is_the_float16_v1_landmarker` moves to face in Task 3. `test_cli_skips_cleanly_without_s3` becomes:

```python
def test_cli_skips_cleanly_without_s3(monkeypatch, capsys):
    for k in ("S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"):
        monkeypatch.delenv(k, raising=False)
    assert main(["sync", "tests.fake_mirror_worker"]) == 0
    assert "S3 is not configured" in capsys.readouterr().out
```

with `tests/fake_mirror_worker.py`:

```python
from worker_common.model_mirror import MirrorNeeds

SERVICE_NAME = "worker-fake"


def mirror_needs() -> MirrorNeeds:
    return MirrorNeeds()
```

`tests/test_boundaries.py`:

```python
"""A worker imports worker_common and itself only; worker_common imports no
worker; nothing imports the old ai_worker package."""
import ast
from pathlib import Path

WORKERS = Path(__file__).resolve().parents[2]  # apps/workers


def _imports(pkg_dir: Path):
    for py in pkg_dir.rglob("*.py"):
        tree = ast.parse(py.read_text(encoding="utf-8"))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    yield py, alias.name.split(".")[0]
            elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
                yield py, node.module.split(".")[0]


def _packages():
    for app in sorted(p for p in WORKERS.iterdir() if p.is_dir()):
        pkg = app / f"worker_{app.name}"
        if pkg.is_dir():
            yield pkg.name, pkg


def test_workers_import_only_common_and_themselves():
    bad = [f"{py.relative_to(WORKERS)} imports {top}"
           for name, pkg in _packages()
           for py, top in _imports(pkg)
           if (top.startswith("worker_") and top not in (name, "worker_common")) or top == "ai_worker"]
    assert bad == []


def test_there_is_at_least_the_common_package():
    assert "worker_common" in dict(_packages())
```

- [ ] **Step 3: Run to verify failure.** `pip install -e "apps/workers/common[test]"` then `python -m pytest apps/workers/common -q` → FAIL (modules missing).

- [ ] **Step 4: Implement.**

`worker_common/__init__.py`: `"""Shared library for the Seagull Python workers. Not a service."""`

`worker_common/settings.py`:

```python
"""Settings every worker shares, read lazily on every access (tests set env
at runtime; a worker must start while S3 or the database is unset, and only a
feature that needs a missing value raises). Worker-specific settings live in
worker_<name>/settings.py, subclassing BaseSettings."""
import os
from pathlib import Path
from typing import Mapping, Optional

from dotenv import load_dotenv

# Find the repo-root .env regardless of the process's cwd, like the Go
# services' goplatform.LoadDotEnv. Inside an image no .env is found.
for _parent in Path(__file__).resolve().parents:
    _candidate = _parent / ".env"
    if _candidate.exists():
        load_dotenv(_candidate)
        break

DEFAULT_LOG_LEVEL = "INFO"
# Same default as the Go services' DEPENDENCY_PING_TIMEOUT (packages/goplatform/common.go).
DEFAULT_HEALTH_DB_TIMEOUT_SECONDS = 3
# Accepted LOG_LEVEL spellings; "WARN" matches the Go services' slog names.
_LOG_LEVELS = {"DEBUG": "DEBUG", "INFO": "INFO", "WARN": "WARNING", "WARNING": "WARNING", "ERROR": "ERROR"}

# Names removed by the workers split (docs/superpowers/specs/2026-09-30-workers-monorepo-design.md),
# refused by every worker. Each worker adds its own in worker.REMOVED_ENV.
REMOVED_ENV = {"AI_WORKER_PROFILES": "nothing: each worker is its own app under apps/workers/"}


def _first(*names: str) -> str:
    for name in names:
        val = os.getenv(name)
        if val:
            return val
    return ""


def check_removed_env(removed: Mapping[str, str]) -> None:
    """Refuse to start while a removed setting is still set, naming what replaces it."""
    found = [f"{name} (use {repl})" for name, repl in {**REMOVED_ENV, **removed}.items() if os.getenv(name)]
    if found:
        raise RuntimeError("removed settings are still set: " + "; ".join(found))


class BaseSettings:
    @property
    def database_url(self) -> str:
        return _first("DATABASE_URL", "DATABASE_PUBLIC_URL", "POSTGRES_URL")

    @property
    def s3_endpoint(self) -> str:
        return os.getenv("S3_ENDPOINT", "")

    @property
    def s3_bucket(self) -> str:
        return os.getenv("S3_BUCKET", "")

    @property
    def s3_access_key_id(self) -> str:
        return os.getenv("S3_ACCESS_KEY_ID", "")

    @property
    def s3_secret_access_key(self) -> str:
        return os.getenv("S3_SECRET_ACCESS_KEY", "")

    @property
    def s3_region(self) -> str:
        return os.getenv("S3_REGION", "auto")

    @property
    def s3_configured(self) -> bool:
        return bool(self.s3_bucket and self.s3_access_key_id and self.s3_secret_access_key)

    def require_s3(self) -> None:
        missing = [name for name, val in (("S3_BUCKET", self.s3_bucket),
                                          ("S3_ACCESS_KEY_ID", self.s3_access_key_id),
                                          ("S3_SECRET_ACCESS_KEY", self.s3_secret_access_key)) if not val]
        if missing:
            raise RuntimeError(f"S3 is not configured: missing {', '.join(missing)}.")

    def require_database(self) -> None:
        if not self.database_url:
            raise RuntimeError("missing DATABASE_URL.")

    def _int_env(self, name: str, default: int) -> int:
        raw = os.getenv(name, "").strip()
        try:
            return int(raw) if raw else default
        except ValueError:
            return default

    def _float_env(self, name: str, default: Optional[float]) -> Optional[float]:
        raw = os.getenv(name, "").strip()
        try:
            return float(raw) if raw else default
        except ValueError:
            return default

    @property
    def log_level(self) -> str:
        raw = os.getenv("LOG_LEVEL", "").strip().upper() or DEFAULT_LOG_LEVEL
        if raw not in _LOG_LEVELS:
            raise RuntimeError(f"LOG_LEVEL: unknown level {raw!r}; use debug, info, warn or error.")
        return _LOG_LEVELS[raw]

    @property
    def health_db_timeout_seconds(self) -> int:
        return self._int_env("HEALTH_DB_TIMEOUT_SECONDS", DEFAULT_HEALTH_DB_TIMEOUT_SECONDS)


settings = BaseSettings()
```

`worker_common/logging.py`: copy `apps/ai-worker/ai_worker/core/logging.py`, changing `JsonFormatter.__init__(self, service: str)` (store `self._service`), the entry to `"service": self._service` with no `"profile"` key, and `configure_logging(service: str, level: str)` building `JsonFormatter(service)`. Drop the `SERVICE_NAME` import.

`worker_common/health.py`:

```python
from typing import Awaitable, Callable, Iterable

from fastapi import APIRouter
from fastapi.responses import JSONResponse

# name -> (state, required); state is "up" | "down" | "disabled" or an informational value.
HealthCheck = Callable[[], Awaitable[dict[str, tuple[str, bool]]]]


def health_router(service: str, check_fns: Iterable[HealthCheck]) -> APIRouter:
    check_fns = list(check_fns)
    router = APIRouter()

    @router.get("/health")
    async def health():
        checks: dict[str, str] = {}
        status_code = 200
        for fn in check_fns:
            for name, (state, required) in (await fn()).items():
                checks[name] = state
                if required and state == "down":
                    status_code = 503
        body = {"status": "ok" if status_code == 200 else "degraded", "service": service, "checks": checks}
        return JSONResponse(body, status_code=status_code)

    return router
```

`worker_common/app.py`:

```python
"""The FastAPI factory every worker's main.py calls with its worker module
(worker_<name>/worker.py: SERVICE_NAME, REMOVED_ENV, ROUTERS, startup,
shutdown, health_checks)."""
from contextlib import asynccontextmanager
from types import ModuleType

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from worker_common.health import health_router
from worker_common.logging import configure_logging
from worker_common.settings import check_removed_env, settings


def create_app(worker: ModuleType) -> FastAPI:
    check_removed_env(getattr(worker, "REMOVED_ENV", {}))
    configure_logging(worker.SERVICE_NAME, settings.log_level)

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        await worker.startup()
        try:
            yield
        finally:
            await worker.shutdown()

    app = FastAPI(title=f"Seagull {worker.SERVICE_NAME}", lifespan=lifespan)
    app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_credentials=True,
                       allow_methods=["*"], allow_headers=["*"])
    app.include_router(health_router(worker.SERVICE_NAME, [worker.health_checks]))
    for router in worker.ROUTERS:
        app.include_router(router)
    return app
```

`worker_common/storage.py` (S3 only; the local-disk fallback is skin's, Task 4):

```python
"""S3 object storage shared by the workers. Path-style addressing so every
request stays on the configured endpoint host (works with S3, MinIO, R2 and
SeaweedFS, and behind a single-host TLS proxy). Every call requires S3."""
import asyncio

import boto3
from botocore.config import Config

from worker_common.settings import settings

_client = None


def _get_client():
    global _client
    if _client is None:
        settings.require_s3()
        _client = boto3.client(
            "s3",
            endpoint_url=settings.s3_endpoint or None,
            aws_access_key_id=settings.s3_access_key_id,
            aws_secret_access_key=settings.s3_secret_access_key,
            region_name=settings.s3_region,
            config=Config(s3={"addressing_style": "path"}),
        )
    return _client


async def upload_bytes(key: str, data: bytes, content_type: str | None = None) -> None:
    client = _get_client()
    extra = {"ContentType": content_type} if content_type else {}
    await asyncio.to_thread(client.put_object, Bucket=settings.s3_bucket, Key=key, Body=data, **extra)


async def download_bytes(key: str) -> bytes:
    obj = await asyncio.to_thread(_get_client().get_object, Bucket=settings.s3_bucket, Key=key)
    return obj["Body"].read()


async def delete_object(key: str) -> None:
    await asyncio.to_thread(_get_client().delete_object, Bucket=settings.s3_bucket, Key=key)


async def presigned_download_url(key: str, expires_in: int = 300) -> str:
    return await asyncio.to_thread(
        _get_client().generate_presigned_url, "get_object",
        Params={"Bucket": settings.s3_bucket, "Key": key}, ExpiresIn=expires_in,
    )
```

`worker_common/model_mirror.py`: copy `apps/ai-worker/ai_worker/core/model_mirror.py` and change only:

1. Module docstring: usage `python -m worker_common.model_mirror sync <worker package>`; say what is fetched is decided by that worker's `mirror_needs()`.
2. Imports: `from worker_common.settings import settings`; add `import importlib` and `from dataclasses import dataclass, field`.
3. Replace `_vision_landmarker_shas()` and `_targets()` with:

```python
@dataclass(frozen=True)
class MirrorNeeds:
    """What one worker takes from the mirror (its worker.mirror_needs())."""
    landmarker_paths: tuple[Path, ...] = ()          # every local path face_landmarker.task goes to
    landmarker_pins: frozenset[str] | None = None    # allowed SHA-256s; None = any build
    pin_source: str = ""                             # where the pins come from, for the error
    hf_cache: Path | None = None                     # Hugging Face cache; None = skip HF entries


def _targets(entries: list[dict], needs: MirrorNeeds) -> list[tuple[dict, list[Path]]]:
    targets: list[tuple[dict, list[Path]]] = []
    for e in entries:
        key = e["key"]
        if key == LANDMARKER_KEY and needs.landmarker_paths:
            if needs.landmarker_pins is not None and e["sha256"] not in needs.landmarker_pins:
                raise MirrorError(f"{key}: sha256 {e['sha256']} is not pinned by {needs.pin_source}; "
                                  "this worker would refuse it")
            targets.append((e, list(dict.fromkeys(Path(p) for p in needs.landmarker_paths))))
        elif key.startswith(HF_PREFIX) and needs.hf_cache is not None:
            org, name, rev, *rel = key[len(HF_PREFIX):].split("/")
            if not rel:
                raise MirrorError(f"manifest key is not <org>/<name>/<revision>/<file>: {key}")
            targets.append((e, [needs.hf_cache / f"models--{org}--{name}" / "snapshots" / rev / Path(*rel)]))
    return targets
```

4. `sync(client, bucket: str, needs: MirrorNeeds) -> dict | None`: same body, calling `_targets(entries, needs)` and using `needs.hf_cache` where it used `hf_cache`.
5. Delete `_hf_cache()`. Replace `main()` with:

```python
def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m worker_common.model_mirror")
    parser.add_argument("command", choices=["sync"])
    parser.add_argument("worker", help="worker package whose worker.mirror_needs() to apply, e.g. worker_face")
    args = parser.parse_args(argv)
    # "worker_face" -> worker_face.worker; a dotted name is imported as given.
    worker = importlib.import_module(args.worker if "." in args.worker else f"{args.worker}.worker")
    if not settings.s3_configured:
        print("model mirror: S3 is not configured; models load the usual way")
        return 0
    from worker_common import storage

    client = storage._get_client()
    try:
        result = sync(client, settings.s3_bucket, worker.mirror_needs())
    except MirrorError as exc:
        print(f"model mirror: {exc}", file=sys.stderr)
        return 1
    if result is None:
        print(f"model mirror: no {MANIFEST_KEY} in bucket {settings.s3_bucket}; models load the usual way")
        return 0
    print(
        f"model mirror: {worker.SERVICE_NAME}: {result['downloaded']} downloaded, "
        f"{result['copied']} copied from a file already synced, {result['present']} already present and verified"
        + (f"; Hugging Face cache: {', '.join(result['repos'])}" if result["repos"] else "")
    )
    return 0
```

(`tests.fake_mirror_worker` in the CLI test contains a dot, so it is imported as given.)

- [ ] **Step 5: Run.** `python -m pytest apps/workers/common -q` → all PASS. Old suite unchanged: `python -m pytest apps/ai-worker -q | tail -1` equals the baseline.
- [ ] **Step 6: Commit.** `git add apps/workers/common && git commit -m "feat(workers): worker_common shared library"`

---

### Task 2: core-engine `SKIN_WORKER_URL` / `FACE_WORKER_URL`

Independent of Tasks 1, 3–7 (may run in parallel).

**Files:** Modify `apps/core-engine/internal/config/config.go`, `config_test.go`, `cmd/server/main.go:156,159,166`, `cmd/facearch-profile/main.go:6,131-133`, `cmd/face-benchmark/main.go:10,418-423`, `internal/facearch/measureclient/client.go:47` (comment).

**Interfaces:** Produces `config.DefaultSkinWorkerURL`, `config.DefaultFaceWorkerURL`, `Config.SkinWorkerURL`, `Config.FaceWorkerURL`; removes `DefaultVisionAIWorkerURL` and `Config.VisionAIWorkerURL`.

- [ ] **Step 1: Failing tests** in `config_test.go`: in `tunables`, replace `"VISION_AI_WORKER_URL"` with `"VISION_AI_WORKER_URL", "SKIN_WORKER_URL", "FACE_WORKER_URL"`; update any existing assertion on `VisionAIWorkerURL` to `SkinWorkerURL`; append:

```go
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
	t.Setenv("VISION_AI_WORKER_URL", "http://ai-worker-vision:8088")
	_, err := config.FromEnv()
	if err == nil || !strings.Contains(err.Error(), "SKIN_WORKER_URL") || !strings.Contains(err.Error(), "FACE_WORKER_URL") {
		t.Fatalf("err = %v, want one naming SKIN_WORKER_URL and FACE_WORKER_URL", err)
	}
}
```

(add `"strings"` to the imports).

- [ ] **Step 2:** `go test ./apps/core-engine/internal/config/...` → FAIL to compile.
- [ ] **Step 3: Implement** in `config.go`: replace `DefaultVisionAIWorkerURL = "http://127.0.0.1:8088"` with

```go
	DefaultSkinWorkerURL = "http://127.0.0.1:8088"
	// DefaultFaceWorkerURL is the face worker's port in docs/PORTS.md.
	DefaultFaceWorkerURL = "http://127.0.0.1:8094"
```

Replace the `VisionAIWorkerURL string` field with

```go
	// SkinWorkerURL is apps/workers/skin (segment-and-pose, model dispatch);
	// FaceWorkerURL is apps/workers/face (face-measure, face architecture).
	SkinWorkerURL string
	FaceWorkerURL string
```

At the top of `FromEnv`, before `LoadCommon`:

```go
	if goplatform.First("VISION_AI_WORKER_URL") != "" {
		return nil, errors.New("VISION_AI_WORKER_URL was removed: set SKIN_WORKER_URL (segment-and-pose, model dispatch) and FACE_WORKER_URL (face-measure) instead")
	}
```

(import `"errors"`), and in the literal replace the `VisionAIWorkerURL:` line with

```go
		SkinWorkerURL:        firstOr("SKIN_WORKER_URL", DefaultSkinWorkerURL),
		FaceWorkerURL:        firstOr("FACE_WORKER_URL", DefaultFaceWorkerURL),
```

`cmd/server/main.go`: lines 156 and 159 use `cfg.SkinWorkerURL`; line 166 uses `cfg.FaceWorkerURL`. `cmd/facearch-profile/main.go` and `cmd/face-benchmark/main.go`: read `goplatform.First("FACE_WORKER_URL")`, default `config.DefaultFaceWorkerURL`, error text `FACE_WORKER_URL: ...`, and doc comments naming `FACE_WORKER_URL`. `measureclient/client.go:47` comment: `(FACE_WORKER_URL)`. Then `grep -rn "VisionAIWorkerURL\|VISION_AI_WORKER_URL" apps/core-engine` must show only the config removal check and its test.
- [ ] **Step 4:** `go test ./apps/core-engine/...`, `npm run lint`, `npm run build:check` → PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat(core-engine): SKIN_WORKER_URL and FACE_WORKER_URL replace VISION_AI_WORKER_URL"`

---

### Task 3: `worker_face`

**Files:**
- Move (`git mv`): `apps/ai-worker/ai_worker/vision/facegeom/` → `apps/workers/face/worker_face/facegeom/`; `ai_worker/vision/models/face_landmarker.py` → `worker_face/face_landmarker.py`; `ai_worker/vision/routers/face_measure.py` → `worker_face/routers/face_measure.py`; `ai_worker/vision/schemas/face_measure.py` → `worker_face/schemas.py`; `apps/ai-worker/scripts/fetch_face_landmarker.py` → `apps/workers/face/scripts/fetch_face_landmarker.py`.
- Move tests: `apps/ai-worker/tests/vision/{face_fixture.py,test_face_landmarker.py,test_face_measure_api.py,test_facegeom_*.py}` and the fixtures they read (`tests/vision/fixtures/sample_face_landmarks.json` and any other fixture referenced by those files — `grep -n fixtures` them) → `apps/workers/face/tests/`; `apps/ai-worker/tests/core/test_settings_face_measure.py` → `apps/workers/face/tests/test_settings.py`.
- Create: `apps/workers/face/{pyproject.toml,Dockerfile}`, `worker_face/{__init__,worker,main,settings}.py`, `worker_face/routers/__init__.py`, `tests/{__init__,conftest,test_worker,test_mirror}.py`.
- Modify: `apps/ai-worker/ai_worker/vision/profile.py`, `apps/ai-worker/ai_worker/core/model_mirror.py` + its test (the vision profile no longer takes the landmarker), `apps/ai-worker/ai_worker/core/settings.py` (drop face-measure settings), `apps/ai-worker/pyproject.toml` package-data.

**Interfaces:** Consumes Task 1 (`worker_common.*`). Produces `worker_face.settings.settings` with `face_landmarker_path: Path`, `face_measure_max_yaw_deg`, `face_measure_max_pitch_deg`, `face_measure_min_iod_px`, `face_measure_noise_samples`, `face_measure_occlusion_delta_e`; `worker_face.facegeom.manifest.load_manifests()` (unchanged).

- [ ] **Step 1: Move the files** listed above with `git mv`. Create empty `worker_face/routers/__init__.py`.
- [ ] **Step 2: Rewrite imports** in `apps/workers/face` with this exact mapping (whole-word, longest first):

| old | new |
|---|---|
| `ai_worker.vision.facegeom` | `worker_face.facegeom` |
| `ai_worker.vision.models.face_landmarker` | `worker_face.face_landmarker` |
| `from ai_worker.vision.models import face_landmarker` | `from worker_face import face_landmarker` |
| `ai_worker.vision.routers.face_measure` | `worker_face.routers.face_measure` |
| `from ai_worker.vision.routers import face_measure` | `from worker_face.routers import face_measure` |
| `ai_worker.vision.schemas.face_measure` | `worker_face.schemas` |
| `ai_worker.core.settings` | `worker_face.settings` |
| `tests.vision.face_fixture` / `tests.vision` | `tests.face_fixture` / `tests` |
| `Settings` (class, in the moved settings test) | `FaceSettings` |
| `VISION_FACE_LANDMARKER_PATH` | `FACE_LANDMARKER_PATH` |
| `VISION_MODEL_STORAGE_DIR` (in tests) | `FACE_MODEL_DIR` |

Also fix comments/docstrings that name `ai_worker/vision/...` paths or the "vision profile" to the new paths / "the face worker". Then `grep -rn "ai_worker" apps/workers/face` → no output.

- [ ] **Step 3: `worker_face/settings.py`**:

```python
"""Face worker settings (face-measure). Shared settings: worker_common.settings."""
import os
from pathlib import Path
from typing import Optional

from worker_common.settings import BaseSettings

# A source checkout keeps the landmarker under apps/workers/face/data/models
# (gitignored); the image sets FACE_MODEL_DIR=/app/data/models.
DEFAULT_MODEL_DIR = Path(__file__).resolve().parents[1] / "data" / "models"
# /api/v1/face-measure policy defaults. None is a measured tolerance; each is
# listed under "Open literals" in
# docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md.
DEFAULT_FACE_MEASURE_MAX_POSE_DEG = 15.0
DEFAULT_FACE_MEASURE_MIN_IOD_PX = 60.0
DEFAULT_FACE_MEASURE_NOISE_SAMPLES = 64


class FaceSettings(BaseSettings):
    @property
    def model_dir(self) -> Path:
        raw = os.getenv("FACE_MODEL_DIR", "").strip()
        return Path(raw) if raw else DEFAULT_MODEL_DIR

    @property
    def face_landmarker_path(self) -> Path:
        # (keep the existing docstring, with FACE_LANDMARKER_PATH and the new manifest path)
        raw = os.getenv("FACE_LANDMARKER_PATH", "").strip()
        if raw:
            return Path(raw)
        return self.model_dir / "face_landmarker" / "face_landmarker.task"

    # face_measure_max_yaw_deg, face_measure_max_pitch_deg, face_measure_min_iod_px,
    # face_measure_noise_samples, face_measure_occlusion_delta_e: copy the five
    # properties verbatim from apps/ai-worker/ai_worker/core/settings.py.


settings = FaceSettings()
```

`FACE_MODEL_DIR` is new (the face worker's own model directory; previously the vision storage dir). Replace the comment line in the block above with the five copied properties — no comment placeholder may remain.

- [ ] **Step 4: `worker_face/worker.py`** (logic from the vision profile's face parts):

```python
"""Face worker: face-measure (landmarks, facial measurements, noise bands),
called by core-engine's face architecture (FACE_WORKER_URL)."""
from worker_common.model_mirror import MirrorNeeds
from worker_face import face_landmarker
from worker_face.facegeom.manifest import load_manifests
from worker_face.routers import face_measure
from worker_face.settings import settings

SERVICE_NAME = "worker-face"
REMOVED_ENV = {"VISION_FACE_LANDMARKER_PATH": "FACE_LANDMARKER_PATH"}
ROUTERS = [face_measure.router]


async def startup() -> None:
    face_measure.warm_up()


async def shutdown() -> None:
    face_landmarker.reset()


async def health_checks() -> dict[str, tuple[str, bool]]:
    # Informational, as before: without the landmarker only face-measure
    # answers 503, and the process stays up to report it.
    return {"face_landmarker": ("up" if face_landmarker.is_available() else "unavailable", False)}


def mirror_needs() -> MirrorNeeds:
    return MirrorNeeds(landmarker_paths=(settings.face_landmarker_path,),
                       landmarker_pins=frozenset(load_manifests()),
                       pin_source="worker_face/facegeom/manifests/")
```

`worker_face/main.py`: the contract's three lines. `worker_face/__init__.py`: `"""Seagull face worker (face-measure)."""`

- [ ] **Step 5: `pyproject.toml`**:

```toml
[project]
name = "worker-face"
version = "0.1.0"
description = "Seagull face worker: face-measure for core-engine's face architecture. Install with apps/workers/common."
requires-python = ">=3.12"
dependencies = [
    "opencv-python-headless>=4.9.0",
    "mediapipe>=0.10.9",
    "numpy>=1.26.0",
]

[project.optional-dependencies]
test = ["pytest>=8.0.0", "pytest-asyncio>=0.23", "pillow>=10.0.0"]

[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[tool.setuptools.packages.find]
include = ["worker_face*"]

# The landmarker manifests are matched against the model file by SHA-256;
# without them every model is refused (permanent 503).
[tool.setuptools.package-data]
"worker_face.facegeom" = ["manifests/*.json"]

[tool.pytest.ini_options]
asyncio_mode = "auto"
```

If a moved test imports a package not listed (check `grep -h "^import\|^from" tests/*.py`), add it to `test`.

- [ ] **Step 6: Tests.** `tests/__init__.py` empty; `tests/conftest.py` = the common one. `tests/test_worker.py`:

```python
import subprocess
import sys
from pathlib import Path

from fastapi.testclient import TestClient

from worker_face.main import app

APP_DIR = Path(__file__).resolve().parents[1]


def test_face_routes_and_health():
    paths = set(app.openapi()["paths"])
    assert "/api/v1/face-measure" in paths and "/health" in paths
    assert "/api/v1/segment-and-pose" not in paths
    body = TestClient(app).get("/health").json()
    assert body["service"] == "worker-face" and "face_landmarker" in body["checks"]


def test_face_never_imports_skin_colour_tryon_or_torch():
    code = ("import sys, worker_face.main; "
            "print(any(m == 'torch' or m.split('.')[0] in ('worker_skin','worker_colour','worker_tryon','ai_worker','onnxruntime','asyncpg') for m in sys.modules))")
    out = subprocess.run([sys.executable, "-c", code], cwd=APP_DIR, capture_output=True, text=True, check=True)
    assert out.stdout.strip().splitlines()[-1] == "False"
```

`tests/test_mirror.py`: move `test_the_real_vision_pin_is_the_float16_v1_landmarker` from the old mirror test here as `test_the_face_pin_is_the_float16_v1_landmarker` (asserting `"64184e229b263107bc2b804c6625db1341ff2bb731874b0bcc2fe6544e0bc9ff" in load_manifests()`), plus:

```python
def test_mirror_needs_pin_the_face_manifests(monkeypatch, tmp_path):
    from worker_face import worker
    monkeypatch.setenv("FACE_LANDMARKER_PATH", str(tmp_path / "f.task"))
    needs = worker.mirror_needs()
    assert needs.landmarker_paths == (tmp_path / "f.task",)
    assert "64184e229b263107bc2b804c6625db1341ff2bb731874b0bcc2fe6544e0bc9ff" in needs.landmarker_pins
    assert needs.hf_cache is None
```

- [ ] **Step 7: `Dockerfile`**:

```dockerfile
# Build context: repository root.
#   docker build -f apps/workers/face/Dockerfile -t worker-face .
FROM python:3.12-slim
# libgl1/libglib2.0-0: OpenCV and mediapipe. libegl1/libgles2: the Face
# Landmarker dlopens libEGL.so.1 at load, even on CPU.
RUN apt-get update && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 libegl1 libgles2 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY apps/workers/common /src/common
COPY apps/workers/face /src/face
RUN pip install --no-cache-dir /src/common /src/face && rm -rf /src
ENV PORT=8094 FACE_MODEL_DIR=/app/data/models
EXPOSE 8094
# Pull face_landmarker.task from the S3 mirror first (docs/MODEL-MIRROR.md).
CMD python -m worker_common.model_mirror sync worker_face && exec uvicorn worker_face.main:app --host 0.0.0.0 --port ${PORT}
```

- [ ] **Step 8: Take face out of the old package.** `apps/ai-worker/ai_worker/vision/profile.py`: drop the `face_landmarker` and `face_measure` imports, `face_measure.router` from `ROUTERS`, `face_measure.warm_up()`, `face_landmarker.reset()`, and the `face_landmarker` health entry. `apps/ai-worker/ai_worker/core/model_mirror.py`: delete `_vision_landmarker_shas()` and the `"vision"` branch in `_targets` (colour keeps its branch); in its test, delete the tests now in `apps/workers/common` / face and keep the colour and tryon ones passing. `apps/ai-worker/ai_worker/core/settings.py`: delete `face_landmarker_path` and the five `face_measure_*` properties and their defaults; `colour_face_landmarker_path` becomes `os.getenv("FACE_LANDMARKER_PATH", "") or str(self.local_model_storage_dir / "face_landmarker" / "face_landmarker.task")` (temporary until Task 7; same default path as before). `apps/ai-worker/pyproject.toml`: remove the `"ai_worker.vision.facegeom"` package-data line. `tests/core/test_app.py`: drop `face_landmarker` from the expected `/health` checks. `grep -rn "facegeom\|face_measure\|face_landmarker\b" apps/ai-worker` → only colour's own `face_landmarker.task`/path references remain.
- [ ] **Step 9: Run.**

```bash
pip install -e apps/workers/common -e "apps/workers/face[test]"
python -m pytest apps/workers/face -q
python -m pytest apps/workers/common -q
python -m pytest apps/ai-worker -q
```

The old suite's count drops by exactly the number of tests moved; face + common + old = baseline (adjusting for the common-mirror rewrite noted in Task 1). Put the arithmetic in the report.
- [ ] **Step 10: Commit.** `git add -A apps/workers/face apps/ai-worker && git commit -m "refactor(workers): face-measure becomes worker_face"`

---

### Task 4: `worker_skin`

**Files:**
- Move (`git mv`) the rest of `apps/ai-worker/ai_worker/vision/` → `apps/workers/skin/worker_skin/` (`db.py`, `zones.py`, `models/`, `routers/{healthz,models,segment}.py`, `schemas/` (now `contracts.py` + `__init__.py`)); `apps/ai-worker/benchmarks/` → `apps/workers/skin/benchmarks/`; `apps/ai-worker/tests/vision/*` (remaining) → `apps/workers/skin/tests/`; `apps/ai-worker/tests/benchmarks/` → `apps/workers/skin/tests/benchmarks/`.
- `apps/ai-worker/ai_worker/vision/profile.py` → content moves into `worker_skin/worker.py` (then delete `profile.py`).
- Create: `apps/workers/skin/{pyproject.toml,Dockerfile}`, `worker_skin/{__init__,worker,main,settings,storage}.py`, `tests/{__init__,conftest,test_worker,test_storage,test_settings}.py`.
- Modify: `apps/ai-worker/ai_worker/core/settings.py` (drop skin settings), `apps/ai-worker/ai_worker/main.py` (`KNOWN_PROFILES`/`PROFILE_EXTRAS` without vision; default profile), `apps/ai-worker/tests/core/test_app.py` (drop vision tests), `.gitignore`.

**Interfaces:** Produces `worker_skin.settings.settings` with `local_model_storage_dir: Path`, `worker_public_url: str`, `model_max_file_bytes`, `model_max_smoke_latency_ms`, `model_cache_max_bytes`, `model_library_warn_bytes`, `schema_lock_timeout_seconds`; `worker_skin.storage.{upload_bytes, download_bytes, delete_object, presigned_download_url}` (local fallback when S3 is unset).

- [ ] **Step 1: Move** as listed.
- [ ] **Step 2: Rewrite imports** in `apps/workers/skin` (longest first): `ai_worker.vision` → `worker_skin`; `ai_worker.core.settings` → `worker_skin.settings`; `from ai_worker.core import storage` / `from ai_worker.vision import db, storage` style combined imports → import `storage` from `worker_skin` and `db` from `worker_skin` (fix by hand; grep `import .*storage`); `ai_worker.core.health` / `logging` users (if any) → `worker_common.*`; `tests.vision` → `tests`; `tests.benchmarks` stays; `VISION_MODEL_STORAGE_DIR` → `SKIN_MODEL_STORAGE_DIR`; `settings.vision_worker_public_url` → `settings.worker_public_url`; `benchmarks.jev` imports unchanged (the package moves with them). Comments naming "vision profile"/`ai_worker/vision` → "skin worker"/`worker_skin`. Then `grep -rn "ai_worker" apps/workers/skin` → no output.
- [ ] **Step 3: `worker_skin/settings.py`**:

```python
"""Skin worker settings (segment-and-pose, model registry and dispatch).
Shared settings: worker_common.settings."""
import os
from pathlib import Path

from worker_common.settings import BaseSettings

# A source checkout keeps registry files under apps/workers/skin/data/models
# (gitignored); the image sets SKIN_MODEL_STORAGE_DIR=/app/data/models.
DEFAULT_LOCAL_MODEL_STORAGE_DIR = Path(__file__).resolve().parents[1] / "data" / "models"
DEFAULT_PORT = "8088"


class SkinSettings(BaseSettings):
    @property
    def local_model_storage_dir(self) -> Path:
        """Fallback storage root when S3 isn't configured (models/<capability>/<file>)."""
        raw = os.getenv("SKIN_MODEL_STORAGE_DIR", "")
        base = Path(raw) if raw else DEFAULT_LOCAL_MODEL_STORAGE_DIR
        base.mkdir(parents=True, exist_ok=True)
        return base

    @property
    def worker_public_url(self) -> str:
        """Base URL this worker is reachable at from the browser, for
        local-storage download links (S3 uses presigned URLs instead)."""
        return os.getenv("SKIN_WORKER_PUBLIC_URL", f"http://localhost:{os.getenv('PORT', DEFAULT_PORT)}")

    # model_max_file_bytes, model_max_smoke_latency_ms, model_cache_max_bytes,
    # model_library_warn_bytes, schema_lock_timeout_seconds: copy the five
    # properties verbatim (with their docstrings) from
    # apps/ai-worker/ai_worker/core/settings.py, changing the docstring path
    # `ai_worker/vision/db.py` to `worker_skin/db.py`.


settings = SkinSettings()
```

Replace the comment with the five copied properties.

`worker_skin/storage.py` (the local fallback that used to live in `ai_worker/core/storage.py`):

```python
"""Model-registry file storage: S3 when configured (worker_common.storage),
otherwise plain files under SKIN_MODEL_STORAGE_DIR, keyed like the S3 objects."""
import asyncio
from pathlib import Path
from urllib.parse import quote

from worker_common import storage as s3
from worker_skin.settings import settings


def _local_path(key: str) -> Path:
    path = (settings.local_model_storage_dir / key).resolve()
    if settings.local_model_storage_dir.resolve() not in path.parents:
        raise ValueError(f"Invalid storage key: {key}")
    return path


async def upload_bytes(key: str, data: bytes, content_type: str | None = None) -> None:
    if settings.s3_configured:
        return await s3.upload_bytes(key, data, content_type=content_type)
    path = _local_path(key)
    path.parent.mkdir(parents=True, exist_ok=True)
    await asyncio.to_thread(path.write_bytes, data)


async def download_bytes(key: str) -> bytes:
    if settings.s3_configured:
        return await s3.download_bytes(key)
    path = _local_path(key)
    if not path.is_file():
        raise FileNotFoundError(key)
    return await asyncio.to_thread(path.read_bytes)


async def delete_object(key: str) -> None:
    if settings.s3_configured:
        return await s3.delete_object(key)
    await asyncio.to_thread(_local_path(key).unlink, True)


async def presigned_download_url(key: str, expires_in: int = 300) -> str:
    if settings.s3_configured:
        return await s3.presigned_download_url(key, expires_in=expires_in)
    # No presigning without S3: point at this worker's own file route.
    base = settings.worker_public_url.rstrip("/")
    return f"{base}/api/v1/models/local-file?key={quote(key, safe='')}"
```

Keep the original comments from `ai_worker/core/storage.py` on `_local_path` and the local-file branch.

- [ ] **Step 4: `worker_skin/worker.py`** — the old `profile.py` minus face:

```python
"""Skin worker: segment-and-pose (zones, head pose, hijab), the model
registry and capability dispatch. core-engine calls it at SKIN_WORKER_URL."""
from worker_common.model_mirror import MirrorNeeds
from worker_skin import db
from worker_skin.routers.healthz import router as healthz_router
from worker_skin.routers.models import router as models_router
from worker_skin.routers.segment import router as segment_router
from worker_skin.settings import settings

SERVICE_NAME = "worker-skin"
REMOVED_ENV = {"VISION_MODEL_STORAGE_DIR": "SKIN_MODEL_STORAGE_DIR",
               "VISION_AI_WORKER_URL": "SKIN_WORKER_PUBLIC_URL (this worker's own URL) / core-engine SKIN_WORKER_URL"}
ROUTERS = [healthz_router, models_router, segment_router]


async def startup() -> None:
    await db.init_pool()


async def shutdown() -> None:
    await db.close_pool()


async def health_checks() -> dict[str, tuple[str, bool]]:
    # (keep the original comment on DATABASE_URL disabled/down semantics)
    if not settings.database_url:
        return {"database": ("disabled", False)}
    up = await db.ping(settings.health_db_timeout_seconds)
    return {"database": ("up" if up else "down", True)}


def mirror_needs() -> MirrorNeeds:
    # Skin's models come from the registry, not the mirror.
    return MirrorNeeds()
```

Replace the `# (keep ...)` line with the original comment text from `profile.py`. `main.py`: the contract's three lines. `__init__.py`: one-line docstring.

- [ ] **Step 5: `pyproject.toml`** — like face's, with `name = "worker-skin"`, dependencies `opencv-python-headless>=4.9.0`, `mediapipe>=0.10.9`, `numpy>=1.26.0`, `onnxruntime>=1.17.0`, `asyncpg>=0.29.0`; `test` extra = the old `test` extra (`pytest`, `pytest-asyncio`, `pytest-benchmark`, `onnx`, `torch`, `torchvision`, `pillow`) keeping its comment; `jev` extra = the old `jev` extra with its comment; packages include `worker_skin*`; no package-data; pytest config:

```toml
[tool.pytest.ini_options]
asyncio_mode = "auto"
addopts = '-m "not benchmark and not jev"'
markers = [
    "benchmark: wall-clock dispatch benchmark; minutes long. Run with `npm run bench:skin`.",
    "jev: JEV benchmark smoke test; loads the real VLM named by JEV_MODEL_ID (skipped when unset). Run with `npm run test:jev`.",
]
```

- [ ] **Step 6: Tests.** `tests/__init__.py`, `tests/conftest.py` (common one). Move the skin-relevant tests out of `apps/ai-worker/tests/core/test_app.py` into `tests/test_worker.py`, adapted to `from worker_skin.main import app` / `create_app(worker)`:

```python
import subprocess
import sys
from pathlib import Path

from fastapi.testclient import TestClient

from worker_skin import db
from worker_skin.main import app

APP_DIR = Path(__file__).resolve().parents[1]


def test_skin_routes_mounted_and_face_is_not():
    paths = set(app.openapi()["paths"])
    assert {"/api/v1/segment-and-pose", "/api/v1/models/dispatch-zones", "/health", "/healthz"} <= paths
    assert "/api/v1/face-measure" not in paths and "/extract" not in paths


def test_health_database_disabled_without_database_url(monkeypatch):
    for k in ("DATABASE_URL", "DATABASE_PUBLIC_URL", "POSTGRES_URL"):
        monkeypatch.delenv(k, raising=False)
    res = TestClient(app).get("/health")
    assert res.status_code == 200
    assert res.json() == {"status": "ok", "service": "worker-skin", "checks": {"database": "disabled"}}


def test_health_503_when_database_configured_and_down(monkeypatch):
    monkeypatch.setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:1/db")

    async def ping(timeout):
        return False

    monkeypatch.setattr(db, "ping", ping)
    res = TestClient(app).get("/health")
    assert res.status_code == 503 and res.json()["checks"] == {"database": "down"}


def test_healthz_alias_keeps_vision_body():
    res = TestClient(app).get("/healthz")
    assert res.status_code == 200 and res.json()["service"] == "vision-ai-worker"


def test_skin_never_imports_face_colour_tryon_or_torch():
    code = ("import sys, worker_skin.main; "
            "print(any(m == 'torch' or m.split('.')[0] in ('worker_face','worker_colour','worker_tryon','ai_worker') for m in sys.modules))")
    out = subprocess.run([sys.executable, "-c", code], cwd=APP_DIR, capture_output=True, text=True, check=True)
    assert out.stdout.strip().splitlines()[-1] == "False"


def test_lifespan_opens_and_closes_the_registry_pool_once(monkeypatch):
    calls = []

    async def init_pool():
        calls.append("init")

    async def close_pool():
        calls.append("close")

    monkeypatch.setattr(db, "init_pool", init_pool)
    monkeypatch.setattr(db, "close_pool", close_pool)
    with TestClient(app):
        assert calls == ["init"]
    assert calls == ["init", "close"]
```

`tests/test_storage.py`: the local-fallback tests from `apps/ai-worker/tests/core/test_storage.py` if any exist, plus:

```python
import pytest

from worker_skin import storage


@pytest.fixture
def local(monkeypatch, tmp_path):
    for k in ("S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"):
        monkeypatch.delenv(k, raising=False)
    monkeypatch.setenv("SKIN_MODEL_STORAGE_DIR", str(tmp_path))
    monkeypatch.setenv("SKIN_WORKER_PUBLIC_URL", "http://skin.example:8088")
    return tmp_path


async def test_without_s3_files_go_under_the_skin_storage_dir(local):
    await storage.upload_bytes("models/c/m.onnx", b"x")
    assert (local / "models/c/m.onnx").read_bytes() == b"x"
    assert await storage.download_bytes("models/c/m.onnx") == b"x"
    await storage.delete_object("models/c/m.onnx")
    assert not (local / "models/c/m.onnx").exists()


async def test_without_s3_download_links_point_at_this_worker(local):
    url = await storage.presigned_download_url("models/c/m.onnx")
    assert url == "http://skin.example:8088/api/v1/models/local-file?key=models%2Fc%2Fm.onnx"


async def test_keys_cannot_escape_the_storage_dir(local):
    with pytest.raises(ValueError):
        await storage.upload_bytes("../../etc/x", b"x")
```

`tests/test_settings.py`: the model-limit defaults from the old `test_defaults_match_both_old_workers` (`model_max_file_bytes == 200 * 1024 * 1024`, `model_max_smoke_latency_ms == 400`) against `SkinSettings()`, plus `SKIN_MODEL_STORAGE_DIR` honoured and `REMOVED_ENV` rejecting `VISION_MODEL_STORAGE_DIR` via `create_app(worker)`.

- [ ] **Step 7: `Dockerfile`**: face's, with `COPY apps/workers/skin /src/skin`, `pip install --no-cache-dir /src/common /src/skin`, `ENV PORT=8088 SKIN_MODEL_STORAGE_DIR=/app/data/models`, `EXPOSE 8088`, `CMD python -m worker_common.model_mirror sync worker_skin && exec uvicorn worker_skin.main:app --host 0.0.0.0 --port ${PORT}`, and the header comment `docker build -f apps/workers/skin/Dockerfile -t worker-skin .`.
- [ ] **Step 8: Old package without vision.** `apps/ai-worker/ai_worker/main.py`: `PROFILE_EXTRAS = {"tryon": "tryon", "colour": "colour"}`; `apps/ai-worker/ai_worker/core/settings.py`: `KNOWN_PROFILES = ("tryon", "colour")`, `DEFAULT_PROFILES = ("colour",)`, drop `vision_worker_public_url`, `model_*`, `schema_lock_timeout_seconds`; keep `local_model_storage_dir` unchanged (still `VISION_MODEL_STORAGE_DIR`), because colour's temporary landmarker default from Task 3 uses it. The old package is transitional: it is deleted in Task 8 and never deployed from this branch. Delete the vision tests from `tests/core/test_app.py`; tests that exercised profile loading generically (`test_unknown_profile_is_a_startup_error`, the missing-extra tests, `test_failed_startup_shuts_down_the_profiles_that_started`) switch to profiles `tryon`/`colour` or are removed with a note if they only made sense with vision. `.gitignore`: add `apps/workers/*/data/` and `/apps/workers/skin/benchmarks/jev/local/`.
- [ ] **Step 9: Run.** `pip install -e "apps/workers/skin[test]"`; `python -m pytest apps/workers/skin -q`; `python -m pytest apps/workers/common -q` (boundary test now scans face and skin); `python -m pytest apps/ai-worker -q`. Record counts and the arithmetic.
- [ ] **Step 10: Commit.** `git commit -m "refactor(workers): vision becomes worker_skin"`

---

### Task 5: `worker_tryon`

**Files:** `git mv apps/ai-worker/ai_worker/tryon apps/workers/tryon/worker_tryon`; `git mv apps/ai-worker/tests/tryon/* apps/workers/tryon/tests/`; `worker_tryon/profile.py` → content into `worker_tryon/worker.py`; create `worker_tryon/{main,settings}.py`, `apps/workers/tryon/{pyproject.toml,Dockerfile}`, `tests/test_storage.py`; modify old `ai_worker/{main.py,core/settings.py}` (drop tryon), old `tests/core/test_storage.py` (drop tryon tests).

**Interfaces:** Produces `worker_tryon.settings.settings` with `match_engine_callback_base_url`, `face_parsing_model_id` (+ base S3).

- [ ] **Step 1: Move and rewrite imports**: `ai_worker.tryon` → `worker_tryon`; `from ai_worker.core import storage` → `from worker_common import storage`; `ai_worker.core.settings` → `worker_tryon.settings`; `ai_worker.main import create_app` in tests → `from worker_tryon.main import app` (`create_app(["tryon"])` → `app`); `tests.tryon` → `tests`. In `tests/conftest.py` keep the collect-ignore guard, drop the `tryon` marker hook (the suite is its own process now) and fix its docstring (`pip install -e apps/workers/common -e "apps/workers/tryon[test]"`). `grep -rn "ai_worker" apps/workers/tryon` → none.
- [ ] **Step 2: `worker_tryon/settings.py`**:

```python
"""Try-on worker settings. Shared settings: worker_common.settings."""
import os

from worker_common.settings import BaseSettings

# tryon-engine's default before the ai-worker merge.
DEFAULT_FACE_PARSING_MODEL_ID = "jonathandinu/face-parsing"


class TryonSettings(BaseSettings):
    @property
    def match_engine_callback_base_url(self) -> str:
        # e.g. http://core-engine:8082/api/matching -- where the extraction callback lives
        return os.getenv("MATCH_ENGINE_URL", "") + "/api/matching"

    @property
    def face_parsing_model_id(self) -> str:
        return os.getenv("FACE_PARSING_MODEL_ID", DEFAULT_FACE_PARSING_MODEL_ID)


settings = TryonSettings()
```

- [ ] **Step 3: `worker_tryon/worker.py`**: the old `profile.py` body plus

```python
from pathlib import Path

from worker_common.model_mirror import MirrorNeeds

SERVICE_NAME = "worker-tryon"
REMOVED_ENV: dict[str, str] = {}


def mirror_needs() -> MirrorNeeds:
    from huggingface_hub import constants  # tryon's dependencies bring it

    return MirrorNeeds(hf_cache=Path(constants.HF_HUB_CACHE))
```

`main.py`: the contract's three lines.
- [ ] **Step 4: `tests/test_storage.py`**: move `test_tryon_upload_returns_the_object_key` and `test_tryon_upload_refuses_local_fallback` from the old storage test, with the `_FakeS3`/`s3` fixture copied from `apps/workers/common/tests/test_storage.py` (importing `from worker_common import storage` and `from worker_tryon.services import storage as tryon_storage`).
- [ ] **Step 5: `pyproject.toml`**: `name = "worker-tryon"`, dependencies = the old `tryon` extra plus `numpy>=1.26.0`, `opencv-python-headless>=4.9.0` if the moved code imports them (check `grep -rh "^import\|^from" worker_tryon`); `test` extra `pytest`, `pytest-asyncio`; packages `worker_tryon*`; pytest `asyncio_mode = "auto"`.
- [ ] **Step 6: `Dockerfile`**: the old `gpu` stage, copying `apps/workers/common` and `apps/workers/tryon` into `/src`, `pip install --no-cache-dir /src/common /src/tryon && rm -rf /src`, `ENV PORT=8090 HF_HUB_OFFLINE=1` (keep its comment), `CMD python -m worker_common.model_mirror sync worker_tryon && exec uvicorn worker_tryon.main:app --host 0.0.0.0 --port ${PORT}`.
- [ ] **Step 7:** Old package: `PROFILE_EXTRAS = {"colour": "colour"}`, `KNOWN_PROFILES = ("colour",)`, drop `match_engine_callback_base_url`/`face_parsing_model_id`, the `tryon` extra and marker in `apps/ai-worker/pyproject.toml`.
- [ ] **Step 8: Run.** `pip install -e "apps/workers/tryon[test]"`; `python -m pytest apps/workers/tryon -q` (≈12 min, background; expect the baseline's 8 tryon tests + the 2 moved storage tests); common and old suites. Record counts.
- [ ] **Step 9: Commit.** `git commit -m "refactor(workers): tryon becomes worker_tryon"`

---

### Task 6: Tooling (compose, scripts, npm, dockerignore)

**Files:** `docker-compose.yml`, `scripts/worker.mjs` (new; delete `scripts/ai-worker.mjs`), `package.json`, `.dockerignore`.

- [ ] **Step 1: `scripts/worker.mjs`**:

```js
// Runs one Python worker for npm scripts. npm uses cmd.exe on Windows, where
// `VAR=value cmd` does not work, so the env is set here. Node built-ins only.
import { spawn } from "node:child_process";

// Worker -> port. Ports are global across the Seagull repositories (docs/PORTS.md).
const WORKER_PORTS = { skin: "8088", tryon: "8090", colour: "8092", face: "8094" };

const [command, worker] = process.argv.slice(2);
if (command !== "dev" || !WORKER_PORTS[worker]) {
  console.error(`usage: node scripts/worker.mjs dev <${Object.keys(WORKER_PORTS).join("|")}>`);
  process.exit(2);
}

const port = WORKER_PORTS[worker];
const appDir = `apps/workers/${worker}`;
const env = { ...process.env, PORT: port };

function run(args, options, onOk) {
  const child = spawn("python", args, { stdio: "inherit", env, ...options });
  child.on("exit", (code) => (code === 0 && onOk ? onOk() : process.exit(code ?? 1)));
  child.on("error", (err) => {
    console.error(`could not start python: ${err.message} (is Python on PATH?)`);
    process.exit(1);
  });
}

// Pull this worker's models from the S3 mirror first (docs/MODEL-MIRROR.md);
// a no-op without S3 or without a mirror.
run(["-m", "worker_common.model_mirror", "sync", `worker_${worker}`], { cwd: appDir }, () =>
  run(["-m", "uvicorn", `worker_${worker}.main:app`, "--app-dir", appDir, "--port", port, "--reload"], {}),
);
```

Until Task 7 lands, `colour` is not under `apps/workers`; `dev:colour` keeps `node scripts/ai-worker.mjs dev colour` — so keep `scripts/ai-worker.mjs` (colour only: `PROFILE_PORTS = { colour: "8092" }`) and delete it in Task 7.

- [ ] **Step 2: `package.json` scripts** (replace the worker-related ones; leave Go ones as they are):

```json
"dev:skin": "node scripts/worker.mjs dev skin",
"dev:face": "node scripts/worker.mjs dev face",
"dev:tryon": "node scripts/worker.mjs dev tryon",
"dev:colour": "node scripts/ai-worker.mjs dev colour",
"dev": "concurrently -n core,ref,skin,face -c \"green,yellow,red,magenta\" \"npm run dev:core\" \"npm run dev:ref\" \"npm run dev:skin\" \"npm run dev:face\"",
"test:common": "pytest apps/workers/common",
"test:skin": "pytest apps/workers/skin",
"test:face": "pytest apps/workers/face",
"test:tryon": "pytest apps/workers/tryon",
"test:colour": "pytest apps/ai-worker -m colour",
"test:jev": "pytest apps/workers/skin -m jev",
"test:workers": "npm run test:common && npm run test:skin && npm run test:face",
"test": "npm run test:go && npm run test:workers",
"bench:skin": "pytest apps/workers/skin/tests/test_dispatch_benchmark.py -m benchmark --benchmark-only",
```

Remove `test:vision` and `bench:vision`.

- [ ] **Step 3: `docker-compose.yml`**: core-engine `environment` lines become

```yaml
      - SKIN_WORKER_URL=${SKIN_WORKER_URL:-http://worker-skin:8088}
      - FACE_WORKER_URL=${FACE_WORKER_URL:-http://worker-face:8094}
      - TRYON_ENGINE_URL=${TRYON_ENGINE_URL:-http://worker-tryon:8090}
      - COLOUR_WORKER_URL=${COLOUR_WORKER_URL:-http://ai-worker-colour:8092}
```

Replace `ai-worker-vision` with `worker-skin` (dockerfile `apps/workers/skin/Dockerfile`, no `target`, port 8088, same healthcheck on 8088) and add `worker-face` (dockerfile `apps/workers/face/Dockerfile`, port `"8094:8094"`, healthcheck on 8094, comment: face_landmarker.task comes from the mirror; without it /health reports it unavailable and face-measure answers 503). Replace `ai-worker-tryon` with `worker-tryon` (dockerfile `apps/workers/tryon/Dockerfile`, no target, rest unchanged). `ai-worker-colour` stays until Task 7.
- [ ] **Step 4: `.dockerignore`**: add `apps/workers/*/tests`, `apps/workers/*/data`, `apps/workers/skin/benchmarks`, `**/__pycache__`.
- [ ] **Step 5: Verify.** `docker compose config --services` is not available locally (no Docker); instead `python -c "import yaml,sys; yaml.safe_load(open('docker-compose.yml'))"` (install `pyyaml` in the venv if missing) and `node scripts/worker.mjs` prints the usage with `skin|tryon|colour|face` and exits 2. `npm run test:workers` passes.
- [ ] **Step 6: Commit.** `git commit -m "build(workers): compose services, dev launcher and npm scripts per worker"`

---

### Task 7: `worker_colour` (gated)

**Precondition:** the controller has the JEV session's agreement (colour lane) recorded in the ledger. Without it, skip this task and Task 8's deletion of `apps/ai-worker/` (it then still holds colour).

**Files:** `git mv apps/ai-worker/ai_worker/colour apps/workers/colour/worker_colour`; `git mv apps/ai-worker/tests/colour/* apps/workers/colour/tests/`; `worker_colour/profile.py` → `worker_colour/worker.py`; create `worker_colour/{main,settings}.py`, `apps/workers/colour/{pyproject.toml,Dockerfile}`; then delete the old package's remaining files (Task 8).

- [ ] **Step 1: Move and rewrite imports**: `ai_worker.colour` → `worker_colour`; `ai_worker.core.settings` → `worker_colour.settings`; tests `from ai_worker.main import create_app` / `create_app(["colour"])` → `from worker_colour.main import app` / `app`; `tests.colour.` → `tests.`; `settings.colour_face_landmarker_path` unchanged (the attribute keeps its name). In `tests/conftest.py` keep the collect-ignore guard, drop the marker hook, fix the install line. `test_profile.py` → `test_worker.py`, its "never imports" check becomes `m == 'torch' or m.split('.')[0] in ('worker_skin','worker_face','worker_tryon','ai_worker')`. `grep -rn "ai_worker" apps/workers/colour` → none.
- [ ] **Step 2: `worker_colour/settings.py`**:

```python
"""Colour worker settings (WCPA). Shared settings: worker_common.settings."""
import os
from pathlib import Path

from worker_common.settings import BaseSettings

_APP_DIR = Path(__file__).resolve().parents[1]
# Local files, not in git (apps/workers/*/data/ is gitignored); the image sets
# COLOUR_LANDMARKER_PATH and LASH_TEMPLATE_DIR explicitly.
DEFAULT_LANDMARKER_PATH = _APP_DIR / "data" / "models" / "face_landmarker" / "face_landmarker.task"
DEFAULT_LASH_TEMPLATE_DIR = _APP_DIR / "data" / "lash_templates"
DEFAULT_LASH_TEMPLATE_SOURCE = "procedural"
# The render recipe ships with the code.
DEFAULT_RENDER_PARAMS_PATH = Path(__file__).resolve().parent / "render_params.json"
# The WCPA notebook's chroma; the thresholds and seeded shade chroma were made with it.
DEFAULT_WCPA_CHROMA_METHOD = "approximation"


class ColourSettings(BaseSettings):
    @property
    def colour_face_landmarker_path(self) -> str:
        """MediaPipe Face Landmarker bundle the colour worker measures and
        renders with (COLOUR_LANDMARKER_PATH). Its own file: the face worker
        pins a specific build, colour does not."""
        return os.getenv("COLOUR_LANDMARKER_PATH", "") or str(DEFAULT_LANDMARKER_PATH)

    # lash_template_source, lash_template_dir, render_params_path,
    # wcpa_chroma_method: copy the four properties verbatim (with docstrings)
    # from apps/ai-worker/ai_worker/core/settings.py, fixing docstring paths
    # ai_worker/colour/... -> worker_colour/....


settings = ColourSettings()
```

Replace the comment with the four copied properties. Colour does NOT list `FACE_LANDMARKER_PATH` in `REMOVED_ENV`: it is the face worker's valid setting and may be set in a shared `.env`; colour simply ignores it.

- [ ] **Step 3: `worker_colour/worker.py`**: the old `profile.py` body plus

```python
from pathlib import Path

from worker_common.model_mirror import MirrorNeeds
from worker_colour.settings import settings

SERVICE_NAME = "worker-colour"
REMOVED_ENV: dict[str, str] = {}


def mirror_needs() -> MirrorNeeds:
    # Colour does not pin a landmarker build (docs/MODEL-MIRROR.md).
    return MirrorNeeds(landmarker_paths=(Path(settings.colour_face_landmarker_path),))
```

`main.py`: the contract's three lines.
- [ ] **Step 4: `pyproject.toml`**: `name = "worker-colour"`, dependencies = `opencv-python-headless>=4.9.0`, `numpy>=1.26.0` plus the old `colour` extra (with its comment); `test` extra `pytest`, `pytest-asyncio`; packages `worker_colour*`; package-data `"worker_colour" = ["render_params.json"]`; pytest `asyncio_mode = "auto"`.
- [ ] **Step 5: `Dockerfile`**: face's system packages, `COPY apps/workers/colour /src/colour`, `pip install --no-cache-dir /src/common /src/colour && rm -rf /src`, `ENV PORT=8092 COLOUR_LANDMARKER_PATH=/app/data/models/face_landmarker/face_landmarker.task LASH_TEMPLATE_DIR=/app/data/lash_templates`, `EXPOSE 8092`, `CMD python -m worker_common.model_mirror sync worker_colour && exec uvicorn worker_colour.main:app --host 0.0.0.0 --port ${PORT}`.
- [ ] **Step 6: Tooling**: `package.json` `"dev:colour": "node scripts/worker.mjs dev colour"`, `"test:colour": "pytest apps/workers/colour"`; delete `scripts/ai-worker.mjs`; compose: replace `ai-worker-colour` with `worker-colour` (dockerfile `apps/workers/colour/Dockerfile`) and core-engine's `COLOUR_WORKER_URL` default to `http://worker-colour:8092`.
- [ ] **Step 7: Run.** `pip install -e "apps/workers/colour[test]"`; `python -m pytest apps/workers/colour -q` — equals the baseline's colour count (landmarker-dependent tests skip without `COLOUR_LANDMARKER_PATH`, as before without the file). Common boundary test.
- [ ] **Step 8: Commit.** `git commit -m "refactor(workers): colour becomes worker_colour"`

---

### Task 8: Remove `apps/ai-worker` and update docs

- [ ] **Step 1:** If Task 7 ran: `git rm -r apps/ai-worker` (by now it holds only `ai_worker/{__init__,main}.py`, `ai_worker/core/`, `tests/core`, `tests/conftest.py`, `pyproject.toml`, `Dockerfile`, `data/` (untracked)). Check first: `git ls-files apps/ai-worker` lists nothing that was not accounted for in Tasks 1–7; anything else is moved to its owner or reported. If Task 7 was skipped, leave `apps/ai-worker` as the colour-only transitional package and say so in `README.md`.
- [ ] **Step 2: Docs**, each edited to the new names, paths, ports and commands (no other content changes):
  - `docs/PORTS.md`: `8088 | worker-skin`, `8090 | worker-tryon`, `8092 | worker-colour` (or `ai-worker (colour profile)` if Task 7 skipped), new `8094 | worker-face`.
  - `docs/MODEL-MIRROR.md`: `python -m worker_common.model_mirror sync <worker package>`; per-worker needs (face pins, colour unpinned, tryon HF, skin none); `COLOUR_LANDMARKER_PATH`/`FACE_LANDMARKER_PATH`.
  - `docs/MODEL-REGISTRY.md`, `docs/FACE-MEASURE.md`, `docs/FACE-ARCHITECTURE.md` (worker URL → `FACE_WORKER_URL`), `docs/COLOUR-LOCAL-RUN.md`, `docs/SHARED-DATABASE.md` (registry tables owned by `apps/workers/skin` (`worker_skin/db.py`)), `docs/OBJECT-STORAGE.md` (storage module paths), `README.md`, `AGENTS.md` (first paragraph: workers `skin`, `face`, `colour`, `tryon` under `apps/workers/`).
  - `grep -rn "ai_worker\|apps/ai-worker\|AI_WORKER_PROFILES\|VISION_AI_WORKER_URL\|VISION_MODEL_STORAGE_DIR\|VISION_FACE_LANDMARKER_PATH\|dev:vision\|test:vision" --include=*.md . | grep -v "docs/superpowers/"` → only intentional historical mentions, each listed in the report.
- [ ] **Step 3: Full check.** `npm run check`; `npm run test:tryon` (background); `npm run test:colour` (if Task 7 ran). Compare with the Task 0 baseline and explain every difference.
- [ ] **Step 4: Commit.** `git commit -m "refactor(workers): remove apps/ai-worker; docs for apps/workers"`

---

### Task 9: Image and wiring verification (VPS build, no deploy)

Docker is not installed on the laptop; images are built on the VPS (`ssh seagull-vps`) from a throwaway copy of the branch, and nothing running there is touched.

- [ ] **Step 1:** `git archive --format=tar refactor/workers-split | ssh seagull-vps 'rm -rf /tmp/workers-split && mkdir /tmp/workers-split && tar -x -C /tmp/workers-split'`.
- [ ] **Step 2:** On the VPS, for `skin` and `face` (and `colour` if Task 7 ran): `docker build -f apps/workers/<w>/Dockerfile -t worker-<w>:verify /tmp/workers-split`.
- [ ] **Step 3:** For each built image:

```bash
docker run --rm worker-face:verify python -c "import importlib.util as u, sys; bad=[m for m in ('worker_skin','worker_colour','worker_tryon','ai_worker','onnxruntime','asyncpg','torch') if u.find_spec(m)]; print(bad); sys.exit(1 if bad else 0)"
docker run --rm worker-skin:verify python -c "import importlib.util as u, sys; bad=[m for m in ('worker_face','worker_colour','worker_tryon','ai_worker','torch') if u.find_spec(m)]; print(bad); sys.exit(1 if bad else 0)"
docker run --rm -e PORT=8094 worker-face:verify python -c "from worker_face.main import app; print(sorted(app.openapi()['paths']))"
```

Expected: empty lists, exit 0; the face route list contains `/api/v1/face-measure` and `/health`.
- [ ] **Step 4:** Local wiring (laptop, free ports, no Docker): `python -m uvicorn worker_face.main:app --app-dir apps/workers/face --port 18094` in the background; `FACE_WORKER_URL=http://127.0.0.1:18094 go run ./apps/core-engine/cmd/facearch-profile validate apps/core-engine/cmd/facearch-profile/seed/<a seed profile json>` → "valid against …"; `VISION_AI_WORKER_URL=x go run ./apps/core-engine/cmd/server` exits with the removal error. Stop the worker.
- [ ] **Step 5:** Clean up: `ssh seagull-vps 'docker rmi worker-skin:verify worker-face:verify worker-colour:verify 2>/dev/null; rm -rf /tmp/workers-split'`.
- [ ] **Step 6:** Report results. No commit (no repo change).

---

## Rollout (after merge; needs the user's go-ahead, not part of the branch)

1. VPS `.env`: remove `VISION_AI_WORKER_URL` if present; core-engine gets `SKIN_WORKER_URL=http://worker-skin:8088`, `FACE_WORKER_URL=http://worker-face:8094` from compose defaults.
2. `cd ~/seagull-core && git pull`; update `compose.vps.yml` loopback ports for `worker-skin` (8088) and `worker-face` (8094); `docker compose ... up -d --build worker-skin worker-face core-engine`; remove the old `ai-worker-vision` container (`docker rm -f seagull-core-ai-worker-vision-1`).
3. `ai-worker-colour` → `worker-colour` only with the JEV session.
4. Laptop: `pip install -e apps/workers/common -e "apps/workers/skin[test]" -e "apps/workers/face[test]"` (and colour/tryon as needed); move gitignored local data: `apps/ai-worker/data/models/face_landmarker/face_landmarker.task` → `apps/workers/face/data/models/face_landmarker/`, registry files → `apps/workers/skin/data/models/`, `apps/ai-worker/data/colour/lash_templates` → `apps/workers/colour/data/lash_templates`.

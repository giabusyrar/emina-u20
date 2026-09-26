# ai-worker Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge `apps/vision-ai-worker` and `apps/tryon-engine` into one package, `apps/ai-worker` (import name `ai_worker`). One FastAPI app runs a `vision` profile, a `tryon` profile or both. There is one shared settings, storage, logging and health layer, and one Dockerfile with `cpu` and `gpu` targets.

**Architecture:**
- Each service's code moves with `git mv` under `ai_worker/vision/` and `ai_worker/tryon/`, keeping its internal layout.
- Shared pieces go in `ai_worker/core/`.
- `ai_worker/main.py:create_app(profiles)` imports only the enabled profiles' `profile.py` modules. Each module exposes `ROUTERS`, `startup()`, `shutdown()` and `health_checks()`.

**Tech Stack:** Python 3.12, FastAPI, pytest, asyncpg, boto3, onnxruntime/mediapipe (vision), torch/transformers/diffusers (tryon), Node (npm helper), Docker (file only).

**Spec:** `docs/superpowers/specs/2026-09-26-ai-worker-merge-design.md`

## Global Constraints

- **HTTP contracts unchanged.** Every `/api/v1/models/...` route stays as it is, including the deprecated 410 ones. The others must stay byte-identical:
  - `POST /api/v1/segment-and-pose`
  - `POST /api/v1/models/dispatch-zones`
  - `POST /extract`
  - the callback to `{MATCH_ENGINE_URL}/api/matching/shades/{shade_id}/extraction-callback`
- **Ports:** vision 8088, tryon 8090.
- **Env names:** every env var either service reads today keeps working, with the same defaults.
- **Registry DB:** `db.py`'s schema statements, `SCHEMA_ADVISORY_LOCK_ID = 1581646198` and the migration and lock logic move byte-identical.
- **core-engine (Go) is not modified.**
- **Baselines that must hold after every task:**
  - vision: `125 passed, 15 skipped, 6 deselected`, from `pytest -m "not benchmark and not tryon"` restricted to `tests/vision`;
  - tryon: `6 passed`, which takes about 8 minutes because it downloads models.

  Tasks state when to run the tryon suite.
- **Moves use `git mv`.** A task's move commit contains moves plus mechanical import rewrites only. Behaviour changes go in later commits.
- **Settings stay lazily read.** Env is read on every property access, never frozen at import.
- **Tryon never falls back to local-disk storage.** It must call `settings.require_s3()` before uploading.
- **Never `git push`.** Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Location:** work in worktree `.claude/worktrees/ai-worker`, branch `ai-worker-merge`. The lane claim is recorded in the main checkout's `docs/LANES.md`.
- **Commands** run from the worktree root in Git Bash, unless a step says otherwise.

## File map (end state)

| Path | Responsibility |
|---|---|
| `apps/ai-worker/pyproject.toml` | the only dependency list; extras `tryon`, `test`; pytest config and markers |
| `apps/ai-worker/Dockerfile` | `cpu` and `gpu` targets |
| `apps/ai-worker/ai_worker/main.py` | `create_app(profiles)`, lifespan, CORS, module-level `app` |
| `apps/ai-worker/ai_worker/core/settings.py` | one lazy `Settings`, named defaults |
| `apps/ai-worker/ai_worker/core/storage.py` | S3 + local fallback + `public_url` (was vision `storage.py`) |
| `apps/ai-worker/ai_worker/core/logging.py` | JSON-lines logging |
| `apps/ai-worker/ai_worker/core/health.py` | `/health` router |
| `apps/ai-worker/ai_worker/vision/...` | vision code (was `apps/vision-ai-worker/app/`), plus `profile.py`, `routers/segment.py` and `routers/healthz.py` |
| `apps/ai-worker/ai_worker/tryon/...` | tryon code (was `apps/tryon-engine/app/`), plus `profile.py` |
| `apps/ai-worker/tests/{core,vision,tryon}/` | tests |
| `scripts/ai-worker.mjs` | `dev <profile>` launcher (sets env vars portably) |

---

### Task 1: Move vision into `apps/ai-worker`

**Files:**
- Move: `apps/vision-ai-worker/app/` → `apps/ai-worker/ai_worker/vision/`
- Move: `apps/vision-ai-worker/tests/` → `apps/ai-worker/tests/vision/`
- Move: `apps/vision-ai-worker/pyproject.toml` → `apps/ai-worker/pyproject.toml`
- Create: `apps/ai-worker/ai_worker/__init__.py`, `apps/ai-worker/ai_worker/main.py` (temporary shim), `apps/ai-worker/tests/__init__.py`
- Modify: every `.py` under the moved trees (import rewrites only)
- Modify: `package.json` (`dev:vision`, `test:vision`, `bench:vision`)

**Interfaces:**
- Produces:
  - package `ai_worker.vision`, whose modules keep today's names: `ai_worker.vision.main`, `.config`, `.db`, `.storage`, `.routers.models`, `.models.*`, `.schemas.*`, `.zones`;
  - `ai_worker.main:app`, re-exported from `ai_worker.vision.main`.

- [ ] **Step 1: Move with git**

```bash
mkdir -p apps/ai-worker/ai_worker apps/ai-worker/tests
git mv apps/vision-ai-worker/app apps/ai-worker/ai_worker/vision
git mv apps/vision-ai-worker/tests apps/ai-worker/tests/vision
git mv apps/vision-ai-worker/pyproject.toml apps/ai-worker/pyproject.toml
ls apps/vision-ai-worker   # expect only requirements.txt (removed in Task 6) and untracked leftovers such as __pycache__
```

- [ ] **Step 2: Rewrite imports**

```bash
cd apps/ai-worker
grep -rl --include='*.py' -e 'from app' -e 'import app' -e '"app\.' -e "'app\." -e 'from tests' . \
  | xargs sed -i -E \
    -e 's/\bfrom app\.(\w)/from ai_worker.vision.\1/g' \
    -e 's/\bfrom app import /from ai_worker.vision import /g' \
    -e 's/\bimport app\.(\w)/import ai_worker.vision.\1/g' \
    -e 's/(["'"'"'])app\.(\w)/\1ai_worker.vision.\2/g' \
    -e 's/\bfrom tests\.(\w)/from tests.vision.\1/g'
cd ../..
grep -rnE '\bfrom app\b|\bimport app\b|"app\.|'"'"'app\.' apps/ai-worker --include='*.py' || echo "no app imports left"
```

Expected: `no app imports left`. `test_model_registry.py` had `logger="app.models.model_registry"`; confirm it now reads `logger="ai_worker.vision.models.model_registry"`.

- [ ] **Step 3: Package files and the temporary entry point**

`apps/ai-worker/ai_worker/__init__.py`:

```python
"""Seagull AI worker: the vision and try-on inference profiles in one package."""
```

`apps/ai-worker/tests/__init__.py`: empty file. `tests/vision/__init__.py` already exists; it moved with the tests.

`apps/ai-worker/ai_worker/main.py` (Task 5 replaces it):

```python
# Temporary entry point while the two workers are merged: serves exactly the
# vision app. Replaced by create_app() in the profiles task.
from ai_worker.vision.main import app  # noqa: F401
```

- [ ] **Step 4: pyproject name and package discovery**

In `apps/ai-worker/pyproject.toml`:
- set `name = "ai-worker"`, `version = "0.3.0"` and `requires-python = ">=3.12"`;
- remove the line `readme = "README.md"`, since there is none;
- add the block below after `[build-system]`;
- change the install comment `pip install -e "apps/vision-ai-worker[test]"` to `pip install -e "apps/ai-worker[test]"`.

```toml
[tool.setuptools.packages.find]
include = ["ai_worker*"]
```

- [ ] **Step 5: npm scripts**

In `package.json`, replace these three commands. The names stay the same.

```json
    "dev:vision": "python -m uvicorn ai_worker.main:app --app-dir apps/ai-worker --port 8088 --reload",
    "test:vision": "pytest apps/ai-worker -m \"not benchmark\"",
    "bench:vision": "pytest apps/ai-worker/tests/vision/test_dispatch_benchmark.py -m benchmark --benchmark-only",
```

- [ ] **Step 6: Verify the baseline**

```bash
cd apps/ai-worker && python -m pytest tests/vision -q -m "not benchmark" 2>&1 | tail -1; cd ../..
npm run test:vision 2>&1 | tail -1
```

Expected, both times: `125 passed, 15 skipped, 6 deselected`.

- [ ] **Step 7: Commit**

```bash
git add -A apps/ai-worker apps/vision-ai-worker package.json
git commit -m "refactor(ai-worker): move vision-ai-worker into apps/ai-worker/ai_worker/vision" -m "Moves and import rewrites only." -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git show --stat -M HEAD | grep -c "=>"   # expect every moved file listed as a rename
```

---

### Task 2: Move tryon into `apps/ai-worker`

**Files:**
- Move: `apps/tryon-engine/app/` → `apps/ai-worker/ai_worker/tryon/`
- Move: `apps/tryon-engine/tests/` → `apps/ai-worker/tests/tryon/` (including `fixtures/`)
- Create: `apps/ai-worker/tests/tryon/conftest.py`
- Modify: `apps/ai-worker/pyproject.toml` (the `tryon` extra, the `tryon` marker, `python-dotenv`)
- Modify: `package.json` (`dev:tryon`, `test:tryon`, `test:vision` marker expression)

**Interfaces:**
- Consumes: the `ai_worker` package from Task 1.
- Produces: `ai_worker.tryon`, whose modules keep today's names: `.main`, `.config`, `.schemas`, `.routers.extraction`, `.services.{face_parsing,shade_generation,storage,uv_extraction}`.

- [ ] **Step 1: Move and rewrite**

```bash
git mv apps/tryon-engine/app apps/ai-worker/ai_worker/tryon
git mv apps/tryon-engine/tests apps/ai-worker/tests/tryon
cd apps/ai-worker
grep -rl --include='*.py' -e 'from app' -e 'import app' -e '"app\.' -e "'app\." ai_worker/tryon tests/tryon \
  | xargs sed -i -E \
    -e 's/\bfrom app\.(\w)/from ai_worker.tryon.\1/g' \
    -e 's/\bfrom app import /from ai_worker.tryon import /g' \
    -e 's/\bimport app\.(\w)/import ai_worker.tryon.\1/g' \
    -e 's/(["'"'"'])app\.(\w)/\1ai_worker.tryon.\2/g'
cd ../..
grep -rnE '\bfrom app\b|\bimport app\b|"app\.|'"'"'app\.' apps/ai-worker --include='*.py' || echo "no app imports left"
```

Expected: `no app imports left`. The `patch("app.routers.extraction...")` strings in `test_extraction_endpoint.py` now read `patch("ai_worker.tryon.routers.extraction...")`.

- [ ] **Step 2: Mark and gate the tryon tests**

`apps/ai-worker/tests/tryon/conftest.py`:

```python
"""Every test under tests/tryon is a `tryon` test and needs torch.

Marked here, not per file, so `-m "not tryon"` (npm run test:vision) and
`-m tryon` (npm run test:tryon) split the suite without touching test modules.
Skipped with a stated reason where torch is not installed, because the tryon
profile cannot import without it.
"""
import importlib.util

import pytest

_TORCH_MISSING = importlib.util.find_spec("torch") is None


def pytest_collection_modifyitems(config, items):
    for item in items:
        if "tests/tryon/" in item.nodeid.replace("\\", "/"):
            item.add_marker(pytest.mark.tryon)
            if _TORCH_MISSING:
                item.add_marker(pytest.mark.skip(reason="torch not installed: pip install -e \"apps/ai-worker[tryon]\""))
```

- [ ] **Step 3: pyproject**
- Add `"python-dotenv>=1.0.0"` to `[project].dependencies`. Both configs import it, and vision never declared it.
- Add the `tryon` extra with the versions installed on this machine as floors. Get them with `python -c "import torch, torchvision, transformers, diffusers, accelerate, PIL, scipy; print(torch.__version__, torchvision.__version__, transformers.__version__, diffusers.__version__, accelerate.__version__, PIL.__version__, scipy.__version__)"`. Use the lower of the installed version and the old `requirements.txt` floor. The old floors were `torch>=2.2.0`, `torchvision>=0.17.0`, `transformers>=4.41.0`, `diffusers>=0.27.0`, `accelerate>=0.30.0`, `pillow>=10.0.0`, `scipy>=1.12.0`, and the old ones are the safe choice:

```toml
tryon = [
    "torch>=2.2.0",
    "torchvision>=0.17.0",
    "transformers>=4.41.0",
    "diffusers>=0.27.0",
    "accelerate>=0.30.0",
    "pillow>=10.0.0",
    "scipy>=1.12.0",
]
```

- Change `addopts` to `'-m "not benchmark and not tryon"'`, and add a marker line:

```toml
    "tryon: try-on profile tests; need torch and download models (~8 min). Run with `npm run test:tryon`.",
```

- [ ] **Step 4: npm scripts**

```json
    "dev:tryon": "python -m uvicorn ai_worker.tryon.main:app --app-dir apps/ai-worker --port 8090 --reload",
    "test:vision": "pytest apps/ai-worker -m \"not benchmark and not tryon\"",
    "test:tryon": "pytest apps/ai-worker -m tryon",
```

`pytest.ini` at the repo root stays until Task 6.

- [ ] **Step 5: Verify both baselines, and one-run collection**

```bash
npm run test:vision 2>&1 | tail -1          # expect: 125 passed, 15 skipped, 12 deselected  (6 benchmark + 6 tryon)
npm run test:tryon 2>&1 | tail -1           # expect: 6 passed, 146 deselected; ~8 min
cd apps/ai-worker && python -m pytest --collect-only -q -m "" 2>&1 | tail -1; cd ../..   # expect: 152 tests collected (146 vision + 6 tryon), no errors
```

The vision number must be exactly `125 passed, 15 skipped`. The tryon run must be exactly `6 passed`. Collection must finish without errors: the shared `app` name that broke root runs is gone.

- [ ] **Step 6: Commit**

```bash
git add -A apps/ai-worker apps/tryon-engine package.json
git commit -m "refactor(ai-worker): move tryon-engine into apps/ai-worker/ai_worker/tryon" -m "Moves, import rewrites and the tryon marker only." -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: One shared lazy `Settings`

**Files:**
- Create: `apps/ai-worker/ai_worker/core/__init__.py` (empty), `apps/ai-worker/ai_worker/core/settings.py`
- Delete: `apps/ai-worker/ai_worker/vision/config.py`, `apps/ai-worker/ai_worker/tryon/config.py`
- Modify: every importer of `ai_worker.vision.config` / `ai_worker.tryon.config` → `ai_worker.core.settings`
- Modify: `.gitignore` (add `apps/ai-worker/data/`)
- Test: `apps/ai-worker/tests/core/__init__.py` (empty), `apps/ai-worker/tests/core/test_settings.py`

**Interfaces:**
- Produces, from `ai_worker.core.settings`:
  - constants `SERVICE_NAME = "ai-worker"`, `KNOWN_PROFILES = ("vision", "tryon")`, `DEFAULT_PROFILES = ("vision",)`, `DEFAULT_LOG_LEVEL = "INFO"`, `DEFAULT_HEALTH_DB_TIMEOUT_SECONDS = 3`, `DEFAULT_LOCAL_MODEL_STORAGE_DIR: Path`, `DEFAULT_FACE_PARSING_MODEL_ID = "jonathandinu/face-parsing"`;
  - `class Settings` with every property the two old classes had, plus `profiles -> tuple[str, ...]`, `log_level -> str` and `health_db_timeout_seconds -> int`;
  - the module-level `settings = Settings()`.

- [ ] **Step 1: Write the failing tests**

`apps/ai-worker/tests/core/test_settings.py`:

```python
import pytest

from ai_worker.core import settings as settings_module
from ai_worker.core.settings import Settings

ENV = [
    "DATABASE_URL", "DATABASE_PUBLIC_URL", "POSTGRES_URL", "S3_BUCKET", "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY", "S3_REGION", "S3_ENDPOINT", "MATCH_ENGINE_URL", "FACE_PARSING_MODEL_ID",
    "LOG_LEVEL", "AI_WORKER_PROFILES", "VISION_MODEL_STORAGE_DIR", "HEALTH_DB_TIMEOUT_SECONDS",
]


@pytest.fixture
def env(monkeypatch):
    for name in ENV:
        monkeypatch.delenv(name, raising=False)
    return monkeypatch


def test_reads_env_lazily_on_every_access(env):
    s = Settings()
    assert s.database_url == ""
    env.setenv("POSTGRES_URL", "postgres://c")
    assert s.database_url == "postgres://c"
    env.setenv("DATABASE_PUBLIC_URL", "postgres://b")
    assert s.database_url == "postgres://b"
    env.setenv("DATABASE_URL", "postgres://a")
    assert s.database_url == "postgres://a"


def test_defaults_match_both_old_workers(env):
    s = Settings()
    assert s.s3_region == "auto"
    assert s.face_parsing_model_id == "jonathandinu/face-parsing"
    assert s.model_max_file_bytes == 200 * 1024 * 1024
    assert s.model_max_smoke_latency_ms == 400
    assert s.schema_lock_timeout_seconds == 60
    assert s.profiles == ("vision",)
    assert s.log_level == "INFO"
    assert s.health_db_timeout_seconds == 3


def test_match_engine_callback_base_url_appends_api_matching(env):
    env.setenv("MATCH_ENGINE_URL", "http://core:8082")
    assert Settings().match_engine_callback_base_url == "http://core:8082/api/matching"


def test_require_s3_names_missing_vars(env):
    env.setenv("S3_BUCKET", "b")
    with pytest.raises(RuntimeError, match="S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY"):
        Settings().require_s3()


def test_profiles_parse_and_trim(env):
    env.setenv("AI_WORKER_PROFILES", " vision , tryon ")
    assert Settings().profiles == ("vision", "tryon")


@pytest.mark.parametrize("raw,expected", [("debug", "DEBUG"), ("warn", "WARNING"), ("Error", "ERROR")])
def test_log_level_normalised(env, raw, expected):
    env.setenv("LOG_LEVEL", raw)
    assert Settings().log_level == expected


def test_invalid_log_level_raises(env):
    env.setenv("LOG_LEVEL", "loud")
    with pytest.raises(RuntimeError, match="LOG_LEVEL"):
        Settings().log_level


def test_local_model_storage_default_is_under_apps_ai_worker(env, tmp_path, monkeypatch):
    assert settings_module.DEFAULT_LOCAL_MODEL_STORAGE_DIR.parts[-3:] == ("ai-worker", "data", "models")
    monkeypatch.setattr(settings_module, "DEFAULT_LOCAL_MODEL_STORAGE_DIR", tmp_path / "m")
    assert Settings().local_model_storage_dir == tmp_path / "m"
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd apps/ai-worker && python -m pytest tests/core -q; cd ../..`
Expected: FAIL at collection with `ModuleNotFoundError: No module named 'ai_worker.core'`.

- [ ] **Step 3: Implement `ai_worker/core/settings.py`**

Build this file from the two current `config.py` files. Copy **every** property of `ai_worker/vision/config.py` verbatim, docstrings included: `database_url`, `s3_*`, `s3_configured`, `local_model_storage_dir`, `vision_worker_public_url`, `require_s3`, `require_database`, `_int_env`, `model_max_file_bytes`, `model_max_smoke_latency_ms`, `model_cache_max_bytes`, `model_library_warn_bytes`, `schema_lock_timeout_seconds`. Then apply exactly these changes:

```python
"""Configuration for every ai-worker profile.

Read lazily, on every property access, never frozen at import: the vision
profile's segment-and-pose must keep working while S3 or the model-registry
database is not configured, and only a feature that needs a missing value
raises. Tests rely on this too (they set env at runtime).
"""
import os
from pathlib import Path

from dotenv import load_dotenv

# Find the repo-root .env regardless of this process's cwd, the same way the
# Go services' goplatform.LoadDotEnv does, so DATABASE_URL/S3_* are picked up
# whether run via `npm run dev:vision` (cwd = repo root) or from apps/ai-worker.
for _parent in Path(__file__).resolve().parents:
    _candidate = _parent / ".env"
    if _candidate.exists():
        load_dotenv(_candidate)
        break

SERVICE_NAME = "ai-worker"
KNOWN_PROFILES = ("vision", "tryon")
# A bare `uvicorn ai_worker.main:app` behaves like the old vision-ai-worker.
DEFAULT_PROFILES = ("vision",)
DEFAULT_LOG_LEVEL = "INFO"
# Same default as the Go services' DEPENDENCY_PING_TIMEOUT (packages/goplatform/common.go).
DEFAULT_HEALTH_DB_TIMEOUT_SECONDS = 3
# apps/ai-worker/data/models -- was apps/vision-ai-worker/data/models before the merge.
DEFAULT_LOCAL_MODEL_STORAGE_DIR = Path(__file__).resolve().parents[2] / "data" / "models"
# tryon-engine's default before the merge.
DEFAULT_FACE_PARSING_MODEL_ID = "jonathandinu/face-parsing"

# Accepted LOG_LEVEL spellings; "WARN" matches the Go services' slog names.
_LOG_LEVELS = {"DEBUG": "DEBUG", "INFO": "INFO", "WARN": "WARNING", "WARNING": "WARNING", "ERROR": "ERROR"}


def _first(*names: str) -> str:
    for name in names:
        val = os.getenv(name)
        if val:
            return val
    return ""


class Settings:
    # ... every property copied from vision's config.py, with two edits:
    #   local_model_storage_dir: `base = Path(raw) if raw else DEFAULT_LOCAL_MODEL_STORAGE_DIR`
    #   (reference the module global by name, so tests can monkeypatch it)
    #   schema_lock_timeout_seconds docstring: "`app/db.py`" -> "`ai_worker/vision/db.py`"

    @property
    def match_engine_callback_base_url(self) -> str:
        # e.g. http://localhost:8082/api/matching -- where CreateShade lives
        return os.getenv("MATCH_ENGINE_URL", "") + "/api/matching"

    @property
    def face_parsing_model_id(self) -> str:
        return os.getenv("FACE_PARSING_MODEL_ID", DEFAULT_FACE_PARSING_MODEL_ID)

    @property
    def profiles(self) -> tuple[str, ...]:
        raw = os.getenv("AI_WORKER_PROFILES", "")
        names = tuple(p.strip() for p in raw.split(",") if p.strip())
        return names or DEFAULT_PROFILES

    @property
    def log_level(self) -> str:
        raw = os.getenv("LOG_LEVEL", "").strip().upper() or DEFAULT_LOG_LEVEL
        if raw not in _LOG_LEVELS:
            raise RuntimeError(f"LOG_LEVEL: unknown level {raw!r}; use debug, info, warn or error.")
        return _LOG_LEVELS[raw]

    @property
    def health_db_timeout_seconds(self) -> int:
        return self._int_env("HEALTH_DB_TIMEOUT_SECONDS", DEFAULT_HEALTH_DB_TIMEOUT_SECONDS)


settings = Settings()
```

Replace the comment block inside `class Settings` with the copied properties. The `# ...` lines are instructions for this step, not code to keep. `require_s3` keeps vision's message, `"S3 is not configured: missing ..."`.

- [ ] **Step 4: Point the importers at the shared settings and delete the old configs**

```bash
cd apps/ai-worker
grep -rl --include='*.py' -e 'ai_worker.vision.config' -e 'ai_worker.tryon.config' -e 'from ai_worker.vision import config' -e 'from ai_worker.tryon import config' . \
  | xargs sed -i -E \
    -e 's/from ai_worker\.(vision|tryon)\.config import/from ai_worker.core.settings import/g' \
    -e 's/ai_worker\.(vision|tryon)\.config/ai_worker.core.settings/g'
git rm -q ai_worker/vision/config.py ai_worker/tryon/config.py
grep -rn "\.config\b" ai_worker tests --include='*.py' || echo "no config module imports left"
cd ../..
printf '\n# ai-worker local model storage (VISION_MODEL_STORAGE_DIR default)\napps/ai-worker/data/\n' >> .gitignore
```

- [ ] **Step 5: Run everything that could break**

```bash
cd apps/ai-worker && python -m pytest tests/core -q 2>&1 | tail -1; cd ../..   # expect: 10 passed (8 test functions; the log-level one is parametrized x3)
npm run test:vision 2>&1 | tail -1                                           # expect: 135 passed, 15 skipped, 12 deselected (125 vision + 10 core)
```

Run `npm run test:tryon`. Expect `6 passed`. It takes about 8 minutes; this task changed tryon's settings import.

- [ ] **Step 6: Commit**

```bash
git add -A apps/ai-worker .gitignore
git commit -m "refactor(ai-worker): one shared lazily-read Settings for both profiles" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: One storage module

**Files:**
- Move: `apps/ai-worker/ai_worker/vision/storage.py` → `apps/ai-worker/ai_worker/core/storage.py`
- Modify: `ai_worker/core/storage.py` (add a `content_type` parameter and `public_url`)
- Modify: `ai_worker/tryon/services/storage.py` (becomes a thin wrapper)
- Modify: importers of `ai_worker.vision.storage` / `from ai_worker.vision import storage`
- Test: `apps/ai-worker/tests/core/test_storage.py`

**Interfaces:**
- Consumes: `ai_worker.core.settings.settings`.
- Produces:
  - `ai_worker.core.storage.upload_bytes(key: str, data: bytes, content_type: str | None = None) -> None`;
  - `public_url(key: str) -> str`;
  - unchanged: `download_bytes`, `delete_object`, `presigned_download_url`, `_get_client`, `_local_path`;
  - `ai_worker.tryon.services.storage.upload_bytes(key, data, content_type="image/png") -> str`, the same as today.

- [ ] **Step 1: Move and repoint**

```bash
git mv apps/ai-worker/ai_worker/vision/storage.py apps/ai-worker/ai_worker/core/storage.py
cd apps/ai-worker
grep -rl --include='*.py' -e 'from ai_worker.vision import storage' -e 'ai_worker.vision.storage' . \
  | xargs sed -i -E \
    -e 's/from ai_worker\.vision import storage/from ai_worker.core import storage/g' \
    -e 's/ai_worker\.vision\.storage/ai_worker.core.storage/g'
cd ../..
git add -A apps/ai-worker
git commit -q -m "refactor(ai-worker): move vision storage to ai_worker.core.storage" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
npm run test:vision 2>&1 | tail -1   # expect unchanged: 135 passed, 15 skipped, 12 deselected
```

- [ ] **Step 2: Write the failing tests**

`apps/ai-worker/tests/core/test_storage.py`:

```python
import pytest

from ai_worker.core import storage
from ai_worker.tryon.services import storage as tryon_storage


class _FakeS3:
    def __init__(self):
        self.calls = []

    def put_object(self, **kwargs):
        self.calls.append(kwargs)


@pytest.fixture
def s3(monkeypatch):
    for k, v in {"S3_BUCKET": "bkt", "S3_ACCESS_KEY_ID": "id", "S3_SECRET_ACCESS_KEY": "secret"}.items():
        monkeypatch.setenv(k, v)
    monkeypatch.delenv("S3_ENDPOINT", raising=False)
    monkeypatch.setenv("S3_REGION", "ap-southeast-1")
    fake = _FakeS3()
    monkeypatch.setattr(storage, "_client", fake)
    return fake


def test_public_url_matches_tryon_engine_construction(s3, monkeypatch):
    assert storage.public_url("shades/1/color.png") == "https://s3.ap-southeast-1.amazonaws.com/bkt/shades/1/color.png"
    monkeypatch.setenv("S3_ENDPOINT", "https://r2.example")
    assert storage.public_url("k") == "https://r2.example/bkt/k"


async def test_upload_bytes_passes_content_type_when_given(s3):
    await storage.upload_bytes("k", b"x", content_type="image/png")
    assert s3.calls == [{"Bucket": "bkt", "Key": "k", "Body": b"x", "ContentType": "image/png"}]


async def test_upload_bytes_without_content_type_is_unchanged_for_vision(s3):
    await storage.upload_bytes("k", b"x")
    assert s3.calls == [{"Bucket": "bkt", "Key": "k", "Body": b"x"}]


async def test_tryon_upload_returns_public_url(s3):
    url = await tryon_storage.upload_bytes("shades/9/alpha.png", b"png")
    assert url == "https://s3.ap-southeast-1.amazonaws.com/bkt/shades/9/alpha.png"
    assert s3.calls[0]["ContentType"] == "image/png"


async def test_tryon_upload_refuses_local_fallback(monkeypatch):
    for k in ("S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"):
        monkeypatch.delenv(k, raising=False)
    with pytest.raises(RuntimeError, match="S3 is not configured"):
        await tryon_storage.upload_bytes("k", b"x")
```

Run: `cd apps/ai-worker && python -m pytest tests/core/test_storage.py -q; cd ../..`

Expected: fails. `public_url` doesn't exist yet, `upload_bytes` doesn't accept `content_type`, and the tryon wrapper still has its own client.

- [ ] **Step 3: Implement**

In `ai_worker/core/storage.py`, change `upload_bytes` and add `public_url`:

```python
async def upload_bytes(key: str, data: bytes, content_type: str | None = None) -> None:
    if not settings.s3_configured:
        path = _local_path(key)
        path.parent.mkdir(parents=True, exist_ok=True)
        await asyncio.to_thread(path.write_bytes, data)
        return

    client = _get_client()
    extra = {"ContentType": content_type} if content_type else {}
    await asyncio.to_thread(
        client.put_object, Bucket=settings.s3_bucket, Key=key, Body=data, **extra
    )


def public_url(key: str) -> str:
    """The object's plain (unsigned) URL -- what tryon hands the web app for
    shade texture maps. Construction unchanged from tryon-engine."""
    endpoint = settings.s3_endpoint or f"https://s3.{settings.s3_region}.amazonaws.com"
    return f"{endpoint}/{settings.s3_bucket}/{key}"
```

Replace the whole of `ai_worker/tryon/services/storage.py` with:

```python
from ai_worker.core import storage
from ai_worker.core.settings import settings


async def upload_bytes(key: str, data: bytes, content_type: str = "image/png") -> str:
    # Tryon hands these URLs to the web app, so the shared storage's
    # local-disk fallback must never apply here: a local write would return a
    # URL that does not exist. Require S3 up front.
    settings.require_s3()
    await storage.upload_bytes(key, data, content_type=content_type)
    return storage.public_url(key)
```

- [ ] **Step 4: Verify**

```bash
cd apps/ai-worker && python -m pytest tests/core -q 2>&1 | tail -1; cd ../..   # expect: 15 passed
npm run test:vision 2>&1 | tail -1                                           # expect: 140 passed, 15 skipped, 12 deselected
grep -rn "boto3" apps/ai-worker/ai_worker --include='*.py'                  # expect: only ai_worker/core/storage.py
```

Run `npm run test:tryon`. Expect `6 passed`.

- [ ] **Step 5: Commit**

```bash
git add -A apps/ai-worker
git commit -m "refactor(ai-worker): one S3 client; tryon keeps its URL and requires S3" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Profiles, lifespan, logging, health

**Files:**
- Create: `ai_worker/core/logging.py`, `ai_worker/core/health.py`
- Create: `ai_worker/vision/profile.py`, `ai_worker/vision/routers/segment.py`, `ai_worker/vision/routers/healthz.py`
- Create: `ai_worker/tryon/profile.py`
- Modify: `ai_worker/vision/db.py` (add `ping`)
- Modify: `ai_worker/tryon/routers/extraction.py` (log job failures)
- Replace: `ai_worker/main.py`
- Delete: `ai_worker/vision/main.py`, `ai_worker/tryon/main.py`
- Modify: tests that imported `ai_worker.vision.main` / `ai_worker.tryon.main`
- Modify: `package.json` (`dev:tryon`)
- Test: `apps/ai-worker/tests/core/test_app.py`

**Interfaces:**
- Consumes: `settings.profiles`, `settings.log_level`, `settings.health_db_timeout_seconds`, `SERVICE_NAME`, `KNOWN_PROFILES`.
- Produces:
  - `ai_worker.main.create_app(profiles: Iterable[str] | None = None) -> FastAPI` and module-level `ai_worker.main.app`;
  - `PROFILE_EXTRAS: dict[str, str | None]`;
  - every profile module exposes `ROUTERS: list[APIRouter]`, `async startup() -> None`, `async shutdown() -> None`, and `async health_checks() -> dict[str, tuple[str, bool]]`, which maps a check name to `(state, required)`;
  - `ai_worker.vision.db.ping(timeout: float) -> bool`;
  - `ai_worker.core.health.health_router(profiles, check_fns) -> APIRouter`;
  - `ai_worker.core.logging.configure_logging(profiles, level) -> None`.

- [ ] **Step 1: Write the failing tests**

`apps/ai-worker/tests/core/test_app.py`:

```python
import json
import logging
import os
import subprocess
import sys
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from ai_worker import main as main_module
from ai_worker.main import create_app

APP_DIR = Path(__file__).resolve().parents[2]


def test_unknown_profile_is_a_startup_error():
    with pytest.raises(RuntimeError, match="unknown profile"):
        create_app(["vision", "ocr"])


def test_missing_extra_names_the_extra(monkeypatch):
    real = main_module.importlib.import_module

    def fake(name, *a, **k):
        if name == "ai_worker.tryon.profile":
            raise ModuleNotFoundError("No module named 'torch'")
        return real(name, *a, **k)

    monkeypatch.setattr(main_module.importlib, "import_module", fake)
    with pytest.raises(RuntimeError, match=r'apps/ai-worker\[tryon\]'):
        create_app(["tryon"])


def test_vision_profile_never_imports_tryon():
    code = "import sys; from ai_worker.main import create_app; create_app(['vision']); print(any(m.startswith('ai_worker.tryon') for m in sys.modules))"
    out = subprocess.run([sys.executable, "-c", code], cwd=APP_DIR, capture_output=True, text=True,
                         env={**os.environ, "AI_WORKER_PROFILES": "vision"}, check=True)
    assert out.stdout.strip().splitlines()[-1] == "False"


def test_health_database_disabled_without_database_url(monkeypatch):
    for k in ("DATABASE_URL", "DATABASE_PUBLIC_URL", "POSTGRES_URL"):
        monkeypatch.delenv(k, raising=False)
    res = TestClient(create_app(["vision"])).get("/health")
    assert res.status_code == 200
    assert res.json() == {"status": "ok", "service": "ai-worker", "profiles": ["vision"], "checks": {"database": "disabled"}}


def test_health_503_when_database_configured_and_down(monkeypatch):
    monkeypatch.setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:1/db")
    from ai_worker.vision import db

    async def ping(timeout):
        return False

    monkeypatch.setattr(db, "ping", ping)
    res = TestClient(create_app(["vision"])).get("/health")
    assert res.status_code == 503
    assert res.json()["status"] == "degraded"
    assert res.json()["checks"] == {"database": "down"}


def test_healthz_alias_keeps_vision_body():
    res = TestClient(create_app(["vision"])).get("/healthz")
    assert res.status_code == 200
    assert res.json()["service"] == "vision-ai-worker"


def test_contract_routes_mounted_for_vision():
    paths = {r.path for r in create_app(["vision"]).routes}
    assert {"/api/v1/segment-and-pose", "/api/v1/models/dispatch-zones", "/health", "/healthz"} <= paths
    assert "/extract" not in paths


def test_logs_are_json_lines_with_service_and_profile(capsys):
    create_app(["vision"])
    logging.getLogger("ai_worker.test").warning("hello %s", "world")
    line = json.loads(capsys.readouterr().out.strip().splitlines()[-1])
    assert line["service"] == "ai-worker" and line["profile"] == "vision"
    assert line["level"] == "WARNING" and line["msg"] == "hello world"
```

Run: `cd apps/ai-worker && python -m pytest tests/core/test_app.py -q; cd ../..`

Expected: FAIL. `create_app` doesn't exist yet; `ai_worker.main` is still the Task 1 shim.

- [ ] **Step 2: `core/logging.py` and `core/health.py`**

`ai_worker/core/logging.py`:

```python
import json
import logging
import sys
from typing import Iterable

from ai_worker.core.settings import SERVICE_NAME


class JsonFormatter(logging.Formatter):
    """One JSON object per line, tagged like the Go services' slog output."""

    def __init__(self, profiles: Iterable[str]):
        super().__init__()
        self._profile = ",".join(profiles)

    def format(self, record: logging.LogRecord) -> str:
        entry = {
            "time": self.formatTime(record, "%Y-%m-%dT%H:%M:%S%z"),
            "level": record.levelname,
            "service": SERVICE_NAME,
            "profile": self._profile,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        if record.exc_info:
            entry["exc"] = self.formatException(record.exc_info)
        return json.dumps(entry)


def configure_logging(profiles: Iterable[str], level: str) -> None:
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter(profiles))
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel(level)
```

`ai_worker/core/health.py`:

```python
from typing import Awaitable, Callable, Iterable

from fastapi import APIRouter
from fastapi.responses import JSONResponse

from ai_worker.core.settings import SERVICE_NAME

# name -> (state, required); state is "up" | "down" | "disabled" or an informational value.
HealthCheck = Callable[[], Awaitable[dict[str, tuple[str, bool]]]]


def health_router(profiles: Iterable[str], check_fns: Iterable[HealthCheck]) -> APIRouter:
    profiles, check_fns = list(profiles), list(check_fns)
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
        body = {
            "status": "ok" if status_code == 200 else "degraded",
            "service": SERVICE_NAME,
            "profiles": profiles,
            "checks": checks,
        }
        return JSONResponse(body, status_code=status_code)

    return router
```

- [ ] **Step 3: Vision profile**

Add to `ai_worker/vision/db.py`, after `close_pool`. Add `import asyncio` at the top if it is missing.

```python
async def ping(timeout: float) -> bool:
    """True when the registry pool exists, its schema applied, and Postgres answers
    within `timeout` seconds. Used by /health; never raises."""
    if _pool is None or _schema_error is not None:
        return False
    try:
        await asyncio.wait_for(_pool.fetchval("SELECT 1"), timeout)
        return True
    except Exception:  # noqa: BLE001 -- any failure means "down" for a health probe
        return False
```

`ai_worker/vision/routers/segment.py`:
- Move the detector singletons (`landmark_detector` … `hijab_detector`) and the whole `segment_and_pose` function from `ai_worker/vision/main.py` into a module that starts with:

```python
import time
from typing import Optional

import cv2
import numpy as np
from fastapi import APIRouter, File, Form, HTTPException, UploadFile

from ai_worker.vision.models.bisenet_v2 import BiSeNetV2Segmenter
from ai_worker.vision.models.head_pose import HeadPoseEstimator
from ai_worker.vision.models.hijab_detector import HijabDetector
from ai_worker.vision.models.landmark_mesh import LandmarkMeshDetector
from ai_worker.vision.models.zone_segmenter import ZoneSegmenter
from ai_worker.vision.schemas.contracts import SegmentAndPoseResponse

router = APIRouter()
```

- Change the decorator from `@app.post(...)` to `@router.post("/api/v1/segment-and-pose", response_model=SegmentAndPoseResponse)`.
- Keep the function body byte-identical.
- Also import `HeadPoseResult` if the body uses it. `main.py` imported it; keep the import only if `grep` finds a use.

`ai_worker/vision/routers/healthz.py`:

```python
from fastapi import APIRouter

router = APIRouter()


# Kept for existing callers of the pre-merge vision-ai-worker; body unchanged.
# /health (ai_worker.core.health) is the standardized endpoint.
@router.get("/healthz")
def healthz():
    return {
        "status": "ok",
        "service": "vision-ai-worker",
        "version": "0.2.0",
        "features": [
            "3d_landmarks",
            "head_pose",
            "8_zone_segmenter",
            "bisenet_v2_dense_masking",
            "hijab_detection",
        ],
    }
```

`ai_worker/vision/profile.py`:

```python
from ai_worker.core.settings import settings
from ai_worker.vision import db
from ai_worker.vision.routers.healthz import router as healthz_router
from ai_worker.vision.routers.models import router as models_router
from ai_worker.vision.routers.segment import router as segment_router

ROUTERS = [healthz_router, models_router, segment_router]


async def startup() -> None:
    await db.init_pool()


async def shutdown() -> None:
    await db.close_pool()


async def health_checks() -> dict[str, tuple[str, bool]]:
    # Without DATABASE_URL the registry is simply not configured (local dev):
    # reported, not a failure. With it, dispatch-zones cannot work unless the
    # database answers, so "down" fails /health -- like the Go services.
    if not settings.database_url:
        return {"database": ("disabled", False)}
    up = await db.ping(settings.health_db_timeout_seconds)
    return {"database": ("up" if up else "down", True)}
```

`db.ping` is looked up through the module attribute at call time. That is what lets the test monkeypatch it.

- [ ] **Step 4: Tryon profile and failure logging**

`ai_worker/tryon/profile.py`:

```python
import torch

from ai_worker.tryon.routers.extraction import router as extraction_router

ROUTERS = [extraction_router]


async def startup() -> None:
    return None


async def shutdown() -> None:
    return None


async def health_checks() -> dict[str, tuple[str, bool]]:
    # Informational only: tryon runs (slowly) on CPU too.
    return {"gpu": ("available" if torch.cuda.is_available() else "unavailable", False)}
```

In `ai_worker/tryon/routers/extraction.py`, add `import logging` and `logger = logging.getLogger(__name__)` after the imports. In the `except Exception as exc:` block of `run_extraction_job`, add this as its first line:

```python
        logger.exception("shade extraction failed for shade %s", req.shade_id)
```

The callback that follows is unchanged.

- [ ] **Step 5: `ai_worker/main.py`**

Replace the shim with:

```python
"""The ai-worker app. Profiles (AI_WORKER_PROFILES) choose which routers load:
`vision` (onnxruntime/mediapipe, CPU) and/or `tryon` (torch; install the
`tryon` extra). A profile's modules are imported only when it is enabled, so a
vision-only process never imports torch."""
import importlib
from contextlib import asynccontextmanager
from typing import Iterable, Optional

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from ai_worker.core.health import health_router
from ai_worker.core.logging import configure_logging
from ai_worker.core.settings import KNOWN_PROFILES, settings

# The pyproject extra each profile needs beyond the base install.
PROFILE_EXTRAS: dict[str, Optional[str]] = {"vision": None, "tryon": "tryon"}


def _load_profile(name: str):
    try:
        return importlib.import_module(f"ai_worker.{name}.profile")
    except ImportError as exc:
        extra = PROFILE_EXTRAS[name]
        target = f"apps/ai-worker[{extra}]" if extra else "apps/ai-worker"
        raise RuntimeError(
            f"profile {name!r} cannot load ({exc}); install its dependencies: pip install -e \"{target}\""
        ) from exc


def create_app(profiles: Optional[Iterable[str]] = None) -> FastAPI:
    profiles = tuple(profiles) if profiles is not None else settings.profiles
    unknown = [p for p in profiles if p not in KNOWN_PROFILES]
    if unknown or not profiles:
        raise RuntimeError(f"unknown profile(s) {unknown or list(profiles)}; choose from {list(KNOWN_PROFILES)}")

    configure_logging(profiles, settings.log_level)
    modules = [_load_profile(p) for p in profiles]

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        for module in modules:
            await module.startup()
        try:
            yield
        finally:
            for module in reversed(modules):
                await module.shutdown()

    app = FastAPI(title="Seagull AI Worker", version="0.3.0", lifespan=lifespan)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    app.include_router(health_router(profiles, [m.health_checks for m in modules]))
    for module in modules:
        for router in module.ROUTERS:
            app.include_router(router)
    return app


app = create_app()
```

Then:

```bash
git rm -q apps/ai-worker/ai_worker/vision/main.py apps/ai-worker/ai_worker/tryon/main.py
grep -rln "ai_worker.vision.main\|ai_worker.tryon.main" apps/ai-worker
```

In each file `grep` lists, a test module, replace the import:

| Old import | New code |
|---|---|
| `from ai_worker.vision.main import app` | `from ai_worker.main import create_app` then `app = create_app(["vision"])` |
| `from ai_worker.tryon.main import app` | `from ai_worker.main import create_app` then `app = create_app(["tryon"])` |

Some patch targets pointed at objects that moved out of `main.py`, for example `ai_worker.vision.main.landmark_detector`. Point those at `ai_worker.vision.routers.segment.<name>`.

`tests/tryon/test_health.py` asserted the old tryon body. Update it to the new contract:
- status 200;
- `body["service"] == "ai-worker"`;
- `body["profiles"] == ["tryon"]`;
- `body["checks"]["gpu"] in {"available", "unavailable"}`.

That body change is intended by the spec.

In `package.json`:

```json
    "dev:tryon": "python -m uvicorn ai_worker.main:app --app-dir apps/ai-worker --port 8090 --reload",
```

This is temporary. It runs the vision profile on 8090 until Task 6 adds the launcher, so note it in the commit body.

- [ ] **Step 6: Verify**

```bash
cd apps/ai-worker && python -m pytest tests/core -q 2>&1 | tail -1; cd ../..   # expect: 23 passed
npm run test:vision 2>&1 | tail -1                                           # expect: 148 passed, 15 skipped, 12 deselected
```

Run `npm run test:tryon`. Expect `6 passed`.

- [ ] **Step 7: Commit**

```bash
git add -A apps/ai-worker package.json
git commit -m "feat(ai-worker): profiles, lifespan, JSON logging and /health" -m "dev:tryon temporarily serves the default profile until the launcher lands." -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Image, launcher, compose, cleanup

**Files:**
- Create: `apps/ai-worker/Dockerfile`, `scripts/ai-worker.mjs`
- Modify: `package.json` (`dev:vision`, `dev:tryon`)
- Modify: `docker-compose.yml`
- Delete: `pytest.ini`, `apps/vision-ai-worker/requirements.txt`, `apps/tryon-engine/requirements.txt`, `apps/tryon-engine/pyproject.toml`, `apps/tryon-engine/Dockerfile`, and the then-empty `apps/vision-ai-worker/` and `apps/tryon-engine/` directories

**Interfaces:**
- Produces: `node scripts/ai-worker.mjs dev <vision|tryon>`.

- [ ] **Step 1: `scripts/ai-worker.mjs`**

```js
// Runs the ai-worker with one profile, for npm scripts. npm uses cmd.exe on
// Windows, where `VAR=value cmd` does not work, so the env is set here.
// Node built-ins only.
import { spawn } from "node:child_process";

// Profile -> port. Ports are global across the Seagull repositories (docs/PORTS.md).
const PROFILE_PORTS = { vision: "8088", tryon: "8090" };

const [command, profile] = process.argv.slice(2);
if (command !== "dev" || !PROFILE_PORTS[profile]) {
  console.error(`usage: node scripts/ai-worker.mjs dev <${Object.keys(PROFILE_PORTS).join("|")}>`);
  process.exit(2);
}

const port = PROFILE_PORTS[profile];
const child = spawn(
  "python",
  ["-m", "uvicorn", "ai_worker.main:app", "--app-dir", "apps/ai-worker", "--port", port, "--reload"],
  { stdio: "inherit", env: { ...process.env, AI_WORKER_PROFILES: profile, PORT: port } },
);
child.on("exit", (code) => process.exit(code ?? 1));
```

In `package.json`:

```json
    "dev:vision": "node scripts/ai-worker.mjs dev vision",
    "dev:tryon": "node scripts/ai-worker.mjs dev tryon",
```

- [ ] **Step 2: `apps/ai-worker/Dockerfile`**

```dockerfile
# Build context: repository root.
#   docker build -f apps/ai-worker/Dockerfile --target cpu .   -> vision profile, :8088
#   docker build -f apps/ai-worker/Dockerfile --target gpu .   -> tryon profile,  :8090

FROM python:3.12-slim AS cpu
# libgl1/libglib2.0-0: OpenCV and mediapipe runtime libraries.
RUN apt-get update && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY apps/ai-worker/pyproject.toml ./
COPY apps/ai-worker/ai_worker ./ai_worker
RUN pip install --no-cache-dir .
ENV AI_WORKER_PROFILES=vision PORT=8088
EXPOSE 8088
CMD uvicorn ai_worker.main:app --host 0.0.0.0 --port ${PORT}

FROM nvidia/cuda:12.1.0-runtime-ubuntu22.04 AS gpu
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends software-properties-common \
    && add-apt-repository -y ppa:deadsnakes/ppa \
    && apt-get update && apt-get install -y --no-install-recommends \
       python3.12 python3.12-venv libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*
RUN python3.12 -m venv /opt/venv
ENV PATH=/opt/venv/bin:$PATH
WORKDIR /app
COPY apps/ai-worker/pyproject.toml ./
COPY apps/ai-worker/ai_worker ./ai_worker
RUN pip install --no-cache-dir ".[tryon]"
ENV AI_WORKER_PROFILES=tryon PORT=8090
EXPOSE 8090
CMD uvicorn ai_worker.main:app --host 0.0.0.0 --port ${PORT}
```

- [ ] **Step 3: `docker-compose.yml`**
- Remove the `host.docker.internal` comment block, the `extra_hosts` block and the two env lines that part C added to `core-engine`.
- Replace them with:

```yaml
      - VISION_AI_WORKER_URL=${VISION_AI_WORKER_URL:-http://ai-worker-vision:8088}
      - TRYON_ENGINE_URL=${TRYON_ENGINE_URL:-http://ai-worker-tryon:8090}
```

Add these services before `networks:` at the top level:

```yaml
  ai-worker-vision:
    build:
      context: .
      dockerfile: apps/ai-worker/Dockerfile
      target: cpu
    ports:
      - "8088:8088"
    env_file:
      - .env
    environment:
      - DATABASE_URL=${DATABASE_URL}
    networks: [seagull]
    healthcheck:
      # The slim image has no wget/curl; /health answers 503 while the registry DB is down.
      test: ["CMD", "python", "-c", "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost:8088/health', timeout=4).status == 200 else 1)"]
      interval: 30s
      timeout: 5s
      retries: 3
    restart: unless-stopped

  # GPU only: `docker compose --profile gpu up`.
  ai-worker-tryon:
    profiles: ["gpu"]
    build:
      context: .
      dockerfile: apps/ai-worker/Dockerfile
      target: gpu
    ports:
      - "8090:8090"
    env_file:
      - .env
    environment:
      - MATCH_ENGINE_URL=${MATCH_ENGINE_URL:-http://core-engine:8082}
    networks: [seagull]
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    restart: unless-stopped
```

Verify: `python -c "import yaml; d=yaml.safe_load(open('docker-compose.yml')); print(sorted(d['services']))"`. Expect `['ai-worker-tryon', 'ai-worker-vision', 'core-engine', 'reference-service']`.

- [ ] **Step 4: Delete the leftovers**

```bash
git rm -q pytest.ini apps/vision-ai-worker/requirements.txt apps/tryon-engine/requirements.txt apps/tryon-engine/pyproject.toml apps/tryon-engine/Dockerfile
git ls-files apps/vision-ai-worker apps/tryon-engine        # expect: nothing
rm -rf apps/vision-ai-worker apps/tryon-engine              # only untracked caches remain there; check `ls` first
```

- [ ] **Step 5: Verify**

```bash
npm run check 2>&1 | grep -E "gofmt|passed|failed|error|ok  apps" ; echo "exit=${PIPESTATUS[0]}"
```

Expect exit 0, with the vision line reading `148 passed, 15 skipped, 12 deselected`.

Then check the launchers, if the runtime lane and ports are free (`netstat -ano | grep -E ":(8088|8090) "` prints nothing):
- Start `npm run dev:vision` in the background. `curl -s localhost:8088/health` should return `{"status":"ok","service":"ai-worker","profiles":["vision"],...}`, and `curl -s localhost:8088/healthz` the old vision body. Stop it.
- Do the same with `npm run dev:tryon` on 8090. Expect `"profiles":["tryon"]` and a `gpu` check. Stop it.
- If the ports are busy, report the launcher checks as not run.

- [ ] **Step 6: Commit**

```bash
git add -A apps/ai-worker scripts/ai-worker.mjs package.json docker-compose.yml pytest.ini apps/vision-ai-worker apps/tryon-engine
git commit -m "build(ai-worker): cpu/gpu Dockerfile, dev launcher, compose services; drop old workers" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Docs

**Files:**
- Modify: `README.md`, `RAILWAY.md`, `AGENTS.md`, `docs/MODEL-REGISTRY.md`, `docs/SHARED-DATABASE.md`, `docs/PORTS.md`, `docs/SPLIT.md`, `docs/LANES.md` (the "Lane map" table only), `docs/superpowers/specs/2026-09-26-ai-worker-merge-design.md` (status line)

- [ ] **Step 1: Replace path references**

```bash
grep -rn "vision-ai-worker\|tryon-engine" README.md RAILWAY.md AGENTS.md docs/MODEL-REGISTRY.md docs/SHARED-DATABASE.md docs/PORTS.md docs/LANES.md
```

For each hit:
- **A path** (`apps/vision-ai-worker/...`, `apps/tryon-engine/...`): rewrite it to the new location under `apps/ai-worker/`, as in the file map above. For example, `apps/vision-ai-worker/app/db.py` becomes `apps/ai-worker/ai_worker/vision/db.py`.
- **A service name in a table or prose:** use `ai-worker (vision profile)` or `ai-worker (tryon profile)`.
- **Ports:** keep them as they are.

`README.md`'s "What is in here" table replaces the two Python rows with:

```markdown
| `apps/ai-worker/` | 8088 / 8090 | Python/FastAPI AI worker. Profile `vision` (:8088; core-engine calls it via `VISION_AI_WORKER_URL`) and profile `tryon` (:8090; try-on and shade extraction, reports back via the extraction callback). One Dockerfile: `--target cpu` (vision) / `--target gpu` (tryon). |
```

`RAILWAY.md`'s "The Python workers are not deployed on Railway" section becomes:

```markdown
## The AI worker is not deployed on Railway (yet)

`apps/ai-worker` has no Railway service. When it gets one, the vision profile
fits Railway as-is: `apps/ai-worker/Dockerfile`, target `cpu`, which honours
Railway's `PORT`. The tryon profile needs torch and, for usable speed, a GPU,
which Railway does not offer: build target `gpu` for a GPU host.
```

`docs/LANES.md` lane map: replace the `vision-ai` and `tryon` rows with:

```markdown
| `ai-worker-core` | `apps/ai-worker/ai_worker/core/`, `apps/ai-worker/ai_worker/main.py`, `apps/ai-worker/tests/core/`, `apps/ai-worker/pyproject.toml`, `apps/ai-worker/Dockerfile` |
| `ai-worker-vision` | `apps/ai-worker/ai_worker/vision/`, `apps/ai-worker/tests/vision/` |
| `ai-worker-tryon` | `apps/ai-worker/ai_worker/tryon/`, `apps/ai-worker/tests/tryon/` |
```

`docs/SPLIT.md`: after the "Where everything went" table, add:

```markdown
Since 2026-09-26 `apps/vision-ai-worker/` and `apps/tryon-engine/` are one
package, `apps/ai-worker/` (profiles `vision` and `tryon`); see
`docs/superpowers/specs/2026-09-26-ai-worker-merge-design.md`.
```

Spec status line: change it to `Status: implemented on branch ai-worker-merge`.

- [ ] **Step 2: Verify**

```bash
grep -rn "apps/vision-ai-worker\|apps/tryon-engine" --include='*.md' --include='*.json' --include='*.yml' --include='*.mjs' . \
  | grep -v "docs/superpowers/" | grep -v "docs/SPLIT.md" | grep -v node_modules || echo "no stale paths"
```

Expect `no stale paths`. SPLIT.md and the old specs and plans are history, so they're excluded.

- [ ] **Step 3: Commit**

```bash
git add README.md RAILWAY.md AGENTS.md docs/MODEL-REGISTRY.md docs/SHARED-DATABASE.md docs/PORTS.md docs/SPLIT.md docs/LANES.md docs/superpowers/specs/2026-09-26-ai-worker-merge-design.md
git commit -m "docs: apps/ai-worker replaces the two Python workers" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

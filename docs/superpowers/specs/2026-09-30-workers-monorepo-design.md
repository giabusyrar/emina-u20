# Python workers as separate apps (seagull-core monorepo) — design

Date: 2026-09-30
Status: approved in chat, pending spec review

## Goal

Every Python worker gets its own folder, its own dependencies, its own tests
and its own Docker image, so each can later move to its own server without
another restructure. `seagull-core` becomes a monorepo of core units:

```
apps/
  core-engine/          Go      orchestration + form/score/match/vision/colour/face-arch API
  reference-service/    Go      reference data
  workers/
    common/             Python  shared library (not a service)
    skin/               Python  :8088  (was the ai-worker "vision" profile)
    face/               Python  :8094  (new; face-measure split out of vision)
    colour/             Python  :8092
    tryon/              Python  :8090  (GPU)
packages/goplatform/    Go      shared library for the Go services
```

The Go services stay where they are: they already follow the pattern (one
app, one image, a shared `packages/goplatform`), and moving them buys nothing.

## Decisions

| Question | Decision |
|---|---|
| Name of the remaining vision worker | `skin` — segment-and-pose, head pose, hijab detection, model registry and dispatch. "baseline" was rejected: it already means the comparison pipeline in `benchmarks/jev` and elsewhere, and says nothing about what the worker does. |
| Structure | Option B: one app per worker plus a shared library, not profiles inside one package. |
| Public API | Unchanged. `/core/vision-engine/...` (including face-architecture) stays on core-engine; gateway routes and Seagull-web are untouched. |
| Database | Unchanged. `vision_models` and the other registry tables keep their names; their owner is recorded as the skin worker. |
| Renaming env | Hard rename, no silent fallback to old names (see Section 2). |

## Section 1 — Layout and packaging

### `apps/workers/common` (package `worker_common`)

A library installed by path into every worker, never run on its own. It holds
only what every worker genuinely shares, moved from `apps/ai-worker/ai_worker/core/`:

- `settings.py`: base settings only — `DATABASE_URL`, `S3_*`, `LOG_LEVEL`,
  `PORT`, `HEALTH_DB_TIMEOUT_SECONDS`, `.env` discovery. Worker-specific
  settings move to the worker (`worker_<name>/settings.py`), built on the same
  lazy-read pattern.
- `storage.py` (S3, path-style), `logging.py` (JSON logs), `health.py`.
- `app.py`: `create_app(worker)` — the FastAPI factory from `ai_worker/main.py`,
  taking one worker module (`ROUTERS`, `startup`, `shutdown`,
  `health_checks`) instead of a profile list.
- `model_mirror.py`: the sync engine. Which mirror entries a worker needs is
  declared by the worker (see Section 2), not hardcoded per profile name.

### One app per worker

Each of `apps/workers/{skin,face,colour,tryon}` has:

- `pyproject.toml` with only that worker's runtime dependencies plus
  `worker-common @ file:../common`, and a `test` extra.
- `worker_<name>/` package: `main.py` (`app = create_app(worker)`),
  `settings.py`, routers, models, schemas.
- `tests/`.
- `Dockerfile` (build context: repository root) that copies only
  `apps/workers/common` and its own folder, installs them, and runs
  `python -m worker_common.model_mirror sync && exec uvicorn worker_<name>.main:app`.

Contents:

| Worker | Moves from `apps/ai-worker/` | Runtime dependencies (indicative) |
|---|---|---|
| skin | `ai_worker/vision/` minus face-measure: `routers/{segment,models,healthz}.py`, `models/` minus `face_landmarker.py`, `schemas/contracts.py`, `zones.py`, `db.py`; `tests/vision/` minus face tests; `benchmarks/` (the JEV ONNX baseline is the skin pipeline) | opencv, mediapipe (landmark mesh), numpy, onnxruntime, asyncpg, boto3 |
| face | `ai_worker/vision/facegeom/` (with `manifests/`), `models/face_landmarker.py`, `routers/face_measure.py`, `schemas/face_measure.py`; face tests (`test_facegeom_*`, `test_face_landmarker.py`, `test_face_measure_api.py`, `face_fixture.py`, fixtures) and `tests/core/test_settings_face_measure.py` | opencv, mediapipe, numpy |
| colour | `ai_worker/colour/`, `tests/colour/` | the current `colour` extra |
| tryon | `ai_worker/tryon/`, `tests/tryon/` | the current `tryon` extra |

Files are moved with `git mv` so per-file history survives. `apps/ai-worker/`
is deleted once empty.

### Boundaries

- A worker may import `worker_common` and itself, nothing else. In an image
  this holds by construction (other workers are not copied). In a dev venv
  where all are installed, a test in `apps/workers/common/tests/` scans every
  worker package and fails on any cross-worker import.
- `worker_common` imports no worker.

## Section 2 — Wiring, configuration, compose, deployment

### core-engine (`internal/config`)

- `SKIN_WORKER_URL` (default `http://127.0.0.1:8088`) replaces
  `VISION_AI_WORKER_URL` for the segmenter and capability dispatch.
- `FACE_WORKER_URL` (default `http://127.0.0.1:8094`) is new, used by
  `facearch/measureclient` in the server, `cmd/facearch-profile` and
  `cmd/face-benchmark`.
- No silent fallback: a `FACE_WORKER_URL` that quietly fell back to
  `VISION_AI_WORKER_URL` would point face at the skin worker, which no longer
  serves face-measure. `VISION_AI_WORKER_URL` is removed; if it is set,
  config loading returns an error naming `SKIN_WORKER_URL` and
  `FACE_WORKER_URL`, so a stale `.env` fails at start instead of misrouting.
- Unchanged: `COLOUR_WORKER_URL`, `TRYON_ENGINE_URL`, and tryon's
  `MATCH_ENGINE_URL`.

### Worker environment

Each worker's settings are prefixed by their owner; old names are removed in
the same change from every `.env` (laptop and VPS):

| Old | New | Owner |
|---|---|---|
| `VISION_MODEL_STORAGE_DIR` | `SKIN_MODEL_STORAGE_DIR` | skin |
| `VISION_AI_WORKER_URL` (skin's own public URL for local-file links) | `SKIN_WORKER_PUBLIC_URL` | skin |
| `VISION_FACE_LANDMARKER_PATH` | `FACE_LANDMARKER_PATH` | face |
| `FACE_LANDMARKER_PATH` (colour's override) | `COLOUR_LANDMARKER_PATH` | colour |
| `AI_WORKER_PROFILES` | removed (one worker per app) | — |

`FACE_MEASURE_*`, `WCPA_*`, `MODEL_*`, `MATCH_ENGINE_URL`,
`FACE_PARSING_MODEL_ID` keep their names and move with their worker. Like the
core-engine side, a worker whose settings module finds a removed name set
fails at start naming its replacement. `/healthz` (legacy body
`vision-ai-worker`) stays on skin for compatibility.

### Model mirror

The engine lives in `worker_common.model_mirror`; each worker passes what it
needs: face and colour the landmarker (face additionally pins the SHA-256
against its `facegeom/manifests/`), tryon the Hugging Face snapshots, skin
nothing (its models come from the registry). The bucket layout and
`MANIFEST.json` are unchanged.

### docker-compose.yml

Services `worker-skin`, `worker-face`, `worker-colour`, `worker-tryon`
(`profiles: ["gpu"]`), each built from its own Dockerfile. Service names are
their DNS names on the `seagull` network, so compose sets core-engine's
`SKIN_WORKER_URL=http://worker-skin:8088`,
`FACE_WORKER_URL=http://worker-face:8094`,
`COLOUR_WORKER_URL=http://worker-colour:8092`,
`TRYON_ENGINE_URL=http://worker-tryon:8090`, and tryon's
`MATCH_ENGINE_URL=http://core-engine:8082`.

### Tooling

- `scripts/ai-worker.mjs` becomes `scripts/worker.mjs`: `npm run dev:<skin|face|colour|tryon>`.
- `npm run test:<common|skin|face|colour|tryon>`; `npm run check` runs
  common, skin and face (CPU, fast). Colour and tryon stay opt-in as today.
- Pytest markers `tryon`/`colour` are no longer needed for selection (each
  suite is its own app); `benchmark` and `jev` stay in skin.

### Deployment (VPS, after merge)

1. Build and start `worker-skin` and `worker-face`.
2. Set `SKIN_WORKER_URL`/`FACE_WORKER_URL` (remove `VISION_AI_WORKER_URL`) in
   the VPS `.env`, restart core-engine.
3. Remove the old `ai-worker-vision` container.
4. `ai-worker-colour` (deployed by the JEV session) is replaced by
   `worker-colour` only in coordination with that session.

### Documentation

`docs/PORTS.md` (8094 face; 8088/8090/8092 renamed), `docs/MODEL-MIRROR.md`,
`docs/MODEL-REGISTRY.md`, `docs/FACE-MEASURE.md`, `docs/COLOUR-LOCAL-RUN.md`,
`docs/SHARED-DATABASE.md` (registry tables owned by skin), `README.md`,
`AGENTS.md` (worker list).

## Section 3 — Order of work and safeguards

On branch `refactor/workers-split` in a worktree. Each step leaves the suite
green:

1. **common** — create `apps/workers/common`, move `core/` and its tests.
2. **face** — move face-measure and its tests; Dockerfile; port 8094;
   `FACE_WORKER_URL` in core-engine.
3. **skin** — move the rest of `vision`; `SKIN_WORKER_URL`.
4. **tryon** — move tryon.
5. **colour** — move colour only after the JEV session (which holds the
   colour lane) agrees; until then colour stays in `apps/ai-worker/`, and no
   other step depends on it.
6. **cleanup** — delete `apps/ai-worker/`; compose, npm scripts, docs.

Safeguards:

- Tests move with their code; none are rewritten to pass. The passing count
  before and after must match (325 default + 8 tryon at the start), recorded
  per step; a changed count is explained, not absorbed.
- The cross-worker import test (Section 1, Boundaries).
- Every image is actually built; inside each, importing another worker fails,
  and the face image contains no onnxruntime, asyncpg or torch.
- Local end-to-end: core-engine reaches `worker-face` for face-architecture
  and `worker-skin` for segment-and-pose.
- No database schema change.

## Out of scope

- Moving the Go services under `apps/services/`.
- Renaming `/core/vision-engine`, `internal/vision` or the `vision_models` tables.
- Splitting the repository.
- Tryon on the VPS (no GPU there).

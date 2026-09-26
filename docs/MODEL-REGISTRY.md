# The plug-and-play model registry

> ## Deploying this is one-way: do not roll the worker back
>
> This release drops the `vision_models_capability_code_key` unique
> constraint (`app/db.py`, last statement in `SCHEMA_STATEMENTS`), because the
> library needs more than one row per capability. The **previous** release's
> `upsert_model` inserted with `ON CONFLICT (capability_code)`, which needs
> exactly that constraint. Once this release has run its migration:
>
> - Rolling the worker back to the previous release makes **every upload
>   fail** with `there is no unique or exclusion constraint matching the ON
>   CONFLICT specification`. Reads, dispatch and scoring keep working; only
>   uploads break.
> - The breakage is immediate — it does not wait for a duplicate upload; the
>   old statement needs the constraint to exist at plan time.
> - Rolling **forward** again is the fix. Do not try to recreate the unique
>   constraint by hand: once two rows share a capability_code (the normal
>   state of a library after the first second upload) it cannot be recreated,
>   and recreating it on a library that happens not to have duplicates yet
>   would break the next upload instead.
>
> Treat a deploy of this worker as a forward-only migration and plan the
> rollback as roll-forward. Same note, from the schema side, in
> `docs/SHARED-DATABASE.md`.

Lives in `apps/ai-worker` (vision profile). Design background:
`docs/superpowers/specs/2026-09-26-plug-and-play-model-registry-design.md`
(read that for the *why*; this document is the *what's actually running*, and
corrects the design doc's own claims where implementation proved them wrong —
those corrections are called out inline there too).

## What the registry is

A **library** of uploaded models plus a **binding** per capability deciding
which model currently serves it.

Before this work, `vision_models.capability_code` was unique: one row per
capability, and a new upload destroyed the old row. Now:

- A model row **declares the capability codes it produces**
  (`vision_models.produces`, a `TEXT[]`), so one multi-class model can serve
  several capabilities at once.
- Uploading **adds a row to the library**. Nothing is ever overwritten or
  destroyed by an upload; every model ever uploaded stays available and
  downloadable.
- Each capability **points at the model currently serving it**
  (`vision_capability_bindings`, one row per capability code). Switching which
  model serves a capability is a pointer move — instant, and free to reverse,
  because sessions are cached per model id and an old model stays warm.

Several models may declare the same capability; the normal state of a
library is more than one candidate per capability, and which one is live is
decided separately, by activation (see below), never implicitly by upload
order.

Nothing is deleted automatically. Deleting a model is a manual, explicit
action (`DELETE /api/v1/models/models/{modelId}`) and is refused while any
capability is still bound to it.

## The runtime spec

Stored per model in `vision_models.runtime_spec` (JSONB), parsed by
`app/models/spec.py`. It declares how to feed the model and how to read what
comes back. Field names follow DeepStream's `nvinfer` vocabulary
(`netScaleFactor`, `offsets`, `colorFormat`) so they mean what a vision
engineer already expects.

```jsonc
{
  "specVersion": 1,
  "input": {
    "width": 384,
    "height": 384,
    "colorFormat": "RGB",        // RGB | BGR | GRAY
    "layout": "NCHW",            // NCHW | NHWC
    "netScaleFactor": 0.017125,  // y = netScaleFactor * (x - offsets)
    "offsets": [123.675, 116.28, 103.53],
    "embedded": false            // true: model carries its own preprocessing
  },
  "output": {
    "kind": "severity_scalar"
    // fields below depend on kind
  }
}
```

`input.width` and `input.height` are **optional when `input.embedded` is
`true`, and required otherwise.** `embedded: true` means the model was built
with its own preprocessing baked into the graph (for example via
`onnxruntime-extensions`); the worker then passes the encoded image bytes
through untouched, and declaring any other `input` field alongside
`embedded: true` is rejected as a contradiction rather than silently ignored.

### Output kinds and their fields

Each kind is an adapter in `app/models/adapters.py`. An adapter turns raw
output tensors into `{capability_code: severity 0-100}` — the same currency
`dispatch-zones` has always returned, so nothing downstream of the worker
changes. `SEVERITY_SCALE = 100.0` is defined once, in `app/models/spec.py`,
as the contract with core-engine — not a tunable.

| Kind | Fields | Produces |
|---|---|---|
| `severity_scalar` | `capability` | One capability; today's behaviour. The raw output is read as a **0..1 fraction**: it is clamped to 0-1 and then multiplied by `SEVERITY_SCALE` (100). A model whose raw scalar exceeds 1.0 therefore saturates at exactly 100.0 on every zone — the upload warns when the smoke run sees that, because the post-clamp value it range-checks looks perfectly legal. |
| `multiclass_softmax` | `classes[]` of `{index, capability}`, optional `temperature` | One capability per mapped class, from its softmax probability. Unmapped indices ignored. |
| `multilabel_sigmoid` | `classes[]` of `{index, capability}` | One capability per mapped class, independently (sigmoid, not softmax). |
| `detection_count` | `capability`, `scoreThreshold`, `countToSeverity`-style `fullScaleCount`, optional `tensorOrder` | Counts detections scoring above `scoreThreshold`; severity scales linearly to `fullScaleCount`, clamped. |
| `mask_area_ratio` | `classes[]` of `{index, capability}`, optional `minPixelRatio`, optional `tensorOrder` | Fraction of mask pixels matching each class, as severity; below `minPixelRatio` reports 0. |

`detection_count` and `mask_area_ratio` need to know which output tensor
carries what, since ONNX does not standardize output ordering — both declare
`tensorOrder` naming the index of `scores`, `boxes`, `classes`, or `mask`.

Every threshold, class index, label and scale in that table is **per model,
stored in the row** — none are code constants. `GET /api/v1/models/kinds`
returns this same catalog (name, required/optional, defaults) so the upload
form can be rendered without a web release when a new field is added.

### Batching — restricted by output kind, and a small win, not the headroom argument

Only `severity_scalar`, `multiclass_softmax` and `multilabel_sigmoid` batch —
each produces one row of independent per-image scores, so slicing a batched
output's axis 0 back apart 1:1 with crop order is safe. `detection_count` and
`mask_area_ratio` **always** run one crop at a time, never batched, because
batched slicing is not safe for them: a common NMS export for detection runs
across the whole batch and emits one `[total_detections, 6]` tensor with a
batch-index column (row *i* is not image *i*), and a segmentation head can
similarly apply batch-level post-processing. Enforced in
`app/models/model_registry.py` (`_BATCHABLE_OUTPUT_KINDS`).

Measured (`.superpowers/sdd/2026-09-26-plug-and-play-model-registry/task-11-report.md`),
median latency for 8 crops:

| Model | Threading | Per-crop (8 calls) | Batched (1 call) | Batching |
|---|---|---|---|---|
| toy mean-reduction graph | default (16 cores) | 20.07 ms | 26.75 ms | **loses, ~33%** |
| resnet18 @224 | default (16 cores) | 129.15 ms | 121.67 ms | wins, ~5.8% |
| resnet18 @224 | 1 intra-op thread | 266.52 ms | 260.34 ms | wins, ~2.3% |

Batching is real but modest on a realistic backbone, and the margin **shrinks
as cores shrink** — the opposite of what a small, core-constrained container
needs. It is kept because the win is free where it applies, but it is not
what keeps dispatch inside its time budget; the model-grouping in dispatch
(one inference per model, not per capability) does that.

## Uploading

`POST /api/v1/models/upload` — one multipart request: the `.onnx` file plus
form fields.

| Field | Required | Meaning |
|---|---|---|
| `file` | yes | Must be a `.onnx` file. |
| `capability` | yes | The model's primary capability code (kept for backward compatibility even once `produces` covers more). |
| `applicableZones` | no | Comma-separated zone codes; omitted means every zone. |
| `runtimeSpec` | no | JSON string, the spec above. Omitted means the legacy default (see below). |
| `label` | no | Human name for the row ("resnet50 384, trained 2026-09"), display only — nothing resolves by it. |
| `activate` | no | Comma-separated capability codes to bind to this model on success. Empty/omitted means upload-only: the model joins the library but serves nothing until explicitly activated. |

Upload never displaces an existing model — there is no conflict check, because
several models declaring the same capability is the normal state of a
library.

### Validation, and what each failure code means

Validation runs before anything is stored, in `app/models/validation.py`
(`validate_upload`), and rejects rather than adapts:

- **400 — malformed request.** Missing `capability`, a non-`.onnx` filename,
  an empty file, unparseable `runtimeSpec` JSON, an unknown output `kind`, an
  unknown field, or a field the declared kind does not take. A typo like
  `scoreThreshhold` is rejected by name, not silently defaulted.
- **422 — model and spec disagree.** The declared input shape doesn't match
  the model's real input (a declared 384 against a model fixed at 224); the
  declared output rank/shape doesn't match what the kind requires (a
  `multiclass_softmax` declaring five classes against a three-output model);
  the smoke run's adapter raises or returns a value outside 0-100; or
  `activate` names a capability this model's spec does not `produce`.
- **503 — storage or database down.** The model file couldn't be written to
  object storage, or the library row couldn't be inserted. If the row insert
  fails after the file was already stored, the worker tries to delete the
  orphaned object and says so in the error either way (cleanup succeeded or
  also failed) — it never leaves a silent dangling object unmentioned.

Steps 2-4 of validation (signature check, output-rank check, smoke run) mean
a model that uploads successfully has demonstrably run once, under its own
declared spec, and produced a usable severity.

### An upload can succeed while activation partially fails

The row insert and the requested activations are separate steps. **A
partially-failed activation still returns HTTP 200** — the upload itself
succeeded (the model is in the library) even though not everything requested
went live:

```jsonc
{
  "id": "...",
  // ...the rest of the model record...
  "activeFor": ["acne"],                         // only what actually bound
  "warnings": ["failed to activate 'redness': ..."]  // names what did not
}
```

The response's `activeFor` reflects what actually bound, not an echo of the
request, and `warnings` names each capability that failed to activate and
why. A caller that only checks the HTTP status code will miss this — check
`warnings`.

## Activating, rolling back, and unbinding

- `GET /api/v1/models/capabilities` — every capability code, its currently
  bound model id (or null), and the candidate model ids that declare it.
  This is what an activation screen renders.
- `PUT /api/v1/models/capabilities/{code}/binding` — body `{"modelId": "..."}`.
  Binds (or rebinds) that capability to the named model. 422 if the model's
  spec does not `produce` that capability — the one way activation can be
  wrong. **Rolling back is this same call with an older model's id** — there
  is no separate rollback endpoint or operation; a rollback and a forward
  switch are the same write.
- `DELETE /api/v1/models/capabilities/{code}/binding` — unbinds. The
  capability becomes unsupported (see below). This is a legitimate,
  supported operation, not an error state.
- `GET /api/v1/models/capabilities/{code}/history` — the append-only
  activation log for one capability (`vision_capability_activation_log`):
  every bind and unbind, in order, with `model_id` (null for an
  unbind/deactivation) and `activated_at`. Answers "why did scores shift on
  the 12th." Never read by the scoring path itself.

Every bind or unbind is written to the activation log in the same
transaction as the binding change (`app/db.py`, `set_binding` /
`clear_binding`), so the two never drift apart.

## Editing a model row, and what a spec change implies

`PATCH /api/v1/models/models/{modelId}` — body may carry `runtimeSpec`,
`applicableZones` and `label`.

- Sending `runtimeSpec` re-runs the full upload validation against the
  **stored** model file, and **recomputes `produces` from the new spec**.
  What a model emits is derived from its spec, so the two cannot be allowed
  to disagree: a stale `produces` would refuse a capability the model now
  emits (a false 422 at bind time) and accept one it no longer emits (bound,
  and then silently scoring nothing, forever).
- **409** if the new spec drops a capability this model is *currently bound
  for*. The response names the capability. Bind it to another model, or
  unbind it, and retry — the worker will not quietly leave a live binding
  pointing at a model that cannot serve it.
- `applicableZones` distinguishes "omitted" (leave the stored scoping alone)
  from "explicitly sent" — an explicit `null`, an empty list, or a list of
  nothing but blanks all mean *clear the scoping*, i.e. applies to every
  zone.
- The response carries the same `warnings` list an upload returns, for the
  same validation run. Re-speccing a model into a non-fatal but consequential
  state — a `severity_scalar` export whose raw output exceeds 1.0, so every
  score it ever produces pins to the top of the scale — used to be **silent**
  on PATCH while the identical upload warned. The list is empty when the
  request carried no `runtimeSpec`: nothing was revalidated, so there is
  nothing to report.

`GET /api/v1/models/download?capability=...` resolves through the **bindings
table**: it hands back the model actually serving that capability. Several
library rows may declare the same `capability_code`, so resolving by that
column alone returned an arbitrary one of them, possibly a superseded
candidate. A capability with no binding is a 404 — there is no model serving
it to download.

## An unbound capability is absent, never zero

A capability with no binding row is **unsupported**: it is absent from
`dispatch-zones`/`dispatch-capabilities` results and reported in
`unsupportedCapabilities`, exactly as an unregistered capability is today.
It is never scored as zero, and no model runs for it. core-engine already
turns `unsupportedCapabilities` into a warning rather than inventing a score,
and this registry does not change that contract.

The same absent-not-zero rule applies if a bound model fails at run time (the
adapter raises, or the model file is missing from storage): that model's
capabilities are absent from the zone's telemetry for that request and a
warning names the model, but other models still run, and the row itself is
not deleted — a transient storage failure is not a deregistration.

## The four limits

Env config, read through `app/config.py`, never literals at call sites:

| Env var | Default | Why |
|---|---|---|
| `MODEL_MAX_FILE_BYTES` | 200 MB | The largest model measured (a 158 MB segmentation export) plus headroom. Enforced at upload; exceeding it is a validation failure. |
| `MODEL_MAX_SMOKE_LATENCY_MS` | 400 ms | Guideline for a single-image run, measured during the upload smoke run. Exceeding it does **not** fail the upload — it succeeds with a warning naming the projected 8-zone cost, because a slow model may be a deliberate choice the deployment should be free to make. |
| `MODEL_CACHE_MAX_BYTES` | 1 GB | Bound on loaded ONNX sessions, keyed by model id (not capability, so switching back to a previous model stays free) and evicted least-recently-used. The just-inserted entry is never evicted to make room for itself — a single model bigger than the whole budget stays resident rather than being silently re-downloaded and rebuilt on every call. |
| `MODEL_LIBRARY_WARN_BYTES` | 10 GB | Not enforced. `GET /api/v1/models` reports total library bytes and flags when the library has passed this, so storage growth from an ever-growing, non-evicting library is visible before it becomes a bill. |

The measured latencies these defaults are set against (median, ONNX Runtime
CPU, single image):

| Model | 16 cores | 2 threads | 1 thread | Session load | File size |
|---|---|---|---|---|---|
| mobilenet_v3_small @224 | 1.8 ms | 1.8 ms | 2.4 ms | 38 ms | 10 MB |
| resnet18 @224 | 9.2 ms | 14 ms | 28 ms | 108 ms | 47 MB |
| resnet50 @384 | 61 ms | 91 ms | 172 ms | 175 ms | 102 MB |
| deeplabv3_resnet50 @224 | 135 ms | 189 ms | 341 ms | 269 ms | 158 MB |

Scaled to 8 zones x 5 capabilities sequential, one thread, a segmentation
model costs about 13.6 s against a 20 s dispatch timeout and a 30 s analysis
timeout — the sequential loop, not model size, is what is at risk of running
out of budget, which is why `MODEL_MAX_SMOKE_LATENCY_MS` warns rather than
silently letting a slow model through unremarked.

## Legacy rows

A row with `runtime_spec = NULL` is a pre-existing model, from before this
spec existed. It runs under `LEGACY_SPEC` (`app/models/spec.py`) — 224x224
RGB, NCHW, scale 1/255, no offsets, single `severity_scalar` output for its
own `capability_code` — expressed as an explicit default spec in code, not as
a separate branch. There is one dispatch code path for legacy and declared
rows alike (`_spec_for` in `app/routers/models.py`).

## Schema migration: concurrent startup, and reporting a failure

`app/db.py`'s `init_pool` applies the whole statement list (`SCHEMA_STATEMENTS`
— the registry's tables, the bindings backfill, the constraint drop) at every
process start, via `apply_schema`.

**Concurrent replicas.** `CREATE TABLE IF NOT EXISTS` and `ADD COLUMN IF NOT
EXISTS` are *not* race-free in Postgres: two backends can both pass the
existence check, and the loser raises `duplicate_table` / `duplicate_column`.
Two replicas booting together is the ordinary case on any rolling deploy. So
`apply_schema` holds `pg_advisory_lock(SCHEMA_ADVISORY_LOCK_ID)` across the
*whole* list — not per statement, because the backfill and the constraint
drop must not interleave with another replica's copy of the same list. The
lock id is a fixed constant in `app/db.py`; it must stay distinct from any
advisory-lock id Seagull-gateway takes on this shared database (a collision
would only serialize unrelated migrations, never corrupt anything).

**The wait for that lock is bounded.** `pg_advisory_lock` blocks forever, and
a session-level lock is held by the backend of a replica that was OOM-killed
mid-migration until TCP keepalive reaps that backend — hours, by default.
`init_pool` is awaited in the startup hook, so blocking there meant the next
replica never answered `/healthz` and the orchestrator restart-looped the
whole worker, taking segment-and-pose down with it — the exact outage
`init_pool` exists to prevent. So `apply_schema` polls
`pg_try_advisory_lock` to a deadline:
`MODEL_SCHEMA_LOCK_TIMEOUT_SECONDS`, **default 60s**
(`Settings.schema_lock_timeout_seconds` in `app/config.py` documents the
choice — many times the honest worst case for the statement list plus the one-time backfill,
and far below any deployment's own timeout). On expiry it raises
`db.SchemaLockTimeout` and **does not run the DDL unlocked** — the statements
are not race-free, and proceeding would trade a hang for corruption. The
timeout is recorded exactly like any other migration failure (below), so the
503 body names it: *another instance appears to hold the registry schema
migration lock*.

**When it still fails.** A migration can fail for reasons the lock does not
address — a permissions error, an unreachable database, a table locked by
something else. `init_pool` deliberately does not take the service down for
that: segment-and-pose keeps working. But the failure is recorded, not only
printed:

- The cause is logged through the module logger (`logging`, not `print`), and
- stored in `db._schema_error`, which `_require_pool` puts **into the 503
  body**: `Model registry database is unavailable: startup schema
  initialization failed with <exception type>: <message>`.

So an operator seeing a 503 from any model endpoint reads the actual cause
off the response. The bare `Model registry database pool is not initialized`
now means only "no pool and no recorded schema failure" — i.e. the database
was never configured or connected at all.

## Running the worker's tests

The AI worker's test dependencies — `pytest`, `pytest-asyncio`,
`pytest-benchmark`, `onnx`, `torch`, `torchvision` — are declared **only** in
the `[test]` extra of `apps/ai-worker/pyproject.toml` — not in
`[project].dependencies`, and not in the `cpu`/`gpu` images built from
`apps/ai-worker/Dockerfile`: the worker infers with `onnxruntime` and never
imports any of them, and `torch` alone is several hundred megabytes. Install
them explicitly:

```bash
pip install -e "apps/ai-worker[test]"
pytest apps/ai-worker      # or: npm run test:vision
```

A bare `pytest apps/ai-worker` from the repository root collects all tests
(vision, tryon, and core) and runs with the default markers. The dispatch
benchmarks are wall-clock measurements (they export a real resnet18, and take
minutes) and say nothing about correctness, so they are deselected from every
ordinary run by the `addopts` in `apps/ai-worker/pyproject.toml`:

```
addopts = '-m "not benchmark and not tryon"'
```

This marker filter removes benchmarks and tryon tests by default. Run the suites
as needed:

- `pytest apps/ai-worker` — collects vision, tryon, and core together (runs with
  default markers: vision + core, deselects tryon)
- `npm run test:vision` — vision and core tests only (same selection as above)
- `npm run test:tryon` — tryon tests (requires torch; about 8 minutes, downloads
  models)
- `npm run bench:vision` — dispatch benchmarks only

The `tests/vision/test_db_schema.py` suite needs a real Postgres and skips
without one. Point `TEST_DATABASE_URL` at a **scratch** cluster — never a
shared or hosted database; it truncates the registry tables between tests.

`/health` returns 503 while the registry database is down, or while startup's
schema-lock wait timed out, and stays that way with no reconnect attempt until
the process restarts. A platform health check that polls `/health` will
therefore fail such a deploy — that is intended: `dispatch-zones` cannot serve
correctly with the registry database unreachable. `/healthz` is unaffected and
stays 200.

## What Seagull-web must change

This work is raised and implemented **in the Seagull-web repository, not from
here** — `Seagull-web` is a separate repo (the Next.js dashboard) and this
repo does not touch it. What follows is the handoff.

### The two replaced 410 paths

`PATCH /api/v1/models/{capability}` and `DELETE /api/v1/models/{capability}`
now return `410 Gone` unconditionally — a capability no longer identifies one
model, so these two shapes cannot mean what they used to. A stale web build
calling them fails legibly (410, with the replacement path named in the
response body) instead of silently doing the wrong thing. Replacements:

- `PATCH /api/v1/models/{capability}` → `PATCH /api/v1/models/models/{modelId}`
  (edits `runtimeSpec`, `applicableZones`, or `label` on a specific library
  row, by model id, not by capability).
- `DELETE /api/v1/models/{capability}` → either
  `DELETE /api/v1/models/models/{modelId}` (removes the library row and its
  stored file entirely; refused with 409 while any capability is bound to it)
  or `DELETE /api/v1/models/capabilities/{code}/binding` (unbinds the
  capability without touching the library row).

### New upload form fields

The existing upload form (file + `capability` + `applicableZones`) keeps
working unchanged. Seagull-web should add three optional fields to that form:

- `runtimeSpec` — a JSON string built from `GET /api/v1/models/kinds`, so a
  new output kind needs no web release to become selectable.
- `label` — free text, for distinguishing library rows.
- `activate` — comma-separated capability codes. **Important behavior
  change:** if left empty, the uploaded model joins the library but serves
  nothing — this is a break from today's implicit behavior where an upload
  went live immediately. The web UI should make this explicit (e.g. an
  "activate immediately for `<capability>`" checkbox that populates
  `activate`), not leave uploaders to discover it by the model silently not
  running.

### The changed `GET /api/v1/models` response shape

This endpoint no longer returns a bare list. It now returns an object:

```jsonc
{
  "models": [ /* VisionModelRecord[], each now carrying produces, label, activeFor */ ],
  "libraryBytes": 123456789,
  "libraryWarn": false
}
```

Any Seagull-web code that treated the response as an array directly (e.g.
`response.map(...)`) must be updated to read `response.models`.

### The three screens the endpoints support

1. **Library** — `GET /api/v1/models` (browse every uploaded model, its spec,
   size, and which capabilities it's active for; `libraryBytes`/`libraryWarn`
   for storage-growth visibility) plus `POST /api/v1/models/upload` and
   `DELETE /api/v1/models/models/{modelId}` to add to or prune the library.
2. **Per-capability activation** — `GET /api/v1/models/capabilities` (every
   capability, its current binding, its candidates) plus
   `PUT`/`DELETE /api/v1/models/capabilities/{code}/binding` to switch, roll
   back (bind to an older model id), or unbind.
3. **History** — `GET /api/v1/models/capabilities/{code}/history`, rendering
   the append-only activation log so "why did scores shift" is answerable per
   capability.

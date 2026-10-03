# The plug-and-play model registry (model server)

> ## Deploying this is one-way: do not roll the model server back
>
> This release drops the `vision_models_capability_code_key` unique
> constraint (`worker_models/db.py`, last statement in `SCHEMA_STATEMENTS`), because the
> library needs more than one row per capability. The **previous** release's
> `upsert_model` inserted with `ON CONFLICT (capability_code)`, which needs
> exactly that constraint. Once this release has run its migration:
>
> - Rolling the model server back to the previous release makes **every upload
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
> Treat a deploy of this server as a forward-only migration and plan the
> rollback as roll-forward. Same note, from the schema side, in
> `docs/SHARED-DATABASE.md`.

Lives in `apps/workers/models` (the model server, `worker-models`, port 8096; it was part of the skin worker until the model-server split). Design background for the server itself: `docs/superpowers/specs/2026-10-01-model-server-design.md`. Registry design:
`docs/superpowers/specs/2026-09-26-plug-and-play-model-registry-design.md`
(read that for the *why*; this document is the *what's actually running*, and
corrects the design doc's own claims where implementation proved them wrong —
those corrections are called out inline there too).

## The model server: where it runs and how models load

The registry and ONNX inference run in their own service, `worker-models`
(`apps/workers/models`, package `worker_models`), on **port 8096**. The
`/api/v1/models/*` routes and their request and response bodies are unchanged
from when they lived in the skin worker. Callers differ:

- **core-engine** reaches it at `MODEL_SERVER_URL` (default
  `http://127.0.0.1:8096`; compose sets `http://worker-models:8096`) for
  capability dispatch only. The skin worker keeps segment-and-pose alone, on
  `SKIN_WORKER_URL`, and its `/health` has no checks any more.
- **Seagull-web** (model-registry pages, orchestrator) must target the model
  server, not the skin worker. That change is reported in
  `../Seagull-web/docs/REPORT-SEAGULL-CORE-WORKERS-SPLIT.md`, section 5.

Environment names changed, with no fallback: `SKIN_MODEL_STORAGE_DIR` is now
`MODEL_STORAGE_DIR` (local storage root when S3 is not configured) and
`SKIN_WORKER_PUBLIC_URL` is now `MODEL_SERVER_PUBLIC_URL` (base URL of local
download links). Both the model server and the skin worker refuse to start
while the old names are set (`REMOVED_ENV` in `worker_models/worker.py` and in
`worker_skin/worker.py`). `MODEL_MAX_FILE_BYTES`, `MODEL_MAX_SMOKE_LATENCY_MS`,
`MODEL_CACHE_MAX_BYTES`, `MODEL_LIBRARY_WARN_BYTES` and
`MODEL_SCHEMA_LOCK_TIMEOUT_SECONDS` keep their names.

### Storage and the disk cache

S3 is the source of truth for model files. The server keeps a cache of them
on disk, `MODEL_CACHE_DIR` (image: `/app/data/model-cache`, the compose named
volume `model-cache`), one file per model named `<sha256>.onnx`
(`worker_models/cache.py`). A cached file is used only after its hash matches
the model's recorded SHA-256, so a restart or redeploy downloads nothing.
`MODEL_DISK_CACHE_MAX_BYTES` (default 20 GB, policy) bounds the cache; the
least recently used files are evicted first, and a file backing a bound model
is never evicted. Orphaned `*.part` files from an interrupted write are
deleted once they are older than an hour.

The hash lives in a new `vision_models.sha256` column (additive, see
`docs/SHARED-DATABASE.md`). New uploads record it; a legacy row without one
gets it the first time it is loaded (at preload, at bind, or at dispatch).

### Preload, load-on-bind, and failure

- **Preload.** Every bound model is loaded at start (`worker_models/preload.py`),
  before `/health` is ready, so — as long as the bound models fit
  `MODEL_CACHE_MAX_BYTES` — no request waits on a download or a session
  build. The in-memory session cache is bounded by `MODEL_CACHE_MAX_BYTES`;
  when the bound models' total size exceeds it, preload logs a warning naming
  both numbers, and the evicted ones are reloaded on request.
- **Preload time bound.** Each startup load is bounded by
  `MODEL_PRELOAD_TIMEOUT_SECONDS` (default 60, policy) so a slow or
  unreachable S3 cannot keep the port closed. A model that times out is
  marked `unavailable` with a reason naming the timeout and is retried by the
  normal backoff (below). The compose healthcheck for `worker-models` sets
  `start_period: 120s` (policy): the first start downloads every bound model.
- **Bind loads first.** `PUT /capabilities/{code}/binding` loads the model
  before moving the pointer. If it cannot load, the answer is 503 and the
  binding is unchanged. Upload with `activate=` does the same per
  capability: a model that cannot load binds nothing, the upload still
  succeeds, and the reason is returned in the response's `warnings`; any
  existing binding and its status are left untouched.
- **`/health`.** Besides `database`, it lists one informational
  `model:<capability>` entry per bound capability: `up`, or
  `unavailable: <reason>`. An unloadable model degrades its capability, not
  the service.
- **Dispatch.** A capability whose model cannot be loaded — or whose model
  raises while scoring — is reported per capability in
  `unsupportedCapabilities` (dispatch-zones only; request-level, not per
  zone) and in the optional response field `unavailableCapabilities`
  (`{capability: reason}`, on dispatch-zones and dispatch-capabilities); the
  other requested capabilities still score. The answer is 503 only when
  nothing requested can be served. It is never scored as zero. core-engine
  turns an `unavailableCapabilities` entry into a `CAPABILITY_UNAVAILABLE`
  warning (not the "no model is uploaded" one).
- **Retry.** A failed model is retried on later requests no more often than
  every `MODEL_RETRY_INTERVAL_SECONDS` (default 30).

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
`worker_models/models/spec.py`. It declares how to feed the model and how to read what
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
`onnxruntime-extensions`); the server then passes the encoded image bytes
through untouched, and declaring any other `input` field alongside
`embedded: true` is rejected as a contradiction rather than silently ignored.

### Output kinds and their fields

Each kind is an adapter in `worker_models/models/adapters.py`. An adapter turns raw
output tensors into `{capability_code: severity 0-100}` — the same currency
`dispatch-zones` has always returned, so nothing downstream of the server
changes. `SEVERITY_SCALE = 100.0` is defined once, in `worker_models/models/spec.py`,
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
`worker_models/models/model_registry.py` (`_BATCHABLE_OUTPUT_KINDS`).

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

Validation runs before anything is stored, in `worker_models/models/validation.py`
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
  fails after the file was already stored, the server tries to delete the
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
transaction as the binding change (`worker_models/db.py`, `set_binding` /
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
  unbind it, and retry — the server will not quietly leave a live binding
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
requested capabilities are absent from the telemetry and reported in
`unavailableCapabilities` with the reason (see *Dispatch* above), other models
still run, and the row itself is not deleted — a transient storage failure is
not a deregistration. The capability is retried on the normal backoff.

## The four limits

Env config, read through `worker_models/settings.py`, never literals at call sites:

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

## `skin_ridge_density`: a built measure, not a trained model

`apps/workers/models/tools/build_ridge_density.py` builds an ONNX graph from
fixed operators (no training, no learned weights) and registers it like any
other model, capability **`skin_ridge_density`**, output `severity_scalar`.

**What it measures.** The fraction of the zone crop's pixels that lie on a
thin, elongated *dark* intensity ridge (a valley), by Frangi's multiscale
vesselness (Frangi et al., MICCAI 1998): the crop is resized to the configured
size and read as grey (the registry's own `colorFormat: GRAY` preprocessing,
intensities 0-1); per scale sigma, the Hessian is computed with
Gaussian-derivative kernels scale-normalised by sigma² (Lindeberg gamma = 1,
the normalisation Frangi uses); its eigenvalues give the line-vs-blob ratio
R_b and the structure strength S; pixels whose across-ridge curvature has the
wrong sign score 0; the vesselness is maximised over scales and the output is
the fraction of pixels above `threshold`, 0-1, which the adapter reports as
0-100. The module docstring has the equations; the tests pin the graph to a
NumPy reference (max abs difference 3e-6, bound 1e-4) and to scikit-image's
`frangi` (bound 0.05 away from the border; see the test for why the border is
excluded). The scikit-image comparison skips where scikit-image is not
installed; it is not yet declared in the model server's `[test]` extra.

**The crop is stretched, so densities do not compare across crops.** The
registry's preprocess resizes every zone crop to `input` width x height with
a plain bilinear `cv2.resize`, not aspect-preserving. The `scales` are in
pixels of that resized crop, not of the source photo, and the crop is
stretched non-uniformly when its aspect ratio differs from the input's. So
how many pixels a given skin line spans, and how strongly it responds,
depends on the source crop's resolution and aspect ratio: the same 1 px dark
line scored 0.39 in a 256x256 crop, 2.34 (horizontal) / 1.95 (vertical) in a
50x25 crop, and 0.78 / 0.39 in a 400x200 crop. Densities are **not
comparable across crop resolutions or aspect ratios**, and a small crop is
measured at sub-pixel source scales (in a 50x25 crop, sigma 1 is about a
fifth of a source pixel horizontally and a tenth vertically). The label says "resized,
not aspect-preserving" and the ONNX metadata (`ridge_density.input`) says the
same.

**What it is not.** It is a ridge density, a geometric property of the image.
It is **not a wrinkle score**, not learned, and not clinically validated:
anything line-shaped and darker than its surroundings counts (a hair, a
crease from the pose, a shadow edge). Step edges count too: the dark side
of a brow, hairline or lip boundary inside the crop curves like a valley,
and a dark band at the crop edge is folded by the reflect padding into a
symmetric valley that scores as a ridge. Frangi also responds on the flanks
of a *bright* line, where the intensity curves upward on either side of it
(on the test's synthetic patch, lines drawn 30 grey levels bright score 9.7
under `ridge: dark`, against 7.0 for the same lines drawn dark).
The model's label says "Frangi ridge density, fixed graph (not trained)" and
never "wrinkle".

**Parameters are policy.** `tools/ridge_density.json` holds `input` size,
`scales` (sigmas, in pixels of the resized crop, >= 1), `beta`, `c`,
`threshold` and `ridge` (`dark`/`bright`). They are tuning choices, not
measurements; the file says so. In particular `c` (0.015) and `threshold`
(0.5) were chosen on synthetic patches only (flat skin tone with noise
scores 0; thin lines 30 grey levels darker score about 7) and are **not
calibrated on real skin**. The builder writes them into the ONNX
metadata (`ridge_density.params`, `ridge_density.params_sha`), so the uploaded
file's sha256 identifies them, into the registry `label`, and as a 12-hex
hash into the output file names (`skin_ridge_density_<hash>.onnx` and
`.runtime-spec.json`). The hash also covers the graph version, the kernel
truncation and the runtime spec (the preprocessing that feeds the graph), so
changing any of them produces a differently named, differently hashed model.

**Building and uploading.**

```bash
cd apps/workers/models
python -m tools.build_ridge_density --out build/                         # writes .onnx + runtime spec
python -m tools.build_ridge_density --out build/ --upload <MODEL_SERVER_URL>
```

`--upload` posts to `<url>/api/v1/models/upload` with capability
`skin_ridge_density`, the runtime spec and the label, and **no `activate`
field**: the model joins the library and serves nothing. There is no flag to
activate from the builder. It also sends no `applicableZones`, so once
activated the model runs on **every** zone, eyes and lips included;
restricting it to some zones is part of the user's binding decision.

**Mapping it to a condition is the user's decision.** Once activated it is
callable through dispatch, but it contributes to no score until a condition
lists `skin_ridge_density` in reference data. Whether a ridge density
should feed a wrinkle condition, and with what weight, is reference data a
person decides; nothing here does it by default.

## Legacy rows

A row with `runtime_spec = NULL` is a pre-existing model, from before this
spec existed. It runs under `LEGACY_SPEC` (`worker_models/models/spec.py`) — 224x224
RGB, NCHW, scale 1/255, no offsets, single `severity_scalar` output for its
own `capability_code` — expressed as an explicit default spec in code, not as
a separate branch. There is one dispatch code path for legacy and declared
rows alike (`_spec_for` in `worker_models/routers/models.py`).

## Schema migration: concurrent startup, and reporting a failure

`worker_models/db.py`'s `init_pool` applies the whole statement list (`SCHEMA_STATEMENTS`
— the registry's tables, the bindings backfill, the constraint drop) at every
process start, via `apply_schema`.

**Concurrent replicas.** `CREATE TABLE IF NOT EXISTS` and `ADD COLUMN IF NOT
EXISTS` are *not* race-free in Postgres: two backends can both pass the
existence check, and the loser raises `duplicate_table` / `duplicate_column`.
Two replicas booting together is the ordinary case on any rolling deploy. So
`apply_schema` holds `pg_advisory_lock(SCHEMA_ADVISORY_LOCK_ID)` across the
*whole* list — not per statement, because the backfill and the constraint
drop must not interleave with another replica's copy of the same list. The
lock id is a fixed constant in `worker_models/db.py`; it must stay distinct from any
advisory-lock id Seagull-gateway takes on this shared database (a collision
would only serialize unrelated migrations, never corrupt anything).

**The wait for that lock is bounded.** `pg_advisory_lock` blocks forever, and
a session-level lock is held by the backend of a replica that was OOM-killed
mid-migration until TCP keepalive reaps that backend — hours, by default.
`init_pool` is awaited in the startup hook, so blocking there meant the next
replica never answered its health check and the orchestrator restart-looped the
whole server, taking capability dispatch down with it — the exact outage
`init_pool` exists to prevent. So `apply_schema` polls
`pg_try_advisory_lock` to a deadline:
`MODEL_SCHEMA_LOCK_TIMEOUT_SECONDS`, **default 60s**
(`ModelServerSettings.schema_lock_timeout_seconds` in `worker_models/settings.py` documents the
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

## Running the model server's tests

The server's test dependencies - `pytest`, `pytest-asyncio`,
`pytest-benchmark`, `onnx`, `torch`, `torchvision` - are declared **only** in
the `[test]` extra of `apps/workers/models/pyproject.toml` - not in
`[project].dependencies`, and not in the image built from
`apps/workers/models/Dockerfile`: the server infers with `onnxruntime` and never
imports any of them, and `torch` alone is several hundred megabytes. Install
them explicitly (workers do not declare `worker-common`, so name both):

```bash
pip install -e apps/workers/common -e "apps/workers/models[test]"
pytest apps/workers/models      # or: npm run test:models
```

The dispatch benchmarks are wall-clock measurements (they export a real
resnet18, and take minutes) and say nothing about correctness, so they are
deselected from every ordinary run by the `addopts` in
`apps/workers/models/pyproject.toml`:

```
addopts = '-m "not benchmark and not jev"'
```

Each worker's suite runs in its own pytest process (`pytest
apps/workers/<name>`); the packages share the test package name `tests`.

- `npm run test:models` - the model server suite (benchmarks and JEV deselected)
- `npm run test:common`, `npm run test:skin`, `npm run test:face` - the shared library, the skin worker and the face worker
- `npm run test:tryon` - tryon tests (requires torch; about 8 minutes, downloads
  models)
- `npm run test:colour` - colour tests (most need `face_landmarker.task`, see
  `docs/COLOUR-LOCAL-RUN.md`)
- `npm run bench:models` - dispatch benchmarks only
- `npm run test:jev` - the JEV benchmark smoke test (skipped without `JEV_MODEL_ID`)

The `apps/workers/models/tests/test_db_schema.py` suite needs a real Postgres and skips
without one. Point `TEST_DATABASE_URL` at a **scratch** cluster - never a
shared or hosted database; it truncates the registry tables between tests.

`/health` returns 503 while the registry database is down, or while startup's
schema-lock wait timed out, and stays that way with no reconnect attempt until
the process restarts. A platform health check that polls `/health` will
therefore fail such a deploy — that is intended: `dispatch-zones` cannot serve
correctly with the registry database unreachable. (`/healthz`, a liveness route, exists on
worker-skin only; the model server has none.)

## What Seagull-web must change

This work is raised and implemented **in the Seagull-web repository, not from
here** — `Seagull-web` is a separate repo (the Next.js dashboard) and this
repo does not touch it. What follows is the handoff. All of it now targets
the model server (`MODEL_SERVER_URL`, default `http://127.0.0.1:8096`), not the
skin worker; the routes are the same. See section 5 of
`../Seagull-web/docs/REPORT-SEAGULL-CORE-WORKERS-SPLIT.md`.

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

## Model assets

Files a worker downloads and runs itself (today: MediaPipe FaceLandmarker,
asset `face_landmarker`, used by skin, face and colour). worker-models stores
and versions them; it never runs them, and capability dispatch never sees them.

| Route | Does |
|---|---|
| `POST /api/v1/assets/{name}` | multipart `file`, optional `label`, optional `activate` (`true`/`1`/`yes` or `false`/`0`/`no`/empty, case-insensitive; anything else is 400); stored in S3 at `assets/<name>/<timestamp>_<file>`, cached by SHA-256 |
| `GET /api/v1/assets` | every version, with `active` |
| `GET /api/v1/assets/{name}/history` | activation log, newest first |
| `PUT /api/v1/assets/{name}/active` | `{"assetId": "<uuid>"}` of a version of that name |
| `DELETE /api/v1/assets/{name}/active` | deactivate |
| `GET /api/v1/assets/{name}` | the active file; `X-Content-SHA256`, `ETag`, `X-Asset-Id`; `If-None-Match` gives 304; 404 nothing active; 503 when the file cannot be served (database down, not cached and S3 unreachable, or a hash mismatch) |

Names: `^[a-z0-9_]{1,64}$`. Size: `MODEL_ASSET_MAX_FILE_BYTES` (default 200 MB).
Asset files share the model disk cache (`MODEL_CACHE_DIR`, `<sha256>.onnx`).
Active assets' files are never evicted from it; `/health` lists
`asset:<name>` as `cached` / `not cached` (informational).

Workers run `python -m worker_common.model_assets fetch <worker>` before
uvicorn (Dockerfiles, `npm run dev:<worker>`). Each worker's `asset_needs()`
names the asset and the path its loader reads (skin: `SKIN_LANDMARKER_PATH`,
set by its image to `/app/data/models/face_landmarker/face_landmarker.task`);
face pins the builds it has facegeom manifests for. A hash or pin mismatch
leaves no file (the worker reports the model unavailable). An unreachable
worker-models is retried for `MODEL_ASSET_FETCH_WAIT_SECONDS` (default 120,
so a worker started alongside it waits for its port); after that the worker
starts with an existing file in use, unchecked, or without one. The fetch
never stops a worker from starting. Compose starts skin, face and colour
after worker-models has *started*, not after it is healthy, because its
health includes the registry database. `MODEL_SERVER_URL` points workers at
worker-models (compose: `http://worker-models:8096`).

**The active `face_landmarker` must be face's pinned build** — sha256
`64184e229b263107bc2b804c6625db1341ff2bb731874b0bcc2fe6544e0bc9ff` (float16
v1, `apps/workers/face/worker_face/facegeom/manifests/`). Skin and colour
accept any build, but face refuses one without a manifest and deletes its
local copy, so activating another build disables face-measure at face's next
restart. Compare the sha256 `import_asset` (or the upload response) prints.

Seeding a file already in the bucket:
`python -m worker_models.import_asset face_landmarker model-mirror/assets/face_landmarker/face_landmarker.task --label "float16 v1"`.
Each run adds a new version row and activates it, even for the same file.

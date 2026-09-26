# Plug-and-play model registry: the declared runtime spec

Date: 2026-09-26
Status: design, awaiting review
Scope: `apps/vision-ai-worker` (registry, dispatch), `apps/core-engine/pkg/vision`
(dispatch client, warnings). The UI is built in Seagull-web against the API
described here.

## The problem

A model can already be swapped without a code change: upload an `.onnx` against
a capability code and the next request uses it. What cannot change is the
*shape* of the model. `model_registry.validate_shapes` requires exactly one
input of `[1, 3, 224, 224]` and one output of rank 2, and
`preprocess_image` hardcodes the matching preprocessing — resize to 224, scale
to 0-1, RGB, NCHW. A model trained at 384, a model expecting ImageNet
normalization, a multi-label classifier and a segmentation model are all
rejected at upload or silently wrong if they slip through.

So the registry stores *where a model is*, but the knowledge of *how to run it
and what its output means* lives in Python. Every new kind of model is a
deploy.

## What this design changes

One thing: the registry gains a **runtime spec** — a declaration, stored with
the model, of how to feed it and how to read what comes back. The worker keeps
a small set of adapters, one per output kind, selected by name at run time.
Uploading a model of a known kind needs no code change; adding a genuinely new
kind does, and that limit is deliberate.

It also flips the key twice over. Today `vision_models.capability_code` is
unique: one capability, one model, and an upload destroys its predecessor.
Instead:

- A model row **declares the capability codes it produces**, so one multi-class
  model can serve several.
- Uploading **adds to a library** rather than replacing anything. Every model
  ever uploaded stays available.
- Each capability **points at the model currently serving it**. Switching is a
  pointer move, instant and reversible; going back to a previous model is the
  same operation, not a restore.

Activation is **per capability**. A model producing acne, redness and pores can
be active for acne and redness while a specialist model serves pores. The cost
of splitting is an extra inference run per zone; the design allows it because
picking the best model per condition is the point.

### Out of scope

Deliberately deferred to their own specs, in this order:

1. **Swappable built-in four** — face parsing, pose, hijab, landmarks as slots.
2. **Per-brand model resolution** — a brand overriding a capability's model.

Versioning and rollback are no longer a separate piece of work: a library plus
an activation pointer gives both, and the previously planned rollback spec is
absorbed here.

Also out of scope: hosted/API models (no file to upload), user-authored
transform expressions, and a frontend-reconfigurable pipeline graph.

## Measured constraints

Wall-clock on ONNX Runtime CPU, random-weight torchvision exports, median of
repeated runs. Dev box is 16 cores; the other columns constrain intra-op
threads to approximate a small container.

| Model | 16 cores | 2 threads | 1 thread | Session load | File |
|---|---|---|---|---|---|
| mobilenet_v3_small @224 | 1.8 ms | 1.8 ms | 2.4 ms | 38 ms | 10 MB |
| resnet18 @224 | 9.2 ms | 14 ms | 28 ms | 108 ms | 47 MB |
| resnet50 @384 | 61 ms | 91 ms | 172 ms | 175 ms | 102 MB |
| deeplabv3_resnet50 @224 | 135 ms | 189 ms | 341 ms | 269 ms | 158 MB |

Scaled to the current dispatch shape — 8 zones x 5 capabilities, sequential,
one image per run — a segmentation model on one thread takes 13.6 s against a
20 s dispatch timeout and a 30 s analysis timeout. Small models take under a
second. **The sequential loop, not model size, is what runs out of budget**,
which is why batching is part of this spec rather than a later optimization.

These numbers are wall-clock only. They say nothing about accuracy.

## Data model

`vision_models` is created by raw SQL in `app/db.py`, not by GORM, and lives in
the database shared with Seagull-gateway (`docs/SHARED-DATABASE.md`). No other
repository reads this table; widening it is safe, but the migration stays in
`db.py` where the table is already defined.

`vision_models` becomes a library: one row per uploaded artifact, nothing
overwritten. A second table records which model currently serves each
capability.

```sql
ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS runtime_spec JSONB;
ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS produces TEXT[];
ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS label TEXT;

CREATE TABLE IF NOT EXISTS vision_capability_bindings (
  capability_code TEXT PRIMARY KEY,
  model_id        UUID NOT NULL REFERENCES vision_models(id),
  activated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_by    TEXT
);

CREATE TABLE IF NOT EXISTS vision_capability_activation_log (
  id              BIGSERIAL PRIMARY KEY,
  capability_code TEXT NOT NULL,
  model_id        UUID,               -- NULL = deactivated
  activated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_by    TEXT
);
```

`capability_code` stays on `vision_models` as the primary capability, so
existing rows keep working; its uniqueness constraint is dropped once bindings
are populated. `produces` is authoritative once set; a row with
`produces = NULL` is read as `[capability_code]`. `label` is a human name for
the row ("resnet50 384, trained 2026-09") so a library of eight acne models is
navigable — it is display only and nothing resolves by it.

A capability with no binding row is **unsupported**: absent from results with a
warning, exactly as an unregistered capability is today. Deactivating is
therefore a legitimate operation, not an error state.

The activation log exists because "why did scores shift on the 12th" must be
answerable. It is append-only and never read by the scoring path.

### Retention

Nothing is deleted automatically. A model that was live last month is the thing
you reach for when this month's is wrong, and an eviction policy that removes
it silently defeats the feature. The library therefore grows, and two things
keep that honest: the API reports total library bytes, and deleting a model is
manual and refused while any capability is bound to it.

A row with `runtime_spec = NULL` is a pre-existing model and runs under the
legacy contract — 224 RGB 0-1 NCHW, single scalar — which is expressed as an
explicit default spec in code, named `LEGACY_SPEC`, not as a branch. There is
one code path.

### The spec

Field names follow DeepStream's `nvinfer` vocabulary where one exists, so the
terms mean what a vision engineer already expects.

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
    "kind": "severity_scalar",
    // fields below depend on kind
  }
}
```

`embedded: true` means the model was built with preprocessing inside the graph
(for example via `onnxruntime-extensions`). The worker then passes encoded
image bytes through untouched and ignores every other `input` field, which the
API rejects as a contradiction rather than silently dropping.

### Output kinds

Each kind is an adapter in the worker. An adapter's job is to turn raw output
tensors into `{capability_code: severity 0-100}` — the same currency
`dispatch-zones` returns today, so nothing downstream changes.

| Kind | Fields | Produces |
|---|---|---|
| `severity_scalar` | `capability` | One capability; today's behaviour. Output clamped to 0-1, x100. |
| `multiclass_softmax` | `classes[]` of `{index, capability}`, optional `temperature` | One capability per mapped class, from its probability. Unmapped indices ignored. |
| `multilabel_sigmoid` | `classes[]` of `{index, capability}` | One capability per mapped class, independently. |
| `detection_count` | `capability`, `scoreThreshold`, `countToSeverity` of `{fullScaleCount}` | Counts detections above threshold; severity scales linearly to `fullScaleCount`, clamped. |
| `mask_area_ratio` | `classes[]` of `{index, capability}`, optional `minPixelRatio` | Fraction of mask pixels of each class, as severity. |

Every threshold, class index, label and scale in that table is **per model,
stored in the row** — none are code constants. The one code constant is the
0-100 severity range itself, which is the contract with core-engine, defined
once and named.

`detection_count` and `mask_area_ratio` need an output layout to read. Both
declare `tensorOrder` naming which output index carries what (`scores`,
`boxes`, `classes`, `mask`), because ONNX output ordering is not standardized.

## Validation at upload

Upload stays one request: file plus spec plus zones. Validation runs before
anything is stored, and rejects rather than adapts:

1. **Parse.** Unknown `kind`, unknown field, or a field the kind does not take
   is a 400 naming the offender. Unknown fields are rejected, not ignored — a
   typo in `scoreThreshold` must not silently take the default.
2. **Signature.** Load the session and compare the declared input against the
   real one. A declared 384 against a model fixed at 224 is a 422. Dynamic
   dimensions accept any declared value.
3. **Output rank.** Each kind states the rank and, where fixed, the shape it
   requires. A `multiclass_softmax` declaring five classes against a
   three-output model is a 422.
4. **Smoke run.** Run one inference on a generated test image and pass the
   result through the adapter. An adapter that raises, or returns a value
   outside 0-100, fails the upload. This catches a spec that is internally
   consistent but wrong about the model.
Steps 2-4 mean a model that uploads successfully has demonstrably run once.

There is no conflict check: an upload never displaces anything. Several models
declaring the same capability is the normal state of a library, and which one
serves it is decided by activation, separately and explicitly.

### Activation

Activating is a single write to `vision_capability_bindings` plus a log row. It
takes effect on the next request — no reload, no restart, no cache flush beyond
dropping nothing (sessions are cached per model, so both old and new stay warm
and switching back is free).

The API refuses to bind a capability to a model that does not declare it in
`produces`, which is the one way activation can be wrong.

### Resource limits

Enforced at upload, configurable, defaulting from the measurements above:

- `MODEL_MAX_FILE_BYTES` — default 200 MB. The 158 MB segmentation export is
  the largest thing measured; the limit exists so a swap cannot exhaust the
  container's disk or its download budget unnoticed.
- `MODEL_MAX_SMOKE_LATENCY_MS` — default 400 ms per single-image run, measured
  during the smoke run. Above it the upload succeeds **with a warning carried
  in the response**, naming the projected dispatch cost. It does not fail: a
  slow model may be a deliberate choice, and refusing it would make this a
  policy decision the deployment cannot override.
- `MODEL_CACHE_MAX_BYTES` — default 1 GB of loaded sessions, keyed by model id
  and evicted least-recently-used. Today's cache is unbounded and keyed by
  capability, which a library of candidates would otherwise make unbounded in a
  second dimension.
- `MODEL_LIBRARY_WARN_BYTES` — default 10 GB. Not enforced; the listing reports
  total bytes and flags when the library passes it, so storage growth is
  visible before it is a bill.

All three are env config, not literals at call sites.

## Dispatch

`dispatch-zones` changes from "for each capability, for each zone, run" to:

1. Resolve requested capabilities through their bindings to the active models.
   One model bound to four requested capabilities is one entry, not four; an
   unbound capability is reported unsupported and no model runs for it.
2. Group models by their *input spec*. Every model sharing width, height,
   color format, layout and normalization shares one preprocessed tensor per
   zone. Preprocessing is done once per (spec, zone), not once per
   (capability, zone).
3. For each model, run its applicable zones **as one batch** where the model's
   input allows a dynamic batch dimension, otherwise sequentially. Batching is
   what buys back the headroom the measurements showed disappearing.
4. Pass raw outputs through the adapter, producing `{capability: severity}`
   per zone.
5. Zone applicability (`applicable_zones`) is unchanged and applies per model.

Capabilities with no model remain `unsupportedCapabilities` in the response.
core-engine already turns that into a warning — it does not invent a score, and
this design does not change that.

### core-engine

`inference.DispatchZones` keeps its request and response shape, so
`dag_engine.go` is untouched. The one visible change: `ExecutionMetrics`
currently counts a capability per zone as an executed model, which double
counts once several capabilities come from one run. `ExecutedModelsCount`
becomes the count of distinct model executions, and `ExecutedModels` keeps
listing capability codes.

## API

Extends the existing `/api/v1/models` router; no new service.

| Endpoint | Change |
|---|---|
| `GET /kinds` | New. Returns the output kinds, their fields, types and defaults. The upload form is rendered from this, so a new kind needs no web release. |
| `POST /upload` | Adds `runtimeSpec` (JSON string in the form), `label`, and `activate` (capability codes to bind on success; empty means upload only). Absent spec means the legacy default. Never displaces an existing model. |
| `GET ""` | The library: every model, with `runtimeSpec`, `produces`, `label`, size, upload time, and the capabilities it is currently active for. Plus total library bytes. |
| `GET /capabilities` | New. Every capability code from Reference Data with its bound model, the candidates that could serve it, and when it was last switched. This is what the activation screen renders. |
| `PUT /capabilities/{code}/binding` | New. Body `{modelId}`. Binds, or rebinds, that capability. 422 if the model does not declare the capability. Rolling back is this call with the older model's id. |
| `DELETE /capabilities/{code}/binding` | New. Unbinds; the capability becomes unsupported. |
| `GET /capabilities/{code}/history` | New. The activation log for one capability. |
| `PATCH /models/{id}` | Zones and `runtimeSpec` edits, by model id. Re-runs validation steps 1, 3 and 4. |
| `DELETE /models/{id}` | Deletes a library entry and its stored file. Refused with 409 while any capability is bound to it, naming them. |

**Breaking changes for Seagull-web**, which the split makes a cross-repository
concern: `PATCH /{capability}` and `DELETE /{capability}` are replaced by their
`/models/{id}` equivalents, because a capability no longer identifies one
model. `POST /upload` stays compatible in shape, but its meaning changes — the
model no longer goes live on upload unless `activate` names the capability.
Both repositories must ship in step. The old paths are kept as stubs returning
410 with the replacement path in the message, so a stale web build fails
legibly instead of appearing to work.

## Error handling

Every failure names the model and the capability. The rule the codebase already
follows holds: a missing or broken model produces an absence and a warning, never
a substituted number. Specifically:

- Adapter raises at run time: that model's capabilities are absent from the
  zone's telemetry and a warning names the model. Other models still run.
- Model file missing from storage: absent plus warning; the row is not deleted,
  because a transient storage failure is not a deregistration.
- Cache eviction during a request: the session reloads on next use.

## Testing

TDD, following the existing suites. New tests:

- **Spec parsing** — each kind round-trips; unknown kind, unknown field and
  wrong-typed field each rejected with the offending name.
- **Adapters** — one test per kind against a hand-built output tensor with a
  known expected severity, including boundary clamping at 0 and 100.
- **Validation** — a mismatched declared input size is rejected; a spec whose
  class count exceeds the model's outputs is rejected; a model that raises in
  the adapter is rejected by the smoke run.
- **Library and activation** — a second model declaring a bound capability
  uploads without touching the binding; binding switches which model runs;
  binding to a model that does not declare the capability is rejected;
  rebinding to the previous model restores the earlier scores exactly;
  unbinding makes the capability unsupported rather than zero; deleting a bound
  model is refused, and succeeds once unbound.
- **Dispatch grouping** — two capabilities from one model cause one inference;
  two models sharing an input spec cause one preprocessing pass; a model with
  `applicable_zones` runs only on those zones.
- **Legacy rows** — a row with `runtime_spec = NULL` scores identically to
  today. This is the regression gate for the migration.
- **Benchmark** — `pytest-benchmark` over the dispatch loop at 8 zones x 5
  capabilities, so the sequential-to-batched improvement is measured rather
  than assumed. There is no vision benchmark in the repo today.

On the Go side: `DispatchZones` against a stub worker returning several
capabilities from one model, and the corrected `ExecutedModelsCount`.

## Migration

1. Add columns and both new tables; existing rows get `runtime_spec = NULL`,
   read as `LEGACY_SPEC`.
2. Backfill `produces` from `capability_code`, and one binding row per existing
   model so every capability keeps the model serving it today.
3. Ship resolution through bindings, plus adapters with `severity_scalar`
   first, behaviourally identical to today and gated by the legacy regression
   test.
4. Drop the uniqueness constraint on `capability_code` only after bindings are
   populated and resolution reads them, so no window exists in which a second
   upload could shadow a live model.
5. Add the remaining output kinds.
6. Add the library, activation and history endpoints; retire the
   capability-keyed paths to 410 stubs once Seagull-web ships its screens.

Steps 1-5 need no frontend change. The current upload form sends no spec and
no `activate`, and through step 5 that keeps its present meaning: the upload
binds the capability immediately, as it does today. The difference is that the
previous model is now kept as a library row instead of being overwritten, so
the change is invisible except that nothing is lost.

Step 6 is the only coordinated release: the default flips to upload-without-
activating, and the activation screens in Seagull-web become the way a model
goes live.

## Open questions

None blocking. Two worth deciding during implementation:

- Whether `GET /kinds` should also carry per-field help text for the form, or
  whether Seagull-web owns the wording. Suggest the API carries it, so the
  vocabulary cannot drift from the validator.
- Whether the smoke-run test image should be a fixed checked-in fixture rather
  than generated noise, so upload latency is comparable between models.

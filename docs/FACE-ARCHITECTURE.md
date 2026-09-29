# Face architecture (core-engine)

The interpretation layer over the ai-worker's face measurements: core-engine
asks the ai-worker (`docs/FACE-MEASURE.md`) for measurements of one photo and
applies a brand's stored **profile** to them, producing a face-shape
classification and per-feature traits.

- Spec: `docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md`
- Plan, including the **Seed profile literals** tables (every number in the seed,
  with its source status): `docs/superpowers/plans/2026-09-29-face-architecture-core.md`
- Code: `apps/core-engine/internal/facearch/` (handler, `Interpret`, `classify/`,
  `traits/`, `profile/`, `measureclient/`) and `apps/core-engine/cmd/facearch-profile/`.

## What it does not do

- **No guidance polygons.** `"guidance"` is always JSON `null` until the guidance
  templates and rules (Plan 3) exist. Nothing is drawn or invented in the meantime.
- **No accuracy claim.** No benchmark has been run (Plan 4). Every response carries
  `provenance.calibration.status`; the only shipped profile is `uncalibrated`.
- **The seed profile is uncalibrated.** Its numbers are heuristics or placeholders
  (see the Seed profile literals tables). Treat its output as a demonstration of the
  mechanism, not a measurement of anyone's face.

## Request

```
POST /api/v1/vision/face-architecture/:brandId/:applicationId
Content-Type: multipart/form-data
  image   the photo (required)
```

Also mounted under `/core/vision-engine/` and `/api/vision/`. The profile is
looked up **before** the ai-worker is called, so a scope with no profile costs
no inference. core-engine always asks the worker for landmarks; they are not
returned yet.

## Response (200)

| Field | Content |
|---|---|
| `quality` | The worker's quality block, unchanged, including `quality.warnings` (passed through unread). |
| `regions` | The worker's per-region visibility map, unchanged. |
| `measurements` | Only the profile's `measurementsShown`, as the worker returned them, in the worker's order. |
| `measurementsMissing` | `measurementsShown` keys the worker did not return (catalogue drift). Omitted when empty. |
| `classifications` | One entry per profile classifier (see below). |
| `traits` | One entry per profile trait (see below). |
| `guidance` | Always `null` for now. |
| `provenance` | `model`, `catalogueVersion` (the worker's), `profile` (`id`, `code`, `version`, and the `catalogueVersion` the profile targets), `calibration` (`status`, and `benchmarkRunId` / `target` when set). |

### Classifications

Each entry has a `status`:

| status | Meaning |
|---|---|
| `single` | The top class's normalised score reached `blendMargin`. `primary` is set. |
| `blend` | It did not. `primary` and `secondary` are the top two. |
| `insufficient_evidence` | No class is named; `reason` says why (see below). |
| `unavailable` | The profile no longer matches the worker's catalogue; `reason` says how. |

Other fields: `scores` (class to normalised score, summing to 1 over the
**assessable** classes), `notAssessable` (classes that could not be scored),
`missingMeasurements`, `observedWeightShare`.

Scoring is a distance to each class's prototype:
`score_c = exp(-1/2 * sum_m w_m * ((x_m - centre_m) / spread_m)^2)` over that class's
usable measurements, normalised across assessable classes. A class is not
assessable when one of its `requires` measurements is not usable, or when none of
its prototype measurements is usable.

**Scores are relative to the assessable classes only.** If a single class is
assessable, its score is 1.0 under `insufficient_evidence`; that is not
confidence, it is the absence of competitors. Read `status` and `reason` first.

`insufficient_evidence` reasons:

- `fewer than two assessable classes (N)`
- `observed weight share X below minObservedWeight Y` (too little of the prototype
  weight was measured)
- `class scores are not finite (check prototype spreads)`

### Traits

Each entry has a `status` of `assessed`, `not_assessable` or `unavailable`.

- Rules are ordered; the first whose conditions all hold wins, otherwise the trait's
  `default`.
- If **any** condition in **any** rule references a measurement that is not usable,
  the trait is `not_assessable` (`missingMeasurements`, `reason`): the default only
  means something if every earlier rule could be tested.
- `boundaryUncertain` / `alternative`: see the next section.

### Noise bands and `boundaryUncertain`

Each worker measurement may carry a noise band. core-engine re-centres it on the
value (`value + P5 - P50`, `value + P95 - P50`), so it always contains the value.
A condition whose threshold lies inside that band could have gone the other way.
Conditions are tried in rule order; the first whose flip would change the winning
label sets `boundaryUncertain: true` and `alternative` to that label.

The bands are **conservative** by construction (independent noise at the model
card's worst regional error), so **`boundaryUncertain` is common**. A `null` band
(bands disabled, or too few finite samples) means *no uncertainty information*,
not "certain": its conditions never straddle.

### What makes a measurement unusable

A measurement is usable only when the worker returned a float. It is not usable
when:

- `visibility` is `occluded` (its region is occluded or out of frame; when the eyes
  are occluded the worker withholds every measurement, reason
  `canonical frame requires region eyes`);
- `visibility` is `not_supported`;
- the value is `null` while visible (reason `degenerate geometry`);
- the key is missing from the response, or the value is a list (catalogue drift).

Visibility `unknown` is usable (the value is computed; only its region could not be
judged, and in v1 feature regions are always `unknown`).

### Catalogue drift

`Interpret` never fails. If the profile's `catalogueVersion` differs from the
worker's, or a referenced measurement is missing, not supported or list-valued in
the response, the affected classifier or trait is `unavailable` with a `reason`
beginning `catalogue drift:`. Occlusion is not drift.

## Errors

All core-engine errors have the shape `{"detail": {"code": "...", ...}}`, the same
shape as the ai-worker's errors, so clients read `detail.code` for both.

| Status | Body | When |
|---|---|---|
| 400 | `missing_image` (with `reason`) | No multipart field `image`, or it cannot be read. |
| 400 | the worker's body, unchanged | The worker could not read the image. |
| 409 | `no_active_face_architecture_profile` (`brandId`, `applicationId`) | No active profile for brand+app, brand+`*`, or `*`+`*`. There are no built-in defaults. |
| 422 | the worker's body, unchanged | Face-count or quality gates failed. |
| 503 | the worker's body, unchanged | Model unavailable on the worker. |
| 503 | `face_measure_unreachable` | The worker was unreachable, timed out, returned an unexpected status or an undecodable body, or returned a 400/422/503 that is **not** a JSON object with a `detail` key (for example an HTML gateway page). |
| 500 | `profile_lookup_failed` | The database lookup failed. |
| 500 | `profile_document_invalid` (`profileId`, `reason`) | The stored document no longer parses or validates. |

Worker 400 / 422 / 503 responses are passed through **only** when the body is JSON
with a `detail` key; otherwise the answer is `503 face_measure_unreachable`.
`face_measure_unreachable` carries **no internal detail** (no URL, no upstream
text); the cause is in the server log.

## Profiles

Table `face_architecture_profiles`, **owned by core-engine** (nothing in
Seagull-gateway reads or migrates it; see `docs/SHARED-DATABASE.md`). Columns:
`brand_id`, `application_id` (`*` = any), `code`, `title`, `status`
(`ACTIVE` / `INACTIVE`), `version`, `document` (jsonb).

**Lookup order:** brand+app, then brand+`*`, then `*`+`*`. The highest active
version in the first scope that has one wins; otherwise 409.

**One active profile per scope.** `upsert` stores the document as the next version
of (brand, app, code), makes it `ACTIVE`, and marks every other active profile in
the same brand+app scope `INACTIVE`. The upsert is serialised per scope by a
two-key advisory lock, `pg_advisory_xact_lock(1581646199, hashtext(brand|app))`,
whose key space does not overlap the ai-worker's single-key schema lock
(`1581646198`). Brand `*` with a specific application is refused: no lookup ever
reads that scope.

**Not yet run against Postgres.** The GORM store and the advisory-lock SQL have
**not** been executed against a real Postgres in development. The in-memory store
is tested always; the Postgres test is opt-in and skipped unless
`FACEARCH_TEST_DATABASE_URL` points at a scratch database. Run it before relying on
the store.

### CLI

```
go run ./apps/core-engine/cmd/facearch-profile validate <file.json>
go run ./apps/core-engine/cmd/facearch-profile upsert <file.json> --brand <id|*> --app <id|*> --code <code> [--title <title>]
```

Both fetch the ai-worker catalogue and validate the document against it.
`validate` never touches the database and prints
`<file>: valid against <catalogue> (N classifiers, N traits, calibration <status>)`.
`upsert` then stores it, migrating only `face_architecture_profiles`.

| Env | Meaning |
|---|---|
| `VISION_AI_WORKER_URL` | ai-worker base URL (default `config.DefaultVisionAIWorkerURL`). |
| `FACE_MEASURE_TIMEOUT` | Timeout for the catalogue fetch (and for the endpoint's measure call). |
| `DATABASE_URL` | Postgres, read as core-engine reads it (`upsert` only). |

Load the seed as the default (any brand, any application) profile:

```
go run ./apps/core-engine/cmd/facearch-profile validate apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json
go run ./apps/core-engine/cmd/facearch-profile upsert apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json --brand '*' --app '*' --code seagull-default-uncalibrated
```

The seed is **uncalibrated**: heuristic and placeholder numbers, each with a
source note in the file. Its prototype spreads and trait bands are deliberately
wide or unsourced, and the worker's noise bands are conservative, so expect
`blend` and `boundaryUncertain` often.

### Document schema (`schemaVersion` 1)

```
schemaVersion, catalogueVersion,
calibration { status, benchmarkRunId, target, heldOutMetrics, sources[{cite, for, note}] },
measurementsShown [key],
classifiers [{ key, blendMargin, minObservedWeight,
               classes [{ label, requires [key], prototype { key: {centre, spread, weight} } }] }],
traits [{ key, default, rules [{ label, when [{measurement, op, value}] }] }],
guidance   (opaque, reserved for Plan 3; carried unchanged, never read)
```

Parsing is strict: an unknown field is an error, so a misspelt key cannot drop a
threshold. Validation reports every problem, not just the first:

- `schemaVersion` is 1; `catalogueVersion` is non-empty and, when validating against
  a live catalogue, equal to it.
- `calibration.status` is `uncalibrated` or `calibrated`; `calibrated` requires
  `benchmarkRunId` and `target`. Source `cite` and `for` are non-empty.
- Every measurement key exists in the catalogue and is supported.
  `measurementsShown` has no duplicates.
- Classifier and trait keys are unique. `blendMargin` is in (0.5, 1];
  `minObservedWeight` is in [0, 1]; at least 2 classes, unique non-empty labels,
  non-empty prototypes; `centre` finite, `spread` and `weight` positive and finite.
- Traits: non-empty `default`, at least one rule, each rule at least one condition,
  `op` one of `< <= > >=`, finite `value`.
- **List-valued measurements** (for example `fifths`, `jaw_contour_angles`) may
  appear in `measurementsShown` but not in `requires`, prototypes or trait
  conditions. The shape is read from the catalogue entry's `shape` (`scalar` or
  `list`); core-engine keeps no table of its own, and an entry with no recognised
  shape is refused rather than guessed.
- **Source coverage (uncalibrated only):** a source covers path P when its `for` is P
  or a path under P. Every classifier's `blendMargin` and `minObservedWeight`, every
  class (`classifiers.<key>.classes.<label>`) and every trait (`traits.<key>`) must
  be covered. A `calibrated` profile is not subject to this rule.

When a stored document is loaded at request time only its structure is checked (no
catalogue); drift against the live catalogue is judged per request.

## Configuration

`FACE_MEASURE_TIMEOUT`: bounds one synchronous ai-worker face-measure call. Default
**20 s** (`config.DefaultFaceMeasureTimeout`). It is not a measured value: it copies
`DefaultVisionDispatchTimeout`, the other synchronous ai-worker model call, because
no latency has been measured yet (the call runs the landmarker plus
`FACE_MEASURE_NOISE_SAMPLES` re-measurements). Benchmark timings replace it.

## Verification status

Unit tests run without a database or a worker. Not exercised in development: the
GORM store against Postgres (opt-in `FACEARCH_TEST_DATABASE_URL`) and a run against
a live ai-worker (the catalogue check is opt-in, `FACEARCH_TEST_AI_WORKER_URL`).

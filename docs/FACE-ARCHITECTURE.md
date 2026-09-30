# Face architecture (core-engine)

The interpretation layer over the face worker's face measurements: core-engine
asks the face worker (`docs/FACE-MEASURE.md`) for measurements of one photo and
applies a brand's stored **profile** to them, producing a face-shape
classification, per-feature traits and makeup-placement polygons (guidance).

- Spec: `docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md`
- Plans, including the **Seed profile literals** tables (every number in the seed,
  with its source status): `docs/superpowers/plans/2026-09-29-face-architecture-core.md`
  (classifier and traits) and `docs/superpowers/plans/2026-09-29-face-architecture-guidance.md`
  (guidance: **Seed guidance literals**).
- Code: `apps/core-engine/internal/facearch/` (handler, `Interpret`, `classify/`,
  `traits/`, `guidance/`, `profile/`, `measureclient/`) and `apps/core-engine/cmd/facearch-profile/`.

## What it does not do

- **No validated makeup geometry.** Guidance polygons are placed from landmarks by
  profile templates. The seed's templates translate Perfect Corp's published placement
  words ("sides of the forehead", "beneath the cheekbones", ...) into landmark geometry
  of our own; Perfect Corp publishes no geometry and no amount. Every seed guidance
  number is ours or a placeholder (Seed guidance literals).
- **No seed guidance for Diamond, Oblong, Triangle or InvTriangle, no Round highlight,
  no blush and no brow.** The only public source used (Perfect Corp's per-shape contour
  guidance) covers contour for Round, Oval, Square and Heart and highlight for Oval,
  Square and Heart, and nothing else. Rather than invent placements, the seed leaves the
  rest out: such faces get empty `regions`.
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
looked up **before** the face worker is called, so a scope with no profile costs
no inference. core-engine always asks the worker for landmarks; it uses them to place
guidance polygons and returns them as `landmarks` for drawing.

## Response (200)

| Field | Content |
|---|---|
| `quality` | The worker's quality block, unchanged, including `quality.warnings` (passed through unread). |
| `regions` | The worker's per-region visibility map, unchanged. |
| `measurements` | Only the profile's `measurementsShown`, as the worker returned them, in the worker's order. |
| `measurementsMissing` | `measurementsShown` keys the worker did not return (catalogue drift). Omitted when empty. |
| `classifications` | One entry per profile classifier (see below). |
| `traits` | One entry per profile trait (see below). |
| `guidance` | `null` when the profile has no `guidance` section; otherwise `regions` and `droppedTemplates` (see Guidance). |
| `landmarks` | The worker's 478 landmarks as `[x, y]` in **image pixels**, the same space as `guidance.regions[].polygon`, at the worker's landmark index (MediaPipe 0-477; `measurements[].landmarks` index into it). Passed through unchanged: no resampling, rounding or reordering; a component beyond `x, y` (a `z`) would be dropped. `null` unless the worker returned all 478, each with finite `x` and `y`: never an empty or partial array. **For display only**: measurements are computed in the canonical IOD frame, not from these pixels. |
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
`measurements`, `missingMeasurements`, `observedWeightShare`.

`measurements` (on classifications and traits alike) lists, sorted, every
measurement key the entry's spec in the active profile reads (a classifier's
`requires` and prototype keys; a trait's condition keys), observed or not.
`missingMeasurements` is the subset that was not usable. It is read from the
profile document the result was computed from, so it is listed even for
`unavailable`; an entry with no spec behind it answers `[]`.

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

Each entry has a `status` of `assessed`, `not_assessable` or `unavailable`, and
`measurements` (see Classifications).

- Rules are ordered; the first whose conditions all hold wins, otherwise the trait's
  `default`.
- If **any** condition in **any** rule references a measurement that is not usable,
  the trait is `not_assessable` (`missingMeasurements`, `reason`): the default only
  means something if every earlier rule could be tested.
- `boundaryUncertain` / `alternative`: see the next section.

### Noise bands and `boundaryUncertain`

Each worker measurement may carry a noise band. The face worker centres each band on
the measured value (`value + P5 − P50`, `value + P95 − P50`), so a band always
contains its value; core-engine uses the band as sent.
A condition whose threshold lies inside that band could have gone the other way.
Conditions are tried in rule order; the first whose flip would change the winning
label sets `boundaryUncertain: true` and `alternative` to that label.
Conditions are flipped one at a time; a label that would change only if two
conditions of one rule flipped together is not detected, so
`boundaryUncertain: false` does not guarantee the label is robust.

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
- the value is `null` with visibility `unknown` (reason `degenerate geometry`);
- the key is missing from the response, or the value is a list (catalogue drift).

Visibility `unknown` is usable (the value is computed; only its region could not be
judged, and in v1 feature regions are always `unknown`).

### Catalogue drift

`Interpret` never fails. If the profile's `catalogueVersion` differs from the
worker's, or a referenced measurement is missing, not supported or list-valued in
the response, the affected classifier or trait is `unavailable` with a `reason`
beginning `catalogue drift:`. Occlusion is not drift.

### Guidance

```json
{"regions": [{"role": "contour", "template": "contour_cheek_hollow_r",
              "polygon": [[204.2, 330.2], [262.5, 411.1], [242.3, 413.2], [199.3, 367.4]],
              "intensity": 0.6, "ruleId": "oval-contour", "match": "primary",
              "textKeys": ["facearch.guidance.oval.contour"],
              "anchorVisibility": "unknown", "unverifiedRegions": ["contour_right", "lips"]}],
 "droppedTemplates": [{"template": "highlight_chin", "ruleId": "oval-highlight",
                       "match": "primary",
                       "reason": "region jaw is occluded (anchor landmark 152)"}]}
```

The example is illustrative only: its numbers are not from a real photo; the region
names are the worker's real ones (the template's lerps end at landmark 61, which is in
`lips`). Both lists are always present. A face on which no rule
matches gets `{"regions": [], "droppedTemplates": []}`: nothing is guessed (the
classifier says `insufficient_evidence`, or the shape has no seed rule). `guidance`
is `null` only when the profile has no guidance section.

- `polygon`: vertices in the uploaded image's pixels, in template order, unrounded and
  **not clipped** to the image.
- `role`: `contour`, `highlight`, `blush` or `brow`.
- `intensity`: the rule's rendering strength in (0, 1]; not a measurement.
- `match` (regions and dropped templates): the grade the applying rule matched at,
  `primary` or `secondary` (see Rules). `secondary` means the placement rests on
  weaker evidence: a blend's secondary class or a `boundaryUncertain` trait label.
- `textKeys`: i18n keys; the brand's content owns the words.
- `placeholder: true`: the rule's choice of this placement has no source (omitted when
  false). The whole profile's `provenance.calibration.status` still applies.
- `anchorVisibility` and `unverifiedRegions`: see Occlusion below.

**Templates.** Each point is a landmark `{"index": i}` or `{"lerp": [a, b, t]}`
(a + t(b - a)). A point may be shifted by `offsetIod: [dx, dy]` in the face worker's
canonical frame:

- origin at the midpoint of the eye centres (an eye centre is the midpoint of that
  eye's two corners);
- eye line horizontal;
- units of eye-centre distance;
- +x toward the subject's left (image right), +y down.

core-engine rebuilds that frame from the returned landmarks by the same definition as
the worker (agrees to 1e-9 on the fixture), resolves each point there and maps it back
to pixels.

**Rules.** `when` maps a classifier or trait key to labels. Every entry must match.
A rule has a grade, that of its weakest entry:

- A classifier entry matches at **primary** grade on its `primary` (status `single` or
  `blend`), and at **secondary** grade on its `secondary` when the result is a `blend`.
- A trait entry matches on its `label`, not on `alternative`: at primary grade, or at
  secondary grade when the trait is `boundaryUncertain` (its label could flip under
  noise, as a blend's secondary could be the primary).
- `insufficient_evidence`, `not_assessable` and `unavailable` never match, nor does a
  key with no result.

Primary-grade rules apply first, then secondary-grade rules, each group in document
order. Each template is placed once, by the first rule that applies it; a later rule
applying it again is skipped, even when the first rule's attempt was dropped.

**Occlusion.** The worker reports a visibility per region. In the default deployment
the occlusion threshold (`FACE_MEASURE_OCCLUSION_DELTA_E`, `docs/FACE-MEASURE.md`) is
unset, so every region is `unknown` except regions out of frame, which are
`occluded`; nothing else is dropped as occluded, so a covered region is `unknown` and
placed: a placed polygon **may sit over a hijab, a hand or anything else covering that
part of the face**. Even with the threshold set, the feature regions (`brows`, `eyes`,
`nose`, `lips`) are always `unknown` under the v1 colour detector (`occluded` only when
out of frame), so a template anchored there can never be `observed`; every seed
template has such an anchor. Each placed region says so:

- `anchorVisibility` is `observed` only when every region containing any anchor
  landmark of the template is `observed`, and, when any point has an `offsetIod`, the
  `eyes` region too (the offset is taken in the canonical frame, which is built from
  the eye corners); otherwise `unknown`.
- `unverifiedRegions` lists (sorted, never null) those regions that were not
  `observed`.

Occlusion is judged **at the anchor landmarks** (an index, or either end of a lerp),
not at the offset vertices, and polygons are not clipped: a polygon whose anchors are
visible can still extend over covered skin.

**Dropped templates.** A template that a matching rule applied is dropped, with a
`reason`, in these cases:

| reason | when |
|---|---|
| `region table recorded for <v>, worker measured with <w>` | The worker's `catalogueVersion` is not the one core-engine's region table was recorded for (see Regions of a landmark); every matched template is dropped. |
| `canonical frame requires region eyes` | The eyes region is occluded (the frame is built from the eye corners). |
| `region <r> is occluded (anchor landmark <i>)` | An anchor landmark is in a region the worker reports `occluded`. That includes regions **out of frame**, which the worker does not distinguish in `regions`. |
| `region <r> not reported by the ai-worker (anchor landmark <i>)` | The worker's `regions` lacks that region. |
| `anchor landmark <i> is in no ai-worker region` | Defence in depth; validation refuses such templates. |
| `the ai-worker returned no landmarks`, `landmarks unusable: ...` | Landmarks are missing or malformed, or the frame is degenerate (`eye centres coincide`, `eye-centre distance is not finite`). |
| `point <j> is not finite in pixels` | A finite but extreme offset or landmark overflowed when mapped to pixels. |
| `template is not defined`, `landmark <i> is not available`, ... | A malformed stored document or landmark list. |

**Regions of a landmark.** core-engine keeps one copy of the worker's region table
(`anchors.REGIONS`, with the eye corners and the landmark count) in
`internal/facearch/measureclient/anchors.go`, recorded for catalogue
`RegionsCatalogueVersion` (`fa-measure/1`). A test pins it to a recording of
`apps/workers/face/worker_face/facegeom/anchors.py` and to that file's SHA-256; the
recording also holds `measures.CATALOGUE_VERSION` (parsed from `measures.py`'s text,
which cannot be imported without numpy) and the SHA-256 of `facegeom/frame.py`, the
frame definition guidance rebuilds. Any change to `anchors.py` or `frame.py`, or to the
catalogue version, fails core-engine's tests until the recording is regenerated, from
the repository root:

```
python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
```

**Seed guidance (uncalibrated).** Perfect Corp's page covers four shapes, and only
contour and highlight. All seed geometry is translated by us from its placement lists.

- Seeded (placements sourced, geometry ours): contour for Round, Oval, Square and
  Heart; highlight for Oval, Square and Heart. 11 templates, 7 rules.
- **Not seeded, deliberately:** any guidance for Diamond, Oblong, Triangle or
  InvTriangle (the page does not cover them), a Round highlight (the page names none),
  and any blush or brow guidance (the page gives no placement). A face classified into
  one of those shapes gets `{"regions": [], "droppedTemplates": []}`. The engine
  supports blush and brow roles and `placeholder: true` rules for profiles that add
  them (a brand's makeup team, or a calibrated profile).
- No seed rule is `placeholder: true`: its placements are sourced. Its intensities
  (contour 0.6, highlight 0.5) are placeholders, stated in each rule's source note and
  covered by `calibration.status: uncalibrated`. A rule is flagged `placeholder` only
  when the choice of placement has no source, not for its intensity.

## Errors

All core-engine errors have the shape `{"detail": {"code": "...", ...}}`, the same
shape as the face worker's errors, so clients read `detail.code` for both.

| Status | Body | When |
|---|---|---|
| 400 | `missing_image` (with a fixed `reason`) | No multipart field `image`, or the upload cannot be opened or read (the cause is logged, not returned). |
| 400 | the worker's body, unchanged | The worker could not read the image. |
| 409 | `no_active_face_architecture_profile` (`brandId`, `applicationId`) | No active profile for brand+app, brand+`*`, or `*`+`*`. There are no built-in defaults. |
| 422 | the worker's body, unchanged | Face-count or quality gates failed. |
| 503 | the worker's body, unchanged | Model unavailable on the worker. |
| 503 | `face_measure_unreachable` | The worker was unreachable, timed out, answered with a redirect (redirects are never followed, so the image is sent to the configured worker only), returned an unexpected status or an undecodable body, or returned a 400/422/503 that is **not** a JSON object with a `detail` key (for example an HTML gateway page). |
| 500 | `profile_lookup_failed` | The database lookup failed. |
| 500 | `profile_document_invalid` (`profileId`, fixed `reason`) | The stored document no longer parses or validates (the cause is logged, not returned). |
| 500 | `face_measure_invalid_response` | The measure client returned neither a measurement nor an error (an internal fault, not unreachability). |

Worker 400 / 422 / 503 responses are passed through **only** when the body is JSON
with a `detail` key whose value is an object or a list (not null, a string or a number); otherwise the answer is `503 face_measure_unreachable`.
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
whose key space does not overlap the skin worker's single-key schema lock
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

Both fetch the face worker's catalogue and validate the document against it.
`validate` never touches the database and prints
`<file>: valid against <catalogue> (N classifiers, N traits, N guidance templates, N guidance rules, calibration <status>)`.
`upsert` then stores it, migrating only `face_architecture_profiles`.

Exit codes: `0` success; `1` read, parse, validation, catalogue, database or upsert
failure; `2` usage error.

| Env | Meaning |
|---|---|
| `FACE_WORKER_URL` | Face worker base URL (default `config.DefaultFaceWorkerURL`, `http://127.0.0.1:8094`). |
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
guidance { templates { <name>: { role, points [{index} | {lerp [a, b, t]}, each with optional offsetIod [dx, dy]] } },
           rules [{ id, when { <classifier|trait key>: [label] }, apply [{template, intensity}], textKeys [key], placeholder }] }
           (optional; absent = "guidance": null)
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
- **Guidance templates:** `role` is one of `contour | highlight | blush | brow`; at
  least 3 points; each point is exactly one of `index` / `lerp`; landmark indices are
  0-477, each in at least one worker region (a landmark in no region has no visibility,
  so its occlusion could not be detected: lerp between region landmarks instead); lerp
  ends differ and `t` is in [0, 1]; offsets are finite. A document with a guidance
  section must target the catalogue the region table was recorded for
  (`fa-measure/1`), reported at path `guidance`.
- Template names and rule ids contain no `.` (source paths are dot-separated, so a
  citation for `guidance.templates.a` would otherwise also cover a template `a.b`).
- **Guidance rules:** unique `id`; `when` is non-empty, each key names exactly one
  classifier or trait of the document, and each label is one it can produce; at least
  one application of a defined template, none twice, `intensity` in (0, 1]; at least
  one text key, with no whitespace.
- **Source coverage (uncalibrated only):** a source covers path P when its `for` is P
  or a path under P. Every classifier's `blendMargin` and `minObservedWeight`, every
  class (`classifiers.<key>.classes.<label>`), every trait (`traits.<key>`), every
  guidance template (`guidance.templates.<name>`) and every guidance rule
  (`guidance.rules.<id>`) must be covered. A `calibrated` profile is not subject to
  this rule.

When a stored document is loaded at request time only its structure is checked (no
catalogue); drift against the live catalogue is judged per request.

## Configuration

`FACE_MEASURE_TIMEOUT`: bounds one synchronous face-worker face-measure call. Default
**20 s** (`config.DefaultFaceMeasureTimeout`). It is not a measured value: it copies
`DefaultVisionDispatchTimeout`, the other synchronous worker model call, because
no latency has been measured yet (the call runs the landmarker plus
`FACE_MEASURE_NOISE_SAMPLES` re-measurements). Benchmark timings replace it.

## Verification status

Unit tests run without a database or a worker. Not exercised in development: the
GORM store against Postgres (opt-in `FACEARCH_TEST_DATABASE_URL`) and a run against
a live face worker (the catalogue check is opt-in, `FACEARCH_TEST_AI_WORKER_URL`).

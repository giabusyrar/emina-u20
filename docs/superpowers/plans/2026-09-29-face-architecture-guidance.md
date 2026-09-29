# Face Architecture Guidance (core-engine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Verified:** every code block in this plan was compiled and its tests run on 2026-09-29 in a scratch copy of `main` at `57af50f` (`apps/core-engine` + `packages/goplatform` + a `go.work` using both, plus a copy of `apps/ai-worker/ai_worker/vision/facegeom/`). The tasks were applied one at a time, in order, and after each one `go vet` and the `internal/facearch/...` and `cmd/facearch-profile` tests passed. At the end `gofmt -l` was clean on every new and changed file and the full `./apps/core-engine/...` suite passed. The recorded anchor file and the frame expectations were produced by the ai-worker code on `main` with the commands given. Not run: anything against a live ai-worker or a database (this plan needs neither). Follow the code as written.

**Goal:** Fill the endpoint's `guidance` (spec build-order step 3). This means four things. First, typed, validated guidance templates and rules in the profile document. Second, a Go rebuild of the ai-worker's canonical frame from the returned landmarks. Third, templates resolved to image-pixel polygons: a template is dropped with a reason when an anchor landmark's region is occluded or out of frame, and rules are matched against the classifier and trait results. Fourth, contour and highlight guidance in the uncalibrated seed profile for the four face shapes Perfect Corp's published per-shape contour guidance covers (Round, Oval, Square, Heart), and nothing else.

**Architecture:** Document types live in `profile` (`profile/guidance.go`). They cannot live in the `guidance` package: `guidance` imports `classify` and `traits`, which import `profile`, so a `profile → guidance` import would be a cycle. The ai-worker's region → landmark table, eye-corner indices and landmark count are recorded once in `measureclient/anchors.go` (the package that mirrors the worker's contract). A test pins them against a recorded JSON and the SHA-256 of `anchors.py`. New package `internal/facearch/guidance` holds `Frame` (canonical frame), `Match` (rule grading) and `Resolve` / `Place` (template → polygon, drop reasons). `facearch.Interpret` calls `guidance.Resolve` when the document has a `guidance` section; otherwise `guidance` stays `null`.

**Tech Stack:** Go 1.25 (`go.work` at the repo root) and testify. Python 3 (ai-worker, no numpy needed) is used only to regenerate one recorded JSON.

**Spec:** `docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md`. Relevant sections: the profile document's `guidance` part, "Semantics → Guidance", "Validation", the Endpoint response's `guidance`, "Seed profile", "Open literals", "Testing" (template resolution, pixel mapping, occlusion drop) and "Build order" step 3. This is Plan 3 of 4. It builds on Plan 2 as merged on `main` (`docs/superpowers/plans/2026-09-29-face-architecture-core.md`). It consumes Plan 1's API unchanged: landmarks are 478 × `[x, y]` image pixels, sent because core-engine always posts `landmarks=true`.

**Research read for the seed (2026-09-29):** Perfect Corp, "AI Face Shape Analyzer for Contour & Makeup", https://www.perfectcorp.com/business/blog/makeup/ai-face-shape-finder-recommend-contour-for-different-face-shapes. The page was opened and read. Its per-shape section covers **four** shapes only, and only contour and highlight:

| Shape | Contour (page's words) | Highlight (page's words) |
|---|---|---|
| Round | sides of the forehead and jawline; beneath the cheekbones | none named |
| Oval | sides of the forehead and under the cheekbones | center of the forehead, nose, and chin |
| Square | jawline and sides of the forehead | center of the forehead and cheekbones |
| Heart | sides of the forehead and beneath the cheekbones | center of the forehead and the chin |

The page does not cover Diamond, Oblong (long), Triangle or InvTriangle. It gives no blush placement and no brow placement; it says only that brow shaping keeps proportional spacing.

The page also has a shape-independent section, "Highlighting & Contouring Basics". It is framed around the golden ratio, which the spec rejects as a scoring target. It lists highlight on the cheekbones, the center of the forehead, the nose bridge and the chin, and contour on the sides of the nose, the jawline and the forehead edges, with contour "diagonally under the cheekbone".

The page gives **no** geometry and no amount, only shade depth ("1–2 tones darker and lighter"). So every seed number below is ours or a placeholder; only the placement words are sourced.

**Seed scope (user decision, 2026-09-29): sourced shapes only.** The seed ships only the 7 rules whose placements the page states: contour for Round, Oval, Square and Heart, and highlight for Oval, Square and Heart. Diamond, Oblong, Triangle and InvTriangle, the Round highlight, and all blush and brow guidance are left out rather than filled with placeholders. A face classified into an uncovered shape gets `{"regions": [], "droppedTemplates": []}`: honest absence. The engine still supports every role and the `placeholder` flag, for makeup-team or calibrated profiles.

## Global Constraints

- **Lanes.** Before Task 1, confirm or add your row in `docs/LANES.md` → Active claims: lane `core-facearch` (Plan 3 guidance). The row owns three areas:
  - `apps/core-engine/internal/facearch/`: new `guidance/`; new `measureclient/anchors.go` with its test and testdata; new `profile/guidance.go` and `profile/validate_guidance.go` with their tests; edits to `interpret.go`, `interpret_test.go`, `profile/document.go`, `profile/document_test.go` and `profile/validate.go`.
  - `apps/core-engine/cmd/facearch-profile/`: the `main.go` summary line, `main_test.go`, the seed JSON, `seed_test.go`, and new `seed_guidance_test.go`.
  - `docs/FACE-ARCHITECTURE.md`.

  A row from the drafting session ("Vision engine for face analysis") may already cover these paths. Prior possession wins, so re-establish contact before assuming that row is stale. Remove your row in Task 10. Never edit another session's row. Do not `git add` `docs/LANES.md`.
- **Plan 1's contract is fixed.** Do not modify the ai-worker. A test reads and hashes `anchors.py`; nothing changes it. `measureclient` stays a mirror of the worker: `anchors.go` records the worker's facts and adds no field to any response type.
- **No hardcoded policy.** No new setting. Every guidance number (lerp `t`, offsets, intensities) lives in the profile document; the seed's numbers live only in the seed file and its tables below. Code constants are facts or definitions, each named once and sourced:
  - `NLandmarks`: 478, from `anchors.N_LANDMARKS`.
  - The eye corners: 33/133 and 263/362, via `anchors.py`, which follows the model card's eye-centre definition.
  - The region table: `anchors.REGIONS`.
  - `minPolygonPoints`: 3, because a polygon needs three vertices.
  - The intensity scale (0, 1] and lerp `t` in [0, 1]: definitions of the schema's terms, not thresholds.
- **Placeholders announce themselves.**
  - Every response carries `provenance.calibration.status` (`uncalibrated` for the seed).
  - A rule whose placements have no source carries `"placeholder": true`, and every region it places repeats it.
  - Every seed template and rule is cited in `calibration.sources`. The seed has no placeholder placements; its only placeholders are intensities, flagged in each rule's source note (Decision 17).
  - A profile without a `guidance` section answers `"guidance": null`. A profile with one, on a face where no rule matches (an uncovered shape, `insufficient_evidence`), answers `{"regions": [], "droppedTemplates": []}`, never a guess.
- **Shared database.** No table changes. The profile document is jsonb in core-engine's own `face_architecture_profiles`.
- **Do not modify** the ai-worker, or any Plan 1 or Plan 2 file beyond the edits named here. Do not touch any core-engine package outside `internal/facearch/` and `cmd/facearch-profile/`. No Go imports from Seagull-gateway or Seagull-web.
- **Tests** run from the repo root: `go test ./apps/core-engine/internal/facearch/<pkg>/ -v`. Whole feature: `go test ./apps/core-engine/internal/facearch/... ./apps/core-engine/cmd/facearch-profile/`.
- **Line endings.** `*.go` is `eol=lf` (`.gitattributes`). If you script edits on Windows, write with `newline=''` (Python) or check that `git diff --stat` shows only your lines.
- **Commits:** one per task as written. Never push.

## Decisions this plan makes where the spec is silent

Each is documented in code where it lives.

1. **Document types live in `profile`** (`profile/guidance.go`), because the `guidance` package cannot hold them (import cycle through `classify`/`traits`). The types:
   - `Guidance{Templates map[string]Template; Rules []GuidanceRule}`
   - `Template{Role; Points []Point}`
   - `Point{Index *int; Lerp *Lerp; OffsetIod *[2]float64}`
   - `Lerp{A, B int; T float64}`, JSON `[a, b, t]`
   - `GuidanceRule{ID; When map[string][]string; Apply []Application; TextKeys []string; Placeholder bool}`
   - `Application{Template; Intensity}`

   `Document.Guidance` changes from opaque `json.RawMessage` to `*Guidance`; `nil` means no section.
2. **Strict parsing of guidance** reuses `Parse`'s `DisallowUnknownFields`, which also applies to nested structs. `Lerp` decodes its own array: exactly 3 numbers, with `a` and `b` JSON integers (`1.5` and `2e2` are refused, not truncated). A pre-existing behaviour this plan leaves unchanged: Go matches JSON field names case-insensitively, so `"offsetIOD"` decodes as `offsetIod`. That maps to the same field, so no value is dropped.
3. **Region table recorded once, pinned twice.** `measureclient/anchors.go` holds `anchors.REGIONS`, the eye corners and `N_LANDMARKS` as Go literals, with the source cited. `anchors_test.go` makes two checks:
   - The Go table matches `testdata/anchors_fa-measure-1.json`, which the Python script in Task 1 writes from `anchors.py` (no numpy needed).
   - The SHA-256 of the checkout's `anchors.py` (CRLF normalised to LF) matches the hash recorded in that JSON.

   Any change to `anchors.py`, even a comment, fails the test until the recording is regenerated and the Go table re-checked. Go neither imports nor runs Python.
4. **Every anchor landmark must belong to a worker region** (validation). A landmark in no region (for example mesh 50 or 205, mid-cheek) has no visibility of its own, so an occluded one could never be detected. Templates reach such points by lerping between region landmarks, for example the cheek as `lerp [1, 234, 0.5]` (nose tip to face-oval extreme).
5. **Index range** is `[0, NLandmarks)`, i.e. 0-477 (spec). Lerp ends must differ and `t` must be in [0, 1] (a point between two landmarks). Offsets must be finite. A template needs at least 3 points.
6. **`when` is a map** from a classifier or trait key to the labels that match it (spec example `{"faceShape": ["Round"]}`). Every entry must match.
   - A classifier entry matches with grade *primary* when the status is `single` or `blend` and the primary is listed. It matches with grade *secondary* when the status is `blend` and the secondary is listed.
   - A trait entry matches when the trait is `assessed` and its `label` is listed. `alternative` is not the label and never matches.
   - `insufficient_evidence`, `not_assessable`, `unavailable` and a missing result never match.

   Validation requires at least one entry. Each key must name exactly one classifier or trait of the document (a key that is both is refused). Every label must be one that classifier (class labels) or trait (rule labels and default) can produce, with no duplicates.
7. **Order and de-duplication.** Rules matched with grade *primary* apply first, then rules matched only through a blend's secondary, each group in document order. Each template is emitted at most once, by the first rule that applies it. A later rule applying the same template is skipped, even when the template was dropped. So a blend Oval/Round gets Oval's placements first and adds only what Round's rules place beyond them.
8. **Drop reasons.** A template that a matching rule applied but that cannot be placed is dropped with one of these reasons. `Resolve` never fails.
   - `canonical frame requires region eyes`: the eyes are occluded, and the frame is built from the eye corners (the worker's own wording).
   - `region <r> is occluded (anchor landmark <i>)`: the worker's regions map reports out-of-frame regions as `occluded` without a reason, so out-of-frame and covered read alike here.
   - `region <r> not reported by the ai-worker (anchor landmark <i>)`: conservative; an unknown state is not treated as visible.
   - `the ai-worker returned no landmarks` / `landmarks unusable: ...`.
   - `template is not defined`: only possible for a document that bypassed validation.
9. **Polygons** are `[[x, y], ...]` in the uploaded image's pixels, in template point order, unrounded and **not clipped** to the image.
10. **Additions beyond the spec example.** In the response, `droppedTemplates[].ruleId` and `regions[].placeholder` (omitted when false). In the document, an optional `rules[].placeholder` (default false). This is how a stand-in placement announces itself on each region, not only through the profile-wide calibration status.
11. **Intensity** is a rendering strength in (0, 1] (opacity or amount for the client), not a measurement. The spec names the field but gives no scale.
12. **Text keys:** at least one per rule, each non-empty with no whitespace. Prose is refused; the brand's content owns the words. The seed uses `facearch.guidance.<scope>.<role>`, where `<scope>` is the lowercased shape.
13. **Source coverage (Plan 2 Decision 13, extended).** An uncalibrated document must also cite every guidance template (`guidance.templates.<name>`, its geometry) and every rule (`guidance.rules.<id>`, its placements and intensities). A parent path such as `guidance` covers nothing. Guidance validation needs no catalogue, so it runs the same on upsert, CLI `validate` and load.
14. **No separate guidance drift check.** When the profile's catalogue version differs from the worker's, every classifier and trait is `unavailable`, so no rule can match. A change to the region table is caught by Decision 3's test, not at request time.
15. **The CLI summary** gains guidance counts: `valid against fa-measure/1 (1 classifiers, 10 traits, 11 guidance templates, 7 guidance rules, calibration uncalibrated)`.
16. **Seed coverage.**
    - Per-shape rules exist where the source differentiates by shape: contour for Round, Oval, Square and Heart, and highlight for Oval, Square and Heart. That is 7 rules, with sourced placements.
    - Nothing else is seeded (user decision): no rule or template for Diamond, Oblong, Triangle or InvTriangle, no Round highlight (the page names none), no blush and no brow. The page's shape-independent "basics" list is not applied to uncovered shapes, because choosing to apply it per shape would itself be an unsourced placement.
    - Faces in an uncovered shape get empty `regions` and `droppedTemplates` (the existing `Resolve` semantics: no match, nothing placed, nothing dropped). `guidance` is `null` only for a profile without a guidance section.
    - Intensities are uniform per role (contour 0.6, highlight 0.5), all placeholders.
17. **A rule whose only placeholder is its intensity is not flagged `placeholder`.** The flag (and `regions[].placeholder`) means "the choice of this placement has no source". An intensity is a rendering amount the whole uncalibrated profile leaves open: each seed rule's source note says its intensity "is a placeholder pending benchmark", and every response carries `calibration.status: uncalibrated`. Flagging every rule for its intensity would make the flag true everywhere and so tell a client nothing. The seed therefore has no `placeholder: true` rule; the engine keeps the flag for profiles that need it.

## Open literals (confirm with the user before Task 9)

AGENTS.md: a literal with no validated source is flagged before it is written.

| Value | Where | Proposed | Why no better source exists | What would replace it |
|---|---|---|---|---|
| 66 seed guidance numbers | `cmd/facearch-profile/seed/seagull-default-uncalibrated.json` → `guidance` | tables below | Perfect Corp publishes placement words, no geometry and no amount; no other public source maps them onto MediaPipe landmarks. | A makeup artist's review of the resolved polygons on consented benchmark faces (Plan 4 data), then a brand profile. Geometry is profile data, so no code change. |

No config value is added. Code constants are facts or definitions (Global Constraints → No hardcoded policy).

## Seed guidance literals

**Counting rule:** one literal per lerp `t`, one per `offsetIod` pair (both components together), and one per intensity. Landmark indices are not counted: they are the MediaPipe topology the template names (`anchors.py`), not tunable numbers.

**Status:**
- "sourced": a public source states the number itself.
- "ours": this plan translated a qualitative public statement (Perfect Corp's placement words) into a number.
- "placeholder": no source at all; the file cites it as `placeholder pending source`.

**Every number is uncalibrated.**

**Frame:** +x toward the subject's left (image right), +y down, IOD units; `_r` means the subject's right, which is image left. Fixture positions are the CC0 `sample_face.jpg` landmarks in that frame. They are shown for scale only (one face, chin out of frame).

**Totals: 66 literals — 0 sourced, 40 ours, 26 placeholder.** Of these, 40 are geometry (all ours) and 26 are intensities (all placeholder). Not seeded, so no literal: Diamond, Oblong, Triangle, InvTriangle, the Round highlight, blush and brow.

### Templates (40 geometry literals)

| # | Template (role) | Points (literals in bold) | Count | Status | Source of the placement |
|---|---|---|---|---|---|
| 1-3 | `contour_forehead_side_r` (contour) | 103, 54, 21; lerp [21, 105, **0.35**]; lerp [54, 151, **0.3**]; lerp [103, 151, **0.25**] | 3 | ours | "sides of the forehead" (Round, Oval, Square, Heart) |
| 4-6 | `contour_forehead_side_l` | mirror: 332, 284, 251; lerp [251, 334, **0.35**]; [284, 151, **0.3**]; [332, 151, **0.25**] | 3 | ours | as above |
| 7-10 | `contour_cheek_hollow_r` (contour) | lerp [234, 61, **0.1**]; [234, 61, **0.5**]; [93, 61, **0.35**]; [93, 61, **0.05**] | 4 | ours | "beneath / under the cheekbones", "diagonally under the cheekbone" (Round, Oval, Heart) |
| 11-14 | `contour_cheek_hollow_l` | mirror on 454, 323, 291, same t | 4 | ours | as above |
| 15-17 | `contour_jaw_r` (contour) | 58, 172, 150; lerp [150, 1, **0.15**]; [172, 1, **0.15**]; [58, 1, **0.15**] | 3 | ours | "jawline" (Round, Square) |
| 18-20 | `contour_jaw_l` | mirror on 288, 397, 379, same t | 3 | ours | as above |
| 21-24 | `highlight_forehead_centre` (highlight) | lerp [10, 151, **0.2**]; [151, 338, **0.35**]; [151, 9, **0.7**]; [151, 109, **0.35**] | 4 | ours | "center of the forehead" (Oval, Square, Heart) |
| 25-30 | `highlight_nose_bridge` (highlight) | 168 + **[-0.03, 0]**; 168 + **[0.03, 0]**; lerp [6, 1, **0.7**] + **[0.03, 0]**; lerp [6, 1, **0.7**] + **[-0.03, 0]** | 6 | ours | "nose" (Oval), "nose bridge" (general list) |
| 31-34 | `highlight_chin` (highlight) | lerp [17, 152, **0.3**]; [377, 17, **0.35**]; [17, 152, **0.75**]; [148, 17, **0.35**] | 4 | ours | "chin" (Oval, Heart) |
| 35-37 | `highlight_cheekbone_r` (highlight) | lerp [127, 1, **0.12**]; [33, 1, **0.3**]; [234, 1, **0.2**] | 3 | ours | "cheekbones" (Square) |
| 38-40 | `highlight_cheekbone_l` | mirror on 356, 263, 454, same t | 3 | ours | as above |

**Fixture check** (canonical frame, on the recorded face). Reference points:
- The cheekbone line (`234`/`454`) sits at y ≈ 0.29.
- The mouth corners sit at y ≈ 1.18.
- The alar base sits at |x| ≈ 0.31.
- The face-oval extremes sit at |x| ≈ 1.1.

Where the seed templates land:
- `contour_cheek_hollow_r` has vertices (-1.03, 0.38), (-0.78, 0.74), (-0.87, 0.74) and (-1.06, 0.54).
- `highlight_cheekbone_r` spans y 0.14 to 0.35.

`TestSeedTemplatesLandWhereTheirNamesSay` pins these relations, not the numbers.

### Rules (26 intensity literals)

| # | Rule id | `when.faceShape` | Templates | Intensity | Placements | Intensity status |
|---|---|---|---|---|---|---|
| 1-6 | `round-contour` | Round | forehead sides r/l, jaw r/l, cheek hollows r/l | 0.6 × 6 | sourced (Contour for Round Face) | placeholder |
| 7-10 | `oval-contour` | Oval | forehead sides r/l, cheek hollows r/l | 0.6 × 4 | sourced (Oval) | placeholder |
| 11-13 | `oval-highlight` | Oval | forehead centre, nose bridge, chin | 0.5 × 3 | sourced (Oval) | placeholder |
| 14-17 | `square-contour` | Square | jaw r/l, forehead sides r/l | 0.6 × 4 | sourced (Square) | placeholder |
| 18-20 | `square-highlight` | Square | forehead centre, cheekbones r/l | 0.5 × 3 | sourced (Square) | placeholder |
| 21-24 | `heart-contour` | Heart | forehead sides r/l, cheek hollows r/l | 0.6 × 4 | sourced (Heart) | placeholder |
| 25-26 | `heart-highlight` | Heart | forehead centre, chin | 0.5 × 2 | sourced (Heart) | placeholder |

Text keys (keys only, no prose):
- `facearch.guidance.{round,oval,square,heart}.contour` and `facearch.guidance.{oval,square,heart}.highlight`, one key per rule, as listed in the file.

No rule is `placeholder: true` (Decision 17): every placement is sourced; only the intensities are placeholders, and each rule's source note says so.

A face classified Diamond, Oblong, Triangle or InvTriangle gets no regions and nothing dropped (`TestSeedNoGuidanceForUncoveredShapes`); a Round face gets contour only. On the recorded fixture face the seed places **nothing**: `faceShape` is `insufficient_evidence` (chin out of frame), so no rule matches. Plan 2's `TestSeedClassifiesAnInFrameFace` uses synthetic in-frame measurements that classify as a blend Oval / Round. With those measurements on the same landmarks, 6 regions are placed (Oval's forehead sides, cheek hollows, forehead centre and nose bridge; Round, the secondary, adds only its jaw contour). The three jaw-anchored templates (`highlight_chin`, `contour_jaw_r`, `contour_jaw_l`) are dropped with `region jaw is occluded (anchor landmark …)`.

---

### Task 1: Recorded ai-worker anchor facts

**Files:**
- Create: `apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json`
- Create: `apps/core-engine/internal/facearch/measureclient/anchors.go`
- Test: `apps/core-engine/internal/facearch/measureclient/anchors_test.go`

**Interfaces:**
- Consumes: `apps/ai-worker/ai_worker/vision/facegeom/anchors.py` (read and hashed by the test; never imported).
- Produces (`measureclient`): `const NLandmarks = 478`; `const ExoR, EndoR, ExoL, EndoL` (33, 133, 263, 362); `const RegionEyes = "eyes"`; `func RegionNames() []string`; `func RegionLandmarks(name string) []int`; `func RegionsOf(i int) []string`.

- [ ] **Step 1: Claim the lane.** Confirm or add your row in `docs/LANES.md` → Active claims exactly as described in Global Constraints. Do not edit other rows.

- [ ] **Step 2: Record the worker's anchors.** Save this script **outside the repository** as `/tmp/record_anchors.py`:
```python
"""Writes core-engine's recorded copy of the ai-worker's region table.

Run from the repository root:
    PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=apps/ai-worker python record_anchors.py \
        > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
anchors.py imports nothing, so this needs no numpy or model.
"""
import hashlib
import json

from ai_worker.vision.facegeom import anchors as A

SOURCE = "apps/ai-worker/ai_worker/vision/facegeom/anchors.py"
src = open(SOURCE, "rb").read().replace(b"\r\n", b"\n")
lines = [
    "{",
    f'  "source": "{SOURCE}",',
    f'  "sha256": "{hashlib.sha256(src).hexdigest()}",',
    f'  "nLandmarks": {A.N_LANDMARKS},',
    f'  "eyeCorners": {{"exoR": {A.EXO_R}, "endoR": {A.ENDO_R}, "exoL": {A.EXO_L}, "endoL": {A.ENDO_L}}},',
    '  "regions": {',
]
items = list(A.REGIONS.items())
for i, (name, idx) in enumerate(items):
    lines.append(f'    "{name}": {json.dumps(list(idx))}' + ("," if i < len(items) - 1 else ""))
lines += ["  }", "}"]
print("\n".join(lines))
```
From the repository root run:
```bash
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=apps/ai-worker python /tmp/record_anchors.py > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
```
The expected file is below. Its hash is of `anchors.py` on `main` at `57af50f`. If yours differs, `anchors.py` changed after this plan was written: stop, and compare its `REGIONS`, eye corners and `N_LANDMARKS` with the table in Step 5 before continuing.
```json
{
  "source": "apps/ai-worker/ai_worker/vision/facegeom/anchors.py",
  "sha256": "f05447a5fe536bac73f7f6ad8c67accc1b17355ce5e0050fdf81dba365837fb1",
  "nLandmarks": 478,
  "eyeCorners": {"exoR": 33, "endoR": 133, "exoL": 263, "endoL": 362},
  "regions": {
    "forehead": [10, 338, 297, 332, 284, 54, 103, 67, 109, 151],
    "contour_right": [162, 127, 234, 93, 132, 21],
    "contour_left": [389, 356, 454, 323, 361, 251],
    "jaw": [58, 172, 136, 150, 149, 176, 148, 152, 377, 400, 378, 379, 365, 397, 288],
    "brows": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285, 9, 8],
    "eyes": [33, 7, 163, 144, 145, 153, 154, 155, 133, 173, 157, 158, 159, 160, 161, 246, 263, 249, 390, 373, 374, 380, 381, 382, 362, 398, 384, 385, 386, 387, 388, 466],
    "nose": [168, 6, 1, 2, 98, 327],
    "lips": [61, 185, 40, 39, 37, 0, 267, 269, 270, 409, 291, 146, 91, 181, 84, 17, 314, 405, 321, 375, 13, 14]
  }
}
```

- [ ] **Step 3: Write the failing test**

`apps/core-engine/internal/facearch/measureclient/anchors_test.go`:
```go
package measureclient_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// recordedAnchors is testdata/anchors_fa-measure-1.json, written from the
// ai-worker's anchors.py by the script in docs/superpowers/plans/
// 2026-09-29-face-architecture-guidance.md (Task 1). Regenerate it from the
// repository root with:
//
//	PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=apps/ai-worker python /tmp/record_anchors.py \
//	    > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
type recordedAnchors struct {
	Source     string           `json:"source"`
	Sha256     string           `json:"sha256"`
	NLandmarks int              `json:"nLandmarks"`
	EyeCorners map[string]int   `json:"eyeCorners"`
	Regions    map[string][]int `json:"regions"`
}

func loadRecordedAnchors(t *testing.T) recordedAnchors {
	t.Helper()
	raw, err := os.ReadFile("testdata/anchors_fa-measure-1.json")
	require.NoError(t, err)
	var r recordedAnchors
	require.NoError(t, json.Unmarshal(raw, &r))
	return r
}

// The Go table equals the recording, region for region, in order.
func TestAnchorTablesMatchRecording(t *testing.T) {
	r := loadRecordedAnchors(t)
	assert.Equal(t, r.NLandmarks, mc.NLandmarks)
	assert.Equal(t, map[string]int{"exoR": mc.ExoR, "endoR": mc.EndoR, "exoL": mc.ExoL, "endoL": mc.EndoL}, r.EyeCorners)
	names := make([]string, 0, len(r.Regions))
	for n := range r.Regions {
		names = append(names, n)
	}
	sort.Strings(names)
	assert.Equal(t, names, mc.RegionNames())
	for _, n := range names {
		assert.Equal(t, r.Regions[n], mc.RegionLandmarks(n), n)
	}
	assert.Contains(t, mc.RegionNames(), mc.RegionEyes)
}

// The recording is of the anchors.py in this checkout. Any change to that
// file (even a comment) fails here until the recording is regenerated and
// the Go table re-checked, so the two cannot drift silently. Line endings
// are normalised to LF, as the recorder does, so a Windows checkout hashes
// the same.
func TestRecordingMatchesAiWorkerAnchorsSource(t *testing.T) {
	r := loadRecordedAnchors(t)
	src, err := os.ReadFile("../../../../../" + r.Source)
	require.NoError(t, err, "the ai-worker source %s must be in this checkout (same repository)", r.Source)
	sum := sha256.Sum256(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")))
	assert.Equal(t, r.Sha256, hex.EncodeToString(sum[:]),
		"%s changed since testdata/anchors_fa-measure-1.json was recorded: regenerate it (see recordedAnchors) and update anchors.go", r.Source)
}

func TestRegionsOf(t *testing.T) {
	assert.Equal(t, []string{"jaw"}, mc.RegionsOf(172))
	assert.Equal(t, []string{"brows"}, mc.RegionsOf(9))
	assert.Equal(t, []string{"contour_right"}, mc.RegionsOf(234))
	assert.Empty(t, mc.RegionsOf(50), "a mid-cheek mesh point is in no region")
	assert.Empty(t, mc.RegionsOf(-1))
	assert.Nil(t, mc.RegionLandmarks("ears"))
}

func TestRegionLandmarksIsACopy(t *testing.T) {
	r := mc.RegionLandmarks("nose")
	r[0] = -1
	assert.Equal(t, 168, mc.RegionLandmarks("nose")[0])
}
```

- [ ] **Step 4: Run; expect FAIL** (build failure: `undefined: mc.NLandmarks`, `mc.RegionNames`, ...).

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 5: Implement**

`apps/core-engine/internal/facearch/measureclient/anchors.go`:
```go
package measureclient

import "sort"

// Landmark facts the ai-worker defines in
// apps/ai-worker/ai_worker/vision/facegeom/anchors.py (catalogue
// fa-measure/1). They describe the MediaPipe mesh and the worker's region
// definitions, not policy, so they are code. They are recorded here once,
// for guidance resolution and validation, and pinned by anchors_test.go
// against testdata/anchors_fa-measure-1.json and the SHA-256 of anchors.py:
// if anchors.py changes, that test fails until the recording is regenerated
// (command in the test) and this table is brought back in line.

// NLandmarks is the number of FaceLandmarker points the worker returns
// (anchors.N_LANDMARKS: 468 mesh points plus 10 iris points).
const NLandmarks = 478

// Eye corners (anchors.EXO_R, ENDO_R, EXO_L, ENDO_L). An eye centre is the
// midpoint of its two corners, the Face Mesh V2 model card's definition,
// which the worker's canonical frame uses (facegeom/frame.py). Subject's
// right is image left.
const (
	ExoR  = 33
	EndoR = 133
	ExoL  = 263
	EndoL = 362
)

// RegionEyes is the region the canonical frame is built from: with it
// occluded the worker withholds every measurement, and guidance places
// nothing.
const RegionEyes = "eyes"

// regions is anchors.REGIONS: the landmarks whose visibility each region
// reports. A landmark in no region has no visibility of its own.
var regions = map[string][]int{
	"forehead":      {10, 338, 297, 332, 284, 54, 103, 67, 109, 151},
	"contour_right": {162, 127, 234, 93, 132, 21},
	"contour_left":  {389, 356, 454, 323, 361, 251},
	"jaw":           {58, 172, 136, 150, 149, 176, 148, 152, 377, 400, 378, 379, 365, 397, 288},
	"brows":         {70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285, 9, 8},
	"eyes":          {33, 7, 163, 144, 145, 153, 154, 155, 133, 173, 157, 158, 159, 160, 161, 246, 263, 249, 390, 373, 374, 380, 381, 382, 362, 398, 384, 385, 386, 387, 388, 466},
	"nose":          {168, 6, 1, 2, 98, 327},
	"lips":          {61, 185, 40, 39, 37, 0, 267, 269, 270, 409, 291, 146, 91, 181, 84, 17, 314, 405, 321, 375, 13, 14},
}

var regionsByLandmark = func() map[int][]string {
	m := map[int][]string{}
	for _, name := range RegionNames() {
		for _, i := range regions[name] {
			m[i] = append(m[i], name)
		}
	}
	return m
}()

// RegionNames returns the worker's region names, sorted.
func RegionNames() []string {
	names := make([]string, 0, len(regions))
	for n := range regions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// RegionLandmarks returns a copy of the landmarks in region name (nil for
// an unknown name), in anchors.py order.
func RegionLandmarks(name string) []int {
	r, ok := regions[name]
	if !ok {
		return nil
	}
	return append([]int(nil), r...)
}

// RegionsOf returns the sorted names of the regions containing landmark i;
// empty when i belongs to none.
func RegionsOf(i int) []string {
	return append([]string(nil), regionsByLandmark[i]...)
}
```

- [ ] **Step 6: Run; expect PASS** (4 new tests; 17 in the package). `TestRecordingMatchesAiWorkerAnchorsSource` reads `apps/ai-worker/ai_worker/vision/facegeom/anchors.py` from the checkout. It must fail, not skip, when the file is missing: a silently skipped pin pins nothing.

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 7: Commit**
```bash
git add apps/core-engine/internal/facearch/measureclient/anchors.go apps/core-engine/internal/facearch/measureclient/anchors_test.go apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
git commit -m "feat(core-engine): record the ai-worker's landmark regions, pinned to anchors.py"
```

---

### Task 2: Guidance document types and strict parsing

**Files:**
- Create: `apps/core-engine/internal/facearch/profile/guidance.go`
- Modify: `apps/core-engine/internal/facearch/profile/document.go` (the `Guidance` field only)
- Modify: `apps/core-engine/internal/facearch/profile/document_test.go` (`TestParse_StrictAndGuidanceOpaque` and one import only)
- Test: `apps/core-engine/internal/facearch/profile/guidance_test.go`

**Interfaces:**
- Consumes: Plan 2's `profile.Document`, `profile.Parse`.
- Produces (`profile`):
  - `const RoleContour, RoleHighlight, RoleBlush, RoleBrow`
  - `type Guidance struct{ Templates map[string]Template; Rules []GuidanceRule }` with `TemplateNames() []string`
  - `type Template struct{ Role string; Points []Point }`
  - `type Point struct{ Index *int; Lerp *Lerp; OffsetIod *[2]float64 }` with `Anchors() []int`
  - `type Lerp struct{ A, B int; T float64 }` with `MarshalJSON` and `UnmarshalJSON`
  - `type GuidanceRule struct{ ID string; When map[string][]string; Apply []Application; TextKeys []string; Placeholder bool }` with `WhenKeys() []string`
  - `type Application struct{ Template string; Intensity float64 }`
  - `Document.Guidance *Guidance`
- Test helpers reused by Task 3: `guidanceJSON`, `withGuidance`.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/profile/guidance_test.go`:
```go
package profile_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// guidanceJSON is a small valid guidance section for validDoc. Its numbers
// are test inputs, not proposed values.
const guidanceJSON = `{
  "templates": {
    "cheek_r": {"role": "contour", "points": [
      {"index": 234}, {"lerp": [234, 61, 0.5]}, {"lerp": [93, 61, 0.35], "offsetIod": [0.0, -0.05]}]}
  },
  "rules": [
    {"id": "long-contour", "when": {"faceShape": ["Long"], "noseWidth": ["Broad", "Average"]},
     "apply": [{"template": "cheek_r", "intensity": 0.5}],
     "textKeys": ["facearch.guidance.long.contour"], "placeholder": true}
  ]
}`

// withGuidance returns validDoc with guidanceJSON and the sources it needs.
func withGuidance(t *testing.T) *profile.Document {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validDoc), &raw))
	var g any
	require.NoError(t, json.Unmarshal([]byte(guidanceJSON), &g))
	raw["guidance"] = g
	cal := raw["calibration"].(map[string]any)
	cal["sources"] = append(cal["sources"].([]any),
		map[string]any{"cite": "test", "for": "guidance.templates.cheek_r"},
		map[string]any{"cite": "test", "for": "guidance.rules.long-contour"})
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	d, err := profile.Parse(b)
	require.NoError(t, err)
	return d
}

func TestParse_Guidance(t *testing.T) {
	d := withGuidance(t)
	tpl := d.Guidance.Templates["cheek_r"]
	assert.Equal(t, profile.RoleContour, tpl.Role)
	require.Len(t, tpl.Points, 3)
	assert.Equal(t, 234, *tpl.Points[0].Index)
	assert.Nil(t, tpl.Points[0].Lerp)
	assert.Equal(t, profile.Lerp{A: 93, B: 61, T: 0.35}, *tpl.Points[2].Lerp)
	assert.Equal(t, [2]float64{0, -0.05}, *tpl.Points[2].OffsetIod)
	assert.Equal(t, []int{93, 61}, tpl.Points[2].Anchors())
	assert.Equal(t, []int{234}, tpl.Points[0].Anchors())
	r := d.Guidance.Rules[0]
	assert.Equal(t, []string{"faceShape", "noseWidth"}, r.WhenKeys())
	assert.True(t, r.Placeholder)

	b, err := json.Marshal(d.Guidance)
	require.NoError(t, err)
	assert.JSONEq(t, guidanceJSON, string(b), "guidance round-trips")
}

func TestParse_GuidanceIsStrict(t *testing.T) {
	for name, bad := range map[string]string{
		"misspelt offset":   strings.Replace(guidanceJSON, `"offsetIod"`, `"offset"`, 1),
		"fractional index":  strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234.5, 61, 0.5]`, 1),
		"exponent index":    strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[2e2, 61, 0.5]`, 1),
		"two-element lerp":  strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234, 61]`, 1),
		"string t":          strings.Replace(guidanceJSON, `[234, 61, 0.5]`, `[234, 61, "half"]`, 1),
		"misspelt rule key": strings.Replace(guidanceJSON, `"textKeys"`, `"textKey"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, guidanceJSON, bad)
			doc := strings.Replace(validDoc, `"traits": [{`, `"guidance": `+bad+`, "traits": [{`, 1)
			_, err := profile.Parse([]byte(doc))
			assert.Error(t, err)
		})
	}
}
```

In `apps/core-engine/internal/facearch/profile/document_test.go`, replace the whole of `TestParse_StrictAndGuidanceOpaque`:
```go
func TestParse_StrictAndGuidanceOpaque(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, "faceShape", d.Classifiers[0].Key)
	assert.Nil(t, d.Guidance)

	_, err := profile.Parse([]byte(`{"schemaVersion": 1, "blendMarign": 0.6}`))
	assert.Error(t, err, "unknown fields are rejected")

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validDoc), &raw))
	raw["guidance"] = map[string]any{"templates": map[string]any{"x": map[string]any{"role": "contour"}}}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	d, err = profile.Parse(b)
	require.NoError(t, err)
	assert.JSONEq(t, `{"templates":{"x":{"role":"contour"}}}`, string(d.Guidance))
}
```
with:
```go
// Guidance parsing is covered in guidance_test.go.
func TestParse_Strict(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, "faceShape", d.Classifiers[0].Key)
	assert.Nil(t, d.Guidance, "no guidance section: nil, answered as null")

	_, err := profile.Parse([]byte(`{"schemaVersion": 1, "blendMarign": 0.6}`))
	assert.Error(t, err, "unknown fields are rejected")
}
```
and remove the now-unused `"encoding/json"` import from that file.

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: profile.RoleContour`, `profile.Lerp`, `d.Guidance.Templates undefined (type json.RawMessage has no field or method Templates)`).

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 3: Implement the types**

`apps/core-engine/internal/facearch/profile/guidance.go`:
```go
package profile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// Guidance roles: what a template's polygon is for. The vocabulary is the
// spec's ("contour | highlight | blush | brow"); it names a product type,
// not a threshold.
const (
	RoleContour   = "contour"
	RoleHighlight = "highlight"
	RoleBlush     = "blush"
	RoleBrow      = "brow"
)

// Guidance is the document's makeup-placement section: named polygon
// templates placed from landmarks, and rules that choose them from
// classifier and trait results. Spec "Semantics → Guidance".
type Guidance struct {
	Templates map[string]Template `json:"templates"`
	Rules     []GuidanceRule      `json:"rules"`
}

// Template is one polygon, vertex by vertex.
type Template struct {
	Role   string  `json:"role"`
	Points []Point `json:"points"`
}

// Point is a landmark ({"index": i}) or a point between two landmarks
// ({"lerp": [a, b, t]}, a + t(b - a)), optionally shifted by OffsetIod:
// [dx, dy] in the worker's canonical frame, in eye-centre-distance units
// (+x toward the subject's left, which is image right; +y down; the eye
// line horizontal). Exactly one of Index and Lerp is set (Validate).
type Point struct {
	Index     *int        `json:"index,omitempty"`
	Lerp      *Lerp       `json:"lerp,omitempty"`
	OffsetIod *[2]float64 `json:"offsetIod,omitempty"`
}

// Lerp is the JSON triple [a, b, t]: landmark indices a and b (integers)
// and t, the fraction of the way from a to b.
type Lerp struct {
	A, B int
	T    float64
}

// MarshalJSON writes [a, b, t].
func (l Lerp) MarshalJSON() ([]byte, error) {
	return json.Marshal([]json.Number{
		json.Number(strconv.Itoa(l.A)),
		json.Number(strconv.Itoa(l.B)),
		json.Number(strconv.FormatFloat(l.T, 'g', -1, 64)),
	})
}

// UnmarshalJSON reads [a, b, t]. a and b must be JSON integers: 1.5 or 1e2
// is not a landmark index and is refused rather than truncated.
func (l *Lerp) UnmarshalJSON(b []byte) error {
	var raw []json.Number
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("lerp must be [a, b, t]: %w", err)
	}
	if len(raw) != 3 {
		return fmt.Errorf("lerp must be [a, b, t], got %d elements", len(raw))
	}
	a, err := strconv.Atoi(raw[0].String())
	if err != nil {
		return fmt.Errorf("lerp landmark a %q is not an integer", raw[0])
	}
	bb, err := strconv.Atoi(raw[1].String())
	if err != nil {
		return fmt.Errorf("lerp landmark b %q is not an integer", raw[1])
	}
	t, err := raw[2].Float64()
	if err != nil {
		return fmt.Errorf("lerp t %q: %w", raw[2], err)
	}
	*l = Lerp{A: a, B: bb, T: t}
	return nil
}

// Anchors returns the landmark indices the point is built from.
func (p Point) Anchors() []int {
	switch {
	case p.Index != nil && p.Lerp == nil:
		return []int{*p.Index}
	case p.Lerp != nil && p.Index == nil:
		return []int{p.Lerp.A, p.Lerp.B}
	}
	return nil
}

// GuidanceRule applies templates when every When entry matches. When maps a
// classifier or trait key to the labels that match it: a classifier
// matches on its primary, or on its secondary when the result is a blend;
// a trait matches on its label. insufficient_evidence, not_assessable and
// unavailable results never match.
//
// Placeholder marks a rule whose choice of placements has no source (the
// document's calibration.sources says why). Every region the rule places
// repeats it, so a client can tell a sourced placement from a stand-in.
type GuidanceRule struct {
	ID          string              `json:"id"`
	When        map[string][]string `json:"when"`
	Apply       []Application       `json:"apply"`
	TextKeys    []string            `json:"textKeys"`
	Placeholder bool                `json:"placeholder,omitempty"`
}

// Application is one template a rule places, with its intensity: a
// strength in (0, 1] the client renders (opacity or amount), not a
// measurement.
type Application struct {
	Template  string  `json:"template"`
	Intensity float64 `json:"intensity"`
}

// TemplateNames returns the template names, sorted.
func (g *Guidance) TemplateNames() []string {
	names := make([]string, 0, len(g.Templates))
	for n := range g.Templates {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// WhenKeys returns the rule's When keys, sorted.
func (r GuidanceRule) WhenKeys() []string {
	keys := make([]string, 0, len(r.When))
	for k := range r.When {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

In `apps/core-engine/internal/facearch/profile/document.go`, replace:
```go
	// Guidance belongs to Plan 3 (templates and rules). It is carried
	// opaquely so a profile with guidance round-trips unchanged; nothing in
	// this plan reads it and responses return guidance as null.
	Guidance json.RawMessage `json:"guidance,omitempty"`
```
with:
```go
	// Guidance is the optional makeup-placement section (guidance.go). A
	// document without it answers "guidance": null.
	Guidance *Guidance `json:"guidance,omitempty"`
```
(`encoding/json` stays imported: `Calibration.HeldOutMetrics` still uses it.)

- [ ] **Step 4: Run; expect PASS.** `TestParse_Guidance` and `TestParse_GuidanceIsStrict` are new; `TestParse_Strict` is the renamed test. `interpret.go` still compiles: it does not read `doc.Guidance` until Task 7.

Run: `go test ./apps/core-engine/internal/facearch/... -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/profile/guidance.go apps/core-engine/internal/facearch/profile/guidance_test.go apps/core-engine/internal/facearch/profile/document.go apps/core-engine/internal/facearch/profile/document_test.go
git commit -m "feat(core-engine): typed, strictly parsed face-architecture guidance section"
```

---

### Task 3: Guidance validation and source coverage

**Files:**
- Create: `apps/core-engine/internal/facearch/profile/validate_guidance.go`
- Modify: `apps/core-engine/internal/facearch/profile/validate.go` (the `Validate` doc comment, one call in `Validate`, one append in `coverage`)
- Test: `apps/core-engine/internal/facearch/profile/validate_guidance_test.go`

**Interfaces:**
- Consumes: Task 1 (`measureclient.NLandmarks`, `RegionsOf`), Task 2 types, and Plan 2's `checker`, `finite`, `sortedSet`, `covered`.
- Produces: guidance rules in `profile.Validate`, and so in `profile.Load`, CLI `validate` and `upsert`; unexported `(*checker).guidance` and `guidanceCoverage`.

- [ ] **Step 1: Write the failing test.** It uses `withGuidance` from Task 2's `guidance_test.go` and `problems` from Plan 2's `validate_test.go`.

`apps/core-engine/internal/facearch/profile/validate_guidance_test.go`:
```go
package profile_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func TestValidate_GuidanceValid(t *testing.T) {
	assert.NoError(t, profile.Validate(withGuidance(t), facearchtest.Catalogue()))
	assert.NoError(t, profile.Validate(withGuidance(t), nil))
}

func TestValidate_GuidanceRules(t *testing.T) {
	idx := func(i int) *int { return &i }
	cases := map[string]struct {
		mutate func(g *profile.Guidance)
		want   string
	}{
		"unknown role": {func(g *profile.Guidance) {
			tp := g.Templates["cheek_r"]
			tp.Role = "bronzer"
			g.Templates["cheek_r"] = tp
		},
			`role: must be one of contour, highlight, blush, brow, got "bronzer"`},
		"too few points": {func(g *profile.Guidance) {
			tp := g.Templates["cheek_r"]
			tp.Points = tp.Points[:2]
			g.Templates["cheek_r"] = tp
		},
			"a polygon needs at least 3 points, got 2"},
		"index above 477": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(478) },
			"points[0].index: landmark 478 is outside 0-477"},
		"negative index": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(-1) },
			"landmark -1 is outside 0-477"},
		"index in no region": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = idx(50) },
			"landmark 50 is in no ai-worker region"},
		"lerp end in no region": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.B = 205 },
			"points[1].lerp[1]: landmark 205 is in no ai-worker region"},
		"lerp t above 1": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.T = 1.2 },
			"points[1].lerp[2]: t must be in [0, 1], got 1.2"},
		"lerp t negative": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.T = -0.1 },
			"t must be in [0, 1], got -0.1"},
		"lerp same ends": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Lerp.B = 234 },
			"landmarks a and b are both 234"},
		"index and lerp": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[1].Index = idx(234) },
			"points[1]: has both index and lerp"},
		"neither": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[0].Index = nil },
			"points[0]: needs index or lerp"},
		"offset not finite": {func(g *profile.Guidance) { g.Templates["cheek_r"].Points[2].OffsetIod[1] = math.Inf(1) },
			"points[2].offsetIod: must be finite"},
		"no rules": {func(g *profile.Guidance) { g.Rules = nil }, "guidance.rules: needs at least 1 rule"},
		"duplicate rule id": {func(g *profile.Guidance) { g.Rules = append(g.Rules, g.Rules[0]) },
			`guidance.rules[1].id: duplicate rule "long-contour"`},
		"unknown when key": {func(g *profile.Guidance) { g.Rules[0].When["lipShape"] = []string{"Full"} },
			`when.lipShape: no classifier or trait "lipShape"`},
		"unknown class label": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = []string{"Oval"} },
			`"faceShape" cannot produce label "Oval" (labels: Long, Wide)`},
		"unknown trait label": {func(g *profile.Guidance) { g.Rules[0].When["noseWidth"] = []string{"Hooked"} },
			`"noseWidth" cannot produce label "Hooked" (labels: Average, Broad)`},
		"empty labels": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = nil },
			"when.faceShape: needs at least 1 label"},
		"empty when": {func(g *profile.Guidance) { g.Rules[0].When = nil },
			"when: needs at least 1 classifier or trait"},
		"duplicate label": {func(g *profile.Guidance) { g.Rules[0].When["faceShape"] = []string{"Long", "Long"} },
			`duplicate label "Long"`},
		"undefined template": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Template = "chin" },
			`apply[0].template: no template "chin"`},
		"template twice": {func(g *profile.Guidance) { g.Rules[0].Apply = append(g.Rules[0].Apply, g.Rules[0].Apply[0]) },
			`template "cheek_r" applied twice`},
		"zero intensity": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Intensity = 0 },
			"apply[0].intensity: must be in (0, 1], got 0"},
		"intensity above 1": {func(g *profile.Guidance) { g.Rules[0].Apply[0].Intensity = 1.5 },
			"must be in (0, 1], got 1.5"},
		"no text keys": {func(g *profile.Guidance) { g.Rules[0].TextKeys = nil },
			"textKeys: needs at least 1 i18n key"},
		"prose text key": {func(g *profile.Guidance) { g.Rules[0].TextKeys = []string{"Contour under the cheekbones"} },
			"must be a non-empty i18n key without whitespace"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := withGuidance(t)
			tc.mutate(d.Guidance)
			msg := problems(t, profile.Validate(d, facearchtest.Catalogue()))
			assert.Contains(t, msg, tc.want)
		})
	}
}

func TestValidate_GuidanceKeyBothClassifierAndTrait(t *testing.T) {
	d := withGuidance(t)
	d.Traits = append(d.Traits, profile.TraitSpec{Key: "faceShape", Default: "Long",
		Rules: []profile.RuleSpec{{Label: "Wide", When: []profile.Condition{{Measurement: "width_length_ratio", Op: ">", Value: 1}}}}})
	d.Calibration.Sources = append(d.Calibration.Sources, profile.Source{Cite: "test", For: "traits.faceShape"})
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, `when.faceShape: "faceShape" is both a classifier and a trait`)
}

// Decision 13, extended: an uncalibrated document cites every guidance
// template and rule.
func TestValidate_GuidanceSourceCoverage(t *testing.T) {
	d := withGuidance(t)
	d.Calibration.Sources = d.Calibration.Sources[:5]
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "no source cited for guidance.templates.cheek_r")
	assert.Contains(t, msg, "no source cited for guidance.rules.long-contour")

	d.Calibration.Sources = append(d.Calibration.Sources, profile.Source{Cite: "vocabulary only", For: "guidance"})
	msg = problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "no source cited for guidance.templates.cheek_r", "a parent path covers nothing")

	runID, target := "run-1", "perfectcorp"
	d.Calibration.Status, d.Calibration.BenchmarkRunID, d.Calibration.Target = profile.CalibrationCalibrated, &runID, &target
	assert.NoError(t, profile.Validate(d, nil), "calibrated profiles are not subject to coverage")
}

func TestLoad_ChecksGuidanceStructure(t *testing.T) {
	d := withGuidance(t)
	d.Guidance.Templates["cheek_r"].Points[0].Index = nil
	b, err := json.Marshal(d)
	require.NoError(t, err)
	_, err = profile.Load(&profile.FaceArchitectureProfile{Document: string(b)})
	assert.ErrorContains(t, err, "needs index or lerp")
}
```

- [ ] **Step 2: Run; expect FAIL.** The invalid cases return no error because guidance is not validated yet; `TestValidate_GuidanceValid` passes.

Run: `go test ./apps/core-engine/internal/facearch/profile/ -run Guidance -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/profile/validate_guidance.go`:
```go
package profile

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// minPolygonPoints: a polygon needs three vertices (geometry, not policy).
const minPolygonPoints = 3

// guidance checks the guidance section (spec "Validation": every template
// index is 0-477). Rules:
//   - templates: known role; at least 3 points; each point exactly one of
//     index / lerp; indices in [0, measureclient.NLandmarks); lerp ends
//     differ and t is in [0, 1]; offsetIod finite; every anchor landmark
//     belongs to at least one ai-worker region (measureclient.RegionsOf),
//     because a landmark in no region has no visibility, so its occlusion
//     could never be detected;
//   - rules: unique non-empty id; at least one When entry, each naming
//     exactly one classifier or trait of this document and only labels it
//     can produce; at least one application, each of a defined template,
//     no template twice, intensity in (0, 1]; at least one text key, none
//     empty or containing whitespace.
func (c *checker) guidance(d *Document) {
	g := d.Guidance
	if len(g.Templates) == 0 {
		c.addf("guidance.templates", "needs at least 1 template")
	}
	if len(g.Rules) == 0 {
		c.addf("guidance.rules", "needs at least 1 rule")
	}
	for _, name := range g.TemplateNames() {
		c.template("guidance.templates."+name, name, g.Templates[name])
	}
	labels, ambiguous := labelSets(d)
	ids := map[string]bool{}
	for i, r := range g.Rules {
		c.guidanceRule(fmt.Sprintf("guidance.rules[%d]", i), r, g, labels, ambiguous, ids)
	}
}

func (c *checker) template(path, name string, t Template) {
	if strings.TrimSpace(name) == "" {
		c.addf(path, "template name is empty")
	}
	switch t.Role {
	case RoleContour, RoleHighlight, RoleBlush, RoleBrow:
	default:
		c.addf(path+".role", "must be one of %s, %s, %s, %s, got %q", RoleContour, RoleHighlight, RoleBlush, RoleBrow, t.Role)
	}
	if len(t.Points) < minPolygonPoints {
		c.addf(path+".points", "a polygon needs at least %d points, got %d", minPolygonPoints, len(t.Points))
	}
	for i, p := range t.Points {
		pp := fmt.Sprintf("%s.points[%d]", path, i)
		switch {
		case p.Index == nil && p.Lerp == nil:
			c.addf(pp, "needs index or lerp")
		case p.Index != nil && p.Lerp != nil:
			c.addf(pp, "has both index and lerp")
		case p.Index != nil:
			c.anchor(pp+".index", *p.Index)
		default:
			c.anchor(pp+".lerp[0]", p.Lerp.A)
			c.anchor(pp+".lerp[1]", p.Lerp.B)
			if p.Lerp.A == p.Lerp.B {
				c.addf(pp+".lerp", "landmarks a and b are both %d", p.Lerp.A)
			}
			if !(p.Lerp.T >= 0 && p.Lerp.T <= 1) {
				c.addf(pp+".lerp[2]", "t must be in [0, 1], got %v", p.Lerp.T)
			}
		}
		if p.OffsetIod != nil && (!finite(p.OffsetIod[0]) || !finite(p.OffsetIod[1])) {
			c.addf(pp+".offsetIod", "must be finite")
		}
	}
}

func (c *checker) anchor(path string, i int) {
	if i < 0 || i >= measureclient.NLandmarks {
		c.addf(path, "landmark %d is outside 0-%d", i, measureclient.NLandmarks-1)
		return
	}
	if len(measureclient.RegionsOf(i)) == 0 {
		c.addf(path, "landmark %d is in no ai-worker region, so its occlusion cannot be judged; anchor on region landmarks (lerp between them to reach other points)", i)
	}
}

// labelSets maps each classifier and trait key to the labels it can
// produce. A key that is both a classifier and a trait is ambiguous in a
// rule's When.
func labelSets(d *Document) (map[string]map[string]bool, map[string]bool) {
	labels := map[string]map[string]bool{}
	ambiguous := map[string]bool{}
	for _, s := range d.Classifiers {
		set := map[string]bool{}
		for _, cl := range s.Classes {
			set[cl.Label] = true
		}
		labels[s.Key] = set
	}
	for _, t := range d.Traits {
		if _, ok := labels[t.Key]; ok {
			ambiguous[t.Key] = true
		}
		set := map[string]bool{t.Default: true}
		for _, r := range t.Rules {
			set[r.Label] = true
		}
		labels[t.Key] = set
	}
	return labels, ambiguous
}

func (c *checker) guidanceRule(path string, r GuidanceRule, g *Guidance, labels map[string]map[string]bool, ambiguous, ids map[string]bool) {
	if strings.TrimSpace(r.ID) == "" {
		c.addf(path+".id", "is empty")
	} else if ids[r.ID] {
		c.addf(path+".id", "duplicate rule %q", r.ID)
	}
	ids[r.ID] = true

	if len(r.When) == 0 {
		c.addf(path+".when", "needs at least 1 classifier or trait")
	}
	for _, key := range r.WhenKeys() {
		wp := path + ".when." + key
		set, ok := labels[key]
		switch {
		case !ok:
			c.addf(wp, "no classifier or trait %q in this document", key)
			continue
		case ambiguous[key]:
			c.addf(wp, "%q is both a classifier and a trait", key)
			continue
		}
		if len(r.When[key]) == 0 {
			c.addf(wp, "needs at least 1 label")
		}
		seen := map[string]bool{}
		for j, l := range r.When[key] {
			lp := fmt.Sprintf("%s[%d]", wp, j)
			if !set[l] {
				c.addf(lp, "%q cannot produce label %q (labels: %s)", key, l, strings.Join(sortedSet(set), ", "))
			}
			if seen[l] {
				c.addf(lp, "duplicate label %q", l)
			}
			seen[l] = true
		}
	}

	if len(r.Apply) == 0 {
		c.addf(path+".apply", "needs at least 1 template")
	}
	applied := map[string]bool{}
	for j, a := range r.Apply {
		ap := fmt.Sprintf("%s.apply[%d]", path, j)
		if _, ok := g.Templates[a.Template]; !ok {
			c.addf(ap+".template", "no template %q", a.Template)
		}
		if applied[a.Template] {
			c.addf(ap+".template", "template %q applied twice", a.Template)
		}
		applied[a.Template] = true
		if !(a.Intensity > 0 && a.Intensity <= 1) {
			c.addf(ap+".intensity", "must be in (0, 1], got %v", a.Intensity)
		}
	}

	if len(r.TextKeys) == 0 {
		c.addf(path+".textKeys", "needs at least 1 i18n key")
	}
	for j, k := range r.TextKeys {
		if k == "" || strings.IndexFunc(k, unicode.IsSpace) >= 0 {
			c.addf(fmt.Sprintf("%s.textKeys[%d]", path, j), "must be a non-empty i18n key without whitespace, got %q", k)
		}
	}
}

// guidanceCoverage lists the source paths an uncalibrated document's
// guidance must cite: every template (its geometry) and every rule (its
// choice of placements and its intensities).
func guidanceCoverage(g *Guidance) []string {
	var paths []string
	for _, name := range g.TemplateNames() {
		paths = append(paths, "guidance.templates."+name)
	}
	ids := make([]string, 0, len(g.Rules))
	for _, r := range g.Rules {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		paths = append(paths, "guidance.rules."+id)
	}
	return paths
}
```

In `apps/core-engine/internal/facearch/profile/validate.go` make exactly these three edits.

(a) In the `Validate` doc comment, replace:
```go
// (0.5, 1]; minObservedWeight is in [0, 1]. An uncalibrated document must
// cite a source for each classifier's blendMargin, minObservedWeight and
// every class, and for every trait: a source covers path P when its "for"
// is P or a path under P.
```
with:
```go
// (0.5, 1]; minObservedWeight is in [0, 1]; guidance as in
// (*checker).guidance. An uncalibrated document must cite a source for each
// classifier's blendMargin, minObservedWeight and every class, for every
// trait, and for every guidance template and rule: a source covers path P
// when its "for" is P or a path under P.
```
(b) In `Validate`, after the traits loop:
```go
	for i, t := range d.Traits {
		c.trait(fmt.Sprintf("traits[%d]", i), t, keys)
	}
```
add:
```go
	if d.Guidance != nil {
		c.guidance(d)
	}
```
(c) In `coverage`, after:
```go
	for _, t := range d.Traits {
		required = append(required, "traits."+t.Key)
	}
```
add:
```go
	if d.Guidance != nil {
		required = append(required, guidanceCoverage(d.Guidance)...)
	}
```

- [ ] **Step 4: Run; expect PASS** (5 new tests; every subtest of `TestValidate_GuidanceRules` names the rule it checks). Plan 2's tests still pass because their documents have no guidance.

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/profile/validate_guidance.go apps/core-engine/internal/facearch/profile/validate_guidance_test.go apps/core-engine/internal/facearch/profile/validate.go
git commit -m "feat(core-engine): validate guidance templates and rules; cite them when uncalibrated"
```

---

### Task 4: Canonical frame in Go

**Files:**
- Create: `apps/core-engine/internal/facearch/guidance/frame.go`
- Test: `apps/core-engine/internal/facearch/guidance/frame_test.go`

**Interfaces:**
- Consumes: Task 1 (`NLandmarks`, eye corners); `facearchtest.FixtureFace()` (recorded landmarks).
- Produces (`guidance`): `type Frame`; `func NewFrame(landmarks [][]float64) (Frame, error)`; `func (Frame) IodPx() float64`; `func (Frame) ToCanonical(p [2]float64) [2]float64`; `func (Frame) ToImage(q [2]float64) [2]float64`.

- [ ] **Step 1: Write the failing test.** The expected fixture values were printed by the worker's own `frame.canonicalise` on the same recorded landmarks. The command is in the test comment; it needs numpy, so it is not part of the test run.

`apps/core-engine/internal/facearch/guidance/frame_test.go`:
```go
package guidance_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// syntheticFace puts every landmark at (400, 300) except the eye corners,
// which make eye centres 200 px apart on a line rolled by rollDeg about
// (400, 300). Synthetic input with a known frame.
func syntheticFace(rollDeg float64) [][]float64 {
	l := make([][]float64, measureclient.NLandmarks)
	for i := range l {
		l[i] = []float64{400, 300}
	}
	r := rollDeg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	at := func(u float64) []float64 { return []float64{400 + u*c, 300 + u*s} }
	// Right eye centre at u = -100 (corners at -130 and -70), left at +100.
	l[measureclient.ExoR], l[measureclient.EndoR] = at(-130), at(-70)
	l[measureclient.EndoL], l[measureclient.ExoL] = at(70), at(130)
	return l
}

func TestFrame_KnownGeometry(t *testing.T) {
	for _, roll := range []float64{0, 7, -12} {
		l := syntheticFace(roll)
		f, err := guidance.NewFrame(l)
		require.NoError(t, err)
		assert.InDelta(t, 200, f.IodPx(), 1e-9)
		r := roll * math.Pi / 180
		// One IOD straight down the face (perpendicular to the eye line).
		below := [2]float64{400 - 200*math.Sin(r), 300 + 200*math.Cos(r)}
		c := f.ToCanonical(below)
		assert.InDelta(t, 0, c[0], 1e-9, "roll %v", roll)
		assert.InDelta(t, 1, c[1], 1e-9, "roll %v", roll)
		leftCentre := [2]float64{400 + 100*math.Cos(r), 300 + 100*math.Sin(r)}
		c = f.ToCanonical(leftCentre)
		assert.InDelta(t, 0.5, c[0], 1e-9, "+x is toward the subject's left (image right)")
		assert.InDelta(t, 0, c[1], 1e-9)
		p := [2]float64{123.4, 567.8}
		back := f.ToImage(f.ToCanonical(p))
		assert.InDelta(t, p[0], back[0], 1e-9)
		assert.InDelta(t, p[1], back[1], 1e-9)
	}
}

// The Go frame reproduces the ai-worker's frame.canonicalise on the same
// recorded landmarks. Expected values were printed by the worker on
// 2026-09-29 (repository root):
//
//	PYTHONPATH=apps/ai-worker python -c "import json, numpy as np; from ai_worker.vision.facegeom.frame import canonicalise; d = json.load(open('apps/core-engine/internal/facearch/facearchtest/testdata/measure_fixture_face.json')); f = canonicalise(np.array(d['landmarks'], float)); print(repr(f.iod_px), [f.points[i].tolist() for i in (1, 152, 234, 454, 10)])"
func TestFrame_AgreesWithWorkerOnFixture(t *testing.T) {
	l := facearchtest.FixtureFace().Landmarks
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	assert.InDelta(t, 228.22322679779995, f.IodPx(), 1e-9)
	want := map[int][2]float64{
		1:   {-0.01580165271881843, 0.594157922907696},
		152: {0.018685105234812413, 1.8204098863431526},
		234: {-1.0923884919493354, 0.2856101388205952},
		454: {1.14199773196521, 0.28003636976872304},
		10:  {-0.019282934688128913, -0.8017088665146592},
	}
	for i, w := range want {
		got := f.ToCanonical([2]float64{l[i][0], l[i][1]})
		assert.InDelta(t, w[0], got[0], 1e-9, "landmark %d x", i)
		assert.InDelta(t, w[1], got[1], 1e-9, "landmark %d y", i)
	}
}

func TestFrame_Errors(t *testing.T) {
	_, err := guidance.NewFrame(make([][]float64, 10))
	assert.ErrorContains(t, err, "want 478 landmarks, got 10")

	l := syntheticFace(0)
	l[5] = []float64{math.NaN(), 1}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "landmark 5 is not a finite [x, y]")

	l = syntheticFace(0)
	l[7] = []float64{1}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "landmark 7 is not a finite [x, y]")

	l = syntheticFace(0)
	for _, i := range []int{measureclient.ExoR, measureclient.EndoR, measureclient.ExoL, measureclient.EndoL} {
		l[i] = []float64{400, 300}
	}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "eye centres coincide")
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: package `guidance` does not exist).

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/guidance/frame.go`:
```go
// Package guidance places a profile's makeup templates on one face and
// chooses them with the profile's rules.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Guidance".
package guidance

import (
	"errors"
	"fmt"
	"math"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// Frame is the ai-worker's canonical frame, rebuilt from the landmarks it
// returned (apps/ai-worker/ai_worker/vision/facegeom/frame.py,
// canonicalise): origin at the midpoint of the two eye centres, an eye
// centre being the midpoint of that eye's two corners; rotated so the eye
// line is horizontal; lengths in eye-centre distances (IOD); +x toward the
// subject's left (image right), +y down. Template offsets are in this frame.
type Frame struct {
	mid      [2]float64
	cos, sin float64 // of the eye line's angle in the image (the roll)
	iod      float64
}

// NewFrame builds the frame from 478 [x, y] image-pixel landmarks.
func NewFrame(landmarks [][]float64) (Frame, error) {
	if len(landmarks) != measureclient.NLandmarks {
		return Frame{}, fmt.Errorf("want %d landmarks, got %d", measureclient.NLandmarks, len(landmarks))
	}
	for i, p := range landmarks {
		if len(p) != 2 || !finite(p[0]) || !finite(p[1]) {
			return Frame{}, fmt.Errorf("landmark %d is not a finite [x, y]", i)
		}
	}
	right := mid(landmarks[measureclient.ExoR], landmarks[measureclient.EndoR])
	left := mid(landmarks[measureclient.ExoL], landmarks[measureclient.EndoL])
	dx, dy := left[0]-right[0], left[1]-right[1]
	iod := math.Hypot(dx, dy)
	if !(iod > 0) {
		return Frame{}, errors.New("eye centres coincide")
	}
	return Frame{
		mid: [2]float64{(right[0] + left[0]) / 2, (right[1] + left[1]) / 2},
		cos: dx / iod,
		sin: dy / iod,
		iod: iod,
	}, nil
}

func mid(a, b []float64) [2]float64 { return [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2} }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// IodPx is the eye-centre distance in pixels.
func (f Frame) IodPx() float64 { return f.iod }

// ToCanonical maps an image point into the frame: rotate by minus the roll
// about the origin, divide by IOD (frame.py: (p - mid) @ rot.T / iod).
func (f Frame) ToCanonical(p [2]float64) [2]float64 {
	x, y := p[0]-f.mid[0], p[1]-f.mid[1]
	return [2]float64{(f.cos*x + f.sin*y) / f.iod, (-f.sin*x + f.cos*y) / f.iod}
}

// ToImage is the inverse of ToCanonical.
func (f Frame) ToImage(q [2]float64) [2]float64 {
	x, y := q[0]*f.iod, q[1]*f.iod
	return [2]float64{f.mid[0] + f.cos*x - f.sin*y, f.mid[1] + f.sin*x + f.cos*y}
}
```

- [ ] **Step 4: Run; expect PASS** (3 tests). On the fixture the frame's IOD is 228.2232 px, against the worker's `quality.iodPx` of 228.2242. The difference is because the recorder rounded landmarks to 2 decimals; live responses are not rounded.

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/guidance/frame.go apps/core-engine/internal/facearch/guidance/frame_test.go
git commit -m "feat(core-engine): rebuild the ai-worker's canonical face frame from landmarks"
```

---

### Task 5: Rule matching

**Files:**
- Create: `apps/core-engine/internal/facearch/guidance/match.go`
- Test: `apps/core-engine/internal/facearch/guidance/match_test.go`

**Interfaces:**
- Consumes: Task 2 (`profile.GuidanceRule`), Plan 2's `classify.Result` and `traits.Result` statuses.
- Produces (`guidance`): `type Evidence struct{ Classifications map[string]classify.Result; Traits map[string]traits.Result }`; `type Grade int` with `NoMatch`, `MatchSecondary`, `MatchPrimary`; `func Match(r profile.GuidanceRule, ev Evidence) Grade`. Task 6's tests reuse the test helper `shape(status, primary, secondary)`.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/guidance/match_test.go`:
```go
package guidance_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

func shape(status, primary, secondary string) guidance.Evidence {
	return guidance.Evidence{
		Classifications: map[string]classify.Result{"faceShape": {Status: status, Primary: primary, Secondary: secondary}},
		Traits:          map[string]traits.Result{},
	}
}

func TestMatch_Classifier(t *testing.T) {
	round := profile.GuidanceRule{ID: "r", When: map[string][]string{"faceShape": {"Round"}}}
	cases := []struct {
		name string
		ev   guidance.Evidence
		want guidance.Grade
	}{
		{"single primary", shape(classify.StatusSingle, "Round", ""), guidance.MatchPrimary},
		{"single other", shape(classify.StatusSingle, "Oval", ""), guidance.NoMatch},
		{"blend primary", shape(classify.StatusBlend, "Round", "Oval"), guidance.MatchPrimary},
		{"blend secondary", shape(classify.StatusBlend, "Oval", "Round"), guidance.MatchSecondary},
		{"blend neither", shape(classify.StatusBlend, "Oval", "Heart"), guidance.NoMatch},
		{"single ignores a stale secondary", shape(classify.StatusSingle, "Oval", "Round"), guidance.NoMatch},
		{"insufficient evidence", shape(classify.StatusInsufficientEvidence, "", ""), guidance.NoMatch},
		{"insufficient evidence with a stray primary", shape(classify.StatusInsufficientEvidence, "Round", ""), guidance.NoMatch},
		{"unavailable", shape(classify.StatusUnavailable, "Round", ""), guidance.NoMatch},
		{"no result", guidance.Evidence{}, guidance.NoMatch},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, guidance.Match(round, tc.ev), tc.name)
	}
}

func TestMatch_Trait(t *testing.T) {
	r := profile.GuidanceRule{ID: "r", When: map[string][]string{"eyebrowShape": {"Straight"}}}
	ev := func(res traits.Result) guidance.Evidence {
		return guidance.Evidence{Traits: map[string]traits.Result{"eyebrowShape": res}}
	}
	assert.Equal(t, guidance.MatchPrimary, guidance.Match(r, ev(traits.Result{Status: traits.StatusAssessed, Label: "Straight"})))
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusAssessed, Label: "Rounded", BoundaryUncertain: true, Alternative: "Straight"})),
		"the alternative label is not the label")
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusNotAssessable})))
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev(traits.Result{Status: traits.StatusUnavailable, Label: "Straight"})))
}

func TestMatch_AllEntriesMustMatch_WeakestGradeWins(t *testing.T) {
	r := profile.GuidanceRule{ID: "r", When: map[string][]string{"faceShape": {"Round"}, "eyebrowShape": {"Straight"}}}
	ev := shape(classify.StatusBlend, "Oval", "Round")
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Straight"}
	assert.Equal(t, guidance.MatchSecondary, guidance.Match(r, ev))
	ev.Traits["eyebrowShape"] = traits.Result{Status: traits.StatusAssessed, Label: "Rounded"}
	assert.Equal(t, guidance.NoMatch, guidance.Match(r, ev))
	assert.Equal(t, guidance.NoMatch, guidance.Match(profile.GuidanceRule{ID: "empty"}, ev), "a rule with no When never matches")
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: guidance.Evidence`, `guidance.Match`, ...).

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/guidance/match.go`:
```go
package guidance

import (
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// Evidence is what rules match on: the response's classifications and
// traits, keyed as in the profile.
type Evidence struct {
	Classifications map[string]classify.Result
	Traits          map[string]traits.Result
}

// Grade says how a rule matched.
type Grade int

const (
	// NoMatch: some When entry did not match.
	NoMatch Grade = iota
	// MatchSecondary: every entry matched, at least one only through a
	// blend's secondary class.
	MatchSecondary
	// MatchPrimary: every entry matched on a primary class or a trait label.
	MatchPrimary
)

// Match grades rule r against the evidence. A classifier entry matches when
// the result is single or blend and its primary is listed (MatchPrimary),
// or the result is a blend and its secondary is listed (MatchSecondary). A
// trait entry matches when the trait is assessed and its label is listed.
// Every other status (insufficient_evidence, not_assessable, unavailable),
// and a key with no result, never matches. The rule's grade is its weakest
// entry's.
func Match(r profile.GuidanceRule, ev Evidence) Grade {
	if len(r.When) == 0 {
		return NoMatch
	}
	grade := MatchPrimary
	for _, key := range r.WhenKeys() {
		g := matchKey(key, r.When[key], ev)
		if g < grade {
			grade = g
		}
	}
	return grade
}

func matchKey(key string, labels []string, ev Evidence) Grade {
	if res, ok := ev.Classifications[key]; ok {
		switch res.Status {
		case classify.StatusSingle, classify.StatusBlend:
			if contains(labels, res.Primary) {
				return MatchPrimary
			}
			if res.Status == classify.StatusBlend && contains(labels, res.Secondary) {
				return MatchSecondary
			}
		}
		return NoMatch
	}
	if res, ok := ev.Traits[key]; ok && res.Status == traits.StatusAssessed && contains(labels, res.Label) {
		return MatchPrimary
	}
	return NoMatch
}

func contains(list []string, s string) bool {
	if s == "" {
		return false
	}
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run; expect PASS** (3 new tests; 6 in the package).

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/guidance/match.go apps/core-engine/internal/facearch/guidance/match_test.go
git commit -m "feat(core-engine): match guidance rules on face-shape and trait results"
```

---

### Task 6: Template resolution and occlusion drop

**Files:**
- Create: `apps/core-engine/internal/facearch/guidance/resolve.go`
- Test: `apps/core-engine/internal/facearch/guidance/resolve_test.go`

**Interfaces:**
- Consumes: Tasks 1, 2, 4 and 5; `facearchtest.FixtureFace()` (landmarks and regions; `jaw` is `occluded` because it is out of frame).
- Produces (`guidance`):
  - `type Region struct{ Role, Template string; Polygon [][2]float64; Intensity float64; RuleID string; TextKeys []string; Placeholder bool }`
  - `type Dropped struct{ Template, RuleID, Reason string }`
  - `type Result struct{ Regions []Region; DroppedTemplates []Dropped }`
  - `const FrameReason`
  - `func Resolve(g *profile.Guidance, ev Evidence, landmarks [][]float64, regions map[string]string) *Result`
  - `func Place(t profile.Template, f Frame, landmarks [][]float64) ([][2]float64, error)`

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/guidance/resolve_test.go`:
```go
package guidance_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// testGuidance's numbers are test inputs, not proposed values.
const testGuidance = `{
  "templates": {
    "tip":  {"role": "highlight", "points": [{"index": 1}, {"lerp": [1, 2, 0.5]}, {"index": 2, "offsetIod": [0, 0.5]}]},
    "jaw":  {"role": "contour",   "points": [{"index": 172}, {"index": 136}, {"lerp": [150, 1, 0.2]}]},
    "brow": {"role": "brow",      "points": [{"index": 70}, {"index": 63}, {"index": 105}]}
  },
  "rules": [
    {"id": "a", "when": {"faceShape": ["Oval"]}, "apply": [{"template": "tip", "intensity": 0.5}, {"template": "jaw", "intensity": 0.6}], "textKeys": ["k.a"]},
    {"id": "b", "when": {"faceShape": ["Round"]}, "apply": [{"template": "brow", "intensity": 0.3}, {"template": "tip", "intensity": 0.9}], "textKeys": ["k.b"], "placeholder": true}
  ]
}`

func testSpec(t *testing.T) *profile.Guidance {
	t.Helper()
	var g profile.Guidance
	require.NoError(t, json.Unmarshal([]byte(testGuidance), &g))
	return &g
}

func fixture() ([][]float64, map[string]string) {
	m := facearchtest.FixtureFace()
	return m.Landmarks, m.Regions
}

func templates(res *guidance.Result) (placed []string, dropped []string) {
	for _, r := range res.Regions {
		placed = append(placed, r.RuleID+":"+r.Template)
	}
	for _, d := range res.DroppedTemplates {
		dropped = append(dropped, d.RuleID+":"+d.Template+": "+d.Reason)
	}
	return placed, dropped
}

// On the recorded face (jaw out of frame, so "occluded"), a jaw-anchored
// template is dropped with the reason and the rest is placed in pixels.
func TestResolve_FixtureSingle(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	placed, dropped := templates(res)
	assert.Equal(t, []string{"a:tip"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)

	tip := res.Regions[0]
	assert.Equal(t, profile.RoleHighlight, tip.Role)
	assert.Equal(t, 0.5, tip.Intensity)
	assert.Equal(t, []string{"k.a"}, tip.TextKeys)
	assert.False(t, tip.Placeholder)
	require.Len(t, tip.Polygon, 3)
	// An index point is the landmark itself.
	assert.InDelta(t, l[1][0], tip.Polygon[0][0], 1e-9)
	assert.InDelta(t, l[1][1], tip.Polygon[0][1], 1e-9)
	// A lerp is the same point in pixels as in the frame (affine map).
	assert.InDelta(t, (l[1][0]+l[2][0])/2, tip.Polygon[1][0], 1e-9)
	assert.InDelta(t, (l[1][1]+l[2][1])/2, tip.Polygon[1][1], 1e-9)
	// offsetIod [0, 0.5] moves half an IOD down the face, perpendicular to
	// the eye line (the fixture's roll is about -1.3 degrees).
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	dx, dy := tip.Polygon[2][0]-l[2][0], tip.Polygon[2][1]-l[2][1]
	assert.InDelta(t, 0.5*f.IodPx(), math.Hypot(dx, dy), 1e-9)
	c := f.ToCanonical(tip.Polygon[2])
	c2 := f.ToCanonical([2]float64{l[2][0], l[2][1]})
	assert.InDelta(t, c2[0], c[0], 1e-9)
	assert.InDelta(t, c2[1]+0.5, c[1], 1e-9)
}

// Primary-matched rules apply before secondary-matched ones; each template
// is placed once, by the first rule that applies it.
func TestResolve_BlendOrderAndDedup(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusBlend, "Oval", "Round"), l, regions)
	placed, dropped := templates(res)
	assert.Equal(t, []string{"a:tip", "b:brow"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)

	res = guidance.Resolve(testSpec(t), shape(classify.StatusBlend, "Round", "Oval"), l, regions)
	placed, dropped = templates(res)
	assert.Equal(t, []string{"b:brow", "b:tip"}, placed)
	assert.Equal(t, []string{"a:jaw: region jaw is occluded (anchor landmark 172)"}, dropped)
	assert.Equal(t, 0.9, res.Regions[1].Intensity)
	assert.True(t, res.Regions[1].Placeholder, "a placeholder rule's regions say so")
}

func TestResolve_NothingMatches(t *testing.T) {
	l, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusInsufficientEvidence, "", ""), l, regions)
	b, err := json.Marshal(res)
	require.NoError(t, err)
	assert.JSONEq(t, `{"regions": [], "droppedTemplates": []}`, string(b))
}

func TestResolve_EyesOccludedDropsEverything(t *testing.T) {
	l, regions := fixture()
	regions["eyes"] = "occluded"
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	placed, dropped := templates(res)
	assert.Empty(t, placed)
	assert.Equal(t, []string{
		"a:tip: canonical frame requires region eyes",
		"a:jaw: canonical frame requires region eyes",
	}, dropped)
}

func TestResolve_BadLandmarksAndRegions(t *testing.T) {
	_, regions := fixture()
	res := guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), nil, regions)
	_, dropped := templates(res)
	assert.Equal(t, []string{"a:tip: the ai-worker returned no landmarks", "a:jaw: the ai-worker returned no landmarks"}, dropped)

	l, _ := fixture()
	res = guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l[:100], regions)
	_, dropped = templates(res)
	assert.Equal(t, "a:tip: landmarks unusable: want 478 landmarks, got 100", dropped[0])

	l, regions = fixture()
	delete(regions, "nose")
	res = guidance.Resolve(testSpec(t), shape(classify.StatusSingle, "Oval", ""), l, regions)
	_, dropped = templates(res)
	assert.Equal(t, "a:tip: region nose not reported by the ai-worker (anchor landmark 1)", dropped[0])
}

func TestResolve_UndefinedTemplateIsDroppedNotFatal(t *testing.T) {
	l, regions := fixture()
	g := testSpec(t)
	g.Rules[0].Apply = append(g.Rules[0].Apply, profile.Application{Template: "gone", Intensity: 0.5})
	res := guidance.Resolve(g, shape(classify.StatusSingle, "Oval", ""), l, regions)
	_, dropped := templates(res)
	assert.Contains(t, dropped, "a:gone: template is not defined")
}

func TestPlace_RejectsMissingLandmark(t *testing.T) {
	l, _ := fixture()
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	big := 900
	_, err = guidance.Place(profile.Template{Role: "contour", Points: []profile.Point{{Index: &big}}}, f, l)
	assert.ErrorContains(t, err, "landmark 900 is not available")
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: guidance.Resolve`, `guidance.Place`, `guidance.Result`).

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/guidance/resolve.go`:
```go
package guidance

import (
	"fmt"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Region is one placed template: a polygon in image pixels.
type Region struct {
	Role     string       `json:"role"`
	Template string       `json:"template"`
	Polygon  [][2]float64 `json:"polygon"`
	// Intensity is the rule's strength for this template, in (0, 1]; a
	// rendering parameter, not a measurement.
	Intensity float64  `json:"intensity"`
	RuleID    string   `json:"ruleId"`
	TextKeys  []string `json:"textKeys"`
	// Placeholder repeats the rule's flag: the choice of this placement has
	// no source. Omitted when false.
	Placeholder bool `json:"placeholder,omitempty"`
}

// Dropped is a template a matching rule applied that could not be placed.
type Dropped struct {
	Template string `json:"template"`
	RuleID   string `json:"ruleId"`
	Reason   string `json:"reason"`
}

// Result is the response's "guidance". Both lists are always present.
type Result struct {
	Regions          []Region  `json:"regions"`
	DroppedTemplates []Dropped `json:"droppedTemplates"`
}

// FrameReason is the drop reason when the eyes are occluded: the canonical
// frame is built from the eye corners (the ai-worker withholds every
// measurement for the same reason, in the same words).
const FrameReason = "canonical frame requires region " + measureclient.RegionEyes

// Resolve chooses and places guidance for one face. It never fails: a
// template that cannot be placed is listed in DroppedTemplates with the
// reason.
//
// Rules matched on primary classes (and trait labels) apply first, then
// rules matched only through a blend's secondary class, each group in
// document order. Each template is placed at most once, by the first rule
// that applies it; a later rule applying it again is skipped, including a
// template already dropped.
//
// A template is dropped when the eyes region is occluded (no canonical
// frame), when the landmarks are missing or malformed, or when any anchor
// landmark (an index, or either end of a lerp) is in a region the worker
// reports occluded, which includes regions out of frame, or does not report
// at all. Polygons are not clipped to the image.
func Resolve(g *profile.Guidance, ev Evidence, landmarks [][]float64, regions map[string]string) *Result {
	out := &Result{Regions: []Region{}, DroppedTemplates: []Dropped{}}
	var primary, secondary []profile.GuidanceRule
	for _, r := range g.Rules {
		switch Match(r, ev) {
		case MatchPrimary:
			primary = append(primary, r)
		case MatchSecondary:
			secondary = append(secondary, r)
		}
	}
	if len(primary)+len(secondary) == 0 {
		return out
	}

	frame, frameReason := frameFor(landmarks, regions)
	done := map[string]bool{}
	for _, r := range append(primary, secondary...) {
		for _, a := range r.Apply {
			if done[a.Template] {
				continue
			}
			done[a.Template] = true
			drop := func(reason string) {
				out.DroppedTemplates = append(out.DroppedTemplates, Dropped{Template: a.Template, RuleID: r.ID, Reason: reason})
			}
			t, ok := g.Templates[a.Template]
			switch {
			case !ok:
				drop("template is not defined")
				continue
			case frameReason != "":
				drop(frameReason)
				continue
			}
			if reason := hidden(t, regions); reason != "" {
				drop(reason)
				continue
			}
			poly, err := Place(t, frame, landmarks)
			if err != nil {
				drop(err.Error())
				continue
			}
			out.Regions = append(out.Regions, Region{
				Role: t.Role, Template: a.Template, Polygon: poly, Intensity: a.Intensity,
				RuleID: r.ID, TextKeys: append([]string{}, r.TextKeys...), Placeholder: r.Placeholder,
			})
		}
	}
	return out
}

func frameFor(landmarks [][]float64, regions map[string]string) (Frame, string) {
	if regions[measureclient.RegionEyes] == measureclient.VisibilityOccluded {
		return Frame{}, FrameReason
	}
	if landmarks == nil {
		return Frame{}, "the ai-worker returned no landmarks"
	}
	f, err := NewFrame(landmarks)
	if err != nil {
		return Frame{}, "landmarks unusable: " + err.Error()
	}
	return f, ""
}

// hidden explains why a template's anchors cannot be trusted, or returns "".
func hidden(t profile.Template, regions map[string]string) string {
	for _, p := range t.Points {
		for _, i := range p.Anchors() {
			names := measureclient.RegionsOf(i)
			if len(names) == 0 {
				return fmt.Sprintf("anchor landmark %d is in no ai-worker region", i)
			}
			for _, name := range names {
				state, ok := regions[name]
				switch {
				case !ok:
					return fmt.Sprintf("region %s not reported by the ai-worker (anchor landmark %d)", name, i)
				case state == measureclient.VisibilityOccluded:
					return fmt.Sprintf("region %s is occluded (anchor landmark %d)", name, i)
				}
			}
		}
	}
	return ""
}

// Place resolves a template's points to image pixels on one face: each
// anchor goes into the canonical frame, a lerp is taken there, the offset
// (IOD units) is added, and the point is mapped back to the image. It does
// not judge occlusion (Resolve does).
func Place(t profile.Template, f Frame, landmarks [][]float64) ([][2]float64, error) {
	at := func(i int) ([2]float64, error) {
		if i < 0 || i >= len(landmarks) || len(landmarks[i]) != 2 {
			return [2]float64{}, fmt.Errorf("landmark %d is not available", i)
		}
		return f.ToCanonical([2]float64{landmarks[i][0], landmarks[i][1]}), nil
	}
	poly := make([][2]float64, 0, len(t.Points))
	for j, p := range t.Points {
		var q [2]float64
		switch {
		case p.Index != nil && p.Lerp == nil:
			c, err := at(*p.Index)
			if err != nil {
				return nil, err
			}
			q = c
		case p.Lerp != nil && p.Index == nil:
			a, err := at(p.Lerp.A)
			if err != nil {
				return nil, err
			}
			b, err := at(p.Lerp.B)
			if err != nil {
				return nil, err
			}
			q = [2]float64{a[0] + p.Lerp.T*(b[0]-a[0]), a[1] + p.Lerp.T*(b[1]-a[1])}
		default:
			return nil, fmt.Errorf("point %d needs exactly one of index and lerp", j)
		}
		if p.OffsetIod != nil {
			q[0] += p.OffsetIod[0]
			q[1] += p.OffsetIod[1]
		}
		poly = append(poly, f.ToImage(q))
	}
	return poly, nil
}
```

- [ ] **Step 4: Run; expect PASS** (7 new tests; 13 in the package).

Run: `go test ./apps/core-engine/internal/facearch/guidance/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/guidance/resolve.go apps/core-engine/internal/facearch/guidance/resolve_test.go
git commit -m "feat(core-engine): resolve guidance templates to pixel polygons, drop occluded ones"
```

---

### Task 7: Guidance in the endpoint response

**Files:**
- Modify: `apps/core-engine/internal/facearch/interpret.go` (imports, `Response.Guidance`, the end of `Interpret`)
- Modify: `apps/core-engine/internal/facearch/interpret_test.go` (one assertion message)
- Test: `apps/core-engine/internal/facearch/interpret_guidance_test.go`

**Interfaces:**
- Consumes: Task 6; Plan 2's `testProfile` (in `interpret_test.go`).
- Produces: `facearch.Response.Guidance *guidance.Result`, which is JSON `null` when the profile has no guidance section. Plan 4's benchmark reads it through `Interpret` like the rest of the response.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/interpret_guidance_test.go`:
```go
package facearch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// testGuidance's numbers are test inputs, not proposed values. The trait
// rule matches the fixture (noseWidth is Broad there); the classifier rule
// cannot (faceShape is insufficient_evidence: the chin is out of frame).
const testGuidance = `{
  "templates": {
    "nose": {"role": "highlight", "points": [{"index": 168}, {"lerp": [6, 1, 0.5]}, {"index": 1}]},
    "jaw":  {"role": "contour",   "points": [{"index": 172}, {"index": 136}, {"index": 150}]}
  },
  "rules": [
    {"id": "broad-nose", "when": {"noseWidth": ["Broad"]}, "apply": [{"template": "nose", "intensity": 0.4}], "textKeys": ["k.nose"]},
    {"id": "long-jaw", "when": {"faceShape": ["Long"]}, "apply": [{"template": "jaw", "intensity": 0.6}], "textKeys": ["k.jaw"]}
  ]
}`

func TestInterpret_Guidance(t *testing.T) {
	p, d := testProfile(t)
	var g profile.Guidance
	require.NoError(t, json.Unmarshal([]byte(testGuidance), &g))
	d.Guidance = &g

	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	require.NotNil(t, out.Guidance)
	require.Len(t, out.Guidance.Regions, 1)
	r := out.Guidance.Regions[0]
	assert.Equal(t, "nose", r.Template)
	assert.Equal(t, "broad-nose", r.RuleID)
	assert.Len(t, r.Polygon, 3)
	assert.Empty(t, out.Guidance.DroppedTemplates, "long-jaw never matched, so nothing was dropped")

	b, err := json.Marshal(out)
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &body))
	var gj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body["guidance"], &gj))
	assert.Contains(t, gj, "regions")
	assert.JSONEq(t, `[]`, string(gj["droppedTemplates"]))
}
```

In `apps/core-engine/internal/facearch/interpret_test.go` (`TestInterpret_JSONShape`), replace:
```go
	assert.Equal(t, "null", string(body["guidance"]), "guidance is null until Plan 3")
```
with:
```go
	assert.Equal(t, "null", string(body["guidance"]), "a profile without a guidance section answers null")
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `out.Guidance.Regions undefined (type json.RawMessage has no field or method Regions)`).

Run: `go test ./apps/core-engine/internal/facearch/ -v`

- [ ] **Step 3: Implement.** In `apps/core-engine/internal/facearch/interpret.go`:

(a) In the import block, remove `"encoding/json"` and add `"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"` after the `classify` import.

(b) Replace:
```go
	// Guidance is filled by Plan 3 (guidance templates and rules). Until
	// then it is always JSON null: no polygons are invented.
	Guidance   json.RawMessage `json:"guidance"`
	Provenance Provenance      `json:"provenance"`
```
with:
```go
	// Guidance is the profile's makeup placement for this face (package
	// guidance); JSON null when the profile has no guidance section.
	Guidance   *guidance.Result `json:"guidance"`
	Provenance Provenance       `json:"provenance"`
```

(c) At the end of `Interpret`, replace:
```go
		out.Traits[spec.Key] = traits.Evaluate(spec, idx)
	}
	return out
}
```
with:
```go
		out.Traits[spec.Key] = traits.Evaluate(spec, idx)
	}
	if doc.Guidance != nil {
		out.Guidance = guidance.Resolve(doc.Guidance,
			guidance.Evidence{Classifications: out.Classifications, Traits: out.Traits},
			m.Landmarks, m.Regions)
	}
	return out
}
```

- [ ] **Step 4: Run; expect PASS** (17 tests in the package, including the handler's).

Run: `go test ./apps/core-engine/internal/facearch/... -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/interpret.go apps/core-engine/internal/facearch/interpret_test.go apps/core-engine/internal/facearch/interpret_guidance_test.go
git commit -m "feat(core-engine): return guidance polygons from the face-architecture endpoint"
```

---

### Task 8: CLI summary counts guidance

**Files:**
- Modify: `apps/core-engine/cmd/facearch-profile/main.go` (the summary line in `load`)
- Modify: `apps/core-engine/cmd/facearch-profile/main_test.go` (`TestValidate` expectation)
- Modify: `apps/core-engine/cmd/facearch-profile/seed_test.go` (`TestValidateSeed` expectation)

**Interfaces:**
- Produces: `validate` and `upsert` print `<file>: valid against <catalogue> (N classifiers, N traits, N guidance templates, N guidance rules, calibration <status>)`.

- [ ] **Step 1: Update the tests.**
  - In `main_test.go` (`TestValidate`), replace `"valid against fa-measure/1 (1 classifiers, 0 traits, calibration uncalibrated)"` with `"valid against fa-measure/1 (1 classifiers, 0 traits, 0 guidance templates, 0 guidance rules, calibration uncalibrated)"`.
  - In `seed_test.go` (`TestValidateSeed`), replace `"valid against fa-measure/1 (1 classifiers, 10 traits, calibration uncalibrated)"` with `"valid against fa-measure/1 (1 classifiers, 10 traits, 0 guidance templates, 0 guidance rules, calibration uncalibrated)"`. Task 9 raises the counts to 15 and 11.

- [ ] **Step 2: Run; expect FAIL** (`TestValidate`, `TestValidateSeed`: the output lacks the guidance counts).

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 3: Implement.** In `main.go` (`load`), replace:
```go
	fmt.Fprintf(stdout, "%s: valid against %s (%d classifiers, %d traits, calibration %s)\n",
		file, cat.CatalogueVersion, len(doc.Classifiers), len(doc.Traits), doc.Calibration.Status)
```
with:
```go
	templates, rules := 0, 0
	if doc.Guidance != nil {
		templates, rules = len(doc.Guidance.Templates), len(doc.Guidance.Rules)
	}
	fmt.Fprintf(stdout, "%s: valid against %s (%d classifiers, %d traits, %d guidance templates, %d guidance rules, calibration %s)\n",
		file, cat.CatalogueVersion, len(doc.Classifiers), len(doc.Traits), templates, rules, doc.Calibration.Status)
```

- [ ] **Step 4: Run; expect PASS** (11 tests in the package).

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/cmd/facearch-profile/main.go apps/core-engine/cmd/facearch-profile/main_test.go apps/core-engine/cmd/facearch-profile/seed_test.go
git commit -m "feat(core-engine): facearch-profile reports guidance template and rule counts"
```

---

### Task 9: Seed guidance

**Files:**
- Modify: `apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json` (18 appended `calibration.sources` entries; a new `guidance` member)
- Modify: `apps/core-engine/cmd/facearch-profile/seed_test.go` (`TestValidateSeed` expectation)
- Test: `apps/core-engine/cmd/facearch-profile/seed_guidance_test.go`

**Interfaces:**
- Consumes: Tasks 1-8; Plan 2's `loadSeed` (in `seed_test.go`) and the synthetic measurements of `TestSeedClassifiesAnInFrameFace`.
- Produces: the seed's 11 templates and 7 rules (tables above); nothing for uncovered shapes, blush or brow.

- [ ] **Step 1: Confirm the Open literals** (the tables above) with the user. Do not write the seed guidance before that.

- [ ] **Step 2: Write the failing test.** `TestSeedTemplatesLandWhereTheirNamesSay` checks each template against landmarks on the recorded face, in the canonical frame:
  - the hollow contour lies below the cheekbone line, above the mouth corner and lateral to the nose;
  - the cheekbone highlight lies above the hollow;
  - the forehead contour lies above the brow tail and lateral to the brow head;
  - the jaw contour lies below the nose tip;
  - the forehead highlight lies above the brows;
  - the nose highlight lies on the nose;
  - the chin highlight lies below the lip.

  These are sanity bounds on uncalibrated geometry, not accuracy claims. `TestSeedGuidanceIsSourcedOnly` pins the scope (the roles per shape equal the page's, no rule is a placeholder, every rule's source flags its intensity) and `TestSeedNoGuidanceForUncoveredShapes` pins the empty answer for uncovered shapes.

`apps/core-engine/cmd/facearch-profile/seed_guidance_test.go`:
```go
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

var faceShapes = []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}

// sourcedRoles is what Perfect Corp's page gives per shape (contour and
// highlight for Round, Oval, Square and Heart; Round has no highlight). The
// seed ships only these; every other shape and role gets no guidance.
var sourcedRoles = map[string]map[string]bool{
	"Round":  {"contour": true},
	"Oval":   {"contour": true, "highlight": true},
	"Square": {"contour": true, "highlight": true},
	"Heart":  {"contour": true, "highlight": true},
}

// The seed ships sourced placements only: no rule is a placeholder and no
// cited source says "placeholder" except for the intensities, which every
// rule's source flags. Each rule's text key and templates share one role.
func TestSeedGuidanceIsSourcedOnly(t *testing.T) {
	_, d := loadSeed(t)
	g := d.Guidance
	require.NotNil(t, g)
	sources := map[string]profile.Source{}
	for _, s := range d.Calibration.Sources {
		sources[s.For] = s
	}
	roles := map[string]map[string]bool{}
	for _, r := range g.Rules {
		src, ok := sources["guidance.rules."+r.ID]
		require.True(t, ok, r.ID)
		assert.False(t, r.Placeholder, "rule %s: the seed ships sourced placements only", r.ID)
		assert.False(t, strings.HasPrefix(src.Cite, "placeholder"), r.ID)
		assert.Contains(t, src.Note, "is a placeholder pending benchmark", "rule %s: its intensity is flagged", r.ID)
		require.Len(t, r.TextKeys, 1, r.ID)
		parts := strings.Split(r.TextKeys[0], ".")
		require.Len(t, parts, 4, r.TextKeys[0])
		assert.Equal(t, []string{"facearch", "guidance"}, parts[:2], r.TextKeys[0])
		role := parts[3]
		for _, a := range r.Apply {
			assert.Equal(t, role, g.Templates[a.Template].Role, "rule %s template %s", r.ID, a.Template)
		}
		for _, shape := range r.When["faceShape"] {
			assert.Equal(t, strings.ToLower(shape), parts[2], r.ID)
			if roles[shape] == nil {
				roles[shape] = map[string]bool{}
			}
			roles[shape][role] = true
		}
	}
	assert.Equal(t, sourcedRoles, roles)
	for name, tpl := range g.Templates {
		src, ok := sources["guidance.templates."+name]
		require.True(t, ok, name)
		assert.False(t, strings.HasPrefix(src.Cite, "placeholder"), name)
		assert.Contains(t, []string{profile.RoleContour, profile.RoleHighlight}, tpl.Role, name)
	}
}

// A face classified into a shape the source does not cover gets no
// regions and nothing dropped: honest absence, not a guess. A Round face
// gets contour only.
func TestSeedNoGuidanceForUncoveredShapes(t *testing.T) {
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	m := facearchtest.FixtureFace()
	for _, shape := range faceShapes {
		ev := guidance.Evidence{
			Classifications: map[string]classify.Result{"faceShape": {Status: classify.StatusSingle, Primary: shape}},
			Traits:          map[string]traits.Result{},
		}
		res := guidance.Resolve(d.Guidance, ev, m.Landmarks, m.Regions)
		got := map[string]bool{}
		for _, r := range res.Regions {
			got[r.Role] = true
		}
		for _, x := range res.DroppedTemplates {
			got[d.Guidance.Templates[x.Template].Role] = true
		}
		want := sourcedRoles[shape]
		if want == nil {
			assert.Empty(t, res.Regions, shape)
			assert.Empty(t, res.DroppedTemplates, shape)
			continue
		}
		assert.Equal(t, want, got, shape)
	}
}

// On the recorded face the face shape is insufficient_evidence (chin out of
// frame), so no rule matches: guidance is present and empty, never guessed.
func TestSeedGuidanceOnFixtureFace(t *testing.T) {
	p, d := loadSeed(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	require.NotNil(t, out.Guidance)
	assert.Empty(t, out.Guidance.Regions)
	assert.Empty(t, out.Guidance.DroppedTemplates)
}

// The synthetic in-frame measurements of TestSeedClassifiesAnInFrameFace
// (blend Oval / Round) on the fixture's landmarks, whose jaw region is still
// reported occluded. Oval's rules apply first, then Round's through the
// secondary; jaw-anchored templates are dropped with the reason.
func TestSeedGuidanceOnSyntheticBlend(t *testing.T) {
	p, d := loadSeed(t)
	m := facearchtest.FixtureFace()
	set := map[string]float64{"width_length_ratio": 0.8521, "jaw_to_mid_width_ratio": 0.8326, "chin_taper": 0.4534}
	for i := range m.Measurements {
		if v, ok := set[m.Measurements[i].Key]; ok {
			v := v
			m.Measurements[i].Value = measureclient.Value{Scalar: &v}
			m.Measurements[i].Visibility = measureclient.VisibilityUnknown
			m.Measurements[i].Band = nil
		}
	}
	out := facearch.Interpret(p, d, m)
	require.NotNil(t, out.Guidance)
	fs := out.Classifications["faceShape"]
	require.Equal(t, classify.StatusBlend, fs.Status)
	require.Equal(t, "Oval", fs.Primary)
	require.Equal(t, "Round", fs.Secondary)

	var placed []string
	for _, r := range out.Guidance.Regions {
		placed = append(placed, r.RuleID+":"+r.Template)
		assert.False(t, r.Placeholder, r.RuleID)
	}
	assert.Equal(t, []string{
		"oval-contour:contour_forehead_side_r", "oval-contour:contour_forehead_side_l",
		"oval-contour:contour_cheek_hollow_r", "oval-contour:contour_cheek_hollow_l",
		"oval-highlight:highlight_forehead_centre", "oval-highlight:highlight_nose_bridge",
	}, placed, "Round adds only its jaw contour, and that is dropped")
	assert.Equal(t, []guidance.Dropped{
		{Template: "highlight_chin", RuleID: "oval-highlight", Reason: "region jaw is occluded (anchor landmark 152)"},
		{Template: "contour_jaw_r", RuleID: "round-contour", Reason: "region jaw is occluded (anchor landmark 58)"},
		{Template: "contour_jaw_l", RuleID: "round-contour", Reason: "region jaw is occluded (anchor landmark 288)"},
	}, out.Guidance.DroppedTemplates)
}

// allTemplates is the seed's guidance with one rule applying every
// template, so each can be resolved on the fixture.
func allTemplates(t *testing.T) (*profile.Guidance, guidance.Evidence) {
	t.Helper()
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	rule := profile.GuidanceRule{ID: "all", When: map[string][]string{"faceShape": {"Oval"}}, TextKeys: []string{"k"}}
	for _, name := range d.Guidance.TemplateNames() {
		rule.Apply = append(rule.Apply, profile.Application{Template: name, Intensity: 1})
	}
	g := &profile.Guidance{Templates: d.Guidance.Templates, Rules: []profile.GuidanceRule{rule}}
	ev := guidance.Evidence{
		Classifications: map[string]classify.Result{"faceShape": {Status: classify.StatusSingle, Primary: "Oval"}},
		Traits:          map[string]traits.Result{},
	}
	return g, ev
}

// The fixture's chin is out of frame (jaw "occluded"): exactly the
// templates with a jaw anchor are dropped, and the rest are placed.
func TestSeedJawAnchoredTemplatesDropOnFixture(t *testing.T) {
	g, ev := allTemplates(t)
	m := facearchtest.FixtureFace()
	res := guidance.Resolve(g, ev, m.Landmarks, m.Regions)
	var dropped []string
	for _, x := range res.DroppedTemplates {
		dropped = append(dropped, x.Template)
		assert.Contains(t, x.Reason, "region jaw is occluded")
	}
	assert.Equal(t, []string{"contour_jaw_l", "contour_jaw_r", "highlight_chin"}, dropped)
	assert.Len(t, res.Regions, len(g.Templates)-3)
	for name, tpl := range g.Templates {
		usesJaw := false
		for _, pt := range tpl.Points {
			for _, i := range pt.Anchors() {
				for _, r := range measureclient.RegionsOf(i) {
					usesJaw = usesJaw || r == "jaw"
				}
			}
		}
		assert.Equal(t, usesJaw, contains(dropped, name), name)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Every seed template lands where its name says, on the recorded face, in
// the canonical frame (+x toward the subject's left, +y down, IOD units;
// subject's right = _r = negative x). All regions are treated as visible
// here so the jaw templates can be placed on the landmarker's extrapolated
// chin. These are sanity bounds on uncalibrated geometry, not accuracy.
func TestSeedTemplatesLandWhereTheirNamesSay(t *testing.T) {
	_, d := loadSeed(t)
	require.NotNil(t, d.Guidance)
	l := facearchtest.FixtureFace().Landmarks
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	lm := func(i int) [2]float64 { return f.ToCanonical([2]float64{l[i][0], l[i][1]}) }
	verts := func(name string) [][2]float64 {
		tpl, ok := d.Guidance.Templates[name]
		require.True(t, ok, name)
		poly, err := guidance.Place(tpl, f, l)
		require.NoError(t, err, name)
		out := make([][2]float64, len(poly))
		for i, p := range poly {
			out[i] = f.ToCanonical(p)
		}
		return out
	}
	abs := func(x float64) float64 {
		if x < 0 {
			return -x
		}
		return x
	}
	each := func(name string, ok func(v [2]float64) bool, what string) {
		for i, v := range verts(name) {
			assert.True(t, ok(v), "%s vertex %d at (%.3f, %.3f): %s", name, i, v[0], v[1], what)
		}
	}
	centroidY := func(name string) float64 {
		var y float64
		vs := verts(name)
		for _, v := range vs {
			y += v[1]
		}
		return y / float64(len(vs))
	}

	for _, side := range []struct {
		s                                  string
		sign                               float64
		oval, alar, mouth, browEnd, browIn int
		endo                               int
	}{
		{"r", -1, 234, 98, 61, 70, 107, 133},
		{"l", 1, 454, 327, 291, 300, 336, 362},
	} {
		onSide := func(v [2]float64) bool { return v[0]*side.sign > 0 }

		hollow := "contour_cheek_hollow_" + side.s
		each(hollow, onSide, "on its own side")
		each(hollow, func(v [2]float64) bool { return v[1] > lm(side.oval)[1] }, "below the cheekbone line (the oval extreme's height)")
		each(hollow, func(v [2]float64) bool { return v[1] < lm(side.mouth)[1] }, "above the mouth corner")
		each(hollow, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.alar)[0]) }, "outside the nose (lateral to the alar base)")

		cheekbone := "highlight_cheekbone_" + side.s
		each(cheekbone, onSide, "on its own side")
		each(cheekbone, func(v [2]float64) bool { return v[1] > 0 && v[1] < lm(1)[1] }, "between the eye line and the nose tip")
		each(cheekbone, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.endo)[0]) }, "lateral to the inner eye corner")
		assert.Less(t, centroidY(cheekbone), centroidY(hollow), "the cheekbone highlight sits above the hollow contour")

		forehead := "contour_forehead_side_" + side.s
		each(forehead, onSide, "on its own side")
		each(forehead, func(v [2]float64) bool { return v[1] < lm(side.browEnd)[1] }, "above the brow tail")
		each(forehead, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.browIn)[0]) }, "lateral to the brow head")

		jaw := "contour_jaw_" + side.s
		each(jaw, onSide, "on its own side")
		each(jaw, func(v [2]float64) bool { return v[1] > lm(1)[1] }, "below the nose tip")
		each(jaw, func(v [2]float64) bool { return abs(v[0]) > abs(lm(side.mouth)[0]) }, "lateral to the mouth corner")

	}

	each("highlight_forehead_centre", func(v [2]float64) bool {
		return v[1] < lm(9)[1] && abs(v[0]) < abs(lm(107)[0]) && abs(v[0]) < abs(lm(336)[0])
	}, "above the brows, between the brow heads")
	each("highlight_nose_bridge", func(v [2]float64) bool {
		return v[1] >= lm(168)[1]-1e-9 && v[1] < lm(1)[1] && abs(v[0]) < abs(lm(98)[0])
	}, "on the nose between 168 and the tip, inside the alar width")
	each("highlight_chin", func(v [2]float64) bool {
		return v[1] > lm(17)[1] && abs(v[0]) < abs(lm(61)[0])
	}, "below the lower lip, inside the mouth width")
}
```

In `seed_test.go` (`TestValidateSeed`), replace `(1 classifiers, 10 traits, 0 guidance templates, 0 guidance rules, calibration uncalibrated)` with `(1 classifiers, 10 traits, 11 guidance templates, 7 guidance rules, calibration uncalibrated)`.

- [ ] **Step 3: Run; expect FAIL.** `TestValidateSeed` fails because the counts are 0. The six new tests fail at `require.NotNil` because the seed has no guidance yet.

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 4: Write the seed guidance.** In `seed/seagull-default-uncalibrated.json`:

(a) In `calibration.sources`, add a comma after the last entry (the one with `"for": "traits.noseLength.rules"`) and append these 18 entries before the closing `]`:
```json
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (perfectcorp.com/business/blog/makeup/ai-face-shape-finder-recommend-contour-for-different-face-shapes, read 2026-09-29): contour on the sides of the forehead (Round, Oval, Square, Heart)", "for": "guidance.templates.contour_forehead_side_r", "note": "Placement words sourced; geometry ours, uncalibrated: the outline from mesh 103 through 54 to 21, closed by an inner edge at lerp t 0.35 toward the brow (105) and 0.3 / 0.25 toward the forehead midline (151). No source gives a width."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): contour on the sides of the forehead (Round, Oval, Square, Heart)", "for": "guidance.templates.contour_forehead_side_l", "note": "Mirror of contour_forehead_side_r (332, 284, 251; brow 334; midline 151), same t values; geometry ours, uncalibrated."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): contour beneath / under the cheekbones (Round, Oval, Heart; 'contour diagonally under the cheekbone')", "for": "guidance.templates.contour_cheek_hollow_r", "note": "Placement words sourced; geometry ours, uncalibrated: a tapered band from the outline just below the oval extreme (234, a face-width proxy, not zygion) diagonally toward the mouth corner (61), upper edge lerp t 0.1 to 0.5 on 234-61, lower edge t 0.05 to 0.35 on 93-61."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): contour beneath / under the cheekbones (Round, Oval, Heart)", "for": "guidance.templates.contour_cheek_hollow_l", "note": "Mirror of contour_cheek_hollow_r (454, 323, 291), same t values; geometry ours, uncalibrated."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): contour along the jawline (Round, Square)", "for": "guidance.templates.contour_jaw_r", "note": "Placement words sourced; geometry ours, uncalibrated: the jaw outline 58-172-150 and the same points lerped t 0.15 toward the nose tip (1). The mesh has no gonion; 172 is a jaw-width proxy."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): contour along the jawline (Round, Square)", "for": "guidance.templates.contour_jaw_l", "note": "Mirror of contour_jaw_r (288, 397, 379), same t; geometry ours, uncalibrated."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): highlight the center of the forehead (Oval, Square, Heart)", "for": "guidance.templates.highlight_forehead_centre", "note": "Placement words sourced; geometry ours, uncalibrated: a diamond on the forehead midline, top lerp t 0.2 from mesh top (10, not the hairline) toward 151, sides t 0.35 from 151 toward 338 / 109, bottom t 0.7 from 151 toward 9 (above the brow band)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): highlight the nose (Oval) / nose bridge (its general highlighting list)", "for": "guidance.templates.highlight_nose_bridge", "note": "Placement words sourced; geometry ours, uncalibrated: a strip from 168 to lerp t 0.7 from 6 toward the nose tip (1), 0.03 IOD either side of the landmarks."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): highlight the chin (Oval, Heart)", "for": "guidance.templates.highlight_chin", "note": "Placement words sourced; geometry ours, uncalibrated: a diamond below the lower lip (17) toward menton (152), top t 0.3, bottom t 0.75, sides t 0.35 from 377 / 148 toward 17."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): highlight the cheekbones (Square)", "for": "guidance.templates.highlight_cheekbone_r", "note": "Placement words sourced; geometry ours, uncalibrated: a triangle above the cheek hollow, vertices lerped toward the nose tip (1) from 127 (t 0.12), from the outer eye corner 33 (t 0.3) and from 234 (t 0.2)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29): highlight the cheekbones (Square)", "for": "guidance.templates.highlight_cheekbone_l", "note": "Mirror of highlight_cheekbone_r (356, 263, 454), same t; geometry ours, uncalibrated."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Round Face", "for": "guidance.rules.round-contour", "note": "Placements sourced (sides of the forehead, jawline, beneath the cheekbones). Intensity 0.6 is a placeholder pending benchmark (no source; the page gives shade depth, 1-2 tones darker, not an amount). The page names no highlight for Round, so the seed gives Round none."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Oval Face", "for": "guidance.rules.oval-contour", "note": "Placements sourced (sides of the forehead, under the cheekbones). Intensity 0.6 is a placeholder pending benchmark (no source)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Oval Face", "for": "guidance.rules.oval-highlight", "note": "Placements sourced (center of the forehead, nose, chin). Intensity 0.5 is a placeholder pending benchmark (no source)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Square Face", "for": "guidance.rules.square-contour", "note": "Placements sourced (jawline, sides of the forehead). Intensity 0.6 is a placeholder pending benchmark (no source)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Square Face", "for": "guidance.rules.square-highlight", "note": "Placements sourced (center of the forehead, cheekbones). Intensity 0.5 is a placeholder pending benchmark (no source)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Heart-Shaped Face", "for": "guidance.rules.heart-contour", "note": "Placements sourced (sides of the forehead, beneath the cheekbones). Intensity 0.6 is a placeholder pending benchmark (no source)."},
      {"cite": "Perfect Corp, AI Face Shape Analyzer for Contour & Makeup (read 2026-09-29), Contour for Heart-Shaped Face", "for": "guidance.rules.heart-highlight", "note": "Placements sourced (center of the forehead, chin). Intensity 0.5 is a placeholder pending benchmark (no source)."}
```

(b) After the `traits` array's closing `]`, add a comma and this member, so the document ends `... ], "guidance": { ... } }`:
```json
  "guidance": {
    "templates": {
      "contour_forehead_side_r": {"role": "contour", "points": [
        {"index": 103}, {"index": 54}, {"index": 21},
        {"lerp": [21, 105, 0.35]}, {"lerp": [54, 151, 0.3]}, {"lerp": [103, 151, 0.25]}]},
      "contour_forehead_side_l": {"role": "contour", "points": [
        {"index": 332}, {"index": 284}, {"index": 251},
        {"lerp": [251, 334, 0.35]}, {"lerp": [284, 151, 0.3]}, {"lerp": [332, 151, 0.25]}]},
      "contour_cheek_hollow_r": {"role": "contour", "points": [
        {"lerp": [234, 61, 0.1]}, {"lerp": [234, 61, 0.5]}, {"lerp": [93, 61, 0.35]}, {"lerp": [93, 61, 0.05]}]},
      "contour_cheek_hollow_l": {"role": "contour", "points": [
        {"lerp": [454, 291, 0.1]}, {"lerp": [454, 291, 0.5]}, {"lerp": [323, 291, 0.35]}, {"lerp": [323, 291, 0.05]}]},
      "contour_jaw_r": {"role": "contour", "points": [
        {"index": 58}, {"index": 172}, {"index": 150},
        {"lerp": [150, 1, 0.15]}, {"lerp": [172, 1, 0.15]}, {"lerp": [58, 1, 0.15]}]},
      "contour_jaw_l": {"role": "contour", "points": [
        {"index": 288}, {"index": 397}, {"index": 379},
        {"lerp": [379, 1, 0.15]}, {"lerp": [397, 1, 0.15]}, {"lerp": [288, 1, 0.15]}]},
      "highlight_forehead_centre": {"role": "highlight", "points": [
        {"lerp": [10, 151, 0.2]}, {"lerp": [151, 338, 0.35]}, {"lerp": [151, 9, 0.7]}, {"lerp": [151, 109, 0.35]}]},
      "highlight_nose_bridge": {"role": "highlight", "points": [
        {"index": 168, "offsetIod": [-0.03, 0]}, {"index": 168, "offsetIod": [0.03, 0]},
        {"lerp": [6, 1, 0.7], "offsetIod": [0.03, 0]}, {"lerp": [6, 1, 0.7], "offsetIod": [-0.03, 0]}]},
      "highlight_chin": {"role": "highlight", "points": [
        {"lerp": [17, 152, 0.3]}, {"lerp": [377, 17, 0.35]}, {"lerp": [17, 152, 0.75]}, {"lerp": [148, 17, 0.35]}]},
      "highlight_cheekbone_r": {"role": "highlight", "points": [
        {"lerp": [127, 1, 0.12]}, {"lerp": [33, 1, 0.3]}, {"lerp": [234, 1, 0.2]}]},
      "highlight_cheekbone_l": {"role": "highlight", "points": [
        {"lerp": [356, 1, 0.12]}, {"lerp": [263, 1, 0.3]}, {"lerp": [454, 1, 0.2]}]}
    },
    "rules": [
      {"id": "round-contour", "when": {"faceShape": ["Round"]}, "textKeys": ["facearch.guidance.round.contour"], "apply": [
        {"template": "contour_forehead_side_r", "intensity": 0.6}, {"template": "contour_forehead_side_l", "intensity": 0.6},
        {"template": "contour_jaw_r", "intensity": 0.6}, {"template": "contour_jaw_l", "intensity": 0.6},
        {"template": "contour_cheek_hollow_r", "intensity": 0.6}, {"template": "contour_cheek_hollow_l", "intensity": 0.6}]},
      {"id": "oval-contour", "when": {"faceShape": ["Oval"]}, "textKeys": ["facearch.guidance.oval.contour"], "apply": [
        {"template": "contour_forehead_side_r", "intensity": 0.6}, {"template": "contour_forehead_side_l", "intensity": 0.6},
        {"template": "contour_cheek_hollow_r", "intensity": 0.6}, {"template": "contour_cheek_hollow_l", "intensity": 0.6}]},
      {"id": "oval-highlight", "when": {"faceShape": ["Oval"]}, "textKeys": ["facearch.guidance.oval.highlight"], "apply": [
        {"template": "highlight_forehead_centre", "intensity": 0.5}, {"template": "highlight_nose_bridge", "intensity": 0.5},
        {"template": "highlight_chin", "intensity": 0.5}]},
      {"id": "square-contour", "when": {"faceShape": ["Square"]}, "textKeys": ["facearch.guidance.square.contour"], "apply": [
        {"template": "contour_jaw_r", "intensity": 0.6}, {"template": "contour_jaw_l", "intensity": 0.6},
        {"template": "contour_forehead_side_r", "intensity": 0.6}, {"template": "contour_forehead_side_l", "intensity": 0.6}]},
      {"id": "square-highlight", "when": {"faceShape": ["Square"]}, "textKeys": ["facearch.guidance.square.highlight"], "apply": [
        {"template": "highlight_forehead_centre", "intensity": 0.5},
        {"template": "highlight_cheekbone_r", "intensity": 0.5}, {"template": "highlight_cheekbone_l", "intensity": 0.5}]},
      {"id": "heart-contour", "when": {"faceShape": ["Heart"]}, "textKeys": ["facearch.guidance.heart.contour"], "apply": [
        {"template": "contour_forehead_side_r", "intensity": 0.6}, {"template": "contour_forehead_side_l", "intensity": 0.6},
        {"template": "contour_cheek_hollow_r", "intensity": 0.6}, {"template": "contour_cheek_hollow_l", "intensity": 0.6}]},
      {"id": "heart-highlight", "when": {"faceShape": ["Heart"]}, "textKeys": ["facearch.guidance.heart.highlight"], "apply": [
        {"template": "highlight_forehead_centre", "intensity": 0.5}, {"template": "highlight_chin", "intensity": 0.5}]}
    ]
  }
```

- [ ] **Step 5: Run; expect PASS** (6 new tests; 17 in the package). Plan 2's seed tests are unchanged, because guidance does not affect classifications or traits.

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 6: Commit**
```bash
git add apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json apps/core-engine/cmd/facearch-profile/seed_test.go apps/core-engine/cmd/facearch-profile/seed_guidance_test.go
git commit -m "feat(core-engine): sourced contour and highlight guidance in the uncalibrated seed profile"
```

---

### Task 10: Documentation, full suite, release the lane

**Files:**
- Modify: `docs/FACE-ARCHITECTURE.md`
- Modify: `docs/LANES.md` (your row only; not committed)

- [ ] **Step 1: Update `docs/FACE-ARCHITECTURE.md`** with exactly these edits.

(a) In the intro, the text `producing a face-shape classification and per-feature traits.` wraps across lines 5-6. Replace it with `producing a face-shape classification, per-feature traits and makeup-placement polygons (guidance).`

(b) In the bullet list under it, replace the `- Plan, including ...` bullet and the `- Code: ...` bullet with:
```markdown
- Plans, including the **Seed profile literals** tables (every number in the seed,
  with its source status): `docs/superpowers/plans/2026-09-29-face-architecture-core.md`
  (classifier and traits) and `docs/superpowers/plans/2026-09-29-face-architecture-guidance.md`
  (guidance: **Seed guidance literals**).
- Code: `apps/core-engine/internal/facearch/` (handler, `Interpret`, `classify/`,
  `traits/`, `guidance/`, `profile/`, `measureclient/`) and `apps/core-engine/cmd/facearch-profile/`.
```

(c) In "What it does not do", replace the bullet beginning `- **No guidance polygons.**` with:
```markdown
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
```

(d) In "Request", the text `core-engine always asks the worker for landmarks; they are not returned yet.` wraps across lines 34-35. Replace it with `core-engine always asks the worker for landmarks; it uses them to place guidance polygons and does not return them.`

(e) In the Response table, replace the `guidance` row with:
```markdown
| `guidance` | `null` when the profile has no `guidance` section; otherwise `regions` and `droppedTemplates` (see Guidance). |
```

(f) After the "Catalogue drift" section (before "## Errors"), insert:
````markdown
### Guidance

```json
{"regions": [{"role": "contour", "template": "contour_cheek_hollow_r",
              "polygon": [[204.2, 330.2], [262.5, 411.1], [242.3, 413.2], [199.3, 367.4]],
              "intensity": 0.6, "ruleId": "oval-contour",
              "textKeys": ["facearch.guidance.oval.contour"]}],
 "droppedTemplates": [{"template": "highlight_chin", "ruleId": "oval-highlight",
                       "reason": "region jaw is occluded (anchor landmark 152)"}]}
```

The polygon is the seed's template on the CC0 test photo, rounded here to one decimal;
responses are unrounded. Both lists are always present. A face on which no rule matches
gets two empty lists: nothing is guessed. For example, `faceShape` is
`insufficient_evidence` on a photo with the chin out of frame.

- `polygon`: vertices in the uploaded image's pixels, in template order, unrounded and
  **not clipped** to the image.
- `role`: `contour`, `highlight`, `blush` or `brow`.
- `intensity`: the rule's rendering strength in (0, 1]; not a measurement.
- `textKeys`: i18n keys; the brand's content owns the words.
- `placeholder: true`: the rule's choice of this placement has no source (omitted when
  false). The whole profile's `provenance.calibration.status` still applies.

**Templates.** Each point is a landmark `{"index": i}` or `{"lerp": [a, b, t]}`
(a + t(b − a)). A point may be shifted by `offsetIod: [dx, dy]` in the ai-worker's
canonical frame:

- origin at the midpoint of the eye centres (an eye centre is the midpoint of that
  eye's two corners);
- eye line horizontal;
- units of eye-centre distance;
- +x toward the subject's left (image right), +y down.

core-engine rebuilds that frame from the returned landmarks exactly as the worker does,
resolves each point there and maps it back to pixels.

**Rules.** `when` maps a classifier or trait key to labels. Every entry must match.

- A classifier matches on its `primary`, or on its `secondary` when the result is a
  `blend`.
- A trait matches on its `label`, not on `alternative`.
- `insufficient_evidence`, `not_assessable` and `unavailable` never match.

Rules matched on a primary apply first, then rules matched only through a secondary,
each group in document order. Each template is placed once, by the first rule that
applies it.

**Dropped templates.** A template that a matching rule applied is dropped, with a
`reason`, in these cases:

| reason | when |
|---|---|
| `canonical frame requires region eyes` | The eyes region is occluded (the frame is built from the eye corners). |
| `region <r> is occluded (anchor landmark <i>)` | A landmark the template is built from (an index, or either end of a lerp) is in a region the worker reports `occluded`. That includes regions **out of frame**, which the worker does not distinguish in `regions`. |
| `region <r> not reported by the ai-worker (anchor landmark <i>)` | The worker's `regions` lacks that region. |
| `the ai-worker returned no landmarks`, `landmarks unusable: ...` | Landmarks are missing or malformed. |

**Regions of a landmark.** core-engine keeps one copy of the worker's region table
(`anchors.REGIONS`, with the eye corners and the landmark count) in
`internal/facearch/measureclient/anchors.go`. A test pins it to a recording of
`apps/ai-worker/ai_worker/vision/facegeom/anchors.py` and to that file's SHA-256. Any
change to `anchors.py` fails core-engine's tests until the recording is regenerated
(the script and command are in the guidance plan, Task 1).

**Seed guidance (uncalibrated).** Perfect Corp's page covers four shapes, and only
contour and highlight.

- Seeded (placements sourced, geometry ours): contour for Round, Oval, Square and
  Heart; highlight for Oval, Square and Heart. 11 templates, 7 rules.
- **Not seeded:** any guidance for Diamond, Oblong, Triangle or InvTriangle (the page
  does not cover them), a Round highlight (the page names none), and any blush or brow
  guidance (the page gives no placement). A face classified into one of those shapes
  gets `{"regions": [], "droppedTemplates": []}`; `guidance` is `null` only when a
  profile has no guidance section. The engine supports blush and brow roles and
  `placeholder: true` rules for profiles that add them (a brand's makeup team, or a
  calibrated profile).
- No seed rule is `placeholder: true`: its placements are sourced. Its intensities
  (contour 0.6, highlight 0.5) are placeholders, stated in each rule's source note and
  covered by `calibration.status: uncalibrated`. A rule is flagged `placeholder` only
  when the choice of placement has no source, not for its intensity.
````

(g) In "CLI", replace the printed line `` `<file>: valid against <catalogue> (N classifiers, N traits, calibration <status>)`. `` with `` `<file>: valid against <catalogue> (N classifiers, N traits, N guidance templates, N guidance rules, calibration <status>)`. ``

(h) In "Document schema", replace the line `guidance   (opaque, reserved for Plan 3; carried unchanged, never read)` with:
```
guidance { templates { <name>: { role, points [{index} | {lerp [a, b, t]}, each with optional offsetIod [dx, dy]] } },
           rules [{ id, when { <classifier|trait key>: [label] }, apply [{template, intensity}], textKeys [key], placeholder }] }
           (optional; absent = "guidance": null)
```
Before the "Source coverage" bullet, add:
```markdown
- **Guidance templates:** `role` is one of `contour | highlight | blush | brow`; at
  least 3 points; each point is exactly one of `index` / `lerp`; landmark indices are
  0-477, each in at least one worker region (a landmark in no region has no visibility,
  so its occlusion could not be detected: lerp between region landmarks instead); lerp
  ends differ and `t` is in [0, 1]; offsets are finite.
- **Guidance rules:** unique `id`; `when` is non-empty, each key names exactly one
  classifier or trait of the document, and each label is one it can produce; at least
  one application of a defined template, none twice, `intensity` in (0, 1]; at least
  one text key, with no whitespace.
```
Then replace the whole "Source coverage" bullet with:
```markdown
- **Source coverage (uncalibrated only):** a source covers path P when its `for` is P
  or a path under P. Every classifier's `blendMargin` and `minObservedWeight`, every
  class (`classifiers.<key>.classes.<label>`), every trait (`traits.<key>`), every
  guidance template (`guidance.templates.<name>`) and every guidance rule
  (`guidance.rules.<id>`) must be covered. A `calibrated` profile is not subject to
  this rule.
```

- [ ] **Step 2: Full suite.**

Run: `go test ./apps/core-engine/... && go vet ./apps/core-engine/internal/facearch/... ./apps/core-engine/cmd/facearch-profile/ ./apps/core-engine/cmd/server/ && gofmt -l apps/core-engine/internal/facearch apps/core-engine/cmd/facearch-profile`
Expected: all `ok`; vet and gofmt silent.

- [ ] **Step 3: Remove your row from `docs/LANES.md` Active claims.**

- [ ] **Step 4: Commit**
```bash
git add docs/FACE-ARCHITECTURE.md
git commit -m "docs: face-architecture guidance semantics and uncalibrated seed geometry"
```
(`docs/LANES.md` is shared coordination state; leave it uncommitted unless the user asks.)

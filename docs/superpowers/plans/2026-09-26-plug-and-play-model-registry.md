# Plug-and-play model registry — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let any ONNX vision model be uploaded, kept in a library, and switched
into service per capability from the dashboard, with no code change and no loss
of the model it replaces.

**Architecture:** Each uploaded model carries a declared runtime spec (how to
preprocess, how to read its output). The worker holds one adapter per output
kind and selects it at run time. `vision_models` becomes a library of artifacts;
a `vision_capability_bindings` table records which model currently serves each
capability. Dispatch resolves capabilities through bindings, groups by model,
and runs each model once per zone batch.

**Tech Stack:** Python 3.10+, FastAPI, Pydantic v2, onnxruntime, asyncpg,
numpy, OpenCV, pytest. Go 1.25 for the core-engine side.

**Spec:** `docs/superpowers/specs/2026-09-26-plug-and-play-model-registry-design.md`

## Global Constraints

- **Severity currency:** adapters always return severity in `0.0 .. 100.0`,
  100 = worst. Defined once as `SEVERITY_SCALE = 100.0` in
  `app/models/spec.py`; never re-stated at a call site.
- **No hardcoded policy:** every threshold, class index, label, input size and
  normalization value lives in the model's row. Limits (`MODEL_MAX_FILE_BYTES`
  = 200 MB, `MODEL_MAX_SMOKE_LATENCY_MS` = 400, `MODEL_CACHE_MAX_BYTES` = 1 GB,
  `MODEL_LIBRARY_WARN_BYTES` = 10 GB) are env config read through
  `app/config.py`, with those defaults.
- **No fabricated measurements:** a model that fails, is missing, or is unbound
  yields an *absent* capability plus a warning. Never a substituted number.
- **Shared database:** `vision_models` lives in the Postgres shared with
  Seagull-gateway. Schema changes stay in `app/db.py` raw SQL, `IF NOT EXISTS`,
  additive only. Read `docs/SHARED-DATABASE.md` before Task 5.
- **Lane discipline:** this plan's work is the `vision-ai` lane plus, for
  Task 10, `core-vision`. Claim both in `docs/LANES.md` before starting.
- **Commit style:** `feat:` / `test:` / `docs:` prefix, present tense. Do not
  `git push` — the user pushes.

---

### Task 1: Runtime spec parsing

The spec object every later task consumes. Strict: an unknown field is an
error, because a typo in `scoreThreshold` silently taking a default is the
failure mode this whole design exists to prevent.

**Files:**
- Create: `apps/vision-ai-worker/app/models/spec.py`
- Test: `apps/vision-ai-worker/tests/test_runtime_spec.py`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `SEVERITY_SCALE: float` (= 100.0)
  - `class InputSpec` — fields `width: int`, `height: int`,
    `colorFormat: Literal["RGB","BGR","GRAY"]`, `layout: Literal["NCHW","NHWC"]`,
    `netScaleFactor: float`, `offsets: list[float]`, `embedded: bool`
  - `class ClassMapping` — `index: int`, `capability: str`
  - Output models: `SeverityScalarOutput` (`kind`, `capability`),
    `MulticlassSoftmaxOutput` (`kind`, `classes`, `temperature`),
    `MultilabelSigmoidOutput` (`kind`, `classes`),
    `DetectionCountOutput` (`kind`, `capability`, `scoreThreshold`,
    `fullScaleCount`, `tensorOrder`),
    `MaskAreaRatioOutput` (`kind`, `classes`, `minPixelRatio`, `tensorOrder`)
  - `class RuntimeSpec` — `specVersion: int`, `input: InputSpec`, `output: <union>`
  - `parse_runtime_spec(data: dict | None) -> RuntimeSpec` — `None` returns
    `LEGACY_SPEC`; raises `SpecError` with a message naming the bad field
  - `class SpecError(ValueError)`
  - `LEGACY_SPEC: RuntimeSpec` — 224x224 RGB NCHW, `netScaleFactor` 1/255,
    zero offsets, `severity_scalar`, capability filled by the caller via
    `spec.with_capability(code)`
  - `capabilities_of(spec: RuntimeSpec) -> list[str]`
  - `kind_catalog() -> list[dict]` — field descriptors for `GET /kinds`

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_runtime_spec.py
import pytest

from app.models.spec import (
    LEGACY_SPEC,
    SEVERITY_SCALE,
    SpecError,
    capabilities_of,
    kind_catalog,
    parse_runtime_spec,
)


def _input(**over):
    base = {
        "width": 384,
        "height": 384,
        "colorFormat": "RGB",
        "layout": "NCHW",
        "netScaleFactor": 0.017125,
        "offsets": [123.675, 116.28, 103.53],
    }
    base.update(over)
    return base


def test_legacy_spec_is_todays_contract():
    assert LEGACY_SPEC.input.width == 224
    assert LEGACY_SPEC.input.height == 224
    assert LEGACY_SPEC.input.colorFormat == "RGB"
    assert LEGACY_SPEC.input.layout == "NCHW"
    assert LEGACY_SPEC.input.netScaleFactor == pytest.approx(1 / 255)
    assert LEGACY_SPEC.input.offsets == [0.0, 0.0, 0.0]
    assert LEGACY_SPEC.output.kind == "severity_scalar"


def test_none_parses_to_legacy_spec():
    assert parse_runtime_spec(None) == LEGACY_SPEC


def test_severity_scalar_round_trips():
    spec = parse_runtime_spec(
        {"specVersion": 1, "input": _input(), "output": {"kind": "severity_scalar", "capability": "acne"}}
    )
    assert spec.input.width == 384
    assert capabilities_of(spec) == ["acne"]


def test_multilabel_lists_every_mapped_capability():
    spec = parse_runtime_spec(
        {
            "specVersion": 1,
            "input": _input(),
            "output": {
                "kind": "multilabel_sigmoid",
                "classes": [
                    {"index": 0, "capability": "acne"},
                    {"index": 2, "capability": "redness"},
                ],
            },
        }
    )
    assert capabilities_of(spec) == ["acne", "redness"]


def test_unknown_kind_is_rejected_by_name():
    with pytest.raises(SpecError, match="banana"):
        parse_runtime_spec({"specVersion": 1, "input": _input(), "output": {"kind": "banana"}})


def test_unknown_field_is_rejected_not_ignored():
    with pytest.raises(SpecError, match="scoreThreshhold"):
        parse_runtime_spec(
            {
                "specVersion": 1,
                "input": _input(),
                "output": {
                    "kind": "detection_count",
                    "capability": "acne",
                    "scoreThreshhold": 0.5,
                    "fullScaleCount": 25,
                },
            }
        )


def test_embedded_input_rejects_preprocessing_fields():
    with pytest.raises(SpecError, match="embedded"):
        parse_runtime_spec(
            {
                "specVersion": 1,
                "input": _input(embedded=True),
                "output": {"kind": "severity_scalar", "capability": "acne"},
            }
        )


def test_duplicate_capability_within_one_spec_is_rejected():
    with pytest.raises(SpecError, match="acne"):
        parse_runtime_spec(
            {
                "specVersion": 1,
                "input": _input(),
                "output": {
                    "kind": "multilabel_sigmoid",
                    "classes": [
                        {"index": 0, "capability": "acne"},
                        {"index": 1, "capability": "acne"},
                    ],
                },
            }
        )


def test_kind_catalog_describes_every_kind():
    kinds = {k["kind"] for k in kind_catalog()}
    assert kinds == {
        "severity_scalar",
        "multiclass_softmax",
        "multilabel_sigmoid",
        "detection_count",
        "mask_area_ratio",
    }
    for entry in kind_catalog():
        assert entry["fields"], f"{entry['kind']} must describe its fields"


def test_severity_scale_is_one_hundred():
    assert SEVERITY_SCALE == 100.0
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_runtime_spec.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'app.models.spec'`

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/models/spec.py
"""The declared runtime spec: how to feed a model and how to read its output.

Stored per model in vision_models.runtime_spec. Field names follow DeepStream's
nvinfer vocabulary (net-scale-factor, offsets, model-color-format) so they mean
what a vision engineer already expects.
"""

from typing import Annotated, Literal, Optional, Union

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator

# The contract with core-engine: severity runs 0..100, 100 = worst. Not a
# tunable -- it is the units both services agree on.
SEVERITY_SCALE = 100.0


class SpecError(ValueError):
    """A runtime spec that cannot be used, with the offending field named."""


class _Strict(BaseModel):
    model_config = ConfigDict(extra="forbid")


class InputSpec(_Strict):
    width: int = Field(gt=0)
    height: int = Field(gt=0)
    colorFormat: Literal["RGB", "BGR", "GRAY"] = "RGB"
    layout: Literal["NCHW", "NHWC"] = "NCHW"
    netScaleFactor: float = 1 / 255
    offsets: list[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0])
    embedded: bool = False


class ClassMapping(_Strict):
    index: int = Field(ge=0)
    capability: str = Field(min_length=1)


class TensorOrder(_Strict):
    """Which output index carries what. ONNX does not standardize this."""

    scores: Optional[int] = None
    boxes: Optional[int] = None
    classes: Optional[int] = None
    mask: Optional[int] = None


class SeverityScalarOutput(_Strict):
    kind: Literal["severity_scalar"]
    capability: str = Field(min_length=1)


class MulticlassSoftmaxOutput(_Strict):
    kind: Literal["multiclass_softmax"]
    classes: list[ClassMapping] = Field(min_length=1)
    temperature: float = Field(default=1.0, gt=0)


class MultilabelSigmoidOutput(_Strict):
    kind: Literal["multilabel_sigmoid"]
    classes: list[ClassMapping] = Field(min_length=1)


class DetectionCountOutput(_Strict):
    kind: Literal["detection_count"]
    capability: str = Field(min_length=1)
    scoreThreshold: float = Field(ge=0.0, le=1.0)
    fullScaleCount: int = Field(gt=0)
    tensorOrder: TensorOrder = Field(default_factory=TensorOrder)


class MaskAreaRatioOutput(_Strict):
    kind: Literal["mask_area_ratio"]
    classes: list[ClassMapping] = Field(min_length=1)
    minPixelRatio: float = Field(default=0.0, ge=0.0, le=1.0)
    tensorOrder: TensorOrder = Field(default_factory=TensorOrder)


OutputSpec = Annotated[
    Union[
        SeverityScalarOutput,
        MulticlassSoftmaxOutput,
        MultilabelSigmoidOutput,
        DetectionCountOutput,
        MaskAreaRatioOutput,
    ],
    Field(discriminator="kind"),
]

KNOWN_KINDS = [
    "severity_scalar",
    "multiclass_softmax",
    "multilabel_sigmoid",
    "detection_count",
    "mask_area_ratio",
]


class RuntimeSpec(_Strict):
    specVersion: Literal[1] = 1
    input: InputSpec
    output: OutputSpec

    @model_validator(mode="after")
    def _check(self) -> "RuntimeSpec":
        if self.input.embedded:
            explicit = self.model_fields_set_of_input()
            if explicit:
                raise ValueError(
                    "input.embedded is true, so preprocessing fields must be omitted: "
                    + ", ".join(sorted(explicit))
                )
        caps = capabilities_of(self)
        duplicates = {c for c in caps if caps.count(c) > 1}
        if duplicates:
            raise ValueError(
                "one spec declares the same capability twice: " + ", ".join(sorted(duplicates))
            )
        return self

    def model_fields_set_of_input(self) -> set[str]:
        preprocessing = {"width", "height", "colorFormat", "layout", "netScaleFactor", "offsets"}
        return preprocessing & self.input.model_fields_set

    def with_capability(self, capability: str) -> "RuntimeSpec":
        """LEGACY_SPEC carries no capability; rows supply theirs."""
        return self.model_copy(update={"output": SeverityScalarOutput(kind="severity_scalar", capability=capability)})


def capabilities_of(spec: RuntimeSpec) -> list[str]:
    out = spec.output
    if isinstance(out, (SeverityScalarOutput, DetectionCountOutput)):
        return [out.capability]
    return [c.capability for c in out.classes]


LEGACY_SPEC = RuntimeSpec(
    specVersion=1,
    input=InputSpec(width=224, height=224, colorFormat="RGB", layout="NCHW", netScaleFactor=1 / 255, offsets=[0.0, 0.0, 0.0]),
    output=SeverityScalarOutput(kind="severity_scalar", capability="__unbound__"),
)


def parse_runtime_spec(data: Optional[dict]) -> RuntimeSpec:
    if data is None:
        return LEGACY_SPEC
    kind = (data.get("output") or {}).get("kind")
    if kind is not None and kind not in KNOWN_KINDS:
        raise SpecError(f"unknown output kind '{kind}'. Known kinds: {', '.join(KNOWN_KINDS)}")
    try:
        return RuntimeSpec.model_validate(data)
    except ValidationError as exc:
        raise SpecError(_first_problem(exc)) from exc


def _first_problem(exc: ValidationError) -> str:
    err = exc.errors()[0]
    location = ".".join(str(p) for p in err["loc"]) or "spec"
    if err["type"] == "extra_forbidden":
        return f"unknown field '{location}'"
    return f"{location}: {err['msg']}"


def kind_catalog() -> list[dict]:
    """Field descriptors the upload form is rendered from, so a new kind needs
    no web release."""
    models = {
        "severity_scalar": SeverityScalarOutput,
        "multiclass_softmax": MulticlassSoftmaxOutput,
        "multilabel_sigmoid": MultilabelSigmoidOutput,
        "detection_count": DetectionCountOutput,
        "mask_area_ratio": MaskAreaRatioOutput,
    }
    catalog = []
    for kind, model in models.items():
        fields = []
        for name, field in model.model_fields.items():
            if name == "kind":
                continue
            fields.append(
                {
                    "name": name,
                    "required": field.is_required(),
                    "default": None if field.is_required() else field.get_default(call_default_factory=True),
                }
            )
        catalog.append({"kind": kind, "fields": fields})
    return catalog
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_runtime_spec.py -v`
Expected: PASS, 9 tests. If `test_embedded_input_rejects_preprocessing_fields`
fails because Pydantic reports `model_fields_set` differently for defaults,
assert on the explicitly-passed keys only — the test's intent is that an
`embedded` spec carrying a width is an error.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/models/spec.py apps/vision-ai-worker/tests/test_runtime_spec.py
git commit -m "feat: declared runtime spec for vision models"
```

---

### Task 2: Output adapters

One adapter per output kind, turning raw tensors into `{capability: severity}`.
This is the only place a model's numbers acquire meaning.

**Files:**
- Create: `apps/vision-ai-worker/app/models/adapters.py`
- Test: `apps/vision-ai-worker/tests/test_adapters.py`

**Interfaces:**
- Consumes: `RuntimeSpec` and its output models from Task 1.
- Produces:
  - `adapt(spec: RuntimeSpec, outputs: list[np.ndarray]) -> dict[str, float]`
  - `class AdapterError(ValueError)`

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_adapters.py
import numpy as np
import pytest

from app.models.adapters import AdapterError, adapt
from app.models.spec import parse_runtime_spec

INPUT = {"width": 224, "height": 224}


def spec(output):
    return parse_runtime_spec({"specVersion": 1, "input": INPUT, "output": output})


def test_severity_scalar_scales_to_0_100():
    s = spec({"kind": "severity_scalar", "capability": "acne"})
    assert adapt(s, [np.array([[0.42]], dtype=np.float32)]) == {"acne": pytest.approx(42.0)}


def test_severity_scalar_clamps_out_of_range():
    s = spec({"kind": "severity_scalar", "capability": "acne"})
    assert adapt(s, [np.array([[1.8]], dtype=np.float32)]) == {"acne": 100.0}
    assert adapt(s, [np.array([[-0.3]], dtype=np.float32)]) == {"acne": 0.0}


def test_multiclass_softmax_uses_probability_per_mapped_class():
    s = spec(
        {
            "kind": "multiclass_softmax",
            "classes": [{"index": 0, "capability": "mild"}, {"index": 2, "capability": "severe"}],
        }
    )
    logits = np.array([[2.0, 0.0, 1.0]], dtype=np.float32)
    result = adapt(s, [logits])
    assert set(result) == {"mild", "severe"}
    assert result["mild"] > result["severe"]
    assert 0.0 <= result["severe"] <= 100.0


def test_multilabel_sigmoid_scores_classes_independently():
    s = spec(
        {
            "kind": "multilabel_sigmoid",
            "classes": [{"index": 0, "capability": "acne"}, {"index": 1, "capability": "redness"}],
        }
    )
    result = adapt(s, [np.array([[0.0, 4.0]], dtype=np.float32)])
    assert result["acne"] == pytest.approx(50.0, abs=0.1)
    assert result["redness"] > 95.0


def test_detection_count_scales_by_full_scale_count():
    s = spec(
        {
            "kind": "detection_count",
            "capability": "acne",
            "scoreThreshold": 0.5,
            "fullScaleCount": 10,
            "tensorOrder": {"scores": 0},
        }
    )
    scores = np.array([[0.9, 0.8, 0.7, 0.4, 0.1]], dtype=np.float32)  # 3 above threshold
    assert adapt(s, [scores]) == {"acne": pytest.approx(30.0)}


def test_detection_count_clamps_above_full_scale():
    s = spec(
        {
            "kind": "detection_count",
            "capability": "acne",
            "scoreThreshold": 0.5,
            "fullScaleCount": 2,
            "tensorOrder": {"scores": 0},
        }
    )
    assert adapt(s, [np.array([[0.9, 0.9, 0.9, 0.9]], dtype=np.float32)]) == {"acne": 100.0}


def test_mask_area_ratio_is_fraction_of_pixels():
    s = spec(
        {
            "kind": "mask_area_ratio",
            "classes": [{"index": 1, "capability": "redness"}],
            "tensorOrder": {"mask": 0},
        }
    )
    mask = np.zeros((1, 10, 10), dtype=np.int64)
    mask[0, :2, :] = 1  # 20 of 100 pixels
    assert adapt(s, [mask]) == {"redness": pytest.approx(20.0)}


def test_mask_area_ratio_below_min_pixel_ratio_reports_zero():
    s = spec(
        {
            "kind": "mask_area_ratio",
            "classes": [{"index": 1, "capability": "redness"}],
            "minPixelRatio": 0.25,
            "tensorOrder": {"mask": 0},
        }
    )
    mask = np.zeros((1, 10, 10), dtype=np.int64)
    mask[0, 0, :] = 1  # 10 of 100 pixels, below the floor
    assert adapt(s, [mask]) == {"redness": 0.0}


def test_class_index_beyond_output_is_an_error():
    s = spec(
        {
            "kind": "multilabel_sigmoid",
            "classes": [{"index": 7, "capability": "acne"}],
        }
    )
    with pytest.raises(AdapterError, match="7"):
        adapt(s, [np.array([[0.1, 0.2]], dtype=np.float32)])
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_adapters.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'app.models.adapters'`

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/models/adapters.py
"""Turns a model's raw output tensors into {capability: severity 0-100}.

Every adapter returns the same currency, so nothing downstream of dispatch
needs to know which kind of model produced a number.
"""

import numpy as np

from app.models.spec import (
    SEVERITY_SCALE,
    DetectionCountOutput,
    MaskAreaRatioOutput,
    MulticlassSoftmaxOutput,
    MultilabelSigmoidOutput,
    RuntimeSpec,
    SeverityScalarOutput,
)


class AdapterError(ValueError):
    """The model's output does not match what its spec declared."""


def _clamp(value: float) -> float:
    return float(min(max(value, 0.0), SEVERITY_SCALE))


def _pick(outputs: list[np.ndarray], index: int | None, what: str) -> np.ndarray:
    idx = 0 if index is None else index
    if idx >= len(outputs):
        raise AdapterError(f"tensorOrder.{what} = {idx} but the model returned {len(outputs)} output(s)")
    return np.asarray(outputs[idx])


def _require_index(flat: np.ndarray, index: int) -> float:
    if index >= flat.size:
        raise AdapterError(f"class index {index} is beyond the model's output of size {flat.size}")
    return float(flat[index])


def adapt(spec: RuntimeSpec, outputs: list[np.ndarray]) -> dict[str, float]:
    out = spec.output

    if isinstance(out, SeverityScalarOutput):
        flat = _pick(outputs, 0, "scores").reshape(-1)
        if flat.size == 0:
            raise AdapterError("model returned an empty output")
        return {out.capability: _clamp(float(flat[0]) * SEVERITY_SCALE)}

    if isinstance(out, MulticlassSoftmaxOutput):
        logits = _pick(outputs, 0, "scores").reshape(-1).astype(np.float64) / out.temperature
        shifted = logits - logits.max()
        probs = np.exp(shifted) / np.exp(shifted).sum()
        return {c.capability: _clamp(_require_index(probs, c.index) * SEVERITY_SCALE) for c in out.classes}

    if isinstance(out, MultilabelSigmoidOutput):
        logits = _pick(outputs, 0, "scores").reshape(-1).astype(np.float64)
        probs = 1.0 / (1.0 + np.exp(-logits))
        return {c.capability: _clamp(_require_index(probs, c.index) * SEVERITY_SCALE) for c in out.classes}

    if isinstance(out, DetectionCountOutput):
        scores = _pick(outputs, out.tensorOrder.scores, "scores").reshape(-1)
        count = int((scores >= out.scoreThreshold).sum())
        return {out.capability: _clamp(count / out.fullScaleCount * SEVERITY_SCALE)}

    if isinstance(out, MaskAreaRatioOutput):
        mask = _pick(outputs, out.tensorOrder.mask, "mask")
        if mask.ndim >= 3 and mask.shape[-3] > 1 and mask.dtype.kind == "f":
            mask = mask.argmax(axis=-3)  # logits per class -> class indices
        total = mask.size
        if total == 0:
            raise AdapterError("model returned an empty mask")
        result = {}
        for c in out.classes:
            ratio = float((mask == c.index).sum()) / total
            result[c.capability] = _clamp(ratio * SEVERITY_SCALE) if ratio >= out.minPixelRatio else 0.0
        return result

    raise AdapterError(f"no adapter for output kind '{out.kind}'")
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_adapters.py -v`
Expected: PASS, 9 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/models/adapters.py apps/vision-ai-worker/tests/test_adapters.py
git commit -m "feat: output adapters for each declared model kind"
```

---

### Task 3: Spec-driven preprocessing

Replaces the hardcoded 224/RGB/0-1 pipeline. Batching lives here too: the
measurements show the per-image loop, not model size, is what exhausts the
20-second dispatch budget.

**Files:**
- Create: `apps/vision-ai-worker/app/models/preprocess.py`
- Test: `apps/vision-ai-worker/tests/test_preprocess.py`

**Interfaces:**
- Consumes: `InputSpec` from Task 1.
- Produces:
  - `preprocess(image_bytes: bytes, spec: InputSpec) -> np.ndarray` — one image,
    leading batch dimension of 1
  - `preprocess_batch(images: list[bytes], spec: InputSpec) -> np.ndarray`
  - `class PreprocessError(ValueError)`

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_preprocess.py
import cv2
import numpy as np
import pytest

from app.models.preprocess import PreprocessError, preprocess, preprocess_batch
from app.models.spec import InputSpec, LEGACY_SPEC


def jpeg(w=300, h=300, colour=(10, 120, 250)) -> bytes:
    img = np.zeros((h, w, 3), dtype=np.uint8)
    img[:, :] = colour  # BGR as OpenCV writes it
    ok, enc = cv2.imencode(".jpg", img)
    assert ok
    return enc.tobytes()


def test_legacy_spec_matches_todays_pipeline():
    tensor = preprocess(jpeg(), LEGACY_SPEC.input)
    assert tensor.shape == (1, 3, 224, 224)
    assert tensor.dtype == np.float32
    assert 0.0 <= tensor.min() and tensor.max() <= 1.0


def test_declared_size_is_honoured():
    tensor = preprocess(jpeg(), InputSpec(width=384, height=256))
    assert tensor.shape == (1, 3, 256, 384)


def test_nhwc_layout():
    tensor = preprocess(jpeg(), InputSpec(width=224, height=224, layout="NHWC"))
    assert tensor.shape == (1, 224, 224, 3)


def test_gray_collapses_to_one_channel():
    tensor = preprocess(jpeg(), InputSpec(width=64, height=64, colorFormat="GRAY"))
    assert tensor.shape == (1, 1, 64, 64)


def test_offsets_and_scale_are_applied():
    spec = InputSpec(width=8, height=8, colorFormat="RGB", netScaleFactor=0.5, offsets=[1.0, 2.0, 3.0])
    tensor = preprocess(jpeg(colour=(0, 0, 0)), spec)  # black: every channel 0
    # y = scale * (x - offset) applied per channel
    assert tensor[0, 0].mean() == pytest.approx(0.5 * (0 - 1.0), abs=1e-3)
    assert tensor[0, 2].mean() == pytest.approx(0.5 * (0 - 3.0), abs=1e-3)


def test_bgr_and_rgb_differ_for_a_coloured_image():
    rgb = preprocess(jpeg(), InputSpec(width=8, height=8, colorFormat="RGB"))
    bgr = preprocess(jpeg(), InputSpec(width=8, height=8, colorFormat="BGR"))
    assert not np.allclose(rgb, bgr)


def test_batch_stacks_images():
    batch = preprocess_batch([jpeg(), jpeg(), jpeg()], InputSpec(width=224, height=224))
    assert batch.shape == (3, 3, 224, 224)


def test_undecodable_bytes_raise():
    with pytest.raises(PreprocessError):
        preprocess(b"not an image", LEGACY_SPEC.input)


def test_embedded_spec_passes_bytes_through_unchanged():
    raw = jpeg()
    out = preprocess(raw, InputSpec(width=1, height=1, embedded=True))
    assert out.dtype == np.uint8
    assert out.tobytes() == raw
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_preprocess.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'app.models.preprocess'`

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/models/preprocess.py
"""Decode and normalize a crop the way a model's InputSpec declares.

y = netScaleFactor * (x - offsets), per channel -- the DeepStream nvinfer
formula the spec's vocabulary is borrowed from.
"""

import cv2
import numpy as np

from app.models.spec import InputSpec


class PreprocessError(ValueError):
    """The image could not be decoded or does not fit the declared spec."""


def _decode(image_bytes: bytes, spec: InputSpec) -> np.ndarray:
    arr = np.frombuffer(image_bytes, np.uint8)
    img_bgr = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img_bgr is None:
        raise PreprocessError("could not decode image")

    if spec.colorFormat == "RGB":
        img = cv2.cvtColor(img_bgr, cv2.COLOR_BGR2RGB)
    elif spec.colorFormat == "GRAY":
        img = cv2.cvtColor(img_bgr, cv2.COLOR_BGR2GRAY)[:, :, None]
    else:
        img = img_bgr

    img = cv2.resize(img, (spec.width, spec.height))
    if img.ndim == 2:
        img = img[:, :, None]
    return img.astype(np.float32)


def _normalize(img: np.ndarray, spec: InputSpec) -> np.ndarray:
    channels = img.shape[2]
    offsets = spec.offsets or [0.0] * channels
    if len(offsets) < channels:
        raise PreprocessError(
            f"input.offsets has {len(offsets)} value(s) but colorFormat {spec.colorFormat} needs {channels}"
        )
    img = img - np.asarray(offsets[:channels], dtype=np.float32)
    return img * np.float32(spec.netScaleFactor)


def preprocess(image_bytes: bytes, spec: InputSpec) -> np.ndarray:
    if spec.embedded:
        # The model carries its own preprocessing; hand it the encoded bytes.
        return np.frombuffer(image_bytes, np.uint8)
    return preprocess_batch([image_bytes], spec)


def preprocess_batch(images: list[bytes], spec: InputSpec) -> np.ndarray:
    if spec.embedded:
        raise PreprocessError("a model with embedded preprocessing cannot be batched here")
    if not images:
        raise PreprocessError("no images to preprocess")

    stacked = np.stack([_normalize(_decode(b, spec), spec) for b in images])  # N,H,W,C
    if spec.layout == "NCHW":
        stacked = np.transpose(stacked, (0, 3, 1, 2))
    return np.ascontiguousarray(stacked, dtype=np.float32)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_preprocess.py -v`
Expected: PASS, 9 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/models/preprocess.py apps/vision-ai-worker/tests/test_preprocess.py
git commit -m "feat: spec-driven preprocessing with batching"
```

---

### Task 4: Session cache keyed by model, with a byte budget

Today's cache is keyed by capability and never evicts. A library of candidates
makes that unbounded in a second dimension, and the measurements put a
segmentation model at 158 MB.

**Files:**
- Modify: `apps/vision-ai-worker/app/models/model_registry.py` (rewrite)
- Modify: `apps/vision-ai-worker/app/config.py` (add the three limits)
- Test: `apps/vision-ai-worker/tests/test_model_registry.py`

**Interfaces:**
- Consumes: Tasks 1-3; `app.storage.download_bytes`.
- Produces:
  - `async get_or_load(model_id: str, s3_key: str) -> ort.InferenceSession`
  - `invalidate(model_id: str) -> None`
  - `cache_bytes() -> int`
  - `load_session_from_bytes(data: bytes) -> ort.InferenceSession` (kept)
  - `async run_model(model_id: str, s3_key: str, spec: RuntimeSpec, crops: list[bytes]) -> list[dict[str, float]]`
    — one `{capability: severity}` per crop, in order
  - `settings.model_max_file_bytes`, `settings.model_max_smoke_latency_ms`,
    `settings.model_cache_max_bytes`, `settings.model_library_warn_bytes`

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_model_registry.py
import io

import numpy as np
import pytest

from app.models import model_registry
from app.models.spec import parse_runtime_spec


def tiny_onnx(size: int = 8) -> bytes:
    """A real ONNX model: mean of the input, so output is one scalar."""
    import onnx
    from onnx import TensorProto, helper

    inp = helper.make_tensor_value_info("input", TensorProto.FLOAT, [1, 3, size, size])
    out = helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 1])
    node = helper.make_node("ReduceMean", ["input"], ["output"], axes=[1, 2, 3], keepdims=1)
    graph = helper.make_graph([node], "mean", [inp], [out])
    model = helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)])
    return model.SerializeToString()


@pytest.fixture(autouse=True)
def clear_cache():
    model_registry.reset_cache()
    yield
    model_registry.reset_cache()


@pytest.fixture
def stub_storage(monkeypatch):
    blobs = {}
    downloads = []

    async def fake_download(key):
        downloads.append(key)
        return blobs[key]

    monkeypatch.setattr(model_registry.storage, "download_bytes", fake_download)
    return blobs, downloads


@pytest.mark.asyncio
async def test_second_load_is_served_from_cache(stub_storage):
    blobs, downloads = stub_storage
    blobs["models/a.onnx"] = tiny_onnx()
    await model_registry.get_or_load("m1", "models/a.onnx")
    await model_registry.get_or_load("m1", "models/a.onnx")
    assert downloads == ["models/a.onnx"]


@pytest.mark.asyncio
async def test_two_models_are_cached_separately(stub_storage):
    blobs, downloads = stub_storage
    blobs["models/a.onnx"] = tiny_onnx()
    blobs["models/b.onnx"] = tiny_onnx()
    await model_registry.get_or_load("m1", "models/a.onnx")
    await model_registry.get_or_load("m2", "models/b.onnx")
    assert len(downloads) == 2
    assert model_registry.cache_bytes() > 0


@pytest.mark.asyncio
async def test_cache_evicts_least_recently_used_past_the_budget(stub_storage, monkeypatch):
    blobs, downloads = stub_storage
    blob = tiny_onnx()
    blobs["models/a.onnx"] = blob
    blobs["models/b.onnx"] = blob
    monkeypatch.setattr(model_registry.settings, "model_cache_max_bytes", len(blob) + 1, raising=False)

    await model_registry.get_or_load("m1", "models/a.onnx")
    await model_registry.get_or_load("m2", "models/b.onnx")
    await model_registry.get_or_load("m1", "models/a.onnx")  # re-downloaded

    assert downloads == ["models/a.onnx", "models/b.onnx", "models/a.onnx"]


@pytest.mark.asyncio
async def test_run_model_returns_one_result_per_crop(stub_storage):
    import cv2

    blobs, _ = stub_storage
    blobs["models/a.onnx"] = tiny_onnx()
    spec = parse_runtime_spec(
        {
            "specVersion": 1,
            "input": {"width": 8, "height": 8},
            "output": {"kind": "severity_scalar", "capability": "acne"},
        }
    )
    ok, enc = cv2.imencode(".jpg", np.full((20, 20, 3), 128, dtype=np.uint8))
    crop = enc.tobytes()

    results = await model_registry.run_model("m1", "models/a.onnx", spec, [crop, crop])

    assert len(results) == 2
    for r in results:
        assert set(r) == {"acne"}
        assert 0.0 <= r["acne"] <= 100.0


@pytest.mark.asyncio
async def test_invalidate_drops_the_session(stub_storage):
    blobs, downloads = stub_storage
    blobs["models/a.onnx"] = tiny_onnx()
    await model_registry.get_or_load("m1", "models/a.onnx")
    model_registry.invalidate("m1")
    await model_registry.get_or_load("m1", "models/a.onnx")
    assert len(downloads) == 2
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_model_registry.py -v`
Expected: FAIL — `AttributeError: module 'app.models.model_registry' has no attribute 'reset_cache'`

If `pytest.mark.asyncio` is unrecognized, add `pytest-asyncio>=0.23` to
`apps/vision-ai-worker/requirements.txt` and `pyproject.toml`, and
`asyncio_mode = "auto"` under `[tool.pytest.ini_options]` in the pyproject.
Do this in this step, not later.

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/config.py  -- add inside class Settings

    def _int_env(self, name: str, default: int) -> int:
        raw = os.getenv(name, "").strip()
        try:
            return int(raw) if raw else default
        except ValueError:
            return default

    @property
    def model_max_file_bytes(self) -> int:
        """Upload ceiling. 200 MB: the largest model measured (a 158 MB
        segmentation export) plus headroom."""
        return self._int_env("MODEL_MAX_FILE_BYTES", 200 * 1024 * 1024)

    @property
    def model_max_smoke_latency_ms(self) -> int:
        """Above this a single run is reported as slow at upload -- a warning,
        not a rejection: a slow model may be a deliberate choice."""
        return self._int_env("MODEL_MAX_SMOKE_LATENCY_MS", 400)

    @property
    def model_cache_max_bytes(self) -> int:
        return self._int_env("MODEL_CACHE_MAX_BYTES", 1024 * 1024 * 1024)

    @property
    def model_library_warn_bytes(self) -> int:
        return self._int_env("MODEL_LIBRARY_WARN_BYTES", 10 * 1024 * 1024 * 1024)
```

```python
# apps/vision-ai-worker/app/models/model_registry.py  (rewrite)
"""Loads models from the library and runs them under their declared spec.

Sessions are cached by model id -- not by capability -- because several models
may serve one capability over time and switching back must stay free. The
cache is bounded and evicts least-recently-used.
"""

import asyncio
from collections import OrderedDict

import numpy as np
import onnxruntime as ort

from app import storage
from app.config import settings
from app.models.adapters import adapt
from app.models.preprocess import preprocess, preprocess_batch
from app.models.spec import RuntimeSpec

_loaded: "OrderedDict[str, tuple[ort.InferenceSession, int]]" = OrderedDict()
_lock = asyncio.Lock()


def reset_cache() -> None:
    _loaded.clear()


def cache_bytes() -> int:
    return sum(size for _, size in _loaded.values())


def invalidate(model_id: str) -> None:
    _loaded.pop(model_id, None)


def load_session_from_bytes(data: bytes) -> ort.InferenceSession:
    return ort.InferenceSession(data, providers=["CPUExecutionProvider"])


def _evict_to_budget() -> None:
    budget = settings.model_cache_max_bytes
    while _loaded and cache_bytes() > budget:
        _loaded.popitem(last=False)


async def get_or_load(model_id: str, s3_key: str) -> ort.InferenceSession:
    async with _lock:
        cached = _loaded.get(model_id)
        if cached is not None:
            _loaded.move_to_end(model_id)
            return cached[0]

    data = await storage.download_bytes(s3_key)
    session = await asyncio.to_thread(load_session_from_bytes, data)

    async with _lock:
        _loaded[model_id] = (session, len(data))
        _loaded.move_to_end(model_id)
        _evict_to_budget()
    return session


def _accepts_batch(session: ort.InferenceSession) -> bool:
    shape = session.get_inputs()[0].shape
    return bool(shape) and not isinstance(shape[0], int)


def _run(session: ort.InferenceSession, tensor: np.ndarray) -> list[np.ndarray]:
    name = session.get_inputs()[0].name
    return session.run(None, {name: tensor})


async def run_model(
    model_id: str, s3_key: str, spec: RuntimeSpec, crops: list[bytes]
) -> list[dict[str, float]]:
    """One {capability: severity} per crop, in the order given."""
    session = await get_or_load(model_id, s3_key)

    if spec.input.embedded or not _accepts_batch(session):
        results = []
        for crop in crops:
            tensor = await asyncio.to_thread(preprocess, crop, spec.input)
            outputs = await asyncio.to_thread(_run, session, tensor)
            results.append(adapt(spec, outputs))
        return results

    batch = await asyncio.to_thread(preprocess_batch, crops, spec.input)
    outputs = await asyncio.to_thread(_run, session, batch)
    return [adapt(spec, [np.asarray(o)[i : i + 1] for o in outputs]) for i in range(len(crops))]
```

Add `import os` at the top of `app/config.py` if it is not already there (it is).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_model_registry.py -v`
Expected: PASS, 5 tests.

Then run the whole worker suite to see what the rewrite broke — the old
`run_capability(capability, bytes)` is gone and `tests/test_dispatch_zones.py`
stubs it:

Run: `pytest apps/vision-ai-worker -v`
Expected: failures in `test_dispatch_zones.py` only. Leave them failing; Task 7
rewrites that file. Note them in the commit message.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/models/model_registry.py apps/vision-ai-worker/app/config.py apps/vision-ai-worker/tests/test_model_registry.py apps/vision-ai-worker/requirements.txt apps/vision-ai-worker/pyproject.toml
git commit -m "feat: model-keyed session cache with an eviction budget

Dispatch tests fail until the router is rewritten in a later task."
```

---

### Task 5: Schema — library, bindings, activation log

**Files:**
- Modify: `apps/vision-ai-worker/app/db.py`
- Test: `apps/vision-ai-worker/tests/test_db_schema.py`

Read `docs/SHARED-DATABASE.md` first. This table is not owned by GORM and no
other repository reads it, but it lives in the shared Postgres: additive SQL
only, `IF NOT EXISTS` everywhere, no `DROP` except the documented uniqueness
constraint in Task 9.

**Interfaces:**
- Consumes: nothing.
- Produces (all `async`, all raising `RuntimeError` when the pool is absent):
  - `list_models() -> list[dict]` (library; each row gains `active_for: list[str]`)
  - `get_model_by_id(model_id: str) -> dict | None`
  - `insert_model(s3_key, original_filename, size_bytes, input_shape, output_shape, runtime_spec: dict | None, produces: list[str], capability_code: str, applicable_zones: list[str] | None, label: str | None) -> dict`
  - `delete_model_by_id(model_id: str) -> dict | None`
  - `update_model_fields(model_id: str, runtime_spec: dict | None, applicable_zones: list[str] | None, label: str | None) -> dict | None`
  - `list_bindings() -> dict[str, str]` — capability code to model id
  - `get_bound_models(capabilities: list[str]) -> dict[str, dict]` — capability to model row
  - `set_binding(capability: str, model_id: str, actor: str | None) -> None`
  - `clear_binding(capability: str) -> bool`
  - `list_activation_log(capability: str) -> list[dict]`
  - `library_bytes() -> int`
  - `SCHEMA_STATEMENTS: list[str]` — every DDL string, applied in order by
    `init_pool`

- [ ] **Step 1: Write the failing test**

This test runs the DDL against a real Postgres when `TEST_DATABASE_URL` is set
and is skipped otherwise, so the suite stays runnable offline.

```python
# apps/vision-ai-worker/tests/test_db_schema.py
import os

import asyncpg
import pytest

from app import db

DSN = os.getenv("TEST_DATABASE_URL", "")
pytestmark = pytest.mark.skipif(not DSN, reason="TEST_DATABASE_URL not set")


@pytest.fixture
async def pool(monkeypatch):
    p = await asyncpg.create_pool(DSN, min_size=1, max_size=2)
    async with p.acquire() as conn:
        for statement in db.SCHEMA_STATEMENTS:
            await conn.execute(statement)
    monkeypatch.setattr(db, "_pool", p)
    yield p
    async with p.acquire() as conn:
        await conn.execute("DELETE FROM vision_capability_activation_log")
        await conn.execute("DELETE FROM vision_capability_bindings")
        await conn.execute("DELETE FROM vision_models")
    await p.close()


async def _model(produces=("acne",), label="test"):
    return await db.insert_model(
        s3_key=f"models/{label}.onnx",
        original_filename=f"{label}.onnx",
        size_bytes=1234,
        input_shape="[1, 3, 224, 224]",
        output_shape="[1, 1]",
        runtime_spec={"specVersion": 1, "input": {"width": 224, "height": 224},
                      "output": {"kind": "severity_scalar", "capability": produces[0]}},
        produces=list(produces),
        capability_code=produces[0],
        applicable_zones=None,
        label=label,
    )


async def test_two_models_can_declare_the_same_capability(pool):
    await _model(label="old")
    await _model(label="new")
    rows = await db.list_models()
    assert len(rows) == 2
    assert all(r["produces"] == ["acne"] for r in rows)


async def test_binding_selects_which_model_serves_a_capability(pool):
    old = await _model(label="old")
    new = await _model(label="new")

    await db.set_binding("acne", old["id"], actor="tester")
    assert (await db.get_bound_models(["acne"]))["acne"]["id"] == old["id"]

    await db.set_binding("acne", new["id"], actor="tester")
    assert (await db.get_bound_models(["acne"]))["acne"]["id"] == new["id"]

    await db.set_binding("acne", old["id"], actor="tester")  # rollback
    assert (await db.get_bound_models(["acne"]))["acne"]["id"] == old["id"]


async def test_unbound_capability_resolves_to_nothing(pool):
    await _model(label="only")
    assert await db.get_bound_models(["acne"]) == {}


async def test_clear_binding_leaves_the_model_in_the_library(pool):
    m = await _model(label="only")
    await db.set_binding("acne", m["id"], actor=None)
    assert await db.clear_binding("acne") is True
    assert await db.get_bound_models(["acne"]) == {}
    assert len(await db.list_models()) == 1


async def test_activation_log_records_every_switch(pool):
    old = await _model(label="old")
    new = await _model(label="new")
    await db.set_binding("acne", old["id"], actor="alice")
    await db.set_binding("acne", new["id"], actor="bob")
    await db.clear_binding("acne")

    history = await db.list_activation_log("acne")
    assert [h["activated_by"] for h in history[:2]] == ["alice", "bob"]
    assert history[-1]["model_id"] is None  # the deactivation


async def test_list_models_reports_what_each_is_active_for(pool):
    m = await _model(produces=("acne", "redness"), label="multi")
    await db.set_binding("acne", m["id"], actor=None)
    row = (await db.list_models())[0]
    assert row["active_for"] == ["acne"]


async def test_library_bytes_sums_the_library(pool):
    await _model(label="a")
    await _model(label="b")
    assert await db.library_bytes() == 2468
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `TEST_DATABASE_URL=postgresql://... pytest apps/vision-ai-worker/tests/test_db_schema.py -v`
Expected: FAIL — `AttributeError: module 'app.db' has no attribute 'SCHEMA_STATEMENTS'`

Without `TEST_DATABASE_URL` the file skips; that is a pass, not a success.
Use a scratch database, never the shared one.

- [ ] **Step 3: Write the implementation**

Replace the DDL constants and add the new functions. Keep every existing
function working — Task 7 removes the last callers of the capability-keyed ones.

```python
# apps/vision-ai-worker/app/db.py  -- replace CREATE_TABLE_SQL/ADD_APPLICABLE_ZONES_SQL

import json

SCHEMA_STATEMENTS = [
    """
    CREATE TABLE IF NOT EXISTS vision_models (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      capability_code TEXT UNIQUE NOT NULL,
      s3_key TEXT NOT NULL,
      original_filename TEXT NOT NULL,
      size_bytes BIGINT NOT NULL,
      input_shape TEXT,
      output_shape TEXT,
      version INTEGER NOT NULL DEFAULT 1,
      uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    """,
    "ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS applicable_zones TEXT[];",
    "ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS runtime_spec JSONB;",
    "ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS produces TEXT[];",
    "ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS label TEXT;",
    """
    CREATE TABLE IF NOT EXISTS vision_capability_bindings (
      capability_code TEXT PRIMARY KEY,
      model_id        UUID NOT NULL REFERENCES vision_models(id),
      activated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
      activated_by    TEXT
    );
    """,
    """
    CREATE TABLE IF NOT EXISTS vision_capability_activation_log (
      id              BIGSERIAL PRIMARY KEY,
      capability_code TEXT NOT NULL,
      model_id        UUID,
      activated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
      activated_by    TEXT
    );
    """,
    # Backfill: a row's own capability is what it produces, and whatever a row
    # served before this migration is what it keeps serving.
    "UPDATE vision_models SET produces = ARRAY[capability_code] WHERE produces IS NULL;",
    """
    INSERT INTO vision_capability_bindings (capability_code, model_id, activated_by)
    SELECT capability_code, id, 'migration' FROM vision_models
    ON CONFLICT (capability_code) DO NOTHING;
    """,
]
```

```python
# apps/vision-ai-worker/app/db.py  -- init_pool body, replacing the two executes

        async with _pool.acquire() as conn:
            for statement in SCHEMA_STATEMENTS:
                await conn.execute(statement)
```

```python
# apps/vision-ai-worker/app/db.py  -- add below the existing functions

def _serialize_row(row) -> dict:
    d = dict(row)
    if d.get("id") is not None:
        d["id"] = str(d["id"])
    if d.get("model_id") is not None:
        d["model_id"] = str(d["model_id"])
    if d.get("uploaded_at") is not None:
        d["uploaded_at"] = d["uploaded_at"].isoformat()
    if d.get("activated_at") is not None:
        d["activated_at"] = d["activated_at"].isoformat()
    if isinstance(d.get("runtime_spec"), str):
        d["runtime_spec"] = json.loads(d["runtime_spec"])
    return d


async def list_models() -> list[dict]:
    pool = _require_pool()
    async with pool.acquire() as conn:
        rows = await conn.fetch(
            """
            SELECT m.*, COALESCE(
                     (SELECT array_agg(b.capability_code ORDER BY b.capability_code)
                        FROM vision_capability_bindings b WHERE b.model_id = m.id),
                     ARRAY[]::text[]
                   ) AS active_for
              FROM vision_models m
             ORDER BY m.uploaded_at DESC
            """
        )
        return [_serialize_row(r) for r in rows]


async def get_model_by_id(model_id: str) -> dict | None:
    pool = _require_pool()
    async with pool.acquire() as conn:
        row = await conn.fetchrow("SELECT * FROM vision_models WHERE id = $1", model_id)
        return _serialize_row(row) if row else None


async def insert_model(
    s3_key: str,
    original_filename: str,
    size_bytes: int,
    input_shape: str,
    output_shape: str,
    runtime_spec: dict | None,
    produces: list[str],
    capability_code: str,
    applicable_zones: list[str] | None,
    label: str | None,
) -> dict:
    pool = _require_pool()
    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            INSERT INTO vision_models
              (capability_code, s3_key, original_filename, size_bytes, input_shape,
               output_shape, runtime_spec, produces, applicable_zones, label)
            VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::text[],$9::text[],$10)
            RETURNING *
            """,
            capability_code,
            s3_key,
            original_filename,
            size_bytes,
            input_shape,
            output_shape,
            json.dumps(runtime_spec) if runtime_spec is not None else None,
            produces,
            applicable_zones,
            label,
        )
        return _serialize_row(row)


async def update_model_fields(
    model_id: str,
    runtime_spec: dict | None = None,
    applicable_zones: list[str] | None = None,
    label: str | None = None,
) -> dict | None:
    pool = _require_pool()
    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            UPDATE vision_models SET
              runtime_spec     = COALESCE($2::jsonb, runtime_spec),
              applicable_zones = COALESCE($3::text[], applicable_zones),
              label            = COALESCE($4, label)
            WHERE id = $1 RETURNING *
            """,
            model_id,
            json.dumps(runtime_spec) if runtime_spec is not None else None,
            applicable_zones,
            label,
        )
        return _serialize_row(row) if row else None


async def delete_model_by_id(model_id: str) -> dict | None:
    pool = _require_pool()
    async with pool.acquire() as conn:
        row = await conn.fetchrow("DELETE FROM vision_models WHERE id = $1 RETURNING *", model_id)
        return _serialize_row(row) if row else None


async def list_bindings() -> dict[str, str]:
    pool = _require_pool()
    async with pool.acquire() as conn:
        rows = await conn.fetch("SELECT capability_code, model_id FROM vision_capability_bindings")
        return {r["capability_code"]: str(r["model_id"]) for r in rows}


async def get_bound_models(capabilities: list[str]) -> dict[str, dict]:
    if not capabilities:
        return {}
    pool = _require_pool()
    async with pool.acquire() as conn:
        rows = await conn.fetch(
            """
            SELECT b.capability_code, m.*
              FROM vision_capability_bindings b
              JOIN vision_models m ON m.id = b.model_id
             WHERE b.capability_code = ANY($1::text[])
            """,
            capabilities,
        )
        return {r["capability_code"]: _serialize_row(r) for r in rows}


async def set_binding(capability: str, model_id: str, actor: str | None) -> None:
    pool = _require_pool()
    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute(
                """
                INSERT INTO vision_capability_bindings (capability_code, model_id, activated_by)
                VALUES ($1, $2, $3)
                ON CONFLICT (capability_code) DO UPDATE
                  SET model_id = EXCLUDED.model_id,
                      activated_at = now(),
                      activated_by = EXCLUDED.activated_by
                """,
                capability,
                model_id,
                actor,
            )
            await conn.execute(
                "INSERT INTO vision_capability_activation_log (capability_code, model_id, activated_by) VALUES ($1,$2,$3)",
                capability,
                model_id,
                actor,
            )


async def clear_binding(capability: str) -> bool:
    pool = _require_pool()
    async with pool.acquire() as conn:
        async with conn.transaction():
            deleted = await conn.execute(
                "DELETE FROM vision_capability_bindings WHERE capability_code = $1", capability
            )
            if deleted.endswith("0"):
                return False
            await conn.execute(
                "INSERT INTO vision_capability_activation_log (capability_code, model_id) VALUES ($1, NULL)",
                capability,
            )
            return True


async def list_activation_log(capability: str) -> list[dict]:
    pool = _require_pool()
    async with pool.acquire() as conn:
        rows = await conn.fetch(
            "SELECT * FROM vision_capability_activation_log WHERE capability_code = $1 ORDER BY id ASC",
            capability,
        )
        return [_serialize_row(r) for r in rows]


async def library_bytes() -> int:
    pool = _require_pool()
    async with pool.acquire() as conn:
        return int(await conn.fetchval("SELECT COALESCE(SUM(size_bytes), 0) FROM vision_models"))
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `TEST_DATABASE_URL=postgresql://... pytest apps/vision-ai-worker/tests/test_db_schema.py -v`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/db.py apps/vision-ai-worker/tests/test_db_schema.py
git commit -m "feat: model library, capability bindings and activation log"
```

---

### Task 6: Upload validation

An upload that succeeds has demonstrably run once. Five gates: parse, signature,
output rank, smoke run, size.

**Files:**
- Create: `apps/vision-ai-worker/app/models/validation.py`
- Test: `apps/vision-ai-worker/tests/test_model_validation.py`

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces:
  - `validate_upload(data: bytes, spec: RuntimeSpec) -> ValidationReport`
  - `class ValidationReport` — `input_shape: str`, `output_shape: str`,
    `latency_ms: int`, `warnings: list[str]`, `sample: dict[str, float]`
  - `class ValidationFailure(ValueError)` — message names the mismatch
  - `SMOKE_IMAGE_BYTES: bytes` — the fixed grey JPEG every smoke run uses, so
    latencies are comparable between models

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_model_validation.py
import numpy as np
import pytest

from app.models.spec import parse_runtime_spec
from app.models.validation import ValidationFailure, validate_upload
from tests.test_model_registry import tiny_onnx


def spec_for(size=8, output=None):
    return parse_runtime_spec(
        {
            "specVersion": 1,
            "input": {"width": size, "height": size},
            "output": output or {"kind": "severity_scalar", "capability": "acne"},
        }
    )


def test_accepts_a_model_matching_its_spec():
    report = validate_upload(tiny_onnx(8), spec_for(8))
    assert report.input_shape == "[1, 3, 8, 8]"
    assert report.latency_ms >= 0
    assert set(report.sample) == {"acne"}
    assert 0.0 <= report.sample["acne"] <= 100.0


def test_rejects_a_declared_size_the_model_does_not_accept():
    with pytest.raises(ValidationFailure, match="384"):
        validate_upload(tiny_onnx(8), spec_for(384))


def test_rejects_a_class_index_beyond_the_models_output():
    output = {"kind": "multilabel_sigmoid", "classes": [{"index": 5, "capability": "acne"}]}
    with pytest.raises(ValidationFailure, match="5"):
        validate_upload(tiny_onnx(8), spec_for(8, output))


def test_rejects_a_file_that_is_not_a_model():
    with pytest.raises(ValidationFailure, match="ONNX"):
        validate_upload(b"definitely not onnx", spec_for(8))


def test_slow_model_warns_but_passes(monkeypatch):
    from app.models import validation

    monkeypatch.setattr(validation.settings, "model_max_smoke_latency_ms", 0, raising=False)
    report = validate_upload(tiny_onnx(8), spec_for(8))
    assert any("slow" in w.lower() for w in report.warnings)


def test_oversized_file_is_rejected(monkeypatch):
    from app.models import validation

    monkeypatch.setattr(validation.settings, "model_max_file_bytes", 10, raising=False)
    with pytest.raises(ValidationFailure, match="too large"):
        validate_upload(tiny_onnx(8), spec_for(8))
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_model_validation.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'app.models.validation'`

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/models/validation.py
"""Upload gates. A model that passes these has run once, under its own spec,
and produced a usable severity -- so a bad export fails at upload rather than
mid-analysis."""

import time
from dataclasses import dataclass, field

import cv2
import numpy as np

from app.config import settings
from app.models.adapters import AdapterError, adapt
from app.models.model_registry import load_session_from_bytes
from app.models.preprocess import PreprocessError, preprocess
from app.models.spec import RuntimeSpec


class ValidationFailure(ValueError):
    """The upload cannot be accepted; the message names what disagrees."""


@dataclass
class ValidationReport:
    input_shape: str
    output_shape: str
    latency_ms: int
    sample: dict
    warnings: list = field(default_factory=list)


def _smoke_image() -> bytes:
    img = np.full((256, 256, 3), 128, dtype=np.uint8)
    cv2.circle(img, (128, 128), 70, (150, 120, 110), -1)
    ok, enc = cv2.imencode(".jpg", img)
    return enc.tobytes()


SMOKE_IMAGE_BYTES = _smoke_image()


def _dims_compatible(actual, declared: int) -> bool:
    """A dynamic dimension (str or None or -1) accepts anything."""
    return not isinstance(actual, int) or actual in (-1, declared)


def validate_upload(data: bytes, spec: RuntimeSpec) -> ValidationReport:
    if len(data) > settings.model_max_file_bytes:
        raise ValidationFailure(
            f"model file is too large: {len(data)} bytes, limit {settings.model_max_file_bytes}"
        )

    try:
        session = load_session_from_bytes(data)
    except Exception as exc:  # noqa: BLE001
        raise ValidationFailure(f"not a loadable ONNX model: {exc}") from exc

    inputs = session.get_inputs()
    if len(inputs) != 1:
        raise ValidationFailure(f"model must have exactly one input, found {len(inputs)}")

    shape = inputs[0].shape
    if not spec.input.embedded:
        if len(shape) != 4:
            raise ValidationFailure(f"model input rank {len(shape)} is not the expected 4 (N,C,H,W or N,H,W,C)")
        if spec.input.layout == "NCHW":
            _, _, h, w = shape
        else:
            _, h, w, _ = shape
        if not _dims_compatible(h, spec.input.height) or not _dims_compatible(w, spec.input.width):
            raise ValidationFailure(
                f"spec declares {spec.input.width}x{spec.input.height} but the model requires {shape}"
            )

    try:
        tensor = preprocess(SMOKE_IMAGE_BYTES, spec.input)
    except PreprocessError as exc:
        raise ValidationFailure(f"preprocessing failed under this spec: {exc}") from exc

    started = time.perf_counter()
    try:
        outputs = session.run(None, {inputs[0].name: tensor})
    except Exception as exc:  # noqa: BLE001
        raise ValidationFailure(f"the model failed to run under this spec: {exc}") from exc
    latency_ms = int((time.perf_counter() - started) * 1000)

    try:
        sample = adapt(spec, [np.asarray(o) for o in outputs])
    except AdapterError as exc:
        raise ValidationFailure(str(exc)) from exc

    for capability, value in sample.items():
        if not (0.0 <= value <= 100.0) or np.isnan(value):
            raise ValidationFailure(f"adapter produced {value} for '{capability}', outside 0-100")

    warnings = []
    if latency_ms > settings.model_max_smoke_latency_ms:
        warnings.append(
            f"slow model: one run took {latency_ms} ms against a {settings.model_max_smoke_latency_ms} ms "
            f"guideline; 8 zones would cost about {latency_ms * 8 / 1000:.1f} s"
        )

    return ValidationReport(
        input_shape=str(shape),
        output_shape=str(session.get_outputs()[0].shape),
        latency_ms=latency_ms,
        sample=sample,
        warnings=warnings,
    )
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_model_validation.py -v`
Expected: PASS, 6 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/models/validation.py apps/vision-ai-worker/tests/test_model_validation.py
git commit -m "feat: upload validation with a smoke run under the declared spec"
```

---

### Task 7: Dispatch through bindings, grouped by model

Rewrites `dispatch-zones`: resolve each capability to its bound model, group so
a model serving four capabilities runs once, and batch zones where the model
allows it. Preprocessing is shared per model, not across models — see the
design doc's dispatch section for why that is enough.

**Files:**
- Modify: `apps/vision-ai-worker/app/routers/models.py` (the two dispatch
  endpoints)
- Modify: `apps/vision-ai-worker/tests/test_dispatch_zones.py` (rewrite)
- Test: `apps/vision-ai-worker/tests/test_dispatch_zones.py`

**Interfaces:**
- Consumes: `db.get_bound_models`, `model_registry.run_model`,
  `spec.parse_runtime_spec`, `spec.LEGACY_SPEC`.
- Produces: unchanged response contracts `DispatchZonesResponse` /
  `DispatchCapabilitiesResponse`, so core-engine needs no change to read them.

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_dispatch_zones.py  (rewrite)
import base64

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app import db
from app.models import model_registry
from app.routers import models as models_router

api = FastAPI()
api.include_router(models_router.router)
client = TestClient(api)

CROP = base64.b64encode(b"jpeg-bytes").decode()


def _zone(code, angle="FRONT", image=CROP):
    return {"zoneCode": code, "sourceAngle": angle, "image": image}


def _row(model_id, produces, zones=None, spec=None, s3_key=None):
    return {
        "id": model_id,
        "s3_key": s3_key or f"models/{model_id}.onnx",
        "produces": produces,
        "capability_code": produces[0],
        "applicable_zones": zones,
        "runtime_spec": spec,
    }


@pytest.fixture
def registry(monkeypatch):
    state = {"bindings": {}, "runs": [], "fail": set()}

    async def fake_get_bound_models(capabilities):
        return {c: state["bindings"][c] for c in capabilities if c in state["bindings"]}

    async def fake_run_model(model_id, s3_key, spec, crops):
        state["runs"].append((model_id, len(crops)))
        if model_id in state["fail"]:
            raise RuntimeError("inference exploded")
        from app.models.spec import capabilities_of

        return [{c: 25.0 for c in capabilities_of(spec)} for _ in crops]

    monkeypatch.setattr(db, "get_bound_models", fake_get_bound_models)
    monkeypatch.setattr(model_registry, "run_model", fake_run_model)
    return state


def _spec(kind_classes):
    return {
        "specVersion": 1,
        "input": {"width": 224, "height": 224},
        "output": {"kind": "multilabel_sigmoid", "classes": kind_classes},
    }


def test_one_model_serving_several_capabilities_runs_once(registry):
    spec = _spec([{"index": 0, "capability": "acne"}, {"index": 1, "capability": "redness"}])
    row = _row("m1", ["acne", "redness"], spec=spec)
    registry["bindings"] = {"acne": row, "redness": row}

    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne", "redness"], "zones": [_zone("ZONE_FOREHEAD")]},
    )

    assert res.status_code == 200
    assert registry["runs"] == [("m1", 1)]
    telemetry = res.json()["results"][0]["telemetry"]
    assert telemetry == {"acne": 25.0, "redness": 25.0}


def test_zones_are_batched_into_one_run(registry):
    spec = _spec([{"index": 0, "capability": "acne"}])
    registry["bindings"] = {"acne": _row("m1", ["acne"], spec=spec)}

    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne"], "zones": [_zone("ZONE_FOREHEAD"), _zone("ZONE_CHIN")]},
    )

    assert res.status_code == 200
    assert registry["runs"] == [("m1", 2)]


def test_applicable_zones_still_filter(registry):
    spec = _spec([{"index": 0, "capability": "acne"}])
    registry["bindings"] = {"acne": _row("m1", ["acne"], zones=["ZONE_FOREHEAD"], spec=spec)}

    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne"], "zones": [_zone("ZONE_FOREHEAD"), _zone("ZONE_CHIN")]},
    )

    body = res.json()
    assert registry["runs"] == [("m1", 1)]
    assert body["results"][0]["telemetry"] == {"acne": 25.0}
    assert body["results"][1]["telemetry"] == {}


def test_unbound_capability_is_unsupported_not_zero(registry):
    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne"], "zones": [_zone("ZONE_FOREHEAD")]},
    )
    body = res.json()
    assert body["unsupportedCapabilities"] == ["acne"]
    assert body["results"][0]["telemetry"] == {}


def test_a_failing_model_does_not_stop_the_others(registry):
    good = _row("good", ["redness"], spec=_spec([{"index": 0, "capability": "redness"}]))
    bad = _row("bad", ["acne"], spec=_spec([{"index": 0, "capability": "acne"}]))
    registry["bindings"] = {"acne": bad, "redness": good}
    registry["fail"].add("bad")

    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne", "redness"], "zones": [_zone("ZONE_FOREHEAD")]},
    )

    assert res.status_code == 200
    assert res.json()["results"][0]["telemetry"] == {"redness": 25.0}


def test_legacy_row_without_a_spec_still_scores(registry):
    registry["bindings"] = {"acne": _row("m1", ["acne"], spec=None)}
    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne"], "zones": [_zone("ZONE_FOREHEAD")]},
    )
    assert res.json()["results"][0]["telemetry"] == {"acne": 25.0}


def test_split_capabilities_run_both_models(registry):
    multi_spec = _spec([{"index": 0, "capability": "acne"}, {"index": 1, "capability": "pores"}])
    multi = _row("multi", ["acne", "pores"], spec=multi_spec)
    specialist = _row("spec1", ["pores"], spec=_spec([{"index": 0, "capability": "pores"}]))
    registry["bindings"] = {"acne": multi, "pores": specialist}

    res = client.post(
        "/api/v1/models/dispatch-zones",
        json={"capabilities": ["acne", "pores"], "zones": [_zone("ZONE_FOREHEAD")]},
    )

    assert sorted(registry["runs"]) == [("multi", 1), ("spec1", 1)]
    assert res.json()["results"][0]["telemetry"] == {"acne": 25.0, "pores": 25.0}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_dispatch_zones.py -v`
Expected: FAIL — the router still calls `db.list_models` and
`model_registry.run_capability`.

- [ ] **Step 3: Write the implementation**

Replace both dispatch endpoints in `app/routers/models.py`:

```python
# apps/vision-ai-worker/app/routers/models.py  -- replace dispatch_capabilities
# and dispatch_zones, and update the imports at the top of the file.

from app.models import model_registry
from app.models.spec import LEGACY_SPEC, SpecError, capabilities_of, parse_runtime_spec


def _spec_for(row: dict):
    """A row without a declared spec runs the legacy contract, with its own
    capability filled in. One code path, no branch on 'is this an old row'."""
    raw = row.get("runtime_spec")
    if raw is None:
        return LEGACY_SPEC.with_capability(row["capability_code"])
    return parse_runtime_spec(raw)


@router.post("/dispatch-zones", response_model=DispatchZonesResponse)
async def dispatch_zones(body: DispatchZonesRequest):
    """Scores every requested capability on every zone crop its model applies
    to. Telemetry values are severity 0-100 (the caller converts to health)."""
    capabilities = list(dict.fromkeys(c.strip() for c in body.capabilities if c.strip()))
    results = [ZoneTelemetry(zoneCode=z.zoneCode, sourceAngle=z.sourceAngle) for z in body.zones]
    if not capabilities or not body.zones:
        return DispatchZonesResponse(results=results, unsupportedCapabilities=[])

    try:
        bound = await db.get_bound_models(capabilities)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc

    unsupported = [c for c in capabilities if c not in bound]

    # Group by model: one model bound to four requested capabilities runs once.
    models_by_id: dict[str, dict] = {}
    wanted: dict[str, set] = {}
    for capability, row in bound.items():
        models_by_id[row["id"]] = row
        wanted.setdefault(row["id"], set()).add(capability)

    crops: list[bytes | None] = []
    for zone in body.zones:
        try:
            crops.append(base64.b64decode(zone.image, validate=True))
        except Exception:  # noqa: BLE001
            crops.append(None)

    for model_id, row in models_by_id.items():
        try:
            spec = _spec_for(row)
        except SpecError as exc:
            print(f"[vision-models] Model {model_id} has an unusable spec: {exc}")
            continue

        allowed = set(row.get("applicable_zones") or [])
        indices = [
            i
            for i, (zone, crop) in enumerate(zip(body.zones, crops))
            if crop and (not allowed or zone.zoneCode in allowed)
        ]
        if not indices:
            continue

        try:
            scored = await model_registry.run_model(
                model_id, row["s3_key"], spec, [crops[i] for i in indices]
            )
        except Exception as exc:  # noqa: BLE001
            print(f"[vision-models] Model {model_id} failed: {exc}")
            continue

        produced = wanted[model_id]
        for i, per_capability in zip(indices, scored):
            for capability, severity in per_capability.items():
                if capability in produced:
                    results[i].telemetry[capability] = severity

    return DispatchZonesResponse(results=results, unsupportedCapabilities=unsupported)


@router.post("/dispatch-capabilities", response_model=DispatchCapabilitiesResponse)
async def dispatch_capabilities(body: DispatchCapabilitiesRequest):
    """Whole-image scoring. Same resolution as dispatch-zones, one crop."""
    if not body.capabilities or not body.images:
        return DispatchCapabilitiesResponse(telemetry={})

    try:
        image_bytes = base64.b64decode(body.images[0])
    except Exception:  # noqa: BLE001
        return DispatchCapabilitiesResponse(telemetry={})

    try:
        bound = await db.get_bound_models(list(dict.fromkeys(body.capabilities)))
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc

    telemetry: dict[str, float] = {}
    seen: set[str] = set()
    for capability, row in bound.items():
        if row["id"] in seen:
            continue
        seen.add(row["id"])
        try:
            spec = _spec_for(row)
            scored = await model_registry.run_model(row["id"], row["s3_key"], spec, [image_bytes])
        except Exception as exc:  # noqa: BLE001
            print(f"[vision-models] Model {row['id']} failed: {exc}")
            continue
        for produced, severity in scored[0].items():
            if produced in bound:
                telemetry[produced] = severity

    return DispatchCapabilitiesResponse(telemetry=telemetry)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker/tests/test_dispatch_zones.py -v`
Expected: PASS, 7 tests.

Then the whole suite: `pytest apps/vision-ai-worker -v`
Expected: PASS except `test_model_zones.py`, which exercises the upload
endpoint Task 8 rewrites. If it fails, leave it and say so in the commit.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/routers/models.py apps/vision-ai-worker/tests/test_dispatch_zones.py
git commit -m "feat: resolve dispatch through bindings and batch per model"
```

---

### Task 8: Upload into the library

Upload stops replacing. It inserts a library row, and binds only the
capabilities named in `activate`.

**Files:**
- Modify: `apps/vision-ai-worker/app/routers/models.py` (upload endpoint)
- Modify: `apps/vision-ai-worker/app/schemas/contracts.py` (record fields)
- Modify: `apps/vision-ai-worker/tests/test_model_zones.py` (adjust to the
  library semantics)
- Test: `apps/vision-ai-worker/tests/test_model_upload.py`

**Interfaces:**
- Consumes: `validation.validate_upload`, `db.insert_model`, `db.set_binding`,
  `spec.parse_runtime_spec`, `spec.capabilities_of`.
- Produces: `VisionModelRecord` gaining `runtimeSpec`, `produces`, `label`,
  `activeFor`; upload response gaining `warnings` and `sample`.

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_model_upload.py
import io
import json

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app import db, storage
from app.routers import models as models_router
from tests.test_model_registry import tiny_onnx

api = FastAPI()
api.include_router(models_router.router)
client = TestClient(api)

SPEC = {
    "specVersion": 1,
    "input": {"width": 8, "height": 8},
    "output": {"kind": "severity_scalar", "capability": "acne"},
}


@pytest.fixture
def stub_db(monkeypatch):
    state = {"inserted": [], "bindings": {}, "uploaded": []}

    async def fake_insert_model(**kwargs):
        row = {"id": f"m{len(state['inserted'])}", "uploaded_at": "2026-09-26T00:00:00+00:00", "version": 1, **kwargs}
        state["inserted"].append(row)
        return row

    async def fake_set_binding(capability, model_id, actor):
        state["bindings"][capability] = model_id

    async def fake_upload_bytes(key, data):
        state["uploaded"].append(key)

    monkeypatch.setattr(db, "insert_model", fake_insert_model)
    monkeypatch.setattr(db, "set_binding", fake_set_binding)
    monkeypatch.setattr(storage, "upload_bytes", fake_upload_bytes)
    return state


def _post(spec=SPEC, activate=None, label="candidate", filename="model.onnx"):
    data = {"capability": "acne", "label": label}
    if spec is not None:
        data["runtimeSpec"] = json.dumps(spec)
    if activate is not None:
        data["activate"] = ",".join(activate)
    return client.post(
        "/api/v1/models/upload",
        files={"file": (filename, io.BytesIO(tiny_onnx(8)), "application/octet-stream")},
        data=data,
    )


def test_upload_adds_to_the_library_without_activating(stub_db):
    res = _post()
    assert res.status_code == 200
    assert len(stub_db["inserted"]) == 1
    assert stub_db["bindings"] == {}
    assert res.json()["produces"] == ["acne"]


def test_upload_can_activate_on_success(stub_db):
    res = _post(activate=["acne"])
    assert res.status_code == 200
    assert stub_db["bindings"]["acne"] == stub_db["inserted"][0]["id"]


def test_second_upload_for_the_same_capability_is_allowed(stub_db):
    _post(label="old")
    res = _post(label="new")
    assert res.status_code == 200
    assert len(stub_db["inserted"]) == 2


def test_activating_a_capability_the_model_does_not_produce_is_rejected(stub_db):
    res = _post(activate=["redness"])
    assert res.status_code == 422
    assert "redness" in res.json()["detail"]
    assert stub_db["inserted"] == []


def test_spec_that_does_not_match_the_model_is_rejected(stub_db):
    bad = {**SPEC, "input": {"width": 384, "height": 384}}
    res = _post(spec=bad)
    assert res.status_code == 422
    assert "384" in res.json()["detail"]
    assert stub_db["uploaded"] == []


def test_malformed_spec_json_is_a_400(stub_db):
    res = client.post(
        "/api/v1/models/upload",
        files={"file": ("model.onnx", io.BytesIO(tiny_onnx(8)), "application/octet-stream")},
        data={"capability": "acne", "runtimeSpec": "{not json"},
    )
    assert res.status_code == 400


def test_non_onnx_extension_is_rejected(stub_db):
    res = _post(filename="model.bin")
    assert res.status_code == 400


def test_slow_model_warning_reaches_the_response(stub_db, monkeypatch):
    from app.models import validation

    monkeypatch.setattr(validation.settings, "model_max_smoke_latency_ms", 0, raising=False)
    body = _post().json()
    assert body["warnings"]
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_model_upload.py -v`
Expected: FAIL — the endpoint still calls `db.upsert_model` and rejects the
unknown `runtimeSpec` form field.

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/schemas/contracts.py  -- replace VisionModelRecord

class VisionModelRecord(BaseModel):
    id: str
    capabilityCode: str = Field(..., alias="capability_code")
    produces: List[str] = Field(default_factory=list)
    label: Optional[str] = None
    sizeBytes: int = Field(..., alias="size_bytes")
    originalFilename: str = Field(..., alias="original_filename")
    inputShape: Optional[str] = Field(None, alias="input_shape")
    outputShape: Optional[str] = Field(None, alias="output_shape")
    runtimeSpec: Optional[dict] = Field(None, alias="runtime_spec")
    version: int = 1
    uploadedAt: str = Field(..., alias="uploaded_at")
    applicableZones: Optional[List[str]] = Field(None, alias="applicable_zones")
    activeFor: List[str] = Field(default_factory=list, alias="active_for")

    class Config:
        populate_by_name = True


class UploadModelResponse(VisionModelRecord):
    warnings: List[str] = Field(default_factory=list)
    sample: dict[str, float] = Field(default_factory=dict)
```

```python
# apps/vision-ai-worker/app/routers/models.py  -- replace upload_model

@router.post("/upload", response_model=UploadModelResponse, response_model_by_alias=False)
async def upload_model(
    file: UploadFile = File(...),
    capability: str = Form(...),
    applicableZones: Optional[str] = Form(None),
    runtimeSpec: Optional[str] = Form(None),
    label: Optional[str] = Form(None),
    activate: Optional[str] = Form(None),
):
    if not capability.strip():
        raise HTTPException(status_code=400, detail="capability is required")
    if not file.filename.lower().endswith(".onnx"):
        raise HTTPException(status_code=400, detail="Model file must be a .onnx file")

    zones = None
    if applicableZones:
        try:
            zones = parse_zone_list(applicableZones.split(","))
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    spec_dict = None
    if runtimeSpec:
        try:
            spec_dict = json.loads(runtimeSpec)
        except json.JSONDecodeError as exc:
            raise HTTPException(status_code=400, detail=f"runtimeSpec is not valid JSON: {exc}") from exc

    try:
        spec = parse_runtime_spec(spec_dict)
        if spec_dict is None:
            spec = spec.with_capability(capability.strip())
    except SpecError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc

    produces = capabilities_of(spec)
    to_activate = [c.strip() for c in (activate or "").split(",") if c.strip()]
    unknown = [c for c in to_activate if c not in produces]
    if unknown:
        raise HTTPException(
            status_code=422,
            detail=f"cannot activate {', '.join(unknown)}: this model produces {', '.join(produces)}",
        )

    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="Uploaded file is empty")

    try:
        report = validate_upload(data, spec)
    except ValidationFailure as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc

    s3_key = f"models/{capability.strip()}/{int(time.time())}_{file.filename}"
    try:
        await storage.upload_bytes(s3_key, data)
        row = await db.insert_model(
            s3_key=s3_key,
            original_filename=file.filename,
            size_bytes=len(data),
            input_shape=report.input_shape,
            output_shape=report.output_shape,
            runtime_spec=spec_dict,
            produces=produces,
            capability_code=capability.strip(),
            applicable_zones=zones,
            label=label,
        )
        for code in to_activate:
            await db.set_binding(code, row["id"], actor=None)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc

    return UploadModelResponse(
        **row,
        active_for=to_activate,
        warnings=report.warnings,
        sample=report.sample,
    )
```

Add to the imports at the top of the router: `import json`, and
`from app.models.validation import ValidationFailure, validate_upload`, and
`UploadModelResponse` to the contracts import.

Note `row` from the stub in tests carries the insert kwargs, so
`UploadModelResponse(**row, ...)` must tolerate extra keys — give
`UploadModelResponse` `model_config = ConfigDict(extra="ignore")` if Pydantic
complains.

Update `tests/test_model_zones.py`: it asserts the upsert behaviour. Change its
expectations to the library semantics — an upload returns `produces` and does
not bind unless `activate` is sent — keeping every zone-parsing assertion it
already makes.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker -v`
Expected: PASS, whole suite green.

- [ ] **Step 5: Commit**

```bash
git add apps/vision-ai-worker/app/routers/models.py apps/vision-ai-worker/app/schemas/contracts.py apps/vision-ai-worker/tests/test_model_upload.py apps/vision-ai-worker/tests/test_model_zones.py
git commit -m "feat: upload adds to the library instead of replacing"
```

---

### Task 9: Library and activation endpoints

The screens in Seagull-web are rendered from these.

**Files:**
- Modify: `apps/vision-ai-worker/app/routers/models.py`
- Modify: `apps/vision-ai-worker/app/schemas/contracts.py`
- Test: `apps/vision-ai-worker/tests/test_activation_api.py`

**Interfaces:**
- Consumes: Task 5's db functions, Task 1's `kind_catalog`.
- Produces:
  - `GET /api/v1/models/kinds`
  - `GET /api/v1/models` — `{models: [...], libraryBytes: int, libraryWarn: bool}`
  - `GET /api/v1/models/capabilities`
  - `PUT /api/v1/models/capabilities/{code}/binding` — body `{"modelId": "..."}`
  - `DELETE /api/v1/models/capabilities/{code}/binding`
  - `GET /api/v1/models/capabilities/{code}/history`
  - `PATCH /api/v1/models/models/{model_id}`
  - `DELETE /api/v1/models/models/{model_id}`
  - 410 stubs at the old `PATCH|DELETE /api/v1/models/{capability}`

- [ ] **Step 1: Write the failing tests**

```python
# apps/vision-ai-worker/tests/test_activation_api.py
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app import db, storage
from app.models import model_registry
from app.routers import models as models_router

api = FastAPI()
api.include_router(models_router.router)
client = TestClient(api)


@pytest.fixture
def stub(monkeypatch):
    state = {
        "models": {
            "m1": {"id": "m1", "produces": ["acne"], "s3_key": "models/a.onnx", "size_bytes": 10},
            "m2": {"id": "m2", "produces": ["acne", "redness"], "s3_key": "models/b.onnx", "size_bytes": 20},
        },
        "bindings": {},
        "log": [],
        "deleted": [],
    }

    async def get_model_by_id(model_id):
        return state["models"].get(model_id)

    async def list_models():
        return [
            {**m, "active_for": sorted(c for c, mid in state["bindings"].items() if mid == m["id"])}
            for m in state["models"].values()
        ]

    async def set_binding(capability, model_id, actor):
        state["bindings"][capability] = model_id
        state["log"].append({"capability_code": capability, "model_id": model_id})

    async def clear_binding(capability):
        existed = state["bindings"].pop(capability, None) is not None
        if existed:
            state["log"].append({"capability_code": capability, "model_id": None})
        return existed

    async def list_bindings():
        return dict(state["bindings"])

    async def list_activation_log(capability):
        return [e for e in state["log"] if e["capability_code"] == capability]

    async def delete_model_by_id(model_id):
        return state["models"].pop(model_id, None)

    async def library_bytes():
        return sum(m["size_bytes"] for m in state["models"].values())

    async def delete_object(key):
        state["deleted"].append(key)

    for name, fn in [
        ("get_model_by_id", get_model_by_id),
        ("list_models", list_models),
        ("set_binding", set_binding),
        ("clear_binding", clear_binding),
        ("list_bindings", list_bindings),
        ("list_activation_log", list_activation_log),
        ("delete_model_by_id", delete_model_by_id),
        ("library_bytes", library_bytes),
    ]:
        monkeypatch.setattr(db, name, fn)
    monkeypatch.setattr(storage, "delete_object", delete_object)
    return state


def test_kinds_are_advertised():
    body = client.get("/api/v1/models/kinds").json()
    assert {k["kind"] for k in body["kinds"]} >= {"severity_scalar", "mask_area_ratio"}


def test_library_lists_models_and_total_bytes(stub):
    body = client.get("/api/v1/models").json()
    assert len(body["models"]) == 2
    assert body["libraryBytes"] == 30


def test_binding_a_capability_switches_the_model(stub):
    assert client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"}).status_code == 200
    assert stub["bindings"]["acne"] == "m1"
    assert client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m2"}).status_code == 200
    assert stub["bindings"]["acne"] == "m2"


def test_rollback_is_the_same_call_with_the_older_model(stub):
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"})
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m2"})
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"})
    assert stub["bindings"]["acne"] == "m1"
    assert len(stub["log"]) == 3


def test_binding_to_a_model_that_does_not_produce_it_is_rejected(stub):
    res = client.put("/api/v1/models/capabilities/redness/binding", json={"modelId": "m1"})
    assert res.status_code == 422
    assert "redness" in res.json()["detail"]
    assert stub["bindings"] == {}


def test_binding_an_unknown_model_is_404(stub):
    assert client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "nope"}).status_code == 404


def test_unbinding_makes_the_capability_unserved(stub):
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"})
    assert client.delete("/api/v1/models/capabilities/acne/binding").status_code == 200
    assert "acne" not in stub["bindings"]
    assert client.delete("/api/v1/models/capabilities/acne/binding").status_code == 404


def test_history_lists_the_switches(stub):
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"})
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m2"})
    history = client.get("/api/v1/models/capabilities/acne/history").json()
    assert [h["model_id"] for h in history["history"]] == ["m1", "m2"]


def test_deleting_a_bound_model_is_refused(stub):
    client.put("/api/v1/models/capabilities/acne/binding", json={"modelId": "m1"})
    res = client.delete("/api/v1/models/models/m1")
    assert res.status_code == 409
    assert "acne" in res.json()["detail"]
    assert "m1" in stub["models"]


def test_deleting_an_unbound_model_removes_it_and_its_file(stub):
    res = client.delete("/api/v1/models/models/m1")
    assert res.status_code == 200
    assert "m1" not in stub["models"]
    assert stub["deleted"] == ["models/a.onnx"]


def test_old_capability_keyed_paths_are_gone_with_a_pointer(stub):
    res = client.delete("/api/v1/models/acne")
    assert res.status_code == 410
    assert "/capabilities/" in res.json()["detail"] or "/models/" in res.json()["detail"]
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pytest apps/vision-ai-worker/tests/test_activation_api.py -v`
Expected: FAIL — 404s, the routes do not exist.

- [ ] **Step 3: Write the implementation**

```python
# apps/vision-ai-worker/app/schemas/contracts.py  -- add

class BindCapabilityRequest(BaseModel):
    modelId: str


class LibraryResponse(BaseModel):
    models: List[VisionModelRecord]
    libraryBytes: int
    libraryWarn: bool


class CapabilityBindingView(BaseModel):
    capabilityCode: str
    modelId: Optional[str] = None
    candidates: List[str] = Field(default_factory=list)
```

```python
# apps/vision-ai-worker/app/routers/models.py  -- replace the list endpoint and
# add the rest. Route order matters: the literal paths below must be declared
# BEFORE the legacy "/{capability}" stubs or FastAPI will match those first.

@router.get("/kinds")
async def list_kinds():
    return {"kinds": kind_catalog()}


@router.get("", response_model=LibraryResponse, response_model_by_alias=False)
async def list_library():
    try:
        rows = await db.list_models()
        total = await db.library_bytes()
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    return LibraryResponse(
        models=[VisionModelRecord(**r) for r in rows],
        libraryBytes=total,
        libraryWarn=total > settings.model_library_warn_bytes,
    )


@router.get("/capabilities")
async def list_capabilities():
    try:
        rows = await db.list_models()
        bindings = await db.list_bindings()
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc

    candidates: dict[str, list] = {}
    for row in rows:
        for capability in row.get("produces") or [row["capability_code"]]:
            candidates.setdefault(capability, []).append(row["id"])

    return {
        "capabilities": [
            {"capabilityCode": capability, "modelId": bindings.get(capability), "candidates": ids}
            for capability, ids in sorted(candidates.items())
        ]
    }


@router.put("/capabilities/{capability}/binding")
async def bind_capability(capability: str, body: BindCapabilityRequest):
    try:
        model = await db.get_model_by_id(body.modelId)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    if model is None:
        raise HTTPException(status_code=404, detail=f"No model with id '{body.modelId}'")

    produces = model.get("produces") or [model["capability_code"]]
    if capability not in produces:
        raise HTTPException(
            status_code=422,
            detail=f"model {body.modelId} does not produce '{capability}' (it produces {', '.join(produces)})",
        )

    await db.set_binding(capability, body.modelId, actor=None)
    return {"capabilityCode": capability, "modelId": body.modelId}


@router.delete("/capabilities/{capability}/binding")
async def unbind_capability(capability: str):
    try:
        cleared = await db.clear_binding(capability)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    if not cleared:
        raise HTTPException(status_code=404, detail=f"'{capability}' is not bound to any model")
    return {"capabilityCode": capability, "modelId": None}


@router.get("/capabilities/{capability}/history")
async def capability_history(capability: str):
    try:
        return {"history": await db.list_activation_log(capability)}
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc


@router.patch("/models/{model_id}", response_model=VisionModelRecord, response_model_by_alias=False)
async def update_model(model_id: str, body: UpdateModelRequest):
    spec_dict = body.runtimeSpec
    if spec_dict is not None:
        model = await db.get_model_by_id(model_id)
        if model is None:
            raise HTTPException(status_code=404, detail=f"No model with id '{model_id}'")
        try:
            spec = parse_runtime_spec(spec_dict)
        except SpecError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
        data = await storage.download_bytes(model["s3_key"])
        try:
            validate_upload(data, spec)
        except ValidationFailure as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    zones = None
    if body.applicableZones is not None:
        try:
            zones = parse_zone_list(body.applicableZones)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    row = await db.update_model_fields(model_id, runtime_spec=spec_dict, applicable_zones=zones, label=body.label)
    if row is None:
        raise HTTPException(status_code=404, detail=f"No model with id '{model_id}'")
    model_registry.invalidate(model_id)
    return VisionModelRecord(**row)


@router.delete("/models/{model_id}")
async def delete_model_entry(model_id: str):
    try:
        bindings = await db.list_bindings()
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc

    bound_to = sorted(c for c, mid in bindings.items() if mid == model_id)
    if bound_to:
        raise HTTPException(
            status_code=409,
            detail=f"model {model_id} still serves {', '.join(bound_to)}; rebind or unbind those first",
        )

    row = await db.delete_model_by_id(model_id)
    if row is None:
        raise HTTPException(status_code=404, detail=f"No model with id '{model_id}'")
    try:
        await storage.delete_object(row["s3_key"])
    except RuntimeError:
        pass  # the row is gone; a dangling object is not fatal
    model_registry.invalidate(model_id)
    return {"success": True}


@router.patch("/{capability}", deprecated=True)
async def patch_by_capability_gone(capability: str):
    raise HTTPException(
        status_code=410,
        detail="a capability no longer identifies one model; use PATCH /api/v1/models/models/{modelId}",
    )


@router.delete("/{capability}", deprecated=True)
async def delete_by_capability_gone(capability: str):
    raise HTTPException(
        status_code=410,
        detail="a capability no longer identifies one model; use DELETE /api/v1/models/models/{modelId} "
        "or DELETE /api/v1/models/capabilities/{code}/binding",
    )
```

Add `UpdateModelRequest` to contracts:

```python
class UpdateModelRequest(BaseModel):
    runtimeSpec: Optional[dict] = None
    applicableZones: Optional[List[str]] = None
    label: Optional[str] = None
```

Remove the now-dead `update_model_zones` and the old `delete_model`, and import
`settings`, `kind_catalog`, and the new contract models at the top.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pytest apps/vision-ai-worker -v`
Expected: PASS, whole suite.

- [ ] **Step 5: Drop the uniqueness constraint**

Only now — bindings are populated and resolution reads them, so no window
exists where a second upload could shadow a live model. Append to
`SCHEMA_STATEMENTS` in `app/db.py`:

```python
    "ALTER TABLE vision_models DROP CONSTRAINT IF EXISTS vision_models_capability_code_key;",
```

Run: `TEST_DATABASE_URL=postgresql://... pytest apps/vision-ai-worker/tests/test_db_schema.py -v`
Expected: PASS — `test_two_models_can_declare_the_same_capability` now exercises
the real constraint-free table.

- [ ] **Step 6: Commit**

```bash
git add apps/vision-ai-worker/app/routers/models.py apps/vision-ai-worker/app/schemas/contracts.py apps/vision-ai-worker/app/db.py apps/vision-ai-worker/tests/test_activation_api.py
git commit -m "feat: library, binding and history endpoints"
```

---

### Task 10: core-engine execution metrics

One model can now fill several capabilities in one run, so counting a
capability per zone as an "executed model" over-reports. Nothing else on the Go
side changes: the dispatch contract is unchanged.

**Files:**
- Modify: `apps/core-engine/pkg/vision/orchestrator/dag_engine.go:180-215`
  (the Phase 4 block and the `ExecutionMetrics` construction)
- Test: `apps/core-engine/pkg/vision/orchestrator/dag_engine_test.go`

**Interfaces:**
- Consumes: `inference.DispatchResult` (unchanged).
- Produces: `ExecutionMetrics.ExecutedModelsCount` = distinct capabilities
  scored, not capability-zone pairs.

- [ ] **Step 1: Write the failing test**

Add to `dag_engine_test.go`, following the stub style already in that file:

```go
func TestExecutedModelsCountDoesNotDoubleCountZones(t *testing.T) {
	// One capability scored on three zones is one executed model, not three.
	capabilities := &stubCapabilityClient{
		result: &inference.DispatchResult{
			Zones: []inference.ZoneScores{
				{ZoneCode: "ZONE_FOREHEAD", Telemetry: map[string]float64{"acne": 20}},
				{ZoneCode: "ZONE_LEFT_CHEEK", Telemetry: map[string]float64{"acne": 30}},
				{ZoneCode: "ZONE_CHIN", Telemetry: map[string]float64{"acne": 40}},
			},
		},
	}

	orch := NewVisionOrchestrator(newStubSegmenter(3), stubOcclusion{}, stubResolver{
		plan: &catalog.Plan{Capabilities: []string{"acne"}},
	}, capabilities)

	resp, err := orch.Execute(context.Background(), &domain.AnalyzeVisionRequest{
		Images: []domain.RawImageInput{{Angle: domain.AngleFront, Data: testJPEG(t)}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := resp.ExecutionMetrics.ExecutedModelsCount; got != 1 {
		t.Errorf("ExecutedModelsCount = %d, want 1", got)
	}
	if got := resp.ExecutionMetrics.SkippedModelsCount; got != 0 {
		t.Errorf("SkippedModelsCount = %d, want 0", got)
	}
}
```

Reuse the existing helpers in that file for `stubCapabilityClient`,
`stubResolver`, `stubOcclusion`, `newStubSegmenter` and `testJPEG`. If a helper
does not exist under that exact name, use the file's equivalent rather than
adding a duplicate.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/core-engine/pkg/vision/orchestrator/ -run TestExecutedModelsCount -v`
Expected: FAIL — `ExecutedModelsCount = 3, want 1`

- [ ] **Step 3: Write the implementation**

In `dag_engine.go`, delete `executedCount` and its `executedCount++` inside the
telemetry loop, then build the count from the distinct set that already exists:

```go
	// Phase 4 ... (unchanged above)
	scoredCapabilities := make(map[string]bool)
	var unsupportedCapabilities []string
```

```go
			for capability, severity := range zoneScores.Telemetry {
				health[capability] = SeverityToHealth(severity)
				scoredCapabilities[capability] = true
			}
```

```go
		ExecutionMetrics: domain.ExecutionMetrics{
			TotalLatencyMs:      time.Since(start).Milliseconds(),
			ExecutedModelsCount: len(executedModels),
			SkippedModelsCount:  len(plan.Capabilities) - len(executedModels),
			ExecutedModels:      executedModels,
		},
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/core-engine/pkg/vision/... -v`
Expected: PASS. Then `npm run test:go` for the whole Go side.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/pkg/vision/orchestrator/dag_engine.go apps/core-engine/pkg/vision/orchestrator/dag_engine_test.go
git commit -m "fix: count executed models once, not once per zone"
```

---

### Task 11: Dispatch benchmark

The repo has Go benchmarks for scoring and none for inference. This one makes
the batching claim measurable rather than asserted.

**Files:**
- Create: `apps/vision-ai-worker/tests/test_dispatch_benchmark.py`
- Modify: `apps/vision-ai-worker/requirements.txt`, `pyproject.toml`
  (add `pytest-benchmark>=4.0`)
- Modify: `package.json` (add `bench:vision` script)

**Interfaces:**
- Consumes: Tasks 3 and 4.
- Produces: `npm run bench:vision`.

- [ ] **Step 1: Write the benchmark**

```python
# apps/vision-ai-worker/tests/test_dispatch_benchmark.py
"""Measures the dispatch shape the design is budgeted against: 8 zones x 5
capabilities. Run with: npm run bench:vision

Wall-clock only -- this says nothing about a model's accuracy."""

import asyncio

import cv2
import numpy as np
import pytest

from app.models import model_registry
from app.models.spec import parse_runtime_spec
from tests.test_model_registry import tiny_onnx

ZONES = 8
CAPABILITIES = 5


def crop() -> bytes:
    ok, enc = cv2.imencode(".jpg", (np.random.rand(320, 320, 3) * 255).astype(np.uint8))
    assert ok
    return enc.tobytes()


@pytest.fixture
def loaded(monkeypatch):
    blob = tiny_onnx(224)

    async def fake_download(_key):
        return blob

    monkeypatch.setattr(model_registry.storage, "download_bytes", fake_download)
    model_registry.reset_cache()
    return blob


def _spec():
    return parse_runtime_spec(
        {
            "specVersion": 1,
            "input": {"width": 224, "height": 224},
            "output": {"kind": "severity_scalar", "capability": "acne"},
        }
    )


def test_batched_dispatch(benchmark, loaded):
    crops = [crop() for _ in range(ZONES)]
    spec = _spec()

    def run():
        asyncio.run(model_registry.run_model("m1", "models/a.onnx", spec, crops))

    benchmark(run)


def test_one_at_a_time_dispatch(benchmark, loaded):
    """The shape dispatch had before this work: one call per zone."""
    crops = [crop() for _ in range(ZONES)]
    spec = _spec()

    def run():
        async def go():
            for c in crops:
                await model_registry.run_model("m1", "models/a.onnx", spec, [c])

        asyncio.run(go())

    benchmark(run)
```

- [ ] **Step 2: Run it to see both numbers**

Add to `package.json` scripts:

```json
    "bench:vision": "pytest apps/vision-ai-worker/tests/test_dispatch_benchmark.py --benchmark-only",
```

Run: `npm run bench:vision`
Expected: both benchmarks report. Record the two medians in the commit message.
The batched case should be the faster of the two; if it is not, say so plainly
rather than adjusting the benchmark — a batching change that does not pay is a
finding, and Task 7's design assumption would need revisiting.

- [ ] **Step 3: Commit**

```bash
git add apps/vision-ai-worker/tests/test_dispatch_benchmark.py apps/vision-ai-worker/requirements.txt apps/vision-ai-worker/pyproject.toml package.json
git commit -m "test: benchmark batched vs per-zone dispatch"
```

---

### Task 12: Documentation and the cross-repository handoff

**Files:**
- Modify: `docs/SHARED-DATABASE.md`
- Create: `docs/MODEL-REGISTRY.md`
- Modify: `apps/vision-ai-worker/README.md` if one exists; otherwise skip
- Modify: `docs/LANES.md` (release the lanes)

- [ ] **Step 1: Write `docs/MODEL-REGISTRY.md`**

Cover, in this order, with no placeholders:

1. **What the registry is** — a library of uploaded models plus one binding per
   capability deciding which serves it.
2. **The runtime spec** — every field, its meaning, and which output kinds take
   it. Copy the field table from the design doc rather than paraphrasing.
3. **Uploading** — the `POST /upload` form fields, what validation checks, and
   what each failure code means (400 malformed, 422 model/spec disagreement,
   503 storage or database down).
4. **Activating and rolling back** — the binding endpoints, and that rollback is
   a bind to an older model id.
5. **What a capability with no binding does** — absent from results, reported
   in `unsupportedCapabilities`, never zero.
6. **The limits** — the four env vars, their defaults, and the measured
   latencies that justify them (copy the table from the design doc).
7. **Legacy rows** — `runtime_spec = NULL` means `LEGACY_SPEC`.

- [ ] **Step 2: Add the registry tables to `docs/SHARED-DATABASE.md`**

Add `vision_capability_bindings` and `vision_capability_activation_log` to
whatever table inventory that document keeps, each marked owned by
`vision-ai-worker`, created by raw SQL in `app/db.py`, not GORM, and not read by
any other repository. Note the dropped uniqueness constraint on
`vision_models.capability_code` and why.

- [ ] **Step 3: Write the Seagull-web handoff note**

`docs/MODEL-REGISTRY.md` ends with a section titled "What Seagull-web must
change", listing: the two 410 paths and their replacements, the new
`activate` form field, and the three screens the endpoints support (library,
per-capability activation, history). State plainly that the work is raised and
done in that repository, not from here.

- [ ] **Step 4: Release the lanes**

Delete this session's rows from the Active claims table in `docs/LANES.md`.

- [ ] **Step 5: Commit**

```bash
git add docs/MODEL-REGISTRY.md docs/SHARED-DATABASE.md docs/LANES.md
git commit -m "docs: model registry usage and the shared-database entries"
```

---

## Verification

Before calling this done, run all three suites and paste the output:

```bash
npm run test:go
```

```bash
pytest apps/vision-ai-worker -v
```

```bash
npm run bench:vision
```

Expected: Go green, worker suite green, both benchmarks reporting. The try-on
suite (`npm run test:tryon`, ~7 minutes) is untouched by this work and need not
run.

Do not claim completion on any suite you have not run and read.

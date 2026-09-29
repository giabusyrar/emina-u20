"""Writes core-engine's recorded copy of the ai-worker's region table.

Run from the repository root (any OS; no PYTHONPATH needed):
    python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py \
        > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
anchors.py imports nothing, so this needs no numpy or model.
"""
import hashlib
import json
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, "apps/ai-worker")

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

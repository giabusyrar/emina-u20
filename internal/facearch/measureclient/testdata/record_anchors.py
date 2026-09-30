"""Writes core-engine's recorded copy of the face worker's region table.

Run from the repository root (any OS; no PYTHONPATH needed):
    python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py \
        > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
anchors.py imports nothing, so this needs no numpy or model. measures.py
imports numpy and cv2, so its CATALOGUE_VERSION (the catalogue the table is
recorded under) is parsed from the source text with a regex rather than
imported. The canonical frame's definition, frame.py, is pinned by its
SHA-256 like anchors.py.
"""
import hashlib
import json
import re
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, "apps/workers/face")

from worker_face.facegeom import anchors as A

SOURCE = "apps/workers/face/worker_face/facegeom/anchors.py"
MEASURES = "apps/workers/face/worker_face/facegeom/measures.py"
FRAME = "apps/workers/face/worker_face/facegeom/frame.py"


def lf(path):
    return open(path, "rb").read().replace(b"\r\n", b"\n")


versions = re.findall(r'^CATALOGUE_VERSION = "([^"]+)"$', lf(MEASURES).decode("utf-8"), re.M)
if len(versions) != 1:
    sys.exit(f"want exactly one CATALOGUE_VERSION assignment in {MEASURES}, found {len(versions)}")

src = lf(SOURCE)
lines = [
    "{",
    f'  "source": "{SOURCE}",',
    f'  "sha256": "{hashlib.sha256(src).hexdigest()}",',
    f'  "catalogueVersion": "{versions[0]}",',
    f'  "catalogueVersionSource": "{MEASURES}",',
    f'  "frameSource": "{FRAME}",',
    f'  "frameSha256": "{hashlib.sha256(lf(FRAME)).hexdigest()}",',
    f'  "nLandmarks": {A.N_LANDMARKS},',
    f'  "eyeCorners": {{"exoR": {A.EXO_R}, "endoR": {A.ENDO_R}, "exoL": {A.EXO_L}, "endoL": {A.ENDO_L}}},',
    '  "regions": {',
]
items = list(A.REGIONS.items())
for i, (name, idx) in enumerate(items):
    lines.append(f'    "{name}": {json.dumps(list(idx))}' + ("," if i < len(items) - 1 else ""))
lines += ["  }", "}"]
print("\n".join(lines))

"""Builds centroids_lab.json from centroids.tsv: each ISCC-NBS centroid's
Munsell notation converted to CIELAB under D65, the space shade colours are
stored in (match_shade_colorimetry lab_*).

Munsell renotation is defined under illuminant C, so the conversion is
Munsell -> xyY (C) -> XYZ -> Bradford adaptation C -> D65 -> CIELAB (D65).

Run from the repository root with colour-science installed (the colour
worker's dependency); the output is committed, so core-engine never runs it:

    python apps/core-engine/internal/colour/domain/isccnbs/build_lab.py
"""

import csv
import json
import pathlib

import colour

HERE = pathlib.Path(__file__).parent
C = colour.CCS_ILLUMINANTS["CIE 1931 2 Degree Standard Observer"]["C"]
D65 = colour.CCS_ILLUMINANTS["CIE 1931 2 Degree Standard Observer"]["D65"]


# colour-science's renotation covers Munsell value 1..9. Four chromatic
# blacks (24, 65, 114, 157) sit at value 0.8-0.9: they are converted at
# value 1 for hue and chroma, and their L* is then set from the true value's
# luminance (ASTM D1535). Those four are approximate; nothing else is.
MIN_VALUE = 1.0


def astm_d1535_luminance(value: float) -> float:
    """Luminance factor Y (0-100) of Munsell value V, ASTM D1535 polynomial."""
    v = value
    return 1.1914 * v - 0.22533 * v**2 + 0.23352 * v**3 - 0.020484 * v**4 + 0.00081939 * v**5


def _lab(spec: str) -> list[float]:
    xyY = colour.munsell_colour_to_xyY(spec)
    XYZ = colour.xyY_to_XYZ(xyY)
    XYZ_d65 = colour.chromatic_adaptation(
        XYZ, colour.xy_to_XYZ(C), colour.xy_to_XYZ(D65), method="Von Kries", transform="Bradford"
    )
    return [float(v) for v in colour.XYZ_to_Lab(XYZ_d65, D65)]


def munsell_to_lab(notation: str) -> list[float]:
    # Neutrals are written "N 9.5/" in the table; colour-science wants "N9.5".
    spec = notation.replace(" ", "") if notation.startswith("N") else notation
    spec = spec.rstrip("/")
    if not spec.startswith("N"):
        hue, vc = spec.split(" ")
        value, chroma = (float(x) for x in vc.split("/"))
        if value < MIN_VALUE:
            L, a, b = _lab(f"{hue} {MIN_VALUE}/{chroma}")
            Y = astm_d1535_luminance(value) / 100
            L = 116 * Y ** (1 / 3) - 16 if Y > (6 / 29) ** 3 else Y * (29 / 3) ** 3
            return [round(L, 3), round(a, 3), round(b, 3)]
    return [round(v, 3) for v in _lab(spec)]


def main() -> None:
    rows = []
    with open(HERE / "centroids.tsv", encoding="utf-8") as f:
        data = (line for line in f if not line.startswith("#"))
        for r in csv.DictReader(data, delimiter="\t"):
            rows.append({
                "number": int(r["number"]),
                "name": r["name"],
                "munsell": r["munsell"],
                "open": r["open"] == "true",
                "lab": munsell_to_lab(r["munsell"]),
            })
    out = {
        "source": "Kelly 1958, J. Res. NBS 61(5) RP2911, Table 1 (centroids.tsv); "
                  "CIELAB D65 via colour-science, Bradford C->D65 (build_lab.py)",
        "centroids": rows,
    }
    (HERE / "centroids_lab.json").write_text(json.dumps(out, indent=1) + "\n", encoding="utf-8")
    print(f"wrote {len(rows)} centroids")


if __name__ == "__main__":
    main()

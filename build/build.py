#!/usr/bin/env python3
"""Assemble index.html from the story, the art, and the engine template.

    python3 build/build.py

Sources:
  story/v3-story.json   the branching script (scenes, options, endings)
  story/art/*.svg       13 scene illustrations + 42 answer vignettes
  build/template.html   engine, CSS, markup
  build/_mascots.js     kawaii mascot SVG generators (carried from the old build)
  build/_arch.js        the six archetypes (names, colours, strengths, paths)

Everything is inlined, so the result is one self-contained static file.
"""

import glob
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BUILD = os.path.join(ROOT, "build")
STORY = os.path.join(ROOT, "story")
ART = os.path.join(STORY, "art")
OUT = os.path.join(ROOT, "index.html")

# Time of day per scene, so the page background walks from pre-dawn to dusk.
SKY = {
    "a1_pintu": "#E6DEF8", "a2_suara": "#FFF0E2", "a2_tangan": "#FFF7E4",
    "a2_papan": "#EDF5FF", "a2_teras": "#DEEAFB", "a3_creator": "#FFEDF5",
    "a3_boss": "#F2ECFF", "a3_artisan": "#FFF7E4", "a3_stage": "#E2D4F5",
    "a3_change": "#E6F8F0", "a3_explorer": "#DCEEFF", "a4_halaman": "#FFEFC9",
    "a5_atap": "#FFE0D2",
}


def read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def js_safe(obj):
    """JSON that is safe to paste inside a <script> block."""
    return json.dumps(obj, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")


def main():
    story = json.loads(read(os.path.join(STORY, "v3-story.json")))
    scenes = {s["id"]: s for s in story["scenes"]}

    # ---- collect art, keyed the way the engine looks it up ----
    art, missing = {}, []
    for sid, sc in scenes.items():
        p = os.path.join(ART, sid + ".svg")
        if os.path.exists(p):
            art[sid] = re.sub(r"\s*\n\s*", "", read(p).strip())
        else:
            missing.append(sid + ".svg")
        for i in range(len(sc["options"])):
            key = "%s.opt%d" % (sid, i)
            p = os.path.join(ART, key + ".svg")
            if os.path.exists(p):
                art[key] = re.sub(r"\s*\n\s*", "", read(p).strip())
            else:
                missing.append(key + ".svg")

    stray = [os.path.basename(f) for f in glob.glob(os.path.join(ART, "*.svg"))
             if os.path.basename(f)[:-4] not in art]

    # ---- trim the story down to what the page actually renders ----
    slim = {
        "title": story["title"],
        "premise": story["premise"],
        "scenes": [{
            "id": s["id"],
            "narration": s["narration"],
            "options": [{"label": o["label"], "transition": o["transition"],
                         "leadsTo": o["leadsTo"], "scores": o["scores"]}
                        for o in s["options"]],
        } for s in story["scenes"]],
        "endings": [{"character": e["character"], "closing": e["closing"]}
                    for e in story["endings"]],
    }

    html = read(os.path.join(BUILD, "template.html"))
    for token, payload in (
        ("/*__MASCOTS__*/", read(os.path.join(BUILD, "_mascots.js")).strip()),
        ("/*__ARCH__*/", read(os.path.join(BUILD, "_arch.js")).strip()),
        ("/*__STORY__*/", js_safe(slim)),
        ("/*__ART__*/", js_safe(art)),
        ("/*__SKY__*/", js_safe(SKY)),
    ):
        if token not in html:
            sys.exit("template is missing %s" % token)
        html = html.replace(token, payload)

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write(html)

    print("scenes      %d" % len(slim["scenes"]))
    print("art pieces  %d" % len(art))
    print("endings     %d" % len(slim["endings"]))
    if missing:
        print("MISSING     %d -> %s" % (len(missing), ", ".join(missing[:8])))
    if stray:
        print("unused art  %d -> %s" % (len(stray), ", ".join(stray[:8])))
    print("wrote       %s (%.1f KB)" % (OUT, os.path.getsize(OUT) / 1024))
    return 1 if missing else 0


if __name__ == "__main__":
    sys.exit(main())

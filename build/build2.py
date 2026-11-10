#!/usr/bin/env python3
"""Assemble the game (v2): portrait scenes with in-scene hotspots + walking actor.

    python3 build/build2.py            # writes index.html
    python3 build/build2.py --out game.html

Sources: story/v4-story.json (falls back to v3), story/art2/*.svg, build/template2.html,
build/_mascots.js, build/_arch.js. Everything is inlined into one static file.
"""
import json, os, re, sys

ROOT  = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BUILD = os.path.join(ROOT, "build")
STORY = os.path.join(ROOT, "story")
ART   = os.path.join(STORY, "art2")
OUT   = os.path.join(ROOT, sys.argv[sys.argv.index("--out")+1] if "--out" in sys.argv else "index.html")

SKY = {
 "a1_pintu":"#CDBEF2","a2_suara":"#FFEAD9","a2_tangan":"#FFF3D6","a2_papan":"#E3F1FF","a2_teras":"#CFE2FB",
 "a3_creator":"#FFE4F0","a3_boss":"#EFE6FF","a3_artisan":"#FFF3D6","a3_deep":"#E8ECFF","a3_stage":"#8A4FD6",
 "a3_change":"#DFF6EC","a3_explorer":"#BFE2FF","a4_halaman":"#FFD86B","a5_atap":"#FFB199"}

def read(p):
    with open(p, encoding="utf-8") as fh: return fh.read()
def js(o): return json.dumps(o, ensure_ascii=False, separators=(",",":")).replace("</","<\\/")

def main():
    sp = os.path.join(STORY, "v4-story.json")
    if not os.path.exists(sp): sp = os.path.join(STORY, "v3-story.json")
    story = json.loads(read(sp))
    scenes = {s["id"]: s for s in story["scenes"]}
    art, missing = {}, []
    for sid in scenes:
        p = os.path.join(ART, sid + ".svg")
        if os.path.exists(p): art[sid] = re.sub(r"\s*\n\s*", "", read(p).strip())
        else: missing.append(sid)
    slim = {"title": story["title"], "premise": story["premise"],
            "scenes": [{"id": s["id"], "narration": s["narration"],
                        "options": [{"label": o["label"], "transition": o["transition"], "leadsTo": o["leadsTo"],
                                     "scores": o["scores"], "sound": o.get("sound","")} for o in s["options"]]}
                       for s in story["scenes"]],
            "endings": [{"character": e["character"], "closing": e["closing"]} for e in story["endings"]]}
    html = read(os.path.join(BUILD, "template2.html"))
    for tok, payload in (("/*__MASCOTS__*/", read(os.path.join(BUILD,"_mascots.js")).strip()),
                         ("/*__ARCH__*/",    read(os.path.join(BUILD,"_arch.js")).strip()),
                         ("/*__STORY__*/",   js(slim)), ("/*__ART__*/", js(art)),
                         ("/*__SKY__*/",     js(SKY)),  ("/*__FLOOR__*/", js(SKY))):
        if tok not in html: sys.exit("template missing " + tok)
        html = html.replace(tok, payload)
    with open(OUT, "w", encoding="utf-8") as fh: fh.write(html)
    print("story   %s" % os.path.basename(sp))
    print("scenes  %d   art %d   endings %d" % (len(slim["scenes"]), len(art), len(slim["endings"])))
    if missing: print("MISSING art for: " + ", ".join(missing))
    print("wrote   %s (%.1f KB)" % (OUT, os.path.getsize(OUT)/1024))
    return 1 if missing else 0

if __name__ == "__main__": sys.exit(main())

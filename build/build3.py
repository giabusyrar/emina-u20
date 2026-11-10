#!/usr/bin/env python3
"""Assemble index.html for the H-1 visual-novel engine.

    python3 build/build3.py            -> index.html
    python3 build/build3.py --out x.html

Sources: story/v5-story.json, build/template3.html, build/_arch.js, build/_mascots.js.
Scene pictures are NOT inlined: the page loads art/<scene>.jpg next to index.html
(and art/hero.jpg on the welcome screen). Missing pictures fall back to a flat colour.
"""
import json, os, sys, time, glob

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
B = os.path.join(ROOT, "build"); S = os.path.join(ROOT, "story")
FB = {"a0_gerbang":"#CFE2FB","a1_aula":"#E6DEF8","a2_medsos":"#EDF5FF","a2_panggung":"#FFEAD9","a2_dana":"#EFE6FF","a2_band":"#E3F1FF",
      "a3_creator":"#FFE4F0","a3_boss":"#EFE6FF","a3_artisan":"#FFF3D6","a3_deep":"#E8ECFF","a3_change":"#DFF6EC",
      "a3_explorer":"#BFE2FF","a4_gelap":"#2A1F45","a5_malam":"#FF9DC0"}

def read(p):
    with open(p, encoding="utf-8") as fh: return fh.read()
def js(o): return json.dumps(o, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")

def main():
    out = os.path.join(ROOT, "index.html")
    if "--out" in sys.argv: out = os.path.join(ROOT, sys.argv[sys.argv.index("--out") + 1])
    story = json.load(open(os.path.join(S, "v5-story.json"), encoding="utf-8"))
    slim = {k: story[k] for k in ("title", "subtitle", "cast", "welcome", "endings", "start", "note") if k in story}
    slim["scenes"] = [{k: v for k, v in sc.items() if k not in ("imagePrompt", "sceneryDesc")} for sc in story["scenes"]]
    pj = os.path.join(ROOT, "art", "char", "poses.json")
    sprites = json.load(open(pj)) if os.path.exists(pj) else None
    html = read(os.path.join(B, "template3.html"))
    for tok, payload in (("/*__MASCOTS__*/", read(os.path.join(B, "_mascots.js")).strip()),
                         ("/*__ARCH__*/", read(os.path.join(B, "_arch.js")).strip()),
                         ("/*__STORY__*/", js(slim)), ("/*__FB__*/", js(FB)), ("/*__SPRITES__*/", js(sprites)), ("__VER__", str(int(time.time()))[-6:])):
        if tok not in html: sys.exit("template is missing " + tok)
        html = html.replace(tok, payload)
    with open(out, "w", encoding="utf-8") as fh: fh.write(html)
    have = {os.path.basename(p)[:-4] for p in glob.glob(os.path.join(ROOT, "art", "*.jpg"))}
    need = [sc["id"] for sc in story["scenes"]] + ["hero"]
    missing = [n for n in need if n not in have]
    print("scenes %d  endings %d  art %d/%d" % (len(slim["scenes"]), len(slim["endings"]), len(need) - len(missing), len(need)))
    if missing: print("no picture yet:", ", ".join(missing))
    print("wrote", out, "(%.1f KB)" % (os.path.getsize(out) / 1024))

if __name__ == "__main__": main()

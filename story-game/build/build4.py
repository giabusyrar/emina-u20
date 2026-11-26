#!/usr/bin/env python3
"""Assemble index.html for the Explorer Quiz (story/v6-explorer.json + story/event.json + build/template4.html)."""
import json, os, sys, time, glob
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
B = os.path.join(ROOT, "build"); S = os.path.join(ROOT, "story")
FB = {"c1":"#FFE9D6","c2":"#DCEFFF","c3":"#FFF1E6","c4":"#2E3A2E","c5":"#3A2535","c6":"#E6E0FF","c7":"#D8E4FF"}
def read(p):
    with open(p, encoding="utf-8") as fh: return fh.read()
def js(o): return json.dumps(o, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")
def main():
    out = os.path.join(ROOT, "index.html")
    if "--out" in sys.argv: out = os.path.join(ROOT, sys.argv[sys.argv.index("--out") + 1])
    story = json.load(open(os.path.join(S, "v6-explorer.json"), encoding="utf-8"))
    event = json.load(open(os.path.join(S, "event.json"), encoding="utf-8"))
    def png_size(fp):
        with open(fp, "rb") as fh:
            fh.read(16); import struct; return struct.unpack(">II", fh.read(8))
    hero = os.path.join(ROOT, "art", "char", "hero.png")
    simple = os.path.exists(hero)
    if simple:
        w, h = png_size(hero); sprites = [{"src": "art/char/hero.png", "w": w, "h": h}]
    else:
        pj = os.path.join(ROOT, "art", "char", "poses.json"); sprites = json.load(open(pj)) if os.path.exists(pj) else None
    typeimg = {k: "art/char/t_%s.png" % k for k in story["types"] if os.path.exists(os.path.join(ROOT, "art", "char", "t_%s.png" % k))}
    html = read(os.path.join(B, "template4.html"))
    for tok, payload in (("/*__STORY__*/", js(story)), ("/*__EVENT__*/", js(event)), ("/*__FB__*/", js(FB)), ("/*__SPRITES__*/", js(sprites)), ("/*__SIMPLE__*/", "true" if simple else "false"), ("/*__TYPEIMG__*/", js(typeimg)), ("__VER__", str(int(time.time()))[-6:])):
        if tok not in html: sys.exit("template is missing " + tok)
        html = html.replace(tok, payload)
    with open(out, "w", encoding="utf-8") as fh: fh.write(html)
    have = [sc["id"] for sc in story["scenes"] if os.path.exists(os.path.join(ROOT, sc["img"]))]
    print("scenes %d  art %d/%d  type images %d  sprites %s" % (len(story["scenes"]), len(have), len(story["scenes"]), len(typeimg), "yes" if sprites else "no"))
    print("wrote", out, "(%.1f KB)" % (os.path.getsize(out) / 1024))
if __name__ == "__main__": main()

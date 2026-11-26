#!/usr/bin/env python3
"""Chroma-key art/char/sheet.png (4x4 grid on #00FF00) into 16 trimmed transparent PNGs + poses.json."""
import json, os
from PIL import Image
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
D = os.path.join(ROOT, "art", "char")
im = Image.open(os.path.join(D, "sheet.png")).convert("RGBA")
W, H = im.size; cell = W // 4
px = im.load()
for y in range(H):
    for x in range(W):
        r, g, b, a = px[x, y]
        m = max(r, b)
        if g > m:
            d = (g - m) / 255.0
            k = min(1.0, max(0.0, (d - 0.16) / 0.26))      # 0 keep .. 1 remove
            if k > 0:
                na = int(a * (1 - k))
                px[x, y] = (r, min(g, m + 12), b, na)       # despill the fringe
poses = []
for i in range(16):
    r, c = divmod(i, 4)
    tile = im.crop((c * cell, r * cell, (c + 1) * cell, (r + 1) * cell))
    bb = tile.getchannel("A").point(lambda v: 255 if v > 40 else 0).getbbox()
    tile = tile.crop(bb)
    name = "p%02d.png" % i
    tile.save(os.path.join(D, name), optimize=True)
    poses.append({"src": "art/char/" + name, "w": tile.width, "h": tile.height})
json.dump(poses, open(os.path.join(D, "poses.json"), "w"), indent=0)
tot = sum(os.path.getsize(os.path.join(D, p["src"].split("/")[-1])) for p in poses)
print("16 poses, total %.0f KB" % (tot / 1024), [(p["w"], p["h"]) for p in poses])

#!/usr/bin/env python3
"""Generate the mascot character sheet (16 poses, 4x4 grid, flat green key) in ONE hit.

    python3 build/gen_char.py --key-file ~/.openrouter.key            # -> art/char/sheet.png
    python3 build/gen_char.py --key-file ... --force                   # regenerate (deletes the old sheet)

References sent with the prompt: art/char/_ref_mascot.png (the Emina mascot design) and
art/a1_aula.jpg (the illustration style of the scenes). Skips if the sheet exists.
"""
import argparse, base64, importlib.util, json, os, sys, time

HERE = os.path.dirname(os.path.abspath(__file__)); ROOT = os.path.dirname(HERE)
spec = importlib.util.spec_from_file_location("gen_art", os.path.join(HERE, "gen_art.py"))
g = importlib.util.module_from_spec(spec); spec.loader.exec_module(g)
OUT = os.path.join(ROOT, "art", "char", "sheet.png")

POSES = ["standing idle, facing front, arms relaxed",
         "walking, left foot forward, arms swinging",
         "walking, right foot forward, arms swinging the other way",
         "running fast, leaning slightly forward, arms pumping",
         "sitting on the floor, legs stretched out in front, hands on lap",
         "sitting on the floor listening, head tilted, one hand near its cheek",
         "standing, holding a pink smartphone up with both hands, looking at the screen",
         "standing, pink smartphone pressed to its ear, talking",
         "standing, writing with a pen on a small clipboard",
         "crouching low, picking up a cable coil from the floor, tidying",
         "standing, one arm raised high waving happily",
         "standing, singing into a handheld microphone, other arm out",
         "standing, holding a clipboard with both hands, looking to the side",
         "standing, talking with both hands gesturing open",
         "standing, holding a small flashlight pointing down-right, light cone visible",
         "standing, painting upward with a paintbrush, a little pink paint on the brush"]

PROMPT = ("Character sheet, 16 poses of the SAME mascot in a strict 4 by 4 grid of equal square cells, "
          "each cell separated by a thick empty gap, the whole background a single flat pure green #00FF00 with nothing else. "
          "The mascot is exactly the one in the first reference image: a plump white egg-shaped body, pure white, "
          "a pink bow on top of the head, two black oval eyes with white glints, soft pink blush on the cheeks, a small smile, "
          "short stubby arms with pale pink hands, short legs with pink shoes. Keep the design identical in every cell. "
          "Render it in the same illustration style as the second reference image (soft pastel shading, thin dark pink outline, clean, no sketch lines). "
          "Full body visible in every cell, same scale in every cell, feet near the bottom of each cell, nothing cropped. "
          "No text, no numbers, no labels, no ground shadows, no props outside what each pose describes. "
          "Row by row, left to right: " + "; ".join("%d) %s" % (i + 1, p) for i, p in enumerate(POSES)) + ".")

def main():
    ap = argparse.ArgumentParser(); ap.add_argument("--key-file", default=os.environ.get("OPENROUTER_KEY_FILE"))
    ap.add_argument("--model", default="auto"); ap.add_argument("--force", action="store_true"); ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    if a.dry_run: print(PROMPT); return
    if os.path.exists(OUT) and not a.force: print("sheet exists, use --force to regenerate"); return
    key = g.read_key(a.key_file); model = g.pick_model(key, a.model); print("model", model)
    refs = [os.path.join(ROOT, "art", "char", "_ref_mascot.png"), os.path.join(ROOT, "art", "a1_aula.jpg")]
    content = [{"type": "text", "text": PROMPT}] + [{"type": "image_url", "image_url": {"url": g.data_url(r)}} for r in refs]
    body = {"model": model, "messages": [{"role": "user", "content": content}], "modalities": ["image", "text"], "image_config": {"aspect_ratio": "1:1"}}
    t = time.time()
    res = g.call(key, "/chat/completions", body)
    msg = res["choices"][0]["message"]; imgs = msg.get("images") or []
    if not imgs: sys.exit("no image: " + (msg.get("content") or "")[:200])
    png = base64.b64decode(imgs[0]["image_url"]["url"].split(",", 1)[1])
    if os.path.exists(OUT): os.remove(OUT)
    open(OUT, "wb").write(png)
    print("ok sheet.png %.0f KB %.1fs cost=%s" % (len(png) / 1024, time.time() - t, res.get("usage", {}).get("cost")))

if __name__ == "__main__": main()

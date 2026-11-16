#!/usr/bin/env python3
"""Explorer Quiz scenes: one Gemini hit per chapter, each with the deck's reference image for that
chapter plus (from c2 on) the generated c1 as the style anchor. Writes art/explorer/c1..c7.jpg.
    python3 build/gen_explorer.py --key-file ~/.openrouter.key [--only c3,c4] [--refdir DIR]
"""
import argparse, importlib.util, os, sys, time
HERE = os.path.dirname(os.path.abspath(__file__)); ROOT = os.path.dirname(HERE)
spec = importlib.util.spec_from_file_location("gen_art", os.path.join(HERE, "gen_art.py"))
g = importlib.util.module_from_spec(spec); spec.loader.exec_module(g)
OUT = os.path.join(ROOT, "art", "explorer")
STYLE = ("Soft storybook illustration, warm pastel colours with gentle Emina pink accents, painterly but clean shading, "
         "dreamy light, cute and inviting, consistent with a teen girls' brand. Portrait 4:5 framing. No text, no letters, no logos, no watermark. "
         "Do NOT draw a main character or protagonist: the viewer is the explorer. The bottom fifth of the frame is open ground "
         "where a small mascot could stand. ")
SCENES = {
 "c1": ("05", "A cosy attic bedroom at sunrise, like the attached reference: a wooden desk by a window overlooking green hills and a winding path, pastel curtains, pinned drawings on the walls, a bookshelf, a bed with a floral quilt. On the desk a sealed envelope with a pink wax seal glowing softly, sparkles around it. Nobody in the room."),
 "c2": ("06", "A whimsical pastel street full of open doors, like the attached reference: candy-coloured shops, a music shop with notes drifting out, an art studio with paint splashes, a games arcade with glowing signs, a tech lab with glowing screens, bunting and fluffy clouds, cobblestone road in the foreground. A few small distant people are fine."),
 "c3": ("07", "A bright creative workshop like the attached reference: a long table covered in paints, brushes, fabric, beads and ribbons, a small camera on a tripod, a mannequin with accessories, a sketchbook, plants, a string of stars, a window with soft light, an hourglass on the table. Nobody at the table."),
 "c4": ("08", "An old wooden door standing half open in a dark leafy garden, like the attached reference: warm golden light and floating sparkles pouring through the gap, ferns and leaves on both sides, a stone path leading to the door, a mood of hesitation at the threshold. Nobody in the frame."),
 "c5": ("09", "A warm theatre seen from the back of the audience, like the attached reference: a lone speaker far away on stage under a spotlight, small and back-lit so the face is indistinct, red curtains, rows of audience silhouettes seen from behind, golden lights and confetti sparkles in the air."),
 "c6": ("10", "A magical arched doorway opening onto a glowing new world, like the attached reference: wisteria and giant leaves around a stone arch, through the door a luminous forest with floating lights and rainbow mist, flowers in the foreground, a stone path leading in. Nobody in the frame."),
 "c7": ("11", "A dreamy explorer map of floating islands in a pastel night sky, like the attached reference: a castle island, a waterfall, a rainbow river, a small village, a tree with lanterns, clouds and stars, a winding path connecting the islands, a blank wooden signpost in the foreground. No labels, no text, no astronaut."),
}
def main():
    ap = argparse.ArgumentParser(); ap.add_argument("--key-file", default=os.environ.get("OPENROUTER_KEY_FILE"))
    ap.add_argument("--only", default=""); ap.add_argument("--model", default="auto")
    ap.add_argument("--refdir", default="/private/tmp/claude-503/-Users-jason-suhendroparacorpgroup-com-Documents-Projects/496eb5d0-98ca-41de-aa81-28f7c0484f6f/scratchpad/deck")
    a = ap.parse_args()
    key = g.read_key(a.key_file); model = g.pick_model(key, a.model); print("model", model)
    only = {x for x in a.only.split(",") if x}; os.makedirs(OUT, exist_ok=True)
    anchor = os.path.join(OUT, "c1.jpg")
    for cid, (slide, desc) in SCENES.items():
        if only and cid not in only: continue
        jpg = os.path.join(OUT, cid + ".jpg")
        if os.path.exists(jpg): print("skip ", cid, "exists"); continue
        ref = os.path.join(a.refdir, "ref_slide%s.png" % slide)
        imgs = [ref] + ([anchor] if cid != "c1" and os.path.exists(anchor) else [])
        prompt = STYLE + desc + (" Match the rendering style, palette and brushwork of the second attached image (our previous scene) so all scenes feel like one set." if len(imgs) > 1 else "")
        content = [{"type": "text", "text": prompt}] + [{"type": "image_url", "image_url": {"url": g.data_url(p)}} for p in imgs]
        body = {"model": model, "messages": [{"role": "user", "content": content}], "modalities": ["image", "text"], "image_config": {"aspect_ratio": "4:5"}}
        t = time.time()
        try:
            res = g.call(key, "/chat/completions", body); msg = res["choices"][0]["message"]; im = (msg.get("images") or [])
            if not im: raise RuntimeError("no image: " + (msg.get("content") or "")[:120])
            import base64; png = base64.b64decode(im[0]["image_url"]["url"].split(",", 1)[1])
        except Exception as e:
            print("FAIL ", cid, str(e)[:200]); continue
        tmp = jpg[:-4] + ".png"; open(tmp, "wb").write(png); g.to_jpeg(tmp, jpg)
        print("ok   ", cid, "%.0f KB %.1fs cost=%s" % (os.path.getsize(jpg) / 1024, time.time() - t, res.get("usage", {}).get("cost")))
if __name__ == "__main__": main()

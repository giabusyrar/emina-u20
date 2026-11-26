#!/usr/bin/env python3
"""Generate one illustration per scene through OpenRouter. ONE API hit per image.

    python3 build/gen_art.py --key-file /path/to/openrouter.key            # all missing scenes + hero
    python3 build/gen_art.py --key-file ... --only a1_aula,a4_gelap          # a subset
    python3 build/gen_art.py --key-file ... --ref art/a1_aula.jpg            # pass a style reference image
    python3 build/gen_art.py --key-file ... --list-models                    # which models output images

Rules baked in:
  * a scene whose art/<id>.jpg already exists is skipped (no accidental re-hits);
  * there is no retry: a failure prints and moves on;
  * the key is read in place from --key-file (or $OPENROUTER_KEY_FILE) and never printed.
Prompts come from story/v5-story.json: `style` + each scene's `imagePrompt`, plus `heroPrompt`.
PNG from the API is converted to JPEG with macOS `sips` (no Pillow needed) and the PNG removed.
"""
import argparse, base64, json, os, subprocess, sys, time, urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
STORY = os.path.join(ROOT, "story", "v5-story.json")
OUT = os.path.join(ROOT, "art")
API = "https://openrouter.ai/api/v1"
DEFAULT_MODEL = "auto"


def read_key(path):
    """The file may hold the bare key, or .env lines (OPENROUTER_API_KEY=...). Never printed."""
    if not path:
        sys.exit("no key file: pass --key-file or set OPENROUTER_KEY_FILE")
    with open(os.path.expanduser(path), encoding="utf-8") as fh:
        lines = [l.strip() for l in fh.read().splitlines() if l.strip() and not l.strip().startswith("#")]
    for l in lines:
        if "OPENROUTER" in l.upper() and "=" in l:
            return l.split("=", 1)[1].strip().strip('"').strip("'")
    for l in lines:
        if "=" not in l:
            return l
    sys.exit("no key found in file")


PREFERRED = ("google/gemini-3-pro-image-preview", "google/gemini-3-pro-image", "google/gemini-2.5-flash-image",
             "google/gemini-2.5-flash-image-preview")


def pick_model(key, want):
    """auto: best Gemini image model this account can reach (reference images keep the set consistent)."""
    ids = {m["id"] for m in call(key, "/models")["data"]
           if "image" in (m.get("architecture", {}).get("output_modalities") or [])}
    if want == "auto":
        for m in PREFERRED:
            if m in ids:
                return m
        sys.exit("no Gemini image model available; run --list-models")
    if want == "gpt":
        for m in ("openai/gpt-5-image", "openai/gpt-image-1.5", "openai/gpt-image-1"):
            if m in ids:
                return m
        sys.exit("no GPT image model available; run --list-models")
    if want not in ids:
        sys.exit("model %s does not output images; run --list-models" % want)
    return want


def call(key, path, body=None):
    req = urllib.request.Request(API + path, method="POST" if body else "GET")
    req.add_header("Authorization", "Bearer " + key)
    req.add_header("Content-Type", "application/json")
    req.add_header("HTTP-Referer", "https://emina-20-under-20.vercel.app")
    req.add_header("X-Title", "Emina 20 Under 20 H-1")
    data = json.dumps(body).encode() if body else None
    with urllib.request.urlopen(req, data=data, timeout=180) as r:
        return json.loads(r.read().decode())


def list_models(key):
    ms = call(key, "/models")["data"]
    for m in ms:
        if "image" in (m.get("architecture", {}).get("output_modalities") or []):
            p = m.get("pricing", {})
            print("%-50s  image=%s  prompt=%s" % (m["id"], p.get("image"), p.get("prompt")))


def data_url(path):
    with open(path, "rb") as fh:
        b = base64.b64encode(fh.read()).decode()
    ext = os.path.splitext(path)[1].lower().lstrip(".")
    return "data:image/%s;base64,%s" % ("jpeg" if ext in ("jpg", "jpeg") else ext, b)


def generate(key, model, prompt, ref, aspect):
    content = [{"type": "text", "text": prompt}]
    if ref:
        content.append({"type": "image_url", "image_url": {"url": data_url(ref)}})
    body = {"model": model, "messages": [{"role": "user", "content": content}],
            "modalities": ["image", "text"], "image_config": {"aspect_ratio": aspect}}
    res = call(key, "/chat/completions", body)
    msg = res["choices"][0]["message"]
    imgs = msg.get("images") or []
    if not imgs:
        raise RuntimeError("no image in response: " + (msg.get("content") or "")[:200])
    url = imgs[0]["image_url"]["url"]
    head, b64 = url.split(",", 1)
    return base64.b64decode(b64), res.get("usage", {})


def to_jpeg(png_path, jpg_path, quality=72):
    subprocess.run(["sips", "-s", "format", "jpeg", "-s", "formatOptions", str(quality), png_path, "--out", jpg_path],
                   check=True, capture_output=True)
    os.remove(png_path)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--key-file", default=os.environ.get("OPENROUTER_KEY_FILE"))
    ap.add_argument("--model", default=DEFAULT_MODEL, help="auto (best Gemini), gpt, or an exact OpenRouter id")
    ap.add_argument("--only", default="")
    ap.add_argument("--ref", default="", help="reference image for style consistency")
    ap.add_argument("--aspect", default="4:5")
    ap.add_argument("--list-models", action="store_true")
    ap.add_argument("--dry-run", action="store_true", help="print prompts, hit nothing")
    a = ap.parse_args()

    key = read_key(a.key_file) if not a.dry_run else ""
    if a.list_models:
        return list_models(key)

    model = a.model if a.dry_run else pick_model(key, a.model)
    print("model", model)
    story = json.load(open(STORY, encoding="utf-8"))
    style = story["style"]
    jobs = [(s["id"], s["imagePrompt"]) for s in story["scenes"]]
    if story.get("heroPrompt"):
        jobs.append(("hero", story["heroPrompt"]))
    only = {x for x in a.only.split(",") if x}
    os.makedirs(OUT, exist_ok=True)

    ref = a.ref or ""
    for sid, scene_prompt in jobs:
        if only and sid not in only:
            continue
        jpg = os.path.join(OUT, sid + ".jpg")
        if os.path.exists(jpg):
            print("skip  %-12s exists" % sid)
            continue
        prompt = style + " " + scene_prompt
        if ref:
            prompt += " Match the visual style, palette and character designs of the attached reference image exactly."
        if a.dry_run:
            print("\n[%s]\n%s" % (sid, prompt))
            continue
        t = time.time()
        try:
            png, usage = generate(key, model, prompt, ref, a.aspect)
        except Exception as e:  # one hit only; report and move on
            print("FAIL  %-12s %s" % (sid, str(e)[:300]))
            continue
        tmp = os.path.join(OUT, sid + ".png")
        with open(tmp, "wb") as fh:
            fh.write(png)
        to_jpeg(tmp, jpg)
        print("ok    %-12s %6.1f KB  %4.1fs  cost=%s" % (sid, os.path.getsize(jpg) / 1024, time.time() - t, usage.get("cost")))
        if not ref:
            ref = jpg  # the first image anchors the style of the rest


if __name__ == "__main__":
    main()

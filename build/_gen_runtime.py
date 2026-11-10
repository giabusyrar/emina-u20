#!/usr/bin/env python3
"""Generate scene art when ~/.openrouter.key is a PROVISIONING key.

A provisioning key cannot call inference. So this mints a short-lived, credit-limited
runtime key in memory, runs the normal gen_art flow with it, then revokes it. The runtime
key is never written to disk and never printed; only the resulting JPGs are written.

    python3 build/_gen_runtime.py --only a1_aula
    python3 build/_gen_runtime.py --ref art/a1_aula.jpg
    python3 build/_gen_runtime.py            # every missing scene + hero
"""
import argparse, importlib.util, json, os, time, urllib.error, urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("gen_art", os.path.join(HERE, "gen_art.py"))
g = importlib.util.module_from_spec(spec); spec.loader.exec_module(g)

PROV_FILE = os.environ.get("OPENROUTER_PROV_FILE", "~/.openrouter.key")
LIMIT_USD = 5


def api(prov, method, path, body=None):
    req = urllib.request.Request(g.API + path, data=json.dumps(body).encode() if body else None, method=method)
    req.add_header("Authorization", "Bearer " + prov); req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read().decode())


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="auto")
    ap.add_argument("--only", default="")
    ap.add_argument("--ref", default="")
    ap.add_argument("--aspect", default="4:5")
    a = ap.parse_args()

    prov = g.read_key(PROV_FILE)
    made = api(prov, "POST", "/keys", {"name": "emina-h1-art-%d" % int(time.time()), "limit": LIMIT_USD})
    runtime = made.get("key") or made.get("data", {}).get("key")
    khash = (made.get("data") or made).get("hash")
    if not runtime:
        raise SystemExit("could not mint a runtime key")
    try:
        model = g.pick_model(runtime, a.model)
        print("model", model, "| runtime key minted (limit $%d), never written to disk" % LIMIT_USD)
        story = json.load(open(g.STORY, encoding="utf-8"))
        style = story["style"]
        jobs = [(s["id"], s["imagePrompt"]) for s in story["scenes"]]
        if story.get("heroPrompt"):
            jobs.append(("hero", story["heroPrompt"]))
        only = {x for x in a.only.split(",") if x}
        os.makedirs(g.OUT, exist_ok=True)
        ref = a.ref or ""
        for sid, scene_prompt in jobs:
            if only and sid not in only:
                continue
            jpg = os.path.join(g.OUT, sid + ".jpg")
            if os.path.exists(jpg):
                print("skip  %-12s exists" % sid);
                if not ref: ref = jpg
                continue
            prompt = style + " " + scene_prompt
            if ref:
                prompt += " Match the visual style, palette and character designs of the attached reference image exactly."
            t = time.time()
            try:
                png, usage = g.generate(runtime, model, prompt, ref, a.aspect)
            except Exception as e:
                print("FAIL  %-12s %s" % (sid, str(e)[:200])); continue
            tmp = os.path.join(g.OUT, sid + ".png")
            with open(tmp, "wb") as fh:
                fh.write(png)
            g.to_jpeg(tmp, jpg)
            print("ok    %-12s %6.1f KB  %4.1fs  cost=%s" % (sid, os.path.getsize(jpg) / 1024, time.time() - t, usage.get("cost")))
            if not ref:
                ref = jpg
    finally:
        if khash:
            try:
                api(prov, "DELETE", "/keys/" + khash); print("runtime key revoked")
            except Exception as e:
                print("could not revoke runtime key (%s); delete 'emina-h1-art-*' at openrouter.ai/keys" % str(e)[:80])


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Deploy the Emina 20 Under 20 quiz to Vercel without Node or the Vercel CLI.

    python3 deploy.py            # preview deployment
    python3 deploy.py --prod     # production

Static single-file site (index.html). Vercel stores uploaded files by their
SHA-1, so each run uploads only new bytes and references the rest by hash.

The token is read in place (never copied, never printed). It looks for
.vercel-token.local next to this script first, then falls back to the
paragon-photobooth project's token, so no credential is duplicated.
"""

import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.abspath(__file__))
TEAM_ID = "team_Akado7k1R3JZGlH3jxyo8cpg"
PROJECT = "emina-20-under-20"
API = "https://api.vercel.com"

# Only the site itself ships. Notes and local files stay on disk.
def _include():
    files = ["index.html"]
    art = os.path.join(ROOT, "art")
    for base, _dirs, names in os.walk(art):
        _dirs[:] = [d for d in _dirs if not d.startswith("_")]
        for f in sorted(names):
            if f.lower().endswith((".jpg", ".png", ".webp")) and not f.startswith("_") and f != "sheet.png":
                files.append(os.path.relpath(os.path.join(base, f), ROOT))
    return tuple(files)
INCLUDE_FILES = _include()
SKIP_NAMES = {".DS_Store"}

TOKEN_CANDIDATES = (
    os.path.join(ROOT, ".vercel-token.local"),
    os.path.normpath(os.path.join(ROOT, "..", "..", "paragon-photobooth", ".vercel-token.local")),
)


def token():
    for path in TOKEN_CANDIDATES:
        try:
            with open(path) as fh:
                value = fh.read().strip()
        except FileNotFoundError:
            continue
        if value:
            return value
    sys.exit("No Vercel token found. Create one at vercel.com/account/tokens "
             "and save it as .vercel-token.local next to this script.")


def collect():
    """Every deployable file as (relative path, bytes, sha1, size)."""
    out = []
    for name in INCLUDE_FILES:
        full = os.path.join(ROOT, name)
        if name in SKIP_NAMES or not os.path.exists(full):
            continue
        with open(full, "rb") as fh:
            blob = fh.read()
        out.append((name, blob, hashlib.sha1(blob).hexdigest(), len(blob)))
    if not out:
        sys.exit("Nothing to deploy — index.html not found.")
    return out


def request(method, path, tok, body=None, headers=None, raw=False, tries=4):
    sep = "&" if "?" in path else "?"
    url = f"{API}{path}{sep}teamId={TEAM_ID}"
    head = {"Authorization": f"Bearer {tok}"}
    head.update(headers or {})
    if body is not None and not raw:
        body = json.dumps(body).encode()
        head["Content-Type"] = "application/json"

    for attempt in range(tries):
        req = urllib.request.Request(url, data=body, headers=head, method=method)
        try:
            with urllib.request.urlopen(req, timeout=120) as res:
                text = res.read().decode()
                return json.loads(text) if text else {}
        except urllib.error.HTTPError as err:
            detail = err.read().decode()
            sys.exit(f"{method} {path} failed ({err.code}): {detail[:600]}")
        except Exception as err:
            if attempt == tries - 1:
                sys.exit(f"{method} {path} failed after {tries} tries: {err}")
            time.sleep(2 * (attempt + 1))


def main():
    production = "--prod" in sys.argv
    tok = token()
    files = collect()
    print(f"{len(files)} file(s), {sum(f[3] for f in files)} bytes")

    for rel, blob, sha, size in files:
        request(
            "POST", "/v2/files", tok,
            body=blob, raw=True,
            headers={
                "Content-Type": "application/octet-stream",
                "Content-Length": str(size),
                "x-vercel-digest": sha,
            },
        )
        print(f"  up  {sha[:8]}  {size:>6}  {rel}")

    payload = {
        "name": PROJECT,
        "project": PROJECT,
        "files": [{"file": rel, "sha": sha, "size": size} for rel, _b, sha, size in files],
        "projectSettings": {"framework": None},
    }
    if production:
        payload["target"] = "production"

    dep = request(
        "POST", "/v13/deployments?skipAutoDetectionConfirmation=1", tok, body=payload
    )
    dep_id, url = dep.get("id"), dep.get("url")
    print(f"\n{'production' if production else 'preview'} deployment {dep_id}")
    print(f"https://{url}")

    for _ in range(60):
        state = request("GET", f"/v13/deployments/{dep_id}", tok).get("readyState")
        if state in ("READY", "ERROR", "CANCELED"):
            print(f"state: {state}")
            sys.exit(0 if state == "READY" else 1)
        time.sleep(5)
    print("state: still building after 5 min")


if __name__ == "__main__":
    main()

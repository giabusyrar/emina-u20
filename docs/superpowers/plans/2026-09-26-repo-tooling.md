# Repo Tooling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `package.json` the single entry point for running, testing and checking Seagull-core. It gains a lint step and a Railway-equivalent build check. The Makefile is removed, and the compose file stops pointing at a service that doesn't exist.

**Architecture:**
- npm scripts stay the runner, as in both sibling repositories.
- Anything that needs env vars, temp directories or recursive deletes goes in one dependency-free Node file, `scripts/go-tools.mjs`. npm runs scripts through `cmd.exe` on Windows, so shell syntax is not portable.

**Tech Stack:** npm 11, Node 24, Go 1.25.0, docker compose (file only; Docker is not installed here).

**Spec:** `docs/superpowers/specs/2026-09-26-repo-tooling-design.md`

## Global Constraints

- **Existing script names and commands stay byte-identical:**
  - `dev`, `dev:core`, `dev:ref`, `dev:vision`, `dev:tryon`
  - `build`, `build:core`, `build:ref`
  - `test`, `test:go`, `test:vision`, `test:tryon`
  - `seed:reference`
- **New scripts** are appended after `seed:reference`: `lint`, `build:check`, `clean`, `check`.
- `scripts/go-tools.mjs` uses Node built-ins only, and defines its app list once.
- The gofmt check ignores `\r`.
- **Never `git push`** (AGENTS.md).
- **Lane claim** is already recorded in the main checkout's `docs/LANES.md`. Work in worktree `.claude/worktrees/tooling`, on branch `repo-tooling`.

---

### Task 1: `lint` catches the known gofmt violation, then passes

**Files:**
- Create: `scripts/go-tools.mjs`
- Create: `.gitattributes`
- Modify: `package.json` (append four scripts)
- Modify: `apps/core-engine/internal/vision/workers/segmenter_worker.go` (gofmt only)

**Interfaces:**
- Produces:
  - `node scripts/go-tools.mjs fmt`: exits 0 when clean; otherwise exits 1 and prints the paths.
  - `node scripts/go-tools.mjs standalone`: exits 0 when all builds pass; otherwise exits 1 and names the failing app and entry point.
  - `node scripts/go-tools.mjs clean`
  - npm scripts `lint`, `build:check`, `clean`, `check`.

- [ ] **Step 1: Write `scripts/go-tools.mjs`**

```js
// Go helpers for the npm scripts in package.json.
//
// npm runs scripts through cmd.exe on Windows, where `VAR=value cmd`,
// `$(mktemp -d)` and `rm -rf` do not work, so everything that needs env vars,
// temp directories or recursive deletes lives here. Node built-ins only.
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// The Go services and the entry points their Dockerfiles build. Railway builds
// each one from its own directory with only its go.mod `replace` to
// packages/goplatform, so go.work does not apply.
const GO_APPS = [
  { dir: "apps/core-engine", mains: ["./cmd/server"] },
  { dir: "apps/reference-service", mains: ["./cmd/server", "./cmd/seed-reference-data"] },
];

// Trees whose .go files must be gofmt-clean.
const FMT_ROOTS = ["apps/core-engine", "apps/reference-service", "packages/goplatform"];

function fmt() {
  // git pathspec `*` matches across directories, so this lists every tracked .go file.
  const files = execFileSync("git", ["ls-files", "--", ...FMT_ROOTS.map((r) => `${r}/*.go`)], {
    encoding: "utf8",
  })
    .split("\n")
    .filter(Boolean);

  const unformatted = [];
  for (const file of files) {
    // core.autocrlf checks files out with CRLF on Windows; git stores LF.
    // Compare what git stores, not what the working tree happens to hold.
    const source = readFileSync(file, "utf8").replace(/\r/g, "");
    const result = spawnSync("gofmt", ["-l"], { input: source, encoding: "utf8" });
    if (result.error || result.status !== 0) {
      console.error(`gofmt could not parse ${file}:\n${result.stderr || result.error}`);
      return 1;
    }
    if (result.stdout.trim() !== "") unformatted.push(file);
  }

  if (unformatted.length > 0) {
    console.error(`gofmt would reformat:\n  ${unformatted.join("\n  ")}`);
    return 1;
  }
  console.log(`gofmt: ${files.length} files clean`);
  return 0;
}

function standalone() {
  const env = { ...process.env, GOWORK: "off", CGO_ENABLED: "0", GOOS: "linux" };
  const outDir = mkdtempSync(join(tmpdir(), "seagull-standalone-"));
  try {
    for (const app of GO_APPS) {
      for (const main of app.mains) {
        const binary = join(outDir, `${app.dir.replaceAll("/", "_")}_${main.split("/").pop()}`);
        const result = spawnSync("go", ["build", "-o", binary, main], {
          cwd: app.dir,
          env,
          stdio: "inherit",
        });
        if (result.error || result.status !== 0) {
          console.error(`standalone build failed: ${app.dir} ${main}`);
          return 1;
        }
        console.log(`ok  ${app.dir} ${main}`);
      }
    }
    return 0;
  } finally {
    rmSync(outDir, { recursive: true, force: true });
  }
}

function clean() {
  for (const app of readdirSync("apps")) {
    const bin = join("apps", app, "bin");
    if (existsSync(bin)) {
      rmSync(bin, { recursive: true, force: true });
      console.log(`removed ${bin}`);
    }
  }
  return 0;
}

const commands = { fmt, standalone, clean };
const name = process.argv[2];
if (!commands[name]) {
  console.error(`usage: node scripts/go-tools.mjs <${Object.keys(commands).join("|")}>`);
  process.exit(2);
}
process.exitCode = commands[name]();
```

- [ ] **Step 2: Append the scripts to `package.json`**

After the `"seed:reference"` line, add a comma to that line and append:

```json
    "lint": "go vet ./apps/core-engine/... ./apps/reference-service/... ./packages/goplatform/... && node scripts/go-tools.mjs fmt",
    "build:check": "node scripts/go-tools.mjs standalone",
    "clean": "node scripts/go-tools.mjs clean",
    "check": "npm run lint && npm run test && npm run build:check"
```

- [ ] **Step 3: Run lint and confirm it fails on exactly the known file (RED)**

Run: `npm run lint`

Expected: exit 1, with output containing

```
gofmt would reformat:
  apps/core-engine/internal/vision/workers/segmenter_worker.go
```

and no other path.

- [ ] **Step 4: Fix the formatting**

Run: `gofmt -w apps/core-engine/internal/vision/workers/segmenter_worker.go`

Then run `git diff --stat`. Only that file may change, and only in `fallbackSegment`'s single-line `if` bodies. Check with `git diff -w --ignore-blank-lines`: the only remaining changes must be line splits. The `gofmt -w` output is LF, and git normalizes line endings on commit.

- [ ] **Step 5: Run lint, build:check and clean (GREEN)**

Run each and expect exit 0:
- `npm run lint`: `go vet` is silent and the output ends with `gofmt: <N> files clean`.
- `npm run build:check`: prints `ok  apps/core-engine ./cmd/server`, `ok  apps/reference-service ./cmd/server` and `ok  apps/reference-service ./cmd/seed-reference-data`.
- `npm run build && npm run clean`: `clean` prints `removed apps/core-engine/bin` and `removed apps/reference-service/bin`.

- [ ] **Step 6: Add `.gitattributes`**

```
# Go sources are LF everywhere, so gofmt stays quiet on Windows checkouts.
*.go text eol=lf
```

- [ ] **Step 7: Commit**

```bash
git add scripts/go-tools.mjs package.json .gitattributes apps/core-engine/internal/vision/workers/segmenter_worker.go
git commit -m "build: lint, build:check, clean and check npm scripts" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Lockfile, Makefile removal, compose fix, README

**Files:**
- Create: `package-lock.json` (generated)
- Delete: `Makefile`
- Modify: `docker-compose.yml`
- Modify: `README.md` ("Running it" section)

- [ ] **Step 1: Generate the lockfile**

Run: `npm install`

Expected: creates `package-lock.json` and `node_modules/` (`node_modules` is gitignored; verify with `git check-ignore node_modules`). Then `npm ci` must succeed.

- [ ] **Step 2: Delete the Makefile**

Run: `git rm Makefile`

- [ ] **Step 3: Fix `docker-compose.yml`**

In the `core-engine` service:
- Replace the `VISION_AI_WORKER_URL` line with the two env lines below.
- Add the `extra_hosts` and `healthcheck` blocks after `networks: [seagull]`.

```yaml
      # The Python workers have no service in this file until sub-project B
      # (docs/superpowers/specs/2026-09-26-go-service-standardization-design.md)
      # builds their images. Run them on the host with `npm run dev:vision` /
      # `npm run dev:tryon`; host.docker.internal reaches the host.
      - VISION_AI_WORKER_URL=${VISION_AI_WORKER_URL:-http://host.docker.internal:8088}
      - TRYON_ENGINE_URL=${TRYON_ENGINE_URL:-http://host.docker.internal:8090}
```

```yaml
    extra_hosts:
      # Docker Desktop resolves host.docker.internal itself; Linux needs this.
      - "host.docker.internal:host-gateway"
    healthcheck:
      # /health answers 503 while Postgres is down (packages/goplatform/health.go).
      test: ["CMD", "wget", "-qO-", "http://localhost:8082/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

In the `reference-service` service, after `networks: [seagull]`:

```yaml
    healthcheck:
      # /health answers 503 while Postgres is down (packages/goplatform/health.go).
      test: ["CMD", "wget", "-qO-", "http://localhost:8086/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Verify that the YAML parses, since Docker is not available here:

```bash
node -e "const s=require('fs').readFileSync('docker-compose.yml','utf8'); if(/\t/.test(s)) throw new Error('tab in yaml'); console.log('no tabs')"
python -c "import yaml,sys; d=yaml.safe_load(open('docker-compose.yml')); print(sorted(d['services'])); print(d['services']['core-engine']['healthcheck']['test'])"
```

If PyYAML is missing (`ModuleNotFoundError`), install nothing. Report the YAML check as skipped and rely on the user's `docker compose config`.

- [ ] **Step 4: Update README "Running it"**

Replace the "On the host instead:" code block with:

````markdown
On the host instead (first run `npm ci` once):

| Command | What it does |
|---|---|
| `npm run dev` | core-engine :8082 + reference-service :8086 + vision worker :8088 |
| `npm run dev:tryon` | try-on engine :8090 (needs torch; not part of `dev`) |
| `npm run test` | Go unit tests + the Python vision suite (`test:tryon` is separate) |
| `npm run lint` | `go vet` + gofmt (line-ending agnostic) |
| `npm run build:check` | builds each Go service the way its Dockerfile does, with `GOWORK=off` — what Railway sees |
| `npm run check` | `lint`, `test`, `build:check` — run before pushing |
| `npm run seed:reference` | one-time reference-data seed |
| `npm run clean` | remove `apps/*/bin` |

There is no Makefile; `npm run` lists every script.
````

- [ ] **Step 5: Full check**

Run: `npm run check`
Expected: exit 0. That means lint is clean, `test:go` and `test:vision` are ok, and all three standalone builds are ok.

- [ ] **Step 6: Commit**

```bash
git add package-lock.json docker-compose.yml README.md
git commit -m "build: lockfile, drop Makefile, compose worker URLs and healthchecks" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

(`git rm Makefile` already staged the deletion.)

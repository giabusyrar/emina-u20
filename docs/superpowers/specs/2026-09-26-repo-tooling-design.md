# Repo tooling: npm scripts as the single entry point

Date: 2026-09-26
Status: design, awaiting review
Scope: `package.json`, `package-lock.json`, `Makefile` (deleted), `scripts/`,
`.gitattributes`, `docker-compose.yml`, `README.md`, and one gofmt-only fix in
`apps/core-engine/internal/vision/workers/segmenter_worker.go`.

This is sub-project **C** of the Seagull-core restructure. Sub-project A (the Go
services, on `main` since `6dc170c`) is done. B, the Python worker merge, waits
for the model registry branch.

## The problem

Seagull-core has two entry points: npm scripts, and a Makefile that only wraps
them. Tooling on this machine, as of 2026-09-26:

- **Missing:** `make`, `task`, `just`, `uv` and `docker`. So the Makefile
  cannot run here.
- **Installed:** `node`, `npm`, `go` and `python`.
- **No lockfile.** There is no `package-lock.json` and `concurrently` is not
  installed. `npm run dev` therefore fails on a fresh checkout until someone
  runs `npm install`, and then resolves whatever `^10.0.5` means that day.

Both sibling repositories already use npm scripts as their runner:
seagull-web uses them alone, and seagull-gateway uses them plus a wrapper
Makefile. So npm scripts are the de facto standard. A new tool such as go-task
would be the odd one out.

Part A also exposed two gaps:

- **Nothing reproduces the Railway build locally.** Railway builds each Go
  service from its own Dockerfile, with only its `replace` to
  `packages/goplatform`, which means `go.work` does not apply. The workspace
  build hides a missing `go.sum` entry that would fail on Railway. Part A only
  caught this by running the standalone build by hand.
- **gofmt is unusable on Windows.** Git stores `.go` files with LF, but
  `core.autocrlf=true` checks them out as CRLF, so `gofmt -l` flags every file.
  One real violation, in `segmenter_worker.go`, has gone unnoticed because of
  that noise.

`docker-compose.yml` has a bug of its own. `VISION_AI_WORKER_URL` defaults to
`http://vision-ai-worker:8088`, a service this compose file does not define.
Inside the container, core-engine therefore points at nothing.

## Design

### `package.json` is the only entry point

Existing script names stay unchanged. `.claude/launch.json` (`dev:vision`) and
the `feat/model-registry` branch (`test:vision`, plus its new `bench:vision`)
depend on them. New scripts are appended at the end of the block, so that
branch merges without a conflict.

| Script | Does | Status |
|---|---|---|
| `dev`, `dev:core`, `dev:ref`, `dev:vision`, `dev:tryon` | run services on the host | unchanged |
| `build`, `build:core`, `build:ref` | build Go binaries into `apps/*/bin` | unchanged |
| `test`, `test:go`, `test:vision`, `test:tryon` | run the test suites | unchanged |
| `seed:reference` | run the reference-data seeder | unchanged |
| `lint` | `go vet`, plus a gofmt check that ignores line endings, over both apps and `packages/goplatform` | new |
| `build:check` | build each Go service the way its Dockerfile does: `GOWORK=off CGO_ENABLED=0 GOOS=linux go build ./cmd/server` from inside the app directory | new |
| `clean` | remove `apps/*/bin` | new (replaces `make clean`) |
| `check` | `lint`, then `test`, then `build:check`: the one command to run before pushing | new |

`test` keeps its current meaning: Go plus the vision suite. The tryon suite
downloads models and needs torch, so it stays opt-in as `test:tryon`.

### `scripts/go-tools.mjs`

On Windows, npm runs scripts through `cmd.exe`. Inline `VAR=value cmd`,
`$(mktemp -d)` and `rm -rf` therefore do not work there. The new scripts call
one dependency-free Node file that has three subcommands:

- **`node scripts/go-tools.mjs fmt`**
  - Lists `.go` files under `apps/core-engine`, `apps/reference-service` and
    `packages/goplatform`, using `git ls-files` so that ignored and vendored
    files are skipped.
  - For each file, strips `\r` and runs the content through `gofmt -l` on
    stdin.
  - Prints the offending paths and exits 1 if any file would be reformatted.
- **`node scripts/go-tools.mjs standalone`**
  - For each Go app, spawns `go build -o <tmpdir>/<app> ./cmd/server` with the
    app directory as the working directory and `GOWORK=off`,
    `CGO_ENABLED=0` and `GOOS=linux` set.
  - The apps are `core-engine` and `reference-service`. For reference-service
    it also builds `./cmd/seed-reference-data`.
  - Exits non-zero on the first failure and names the app.
- **`node scripts/go-tools.mjs clean`** calls `fs.rmSync` on `apps/*/bin`.

The list of apps is written once, as a constant at the top of the file. Part B
adds nothing here, because it is Python.

`lint` is `go vet ./apps/core-engine/... ./apps/reference-service/... ./packages/goplatform/... && node scripts/go-tools.mjs fmt`.

### Lockfile

Run `npm install` once and commit `package-lock.json`, so that `npm ci` gives
the same `concurrently` everywhere.

### Makefile

Delete it. Each target maps to an npm script:

| Makefile target | npm script |
|---|---|
| `help` | `npm run` |
| `dev` | `dev` |
| `build` | `build` |
| `test` | `test` |
| `test-vision` | `test:vision` |
| `clean` | `clean` |

### `.gitattributes`

```
*.go text eol=lf
```

With this, fresh checkouts on any OS get LF `.go` files. Existing working trees
keep their CRLF copies until a file is next checked out. That is harmless,
because the `fmt` check ignores line endings. No renormalization commit is
needed: the index is already LF.

### `docker-compose.yml`

- **Worker URLs point at the host until Part B.** `VISION_AI_WORKER_URL`
  defaults to `http://host.docker.internal:8088` and `TRYON_ENGINE_URL` to
  `http://host.docker.internal:8090`. A comment explains that the Python
  workers have no compose service until Part B, so they run on the host with
  `npm run dev:vision` or `npm run dev:tryon`. Docker Desktop on Windows and
  macOS resolves `host.docker.internal`. On Linux, add
  `extra_hosts: ["host.docker.internal:host-gateway"]`, and put it in this
  file so that the compose setup works everywhere.
- **Healthchecks.** Both Go services get
  `healthcheck: {test: ["CMD", "wget", "-qO-", "http://localhost:<port>/health"], interval: 30s, timeout: 5s, retries: 3}`.
  The runtime image is `alpine:3.20`, whose busybox provides `wget`.
  `/health` returns 503 when Postgres is down, so compose reports the
  container unhealthy. That is the behaviour sub-project A intended.

### `segmenter_worker.go`

Run `gofmt -w` on it. The change is formatting only: the single-line `if`
bodies in `fallbackSegment`. Without this fix, `lint` fails on its first run.

### README

Replace the two-line "On the host instead" block with the script table above,
and mention `npm ci` as the setup step.

## Out of scope

- **Python Dockerfiles and Python compose services.** Part B builds them, as
  the ai-worker CPU and GPU images.
- **A CI pipeline.** There is no `.gitlab-ci.yml` today. `npm run check` is the
  command a pipeline would run.
- **Adding `lint` or `check` to sibling repositories.** That is their call.
- **`.claude/launch.json`.** It keeps working unchanged.

## Verification

- Run `npm ci`, `npm run lint`, `npm run build:check`, `npm run test:go`,
  `npm run clean` and `npm run check` on this Windows machine, and record their
  output.
- Show that `lint` catches a problem: before the gofmt fix, `npm run lint`
  must fail naming `segmenter_worker.go` and nothing else.
- The compose changes cannot be validated here, because Docker is not
  installed. The user runs `docker compose config` and `docker compose up`
  after merge.

## Risk

- **Merging `feat/model-registry` later touches `package.json`.** It edits
  the `test:vision` line and adds `bench:vision`. This change leaves existing
  lines alone and appends after `seed:reference`, so the expected conflict is
  at most adjacent-line, and trivial to resolve.
- **The `fmt` check runs gofmt once per file.** That is about 140 process
  launches, a few seconds on this repository, which is acceptable.

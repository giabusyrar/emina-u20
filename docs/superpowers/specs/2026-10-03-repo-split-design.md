# Split seagull-core into one repository per service — design (sub-project 1)

Date: 2026-10-03
Status: approved 2026-10-03 (goplatform copied in, not a library repo)
Decided in chat: one repo per deployed service (option A), shared libraries as
their own repos published as versioned packages, each repo deploys with its own
compose file (option B), history preserved with `git filter-repo`, the user
creates the GitLab projects and pushes. goplatform is copied into both Go
repos as `internal/platform` instead of becoming its own module (decided
after approval: two consumers do not justify a private Go module). Sub-project 2 (a Next.js simulator per
repo, built on the published web packages) follows separately.

## Repositories

Built as local git repositories under
`C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull/`, one folder each,
to be pushed by the user to the same GitLab group as seagull-core
(`gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/`).

| Repo | From seagull-core | Kind |
|---|---|---|
| `seagull-core-engine` | `apps/core-engine` | Go service :8082 |
| `seagull-reference-service` | `apps/reference-service` | Go service :8086 |
| `seagull-worker-skin` | `apps/workers/skin` | Python worker :8088 |
| `seagull-worker-models` | `apps/workers/models` | Python worker :8096 (model registry + assets) |
| `seagull-worker-face` | `apps/workers/face` | Python worker :8094 |
| `seagull-worker-colour` | `apps/workers/colour` | Python worker :8092 |
| `seagull-worker-tryon` | `apps/workers/tryon` | Python worker :8090 |
| `seagull-conversation-engine` | `apps/conversation-engine` | Python service :8098 (agreed with its owner, the Live agent session; no goplatform/worker_common dependency; talks to core-engine over HTTP only) |

Not carried over: `apps/ai-worker` (untracked leftover), `RAILWAY.md`,
`railway.json` (Railway is no longer used), `node_modules`, `go.work*`.

## History

Each repo is a fresh clone of seagull-core filtered with `git filter-repo`
(installed with `pip install git-filter-repo`): keep the service's path and the
docs/specs that are only about it, and move the service's path to the repo
root (`--path apps/workers/skin/ --path-rename apps/workers/skin/:`). Commits
that touched other paths drop out; messages, authors and dates stay. The
original seagull-core is untouched until cut-over.

## Layout of a service repo

```
<repo>/
  <service code at the root>      # Go module or Python package, tests
  Dockerfile                      # builds from this repo only
  docker-compose.yml              # this service, joins the external `seagull` network
  compose.vps.example.yml         # VPS override: bind to 127.0.0.1:<port>
  .env.example                    # only the variables this service reads
  .gitlab-ci.yml                  # lint + tests (+ image build check)
  AGENTS.md / CLAUDE.md           # ground rules (copied) + repo-specific notes
  README.md                       # what it is, run, test, deploy, API summary
  docs/                           # docs that belong to this service
```

- The `seagull` network is external and created once per host
  (`docker network create seagull`); every repo's compose joins it, so services
  keep reaching each other by name (`core-engine:8082`, `worker-models:8096`).
- Cross-repo start order is not expressed in compose. Workers already wait for
  worker-models when fetching assets (`MODEL_ASSET_FETCH_WAIT_SECONDS`).
- Ports stay as in `docs/PORTS.md`.

## Shared libraries

**goplatform — copied, not shared.** `packages/goplatform` (with its history,
via `--path packages/goplatform/ --path-rename packages/goplatform/:internal/platform/`)
goes into both seagull-core-engine and seagull-reference-service as
`internal/platform`, package name unchanged (`goplatform`), imported as
`<module>/internal/platform`. No private Go module, GOPRIVATE or `.netrc`.
- Drift guard: both copies carry a `README.md` saying the package is mirrored
  in the other repo and a change to one goes to both; each repo's CI asserts
  that `gatewayOwnedTables` in `dbguard.go` lists exactly
  `api_keys, collections, routes, route_groups`, so the gateway-DB guard
  cannot silently diverge.
- Each service keeps its own Go module path, renamed to its GitLab path
  (`gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine`
  and `.../seagull-reference-service`); the `replace` to `../../packages/goplatform` is removed.

**worker_common — copied, not shared** (decided 2026-10-03, like goplatform).
`apps/workers/common/worker_common/` → `worker_common/` and its tests →
`tests/worker_common/` in each of the five worker repos, with history. Its
dependencies join each worker's `pyproject.toml`; no package registry, deploy
token or extra build context. `scripts/check-common-mirror.sh` diffs the copy
against sibling worker checkouts; `worker_common/README.md` says a change goes
to all five.

**Location.** All repos sit side by side in `Paragon/seagull/`, built by
`Paragon/seagull/seagull-split-tools/build.py` (re-runnable until the push).

## Documentation

- Cross-cutting docs (`PORTS.md`, `SHARED-DATABASE.md`, `MODEL-REGISTRY.md`,
  `LANES.md` protocol, `SPLIT.md`) go to `seagull-core-engine/docs/` as the
  system reference, and every other repo's README links there. Service docs go
  with their service (e.g. `FACE-HEAD.md` → worker-face, `TRYON.md` →
  worker-tryon, `PCA-VTO-CONTRACT.md` → core-engine and worker-colour).
- `AGENTS.md` ground rules are copied into each repo unchanged; the
  repo-specific part names the repo's lane and the other repos.
- seagull-core gets a final commit: README "archived — moved to …" with the
  repo table; it stays read-only.

## Deployment (VPS)

Each repo is checked out at `~/seagull/<repo>` and deployed with its own
`docker compose -f docker-compose.yml -f compose.vps.yml up -d --build`.
Cut-over, once the user has pushed: create the `seagull` network if missing →
stop the old seagull-core stack service by service, starting the new repo's
service in its place (same container name, same port, same volumes:
`model-cache`) → smoke tests as in previous deploys. Rollback: the old
`~/seagull-core` stack stays on disk until the user retires it.

## Order

2. Go services: core-engine, reference-service.
3. Workers: models, skin, face, colour, tryon.
4. conversation-engine (owner agreed 2026-10-03; carries its spec and plan `docs/superpowers/{specs,plans}/2026-10-03-conversation-engine*`; Dockerfile becomes `COPY . /app`; its compose drops `depends_on: core-engine`; keeps container/upstream name `conversation-engine:8098` for APISIX; VPS env CONV_LIVE_MODEL, CONV_VOICE_DEFAULT, GOOGLE_API_KEY move with it).
5. seagull-core archive commit.
6. After the user pushes: VPS cut-over.

Each repo is verified locally before hand-off: its tests pass, its Docker
image builds, `docker compose config` is valid where Docker is available
(otherwise on the VPS), and `git log` shows its filtered history.

## Out of scope

Simulators (sub-project 2); changing service code beyond import paths,
Dockerfiles and config; seagull-web and seagull-gateway.

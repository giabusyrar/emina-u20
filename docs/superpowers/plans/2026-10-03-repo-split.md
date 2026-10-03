# Repo split (sub-project 1) Implementation Plan

> **For agentic workers:** executed inline by the Core session; the build is one
> re-runnable script, so tasks are its stages.

**Goal:** Build nine stand-alone git repositories from seagull-core under
`../seagull-repos/`, history preserved, each building and testing on its own.

**Architecture:** `seagull-repos/split-tools/build.py` clones seagull-core
once per repo, filters it with `git filter-repo`, then writes the per-repo
scaffolding (Dockerfile, compose, CI, env example, AGENTS/README) and commits.
It deletes and rebuilds a repo folder on every run, so later seagull-core
commits are picked up by re-running it before the user pushes.

**Tech Stack:** git-filter-repo a40bce548d2c, Go 1.25, Python 3.12, Docker
Compose v2 (BuildKit `additional_contexts`), GitLab CI.

**Spec:** `docs/superpowers/specs/2026-10-03-repo-split-design.md`

## Global Constraints

- GitLab group: `gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/`.
- Ports unchanged (`docs/PORTS.md`): core-engine 8082, reference-service 8086,
  worker-skin 8088, worker-tryon 8090, worker-colour 8092, worker-face 8094,
  worker-models 8096, conversation-engine 8098.
- goplatform is copied into both Go repos as `internal/platform` (package
  name `goplatform`); no private Go module.
- Python library renamed to `seagull-worker-common` 0.1.0, import name
  `worker_common`; workers depend on `seagull-worker-common>=0.1,<0.2`.
- No push, no `.env` values in any file, no hardcoded hosts beyond the
  compose service-name defaults that exist today.
- seagull-core is only read; its archive commit waits for the cut-over
  (other sessions still work in it).

## Ruling recorded during planning

- Worker images get worker-common through a named BuildKit context
  (`additional_contexts: worker_common: ${WORKER_COMMON_CONTEXT:-../seagull-worker-common}`)
  until the package is published; the Dockerfile then switches to the
  registry (post-push task). Cost if wrong: one Dockerfile line per worker.
- Docs before 2026-09-30 lived at other paths (`apps/ai-worker`, …); filtered
  history keeps only commits under the current paths. Earlier history stays
  in the archived seagull-core.
- The archive commit to seagull-core is deferred to cut-over — Skin, 3D head
  and Live agent sessions are still committing there.

## Tasks

### Task 1: Libraries — seagull-worker-common
Filter `apps/workers/common/` to the root; rename the distribution to
`seagull-worker-common`; CI runs tests and publishes to the project PyPI
registry on a `v*` tag. Verify: `python -m pytest` passes.

### Task 2: Go services — seagull-core-engine, seagull-reference-service
Filter the service path to the root plus `packages/goplatform/` →
`internal/platform/`; delete `internal/platform/go.mod|go.sum`; rewrite module
path to the GitLab path and goplatform imports to `<module>/internal/platform`;
drop the `replace`; `go mod tidy`; gofmt. Dockerfile builds from the repo
root. CI: gofmt, vet, test, build, and `scripts/check-platform-mirror.sh`
(the gateway-table list). Verify: `GOWORK=off go vet ./... && go test ./... && go build ./...`.

### Task 3: Workers — models, skin, face, colour, tryon
Filter each worker to the root plus its docs; add the
`seagull-worker-common` dependency; Dockerfile copies `.` and the
`worker_common` named context; compose without cross-repo `depends_on`;
`compose.vps.example.yml` binds to loopback (worker-models reuses the existing
`seagull-core_model-cache` volume). Verify: `python -m pytest` with
`PYTHONPATH=../seagull-worker-common`.

### Task 4: conversation-engine
Filter to the root with its spec/plan; Dockerfile `COPY . /app`; compose
drops `depends_on`. Verify: `python -m pytest`.

### Task 5: Image build check (VPS, read-only on the live stack)
Copy the repos to `~/seagull-split-check/` on the VPS and run
`docker compose build` for each except tryon (CUDA image, not built on the
VPS); remove the scratch folder and dangling build images afterwards.

### Task 6 (after the user pushes): publish and cut over
Tag `v0.1.0` on seagull-worker-common, switch worker Dockerfiles to the
registry, VPS cut-over service by service, then the seagull-core archive commit.

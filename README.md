# seagull-core-engine

The assessment engine: forms/surveys, scoring, product match, vision (skin analysis orchestration), colour (WCPA), face architecture, assessments. Port **8082**.

Split out of seagull-core on 2026-10-03 with its git history. Ground rules and
the list of the other repositories: `AGENTS.md`. System reference (ports,
databases, model registry): seagull-core-engine `docs/`. Docs carried over from
seagull-core may still name the old `apps/...` paths.

## Run locally

```bash
cp .env.example .env        # set DATABASE_URL to the core database
go run ./cmd/server
```

## Test

```bash
go vet ./... && go test ./...
sh scripts/check-platform-mirror.sh
```

Seeders run on the VPS, not a laptop: `go run ./cmd/seed-colour-shades`.

## Deploy (VPS)

Checked out at `~/seagull/seagull-core-engine`; `compose.vps.yml` is a copy of
`compose.vps.example.yml`, `.env` holds the secrets (never committed).

```bash
docker compose -f docker-compose.yml -f compose.vps.yml up -d --build
```

The container joins the external `seagull` network as `core-engine`; APISIX
reaches it there on port 8082.

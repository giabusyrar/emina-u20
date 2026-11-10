# internal/platform (package `goplatform`)

Shared bootstrap for the Go services: `OpenPostgres` with the gateway-database
guard, env helpers, the Echo server and graceful shutdown, `/health`, the JSON
error envelope, slog/GORM logging and `OpenRedis`.

**Mirrored.** The same package lives in seagull-core-engine and
seagull-reference-service (it was `packages/goplatform` in seagull-core). A
change to one goes to both. CI checks the gateway-table guard
(`scripts/check-platform-mirror.sh`).

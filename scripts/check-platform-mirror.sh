#!/bin/sh
# internal/platform is mirrored in seagull-core-engine and
# seagull-reference-service. The gateway-database guard must name the same
# tables in both: they are Seagull-gateway's gateway-engine tables, a fact about
# that repository's schema (internal/platform/dbguard.go). If gateway-engine's
# AutoMigrate list changes, update both repositories and this line together.
set -eu
expected='var gatewayOwnedTables = []string{"api_keys", "collections", "routes", "route_groups"}'
if ! grep -qxF "$expected" internal/platform/dbguard.go; then
  echo "internal/platform/dbguard.go: gatewayOwnedTables differs from the mirrored list:" >&2
  echo "  $expected" >&2
  exit 1
fi
echo "platform mirror: gateway-table guard matches"

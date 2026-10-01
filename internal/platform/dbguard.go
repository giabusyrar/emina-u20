package goplatform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// gatewayOwnedTables are tables that Seagull-gateway's gateway-engine creates
// (its AutoMigrate list in apps/gateway-engine/cmd/server/main.go) and that no
// seagull-core service ever creates. Finding any of them means DATABASE_URL
// points at the gateway's database, not the core one. This describes another
// repository's schema, not a deployment choice, so it is not configurable.
var gatewayOwnedTables = []string{"api_keys", "collections", "routes", "route_groups"}

// tableExists reports whether public.<name> exists.
type tableExists func(ctx context.Context, name string) (bool, error)

// refuseGatewayDatabase returns an error if any gateway-owned table exists.
// seagull-core and Seagull-gateway use separate databases; running core
// migrations or seeders against the gateway's would put reference, product
// and assessment data where it does not belong (it happened once, 2026-10).
func refuseGatewayDatabase(ctx context.Context, exists tableExists) error {
	var found []string
	for _, name := range gatewayOwnedTables {
		ok, err := exists(ctx, name)
		if err != nil {
			return fmt.Errorf("check for gateway table %s: %w", name, err)
		}
		if ok {
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("DATABASE_URL points at the Seagull-gateway database (found its tables: %s); "+
			"seagull-core services must use the core database", strings.Join(found, ", "))
	}
	return nil
}

func sqlTableExists(db *sql.DB) tableExists {
	return func(ctx context.Context, name string) (bool, error) {
		var ok bool
		err := db.QueryRowContext(ctx, "SELECT to_regclass('public.' || $1) IS NOT NULL", name).Scan(&ok)
		return ok, err
	}
}

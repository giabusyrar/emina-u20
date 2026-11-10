package goplatform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func existsIn(tables ...string) tableExists {
	set := map[string]bool{}
	for _, t := range tables {
		set[t] = true
	}
	return func(_ context.Context, name string) (bool, error) { return set[name], nil }
}

func TestRefuseGatewayDatabaseAcceptsCoreDatabase(t *testing.T) {
	if err := refuseGatewayDatabase(context.Background(), existsIn("score_rulesets", "ref_dimensions")); err != nil {
		t.Fatalf("core database refused: %v", err)
	}
}

func TestRefuseGatewayDatabaseRefusesGatewayDatabase(t *testing.T) {
	err := refuseGatewayDatabase(context.Background(), existsIn("api_keys", "routes"))
	if err == nil {
		t.Fatal("want error for the gateway database")
	}
	if !strings.Contains(err.Error(), "api_keys") || !strings.Contains(err.Error(), "routes") {
		t.Fatalf("error should name the tables found: %v", err)
	}
}

func TestRefuseGatewayDatabaseReportsLookupFailure(t *testing.T) {
	boom := errors.New("boom")
	err := refuseGatewayDatabase(context.Background(), func(context.Context, string) (bool, error) { return false, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want lookup error, got %v", err)
	}
}

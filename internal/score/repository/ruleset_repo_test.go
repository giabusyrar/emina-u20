package repository

import (
	"context"
	"testing"
)

func TestRulesetRepo_EmptyState(t *testing.T) {
	repo := NewRulesetRepo(nil)

	// When DB is nil or empty, GetActiveRuleset should return an error
	ruleset, err := repo.GetActiveRuleset(context.Background(), "wardah", "app_1", "baumann_16_types")
	if err == nil || ruleset != nil {
		t.Fatalf("expected error when querying active ruleset on empty repo, got: %v", ruleset)
	}

	// ListRulesets should return an empty slice
	list, err := repo.ListRulesets(context.Background(), "*", "*")
	if err != nil {
		t.Fatalf("unexpected error from ListRulesets: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 rulesets in empty repo, got %d", len(list))
	}
}

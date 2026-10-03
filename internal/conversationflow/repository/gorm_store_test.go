package repository

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/service"
)

// pgUniqueViolation stands in for *pgconn.PgError, which exposes SQLState.
type pgUniqueViolation struct{}

func (pgUniqueViolation) Error() string {
	return `ERROR: duplicate key value violates unique constraint "idx_conversation_flow_version" (SQLSTATE 23505)`
}
func (pgUniqueViolation) SQLState() string { return "23505" }

func TestTranslateUniqueViolation(t *testing.T) {
	for name, err := range map[string]error{
		"pgx":  fmt.Errorf("insert: %w", pgUniqueViolation{}),
		"gorm": gorm.ErrDuplicatedKey,
	} {
		if got := translate(err); got != service.ErrDuplicateVersion {
			t.Errorf("%s: got %v, want ErrDuplicateVersion", name, got)
		}
	}
	other := errors.New("connection reset")
	if got := translate(other); got != other {
		t.Errorf("other error: got %v, want it unchanged", got)
	}
	if translate(nil) != nil {
		t.Error("nil error must stay nil")
	}
}

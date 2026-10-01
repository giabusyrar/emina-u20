package brand

import (
	"context"
	"errors"
	"testing"
)

func TestPick(t *testing.T) {
	row := func(id, code string) struct{ ID, Code string } { return struct{ ID, Code string }{id, code} }

	b, err := pick("WARDAH", []struct{ ID, Code string }{row("brd-1", "WARDAH ")})
	if err != nil || b.ID != "brd-1" || b.Code != "wardah" {
		t.Fatalf("got %+v %v, want id brd-1 and the code lower-cased", b, err)
	}
	if _, err := pick("nope", nil); !errors.Is(err, ErrUnknown) {
		t.Fatalf("no row must be ErrUnknown, got %v", err)
	}
	if _, err := pick("x", []struct{ ID, Code string }{row("a", "x"), row("x", "y")}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("two rows must be refused, not guessed: %v", err)
	}
}

func TestResolveRequiresABrand(t *testing.T) {
	if _, err := NewResolver(nil).Resolve(context.Background(), "  "); !errors.Is(err, ErrRequired) {
		t.Fatalf("got %v, want ErrRequired", err)
	}
	if _, err := NewResolver(nil).Resolve(context.Background(), "wardah"); err == nil || errors.Is(err, ErrUnknown) {
		t.Fatalf("no database must be an error of its own, got %v", err)
	}
}

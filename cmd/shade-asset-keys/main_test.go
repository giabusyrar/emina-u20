package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type fakeStore struct {
	hasURLCols bool
	rows       []legacyRow
	prepared   bool
	applied    []keyedRow
}

func (f *fakeStore) HasURLColumns(context.Context) (bool, error)  { return f.hasURLCols, nil }
func (f *fakeStore) LegacyRows(context.Context) ([]legacyRow, error) { return f.rows, nil }
func (f *fakeStore) Prepare(context.Context) error                  { f.prepared = true; return nil }
func (f *fakeStore) Apply(_ context.Context, rows []keyedRow) error { f.applied = rows; return nil }

var testPrefixes = prefixSource{PublicURL: "https://pub.example.dev"}

func runWith(t *testing.T, s *fakeStore, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := run(context.Background(), args, &out, &out, s, testPrefixes)
	return code, out.String()
}

func goodRows() []legacyRow {
	return []legacyRow{{ID: "a1", ColorURL: "https://pub.example.dev/shades/1/color.png", AlphaURL: "https://pub.example.dev/shades/1/alpha.png"}}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	code, out := runWith(t, s)
	if code != 0 || s.applied != nil || s.prepared {
		t.Fatalf("code=%d applied=%v prepared=%v", code, s.applied, s.prepared)
	}
	if !strings.Contains(out, "shades/1/color.png") || !strings.Contains(out, "dry run") {
		t.Fatalf("output = %q", out)
	}
}

func TestApplyWritesPlannedRows(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	if code, out := runWith(t, s, "-apply"); code != 0 || len(s.applied) != 1 {
		t.Fatalf("code=%d applied=%v out=%q", code, s.applied, out)
	}
}

func TestUnrecognisedRowAbortsWithoutWriting(t *testing.T) {
	rows := append(goodRows(), legacyRow{ID: "a2", ColorURL: "https://other.example/c.png", AlphaURL: "https://pub.example.dev/a.png"})
	s := &fakeStore{hasURLCols: true, rows: rows}
	code, out := runWith(t, s, "-apply")
	if code != 1 || s.applied != nil || !strings.Contains(out, "a2: color_map_url https://other.example/c.png") {
		t.Fatalf("code=%d applied=%v out=%q", code, s.applied, out)
	}
}

func TestAlreadyMigratedIsANoOp(t *testing.T) {
	s := &fakeStore{hasURLCols: false}
	code, out := runWith(t, s, "-apply")
	if code != 0 || s.applied != nil || !strings.Contains(out, "already") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestPrepareOnlyDropsConstraints(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	if code, _ := runWith(t, s, "-prepare"); code != 0 || !s.prepared || s.applied != nil {
		t.Fatalf("code=%d prepared=%v applied=%v", code, s.prepared, s.applied)
	}
}

func TestPrepareAndApplyTogetherIsUsage(t *testing.T) {
	if code, _ := runWith(t, &fakeStore{hasURLCols: true}, "-prepare", "-apply"); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestNoPrefixesIsAnError(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"-apply"}, &out, &out, &fakeStore{hasURLCols: true, rows: goodRows()}, prefixSource{})
	if code != 1 || !strings.Contains(out.String(), "S3_PUBLIC_URL") {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}

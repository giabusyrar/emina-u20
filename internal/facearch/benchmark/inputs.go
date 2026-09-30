// Package benchmark is the face-architecture benchmark and calibration
// harness behind cmd/face-benchmark: it measures a local, consented image
// set through the ai-worker, caches the worker's JSON, scores a profile
// file against hand-collected vendor and rater labels, and fits a
// calibrated profile.
//
// Inputs and outputs stay on the operator's machine. The only network call
// is to the configured ai-worker. Reports carry counts and metrics, never
// image paths or per-image rows.
//
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Benchmark and calibration harness".
package benchmark

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Result sources (results.csv "source"). Raters are "rater:<id>".
const (
	SourcePerfectCorp = "perfectcorp"
	SourceWardah      = "wardah"
	SourceMakeover    = "makeover"
	RaterPrefix       = "rater:"
	// FamilyRater is the label-map family every "rater:<id>" source reads.
	FamilyRater = "rater"
	// TargetRaterConsensus is the derived source: the strict-majority label
	// of the raters (see BuildLabels).
	TargetRaterConsensus = "rater-consensus"
)

// ManifestRow is one image in manifest.csv.
type ManifestRow struct {
	ImageID string
	// ImagePath is resolved: a relative path in the CSV is relative to the
	// manifest's directory.
	ImagePath  string
	SubjectID  string
	ConsentRef string
	Hijab      bool
	Notes      string
}

var (
	manifestColumns = []string{"image_id", "image_path", "subject_id", "consent_ref", "hijab", "notes"}
	resultColumns   = []string{"image_id", "source", "attribute", "value"}
)

// utf8BOM is the UTF-8 byte-order mark spreadsheet exports start with.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// csvRow is one data row by column name, with its 1-based line number.
type csvRow struct {
	line   int
	fields map[string]string
}

// readCSV reads a CSV whose header names exactly the given columns, in any
// order. A UTF-8 byte-order mark (spreadsheet exports) is ignored; fields
// are trimmed.
func readCSV(r io.Reader, name string, columns []string) ([]csvRow, error) {
	// Drop a UTF-8 byte-order mark before parsing: left in, it sits in front
	// of a quoted first header cell and breaks the quoting.
	br := bufio.NewReader(r)
	if lead, _ := br.Peek(len(utf8BOM)); bytes.Equal(lead, utf8BOM) {
		_, _ = br.Discard(len(utf8BOM))
	}
	cr := csv.NewReader(br)
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: empty file, want header %s", name, strings.Join(columns, ","))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	index := map[string]int{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		if _, dup := index[h]; dup {
			return nil, fmt.Errorf("%s: duplicate column %q", name, h)
		}
		index[h] = i
	}
	want := map[string]bool{}
	for _, c := range columns {
		want[c] = true
		if _, ok := index[c]; !ok {
			return nil, fmt.Errorf("%s: missing column %q (want %s)", name, c, strings.Join(columns, ","))
		}
	}
	for h := range index {
		if !want[h] {
			return nil, fmt.Errorf("%s: unknown column %q (want %s)", name, h, strings.Join(columns, ","))
		}
	}
	var rows []csvRow
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		line, _ := cr.FieldPos(0)
		fields := make(map[string]string, len(columns))
		for _, c := range columns {
			fields[c] = strings.TrimSpace(rec[index[c]])
		}
		rows = append(rows, csvRow{line: line, fields: fields})
	}
}

// parseHijab reads the manifest's hijab column: true/false, yes/no or 1/0,
// any case. Anything else is an error: the hijab slice is a reported
// split, so a guess would move images between slices.
func parseHijab(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("hijab must be true/false, yes/no or 1/0, got %q", s)
}

// ReadManifest reads manifest.csv (image_id, image_path, subject_id,
// consent_ref, hijab, notes). Every row needs a unique image_id, an
// image_path, a subject_id and a consent_ref: no image is measured without
// a consent reference.
func ReadManifest(path string) ([]ManifestRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := readCSV(f, path, manifestColumns)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	seen := map[string]int{}
	var out []ManifestRow
	var problems []string
	for _, r := range rows {
		bad := func(format string, args ...any) {
			problems = append(problems, fmt.Sprintf("line %d: ", r.line)+fmt.Sprintf(format, args...))
		}
		m := ManifestRow{
			ImageID:    r.fields["image_id"],
			ImagePath:  r.fields["image_path"],
			SubjectID:  r.fields["subject_id"],
			ConsentRef: r.fields["consent_ref"],
			Notes:      r.fields["notes"],
		}
		for _, c := range []string{"image_id", "image_path", "subject_id", "consent_ref"} {
			if r.fields[c] == "" {
				bad("%s is required", c)
			}
		}
		if prev, dup := seen[m.ImageID]; dup && m.ImageID != "" {
			bad("image_id %q repeats line %d", m.ImageID, prev)
		}
		seen[m.ImageID] = r.line
		h, err := parseHijab(r.fields["hijab"])
		if err != nil {
			bad("%v", err)
		}
		m.Hijab = h
		if m.ImagePath != "" && !filepath.IsAbs(m.ImagePath) {
			m.ImagePath = filepath.Join(dir, m.ImagePath)
		}
		out = append(out, m)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s:\n  %s", path, strings.Join(problems, "\n  "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no images", path)
	}
	return out, nil
}

// ResultRow is one label in results.csv (long format).
type ResultRow struct {
	Line      int
	ImageID   string
	Source    string
	Attribute string
	Value     string
}

// SourceFamily is the label-map family of a source: "rater" for every
// "rater:<id>", otherwise the source itself.
func SourceFamily(source string) string {
	if strings.HasPrefix(source, RaterPrefix) {
		return FamilyRater
	}
	return source
}

// ValidSource reports whether s is perfectcorp, wardah, makeover or
// rater:<id> with a non-empty id.
func ValidSource(s string) bool {
	switch s {
	case SourcePerfectCorp, SourceWardah, SourceMakeover:
		return true
	}
	return strings.HasPrefix(s, RaterPrefix) && len(s) > len(RaterPrefix)
}

// ReadResults reads results.csv (image_id, source, attribute, value). Each
// (image_id, source, attribute) may appear once, and every image_id must
// be in the manifest.
func ReadResults(path string, manifest []ManifestRow) ([]ResultRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := readCSV(f, path, resultColumns)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(manifest))
	for _, m := range manifest {
		known[m.ImageID] = true
	}
	seen := map[[3]string]int{}
	var out []ResultRow
	var problems []string
	for _, r := range rows {
		bad := func(format string, args ...any) {
			problems = append(problems, fmt.Sprintf("line %d: ", r.line)+fmt.Sprintf(format, args...))
		}
		rr := ResultRow{Line: r.line, ImageID: r.fields["image_id"], Source: r.fields["source"],
			Attribute: r.fields["attribute"], Value: r.fields["value"]}
		for _, c := range resultColumns {
			if r.fields[c] == "" {
				bad("%s is required", c)
			}
		}
		if rr.Source != "" && !ValidSource(rr.Source) {
			bad("source must be perfectcorp, wardah, makeover or rater:<id>, got %q", rr.Source)
		}
		if rr.ImageID != "" && !known[rr.ImageID] {
			bad("image_id %q is not in the manifest", rr.ImageID)
		}
		k := [3]string{rr.ImageID, rr.Source, rr.Attribute}
		if prev, dup := seen[k]; dup {
			bad("%s/%s/%s repeats line %d", rr.ImageID, rr.Source, rr.Attribute, prev)
		}
		seen[k] = r.line
		out = append(out, rr)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s:\n  %s", path, strings.Join(problems, "\n  "))
	}
	return out, nil
}

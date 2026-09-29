package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// SchemaVersion is the document schema this code reads.
const SchemaVersion = 1

// Calibration statuses. Every response repeats the profile's status.
const (
	CalibrationUncalibrated = "uncalibrated"
	CalibrationCalibrated   = "calibrated"
)

// Document is the JSON stored in FaceArchitectureProfile.Document.
type Document struct {
	SchemaVersion     int              `json:"schemaVersion"`
	CatalogueVersion  string           `json:"catalogueVersion"`
	Calibration       Calibration      `json:"calibration"`
	MeasurementsShown []string         `json:"measurementsShown"`
	Classifiers       []ClassifierSpec `json:"classifiers"`
	Traits            []TraitSpec      `json:"traits"`
	// Guidance is the optional makeup-placement section (guidance.go). A
	// document without it answers "guidance": null.
	Guidance *Guidance `json:"guidance,omitempty"`
}

// Calibration says where the profile's numbers came from.
type Calibration struct {
	Status         string          `json:"status"`
	BenchmarkRunID *string         `json:"benchmarkRunId"`
	Target         *string         `json:"target"`
	HeldOutMetrics json.RawMessage `json:"heldOutMetrics"`
	Sources        []Source        `json:"sources"`
}

// Source cites where a value came from. For is a document path such as
// "classifiers.faceShape.blendMargin" or "traits.noseWidth"; see Validate.
type Source struct {
	Cite string `json:"cite"`
	For  string `json:"for"`
	Note string `json:"note,omitempty"`
}

// ClassifierSpec is one prototype classifier (e.g. faceShape).
type ClassifierSpec struct {
	Key               string      `json:"key"`
	BlendMargin       float64     `json:"blendMargin"`
	MinObservedWeight float64     `json:"minObservedWeight"`
	Classes           []ClassSpec `json:"classes"`
}

// ClassSpec is one class and its prototype.
type ClassSpec struct {
	Label     string                   `json:"label"`
	Requires  []string                 `json:"requires"`
	Prototype map[string]PrototypeTerm `json:"prototype"`
}

// PrototypeTerm is one measurement's centre, spread and weight in a prototype.
type PrototypeTerm struct {
	Centre float64 `json:"centre"`
	Spread float64 `json:"spread"`
	Weight float64 `json:"weight"`
}

// TraitSpec is an ordered rule list; the first rule whose conditions all
// hold wins, otherwise Default.
type TraitSpec struct {
	Key     string     `json:"key"`
	Rules   []RuleSpec `json:"rules"`
	Default string     `json:"default"`
}

// RuleSpec assigns Label when every condition holds.
type RuleSpec struct {
	Label string      `json:"label"`
	When  []Condition `json:"when"`
}

// Condition compares one measurement with a threshold.
type Condition struct {
	Measurement string  `json:"measurement"`
	Op          string  `json:"op"`
	Value       float64 `json:"value"`
}

// Comparison operators a Condition may use.
const (
	OpLess         = "<"
	OpLessEqual    = "<="
	OpGreater      = ">"
	OpGreaterEqual = ">="
)

// Holds reports whether x satisfies the condition. Unknown operators never
// hold; Validate rejects them before a document is stored.
func (c Condition) Holds(x float64) bool {
	switch c.Op {
	case OpLess:
		return x < c.Value
	case OpLessEqual:
		return x <= c.Value
	case OpGreater:
		return x > c.Value
	case OpGreaterEqual:
		return x >= c.Value
	}
	return false
}

// SortedKeys returns the prototype's measurement keys in sorted order, so
// sums over a prototype are deterministic.
func (c ClassSpec) SortedKeys() []string {
	keys := make([]string, 0, len(c.Prototype))
	for k := range c.Prototype {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Keys returns every measurement key the classifier references, sorted.
func (s ClassifierSpec) Keys() []string {
	set := map[string]bool{}
	for _, c := range s.Classes {
		for _, k := range c.Requires {
			set[k] = true
		}
		for k := range c.Prototype {
			set[k] = true
		}
	}
	return sortedSet(set)
}

// Keys returns every measurement key the trait's conditions reference, sorted.
func (s TraitSpec) Keys() []string {
	set := map[string]bool{}
	for _, r := range s.Rules {
		for _, c := range r.When {
			set[c.Measurement] = true
		}
	}
	return sortedSet(set)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Parse decodes a document strictly: unknown fields are an error, so a
// misspelt key cannot silently drop a threshold.
func Parse(raw []byte) (*Document, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("face-architecture profile document: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("face-architecture profile document: trailing data after the JSON object")
	}
	return &d, nil
}

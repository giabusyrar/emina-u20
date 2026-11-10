package profile

import (
	"fmt"
	"math"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

// Problem is one validation failure at a document path.
type Problem struct {
	Path    string
	Message string
}

// ValidationError lists every problem found, not just the first.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.Path + ": " + p.Message
	}
	return "invalid face-architecture profile:\n  " + strings.Join(lines, "\n  ")
}

type checker struct {
	cat      *measureclient.Catalogue
	problems []Problem
}

func (c *checker) addf(path, format string, args ...any) {
	c.problems = append(c.problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

// measurement checks a key against the catalogue. scalar requires a float
// value: classifiers and trait rules compare numbers, so a list-valued
// measurement (fifths, jaw_contour_angles) cannot be referenced by them.
// Without a catalogue only emptiness is checked.
func (c *checker) measurement(path, key string, scalar bool) {
	if key == "" {
		c.addf(path, "measurement key is empty")
		return
	}
	if c.cat == nil {
		return
	}
	e, ok := c.cat.Entry(key)
	if !ok {
		c.addf(path, "measurement %q is not in catalogue %s", key, c.cat.CatalogueVersion)
		return
	}
	if !e.Supported {
		c.addf(path, "measurement %q is not supported by catalogue %s", key, c.cat.CatalogueVersion)
		return
	}
	if !scalar {
		return
	}
	shape, err := c.cat.Shape(key)
	if err != nil {
		c.addf(path, "%v", err)
		return
	}
	if shape != measureclient.ShapeScalar {
		c.addf(path, "measurement %q is list-valued; classifiers and trait rules take scalar measurements only", key)
	}
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Validate checks a document. With a catalogue (upsert, CLI validate) it
// also checks every measurement key against it; with nil (load) it checks
// structure only. It returns a *ValidationError listing every problem.
//
// Rules (spec "Validation"): keys exist and are supported; prototype and
// rule keys are scalar; spreads and weights are positive; blendMargin is in
// (0.5, 1]; minObservedWeight is in [0, 1]; guidance as in
// (*checker).guidance. An uncalibrated document must cite a source for each
// classifier's blendMargin, minObservedWeight and every class, for every
// trait, and for every guidance template and rule: a source covers path P
// when its "for" is P or a path under P.
func Validate(d *Document, cat *measureclient.Catalogue) error {
	c := &checker{cat: cat}
	if d.SchemaVersion != SchemaVersion {
		c.addf("schemaVersion", "got %d, this code reads %d", d.SchemaVersion, SchemaVersion)
	}
	switch {
	case d.CatalogueVersion == "":
		c.addf("catalogueVersion", "is empty")
	case cat != nil && d.CatalogueVersion != cat.CatalogueVersion:
		c.addf("catalogueVersion", "document targets %s, ai-worker catalogue is %s", d.CatalogueVersion, cat.CatalogueVersion)
	}
	c.calibration(d)

	seen := map[string]bool{}
	for i, k := range d.MeasurementsShown {
		path := fmt.Sprintf("measurementsShown[%d]", i)
		if seen[k] {
			c.addf(path, "duplicate %q", k)
		}
		seen[k] = true
		c.measurement(path, k, false)
	}

	keys := map[string]bool{}
	for i, s := range d.Classifiers {
		c.classifier(fmt.Sprintf("classifiers[%d]", i), s, keys)
	}
	keys = map[string]bool{}
	for i, t := range d.Traits {
		c.trait(fmt.Sprintf("traits[%d]", i), t, keys)
	}
	if d.Guidance != nil {
		c.guidance(d)
	}

	if d.Calibration.Status == CalibrationUncalibrated {
		c.coverage(d)
	}
	if len(c.problems) > 0 {
		return &ValidationError{Problems: c.problems}
	}
	return nil
}

func (c *checker) calibration(d *Document) {
	cal := d.Calibration
	switch cal.Status {
	case CalibrationUncalibrated:
	case CalibrationCalibrated:
		if cal.BenchmarkRunID == nil || *cal.BenchmarkRunID == "" {
			c.addf("calibration.benchmarkRunId", "required when status is calibrated")
		}
		if cal.Target == nil || *cal.Target == "" {
			c.addf("calibration.target", "required when status is calibrated")
		}
	default:
		c.addf("calibration.status", "must be %q or %q, got %q", CalibrationUncalibrated, CalibrationCalibrated, cal.Status)
	}
	for i, s := range cal.Sources {
		if strings.TrimSpace(s.Cite) == "" {
			c.addf(fmt.Sprintf("calibration.sources[%d].cite", i), "is empty")
		}
		if strings.TrimSpace(s.For) == "" {
			c.addf(fmt.Sprintf("calibration.sources[%d].for", i), "is empty")
		}
	}
}

func (c *checker) classifier(path string, s ClassifierSpec, keys map[string]bool) {
	if s.Key == "" {
		c.addf(path+".key", "is empty")
	} else if keys[s.Key] {
		c.addf(path+".key", "duplicate classifier %q", s.Key)
	}
	keys[s.Key] = true
	if !(s.BlendMargin > 0.5 && s.BlendMargin <= 1) {
		c.addf(path+".blendMargin", "must be in (0.5, 1], got %v", s.BlendMargin)
	}
	if !(s.MinObservedWeight >= 0 && s.MinObservedWeight <= 1) {
		c.addf(path+".minObservedWeight", "must be in [0, 1], got %v", s.MinObservedWeight)
	}
	if len(s.Classes) < 2 {
		c.addf(path+".classes", "needs at least 2 classes, got %d", len(s.Classes))
	}
	labels := map[string]bool{}
	for j, cl := range s.Classes {
		cp := fmt.Sprintf("%s.classes[%d]", path, j)
		if cl.Label == "" {
			c.addf(cp+".label", "is empty")
		} else if labels[cl.Label] {
			c.addf(cp+".label", "duplicate class %q", cl.Label)
		}
		labels[cl.Label] = true
		if len(cl.Prototype) == 0 {
			c.addf(cp+".prototype", "is empty")
		}
		for _, k := range cl.SortedKeys() {
			t := cl.Prototype[k]
			tp := cp + ".prototype." + k
			c.measurement(tp, k, true)
			if !finite(t.Centre) {
				c.addf(tp+".centre", "must be finite")
			}
			if !(t.Spread > 0) || !finite(t.Spread) {
				c.addf(tp+".spread", "must be positive and finite, got %v", t.Spread)
			}
			if !(t.Weight > 0) || !finite(t.Weight) {
				c.addf(tp+".weight", "must be positive and finite, got %v", t.Weight)
			}
		}
		req := map[string]bool{}
		for r, k := range cl.Requires {
			rp := fmt.Sprintf("%s.requires[%d]", cp, r)
			if req[k] {
				c.addf(rp, "duplicate %q", k)
			}
			req[k] = true
			c.measurement(rp, k, true)
		}
	}
}

func (c *checker) trait(path string, t TraitSpec, keys map[string]bool) {
	if t.Key == "" {
		c.addf(path+".key", "is empty")
	} else if keys[t.Key] {
		c.addf(path+".key", "duplicate trait %q", t.Key)
	}
	keys[t.Key] = true
	if t.Default == "" {
		c.addf(path+".default", "is empty")
	}
	if len(t.Rules) == 0 {
		c.addf(path+".rules", "needs at least 1 rule")
	}
	for i, r := range t.Rules {
		rp := fmt.Sprintf("%s.rules[%d]", path, i)
		if r.Label == "" {
			c.addf(rp+".label", "is empty")
		}
		if len(r.When) == 0 {
			c.addf(rp+".when", "needs at least 1 condition")
		}
		for j, cond := range r.When {
			cp := fmt.Sprintf("%s.when[%d]", rp, j)
			c.measurement(cp+".measurement", cond.Measurement, true)
			switch cond.Op {
			case OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
			default:
				c.addf(cp+".op", "must be one of < <= > >=, got %q", cond.Op)
			}
			if !finite(cond.Value) {
				c.addf(cp+".value", "must be finite")
			}
		}
	}
}

func (c *checker) coverage(d *Document) {
	var required []string
	for _, s := range d.Classifiers {
		base := "classifiers." + s.Key
		required = append(required, base+".blendMargin", base+".minObservedWeight")
		for _, cl := range s.Classes {
			required = append(required, base+".classes."+cl.Label)
		}
	}
	for _, t := range d.Traits {
		required = append(required, "traits."+t.Key)
	}
	if d.Guidance != nil {
		required = append(required, guidanceCoverage(d.Guidance)...)
	}
	for _, p := range required {
		if !covered(d.Calibration.Sources, p) {
			c.addf("calibration.sources", "no source cited for %s (uncalibrated profiles cite every value)", p)
		}
	}
}

func covered(sources []Source, path string) bool {
	for _, s := range sources {
		if s.For == path || strings.HasPrefix(s.For, path+".") {
			return true
		}
	}
	return false
}

// Load parses a stored profile's document and checks its structure. The
// catalogue is not consulted here: drift against the live catalogue is
// judged per request from the measure response (facearch.Interpret).
func Load(p *FaceArchitectureProfile) (*Document, error) {
	d, err := Parse([]byte(p.Document))
	if err != nil {
		return nil, err
	}
	if err := Validate(d, nil); err != nil {
		return nil, err
	}
	return d, nil
}

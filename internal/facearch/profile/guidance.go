package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// Guidance roles: what a template's polygon is for. The vocabulary is the
// spec's ("contour | highlight | blush | brow"); it names a product type,
// not a threshold.
const (
	RoleContour   = "contour"
	RoleHighlight = "highlight"
	RoleBlush     = "blush"
	RoleBrow      = "brow"
)

// Guidance is the document's makeup-placement section: named polygon
// templates placed from landmarks, and rules that choose them from
// classifier and trait results. Spec "Semantics → Guidance".
type Guidance struct {
	Templates map[string]Template `json:"templates"`
	Rules     []GuidanceRule      `json:"rules"`
}

// Template is one polygon, vertex by vertex.
type Template struct {
	Role   string  `json:"role"`
	Points []Point `json:"points"`
}

// Point is a landmark ({"index": i}) or a point between two landmarks
// ({"lerp": [a, b, t]}, a + t(b - a)), optionally shifted by OffsetIod:
// [dx, dy] in the worker's canonical frame, in eye-centre-distance units
// (+x toward the subject's left, which is image right; +y down; the eye
// line horizontal). Exactly one of Index and Lerp is set (Validate).
type Point struct {
	Index     *int    `json:"index,omitempty"`
	Lerp      *Lerp   `json:"lerp,omitempty"`
	OffsetIod *Offset `json:"offsetIod,omitempty"`
}

// Offset is [dx, dy]. It is a JSON array of exactly two numbers; a plain
// [2]float64 would truncate [0,1,2] and zero-pad [0].
type Offset [2]float64

// MarshalJSON writes [dx, dy].
func (o Offset) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]float64(o))
}

// UnmarshalJSON reads exactly two JSON numbers.
func (o *Offset) UnmarshalJSON(b []byte) error {
	nums, err := numberTokens(b)
	if err != nil {
		return fmt.Errorf("offsetIod must be [dx, dy]: %w", err)
	}
	if len(nums) != 2 {
		return fmt.Errorf("offsetIod must be [dx, dy], got %d elements", len(nums))
	}
	for i, n := range nums {
		v, err := n.Float64()
		if err != nil {
			return fmt.Errorf("offsetIod element %q: %w", n, err)
		}
		o[i] = v
	}
	return nil
}

// numberTokens decodes a JSON array whose every element must be a bare JSON
// number token; quoted numbers ("0.5") are refused, which json.Number alone
// would accept.
func numberTokens(b []byte) ([]json.Number, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(b, &elems); err != nil {
		return nil, err
	}
	out := make([]json.Number, len(elems))
	for i, e := range elems {
		t := bytes.TrimSpace(e)
		if len(t) == 0 || t[0] == '"' {
			return nil, fmt.Errorf("element %d is not a JSON number: %s", i, t)
		}
		out[i] = json.Number(t)
	}
	return out, nil
}

// Lerp is the JSON triple [a, b, t]: landmark indices a and b (integers)
// and t, the fraction of the way from a to b.
type Lerp struct {
	A, B int
	T    float64
}

// MarshalJSON writes [a, b, t].
func (l Lerp) MarshalJSON() ([]byte, error) {
	return json.Marshal([]json.Number{
		json.Number(strconv.Itoa(l.A)),
		json.Number(strconv.Itoa(l.B)),
		json.Number(strconv.FormatFloat(l.T, 'g', -1, 64)),
	})
}

// UnmarshalJSON reads [a, b, t]. a and b must be JSON integers: 1.5 or 1e2
// is not a landmark index and is refused rather than truncated.
func (l *Lerp) UnmarshalJSON(b []byte) error {
	raw, err := numberTokens(b)
	if err != nil {
		return fmt.Errorf("lerp must be [a, b, t]: %w", err)
	}
	if len(raw) != 3 {
		return fmt.Errorf("lerp must be [a, b, t], got %d elements", len(raw))
	}
	a, err := strconv.Atoi(raw[0].String())
	if err != nil {
		return fmt.Errorf("lerp landmark a %q is not an integer", raw[0])
	}
	bb, err := strconv.Atoi(raw[1].String())
	if err != nil {
		return fmt.Errorf("lerp landmark b %q is not an integer", raw[1])
	}
	t, err := raw[2].Float64()
	if err != nil {
		return fmt.Errorf("lerp t %q: %w", raw[2], err)
	}
	*l = Lerp{A: a, B: bb, T: t}
	return nil
}

// Anchors returns the landmark indices the point is built from.
func (p Point) Anchors() []int {
	switch {
	case p.Index != nil && p.Lerp == nil:
		return []int{*p.Index}
	case p.Lerp != nil && p.Index == nil:
		return []int{p.Lerp.A, p.Lerp.B}
	}
	return nil
}

// GuidanceRule applies templates when every When entry matches. When maps a
// classifier or trait key to the labels that match it: a classifier
// matches on its primary, or on its secondary when the result is a blend;
// a trait matches on its label. insufficient_evidence, not_assessable and
// unavailable results never match.
//
// Placeholder marks a rule whose choice of placements has no source (the
// document's calibration.sources says why). Every region the rule places
// repeats it, so a client can tell a sourced placement from a stand-in.
type GuidanceRule struct {
	ID          string              `json:"id"`
	When        map[string][]string `json:"when"`
	Apply       []Application       `json:"apply"`
	TextKeys    []string            `json:"textKeys"`
	Placeholder bool                `json:"placeholder,omitempty"`
}

// Application is one template a rule places, with its intensity: a
// strength in (0, 1] the client renders (opacity or amount), not a
// measurement.
type Application struct {
	Template  string  `json:"template"`
	Intensity float64 `json:"intensity"`
}

// TemplateNames returns the template names, sorted.
func (g *Guidance) TemplateNames() []string {
	names := make([]string, 0, len(g.Templates))
	for n := range g.Templates {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// WhenKeys returns the rule's When keys, sorted.
func (r GuidanceRule) WhenKeys() []string {
	keys := make([]string, 0, len(r.When))
	for k := range r.When {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

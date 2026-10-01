package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// RatioPrefix marks a results.csv attribute holding a vendor's number
// ("ratio:face_aspect_ratio"), compared by CompareMeasurements. evaluate and
// calibrate count such rows as unmapped, like any attribute the label map
// does not list.
const RatioPrefix = "ratio:"

// How our value(s) become a number on the vendor's scale.
const (
	// TransformIdentity: one scalar key, unchanged.
	TransformIdentity = "identity"
	// TransformInverse: one scalar key, 1/x.
	TransformInverse = "inverse"
	// TransformPercent: one list key, each element ×100.
	TransformPercent = "percent"
	// TransformSharePercent: several scalar keys, each as a percent of their sum.
	TransformSharePercent = "sharePercent"
)

// MeasureMap is measure-map.json: how one vendor's numbers line up with our
// measurements. It names the catalogue it was written for, which selects
// the cache directory.
type MeasureMap struct {
	CatalogueVersion string            `json:"catalogueVersion"`
	Source           string            `json:"source"`
	Cite             string            `json:"cite"`
	Comparisons      []Comparison      `json:"comparisons"`
	Unused           map[string]string `json:"unused"`
}

// Comparison is one vendor number against ours. Several Vendor fields (a
// left and a right) are averaged per image.
type Comparison struct {
	Name       string   `json:"name"`
	Vendor     []string `json:"vendor"`
	Components int      `json:"components"`
	Ours       []string `json:"ours"`
	Transform  string   `json:"transform"`
	Note       string   `json:"note"`
}

// ReadMeasureMap decodes measure-map.json strictly and checks its structure.
func ReadMeasureMap(path string) (*MeasureMap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m MeasureMap
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s: trailing data after the JSON object", path)
	}
	if err := m.Check(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Check reports every structural problem of the map.
func (m *MeasureMap) Check() error {
	var p []string
	if m.CatalogueVersion == "" {
		p = append(p, "catalogueVersion is empty")
	}
	if !ValidSource(m.Source) {
		p = append(p, fmt.Sprintf("source %q is not perfectcorp, wardah or makeover", m.Source))
	}
	if m.Cite == "" {
		p = append(p, "cite is empty")
	}
	if len(m.Comparisons) == 0 {
		p = append(p, "no comparisons")
	}
	names, vendor := map[string]bool{}, map[string]string{}
	for i, c := range m.Comparisons {
		at := fmt.Sprintf("comparisons[%d] %q", i, c.Name)
		if c.Name == "" {
			p = append(p, at+": name is empty")
		}
		if names[c.Name] {
			p = append(p, at+": name repeated")
		}
		names[c.Name] = true
		if len(c.Vendor) == 0 {
			p = append(p, at+": no vendor field")
		}
		for _, v := range c.Vendor {
			if prev, dup := vendor[v]; dup {
				p = append(p, fmt.Sprintf("%s: vendor field %q already used by %q", at, v, prev))
			}
			vendor[v] = c.Name
			if _, unused := m.Unused[v]; unused {
				p = append(p, fmt.Sprintf("%s: vendor field %q is also listed as unused", at, v))
			}
		}
		if c.Note == "" {
			p = append(p, at+": note is empty (state how the definitions differ)")
		}
		switch c.Transform {
		case TransformIdentity, TransformInverse:
			if len(c.Ours) != 1 || c.Components != 1 {
				p = append(p, at+": "+c.Transform+" needs one key and components 1")
			}
		case TransformPercent:
			if len(c.Ours) != 1 || c.Components < 2 {
				p = append(p, at+": percent needs one list key and components >= 2")
			}
		case TransformSharePercent:
			if len(c.Ours) < 2 || c.Components != len(c.Ours) {
				p = append(p, at+": sharePercent needs two or more keys and components equal to their number")
			}
		default:
			p = append(p, fmt.Sprintf("%s: unknown transform %q", at, c.Transform))
		}
	}
	for v, why := range m.Unused {
		if why == "" {
			p = append(p, fmt.Sprintf("unused %q: reason is empty", v))
		}
	}
	if len(p) > 0 {
		return fmt.Errorf("measure map:\n  %s", strings.Join(p, "\n  "))
	}
	return nil
}

// CheckShapes checks the map's keys that the cached catalogue lists: each
// must be supported and have the shape its transform reads. A key the
// catalogue lacks is not an error (the cache may predate it);
// CompareMeasurements reports that comparison as not comparable.
func (m *MeasureMap) CheckShapes(cat *measureclient.Catalogue) error {
	var p []string
	for _, c := range m.Comparisons {
		want := "scalar"
		if c.Transform == TransformPercent {
			want = "list"
		}
		for _, k := range c.Ours {
			e, ok := cat.Entry(k)
			if !ok {
				continue
			}
			if !e.Supported {
				p = append(p, fmt.Sprintf("%s: %s is not supported by catalogue %s", c.Name, k, cat.CatalogueVersion))
			} else if e.Shape != want {
				p = append(p, fmt.Sprintf("%s: %s is %s, %s needs %s", c.Name, k, e.Shape, c.Transform, want))
			}
		}
	}
	if len(p) > 0 {
		return fmt.Errorf("measure map against the cached catalogue:\n  %s", strings.Join(p, "\n  "))
	}
	return nil
}

// ParseVendorValue reads a number as a vendor's screen shows it. With
// components 1: "a:b" (b/a, a > 0) or a bare number. With more:
// exactly that many numbers separated by "/". Anything else, a comma decimal
// included, is not parsed: the caller excludes it rather than guess.
func ParseVendorValue(raw string, components int) ([]float64, bool) {
	s := strings.TrimSpace(raw)
	if components == 1 {
		if a, b, ok := strings.Cut(s, ":"); ok {
			x, okA := parseFinite(a)
			y, okB := parseFinite(b)
			if !okA || !okB || x <= 0 {
				return nil, false
			}
			return []float64{y / x}, true
		}
		v, ok := parseFinite(s)
		if !ok {
			return nil, false
		}
		return []float64{v}, true
	}
	parts := strings.Split(s, "/")
	if len(parts) != components {
		return nil, false
	}
	out := make([]float64, len(parts))
	for i, part := range parts {
		v, ok := parseFinite(part)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func parseFinite(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

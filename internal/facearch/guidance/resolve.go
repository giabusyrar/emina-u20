package guidance

import (
	"fmt"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Region is one placed template: a polygon in image pixels.
type Region struct {
	Role     string       `json:"role"`
	Template string       `json:"template"`
	Polygon  [][2]float64 `json:"polygon"`
	// Intensity is the rule's strength for this template, in (0, 1]; a
	// rendering parameter, not a measurement.
	Intensity float64  `json:"intensity"`
	RuleID    string   `json:"ruleId"`
	TextKeys  []string `json:"textKeys"`
	// Placeholder repeats the rule's flag: the choice of this placement has
	// no source. Omitted when false.
	Placeholder bool `json:"placeholder,omitempty"`
}

// Dropped is a template a matching rule applied that could not be placed.
type Dropped struct {
	Template string `json:"template"`
	RuleID   string `json:"ruleId"`
	Reason   string `json:"reason"`
}

// Result is the response's "guidance". Both lists are always present.
type Result struct {
	Regions          []Region  `json:"regions"`
	DroppedTemplates []Dropped `json:"droppedTemplates"`
}

// FrameReason is the drop reason when the eyes are occluded: the canonical
// frame is built from the eye corners (the ai-worker withholds every
// measurement for the same reason, in the same words).
const FrameReason = "canonical frame requires region " + measureclient.RegionEyes

// Resolve chooses and places guidance for one face. It never fails: a
// template that cannot be placed is listed in DroppedTemplates with the
// reason.
//
// Rules matched on primary classes (and trait labels) apply first, then
// rules matched only through a blend's secondary class, each group in
// document order. Each template is placed at most once, by the first rule
// that applies it; a later rule applying it again is skipped, including a
// template already dropped.
//
// A template is dropped when the eyes region is occluded (no canonical
// frame), when the landmarks are missing or malformed, or when any anchor
// landmark (an index, or either end of a lerp) is in a region the worker
// reports occluded, which includes regions out of frame, or does not report
// at all. Polygons are not clipped to the image.
func Resolve(g *profile.Guidance, ev Evidence, landmarks [][]float64, regions map[string]string) *Result {
	out := &Result{Regions: []Region{}, DroppedTemplates: []Dropped{}}
	var primary, secondary []profile.GuidanceRule
	for _, r := range g.Rules {
		switch Match(r, ev) {
		case MatchPrimary:
			primary = append(primary, r)
		case MatchSecondary:
			secondary = append(secondary, r)
		}
	}
	if len(primary)+len(secondary) == 0 {
		return out
	}

	frame, frameReason := frameFor(landmarks, regions)
	done := map[string]bool{}
	for _, r := range append(primary, secondary...) {
		for _, a := range r.Apply {
			if done[a.Template] {
				continue
			}
			done[a.Template] = true
			drop := func(reason string) {
				out.DroppedTemplates = append(out.DroppedTemplates, Dropped{Template: a.Template, RuleID: r.ID, Reason: reason})
			}
			t, ok := g.Templates[a.Template]
			switch {
			case !ok:
				drop("template is not defined")
				continue
			case frameReason != "":
				drop(frameReason)
				continue
			}
			if reason := hidden(t, regions); reason != "" {
				drop(reason)
				continue
			}
			poly, err := Place(t, frame, landmarks)
			if err != nil {
				drop(err.Error())
				continue
			}
			out.Regions = append(out.Regions, Region{
				Role: t.Role, Template: a.Template, Polygon: poly, Intensity: a.Intensity,
				RuleID: r.ID, TextKeys: append([]string{}, r.TextKeys...), Placeholder: r.Placeholder,
			})
		}
	}
	return out
}

func frameFor(landmarks [][]float64, regions map[string]string) (Frame, string) {
	if regions[measureclient.RegionEyes] == measureclient.VisibilityOccluded {
		return Frame{}, FrameReason
	}
	if landmarks == nil {
		return Frame{}, "the ai-worker returned no landmarks"
	}
	f, err := NewFrame(landmarks)
	if err != nil {
		return Frame{}, "landmarks unusable: " + err.Error()
	}
	return f, ""
}

// hidden explains why a template's anchors cannot be trusted, or returns "".
func hidden(t profile.Template, regions map[string]string) string {
	for _, p := range t.Points {
		for _, i := range p.Anchors() {
			names := measureclient.RegionsOf(i)
			if len(names) == 0 {
				return fmt.Sprintf("anchor landmark %d is in no ai-worker region", i)
			}
			for _, name := range names {
				state, ok := regions[name]
				switch {
				case !ok:
					return fmt.Sprintf("region %s not reported by the ai-worker (anchor landmark %d)", name, i)
				case state == measureclient.VisibilityOccluded:
					return fmt.Sprintf("region %s is occluded (anchor landmark %d)", name, i)
				}
			}
		}
	}
	return ""
}

// Place resolves a template's points to image pixels on one face: each
// anchor goes into the canonical frame, a lerp is taken there, the offset
// (IOD units) is added, and the point is mapped back to the image. It does
// not judge occlusion (Resolve does).
func Place(t profile.Template, f Frame, landmarks [][]float64) ([][2]float64, error) {
	at := func(i int) ([2]float64, error) {
		if i < 0 || i >= len(landmarks) || len(landmarks[i]) != 2 {
			return [2]float64{}, fmt.Errorf("landmark %d is not available", i)
		}
		return f.ToCanonical([2]float64{landmarks[i][0], landmarks[i][1]}), nil
	}
	poly := make([][2]float64, 0, len(t.Points))
	for j, p := range t.Points {
		var q [2]float64
		switch {
		case p.Index != nil && p.Lerp == nil:
			c, err := at(*p.Index)
			if err != nil {
				return nil, err
			}
			q = c
		case p.Lerp != nil && p.Index == nil:
			a, err := at(p.Lerp.A)
			if err != nil {
				return nil, err
			}
			b, err := at(p.Lerp.B)
			if err != nil {
				return nil, err
			}
			q = [2]float64{a[0] + p.Lerp.T*(b[0]-a[0]), a[1] + p.Lerp.T*(b[1]-a[1])}
		default:
			return nil, fmt.Errorf("point %d needs exactly one of index and lerp", j)
		}
		if p.OffsetIod != nil {
			q[0] += p.OffsetIod[0]
			q[1] += p.OffsetIod[1]
		}
		poly = append(poly, f.ToImage(q))
	}
	return poly, nil
}

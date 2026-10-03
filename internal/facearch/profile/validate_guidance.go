package profile

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

// minPolygonPoints: a polygon needs three vertices (geometry, not policy).
const minPolygonPoints = 3

// guidance checks the guidance section (spec "Validation": every template
// index is 0-477). Rules:
//   - the document's catalogueVersion is measureclient.RegionsCatalogueVersion:
//     anchors are checked against the worker region table recorded for that
//     version, which another version may define differently;
//   - template names and rule ids contain no "." (a source's "for" path
//     guidance.templates.a would otherwise also cover a template "a.b");
//   - templates: known role; at least 3 points; each point exactly one of
//     index / lerp; indices in [0, measureclient.NLandmarks); lerp ends
//     differ and t is in [0, 1]; offsetIod finite; every anchor landmark
//     belongs to at least one ai-worker region (measureclient.RegionsOf),
//     because a landmark in no region has no visibility, so its occlusion
//     could never be detected;
//   - rules: unique non-empty id; at least one When entry, each naming
//     exactly one classifier or trait of this document and only labels it
//     can produce; at least one application, each of a defined template,
//     no template twice, intensity in (0, 1]; at least one text key, none
//     empty or containing whitespace.
func (c *checker) guidance(d *Document) {
	g := d.Guidance
	if d.CatalogueVersion != measureclient.RegionsCatalogueVersion {
		c.addf("guidance", "the ai-worker region table is recorded for %s, document targets %s", measureclient.RegionsCatalogueVersion, d.CatalogueVersion)
	}
	if len(g.Templates) == 0 {
		c.addf("guidance.templates", "needs at least 1 template")
	}
	if len(g.Rules) == 0 {
		c.addf("guidance.rules", "needs at least 1 rule")
	}
	for _, name := range g.TemplateNames() {
		c.template("guidance.templates."+name, name, g.Templates[name])
	}
	labels, ambiguous := labelSets(d)
	ids := map[string]bool{}
	for i, r := range g.Rules {
		c.guidanceRule(fmt.Sprintf("guidance.rules[%d]", i), r, g, labels, ambiguous, ids)
	}
}

func (c *checker) template(path, name string, t Template) {
	if strings.TrimSpace(name) == "" {
		c.addf(path, "template name is empty")
	}
	if strings.Contains(name, ".") {
		c.addf(path, "template name must not contain %q (source paths are dot-separated)", ".")
	}
	switch t.Role {
	case RoleContour, RoleHighlight, RoleBlush, RoleBrow:
	default:
		c.addf(path+".role", "must be one of %s, %s, %s, %s, got %q", RoleContour, RoleHighlight, RoleBlush, RoleBrow, t.Role)
	}
	if len(t.Points) < minPolygonPoints {
		c.addf(path+".points", "a polygon needs at least %d points, got %d", minPolygonPoints, len(t.Points))
	}
	for i, p := range t.Points {
		pp := fmt.Sprintf("%s.points[%d]", path, i)
		switch {
		case p.Index == nil && p.Lerp == nil:
			c.addf(pp, "needs index or lerp")
		case p.Index != nil && p.Lerp != nil:
			c.addf(pp, "has both index and lerp")
		case p.Index != nil:
			c.anchor(pp+".index", *p.Index)
		default:
			c.anchor(pp+".lerp[0]", p.Lerp.A)
			c.anchor(pp+".lerp[1]", p.Lerp.B)
			if p.Lerp.A == p.Lerp.B {
				c.addf(pp+".lerp", "landmarks a and b are both %d", p.Lerp.A)
			}
			if !(p.Lerp.T >= 0 && p.Lerp.T <= 1) {
				c.addf(pp+".lerp[2]", "t must be in [0, 1], got %v", p.Lerp.T)
			}
		}
		if p.OffsetIod != nil && (!finite(p.OffsetIod[0]) || !finite(p.OffsetIod[1])) {
			c.addf(pp+".offsetIod", "must be finite")
		}
	}
}

func (c *checker) anchor(path string, i int) {
	if i < 0 || i >= measureclient.NLandmarks {
		c.addf(path, "landmark %d is outside 0-%d", i, measureclient.NLandmarks-1)
		return
	}
	if len(measureclient.RegionsOf(i)) == 0 {
		c.addf(path, "landmark %d is in no ai-worker region, so its occlusion cannot be judged; anchor on region landmarks (lerp between them to reach other points)", i)
	}
}

// labelSets maps each classifier and trait key to the labels it can
// produce. A key that is both a classifier and a trait is ambiguous in a
// rule's When.
func labelSets(d *Document) (map[string]map[string]bool, map[string]bool) {
	labels := map[string]map[string]bool{}
	ambiguous := map[string]bool{}
	for _, s := range d.Classifiers {
		set := map[string]bool{}
		for _, cl := range s.Classes {
			set[cl.Label] = true
		}
		labels[s.Key] = set
	}
	for _, t := range d.Traits {
		if _, ok := labels[t.Key]; ok {
			ambiguous[t.Key] = true
		}
		set := map[string]bool{t.Default: true}
		for _, r := range t.Rules {
			set[r.Label] = true
		}
		labels[t.Key] = set
	}
	return labels, ambiguous
}

func (c *checker) guidanceRule(path string, r GuidanceRule, g *Guidance, labels map[string]map[string]bool, ambiguous, ids map[string]bool) {
	if strings.TrimSpace(r.ID) == "" {
		c.addf(path+".id", "is empty")
	} else if ids[r.ID] {
		c.addf(path+".id", "duplicate rule %q", r.ID)
	}
	if strings.Contains(r.ID, ".") {
		c.addf(path+".id", "rule id must not contain %q (source paths are dot-separated)", ".")
	}
	ids[r.ID] = true

	if len(r.When) == 0 {
		c.addf(path+".when", "needs at least 1 classifier or trait")
	}
	for _, key := range r.WhenKeys() {
		wp := path + ".when." + key
		set, ok := labels[key]
		switch {
		case !ok:
			c.addf(wp, "no classifier or trait %q in this document", key)
			continue
		case ambiguous[key]:
			c.addf(wp, "%q is both a classifier and a trait", key)
			continue
		}
		if len(r.When[key]) == 0 {
			c.addf(wp, "needs at least 1 label")
		}
		seen := map[string]bool{}
		for j, l := range r.When[key] {
			lp := fmt.Sprintf("%s[%d]", wp, j)
			if !set[l] {
				c.addf(lp, "%q cannot produce label %q (labels: %s)", key, l, strings.Join(sortedSet(set), ", "))
			}
			if seen[l] {
				c.addf(lp, "duplicate label %q", l)
			}
			seen[l] = true
		}
	}

	if len(r.Apply) == 0 {
		c.addf(path+".apply", "needs at least 1 template")
	}
	applied := map[string]bool{}
	for j, a := range r.Apply {
		ap := fmt.Sprintf("%s.apply[%d]", path, j)
		if _, ok := g.Templates[a.Template]; !ok {
			c.addf(ap+".template", "no template %q", a.Template)
		}
		if applied[a.Template] {
			c.addf(ap+".template", "template %q applied twice", a.Template)
		}
		applied[a.Template] = true
		if !(a.Intensity > 0 && a.Intensity <= 1) {
			c.addf(ap+".intensity", "must be in (0, 1], got %v", a.Intensity)
		}
	}

	if len(r.TextKeys) == 0 {
		c.addf(path+".textKeys", "needs at least 1 i18n key")
	}
	for j, k := range r.TextKeys {
		if k == "" || strings.IndexFunc(k, unicode.IsSpace) >= 0 {
			c.addf(fmt.Sprintf("%s.textKeys[%d]", path, j), "must be a non-empty i18n key without whitespace, got %q", k)
		}
	}
}

// guidanceCoverage lists the source paths an uncalibrated document's
// guidance must cite: every template (its geometry) and every rule (its
// choice of placements and its intensities).
func guidanceCoverage(g *Guidance) []string {
	var paths []string
	for _, name := range g.TemplateNames() {
		paths = append(paths, "guidance.templates."+name)
	}
	ids := make([]string, 0, len(g.Rules))
	for _, r := range g.Rules {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		paths = append(paths, "guidance.rules."+id)
	}
	return paths
}

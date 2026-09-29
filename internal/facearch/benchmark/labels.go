package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// What a label-map attribute is compared with.
const (
	KindClassifier = "classifier"
	KindTrait      = "trait"
)

// LabelMap is label-map.json: for each results.csv attribute, the profile
// classifier or trait it is compared with, and per source family
// (perfectcorp, wardah, makeover, rater) each vendor value's label in our
// vocabulary. A JSON null label means the source gave no answer (for
// example Perfect Corp's "Unknown"); that row is counted as abstained,
// never compared.
type LabelMap struct {
	Attributes map[string]AttributeMap `json:"attributes"`
}

// AttributeMap maps one results.csv attribute.
type AttributeMap struct {
	// Ours is a classifier key or trait key of the profile.
	Ours string `json:"ours"`
	// Values is family -> vendor value -> our label, or null for "no answer".
	Values map[string]map[string]*string `json:"values"`
}

// Target is the profile entry an attribute is compared with.
type Target struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

// ReadLabelMap decodes label-map.json strictly (unknown fields are errors).
func ReadLabelMap(path string) (*LabelMap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m LabelMap
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s: trailing data after the JSON object", path)
	}
	if len(m.Attributes) == 0 {
		return nil, fmt.Errorf("%s: no attributes", path)
	}
	return &m, nil
}

// emittable returns the labels each classifier and trait of doc can answer.
func emittable(doc *profile.Document) map[Target]map[string]bool {
	out := map[Target]map[string]bool{}
	for _, c := range doc.Classifiers {
		set := map[string]bool{}
		for _, cl := range c.Classes {
			set[cl.Label] = true
		}
		out[Target{Kind: KindClassifier, Key: c.Key}] = set
	}
	for _, t := range doc.Traits {
		set := map[string]bool{t.Default: true}
		for _, r := range t.Rules {
			set[r.Label] = true
		}
		out[Target{Kind: KindTrait, Key: t.Key}] = set
	}
	return out
}

// Resolve checks the map against a profile and returns each attribute's
// target. Every "ours" must name exactly one classifier or trait, no two
// attributes may share one, families must be known, and every non-null
// label must be one the classifier or trait can answer.
func (m *LabelMap) Resolve(doc *profile.Document) (map[string]Target, error) {
	labels := emittable(doc)
	out := map[string]Target{}
	used := map[Target]string{}
	var problems []string
	for _, attr := range sortedKeys(m.Attributes) {
		am := m.Attributes[attr]
		var found []Target
		for _, kind := range []string{KindClassifier, KindTrait} {
			if _, ok := labels[Target{Kind: kind, Key: am.Ours}]; ok {
				found = append(found, Target{Kind: kind, Key: am.Ours})
			}
		}
		switch len(found) {
		case 0:
			problems = append(problems, fmt.Sprintf("%s: ours %q is not a classifier or trait of the profile", attr, am.Ours))
			continue
		case 2:
			problems = append(problems, fmt.Sprintf("%s: ours %q is both a classifier and a trait", attr, am.Ours))
			continue
		}
		t := found[0]
		if prev, dup := used[t]; dup {
			problems = append(problems, fmt.Sprintf("%s: ours %q is already mapped by %s", attr, am.Ours, prev))
		}
		used[t] = attr
		out[attr] = t
		for _, fam := range sortedKeys(am.Values) {
			switch fam {
			case SourcePerfectCorp, SourceWardah, SourceMakeover, FamilyRater:
			default:
				problems = append(problems, fmt.Sprintf("%s: unknown source family %q (want perfectcorp, wardah, makeover or rater)", attr, fam))
			}
			for _, v := range sortedKeys(am.Values[fam]) {
				if l := am.Values[fam][v]; l != nil && !labels[t][*l] {
					problems = append(problems, fmt.Sprintf("%s: %s value %q maps to %q, which %s %s cannot answer", attr, fam, v, *l, t.Kind, t.Key))
				}
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("label map does not fit the profile:\n  %s", strings.Join(problems, "\n  "))
	}
	return out, nil
}

// LabelSet is results.csv in our vocabulary.
type LabelSet struct {
	// Targets maps each label-map attribute to its classifier or trait.
	Targets map[string]Target
	// Labels is attribute -> source -> image id -> our label. Sources
	// include TargetRaterConsensus when any rater labelled the attribute.
	// Abstentions are absent.
	Labels map[string]map[string]map[string]string
	// Abstained counts null-mapped rows per attribute and source.
	Abstained map[string]map[string]int
	// Unmapped counts results.csv rows per attribute the label map does not
	// list (for example Wardah's raw measurements); they are not compared.
	Unmapped map[string]int
	// NoConsensus counts, per attribute, images that raters labelled
	// without reaching a consensus.
	NoConsensus map[string]int
}

// BuildLabels maps results through the label map. A vendor value the map
// does not list is an error, never a guess. The rater consensus of an
// image is the label given by a strict majority of the raters who answered,
// and needs at least minConsensusRaters answers.
func BuildLabels(results []ResultRow, m *LabelMap, targets map[string]Target, minConsensusRaters int) (*LabelSet, error) {
	if minConsensusRaters < 1 {
		return nil, fmt.Errorf("minimum consensus raters must be at least 1, got %d", minConsensusRaters)
	}
	ls := &LabelSet{
		Targets:     targets,
		Labels:      map[string]map[string]map[string]string{},
		Abstained:   map[string]map[string]int{},
		Unmapped:    map[string]int{},
		NoConsensus: map[string]int{},
	}
	var problems []string
	for _, r := range results {
		am, ok := m.Attributes[r.Attribute]
		if !ok {
			ls.Unmapped[r.Attribute]++
			continue
		}
		fam := SourceFamily(r.Source)
		label, ok := am.Values[fam][r.Value]
		if !ok {
			problems = append(problems, fmt.Sprintf("results line %d: %s value %q for %s is not in the label map", r.Line, fam, r.Value, r.Attribute))
			continue
		}
		if label == nil {
			if ls.Abstained[r.Attribute] == nil {
				ls.Abstained[r.Attribute] = map[string]int{}
			}
			ls.Abstained[r.Attribute][r.Source]++
			continue
		}
		ls.put(r.Attribute, r.Source, r.ImageID, *label)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("unmapped labels:\n  %s", strings.Join(problems, "\n  "))
	}
	for attr, bySource := range ls.Labels {
		votes := map[string]map[string]int{}
		for src, byImage := range bySource {
			if !strings.HasPrefix(src, RaterPrefix) {
				continue
			}
			for img, l := range byImage {
				if votes[img] == nil {
					votes[img] = map[string]int{}
				}
				votes[img][l]++
			}
		}
		for img, v := range votes {
			total := 0
			for _, n := range v {
				total += n
			}
			winner := ""
			for l, n := range v {
				if 2*n > total {
					winner = l
				}
			}
			if total < minConsensusRaters || winner == "" {
				ls.NoConsensus[attr]++
				continue
			}
			ls.put(attr, TargetRaterConsensus, img, winner)
		}
	}
	return ls, nil
}

func (ls *LabelSet) put(attr, source, img, label string) {
	if ls.Labels[attr] == nil {
		ls.Labels[attr] = map[string]map[string]string{}
	}
	if ls.Labels[attr][source] == nil {
		ls.Labels[attr][source] = map[string]string{}
	}
	ls.Labels[attr][source][img] = label
}

// Sources returns every source that labelled attr, sorted.
func (ls *LabelSet) Sources(attr string) []string {
	return sortedKeys(ls.Labels[attr])
}

// ForTarget returns, per our classifier or trait key, the labels the given
// source (or TargetRaterConsensus) gave: key -> image id -> label.
func (ls *LabelSet) ForTarget(source string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for attr, t := range ls.Targets {
		if byImage := ls.Labels[attr][source]; len(byImage) > 0 {
			out[t.Key] = byImage
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

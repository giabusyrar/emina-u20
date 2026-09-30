package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// The collection kit (docs/FACE-BENCHMARK.md, "Collecting a data set") is
// generated from the shipped seed profile; these tests fail when the seed's
// vocabulary changes and the kit does not.
const (
	kitDir       = "kit"
	kitSeedPath  = "../facearch-profile/seed/seagull-default-uncalibrated.json"
	kitLabelMap  = "label-map.template.json"
	kitManifest  = "manifest.template.csv"
	kitResults   = "results.template.csv"
	kitFamilyRat = "rater"
)

func kitSeed(t *testing.T) *profile.Document {
	t.Helper()
	raw, err := os.ReadFile(kitSeedPath)
	require.NoError(t, err)
	d, err := profile.Load(&profile.FaceArchitectureProfile{Document: string(raw)})
	require.NoError(t, err)
	return d
}

func TestKitLabelMapFitsTheSeed(t *testing.T) {
	d := kitSeed(t)
	m, err := benchmark.ReadLabelMap(filepath.Join(kitDir, kitLabelMap))
	require.NoError(t, err)
	targets, err := m.Resolve(d)
	require.NoError(t, err)
	assert.Len(t, targets, len(d.Classifiers)+len(d.Traits), "one attribute per classifier and trait")

	labels := map[string][]string{}
	for _, c := range d.Classifiers {
		for _, cl := range c.Classes {
			labels[c.Key] = append(labels[c.Key], cl.Label)
		}
	}
	for _, tr := range d.Traits {
		for _, r := range tr.Rules {
			labels[tr.Key] = append(labels[tr.Key], r.Label)
		}
		labels[tr.Key] = append(labels[tr.Key], tr.Default)
	}
	for attr, am := range m.Attributes {
		assert.Equal(t, attr, am.Ours, "results.csv attributes use our keys")
		rater := am.Values[kitFamilyRat]
		assert.Len(t, rater, len(labels[attr]), "%s: raters answer in our vocabulary, every label", attr)
		for _, l := range labels[attr] {
			if assert.Contains(t, rater, l, attr) {
				assert.Equal(t, l, *rater[l], attr)
			}
		}
		// Vendor wording is collected by hand, never guessed: until a person
		// fills it in, any vendor row stops the run as "not in the label map".
		for _, fam := range []string{benchmark.SourcePerfectCorp, benchmark.SourceWardah, benchmark.SourceMakeover} {
			assert.Empty(t, am.Values[fam], "%s %s", attr, fam)
		}
	}
}

func TestKitCSVTemplatesHaveTheRequiredHeaders(t *testing.T) {
	for file, header := range map[string]string{
		kitManifest: "image_id,image_path,subject_id,consent_ref,hijab,notes",
		kitResults:  "image_id,source,attribute,value",
	} {
		raw, err := os.ReadFile(filepath.Join(kitDir, file))
		require.NoError(t, err)
		assert.Equal(t, header, strings.TrimSpace(string(raw)), file)
	}
}

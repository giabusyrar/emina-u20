package benchmark_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/benchmark"
)

// bom is the UTF-8 byte-order mark spreadsheet exports start with.
const bom = "\xef\xbb\xbf"

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	// A spreadsheet export: byte-order mark, columns in another order, spaces.
	path := writeFile(t, dir, "manifest.csv", bom+"subject_id,image_id,image_path,hijab,consent_ref,notes\n"+
		"s1, a , photos/a.jpg,Yes,c-1,\n"+
		"s1,b,"+filepath.Join(dir, "b.png")+",0,c-2,second visit\n")
	rows, err := benchmark.ReadManifest(path)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, benchmark.ManifestRow{ImageID: "a", ImagePath: filepath.Join(dir, "photos", "a.jpg"), SubjectID: "s1", ConsentRef: "c-1", Hijab: true}, rows[0])
	assert.Equal(t, filepath.Join(dir, "b.png"), rows[1].ImagePath)
	assert.False(t, rows[1].Hijab)
	assert.Equal(t, "second visit", rows[1].Notes)
}

// A BOM before a quoted first header cell and CRLF line endings, as some
// spreadsheet exports write them.
func TestReadManifestBOMQuotedCRLF(t *testing.T) {
	path := writeFile(t, t.TempDir(), "manifest.csv", bom+"\"image_id\",image_path,subject_id,consent_ref,hijab,notes\r\n"+
		"a,a.jpg,s1,c-1,no,\r\n")
	rows, err := benchmark.ReadManifest(path)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "a", rows[0].ImageID)
}

func TestReadManifestRejects(t *testing.T) {
	const header = "image_id,image_path,subject_id,consent_ref,hijab,notes\n"
	for name, tc := range map[string]struct{ body, want string }{
		"no consent":      {header + "a,a.jpg,s1,,false,\n", "line 2: consent_ref is required"},
		"no subject":      {header + "a,a.jpg,,c,false,\n", "line 2: subject_id is required"},
		"duplicate id":    {header + "a,a.jpg,s1,c,false,\na,b.jpg,s2,c,true,\n", `line 3: image_id "a" repeats line 2`},
		"bad hijab":       {header + "a,a.jpg,s1,c,maybe,\n", `hijab must be true/false, yes/no or 1/0, got "maybe"`},
		"missing column":  {"image_id,image_path,subject_id,hijab,notes\n", `missing column "consent_ref"`},
		"unknown column":  {"image_id,image_path,subject_id,consent_ref,hijab,notes,name\n", `unknown column "name"`},
		"empty":           {"", "empty file"},
		"no images":       {header, "no images"},
		"short record":    {header + "a,a.jpg,s1\n", "wrong number of fields"},
		"duplicate colum": {"image_id,image_id,image_path,subject_id,consent_ref,hijab,notes\n", `duplicate column "image_id"`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := benchmark.ReadManifest(writeFile(t, t.TempDir(), "manifest.csv", tc.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestReadResults(t *testing.T) {
	dir := t.TempDir()
	manifest := []benchmark.ManifestRow{{ImageID: "a"}, {ImageID: "b"}}
	rows, err := benchmark.ReadResults(writeFile(t, dir, "results.csv",
		"image_id,source,attribute,value\na,perfectcorp,faceShape,Oval\na,rater:r1,faceShape,oval\nb,wardah,faceBalance,Curved Thin\n"), manifest)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, benchmark.ResultRow{Line: 3, ImageID: "a", Source: "rater:r1", Attribute: "faceShape", Value: "oval"}, rows[1])
	assert.Equal(t, "rater", benchmark.SourceFamily("rater:r1"))
	assert.Equal(t, "wardah", benchmark.SourceFamily("wardah"))

	_, err = benchmark.ReadResults(writeFile(t, dir, "bad.csv",
		"image_id,source,attribute,value\na,perfect,faceShape,Oval\nz,wardah,faceShape,Oval\na,rater:,faceShape,Oval\nb,makeover,faceShape,Oval\nb,makeover,faceShape,Round\n"), manifest)
	require.Error(t, err)
	for _, want := range []string{
		`line 2: source must be perfectcorp, wardah, makeover or rater:<id>, got "perfect"`,
		`line 3: image_id "z" is not in the manifest`,
		`line 4: source must be`,
		`line 6: b/makeover/faceShape repeats line 5`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}

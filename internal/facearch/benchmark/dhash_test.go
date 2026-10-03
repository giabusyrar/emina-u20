package benchmark_test

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/benchmark"
)

// gradient is w x h grey, brightness rising left to right (or falling when
// reverse), scaled into [lo, hi].
func gradient(w, h int, reverse bool, lo, hi float64) *image.Gray {
	im := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x) / float64(w-1)
			if reverse {
				t = 1 - t
			}
			im.SetGray(x, y, color.Gray{Y: uint8(lo + t*(hi-lo))})
		}
	}
	return im
}

// halves is dark on the top half and a rising gradient on the bottom half.
func halves(w, h int) *image.Gray {
	im := gradient(w, h, true, 0, 255)
	for y := h / 2; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetGray(x, y, color.Gray{Y: uint8(255 * x / (w - 1))})
		}
	}
	return im
}

func writePNG(t *testing.T, dir, name string, im image.Image) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, im))
	require.NoError(t, f.Close())
	return path
}

func TestDHashKnownAnswers(t *testing.T) {
	// Rising left to right: no cell is brighter than its right neighbour.
	h, err := benchmark.DHash(gradient(90, 80, false, 0, 255))
	require.NoError(t, err)
	assert.Equal(t, uint64(0), h)
	// Falling: every cell is brighter than its right neighbour.
	h, err = benchmark.DHash(gradient(90, 80, true, 0, 255))
	require.NoError(t, err)
	assert.Equal(t, ^uint64(0), h)
	// Top four rows falling, bottom four rising: the low 32 bits set.
	h, err = benchmark.DHash(halves(90, 80))
	require.NoError(t, err)
	assert.Equal(t, uint64(0xFFFFFFFF), h)

	_, err = benchmark.DHash(gradient(8, 8, false, 0, 255))
	assert.ErrorContains(t, err, "dHash needs at least 9x8")
}

func TestDHashFileIgnoresResizeAndRecompressionButNotContent(t *testing.T) {
	dir := t.TempDir()
	orig := writePNG(t, dir, "orig.png", halves(180, 160))
	smaller := writePNG(t, dir, "smaller.png", halves(90, 80))
	other := writePNG(t, dir, "other.png", gradient(180, 160, false, 0, 255))
	jpegPath := filepath.Join(dir, "orig.jpg")
	f, err := os.Create(jpegPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(f, halves(180, 160), &jpeg.Options{Quality: 75}))
	require.NoError(t, f.Close())

	hash := func(p string) uint64 {
		h, err := benchmark.DHashFile(p, "img-"+filepath.Base(p))
		require.NoError(t, err)
		return h
	}
	assert.Equal(t, 0, benchmark.Hamming(hash(orig), hash(smaller)), "resized copy")
	assert.LessOrEqual(t, benchmark.Hamming(hash(orig), hash(jpegPath)), 2, "JPEG re-encode")
	assert.Equal(t, 32, benchmark.Hamming(hash(orig), hash(other)))

	_, err = benchmark.DHashFile(writeFile(t, dir, "not-an-image.jpg", "text"), "img-7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image img-7: ")
	assert.Contains(t, err.Error(), "JPEG and PNG are supported")
	assert.NotContains(t, err.Error(), dir)
	assert.NotContains(t, err.Error(), "not-an-image")
}

func TestDHashFileErrorsNameTheIDNotThePath(t *testing.T) {
	dir := t.TempDir()
	_, err := benchmark.DHashFile(filepath.Join(dir, "secret-subject-name.jpg"), "img-9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image img-9: ")
	assert.NotContains(t, err.Error(), dir)
	assert.NotContains(t, err.Error(), "secret-subject-name")

	small := writePNG(t, dir, "tiny-secret.png", gradient(4, 4, false, 0, 255))
	_, err = benchmark.DHashFile(small, "img-10")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image img-10: ")
	assert.Contains(t, err.Error(), "dHash needs at least 9x8")
	assert.NotContains(t, err.Error(), "tiny-secret")
}

func TestDedupKeepsTheFirstOfEachNearDuplicateGroup(t *testing.T) {
	res := benchmark.Dedup([]benchmark.HashedImage{
		{ImageID: "a", SubjectID: "s1", Hash: 0},
		{ImageID: "b", SubjectID: "s1", Hash: 0b111},     // 3 bits from a
		{ImageID: "c", SubjectID: "s2", Hash: 0xFF},      // 8 bits from a: kept
		{ImageID: "d", SubjectID: "s3", Hash: 0xFF << 1}, // 2 bits from c, other subject
	}, 3)
	assert.Equal(t, []string{"a", "c"}, res.Kept)
	assert.Equal(t, []benchmark.Duplicate{
		{ImageID: "b", KeptID: "a", Distance: 3},
		{ImageID: "d", KeptID: "c", Distance: 2, CrossSubject: true},
	}, res.Removed)
}

func TestDedupIsGreedyAndNotTransitive(t *testing.T) {
	// a~b (3 bits), b~c (3 bits), a!~c (6 bits): c is compared only with
	// kept images, so it survives even though it neighbours the removed b.
	res := benchmark.Dedup([]benchmark.HashedImage{
		{ImageID: "a", SubjectID: "s", Hash: 0},
		{ImageID: "b", SubjectID: "s", Hash: 0b111},
		{ImageID: "c", SubjectID: "s", Hash: 0b111111},
	}, 3)
	assert.Equal(t, []string{"a", "c"}, res.Kept)
	assert.Equal(t, []benchmark.Duplicate{{ImageID: "b", KeptID: "a", Distance: 3}}, res.Removed)
}

func TestDedupEmptyInput(t *testing.T) {
	res := benchmark.Dedup(nil, 3)
	assert.Empty(t, res.Kept)
	assert.Empty(t, res.Removed)
}

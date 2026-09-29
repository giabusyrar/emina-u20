package benchmark

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoding (standard library)
	_ "image/png"  // register PNG decoding (standard library)
	"math/bits"
	"os"
)

// The difference hash (dHash; N. Krawetz, "Kind of Like That",
// hackerfactor.com, 2013) is defined on a 9x8 (width x height) greyscale
// grid: each pixel is compared with its right neighbour, so each of the 8
// rows gives 8 comparisons, 8 x 8 = 64 bits in all (DHashBits). These are
// the definition, not settings.
const (
	dhashWidth  = 9
	dhashHeight = 8
)

// Compile-time check that the grid and DHashBits agree: this fails to build
// (constant overflow of uint) if (dhashWidth-1)*dhashHeight != DHashBits.
const (
	_ = uint((dhashWidth-1)*dhashHeight - DHashBits)
	_ = uint(DHashBits - (dhashWidth-1)*dhashHeight)
)

// DHash returns the 64-bit difference hash of img: the image is reduced to
// 9 columns x 8 rows of cells by averaging every pixel in each cell (box
// filter), in ITU-R BT.601 luma (the weights of image/color.GrayModel); bit
// y*8+x is set when cell (x, y) is brighter than cell (x+1, y). Images
// smaller than 9x8 pixels are refused. The bit direction is a convention;
// Hamming distances do not depend on it.
func DHash(img image.Image) (uint64, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < dhashWidth || h < dhashHeight {
		return 0, fmt.Errorf("image is %dx%d, dHash needs at least %dx%d", w, h, dhashWidth, dhashHeight)
	}
	var sum [dhashHeight][dhashWidth]float64
	var count [dhashHeight][dhashWidth]int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		cy := (y - b.Min.Y) * dhashHeight / h
		for x := b.Min.X; x < b.Max.X; x++ {
			cx := (x - b.Min.X) * dhashWidth / w
			r, g, bl, _ := img.At(x, y).RGBA()
			sum[cy][cx] += float64(19595*r+38470*g+7471*bl+1<<15) / 65536
			count[cy][cx]++
		}
	}
	var hash uint64
	for y := 0; y < dhashHeight; y++ {
		for x := 0; x < dhashWidth-1; x++ {
			left := sum[y][x] / float64(count[y][x])
			right := sum[y][x+1] / float64(count[y][x+1])
			if left > right {
				hash |= 1 << uint(y*(dhashWidth-1)+x)
			}
		}
	}
	return hash, nil
}

// DHashFile decodes an image file (JPEG or PNG: standard-library decoders
// only) and returns its dHash. EXIF orientation is not applied, so the
// same photo stored rotated hashes differently.
func DHashFile(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode (JPEG and PNG are supported): %w", err)
	}
	h, err := DHash(img)
	if err != nil {
		return 0, fmt.Errorf("%s image: %w", format, err)
	}
	return h, nil
}

// Hamming is the number of differing bits.
func Hamming(a, b uint64) int { return bits.OnesCount64(a ^ b) }

// HashedImage is one measured image's hash.
type HashedImage struct {
	ImageID   string
	SubjectID string
	Hash      uint64
}

// Duplicate is an image removed as a near-duplicate of a kept one.
type Duplicate struct {
	ImageID  string `json:"-"`
	KeptID   string `json:"-"`
	Distance int    `json:"distance"`
	// CrossSubject: the two images carry different subject_ids, which
	// usually means a manifest error (or a threshold loose enough to join
	// different people); the report counts these.
	CrossSubject bool `json:"crossSubject"`
}

// DedupResult says which images survive.
type DedupResult struct {
	Kept    []string
	Removed []Duplicate
}

// Dedup walks images in order and keeps each one unless its Hamming
// distance to an already kept image is at most maxDistance; the first
// such kept image is recorded as its twin. Keeping the earliest is
// deterministic for a given manifest order.
func Dedup(images []HashedImage, maxDistance int) DedupResult {
	var res DedupResult
	var kept []HashedImage
	for _, im := range images {
		dup := false
		for _, k := range kept {
			if d := Hamming(im.Hash, k.Hash); d <= maxDistance {
				res.Removed = append(res.Removed, Duplicate{ImageID: im.ImageID, KeptID: k.ImageID, Distance: d, CrossSubject: im.SubjectID != k.SubjectID})
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, im)
			res.Kept = append(res.Kept, im.ImageID)
		}
	}
	return res
}

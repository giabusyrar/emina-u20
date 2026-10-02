package orchestrator

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uprightPortrait is 40 wide, 80 tall, red in its top-left quarter, blue elsewhere.
func uprightPortrait() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 40, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 40; x++ {
			c := color.RGBA{B: 255, A: 255}
			if x < 20 && y < 40 {
				c = color.RGBA{R: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// rotateCCW is the inverse of orientation 6: how a phone stores a portrait
// sideways before tagging it orientation 6.
func rotateCCW(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetRGBA(y, b.Dx()-1-x, img.RGBAAt(x, y))
		}
	}
	return out
}

// withOrientation inserts an APP1 EXIF block carrying orientation o right
// after the JPEG's SOI marker.
func withOrientation(t *testing.T, jpg []byte, o uint16, order binary.ByteOrder, tag string) []byte {
	t.Helper()
	tiff := new(bytes.Buffer)
	tiff.WriteString(tag)
	_ = binary.Write(tiff, order, uint16(42))
	_ = binary.Write(tiff, order, uint32(8))
	_ = binary.Write(tiff, order, uint16(1))      // one IFD entry
	_ = binary.Write(tiff, order, uint16(0x0112)) // orientation
	_ = binary.Write(tiff, order, uint16(3))      // SHORT
	_ = binary.Write(tiff, order, uint32(1))
	_ = binary.Write(tiff, order, o)
	_ = binary.Write(tiff, order, uint16(0))
	_ = binary.Write(tiff, order, uint32(0)) // no next IFD
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(payload)+2))
	out := append([]byte{}, jpg[:2]...)
	out = append(out, app1...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}))
	return buf.Bytes()
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xC000 && g < 0x4000 && b < 0x4000
}

func TestExifOrientation_SidewaysPortraitIsAnalysedUpright(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order binary.ByteOrder
		tag   string
	}{{"little-endian", binary.LittleEndian, "II"}, {"big-endian", binary.BigEndian, "MM"}} {
		t.Run(tc.name, func(t *testing.T) {
			stored := encodeJPEG(t, rotateCCW(uprightPortrait())) // 80 wide, 40 tall on disk
			data := withOrientation(t, stored, 6, tc.order, tc.tag)
			require.Equal(t, 6, exifOrientation(data))

			decoded, _, err := image.Decode(bytes.NewReader(data))
			require.NoError(t, err)
			assert.Equal(t, 80, decoded.Bounds().Dx(), "Go's decoder alone leaves it sideways")

			upright := applyOrientation(decoded, exifOrientation(data))
			assert.Equal(t, 40, upright.Bounds().Dx())
			assert.Equal(t, 80, upright.Bounds().Dy())
			assert.True(t, isRed(upright.At(5, 5)), "the top-left marker is back at the top left")
			assert.False(t, isRed(upright.At(35, 75)))
		})
	}
}

func TestExifOrientation_EveryCodeRoundTrips(t *testing.T) {
	src := uprightPortrait()
	// For each orientation, store the inverse transform and check applyOrientation restores the original.
	inverse := map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 8, 7: 7, 8: 6}
	for o := 1; o <= 8; o++ {
		stored := applyOrientation(src, inverse[o])
		back := applyOrientation(stored, o)
		require.Equal(t, src.Bounds().Size(), back.Bounds().Size(), "orientation %d", o)
		assert.True(t, isRed(back.At(5, 5)), "orientation %d", o)
		assert.False(t, isRed(back.At(35, 75)), "orientation %d", o)
	}
}

func TestExifOrientation_NoExifOrNotJPEGIsUpright(t *testing.T) {
	assert.Equal(t, 1, exifOrientation(encodeJPEG(t, uprightPortrait())))
	assert.Equal(t, 1, exifOrientation([]byte("\x89PNG\r\n\x1a\n")))
	assert.Equal(t, 1, exifOrientation(nil))
}

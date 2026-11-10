package orchestrator

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

func solidImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 180, B: 170, A: 255})
		}
	}
	return img
}

func decodedSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return decoded.Bounds().Dx(), decoded.Bounds().Dy()
}

func TestCropZone_EncodesBoundingBoxAsPNG(t *testing.T) {
	data, ok := cropZone(solidImage(100, 100), domain.BoundingBox{MinX: 0.25, MinY: 0.25, MaxX: 0.75, MaxY: 0.5})

	require.True(t, ok)
	width, height := decodedSize(t, data)
	assert.Equal(t, 50, width)
	assert.Equal(t, 25, height)
}

func TestCropZone_SkipsBoxesSmallerThan16Pixels(t *testing.T) {
	_, ok := cropZone(solidImage(100, 100), domain.BoundingBox{MinX: 0.5, MinY: 0.25, MaxX: 0.625, MaxY: 0.5})

	assert.False(t, ok)
}

func TestCropZone_ClampsBoxToImage(t *testing.T) {
	data, ok := cropZone(solidImage(100, 100), domain.BoundingBox{MinX: -0.5, MinY: -0.5, MaxX: 0.25, MaxY: 1.5})

	require.True(t, ok)
	width, height := decodedSize(t, data)
	assert.Equal(t, 25, width)
	assert.Equal(t, 100, height)
}

func TestCropZone_NilImageIsSkipped(t *testing.T) {
	_, ok := cropZone(nil, domain.BoundingBox{MaxX: 1, MaxY: 1})

	assert.False(t, ok)
}

func TestCropZone_IsLossless(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for x := 0; x < 40; x++ {
		for y := 0; y < 40; y++ {
			src.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 6), B: uint8((x + y) * 3), A: 255})
		}
	}
	data, ok := cropZone(src, domain.BoundingBox{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1})
	require.True(t, ok)
	decoded, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	for x := 0; x < 40; x++ {
		for y := 0; y < 40; y++ {
			r1, g1, b1, _ := src.At(x, y).RGBA()
			r2, g2, b2, _ := decoded.At(x, y).RGBA()
			require.Equal(t, [3]uint32{r1, g1, b1}, [3]uint32{r2, g2, b2}, "pixel %d,%d changed", x, y)
		}
	}
}

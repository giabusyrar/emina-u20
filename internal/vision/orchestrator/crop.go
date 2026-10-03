package orchestrator

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

// minCropPixels: a crop narrower or shorter than this is not scored.
const minCropPixels = 16

// Crops are PNG, lossless, so the scoring models never see compression
// artefacts. BestSpeed keeps the encode cheap; the crops are small.
var cropEncoder = png.Encoder{CompressionLevel: png.BestSpeed}

// cropZone cuts a zone's normalized bounding box out of its source image and
// encodes it as PNG. It reports false for a missing image or a crop smaller
// than 16x16 pixels.
func cropZone(img image.Image, box domain.BoundingBox) ([]byte, bool) {
	if img == nil {
		return nil, false
	}

	bounds := img.Bounds()
	width, height := float64(bounds.Dx()), float64(bounds.Dy())
	rect := image.Rect(
		bounds.Min.X+int(math.Floor(clampUnit(box.MinX)*width)),
		bounds.Min.Y+int(math.Floor(clampUnit(box.MinY)*height)),
		bounds.Min.X+int(math.Ceil(clampUnit(box.MaxX)*width)),
		bounds.Min.Y+int(math.Ceil(clampUnit(box.MaxY)*height)),
	).Intersect(bounds)

	if rect.Dx() < minCropPixels || rect.Dy() < minCropPixels {
		return nil, false
	}

	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), img, rect.Min, draw.Src)

	var buf bytes.Buffer
	if err := cropEncoder.Encode(&buf, cropped); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func clampUnit(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

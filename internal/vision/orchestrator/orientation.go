package orchestrator

import (
	"encoding/binary"
	"image"
	"image/draw"
)

// EXIF orientation (TIFF tag 0x0112), values 1-8 as defined by the EXIF
// standard (CIPA DC-008): how the stored pixels must be transformed to display
// the photo upright. Phones store a portrait sideways and set 6 or 8.
const (
	exifOrientationTag  = 0x0112
	orientationUpright  = 1
	orientationLastCode = 8
)

// exifOrientation reads the orientation tag from a JPEG's EXIF block. It
// returns 1 (upright) when the data is not a JPEG, has no EXIF, or the tag is
// missing or out of range: there is nothing to undo.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return orientationUpright
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return orientationUpright
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image: no EXIF before the pixels
			return orientationUpright
		}
		size := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if size < 2 || i+2+size > len(data) {
			return orientationUpright
		}
		payload := data[i+4 : i+2+size]
		if marker == 0xE1 && len(payload) >= 6 && string(payload[:6]) == "Exif\x00\x00" {
			return tiffOrientation(payload[6:])
		}
		i += 2 + size
	}
	return orientationUpright
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return orientationUpright
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return orientationUpright
	}
	ifd := int(order.Uint32(tiff[4:8]))
	if ifd+2 > len(tiff) {
		return orientationUpright
	}
	count := int(order.Uint16(tiff[ifd : ifd+2]))
	for n := 0; n < count; n++ {
		entry := ifd + 2 + n*12
		if entry+12 > len(tiff) {
			return orientationUpright
		}
		if order.Uint16(tiff[entry:entry+2]) != exifOrientationTag {
			continue
		}
		v := int(order.Uint16(tiff[entry+8 : entry+10])) // SHORT, stored in the value field
		if v < orientationUpright || v > orientationLastCode {
			return orientationUpright
		}
		return v
	}
	return orientationUpright
}

// applyOrientation returns the photo as it is meant to be seen: the stored
// pixels transformed by EXIF orientation o. Orientation 1 returns img itself.
func applyOrientation(img image.Image, o int) image.Image {
	if o <= orientationUpright || o > orientationLastCode {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 { // 5-8 swap width and height
		dw, dh = h, w
	}
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // mirrored horizontally
				sx, sy = w-1-x, y
			case 3: // rotated 180
				sx, sy = w-1-x, h-1-y
			case 4: // mirrored vertically
				sx, sy = x, h-1-y
			case 5: // transposed
				sx, sy = y, x
			case 6: // stored rotated 90 counter-clockwise: rotate 90 clockwise
				sx, sy = y, h-1-x
			case 7: // transversed
				sx, sy = w-1-y, h-1-x
			case 8: // stored rotated 90 clockwise: rotate 90 counter-clockwise
				sx, sy = w-1-y, x
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

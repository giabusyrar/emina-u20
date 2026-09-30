// Package guidance places a profile's makeup templates on one face and
// chooses them with the profile's rules.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Guidance".
package guidance

import (
	"errors"
	"fmt"
	"math"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// Frame is the face worker's canonical frame, rebuilt from the landmarks it
// returned (apps/workers/face/worker_face/facegeom/frame.py,
// canonicalise): origin at the midpoint of the two eye centres, an eye
// centre being the midpoint of that eye's two corners; rotated so the eye
// line is horizontal; lengths in eye-centre distances (IOD); +x toward the
// subject's left (image right), +y down. Template offsets are in this frame.
type Frame struct {
	mid      [2]float64
	cos, sin float64 // of the eye line's angle in the image (the roll)
	iod      float64
}

// NewFrame builds the frame from 478 [x, y] image-pixel landmarks.
func NewFrame(landmarks [][]float64) (Frame, error) {
	if len(landmarks) != measureclient.NLandmarks {
		return Frame{}, fmt.Errorf("want %d landmarks, got %d", measureclient.NLandmarks, len(landmarks))
	}
	for i, p := range landmarks {
		if len(p) != 2 || !finite(p[0]) || !finite(p[1]) {
			return Frame{}, fmt.Errorf("landmark %d is not a finite [x, y]", i)
		}
	}
	right := mid(landmarks[measureclient.ExoR], landmarks[measureclient.EndoR])
	left := mid(landmarks[measureclient.ExoL], landmarks[measureclient.EndoL])
	dx, dy := left[0]-right[0], left[1]-right[1]
	iod := math.Hypot(dx, dy)
	if !(iod > 0) {
		return Frame{}, errors.New("eye centres coincide")
	}
	if math.IsInf(iod, 0) {
		// Finite but extreme landmarks can overflow; every offset would then
		// map to infinity or NaN in pixels.
		return Frame{}, errors.New("eye-centre distance is not finite")
	}
	return Frame{
		mid: [2]float64{(right[0] + left[0]) / 2, (right[1] + left[1]) / 2},
		cos: dx / iod,
		sin: dy / iod,
		iod: iod,
	}, nil
}

func mid(a, b []float64) [2]float64 { return [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2} }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// IodPx is the eye-centre distance in pixels.
func (f Frame) IodPx() float64 { return f.iod }

// ToCanonical maps an image point into the frame: rotate by minus the roll
// about the origin, divide by IOD (frame.py: (p - mid) @ rot.T / iod).
func (f Frame) ToCanonical(p [2]float64) [2]float64 {
	x, y := p[0]-f.mid[0], p[1]-f.mid[1]
	return [2]float64{(f.cos*x + f.sin*y) / f.iod, (-f.sin*x + f.cos*y) / f.iod}
}

// ToImage is the inverse of ToCanonical.
func (f Frame) ToImage(q [2]float64) [2]float64 {
	x, y := q[0]*f.iod, q[1]*f.iod
	return [2]float64{f.mid[0] + f.cos*x - f.sin*y, f.mid[1] + f.sin*x + f.cos*y}
}

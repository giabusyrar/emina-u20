package domain

import "math"

// DeltaE2000 is the CIEDE2000 colour difference between two CIELAB colours
// (CIE 142-2001), with the parametric factors kL = kC = kH = 1. The formula
// and its test pairs are from G. Sharma, W. Wu, E. N. Dalal, "The CIEDE2000
// Color-Difference Formula", Color Res. Appl. 30(1), 2005.
func DeltaE2000(x, y Lab) float64 {
	const deg = math.Pi / 180
	L1, a1, b1 := x[0], x[1], x[2]
	L2, a2, b2 := y[0], y[1], y[2]

	cBar := (math.Hypot(a1, b1) + math.Hypot(a2, b2)) / 2
	c7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(c7/(c7+math.Pow(25, 7))))
	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p, c2p := math.Hypot(a1p, b1), math.Hypot(a2p, b2)
	h1p, h2p := hueDeg(b1, a1p), hueDeg(b2, a2p)

	dLp := L2 - L1
	dCp := c2p - c1p
	var dhp float64
	switch {
	case c1p*c2p == 0:
		dhp = 0
	case math.Abs(h2p-h1p) <= 180:
		dhp = h2p - h1p
	case h2p-h1p > 180:
		dhp = h2p - h1p - 360
	default:
		dhp = h2p - h1p + 360
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(dhp/2*deg)

	lBarp := (L1 + L2) / 2
	cBarp := (c1p + c2p) / 2
	var hBarp float64
	switch {
	case c1p*c2p == 0:
		hBarp = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hBarp = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hBarp = (h1p + h2p + 360) / 2
	default:
		hBarp = (h1p + h2p - 360) / 2
	}
	t := 1 - 0.17*math.Cos((hBarp-30)*deg) + 0.24*math.Cos(2*hBarp*deg) +
		0.32*math.Cos((3*hBarp+6)*deg) - 0.20*math.Cos((4*hBarp-63)*deg)
	dTheta := 30 * math.Exp(-math.Pow((hBarp-275)/25, 2))
	cBarp7 := math.Pow(cBarp, 7)
	rC := 2 * math.Sqrt(cBarp7/(cBarp7+math.Pow(25, 7)))
	sL := 1 + 0.015*math.Pow(lBarp-50, 2)/math.Sqrt(20+math.Pow(lBarp-50, 2))
	sC := 1 + 0.045*cBarp
	sH := 1 + 0.015*cBarp*t
	rT := -math.Sin(2*dTheta*deg) * rC

	l, c, h := dLp/sL, dCp/sC, dHp/sH
	return math.Sqrt(l*l + c*c + h*h + rT*c*h)
}

// hueDeg is atan2(b, a) in degrees, 0..360, and 0 for a neutral.
func hueDeg(b, a float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

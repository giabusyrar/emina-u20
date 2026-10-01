package domain

import (
	"math"
	"sort"
)

// Munsell is what the colour worker returns for a colour (WCPA notebook §1).
type Munsell struct {
	V            float64 `json:"V"`
	C            float64 `json:"C"`
	HueFamily    string  `json:"hueFamily"`
	HueAngle     float64 `json:"hueAngle"`
	HueWardah    string  `json:"hueWardah"`
	ChromaSource string  `json:"chromaSource"`
}

// Lab is CIELAB D65.
type Lab [3]float64

// TemperatureOf: temperature from b* - a* (notebook temperature_dari_lab). Returns the label
// and the b*-a* value.
func (c *Config) TemperatureOf(lab Lab) (string, float64) {
	d := lab[2] - lab[1]
	switch {
	case d >= c.Temperature.WarmAtLeast:
		return "warm", d
	case d <= c.Temperature.CoolAtMost:
		return "cool", d
	default:
		return "neutral", d
	}
}

// snap is the notebook's snap_kuadran: nearest centre in scale-normalised V/C.
// Ties resolve to the quadrant listed first in the config, so results do not
// depend on map iteration order.
func (c *Config) snap(v, ch float64, centres map[string][2]float64, scale [2]float64) (string, float64) {
	best, bestD := "", math.Inf(1)
	for _, q := range c.Quadrants {
		p := centres[q.Code]
		dv := (v - p[0]) / scale[0]
		dc := (ch - p[1]) / scale[1]
		d := dv*dv + dc*dc
		if d < bestD {
			best, bestD = q.Code, d
		}
	}
	return best, math.Round(math.Sqrt(bestD)*100) / 100
}

// PersonQuadrant is the notebook's kuadran_orang: quadrant code, value group,
// chroma group, distance to the quadrant centre.
func (c *Config) PersonQuadrant(v, ch float64) (code, valueGroup, chromaGroup string, dist float64) {
	code, dist = c.snap(v, ch, c.Person.Centres, c.Person.Scale)
	switch {
	case v >= c.Person.ValueHigh:
		valueGroup = "high"
	case v < c.Person.ValueLow:
		valueGroup = "low"
	default:
		valueGroup = "medium"
	}
	switch {
	case ch >= c.Person.ChromaHigh:
		chromaGroup = "high"
	case ch < c.Person.ChromaLow:
		chromaGroup = "low"
	default:
		chromaGroup = "medium"
	}
	return
}

// ShadeQuadrant is the notebook's shade_kuadran with the shade bands from config.
func (c *Config) ShadeQuadrant(v, ch float64) (code, valueGroup, chromaGroup string) {
	code, _ = c.snap(v, ch, c.Shade.Centres, c.Shade.Scale)
	switch {
	case v >= c.Shade.Value["high"][0]:
		valueGroup = "high"
	case v <= c.Shade.Value["low"][1]:
		valueGroup = "low"
	default:
		valueGroup = "medium"
	}
	if ch >= c.Shade.Chroma["high"][0] {
		chromaGroup = "high"
	} else {
		chromaGroup = "low"
	}
	return
}

// FoundationBand maps skin L* to the Cocory foundation band.
func (c *Config) FoundationBand(l float64) string {
	for _, b := range c.FoundationBands.Bands {
		if l > b.Above {
			return b.Name
		}
	}
	return c.FoundationBands.Bands[len(c.FoundationBands.Bands)-1].Name
}

// HueAngle of a Lab colour in degrees [0, 360).
func HueAngle(lab Lab) float64 {
	h := math.Atan2(lab[2], lab[1]) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

// HueName is the ISCC-NBS colour name of lab (ISCCNBSName). It replaced the
// notebook's wardah_hue_name, whose families went by hue angle alone and so
// named saturated reds "Peach": in CIELAB a vivid red and a peach share a
// hue angle and differ in lightness and chroma. LoadConfig checks the
// centroid table, so the error branch is unreachable after startup; it
// returns no name rather than a wrong one.
func (c *Config) HueName(lab Lab) string {
	name, err := ISCCNBSName(lab)
	if err != nil {
		return ""
	}
	return name
}

// HueNameOf is the shade's hue name, or "" for a category not named
// (config hueNames.categories).
func (c *Config) HueNameOf(s ShadeCandidate) string {
	if !c.NamesHue(s.Category) {
		return ""
	}
	return c.HueName(s.Lab)
}

// temperatureAllowed is the notebook's cocok_temperature.
func temperatureAllowed(shadeTemp, rule string) bool {
	switch rule {
	case "":
		return true
	case "neutral":
		return shadeTemp == "neutral"
	case "neutral_to_cool":
		return shadeTemp == "neutral" || shadeTemp == "cool"
	case "neutral_to_warm":
		return shadeTemp == "neutral" || shadeTemp == "warm"
	default:
		return true
	}
}

// ShadeCandidate is a catalog shade with its WCPA colorimetry.
type ShadeCandidate struct {
	ShadeID      string
	ProductID    string
	ProductName  string
	ShadeName    string
	Category     string
	HexColor     string
	Lab          Lab
	V, C         float64
	ColourSource string // SourceCube | SourceSwatch | SourceEstimate
}

// Rating of a shade against a person's quadrant.
const (
	RatingSameQuadrant  = "same_quadrant"
	RatingComplementary = "complementary"
)

// Recommendation is one shade in the result, with why it was kept.
type Recommendation struct {
	ShadeCandidate
	Quadrant    string
	Status      string // RatingSameQuadrant | RatingComplementary
	Distance    float64
	HueName     string
	Temperature string
}

// Recommend is the notebook's rekomendasi(): same quadrant first, then the
// complementary quadrants whose temperature rule the shade satisfies; within a
// status, closest to the person's V/C first. Shades outside are dropped.
//
// Deviation, deliberate: the notebook sorts by the status *string*, which puts
// "komplementer" before "kuadran_sama" alphabetically — the opposite of its own
// §8 text ("same quadrant → safe; complementary → allowed"). Same quadrant
// comes first here.
func (c *Config) Recommend(personQuadrant string, personV, personC float64, shades []ShadeCandidate) []Recommendation {
	var out []Recommendation
	for _, s := range shades {
		q, status, temp := c.Rate(personQuadrant, s)
		if status == "" {
			continue
		}
		out = append(out, Recommendation{
			ShadeCandidate: s,
			Quadrant:       q,
			Status:         status,
			Distance:       math.Round((math.Abs(s.V-personV)/2+math.Abs(s.C-personC)/2)*100) / 100,
			HueName:        c.HueNameOf(s),
			Temperature:    temp,
		})
	}
	rank := map[string]int{RatingSameQuadrant: 0, RatingComplementary: 1}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Status] != rank[out[j].Status] {
			return rank[out[i].Status] < rank[out[j].Status]
		}
		return out[i].Distance < out[j].Distance
	})
	if len(out) > c.Recommendation.PerCategory {
		out = out[:c.Recommendation.PerCategory]
	}
	return out
}

// Rate is one shade against a person: the shade's quadrant, its rating
// (RatingSameQuadrant, RatingComplementary, or "" when the rules keep it
// out) and its temperature. Recommend and the try-on catalog share it.
func (c *Config) Rate(personQuadrant string, s ShadeCandidate) (quadrant, rating, temperature string) {
	comp := c.Complementary.Map[personQuadrant]
	q, _, _ := c.ShadeQuadrant(s.V, s.C)
	temp, _ := c.TemperatureOf(s.Lab)
	if q == personQuadrant {
		return q, RatingSameQuadrant, temp
	}
	for _, k := range comp.Codes {
		if k == q && temperatureAllowed(temp, comp.Temperature) {
			return q, RatingComplementary, temp
		}
	}
	return q, "", temp
}

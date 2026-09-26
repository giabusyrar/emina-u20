package domain

type ImageAngle string

const (
	AngleFront ImageAngle = "FRONT"
	AngleLeft  ImageAngle = "LEFT"
	AngleRight ImageAngle = "RIGHT"
)

const (
	ZoneForehead           = "ZONE_FOREHEAD"
	ZoneTZoneNose          = "ZONE_T_ZONE_NOSE"
	ZoneLeftCheekInner     = "ZONE_LEFT_CHEEK_INNER"
	ZoneLeftCheekOuter     = "ZONE_LEFT_CHEEK_OUTER"
	ZoneRightCheekInner    = "ZONE_RIGHT_CHEEK_INNER"
	ZoneRightCheekOuter    = "ZONE_RIGHT_CHEEK_OUTER"
	ZonePeriorbitalEyelids = "ZONE_PERIORBITAL_EYELIDS"
	ZoneChinJawline        = "ZONE_CHIN_JAWLINE"
)

type NormalizedPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type BoundingBox struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

type FacialZoneDefinition struct {
	Code           string            `json:"code"`
	Name           string            `json:"name"`
	DefaultPolygon []NormalizedPoint `json:"defaultPolygon"`
}

func GetStandardFacialZones() []FacialZoneDefinition {
	return []FacialZoneDefinition{
		{
			Code: ZoneForehead,
			Name: "Forehead",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.25, Y: 0.12}, {X: 0.50, Y: 0.08}, {X: 0.75, Y: 0.12},
				{X: 0.72, Y: 0.30}, {X: 0.28, Y: 0.30},
			},
		},
		{
			Code: ZoneTZoneNose,
			Name: "T-Zone & Nose",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.44, Y: 0.30}, {X: 0.56, Y: 0.30}, {X: 0.58, Y: 0.62},
				{X: 0.50, Y: 0.66}, {X: 0.42, Y: 0.62},
			},
		},
		{
			Code: ZoneLeftCheekInner,
			Name: "Left Cheek (Inner)",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.28, Y: 0.45}, {X: 0.42, Y: 0.42}, {X: 0.42, Y: 0.65}, {X: 0.26, Y: 0.62},
			},
		},
		{
			Code: ZoneLeftCheekOuter,
			Name: "Left Cheek (Outer)",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.15, Y: 0.38}, {X: 0.28, Y: 0.45}, {X: 0.26, Y: 0.68}, {X: 0.14, Y: 0.58},
			},
		},
		{
			Code: ZoneRightCheekInner,
			Name: "Right Cheek (Inner)",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.58, Y: 0.42}, {X: 0.72, Y: 0.45}, {X: 0.74, Y: 0.62}, {X: 0.58, Y: 0.65},
			},
		},
		{
			Code: ZoneRightCheekOuter,
			Name: "Right Cheek (Outer)",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.72, Y: 0.45}, {X: 0.85, Y: 0.38}, {X: 0.86, Y: 0.58}, {X: 0.74, Y: 0.68},
			},
		},
		{
			Code: ZonePeriorbitalEyelids,
			Name: "Periorbital & Eyelids",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.22, Y: 0.32}, {X: 0.42, Y: 0.32}, {X: 0.42, Y: 0.42}, {X: 0.22, Y: 0.42},
				{X: 0.58, Y: 0.32}, {X: 0.78, Y: 0.32}, {X: 0.78, Y: 0.42}, {X: 0.58, Y: 0.42},
			},
		},
		{
			Code: ZoneChinJawline,
			Name: "Chin & Jawline",
			DefaultPolygon: []NormalizedPoint{
				{X: 0.32, Y: 0.68}, {X: 0.68, Y: 0.68}, {X: 0.62, Y: 0.90},
				{X: 0.50, Y: 0.94}, {X: 0.38, Y: 0.90},
			},
		},
	}
}

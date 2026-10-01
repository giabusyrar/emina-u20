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

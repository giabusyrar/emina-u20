package domain

// Where a catalog shade's colour comes from, best first. The API reports it
// with every shade, so the UI can say how far to trust the colour.
const (
	SourceCube     = "cube"     // measured on skin with the cube (ground truth)
	SourceSwatch   = "swatch"   // the product's swatch hex ([SWATCH] in the catalog CSV)
	SourceEstimate = "estimate" // estimated for a demo, not from the brand ([PERKIRAAN] in the catalog CSV)
)

// SourceRank orders colour sources best first; an unknown source ranks last.
func SourceRank(source string) int {
	switch source {
	case SourceCube:
		return 0
	case SourceSwatch:
		return 1
	case SourceEstimate:
		return 2
	}
	return 3
}

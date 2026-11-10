package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sync"
)

// The ISCC-NBS colour-name blocks are a published standard (Kelly 1958, NBS
// RP2911), not a deployment choice, so their centroids live here rather than
// in wcpa.json. isccnbs/centroids.tsv is the source table; centroids_lab.json
// is generated from it by isccnbs/build_lab.py.
//
//go:embed isccnbs/centroids_lab.json
var isccnbsJSON []byte

// ISCCNBSCentroid is one of the 267 ISCC-NBS colour-name blocks.
type ISCCNBSCentroid struct {
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Munsell string `json:"munsell"`
	Open    bool   `json:"open"`
	Lab     Lab    `json:"lab"`
}

var (
	isccnbsOnce      sync.Once
	isccnbsCentroids []ISCCNBSCentroid
	isccnbsErr       error
)

// ISCCNBSCentroids returns the 267 centroids, parsed once.
func ISCCNBSCentroids() ([]ISCCNBSCentroid, error) {
	isccnbsOnce.Do(func() {
		var doc struct {
			Centroids []ISCCNBSCentroid `json:"centroids"`
		}
		if err := json.Unmarshal(isccnbsJSON, &doc); err != nil {
			isccnbsErr = fmt.Errorf("ISCC-NBS centroids: %w", err)
			return
		}
		if len(doc.Centroids) != 267 {
			isccnbsErr = fmt.Errorf("ISCC-NBS centroids: %d, want 267", len(doc.Centroids))
			return
		}
		isccnbsCentroids = doc.Centroids
	})
	return isccnbsCentroids, isccnbsErr
}

// ISCCNBSName is the ISCC-NBS level-3 name of the block whose centroid is
// nearest to lab by CIEDE2000, e.g. "vivid red", "light yellowish pink".
//
// Nearest centroid approximates block membership: the standard defines each
// block by Munsell boundaries, and a colour near a boundary can be closer to
// a neighbouring block's centroid. For the 32 blocks Kelly marks "+" (open in
// chroma) the centroid is a lower bound, so very saturated colours lean
// towards them less than the blocks themselves would.
func ISCCNBSName(lab Lab) (string, error) {
	cs, err := ISCCNBSCentroids()
	if err != nil {
		return "", err
	}
	best, bestD := "", math.Inf(1)
	for _, c := range cs {
		if d := DeltaE2000(lab, c.Lab); d < bestD {
			best, bestD = c.Name, d
		}
	}
	return best, nil
}

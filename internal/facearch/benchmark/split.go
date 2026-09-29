package benchmark

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
)

// Split assigns whole subjects to training or held-out, so no person's
// face is on both sides.
type Split struct {
	Train   []string
	HeldOut []string
}

// SplitSubjects ranks the distinct subject ids by SHA-256(seed as 8
// big-endian bytes || subject id) and holds out the first
// round(heldOutFraction x N), at least 1 and at most N-1. The ranking does
// not depend on input order, on Go's random number generators or on the
// Go version.
func SplitSubjects(subjects []string, heldOutFraction float64, seed uint64) (Split, error) {
	if !(heldOutFraction > 0 && heldOutFraction < 1) {
		return Split{}, fmt.Errorf("held-out fraction must be in (0, 1), got %v", heldOutFraction)
	}
	set := map[string]bool{}
	for _, s := range subjects {
		set[s] = true
	}
	if len(set) < 2 {
		return Split{}, fmt.Errorf("need at least 2 subjects to split, got %d", len(set))
	}
	type ranked struct{ id, key string }
	var seedBytes [8]byte
	binary.BigEndian.PutUint64(seedBytes[:], seed)
	rs := make([]ranked, 0, len(set))
	for s := range set {
		sum := sha256.Sum256(append(seedBytes[:], s...))
		rs = append(rs, ranked{id: s, key: hex.EncodeToString(sum[:])})
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].key != rs[j].key {
			return rs[i].key < rs[j].key
		}
		return rs[i].id < rs[j].id
	})
	n := int(math.Round(heldOutFraction * float64(len(rs))))
	n = max(1, min(n, len(rs)-1))
	var sp Split
	for i, r := range rs {
		if i < n {
			sp.HeldOut = append(sp.HeldOut, r.id)
		} else {
			sp.Train = append(sp.Train, r.id)
		}
	}
	sort.Strings(sp.Train)
	sort.Strings(sp.HeldOut)
	return sp, nil
}

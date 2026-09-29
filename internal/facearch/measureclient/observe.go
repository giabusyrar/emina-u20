package measureclient

// Observation states: whether a measurement can be used by a classifier
// or trait rule.
const (
	// StateUsable: the value is a float. Visibility is "observed" or
	// "unknown"; "unknown" values are computed, only their region could not
	// be judged (in v1 feature regions are always unknown).
	StateUsable = "usable"
	// StateOccluded: a region the measurement needs is occluded or out of
	// frame (or the eyes are, which withholds every measurement).
	StateOccluded = "occluded"
	// StateNotSupported: the catalogue lists the key but cannot measure it.
	StateNotSupported = "not_supported"
	// StateMissing: the key is not in the response at all (catalogue drift).
	StateMissing = "missing"
	// StateNotScalar: the value is a list; profiles reference scalars only.
	StateNotScalar = "not_scalar"
	// StateNoValue: visible but null (the worker's "degenerate geometry").
	StateNoValue = "no_value"
)

// Observation is one measurement as a classifier or trait sees it.
type Observation struct {
	Key   string
	State string
	Value float64
	// Band is the worker's noise band re-centred on Value
	// (Value + P5 - P50, Value + P95 - P50), so it contains Value. It is
	// conservative (independent noise at the model card's worst regional
	// error). Nil when the worker sent none (bands disabled, or too few
	// finite perturbed samples): no uncertainty information, not "certain".
	Band *[2]float64
}

// Usable reports whether the observation carries a float value.
func (o Observation) Usable() bool { return o.State == StateUsable }

// Index looks measurements up by key.
type Index struct {
	byKey map[string]Measurement
}

// Index builds an Index over the response's measurements.
func (r *Response) Index() Index {
	ix := Index{byKey: make(map[string]Measurement, len(r.Measurements))}
	for _, m := range r.Measurements {
		ix.byKey[m.Key] = m
	}
	return ix
}

// Observe classifies the measurement named key.
func (ix Index) Observe(key string) Observation {
	m, ok := ix.byKey[key]
	switch {
	case !ok:
		return Observation{Key: key, State: StateMissing}
	case m.Visibility == VisibilityNotSupported:
		return Observation{Key: key, State: StateNotSupported}
	case m.Visibility == VisibilityOccluded:
		return Observation{Key: key, State: StateOccluded}
	case m.Value.IsList():
		return Observation{Key: key, State: StateNotScalar}
	case m.Value.Scalar == nil:
		return Observation{Key: key, State: StateNoValue}
	}
	o := Observation{Key: key, State: StateUsable, Value: *m.Value.Scalar}
	if m.Band != nil && m.Band[0].Scalar != nil && m.Band[1].Scalar != nil {
		o.Band = &[2]float64{*m.Band[0].Scalar, *m.Band[1].Scalar}
	}
	return o
}

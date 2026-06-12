package monster

import "patel.codes/cgt/generator"

// This file holds the cgt-specific adapter that lets a
// monster element *MM act as a generator.GroupElem, so that
// *MM values can drive the provider-agnostic Orbit_Lin2
// engine in cgt/generator. The engine and the GroupElem
// interface live in the generator package; only this
// *MM-typed glue stays in flat cgt.

// MMGroupElem adapts a monster element *MM to the
// generator.GroupElem interface so that *MM values can be
// used as Orbit_Lin2 generators. *MM already has
// Mul(*MM) *MM and Pow(int) *MM; MMGroupElem wraps those
// into the interface-typed methods.
type MMGroupElem struct {
	M *MM
}

// Mul returns the product self * other; other must wrap an
// *MM (i.e. be an MMGroupElem). Mirrors MM.__mul__ at the
// interface level.
func (g MMGroupElem) Mul(other generator.GroupElem) generator.GroupElem {
	return MMGroupElem{M: g.M.Mul(other.(MMGroupElem).M)}
}

// Pow returns self raised to the power e. Mirrors MM.__pow__
// at the interface level.
func (g MMGroupElem) Pow(e int) generator.GroupElem {
	return MMGroupElem{M: g.M.Pow(e)}
}

// Mmdata exposes the underlying monster atom representation,
// letting MMGroupElem be used as a Leech2OrbitsRaw generator
// as well.
func (g MMGroupElem) Mmdata() []uint32 {
	return g.M.Mmdata()
}

package cgt

import (
	"fmt"
	"math/rand/v2"

	"patel.codes/cgt/generator"
	"patel.codes/cgt/leech"
	"patel.codes/cgt/mat24"
	"patel.codes/cgt/mmindex"
)

// The mm-coupled XLeech2 constructors: the ones that
// take or yield a monster element, parse a named
// q-element, or index the short-vector table. They
// build their result through the exported leech surface
// (leech.NewXLeech2FromInt) rather than reaching into
// the unexported XLeech2 state; the pure constructors,
// the XLeech2 type, and the lattice operations live in
// package leech.

// NewXLeech2RandomType returns a random element of
// Q_{x0} whose Leech-lattice-mod-2 image has the
// given type. It mirrors XLeech2('r', vtype).
//
// NewXLeech2RandomType panics if vtype is not 0, 2,
// 3 or 4, or if no element of type 3 or 4 is found
// after 1000 rejection samples.
func NewXLeech2RandomType(vtype int) *leech.XLeech2 {
	return leech.NewXLeech2FromInt(RandXleech2Type(vtype))
}

// RandXleech2Type returns the Leech-lattice encoding
// of a random element of Q_{x0} whose image in the
// Leech lattice mod 2 has the given type. It mirrors
// rand_xleech2_type.
//
// RandXleech2Type panics if vtype is not 0, 2, 3 or
// 4, or if no element of type 3 or 4 is found after
// 1000 rejection samples.
func RandXleech2Type(vtype int) uint32 {
	switch vtype {
	case 0:
		return 0
	case 2:
		ve := 300 + rand.IntN(98280) // randint(300, 98579)
		vs := mmindex.IndexExternToSparse(ve)
		sign := uint32(rand.IntN(2))
		return mmindex.IndexSparseToLeech2(vs) ^ (sign << 24)
	case 3, 4:
		for i := 0; i < 1000; i++ {
			v := uint32(rand.IntN(0x2000000)) // randint(0, 0x1ffffff)
			if int(generator.Leech2Type(v)) == vtype {
				return v
			}
		}
		panic("RandXleech2Type: no random type-3/4 element found")
	default:
		panic(fmt.Sprintf("RandXleech2Type: illegal type %d", vtype))
	}
}

// NewXLeech2FromShort returns the index-th positive
// short element of Q_{x0}. It mirrors
// XLeech2('short', index).
//
// NewXLeech2FromShort panics if index is not in the
// range 0 <= index < 98280.
func NewXLeech2FromShort(index int) *leech.XLeech2 {
	if index < 0 || index >= 98280 {
		panic(fmt.Sprintf("NewXLeech2FromShort: index %d out of range [0, 98280)", index))
	}
	vs := mmindex.IndexExternToSparse(index + 300)
	return leech.NewXLeech2FromInt(mmindex.IndexSparseToLeech2(vs))
}

// NewXLeech2FromMM extracts the Q_{x0} component of
// the monster element g. It mirrors XLeech2(MM).
//
// NewXLeech2FromMM panics if g is nil, or if g does
// not lie in the subgroup Q_{x0} of the monster.
func NewXLeech2FromMM(g *MM) *leech.XLeech2 {
	if g == nil {
		panic("NewXLeech2FromMM: nil MM")
	}
	return leech.NewXLeech2FromInt(MMToQX0(g))
}

// MMToQX0 returns the Leech-lattice encoding of the
// monster element g, which must lie in the subgroup
// Q_{x0}. It mirrors MM_to_Q_x0: it checks G_x0
// membership, reduces g, then folds the x (tag 3)
// and d (tag 1) atoms of the reduced word into a
// Leech-lattice value.
//
// MMToQX0 panics if g is not in the subgroup Q_{x0}
// of the monster.
func MMToQX0(g *MM) uint32 {
	// Operate on a copy so the caller's element is
	// neither mutated nor reduced as a side effect.
	h := &MM{data: append([]uint32(nil), g.data...)}
	if h.checkInGx0() == nil {
		panic("MMToQX0: monster element is not in subgroup Q_x0")
	}
	h.Reduce()
	var res uint32
	for _, atom := range h.data {
		tag := (atom >> 28) & 0x0f
		switch {
		case res == 0 && tag == 3:
			res = ((atom & 0x1fff) << 12) ^ mat24.PloopTheta(atom)
		case tag == 1:
			res ^= atom & 0xfff
		case tag != 0:
			panic("MMToQX0: monster element is not in subgroup Q_x0")
		}
	}
	return res
}

// NewXLeech2FromBasisVector returns the Q_{x0}
// element corresponding to a (possibly negated)
// basis vector of the representation rho, named by
// a single tag letter (one of B, C, T, X) and the
// indices i0, i1. It mirrors the BCTXE letter path
// of the XLeech2 constructor.
//
// NewXLeech2FromBasisVector panics if the tag is not
// a recognized short-vector tag or if the resulting
// basis vector does not correspond to an element of
// Q_{x0}.
func NewXLeech2FromBasisVector(tag byte, i0, i1 int) *leech.XLeech2 {
	t, ok := parseTagLetter(tag)
	if !ok {
		panic(fmt.Sprintf("NewXLeech2FromBasisVector: illegal tag %q", string(tag)))
	}
	a := tupleToSparse(0xff, Tuple{Factor: 1, Tag: t, I0: i0, I1: i1})
	if len(a) != 1 {
		panic(fmt.Sprintf("NewXLeech2FromBasisVector: tag %q does not yield a Q_x0 element", string(tag)))
	}
	a0 := a[0]
	d := mmindex.IndexSparseToLeech2(a0)
	switch a0 & 0xff {
	case 0xfe: // scalar -1: negate
		d ^= 0x1000000
	case 1: // scalar +1: keep
	default:
		d = 0
	}
	if d == 0 {
		panic(fmt.Sprintf("NewXLeech2FromBasisVector: tag %q does not yield a Q_x0 element", string(tag)))
	}
	return leech.NewXLeech2FromInt(d)
}

// NewXLeech2FromName returns a named Q_{x0} element.
// Recognized names include "v+", "v-", "Omega",
// "-Omega", "+", "-", "omega" and "-omega", as well
// as any value string accepted by the q-atom parser.
// It mirrors the std_q_element("q", name) path of
// the XLeech2 constructor.
//
// NewXLeech2FromName panics if name cannot be parsed
// as a Q_{x0} element.
func NewXLeech2FromName(name string) *leech.XLeech2 {
	v, err := qElement(name)
	if err != nil {
		panic(fmt.Sprintf("NewXLeech2FromName: cannot convert %q to XLeech2: %v", name, err))
	}
	return leech.NewXLeech2FromInt(v)
}

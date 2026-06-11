package xsp2co1

// Exported surface consumed by the flat cgt (mm)
// layer. These wrappers expose the G_x0 word/element
// primitives that the monster reduction and N_0
// bridging code in flat cgt drive directly on the
// 26-word G_x0 representation. The element buffer
// elem is the raw G_x0 representation: a length-26
// []uint64 (elem[0] the Leech-mod-3 vector, elem[1:26]
// the inverse of x_g). The mm layer owns these buffers
// for words it is in the middle of reducing.

// SetElemWordScan sets (mul=false) or multiplies
// (mul=true) elem with the longest G_{x0} prefix of
// the atom word a, returning the prefix length.
func SetElemWordScan(elem []uint64, a []uint32, mul bool) int {
	return xsp2co1SetElemWordScan(elem, a, mul)
}

// SetElemWord converts the atom word a to G_x0
// representation in elem. It returns a non-nil error
// if a leaves G_{x0}.
func SetElemWord(elem []uint64, a []uint32) error {
	return xsp2co1SetElemWord(elem, a)
}

// MulElemWord right-multiplies elem by the atom word
// a. It returns a non-nil error if a leaves G_{x0}.
func MulElemWord(elem []uint64, a []uint32) error {
	return xsp2co1MulElemWord(elem, a)
}

// ElemSubtype returns the subtype of elem (as
// gen_leech2_subtype) or -1 on error.
func ElemSubtype(elem []uint64) int32 {
	return xsp2co1ElemSubtype(elem)
}

// ElemToWord converts elem to a reduced word in the
// generators of G_{x0}, stored in a, returning its
// length (at most 10).
func ElemToWord(elem []uint64, a []uint32) int {
	return xsp2co1ElemToWord(elem, a)
}

// ElemToN0 converts elem to an element of N_0 in
// g[:5]. It returns a non-nil error if elem is not
// in N_0.
func ElemToN0(elem []uint64, g []uint32) error {
	return xsp2co1ElemToN0(elem, g)
}

// FromVectMod3 converts a vector in the (Z/3)^24
// encoding used by mm_op to the Leech-mod-3 encoding.
// It is the inverse of the internal vect-mod-3
// conversion (C xsp2co1_from_vect_mod3).
func FromVectMod3(x uint64) uint64 {
	return xsp2co1FromVectMod3(x)
}

// Short2ToLeech writes the integer coordinates of the
// short Leech-mod-2 vector x into pdest (length 24),
// normalized to norm 32; the sign is arbitrary. It
// panics if x is not a short Leech-mod-2 vector.
func Short2ToLeech(x uint32, pdest []int8) {
	short2ToLeech(x, pdest)
}

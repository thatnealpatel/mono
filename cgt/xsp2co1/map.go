package xsp2co1

import (
	"patel.codes/cgt/leech"
	"patel.codes/cgt/mat24"
)

// Provenance helpers for xsp2co1_elem_from_mapping
// (mmgroup/dev/clifford12/xsp2co1_map.ske). They
// recover an element of G_{x0} from its action on
// Q_{x0}, used by the Bimonster construction in
// flat cgt (bimm.go).

/*************************************************************************
*** xsp2co1_elem_from_mapping (xsp2co1_map.ske)
*************************************************************************/

// parity64Tab encodes the bit parity of x in
// (parity64Tab >> x) & 1 for 0 <= x < 64. This is
// the PARITY64 constant from xsp2co1_map.ske.
const parity64Tab = uint64(0x6996966996696996)

// xsp2co1Co1GetMapping computes the element g of
// G_{x0} that maps m1[j] to m2[j] (Leech lattice
// encoding) as the image mOut[j] of the standard
// basis. It returns the odd part o of the order of
// g, or a negative value on error.
//
// This is a port of xsp2co1_Co1_get_mapping.
func xsp2co1Co1GetMapping(m1, m2, mOut []uint32) int {
	var m [24]uint64
	// Store columns 0..23 of m1[i] in columns 24..47,
	// columns 0..23 of m2[i] in columns 0..23, and the
	// XOR of column 24 of m1[i], m2[i] in column 48.
	for row := 0; row < 24; row++ {
		m[row] = ((uint64(m2[row]) & 0x1ffffff) << 24) ^
			(uint64(m1[row]) & 0xffffff) ^
			((uint64(m1[row]) & 0x1000000) << 24)
	}

	// Check that scalar products and types (mod 2)
	// agree between m1 and m2; abort otherwise.
	var acc uint64
	for k1 := 0; k1 < 24; k1++ {
		v := m[k1]
		sign := v & (v >> 12)
		sign ^= sign >> 24
		sign ^= sign >> 6
		acc |= parity64Tab >> (sign & 0x3f)
		for k2 := k1 + 1; k2 < 24; k2++ {
			w := m[k2]
			sign = (v & (w >> 12)) ^ (w & (v >> 12))
			sign ^= sign >> 24
			sign ^= sign >> 6
			acc |= parity64Tab >> (sign & 0x3f)
		}
	}
	if acc&1 != 0 {
		return -2
	}

	// Echelonize columns 0..23, adding extraspecial
	// elements (carrying signs in columns 48..59).
	var k1 int
	for col := 0; col < 24; col++ {
		colMask := uint64(1) << uint(col)
		k1 = 23
		for ; k1 >= col; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					if m[k2]&colMask != 0 {
						sign := v & (m[k2] >> 12)
						sign ^= sign >> 24
						m[k2] ^= v ^ (sign << 48)
					}
				}
				m[k1] = m[col]
				m[col] = v
				break
			}
		}
		if k1 < col {
			return -1
		}
	}

	// Copy the image (with sign) of each basis vector.
	for row := 0; row < 24; row++ {
		sign := ((m[row] >> 48) ^ (m[row] >> 54)) & 0x3f
		sign = (parity64Tab >> sign) & 1
		m[row] = ((m[row] >> 24) & 0xffffff) | (sign << 24)
		mOut[row] = uint32(m[row])
	}

	var bm [24]uint64
	copy(bm[:], m[:])
	return xsp2co1OddOrderBitmatrix(bm[:])
}

// xsp2co1Co1MatrixToWord computes a word g of
// generators of G_{x0} that acts on Q_{x0} as the
// automorphism described by the 24x25 bit matrix m
// (row m[i] is the image of the i-th basis vector,
// Leech lattice encoding). It returns the word
// length, or a negative value on failure. The
// buffer g must have length at least 10.
//
// This is a port of xsp2co1_Co1_matrix_to_word.
func xsp2co1Co1MatrixToWord(m, g []uint32) int {
	var a [24]uint32
	// g' maps the image of Omega (m[23]) to +-Omega.
	res := leech.GenLeech2ReduceType4(m[23], g)
	if res < 0 {
		return res
	}
	length := res
	copy(a[:], m[:24])
	if leech.GenLeech2OpWordMany(a[:], g[:length]) != length {
		return -100001
	}

	// Compute a permutation in M24 from the bit matrix
	// a[12:23, 12:23].
	var aPi [12]uint32
	for i := 0; i < 11; i++ {
		aPi[i] = (a[i+12] >> 12) & 0x7ff
	}
	aPi[11] = 0
	mat24.MatrixFromModOmega(aPi[:])
	perm := mat24.MatrixToPerm(aPi[:])
	if mat24.PermCheck(perm) != nil {
		return -100002
	}
	pi := mat24.PermToM24num(perm)
	if pi != 0 {
		g[length] = atomTagIP + pi
		if leech.GenLeech2OpWordMany(a[:], g[length:length+1]) != 1 {
			return -100003
		}
		length++
	}

	// Compute a y_d generator from the image a[11] of
	// an odd Golay cocode vector.
	y := (a[11] >> 12) & 0x7ff
	if y != 0 {
		g[length] = atomTagIY + y
		if leech.GenLeech2OpWordMany(a[:], g[length:length+1]) != 1 {
			return -100004
		}
		length++
	}

	// a[i] must equal the i-th basis vector up to sign.
	var accU, q uint32
	for i := 0; i < 24; i++ {
		accU |= a[i] ^ (uint32(1) << uint(i))
		q ^= ((a[i] >> 24) & 1) << uint(i)
	}
	if accU&0xffffff != 0 {
		return -100004
	}

	// Convert the residual x' to generators.
	x := q & 0xfff
	q = (q >> 12) & 0xfff
	q ^= mat24.PloopTheta(x&0x7ff) & 0x7ff
	if q != 0 {
		g[length] = atomTagID + q
		length++
	}
	g[length] = atomTagIX + x
	length++

	invertWord(g[:length])
	return length
}

// chi244096 returns the character of the
// representation rho_24 (x) rho_4096 of the element
// elem. The second result is false on failure
// (matching the CHI_24_4096_BAD path in C).
func chi244096(elem []uint64) (int32, bool) {
	var atrace [4]int32
	if !tracesSmallOK(elem, atrace[:]) {
		return 0, false
	}
	return atrace[0] * atrace[2], true
}

// ElemFromMapping computes g in G_{x0} from its
// action m1[j] -> m2[j] on Q_{x0} (Leech lattice
// encoding). On success it stores g as a word of
// generators in the buffer g (length >= 10) and
// returns (length | order<<8 | notZero<<16). It
// returns a negative value on failure.
//
// g is determined up to sign; the function picks the
// representative of odd order if one exists, else the
// one with non-negative character chi(g^o).
//
// This is a port of xsp2co1_elem_from_mapping.
func ElemFromMapping(m1, m2, g []uint32) int {
	var m [24]uint32
	o := xsp2co1Co1GetMapping(m1, m2, m[:])
	if o < 0 {
		return o
	}
	length := xsp2co1Co1MatrixToWord(m[:], g)
	if length < 0 {
		return length
	}
	var elem [26]uint64
	if err := xsp2co1SetElemWord(elem[:], g[:length]); err != nil {
		return -100006
	}
	xsp2co1PowerElem(elem[:], int64(o), elem[:])
	chi, ok := chi244096(elem[:])
	if !ok {
		return -100007
	}
	if chi < 0 {
		g[0] ^= 0x1000
		xsp2co1NegElem(elem[:])
	}
	oo := o
	i := 0
	for ; i < 7; i++ {
		if xsp2co1IsUnitElem(elem[:]) {
			break
		}
		xsp2co1MulElem(elem[:], elem[:], elem[:])
		oo <<= 1
	}
	if i == 7 {
		return -100008
	}
	notZero := 0
	if chi != 0 {
		notZero = 1
	}
	return (notZero << 16) + (oo << 8) + length
}

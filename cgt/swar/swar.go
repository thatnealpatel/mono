// Package swar provides bit-matrix primitives (the bm64
// family from bitmatrix64.c) operating on []uint64 row
// vectors: rotation, transposition, multiplication,
// echelonization, and inversion.
package swar

import "math/bits"

/*************************************************************************
*** Permuting bit arguments (bitmatrix64.c)
*************************************************************************/

// Bm64RotBits rotates columns n0..n0+nrot-1 of
// the i-row bit matrix m by rot.
//
// Bm64RotBits panics if nrot+n0 exceeds 64.
func Bm64RotBits(m []uint64, i, rot, nrot, n0 int) {
	if nrot+n0 > 64 {
		panic("swar: qubit index out of range")
	}
	if nrot < 2 {
		return
	}
	if rot < 0 {
		rot += nrot*(-rot/nrot) + nrot
	}
	rot %= nrot
	if rot == 0 {
		return
	}
	nmax := nrot + n0
	maskL := uint64(1) << uint(nmax-rot)
	var maskH uint64
	if nmax < 64 {
		maskH = uint64(1) << uint(nmax)
	}
	maskH -= maskL
	maskL -= uint64(1) << uint(n0)
	mask := ^(maskL | maskH)
	sh := uint(nrot - rot)
	for k := 0; k < i; k++ {
		m[k] = (m[k] & mask) | ((m[k] & maskL) << uint(rot)) | ((m[k] & maskH) >> sh)
	}
}

// Bm64XchBits exchanges column j with column
// j+sh of the i-row bit matrix m when bit j of
// mask is set.
//
// Bm64XchBits panics if sh >= 64 or if mask and
// mask>>sh overlap.
func Bm64XchBits(m []uint64, i, sh int, mask uint64) {
	if mask == 0 {
		return
	}
	if sh >= 64 || mask&(mask>>uint(sh)) != 0 {
		panic("swar: qubit index out of range")
	}
	for k := 0; k < i; k++ {
		v := (m[k] ^ (m[k] >> uint(sh))) & mask
		m[k] ^= v ^ (v << uint(sh))
	}
}

// Bm64ReverseBits reverses n columns of the
// i-row bit matrix m starting at column n0.
//
// Bm64ReverseBits panics if n+n0 exceeds 64.
func Bm64ReverseBits(m []uint64, i, n, n0 int) {
	if n+n0 > 64 {
		panic("swar: qubit index out of range")
	}
	if n < 2 {
		return
	}
	for k := 0; k < i; k++ {
		mask := uint64(1) << uint(n0)
		v := m[k]
		for sh := n - 1; sh > 0; sh -= 2 {
			w := (v ^ (v >> uint(sh))) & mask
			v ^= w ^ (w << uint(sh))
			mask <<= 1
		}
		m[k] = v
	}
}

// Bm64T writes the transpose of the i x j bit
// matrix m1 into m2.
func Bm64T(m1 []uint64, i, j int, m2 []uint64) {
	for j1 := 0; j1 < j; j1++ {
		var v uint64
		for i1 := 0; i1 < i; i1++ {
			v |= ((m1[i1] >> uint(j1)) & 1) << uint(i1)
		}
		m2[j1] = v
	}
}

// Bm64Mul writes the product m1*m2 into m3.
// m1 has i1 rows, only the lowest i2 columns of
// m1 are inspected. m3 may alias m1.
func Bm64Mul(m1, m2 []uint64, i1, i2 int, m3 []uint64) {
	if i2 > 64 {
		i2 = 64
	}
	for i := 0; i < i1; i++ {
		mi := m1[i]
		var mo uint64
		for j := 0; j < i2; j++ {
			mo ^= -((mi >> uint(j)) & 1) & m2[j]
		}
		m3[i] = mo
	}
}

// Bm64MaskRows ands mask into the first i rows.
func Bm64MaskRows(m []uint64, i int, mask uint64) {
	for k := 0; k < i; k++ {
		m[k] &= mask
	}
}

// Bm64AddDiag adds one to entries m[k, j+k].
func Bm64AddDiag(m []uint64, i, j int) {
	if j >= 64 {
		return
	}
	mask := uint64(1) << uint(j)
	for k := 0; k < i; k++ {
		m[k] ^= mask
		mask <<= 1
	}
}

// Bm64EchelonH converts m to reduced echelon
// form with leading bits the most significant,
// pivoting columns j0-1..j0-n. It returns the
// number of nonzero pivoted rows.
func Bm64EchelonH(m []uint64, i, j0, n int) int {
	rowPos := 0
	if j0 > 64 {
		j0 = 64
	}
	if n > j0 {
		n = j0
	}
	if i == 0 || n == 0 {
		return 0
	}
	for col := j0 - 1; col >= j0-n; col-- {
		colMask := uint64(1) << uint(col)
		for k1 := i - 1; k1 >= rowPos; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					m[k2] ^= -((m[k2] >> uint(col)) & 1) & v
				}
				m[k1] = m[rowPos]
				m[rowPos] = v
				rowPos++
				break
			}
		}
	}
	return rowPos
}

// Bm64EchelonL converts m to reduced echelon
// form with leading bits the least significant,
// pivoting columns j0..j0+n-1. It returns the
// number of nonzero pivoted rows.
func Bm64EchelonL(m []uint64, i, j0, n int) int {
	rowPos := 0
	if j0 >= 64 || i == 0 || n == 0 {
		return 0
	}
	if j0+n > 64 {
		n = 64 - j0
	}
	for col := j0; col < j0+n; col++ {
		colMask := uint64(1) << uint(col)
		for k1 := i - 1; k1 >= rowPos; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					m[k2] ^= -((m[k2] >> uint(col)) & 1) & v
				}
				m[k1] = m[rowPos]
				m[rowPos] = v
				rowPos++
				break
			}
		}
	}
	return rowPos
}

// Bm64Inv inverts the i x i bit matrix m in
// place. It returns false if m is singular.
//
// Bm64Inv panics if i exceeds 32.
func Bm64Inv(m []uint64, i int) bool {
	if i > 32 {
		panic("swar: bit matrix too large to invert")
	}
	if i == 0 {
		return true
	}
	Bm64MaskRows(m, i, (uint64(1)<<uint(i))-1)
	Bm64AddDiag(m, i, i)
	Bm64EchelonL(m, i, 0, 2*i)
	if m[i-1]&((uint64(1)<<uint(i))-1) == 0 {
		return false
	}
	Bm64RotBits(m, i, i, 2*i, 0)
	Bm64MaskRows(m, i, (uint64(1)<<uint(i))-1)
	return true
}

// Bm64FindLowBit returns the lowest position k
// (imin <= k < imax) of a set bit in m, or imax.
func Bm64FindLowBit(m []uint64, imin, imax int) int {
	if imin >= imax {
		return imax
	}
	n := imin >> 6
	nmax := (imax + 63) >> 6
	v := m[n] & -(uint64(1) << uint(imin&63))
	if v != 0 {
		return (n << 6) + bits.TrailingZeros64(v)
	}
	// C uses n <= nmax, which over-reads one word past the
	// allocation when imax is a 64-multiple. The trailing word
	// never contains in-range bits (res < imax discards them).
	for n++; n < nmax; n++ {
		if v = m[n]; v != 0 {
			res := (n << 6) + bits.TrailingZeros64(v)
			if res < imax {
				return res
			}
			return imax
		}
	}
	return imax
}

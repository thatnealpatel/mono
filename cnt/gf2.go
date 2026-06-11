// Package cnt implements a lazy
// computational number theory
// library for fun.
package cnt

import "math/bits"

// EchelonH reduces the bit matrix m
// to RREF, pivoting columns j0-1,
// j0-2, ..., j0-n (from high end).
// Row ops span all 64 columns.
func EchelonH(m []uint64, j0, n int) int {
	i := len(m)
	if j0 > 64 {
		j0 = 64
	}
	if n > j0 {
		n = j0
	}
	if i == 0 || n == 0 {
		return 0
	}
	rowPos := 0
	for col := j0 - 1; col >= j0-n; col-- {
		colMask := uint64(1) << uint(col)
		for k1 := i - 1; k1 >= rowPos; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					m[k2] ^= (0 - ((m[k2] >> uint(col)) & 1)) & v
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

// EchelonL reduces the bit matrix m
// to RREF, pivoting columns j0,
// j0+1, ..., j0+n-1 (from low end).
func EchelonL(m []uint64, j0, n int) int {
	i := len(m)
	if j0 >= 64 || i == 0 || n == 0 {
		return 0
	}
	if j0+n > 64 {
		n = 64 - j0
	}
	rowPos := 0
	for col := j0; col < j0+n; col++ {
		colMask := uint64(1) << uint(col)
		for k1 := i - 1; k1 >= rowPos; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					m[k2] ^= (0 - ((m[k2] >> uint(col)) & 1)) & v
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

// Rank returns the rank of m in
// columns [j0-n, j0). Does not
// modify m.
func Rank(m []uint64, j0, n int) int {
	cp := append([]uint64(nil), m...)
	return EchelonH(cp, j0, n)
}

// Solve solves M*x = v over GF(2).
// m has i rows, j columns; column j
// is the RHS vector v. Echelonizes
// in place.
func Solve(m []uint64, i, j int) (uint64, bool) {
	if j > 63 {
		return 0, false
	}
	mask := ((uint64(1) << uint(j)) << 1) - 1
	n := EchelonL(m[:i], 0, j+1)
	if n == 0 {
		return 0, true
	}
	if m[n-1]&mask == uint64(1)<<uint(j) {
		return 0, false
	}
	var res uint64
	for k := range n {
		if (m[k]>>uint(j))&1 != 0 {
			res |= m[k] & (0 - m[k])
		}
	}
	return res, true
}

// Transpose writes the transpose of the
// i×j bit matrix src into dst. Bit
// src[r,c] is (src[r]>>c)&1, so dst is
// the j×i matrix with dst[c,r] = src[r,c].
//
// Transpose reads src[0:i] and writes
// dst[0:j].
func Transpose(dst, src []uint64, i, j int) {
	for col := range j {
		var v uint64
		for row := range i {
			v |= ((src[row] >> uint(col)) & 1) << uint(row)
		}
		dst[col] = v
	}
}

// MatMul multiplies GF(2) bit matrices
// a and b into dst. a is len(a)×len(b);
// b is len(b)×64. dst receives the
// len(a)×64 product.
//
// MatMul panics if len(b) > 64.
func MatMul(dst, a, b []uint64) {
	n := len(b)
	if n > 64 {
		panic("matmul: len(b) exceeds 64")
	}
	for i := range a {
		mi := a[i]
		var mo uint64
		for j := range n {
			mo ^= (0 - ((mi >> uint(j)) & 1)) & b[j]
		}
		dst[i] = mo
	}
}

// MatInv inverts the n×n GF(2) bit
// matrix m in place.
func MatInv(m []uint64, n int) bool {
	if n > 32 {
		return false
	}
	if n == 0 {
		return true
	}
	work := make([]uint64, n)
	lowMask := (uint64(1) << uint(n)) - 1
	// Keep only the low n columns, then add the identity in columns [n, 2n).
	for k := range n {
		work[k] = m[k] & lowMask
		work[k] ^= uint64(1) << uint(n+k)
	}
	// Echelonize the augmented matrix from the low end over all 2n columns.
	// EchelonL returns the (always non-negative) pivot-row count; it is only
	// called here for its in-place echelonization side effect.
	EchelonL(work, 0, 2*n)
	// The last pivot row must still have a nonzero entry in the low n columns,
	// otherwise the left block did not reduce to the identity (singular).
	if work[n-1]&lowMask == 0 {
		return false
	}
	// Rotate columns: map column c to (c+n) mod 2n, swapping the low and high
	// blocks so the inverse moves into the low n columns.
	if !rotBits(work, n, 2*n, 0) {
		return false
	}
	for k := range n {
		m[k] = work[k] & lowMask
	}
	return true
}

// rotBits rotates columns [n0, n0+nrot)
// of every row left by rot.
func rotBits(m []uint64, rot, nrot, n0 int) bool {
	if nrot+n0 > 64 {
		return false
	}
	if nrot < 2 {
		return true
	}
	rot %= nrot
	if rot < 0 {
		rot += nrot
	}
	if rot == 0 {
		return true
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
	sh := nrot - rot
	for k := range m {
		m[k] = (m[k] & mask) | ((m[k] & maskL) << uint(rot)) | ((m[k] & maskH) >> uint(sh))
	}
	return true
}

// CapH echelonizes a and b over
// columns [j0-n, j0), computes the
// intersection of their row spaces,
// and returns rank(a) - dim(cap).
//
// a and b are modified in place.
func CapH(a, b []uint64, j0, n int) int {
	rows1 := EchelonH(a, j0, n)
	rows2 := EchelonH(b, j0, n)
	_ = rows2
	return rows1 - capH(a, b, len(a), len(b), j0, n)
}

// capH computes row-space intersection
// of echelonized m1 and m2 over
// columns [j0-n, j0). Returns n_cap.
func capH(m1, m2 []uint64, i1, i2, j0, n int) int {
	var (
		m1a, m2a                      [64]uint64
		rowsUsed1, rowsUsed2, rowsEqu uint64
		v1, v2                        uint64
	)

	if j0 > 64 {
		j0 = 64
	}
	if n > j0 {
		n = j0
	}
	if n == 0 {
		return 0
	}

	// Mask of the relevant columns [j0-n, j0).
	mask := (((uint64(1) << 1) << uint(n-1)) - 1) << uint(j0-n)
	for i1 > 0 && m1[i1-1]&mask == 0 {
		i1--
	}
	for i2 > 0 && m2[i2-1]&mask == 0 {
		i2--
	}
	if i1 == 0 || i2 == 0 {
		return 0
	}

	rowPos1, rowPos2, rowPos := 0, 0, 0
	for col := j0 - 1; col >= j0-n; col-- {
		var b1, b2 uint64
		if rowPos1 < i1 {
			b1 = (m1[rowPos1] >> uint(col)) & 1
		}
		if rowPos2 < i2 {
			b2 = (m2[rowPos2] >> uint(col)) & 1
		}
		rowsUsed1 |= b1 << uint(rowPos)
		rowsUsed2 |= b2 << uint(rowPos)

		if b1 != 0 {
			if b2 != 0 {
				m1a[rowPos] = m1[rowPos1]
				rowPos1++
				m2a[rowPos] = m2[rowPos2]
				rowPos2++
				rowsEqu |= uint64(1) << uint(rowPos)
				rowPos++
			} else {
				v1 = m1[rowPos1]
				rowPos1++
				m1a[rowPos] = v1
				for row := rowPos - 1; row >= 0; row-- {
					m1a[row] ^= v1 & (0 - ((rowsEqu >> uint(row)) & (m2a[row] >> uint(col)) & 1))
				}
				rowPos++
			}
		} else {
			if b2 != 0 {
				v2 = m2[rowPos2]
				rowPos2++
				m2a[rowPos] = v2
				for row := rowPos - 1; row >= 0; row-- {
					m2a[row] ^= v2 & (0 - ((rowsEqu >> uint(row)) & (m1a[row] >> uint(col)) & 1))
				}
				rowPos++
			} else {
				row := rowPos - 1
				for ; row >= 0; row-- {
					if ((m1a[row]^m2a[row])>>uint(col))&(rowsEqu>>uint(row))&1 != 0 {
						rowsEqu &^= uint64(1) << uint(row)
						v1 = m1a[row]
						v2 = m2a[row]
						break
					}
				}
				for row--; row >= 0; row-- {
					if ((m1a[row]^m2a[row])>>uint(col))&(rowsEqu>>uint(row))&1 != 0 {
						m1a[row] ^= v1
						m2a[row] ^= v2
					}
				}
			}
		}
	}

	restoreCapH(m1a[:], m1, rowsUsed1, rowsEqu, i1)
	return restoreCapH(m2a[:], m2, rowsUsed2, rowsEqu, i2)
}

// restoreCapH copies rows of ma marked
// in rowsUsed back into m, placing
// intersection rows (rowsCap) last.
//
// restoreCapH panics if m has fewer
// rows than the used-row count.
func restoreCapH(ma, m []uint64, rowsUsed, rowsCap uint64, maxRows int) int {
	iMax := bits.Len64(rowsUsed)
	nUsed := bits.OnesCount64(rowsUsed)
	rowsCap &= rowsUsed
	nCap := bits.OnesCount64(rowsCap)
	rowH := nUsed - nCap
	if maxRows < nUsed {
		panic("cnt: restoreCapH: destination too small")
	}
	row := 0
	for i := range iMax {
		if (rowsUsed>>uint(i))&1 != 0 {
			if (rowsCap>>uint(i))&1 != 0 {
				m[rowH] = ma[i]
				rowH++
			} else {
				m[row] = ma[i]
				row++
			}
		}
	}
	return nCap
}

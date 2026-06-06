package cgt

// This file ports the triality operation tau^e on
// the tag-A part (mm_op*_t_A), plus the mod-15
// eval_A helpers. The full mm_op*_t (operating on
// all tags via Hadamard butterflies) is left for a
// later pass; only the A-part variant is exercised
// by EvalA/AxisType.

// inv2 returns the inverse of 2 modulo p (p odd).
func inv2(p int) int { return (p + 1) / 2 }

// OpTA computes the tag-A part of v * tau^e and
// stores it in vOut (modulo p). The non-A entries of
// vOut are not changed. C mm_op*_t_A.
func OpTA(p int, src []uint64, e int, dst []uint64) {
	per := rowsPer24(p) // words per 24-entry row
	// exp = 0 leaves A unchanged (identity on A).
	if (e-1)&2 != 0 {
		// e == 0 or e >= 3: copy A part (24 rows).
		n := 24 * per
		copy(dst[:n], src[:n])
		return
	}
	neg := e == 2 // exp1 == -1 in C
	half := inv2(p)
	for i := 0; i < 24; i++ {
		for j := 0; j < 24; j++ {
			var val int
			if i == j {
				val = aEntry(p, src, i, j) // preserve diagonal
			} else {
				b := bEntry(p, src, i, j)
				c := cEntry(p, src, i, j)
				if neg {
					val = ((b-c)%p + p) % p
				} else {
					val = (b + c) % p
				}
				val = (val * half) % p
			}
			putAEntry(p, dst, i, j, val)
		}
	}
}

// bEntry returns B[row][col] (tag B, internal offset
// 768) of vector v modulo p.
func bEntry(p int, v []uint64, row, col int) int {
	return int(getMMV(p, v, uint32(mmAuxOfsB+row*32+col)))
}

// cEntry returns C[row][col] (tag C, internal offset
// 1536) of vector v modulo p.
func cEntry(p int, v []uint64, row, col int) int {
	return int(getMMV(p, v, uint32(mmAuxOfsC+row*32+col)))
}

// putAEntry sets A[row][col] of vector v modulo p,
// updating the symmetric twin.
func putAEntry(p int, v []uint64, row, col, val int) {
	putMMV(p, uint8(val%p), v, uint32(row*32+col))
}

// evalA15 returns v2 * A * v2^T modulo 15 for the
// tag-A matrix of v and short Leech-mod-2 vector v2.
// C mm_op15_eval_A. A faithful port requires the
// mod-15 bit-spread auxiliary (mm_op15_eval_A_aux),
// which is not reproduced here.
//
// evalA15 panics: it is not implemented.
func evalA15(v []uint64, v2 uint32) int {
	panic("cgt: EvalA not implemented")
}

// leech3matrixRank is the rank routine from
// leech3matrix.c (Gaussian elimination over GF(3)
// on the packed mod-3 matrix encoding). It is not
// reproduced here.
//
// leech3matrixRank panics: it is not implemented.
func leech3matrixRank(a []uint64, d uint32) int64 {
	panic("cgt: leech3matrixRank not implemented")
}

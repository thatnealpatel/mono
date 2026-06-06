package cgt

// This file ports the evaluation helpers for the
// tag-A matrix and the 98280_x part: norm_A,
// eval_A, eval_X_count_abs, load_leech3matrix and
// eval_A_rank_mod3.

// aEntry returns A[row][col] of the tag-A matrix of
// vector v modulo p (internal offset 0).
func aEntry(p int, v []uint64, row, col int) int {
	return int(getMMV(p, v, uint32(row*32+col)))
}

// OpNormA returns the norm of the tag-A matrix of v
// modulo p: the sum of squares of the 24*24 entries.
// C mm_op*_norm_A.
func OpNormA(p int, v []uint64) int {
	norm := 0
	for i := 0; i < 24; i++ {
		for j := 0; j < 24; j++ {
			e := aEntry(p, v, i, j)
			norm += e * e
		}
	}
	return norm % p
}

// rowsPer24 returns the number of uint64 words that
// hold one 24-entry row for modulus p.
func rowsPer24(p int) int { return 32 >> logIntFields(p) }

// CountShort returns a histogram of absolute values
// of the monomial part (tags B, C, T, X) of the
// vector. Entry i counts entries with absolute value
// i for 0 <= i <= (p-1)/2. C mm_op_eval_X_count_abs.
//
// CountShort panics if p != 15.
func (v *MMVector) CountShort() []int {
	if v.p != 15 {
		panic("cgt: CountShort supported for p = 15 only")
	}
	p := 15
	fw := fieldWidth(p)
	nf := uint(64) / fw
	var a [16]int

	// Tags B, C: 48 rows of 24 entries.
	countShort24(p, v.data, ofsWords(p, mmAuxOfsB), 48, fw, nf, a[:])
	a[0] = (a[0] + a[15] - 48) >> 1
	for i := 1; i < 15; i++ {
		a[i] >>= 1
	}
	a[15] = 0
	// Tag T: 759 rows of 64 entries (4 words each).
	countShortRows(p, v.data, ofsWords(p, mmAuxOfsT), 759*4, fw, nf, a[:])
	// Tag X: 2048 rows of 24 entries.
	countShort24(p, v.data, ofsWords(p, mmAuxOfsX), 2048, fw, nf, a[:])

	out := make([]int, 8)
	for i := 0; i < 8; i++ {
		out[i] = a[i] + a[15-i]
	}
	return out
}

// countShort24 histograms nRows rows of 24 entries
// (each spanning rowsPer24 words) into hist.
func countShort24(p int, v []uint64, base, nRows int, fw, nf uint, hist []int) {
	per := rowsPer24(p)
	for r := 0; r < nRows; r++ {
		off := base + r*per
		got := 0
		for w := 0; w < per && got < 24; w++ {
			word := v[off+w]
			for j := uint(0); j < nf && got < 24; j++ {
				hist[(word>>(j*fw))&15]++
				got++
			}
		}
	}
}

// countShortRows histograms nWords consecutive words
// (all fields) into hist.
func countShortRows(p int, v []uint64, base, nWords int, fw, nf uint, hist []int) {
	for w := 0; w < nWords; w++ {
		word := v[base+w]
		for j := uint(0); j < nf; j++ {
			hist[(word>>(j*fw))&15]++
		}
	}
}

// OpLoadLeech3Matrix loads the tag-A part of v
// (modulo 3 or 15) into a as a Leech-mod-3 matrix
// (24 rows, 3 words each). C mm_op*_load_leech3matrix.
//
// OpLoadLeech3Matrix panics if p is not 3 or 15.
func OpLoadLeech3Matrix(p int, v []uint64, a []uint64) {
	switch p {
	case 3:
		ai := 0
		vi := 0
		for i := 0; i < 24; i++ {
			a[ai] = v[vi] & 0xffffffff
			a[ai+1] = (v[vi] >> 32) & 0xffff
			vi++
			a[ai] = expand315(a[ai])
			a[ai+1] = expand315(a[ai+1])
			a[ai+2] = 0
			ai += 3
		}
	case 15:
		ai := 0
		per := rowsPer24(15)
		for i := 0; i < 24; i++ {
			off := i * per
			a[ai] = v[off]
			a[ai+1] = v[off+1] & 0xffffffff
			a[ai+2] = 0
			ai += 3
		}
	default:
		panic("cgt: OpLoadLeech3Matrix supported for p = 3, 15 only")
	}
}

// expand315 expands the lower 16 two-bit fields of a
// to four-bit fields. C macro EXPAND_3_15.
func expand315(a uint64) uint64 {
	a = (a & 0xffff) + ((a & 0xffff0000) << 16)
	a = (a & 0xff000000ff) + ((a & 0xff000000ff00) << 8)
	a = (a & 0xf000f000f000f) + ((a & 0xf000f000f000f0) << 4)
	a = (a & 0x303030303030303) + ((a & 0xc0c0c0c0c0c0c0c) << 2)
	return a
}

// OpEvalARankMod3 returns (rank << 48) + w for the
// matrix A - d*I, where A is the tag-A part of v and
// w is a kernel vector (Leech mod 3 encoding) when
// the corank is 1. C mm_op*_eval_A_rank_mod3.
//
// OpEvalARankMod3 panics if p is not 3 or 15.
func OpEvalARankMod3(p int, v []uint64) int {
	// The C function signature takes d; the Go stub
	// omits it, so we evaluate at d = 0.
	if p != 3 && p != 15 {
		panic("cgt: OpEvalARankMod3 supported for p = 3, 15 only")
	}
	a := make([]uint64, 24*3)
	OpLoadLeech3Matrix(p, v, a)
	return int(leech3matrixRank(a, 0))
}

// EvalA returns v2 * A * v2^T modulo p, where A is
// the tag-A matrix of v and v2 is a short Leech
// lattice vector (encoded mod 2). C mm_op15_eval_A.
//
// EvalA panics if p != 15; it returns -1 if v2 is
// not of type 2.
func (v *MMVector) EvalA(v2 uint64, e int) int {
	if v.p != 15 {
		panic("cgt: EvalA supported for p = 15 only")
	}
	// Apply the triality A-part first (mm_op_t_A).
	work := make([]uint64, len(v.data))
	OpTA(v.p, v.data, ((e%3)+3)%3, work)
	return evalA15(work, uint32(v2))
}

// AxisType returns a short string describing the 2A
// axis type of v (after applying tau^e). The full
// disambiguation depends on the reduction engine in
// mm_reduce.c, which is out of scope here.
//
// AxisType panics: it is not implemented.
func (v *MMVector) AxisType(e int) string {
	if v.p != 15 {
		panic("cgt: AxisType supported for p = 15 only")
	}
	panic("cgt: AxisType not implemented (requires mm_reduce engine)")
}

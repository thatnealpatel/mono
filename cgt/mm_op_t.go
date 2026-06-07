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
// tag-A matrix of v and short Leech-mod-2 vector v2,
// or -1 if v2 is not of type 2. C mm_op15_eval_A.
func evalA15(v []uint64, v2 uint32) int {
	var res uint64
	switch Leech2Type2(v2) {
	case 0x20:
		// Compute cocode entries of v2.
		syn := uint32(mat24SyndromeTable[(v2^mat24RecipBasis[23])&0x7ff])
		syn &= 0x3ff
		// Bits 9..5 and 4..0 hold the high and low
		// cocode bit index. Change a high index 24
		// to 23.
		syn -= ((syn + 0x100) & 0x400) >> 5
		i, j := syn&0x1f, syn>>5
		res = uint64(entryV(v, i, j))
		res ^= uint64((((v2 >> 23) & 1) - 1) & 15)
		res += res
		res += uint64(entryV(v, i, i)) + uint64(entryV(v, j, j))
		res <<= 4
	case 0x21:
		v2 &= 0x7fffff
		theta := uint32(mat24ThetaTable[v2>>12])
		vect := GcodeToVect(v2 >> 12)
		i := uint32(mat24SyndromeTable[(v2^theta)&0x7ff]) & 0x1f
		vect ^= 0 - ((vect >> i) & 1)
		r := evalA15Aux(v, 0xffffff, vect, i)
		resRow := r >> 16
		res = uint64(r & 0xffff)
		res += 7 * uint64(resRow)
		res += uint64(entryV(v, i, i))
	case 0x22:
		v2 &= 0x7fffff
		theta := uint32(mat24ThetaTable[v2>>12])
		vect := GcodeToVect(v2 >> 12)
		vect ^= ((theta >> 13) & 1) - 1
		coc := (v2 ^ theta) & 0x7ff
		lsb := lsbit24(vect)
		coc ^= mat24RecipBasis[lsb]
		syn := uint32(mat24SyndromeTable[coc&0x7ff])
		cocodev := synFromTable(syn) ^ (1 << lsb)
		res = 4 * uint64(evalA15Aux(v, vect, cocodev, 24))
	default:
		return -1
	}
	return int(res % 15)
}

// entryV reads the raw 4-bit field at A[i][j] of a
// mod-15 vector (internal index i*32+j), without
// reducing 15 to 0. C entry_v in mm15_op_eval_A.c.
func entryV(v []uint64, i, j uint32) uint32 {
	i = (i << 5) + j
	w := v[i>>4]
	w >>= (i & 0xf) << 2
	return uint32(w & 15)
}

// evalA15Aux evaluates y * A * y^T modulo 15 (not
// reduced) for the tag-A matrix A of v, where bit i
// of y is mAnd[i] * (-1)**mXor[i]. If row >= 24 the
// result res (with 0 < res < 0x8000) is returned. If
// row < 24, let z agree with y on coordinate row and
// be 0 elsewhere; with zz = z * A * y^T the function
// returns 0x10000*zz + res. C mm_op15_eval_A_aux.
func evalA15Aux(v []uint64, mAnd, mXor, row uint32) int {
	xorMask0 := spread16To15(mXor)
	andMask0 := spread16To15(mAnd)
	xorMask1 := spread16To15(mXor >> 16)
	andMask1 := spread16To15(mAnd >> 16)

	var total, aRow1 uint64
	for i := uint32(0); i < 24; i++ {
		xorMaskRow := 0 - (1 & (uint64(mXor) >> i))
		andMaskRow := 0 - (1 & (uint64(mAnd) >> i))
		var rowsum uint64
		w := v[2*i] ^ xorMask0 ^ xorMaskRow
		w &= andMask0 & andMaskRow
		rowsum += nibbleSum(w)
		w = v[2*i+1] ^ xorMask1 ^ xorMaskRow
		w &= andMask1 & andMaskRow
		rowsum += nibbleSum(w & 0xffffffff)
		total += rowsum
		if i == row {
			aRow1 = rowsum
		}
	}
	return int((aRow1 << 16) + total)
}

// spread16To15 spreads bits 0..15 of x to 4-bit
// fields, setting a field to 0xf when its bit is
// set. C macro MMV_UINT_SPREAD followed by *= 15.
func spread16To15(x uint32) uint64 {
	a := uint64(x & 0xffff)
	a = (a & 0xff) + ((a & 0xff00) << 24)
	a = (a & 0xf0000000f) + ((a & 0xf0000000f0) << 12)
	a = (a & 0x3000300030003) + ((a & 0xc000c000c000c) << 6)
	a = (a & 0x101010101010101) + ((a & 0x202020202020202) << 3)
	return a * 15
}

// nibbleSum returns the sum of all sixteen 4-bit
// fields of w as a single integer.
func nibbleSum(w uint64) uint64 {
	w = (w & 0xf0f0f0f0f0f0f0f) + ((w >> 4) & 0xf0f0f0f0f0f0f0f)
	w += w >> 8
	w += w >> 16
	w += w >> 32
	return w & 0xff
}

// leech3matrixRank returns (r << 48) + w, where r
// is the rank modulo 3 of the 24x24 matrix b = a - d*I
// and w is a kernel vector (Leech-mod-3 encoding) when
// r == 23, else 0. a is a 24x24 matrix in matrix-mod-3
// encoding (24 rows, 3 words each) and is destroyed.
// C leech3matrix_rank.
func leech3matrixRank(a []uint64, d uint32) int64 {
	// Treat a as a pair (Ah, Al): Al in columns 0..23,
	// Ah in columns 24..47. Store b - d*I in Al and the
	// unit matrix in Ah.
	for i := 0; i < 72; i += 3 {
		a[i+1] &= 0xffffffff
		a[i+2] = 0
	}
	leech3SubDiag(a, uint64(d), 0)
	leech3SubDiag(a, 2, 24)
	// Echelonize (Ah, Al) together so Al is in echelon
	// form; then the 24-r highest rows of Ah hold the
	// kernel of b.
	leech3Echelon(a)
	// Compress Al into a[0..23] and Ah into a[24..47].
	leech3Compress(a)
	// The rank of Al is the highest index i with a
	// nonzero row.
	i := 24
	for ; i > 0; i-- {
		if a[i-1] != 0 {
			break
		}
	}
	if i != 23 {
		return int64(uint64(i) << 48)
	}
	// Corank 1: row 23 of Ah is the kernel vector.
	return int64((uint64(23) << 48) + xsp2co1FromVectMod3(a[24+23]))
}

// leech3SubDiag subtracts diag from every entry
// a[i][i+offset], i = 0..23, modulo 3. a is in
// matrix-mod-3 encoding. C leech3matrix_sub_diag.
func leech3SubDiag(a []uint64, diag uint64, offset uint32) {
	diag %= 3
	if diag == 0 {
		return
	}
	diag = 3 - diag
	for i := 0; i < 24; i++ {
		colOfs := offset >> 4
		colSh := (offset & 15) << 2
		p := i*3 + int(colOfs)
		a[p] += diag << colSh
		a[p] = reduceMod3(a[p])
		offset++
	}
}

// reduceMod3 reduces each 4-bit field of a (lower 3
// bits may be set) to its 2-bit value modulo 3.
// C macro REDUCE_MOD3.
func reduceMod3(a uint64) uint64 {
	return (a + ((a >> 2) & 0x1111111111111111)) & negMaskMod3
}

const negMaskMod3 = 0x3333333333333333

// leech3Echelon transforms a (matrix-mod-3 encoding)
// to row echelon form over columns 0..23 and returns
// the number of nonzero rows. C leech3matrix_echelon.
func leech3Echelon(a []uint64) int {
	rows := 0
	for col := uint32(0); col < 24; col++ {
		rows += leech3Pivot3(a, rows, col)
	}
	return rows
}

// leech3Pivot3 pivots the rows of a (matrix-mod-3
// encoding) at and below row first over the given
// column. It zeroes that column in all lower rows
// using the first row with a nonzero entry, swaps
// that row into position first, and returns 1 if a
// pivot was found, else 0. C pivot3.
func leech3Pivot3(a []uint64, first int, column uint32) int {
	colOfs := int(column >> 4)
	colSh := (column & 15) << 2
	pivot := -1
	var signPivot uint64
	for r := first; r < 24; r++ {
		signPivot = (a[r*3+colOfs] >> colSh) + 1
		if signPivot&2 != 0 {
			pivot = r
			break
		}
	}
	if pivot < 0 {
		return 0
	}
	signPivot++
	for r := pivot + 1; r < 24; r++ {
		sign := (a[r*3+colOfs] >> colSh) + 1
		if sign&2 != 0 {
			sign += signPivot
			addRowMod3(a, r*3, pivot*3, sign)
		}
	}
	for k := 0; k < 3; k++ {
		a[first*3+k], a[pivot*3+k] = a[pivot*3+k], a[first*3+k]
	}
	return 1
}

// addRowMod3 adds (-1)**sign times the source row at
// word offset src to the destination row at word
// offset dst, modulo 3. Each row is 3 words in
// matrix-mod-3 encoding. C macro ADDROW_MOD3.
func addRowMod3(a []uint64, dst, src int, sign uint64) {
	sign = (0 - (sign & 1)) & negMaskMod3
	for k := 0; k < 3; k++ {
		a[dst+k] += a[src+k] ^ sign
		a[dst+k] = reduceMod3(a[dst+k])
	}
}

// leech3Compress compresses the 24x48 matrix a (in
// matrix-mod-3 encoding) in place: Al (columns 0..23)
// is stored in a[0..23] and Ah (columns 24..47) in
// a[24..47], with each column reduced to a 2-bit value
// in bits 2j+1, 2j of the entry. C leech3matrix_compress.
func leech3Compress(a []uint64) {
	var w [24]uint64
	for i := 0; i < 24; i++ {
		j := i * 3
		v0 := compress153(a[j])
		tmp := compress153(a[j+1])
		v1 := compress153(a[j+2])
		v0 += (tmp & 0xffff) << 32
		v1 = (v1 << 16) + (tmp >> 16)
		a[i] = reduceFinalMod3(v0)
		w[i] = reduceFinalMod3(v1)
	}
	for i := 0; i < 24; i++ {
		a[i+24] = w[i]
	}
}

// compress153 compresses the sixteen 4-bit fields of
// a (lower 2 bits may be set) to adjacent 2-bit fields
// in the lower half of the result. C macro COMPRESS_15_3.
func compress153(a uint64) uint64 {
	a = (a & 0x303030303030303) + ((a >> 2) & 0xc0c0c0c0c0c0c0c)
	a = (a & 0xf000f000f000f) + ((a >> 4) & 0xf000f000f000f0)
	a = (a & 0xff000000ff) + ((a >> 8) & 0xff000000ff00)
	a = (a & 0xffff) + ((a >> 16) & 0xffff0000)
	return a
}

// reduceFinalMod3 maps each 2-bit field of value 3 to
// 0, the others unchanged. C macro REDUCE_FINAL_MOD3.
func reduceFinalMod3(a uint64) uint64 {
	tmp := a & (a >> 1) & 0x5555555555555555
	return a ^ tmp ^ (tmp << 1)
}

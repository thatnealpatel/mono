package cgt

// G_x0 membership test via the precomputed order
// vector v_1, ported from mm_order.c (find_in_Gx0,
// find_in_Qx0, mm_order_check_in_Gx0) and the helpers
// in mm_order_vector.c, mm15_op_eval_A.c and
// leech2matrix.c.
//
// The membership decision is NOT a syntactic property
// of the N_0-reduced word: a power g^e may reduce to a
// word that still contains tau atoms yet acts as an
// element of G_x0. The order vector v_1 identifies
// such an element from its image v_1 . g^e, following
// [Sey22].

import "math/bits"

//////////////////////////////////////////////////
// Part 1: layout of the precomputed tag data
//////////////////////////////////////////////////

// Offsets into orderTagTable (in uint32 units),
// matching the C enum tag_offsets generated from the
// VECTOR_LENGTHS ordering in
// mmgroup.dev.mm_reduce.order_vector.
const (
	otWatermarkPerm = 0             // len 24
	otTagsX         = 24            // len 24
	otSolveX        = otTagsX + 24  // len 24
	otTagsY         = otSolveX + 24 // len 11
	otSolveY        = otTagsY + 11  // len 11
	otTagSign       = otSolveY + 11 // len 1
	otNormA         = otTagSign + 1 // len 1
)

// ovDestZ is the destination word offset of tag Z in a
// p=15 vector. C macro MM_OP15_OFS_Z = MM_AUX_OFS_Z >> 4.
const ovDestZ = mmAuxOfsZ >> 4

// orderTagNormA returns the precomputed sum of squares
// of the A part of v_1 mod 15. C TAG_VECTOR[OFS_NORM_A].
func orderTagNormA() uint32 {
	return orderTagTable[otNormA]
}

//////////////////////////////////////////////////
// Part 2: small primitives
//////////////////////////////////////////////////

// uint64Parity returns the parity of the bits of x. C
// uint64_parity.
func uint64Parity(x uint64) uint32 {
	return uint32(bits.OnesCount64(x) & 1)
}

// mmvExtractSparseSigns returns the bitmask whose bit i
// is set iff entry sp[i] of v is the negative of the
// value coded in sp[i]. It returns -1 if any selected
// coordinate of v is zero or has a value other than
// plus or minus the coded value, or if length exceeds
// 31. C mm_aux_mmv_extract_sparse_signs.
func mmvExtractSparseSigns(p int, v []uint64, sp []uint32, length int) int32 {
	if mmAuxBadP(p) || length > 31 {
		return -1
	}
	var v1 uint32
	sp1 := make([]uint32, length)
	for i := 0; i < length; i++ {
		sp1[i] = sp[i] & 0xffffff00
	}
	mmvExtractSparse(p, v, sp1, length)
	for i := 0; i < length; i++ {
		if sp1[i]&uint32(p) == 0 {
			return -1
		}
		t := (sp[i] ^ sp1[i]) & uint32(p)
		if t != 0 && t != uint32(p) {
			return -1
		}
		v1 |= (t & 1) << uint(i)
	}
	return int32(v1)
}

// leech2MatrixSolveEqn returns v * b, the XOR of those
// rows b[row] for which bit row of v is set. The matrix
// b is the precomputed echelon matrix stored in the tag
// data (output of the C leech2matrix_prep_eqn). C
// leech2matrix_solve_eqn.
func leech2MatrixSolveEqn(b []uint32, nrows int, v uint64) uint32 {
	var w uint32
	for row := 0; row < nrows; row++ {
		if v&(1<<uint(row)) != 0 {
			w ^= b[row]
		}
	}
	return w
}

//////////////////////////////////////////////////
// Part 3: watermarking the A part mod 15
//////////////////////////////////////////////////

// opWatermarkA writes into w the sorted per-row
// watermarks of the tag-A matrix of the p=15 vector v.
// Watermark w[i] = i + 32*S(A,i), where S(A,i) is
// invariant under sign changes and row/column fixing
// permutations. It returns false if two watermarks
// coincide (ignoring the row in the low 5 bits). C
// mm_op15_watermark_A.
func opWatermarkA(v []uint64, w []uint32) bool {
	var d [8]uint64
	d[0] = 0
	d[1] = 0x20
	for i := 2; i < 8; i++ {
		d[i] = 13 * d[i-1]
	}
	for i := 0; i < 24; i++ {
		var m uint64
		for j := 0; j < 2; j++ {
			x := v[2*i+j]
			y := x & 0x8888888888888888
			y = (y << 1) - (y >> 3)
			x ^= y
			for k := 0; k < 64-(j<<5); k += 4 {
				m += d[(x>>uint(k))&7]
			}
		}
		w[i] = uint32(m&0xffffffe0) + uint32(i)
	}
	insertsortU32(w[:24])
	for i := 0; i < 23; i++ {
		if (w[i]^w[i+1])&0xffffffe0 == 0 {
			return false
		}
	}
	return true
}

// opWatermarkAPermNum watermarks the tag-A matrix of
// the p=15 vector v and returns the number of the M_24
// permutation that maps the matrix A' watermarked by w
// to A. It returns -1 if no such permutation exists or
// if it is not in M_24. C mm_op15_watermark_A_perm_num.
func opWatermarkAPermNum(w []uint32, v []uint64) int32 {
	var w1 [24]uint32
	if !opWatermarkA(v, w1[:]) {
		return -1
	}
	var perm [32]byte
	var err uint32
	for i := 0; i < 24; i++ {
		perm[i] = 24
	}
	for i := 0; i < 24; i++ {
		err |= w[i] ^ w1[i]
		perm[w[i]&0x1f] = byte(w1[i] & 0x1f)
	}
	if err&0xffffffe0 != 0 || permCheck(perm[:]) != 0 {
		return -1
	}
	return int32(PermToM24num(perm[:]))
}

// insertsortU32 sorts a ascending in place. C
// insertsort (in mm15_op_eval_A.c).
func insertsortU32(a []uint32) {
	for i := 1; i < len(a); i++ {
		x := a[i]
		j := i
		for j >= 1 && a[j-1] > x {
			a[j] = a[j-1]
			j--
		}
		a[j] = x
	}
}

//////////////////////////////////////////////////
// Part 4: comparing against the order vector
//////////////////////////////////////////////////

// orderComparePartA reports whether the tag-A part of
// the p=15 vector v (its first 48 words) differs from
// the tag-A part of the precomputed order vector v_1.
// C mm_order_compare_vector_part_A returns 0 on
// equality, so this returns the negation of equality.
func orderComparePartA(v []uint64) bool {
	v1 := loadOrderVector()
	a := v[ovDestA : ovDestA+48]
	b := v1.data[ovDestA : ovDestA+48]
	reduceMMVFields(15, a, 24*32)
	reduceMMVFields(15, b, 24*32)
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// orderCompareVector reports whether the p=15 vector v
// differs from the precomputed order vector v_1. C
// mm_order_compare_vector returns 0 on equality, so
// this returns the negation of equality.
func orderCompareVector(v []uint64) bool {
	w := &MMVector{p: 15, data: v}
	return !w.Equal(loadOrderVector())
}

//////////////////////////////////////////////////
// Part 5: recover a G_x0 element from v_1 . g
//////////////////////////////////////////////////

// findInGx0 assumes the p=15 vector v is the image of
// the order vector v_1 under an unknown monster element
// g. If g lies in G_x0 it computes an element g1 (a
// word of at most 11 G_x0 generators, written to g)
// with g^{-1} g1 in Q_x0; otherwise it detects this
// with high probability. It returns the length of g1,
// or a value >= 0x100 if g is found not to be in G_x0.
// C find_in_Gx0 with mode 0.
func findInGx0(v []uint64, g []uint32) int {
	if uint32(OpNormA(15, v)) != orderTagNormA() {
		return 0x101
	}
	w3 := uint64(evalARankMod3(v, 0)) & 0xffffffffffff
	if w3 == 0 {
		return 0x102
	}
	wType4 := genLeech3To2Type4(w3)
	if wType4 == 0 {
		return 0x103
	}

	// Work on a copy of the A part only (48 words); the
	// monomial part is irrelevant to find_in_Gx0.
	work := append([]uint64(nil), v[:48]...)

	lenG := genLeech2ReduceType4(wType4, g)
	if lenG < 0 {
		return lenG
	}
	if err := OpWordTagA(15, work, g, lenG, 1); err != nil {
		return -1
	}

	permNum := opWatermarkAPermNum(orderTagTable[otWatermarkPerm:], work)
	if permNum < 0 {
		return 0x104
	}
	if permNum > 0 {
		g[lenG] = 0xA0000000 + uint32(permNum)
		if err := OpWordTagA(15, work, g[lenG:], 1, 1); err != nil {
			return -1
		}
		lenG++
	}

	vY := mmvExtractSparseSigns(15, work, orderTagTable[otTagsY:], 11)
	if vY < 0 {
		return 0x105
	}
	y := leech2MatrixSolveEqn(orderTagTable[otSolveY:], 11, uint64(vY))
	if y > 0 {
		g[lenG] = 0xC0000000 + y
		if err := OpWordTagA(15, work, g[lenG:], 1, 1); err != nil {
			return -1
		}
		lenG++
	}

	if orderComparePartA(work) {
		return 0x106
	}
	for i := lenG; i < 11; i++ {
		g[i] = 0
	}
	return lenG
}

// findInQx0 assumes the p=15 vector v is the image of
// the order vector v_1 under an unknown monster element
// g, and that g contains the output of a successful
// findInGx0 call. It appends the Q_x0 part of g (atoms
// with tags x and delta) to g and returns the new word
// length, or a value >= 0x100 on failure. It destroys
// v. C find_in_Qx0.
func findInQx0(v []uint64, g []uint32, work []uint64) int {
	length := 10
	for length > 0 && g[length-1] == 0 {
		length--
	}
	if g[0] != 0 {
		if err := OpWord(15, v, g, length, 1, work); err != nil {
			return -1
		}
	}

	vX := mmvExtractSparseSigns(15, v, orderTagTable[otTagsX:], 24)
	if vX < 0 {
		return 0x107
	}
	x := leech2MatrixSolveEqn(orderTagTable[otSolveX:], 24, uint64(vX)) & 0xffffff
	vSign := ((x >> 12) & 0x7ff) ^ (x & 0x800)
	aa := [1]uint32{orderTagTable[otTagSign] ^ (vSign << 14)}
	sign := mmvExtractSparseSigns(15, v, aa[:], 1)
	if sign < 0 {
		return 0x108
	}

	signBit := uint32(sign) ^ uint64Parity(uint64(x&(x>>12)&0x7ff))
	x ^= (signBit & 1) << 24
	x ^= PloopTheta(x >> 12)

	length1 := length
	if x&0xfff != 0 {
		g[length1] = 0x90000000 + (x & 0xfff)
		length1++
	}
	x = (x >> 12) & 0x1fff
	if x != 0 {
		g[length1] = 0xB0000000 + x
		length1++
	}
	if length1 > length {
		if err := OpWord(15, v, g[length:], length1-length, 1, work); err != nil {
			return -1
		}
	}
	if orderCompareVector(v) {
		return 0x209
	}
	return length1
}

// orderCheckInGx0Mode1 assumes the p=15 vector v is the
// image of the order vector v_1 under an unknown monster
// element g, where g is the element still being reduced.
// If g lies in G_x0 it writes g^{-1} into g as a word of
// G_x0 generators (NOT inverted to g, matching C mode
// bit 0) and returns its length. It returns a value >=
// 0x100 if g is found not to be in G_x0, or a negative
// value on error. It destroys v. C mm_order_check_in_Gx0
// with mode 1 (return inverse, operate in place, no
// comment atom). The work buffer must hold a p=15
// vector.
func orderCheckInGx0Mode1(v []uint64, g []uint32, work []uint64) int {
	res := findInGx0(v, g)
	if res >= 0x100 || res < 0 {
		return res
	}
	res = findInQx0(v, g, work)
	if res >= 0x100 || res < 0 {
		return res
	}
	// mode bit 0 is set: leave g as the inverse element
	// g^{-1} that find_in_Gx0/find_in_Qx0 produced; the
	// caller inverts the whole accumulated word.
	return res
}

// orderCheckInGx0 assumes the p=15 vector v is the
// image of the order vector v_1 under an unknown
// monster element g. If g lies in G_x0 it returns g as
// a word of at most 10 G_x0 generators; otherwise it
// returns nil. It does not modify v. C
// mm_order_check_in_Gx0 with mode 8 (preserve v).
func orderCheckInGx0(v []uint64) []uint32 {
	var g [12]uint32
	res := findInGx0(v, g[:])
	if res >= 0x100 || res < 0 {
		return nil
	}

	// find_in_Qx0 destroys its input, so operate on a copy.
	vc := append([]uint64(nil), v...)
	work := make([]uint64, MMVSize(15))
	res = findInQx0(vc, g[:], work)
	if res >= 0x100 || res < 0 {
		return nil
	}

	// mode bit 0 is unset: return g (the element g, not its
	// inverse) as the word of G_x0 generators. A successful
	// match with res == 0 means g is the neutral element;
	// return a non-nil empty word to distinguish it from a
	// failed match (nil).
	invertWord(g[:res])
	return append(make([]uint32, 0, res), g[:res]...)
}

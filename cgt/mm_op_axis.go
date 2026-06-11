package cgt

import (
	"patel.codes/cgt/mmindex"
	"patel.codes/cgt/xsp2co1"
)

// This file ports mm_op*_mul_std_axis (Griess
// algebra product with the standard axis v+) and
// mm_op*_store_axis, field-generically.

// mod returns x reduced into [0, p).
func mod(x, p int) int {
	x %= p
	if x < 0 {
		x += p
	}
	return x
}

// abcEntry returns M[row][col] from the flat
// A/B/C block, where row indexes rows 0..71 across
// the three 24x24 matrices. Internal layout: matrix
// m, local row r -> global row 24*m + r at internal
// index (24*m + r)*32 + col.
func abcEntry(p int, v []uint64, row, col int) int {
	return int(getMMV(p, v, uint32(row*32+col)))
}

// putAbcEntry writes M[row][col] in the flat A/B/C
// block, updating the symmetric twin (handled by
// putMMV for the A/B/C tags).
func putAbcEntry(p int, v []uint64, row, col, val int) {
	putMMV(p, uint8(mod(val, p)), v, uint32(row*32+col))
}

// MulStdAxis replaces v by v * v+ (Griess product
// with the standard 2A axis). C mm_op*_mul_std_axis.
func MulStdAxis(p int, v []uint64) {
	mulStdAxisABC(p, v)
	mulStdAxisT(p, v)
	mulStdAxisXYZ(p, v)
}

// mulStdAxisABC ports do_ABC. It reads the A part
// snapshot first so the rewrites are consistent.
func mulStdAxisABC(p int, v []uint64) {
	a22 := abcEntry(p, v, 2, 2)
	a33 := abcEntry(p, v, 3, 3)
	a23 := abcEntry(p, v, 2, 3)
	b23 := abcEntry(p, v, 24+2, 3)
	a16 := mod(4*(a22+a33)-8*(a23+b23), p)
	a4 := mod(4*(a22-a33), p)

	var diag [3][4]int
	diag[0][0] = mod(a16+a4, p)
	diag[0][1] = mod(-a16, p)
	diag[0][2] = mod(a16-a4, p)
	diag[1][1] = mod(-2*a16, p)

	for m := 0; m < 3; m++ {
		base := 24 * m
		// Snapshot rows 2 and 3 before overwriting.
		var r2, r3 [24]int
		for j := 0; j < 24; j++ {
			r2[j] = abcEntry(p, v, base+2, j)
			r3[j] = abcEntry(p, v, base+3, j)
		}
		// Rows 2 and 3: M'[2][j] = 4*(M[2][j]-M[3][j]);
		// M'[3][j] = -that.
		newRow2 := make([]int, 24)
		for j := 0; j < 24; j++ {
			d := mod(4*(r2[j]-r3[j]), p)
			newRow2[j] = d
			putAbcEntry(p, v, base+2, j, d)
			putAbcEntry(p, v, base+3, j, mod(-d, p))
		}
		// Overwrite the diagonal block columns 2,3 of
		// rows 2,3 from diag[m] (DMASK = cols 2,3).
		putAbcEntry(p, v, base+2, 2, diag[m][0])
		putAbcEntry(p, v, base+2, 3, diag[m][1])
		putAbcEntry(p, v, base+3, 2, diag[m][1])
		putAbcEntry(p, v, base+3, 3, diag[m][2])
		// All other rows: set columns 2,3 from the new
		// row-2 value at that column and zero the rest.
		for row := 0; row < 24; row++ {
			if row == 2 || row == 3 {
				continue
			}
			val := newRow2[row]
			for j := 0; j < 24; j++ {
				putAbcEntry(p, v, base+row, j, 0)
			}
			putAbcEntry(p, v, base+row, 2, val)
			putAbcEntry(p, v, base+row, 3, mod(-val, p))
		}
	}
}

// mulStdAxisT ports do_T. Each octad row is scaled
// according to TABLE_OCTAD_TO_STD_AX_OP: 0 zeros the
// row, 1 leaves it, 2 applies the pos1 transform, 3
// the pos6 transform. Both transforms compute
// 4*(a-b) on selected suboctad pairs.
func mulStdAxisT(p int, v []uint64) {
	base := mmAuxOfsT
	for o := 0; o < 759; o++ {
		c := subTableOctadToStdAxOp(uint32(o))
		rowBase := base + o*64
		switch c {
		case 0:
			for j := 0; j < 64; j++ {
				putMMV(p, 0, v, uint32(rowBase+j))
			}
		case 1:
			// no change
		case 2:
			mulStdAxisTPos1(p, v, rowBase)
		case 3:
			mulStdAxisTPos6(p, v, rowBase)
		}
	}
}

// Field sets for the std-axis tag-T transforms,
// decoded from the SWAR masks in do_T_case_pos1 and
// do_T_case_pos6. The pos1 case pairs field f with
// f+1; word w uses smask0 (f in {0,6,10,12}) when
// (0x96>>w)&1, else smask1 (f in {2,4,8,14}). The
// pos6 case pairs field f with f+2 over {2,3,10,11}.
var (
	stdAxTPos1Fields0 = []int{0, 6, 10, 12} // SMASK0
	stdAxTPos1Fields1 = []int{2, 4, 8, 14}  // SMASK1
	stdAxTPos6Fields  = []int{2, 3, 10, 11}
)

// mulStdAxisTPos1 applies the pos1 transform to the
// 64-entry octad row at rowBase. For each selected
// suboctad pair (s, s+1) the entries become
// 4*(a-b) and -4*(a-b).
func mulStdAxisTPos1(p int, v []uint64, rowBase int) {
	for w := 0; w < 4; w++ {
		fields := stdAxTPos1Fields1
		if (0x96>>w)&1 != 0 {
			fields = stdAxTPos1Fields0
		}
		wordBase := rowBase + 16*w
		for _, fLo := range fields {
			transformPair(p, v, wordBase, fLo, fLo+1)
		}
	}
}

// mulStdAxisTPos6 applies the pos6 transform to the
// octad row: each selected pair (s, s+2) becomes
// 4*(a-b) and -4*(a-b).
func mulStdAxisTPos6(p int, v []uint64, rowBase int) {
	for w := 0; w < 4; w++ {
		wordBase := rowBase + 16*w
		for _, fLo := range stdAxTPos6Fields {
			transformPair(p, v, wordBase, fLo, fLo+2)
		}
	}
}

// transformPair sets entry at index base+lo to
// 4*(old_lo - old_hi) mod p and entry base+hi to its
// negative.
func transformPair(p int, v []uint64, base, lo, hi int) {
	a := int(getMMV(p, v, uint32(base+lo)))
	b := int(getMMV(p, v, uint32(base+hi)))
	d := mod(4*(a-b), p)
	putMMV(p, uint8(d), v, uint32(base+lo))
	putMMV(p, uint8(mod(-d, p)), v, uint32(base+hi))
}

// mulStdAxisXYZ ports do_XYZ: for the tags X, Z, Y
// (6 blocks of 0x200 rows = 2048 rows total spanning
// X,Z,Y... actually X has 2048 rows; the C iterates
// 6 outer * 0x200 = 3072 active rows with a 0x200
// gap), each active row keeps only column 2, set to
// 4*(M[row][2]-M[row][3]) at column 2 and its
// negative at column 3 (mask SMASK = 0x700 = field
// 2). All other columns are zeroed.
func mulStdAxisXYZ(p int, v []uint64) {
	base := ofsWords(p, mmAuxOfsX)
	per := rowsPer24(p)
	// The C pattern: 6 groups, each processes 0x200
	// rows then skips 0x200 rows. That covers rows
	// 0..0x200, 0x400..0x600, 0x800..0xA00 of each of
	// X, Z, Y (the even halves), matching the short
	// Leech vectors. We reproduce the exact stride.
	rowWords := per
	idx := base
	for g := 0; g < 6; g++ {
		for j := 0; j < 0x200; j++ {
			row := (idx - base) / rowWords
			col2 := int(getMMV(p, v, uint32(mmAuxOfsX+row*32+2)))
			col3 := int(getMMV(p, v, uint32(mmAuxOfsX+row*32+3)))
			d := mod(4*(col2-col3), p)
			for c := 0; c < 24; c++ {
				putMMV(p, 0, v, uint32(mmAuxOfsX+row*32+c))
			}
			putMMV(p, uint8(d), v, uint32(mmAuxOfsX+row*32+2))
			putMMV(p, uint8(mod(-d, p)), v, uint32(mmAuxOfsX+row*32+3))
			idx += rowWords
		}
		idx += 0x200 * rowWords
	}
}

// OpStoreAxis stores the 2A axis corresponding to the
// short Leech-mod-2 vector x into dst (modulo p). C
// mm_op*_store_axis. x must be an element of Q_x0
// mapping to a short Leech lattice vector, given in
// Leech-lattice encoding.
//
// It panics if x is not a short Leech-mod-2 vector
// (via xsp2co1.Short2ToLeech). The C source uses p=15; here
// the modulus is the field parameter p.
func OpStoreAxis(p int, x uint32, dst []uint64) {
	zeroMMV(p, dst)

	// Short Leech coordinates, norm 32, arbitrary sign.
	var a [24]int8
	xsp2co1.Short2ToLeech(x, a[:])

	// Reduce each biased, scaled coordinate mod p. The
	// bias (p << 8) keeps the value non-negative before
	// the modulo, since a[i] is signed. The scale shift
	// is P_BITS-2 (the coordinate is divided by 4 in
	// units of the field width); for p=15 this is 2.
	pBits := uint((mmvConst(p) >> 15) & 15)
	var ua [24]uint32
	for i := 0; i < 24; i++ {
		entry := uint32(int32(a[i])+int32(p)<<8) << (pBits - 2)
		ua[i] = entry % uint32(p)
	}

	// Outer product ua x ua, mod p, written row by row.
	var b [32]uint8
	for i := 0; i < 24; i++ {
		uai := ua[i]
		for j := 0; j < 24; j++ {
			b[j] = uint8(uai * ua[j] % uint32(p))
		}
		// C: mm_aux_write_mmv24(p, b, mv + 2*i, 0, 1).
		// Passing row index i with the full dst slice is
		// equivalent: writeMMV24 advances to row i.
		writeMMV24(p, b[:], dst, uint32(i), 1)
	}

	// Add the central diagonal entry for the axis.
	ind := mmindex.IndexLeech2ToSparse(x) + 2
	if x&0x1000000 == 0 {
		ind ^= uint32(p)
	}
	mmvSetSparse(p, dst, []uint32{ind}, 1)
}

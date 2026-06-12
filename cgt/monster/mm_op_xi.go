package monster

// mm_op_xi.go builds the twenty xi operation tables
// (xiPerm00..xiPerm41 and xiSign00..xiSign41) at
// package init time. They were formerly emitted as a
// 25.8k-line generated source file
// (mm_op_xi_gen.go); the runtime now derives them
// directly from the package generator's reference xi
// operation, generator.XiOpXiShort.
//
// The pipeline mirrors mmgroup's Pre_MM_TablesXi
// (src/mmgroup/dev/mm_basics/mm_tables_xi.py): for
// each of the five table groups n and each exponent
// exp1 in {0,1}, build the forward image table of a
// box under xi**(exp1+1), invert it into image-index
// order, symmetrise the BC group, then split into a
// permutation half (entries reduced mod the row
// length) and a sign half (32 packed sign bits per
// row of 32). Image boxes whose row length is 24 are
// cut from 32 to 24 columns.
//
// The independent first-principles derivation that
// formerly verified the emitted file (a self-
// contained Golay/GenXi reimplementation) now lives
// as a regression cross-check in
// mm_op_xi_regress_test.go, which rebuilds every
// table from scratch and compares against these
// init-built tables element for element.

import "patel.codes/cgt/generator"

// xi box-permutation tables, built by init.
//
//	xiPerm{n}{exp1} is the permutation half of group
//	n under xi**(exp1+1); xiSign{n}{exp1} is the sign
//	half (one uint32 of 32 packed sign bits per row).
var (
	xiPerm00, xiSign00 = xiBuildTable(0, 0)
	xiPerm01, xiSign01 = xiBuildTable(0, 1)
	xiPerm10, xiSign10 = xiBuildTable(1, 0)
	xiPerm11, xiSign11 = xiBuildTable(1, 1)
	xiPerm20, xiSign20 = xiBuildTable(2, 0)
	xiPerm21, xiSign21 = xiBuildTable(2, 1)
	xiPerm30, xiSign30 = xiBuildTable(3, 0)
	xiPerm31, xiSign31 = xiBuildTable(3, 1)
	xiPerm40, xiSign40 = xiBuildTable(4, 0)
	xiPerm41, xiSign41 = xiBuildTable(4, 1)
)

// xiBuildTable derives the (perm, sign) pair for
// table group n and exponent exp1 in {0,1}. It runs
// the full Pre_MM_TablesXi pipeline against
// generator.XiOpXiShort.
func xiBuildTable(n, exp1 int) (perm []uint16, sign []uint32) {
	box := xiMapXi[n][exp1][0]
	img := xiMapXi[n][exp1][1]
	shape := xiBoxShapeOf(box)
	imgShape := xiBoxShapeOf(img)

	table := xiMakeTable(box, exp1+1)
	imgLen := imgShape.rows * imgShape.cols * 32
	inv := xiInvertTable(table, shape.rowLen, imgLen)
	if box == xiBoxBC {
		xiMakeTableBcSymmetric(inv)
	}
	perm, sign = xiSplitTable(inv, shape.cols*32)
	if imgShape.rowLen == 24 {
		perm = xiCut24(perm)
	}
	return perm, sign
}

// xiTSize is the number of live entries per box
// (index 1..5), matching GenXi.make_table's t_size.
var xiTSize = [6]int{0, 2496, 23040, 24576, 32768, 32768}

// xiMakeTable returns the low 16 bits of the image of
// every entry of box uBox under xi**uExp, using the
// runtime reference operation. C GenXi.make_table.
func xiMakeTable(uBox, uExp int) []uint16 {
	length := xiTSize[uBox]
	a := make([]uint16, length)
	base := uint32(uBox) << 16
	for i := 0; i < length; i++ {
		a[i] = uint16(generator.XiOpXiShort(base+uint32(i), uExp) & 0xffff)
	}
	return a
}

// xiInvertTable inverts a permutation table. For each
// source index i with column (i&31) below nColumns
// whose image r&0x7fff is below lenResult,
// result[r&0x7fff] receives i with the sign bit
// r&0x8000 carried over. C GenXi.invert_table.
//
// xiInvertTable panics if either length is not a
// multiple of 32 (a static property of the fixed box
// shapes).
func xiInvertTable(table []uint16, nColumns, lenResult int) []uint16 {
	if len(table)&31 != 0 || lenResult&31 != 0 {
		panic("xiInvertTable: lengths must be multiples of 32")
	}
	result := make([]uint16, lenResult)
	for i, r := range table {
		if (i&31) < nColumns && int(r&0x7fff) < lenResult {
			result[r&0x7fff] = uint16(i) | (r & 0x8000)
		}
	}
	return result
}

// xiMakeTableBcSymmetric symmetrises the inverted BC
// table in place across its B and C 24x24 blocks
// (rows of 32). C make_table_bc_symmetric.
func xiMakeTableBcSymmetric(table []uint16) {
	b := func(i, j int) int { return 32*i + j }
	c := func(i, j int) int { return 32*(i+24) + j }
	for i := 0; i < 24; i++ {
		for j := 0; j < i; j++ {
			table[b(j, i)] = table[b(i, j)]
			table[c(j, i)] = table[c(i, j)]
		}
		table[b(i, i)] = uint16(b(i, i))
		table[c(i, i)] = uint16(c(i, i))
	}
}

// xiSplitTable splits a table into a permutation
// table (entries reduced mod modulus) and a sign
// table (one uint32 of 32 packed sign bits per 32
// entries). C GenXi.split_table.
//
// xiSplitTable panics if the table length is not a
// multiple of 32 (a static property of the box
// shapes).
func xiSplitTable(table []uint16, modulus int) ([]uint16, []uint32) {
	length := len(table)
	if length&31 != 0 {
		panic("xiSplitTable: length must be a multiple of 32")
	}
	sign := make([]uint32, length>>5)
	for i := 0; i < length; i += 32 {
		var s uint32
		for j := 0; j < 32; j++ {
			s |= uint32((table[i+j]>>15)&1) << uint(j)
		}
		sign[i>>5] = s
	}
	perm := make([]uint16, length)
	for i, v := range table {
		perm[i] = uint16(int(v&0x7fff) % modulus)
	}
	return perm, sign
}

// xiCut24 keeps the first 24 of every 32 entries. C
// cut24.
func xiCut24(table []uint16) []uint16 {
	out := make([]uint16, 0, len(table)/32*24)
	for i := 0; i < len(table); i += 32 {
		out = append(out, table[i:i+24]...)
	}
	return out
}

// xiBoxShape is one (rows, columns, row_length) box
// shape. C Pre_MM_TablesXi.BOX_SHAPES entries.
type xiBoxShape struct {
	rows, cols, rowLen int
}

// xi box-shape constants. C BOX_SHAPES.
var (
	xiShapeBC = xiBoxShape{1, 78, 32}
	xiShapeT0 = xiBoxShape{45, 16, 32}
	xiShapeT1 = xiBoxShape{64, 12, 32}
	xiShapeX0 = xiBoxShape{64, 16, 24}
	xiShapeX1 = xiBoxShape{64, 16, 24}
)

// box tag identifiers, matching the numeric ids used
// by Pre_MM_TablesXi (BC=1..X1=5).
const (
	xiBoxBC = 1
	xiBoxT0 = 2
	xiBoxT1 = 3
	xiBoxX0 = 4
	xiBoxX1 = 5
)

// xiBoxShapeOf returns the shape of a box id.
//
// xiBoxShapeOf panics on an unknown box id.
func xiBoxShapeOf(box int) xiBoxShape {
	switch box {
	case xiBoxBC:
		return xiShapeBC
	case xiBoxT0:
		return xiShapeT0
	case xiBoxT1:
		return xiShapeT1
	case xiBoxX0:
		return xiShapeX0
	case xiBoxX1:
		return xiShapeX1
	}
	panic("xiBoxShapeOf: bad box id")
}

// xiMapXi is Pre_MM_TablesXi.MAP_XI: for each of the
// five table groups n and each exponent exp1 in
// {0,1}, the [source, destination] box pair. xi
// permutes the boxes 1->1, 2->2, 3->4->5->3.
var xiMapXi = [5][2][2]int{
	{{xiBoxBC, xiBoxBC}, {xiBoxBC, xiBoxBC}},
	{{xiBoxT0, xiBoxT0}, {xiBoxT0, xiBoxT0}},
	{{xiBoxT1, xiBoxX0}, {xiBoxT1, xiBoxX1}},
	{{xiBoxX0, xiBoxX1}, {xiBoxX1, xiBoxX0}},
	{{xiBoxX1, xiBoxT1}, {xiBoxX0, xiBoxT1}},
}

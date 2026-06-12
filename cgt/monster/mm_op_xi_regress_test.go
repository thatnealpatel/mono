package monster

// mm_op_xi_regress_test.go is the regression cross-
// check for the init-built xi tables in mm_op_xi.go.
// It re-derives all twenty tables from first
// principles -- reconstructing the Golay code basis
// from the hexacode (Conway-Sloane SPLAG ch. 11),
// rebuilding every Mathieu primitive (theta,
// syndromes, octads, suboctads) from that basis,
// porting the reference xi operation from mmgroup's
// dev/generators/gen_xi_ref.py, and running its
// Pre_MM_TablesXi pipeline -- then compares the
// result element for element against the tables the
// package builds at init from generator.XiOpXiShort.
//
// This replaces the former generate-time golden-file
// verification: when the 25.8k-line mm_op_xi_gen.go
// was deleted in favour of init() computation, its
// independent derivation moved here so the two
// derivations still cross-check each other on every
// test run. The derivation below is a verbatim port
// of cgt/_gen/mm_op_xi.go (every package-level symbol
// re-prefixed rgx) and intentionally shares no code
// with the runtime path it validates.
//
// Provenance (mmgroup sources reproduced):
//
//	src/mmgroup/dev/mat24/mat24tables.py
//	src/mmgroup/dev/mat24/mat24theta.py
//	src/mmgroup/dev/mat24/mat24aux.py
//	src/mmgroup/dev/generators/gen_xi_ref.py
//	src/mmgroup/dev/mm_basics/mm_tables_xi.py

import (
	"fmt"
	"math/bits"
	"testing"
)

// rgxRuntimeTables indexes the init-built package
// tables by (n, exp1), parallel to the order
// rgxBuildTables emits.
var rgxRuntimePerm = [5][2][]uint16{
	{xiPerm00[:], xiPerm01[:]},
	{xiPerm10[:], xiPerm11[:]},
	{xiPerm20[:], xiPerm21[:]},
	{xiPerm30[:], xiPerm31[:]},
	{xiPerm40[:], xiPerm41[:]},
}

var rgxRuntimeSign = [5][2][]uint32{
	{xiSign00[:], xiSign01[:]},
	{xiSign10[:], xiSign11[:]},
	{xiSign20[:], xiSign21[:]},
	{xiSign30[:], xiSign31[:]},
	{xiSign40[:], xiSign41[:]},
}

// TestXiTablesRegression rebuilds all twenty xi
// tables independently and compares them against the
// init-built runtime tables, every entry.
func TestXiTablesRegression(t *testing.T) {
	m := newXiMat24()
	g := newXiGenXi(m)
	for _, b := range g.rgxBuildTables() {
		wantPerm := rgxRuntimePerm[b.n][b.exp1]
		wantSign := rgxRuntimeSign[b.n][b.exp1]
		if len(b.sign) != len(wantSign) {
			t.Fatalf("xiSign%d%d length %d, runtime %d",
				b.n, b.exp1, len(b.sign), len(wantSign))
		}
		for i := range b.sign {
			if b.sign[i] != wantSign[i] {
				t.Fatalf("xiSign%d%d[%d] = 0x%08x, runtime 0x%08x",
					b.n, b.exp1, i, b.sign[i], wantSign[i])
			}
		}
		if len(b.perm) != len(wantPerm) {
			t.Fatalf("xiPerm%d%d length %d, runtime %d",
				b.n, b.exp1, len(b.perm), len(wantPerm))
		}
		for i := range b.perm {
			if b.perm[i] != wantPerm[i] {
				t.Fatalf("xiPerm%d%d[%d] = 0x%04x, runtime 0x%04x",
					b.n, b.exp1, i, b.perm[i], wantPerm[i])
			}
		}
	}
}

//////////////////////////////////////////////////
// Golay code basis from the hexacode
//////////////////////////////////////////////////

// rgxHexEncode is HexacodeToGolay.HEXA_ENCODE: the
// even MOG-column interpretation of a hexacode
// score j. rgxHexEncode[j] << (4*i) is the unique
// MOG vector even in column i (and zero elsewhere
// and in row 0) with score j.
var rgxHexEncode = [4]uint32{0x0, 0xc, 0xa, 0x6}

// rgxHBasis is HexacodeToGolay.HBASIS: the GF(2)
// basis of the hexacode in SPLAG ch. 11. Each row
// is six GF(4) entries (0..3).
var rgxHBasis = [6][6]int{
	{1, 0, 0, 1, 3, 2}, {0, 1, 0, 1, 2, 3}, {0, 0, 1, 1, 1, 1},
	{2, 0, 0, 2, 1, 3}, {0, 2, 0, 2, 3, 1}, {0, 0, 2, 2, 2, 2},
}

// rgxHexacodeVectorToMog maps a hexacode word h
// (six entries 0..3) to a Golay code word, mapping
// entry h[i] to an even interpretation of MOG
// column i with score h[i] and zero top bit. C
// HexacodeToGolay.hexacode_vector_to_mog.
func rgxHexacodeVectorToMog(h [6]int) uint32 {
	var s uint32
	for i, x := range h {
		s += rgxHexEncode[x] << (4 * uint(i))
	}
	return s
}

// rgxMogStdOctad is HexacodeToGolay.mog_std_octad:
// the MOG octad with ones in row 0 and column i,
// minus the (row 0, column i) intersection bit.
func rgxMogStdOctad(i uint) uint32 {
	return 0x111111 ^ (0xf << (4 * i))
}

// rgxHibit returns the position of the highest set
// bit of x, or -1 if x is zero. C bitfunctions
// hibit.
func rgxHibit(x uint32) int { return bits.Len32(x) - 1 }

// rgxV2 returns the 2-adic valuation of x (the
// index of its lowest set bit). x must be nonzero.
// C bitfunctions v2.
func rgxV2(x uint32) int { return bits.TrailingZeros32(x) }

// rgxPivotBinaryHigh performs Gaussian elimination
// over GF(2) on the rows of a, pivoting on the
// highest available bit column each step and
// dropping dependent rows. It returns the reduced
// basis. C bitfunctions pivot_binary_high (only
// the basis, not the column list, is needed here).
func rgxPivotBinaryHigh(a []uint32) []uint32 {
	basis := make([]uint32, len(a))
	copy(basis, a)
	n := 0 // rows kept so far
	for i := 0; i < len(basis); i++ {
		// Choose the max element in basis[i:] (so the
		// highest pivot column is taken).
		maxIdx := i
		for j := i + 1; j < len(basis); j++ {
			if basis[j] > basis[maxIdx] {
				maxIdx = j
			}
		}
		m := basis[maxIdx]
		if m == 0 {
			break // remaining rows are dependent
		}
		basis[i], basis[maxIdx] = m, basis[i]
		mask := uint32(1) << uint(rgxHibit(m))
		for j := 0; j < len(basis); j++ {
			if j != i && basis[j]&mask != 0 {
				basis[j] ^= m
			}
		}
		n = i + 1
	}
	return basis[:n]
}

// rgxBitMatInverse returns the inverse of the
// square GF(2) bit matrix a (each entry a row of
// bits). C bitfunctions bit_mat_inverse.
//
// rgxBitMatInverse panics if a is not square or not
// invertible; both are static properties of the
// fixed Golay basis, so a panic signals a coding
// error, not bad runtime input.
func rgxBitMatInverse(a []uint32) []uint32 {
	n := len(a)
	// Augment each row with an identity column block
	// above bit n: ah[i] = a[i] | (1 << (n+i)).
	ah := make([]uint64, n)
	for i := range a {
		ah[i] = uint64(a[i]) | (uint64(1) << uint(n+i))
	}
	perm := make([]int, n)
	for i := range perm {
		perm[i] = -1
	}
	for i := 0; i < n; i++ {
		piv := bits.TrailingZeros64(ah[i] & ((uint64(1) << uint(n)) - 1))
		if piv >= n {
			panic("rgxBitMatInverse: matrix not invertible")
		}
		perm[piv] = i
		msk := uint64(1) << uint(piv)
		for j := 0; j < n; j++ {
			if j != i && ah[j]&msk != 0 {
				ah[j] ^= ah[i]
			}
		}
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = uint32(ah[perm[i]] >> uint(n))
	}
	return out
}

// rgxBitMatTranspose returns the transpose of the
// bit matrix a with ncols columns (each result row
// is a column of a). C bitfunctions
// bit_mat_transpose.
func rgxBitMatTranspose(a []uint32, ncols int) []uint32 {
	t := make([]uint32, ncols)
	for i, row := range a {
		for j := 0; j < ncols; j++ {
			if (row>>uint(j))&1 != 0 {
				t[j] ^= 1 << uint(i)
			}
		}
	}
	return t
}

// rgxAnyGolayBasis returns a basis of the Golay
// code computed directly from the hexacode. C
// HexacodeToGolay.any_golay_basis.
func rgxAnyGolayBasis() []uint32 {
	basis := make([]uint32, 0, 12)
	for _, h := range rgxHBasis {
		basis = append(basis, rgxHexacodeVectorToMog(h))
	}
	for i := uint(0); i < 6; i++ {
		basis = append(basis, rgxMogStdOctad(i))
	}
	return rgxPivotBinaryHigh(basis)
}

// rgxBetterCocodeBasis returns a good basis of a
// transversal of the Golay cocode. C
// HexacodeToGolay.better_cocode_basis.
func rgxBetterCocodeBasis() []uint32 {
	// hexa = e(i,a) for the listed (i,a) pairs.
	idx := [6]int{2, 2, 1, 1, 0, 0}
	val := [6]int{2, 1, 2, 1, 2, 1}
	basis := []uint32{0x111111, 0x1, 0x110, 0x1010, 0x10010, 0x100010}
	for k := 0; k < 6; k++ {
		var e [6]int
		e[idx[k]] = val[k]
		basis = append(basis, rgxHexacodeVectorToMog(e))
	}
	// basis = basis[2:] + basis[:2]
	rot := append([]uint32{}, basis[2:]...)
	rot = append(rot, basis[:2]...)
	return rot
}

// rgxBetterCodeBasis returns the Golay code basis
// orthogonal to the cocode basis. C
// HexacodeToGolay.better_code_basis via
// code_basis_from_cocode_basis.
func rgxBetterCodeBasis(cocodeBasis []uint32) []uint32 {
	full := append(append([]uint32{}, rgxAnyGolayBasis()...), cocodeBasis...)
	inv := rgxBitMatInverse(full)
	return rgxBitMatTranspose(inv, 24)[12:]
}

//////////////////////////////////////////////////
// Mat24 reference (built from the Golay basis)
//////////////////////////////////////////////////

// rgxMat24 holds the Golay basis and the small set
// of derived tables needed by the xi pipeline. It
// is the package-main analogue of mmgroup's
// Mat24Tables/Mat24 reference classes, trimmed to
// the methods GenXi calls.
type rgxMat24 struct {
	// basis[0:12] is the cocode transversal basis,
	// basis[12:24] the Golay code basis. recipBasis
	// is its GF(2) inverse.
	basis      [24]uint32
	recipBasis [24]uint32

	// syndromeTable[c&0x7ff] packs the weight<=3
	// syndrome of cocode word c. Bit 15 marks an
	// even cocode word of weight 2. C
	// make_syndrome_table.
	syndromeTable [0x800]uint16

	// thetaTable[v&0x7ff] is the Parker-loop cocycle
	// theta(v) in cocode rep, with the halved Golay
	// weight in bits 14..12. C
	// make_augmented_theta_table.
	thetaTable [0x800]uint16

	// octDecTable[o] is the gcode of octad o
	// (0<=o<759). octEncTable[v&0x7ff] encodes the
	// octad number of a (possibly complemented)
	// octad v. octadTable[8*o..8*o+8] lists the bit
	// positions of octad o. C make_octad_tables.
	octDecTable [759]uint16
	octEncTable [2048]uint16
	octadTable  [759 * 8]uint8
}

// rgxBw24 returns the bit weight of the low 24 bits
// of v. C bitfunctions bw24.
func rgxBw24(v uint32) int { return bits.OnesCount32(v & 0xffffff) }

// rgxLsbit24 returns the index of the lowest set
// bit of v, or 24 if no bit below 24 is set. C
// Mat24Tables.lsbit24.
func rgxLsbit24(v uint32) int {
	if v&0xffffff == 0 {
		return 24
	}
	return bits.TrailingZeros32(v & 0xffffff)
}

// rgxBitMatMulVec multiplies the bit vector v by
// the bit matrix m (xor of the matrix rows
// selected by the set bits of v). C bitfunctions
// bit_mat_mul for an integer left argument.
func rgxBitMatMulVec(v uint32, m []uint32) uint32 {
	var r uint32
	for i, row := range m {
		if (v>>uint(i))&1 != 0 {
			r ^= row
		}
	}
	return r
}

// newXiMat24 builds the Golay basis and all
// derived tables.
func newXiMat24() *rgxMat24 {
	m := &rgxMat24{}
	cocode := rgxBetterCocodeBasis()
	code := rgxBetterCodeBasis(cocode)
	for i := 0; i < 12; i++ {
		m.basis[i] = cocode[i]
		m.basis[12+i] = code[i]
	}
	inv := rgxBitMatInverse(m.basis[:])
	copy(m.recipBasis[:], inv)

	m.makeSyndromeTable()
	m.makeThetaTable()
	m.makeOctadTables()
	return m
}

// vectToVintern converts a vector in GF(2)^24
// (vect rep) to internal rep: v times recipBasis.
// C vect_to_vintern (here via the basis rather
// than the byte encoding tables).
func (m *rgxMat24) vectToVintern(v uint32) uint32 {
	return rgxBitMatMulVec(v, m.recipBasis[:])
}

// vinternToVect is the inverse of vectToVintern: v
// times basis. C vintern_to_vect.
func (m *rgxMat24) vinternToVect(v uint32) uint32 {
	return rgxBitMatMulVec(v, m.basis[:])
}

// vectToCocode returns the cocode (low 12 bits of
// internal rep) of vector v. C vect_to_cocode.
func (m *rgxMat24) vectToCocode(v uint32) uint32 {
	return m.vectToVintern(v) & 0xfff
}

// gcodeToVect maps a Golay code number (gcode rep)
// to its bit vector. C gcode_to_vect: internal rep
// (v<<12) through the basis.
func (m *rgxMat24) gcodeToVect(v uint32) uint32 {
	return m.vinternToVect((v & 0xfff) << 12)
}

// cocodeToVect maps a cocode number to a
// representative vector. C cocode_to_vect.
func (m *rgxMat24) cocodeToVect(c uint32) uint32 {
	return m.vinternToVect(c & 0xfff)
}

// vectToGcode maps a Golay code vector to its
// gcode number. C vect_to_gcode. It panics if v is
// not a code word (a static property of all
// callers here).
func (m *rgxMat24) vectToGcode(v uint32) uint32 {
	cn := m.vectToVintern(v)
	if cn&0xfff != 0 {
		panic("rgxMat24.vectToGcode: not a Golay code word")
	}
	return cn >> 12
}

// makeSyndromeTable fills syndromeTable from the
// reciprocal basis. C make_syndrome_table.
func (m *rgxMat24) makeSyndromeTable() {
	const c1 = (24 << 5) | (24 << 10)
	var rb [24]uint32
	for i := 0; i < 24; i++ {
		if m.recipBasis[i]&0x800 == 0 {
			panic("rgxMat24.makeSyndromeTable: basis vector lacks odd-parity bit")
		}
		rb[i] = m.recipBasis[i] & 0x7ff
	}
	for i := 0; i < 24; i++ {
		bi := rb[i]
		m.syndromeTable[bi] ^= uint16(i) | c1
		for j := i + 1; j < 24; j++ {
			bj := bi ^ rb[j]
			m.syndromeTable[bj] ^= 0x8000
			for k := j + 1; k < 24; k++ {
				bk := bj ^ rb[k]
				m.syndromeTable[bk] ^= uint16(i) | uint16(j<<5) | uint16(k<<10)
			}
		}
	}
}

// cocodeSyndrome returns the minimum-weight cocode
// representative equivalent to c1 (weight at most
// four), as a bit vector. uTetrad selects a bit of
// a weight-four syndrome; 24 means don't care. C
// cocode_syndrome.
//
// cocodeSyndrome panics if a weight-four syndrome
// is requested with uTetrad==24 in the ambiguous
// case (matching the reference's ValueError); no
// xi caller triggers it.
func (m *rgxMat24) cocodeSyndrome(c1 uint32, uTetrad int) uint32 {
	if uTetrad < 0 || uTetrad > 24 {
		panic("rgxMat24.cocodeSyndrome: bad tetrad")
	}
	bad := (uTetrad >= 24) && ((c1>>11)+1)&1 != 0
	uTetrad -= (uTetrad + 8) >> 5 // 24 -> 23
	var y uint32
	if (c1>>11)&1 == 0 { // c1 is even
		y = ^uint32(0)
	}
	c1 ^= m.recipBasis[uTetrad] & y
	y &= 1 << uint(uTetrad)
	syn := uint32(m.syndromeTable[c1&0x7ff])
	synv := (uint32(1) << (syn & 31)) |
		(uint32(1) << ((syn >> 5) & 31)) |
		(uint32(1) << ((syn >> 10) & 31))
	// bit 24 set iff weight 1.
	if bad && synv&(y|0x1000000) == 0 {
		panic("rgxMat24.cocodeSyndrome: syndrome not unique")
	}
	synv ^= y
	return synv & 0xffffff
}

// gcodeWeight returns the bit weight of Golay code
// word v divided by 4. C gcode_weight.
func (m *rgxMat24) gcodeWeight(v uint32) uint32 {
	t := uint32(0) - ((v >> 11) & 1)
	w := (uint32(m.thetaTable[v&0x7ff]) >> 12) & 7
	return ((w & 7) ^ t) + (t & 7)
}

// scalarProd returns the scalar product (v,c) of a
// Golay code vector v (gcode rep) and a cocode
// vector c (cocode rep). C scalar_prod.
func (m *rgxMat24) scalarProd(v, c uint32) uint32 {
	r := v & c
	r ^= r >> 6
	r ^= r >> 3
	return (0x96 >> (r & 7)) & 1
}

// suboctadWeight returns the parity of half the
// bit weight of suboctad uSub. C suboctad_weight.
func rgxSuboctadWeight(uSub uint32) uint32 {
	w := rgxBw24(uSub & 0x3f)
	return ((uint32(w) + 1) >> 1) & 1
}

// ploopTheta returns theta(v) for the gcode word
// v, as a cocode word (low 12 bits). C
// ploop_theta is theta_table[v&0x7ff] & 0xfff,
// folding the high Golay half (v bit 11) onto the
// low half, which leaves theta unchanged since
// theta(Omega)=0.
func (m *rgxMat24) ploopTheta(v uint32) uint32 {
	return uint32(m.thetaTable[v&0x7ff]) & 0xfff
}

// thetaToBasisVector returns theta(v) for a Golay
// code basis vector v (vect rep) by Seysen Lemma
// 3.9. C theta_to_basis_vector.
func (m *rgxMat24) thetaToBasisVector(v uint32) uint32 {
	bw, col := rgxSplitGolayCodevector(v)
	if col != 0 && bw == 0 {
		return ((col >> 1) | (col >> 2) | (col >> 3)) & 0x111111
	}
	if col != 0 {
		panic("rgxMat24.thetaToBasisVector: colored part with nonzero bw")
	}
	bw &= 0x111111
	switch rgxBw24(bw) {
	case 2:
		return bw ^ 0x111111
	case 3:
		return 0x111111
	case 4:
		return bw
	default: // weight < 2 or > 4
		return 0
	}
}

// rgxSplitGolayCodevector splits a Golay code
// vector into its blackwhite and colored parts. C
// mat24aux.split_golay_codevector. It returns
// (blackwhite, colored) with their sum equal to v.
func rgxSplitGolayCodevector(v uint32) (uint32, uint32) {
	color := [8]uint32{0, 6, 5, 3, 3, 5, 6, 0}
	var col uint32
	for i := uint(1); i < 25; i += 4 {
		col |= color[(v>>i)&7] << i
	}
	v ^= col
	return v, col
}

// makeThetaTable fills thetaTable. C
// make_theta_table followed by augment_theta_table.
func (m *rgxMat24) makeThetaTable() {
	// theta on each code basis vector, in cocode rep.
	for i := 0; i < 11; i++ {
		bv := m.basis[12+i]
		m.thetaTable[1<<uint(i)] = uint16(m.vectToCocode(m.thetaToBasisVector(bv)))
	}
	// Extend to all Golay code numbers via the
	// associated bilinear form B(x,y) = x cap y.
	for i := uint32(0); i < 0x800; i++ {
		if i&(i-1) == 0 {
			continue // 0 or a power of two
		}
		i0 := uint32(1) << uint(rgxV2(i))
		i1 := i ^ i0
		capv := m.gcodeToVect(i0) & m.gcodeToVect(i1)
		cc := m.vectToCocode(capv)
		m.thetaTable[i] = m.thetaTable[i0] ^ m.thetaTable[i1] ^ uint16(cc)
	}
	// Augment with halved Golay weight in bits 14..12.
	for i := uint32(0); i < 0x800; i++ {
		w := rgxBw24(m.gcodeToVect(i))
		m.thetaTable[i] |= uint16((w >> 2) << 12)
	}
}

// rgxLinTable returns the linear span table of lst:
// out[0]=0, out[1<<i]=lst[i], out[i^j]=out[i]^out[j].
// C bitfunctions lin_table with t0=0.
func rgxLinTable(lst []uint32) []uint32 {
	out := make([]uint32, 1<<uint(len(lst)))
	for i, x := range lst {
		hi := 1 << uint(i)
		for j := 0; j < hi; j++ {
			out[hi+j] = x ^ out[j]
		}
	}
	return out
}

// rgxBits2List returns the ascending bit positions
// set in v (below 24). C bitfunctions bits2list.
func rgxBits2List(v uint32) []int {
	var l []int
	for i := 0; i < 24; i++ {
		if (v>>uint(i))&1 != 0 {
			l = append(l, i)
		}
	}
	return l
}

// rgxOddOctadsDict mirrors mat24tables.py
// ODD_OCTADS_DICT: reordering of the first MOG
// column of an octad whose low-nibble weight is
// odd, keyed by that nibble.
var rgxOddOctadsDict = map[uint32][3]int{
	7: {0, 1, 2}, 11: {1, 0, 2}, 13: {1, 2, 0}, 14: {2, 1, 0},
}

// rgxOctadToBitlist returns the ordered bit
// positions of an octad (a weight-8 vector),
// applying the ODD_OCTADS reordering. C
// octad_to_bitlist with ODD_OCTADS_SPECIAL == 1.
func rgxOctadToBitlist(vector uint32) []int {
	// ODD_OCTADS_SPECIAL == 1: reorder only when the
	// octad's low MOG nibble has odd weight.
	if rgxBw24(vector&15)&1 == 0 {
		return rgxBits2List(vector)
	}
	for i := uint(0); i < 24; i += 4 {
		v3 := vector & (15 << i)
		if seq, ok := rgxOddOctadsDict[v3>>i]; ok {
			first := rgxBits2List(v3)
			last := rgxBits2List(vector &^ v3)
			out := []int{first[seq[0]], first[seq[1]], first[seq[2]]}
			return append(out, last...)
		}
	}
	panic("rgxOctadToBitlist: cannot order odd octad")
}

// makeOctadTables fills octDecTable, octEncTable
// and octadTable from the 11-vector code basis. C
// make_octad_tables.
func (m *rgxMat24) makeOctadTables() {
	for i := range m.octEncTable {
		m.octEncTable[i] = 0xffff
	}
	codewords := rgxLinTable(m.basis[12 : 12+11])
	octad := 0
	for gcode := 0; gcode < 2048; gcode++ {
		vector := codewords[gcode]
		weight := rgxBw24(vector)
		if weight == 8 || weight == 16 {
			m.octDecTable[octad] = uint16(gcode) + uint16((weight&16)<<7)
			m.octEncTable[gcode] = uint16((weight-8)>>3) + uint16(2*octad)
			octVec := vector
			if weight == 16 {
				octVec = vector ^ 0xffffff
			}
			blist := rgxOctadToBitlist(octVec)
			for j := 0; j < 8; j++ {
				m.octadTable[8*octad+j] = uint8(blist[j])
			}
			octad++
		}
	}
	if octad != 759 {
		panic("rgxMat24.makeOctadTables: octad count != 759")
	}
}

// gcodeToOctad returns the octad number of a
// (possibly complemented) octad in gcode rep. C
// gcode_to_octad with u_strict=0 (no parity
// check). It panics if v is not an octad.
func (m *rgxMat24) gcodeToOctad(v uint32) uint32 {
	y := uint32(m.octEncTable[v&0x7ff])
	if y>>15 != 0 {
		panic("rgxMat24.gcodeToOctad: not an octad")
	}
	return y >> 1
}

// octadToGcode returns the gcode of octad uOctad
// (0<=uOctad<759). C octad_to_gcode.
func (m *rgxMat24) octadToGcode(uOctad uint32) uint32 {
	if uOctad >= 759 {
		panic("rgxMat24.octadToGcode: octad out of range")
	}
	return uint32(m.octDecTable[uOctad]) & 0xfff
}

// suboctadToCocode converts even suboctad uSub of
// the octad with number octad (0<=octad<759) to
// cocode rep. C mat24_inline_suboctad_to_cocode /
// Mat24.suboctad_to_cocode: the second argument is
// an octad number used directly as the octadTable
// index, not a gcode.
func (m *rgxMat24) suboctadToCocode(uSub, octad uint32) uint32 {
	parity := uint32(0x96>>((uSub^(uSub>>3))&7)) & 1
	sub := parity + ((uSub & 0x3f) << 1)
	o := int(octad) // octad number, indexes octadTable
	var vector uint32
	for i := 0; i < 8; i++ {
		if (1<<uint(i))&sub != 0 {
			vector |= 1 << uint(m.octadTable[8*o+i])
		}
	}
	return m.vectToCocode(vector)
}

// cocodeToSuboctad converts cocode element c1 to
// the suboctad number of octad v1 (gcode rep). C
// cocode_to_suboctad with u_strict=0. It panics if
// c1 is not an even subset of the octad v1.
func (m *rgxMat24) cocodeToSuboctad(c1, v1 uint32) uint32 {
	octad := m.gcodeToOctad(v1)
	o := int(octad)
	syn := m.cocodeSyndrome(c1, int(m.octadTable[8*o+0]))
	var v uint32
	for i := 0; i < 8; i++ {
		v |= 1 << uint(m.octadTable[8*o+i])
	}
	if c1&0x800 != 0 || syn&v != syn {
		panic("rgxMat24.cocodeToSuboctad: cocode word is not a suboctad")
	}
	var suboctad uint32
	for i := 0; i < 8; i++ {
		if (1<<uint(m.octadTable[8*o+i]))&syn != 0 {
			suboctad |= 1 << uint(i)
		}
	}
	if suboctad&0x80 != 0 {
		suboctad ^= 0xff
	}
	return (octad << 6) + (suboctad >> 1)
}

//////////////////////////////////////////////////
// GenXi reference operation (gen_xi_ref.py)
//////////////////////////////////////////////////

// rgxCompressGray packs the gray part of a Golay
// code or cocode word as a 6-bit number. C
// compress_gray.
func rgxCompressGray(x uint32) uint32 {
	return (x & 0x0f) + ((x >> 6) & 0x30)
}

// rgxExpandGray inverts rgxCompressGray. C
// expand_gray.
func rgxExpandGray(x uint32) uint32 {
	return (x & 0x0f) + ((x & 0x30) << 6)
}

// rgxGenXi holds the two gray lookup tables of the
// reference xi operation, plus the Mat24
// primitives it rests on. C class GenXi.
type rgxGenXi struct {
	m       *rgxMat24
	gGray   [64]uint8 // C GenXi.tab_g_gray
	gCocode [64]uint8 // C GenXi.tab_g_cocode
}

// newXiGenXi builds the gray tables from the
// gamma/w2 functions. C GenXi class-body loop.
func newXiGenXi(m *rgxMat24) *rgxGenXi {
	g := &rgxGenXi{m: m}
	for x := uint32(0); x < 64; x++ {
		w2, c := g.w2Gamma(rgxExpandGray(x))
		cx := rgxCompressGray(c)
		w2x := w2 << 7
		g.gGray[x] = uint8(w2x + cx)
		g.gCocode[cx] = uint8(w2x + x)
	}
	return g
}

// w2Gamma implements gamma() (Seysen sec. 3.3) for
// a Golay code vector v in gcode rep. It returns
// (w2, c): c is gamma(v) in cocode rep, w2=w2(c). C
// w2_gamma.
func (g *rgxGenXi) w2Gamma(v uint32) (uint32, uint32) {
	x := g.m.gcodeToVect(v)
	var x1 uint32
	for i := uint(1); i < 4; i++ {
		x1 += (x >> i) & 0x111111
	}
	x1 = (x1 >> 1) & 0x111111
	w2 := uint32(rgxBw24(x1))
	w2 = ((w2 * (w2 - 1)) >> 1) & 1
	c := g.m.vectToCocode(x1)
	return w2, c
}

// opXi returns xi**exp x xi**(-exp) acting on x in
// Q_x0 (Leech lattice encoding). C
// GenXi.gen_xi_op_xi.
func (g *rgxGenXi) opXi(x uint32, exp int) uint32 {
	exp = ((exp % 3) + 3) % 3
	if exp == 0 {
		return x
	}
	scal := uint32(rgxBw24((x>>12)&x&0xc0f)) & 1
	x ^= scal << 24 // xor scalar product to sign

	tv := uint32(g.gGray[rgxCompressGray(x>>12)])
	w2v, gv := tv>>7, rgxExpandGray(tv)
	tc := uint32(g.gCocode[rgxCompressGray(x)])
	w2c, gc := tc>>7, rgxExpandGray(tc)
	if exp == 1 {
		x &^= 0xc0f000 // kill gray code part
		x ^= w2c << 24 // xor w2(cocode) to sign
	} else {
		x &^= 0xc0f    // kill gray cocode part
		x ^= w2v << 24 // xor w2(code) to sign
	}
	x ^= gv       // xor g(code) to cocode
	x ^= gc << 12 // xor g(cocode) to code
	return x
}

// shortToLeech converts x1 from short vector to
// Leech lattice encoding, or 0 if invalid. C
// GenXi.gen_xi_short_to_leech.
func (g *rgxGenXi) shortToLeech(x1 uint32) uint32 {
	m := g.m
	box := x1 >> 16
	sign := (x1 >> 15) & 1
	code := x1 & 0x7fff
	octad := uint32(0xffff)
	var gcode, cocode uint32
	switch {
	case box == 1:
		switch {
		case code < 1536: // 2 * 24 * 32
			var gc uint32
			if code >= 768 {
				gc = 1
				code -= 768
			}
			gcode = gc << 11
			i := code >> 5
			j := code & 31
			cocode = m.vectToCocode((1 << i) ^ (1 << j))
			if cocode == 0 || cocode&0x800 != 0 {
				return 0
			}
		case code < 2496: // 2 * 24 * 32 + 15 * 64
			octad = code - 1536
		default:
			return 0
		}
	case box == 2:
		if code >= 23040 { // 360 * 64
			return 0
		}
		octad = code + 960 // 15 * 64
	case box == 3:
		if code >= 24576 { // 384 * 64
			return 0
		}
		octad = code + 24000 // (15 + 360) * 64
	case box < 6:
		code += (box - 4) << 15
		cocode = m.vectToCocode(1 << (x1 & 31))
		if cocode == 0 {
			return 0
		}
		gcode = (code >> 5) & 0x7ff
		w := m.gcodeWeight(gcode) ^ m.scalarProd(gcode, cocode)
		gcode ^= (w & 1) << 11
	default:
		return 0
	}
	// 759 * 64 == 48576. gen_xi_ref.py writes 48756
	// (a digit-swap typo), but every reachable octad
	// value is <= 48575, so both bounds agree; the C
	// gen_xi_functions.c uses the correct 48576.
	if octad < 48576 {
		cc := octad & 0x3f
		w := rgxSuboctadWeight(cc)
		gcode = m.octadToGcode(octad >> 6)
		cocode = m.suboctadToCocode(cc, octad>>6)
		gcode ^= w << 11
	}
	cocode ^= m.ploopTheta(gcode)
	return (sign << 24) | (gcode << 12) | cocode
}

// leechToShort converts x1 from Leech lattice to
// short vector encoding, or 0 if x1 is not short. C
// GenXi.gen_xi_leech_to_short.
func (g *rgxGenXi) leechToShort(x1 uint32) uint32 {
	m := g.m
	sign := (x1 >> 24) & 1
	x1 ^= m.ploopTheta(x1 >> 12)
	gcodev := m.gcodeToVect(x1 >> 12)
	tetrad := rgxLsbit24(gcodev)
	if tetrad > 23 {
		tetrad = 23
	}
	cocodev := m.cocodeSyndrome(x1, tetrad)
	w := m.gcodeWeight(x1 >> 12)
	var box, code uint32
	if x1&0x800 != 0 {
		if rgxBw24(cocodev) > 1 || m.scalarProd(x1>>12, x1) != (w&1) {
			return 0
		}
		y := uint32(rgxLsbit24(cocodev))
		code = ((x1 & 0x7ff000) >> 7) | y
		box = 4 + (code >> 15)
		code &= 0x7fff
	} else {
		switch w {
		case 3:
			return 0
		case 2, 4:
			code = m.cocodeToSuboctad(x1, x1>>12)
			switch {
			case code >= 24000: // (15 + 360) * 64
				code -= 24000
				box = 3
			case code >= 960: // 15 * 64
				code -= 960
				box = 2
			default:
				code += 1536
				box = 1
			}
		default:
			y1 := uint32(rgxLsbit24(cocodev))
			cocodev ^= 1 << y1
			y2 := uint32(rgxLsbit24(cocodev))
			if cocodev != (1<<y2) || y1 >= 24 {
				return 0
			}
			code = 384*(w&2) + 32*y2 + y1
			box = 1
		}
	}
	return (box << 16) | (sign << 15) | code
}

// opXiShort returns xi**exp x xi**(-exp) acting on
// x in short vector encoding. An invalid x is
// returned unchanged. C
// GenXi.gen_xi_op_xi_short.
func (g *rgxGenXi) opXiShort(x uint32, exp int) uint32 {
	y := g.shortToLeech(x)
	if y == 0 {
		return x
	}
	y = g.opXi(y, exp)
	if y == 0 {
		return x
	}
	y = g.leechToShort(y)
	if y == 0 {
		return x
	}
	return y
}

//////////////////////////////////////////////////
// Table pipeline (mm_tables_xi.py)
//////////////////////////////////////////////////

// rgxTSize is GenXi.make_table's t_size: the number
// of live entries per box (index 1..5).
var rgxTSize = [6]int{0, 2496, 23040, 24576, 32768, 32768}

// makeTable returns the low 16 bits of the image
// of every entry of box uBox under xi**uExp. C
// GenXi.make_table.
func (g *rgxGenXi) makeTable(uBox, uExp int) []uint16 {
	length := rgxTSize[uBox]
	a := make([]uint16, length)
	base := uint32(uBox) << 16
	for i := 0; i < length; i++ {
		a[i] = uint16(g.opXiShort(base+uint32(i), uExp) & 0xffff)
	}
	return a
}

// rgxInvertTable inverts a permutation table. For
// each source index i with column (i&31) below
// nColumns whose image r&0x7fff is below
// lenResult, result[r&0x7fff] receives i with the
// sign bit r&0x8000 carried over. C
// GenXi.invert_table.
func rgxInvertTable(table []uint16, nColumns, lenResult int) []uint16 {
	if len(table)&31 != 0 || lenResult&31 != 0 {
		panic("rgxInvertTable: lengths must be multiples of 32")
	}
	result := make([]uint16, lenResult)
	for i, r := range table {
		if (i&31) < nColumns && int(r&0x7fff) < lenResult {
			result[r&0x7fff] = uint16(i) | (r & 0x8000)
		}
	}
	return result
}

// rgxMakeTableBcSymmetric symmetrises the inverted
// BC table in place across its B and C 24x24
// blocks (rows of 32). C make_table_bc_symmetric.
func rgxMakeTableBcSymmetric(table []uint16) {
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

// rgxSplitTable splits a table into a permutation
// table (entries reduced mod modulus) and a sign
// table (one uint32 of 32 packed sign bits per 32
// entries). C GenXi.split_table.
func rgxSplitTable(table []uint16, modulus int) ([]uint16, []uint32) {
	length := len(table)
	if length&31 != 0 {
		panic("rgxSplitTable: length must be a multiple of 32")
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

// rgxCut24 keeps the first 24 of every 32 entries.
// C cut24.
func rgxCut24(table []uint16) []uint16 {
	out := make([]uint16, 0, len(table)/32*24)
	for i := 0; i < len(table); i += 32 {
		out = append(out, table[i:i+24]...)
	}
	return out
}

// rgxBoxShape is one (rows, columns, row_length)
// box shape. C Pre_MM_TablesXi.BOX_SHAPES entries.
type rgxBoxShape struct {
	rows, cols, rowLen int
}

// xi box-shape constants. C BOX_SHAPES.
var (
	rgxShapeBC = rgxBoxShape{1, 78, 32}
	rgxShapeT0 = rgxBoxShape{45, 16, 32}
	rgxShapeT1 = rgxBoxShape{64, 12, 32}
	rgxShapeX0 = rgxBoxShape{64, 16, 24}
	rgxShapeX1 = rgxBoxShape{64, 16, 24}
)

// box tag identifiers, matching the numeric ids
// used by Pre_MM_TablesXi (BC=1..X1=5).
const (
	rgxBoxBC = 1
	rgxBoxT0 = 2
	rgxBoxT1 = 3
	rgxBoxX0 = 4
	rgxBoxX1 = 5
)

// rgxBoxShapeOf returns the shape of a box id.
func rgxBoxShapeOf(box int) rgxBoxShape {
	switch box {
	case rgxBoxBC:
		return rgxShapeBC
	case rgxBoxT0:
		return rgxShapeT0
	case rgxBoxT1:
		return rgxShapeT1
	case rgxBoxX0:
		return rgxShapeX0
	case rgxBoxX1:
		return rgxShapeX1
	}
	panic("rgxBoxShapeOf: bad box id")
}

// rgxMapXi is Pre_MM_TablesXi.MAP_XI: for each of
// the five table groups n and each exponent
// exp1 in {0,1}, the [source, destination] box
// pair. xi permutes the boxes 1->1, 2->2,
// 3->4->5->3.
var rgxMapXi = [5][2][2]int{
	{{rgxBoxBC, rgxBoxBC}, {rgxBoxBC, rgxBoxBC}},
	{{rgxBoxT0, rgxBoxT0}, {rgxBoxT0, rgxBoxT0}},
	{{rgxBoxT1, rgxBoxX0}, {rgxBoxT1, rgxBoxX1}},
	{{rgxBoxX0, rgxBoxX1}, {rgxBoxX1, rgxBoxX0}},
	{{rgxBoxX1, rgxBoxT1}, {rgxBoxX0, rgxBoxT1}},
}

// rgxBuiltTable is one fully derived (perm, sign)
// pair for table group n and exponent exp1.
type rgxBuiltTable struct {
	n, exp1 int
	perm    []uint16
	sign    []uint32
}

// rgxBuildTables runs the full Pre_MM_TablesXi
// pipeline and returns all ten (perm, sign) pairs,
// in (n, exp1) order. The xi box-permutation
// assumption is checked for every group.
func (g *rgxGenXi) rgxBuildTables() []rgxBuiltTable {
	// Sanity: xi must permute the boxes exactly as
	// MAP_XI claims. C Pre_MM_TablesXi __init__
	// asserts.
	for n := 0; n < 5; n++ {
		for exp1 := 0; exp1 < 2; exp1++ {
			box := rgxMapXi[n][exp1][0]
			wantImg := rgxMapXi[n][exp1][1]
			img := int(g.opXiShort(uint32(box)<<16, exp1+1) >> 16)
			if img != wantImg {
				panic(fmt.Sprintf("rgxBuildTables: box %d under xi**%d -> %d, want %d",
					box, exp1+1, img, wantImg))
			}
		}
	}

	var built []rgxBuiltTable
	for n := 0; n < 5; n++ {
		for exp1 := 0; exp1 < 2; exp1++ {
			box := rgxMapXi[n][exp1][0]
			img := rgxMapXi[n][exp1][1]
			shape := rgxBoxShapeOf(box)
			imgShape := rgxBoxShapeOf(img)

			table := g.makeTable(box, exp1+1)
			if shape.rows != imgShape.rows {
				panic("rgxBuildTables: source/image row mismatch")
			}
			if len(table) != shape.rows*shape.cols*32 {
				panic("rgxBuildTables: table length mismatch")
			}
			imgLen := imgShape.rows * imgShape.cols * 32

			invTable := rgxInvertTable(table, shape.rowLen, imgLen)
			if box == rgxBoxBC {
				rgxMakeTableBcSymmetric(invTable)
			}
			perm, sign := rgxSplitTable(invTable, shape.cols*32)
			if imgShape.rowLen == 24 {
				perm = rgxCut24(perm)
			}
			built = append(built, rgxBuiltTable{
				n: n, exp1: exp1, perm: perm, sign: sign,
			})
		}
	}
	return built
}

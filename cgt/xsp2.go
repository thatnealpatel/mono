package cgt

// Operations on the subgroup G_{x0} (of
// structure 2^{1+24}.Co_1) of the monster,
// ported from the mmgroup C files xsp2co1.c,
// xsp2co1_elem.c, xsp2co1_word.c, xsp2co1_map.c,
// xsp2co1_traces.c and involutions.c, plus the
// gen_leech_reduce.c and mm_group_n.c helpers
// they depend on.
//
// An element g of G_{x0} is stored in G_x0
// representation as a pair (x_g, v_g) in
// G(4096_x) x Lambda/3Lambda. The array data has
// 26 entries: data[0] is v_g in Leech-mod-3
// encoding; data[1..25] hold the inverse of x_g
// as a qstate12 of shape (12,12) (25 = 1 + 12 +
// 12 rows of bit matrix). The components are
// reduced automatically by constructors.

// XspAtom is a generator atom for the constructor
// of Xsp2Co1: a one-letter Tag (d, p, x, y, l)
// and an integer I.
type XspAtom struct {
	Tag string
	I   int
}

// xsp2Co1Words is the number of uint64 words in
// the G_x0 representation of an element.
const xsp2Co1Words = 26

// Xsp2Co1 is an element of the subgroup G_{x0}
// (of structure 2^{1+24}.Co_1) of the monster,
// stored in G_x0 representation.
type Xsp2Co1 struct {
	data [xsp2Co1Words]uint64
}

// errNotInGx0 reports that a word of generators
// leaves the subgroup G_{x0}.
var errNotInGx0 = newXspError("cgt: element not in subgroup G_x0")

func newXspError(s string) error { return &xspError{s} }

type xspError struct{ s string }

func (e *xspError) Error() string { return e.s }

const (
	// xspStdV3 is the standard short Leech vector
	// mod 3, STD_V3 = (0,0,1,-1,0,...).
	xspStdV3    = 0x8000004
	xspStdV3Neg = 0x4000008
	// xspMaxRowsElem is the qstate buffer capacity
	// for an element of G_{x0} (C MAXROWS_ELEM).
	xspMaxRowsElem = 30
)

// atom tag encodings (mmgroup_generators.h).
const (
	atomTag1  = 0x00000000
	atomTagI1 = 0x80000000
	atomTagD  = 0x10000000
	atomTagID = 0x90000000
	atomTagP  = 0x20000000
	atomTagIP = 0xA0000000
	atomTagX  = 0x30000000
	atomTagIX = 0xB0000000
	atomTagY  = 0x40000000
	atomTagIY = 0xC0000000
	atomTagT  = 0x50000000
	atomTagIT = 0xD0000000
	atomTagL  = 0x60000000
	atomTagIL = 0xE0000000
)

/*************************************************************************
*** Public type and constructors
*************************************************************************/

// NewXsp2Co1 builds the element of G_{x0} given
// by the word of generator atoms. Each XspAtom
// has a one-letter tag (d, p, x, y, l) and an
// integer. NewXsp2Co1 panics if any atom is not
// in G_{x0}.
func NewXsp2Co1(atoms ...XspAtom) *Xsp2Co1 {
	a := atomsToWord(atoms)
	g := &Xsp2Co1{}
	if err := xsp2co1SetElemWord(g.data[:], a); err != nil {
		panic(err.Error())
	}
	return g
}

// Xsp2Co1Identity returns the neutral element of
// G_{x0}.
func Xsp2Co1Identity() *Xsp2Co1 {
	g := &Xsp2Co1{}
	xsp2co1UnitElem(g.data[:])
	return g
}

// Xsp2FromXsp returns the element of Q_{x0} given
// by x in Leech lattice encoding.
func Xsp2FromXsp(x uint32) *Xsp2Co1 {
	g := &Xsp2Co1{}
	if err := xsp2co1ElemXspecial(g.data[:], x); err != nil {
		panic(err.Error())
	}
	return g
}

// atomsToWord converts XspAtoms to a word of
// generator encodings. It panics on an unknown
// tag.
func atomsToWord(atoms []XspAtom) []uint32 {
	a := make([]uint32, 0, len(atoms))
	for _, at := range atoms {
		var tag uint32
		switch at.Tag {
		case "d":
			tag = atomTagD
		case "p":
			tag = atomTagP
		case "x":
			tag = atomTagX
		case "y":
			tag = atomTagY
		case "t":
			tag = atomTagT
		case "l":
			tag = atomTagL
		default:
			panic("cgt: unknown XspAtom tag " + at.Tag)
		}
		a = append(a, tag|(uint32(at.I)&atomData))
	}
	return a
}

/*************************************************************************
*** Conversion between basis (d)' and standard basis of 4096_x
*************************************************************************/

// convPauliVectorXspecial performs the basis
// conversion of conv_pauli_vector_xspecial on an
// element x of Q_{x0} in Leech encoding.
func convPauliVectorXspecial(x uint32) uint32 {
	x &= 0x1ffffff
	t := (x ^ (x >> 12)) & 0x800
	x ^= (t << 12) ^ t
	t = x & (x >> 12) & 0x7ff
	t = Parity12(t)
	x ^= t << 24
	return x
}

// convPauliVectorXspecialNoSign is like
// convPauliVectorXspecial but ignores the sign.
func convPauliVectorXspecialNoSign(x uint32) uint32 {
	t := (x ^ (x >> 12)) & 0x800
	x ^= (t << 12) ^ t
	return x
}

// convConjugateBasis applies a Hadamard gate to
// the sign qubit, converting between the standard
// basis and the basis (d)' of 4096_x.
func convConjugateBasis(s *qs12) {
	qsGateH(s, 0x800800)
}

/*************************************************************************
*** Short-vector arithmetic in the Leech lattice mod 3
*************************************************************************/

// short3Scalprod returns the scalar product of
// two Leech-mod-3 vectors (0, 1 or 2). C
// short_3_scalprod.
func short3Scalprod(v31, v32 uint64) uint32 {
	// zero has bit i clear where the product of
	// entries i is 0 (one operand entry is 0).
	zero := ((v31 ^ (v31 >> 24)) & (v32 ^ (v32 >> 24))) & 0xffffff
	// res holds the 24 per-entry products, each in
	// {0,1,2} as a 2-bit field; the high 24 bits are
	// counted twice by the fold below.
	res := (v31 ^ v32) & 0xffffff000000
	res = (res & (zero << 24)) | (zero &^ (res >> 24))
	// Horizontal popcount fold of the 48 bits. After
	// the four steps res holds the byte-wise sums; the
	// final two lines collapse to one value with
	// 0 <= res <= 48 (24 entries times 2, high half
	// counted twice).
	res = (res & 0x555555555555) + ((res >> 1) & 0x555555555555)
	res = (res & 0x333333333333) + ((res >> 2) & 0x333333333333)
	res = (res & 0x0f0f0f0f0f0f) + ((res >> 4) & 0x0f0f0f0f0f0f)
	res = (res & 0xffffff) + ((res >> 23) & 0x1fffffe)
	res = ((res >> 16) + (res >> 8) + res) & 0xff
	// Reduce 0 <= res <= 48 mod 3. After this step
	// 0 <= res <= 19 (= 16 + 3), so res << 1 <= 38 and
	// the table shift below stays within 64 bits.
	res = (res & 3) + (res >> 2)
	// 0x9249...924 is the 2-bit-per-slot table of
	// n mod 3 for n in 0..31; index it by 2*res.
	res = (0x924924924924924 >> (res << 1)) & 3
	return uint32(res)
}

// compute3Sum adds two reduced Leech-mod-3
// vectors and returns the reduced sum.
func compute3Sum(v31, v32 uint64) uint64 {
	a1 := v31 ^ v32
	a2 := v31 & v32 & 0xffffffffffff
	a2 = ((a2 << 24) | (a2 >> 24)) & 0xffffffffffff
	v31 = a1 ^ a2
	v32 = a1 & a2 & 0xffffffffffff
	v32 = ((v32 << 24) | (v32 >> 24)) & 0xffffffffffff
	return v31 | v32
}

// genLeech3Neg negates a Leech-mod-3 vector.
func genLeech3Neg(v3 uint64) uint64 {
	return short3Reduce(((v3 & 0xffffff) << 24) | ((v3 >> 24) & 0xffffff))
}

/*************************************************************************
*** qstate helpers not provided by qstate.go
*************************************************************************/

// qsMonomialColumnMatrix sets s to a monomial
// column matrix of nqb qubits defined by pa, as
// qstate12_monomial_column_matrix.
func qsMonomialColumnMatrix(s *qs12, nqb int, pa []uint64) {
	factor := int64(((pa[0] >> uint(nqb)) & 1) << 2)
	s.nrows = nqb + 1
	s.ncols = nqb << 1
	if s.nrows+s.ncols > qsMaxCols || s.nrows > qsMaxRows {
		panic("cgt: quadratic state too large")
	}
	s.grow(s.nrows)
	m := s.data
	mask1 := (uint64(1) << uint(nqb)) - 1
	m[0] = (pa[0] & mask1) << uint(nqb)
	for i := 1; i <= nqb; i++ {
		mask1 += mask1 + 1
		m[i] = (uint64(1) << uint(i-1)) | ((pa[i] & mask1) << uint(nqb))
	}
	qsSet1(s, 2*nqb, nqb+1)
	s.shape1 = nqb
	s.factor = factor
}

// qsSet1 applies qstate12_set with mode 1 to the
// first nrows rows of s, treating its ncols-bit A
// part and the trailing Q part.
func qsSet1(s *qs12, nqb, nrows int) {
	m := s.data
	mask := ((uint64(1) << uint(nqb)) << uint(nrows)) - 1
	for i := 0; i < nrows; i++ {
		m[i] &= mask
	}
	mask = uint64(1) << uint(nqb)
	m[0] &= mask - 1
	for i := 1; i < nrows; i++ {
		m[i] &= (mask << uint(i+1)) - 1
	}
	for i := 0; i < nrows; i++ {
		for j := i + 1; j < nrows; j++ {
			m[i] ^= ((m[j] >> uint(i)) & mask) << uint(j)
		}
	}
	s.nrows = nrows
	s.ncols = nqb
	s.factor = 0
	s.shape1 = 0
	s.reduced = false
}

// qsMonomialMatrixRowOp obtains the affine
// operation of a monomial state on its unit
// vector labels, as qstate12_monomial_matrix_row_op.
// It returns the number r+1 of rows, or a negative
// value if s is not monomial.
func qsMonomialMatrixRowOp(s *qs12, pa []uint32) int {
	qsReduce(s)
	cols := s.shape1
	rows := s.ncols - cols
	if s.nrows != rows+1 {
		return -1
	}
	m := s.data
	rowMask := ((uint64(1) << uint(rows)) - 1) << uint(cols)
	colMask := (uint64(1) << uint(cols)) - 1
	dataMask := uint64(1) << uint(rows+cols)
	var err uint64
	pa[0] = uint32(m[0] & colMask)
	for i := 1; i <= rows; i++ {
		dataMask >>= 1
		err |= (m[i] ^ dataMask) & rowMask
		pa[rows+1-i] = uint32(m[i] & colMask)
	}
	if err != 0 {
		return -1
	}
	return rows + 1
}

// qsMatTraceFactor reduces s, requires it to be
// square, and returns the trace as an encoded
// factor (qstate12_mat_trace_factor).
func qsMatTraceFactor(s *qs12) int64 {
	nrows := s.shape1
	qsReduce(s)
	if 2*nrows != s.ncols {
		panic("cgt: trace of non-square matrix")
	}
	q := s.copy()
	for i := 0; i < nrows; i++ {
		qsGateCtrlNot(q, uint64(1)<<uint(i), uint64(1)<<uint(nrows+i))
	}
	qsRestrict(q, nrows, nrows)
	qsSumCols(q, 0, nrows)
	qsReduce(q)
	if q.ncols != 0 {
		panic("cgt: trace internal error")
	}
	if q.nrows != 0 {
		return int64(uint64(q.factor) & factorMask)
	}
	return 8
}

// qsMatItrace returns the integer trace of the
// square state s (qstate12_mat_itrace).
func qsMatItrace(s *qs12) int64 {
	return factorToInt32(qsMatTraceFactor(s))
}

// qsToSymplecticRow returns row n of the
// symplectic bit matrix of the invertible square
// state s (qstate12_to_symplectic_row). It panics
// if s is not an invertible (k,k) matrix or n is
// out of range.
func qsToSymplecticRow(s *qs12, n int) uint32 {
	qsReduce(s)
	k := s.shape1
	if 2*k != s.ncols || s.nrows <= k {
		panic("cgt: matrix is not invertible")
	}
	dRows := s.nrows - 1
	if k > qsMaxCols/3 || dRows > 2*qsMaxCols/3 {
		panic("cgt: quadratic state too large")
	}
	if k == 0 {
		return 0
	}
	m := make([]uint64, 2*qsMaxCols/3+1)
	var v uint64
	mask := uint64(1) << uint(2*k-1)
	for j := 0; j < dRows; j++ {
		m[j] = s.data[j+1]
		v |= m[j] ^ (mask >> uint(j))
	}
	mask = (uint64(1) << uint(k)) - 1
	if (v>>uint(k))&mask != 0 {
		panic("cgt: matrix is not invertible")
	}
	Bm64XchBits(m, dRows, 2*k+1, (1<<uint(k))-1)
	res := Bm64EchelonL(m, dRows, 2*k+1, dRows)
	if res != dRows {
		panic("cgt: matrix is not invertible")
	}
	var a uint64
	if n < k {
		pd := s.data[1:]
		for j := 0; j < k; j++ {
			a ^= ((pd[j] >> uint(n)) & 1) << uint(j)
		}
		for j := k; j < dRows; j++ {
			mask = 0 - ((pd[j] >> uint(n)) & 1)
			a ^= m[j] & mask
		}
	} else if n < k+k {
		a = m[n-k]
	} else {
		panic("cgt: qubit index out of range")
	}
	a &= (uint64(1) << uint(2*k)) - 1
	one := []uint64{a}
	Bm64ReverseBits(one, 1, k, 0)
	return uint32(one[0])
}

/*************************************************************************
*** Conversion between G_x0 representation and qstate12
*************************************************************************/

// xsp2co1ElemToQsI loads x_g^{-1} from an element
// of G_{x0} into a fresh qstate (the inverse of
// component x_g, of shape (12,12)).
func xsp2co1ElemToQsI(elem []uint64) *qs12 {
	s := &qs12{
		nrows:  25,
		ncols:  24,
		shape1: 12,
		factor: -12 * 16,
	}
	s.data = make([]uint64, 25, qsMaxRows)
	copy(s.data, elem[1:26])
	for s.nrows > 1 && s.data[s.nrows-1] == 0 {
		s.nrows--
		s.factor += 16
	}
	return s
}

// xsp2co1QsToElemINoreduce stores the (unreduced)
// element with x_g^{-1} given by s and Leech-mod-3
// vector v3 into elem.
func xsp2co1QsToElemINoreduce(s *qs12, v3 uint64, elem []uint64) {
	if s.nrows > 25 {
		panic("cgt: quadratic state buffer overflow")
	}
	for i := 0; i < s.nrows; i++ {
		elem[i+1] = s.data[i] & 0x3fffffeffffff
	}
	for i := s.nrows; i < 25; i++ {
		elem[i+1] = 0
	}
	if s.factor&4 != 0 {
		v3 ^= 0xffffffffffff
	}
	elem[0] = short3Reduce(v3)
}

// xsp2co1QsToElemI reduces and checks s, then
// stores the element with x_g^{-1} = s and
// Leech-mod-3 vector vg into elem.
func xsp2co1QsToElemI(s *qs12, vg uint64, elem []uint64) {
	qsReduce(s)
	qsCheck(s)
	xsp2co1QsToElemINoreduce(s, vg, elem)
}

// xsp2co1ReduceElem reduces the components of an
// element of G_{x0} in place.
func xsp2co1ReduceElem(elem []uint64) {
	s := xsp2co1ElemToQsI(elem)
	xsp2co1QsToElemI(s, elem[0], elem)
}

/*************************************************************************
*** Elementary operations on elements of G_{x0}
*************************************************************************/

// xsp2co1NegElem negates an element of G_{x0} in
// place (multiplication by x_{-1}).
func xsp2co1NegElem(elem []uint64) {
	elem[0] = short3Reduce(^elem[0] & 0xffffffffffff)
}

// xsp2co1CopyElem copies a G_{x0} element.
func xsp2co1CopyElem(src, dst []uint64) {
	copy(dst[:26], src[:26])
}

// xsp2co1UnitElem stores the neutral element of
// G_{x0} into elem.
func xsp2co1UnitElem(elem []uint64) {
	mask := uint64(0x800800)
	elem[0] = xspStdV3
	elem[1] = 0
	for i := 2; i < 14; i++ {
		elem[i] = mask
		mask >>= 1
	}
	for i := 14; i < 26; i++ {
		elem[i] = 0
	}
}

// xsp2co1IsUnitElem reports whether elem is the
// neutral element of G_{x0}.
func xsp2co1IsUnitElem(elem []uint64) bool {
	mask := uint64(0x800800)
	acc := elem[0] ^ xspStdV3
	acc |= elem[1]
	for i := 2; i < 14; i++ {
		acc |= elem[i] ^ mask
		mask >>= 1
	}
	for i := 14; i < 26; i++ {
		acc |= elem[i]
	}
	return acc == 0
}

// xsp2co1ElemToBitmatrix maps an element of
// G_{x0} to the 24x24 bit matrix acting on the
// Leech lattice mod 2 by right multiplication.
func xsp2co1ElemToBitmatrix(elem, pA []uint64) {
	s := xsp2co1ElemToQsI(elem)
	bm := qsToSymplectic(s)
	copy(pA[:len(bm)], bm)
	for i := 0; i < 24; i++ {
		w := (pA[i] ^ (pA[i] >> 12)) & 0x800
		pA[i] ^= (w << 12) ^ w
	}
	pA[11], pA[23] = pA[23], pA[11]
}

/*************************************************************************
*** Chains of short vectors in the Leech lattice mod 3
*************************************************************************/

// xsp2co1FindChainShort3 returns a short vector
// not orthogonal to both v3_1 and v3_2, or 0 if
// none exists.
func xsp2co1FindChainShort3(v31, v32 uint64) uint64 {
	v31 = short3Reduce(v31)
	v32 = short3Reduce(v32)
	support1 := uint32((v31 | (v31 >> 24)) & 0xffffff)
	support2 := uint32((v32 | (v32 >> 24)) & 0xffffff)
	if support1 & ^support2 != 0 {
		c1 := lsbit24(support1 & ^support2)
		c2 := lsbit24(support2)
		if c2 >= 24 {
			return 0
		}
		mask := (uint64(1) << c1) ^ (uint64(1) << c2)
		mask = v31 & (mask | (mask << 24))
		if mask&(mask-1) == 0 {
			mask |= uint64(1) << c2
		}
		return mask
	}
	if support2 & ^support1 != 0 {
		c2 := lsbit24(support2 & ^support1)
		c1 := lsbit24(support1)
		if c1 >= 24 {
			return 0
		}
		mask := (uint64(1) << c1) ^ (uint64(1) << c2)
		mask = v32 & (mask | (mask << 24))
		if mask&(mask-1) == 0 {
			mask |= uint64(1) << c1
		}
		return mask
	}
	if support2 != 0 {
		c1 := lsbit24(support1 & support2)
		c2 := lsbit24(^support1 & ^support2 & 0xffffff)
		if c2 < 24 {
			return (uint64(1) << c1) ^ (uint64(1) << c2)
		}
		mask := (v31 ^ v32) & 0xffffff
		if mask&(mask-1) == 0 {
			mask ^= 0xfffffff
		}
		c1 = lsbit24(uint32(mask))
		mask ^= uint64(1) << c1
		c2 = lsbit24(uint32(mask))
		mask = (uint64(1) << c1) ^ (uint64(1) << c2)
		return v31 & (mask | (mask << 24))
	}
	return 0
}

// xsp2co1ChainShort3 computes images of a chain
// of short Leech-mod-3 vectors under the element
// represented by s, given the image of the first.
// It panics if adjacent vectors are orthogonal or
// not short (the C error path).
func xsp2co1ChainShort3(s *qs12, n int, psrc, pdest []uint64) {
	if s.ncols != 24 || s.shape1 != 12 {
		panic("cgt: bad quadratic state shape for chain")
	}
	if n <= 1 {
		return
	}
	bm := qsToSymplectic(s)
	ok := true
	for i := 1; i < n; i++ {
		v := uint64(Leech3To2Short(psrc[i]))
		v = uint64(convPauliVectorXspecialNoSign(uint32(v)))
		var w uint64
		for j := 0; j < 24; j++ {
			w ^= bm[j] & (0 - ((v >> uint(j)) & 1))
		}
		w = uint64(convPauliVectorXspecialNoSign(uint32(w)))
		w = Leech2To3Short(uint32(w))
		srcProd := short3Scalprod(psrc[i-1], psrc[i])
		prod := short3Scalprod(pdest[i-1], w)
		if prod != srcProd {
			w = short3Reduce(^w & 0xffffffffffff)
		}
		pdest[i] = w
		ok = ok && srcProd != 0 && prod != 0
	}
	if !ok {
		panic("cgt: Leech mod 3 chain operation failed")
	}
}

// xsp2co1MapShort3 returns the image of src2
// under the element represented by s, given that
// s maps src1 to dest1.
func xsp2co1MapShort3(s *qs12, src1, dest1, src2 uint64) uint64 {
	var asrc, adest [3]uint64
	asrc[0] = src1
	adest[0] = dest1
	asrc[2] = src2
	asrc[1] = xsp2co1FindChainShort3(asrc[0], asrc[2])
	xsp2co1ChainShort3(s, 3, asrc[:], adest[:])
	return adest[2]
}

/*************************************************************************
*** Multiplication, inversion and conjugation in G_{x0}
*************************************************************************/

// xsp2co1MulElem computes elem1 * elem2 into
// elem3 (any overlap allowed).
func xsp2co1MulElem(elem1, elem2, elem3 []uint64) {
	qs1 := xsp2co1ElemToQsI(elem1)
	qs2 := xsp2co1ElemToQsI(elem2)
	qs3 := qsMatmul(qs2, qs1)
	v := xsp2co1MapShort3(qs2, xspStdV3, elem2[0], elem1[0])
	xsp2co1QsToElemINoreduce(qs3, v, elem3)
}

// xsp2co1InvElem computes elem1^{-1} into elem2.
func xsp2co1InvElem(elem1, elem2 []uint64) {
	qs1 := xsp2co1ElemToQsI(elem1)
	qs2 := qs1.copy()
	qsMatInv(qs2)
	v := xsp2co1MapShort3(qs2, elem1[0], xspStdV3, xspStdV3)
	xsp2co1QsToElemINoreduce(qs2, v, elem2)
}

// xsp2co1ConjElem computes elem2^{-1} * elem1 *
// elem2 into elem3.
func xsp2co1ConjElem(elem1, elem2, elem3 []uint64) {
	qs2 := xsp2co1ElemToQsI(elem2)
	qs3 := qs2.copy()
	qsMatInv(qs3)
	qs1 := xsp2co1ElemToQsI(elem1)
	qs3 = qsMatmul(qs1, qs3)
	qs3 = qsMatmul(qs2, qs3)
	v2 := xsp2co1MapShort3(qs2, xspStdV3, elem2[0], elem1[0])
	v := xsp2co1MapShort3(qs3, elem2[0], v2, xspStdV3)
	xsp2co1QsToElemI(qs3, v, elem3)
}

// xsp2co1PowerElem computes elem1^e into elem2.
func xsp2co1PowerElem(elem1 []uint64, e int64, elem2 []uint64) {
	var elem [26]uint64
	if e == 0 {
		xsp2co1UnitElem(elem2)
		return
	}
	var ee uint64
	if e < 0 {
		ee = uint64(-e)
		xsp2co1InvElem(elem1, elem[:])
	} else {
		ee = uint64(e)
		xsp2co1CopyElem(elem1, elem[:])
	}
	xsp2co1CopyElem(elem[:], elem2)
	mask := uint64(1)
	for mask <= ee {
		mask += mask
	}
	mask >>= 1
	mask >>= 1
	for mask != 0 {
		xsp2co1MulElem(elem2, elem2, elem2)
		if ee&mask != 0 {
			xsp2co1MulElem(elem[:], elem2, elem2)
		}
		mask >>= 1
	}
}

/*************************************************************************
*** Conjugate elements of Q_{x0} with an element of G_{x0}
*************************************************************************/

// xsp2co1XspecialConjugate replaces each entry of
// ax by g^{-1} x_i g, where g = elem. If sign is
// false the result signs are not computed.
func xsp2co1XspecialConjugate(elem []uint64, ax []uint64, sign bool) {
	if !sign {
		var data [24]uint64
		xsp2co1ElemToBitmatrix(elem, data[:])
		out := make([]uint64, len(ax))
		Bm64Mul(ax, data[:], len(ax), 24, out)
		copy(ax, out)
		return
	}
	qs := xsp2co1ElemToQsI(elem)
	qs1 := qs.copy()
	for i := range ax {
		ax[i] = uint64(convPauliVectorXspecial(uint32(ax[i])))
	}
	out := qsPauliConjugate(qs1, ax)
	copy(ax, out)
	for i := range ax {
		ax[i] = uint64(convPauliVectorXspecial(uint32(ax[i])))
	}
}

// xsp2co1XspecialImgOmega returns g^{-1} Omega g
// in Leech lattice encoding, ignoring the sign.
func xsp2co1XspecialImgOmega(elem []uint64) uint32 {
	qs := xsp2co1ElemToQsI(elem)
	res := qsToSymplecticRow(qs, 11)
	sh := (res ^ (res >> 12)) & 0x800
	res ^= sh ^ (sh << 12)
	return res & 0xffffff
}

/*************************************************************************
*** Convert x in Q_{x0} between G_x0 rep and Leech encoding
*************************************************************************/

// xsp2co1XspecialVector returns the element x of
// Q_{x0} stored in elem as a Leech lattice vector,
// or a negative value if elem is not in Q_{x0}.
func xsp2co1XspecialVector(elem []uint64) int32 {
	qs := xsp2co1ElemToQsI(elem)
	qs1 := qs.copy()
	v, ok := qsPauliVectorSafe(qs1)
	if !ok {
		return -1
	}
	e0 := short3Reduce(elem[0])
	switch e0 {
	case xspStdV3Neg:
		v ^= 0x1000000
	case xspStdV3:
	default:
		return -1
	}
	return int32(convPauliVectorXspecial(uint32(v)))
}

// qsPauliVectorSafe returns the encoded Pauli
// vector of s, with ok false when s is not in the
// Pauli group (instead of panicking). This
// mirrors the C error return of
// qstate12_pauli_vector.
func qsPauliVectorSafe(s *qs12) (v uint64, ok bool) {
	defer func() {
		if recover() != nil {
			v, ok = 0, false
		}
	}()
	_, v = qsPauliVector(s)
	return v, true
}

// mulQsXspecial right-multiplies x_g (in s) by
// the element x of Q_{x0} in Leech encoding.
func mulQsXspecial(s *qs12, x uint32) {
	x = convPauliVectorXspecial(x)
	qsReduce(s)
	sh := uint(s.shape1)
	qsGatePhi(s, uint64(x&0xfff)<<sh, 2)
	qsGateNot(s, uint64((x>>12)&0xfff)<<sh)
	s.factor ^= int64((x >> 22) & 4)
}

// xsp2co1ElemXspecial converts x in Q_{x0} (Leech
// encoding) to G_x0 representation in elem.
func xsp2co1ElemXspecial(elem []uint64, x uint32) error {
	s := &qs12{}
	s.grow(xspMaxRowsElem)
	qsUnitMatrix(s, 12)
	mulQsXspecial(s, x)
	xsp2co1QsToElemI(s, xspStdV3, elem)
	return nil
}

/*************************************************************************
*** Building an element from a word: auxiliary x_pi, y_d, xi gates
*************************************************************************/

// setQsDeltaPiAut sets s to the element x_pi of
// G(4096_x) for the Parker loop automorphism aut.
func setQsDeltaPiAut(s *qs12, aut []uint32) {
	data := make([]uint64, 13)
	for i := 0; i < 12; i++ {
		data[i+1] = uint64(aut[i])
	}
	qsMonomialColumnMatrix(s, 12, data)
	convConjugateBasis(s)
}

// setQsY sets s to the element y_d of G(4096_x)
// for the Parker loop element y.
func setQsY(s *qs12, y uint32) {
	thetaY := uint64(mat24ThetaTable[y&0x7ff]) & 0x7ff
	data := make([]uint64, 13)
	data[0] = uint64(y & 0x17ff)
	for i := 0; i < 11; i++ {
		d := uint32(1) << uint(i)
		thetaD := uint64(mat24ThetaTable[d&0x7ff])
		sv := thetaD & uint64(y)
		sv = uint64(Parity12(uint32(sv)))
		assoc := uint64(mat24ThetaTable[(d^y)&0x7ff]) ^ thetaD ^ thetaY
		data[i+1] = uint64(d) + (sv << 12) + ((assoc & 0x7ff) << 13)
	}
	data[12] = data[0] + 0x800 + (thetaY << 13)
	qsMonomialColumnMatrix(s, 12, data)
}

// mulQsXi1 right-multiplies x_g (in s) by xi_g.
func mulQsXi1(s *qs12) {
	sh := uint(s.shape1)
	qsGateNot(s, 0x400<<sh)
	qsGateCtrlNot(s, 0x800<<sh, 0x400<<sh)
	qsGateCtrlNot(s, 0xf<<sh, 0xf<<sh)
	qsGateH(s, 0xf<<sh)
}

// mulQsXi2 right-multiplies x_g (in s) by
// xi_gamma.
func mulQsXi2(s *qs12) {
	sh := uint(s.shape1)
	qsXchBitsRaw(s, 1, 0x400<<sh)
	qsGateCtrlPhi(s, 8<<sh, 7<<sh)
	qsGateCtrlPhi(s, 4<<sh, 3<<sh)
	qsGateCtrlPhi(s, 2<<sh, 1<<sh)
}

// qsXchBitsRaw exchanges bit pairs of every data
// row of s (qstate12_xch_bits), shifting bits in
// mask by sh.
func qsXchBitsRaw(s *qs12, sh int, mask uint64) {
	Bm64XchBits(s.data, s.nrows, sh, mask)
	s.reduced = false
}

// mulQsXi right-multiplies x_g (in s) by xi^e,
// where e = eMinus1 + 1 is 1 or 2.
func mulQsXi(s *qs12, eMinus1 uint32) {
	if eMinus1 != 0 {
		mulQsXi2(s)
		mulQsXi1(s)
	} else {
		mulQsXi1(s)
		mulQsXi2(s)
	}
}

/*************************************************************************
*** Multiply a decomposed element of G_{x0} by a word of generators
*************************************************************************/

// xsp2co1MulQsV3Word multiplies the element g of
// G_{x0} (given as qstate s and Leech-mod-3 vector
// *pv3) by the longest prefix of the word a that
// lies in G_{x0}. If setOne, s is reset to the
// unit matrix and v3 to STD_V3 first. It returns
// the number of atoms processed.
func xsp2co1MulQsV3Word(s *qs12, pv3 *uint64, a []uint32, setOne bool) int {
	n := len(a)
	maxrows := s.ncols + 1
	if maxrows < 14 {
		maxrows = 14
	}
	if maxrows > xspMaxRowsElem {
		maxrows = xspMaxRowsElem
	}
	qs := &qs12{}
	qs.grow(xspMaxRowsElem)
	qsAtom := &qs12{}
	qsAtom.grow(xspMaxRowsElem)
	var v3 uint64
	var pAtom *qs12
	if setOne {
		qsUnitMatrix(qs, 12)
		v3 = xspStdV3
		pAtom = qs
	} else {
		qsCopyInto(s, qs)
		v3 = *pv3
		pAtom = qsAtom
	}

	i := 0
	for ; i < n; i++ {
		v := a[i]
		tag := v & atomTagAll
		v &= atomData
		multiply := false
		var x uint32
		switch tag {
		case atomTag1, atomTagI1:
		case atomTagID, atomTagD:
			mulQsXspecial(qs, v&0xfff)
		case atomTagIP:
			perm := m24numToPermSafe(v)
			aut := PermToAutpl(0, perm)
			permI := InvPerm(perm)
			autI := InvAutpl(aut)
			setQsDeltaPiAut(pAtom, autI)
			v3 = leech3OpPi(v3, permI)
			multiply = pAtom != qs
		case atomTagP:
			perm := m24numToPermSafe(v)
			aut := PermToAutpl(0, perm)
			setQsDeltaPiAut(pAtom, aut)
			v3 = leech3OpPi(v3, perm)
			multiply = pAtom != qs
		case atomTagIX:
			x ^= (uint32(mat24ThetaTable[v&0x7ff]) & 0x1000) << 12
			x ^= (v & 0x1fff) << 12
			x ^= uint32(mat24ThetaTable[v&0x7ff]) & 0xfff
			mulQsXspecial(qs, x)
		case atomTagX:
			x ^= (v & 0x1fff) << 12
			x ^= uint32(mat24ThetaTable[v&0x7ff]) & 0xfff
			mulQsXspecial(qs, x)
		case atomTagIY:
			x ^= uint32(mat24ThetaTable[v&0x7ff]) & 0x1000
			x ^= v & 0x1fff
			setQsY(pAtom, x)
			v3 = leech3OpY(v3, x)
			multiply = pAtom != qs
		case atomTagY:
			x ^= v & 0x1fff
			setQsY(pAtom, x)
			v3 = leech3OpY(v3, x)
			multiply = pAtom != qs
		case atomTagIT, atomTagT:
			if v%3 != 0 {
				goto final
			}
		case atomTagIL:
			v ^= 0xfffffff
			fallthrough
		case atomTagL:
			v = v % 3
			if v != 0 {
				mulQsXi(qs, v-1)
				v3 = leech3OpXi(v3, v)
			}
		default:
			goto final
		}
		if multiply {
			qs2 := qsMatmul(qsAtom, qs)
			qsCopyInto(qs2, qs)
		} else if qs.nrows > maxrows {
			qsReduce(qs)
		}
		pAtom = qsAtom
	}

final:
	qsReduce(qs)
	qsCopyInto(qs, s)
	*pv3 = v3
	return i
}

// qsCopyInto copies the contents of src into dst
// (preserving dst's backing capacity).
func qsCopyInto(src, dst *qs12) {
	dst.grow(src.nrows)
	copy(dst.data, src.data[:src.nrows])
	dst.nrows = src.nrows
	dst.ncols = src.ncols
	dst.shape1 = src.shape1
	dst.factor = src.factor
	dst.reduced = src.reduced
}

/*************************************************************************
*** Multiply / set an element of G_{x0} by a word of generators
*************************************************************************/

// mulSetElemWord multiplies elem by the longest
// G_{x0} prefix of a (or sets elem from it if
// setOne). It returns the number of atoms
// processed.
func mulSetElemWord(elem []uint64, a []uint32, setOne bool) int {
	s := xsp2co1ElemToQsI(elem)
	k := xsp2co1MulQsV3Word(s, &elem[0], a, setOne)
	xsp2co1QsToElemI(s, elem[0], elem)
	return k
}

// xsp2co1MulElemWord right-multiplies elem by the
// word a. It returns errNotInGx0 if a leaves
// G_{x0}.
func xsp2co1MulElemWord(elem []uint64, a []uint32) error {
	if mulSetElemWord(elem, a, false) == len(a) {
		return nil
	}
	return errNotInGx0
}

// xsp2co1SetElemWord converts the word a to G_x0
// representation in elem. It returns errNotInGx0
// if a leaves G_{x0}.
func xsp2co1SetElemWord(elem []uint64, a []uint32) error {
	if mulSetElemWord(elem, a, true) == len(a) {
		return nil
	}
	return errNotInGx0
}

// xsp2co1SetElemWordScan sets (mul=false) or
// multiplies (mul=true) elem with the longest
// G_{x0} prefix of a and returns its length.
func xsp2co1SetElemWordScan(elem []uint64, a []uint32, mul bool) int {
	return mulSetElemWord(elem, a, !mul)
}

/*************************************************************************
*** Public group operations
*************************************************************************/

// Mul returns g * h.
func (g *Xsp2Co1) Mul(h *Xsp2Co1) *Xsp2Co1 {
	out := &Xsp2Co1{}
	xsp2co1MulElem(g.data[:], h.data[:], out.data[:])
	return out
}

// Inv returns g^{-1}.
func (g *Xsp2Co1) Inv() *Xsp2Co1 {
	out := &Xsp2Co1{}
	xsp2co1InvElem(g.data[:], out.data[:])
	return out
}

// Pow returns g^e.
func (g *Xsp2Co1) Pow(e int) *Xsp2Co1 {
	out := &Xsp2Co1{}
	xsp2co1PowerElem(g.data[:], int64(e), out.data[:])
	return out
}

// Equal reports whether g and h are equal,
// comparing their reduced G_x0 representations.
func (g *Xsp2Co1) Equal(h *Xsp2Co1) bool {
	var a, b [26]uint64
	xsp2co1CopyElem(g.data[:], a[:])
	xsp2co1CopyElem(h.data[:], b[:])
	xsp2co1ReduceElem(a[:])
	xsp2co1ReduceElem(b[:])
	return a == b
}

// AsXsp returns g as a vector of Q_{x0} in Leech
// lattice encoding. AsXsp panics if g is not in
// Q_{x0}.
func (g *Xsp2Co1) AsXsp() uint32 {
	v := xsp2co1XspecialVector(g.data[:])
	if v < 0 {
		panic("cgt: element is not in Q_x0")
	}
	return uint32(v)
}

// XspConjugate returns the list g^{-1} v_i g for
// each element v_i of Q_{x0} in v (Leech lattice
// encoding).
func (g *Xsp2Co1) XspConjugate(v []uint32) []uint32 {
	ax := make([]uint64, len(v))
	for i, x := range v {
		ax[i] = uint64(x)
	}
	xsp2co1XspecialConjugate(g.data[:], ax, true)
	out := make([]uint32, len(v))
	for i, x := range ax {
		out[i] = uint32(x)
	}
	return out
}

/*************************************************************************
*** Order of an element of G_{x0}
*************************************************************************/

// isNeutralCo1 reports whether bm is the 24x24
// unit bit matrix.
func isNeutralCo1(bm []uint64) bool {
	var acc uint64
	for i := 0; i < 24; i++ {
		acc |= bm[i] ^ (uint64(1) << uint(i))
	}
	return acc&0xffffff == 0
}

// xsp2co1OddOrderBitmatrix returns the odd part
// of the order of the Co_1 element given by the
// 24x24 bit matrix bm. It panics on failure.
func xsp2co1OddOrderBitmatrix(bm []uint64) int {
	var bm1, bm2 [24]uint64
	for i := 0; i < 24; i++ {
		bm1[i] = bm[i] & 0xffffff
	}
	for i := 0; i < 2; i++ {
		Bm64Mul(bm1[:], bm1[:], 24, 24, bm2[:])
		Bm64Mul(bm2[:], bm2[:], 24, 24, bm1[:])
	}
	if isNeutralCo1(bm1[:]) {
		return 1
	}
	Bm64Mul(bm1[:], bm1[:], 24, 24, bm2[:])
	for i := 3; i <= 39; i += 2 {
		Bm64Mul(bm1[:], bm2[:], 24, 24, bm1[:])
		if isNeutralCo1(bm1[:]) {
			return i
		}
	}
	panic("cgt: order of G_x0 element not found")
}

// xsp2co1HalfOrderElem returns the order o of the
// element elem1. If o is even it stores
// elem1^(o/2) in elem2; otherwise it stores the
// unit element. elem2 may be nil.
func xsp2co1HalfOrderElem(elem1, elem2 []uint64) int {
	var bm [24]uint64
	xsp2co1ElemToBitmatrix(elem1, bm[:])
	o := xsp2co1OddOrderBitmatrix(bm[:])
	var elemH [26]uint64
	xsp2co1PowerElem(elem1, int64(o), elemH[:])
	if elem2 != nil {
		xsp2co1UnitElem(elem2)
	}
	for i := 0; i < 6; i++ {
		if xsp2co1IsUnitElem(elemH[:]) {
			return o
		}
		if elem2 != nil {
			xsp2co1CopyElem(elemH[:], elem2)
		}
		xsp2co1MulElem(elemH[:], elemH[:], elemH[:])
		o += o
	}
	panic("cgt: order of G_x0 element not found")
}

// Order returns the order of g in the monster.
func (g *Xsp2Co1) Order() int {
	return xsp2co1HalfOrderElem(g.data[:], nil)
}

// HalfOrder returns the order o of g and, if o is
// even, the element g^(o/2); otherwise it returns
// the identity.
func (g *Xsp2Co1) HalfOrder() (int, *Xsp2Co1) {
	out := &Xsp2Co1{}
	o := xsp2co1HalfOrderElem(g.data[:], out.data[:])
	return o, out
}

/*************************************************************************
*** Operation of G_{x0} on Q_{x0}: gen_leech2_op_word
*************************************************************************/

// Leech2OpWord returns g^{-1} q0 g for q0 in
// Q_{x0} (Leech lattice encoding) and a word g of
// generators of G_{x0}. Leech2OpWord panics if an
// atom of g is illegal.
func Leech2OpWord(x uint32, g []uint32) uint32 {
	q0 := x & 0x1ffffff
	for _, atom := range g {
		q0 = Leech2OpAtom(q0, atom)
		if q0 == 0xffffffff {
			panic("cgt: illegal atom in Leech2OpWord")
		}
	}
	return q0
}

// genLeech2OpWordMany applies the word g to every
// entry of q in place, returning the number of
// atoms successfully applied to all entries.
func genLeech2OpWordMany(q []uint32, g []uint32) int {
	for j := range q {
		q[j] &= 0x1ffffff
	}
	for i, atom := range g {
		next := make([]uint32, len(q))
		for j, qv := range q {
			r := Leech2OpAtom(qv, atom)
			if r == 0xffffffff {
				return i
			}
			next[j] = r
		}
		copy(q, next)
	}
	return len(g)
}

// opPermNoSign applies the permutation pi to the Leech
// lattice mod 2 vector v, ignoring the sign. C function
// op_perm_nosign.
func opPermNoSign(v uint32, pi []byte) uint32 {
	xd := (v >> 12) & 0xfff
	xdelta := (v ^ uint32(mat24ThetaTable[xd&0x7ff])) & 0xfff
	xd = OpGcodePerm(xd, pi)
	xdelta = OpCocodePerm(xdelta, pi)
	return (xd << 12) ^ xdelta ^ (uint32(mat24ThetaTable[xd&0x7ff]) & 0xfff)
}

// opYNoSign applies x_d to the Leech lattice mod 2
// vector q0, ignoring the sign. C function op_y_nosign.
func opYNoSign(q0, d uint32) uint32 {
	odd := 0 - ((q0 >> 11) & 1)
	thetaQ0 := uint32(mat24ThetaTable[(q0>>12)&0x7ff])
	thetaY := uint32(mat24ThetaTable[d&0x7ff])
	o := (thetaY & (q0 >> 12)) ^ (q0 & d)
	o ^= (thetaY >> 12) & 1 & odd
	o = Parity12(o)
	eps := thetaQ0 ^ (thetaY & ^odd) ^ uint32(mat24ThetaTable[((q0>>12)^d)&0x7ff])
	q0 ^= (eps & 0xfff) ^ ((d << 12) & 0xfff000 & odd)
	q0 ^= o << 23
	return q0
}

// genLeech2OpWordLeech2Many applies the word g (or its
// inverse if back) to every entry of a in place,
// ignoring the sign of each Leech-mod-2 vector. It
// returns 0 on success or a negative value if any
// generator in g is not in G_x0. C function
// gen_leech2_op_word_leech2_many.
//
// Unlike genLeech2OpWordMany, this is the sign-free
// operation: tags x, d and the identity are ignored (they
// only change signs), tags p/y/l act via the nosign
// helpers, and a nonzero tau (tag t) or opaque atom
// (tag 7) makes the word leave G_x0.
func genLeech2OpWordLeech2Many(a []uint32, g []uint32, back bool) int {
	step := 1
	imask := uint32(0)
	idx := 0
	if back {
		step = -1
		imask = 0x80000000
		idx = len(g) - 1
	}
	for n := len(g); n > 0; n-- {
		v := g[idx] ^ imask
		idx += step
		tag := v & 0xf0000000
		v &= 0xfffffff
		switch tag {
		case 0xa0000000: // Ip
			perm := m24numToPermSafe(v)
			perm = InvPerm(perm)
			for j := range a {
				a[j] = opPermNoSign(a[j], perm)
			}
		case 0x20000000: // p
			perm := m24numToPermSafe(v)
			for j := range a {
				a[j] = opPermNoSign(a[j], perm)
			}
		case 0xc0000000: // Iy
			y := uint32(mat24ThetaTable[v&0x7ff]) & 0x1000
			y ^= v & 0x1fff
			for j := range a {
				a[j] = opYNoSign(a[j], y&0x1fff)
			}
		case 0x40000000: // y
			y := v & 0x1fff
			for j := range a {
				a[j] = opYNoSign(a[j], y&0x1fff)
			}
		case 0xe0000000, 0x60000000: // Il, l
			if tag == 0xe0000000 {
				v ^= 3
			}
			for j := range a {
				a[j] = XiOpXiNoSign(a[j], int(v))
			}
		case 0xd0000000, 0x50000000: // It, t
			if (v-1)&2 == 0 {
				return -1
			}
		case 0x70000000, 0xf0000000:
			if v != 0 {
				return -1
			}
		default: // 1, I1, d, Id, x, Ix: no effect on sign-free image
		}
	}
	return 0
}

/*************************************************************************
*** Subtype of an element of G_{x0}
*************************************************************************/

// xsp2co1ElemSubtype returns the subtype of elem
// (as gen_leech2_subtype) or -1 on error.
func xsp2co1ElemSubtype(elem []uint64) int32 {
	subtypes := [8]int8{0x48, -1, 0x40, 0x42, 0x44, 0x43, 0x46, -1}
	i := 26
	for i != 0 && elem[i-1] == 0 {
		i--
	}
	i -= 14
	if i&0xfffffff1 != 0 {
		return -1
	}
	return int32(subtypes[i>>1])
}

// Subtype returns 16*type + subtype as a packed
// value. Python .subtype unpacks to (type, subtype).
func (g *Xsp2Co1) Subtype() uint32 {
	res := xsp2co1ElemSubtype(g.data[:])
	if res&0x40 != 0x40 {
		panic("cgt: bad subtype of G_x0 element")
	}
	return uint32(res)
}

// TypeQx0 returns the type (0, 2, 3 or 4) of g as
// a vector in the Leech lattice mod 2. TypeQx0
// panics if g is not in Q_{x0}.
func (g *Xsp2Co1) TypeQx0() uint32 {
	return Leech2Type(g.AsXsp())
}

/*************************************************************************
*** Convert element of G_{x0} to Leech lattice matrix (8 * L)
*************************************************************************/

// xsp2co1ToVectMod3 converts a Leech-mod-3 vector
// to the (Z/3)^24 encoding used by mm_op.
func xsp2co1ToVectMod3(x uint64) uint64 {
	x = short3Reduce(x)
	x = (x & 0xffffff) + ((x & 0xffffff000000) << 8)
	x = shiftMasked(x, 0x00000000FFFF0000, 16)
	x = shiftMasked(x, 0x0000FF000000FF00, 8)
	x = shiftMasked(x, 0x00F000F000F000F0, 4)
	x = shiftMasked(x, 0x0C0C0C0C0C0C0C0C, 2)
	x = shiftMasked(x, 0x2222222222222222, 1)
	return x
}

// xsp2co1FromVectMod3 converts a vector in the
// (Z/3)^24 encoding used by mm_op to Leech-mod-3
// encoding. Inverse of xsp2co1ToVectMod3.
// C xsp2co1_from_vect_mod3.
func xsp2co1FromVectMod3(x uint64) uint64 {
	x = shiftMasked(x, 0x2222222222222222, 1)
	x = shiftMasked(x, 0x0C0C0C0C0C0C0C0C, 2)
	x = shiftMasked(x, 0x00F000F000F000F0, 4)
	x = shiftMasked(x, 0x0000FF000000FF00, 8)
	x = shiftMasked(x, 0x00000000FFFF0000, 16)
	x = (x & 0xffffff) + ((x & 0xffffff00000000) >> 8)
	return short3Reduce(x)
}

// shiftMasked implements the SHIFT_MASKED macro:
// exchange bits in mask with bits in mask<<sh.
func shiftMasked(a, mask uint64, sh uint) uint64 {
	aux := (a ^ (a >> sh)) & mask
	return a ^ aux ^ (aux << sh)
}

// xsp2co1AddShort3Leech computes dest = src +
// factor * x, where x is a short Leech-mod-3
// vector and src, dest are length-24 int8 slices.
// It panics on an illegal x.
func xsp2co1AddShort3Leech(x uint64, factor int8, src, dest []int8) {
	var f [4]int8
	x = short3Reduce(x)
	w1 := Bw24(uint32(x))
	w2 := Bw24(uint32(x >> 24))
	var gcodev uint32
	switch w1 + w2 {
	case 23:
		cocodev := ^uint32(x|(x>>24)) & 0xffffff
		if cocodev == 0 || cocodev&(cocodev-1) != 0 {
			panic("cgt: Leech mod 3 operation failed")
		}
		if w1&1 != 0 {
			f[0] = factor * -3
		} else {
			f[0] = factor * 3
		}
		f[1] = factor
		gcodev = uint32(x>>((0-(w1&1))&24)) & 0xffffff
	case 8:
		if w1&1 != 0 {
			panic("cgt: Leech mod 3 operation failed")
		}
		gcodev = uint32(x|(x>>24)) & 0xffffff
		f[1] = -2 * factor
	case 2:
		gcodev = 0
		f[1] = 4 * factor
	default:
		panic("cgt: Leech mod 3 operation failed")
	}
	f[2] = -f[1]
	gcodev = VectToGcode(gcodev)
	if gcodev&0xfffff000 != 0 {
		panic("cgt: Leech mod 3 operation failed")
	}
	xv := xsp2co1ToVectMod3(x)
	for i := 0; i < 24; i++ {
		dest[i] = src[i] + f[(xv>>uint(i<<1))&3]
	}
}

// short3ToLeech writes the integer coordinates of
// the short Leech-mod-3 vector x into pdest (length
// 24), normalized to norm 32. It panics if x is not
// a short Leech-mod-3 vector.
// C xsp2co1_short_3_to_leech.
func short3ToLeech(x uint64, pdest []int8) {
	for i := 0; i < 24; i++ {
		pdest[i] = 0
	}
	xsp2co1AddShort3Leech(x, 1, pdest, pdest)
}

// short2ToLeech writes the integer coordinates of
// the short Leech-mod-2 vector x into pdest (length
// 24), normalized to norm 32; the sign is arbitrary.
// It panics if x is not a short Leech-mod-2 vector.
// C xsp2co1_short_2_to_leech.
func short2ToLeech(x uint32, pdest []int8) {
	x3 := Leech2To3Short(x & 0xffffff)
	if x3 == 0 {
		panic("cgt: short2ToLeech: vector is not short")
	}
	short3ToLeech(x3, pdest)
}

// xsp2co1ElemToLeechOp computes the 24x24 integer
// matrix L_g with 8*L_g equal to the action of g
// on the Leech lattice, stored row-major in pdest
// (length 576).
func xsp2co1ElemToLeechOp(elem []uint64, pdest []int8) {
	var src3, dest3 [25]uint64
	for i := 0; i <= 20; i++ {
		src3[i] = xspStdV3 << uint(i)
	}
	src3[21] = 0x1800000
	src3[22] = xspStdV3 >> 2
	src3[23] = xspStdV3 >> 1
	src3[24] = 0xc
	dest3[0] = elem[0]
	qs := xsp2co1ElemToQsI(elem)
	xsp2co1ChainShort3(qs, 25, src3[:], dest3[:])
	for i := 0; i < 24; i++ {
		pdest[2*24+i] = 0
	}
	xsp2co1AddShort3Leech(dest3[24], 1, pdest[2*24:], pdest[2*24:])
	copy(pdest[3*24:3*24+24], pdest[2*24:2*24+24])
	xsp2co1AddShort3Leech(dest3[0], 1, pdest[2*24:], pdest[2*24:])
	xsp2co1AddShort3Leech(dest3[0], -1, pdest[3*24:], pdest[3*24:])
	xsp2co1AddShort3Leech(dest3[23], 2, pdest[2*24:], pdest[1*24:])
	xsp2co1AddShort3Leech(dest3[22], 2, pdest[1*24:], pdest[0*24:])
	xsp2co1AddShort3Leech(dest3[21], 2, pdest[0*24:], pdest[23*24:])
	for i := 20; i >= 2; i-- {
		xsp2co1AddShort3Leech(dest3[i], 2, pdest[(i+3)*24:], pdest[(i+2)*24:])
	}
}

// AsCo1Bitmatrix returns the 24x24 bit matrix of
// g acting on the Leech lattice mod 2 by right
// multiplication, as 24 uint64 rows.
func (g *Xsp2Co1) AsCo1Bitmatrix() []uint64 {
	m := make([]uint64, 24)
	xsp2co1ElemToBitmatrix(g.data[:], m)
	for i := range m {
		m[i] &= 0xffffff
	}
	return m
}

/*************************************************************************
*** Counting type-2 vectors in an affine subspace of Leech mod 2
*************************************************************************/

// xsp2co1Leech2CountType2 returns the number of
// type-2 vectors in the affine space spanned by
// the n bit vectors in a (a[0] is the offset). It
// may modify a.
func xsp2co1Leech2CountType2(a []uint64, n int) uint32 {
	const lsteps = 7
	if n == 0 {
		return 0
	}
	// Track the data via an offset into a so the
	// final restore (C: --a) is faithful.
	Bm64XchBits(a, n, 12, 0x800)
	v := uint32(a[0])
	off := 1
	babysteps := 1
	m := a[off:]
	nEch := Bm64EchelonH(m, n-1, 24, 24)
	var b [1 << lsteps]uint16
	b[0] = 0
	nh := 0
	for nh < nEch {
		vh := m[nEch-nh-1]
		vh1 := uint16(vh & 0xfff)
		if nh == lsteps || vh&0xfff000 != 0 {
			break
		}
		for j := 0; j < babysteps; j++ {
			b[j+babysteps] = b[j] ^ vh1
		}
		babysteps <<= 1
		nh++
	}
	nEch -= nh
	bigsteps := 1 << uint(nEch)

	var count uint32
	for i := 1; ; i++ {
		if v&0x800000 != 0 {
			gcode := (v >> 12) & 0xfff
			theta := uint32(mat24ThetaTable[gcode&0x7ff]) ^ v
			for j := 0; j < babysteps; j++ {
				tab := uint32(mat24SyndromeTable[(theta^uint32(b[j]))&0x7ff])
				if (tab & 0x3ff) < (24 << 5) {
					continue
				}
				scalar := gcode & (v ^ uint32(b[j]))
				scalar ^= scalar >> 6
				scalar ^= scalar >> 3
				scalar = (0x69 >> (scalar & 7)) & 1
				count += scalar
			}
		} else if (v & 0x7ff000) == 0 {
			basis0 := uint32(mat24RecipBasis[0]) ^ v
			for j := 0; j < babysteps; j++ {
				tab := uint32(mat24SyndromeTable[(uint32(b[j])^basis0)&0x7ff])
				var b1, b0 uint32
				if (tab & 0x3ff) < (24 << 5) {
					b1 = 1
				}
				if tab&0x1f != 0 {
					b0 = 1
				}
				count += b0 ^ b1
			}
		} else if notNonstrictOctad(v>>12) == 0 {
			vect := GcodeToVect((v >> 12) & 0x7ff)
			theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
			w0 := (theta >> 13) & 1
			vect ^= w0 - 1
			w0 ^= (v >> 11) & 1
			lsb := lsbit24(vect)
			theta ^= v ^ uint32(mat24RecipBasis[lsb])
			for j := 0; j < babysteps; j++ {
				tab := uint32(mat24SyndromeTable[(uint32(b[j])^theta)&0x7ff])
				syn := (uint32(1) << (tab & 31)) ^ (uint32(1) << ((tab >> 5) & 31)) ^ (uint32(1) << ((tab >> 10) & 31))
				if vect&syn != syn {
					continue
				}
				var b1, b0 uint32
				if (tab & 0x3ff) < (24 << 5) {
					b1 = 1
				}
				if tab&0x1f != lsb {
					b0 = 1
				}
				b0 ^= b1
				count += b0 ^ w0 ^ (uint32(b[j]) >> 11)
			}
		}
		if i == bigsteps {
			break
		}
		v ^= uint32(a[off+int(lsbit24(uint32(i)))])
	}

	// C: --a; n += nh + 1; xch_bits(a, n, ...)
	Bm64XchBits(a, n, 12, 0x800)
	return count
}

// notNonstrictOctad returns 0 if v (or its
// complement) is an octad and 1 otherwise.
func notNonstrictOctad(v uint32) uint32 {
	return uint32(mat24OctEncTable[v&0x7ff]) >> 15
}

/*************************************************************************
*** Characters / traces of an element of G_{x0}
*************************************************************************/

// xsp2co1Trace98280 returns the character of the
// representation rho_98280, optionally trying the
// fast table-based fFast first.
func xsp2co1Trace98280(elem []uint64, fFast func([]uint64) (int32, bool)) int32 {
	var data [25]uint64
	pa := data[1:]
	for i := 0; i < 24; i++ {
		pa[i] = uint64(1) << uint(i)
	}
	xsp2co1XspecialConjugate(elem, pa, false)
	mask := uint64(0x1000001)
	for i := 0; i < 24; i++ {
		pa[i] = ((pa[i] & 0xffffff) << 24) ^ mask
		mask <<= 1
	}
	nn := Bm64EchelonH(pa, 24, 48, 24)
	pa = pa[nn:]
	n := 24 - nn
	if n == 0 {
		return 0
	}
	if fFast != nil && n >= 12 {
		if r, ok := fFast(elem); ok {
			return r
		}
	}
	xsp2co1XspecialConjugate(elem, pa[:n], true)
	idx := Bm64EchelonH(pa, n, 25, 1)
	var res int32
	if idx != 0 {
		res = 0 - int32(xsp2co1Leech2CountType2(pa, n))
	} else {
		pa = data[1+nn-1:]
		n++
	}
	pa[0] = 0
	res += int32(xsp2co1Leech2CountType2(pa, n))
	return res
}

// trace4096 computes the character of rho_4096 of
// elem (defined up to sign).
func trace4096(elem []uint64) int32 {
	qs := xsp2co1ElemToQsI(elem)
	qs1 := qs.copy()
	return int32(qsMatItrace(qs1))
}

// tracesVerySmall computes the characters of
// rho_24 and rho_576 into ptrace[0], ptrace[1].
func tracesVerySmall(elem []uint64, ptrace []int32) {
	var a [576]int8
	xsp2co1ElemToLeechOp(elem, a[:])
	acc := 0
	for i := 0; i < 24; i++ {
		acc += int(a[25*i])
	}
	if acc&7 != 0 {
		panic("cgt: scalar factor overflow")
	}
	ptrace[0] = int32(acc >> 3)
	acc = 0
	for i := 0; i < 24; i++ {
		for j := 0; j < 24; j++ {
			acc += int(a[24*i+j]) * int(a[24*j+i])
		}
	}
	if acc&63 != 0 {
		panic("cgt: scalar factor overflow")
	}
	ptrace[1] = int32(acc >> 6)
	if (ptrace[0]+ptrace[1])&1 != 0 {
		panic("cgt: scalar factor overflow")
	}
}

// xsp2co1TracesSmall computes characters of
// rho_24, rho_576, rho_4096 into ptrace[0..2],
// normalizing the (24, 4096) sign.
func xsp2co1TracesSmall(elem []uint64, ptrace []int32) {
	ptrace[3] = -0x2000000
	tracesVerySmall(elem, ptrace)
	ptrace[2] = trace4096(elem)
	if ptrace[0] < 0 {
		ptrace[0] = -ptrace[0]
		ptrace[2] = -ptrace[2]
	} else if ptrace[0] == 0 && ptrace[2] < 0 {
		ptrace[2] = -ptrace[2]
	}
}

// ChiGx0 returns (chi_M, chi_299, chi_24,
// chi_4096) for g.
func (g *Xsp2Co1) ChiGx0() [4]int {
	var a [4]int32
	xsp2co1TracesFast(g.data[:], a[:])
	chi24, chisq24, chi4096, chi98260 := int(a[0]), int(a[1]), int(a[2]), int(a[3])
	chi299 := (chi24*chi24+chisq24)/2 - 1
	chiM := chi299 + chi98260 + chi24*chi4096
	return [4]int{chiM, chi299, chi24, chi4096}
}

/*************************************************************************
*** Convert element of G_{x0} to a word in its generators
*************************************************************************/

// xsp2co1ElemMonomialToXsp computes a word w of
// length at most 2 (tags p, y) with g*w in
// Q_{x0}, given that g (=elem) is monomial in
// 4096_x. It returns the word length, or -1 if g
// is not monomial.
func xsp2co1ElemMonomialToXsp(elem []uint64, a []uint32) int {
	qsI := xsp2co1ElemToQsI(elem)
	var monomial [13]uint32
	if qsMonomialMatrixRowOp(qsI, monomial[:]) < 0 {
		return -1
	}
	y := monomial[12] & 0x7ff
	MatrixFromModOmega(monomial[1:])
	perm := AutplToPerm(monomial[1:])
	perm = InvPerm(perm)
	pi := PermToM24num(perm)
	lenA := 0
	if pi != 0 {
		a[lenA] = 0xA0000000 + pi
		lenA++
	}
	if y != 0 {
		a[lenA] = 0xC0000000 + y
		lenA++
	}
	return lenA
}

// elemToWord is the workhorse for
// xsp2co1ElemToWord. It destroys elem and stores
// the inverse of the reduced word in a, returning
// its length. imgOmega should usually be 0.
func elemToWord(elem []uint64, a []uint32, imgOmega uint32) int {
	imgOmega &= 0xffffff
	if imgOmega == 0 {
		imgOmega = xsp2co1XspecialImgOmega(elem)
	}
	lenA := 0
	if imgOmega != 0x800000 {
		res := genLeech2ReduceType4(imgOmega, a)
		if res < 0 || res > 6 {
			panic("cgt: Leech mod 2 operation failed")
		}
		lenA = res
		if err := xsp2co1MulElemWord(elem, a[:lenA]); err != nil {
			panic(err.Error())
		}
	}
	res := xsp2co1ElemMonomialToXsp(elem, a[lenA:])
	if res < 0 || res > 2 {
		panic("cgt: Leech mod 2 operation failed")
	}
	if err := xsp2co1MulElemWord(elem, a[lenA:lenA+res]); err != nil {
		panic(err.Error())
	}
	lenA += res
	x := xsp2co1XspecialVector(elem)
	if x < 0 {
		panic("cgt: element is not in Q_x0")
	}
	xv := uint32(x) ^ PloopTheta(uint32(x)>>12)
	if xv&0xfff != 0 {
		a[lenA] = 0x90000000 + (xv & 0xfff)
		lenA++
	}
	if xv&0x1fff000 != 0 {
		a[lenA] = 0xB0000000 + (xv >> 12)
		lenA++
	}
	return lenA
}

// xsp2co1ElemToWord converts elem to a reduced
// word in the generators of G_{x0}, stored in a,
// returning its length (at most 10).
func xsp2co1ElemToWord(elem []uint64, a []uint32) int {
	var elemReduced [26]uint64
	xsp2co1CopyElem(elem, elemReduced[:])
	res := elemToWord(elemReduced[:], a, 0)
	if res > 10 {
		panic("cgt: Leech mod 2 operation failed")
	}
	invertWord(a[:res])
	return res
}

// Mmdata returns g as a reduced word of monster
// generator atoms.
func (g *Xsp2Co1) Mmdata() []uint32 {
	var a [10]uint32
	n := xsp2co1ElemToWord(g.data[:], a[:])
	out := make([]uint32, n)
	copy(out, a[:n])
	return out
}

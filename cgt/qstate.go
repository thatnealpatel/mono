package cgt

import (
	"errors"
	"math"
	"math/bits"
	"math/rand"
)

// errQNotSymmetric indicates the Q (quadratic
// form) block of a QStateMatrix is not symmetric.
var errQNotSymmetric = errors.New("cgt: Q part of quadratic state is not symmetric")

// Quadratic state matrices over the Clifford
// group, ported from the mmgroup C library
// (qstate12.c, qmatrix12.c, qstate12io.c and
// the bitmatrix64 helpers in bitmatrix64.c).
//
// A QState describes the representation
// (e', A, Q) of a quadratic mapping. A is a
// (1+m) x n bit matrix and Q a symmetric
// (1+m) x (1+m) bit matrix with Q[0,0] = 0.
// Matrices A and Q are concatenated into a
// single (m+1) x (n+m+1) bit matrix M, stored
// row-wise in data: bit j of data[i] is M[i,j],
// with j < n the A part and j >= n the Q part.
//
// The struct fields map onto the C structure
// qstate12_type as follows: the C ncols equals
// rows+cols, the C shape1 equals cols, the C
// nrows equals len(data), and the C int32
// factor is stored in factor[0] (factor[1] is
// unused). The matrix has complex shape
// 2**rows x 2**cols.

// QState is a quadratic state matrix. The fields
// rows and cols give the complex matrix shape
// (2**rows x 2**cols). factor[0] holds the C-style
// int32 scalar factor; factor[1] is unused. data
// holds the (1+m) rows of the concatenated bit
// matrix M = (A, Q), capped at qsMaxRows entries.
type QState struct {
	rows   int
	cols   int
	factor [2]int
	data   []uint64
}

const (
	qsMaxCols  = 64
	qsMaxRows  = qsMaxCols + 1
	qsUndefRow = 0xff
)

// factorMask masks the valid bits of factor
// (all bits except bit 3).
const factorMask = ^uint64(8)

// qsAddFactors adds two factor encodings,
// keeping only valid bits.
func qsAddFactors(e1, e2 int64) int64 {
	return int64((uint64(e1)&factorMask + uint64(e2)) & factorMask)
}

// qs12 is the mutable working representation
// mirroring qstate12_type. Algorithms operate
// on a qs12; QState methods convert in and out.
type qs12 struct {
	nrows   int
	ncols   int
	shape1  int
	factor  int64
	reduced bool
	data    []uint64
}

// toQS12 builds a working state from q. The data
// slice is copied with capacity qsMaxRows.
func (q *QState) toQS12() *qs12 {
	s := &qs12{
		nrows:  len(q.data),
		ncols:  q.rows + q.cols,
		shape1: q.cols,
		factor: int64(q.factor[0]),
	}
	s.data = make([]uint64, len(q.data), qsMaxRows)
	copy(s.data, q.data)
	return s
}

// store writes the working state back into q.
func (q *QState) store(s *qs12) {
	q.rows = s.ncols - s.shape1
	q.cols = s.shape1
	q.factor[0] = int(s.factor)
	q.factor[1] = 0
	q.data = make([]uint64, s.nrows)
	copy(q.data, s.data[:s.nrows])
}

// fromQS12 wraps a working state in a QState.
func fromQS12(s *qs12) *QState {
	q := &QState{}
	q.store(s)
	return q
}

// copy returns a deep copy of s with capacity
// qsMaxRows.
func (s *qs12) copy() *qs12 {
	d := make([]uint64, s.nrows, qsMaxRows)
	copy(d, s.data[:s.nrows])
	return &qs12{
		nrows:   s.nrows,
		ncols:   s.ncols,
		shape1:  s.shape1,
		factor:  s.factor,
		reduced: s.reduced,
		data:    d,
	}
}

// grow ensures data has at least n usable rows.
func (s *qs12) grow(n int) {
	if cap(s.data) < n {
		d := make([]uint64, n, qsMaxRows)
		copy(d, s.data)
		s.data = d
	}
	if len(s.data) < n {
		s.data = s.data[:n]
	}
}

/*************************************************************************
*** Bit-vector primitives
*************************************************************************/

// qsParity returns the parity of v.
func qsParity(v uint64) uint64 {
	return uint64(bits.OnesCount64(v) & 1)
}

/*************************************************************************
*** Low-level state functions (qstate12.c)
*************************************************************************/

// qsCheck enforces consistency of s: it copies
// Q[0,i] to Q[i,0], clears Q[0,0], masks junk
// bits, and panics if Q is not symmetric.
func qsCheck(s *qs12) {
	if s.nrows+s.ncols > qsMaxCols || s.nrows > qsMaxRows || s.shape1 > s.ncols {
		panic("cgt: inconsistent quadratic state")
	}
	s.factor = int64(uint64(s.factor) & factorMask)
	// C indexes data[0] unconditionally;
	// safe there because data is a fixed
	// array. Go slice may be empty.
	if s.nrows == 0 {
		s.factor = 0
		return
	}
	c := uint(s.ncols)
	m := s.data
	mask := ((uint64(1) << c) << uint(s.nrows)) - 1
	mask &^= uint64(1) << c
	m[0] &= mask
	var err uint64
	for i := 1; i < s.nrows; i++ {
		m[i] &= mask
		m[i] |= (m[0] >> uint(i)) & (uint64(1) << c)
		for j := 0; j < i; j++ {
			err |= (m[i] >> (c + uint(j))) ^ (m[j] >> (c + uint(i)))
		}
	}
	if err&1 != 0 {
		panic(errQNotSymmetric)
	}
}

// qsGetCol returns column j of bit matrix m up to
// and including row length.
func qsGetCol(m []uint64, j, length int) uint64 {
	var a uint64
	for i := 0; i <= length; i++ {
		a |= ((m[i] >> uint(j)) & 1) << uint(i)
	}
	return a
}

// qsFindPivot returns the highest row index i with
// A[i,j] = 1, or -1 if none.
func qsFindPivot(m []uint64, nrows, j int) int {
	mask := uint64(1) << uint(j)
	i := nrows - 1
	for i >= 0 && m[i]&mask == 0 {
		i--
	}
	return i
}

// qsXchRows exchanges rows i1 and i2 of s and
// adjusts Q. This does not change the state.
func qsXchRows(s *qs12, i1, i2 int) {
	m := s.data
	m[i1], m[i2] = m[i2], m[i1]
	a1 := uint(i1 + s.ncols)
	a2 := uint(i2 + s.ncols)
	v := (uint64(1) << a1) ^ (uint64(1) << a2)
	if v != 0 {
		for k := 0; k < s.nrows; k++ {
			m[k] ^= v & -(((m[k] >> a1) ^ (m[k] >> a2)) & 1)
		}
	}
}

// qsPivot pivots rows of s controlled by v: if
// k < i and bit k of v is set, row i is xored
// into row k (columns of Q adjusted). Does not
// change the state.
func qsPivot(s *qs12, i int, v uint64) {
	m := s.data
	colMask := uint64(1) << uint(s.ncols)
	var colUpdate uint64
	s.reduced = false
	for k := i - 1; k > 0; k-- {
		if v&(uint64(1)<<uint(k)) != 0 {
			m[0] ^= ((m[k] & (m[i] >> uint(i-k))) ^ m[i]) & (colMask << uint(k))
			colUpdate |= colMask << uint(k)
			m[k] ^= m[i]
		}
	}
	sh := uint(s.ncols + i)
	if colUpdate != 0 {
		for k := 0; k < s.nrows; k++ {
			m[k] ^= -((m[k] >> sh) & 1) & colUpdate
		}
	}
	if v&1 != 0 {
		k := ((m[0] >> sh) & 1) << 1
		k += (m[i] >> sh) & 1
		s.factor = qsAddFactors(s.factor, int64(k<<1))
		m[0] ^= m[i]
	}
}

// qsPivotRotRows rotates rows fst..last down by
// one (row last moves to row fst), adjusting Q.
// Does not change the state.
func qsPivotRotRows(s *qs12, fst, last int) {
	if fst >= last || fst == 0 || last >= s.nrows {
		return
	}
	m := s.data
	s.reduced = false
	tmp := m[last]
	for i := last; i > fst; i-- {
		m[i] = m[i-1]
	}
	m[fst] = tmp
	c := uint64(1) << uint(s.ncols)
	maskShl := (c << uint(last)) - (c << uint(fst))
	maskShr := c << uint(last)
	maskKeep := ^(maskShl | maskShr)
	for i := 0; i < s.nrows; i++ {
		t := m[i]
		m[i] = (t & maskKeep) | ((t & maskShl) << 1) | ((t & maskShr) >> uint(last-fst))
	}
}

// qsDelRows deletes all rows i (1 <= i < nrows)
// with bit i set in v, adjusting Q. Row 0 is
// never deleted.
func qsDelRows(s *qs12, v uint64) {
	m := s.data
	rowPos := 1
	shifted := 0
	for v&(uint64(1)<<uint(rowPos)) == 0 && rowPos < s.nrows {
		rowPos++
	}
	for i := rowPos; i < s.nrows; i++ {
		if (v>>uint(i))&1 != 0 {
			continue
		}
		m[rowPos] = m[i]
		sh := uint(i - rowPos - shifted)
		if sh != 0 {
			mask := ((uint64(1) << uint(s.ncols)) << uint(rowPos)) - 1
			for k := 0; k < s.nrows; k++ {
				m[k] = (m[k] & mask) | ((m[k] >> sh) &^ mask)
			}
			shifted += int(sh)
		}
		rowPos++
	}
	s.nrows = rowPos
}

// qsInsertRows inserts nrows zero rows before
// row i, multiplying the state by 2**nrows.
// 1 <= i <= nrows must hold.
func qsInsertRows(s *qs12, i, nrows int) {
	if s.ncols+s.nrows+nrows > qsMaxCols {
		panic("cgt: quadratic state too large")
	}
	if i == 0 || i > s.nrows {
		panic("cgt: bad row index")
	}
	s.grow(s.nrows + nrows)
	m := s.data
	for k := s.nrows - 1; k >= i; k-- {
		m[k+nrows] = m[k]
	}
	for k := i + nrows - 1; k >= i; k-- {
		m[k] = 0
	}
	mask := ((uint64(1) << uint(s.ncols)) << uint(i)) - 1
	for k := 0; k < s.nrows+nrows; k++ {
		m[k] = (m[k] & mask) | ((m[k] &^ mask) << uint(nrows))
	}
	s.nrows += nrows
	s.reduced = false
}

// qsMulAv returns A * transpose(v) as a bit
// vector.
func qsMulAv(s *qs12, v uint64) uint64 {
	var w uint64
	m := s.data
	v &= (uint64(1) << uint(s.ncols)) - 1
	if v&(v-1) != 0 {
		for i := 0; i < s.nrows; i++ {
			w += qsParity(m[i]&v) << uint(i)
		}
	} else if v != 0 {
		sh := uint(bits.TrailingZeros64(v))
		for i := 0; i < s.nrows; i++ {
			w += ((m[i] >> sh) & 1) << uint(i)
		}
	}
	return w
}

/*************************************************************************
*** Reducing a state (qstate12.c)
*************************************************************************/

// qsSumUpKernel sums up the kernel of the
// echelonized matrix A of s.
func qsSumUpKernel(s *qs12) {
	m := s.data
	mask := (uint64(1) << uint(s.ncols)) - 1
	var delRows uint64
	oldSign := ^uint64(s.factor) & 0x80000000
	for s.nrows > 1 && m[s.nrows-1]&mask == 0 {
		n := s.nrows - 1
		if delRows&(uint64(1)<<uint(n)) != 0 {
			s.nrows--
			continue
		}
		i := qsFindPivot(m, s.nrows, s.ncols+n)
		if i <= 0 {
			if i == -1 {
				s.factor += 32
			} else {
				s.factor = 0
				s.nrows = 0
				return
			}
		} else {
			v := qsGetCol(m, s.ncols+n, i)
			qsPivot(s, i, v)
			if i == n {
				s.factor = qsAddFactors(s.factor, 0x11)
			} else {
				v = (m[i] >> uint(s.ncols)) & uint64(0xfffffffffffffffe)
				v |= (m[0] >> uint(s.ncols+i)) & 1
				qsPivot(s, n, v)
				delRows ^= uint64(1) << uint(i)
				m[i] = 0
				s.factor += 32
			}
		}
		s.nrows--
	}
	if oldSign&uint64(s.factor) != 0 {
		panic("cgt: scalar factor overflow")
	}
	if delRows != 0 {
		qsDelRows(s, delRows)
	}
}

// qsEchelonize converts s to (non-reduced)
// echelon form and sums up the kernel. Does not
// change the state.
func qsEchelonize(s *qs12) {
	if s.reduced {
		return
	}
	if s.nrows == 0 {
		s.factor = 0
		return
	}
	m := s.data
	rowPos := 1
	for col := s.ncols - 1; col >= 0; col-- {
		i := s.nrows - 1
		mask := uint64(1) << uint(col)
		var v uint64
		for i >= rowPos && m[i]&mask == 0 {
			i--
		}
		if i >= rowPos {
			if i > rowPos {
				for i1 := rowPos; i1 < i; i1++ {
					v |= ((m[i1] >> uint(col)) & 1) << uint(i1)
				}
				if v != 0 {
					qsPivot(s, i, v)
				}
				qsXchRows(s, i, rowPos)
			}
			rowPos++
			if rowPos >= s.nrows {
				break
			}
		}
	}
	qsSumUpKernel(s)
}

// qsCheckReduced sets s.reduced if s is already
// reduced and performs some easy reduction
// steps. It returns the new value of reduced.
func qsCheckReduced(s *qs12) bool {
	m := s.data
	if s.reduced {
		return true
	}
	if s.nrows <= 1 {
		if s.nrows == 0 {
			s.factor = 0
		}
		s.reduced = true
		return true
	}
	var v1, v2 uint64
	for row := 1; row < s.nrows; row++ {
		v2 |= m[row] & v1
		v1 |= m[row]
	}
	lastHi := uint64(1) << 62
	mask := (uint64(1) << uint(s.ncols)) - 1
	var msk0 uint64
	for row := 1; row < s.nrows; row++ {
		hi := m[row] & mask
		hi |= hi >> 1
		hi |= hi >> 2
		hi |= hi >> 4
		hi |= hi >> 8
		hi |= hi >> 16
		hi = (hi + 1) >> 1
		if hi >= lastHi || hi&v2 != 0 || hi == 0 {
			return false
		}
		lastHi = hi
		msk0 |= hi
	}
	if msk0&m[0] == 0 {
		s.reduced = true
		return true
	}
	for row := s.nrows - 1; row > 0; row-- {
		hi := msk0 & -msk0
		if hi&m[0] != 0 {
			if hi&m[row] == 0 {
				return false
			}
			sh := uint(s.ncols + row)
			k := ((m[0] >> sh) & 1) << 1
			k += (m[row] >> sh) & 1
			s.factor = qsAddFactors(s.factor, int64(k<<1))
			m[0] ^= m[row]
		}
		msk0 &^= hi
	}
	s.reduced = true
	return true
}

// qsReduce converts s to reduced echelon form
// and sums up the kernel. Does not change the
// state.
func qsReduce(s *qs12) {
	if qsCheckReduced(s) {
		return
	}
	m := s.data
	rowPos := 1
	for col := s.ncols - 1; col >= 0; col-- {
		i := s.nrows - 1
		mask := uint64(1) << uint(col)
		for i >= rowPos && m[i]&mask == 0 {
			i--
		}
		if i >= rowPos {
			var v uint64
			for i1 := 0; i1 < i; i1++ {
				v |= ((m[i1] >> uint(col)) & 1) << uint(i1)
			}
			if v != 0 {
				qsPivot(s, i, v)
			}
			if i > rowPos {
				qsXchRows(s, i, rowPos)
			}
			rowPos++
			if rowPos >= s.nrows {
				break
			}
		}
	}
	qsSumUpKernel(s)
	s.reduced = true
}

// qsRowTable fills rowTable so rowTable[j] = i
// if the leading bit of row i is in column j,
// else qsUndefRow. s must be echelonized.
func qsRowTable(s *qs12, rowTable []uint8) {
	for col := s.ncols - 1; col >= 0; col-- {
		rowTable[col] = qsUndefRow
	}
	if s.nrows == 0 {
		return
	}
	m := s.data
	rowPos := 1
	for col := s.ncols - 1; col >= 0; col-- {
		i := s.nrows - 1
		mask := uint64(1) << uint(col)
		for i >= rowPos && m[i]&mask == 0 {
			i--
		}
		if i >= rowPos {
			rowPos = i
			rowTable[col] = uint8(i)
			rowPos++
		}
	}
}

// qsJoinImaginary reduces s and ensures Q has at
// most one nonzero diagonal entry, in row 1. It
// returns 1 if that entry is present, else 0.
func qsJoinImaginary(s *qs12) int {
	qsReduce(s)
	m := s.data
	M := uint64(1) << uint(s.ncols)
	row := 0
	i := s.nrows - 1
	for ; i > 0; i-- {
		if m[i]&(M<<uint(i)) != 0 {
			row = i
			break
		}
	}
	if row == 0 {
		return 0
	}
	i--
	var v uint64
	for ; i > 0; i-- {
		v |= m[i] & (M << uint(i))
	}
	v >>= uint(s.ncols)
	qsPivot(s, row, v)
	qsPivotRotRows(s, 1, row)
	return 1
}

/*************************************************************************
*** Extending and restricting a state (qstate12.c)
*************************************************************************/

// qsExtendZero inserts nqb zero qubits at
// position j, set to 0.
func qsExtendZero(s *qs12, j, nqb int) {
	if j > s.ncols {
		panic("cgt: qubit index out of range")
	}
	if s.ncols+nqb+s.nrows > qsMaxCols {
		panic("cgt: quadratic state too large")
	}
	m := s.data
	mask := (uint64(1) << uint(j)) - 1
	s.ncols += nqb
	s.shape1 = 0
	for k := 0; k < s.nrows; k++ {
		m[k] = (m[k] & mask) | ((m[k] &^ mask) << uint(nqb))
	}
}

// qsExtend inserts nqb qubits at position j.
func qsExtend(s *qs12, j, nqb int) {
	qsExtendZero(s, j, nqb)
	if s.nrows == 0 {
		return
	}
	s.reduced = false
	i := s.nrows
	qsInsertRows(s, i, nqb)
	m := s.data
	mask := uint64(1) << uint(j)
	for k := 0; k < nqb; k++ {
		m[i+k] ^= mask << uint(k)
	}
}

// qsSumCols sums up nqb qubits at position j,
// decrementing ncols by nqb.
func qsSumCols(s *qs12, j, nqb int) {
	if nqb+j > s.ncols {
		panic("cgt: qubit index out of range")
	}
	m := s.data
	mask := (uint64(1) << uint(j)) - 1
	s.ncols -= nqb
	s.shape1 = 0
	s.reduced = false
	for k := 0; k < s.nrows; k++ {
		m[k] = (m[k] & mask) | ((m[k] >> uint(nqb)) &^ mask)
	}
}

// qsRestrictZero restricts nqb qubits at
// position j to 0, keeping the shape.
func qsRestrictZero(s *qs12, j, nqb int) {
	if nqb+j > s.ncols {
		panic("cgt: qubit index out of range")
	}
	if s.nrows == 0 {
		return
	}
	m := s.data
	var deleted uint64
	for colPos := j; colPos < j+nqb; colPos++ {
		i := qsFindPivot(m, s.nrows, colPos)
		if i > 0 {
			v := qsGetCol(m, colPos, i)
			qsPivot(s, i, v)
			m[i] = 0
			deleted |= uint64(1) << uint(i)
		} else if i == 0 {
			s.nrows = 0
			return
		}
	}
	qsDelRows(s, deleted)
}

// qsRestrict restricts nqb qubits at position j
// to 0 and deletes them.
func qsRestrict(s *qs12, j, nqb int) {
	qsRestrictZero(s, j, nqb)
	s.reduced = false
	qsSumCols(s, j, nqb)
}

/*************************************************************************
*** Applying gates to a state (qstate12.c)
*************************************************************************/

// qsGateNot applies a not gate: qs1(x) = qs(x^v).
func qsGateNot(s *qs12, v uint64) {
	if s.nrows == 0 {
		return
	}
	s.data[0] ^= v & ((uint64(1) << uint(s.ncols)) - 1)
	s.reduced = false
}

// qsGateCtrlNot applies a controlled not gate:
// qs1(x) = qs(x ^ <vc,x>*v).
func qsGateCtrlNot(s *qs12, vc, v uint64) {
	m := s.data
	v &= (uint64(1) << uint(s.ncols)) - 1
	if qsParity(v&vc) != 0 {
		panic("cgt: invalid ctrl-not gate")
	}
	wc := qsMulAv(s, vc)
	s.reduced = false
	if wc != 0 {
		for i := 0; i < s.nrows; i++ {
			m[i] ^= -((wc >> uint(i)) & 1) & v
		}
	}
}

// qsGatePhi applies a phase gate:
// qs1(x) = qs(x) * sqrt(-1)**(phi*<v,x>).
func qsGatePhi(s *qs12, v uint64, phi int) {
	m := s.data
	c := uint64(1) << uint(s.ncols)
	w := qsMulAv(s, v)
	if w == 0 {
		return
	}
	wsh := w << uint(s.ncols)
	if phi&1 != 0 {
		s.factor = qsAddFactors(s.factor, int64((w&1)<<1))
		m[0] ^= wsh & -(w & 1) &^ c
		for i := 1; i < s.nrows; i++ {
			m[0] ^= wsh & m[i] & (c << uint(i))
			m[i] ^= wsh & -((w >> uint(i)) & 1)
		}
	}
	if phi&2 != 0 {
		s.factor ^= int64((w & 1) << 2)
		m[0] ^= wsh &^ c
	}
}

// qsGateCtrlPhi applies a controlled phase gate:
// qs1(x) = qs(x) * (-1)**(<v1,x>*<v2,x>).
func qsGateCtrlPhi(s *qs12, v1, v2 uint64) {
	m := s.data
	w1 := qsMulAv(s, v1)
	w2 := qsMulAv(s, v2)
	w1sh := (w1 &^ 1) << uint(s.ncols)
	w2sh := (w2 &^ 1) << uint(s.ncols)
	s.factor ^= int64((w1 & w2 & 1) << 2)
	m[0] ^= (w1sh & -(w2 & 1)) ^ (w2sh & -(w1 & 1)) ^ (w1sh & w2sh)
	for i := 1; i < s.nrows; i++ {
		m[i] ^= (w1sh & -((w2 >> uint(i)) & 1)) ^ (w2sh & -((w1 >> uint(i)) & 1))
	}
}

// qsGateH applies Hadamard gates to all qubits j
// with bit j of v set.
func qsGateH(s *qs12, v uint64) {
	if s.nrows == 0 {
		return
	}
	maxRows := 2*s.ncols + 2
	if qsMaxRows-1 < maxRows {
		maxRows = qsMaxRows - 1
	}
	if qsMaxCols-s.ncols-1 < maxRows {
		maxRows = qsMaxCols - s.ncols - 1
	}
	for j := 0; j < s.ncols; j++ {
		if v&(uint64(1)<<uint(j)) == 0 {
			continue
		}
		s.reduced = false
		if s.nrows >= maxRows {
			qsEchelonize(s)
			if s.nrows >= maxRows {
				panic("cgt: quadratic state buffer overflow")
			}
		}
		s.grow(s.nrows + 1)
		m := s.data
		var w uint64
		sh := uint(s.nrows + s.ncols)
		mask1 := (uint64(1) << sh) - (uint64(1) << uint(j)) - 1
		for i := 0; i < s.nrows; i++ {
			c := (m[i] >> uint(j)) & 1
			m[i] = (m[i] & mask1) | (c << sh)
			w |= c << uint(i)
		}
		m[s.nrows] = (uint64(1) << uint(j)) + (w << uint(s.ncols))
		s.nrows++
		s.factor -= 0x10
	}
}

/*************************************************************************
*** Scalar factor conversion (qstate12.c)
*************************************************************************/

// factorToComplex converts a scalar factor to a
// complex number 2**(exp/2) * exp(phi*pi*i/4).
func factorToComplex(factor int64) complex128 {
	phi := factor & 7
	exp := factor >> 4
	if factor&8 != 0 {
		return 0
	}
	phases := [8][2]float64{
		{1, 0}, {1, 1}, {0, 1}, {-1, 1},
		{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
	}
	exp -= phi & 1
	f := 1.0
	if exp&1 != 0 {
		f = math.Sqrt2
	}
	f = math.Ldexp(f, int(exp>>1))
	var re, im float64
	if phases[phi][0] != 0 {
		re = math.Copysign(f, phases[phi][0])
	}
	if phases[phi][1] != 0 {
		im = math.Copysign(f, phases[phi][1])
	}
	return complex(re, im)
}

// factorToInt32 converts a scalar factor to an
// integer. It panics if the factor is not an
// integer.
func factorToInt32(factor int64) int64 {
	if factor&8 != 0 {
		return 0
	}
	if factor < 0 || factor&0x13 != 0 {
		panic("cgt: scalar factor is not an integer")
	}
	if factor >= (62 << 4) {
		panic("cgt: scalar factor overflow")
	}
	pi := int64(1) << uint(factor>>5)
	if factor&4 != 0 {
		pi = -pi
	}
	return pi
}

/*************************************************************************
*** Traversing the support of a state (qstate12io.c)
*************************************************************************/

// qstate12_lsbtab[i] is the position of the
// least significant bit of i|0x40.
var qsLsbTab = [64]uint8{
	6, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0,
	4, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0,
	5, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0,
	4, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0,
}

// qsSupport iterates the nonzero entries of a
// reduced state in batches, mirroring
// qstate12_support_type.
type qsSupport struct {
	size          int
	weight        int
	factor        int64
	factorNew     bool
	batchLength   int
	nBatches      int
	indices       [64]uint32
	signs         [64]uint8
	lbBatchLength int
	qs            *qs12
	qf            uint64
	assoc         uint64
	i             int
	cWeight       int
}

// qsSupportInit initializes a support iterator
// from a state.
func qsSupportInit(s *qs12) *qsSupport {
	qsReduce(s)
	if s.ncols > 30 || s.nrows > 31 {
		panic("cgt: quadratic state too large for support iteration")
	}
	sup := &qsSupport{qs: s.copy()}
	pqs := sup.qs
	sup.size = 1 << uint(pqs.ncols)
	sup.i = 0
	if pqs.nrows == 0 {
		sup.cWeight = 0
		sup.weight = 0
		sup.factor = 0
		sup.batchLength = 0
		sup.nBatches = 1
		return sup
	}
	sup.cWeight = 1 << uint(pqs.nrows-1)
	sup.weight = sup.cWeight
	if pqs.nrows < 7 {
		sup.lbBatchLength = pqs.nrows - 1
	} else {
		sup.lbBatchLength = 6
	}
	status := qsJoinImaginary(pqs)
	if status > 0 {
		if sup.lbBatchLength == pqs.nrows-1 {
			sup.lbBatchLength--
		}
		sup.cWeight >>= 1
	}
	sup.batchLength = 1 << uint(sup.lbBatchLength)
	sup.nBatches = 1 << uint(pqs.nrows-1-sup.lbBatchLength)
	sup.factor = pqs.factor
	sup.assoc = pqs.data[0]
	sup.qf = 0
	if sup.factor&4 != 0 {
		sup.factor ^= 4
		sup.qf ^= 1
	}
	return sup
}

// qsSupportNext reads the next batch of nonzero
// entries. It returns the batch length.
func qsSupportNext(sup *qsSupport) int {
	pqs := sup.qs
	sup.factorNew = false
	if sup.i&^sup.cWeight == 0 {
		sup.factorNew = true
		if sup.i == sup.weight || sup.batchLength == 0 {
			sup.batchLength = 0
			return 0
		}
		if sup.i == sup.cWeight {
			sup.factor = (sup.factor + 2) &^ 8
		}
	}
	mEnd := pqs.nrows - 1
	m := pqs.data
	mask := (uint64(1) << uint(pqs.ncols)) - 1
	nrc := uint(pqs.ncols + pqs.nrows - 1)
	assoc := sup.assoc
	qf := sup.qf & 1
	i := 0
	for {
		sup.signs[i] = uint8(qf)
		sup.indices[i] = uint32(assoc & mask)
		i++
		if i == sup.batchLength {
			break
		}
		d := uint(qsLsbTab[i])
		qf ^= (assoc >> (nrc - d)) & 1
		assoc ^= m[mEnd-int(d)]
	}
	sup.i += sup.batchLength
	d := sup.lbBatchLength
	ii := sup.i >> uint(d)
	for {
		d1 := int(qsLsbTab[ii&63])
		d += d1
		if d1 < 6 {
			break
		}
		ii >>= uint(d1)
		if ii == 0 {
			panic("cgt: support iteration overran state")
		}
	}
	sup.qf = (qf + (assoc >> (nrc - uint(d)))) & 1
	assoc ^= m[mEnd-d]
	sup.assoc = assoc
	return sup.batchLength
}

// qsSupportAdjustReal makes the support factor
// positive and panics if any entry is not real.
func qsSupportAdjustReal(sup *qsSupport) {
	if sup.factor&0x3 != 0 || sup.cWeight != sup.weight {
		panic("cgt: matrix is not real")
	}
	if sup.factor&0x4 != 0 {
		sup.factor ^= 0x4
		sup.qf ^= 0x1
	}
}

/*************************************************************************
*** Converting a state to complex numbers (qstate12io.c)
*************************************************************************/

// qsComplex expands s to its 2**ncols complex
// entries, indexed by bit vector. It reduces s.
func qsComplex(s *qs12) []complex128 {
	qsReduce(s)
	ncols := s.ncols
	out := make([]complex128, 1<<uint(ncols))
	if s.nrows == 0 {
		return out
	}
	m := s.data
	assoc := m[0]
	mEnd := s.nrows - 1
	nrc := uint(ncols + s.nrows - 1)
	nIter := 1 << uint(s.nrows-1)
	mask := (uint64(1) << uint(ncols)) - 1
	base := factorToComplex(int64(uint64(s.factor) & factorMask))
	freal := [4]complex128{base, base * 1i, -base, base * -1i}
	var qf uint64
	for i := 1; i <= nIter; i++ {
		index := assoc & mask
		out[index] = freal[qf&3]
		i1 := i
		d := int(qsLsbTab[i1&63])
		d1 := d
		for d1 == 6 {
			i1 >>= 6
			d1 = int(qsLsbTab[i1&63])
			d += d1
		}
		diag := (m[mEnd-d] >> (nrc - uint(d))) & 1
		qf += ((assoc >> (nrc - 1 - uint(d))) & 2) + diag
		assoc ^= m[mEnd-d]
	}
	return out
}

/*************************************************************************
*** Converting signs of a real state (qstate12io.c)
*************************************************************************/

// subbatchLength returns the sub-batch length
// for sign extraction.
func subbatchLength(sup *qsSupport) int {
	if sup.batchLength < 2 {
		return sup.batchLength
	}
	pqs := sup.qs
	m := pqs.data
	mEnd := pqs.nrows - 1
	mMid := mEnd
	mask := ((uint64(1) << uint(pqs.ncols)) - 1) &^ 0x1f
	for mMid > 0 && m[mMid]&mask == 0 {
		mMid--
	}
	dMid := mEnd - mMid
	batchLength := 1 << uint(dMid)
	if batchLength > sup.batchLength {
		panic("cgt: inconsistent quadratic state")
	}
	return batchLength
}

// qsToSigns stores the signs of a real state in
// bmap: t = 0,1,3 for zero, positive, negative
// entries packed two bits per entry.
func qsToSigns(s *qs12) []uint64 {
	sup := qsSupportInit(s)
	qsSupportAdjustReal(sup)
	subLen := subbatchLength(sup)
	nOut := sup.size >> 5
	if nOut == 0 {
		nOut++
	}
	bmap := make([]uint64, nOut)
	if subLen == 0 {
		return bmap
	}
	var addr uint32
	for i := 0; i < sup.nBatches; i++ {
		qsSupportNext(sup)
		for j := 0; j < sup.batchLength; j += subLen {
			var v uint64
			for k := 0; k < subLen; k++ {
				addr = sup.indices[j+k]
				sign := uint64(sup.signs[j+k])
				sh := uint((addr & 0x1f) << 1)
				v |= (uint64(1) << sh) | (sign << (sh + 1))
			}
			bmap[addr>>5] = v
		}
	}
	return bmap
}

// qsCompareSigns reports whether the signs of a
// real state s match bmap.
func qsCompareSigns(s *qs12, bmap []uint64) bool {
	defer func() { recover() }()
	sup := qsSupportInit(s)
	if sup.factor&0x3 != 0 || sup.cWeight != sup.weight {
		return false
	}
	if sup.factor&0x4 != 0 {
		sup.factor ^= 0x4
		sup.qf ^= 0x1
	}
	subLen := subbatchLength(sup)
	nOut := sup.size >> 5
	if nOut == 0 {
		nOut++
	}
	var mskOut uint64
	if sup.size < 32 {
		mskOut = uint64(1) << uint(2*sup.size)
	}
	mskOut--
	if subLen == 0 {
		for i := 0; i < nOut; i++ {
			if bmap[i]&mskOut != 0 {
				return false
			}
		}
		return true
	}
	nonzero := make([]uint64, (nOut>>6)+1)
	for i := 0; i < sup.nBatches; i++ {
		qsSupportNext(sup)
		for j := 0; j < sup.batchLength; j += subLen {
			var v uint64
			var addr uint32
			for k := 0; k < subLen; k++ {
				addr = sup.indices[j+k]
				sign := uint64(sup.signs[j+k])
				sh := uint((addr & 0x1f) << 1)
				v |= (uint64(1) << sh) | (sign << (sh + 1))
			}
			index := int(addr >> 5)
			if (bmap[index]^v)&mskOut != 0 {
				return false
			}
			nonzero[index>>6] |= uint64(1) << uint(index&0x3f)
		}
	}
	if nOut > 64 {
		for i := 0; i < nOut; i += 64 {
			v := nonzero[i>>6]
			for j := 0; j < 64 && i+j < nOut; j++ {
				if (v>>uint(j))&1 == 0 && bmap[i+j] != 0 {
					return false
				}
			}
		}
	} else {
		v := nonzero[0]
		for j := 0; j < nOut; j++ {
			if (v>>uint(j))&1 == 0 && bmap[j] != 0 {
				return false
			}
		}
	}
	return true
}

// loadBmap reads the two-bit sign code for index
// i from bmap.
func loadBmap(bmap []uint64, i uint64) uint64 {
	return bmap[i>>5] >> ((i & 31) << 1)
}

// scanAffine finds an affine subspace covering
// the nonzero indices of bmap, encoded in s.
func scanAffine(bmap []uint64, n int, s *qs12) {
	maxlen := 1 << uint(n)
	s.nrows = 0
	s.ncols = n
	s.shape1 = 0
	s.factor = 0
	s.reduced = false
	if n >= qsMaxRows || n > 30 {
		panic("cgt: quadratic state buffer overflow")
	}
	s.grow(maxlen + 1)
	m := s.data
	m0 := uint64(Bm64FindLowBit(bmap, 0, 2*maxlen) >> 1)
	if m0 >= uint64(maxlen) {
		s.nrows = 0
		return
	}
	rows := 1
	var mask uint64
	for mask = 1; mask <= m0; mask <<= 1 {
		if mask&m0 == 0 {
			imin := int((m0 & -mask) + mask)
			imax := imin + int(mask)
			imin = Bm64FindLowBit(bmap, imin<<1, imax<<1) >> 1
			if imin < imax {
				m[rows] = uint64(imin) ^ m0
				rows++
			}
		}
	}
	for ; mask < uint64(1)<<uint(n); mask <<= 1 {
		imax := int(mask << 1)
		imin := Bm64FindLowBit(bmap, int(mask)<<1, imax<<1) >> 1
		if imin < imax {
			m[rows] = uint64(imin) ^ m0
			rows++
		}
	}
	m[0] = m0 | uint64(maxlen)
	Bm64EchelonH(m, rows, n+1, n+1)
	s.nrows = rows
	m[0] &^= uint64(maxlen)
}

// fillAffine fills the Q part of s from the
// signs in bmap. It returns false if bmap is not
// a quadratic state vector.
func fillAffine(bmap []uint64, s *qs12) bool {
	m := s.data
	ncols1 := uint64(s.ncols)
	nrows := s.nrows
	amask := (uint64(1) << uint(s.ncols)) - 1
	bmapAcc := uint64(1)
	if nrows == 0 {
		return true
	}
	m0 := m[0] & amask
	index := m0
	sumIndex := m0
	entry := loadBmap(bmap, index)
	bmapAcc &= entry
	sign := entry & 2
	sumEntry := sign
	s.factor |= int64(sign << 1)
	var row0 uint64
	signRow0 := -(sign >> 1)
	if ncols1 == 0 {
		if bmapAcc != 0 {
			return true
		}
		s.factor = 0
		return false
	}
	ncols1--
	for i := 1; i < nrows; i++ {
		index = (m0 ^ m[i]) & amask
		entry = loadBmap(bmap, index)
		sumIndex = (sumIndex ^ m[i]) & amask
		bmapAcc &= entry
		entry = (entry ^ sign) & 2
		sumEntry ^= entry
		m[i] |= entry << ncols1
		entry <<= uint(i)
		row0 |= entry
		signRow0 ^= entry
		for j := 1; j < i; j++ {
			index = (m0 ^ m[i] ^ m[j]) & amask
			entry = loadBmap(bmap, index)
			bmapAcc &= entry
			entry ^= (row0 >> uint(i)) ^ (signRow0 >> uint(j))
			entry &= 2
			m[i] |= entry << (ncols1 + uint64(j))
			m[j] |= entry << (ncols1 + uint64(i))
			sumEntry ^= entry
		}
		entry = loadBmap(bmap, sumIndex&amask) & 3
		entry ^= sumEntry & 2
		entry &= 2 | bmapAcc
		if entry != 1 {
			s.nrows = 0
			return false
		}
	}
	m[0] |= row0 << ncols1
	return true
}

/*************************************************************************
*** Multiplying states (qmatrix12.c)
*************************************************************************/

// qsCopyRow copies row i1 to row i2 (i2 <= i1)
// adjusting Q.
func qsCopyRow(m []uint64, ncols, nrows, i1, i2 int) {
	if i2 < i1 {
		m[i2] = m[i1]
		mask := uint64(1) << uint(ncols+i2)
		sh := uint(i1 - i2)
		for k := 0; k < nrows; k++ {
			m[k] = (m[k] &^ mask) | ((m[k] >> sh) & mask)
		}
	}
}

// qsPrepMul prepares s1 and s2 for matrix
// multiplication summing over the first nqb
// qubits. It returns rowPos.
func qsPrepMul(s1, s2 *qs12, nqb int) int {
	qsReduce(s1)
	qsReduce(s2)
	if nqb > s1.ncols || nqb > s2.ncols {
		panic("cgt: qubit index out of range")
	}
	if s1.nrows == 0 || s2.nrows == 0 {
		s1.nrows = 0
		s2.nrows = 0
		return 0
	}
	s1.reduced = false
	s2.reduced = false
	m1 := s1.data
	m2 := s2.data
	nDeleted := 0
	rowPos := 1
	rowPos1 := 1
	rowPos2 := 1
	var deleted uint64
	minRow := 0

	col1 := s1.ncols - nqb
	col2 := s2.ncols - nqb
	ii := s1.nrows
	if s2.nrows < ii {
		ii = s2.nrows
	}
	mask := (uint64(1) << uint(nqb)) - 1
	normal := false
	if ((m1[0]>>uint(col1))^(m2[0]>>uint(col2)))&mask == 0 {
		for rowPos = 1; rowPos < ii; rowPos++ {
			v := (m1[rowPos] >> uint(col1)) & mask
			v2 := (m2[rowPos] >> uint(col2)) & mask
			if v^v2 != 0 || v == 0 || v2 == 0 {
				if v == 0 && v2 == 0 {
					return rowPos
				}
				minRow = rowPos
				rowPos1 = rowPos
				rowPos2 = rowPos
				normal = true
				break
			}
		}
		if !normal {
			var v, v2 uint64
			if s1.nrows != ii {
				v = (m1[ii] >> uint(col1)) & mask
			}
			if s2.nrows != ii {
				v2 = (m2[ii] >> uint(col2)) & mask
			}
			if v == 0 && v2 == 0 {
				return ii
			}
			minRow = ii
			rowPos1 = ii
			rowPos2 = ii
		}
	}

	for colPos := 1; colPos <= nqb; colPos++ {
		col1 = s1.ncols - colPos
		col2 = s2.ncols - colPos
		var i1, i2 uint64
		if rowPos1 < s1.nrows {
			i1 = (m1[rowPos1] >> uint(col1)) & 1
		}
		if rowPos2 < s2.nrows {
			i2 = (m2[rowPos2] >> uint(col2)) & 1
		}
		var v uint64
		if i1 != 0 {
			if i2 != 0 {
				qsCopyRow(m1, s1.ncols, s1.nrows, rowPos1, rowPos)
				rowPos1++
				qsCopyRow(m2, s2.ncols, s2.nrows, rowPos2, rowPos)
				rowPos2++
				rowPos++
			} else {
				for k := minRow; k < rowPos; k++ {
					v |= ((m2[k] >> uint(col2)) & 1) << uint(k)
				}
				qsPivot(s1, rowPos1, v)
				rowPos1++
			}
		} else {
			if i2 != 0 {
				for k := minRow; k < rowPos; k++ {
					v |= ((m1[k] >> uint(col1)) & 1) << uint(k)
				}
				qsPivot(s2, rowPos2, v)
				rowPos2++
			} else {
				i := rowPos - 1
				for k := minRow; k < rowPos; k++ {
					v |= (((m1[k] >> uint(col1)) ^ (m2[k] >> uint(col2))) & 1) << uint(k)
				}
				for i >= minRow && (uint64(1)<<uint(i))&v == 0 {
					i--
				}
				if i >= minRow {
					if i == 0 {
						s1.nrows = 0
						s2.nrows = 0
						return 0
					}
					qsPivot(s1, i, v)
					qsPivot(s2, i, v)
					deleted |= uint64(1) << uint(i)
					nDeleted++
					m1[i] = 0
					m2[i] = 0
				}
			}
		}
	}

	v := deleted + (uint64(1) << uint(rowPos1)) - (uint64(1) << uint(rowPos))
	qsDelRows(s1, v)
	v = deleted + (uint64(1) << uint(rowPos2)) - (uint64(1) << uint(rowPos))
	qsDelRows(s2, v)
	rowPos -= nDeleted
	return rowPos
}

// qsShiftA extracts columns 0..n-1 of A and
// inserts iLo low and iHi high zero columns.
func qsShiftA(s *qs12, n, iLo, iHi int) {
	if n > s.ncols {
		panic("cgt: qubit index out of range")
	}
	shlQ := n + iLo + iHi
	shrQ := uint(s.ncols)
	if shlQ+s.nrows > qsMaxCols {
		panic("cgt: quadratic state too large")
	}
	m := s.data
	maskA := (uint64(1) << uint(n)) - 1
	maskQ := ((uint64(1) << uint(s.nrows)) - 1) & ^uint64(1)
	for i := 0; i < s.nrows; i++ {
		m[i] = ((m[i] & maskA) << uint(iLo)) + (((m[i] >> shrQ) & maskQ) << uint(shlQ))
	}
	s.ncols = shlQ
	s.shape1 = 0
}

// qsMulElements combines s1 and s2 by adding
// their A and Q parts; result stored in s1.
func qsMulElements(s1, s2 *qs12, rowPos int) {
	c := uint64(1) << uint(s1.ncols)
	mask := (uint64(1) << uint(s2.ncols+s2.nrows)) - 1
	if rowPos > s2.nrows {
		panic("cgt: bad row index")
	}
	qsInsertRows(s1, rowPos, s2.nrows-rowPos)
	m1 := s1.data
	m2 := s2.data
	s1.reduced = false
	var v uint64
	for k := 1; k < rowPos; k++ {
		m2m := m2[k] & mask
		v ^= m1[k] & m2m & (c << uint(k))
		m1[k] ^= m2m
	}
	for k := rowPos; k < s2.nrows; k++ {
		m1[k] = m2[k] & mask
	}
	m1[0] ^= m2[0] ^ v
	s1.factor = qsAddFactors(s1.factor, s2.factor)
}

// qsProductInto computes a product of s1 and s2
// in place in s1, destroying s2.
func qsProductInto(s1, s2 *qs12, nqb, nc int) {
	rowPos := qsPrepMul(s1, s2, nqb)
	if nc > nqb {
		panic("cgt: qubit index out of range")
	}
	cols1 := s1.ncols - nc
	cols2 := s2.ncols - nqb
	qsShiftA(s1, cols1, cols2, 0)
	qsShiftA(s2, cols2, 0, cols1)
	if s1.nrows == 0 || s2.nrows == 0 {
		s1.nrows = 0
		s1.factor = 0
		return
	}
	qsMulElements(s1, s2, rowPos)
	qsReduce(s1)
}

// qsProduct returns a product of s1 and s2
// without modifying either.
func qsProduct(s1, s2 *qs12, nqb, nc int) *qs12 {
	a := s1.copy()
	b := s2.copy()
	qsProductInto(a, b, nqb, nc)
	return a
}

// qsMatT transposes s in place.
func qsMatT(s *qs12) {
	nqb := s.ncols - s.shape1
	s.shape1 = nqb
	Bm64RotBits(s.data, s.nrows, nqb, s.ncols, 0)
	s.reduced = false
}

// qsConjugate conjugates s in place.
func qsConjugate(s *qs12) {
	m := s.data
	c := uint64(1) << uint(s.ncols)
	for k := 1; k < s.nrows; k++ {
		m[0] ^= m[k] & (c << uint(k))
	}
	s.factor = int64((((uint64(s.factor) & factorMask) ^ 7) + 1) & factorMask)
}

// qsMatmul returns the matrix product s1 @ s2.
func qsMatmul(s1, s2 *qs12) *qs12 {
	nqb := s1.shape1
	cols := s2.shape1
	if s2.ncols-s2.shape1 != nqb {
		panic("cgt: matrix shape mismatch")
	}
	a := s1.copy()
	b := s2.copy()
	Bm64RotBits(a.data, a.nrows, -nqb, a.ncols, 0)
	a.reduced = false
	qsProductInto(a, b, nqb, nqb)
	a.shape1 = cols
	return a
}

/*************************************************************************
*** Pauli group (qmatrix12.c)
*************************************************************************/

// bitRev reverses the lower length bits of n.
func bitRev(length int, n uint64) uint64 {
	var v uint64
	for i := 0; i < length; i++ {
		v |= ((n >> uint(length-i-1)) & 1) << uint(i)
	}
	return v
}

// qsStdMatrix sets s to a 2**rows x 2**cols
// matrix with rk diagonal ones.
func qsStdMatrix(s *qs12, rows, cols, rk int) {
	s.nrows = rk + 1
	s.ncols = rows + cols
	s.shape1 = cols
	s.factor = 0
	s.grow(rk + 1)
	s.data[0] = 0
	if s.nrows+s.ncols > qsMaxCols || s.nrows > qsMaxRows || s.shape1 > s.ncols {
		panic("cgt: quadratic state too large")
	}
	if rk > rows || rk > cols {
		panic("cgt: qubit index out of range")
	}
	mask := ((uint64(1) << uint(cols)) + 1) << uint(rk-1)
	for i := 1; i < s.nrows; i++ {
		s.data[i] = mask
		mask >>= 1
	}
	s.reduced = true
}

// qsUnitMatrix sets s to a 2**nqb x 2**nqb unit
// matrix.
func qsUnitMatrix(s *qs12, nqb int) {
	qsStdMatrix(s, nqb, nqb, nqb)
}

// qsPauliMatrix sets s to the Pauli group
// element v of nqb qubits.
func qsPauliMatrix(s *qs12, nqb int, v uint64) {
	qsStdMatrix(s, nqb, nqb, nqb)
	m := s.data
	mask := (uint64(1) << uint(nqb)) - 1
	m[0] |= bitRev(nqb, v) << uint(2*nqb+1)
	m[0] |= v & (mask << uint(nqb))
	v >>= uint(2 * nqb)
	s.reduced = false
	s.factor |= int64((v & 1) << 2)
	s.factor |= int64(v & 2)
}

// qsPauliVector returns nqb and the encoded
// Pauli vector of s. It panics if s is not in
// the Pauli group.
func qsPauliVector(s *qs12) (int, uint64) {
	qsReduce(s)
	nqb := s.shape1
	m := s.data
	mask := (uint64(1) + (uint64(1) << uint(nqb))) << uint(nqb-1)
	if s.ncols != nqb<<1 || s.nrows != nqb+1 {
		panic("cgt: matrix is not in the Pauli group")
	}
	if uint32(s.factor)&(^uint32(0xe)) != 0 {
		panic("cgt: matrix is not in the Pauli group")
	}
	var w uint64
	for i := 0; i < nqb; i++ {
		w |= m[i+1] ^ mask
		mask >>= 1
	}
	chk := (((uint64(1) << uint(s.nrows)) - 1) << uint(s.ncols)) - 1
	if w&chk != 0 {
		panic("cgt: matrix is not in the Pauli group")
	}
	colMask := (uint64(1) << uint(nqb)) - 1
	w = bitRev(nqb, m[0]>>uint(s.ncols+1))
	w |= (m[0] & colMask) << uint(nqb)
	w |= ((uint64(s.factor>>2) & 1) ^ qsParity(w&m[0]&colMask)) << uint(s.ncols)
	w |= (uint64(s.factor>>1) & 1) << uint(s.ncols+1)
	return nqb, w
}

// qsPauliVectorMul returns the product v1*v2 of
// two Pauli vectors.
func qsPauliVectorMul(nqb int, v1, v2 uint64) uint64 {
	if nqb >= qsMaxCols/2 {
		return v1 ^ v2
	}
	s := (v1 & (v2 >> uint(nqb))) & ((uint64(1) << uint(nqb)) - 1)
	s ^= ((v1 & v2) >> uint(2*nqb+1)) & 1
	s = qsParity(s)
	return (v1 ^ v2 ^ (s << uint(nqb<<1))) & (((4 * uint64(1)) << uint(2*nqb)) - 1)
}

// qsPauliVectorExp returns the power v**e of a
// Pauli vector.
func qsPauliVectorExp(nqb int, v uint64, e int) uint64 {
	var s uint64
	if e&2 != 0 && nqb < qsMaxCols/2 {
		s = (v & (v >> uint(nqb))) & ((uint64(1) << uint(nqb)) - 1)
		s ^= (v >> uint(2*nqb+1)) & 1
		s = qsParity(s)
		s <<= uint(nqb << 1)
	}
	s ^= -(uint64(e) & 1) & v
	return s & (((4 * uint64(1)) << uint(2*nqb)) - 1)
}

/*************************************************************************
*** Special reduction and rank (qmatrix12.c)
*************************************************************************/

// qsFindMaskedPivot returns the highest row i
// with A[i,j] = 1 and bit i of mask cleared, or
// -1.
func qsFindMaskedPivot(m []uint64, nrows, j int, mask uint64) int {
	mask = ^mask
	i := nrows - 1
	for i >= 0 && (m[i]>>uint(j))&(mask>>uint(i))&1 == 0 {
		i--
	}
	return i
}

// reduceMatrix converts a reduced state to
// reduced matrix representation, filling
// rowTable.
func reduceMatrix(s *qs12, rowTable []uint8) {
	n1 := s.shape1
	n0 := s.ncols - n1
	qsRowTable(s, rowTable)
	if s.nrows == 0 {
		return
	}
	m := s.data
	s.reduced = false
	fstRow := s.nrows
	v := ((uint64(1) << uint(n0)) - 1) << uint(n1)
	for i := s.nrows - 1; i > 0; i-- {
		rowTable[s.ncols+i] = qsUndefRow
		if m[i]&v == 0 {
			fstRow = i
		}
	}
	kernel := -(uint64(1) << uint(fstRow))
	rowTable[s.ncols] = uint8(fstRow)
	for j := n1 - 1; j >= 0; j-- {
		if rowTable[j] == qsUndefRow {
			i := qsFindMaskedPivot(m, fstRow, j, kernel)
			if i > 0 {
				kernel |= uint64(1) << uint(i)
				v = qsGetCol(m, j, i) &^ kernel
				qsPivot(s, i, v)
				rowTable[j] = uint8(i)
				rowTable[s.ncols+i] = uint8(j)
			}
		}
	}
	for j := s.nrows - 1; j >= fstRow; j-- {
		i := qsFindMaskedPivot(m, fstRow, j+s.ncols, kernel)
		if i > 0 {
			v = ((m[j] >> uint(s.ncols)) & ^uint64(1)) + ((m[0] >> uint(j+s.ncols)) & 1)
			v &^= kernel
			qsPivot(s, i, v)
			kernel |= uint64(1) << uint(i)
			rowTable[s.ncols+j] = uint8(i)
		}
	}
}

// lbRankReduced returns the binary logarithm of
// the rank of a state reduced with reduceMatrix.
func lbRankReduced(s *qs12, rowTable []uint8) int {
	nqb := s.shape1
	if s.nrows == 0 {
		return -1
	}
	rk := 0
	fstRow := int(rowTable[s.ncols])
	for i := 0; i < nqb; i++ {
		if int(rowTable[i]) < fstRow {
			rk++
		}
	}
	for i := s.ncols + fstRow; i < s.ncols+s.nrows; i++ {
		if rowTable[i] != qsUndefRow {
			rk++
		}
	}
	return rk
}

// qsMatLbRank returns the binary logarithm of
// the rank of s (-1 if zero). It reduces s.
func qsMatLbRank(s *qs12) int {
	qsReduce(s)
	q := s.copy()
	rowTable := make([]uint8, qsMaxCols+4)
	reduceMatrix(q, rowTable)
	return lbRankReduced(q, rowTable)
}

// qsMatInv inverts s in place. It panics if s is
// not invertible.
func qsMatInv(s *qs12) {
	nqb := s.shape1
	qsMatT(s)
	qsConjugate(s)
	qsReduce(s)
	rk := qsMatLbRank(s)
	if 2*nqb != s.ncols || rk != nqb {
		panic("cgt: matrix is not invertible")
	}
	f := (s.factor & -16) >> 4
	f += int64(s.nrows - 1)
	f -= int64(s.ncols - nqb)
	qsMulScalar(s, int(-2*f), 0)
}

/*************************************************************************
*** Symplectic and Pauli conjugation (qmatrix12.c)
*************************************************************************/

// qsToSymplectic returns the 2k x 2k symplectic
// bit matrix of the conjugation action of s on
// the Pauli group. It panics if s is not an
// invertible (k,k) matrix.
func qsToSymplectic(s *qs12) []uint64 {
	qsReduce(s)
	k := s.shape1
	if 2*k != s.ncols || s.nrows <= k {
		panic("cgt: matrix is not invertible")
	}
	dRows := s.nrows - 1
	if k > qsMaxCols/3 || dRows > 2*qsMaxCols/3 {
		panic("cgt: quadratic state too large")
	}
	pA := make([]uint64, 2*k)
	if k == 0 {
		return pA
	}
	m := make([]uint64, 2*qsMaxCols/3+1)
	at := make([]uint64, qsMaxCols/3+1)
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
	Bm64T(m, dRows, k, at)
	for j := 0; j < k; j++ {
		pA[j] = at[j] & mask
		at[j] >>= uint(k)
	}
	Bm64XchBits(m, dRows, 2*k+1, (1<<uint(k))-1)
	res := Bm64EchelonL(m, dRows, 2*k+1, dRows)
	if res != dRows {
		panic("cgt: matrix is not invertible")
	}
	Bm64Mul(at, m[k:], k, dRows-k, at)
	mask = (uint64(1) << uint(2*k)) - 1
	for j := 0; j < k; j++ {
		pA[j] = (pA[j] ^ at[j]) & mask
		pA[k+j] = m[j] & mask
	}
	Bm64ReverseBits(pA, 2*k, k, 0)
	return pA
}

// qsPauliConjugateNoArg replaces each entry of v
// by qs*v*qs^-1 using the symplectic matrix,
// ignoring the complex argument.
func qsPauliConjugateNoArg(s *qs12, v []uint64) []uint64 {
	m := qsToSymplectic(s)
	out := make([]uint64, len(v))
	Bm64Mul(v, m, len(v), len(m), out)
	return out
}

// qsPauliConjugate replaces each entry of v by
// the Pauli group element qs*v*qs^-1, computing
// the complex argument. It panics if s is not an
// invertible (k,k) matrix.
func qsPauliConjugate(s *qs12, v []uint64) []uint64 {
	qsReduce(s)
	q := s.copy()
	rowTable := make([]uint8, qsMaxCols)
	reduceMatrix(q, rowTable)
	nqb := q.shape1
	fstRow := int(rowTable[q.ncols])
	if 2*nqb != q.ncols || nqb != lbRankReduced(q, rowTable) {
		panic("cgt: matrix is not invertible")
	}
	out := make([]uint64, len(v))
	if nqb == 0 {
		return out
	}
	m := q.data
	aT := make([]uint64, 2*qsMaxCols/3+1)
	Bm64T(m, q.nrows, q.ncols, aT)
	for j := 0; j < q.ncols; j++ {
		aT[j] <<= uint(q.ncols)
	}
	mask := ^(uint64(1) << uint(q.ncols))
	m[0] &= mask
	for i := 1; i < q.nrows; i++ {
		m[i] &= mask
		m[i] |= (m[0] >> uint(i)) &^ mask
	}
	for idx, vv := range v {
		m0 := m[0]
		f := int64(0x3120 >> (((vv >> uint(q.ncols)) & 3) << 2))
		mask = (uint64(1) << uint(nqb)) - 1
		m0 ^= (vv >> uint(nqb)) & mask
		for j := 0; j < nqb; j++ {
			if (vv>>uint(j))&1 != 0 {
				m0 ^= aT[j]
				f += int64(m0 >> uint(j) << 1)
			}
		}
		for j := nqb - 1; j >= 0; j-- {
			if (m0>>uint(j))&1 != 0 {
				i := int(rowTable[j])
				sh := uint(q.ncols + i)
				f += int64(((m0 >> sh) & 1) << 1)
				f += int64((m[i] >> sh) & 1)
				m0 ^= m[i]
			}
		}
		j0 := q.ncols + fstRow
		j1 := q.ncols + q.nrows
		for j := j1 - 1; j >= j0; j-- {
			if (m0>>uint(j))&1 != 0 {
				i := int(rowTable[j])
				sh := uint(q.ncols + i)
				f += int64(((m0 >> sh) & 1) << 1)
				f += int64((m[i] >> sh) & 1)
				m0 ^= m[i]
			}
		}
		var vOut uint64
		mask = uint64(1) << uint(q.ncols+nqb)
		m0 &^= uint64(1) << uint(q.ncols)
		for j := nqb; j < q.ncols; j++ {
			if (m[0]^m0)&mask != 0 {
				vOut ^= uint64(1) << uint(j)
				f += int64(m0 >> uint(j) << 1)
				m0 ^= aT[j]
			}
			mask >>= 1
		}
		vOut >>= uint(nqb)
		vOut ^= (m0 ^ m[0]) & (((uint64(1) << uint(nqb)) - 1) << uint(nqb))
		mask = (uint64(1) << uint(nqb)) - 1
		f ^= int64(qsParity(vOut&(vOut>>uint(nqb))&mask) << 1)
		f = (0x3120 >> ((f & 3) << 2)) & 3
		vOut ^= uint64(f) << uint(q.ncols)
		out[idx] = vOut
	}
	return out
}

/*************************************************************************
*** Scalar multiplication (qstate12.c)
*************************************************************************/

// qsMulScalar multiplies s by 2**(e/2) *
// exp(phi*pi*i/4).
func qsMulScalar(s *qs12, e, phi int) {
	if s.nrows == 0 {
		return
	}
	s.factor = qsAddFactors(s.factor, int64((e<<4)+(phi&7)))
}

/*************************************************************************
*** Exported QState API
*************************************************************************/

// NewQState builds a quadratic state matrix of
// complex shape 2**rows x 2**cols from data.
// Each data entry packs an A row in its low
// rows+cols bits and a Q row above. mode 0
// requires Q symmetric, mode 1 mirrors the upper
// triangle, mode 2 mirrors the lower triangle.
//
// NewQState panics if the dimensions overflow the
// 64-column limit or, for mode 0, if Q is not
// symmetric.
func NewQState(rows, cols int, data []uint64, mode int) *QState {
	nqb := rows + cols
	nrows := len(data)
	if nqb+nrows > qsMaxCols || nrows > qsMaxRows {
		panic("cgt: quadratic state too large")
	}
	s := &qs12{nrows: nrows, ncols: nqb, shape1: cols}
	s.data = make([]uint64, nrows, qsMaxRows)
	m := s.data
	mask := ((uint64(1) << uint(nqb)) << uint(nrows)) - 1
	for i := 0; i < nrows; i++ {
		m[i] = data[i] & mask
	}
	colMask := uint64(1) << uint(nqb)
	switch mode {
	case 1:
		if nrows > 0 {
			m[0] &= colMask - 1
		}
		for i := 1; i < nrows; i++ {
			m[i] &= (colMask << uint(i+1)) - 1
		}
		for i := 0; i < nrows; i++ {
			for j := i + 1; j < nrows; j++ {
				m[i] ^= ((m[j] >> uint(i)) & colMask) << uint(j)
			}
		}
	case 2:
		maskR := (colMask << uint(nrows)) - 1
		if nrows > 0 {
			m[0] &= maskR - colMask
		}
		for i := 1; i < nrows; i++ {
			m[i] &= maskR - ((colMask << uint(i)) - colMask)
		}
		for i := 0; i < nrows; i++ {
			for j := 0; j < i; j++ {
				m[i] ^= ((m[j] >> uint(i)) & colMask) << uint(j)
			}
		}
	default:
		qsCheck(s)
	}
	return fromQS12(s)
}

// UnitMatrix returns the 2**nqb x 2**nqb unit
// matrix.
func UnitMatrix(nqb int) *QState {
	s := &qs12{}
	qsUnitMatrix(s, nqb)
	return fromQS12(s)
}

// RandMatrix returns a random quadratic state
// matrix of shape (rows, cols) with dataRows
// rows.
func RandMatrix(rows, cols, dataRows int) *QState {
	limit := (uint64(1) << uint(rows+cols+dataRows)) - 1
	data := make([]uint64, dataRows)
	for i := range data {
		data[i] = rand.Uint64() & limit
	}
	return NewQState(rows, cols, data, 1)
}

// RandRealMatrix returns a random real quadratic
// state matrix of shape (rows, cols) with
// dataRows rows.
func RandRealMatrix(rows, cols, dataRows int) *QState {
	limit := (uint64(1) << uint(rows+cols+dataRows)) - 1
	data := make([]uint64, dataRows)
	for i := range data {
		data[i] = rand.Uint64() & limit
		data[i] &^= uint64(1) << uint(rows+cols+i)
	}
	return NewQState(rows, cols, data, 1)
}

// PauliMatrix returns the matrix of the Pauli
// group element v of nqb qubits.
func PauliMatrix(nqb int, v uint64) *QState {
	s := &qs12{}
	qsPauliMatrix(s, nqb, v)
	return fromQS12(s)
}

// CtrlNotMatrix returns the controlled-not
// transformation matrix for nqb qubits.
func CtrlNotMatrix(nqb int, vc, v uint64) *QState {
	q := UnitMatrix(nqb)
	mask := (uint64(1) << uint(nqb)) - 1
	q.GateCtrlNot(vc&mask, v&mask)
	return q.Reduce()
}

// PhiMatrix returns the phase gate matrix for
// nqb qubits.
func PhiMatrix(nqb int, v uint64, phi int) *QState {
	q := UnitMatrix(nqb)
	return q.GatePhi(v<<uint(nqb), phi)
}

// CtrlPhiMatrix returns the controlled phase
// gate matrix for nqb qubits.
func CtrlPhiMatrix(nqb int, v1, v2 uint64) *QState {
	q := UnitMatrix(nqb)
	return q.GateCtrlPhi(v1<<uint(nqb), v2)
}

// HadamardMatrix returns the Hadamard gate
// matrix for nqb qubits.
func HadamardMatrix(nqb int, v uint64) *QState {
	q := UnitMatrix(nqb)
	q.GateH(v)
	return q.Reduce()
}

// ColumnMonomialMatrix returns a real monomial
// matrix with one nonzero entry per column,
// defined by data of length nqb+1.
func ColumnMonomialMatrix(data []uint64) *QState {
	nqb := len(data) - 1
	s := &qs12{}
	qsMonomialColumnMatrix(s, nqb, data)
	return fromQS12(s)
}

// RowMonomialMatrix returns the transpose of
// ColumnMonomialMatrix(data): a real monomial
// matrix with one nonzero entry per row.
func RowMonomialMatrix(data []uint64) *QState {
	nqb := len(data) - 1
	q := ColumnMonomialMatrix(data)
	s := q.toQS12()
	Bm64RotBits(s.data, s.nrows, nqb, 2*nqb, 0)
	s.reduced = false
	q.store(s)
	return q
}

// FromSigns reconstructs a state vector of shape
// (0, n) from a sign bitmap as produced by
// ToSigns, or nil if bmap is not a quadratic
// state vector.
func FromSigns(bmap []uint64, n int) *QState {
	s := &qs12{}
	scanAffine(bmap, n, s)
	if !fillAffine(bmap, s) {
		return nil
	}
	if !qsCompareSigns(s.copy(), bmap) {
		return nil
	}
	return fromQS12(s)
}

// PauliVectorMul returns the product v1*v2 of two
// encoded Pauli group elements of nqb qubits.
func PauliVectorMul(nqb int, v1, v2 uint64) uint64 {
	return qsPauliVectorMul(nqb, v1, v2)
}

// PauliVectorExp returns the power v**e of an
// encoded Pauli group element of nqb qubits.
func PauliVectorExp(nqb int, v uint64, e int) uint64 {
	return qsPauliVectorExp(nqb, v, e)
}

// FlatProduct returns the product of a and b
// summing over the first nqb qubits and
// contracting nc of them.
func FlatProduct(a, b *QState, nqb, nc int) *QState {
	sa := a.toQS12()
	sb := b.toQS12()
	qsProductInto(sa, sb, nqb, nc)
	return fromQS12(sa)
}

// Copy returns a deep copy of q.
func (q *QState) Copy() *QState {
	c := &QState{rows: q.rows, cols: q.cols, factor: q.factor}
	c.data = make([]uint64, len(q.data))
	copy(c.data, q.data)
	return c
}

// Shape returns the complex matrix shape as
// (rows, cols), i.e. a 2**rows x 2**cols matrix.
func (q *QState) Shape() (int, int) {
	return q.rows, q.cols
}

// Factor returns the scalar factor as a pair
// (e, phi) denoting 2**(e/2) * exp(phi*pi*i/4).
func (q *QState) Factor() (int, int) {
	if len(q.data) == 0 {
		return 0, 0
	}
	f := int64(q.factor[0])
	return int((f & -0x10) >> 4), int(f & 7)
}

// Data returns the underlying bit matrix rows.
func (q *QState) Data() []uint64 {
	return q.data
}

// NRows returns the number of bit matrix rows.
func (q *QState) NRows() int {
	return len(q.data)
}

// NCols returns the number of qubits (rows+cols).
func (q *QState) NCols() int {
	return q.rows + q.cols
}

// Matrix expands q to its 2**(rows+cols) complex
// entries in row-major order. It reduces q.
func (q *QState) Matrix() []complex128 {
	s := q.toQS12()
	out := qsComplex(s)
	q.store(s)
	return out
}

// MulScalar multiplies q by 2**(e/2) *
// exp(phi*pi*i/4) in place.
func (q *QState) MulScalar(e, phi int) *QState {
	s := q.toQS12()
	qsMulScalar(s, e, phi)
	q.store(s)
	return q
}

// Reduce converts q to reduced echelon form in
// place. This does not change the matrix.
func (q *QState) Reduce() *QState {
	s := q.toQS12()
	qsReduce(s)
	q.store(s)
	return q
}

// Echelon converts q to (non-reduced) echelon
// form in place. This does not change the matrix.
func (q *QState) Echelon() *QState {
	s := q.toQS12()
	qsEchelonize(s)
	q.store(s)
	return q
}

// ReduceMatrix converts q to reduced matrix
// representation in place and returns the row
// table.
func (q *QState) ReduceMatrix() []int {
	s := q.toQS12()
	qsReduce(s)
	rowTable := make([]uint8, s.ncols+s.nrows+4)
	reduceMatrix(s, rowTable)
	q.store(s)
	out := make([]int, len(rowTable))
	for i, b := range rowTable {
		out[i] = int(b)
	}
	return out
}

// Reshape changes the complex matrix shape to
// 2**rows x 2**cols in place. A value of -1 for
// rows or cols is computed from the total. It
// panics on a shape mismatch.
func (q *QState) Reshape(rows, cols int) *QState {
	s := q.toQS12()
	if cols == -1 {
		if rows == -1 {
			rows = 0
		}
		cols = s.ncols - rows
	} else if rows == -1 {
		rows = s.ncols - cols
	}
	if rows < 0 || cols < 0 || rows+cols != s.ncols {
		panic("cgt: matrix shape mismatch")
	}
	s.shape1 = cols
	q.store(s)
	return q
}

// Transpose transposes q in place.
func (q *QState) Transpose() *QState {
	s := q.toQS12()
	qsMatT(s)
	q.store(s)
	return q
}

// Conjugate replaces q by its complex conjugate
// in place.
func (q *QState) Conjugate() *QState {
	s := q.toQS12()
	qsConjugate(s)
	q.store(s)
	return q
}

// T returns the transpose of q as a new matrix.
func (q *QState) T() *QState {
	return q.Copy().Transpose()
}

// H returns the conjugate transpose of q as a
// new matrix.
func (q *QState) H() *QState {
	return q.Copy().Transpose().Conjugate()
}

// RotBits rotates qubits start..start+nrot-1 by
// rot in place.
func (q *QState) RotBits(rot, nrot, start int) *QState {
	s := q.toQS12()
	if nrot+start > s.ncols {
		panic("cgt: qubit index out of range")
	}
	if nrot >= 2 {
		s.reduced = false
		Bm64RotBits(s.data, s.nrows, rot, nrot, start)
	}
	q.store(s)
	return q
}

// XchBits exchanges qubit j with qubit j+sh for
// each bit j of mask, in place.
func (q *QState) XchBits(sh int, mask uint64) *QState {
	s := q.toQS12()
	if mask != 0 {
		s.reduced = false
		if sh >= s.ncols || mask&((mask|(^uint64(0)<<uint(s.ncols)))>>uint(sh)) != 0 {
			panic("cgt: qubit index out of range")
		}
		Bm64XchBits(s.data, s.nrows, sh, mask)
	}
	q.store(s)
	return q
}

// Extend inserts nqb qubits at position j in
// place.
func (q *QState) Extend(j, nqb int) *QState {
	s := q.toQS12()
	qsExtend(s, j, nqb)
	q.store(s)
	return q
}

// ExtendZero inserts nqb zero qubits at position
// j in place.
func (q *QState) ExtendZero(j, nqb int) *QState {
	s := q.toQS12()
	qsExtendZero(s, j, nqb)
	q.store(s)
	return q
}

// Restrict restricts nqb qubits at position j to
// 0 and deletes them, in place.
func (q *QState) Restrict(j, nqb int) *QState {
	s := q.toQS12()
	qsRestrict(s, j, nqb)
	q.store(s)
	return q
}

// RestrictZero restricts nqb qubits at position
// j to 0 in place, keeping the shape.
func (q *QState) RestrictZero(j, nqb int) *QState {
	s := q.toQS12()
	qsRestrictZero(s, j, nqb)
	q.store(s)
	return q
}

// Sumup sums over nqb qubits at position j in
// place.
func (q *QState) Sumup(j, nqb int) *QState {
	s := q.toQS12()
	qsSumCols(s, j, nqb)
	q.store(s)
	return q
}

// GateNot applies a not gate qs(x) = qs(x^v) in
// place.
func (q *QState) GateNot(v uint64) *QState {
	s := q.toQS12()
	qsGateNot(s, v)
	q.store(s)
	return q
}

// GateCtrlNot applies a controlled not gate in
// place. It panics if vc and v are not
// orthogonal.
func (q *QState) GateCtrlNot(vc, v uint64) *QState {
	s := q.toQS12()
	qsGateCtrlNot(s, vc, v)
	q.store(s)
	return q
}

// GatePhi applies a phase gate qs(x) =
// qs(x)*sqrt(-1)**(phi*<v,x>) in place.
func (q *QState) GatePhi(v uint64, phi int) *QState {
	s := q.toQS12()
	qsGatePhi(s, v, phi)
	q.store(s)
	return q
}

// GateCtrlPhi applies a controlled phase gate
// qs(x) = qs(x)*(-1)**(<v1,x>*<v2,x>) in place.
func (q *QState) GateCtrlPhi(v1, v2 uint64) *QState {
	s := q.toQS12()
	qsGateCtrlPhi(s, v1, v2)
	q.store(s)
	return q
}

// GateH applies Hadamard gates to all qubits j
// with bit j of v set, in place.
func (q *QState) GateH(v uint64) *QState {
	s := q.toQS12()
	qsGateH(s, v)
	q.store(s)
	return q
}

// ToSigns returns the signs of a real matrix
// packed two bits per entry (0 zero, 1 positive,
// 3 negative).
func (q *QState) ToSigns() []uint64 {
	s := q.toQS12()
	out := qsToSigns(s)
	q.store(s)
	return out
}

// CompareSigns reports whether the signs of q
// match bmap as produced by ToSigns.
func (q *QState) CompareSigns(bmap []uint64) bool {
	return qsCompareSigns(q.toQS12(), bmap)
}

// LbRank returns the binary logarithm of the
// rank of q, or -1 if q is the zero matrix.
func (q *QState) LbRank() int {
	s := q.toQS12()
	r := qsMatLbRank(s)
	q.store(s)
	return r
}

// LbNorm2 returns the binary logarithm of the
// squared operator norm of q, or -1 if q is
// zero.
func (q *QState) LbNorm2() int {
	s := q.toQS12()
	qsReduce(s)
	q.store(s)
	if s.nrows == 0 {
		return -1
	}
	return int(s.factor>>4) + s.nrows - 1 - qsMatLbRank(s.copy())
}

// Trace returns the trace of a square matrix.
func (q *QState) Trace() complex128 {
	s := q.toQS12()
	v := factorToComplex(qsMatTraceFactor(s))
	q.store(s)
	return v
}

// Inv returns the inverse matrix. It panics if q
// is not invertible.
func (q *QState) Inv() *QState {
	s := q.toQS12()
	qsMatInv(s)
	return fromQS12(s)
}

// Power returns the e-th power of q. Power(0) is
// the unit matrix, Power(-1) is the inverse.
func (q *QState) Power(e int) *QState {
	if e > 1 {
		mask := 1 << uint(bits.Len(uint(e))-2)
		acc := q.Copy()
		for mask != 0 {
			acc = acc.MatMul(acc)
			if e&mask != 0 {
				acc = acc.MatMul(q)
			}
			mask >>= 1
		}
		return acc
	}
	if e >= 0 {
		nqb := q.rows
		if nqb != q.cols {
			panic("cgt: matrix shape mismatch")
		}
		if e != 0 {
			return q.Copy().Reduce()
		}
		return UnitMatrix(nqb)
	}
	return q.Inv().Power(-e)
}

// Order returns the smallest positive e <=
// maxOrder with q**e the unit matrix. It panics
// if q is not invertible or has infinite order.
func (q *QState) Order(maxOrder int) int {
	nqb := q.rows
	if q.LbRank() != nqb || nqb != q.cols {
		panic("cgt: matrix is not invertible")
	}
	if q.LbNorm2() != 0 {
		panic("cgt: matrix has infinite order")
	}
	unit := UnitMatrix(nqb)
	d := map[string]int{unit.key(): 0}
	n := int(math.Sqrt(float64(maxOrder))) + 2
	m := q.Copy()
	for i := 1; i < n; i++ {
		if m.Equal(unit) {
			return i
		}
		d[m.key()] = i
		m = m.MatMul(q)
	}
	m = m.Inv()
	acc := m.Copy()
	for i := 1; i < n; i++ {
		if j, ok := d[acc.key()]; ok {
			return i*n + j
		}
		acc = acc.MatMul(m)
	}
	panic("cgt: order of matrix not found")
}

// key returns a reduced-form fingerprint of q
// for use as a map key in Order.
func (q *QState) key() string {
	r := q.Copy().Reduce()
	b := make([]byte, 0, 8*(len(r.data)+1))
	for _, w := range r.data {
		for s := 0; s < 64; s += 8 {
			b = append(b, byte(w>>uint(s)))
		}
	}
	f := uint64(r.factor[0])
	for s := 0; s < 64; s += 8 {
		b = append(b, byte(f>>uint(s)))
	}
	return string(b)
}

// MatMul returns the matrix product q @ other.
func (q *QState) MatMul(other *QState) *QState {
	sa := q.toQS12()
	sb := other.toQS12()
	return fromQS12(qsMatmul(sa, sb))
}

// Mul returns the elementwise product of q and
// other, which must have the same shape.
func (q *QState) Mul(other *QState) *QState {
	sa := q.toQS12()
	sb := other.toQS12()
	if sa.ncols-sa.shape1 != sb.ncols-sb.shape1 || sa.shape1 != sb.shape1 {
		panic("cgt: matrices must have the same shape")
	}
	shape1 := sa.shape1
	qsProductInto(sa, sb, sa.ncols, 0)
	sa.shape1 = shape1
	return fromQS12(sa)
}

// Equal reports whether q and other represent the
// same matrix. Both are reduced before comparison.
func (q *QState) Equal(other *QState) bool {
	s1 := q.toQS12()
	s2 := other.toQS12()
	qsReduce(s1)
	qsReduce(s2)
	if s1.nrows|s2.nrows == 0 {
		return true
	}
	if (uint64(s1.factor)^uint64(s2.factor))&factorMask != 0 || s1.nrows != s2.nrows {
		return false
	}
	mask := (((uint64(1) << uint(s1.nrows)) - 1) << uint(s1.ncols)) - 1
	var diff uint64
	for i := 0; i < s1.nrows; i++ {
		diff |= (s1.data[i] ^ s2.data[i]) & mask
	}
	return diff == 0
}

// PauliVector returns q as an encoded Pauli
// vector. It panics if q is not in the Pauli
// group.
func (q *QState) PauliVector() uint64 {
	s := q.toQS12()
	_, v := qsPauliVector(s)
	q.store(s)
	return v
}

// PauliConjugate replaces each element of v by
// the Pauli group element q*v*q^-1. When arg is
// false the complex argument is omitted. q must
// be an invertible (k,k) matrix.
func (q *QState) PauliConjugate(v []uint64, arg bool) []uint64 {
	s := q.toQS12()
	if !arg {
		return qsPauliConjugateNoArg(s, v)
	}
	return qsPauliConjugate(s, v)
}

// ToSymplectic returns the 2k x 2k symplectic bit
// matrix of q acting on the Pauli group. q must
// be an invertible (k,k) matrix.
func (q *QState) ToSymplectic() []uint64 {
	s := q.toQS12()
	out := qsToSymplectic(s)
	q.store(s)
	return out
}

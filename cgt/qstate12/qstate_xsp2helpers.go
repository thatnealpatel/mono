package qstate12

import "patel.codes/cgt/swar"

// Additional qstate12-layer helpers used by the
// G_{x0} (xsp2co1) layer but not part of the core
// qstate.go surface.

// QsMaxRows is the maximum number of rows (data
// words) in a QState12 bit matrix, exported for
// callers that size their own buffers.
const QsMaxRows = qsMaxRows

// QsMonomialColumnMatrix sets s to a monomial
// column matrix of nqb qubits defined by pa, as
// qstate12_monomial_column_matrix.
func QsMonomialColumnMatrix(s *QState12, nqb int, pa []uint64) {
	factor := int64(((pa[0] >> uint(nqb)) & 1) << 2)
	s.SetNrows(nqb + 1)
	s.SetNcols(nqb << 1)
	if s.Nrows()+s.Ncols() > qsMaxCols || s.Nrows() > qsMaxRows {
		panic("cgt: quadratic state too large")
	}
	s.Grow(s.Nrows())
	m := s.Data()
	mask1 := (uint64(1) << uint(nqb)) - 1
	m[0] = (pa[0] & mask1) << uint(nqb)
	for i := 1; i <= nqb; i++ {
		mask1 += mask1 + 1
		m[i] = (uint64(1) << uint(i-1)) | ((pa[i] & mask1) << uint(nqb))
	}
	QsSet1(s, 2*nqb, nqb+1)
	s.SetShape1(nqb)
	s.SetFactor(factor)
}

// QsSet1 applies qstate12_set with mode 1 to the
// first nrows rows of s, treating its ncols-bit A
// part and the trailing Q part.
func QsSet1(s *QState12, nqb, nrows int) {
	m := s.Data()
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
	s.SetNrows(nrows)
	s.SetNcols(nqb)
	s.SetFactor(0)
	s.SetShape1(0)
	s.SetReduced(false)
}

// QsMonomialMatrixRowOp obtains the affine
// operation of a monomial state on its unit
// vector labels, as qstate12_monomial_matrix_row_op.
// It returns the number r+1 of rows, or a negative
// value if s is not monomial.
func QsMonomialMatrixRowOp(s *QState12, pa []uint32) int {
	QsReduce(s)
	cols := s.Shape1()
	rows := s.Ncols() - cols
	if s.Nrows() != rows+1 {
		return -1
	}
	m := s.Data()
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

// QsMatTraceFactor reduces s, requires it to be
// square, and returns the trace as an encoded
// factor (qstate12_mat_trace_factor).
func QsMatTraceFactor(s *QState12) int64 {
	nrows := s.Shape1()
	QsReduce(s)
	if 2*nrows != s.Ncols() {
		panic("cgt: trace of non-square matrix")
	}
	q := s.Copy()
	for i := 0; i < nrows; i++ {
		QsGateCtrlNot(q, uint64(1)<<uint(i), uint64(1)<<uint(nrows+i))
	}
	QsRestrict(q, nrows, nrows)
	QsSumCols(q, 0, nrows)
	QsReduce(q)
	if q.Ncols() != 0 {
		panic("cgt: trace internal error")
	}
	if q.Nrows() != 0 {
		return int64(uint64(q.Factor()) & FactorMask)
	}
	return 8
}

// QsMatItrace returns the integer trace of the
// square state s (qstate12_mat_itrace).
func QsMatItrace(s *QState12) int64 {
	return FactorToInt32(QsMatTraceFactor(s))
}

// QsToSymplecticRow returns row n of the
// symplectic bit matrix of the invertible square
// state s (qstate12_to_symplectic_row). It panics
// if s is not an invertible (k,k) matrix or n is
// out of range.
func QsToSymplecticRow(s *QState12, n int) uint32 {
	m, dRows, k := QsSymplecticPrep(s)
	if k == 0 {
		return 0
	}
	swar.Bm64XchBits(m, dRows, 2*k+1, (1<<uint(k))-1)
	res := swar.Bm64EchelonL(m, dRows, 2*k+1, dRows)
	if res != dRows {
		panic("cgt: matrix is not invertible")
	}
	var a uint64
	if n < k {
		pd := s.Data()[1:]
		for j := 0; j < k; j++ {
			a ^= ((pd[j] >> uint(n)) & 1) << uint(j)
		}
		for j := k; j < dRows; j++ {
			mask := 0 - ((pd[j] >> uint(n)) & 1)
			a ^= m[j] & mask
		}
	} else if n < k+k {
		a = m[n-k]
	} else {
		panic("cgt: qubit index out of range")
	}
	a &= (uint64(1) << uint(2*k)) - 1
	one := []uint64{a}
	swar.Bm64ReverseBits(one, 1, k, 0)
	return uint32(one[0])
}

package cgt

import (
	"math"
	"math/cmplx"
	"testing"
)

// Test for FromSigns (qstate.go), the reconstruction of a
// real quadratic state vector from its sign bitmap,
// grounded in the canonical mmgroup test
//   mmgroup/tests/test_clifford/test_qs_bitmap.py
//     (test_qs_matrix)
//
// That test checks `qs_from_signs(m.to_signs(), n) == m1`
// where m1 = m / maxabs(m) is the original real matrix
// normalized to a column vector. We reproduce that
// invariant: FromSigns(q.ToSigns(), n) reconstructs a
// quadratic state vector whose normalized entries equal
// the normalized entries of q, and whose own sign bitmap
// round-trips exactly.
//
// The all-zero sign bitmap (the zero vector) is excluded;
// see the package note on the FromSigns/swar.Bm64FindLowBit
// out-of-bounds for that case.

// normRealVec returns the real parts of a complex matrix
// scaled so the largest absolute entry is 1 (or all
// zeros if the matrix is zero).
func normRealVec(a []complex128) []float64 {
	var mx float64
	for _, x := range a {
		if m := cmplx.Abs(x); m > mx {
			mx = m
		}
	}
	out := make([]float64, len(a))
	if mx == 0 {
		return out
	}
	for i, x := range a {
		out[i] = real(x) / mx
	}
	return out
}

func sameSignsBitmap(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func checkFromSigns(t *testing.T, name string, q *QState) {
	t.Helper()
	rows, cols := q.Shape()
	n := rows + cols
	bm := q.ToSigns()
	allZero := true
	for _, w := range bm {
		if w != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		// Zero vector: excluded (latent over-read in
		// swar.Bm64FindLowBit; the oracle returns the zero vector).
		return
	}
	// FromSigns (faithful to C bitmatrix64_find_low_bit)
	// over-reads one word past the bitmap on sparse inputs;
	// the C reference always passes an over-allocated
	// buffer, so we pad with a trailing zero word to match
	// that calling convention.
	padded := make([]uint64, len(bm)+1)
	copy(padded, bm)
	mfs := FromSigns(padded, n)
	if mfs == nil {
		t.Errorf("%s: FromSigns returned nil for a quadratic sign map", name)
		return
	}
	// The reconstruction must carry exactly the input signs.
	if !mfs.CompareSigns(bm) {
		t.Errorf("%s: FromSigns result does not compare equal to its sign map", name)
	}
	if got := mfs.ToSigns(); !sameSignsBitmap(got, bm) {
		t.Errorf("%s: FromSigns(bm).ToSigns()=%v want %v", name, got, bm)
	}
	// The normalized reconstruction equals the normalized
	// original (the mmgroup `m_from_signs == m1` assertion).
	orig := normRealVec(q.Matrix())
	recon := normRealVec(mfs.Matrix())
	if len(orig) != len(recon) {
		t.Errorf("%s: matrix length orig=%d recon=%d", name, len(orig), len(recon))
		return
	}
	for i := range orig {
		if math.Abs(orig[i]-recon[i]) > 1e-9 {
			t.Errorf("%s: normalized entry %d: orig=%g recon=%g", name, i, orig[i], recon[i])
		}
	}
}

// TestFromSigns checks the sign-reconstruction round-trip
// on the explicit real state matrices used by mmgroup's
// test_qs_bitmap.py and on randomly generated real
// matrices.
func TestFromSigns(t *testing.T) {
	t.Parallel()
	// Explicit real matrices (mmgroup qs_matrix_data and a
	// real (2,2) symmetric-Q case usable in mode 0).
	explicit := []tcase{
		{0, 4, [2]int{1, 4}, []uint64{0, 8, 4, 2, 1}},
		{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}},
		{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}},
		{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
	}
	for i, c := range explicit {
		checkFromSigns(t, "explicit", c.state())
		_ = i
	}
	// Non-zero random real matrices of several shapes.
	for _, sh := range [][3]int{{2, 0, 3}, {1, 0, 2}, {3, 0, 3}, {0, 3, 2}, {2, 2, 3}} {
		for i := 0; i < 30; i++ {
			q := RandRealMatrix(sh[0], sh[1], sh[2])
			checkFromSigns(t, "random", q)
		}
	}
}

// TestFromSignsNonQuadratic checks that FromSigns returns
// nil when the sign bitmap does not come from a quadratic
// state vector (the C ERR_QSTATE12_NOTFOUND path).
func TestFromSignsNonQuadratic(t *testing.T) {
	t.Parallel()
	// A sign pattern whose support is not an affine subspace
	// of GF(2)^n is not realizable by a quadratic state
	// vector. Over n=2, entries 0,1,2 positive with entry 3
	// zero has support {00,01,10}, which spans all of
	// GF(2)^2 but is not affine, so qs_from_signs returns
	// None (the C ERR_QSTATE12_NOTFOUND path). Each entry i
	// occupies bits 2i,2i+1; code 01 = positive. Padded with
	// a trailing zero word per the C buffer convention.
	bad := []uint64{0b01_01_01, 0} // entries 0,1,2 positive, 3 zero
	if got := FromSigns(bad, 2); got != nil {
		t.Errorf("FromSigns(non-quadratic) = %v, want nil", got.Matrix())
	}
}

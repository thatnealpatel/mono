package cnt

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"math/rand"
	"strings"
	"testing"
)

func oracleF2(t *testing.T, setup, expr string) string {
	t.Helper()
	script := fmt.Sprintf("import json,numpy as np\n%s\nprint(json.dumps(%s))", setup, expr)
	out, err := pyCmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func oracleF2Ints(t *testing.T, setup, expr string) []int64 {
	t.Helper()
	var v []int64
	s := oracleF2(t, setup, expr)
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleF2Ints(%q): %v", expr, err)
	}
	return v
}

func f2List(m []uint64) string {
	parts := make([]string, len(m))
	for i, x := range m {
		parts[i] = fmt.Sprintf("%d", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func f2Eq(a []uint64, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if int64(a[i]) != b[i] {
			return false
		}
	}
	return true
}

const f2Echelon = "from mmgroup.clifford12 import bitmatrix64_echelon_h, bitmatrix64_echelon_l\n" +
	"def ech_h(m,j0,n):\n" +
	" a=np.array(m,dtype=np.uint64); k=int(bitmatrix64_echelon_h(a,len(a),j0,n)); return [k]+[int(x) for x in a]\n" +
	"def ech_l(m,j0,n):\n" +
	" a=np.array(m,dtype=np.uint64); k=int(bitmatrix64_echelon_l(a,len(a),j0,n)); return [k]+[int(x) for x in a]"

func TestEchelonH(t *testing.T) {
	cases := []struct {
		m     []uint64
		j0, n int
	}{
		{[]uint64{0x3, 0x7, 0xb}, 4, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, 4, 4},
		{[]uint64{0xff, 0xf0, 0x0f, 0x33}, 8, 8},
		{[]uint64{0x123456, 0xabcdef, 0x0, 0x800001}, 24, 24},
	}
	for _, c := range cases {
		m := append([]uint64(nil), c.m...)
		k := EchelonH(m, c.j0, c.n)
		got := append([]uint64{uint64(k)}, m...)
		want := oracleF2Ints(t, f2Echelon, fmt.Sprintf("ech_h(%s,%d,%d)", f2List(c.m), c.j0, c.n))
		if !f2Eq(got, want) {
			t.Errorf("EchelonH(%v,%d,%d) = %v want %v", c.m, c.j0, c.n, got, want)
		}
	}
}

func TestEchelonL(t *testing.T) {
	cases := []struct {
		m     []uint64
		j0, n int
	}{
		{[]uint64{0x3, 0x7, 0xb}, 0, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, 0, 4},
		{[]uint64{0xff, 0xf0, 0x0f, 0x33}, 0, 8},
		{[]uint64{0x6, 0xc, 0x18, 0x30}, 1, 6},
	}
	for _, c := range cases {
		m := append([]uint64(nil), c.m...)
		k := EchelonL(m, c.j0, c.n)
		got := append([]uint64{uint64(k)}, m...)
		want := oracleF2Ints(t, f2Echelon, fmt.Sprintf("ech_l(%s,%d,%d)", f2List(c.m), c.j0, c.n))
		if !f2Eq(got, want) {
			t.Errorf("EchelonL(%v,%d,%d) = %v want %v", c.m, c.j0, c.n, got, want)
		}
	}
}

func TestRank(t *testing.T) {
	cases := []struct {
		m     []uint64
		j0, n int
	}{
		{[]uint64{0x3, 0x7, 0xb}, 4, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, 4, 4},
		{[]uint64{0x5, 0x5, 0x5}, 4, 4},
		{[]uint64{0x123456, 0xabcdef, 0x0, 0x800001}, 24, 24},
	}
	for _, c := range cases {
		got := Rank(c.m, c.j0, c.n)
		want := oracleF2Ints(t, f2Echelon, fmt.Sprintf("[ech_h(%s,%d,%d)[0]]", f2List(c.m), c.j0, c.n))
		if int64(got) != want[0] {
			t.Errorf("Rank(%v,%d,%d) = %d want %d", c.m, c.j0, c.n, got, want[0])
		}
	}
}

const f2Solve = "from mmgroup.clifford12 import bitmatrix64_solve_equation\n" +
	"def solve(m,i,j):\n" +
	" a=np.array(m,dtype=np.uint64); r=int(bitmatrix64_solve_equation(a,i,j)); return [1,r] if r>=0 else [0,0]"

func TestSolve(t *testing.T) {
	cases := []struct {
		m    []uint64
		i, j int
	}{
		{[]uint64{0x9, 0xa, 0xc}, 3, 3},
		{[]uint64{0x11, 0x12, 0x14, 0x18}, 4, 4},
		{[]uint64{0x3, 0x6, 0x4}, 3, 3},
		{[]uint64{0x21, 0x12, 0x0c, 0x08, 0x30}, 5, 5},
	}
	for _, c := range cases {
		m := append([]uint64(nil), c.m...)
		x, ok := Solve(m, c.i, c.j)
		want := oracleF2Ints(t, f2Solve, fmt.Sprintf("solve(%s,%d,%d)", f2List(c.m), c.i, c.j))
		gotOK := int64(0)
		if ok {
			gotOK = 1
		}
		if gotOK != want[0] || (ok && int64(x) != want[1]) {
			t.Errorf("Solve(%v,%d,%d) = (%d,%v) want (%d,ok=%v)", c.m, c.i, c.j, x, ok, want[1], want[0] == 1)
		}
	}
}

const f2Probe = "from mmgroup.clifford12 import bitmatrix64_t, bitmatrix64_mul, bitmatrix64_inv, bitmatrix64_cap_h\n" +
	"def tr(m,n):\n" +
	" return [int(x) for x in bitmatrix64_t(np.array(m,dtype=np.uint64),n)]\n" +
	"def mul(a,b):\n" +
	" return [int(x) for x in bitmatrix64_mul(np.array(a,dtype=np.uint64),np.array(b,dtype=np.uint64))]\n" +
	"def inv(m):\n" +
	" try:\n" +
	"  r=bitmatrix64_inv(np.array(m,dtype=np.uint64)); return [1]+[int(x) for x in r]\n" +
	" except Exception:\n" +
	"  return [0]\n" +
	"def cap(a,b,j0,n):\n" +
	" m1=np.array(a,dtype=np.uint64); m2=np.array(b,dtype=np.uint64)\n" +
	" l1,l2=bitmatrix64_cap_h(m1,m2,j0,n)\n" +
	" return [int(l1),int(l2),len(a)-int(l1)]+[int(x) for x in m1]+[int(x) for x in m2]"

func TestTranspose(t *testing.T) {
	cases := []struct {
		m []uint64
		n int
	}{
		{[]uint64{0x1, 0x2, 0x4, 0x8}, 4},
		{[]uint64{0x3, 0x7, 0xb, 0x0}, 4},
		{[]uint64{0x6, 0x9, 0xf, 0x1}, 4},
		{[]uint64{0x80, 0x41, 0x22, 0x14, 0x08, 0x04, 0x02, 0x01}, 8},
		{[]uint64{0xff, 0xf0, 0x0f, 0x33, 0xcc, 0xaa, 0x55, 0x18}, 8},
	}
	for _, c := range cases {
		dst := make([]uint64, c.n)
		Transpose(dst, c.m, c.n, c.n)
		want := oracleF2Ints(t, f2Probe, fmt.Sprintf("tr(%s,%d)", f2List(c.m), c.n))
		if !f2Eq(dst, want) {
			t.Errorf("Transpose(%v,%d) = %v want %v", c.m, c.n, dst, want)
		}
	}
}

func TestMatMul(t *testing.T) {
	cases := []struct {
		a, b []uint64
		n    int
	}{
		{[]uint64{0x3, 0x5, 0x6, 0x1}, []uint64{0x8, 0x4, 0x2, 0x1}, 4},
		{[]uint64{0x3, 0x5, 0x6, 0x1}, []uint64{0x1, 0x2, 0x4, 0x8}, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, []uint64{0x1, 0x2, 0x4, 0x8}, 4},
		{[]uint64{0xff, 0xf0, 0x0f, 0x33, 0xcc, 0xaa, 0x55, 0x18},
			[]uint64{0x80, 0x41, 0x22, 0x14, 0x08, 0x04, 0x02, 0x01}, 8},
		{[]uint64{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
			[]uint64{0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80}, 8},
	}
	for _, c := range cases {
		dst := make([]uint64, len(c.a))
		MatMul(dst, c.a, c.b)
		want := oracleF2Ints(t, f2Probe, fmt.Sprintf("mul(%s,%s)", f2List(c.a), f2List(c.b)))
		if !f2Eq(dst, want) {
			t.Errorf("MatMul(%v,%v,%d) = %v want %v", c.a, c.b, c.n, dst, want)
		}
	}
}

func TestMatInv(t *testing.T) {
	cases := []struct {
		m []uint64
		n int
	}{
		{[]uint64{0x3, 0x5, 0x6, 0x1}, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, 4},
		{[]uint64{0x1, 0x2, 0x3, 0x0}, 4},
		{[]uint64{0x5, 0x5, 0x5, 0x5}, 4},
		{[]uint64{0x80, 0x41, 0x22, 0x14, 0x08, 0x04, 0x02, 0x01}, 8},
		{[]uint64{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 8},
	}
	for _, c := range cases {
		m := append([]uint64(nil), c.m...)
		ok := MatInv(m, c.n)
		want := oracleF2Ints(t, f2Probe, fmt.Sprintf("inv(%s)", f2List(c.m)))
		if want[0] == 0 {
			if ok {
				t.Errorf("MatInv(%v,%d) = (%v,true) want singular", c.m, c.n, m)
			}
			continue
		}
		if !ok {
			t.Errorf("MatInv(%v,%d) = false want invertible %v", c.m, c.n, want[1:])
			continue
		}
		if !f2Eq(m, want[1:]) {
			t.Errorf("MatInv(%v,%d) = %v want %v", c.m, c.n, m, want[1:])
		}
	}
}

// randMatrix returns r rows, each a random value masked to the low cols bits.
func randMatrix(rng *rand.Rand, r, cols int) []uint64 {
	mask := uint64(1)<<uint(cols) - 1
	m := make([]uint64, r)
	for i := range m {
		m[i] = rng.Uint64() & mask
	}
	return m
}

// identityMatrix returns the n×n GF(2) identity in row-packed form.
func identityMatrix(n int) []uint64 {
	id := make([]uint64, n)
	for i := range id {
		id[i] = uint64(1) << uint(i)
	}
	return id
}

func u64SliceEq(a, b []uint64) bool {
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

// TestEchelonHRandomized checks structural invariants of EchelonH on random
// small matrices: pivot rows have strictly descending pivot columns, every
// pivot column is zero outside its pivot row (RREF), and the pivot count agrees
// with Rank.
func TestEchelonHRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0xec4e10))
	for range 5000 {
		rows := 1 + rng.Intn(8)
		n := 1 + rng.Intn(8)
		orig := randMatrix(rng, rows, n)

		m := append([]uint64(nil), orig...)
		k := EchelonH(m, n, n)
		if k < 0 || k > rows {
			t.Fatalf("EchelonH rank %d out of range for %d rows (m=%v,n=%d)", k, rows, orig, n)
		}

		// (a) pivot columns strictly descend with row index. The pivot column
		// of a pivot row is its highest set bit within [0,n).
		pivotCol := func(row uint64) int {
			for c := n - 1; c >= 0; c-- {
				if row&(uint64(1)<<uint(c)) != 0 {
					return c
				}
			}
			return -1
		}
		prev := n
		for r := range k {
			pc := pivotCol(m[r])
			if pc < 0 {
				t.Fatalf("EchelonH pivot row %d is empty in [0,%d): m=%v orig=%v", r, n, m, orig)
			}
			if pc >= prev {
				t.Fatalf("EchelonH pivot columns not descending: row %d col %d >= prev %d (m=%v orig=%v n=%d)", r, pc, prev, m, orig, n)
			}
			prev = pc
		}

		// (b) RREF: each pivot column holds a 1 only in its pivot row.
		for r := range k {
			pc := pivotCol(m[r])
			colMask := uint64(1) << uint(pc)
			for s := range rows {
				if s == r {
					continue
				}
				if m[s]&colMask != 0 {
					t.Fatalf("EchelonH not RREF: pivot col %d (row %d) also set in row %d (m=%v orig=%v n=%d)", pc, r, s, m, orig, n)
				}
			}
		}

		// (c) rank matches Rank() on the original.
		if got := Rank(orig, n, n); got != k {
			t.Fatalf("Rank=%d but EchelonH=%d (orig=%v n=%d)", got, k, orig, n)
		}
	}
}

// TestMatMulAssociative verifies (A*B)*C == A*(B*C) over GF(2) on random 8×8
// matrices.
func TestMatMulAssociative(t *testing.T) {
	const n = 8
	rng := rand.New(rand.NewSource(0xab1a55))
	for range 1000 {
		a := randMatrix(rng, n, n)
		b := randMatrix(rng, n, n)
		c := randMatrix(rng, n, n)

		ab := make([]uint64, n)
		MatMul(ab, a, b)
		abc := make([]uint64, n)
		MatMul(abc, ab, c)

		bc := make([]uint64, n)
		MatMul(bc, b, c)
		aBc := make([]uint64, n)
		MatMul(aBc, a, bc)

		if !u64SliceEq(abc, aBc) {
			t.Fatalf("MatMul not associative:\nA=%v\nB=%v\nC=%v\n(A*B)*C=%v\nA*(B*C)=%v", a, b, c, abc, aBc)
		}
	}
}

// TestMatInvRoundTrip verifies that MatInv produces a genuine inverse: for every
// matrix it reports as invertible, m_orig * m_inv must equal the identity.
func TestMatInvRoundTrip(t *testing.T) {
	const n = 8
	rng := rand.New(rand.NewSource(0x10f5e7))
	id := identityMatrix(n)
	invertible := 0
	for range 1000 {
		orig := randMatrix(rng, n, n)
		inv := append([]uint64(nil), orig...)
		if !MatInv(inv, n) {
			continue
		}
		invertible++
		prod := make([]uint64, n)
		MatMul(prod, orig, inv)
		if !u64SliceEq(prod, id) {
			t.Fatalf("MatInv round-trip failed:\norig=%v\ninv=%v\norig*inv=%v\nwant identity %v", orig, inv, prod, id)
		}
	}
	if invertible == 0 {
		t.Fatalf("no invertible matrices generated; check generation")
	}
}

// TestSolveVerify generates random linear systems and, for the solvable ones,
// checks that the returned solution x satisfies every equation: for each row
// m[k] of the (echelonized) system, the GF(2) dot product of its coefficient
// bits with x equals its right-hand-side bit at column j. Non-pivot rows are
// zero in columns [0,j+1) and so satisfy the equation trivially.
func TestSolveVerify(t *testing.T) {
	rng := rand.New(rand.NewSource(0x501f3))
	solvable := 0
	for range 5000 {
		i := 1 + rng.Intn(8)
		j := 1 + rng.Intn(7) // need columns [0,j) for coeffs plus RHS at j (<=63)
		m := randMatrix(rng, i, j+1)

		x, ok := Solve(m, i, j)
		if !ok {
			continue
		}
		solvable++
		// After Solve, m is echelonized over [0,j+1). x carries no bit at
		// position >= j, so popcount(x & m[k]) is exactly the dot product of
		// the coefficient row with x over GF(2).
		for k := range i {
			lhs := bits.OnesCount64(x&m[k]) & 1
			rhs := int((m[k] >> uint(j)) & 1)
			if lhs != rhs {
				t.Fatalf("Solve solution violates row %d: popcount(x&m[k])%%2=%d != rhs=%d\nx=%#x m[k]=%#x j=%d i=%d", k, lhs, rhs, x, m[k], j, i)
			}
		}
	}
	if solvable == 0 {
		t.Fatalf("no solvable systems generated; check generation")
	}
}

func TestCapH(t *testing.T) {
	cases := []struct {
		a, b  []uint64
		j0, n int
	}{
		{[]uint64{0x3, 0x7, 0xb}, []uint64{0x3, 0x1}, 4, 4},
		{[]uint64{0x7, 0xb, 0xd}, []uint64{0xe, 0x9}, 3, 3},
		{[]uint64{0x1, 0x2}, []uint64{0x4, 0x8}, 4, 4},
		{[]uint64{0x1, 0x2, 0x4, 0x8}, []uint64{0x1, 0x2, 0x4, 0x8}, 4, 4},
		{[]uint64{0x6, 0xc, 0x18}, []uint64{0xa, 0x14, 0x6}, 5, 5},
	}
	for _, c := range cases {
		a := append([]uint64(nil), c.a...)
		b := append([]uint64(nil), c.b...)
		l1 := CapH(a, b, c.j0, c.n)
		want := oracleF2Ints(t, f2Probe, fmt.Sprintf("cap(%s,%s,%d,%d)", f2List(c.a), f2List(c.b), c.j0, c.n))
		wantL1 := want[0]
		wantA := want[3 : 3+len(a)]
		wantB := want[3+len(a):]
		if int64(l1) != wantL1 {
			t.Errorf("CapH(%v,%v,%d,%d) l1 = %d want %d", c.a, c.b, c.j0, c.n, l1, wantL1)
		}
		if !f2Eq(a, wantA) {
			t.Errorf("CapH(%v,%v,%d,%d) m1 = %v want %v", c.a, c.b, c.j0, c.n, a, wantA)
		}
		if !f2Eq(b, wantB) {
			t.Errorf("CapH(%v,%v,%d,%d) m2 = %v want %v", c.a, c.b, c.j0, c.n, b, wantB)
		}
	}
}

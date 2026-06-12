package swar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

// Oracle-parity tests for the bm64 bit-matrix primitives
// (bitmatrix64.c). Every Bm64* function here mirrors a C
// bitmatrix64_* entry point exposed by mmgroup.clifford12,
// so the package boundary is the natural test boundary:
// these are leaf SWAR helpers with a direct oracle at this
// exact granularity (the Td2 "leave as property tests only
// when no oracle exists" carve-out does not apply).
//
// Each test feeds the same random []uint64 row vector to
// the Go port and to the Cython wrapper and asserts that
// the mutated matrix (or returned matrix/scalar) is
// byte-identical. The driver builds a small numpy script
// per call rather than going through the typed Driver,
// because the oracle operates on uint64 arrays.

// bm64Oracle runs a bitmatrix64 oracle expression and
// decodes its JSON result as a []uint64. The script imports
// numpy and mmgroup.clifford12 as cl and must print
// json.dumps of a list of ints.
func bm64Oracle(t *testing.T, body string) []uint64 {
	t.Helper()
	script := "import json, numpy as np\nimport mmgroup.clifford12 as cl\n" + body
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("bitmatrix64 oracle failed: %v\n%s", err, out)
	}
	var v []uint64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("bitmatrix64 oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// bm64OracleScalar runs a bitmatrix64 oracle expression and
// decodes its JSON result as a single int64.
func bm64OracleScalar(t *testing.T, body string) int64 {
	t.Helper()
	script := "import json, numpy as np\nimport mmgroup.clifford12 as cl\n" + body
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("bitmatrix64 oracle failed: %v\n%s", err, out)
	}
	var v int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("bitmatrix64 oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// pyU64List renders a []uint64 as a Python list literal.
func pyU64List(m []uint64) string {
	parts := make([]string, len(m))
	for i, v := range m {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// npU64 renders a Python numpy-array constructor for m.
func npU64(m []uint64) string {
	return "np.array(" + pyU64List(m) + ",dtype=np.uint64)"
}

// eqU64 reports whether a and b are equal element-wise.
func eqU64(a, b []uint64) bool {
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

// lcg64 is a small deterministic generator of pseudo-random
// uint64 rows for the parity tests, keeping the oracle calls
// reproducible without a math/rand seed dance.
type lcg64 uint64

func (s *lcg64) next() uint64 {
	*s = lcg64(uint64(*s)*6364136223846793005 + 1442695040888963407)
	return uint64(*s)
}

// randRows returns n rows of width-bit random values (bits
// width..63 cleared so column indices stay in range).
func (s *lcg64) randRows(n, width int) []uint64 {
	mask := ^uint64(0)
	if width < 64 {
		mask = (uint64(1) << uint(width)) - 1
	}
	rows := make([]uint64, n)
	for i := range rows {
		rows[i] = s.next() & mask
	}
	return rows
}

func TestBm64TOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 1
	for _, dims := range [][2]int{{1, 1}, {3, 5}, {8, 8}, {12, 7}, {1, 24}} {
		i, j := dims[0], dims[1]
		m1 := s.randRows(i, j)
		m2 := make([]uint64, j)
		Bm64T(m1, i, j, m2)
		want := bm64Oracle(t, fmt.Sprintf(
			"print(json.dumps([int(x) for x in cl.bitmatrix64_t(%s, %d)]))", npU64(m1), j))
		if !eqU64(m2, want) {
			t.Errorf("Bm64T(i=%d,j=%d) = %v, want %v", i, j, m2, want)
		}
	}
}

func TestBm64MulOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 2
	for _, dims := range [][2]int{{1, 1}, {3, 3}, {5, 8}, {8, 5}, {12, 12}} {
		i1, i2 := dims[0], dims[1]
		m1 := s.randRows(i1, i2)
		m2 := s.randRows(i2, 24)
		m3 := make([]uint64, i1)
		Bm64Mul(m1, m2, i1, i2, m3)
		want := bm64Oracle(t, fmt.Sprintf(
			"print(json.dumps([int(x) for x in cl.bitmatrix64_mul(%s, %s)]))",
			npU64(m1), npU64(m2)))
		if !eqU64(m3, want) {
			t.Errorf("Bm64Mul(i1=%d,i2=%d) = %v, want %v", i1, i2, m3, want)
		}
	}
}

func TestBm64InvOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 3
	tried := 0
	for tried < 6 {
		i := 1 + int(s.next()%8)
		m := s.randRows(i, i)
		out := make([]uint64, i)
		copy(out, m)
		if !Bm64Inv(out, i) {
			// Singular: the oracle raises rather than returning;
			// skip and draw another matrix.
			continue
		}
		tried++
		want := bm64Oracle(t, fmt.Sprintf(
			"print(json.dumps([int(x) for x in cl.bitmatrix64_inv(%s)]))", npU64(m)))
		if !eqU64(out, want) {
			t.Errorf("Bm64Inv(i=%d, m=%v) = %v, want %v", i, m, out, want)
		}
	}
}

func TestBm64EchelonLOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 4
	for _, p := range []struct{ i, j0, n int }{
		{4, 0, 4}, {8, 0, 8}, {6, 2, 4}, {10, 3, 6}, {12, 0, 24},
	} {
		m := s.randRows(p.i, p.j0+p.n)
		got := append([]uint64(nil), m...)
		rank := Bm64EchelonL(got, p.i, p.j0, p.n)
		// The Cython wrapper mutates m in place and returns the
		// rank; capture both the rank and the resulting matrix.
		body := fmt.Sprintf(
			"m=%s\nr=int(cl.bitmatrix64_echelon_l(m,%d,%d,%d))\nprint(json.dumps([r]+[int(x) for x in m]))",
			npU64(m), p.i, p.j0, p.n)
		res := bm64Oracle(t, body)
		wantRank, wantM := int(res[0]), res[1:]
		if rank != wantRank {
			t.Errorf("Bm64EchelonL(i=%d,j0=%d,n=%d) rank=%d want %d", p.i, p.j0, p.n, rank, wantRank)
		}
		if !eqU64(got, wantM) {
			t.Errorf("Bm64EchelonL(i=%d,j0=%d,n=%d) m=%v want %v", p.i, p.j0, p.n, got, wantM)
		}
	}
}

func TestBm64EchelonHOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 5
	for _, p := range []struct{ i, j0, n int }{
		{4, 4, 4}, {8, 8, 8}, {6, 6, 4}, {10, 9, 6}, {12, 24, 24},
	} {
		m := s.randRows(p.i, p.j0)
		got := append([]uint64(nil), m...)
		rank := Bm64EchelonH(got, p.i, p.j0, p.n)
		body := fmt.Sprintf(
			"m=%s\nr=int(cl.bitmatrix64_echelon_h(m,%d,%d,%d))\nprint(json.dumps([r]+[int(x) for x in m]))",
			npU64(m), p.i, p.j0, p.n)
		res := bm64Oracle(t, body)
		wantRank, wantM := int(res[0]), res[1:]
		if rank != wantRank {
			t.Errorf("Bm64EchelonH(i=%d,j0=%d,n=%d) rank=%d want %d", p.i, p.j0, p.n, rank, wantRank)
		}
		if !eqU64(got, wantM) {
			t.Errorf("Bm64EchelonH(i=%d,j0=%d,n=%d) m=%v want %v", p.i, p.j0, p.n, got, wantM)
		}
	}
}

func TestBm64RotBitsOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 6
	for _, p := range []struct{ rot, nrot, n0 int }{
		{1, 8, 0}, {3, 12, 4}, {-2, 16, 8}, {5, 24, 0}, {-7, 11, 13},
	} {
		m := s.randRows(5, p.n0+p.nrot)
		got := append([]uint64(nil), m...)
		Bm64RotBits(got, len(got), p.rot, p.nrot, p.n0)
		body := fmt.Sprintf(
			"m=%s\ncl.bitmatrix64_rot_bits(m,%d,%d,%d)\nprint(json.dumps([int(x) for x in m]))",
			npU64(m), p.rot, p.nrot, p.n0)
		want := bm64Oracle(t, body)
		if !eqU64(got, want) {
			t.Errorf("Bm64RotBits(rot=%d,nrot=%d,n0=%d) = %v, want %v", p.rot, p.nrot, p.n0, got, want)
		}
	}
}

func TestBm64ReverseBitsOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 7
	for _, p := range []struct{ n, n0 int }{
		{8, 0}, {12, 4}, {16, 8}, {24, 0}, {11, 13},
	} {
		m := s.randRows(5, p.n0+p.n)
		got := append([]uint64(nil), m...)
		Bm64ReverseBits(got, len(got), p.n, p.n0)
		body := fmt.Sprintf(
			"m=%s\ncl.bitmatrix64_reverse_bits(m,%d,%d)\nprint(json.dumps([int(x) for x in m]))",
			npU64(m), p.n, p.n0)
		want := bm64Oracle(t, body)
		if !eqU64(got, want) {
			t.Errorf("Bm64ReverseBits(n=%d,n0=%d) = %v, want %v", p.n, p.n0, got, want)
		}
	}
}

func TestBm64XchBitsOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 8
	// mask and mask>>sh must not overlap (the function's
	// contract); choose disjoint low/high column blocks.
	for _, p := range []struct {
		sh   int
		mask uint64
	}{
		{1, 0x5}, {4, 0x0f0f}, {8, 0x00ff}, {16, 0xffff}, {2, 0x33},
	} {
		m := s.randRows(6, 64)
		got := append([]uint64(nil), m...)
		Bm64XchBits(got, len(got), p.sh, p.mask)
		body := fmt.Sprintf(
			"m=%s\ncl.bitmatrix64_xch_bits(m,%d,%d)\nprint(json.dumps([int(x) for x in m]))",
			npU64(m), p.sh, p.mask)
		want := bm64Oracle(t, body)
		if !eqU64(got, want) {
			t.Errorf("Bm64XchBits(sh=%d,mask=%#x) = %v, want %v", p.sh, p.mask, got, want)
		}
	}
}

func TestBm64MaskRowsOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 9
	for _, mask := range []uint64{0, 0xff, 0xf0f0, 0xffffffff, 0xabcdef} {
		m := s.randRows(7, 64)
		got := append([]uint64(nil), m...)
		Bm64MaskRows(got, len(got), mask)
		body := fmt.Sprintf(
			"m=%s\ncl.bitmatrix64_mask_rows(m,%d)\nprint(json.dumps([int(x) for x in m]))",
			npU64(m), mask)
		want := bm64Oracle(t, body)
		if !eqU64(got, want) {
			t.Errorf("Bm64MaskRows(mask=%#x) = %v, want %v", mask, got, want)
		}
	}
}

func TestBm64AddDiagOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 10
	for _, j := range []int{0, 1, 5, 20, 40} {
		m := s.randRows(6, 64)
		got := append([]uint64(nil), m...)
		Bm64AddDiag(got, len(got), j)
		body := fmt.Sprintf(
			"m=%s\ncl.bitmatrix64_add_diag(m,%d)\nprint(json.dumps([int(x) for x in m]))",
			npU64(m), j)
		want := bm64Oracle(t, body)
		if !eqU64(got, want) {
			t.Errorf("Bm64AddDiag(j=%d) = %v, want %v", j, got, want)
		}
	}
}

func TestBm64FindLowBitOracle(t *testing.T) {
	t.Parallel()
	var s lcg64 = 11
	for _, p := range []struct{ imin, imax int }{
		{0, 64}, {3, 64}, {0, 128}, {10, 50}, {64, 192},
	} {
		// Width enough rows to cover imax bits: one row per 64
		// columns, plus a guard row.
		rows := (p.imax+63)/64 + 1
		m := s.randRows(rows, 64)
		got := Bm64FindLowBit(m, p.imin, p.imax)
		want := bm64OracleScalar(t, fmt.Sprintf(
			"print(json.dumps(int(cl.bitmatrix64_find_low_bit(%s,%d,%d))))",
			npU64(m), p.imin, p.imax))
		if int64(got) != want {
			t.Errorf("Bm64FindLowBit(imin=%d,imax=%d, m=%v) = %d, want %d", p.imin, p.imax, m, got, want)
		}
	}
}

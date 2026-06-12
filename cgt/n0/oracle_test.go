package n0

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

// Oracle-parity tests for the N_0 group arithmetic
// (mm_group_n.c). Each exported function here mirrors a C
// mm_group_n_* entry point exposed by mmgroup.generators,
// so the package boundary is the natural test boundary.
//
// An element of N_0 is the five-component vector
// [t, y, x, d, pi]; the oracle holds it as a length-5 numpy
// uint32 array, identical to N0Elem. Every test builds a
// random element from a random N_0 word (so both sides start
// from the same state), runs the Go port and the Cython
// wrapper, and asserts that the mutated array / returned
// word / scalar agree byte-for-byte.

// n0Oracle runs an N_0 oracle script and decodes its JSON
// result as a []int64. The script imports numpy and
// mmgroup.generators as G and must print json.dumps of a
// list of ints.
func n0Oracle(t *testing.T, body string) []int64 {
	t.Helper()
	script := "import json, numpy as np\nimport mmgroup.generators as G\n" + body
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("mm_group_n oracle failed: %v\n%s", err, out)
	}
	var v []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("mm_group_n oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// n0OracleScalar runs an N_0 oracle script and decodes its
// JSON result as a single int64.
func n0OracleScalar(t *testing.T, body string) int64 {
	t.Helper()
	script := "import json, numpy as np\nimport mmgroup.generators as G\n" + body
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("mm_group_n oracle failed: %v\n%s", err, out)
	}
	var v int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("mm_group_n oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// lcg32 is a deterministic pseudo-random generator used to
// build reproducible N_0 words for the parity tests.
type lcg32 uint64

func (s *lcg32) next() uint32 {
	*s = lcg32(uint64(*s)*6364136223846793005 + 1442695040888963407)
	return uint32(uint64(*s) >> 32)
}

// randWord returns a random word of n N_0 generator atoms
// (tags d=1, p=2, x=3, y=4, t=5). The atom payloads are
// masked into the legal range for each tag.
func (s *lcg32) randWord(n int) []uint32 {
	w := make([]uint32, n)
	for i := range w {
		switch s.next() % 5 {
		case 0:
			w[i] = 0x10000000 | (s.next() & 0xfff) // d
		case 1:
			w[i] = 0x20000000 | (s.next() % 244823040) // p (Mat24 order)
		case 2:
			w[i] = 0x30000000 | (s.next() & 0x1fff) // x
		case 3:
			w[i] = 0x40000000 | (s.next() & 0x1fff) // y
		default:
			w[i] = 0x50000000 | (s.next() % 3) // t
		}
	}
	return w
}

// pyU32List renders a []uint32 as a Python list literal.
func pyU32List(w []uint32) string {
	parts := make([]string, len(w))
	for i, v := range w {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// npU32 renders a numpy uint32 array constructor for w.
func npU32(w []uint32) string {
	return "np.array(" + pyU32List(w) + ",dtype=np.uint32)"
}

// elemFromWord scans w into a Go N0Elem.
func elemFromWord(w []uint32) N0Elem {
	var g N0Elem
	MulWordScan(&g, w)
	return g
}

// oracleElemFromWord is the Python statement prefix that
// builds the same N_0 element g from word w into a length-5
// numpy array. It leaves the variable g bound.
func oracleElemFromWord(w []uint32) string {
	return fmt.Sprintf("g=np.zeros(5,dtype=np.uint32)\nw=%s\nG.mm_group_n_mul_word_scan(g,w,len(w))\n", npU32(w))
}

// eqElem reports whether g matches the oracle's length-5
// result.
func eqElem(g *N0Elem, want []int64) bool {
	if len(want) != 5 {
		return false
	}
	for i := 0; i < 5; i++ {
		if int64(g[i]) != want[i] {
			return false
		}
	}
	return true
}

func TestMulWordScanOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 1
	for _, n := range []int{0, 1, 3, 7, 12} {
		w := s.randWord(n)
		var g N0Elem
		got := MulWordScan(&g, w)
		body := oracleElemFromWord(w) +
			"l=int(G.mm_group_n_mul_word_scan(np.zeros(5,dtype=np.uint32),w,len(w)))\n" +
			"print(json.dumps([l]+[int(x) for x in g]))"
		res := n0Oracle(t, body)
		if int(got) != int(res[0]) {
			t.Errorf("MulWordScan(n=%d) len=%d want %d", n, got, res[0])
		}
		if !eqElem(&g, res[1:]) {
			t.Errorf("MulWordScan(n=%d) elem=%v want %v", n, g, res[1:])
		}
	}
}

func TestMulAtomOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 2
	for i := 0; i < 12; i++ {
		w := s.randWord(4)
		atom := s.randWord(1)[0]
		g := elemFromWord(w)
		got := MulAtom(&g, atom)
		body := oracleElemFromWord(w) +
			fmt.Sprintf("r=int(G.mm_group_n_mul_atom(g,%d))\n", atom) +
			"print(json.dumps([r]+[int(x) for x in g]))"
		res := n0Oracle(t, body)
		if int64(got) != res[0] {
			t.Errorf("MulAtom(atom=%#x) ret=%#x want %#x", atom, got, res[0])
		}
		if !eqElem(&g, res[1:]) {
			t.Errorf("MulAtom(atom=%#x) elem=%v want %v", atom, g, res[1:])
		}
	}
}

func TestReduceElementOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 3
	for i := 0; i < 12; i++ {
		w := s.randWord(8)
		g := elemFromWord(w)
		got := ReduceElement(&g)
		body := oracleElemFromWord(w) +
			"r=int(G.mm_group_n_reduce_element(g))\n" +
			"print(json.dumps([r]+[int(x) for x in g]))"
		res := n0Oracle(t, body)
		// The C return is the OR of components (nonzero iff g is
		// non-neutral); the Go contract only fixes the zero/nonzero
		// distinction, so compare booleans.
		if (got != 0) != (res[0] != 0) {
			t.Errorf("ReduceElement neutral=%v want %v", got == 0, res[0] == 0)
		}
		if !eqElem(&g, res[1:]) {
			t.Errorf("ReduceElement elem=%v want %v", g, res[1:])
		}
	}
}

func TestToWordOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 4
	for i := 0; i < 12; i++ {
		w := s.randWord(8)
		g := elemFromWord(w)
		var out [5]uint32
		n := ToWord(&g, out[:])
		body := oracleElemFromWord(w) +
			"o=np.zeros(5,dtype=np.uint32)\nk=int(G.mm_group_n_to_word(g,o))\n" +
			"print(json.dumps([k]+[int(x) for x in o[:k]]))"
		res := n0Oracle(t, body)
		if int(n) != int(res[0]) {
			t.Errorf("ToWord len=%d want %d", n, res[0])
			continue
		}
		for j := 0; j < int(n); j++ {
			if int64(out[j]) != res[1+j] {
				t.Errorf("ToWord[%d]=%#x want %#x", j, out[j], res[1+j])
			}
		}
	}
}

func TestToWordStdOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 5
	for i := 0; i < 12; i++ {
		w := s.randWord(8)
		g := elemFromWord(w)
		var out [5]uint32
		n := ToWordStd(&g, out[:])
		body := oracleElemFromWord(w) +
			"o=np.zeros(5,dtype=np.uint32)\nk=int(G.mm_group_n_to_word_std(g,o))\n" +
			"print(json.dumps([k]+[int(x) for x in o[:k]]))"
		res := n0Oracle(t, body)
		if int(n) != int(res[0]) {
			t.Errorf("ToWordStd len=%d want %d", n, res[0])
			continue
		}
		for j := 0; j < int(n); j++ {
			if int64(out[j]) != res[1+j] {
				t.Errorf("ToWordStd[%d]=%#x want %#x", j, out[j], res[1+j])
			}
		}
	}
}

func TestInvElementOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 6
	for i := 0; i < 12; i++ {
		w := s.randWord(6)
		g1 := elemFromWord(w)
		var g2 N0Elem
		InvElement(&g1, &g2)
		body := oracleElemFromWord(w) +
			"o=np.zeros(5,dtype=np.uint32)\nG.mm_group_n_inv_element(g,o)\n" +
			"print(json.dumps([int(x) for x in o]))"
		res := n0Oracle(t, body)
		if !eqElem(&g2, res) {
			t.Errorf("InvElement elem=%v want %v", g2, res)
		}
	}
}

func TestMulElementOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 7
	for i := 0; i < 12; i++ {
		w1, w2 := s.randWord(5), s.randWord(5)
		g1, g2 := elemFromWord(w1), elemFromWord(w2)
		var g3 N0Elem
		MulElement(&g1, &g2, &g3)
		body := fmt.Sprintf(
			"a=np.zeros(5,dtype=np.uint32)\nw1=%s\nG.mm_group_n_mul_word_scan(a,w1,len(w1))\n"+
				"b=np.zeros(5,dtype=np.uint32)\nw2=%s\nG.mm_group_n_mul_word_scan(b,w2,len(w2))\n"+
				"o=np.zeros(5,dtype=np.uint32)\nG.mm_group_n_mul_element(a,b,o)\n"+
				"print(json.dumps([int(x) for x in o]))",
			npU32(w1), npU32(w2))
		res := n0Oracle(t, body)
		if !eqElem(&g3, res) {
			t.Errorf("MulElement elem=%v want %v", g3, res)
		}
	}
}

func TestConjugateElementOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 8
	for i := 0; i < 12; i++ {
		w1, w2 := s.randWord(5), s.randWord(5)
		g1, g2 := elemFromWord(w1), elemFromWord(w2)
		var g3 N0Elem
		ConjugateElement(&g1, &g2, &g3)
		body := fmt.Sprintf(
			"a=np.zeros(5,dtype=np.uint32)\nw1=%s\nG.mm_group_n_mul_word_scan(a,w1,len(w1))\n"+
				"b=np.zeros(5,dtype=np.uint32)\nw2=%s\nG.mm_group_n_mul_word_scan(b,w2,len(w2))\n"+
				"o=np.zeros(5,dtype=np.uint32)\nG.mm_group_n_conjugate_element(a,b,o)\n"+
				"print(json.dumps([int(x) for x in o]))",
			npU32(w1), npU32(w2))
		res := n0Oracle(t, body)
		if !eqElem(&g3, res) {
			t.Errorf("ConjugateElement elem=%v want %v", g3, res)
		}
	}
}

func TestRightCosetNx0Oracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 9
	for i := 0; i < 12; i++ {
		w := s.randWord(7)
		g := elemFromWord(w)
		got := RightCosetNx0(g[:])
		body := oracleElemFromWord(w) +
			"e=int(G.mm_group_n_right_coset_N_x0(g))\n" +
			"print(json.dumps([e]+[int(x) for x in g]))"
		res := n0Oracle(t, body)
		if int64(got) != res[0] {
			t.Errorf("RightCosetNx0 e=%d want %d", got, res[0])
		}
		if !eqElem(&g, res[1:]) {
			t.Errorf("RightCosetNx0 elem=%v want %v", g, res[1:])
		}
	}
}

func TestConjToQx0Oracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 10
	for i := 0; i < 16; i++ {
		w := s.randWord(6)
		g := elemFromWord(w)
		got := ConjToQx0(g[:])
		body := oracleElemFromWord(w) +
			"print(json.dumps(int(G.mm_group_n_conj_to_Q_x0(g))))"
		want := n0OracleScalar(t, body)
		if int64(got) != want {
			t.Errorf("ConjToQx0(g=%v) = %d want %d", g, got, want)
		}
	}
}

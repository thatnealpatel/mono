package cgt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// bitMat is a test-only GroupElem: an invertible n x n bit
// matrix over GF(2) whose rows are stored as integers
// (row i is rows[i], acting on the basis vector 1<<i). It
// acts on a vector v by right multiplication v -> v*rows.
type bitMat struct {
	n    uint32
	rows []uint32
}

// linMap is the MapFunc for bitMat group elements under the
// identity representation (Python's lambda x: x): it returns
// the matrix rows with a zero affine offset appended.
func (m bitMat) mapEntries() []uint32 {
	out := make([]uint32, m.n+1)
	copy(out, m.rows)
	return out
}

func (m bitMat) Mul(other GroupElem) GroupElem {
	o := other.(bitMat)
	res := make([]uint32, m.n)
	for i := uint32(0); i < m.n; i++ {
		res[i] = vmatmulAff(m.rows[i], append(append([]uint32(nil), o.rows...), 0), m.n)
	}
	return bitMat{n: m.n, rows: res}
}

func (m bitMat) Pow(e int) GroupElem {
	switch e {
	case 0:
		res := make([]uint32, m.n)
		for i := range res {
			res[i] = 1 << uint(i)
		}
		return bitMat{n: m.n, rows: res}
	case 1:
		return m
	case -1:
		mm := make([]uint32, m.n+1)
		copy(mm, m.rows)
		inv := make([]uint32, m.n+1)
		if matInverseAff(mm, m.n, inv) != 0 {
			panic("bitMat.Pow(-1): not invertible")
		}
		return bitMat{n: m.n, rows: inv[:m.n]}
	default:
		panic("bitMat.Pow: unsupported exponent")
	}
}

// orbitLin2Map is the identity MapFunc used in the tests.
func orbitLin2Map(g GroupElem) []uint32 {
	return g.(bitMat).mapEntries()
}

// orbitOracle builds an Orbit_Lin2 from the given bit-matrix
// rows in Python and prints the requested expression as
// JSON.
func orbitOracle(t *testing.T, gens [][]uint32, expr string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("import json\nfrom mmgroup.general import Orbit_Lin2\n")
	b.WriteString("gens=[")
	for i, g := range gens {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(pyListU32(g))
	}
	b.WriteString("]\n")
	b.WriteString("o=Orbit_Lin2(lambda x: x, gens)\n")
	fmt.Fprintf(&b, "print(json.dumps(%s))\n", expr)
	out, err := pyCmd(b.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("orbit oracle failed: %v\n%s", err, out)
	}
	return []byte(strings.TrimSpace(string(out)))
}

func orbitOracleInts(t *testing.T, gens [][]uint32, expr string) []int64 {
	t.Helper()
	var v []int64
	if err := json.Unmarshal(orbitOracle(t, gens, expr), &v); err != nil {
		t.Fatalf("unmarshal %q: %v", expr, err)
	}
	return v
}

func orbitOracleInt(t *testing.T, gens [][]uint32, expr string) int64 {
	t.Helper()
	var v int64
	if err := json.Unmarshal(orbitOracle(t, gens, expr), &v); err != nil {
		t.Fatalf("unmarshal %q: %v", expr, err)
	}
	return v
}

func newBitMatOrbit(gens [][]uint32) *OrbitLin2 {
	n := uint32(len(gens[0]))
	elems := make([]GroupElem, len(gens))
	for i, g := range gens {
		rows := append([]uint32(nil), g...)
		elems[i] = bitMat{n: n, rows: rows}
	}
	return NewOrbitLin2(orbitLin2Map, elems)
}

func equalU32Int64(a []uint32, b []int64) bool {
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

// TestOrbitLin2SmallLinear compares the engine-delegating
// OrbitLin2 surface against mmgroup.general.Orbit_Lin2 on a
// small linear group acting on GF(2)^3.
func TestOrbitLin2SmallLinear(t *testing.T) {
	gens := [][]uint32{
		{0b010, 0b001, 0b100}, // cyclic shift of coordinates
		{0b100, 0b010, 0b001}, // a reflection
	}
	o := newBitMatOrbit(gens)

	if got, want := int64(o.Dim()), orbitOracleInt(t, gens, "o.dim"); got != want {
		t.Fatalf("Dim()=%d want %d", got, want)
	}
	if got, want := int64(o.NOrbits()), orbitOracleInt(t, gens, "o.n_orbits()"); got != want {
		t.Fatalf("NOrbits()=%d want %d", got, want)
	}

	reps := o.Representatives()
	wantReps := orbitOracleInts(t, gens, "[int(x) for x in o.representatives()[0]]")
	wantSizes := orbitOracleInts(t, gens, "[int(x) for x in o.representatives()[1]]")
	if !equalU32Int64(reps.Reps, wantReps) {
		t.Fatalf("Representatives reps=%v want %v", reps.Reps, wantReps)
	}
	if !equalU32Int64(reps.Sizes, wantSizes) {
		t.Fatalf("Representatives sizes=%v want %v", reps.Sizes, wantSizes)
	}

	for v := 0; v < 8; v++ {
		if got, want := int64(o.OrbitRep(v)), orbitOracleInt(t, gens, fmt.Sprintf("int(o.orbit_rep(%d))", v)); got != want {
			t.Errorf("OrbitRep(%d)=%d want %d", v, got, want)
		}
		if got, want := int64(o.OrbitSize(v)), orbitOracleInt(t, gens, fmt.Sprintf("int(o.orbit_size(%d))", v)); got != want {
			t.Errorf("OrbitSize(%d)=%d want %d", v, got, want)
		}
		got := o.Orbit(v)
		want := orbitOracleInts(t, gens, fmt.Sprintf("sorted(int(x) for x in o.orbit(%d))", v))
		// The engine's orbit order is not sorted; compare as
		// sets by sorting the Go result.
		sortU32(got)
		if !equalU32Int64(got, want) {
			t.Errorf("Orbit(%d)=%v want %v", v, got, want)
		}
	}
}

// TestOrbitLin2Compress checks that Compress produces a
// compressed object whose orbit queries match Python's
// Orbit_Lin2.compress.
func TestOrbitLin2Compress(t *testing.T) {
	gens := [][]uint32{
		{0b010, 0b001, 0b100},
		{0b100, 0b010, 0b001},
	}
	o := newBitMatOrbit(gens)
	c := o.Compress([]int{1})

	setup := "o=Orbit_Lin2(lambda x: x, gens)\nc=o.compress([1])\n"
	wantN := orbitOracleSetup(t, gens, setup, "int(c.n_orbits())")
	if got := int64(c.NOrbits()); got != wantN {
		t.Fatalf("compressed NOrbits()=%d want %d", got, wantN)
	}
	if got, want := int64(c.OrbitRep(2)), orbitOracleSetup(t, gens, setup, "int(c.orbit_rep(2))"); got != want {
		t.Errorf("compressed OrbitRep(2)=%d want %d", got, want)
	}
	if got, want := int64(c.OrbitSize(1)), orbitOracleSetup(t, gens, setup, "int(c.orbit_size(1))"); got != want {
		t.Errorf("compressed OrbitSize(1)=%d want %d", got, want)
	}
}

// orbitOracleSetup runs an Orbit_Lin2 oracle with a custom
// setup block (which may rebind `o` and define `c`) and
// returns the requested int expression.
func orbitOracleSetup(t *testing.T, gens [][]uint32, setup, expr string) int64 {
	t.Helper()
	var b strings.Builder
	b.WriteString("import json\nfrom mmgroup.general import Orbit_Lin2\n")
	b.WriteString("gens=[")
	for i, g := range gens {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(pyListU32(g))
	}
	b.WriteString("]\n")
	b.WriteString(setup)
	fmt.Fprintf(&b, "print(json.dumps(%s))\n", expr)
	out, err := pyCmd(b.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("orbit oracle setup failed: %v\n%s", err, out)
	}
	var v int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("unmarshal %q: %v", expr, err)
	}
	return v
}

// TestOrbitLin2MapWord checks that MapVWordG produces a word
// that actually maps v to its orbit representative, and that
// the engine word matches the oracle's map_v_word_G generator
// indices.
func TestOrbitLin2MapWord(t *testing.T) {
	gens := [][]uint32{
		{0b010, 0b001, 0b100},
		{0b100, 0b010, 0b001},
	}
	o := newBitMatOrbit(gens)

	for v := 0; v < 8; v++ {
		w := o.MapVWordG(v, -1)
		// Apply the word to v and check we reach the rep.
		cur := uint32(v)
		gg := [2][]bitMat{}
		for _, g := range gens {
			gg[0] = append(gg[0], bitMat{n: 3, rows: g})
		}
		// Build inverses for sign -1.
		for _, g := range gens {
			inv := bitMat{n: 3, rows: g}.Pow(-1).(bitMat)
			gg[1] = append(gg[1], inv)
		}
		for _, e := range w {
			m := gg[0][e.Gen]
			if e.Sign < 0 {
				m = gg[1][e.Gen]
			}
			cur = vmatmulAff(cur, append(append([]uint32(nil), m.rows...), 0), 3)
		}
		if int(cur) != o.OrbitRep(v) {
			t.Errorf("MapVWordG(%d) word maps %d to %d, want rep %d", v, v, cur, o.OrbitRep(v))
		}
	}

	// Oracle cross-check of the generator indices for the
	// vectors with nontrivial words. The oracle requires real
	// group elements supporting **; we reconstruct map_v_word_G
	// via the raw engine map_v on the Python side instead.
	wantV2 := orbitMapWordGenSigns(t, gens, 2)
	gotV2 := o.MapVWordG(2, -1)
	if len(gotV2) != len(wantV2) {
		t.Fatalf("MapVWordG(2) len=%d want %d", len(gotV2), len(wantV2))
	}
	for i := range gotV2 {
		if gotV2[i].Gen != wantV2[i][0] || gotV2[i].Sign != wantV2[i][1] {
			t.Errorf("MapVWordG(2)[%d]=%v want gen=%d sign=%d",
				i, gotV2[i], wantV2[i][0], wantV2[i][1])
		}
	}
}

// orbitMapWordGenSigns reconstructs map_v_word_G's
// (generator-index, sign) pairs on the Python side via the
// raw engine map_v call, avoiding the need for ** on the
// generators.
func orbitMapWordGenSigns(t *testing.T, gens [][]uint32, v uint32) [][2]int {
	t.Helper()
	var b strings.Builder
	b.WriteString("import json, numpy as np\nfrom mmgroup import generators as G\n")
	fmt.Fprintf(&b, "n=%d\nnG=%d\n", len(gens[0]), len(gens))
	b.WriteString("sz=G.gen_ufind_lin2_size(n,nG)\na=np.zeros(sz,dtype=np.uint32)\nG.gen_ufind_lin2_init(a,sz,n,nG)\n")
	for _, g := range gens {
		fmt.Fprintf(&b, "g=np.append(np.array(%s,dtype=np.uint32),np.uint32(0))\nG.gen_ufind_lin2_add(a,g,n+1)\n", pyListU32(g))
	}
	b.WriteString("G.gen_ufind_lin2_finalize(a)\n")
	fmt.Fprintf(&b, "buf=np.zeros(64,dtype=np.uint8)\nr=G.gen_ufind_lin2_map_v(a,%d,buf,64)\n", v)
	b.WriteString("print(json.dumps([[int(x>>1), 1 if (x&1)==0 else -1] for x in buf[:r]]))\n")
	out, err := pyCmd(b.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("map word oracle failed: %v\n%s", err, out)
	}
	var pairs [][2]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &pairs); err != nil {
		t.Fatalf("unmarshal map word: %v", err)
	}
	return pairs
}

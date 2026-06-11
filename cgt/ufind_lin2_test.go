package cgt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ufindOracle runs a Python script against the mmgroup
// generators module and returns the JSON it prints. The
// script must `print(json.dumps(...))`. numpy, json and
// generators are imported into the script namespace.
func ufindOracle(t *testing.T, body string) []byte {
	t.Helper()
	script := "import json, numpy as np\nfrom mmgroup import generators\n" + body
	out, err := pyCmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("ufind oracle failed: %v\n%s", err, out)
	}
	return []byte(strings.TrimSpace(string(out)))
}

// buildOracleArray asks the oracle to build, add the
// generators in gens, finalize, and return the full
// packed orbit array a as a list of uint32.
func buildOracleArray(t *testing.T, n uint32, nG uint32, gens [][]uint32) []uint32 {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "n=%d\nnG=%d\n", n, nG)
	b.WriteString("sz=generators.gen_ufind_lin2_size(n, nG)\n")
	b.WriteString("a=np.zeros(sz, dtype=np.uint32)\n")
	b.WriteString("generators.gen_ufind_lin2_init(a, sz, n, nG)\n")
	for _, g := range gens {
		fmt.Fprintf(&b, "g=np.array(%s, dtype=np.uint32)\n", pyListU32(g))
		b.WriteString("generators.gen_ufind_lin2_add(a, g, len(g))\n")
	}
	b.WriteString("generators.gen_ufind_lin2_finalize(a)\n")
	b.WriteString("print(json.dumps(a.tolist()))\n")
	out := ufindOracle(t, b.String())
	var arr []uint32
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unmarshal oracle array %q: %v", out, err)
	}
	return arr
}

func pyListU32(g []uint32) string {
	parts := make([]string, len(g))
	for i, v := range g {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// buildGoArray performs the same construction in Go and
// returns the finalized packed array.
func buildGoArray(t *testing.T, n uint32, nG uint32, gens [][]uint32) []uint32 {
	t.Helper()
	sz := UFindLin2Size(n, nG)
	a := make([]uint32, sz)
	UFindLin2Init(a, uint32(sz), n, nG)
	for _, g := range gens {
		UFindLin2Add(a, g, uint32(len(g)))
	}
	UFindLin2Finalize(a)
	return a
}

// testGroups is a set of deterministic small bit-matrix
// groups acting on GF(2)^n.
var testGroups = []struct {
	name string
	n    uint32
	nG   uint32
	gens [][]uint32
}{
	{
		// n=3, single transposition swapping bits 0,1.
		name: "swap01_n3",
		n:    3, nG: 4,
		gens: [][]uint32{{0b010, 0b100, 0b001}},
	},
	{
		// n=4, cyclic shift of all 4 coordinates.
		name: "cyclic_n4",
		n:    4, nG: 4,
		gens: [][]uint32{{0b0010, 0b0100, 0b1000, 0b0001}},
	},
	{
		// n=5, two generators: a transposition and a
		// 3-cycle, generating a larger orbit structure.
		name: "two_gen_n5",
		n:    5, nG: 4,
		gens: [][]uint32{
			{0b00010, 0b00001, 0b00100, 0b01000, 0b10000},
			{0b00010, 0b00100, 0b00001, 0b01000, 0b10000},
		},
	},
	{
		// n=6, full cyclic shift; large single sweep.
		name: "cyclic_n6",
		n:    6, nG: 4,
		gens: [][]uint32{{0b000010, 0b000100, 0b001000, 0b010000, 0b100000, 0b000001}},
	},
}

func TestUFindLin2ArrayParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := buildOracleArray(t, tc.n, tc.nG, tc.gens)
			got := buildGoArray(t, tc.n, tc.nG, tc.gens)
			if len(got) != len(want) {
				t.Fatalf("array length: got %d want %d", len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("array[%d]: got %d want %d\nfull got=%v\nfull want=%v",
						i, got[i], want[i], got, want)
				}
			}
		})
	}
}

func TestUFindLin2OrbitInfoParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := buildGoArray(t, tc.n, tc.nG, tc.gens)

			// Oracle: counts, reps, lengths, table, map,
			// per-vector rep and orbit length.
			var b strings.Builder
			fmt.Fprintf(&b, "n=%d\nnG=%d\n", tc.n, tc.nG)
			b.WriteString("sz=generators.gen_ufind_lin2_size(n, nG)\n")
			b.WriteString("a=np.zeros(sz, dtype=np.uint32)\n")
			b.WriteString("generators.gen_ufind_lin2_init(a, sz, n, nG)\n")
			for _, g := range tc.gens {
				fmt.Fprintf(&b, "g=np.array(%s, dtype=np.uint32)\n", pyListU32(g))
				b.WriteString("generators.gen_ufind_lin2_add(a, g, len(g))\n")
			}
			b.WriteString("generators.gen_ufind_lin2_finalize(a)\n")
			b.WriteString("N=1<<n\n")
			b.WriteString("norb=generators.gen_ufind_lin2_n_orbits(a)\n")
			b.WriteString("r=np.zeros(N,dtype=np.uint32); nr=generators.gen_ufind_lin2_representatives(a,r,N)\n")
			b.WriteString("l=np.zeros(N,dtype=np.uint32); nl=generators.gen_ufind_lin2_orbit_lengths(a,l,N)\n")
			b.WriteString("tb=np.zeros(N,dtype=np.uint32); generators.gen_ufind_lin2_get_table(a,tb,N)\n")
			b.WriteString("mp=np.zeros(N,dtype=np.uint32); generators.gen_ufind_lin2_get_map(a,mp,N)\n")
			b.WriteString("repv=[int(generators.gen_ufind_lin2_rep_v(a,v)) for v in range(N)]\n")
			b.WriteString("lenv=[int(generators.gen_ufind_lin2_len_orbit_v(a,v)) for v in range(N)]\n")
			b.WriteString("out={'norb':int(norb),'reps':r[:nr].tolist(),'lens':l[:nl].tolist()," +
				"'table':tb.tolist(),'map':mp.tolist(),'repv':repv,'lenv':lenv}\n")
			b.WriteString("print(json.dumps(out))\n")
			raw := ufindOracle(t, b.String())
			var want struct {
				Norb  int      `json:"norb"`
				Reps  []uint32 `json:"reps"`
				Lens  []uint32 `json:"lens"`
				Table []uint32 `json:"table"`
				Map   []uint32 `json:"map"`
				RepV  []int32  `json:"repv"`
				LenV  []int32  `json:"lenv"`
			}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("unmarshal oracle info %q: %v", raw, err)
			}

			N := uint32(1) << tc.n

			if gotN := UFindLin2NOrbits(a); int(gotN) != want.Norb {
				t.Errorf("n_orbits: got %d want %d", gotN, want.Norb)
			}

			reps := make([]uint32, N)
			nr := UFindLin2Representatives(a, reps, N)
			gotReps := reps[:nr]
			if !equalU32(gotReps, want.Reps) {
				t.Errorf("representatives: got %v want %v", gotReps, want.Reps)
			}

			lens := make([]uint32, N)
			nl := UFindLin2OrbitLengths(a, lens, N)
			gotLens := lens[:nl]
			if !equalU32(gotLens, want.Lens) {
				t.Errorf("orbit_lengths: got %v want %v", gotLens, want.Lens)
			}

			tbl := make([]uint32, N)
			UFindLin2GetTable(a, tbl, N)
			if !equalU32(tbl, want.Table) {
				t.Errorf("get_table: got %v want %v", tbl, want.Table)
			}

			mp := make([]uint32, N)
			UFindLin2GetMap(a, mp, N)
			if !equalU32(mp, want.Map) {
				t.Errorf("get_map: got %v want %v", mp, want.Map)
			}

			for v := uint32(0); v < N; v++ {
				if got := UFindLin2RepV(a, v); got != want.RepV[v] {
					t.Errorf("rep_v(%d): got %d want %d", v, got, want.RepV[v])
				}
				if got := UFindLin2LenOrbitV(a, v); got != want.LenV[v] {
					t.Errorf("len_orbit_v(%d): got %d want %d", v, got, want.LenV[v])
				}
			}
		})
	}
}

// TestUFindLin2OrbitVParity checks that the explicit
// orbit of each representative matches the oracle.
func TestUFindLin2OrbitVParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := buildGoArray(t, tc.n, tc.nG, tc.gens)
			N := uint32(1) << tc.n

			reps := make([]uint32, N)
			nr := UFindLin2Representatives(a, reps, N)

			for _, rep := range reps[:nr] {
				want := oracleOrbitV(t, tc.n, tc.nG, tc.gens, rep)
				ln := uint32(UFindLin2LenOrbitV(a, rep))
				got := make([]uint32, ln)
				gn := UFindLin2OrbitV(a, rep, got, ln)
				got = got[:gn]
				if !equalU32(got, want) {
					t.Errorf("orbit_v(%d): got %v want %v", rep, got, want)
				}
			}
		})
	}
}

func oracleOrbitV(t *testing.T, n, nG uint32, gens [][]uint32, v uint32) []uint32 {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "n=%d\nnG=%d\nv=%d\n", n, nG, v)
	b.WriteString("sz=generators.gen_ufind_lin2_size(n, nG)\n")
	b.WriteString("a=np.zeros(sz, dtype=np.uint32)\n")
	b.WriteString("generators.gen_ufind_lin2_init(a, sz, n, nG)\n")
	for _, g := range gens {
		fmt.Fprintf(&b, "g=np.array(%s, dtype=np.uint32)\n", pyListU32(g))
		b.WriteString("generators.gen_ufind_lin2_add(a, g, len(g))\n")
	}
	b.WriteString("generators.gen_ufind_lin2_finalize(a)\n")
	b.WriteString("ln=generators.gen_ufind_lin2_len_orbit_v(a,v)\n")
	b.WriteString("r=np.zeros(ln,dtype=np.uint32); generators.gen_ufind_lin2_orbit_v(a,v,r,ln)\n")
	b.WriteString("print(json.dumps(r.tolist()))\n")
	out := ufindOracle(t, b.String())
	var arr []uint32
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unmarshal orbit_v %q: %v", out, err)
	}
	return arr
}

// TestUFindLin2MapVParity checks the Schreier-vector
// word produced for each vector against the oracle's
// transform-back behaviour and the oracle word itself.
func TestUFindLin2MapVParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := buildGoArray(t, tc.n, tc.nG, tc.gens)
			N := uint32(1) << tc.n
			oracleWords := oracleMapV(t, tc.n, tc.nG, N, tc.gens)
			for v := uint32(0); v < N; v++ {
				b := make([]uint8, N)
				ln := UFindLin2MapV(a, v, b, N)
				got := b[:ln]
				want := oracleWords[v]
				if len(got) != len(want) {
					t.Fatalf("map_v(%d) length: got %v want %v", v, got, want)
				}
				for i := range want {
					if uint32(got[i]) != want[i] {
						t.Fatalf("map_v(%d)[%d]: got %d want %d", v, i, got[i], want[i])
					}
				}
				// The word must transform v to its rep.
				rep := uint32(UFindLin2RepV(a, v))
				img := uint32(UFindLin2TransformV(a, v, got, uint32(len(got))))
				if img != rep {
					t.Errorf("map_v(%d) word maps to %d, not rep %d", v, img, rep)
				}
			}
		})
	}
}

func oracleMapV(t *testing.T, n, nG, N uint32, gens [][]uint32) [][]uint32 {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "n=%d\nnG=%d\nN=%d\n", n, nG, N)
	b.WriteString("sz=generators.gen_ufind_lin2_size(n, nG)\n")
	b.WriteString("a=np.zeros(sz, dtype=np.uint32)\n")
	b.WriteString("generators.gen_ufind_lin2_init(a, sz, n, nG)\n")
	for _, g := range gens {
		fmt.Fprintf(&b, "g=np.array(%s, dtype=np.uint32)\n", pyListU32(g))
		b.WriteString("generators.gen_ufind_lin2_add(a, g, len(g))\n")
	}
	b.WriteString("generators.gen_ufind_lin2_finalize(a)\n")
	b.WriteString("res=[]\n")
	b.WriteString("for v in range(N):\n")
	b.WriteString("  bb=np.zeros(N,dtype=np.uint8); ln=generators.gen_ufind_lin2_map_v(a,v,bb,N)\n")
	b.WriteString("  res.append(bb[:ln].tolist())\n")
	b.WriteString("print(json.dumps(res))\n")
	out := ufindOracle(t, b.String())
	var arr [][]uint32
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unmarshal map_v %q: %v", out, err)
	}
	return arr
}

// emitBuild writes the Python prelude that builds and
// finalizes the orbit array a for n, nG, gens.
func emitBuild(b *strings.Builder, n, nG uint32, gens [][]uint32) {
	fmt.Fprintf(b, "n=%d\nnG=%d\n", n, nG)
	b.WriteString("sz=generators.gen_ufind_lin2_size(n, nG)\n")
	b.WriteString("a=np.zeros(sz, dtype=np.uint32)\n")
	b.WriteString("generators.gen_ufind_lin2_init(a, sz, n, nG)\n")
	for _, g := range gens {
		fmt.Fprintf(b, "g=np.array(%s, dtype=np.uint32)\n", pyListU32(g))
		b.WriteString("generators.gen_ufind_lin2_add(a, g, len(g))\n")
	}
	b.WriteString("generators.gen_ufind_lin2_finalize(a)\n")
}

// TestUFindLin2CompressParity checks byte-exact parity
// of the compressed orbit array and that orbit queries
// over the compressed array match the oracle.
func TestUFindLin2CompressParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Select the orbit of vector 1 (and vector 3
			// where present) to compress.
			sel := []uint32{1}
			if (uint32(1) << tc.n) > 3 {
				sel = append(sel, 3)
			}

			// Oracle: compressed array + queries.
			var b strings.Builder
			emitBuild(&b, tc.n, tc.nG, tc.gens)
			fmt.Fprintf(&b, "o=np.array(%s,dtype=np.uint32)\n", pyListU32(sel))
			b.WriteString("lc=generators.gen_ufind_lin2_compressed_size(a,o,len(o))\n")
			b.WriteString("c=np.zeros(lc,dtype=np.uint32)\n")
			b.WriteString("generators.gen_ufind_lin2_compress(a,o,len(o),c,lc)\n")
			b.WriteString("nor=int(generators.gen_ufind_lin2_n_orbits(c))\n")
			b.WriteString("rr=np.zeros(nor,dtype=np.uint32); generators.gen_ufind_lin2_representatives(c,rr,nor)\n")
			b.WriteString("ll=np.zeros(nor,dtype=np.uint32); generators.gen_ufind_lin2_orbit_lengths(c,ll,nor)\n")
			b.WriteString("repv=[int(generators.gen_ufind_lin2_rep_v(c,int(v))) for v in o]\n")
			b.WriteString("out={'lc':int(lc),'c':c.tolist(),'nor':nor,'reps':rr.tolist(),'lens':ll.tolist(),'repv':repv}\n")
			b.WriteString("print(json.dumps(out))\n")
			raw := ufindOracle(t, b.String())
			var want struct {
				LC   int      `json:"lc"`
				C    []uint32 `json:"c"`
				Nor  int      `json:"nor"`
				Reps []uint32 `json:"reps"`
				Lens []uint32 `json:"lens"`
				RepV []int32  `json:"repv"`
			}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("unmarshal compress oracle %q: %v", raw, err)
			}

			a := buildGoArray(t, tc.n, tc.nG, tc.gens)
			lc := UFindLin2CompressedSize(a, sel, uint32(len(sel)))
			if int(lc) != want.LC {
				t.Fatalf("compressed_size: got %d want %d", lc, want.LC)
			}
			c := make([]uint32, lc)
			rc := UFindLin2Compress(a, sel, uint32(len(sel)), c, uint32(lc))
			if int(rc) != want.LC {
				t.Fatalf("compress return: got %d want %d", rc, want.LC)
			}
			if !equalU32(c, want.C) {
				t.Fatalf("compressed array:\n got %v\nwant %v", c, want.C)
			}

			if gn := UFindLin2NOrbits(c); int(gn) != want.Nor {
				t.Errorf("compressed n_orbits: got %d want %d", gn, want.Nor)
			}
			reps := make([]uint32, want.Nor)
			UFindLin2Representatives(c, reps, uint32(want.Nor))
			if !equalU32(reps, want.Reps) {
				t.Errorf("compressed reps: got %v want %v", reps, want.Reps)
			}
			lens := make([]uint32, want.Nor)
			UFindLin2OrbitLengths(c, lens, uint32(want.Nor))
			if !equalU32(lens, want.Lens) {
				t.Errorf("compressed lens: got %v want %v", lens, want.Lens)
			}
			for i, v := range sel {
				if got := UFindLin2RepV(c, v); got != want.RepV[i] {
					t.Errorf("compressed rep_v(%d): got %d want %d", v, got, want.RepV[i])
				}
			}
		})
	}
}

// TestUFindLin2OrbitsParity checks the union-find-style
// partition output (t, x) against the oracle.
func TestUFindLin2OrbitsParity(t *testing.T) {
	for _, tc := range testGroups {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			emitBuild(&b, tc.n, tc.nG, tc.gens)
			b.WriteString("N=1<<n\n")
			b.WriteString("nor=int(generators.gen_ufind_lin2_n_orbits(a))\n")
			b.WriteString("tt=np.zeros(N,dtype=np.uint32)\n")
			b.WriteString("xx=np.zeros(nor+1,dtype=np.uint32)\n")
			b.WriteString("generators.gen_ufind_lin2_orbits(a,tt,N,xx,nor+1)\n")
			b.WriteString("print(json.dumps({'t':tt.tolist(),'x':xx.tolist()}))\n")
			raw := ufindOracle(t, b.String())
			var want struct {
				T []uint32 `json:"t"`
				X []uint32 `json:"x"`
			}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("unmarshal orbits oracle %q: %v", raw, err)
			}

			a := buildGoArray(t, tc.n, tc.nG, tc.gens)
			N := uint32(1) << tc.n
			nor := uint32(UFindLin2NOrbits(a))
			tt := make([]uint32, N)
			xx := make([]uint32, nor+1)
			UFindLin2Orbits(a, tt, N, xx, nor+1)
			if !equalU32(tt, want.T) {
				t.Errorf("orbits t: got %v want %v", tt, want.T)
			}
			if !equalU32(xx, want.X) {
				t.Errorf("orbits x: got %v want %v", xx, want.X)
			}
		})
	}
}

// TestUFindLin2AffineParity exercises the affine
// generator path (l_g > n), comparing the finalized
// array byte-exactly against the oracle.
func TestUFindLin2AffineParity(t *testing.T) {
	// n=3 with an affine generator: identity matrix plus
	// translation b = 0b001.
	n := uint32(3)
	nG := uint32(4)
	gens := [][]uint32{{0b001, 0b010, 0b100, 0b001}} // rows + affine offset in g[n]

	var b strings.Builder
	emitBuild(&b, n, nG, gens)
	b.WriteString("print(json.dumps(a.tolist()))\n")
	raw := ufindOracle(t, b.String())
	var want []uint32
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal affine oracle %q: %v", raw, err)
	}

	got := buildGoArray(t, n, nG, gens)
	if !equalU32(got, want) {
		t.Fatalf("affine array:\n got %v\nwant %v", got, want)
	}
}

func equalU32(a, b []uint32) bool {
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

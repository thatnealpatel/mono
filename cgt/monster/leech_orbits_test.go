package monster

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
	"patel.codes/cgt/leech"
)

// leechOrbitsOracle runs the leech2_orbits_raw driver inline
// in Python (the upstream function itself cannot run; see the
// note on Leech2OrbitsRaw) and returns the requested JSON
// expression. The local name `r` is bound to the tuple
// (n_sets, indices, data).
func leechOrbitsOracle(t *testing.T, words []string, expr string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("import json, numpy as np\nfrom mmgroup import MM0\nfrom mmgroup import generators as G\n")
	b.WriteString("glist=[")
	for i, w := range words {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("MM0(" + w + ")")
	}
	b.WriteString("]\n")
	b.WriteString(`n=24
sz=G.gen_ufind_lin2_size(n,len(glist))
a=np.zeros(sz,dtype=np.uint32)
G.gen_ufind_lin2_init(a,sz,n,len(glist))
for g in glist:
    md=g.mmdata
    ag=np.zeros(24,dtype=np.uint32)
    assert G.gen_leech2_op_word_matrix24(md,len(md),0,ag)>=0
    assert G.gen_ufind_lin2_add(a,ag,len(ag))>=0
G.gen_ufind_lin2_finalize(a)
n_sets=int(G.gen_ufind_lin2_n_orbits(a))
ldata=1<<int(G.gen_ufind_lin2_dim(a))
data=np.zeros(ldata,dtype=np.uint32)
indices=np.zeros(n_sets+1,dtype=np.uint32)
G.gen_ufind_lin2_orbits(a,data,ldata,indices,n_sets+1)
r=(n_sets, indices, data)
`)
	fmt.Fprintf(&b, "print(json.dumps(%s))\n", expr)
	out, err := oraclepkg.Cmd(b.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("leech orbits oracle failed: %v\n%s", err, out)
	}
	return []byte(strings.TrimSpace(string(out)))
}

// TestLeech2OpWordMatrix24 checks the 24x24 action matrix of
// a monster word against the oracle.
func TestLeech2OpWordMatrix24(t *testing.T) {
	words := []string{"'l', 1", "'p', 12345", "'y', 0x123"}
	for _, w := range words {
		got := leech.Leech2OpWordMatrix24(mustMM(t, mmWordFromPy(w)).Mmdata(), false)
		out := leechMatrix24Oracle(t, w)
		if !equalU32Int64(got, out) {
			t.Errorf("Leech2OpWordMatrix24(%s)=%v want %v", w, got, out)
		}
	}
}

func leechMatrix24Oracle(t *testing.T, word string) []int64 {
	t.Helper()
	script := "import json, numpy as np\nfrom mmgroup import MM0\nfrom mmgroup import generators as G\n" +
		"g=MM0(" + word + ")\nmd=g.mmdata\nag=np.zeros(24,dtype=np.uint32)\n" +
		"assert G.gen_leech2_op_word_matrix24(md,len(md),0,ag)>=0\n" +
		"print(json.dumps([int(x) for x in ag]))\n"
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("matrix24 oracle failed: %v\n%s", err, out)
	}
	var v []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("unmarshal matrix24: %v", err)
	}
	return v
}

// mmWordFromPy converts a Python-style MM0 argument like
// "'l', 1" into the cgt NewMM string form "l_1".
func mmWordFromPy(pyArg string) string {
	// Parse "'tag', value" where value may be hex (0x...) or
	// decimal.
	parts := strings.SplitN(pyArg, ",", 2)
	tag := strings.Trim(strings.TrimSpace(parts[0]), "'")
	val := strings.TrimSpace(parts[1])
	return tag + "_" + pyHexToCgt(val)
}

// pyHexToCgt normalises a Python integer literal to the cgt
// MM-word value form: hex stays hex (0x -> trailing h), decimal
// stays decimal.
func pyHexToCgt(s string) string {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return s[2:] + "h"
	}
	return s
}

// TestLeech2OrbitsRaw compares the full 2^24 Leech-mod-2
// orbit partition against the inline driver oracle. The
// finalize over 2^24 vectors is heavy, so this is skipped
// under -short.
func TestLeech2OrbitsRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 2^24 Leech orbit computation in -short mode")
	}
	words := []string{"'l', 1", "'p', 12345"}
	gList := []leech.Leech2OrbitGen{
		mustMM(t, "l_1"),
		mustMM(t, "p_12345"),
	}
	res := leech.Leech2OrbitsRaw(gList, false)

	wantNSets := int(int64FromJSON(t, leechOrbitsOracle(t, words, "int(r[0])")))
	if res.NSets != wantNSets {
		t.Fatalf("NSets=%d want %d", res.NSets, wantNSets)
	}
	wantIdxHead := orbitInts64(t, leechOrbitsOracle(t, words, "[int(x) for x in r[1][:8]]"))
	if !equalU32Int64(res.Indices[:8], wantIdxHead) {
		t.Fatalf("Indices[:8]=%v want %v", res.Indices[:8], wantIdxHead)
	}
	// Indices must cover all 2^24 vectors.
	if got := res.Indices[len(res.Indices)-1]; got != 1<<24 {
		t.Fatalf("Indices[-1]=%d want %d", got, 1<<24)
	}
	// Compare each orbit's first element (orbit reps) — these
	// pin down the partition without shipping 2^24 ints over
	// the wire.
	gotReps := make([]uint32, res.NSets)
	for i := 0; i < res.NSets; i++ {
		gotReps[i] = res.Data[res.Indices[i]]
	}
	wantReps := orbitInts64(t, leechOrbitsOracle(t, words,
		"[int(r[2][r[1][i]]) for i in range(int(r[0]))]"))
	if !equalU32Int64(gotReps, wantReps) {
		t.Fatalf("orbit reps=%v want %v", gotReps, wantReps)
	}
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

func int64FromJSON(t *testing.T, raw []byte) int64 {
	t.Helper()
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal int %q: %v", raw, err)
	}
	return v
}

func orbitInts64(t *testing.T, raw []byte) []int64 {
	t.Helper()
	var v []int64
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal ints %q: %v", raw, err)
	}
	return v
}

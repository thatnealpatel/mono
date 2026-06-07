package cgt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func oracleMisc(t *testing.T, setup, expr string) string {
	t.Helper()
	script := fmt.Sprintf("import json,mmgroup\n%s\nprint(json.dumps(%s))", setup, expr)
	out, err := pyCmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func oracleMiscInt(t *testing.T, setup, expr string) int64 {
	t.Helper()
	var v int64
	s := oracleMisc(t, setup, expr)
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleMiscInt(%q): %v", expr, err)
	}
	return v
}

func oracleMiscInts(t *testing.T, setup, expr string) []int64 {
	t.Helper()
	var v []int64
	s := oracleMisc(t, setup, expr)
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleMiscInts(%q): %v", expr, err)
	}
	return v
}

func oracleMiscBool(t *testing.T, setup, expr string) bool {
	t.Helper()
	return oracleMisc(t, setup, expr) == "true"
}

func miscList(v []int) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func miscEq(a []int, b []int64) bool {
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

const miscBF = "from mmgroup.bitfunctions import bitparity, bitweight"

func TestBitParity(t *testing.T) {
	t.Parallel()
	for _, x := range []uint64{0, 0x7, 0x3, 0xfedcba, 0xffffffffff} {
		want := int(oracleMiscInt(t, miscBF, fmt.Sprintf("bitparity(%d)", x)))
		if got := BitParity(x); got != want {
			t.Errorf("BitParity(%#x)=%d want %d", x, got, want)
		}
	}
}

func TestBitWeight(t *testing.T) {
	t.Parallel()
	for _, x := range []uint64{0, 0xff, 0x1234, 0xfedcba, 0xffffffffff} {
		want := int(oracleMiscInt(t, miscBF, fmt.Sprintf("bitweight(%d)", x)))
		if got := BitWeight(x); got != want {
			t.Errorf("BitWeight(%#x)=%d want %d", x, got, want)
		}
	}
}

func TestHadamardSign(t *testing.T) {
	t.Parallel()
	cases := [][2]int{{0, 0}, {1, 1}, {3, 5}, {7, 7}, {0x2a, 0x1c}}
	for _, c := range cases {
		want := int(oracleMiscInt(t, miscBF, fmt.Sprintf("(-1)**bitparity(%d & %d)", c[0], c[1])))
		if got := HadamardSign(c[0], c[1]); got != want {
			t.Errorf("HadamardSign(%d,%d)=%d want %d", c[0], c[1], got, want)
		}
	}
}

func TestParityHadamardSign(t *testing.T) {
	t.Parallel()
	cases := [][2]int{{0, 0}, {1, 2}, {3, 3}, {6, 5}, {0x2a, 0x1c}}
	for _, c := range cases {
		expr := fmt.Sprintf("(-1)**(bitparity(%d & %d) ^ (bitparity(%d) & bitparity(%d)))", c[0], c[1], c[0], c[1])
		want := int(oracleMiscInt(t, miscBF, expr))
		if got := ParityHadamardSign(c[0], c[1]); got != want {
			t.Errorf("ParityHadamardSign(%d,%d)=%d want %d", c[0], c[1], got, want)
		}
	}
}

func TestXchParity(t *testing.T) {
	t.Parallel()
	vecs := [][]int{
		{0, 1, 2, 3},
		{5, 6, 7, 8, 9, 10, 11, 12},
		{0, 1, 0, 0, 0, 0, 0, 1},
	}
	for _, v := range vecs {
		l := miscList(v)
		expr := fmt.Sprintf("[%s[len(%s)-i-1 if bitparity(i) else i] for i in range(len(%s))]", l, l, l)
		want := oracleMiscInts(t, miscBF, expr)
		if got := XchParity(v); !miscEq(got, want) {
			t.Errorf("XchParity(%v)=%v want %v", v, got, want)
		}
	}
}

func TestHadamardTransform(t *testing.T) {
	t.Parallel()
	setup := miscBF + "\n" +
		"def p2(e,p):\n" +
		" return pow((p+1)>>1,-e,p) if e<0 else pow(2,e,p)\n" +
		"def had(p,v):\n" +
		" k=len(v).bit_length()-1\n" +
		" n=1<<k\n" +
		" q=p2(-k>>1,p)\n" +
		" h=[[(-1)**bitparity(i&j) for j in range(n)] for i in range(n)]\n" +
		" return [ (sum(v[i]*h[i][j] for i in range(n))*q)%p for j in range(n)]"
	cases := []struct {
		p int
		v []int
	}{
		{3, []int{1, 0, 0, 0}},
		{3, []int{0, 1, 0, 0}},
		{7, []int{1, 2, 0, 1}},
		{7, []int{1, 2, 3, 4, 5, 6, 0, 1, 2, 3, 4, 5, 6, 0, 1, 2}},
	}
	for _, c := range cases {
		want := oracleMiscInts(t, setup, fmt.Sprintf("had(%d,%s)", c.p, miscList(c.v)))
		if got := HadamardTransform(c.p, c.v); !miscEq(got, want) {
			t.Errorf("HadamardTransform(%d,%v)=%v want %v", c.p, c.v, got, want)
		}
	}
}

const miscUF = "import numpy as np\n" +
	"from mmgroup.generators import gen_ufind_init, gen_ufind_union, gen_ufind_find, gen_ufind_find_all_min\n" +
	"def uf(n,unions):\n" +
	" t=np.zeros(n,dtype=np.uint32); gen_ufind_init(t,n)\n" +
	" for i,j in unions: gen_ufind_union(t,n,i,j)\n" +
	" return t"

func miscUnionFind(t *testing.T, n int, unions [][2]uint32, queries []uint32) {
	t.Helper()
	pairs := make([]string, len(unions))
	for i, u := range unions {
		pairs[i] = fmt.Sprintf("(%d,%d)", u[0], u[1])
	}
	plist := "[" + strings.Join(pairs, ",") + "]"
	tbl := make([]uint32, n)
	UFindInit(tbl)
	for _, u := range unions {
		UFindUnion(tbl, u[0], u[1])
	}
	for _, q := range queries {
		want := uint32(oracleMiscInt(t, miscUF, fmt.Sprintf("int(gen_ufind_find(uf(%d,%s),%d,%d))", n, plist, n, q)))
		if got := UFindFind(tbl, q); got != want {
			t.Errorf("UFindFind(%d)=%d want %d", q, got, want)
		}
	}
	wantN := int(oracleMiscInt(t, miscUF, fmt.Sprintf("int(gen_ufind_find_all_min(uf(%d,%s),%d))", n, plist, n)))
	if got := UFindFindAllMin(tbl); got != wantN {
		t.Errorf("UFindFindAllMin()=%d want %d", got, wantN)
	}
}

func TestUFindFind(t *testing.T) {
	t.Parallel()
	miscUnionFind(t, 8, [][2]uint32{{0, 1}, {2, 3}, {1, 3}}, []uint32{0, 3, 4})
	miscUnionFind(t, 16, [][2]uint32{{0, 4}, {4, 8}, {8, 12}, {1, 5}}, []uint32{12, 5, 7})
	miscUnionFind(t, 8, [][2]uint32{{0, 7}, {1, 6}, {2, 5}, {3, 4}}, []uint32{7, 6, 0})
}

const miscInc = "from mmgroup.bimm import P3_node, P3_incidence, P3_incidences, P3_is_collinear"

func TestP3Incidence(t *testing.T) {
	t.Parallel()
	cases := [][2]int{{0, 1}, {0, 3}, {1, 3}, {2, 5}}
	for _, c := range cases {
		want := int(oracleMiscInt(t, miscInc, fmt.Sprintf("P3_incidence(%d,%d).ord", c[0], c[1])))
		if got := P3Incidence(c[0], c[1]).Ord(); got != want {
			t.Errorf("P3Incidence(%d,%d).Ord()=%d want %d", c[0], c[1], got, want)
		}
	}
}

func TestP3NodeName(t *testing.T) {
	t.Parallel()
	for _, o := range []int{0, 5, 12, 13, 25} {
		want := strings.Trim(oracleMisc(t, miscInc, fmt.Sprintf("P3_node(%d).name()", o)), "\"")
		if got := NewP3Node(o).Name(); got != want {
			t.Errorf("P3Node(%d).Name()=%q want %q", o, got, want)
		}
	}
}

func TestP3IsCollinear(t *testing.T) {
	t.Parallel()
	cases := [][]int{{0, 1, 2}, {0, 1, 3}, {2, 5, 6}, {1, 3, 9}}
	for _, c := range cases {
		want := oracleMiscBool(t, miscInc, fmt.Sprintf("P3_is_collinear(%s)", miscList(c)))
		if got := P3IsCollinear(c); got != want {
			t.Errorf("P3IsCollinear(%v)=%v want %v", c, got, want)
		}
	}
}

const miscBM = "from mmgroup.bimm import P3_BiMM"

func TestBiMMCoxeterExp(t *testing.T) {
	t.Parallel()
	ref := "def cox(x,y):\n" +
		" mi,ma=min(x,y),max(x,y)\n" +
		" if mi<13 and ma>=13 and (mi+ma)%13 in (0,1,3,9): return 3\n" +
		" return 2 if mi!=ma else 1"
	cases := [][2]int{{0, 0}, {0, 1}, {0, 13}, {0, 14}, {1, 25}}
	for _, c := range cases {
		want := int(oracleMiscInt(t, ref, fmt.Sprintf("cox(%d,%d)", c[0], c[1])))
		if got := BiMMCoxeterExp(c[0], c[1]); got != want {
			t.Errorf("BiMMCoxeterExp(%d,%d)=%d want %d", c[0], c[1], got, want)
		}
	}
}

func TestBiMMOrder(t *testing.T) {
	t.Parallel()
	words := [][]int{{}, {0}, {0, 13}}
	for _, w := range words {
		got := P3BiMM(w).Order()
		want := int(oracleMiscInt(t, miscBM, fmt.Sprintf("P3_BiMM(%s).order()", miscList(w))))
		if got != want {
			t.Errorf("P3BiMM(%v).Order()=%d want %d", w, got, want)
		}
	}
}

func TestBiMMSpiderOrder(t *testing.T) {
	t.Parallel()
	got := P3BiMM([]int{0, 14, 15, 0, 16, 17, 0, 18, 19}).Order()
	want := int(oracleMiscInt(t, miscBM, "P3_BiMM([0,14,15,0,16,17,0,18,19]).order()"))
	if got != want {
		t.Errorf("spider order=%d want %d", got, want)
	}
}

func TestConjugateInvolutionType(t *testing.T) {
	t.Parallel()
	setup := "from mmgroup import MM0\n" +
		"def conj(zt,zv,ct,cv):\n" +
		" z=MM0(zt,zv); c=MM0(ct,cv); return c**(-1)*z*c\n" +
		"def itype(zt,zv,ct,cv):\n" +
		" g=conj(zt,zv,ct,cv); it,h=g.conjugate_involution(); return [it, int(g**h==MM0(zt,zv))]"
	cases := []struct {
		zt string
		zv int
		ct string
		cv int
	}{
		{"x", 0x1000, "d", 0x456},
		{"x", 0x1000, "x", 0x321},
		{"x", 0x1000, "y", 0x789},
	}
	for _, c := range cases {
		z := MMGen(c.zt, c.zv)
		cc := MMGen(c.ct, c.cv)
		g := cc.Inv().Mul(z).Mul(cc)
		got, h := ConjugateInvolutionType(g)
		res := oracleMiscInts(t, setup, fmt.Sprintf("itype(%q,%d,%q,%d)", c.zt, c.zv, c.ct, c.cv))
		if got != int(res[0]) {
			t.Errorf("ConjugateInvolutionType type=%d want %d", got, res[0])
		}
		if res[1] != 1 {
			t.Fatalf("oracle: g**h != z for %v", c)
		}
		if !h.Inv().Mul(g).Mul(h).Equal(z) {
			t.Errorf("h^-1 g h != z for %v", c)
		}
	}
}

func TestAutP3Type(t *testing.T) {
	t.Parallel()
	g := NewAutP3Rand()
	if g.Order() <= 0 {
		t.Fatalf("AutP3Rand.Order() = %d, want > 0", g.Order())
	}
	h := NewAutP3Rand()
	gh := g.Mul(h)
	if gh.Order() <= 0 {
		t.Fatalf("AutP3.Mul.Order() = %d, want > 0", gh.Order())
	}
	p := gh.Perm()
	if len(p) == 0 {
		t.Fatalf("AutP3.Perm() returned empty")
	}
}

func TestBiMMGroupOps(t *testing.T) {
	t.Parallel()
	id := BiMMIdentity()
	if id.Order() != 1 {
		t.Fatalf("BiMMIdentity.Order() = %d, want 1", id.Order())
	}
	b := P3BiMM([]int{0})
	if !b.Mul(b.Inv()).Equal(id) {
		t.Errorf("BiMM b*b^-1 != identity")
	}
	b2 := b.Pow(2)
	if !b2.Equal(b.Mul(b)) {
		t.Errorf("BiMM.Pow(2) != Mul(self)")
	}
	o1, o2, oa := b.Orders()
	want1 := int(oracleMiscInt(t, miscBM, fmt.Sprintf("P3_BiMM([0]).orders()[0]")))
	want2 := int(oracleMiscInt(t, miscBM, fmt.Sprintf("P3_BiMM([0]).orders()[1]")))
	wanta := int(oracleMiscInt(t, miscBM, fmt.Sprintf("P3_BiMM([0]).orders()[2]")))
	if o1 != want1 || o2 != want2 || oa != wanta {
		t.Errorf("BiMM.Orders() = (%d,%d,%d) want (%d,%d,%d)", o1, o2, oa, want1, want2, wanta)
	}
}

func TestP3Incidences(t *testing.T) {
	t.Parallel()
	nodes := P3Incidences(0, 1, 3)
	want := oracleMiscInts(t, miscInc, "[n.ord for n in P3_incidences(0, 1, 3)]")
	if len(nodes) != len(want) {
		t.Fatalf("P3Incidences(0,1,3) len=%d want %d", len(nodes), len(want))
	}
	for i, n := range nodes {
		if int64(n.Ord()) != want[i] {
			t.Errorf("P3Incidences(0,1,3)[%d].Ord()=%d want %d", i, n.Ord(), want[i])
		}
	}
}

func TestUFindPartition(t *testing.T) {
	t.Parallel()
	tbl := make([]uint32, 8)
	UFindInit(tbl)
	UFindUnion(tbl, 0, 1)
	UFindUnion(tbl, 2, 3)
	UFindUnion(tbl, 1, 3)
	UFindUnion(tbl, 5, 6)
	UFindFindAllMin(tbl)
	data := make([]uint32, 8)
	ind := make([]uint32, 9)
	nsets := UFindPartition(tbl, data, ind)
	if nsets <= 0 {
		t.Fatalf("UFindPartition returned %d sets", nsets)
	}
}

const miscMakeMap = "import numpy as np\n" +
	"from mmgroup.generators import gen_ufind_init, gen_ufind_union, gen_ufind_find_all_min, gen_ufind_make_map\n" +
	"def mkmap(n,unions):\n" +
	" t=np.zeros(n,dtype=np.uint32); gen_ufind_init(t,n)\n" +
	" for i,j in unions: gen_ufind_union(t,n,i,j)\n" +
	" gen_ufind_find_all_min(t,n)\n" +
	" m=np.zeros(n,dtype=np.uint32); gen_ufind_make_map(t,n,m)\n" +
	" return [int(x) for x in m]"

func TestUFindMakeMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n      int
		unions [][2]uint32
	}{
		{8, [][2]uint32{{0, 1}, {2, 3}, {1, 3}}},
		{16, [][2]uint32{{0, 4}, {4, 8}, {8, 12}, {1, 5}}},
		{8, [][2]uint32{{0, 7}, {1, 6}, {2, 5}, {3, 4}}},
	}
	for _, c := range cases {
		tbl := make([]uint32, c.n)
		UFindInit(tbl)
		for _, u := range c.unions {
			UFindUnion(tbl, u[0], u[1])
		}
		UFindFindAllMin(tbl)
		got := UFindMakeMap(tbl)
		pairs := make([]string, len(c.unions))
		for i, u := range c.unions {
			pairs[i] = fmt.Sprintf("(%d,%d)", u[0], u[1])
		}
		plist := "[" + strings.Join(pairs, ",") + "]"
		want := oracleMiscInts(t, miscMakeMap, fmt.Sprintf("mkmap(%d,%s)", c.n, plist))
		if !miscEq32(got, want) {
			t.Errorf("UFindMakeMap(n=%d,%v)=%v want %v", c.n, c.unions, got, want)
		}
	}
}

func miscEq32(a []uint32, b []int64) bool {
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

const miscRemain = "from mmgroup.bimm import P3_node, P3_remaining_nodes"

func TestP3RemainingNodes(t *testing.T) {
	t.Parallel()
	cases := [][2]int{{0, 1}, {0, 3}, {1, 3}, {2, 5}, {13, 14}}
	for _, c := range cases {
		got := P3RemainingNodes(c[0], c[1])
		want := oracleMiscInts(t, miscRemain, fmt.Sprintf("[n.ord for n in P3_remaining_nodes(%d,%d)]", c[0], c[1]))
		if len(got) != len(want) {
			t.Fatalf("P3RemainingNodes(%d,%d) len=%d want %d", c[0], c[1], len(got), len(want))
		}
		for i, n := range got {
			if int64(n.Ord()) != want[i] {
				t.Errorf("P3RemainingNodes(%d,%d)[%d].Ord()=%d want %d", c[0], c[1], i, n.Ord(), want[i])
			}
		}
	}
}

func TestAutP3Inv(t *testing.T) {
	t.Parallel()
	id := NewAutP3(nil)
	for i := 0; i < 5; i++ {
		g := NewAutP3Rand()
		if !g.Mul(g.Inv()).Equal(id) {
			t.Errorf("AutP3 g*g^-1 != identity")
		}
		if !g.Inv().Mul(g).Equal(id) {
			t.Errorf("AutP3 g^-1*g != identity")
		}
		if !g.Inv().Inv().Equal(g) {
			t.Errorf("AutP3 (g^-1)^-1 != g")
		}
	}
}

func TestAutP3Pow(t *testing.T) {
	t.Parallel()
	id := NewAutP3(nil)
	for i := 0; i < 5; i++ {
		g := NewAutP3Rand()
		if !g.Pow(2).Equal(g.Mul(g)) {
			t.Errorf("AutP3.Pow(2) != Mul(self)")
		}
		if !g.Pow(3).Equal(g.Mul(g).Mul(g)) {
			t.Errorf("AutP3.Pow(3) != g*g*g")
		}
		if !g.Pow(0).Equal(id) {
			t.Errorf("AutP3.Pow(0) != identity")
		}
		if !g.Pow(-1).Equal(g.Inv()) {
			t.Errorf("AutP3.Pow(-1) != Inv()")
		}
	}
}

func miscPerm13(v []int) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestAutP3PointMap(t *testing.T) {
	t.Parallel()
	for i := 0; i < 5; i++ {
		g := NewAutP3Rand()
		pm := g.PointMap()
		if len(pm) != 13 {
			t.Fatalf("AutP3.PointMap() len=%d want 13", len(pm))
		}
		want := oracleMiscInts(t, miscInc, fmt.Sprintf("__import__('mmgroup.bimm',fromlist=['AutP3']).AutP3('p',%s).point_map()", miscPerm13(pm)))
		if !miscEq(pm, want) {
			t.Errorf("AutP3.PointMap()=%v not consistent with oracle %v", pm, want)
		}
	}
}

func TestAutP3LineMap(t *testing.T) {
	t.Parallel()
	for i := 0; i < 5; i++ {
		g := NewAutP3Rand()
		pm := g.PointMap()
		lm := g.LineMap()
		if len(lm) != 13 {
			t.Fatalf("AutP3.LineMap() len=%d want 13", len(lm))
		}
		want := oracleMiscInts(t, miscInc, fmt.Sprintf("__import__('mmgroup.bimm',fromlist=['AutP3']).AutP3('p',%s).line_map()", miscPerm13(pm)))
		if !miscEq(lm, want) {
			t.Errorf("AutP3.LineMap()=%v want %v", lm, want)
		}
	}
}

func TestBiMMDecompose(t *testing.T) {
	t.Parallel()
	words := [][]int{{0}, {0, 13}, {0, 14, 15}}
	for _, w := range words {
		b := P3BiMM(w)
		m1, m2, e := b.Decompose()
		setup := miscBM + "\ndef dec(word):\n d=P3_BiMM(word).decompose(); return [list(int(x) for x in d[0].mmdata), list(int(x) for x in d[1].mmdata), int(d[2])]"
		wantM1 := oracleMiscInts(t, setup, fmt.Sprintf("dec(%s)[0]", miscList(w)))
		wantM2 := oracleMiscInts(t, setup, fmt.Sprintf("dec(%s)[1]", miscList(w)))
		wantE := int(oracleMiscInt(t, setup, fmt.Sprintf("dec(%s)[2]", miscList(w))))
		if e != wantE {
			t.Errorf("P3BiMM(%v).Decompose() alpha=%d want %d", w, e, wantE)
		}
		if !miscEqU32(m1.Mmdata(), wantM1) {
			t.Errorf("P3BiMM(%v).Decompose() m1=%v want %v", w, m1.Mmdata(), wantM1)
		}
		if !miscEqU32(m2.Mmdata(), wantM2) {
			t.Errorf("P3BiMM(%v).Decompose() m2=%v want %v", w, m2.Mmdata(), wantM2)
		}
	}
}

func miscEqU32(a []uint32, b []int64) bool {
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

func TestP3NodeApply(t *testing.T) {
	t.Parallel()
	nodes := []int{0, 1, 5, 13, 14, 25}
	for i := 0; i < 4; i++ {
		g := NewAutP3Rand()
		pm := g.PointMap()
		for _, n := range nodes {
			got := NewP3Node(n).Apply(g)
			want := int(oracleMiscInt(t, miscInc, fmt.Sprintf("(P3_node(%d)*__import__('mmgroup.bimm',fromlist=['AutP3']).AutP3('p',%s)).ord", n, miscPerm13(pm))))
			if got.Ord() != want {
				t.Errorf("P3Node(%d).Apply(g).Ord()=%d want %d", n, got.Ord(), want)
			}
		}
	}
}

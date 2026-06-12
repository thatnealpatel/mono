package monster

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func oraclePair(t *testing.T, pyExpr string) (int64, string) {
	t.Helper()
	s := oracle(t, pyExpr)
	var v []json.RawMessage
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oraclePair(%q): %v", pyExpr, err)
	}
	var o int64
	if err := json.Unmarshal(v[0], &o); err != nil {
		t.Fatalf("oraclePair(%q): order: %v", pyExpr, err)
	}
	var h string
	if err := json.Unmarshal(v[1], &h); err != nil {
		h = ""
	}
	return o, h
}

func mmExpr(spec string) string {
	return fmt.Sprintf("mmgroup.MM(%q)", spec)
}

// stripMM normalizes a Python MM string for
// comparison against Go's MM.String(): it drops a
// surrounding JSON quote pair (when present) and
// the "M<...>" wrapper. "M<1>" becomes "1", which
// matches MM.String() for the neutral element.
func stripMM(s string) string {
	s = strings.Trim(s, "\"")
	s = strings.TrimPrefix(s, "M<")
	s = strings.TrimSuffix(s, ">")
	return s
}

func TestMonsterMul(t *testing.T) {
	t.Parallel()
	cases := [][2]string{
		{"M<x_1h>", "M<y_2h>"},
		{"M<t_1>", "M<l_1>"},
		{"M<d_2h*p_1>", "M<y_0c00h*t_2>"},
		{"M<x_1h*y_2h*t_1*l_2>", "M<d_5h*p_100*t_1>"},
		{"M<l_1*t_2*l_2*t_1>", "M<x_1abh*y_3h*d_4h>"},
	}
	for _, c := range cases {
		g := mustMM(t, c[0])
		h := mustMM(t, c[1])
		got := g.Mul(h).String()
		want := stripMM(oracle(t, fmt.Sprintf("str((%s*%s).reduce())", mmExpr(c[0]), mmExpr(c[1]))))
		if got != want {
			t.Errorf("Mul(%q,%q)=%q want %q", c[0], c[1], got, want)
		}
	}
}

func TestMonsterInv(t *testing.T) {
	t.Parallel()
	cases := []string{
		"M<x_1h>",
		"M<t_1>",
		"M<x_1h*y_2h*t_1*l_2>",
		"M<d_5h*p_100*l_1*t_2>",
		"M<l_1*t_2*l_2*t_1*x_3abh>",
	}
	for _, c := range cases {
		g := mustMM(t, c)
		if !g.Mul(g.Inv()).Equal(MMIdentity()) {
			t.Errorf("g*g^-1 != 1 for %q", c)
		}
		got := g.Inv().String()
		want := stripMM(oracle(t, fmt.Sprintf("str((%s**-1).reduce())", mmExpr(c))))
		if got != want {
			t.Errorf("Inv(%q)=%q want %q", c, got, want)
		}
	}
}

func TestMonsterOrder(t *testing.T) {
	t.Parallel()
	cases := []string{
		"M<1>",
		"M<t_1>",
		"M<d_2h*d_3h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, c := range cases {
		got := int64(mustMM(t, c).Order())
		want := oracleInt(t, fmt.Sprintf("%s.order()", mmExpr(c)))
		if got != want {
			t.Errorf("Order(%q)=%d want %d", c, got, want)
		}
	}
}

func TestMonsterHalfOrder(t *testing.T) {
	t.Parallel()
	cases := []string{
		"M<t_1>",
		"M<d_2h*d_3h>",
		"M<x_1000h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
	}
	for _, c := range cases {
		o, h := mustMM(t, c).HalfOrder()
		wantO, wantH := oraclePair(t, fmt.Sprintf(
			"(lambda r: [r[0], None if r[1] is None else str(r[1].reduce())])(%s.half_order())",
			mmExpr(c)))
		wantH = stripMM(wantH)
		if int64(o) != wantO {
			t.Errorf("HalfOrder(%q) order=%d want %d", c, o, wantO)
		}
		gotH := ""
		if h != nil {
			gotH = h.String()
		}
		if gotH != wantH {
			t.Errorf("HalfOrder(%q) half=%q want %q", c, gotH, wantH)
		}
	}
}

func TestMonsterEqual(t *testing.T) {
	t.Parallel()
	cases := [][2]string{
		{"M<1>", "M<1>"},
		{"M<d_2h*d_2h>", "M<1>"},
		{"M<x_1h>", "M<y_2h>"},
		{"M<t_1*t_1*t_1>", "M<1>"},
		{"M<x_1h*y_2h>", "M<y_2h*x_1h>"},
	}
	for _, c := range cases {
		got := mustMM(t, c[0]).Equal(mustMM(t, c[1]))
		want := oracleBool(t, fmt.Sprintf("(%s == %s)", mmExpr(c[0]), mmExpr(c[1])))
		if got != want {
			t.Errorf("Equal(%q,%q)=%v want %v", c[0], c[1], got, want)
		}
	}
}

func TestMonsterReduce(t *testing.T) {
	t.Parallel()
	cases := []string{
		"M<t_1*t_1*t_1>",
		"M<d_2h*d_2h>",
		"M<x_1h*y_2h*t_1*l_2*p_100*l_1*t_2>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h*y_5h*p_200>",
	}
	for _, c := range cases {
		got := mustMM(t, c).Reduce().String()
		want := stripMM(oracle(t, fmt.Sprintf("str(%s.reduce())", mmExpr(c))))
		if got != want {
			t.Errorf("Reduce(%q)=%q want %q", c, got, want)
		}
	}
}

func TestMonsterAsIntRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag string
		i   int
	}{
		{"y", 0xc00},
		{"x", 1},
		{"d", 5},
		{"p", 100},
		{"l", 1},
	}
	for _, c := range cases {
		// No "h" suffix: the index is decimal here, so
		// it must match c.i parsed as a Go int. With "h"
		// the parser would read it as hex (e.g. p_100h
		// = 0x100 = 256, not 100).
		spec := fmt.Sprintf("M<%s_%d>", c.tag, c.i)
		n := MMGen(c.tag, c.i).AsInt()
		want := oracleUint(t, fmt.Sprintf("%s.as_int()", mmExpr(spec)))
		if n != want {
			t.Errorf("AsInt(%q)=%d want %d", spec, n, want)
		}
		got, err := MMFromInt(n)
		if err != nil {
			t.Fatalf("MMFromInt(AsInt(%q)): %v", spec, err)
		}
		if !got.Equal(MMGen(c.tag, c.i)) {
			t.Errorf("MMFromInt(AsInt(%q)) != original", spec)
		}
	}
}

func TestMonsterInGx0(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"M<d_2h>":          true,
		"M<x_0a71h>":       true,
		"M<y_0cd1h>":       true,
		"M<t_1>":           false,
		"M<x_1h*y_2h*t_1>": false,
	}
	for spec, want := range cases {
		got := mustMM(t, spec).InGx0()
		oWant := oracleBool(t, fmt.Sprintf("%s.in_G_x0()", mmExpr(spec)))
		if oWant != want {
			t.Fatalf("oracle disagrees with case table for %q: %v vs %v", spec, oWant, want)
		}
		if got != want {
			t.Errorf("InGx0(%q)=%v want %v", spec, got, want)
		}
	}
}

func TestMonsterEvalA(t *testing.T) {
	t.Parallel()
	cases := []struct {
		i, j   int
		c0, c1 int
	}{
		{2, 2, 2, 3},
		{3, 5, 0, 1},
		{7, 7, 4, 7},
		{1, 4, 10, 13},
		{0, 9, 1, 22},
	}
	for _, c := range cases {
		v2 := oracleUint(t, fmt.Sprintf("mmgroup.Cocode([%d,%d]).ord", c.c0, c.c1))
		got := int64(BasisVector(15, TagA, c.i, c.j).EvalA(v2, 0))
		want := oracleInt(t, fmt.Sprintf(
			"mmgroup.MMV(15)('A',%d,%d).eval_A(mmgroup.Cocode([%d,%d]).ord)",
			c.i, c.j, c.c0, c.c1))
		if got != want {
			t.Errorf("EvalA(A_%d_%d, [%d,%d])=%d want %d", c.i, c.j, c.c0, c.c1, got, want)
		}
	}
}

func TestMonsterAxisType(t *testing.T) {
	t.Parallel()
	specs := []string{
		"M<x_1h*y_2h>",
		"M<t_1*l_1*t_2>",
		"M<d_5h*p_100*l_1*t_2>",
		"M<l_2*t_1*x_1abh*y_3h>",
		// Short MM words keep v^+ in its own G_x0 orbit ("2A"). The
		// following are the canonical per-orbit representatives from
		// mmgroup's tests/axes/sample_axes.py (g_strings), which land
		// in the 2B, 4A, 4C, and 6C orbits respectively.
		"M<y_29bh*x_1e0ch*d_39fh*p_108582478*l_2*p_960*l_2*p_10667712*l_2*p_5197440*t_1>",
		"M<y_72eh*x_8b5h*d_0ff3h*p_203638538*l_2*p_1394880*l_1*p_10665792*l_1*p_6125760*t_1>",
		"M<y_175h*x_0bd6h*d_5dfh*p_77235657*l_1*p_1499520*l_2*p_10773794*t_1*l_1*p_1481280*t_2*l_1*p_130560*l_1*t_1>",
		"M<y_2b8h*x_429h*d_553h*p_237253688*l_2*p_1900800*l_2*p_85835153*l_2*p_21796800*t_2*l_1*p_13326720*l_1*p_11552640*l_1*t_1>",
	}
	for _, spec := range specs {
		got := mustAxisFor(t, mustMM(t, spec)).Type()
		want := strings.Trim(oracle(t, fmt.Sprintf(
			"__import__('mmgroup.tests.axes.axis',fromlist=['Axis']).Axis(%s).axis_type()", mmExpr(spec))), "\"")
		if got != want {
			t.Errorf("AxisType(%q)=%q want %q", spec, got, want)
		}
	}
}

func TestMonsterPow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		spec string
		e    int
	}{
		{"M<t_1>", 0},
		{"M<t_1>", 2},
		{"M<x_1h*y_2h>", 3},
		{"M<d_5h*p_100*l_1*t_2>", -1},
		{"M<l_1*t_2*l_2*t_1*x_3abh>", -2},
	}
	for _, c := range cases {
		got := mustMM(t, c.spec).Pow(c.e).String()
		want := stripMM(oracle(t, fmt.Sprintf("str((%s**%d).reduce())", mmExpr(c.spec), c.e)))
		if got != want {
			t.Errorf("Pow(%q,%d)=%q want %q", c.spec, c.e, got, want)
		}
		if c.e >= 2 {
			manual := mustMM(t, c.spec)
			for i := 1; i < c.e; i++ {
				manual = manual.Mul(mustMM(t, c.spec))
			}
			if !mustMM(t, c.spec).Pow(c.e).Equal(manual) {
				t.Errorf("Pow(%q,%d) != manual multiplication", c.spec, c.e)
			}
		}
	}
}

// filterCommentAtoms drops tag-0 comment atoms from an
// oracle atom word, using the same (atom>>28)&0x7 == 0
// test as production stripCommentAtoms. mmgroup's
// default mm_reduce_M leaves these neutral atoms in
// .mmdata for the mod-15 axis-reducer path; Go's
// Reduce strips them, so the oracle word must be
// filtered before comparing against Go's public API.
func filterCommentAtoms(w []int64) []int64 {
	out := make([]int64, 0, len(w))
	for _, atom := range w {
		if (atom>>28)&0x7 == 0 {
			continue
		}
		out = append(out, atom)
	}
	return out
}

func TestMonsterMmdata(t *testing.T) {
	t.Parallel()
	specs := []string{
		"M<1>",
		"M<x_1h>",
		"M<t_1*t_1*t_1>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, spec := range specs {
		want := oracleInts(t, fmt.Sprintf("[int(x) for x in %s.mmdata]", mmExpr(spec)))

		// Internal tripwire: the raw reduced word (before
		// comment-atom stripping) must byte-match the
		// oracle's mm_reduce_M output. This is the reducer
		// regression detector; it covers both the prereduce
		// path (G_x0/N_0 specs) and the mod-15 reduceM path
		// (strategy-3 specs, where comment atoms appear in
		// both Go and the oracle).
		g := mustMM(t, spec)
		raw := reduceRaw(g.data)
		if len(raw) != len(want) {
			t.Fatalf("reduceRaw(%q) len=%d want %d\n got=%#x\nwant=%#x",
				spec, len(raw), len(want), raw, want)
		}
		for i := range want {
			if int64(raw[i]) != want[i] {
				t.Errorf("reduceRaw(%q)[%d]=%#x want %#x", spec, i, raw[i], want[i])
			}
		}

		// Public API check: Mmdata strips comment atoms, so
		// it must equal the oracle word with comment atoms
		// filtered out.
		got := mustMM(t, spec).Mmdata()
		wantStripped := filterCommentAtoms(want)
		if len(got) != len(wantStripped) {
			t.Fatalf("Mmdata(%q) len=%d want %d (stripped from %d)",
				spec, len(got), len(wantStripped), len(want))
		}
		for i := range wantStripped {
			if int64(got[i]) != wantStripped[i] {
				t.Errorf("Mmdata(%q)[%d]=%#x want %#x", spec, i, got[i], wantStripped[i])
			}
		}
		// Mmdata must not leak any comment atoms.
		for i, atom := range got {
			if (atom>>28)&0x7 == 0 {
				t.Errorf("Mmdata(%q)[%d]=%#x is a comment atom (should be stripped)", spec, i, atom)
			}
		}
	}
}

func TestMonsterInNx0(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"M<d_2h>":          true,
		"M<x_0a71h>":       true,
		"M<y_0cd1h>":       true,
		"M<p_100>":         true,
		"M<t_1>":           false,
		"M<l_1>":           false,
		"M<x_1h*y_2h*t_1>": false,
	}
	for spec, want := range cases {
		got := mustMM(t, spec).InNx0()
		oWant := oracleBool(t, fmt.Sprintf("%s.in_N_x0()", mmExpr(spec)))
		if oWant != want {
			t.Fatalf("oracle disagrees with case table for %q: %v vs %v", spec, oWant, want)
		}
		if got != want {
			t.Errorf("InNx0(%q)=%v want %v", spec, got, want)
		}
	}
}

func TestMonsterInQx0(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"M<x_0a71h>":   true,
		"M<d_2h>":      true,
		"M<x_1h*d_5h>": true,
		"M<y_0cd1h>":   false,
		"M<p_100>":     false,
		"M<t_1>":       false,
		"M<l_1>":       false,
	}
	for spec, want := range cases {
		got := mustMM(t, spec).InQx0()
		oWant := oracleBool(t, fmt.Sprintf("%s.in_Q_x0()", mmExpr(spec)))
		if oWant != want {
			t.Fatalf("oracle disagrees with case table for %q: %v vs %v", spec, oWant, want)
		}
		if got != want {
			t.Errorf("InQx0(%q)=%v want %v", spec, got, want)
		}
	}
}

func TestMonsterChiGx0(t *testing.T) {
	t.Parallel()
	specs := []string{
		"M<1>",
		"M<d_2h>",
		"M<x_0a71h>",
		"M<y_0cd1h>",
		"M<x_1abh*y_3h*d_4h>",
	}
	for _, spec := range specs {
		got := mustMM(t, spec).ChiGx0()
		want := oracleInts(t, fmt.Sprintf("list(int(x) for x in %s.chi_G_x0())", mmExpr(spec)))
		if len(want) != 4 {
			t.Fatalf("oracle chi_G_x0(%q) len=%d want 4", spec, len(want))
		}
		for i := 0; i < 4; i++ {
			if int64(got[i]) != want[i] {
				t.Errorf("ChiGx0(%q)[%d]=%d want %d", spec, i, got[i], want[i])
			}
		}
	}
}

// TestMonsterIsReduced checks that an element is in
// reduced form after Reduce(). The oracle's
// is_reduced() compares self.length (an int) to
// self.reduced (a 0/1 flag), so it returns False for
// any element of length != 1 even after reduce(); it is
// not a usable reference. Go has no reduced flag: the
// word IS the canonical form, so IsReduced reduces a
// copy and compares word-for-word, and must report true
// for any already-reduced element.
func TestMonsterIsReduced(t *testing.T) {
	t.Parallel()
	specs := []string{
		"M<t_1*t_1*t_1>",
		"M<d_2h*d_2h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, spec := range specs {
		if got := mustMM(t, spec).Reduce().IsReduced(); !got {
			t.Errorf("reduced element %q reports IsReduced()=false", spec)
		}
	}
}

func TestMonsterAxisMul(t *testing.T) {
	t.Parallel()
	cases := []struct {
		spec  string
		gspec string
	}{
		{"M<x_1h*y_2h>", "M<t_1>"},
		{"M<t_1*l_1*t_2>", "M<x_1abh>"},
		{"M<d_5h*p_100>", "M<l_1*t_2>"},
	}
	for _, c := range cases {
		ax := mustAxisFor(t, mustMM(t, c.spec)).Mul(mustMM(t, c.gspec))
		got := ax.Type()
		want := strings.Trim(oracle(t, fmt.Sprintf(
			"(__import__('mmgroup.tests.axes.axis',fromlist=['Axis']).Axis(%s)*%s).axis_type()",
			mmExpr(c.spec), mmExpr(c.gspec))), "\"")
		if got != want {
			t.Errorf("Axis(%q).Mul(%q).Type()=%q want %q", c.spec, c.gspec, got, want)
		}
	}
}

func TestMonsterAxisEqual(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
	}{
		{"M<x_1h*y_2h>", "M<x_1h*y_2h>"},
		{"M<t_1*l_1*t_2>", "M<x_1abh*d_4h>"},
		{"M<d_5h*p_100>", "M<d_5h*p_100>"},
	}
	for _, c := range cases {
		axA := mustAxisFor(t, mustMM(t, c.a))
		axB := mustAxisFor(t, mustMM(t, c.b))
		if !axA.Equal(axA) {
			t.Errorf("Axis(%q) not equal to itself", c.a)
		}
		got := axA.Equal(axB)
		want := oracleBool(t, fmt.Sprintf(
			"bool(__import__('mmgroup.tests.axes.axis',fromlist=['Axis']).Axis(%s)==__import__('mmgroup.tests.axes.axis',fromlist=['Axis']).Axis(%s))",
			mmExpr(c.a), mmExpr(c.b)))
		if got != want {
			t.Errorf("Axis(%q).Equal(Axis(%q))=%v want %v", c.a, c.b, got, want)
		}
	}
}

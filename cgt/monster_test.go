package cgt

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
	cases := [][2]string{
		{"M<x_1h>", "M<y_2h>"},
		{"M<t_1>", "M<l_1>"},
		{"M<d_2h*p_1>", "M<y_0c00h*t_2>"},
		{"M<x_1h*y_2h*t_1*l_2>", "M<d_5h*p_100*t_1>"},
		{"M<l_1*t_2*l_2*t_1>", "M<x_1abh*y_3h*d_4h>"},
	}
	for _, c := range cases {
		g := mustMM(t,c[0])
		h := mustMM(t,c[1])
		got := g.Mul(h).String()
		want := stripMM(oracle(t, fmt.Sprintf("str((%s*%s).reduce())", mmExpr(c[0]), mmExpr(c[1]))))
		if got != want {
			t.Errorf("Mul(%q,%q)=%q want %q", c[0], c[1], got, want)
		}
	}
}

func TestMonsterInv(t *testing.T) {
	cases := []string{
		"M<x_1h>",
		"M<t_1>",
		"M<x_1h*y_2h*t_1*l_2>",
		"M<d_5h*p_100*l_1*t_2>",
		"M<l_1*t_2*l_2*t_1*x_3abh>",
	}
	for _, c := range cases {
		g := mustMM(t,c)
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
	cases := []string{
		"M<1>",
		"M<t_1>",
		"M<d_2h*d_3h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, c := range cases {
		got := int64(mustMM(t,c).Order())
		want := oracleInt(t, fmt.Sprintf("%s.order()", mmExpr(c)))
		if got != want {
			t.Errorf("Order(%q)=%d want %d", c, got, want)
		}
	}
}

func TestMonsterHalfOrder(t *testing.T) {
	cases := []string{
		"M<t_1>",
		"M<d_2h*d_3h>",
		"M<x_1000h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
	}
	for _, c := range cases {
		o, h := mustMM(t,c).HalfOrder()
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
	cases := [][2]string{
		{"M<1>", "M<1>"},
		{"M<d_2h*d_2h>", "M<1>"},
		{"M<x_1h>", "M<y_2h>"},
		{"M<t_1*t_1*t_1>", "M<1>"},
		{"M<x_1h*y_2h>", "M<y_2h*x_1h>"},
	}
	for _, c := range cases {
		got := mustMM(t,c[0]).Equal(mustMM(t,c[1]))
		want := oracleBool(t, fmt.Sprintf("(%s == %s)", mmExpr(c[0]), mmExpr(c[1])))
		if got != want {
			t.Errorf("Equal(%q,%q)=%v want %v", c[0], c[1], got, want)
		}
	}
}

func TestMonsterReduce(t *testing.T) {
	cases := []string{
		"M<t_1*t_1*t_1>",
		"M<d_2h*d_2h>",
		"M<x_1h*y_2h*t_1*l_2*p_100*l_1*t_2>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h*y_5h*p_200>",
	}
	for _, c := range cases {
		got := mustMM(t,c).Reduce().String()
		want := stripMM(oracle(t, fmt.Sprintf("str(%s.reduce())", mmExpr(c))))
		if got != want {
			t.Errorf("Reduce(%q)=%q want %q", c, got, want)
		}
	}
}

func TestMonsterAsIntRoundTrip(t *testing.T) {
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
		if !MMFromInt(n).Equal(MMGen(c.tag, c.i)) {
			t.Errorf("MMFromInt(AsInt(%q)) != original", spec)
		}
	}
}

func TestMonsterInGx0(t *testing.T) {
	cases := map[string]bool{
		"M<d_2h>":          true,
		"M<x_0a71h>":       true,
		"M<y_0cd1h>":       true,
		"M<t_1>":           false,
		"M<x_1h*y_2h*t_1>": false,
	}
	for spec, want := range cases {
		got := mustMM(t,spec).InGx0()
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
	specs := []string{
		"M<x_1h*y_2h>",
		"M<t_1*l_1*t_2>",
		"M<d_5h*p_100*l_1*t_2>",
		"M<l_2*t_1*x_1abh*y_3h>",
	}
	for _, spec := range specs {
		got := AxisFor(mustMM(t,spec)).Type()
		want := strings.Trim(oracle(t, fmt.Sprintf(
			"__import__('mmgroup.tests.axes.axis',fromlist=['Axis']).Axis(%s).axis_type()", mmExpr(spec))), "\"")
		if got != want {
			t.Errorf("AxisType(%q)=%q want %q", spec, got, want)
		}
	}
}

func TestMonsterPow(t *testing.T) {
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
		got := mustMM(t,c.spec).Pow(c.e).String()
		want := stripMM(oracle(t, fmt.Sprintf("str((%s**%d).reduce())", mmExpr(c.spec), c.e)))
		if got != want {
			t.Errorf("Pow(%q,%d)=%q want %q", c.spec, c.e, got, want)
		}
		if c.e >= 2 {
			manual := mustMM(t,c.spec)
			for i := 1; i < c.e; i++ {
				manual = manual.Mul(mustMM(t,c.spec))
			}
			if !mustMM(t,c.spec).Pow(c.e).Equal(manual) {
				t.Errorf("Pow(%q,%d) != manual multiplication", c.spec, c.e)
			}
		}
	}
}

func TestMonsterMmdata(t *testing.T) {
	specs := []string{
		"M<1>",
		"M<x_1h>",
		"M<t_1*t_1*t_1>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, spec := range specs {
		got := mustMM(t,spec).Mmdata()
		want := oracleInts(t, fmt.Sprintf("[int(x) for x in %s.mmdata]", mmExpr(spec)))
		if len(got) != len(want) {
			t.Fatalf("Mmdata(%q) len=%d want %d", spec, len(got), len(want))
		}
		for i := range want {
			if int64(got[i]) != want[i] {
				t.Errorf("Mmdata(%q)[%d]=%#x want %#x", spec, i, got[i], want[i])
			}
		}
	}
}

func TestMonsterInNx0(t *testing.T) {
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
		got := mustMM(t,spec).InNx0()
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
		got := mustMM(t,spec).InQx0()
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
	specs := []string{
		"M<1>",
		"M<d_2h>",
		"M<x_0a71h>",
		"M<y_0cd1h>",
		"M<x_1abh*y_3h*d_4h>",
	}
	for _, spec := range specs {
		got := mustMM(t,spec).ChiGx0()
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

// TODO(phase2): This test is self-contradictory. The oracle's
// MM.is_reduced() compares self.length == self.reduced (length vs a
// 0/1 flag), so it returns False even after reduce(). Line 376 checks
// got == want (want=false from oracle), but line 378 requires got=true.
// Both cannot pass. Resolve jointly with IsReduced semantics and
// comment-atom stripping (PLAN step 9 / Phase 2).
func TestMonsterIsReduced(t *testing.T) {
	specs := []string{
		"M<t_1*t_1*t_1>",
		"M<d_2h*d_2h>",
		"M<x_1h*y_2h*t_1*l_2*p_100>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h>",
	}
	for _, spec := range specs {
		got := mustMM(t,spec).Reduce().IsReduced()
		want := oracleBool(t, fmt.Sprintf("(lambda g:(g.reduce(),g.is_reduced())[1])(%s)", mmExpr(spec)))
		if got != want {
			t.Errorf("IsReduced(reduce(%q))=%v want %v", spec, got, want)
		}
		if !got {
			t.Errorf("reduced element %q reports IsReduced()=false", spec)
		}
	}
}

func TestMonsterAxisMul(t *testing.T) {
	cases := []struct {
		spec  string
		gspec string
	}{
		{"M<x_1h*y_2h>", "M<t_1>"},
		{"M<t_1*l_1*t_2>", "M<x_1abh>"},
		{"M<d_5h*p_100>", "M<l_1*t_2>"},
	}
	for _, c := range cases {
		ax := AxisFor(mustMM(t,c.spec)).Mul(mustMM(t,c.gspec))
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
	cases := []struct {
		a, b string
	}{
		{"M<x_1h*y_2h>", "M<x_1h*y_2h>"},
		{"M<t_1*l_1*t_2>", "M<x_1abh*d_4h>"},
		{"M<d_5h*p_100>", "M<d_5h*p_100>"},
	}
	for _, c := range cases {
		axA := AxisFor(mustMM(t,c.a))
		axB := AxisFor(mustMM(t,c.b))
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

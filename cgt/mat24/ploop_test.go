package mat24

import (
	"fmt"
	"testing"
)

func intsToInts(a []int) []int64 {
	r := make([]int64, len(a))
	for i, x := range a {
		r[i] = int64(x)
	}
	return r
}

func TestPLoop(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x0000, 0x0001, 0x0800, 0x1234, 0x1abc} {
		p := NewPLoop(v)
		if got, want := uint64(p.Ord()), oracleUint(t, fmt.Sprintf("mmgroup.PLoop(%d).ord", v)); got != want {
			t.Errorf("PLoop(%#x).Ord = %d, want %d", v, got, want)
		}
		if got, want := int64(p.Sign()), oracleInt(t, fmt.Sprintf("mmgroup.PLoop(%d).sign", v)); got != want {
			t.Errorf("PLoop(%#x).Sign = %d, want %d", v, got, want)
		}
		if got, want := int64(p.Len()), oracleInt(t, fmt.Sprintf("len(mmgroup.PLoop(%d))", v)); got != want {
			t.Errorf("PLoop(%#x).Len = %d, want %d", v, got, want)
		}
		if got, want := uint64(p.Neg().Ord()), oracleUint(t, fmt.Sprintf("(-mmgroup.PLoop(%d)).ord", v)); got != want {
			t.Errorf("PLoop(%#x).Neg = %d, want %d", v, got, want)
		}
		if got, want := uint64(p.Invert().Ord()), oracleUint(t, fmt.Sprintf("(~mmgroup.PLoop(%d)).ord", v)); got != want {
			t.Errorf("PLoop(%#x).Invert = %d, want %d", v, got, want)
		}
		if got, want := uint64(p.Theta().Ord()), oracleUint(t, fmt.Sprintf("mmgroup.PLoop(%d).theta().ord", v)); got != want {
			t.Errorf("PLoop(%#x).Theta = %d, want %d", v, got, want)
		}
	}
}

func TestPLoopMul(t *testing.T) {
	t.Parallel()
	pairs := [][2]int{{0x1234, 0x0567}, {0x0001, 0x0002}, {0x1abc, 0x0def}, {0x0800, 0x1000}}
	for _, pr := range pairs {
		a, b := pr[0], pr[1]
		pa, pb := NewPLoop(a), NewPLoop(b)
		if got, want := uint64(pa.Mul(pb).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.PLoop(%d) * mmgroup.PLoop(%d)).ord", a, b)); got != want {
			t.Errorf("PLoop(%#x).Mul(%#x) = %d, want %d", a, b, got, want)
		}
		if got, want := uint64(pa.Cap(pb).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.PLoop(%d) & mmgroup.PLoop(%d)).ord", a, b)); got != want {
			t.Errorf("PLoop(%#x).Cap(%#x) = %d, want %d", a, b, got, want)
		}
		if got, want := int64(pa.Comm(pb)), oracleInt(t, fmt.Sprintf("mat24.ploop_comm(%d, %d)", a, b)); got != want {
			t.Errorf("PLoop(%#x).Comm(%#x) = %d, want %d", a, b, got, want)
		}
	}
	p := NewPLoop(0x1234)
	for _, e := range []int{0, 1, 2, 3} {
		if got, want := uint64(p.Pow(e).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.PLoop(0x1234)**%d).ord", e)); got != want {
			t.Errorf("PLoop(0x1234).Pow(%d) = %d, want %d", e, got, want)
		}
	}
}

func TestPLoopAssoc(t *testing.T) {
	t.Parallel()
	triples := [][3]int{{0x1234, 0x0567, 0x0abc}, {0x0001, 0x0002, 0x0004}, {0x1abc, 0x0def, 0x0111}, {0x0800, 0x0fff, 0x0123}}
	for _, tr := range triples {
		a, b, c := tr[0], tr[1], tr[2]
		pa, pb, pc := NewPLoop(a), NewPLoop(b), NewPLoop(c)
		if got, want := int64(pa.Assoc(pb, pc)), oracleInt(t, fmt.Sprintf("mat24.ploop_assoc(%d, %d, %d)", a, b, c)); got != want {
			t.Errorf("PLoop.Assoc(%#x,%#x,%#x) = %d, want %d", a, b, c, got, want)
		}
	}
}

func TestGCodeStruct(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x000, 0x001, 0x800, 0x234, 0xabc} {
		g := NewGCode(v)
		if got, want := int64(g.Len()), oracleInt(t, fmt.Sprintf("len(mmgroup.GCode(%d))", v)); got != want {
			t.Errorf("GCode(%#x).Len = %d, want %d", v, got, want)
		}
		if got, want := uint64(g.Vector()), oracleUint(t, fmt.Sprintf("mmgroup.GCode(%d).vector", v)); got != want {
			t.Errorf("GCode(%#x).Vector = %d, want %d", v, got, want)
		}
		eqInts(t, fmt.Sprintf("GCode(%#x).BitList", v), intsToInts(g.BitList()), oracleInts(t, fmt.Sprintf("mmgroup.GCode(%d).bit_list", v)))
	}
	pairs := [][2]int{{0x234, 0xabc}, {0x001, 0x002}, {0x800, 0xfff}}
	for _, pr := range pairs {
		a, b := pr[0], pr[1]
		if got, want := uint64(NewGCode(a).Add(NewGCode(b)).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.GCode(%d) + mmgroup.GCode(%d)).ord", a, b)); got != want {
			t.Errorf("GCode(%#x).Add(%#x) = %d, want %d", a, b, got, want)
		}
		if got, want := int64(NewGCode(a).ScalarProd(NewCocode(b)).Int()), oracleInt(t, fmt.Sprintf("(mmgroup.GCode(%d) & mmgroup.Cocode(%d)).ord", a, b)); got != want {
			t.Errorf("GCode(%#x).ScalarProd(Cocode %#x) = %d, want %d", a, b, got, want)
		}
	}
}

func TestCocodeStruct(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x000, 0x823, 0x7ef, 0x001, 0xabc} {
		c := NewCocode(v)
		if got, want := int64(c.Len()), oracleInt(t, fmt.Sprintf("len(mmgroup.Cocode(%d))", v)); got != want {
			t.Errorf("Cocode(%#x).Len = %d, want %d", v, got, want)
		}
		if got, want := int64(c.Parity().Int()), oracleInt(t, fmt.Sprintf("mmgroup.Cocode(%d).parity", v)); got != want {
			t.Errorf("Cocode(%#x).Parity = %d, want %d", v, got, want)
		}
		if got, want := uint64(c.Syndrome(0)), oracleUint(t, fmt.Sprintf("mmgroup.Cocode(%d).syndrome(0).ord", v)); got != want {
			t.Errorf("Cocode(%#x).Syndrome(0) = %d, want %d", v, got, want)
		}
		eqInts(t, fmt.Sprintf("Cocode(%#x).SyndromeList(0)", v), intsToInts(c.SyndromeList(0)), oracleInts(t, fmt.Sprintf("mmgroup.Cocode(%d).syndrome(0).bit_list", v)))
	}
	for _, v := range []int{0x000, 0x823, 0xabc} {
		eqInts(t, fmt.Sprintf("Cocode(%#x).AllSyndromes", v), u32sToInts(NewCocode(v).AllSyndromes()), oracleInts(t, fmt.Sprintf("[x.ord for x in mmgroup.Cocode(%d).all_syndromes()]", v)))
	}
}

func TestParkerLoopGCode(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x0000, 0x1234, 0x0800, 0x1abc} {
		p := NewPLoop(v)
		if got, want := uint64(p.Abs().Ord()), oracleUint(t, fmt.Sprintf("abs(mmgroup.PLoop(%d)).ord", v)); got != want {
			t.Errorf("PLoop(%#x).Abs = %d, want %d", v, got, want)
		}
		if got, want := uint64(p.GCode().Ord()), oracleUint(t, fmt.Sprintf("mmgroup.GCode(mmgroup.PLoop(%d)).ord", v)); got != want {
			t.Errorf("PLoop(%#x).GCode = %d, want %d", v, got, want)
		}
		es, eo, pp := p.Split()
		ws := oracleInt(t, fmt.Sprintf("mmgroup.PLoop(%d).split()[0]", v))
		wo := oracleInt(t, fmt.Sprintf("mmgroup.PLoop(%d).split()[1]", v))
		wp := oracleUint(t, fmt.Sprintf("mmgroup.PLoop(%d).split()[2].ord", v))
		if int64(es) != ws || int64(eo) != wo || uint64(pp.Ord()) != wp {
			t.Errorf("PLoop(%#x).Split = (%d,%d,%d), want (%d,%d,%d)", v, es, eo, pp.Ord(), ws, wo, wp)
		}
	}
}

func TestAutPLGroup(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c1, p1, c2, p2 int
	}{
		{0x000, 0, 0x000, 12345},
		{0x123, 6789, 0x456, 100000},
		{0x7ef, 1, 0x823, 244823039},
		{0xfff, 555, 0x001, 9999},
	}
	for _, tc := range cases {
		a := NewAutPL(tc.c1, tc.p1)
		b := NewAutPL(tc.c2, tc.p2)
		ab := a.Mul(b)
		wc := oracleUint(t, fmt.Sprintf("(mmgroup.AutPL(%d, %d) * mmgroup.AutPL(%d, %d)).cocode", tc.c1, tc.p1, tc.c2, tc.p2))
		wp := oracleUint(t, fmt.Sprintf("(mmgroup.AutPL(%d, %d) * mmgroup.AutPL(%d, %d)).perm_num", tc.c1, tc.p1, tc.c2, tc.p2))
		if uint64(ab.Cocode()) != wc || uint64(ab.PermNum()) != wp {
			t.Errorf("AutPL(%#x,%d).Mul(AutPL(%#x,%d)) = (%#x,%d), want (%#x,%d)", tc.c1, tc.p1, tc.c2, tc.p2, ab.Cocode(), ab.PermNum(), wc, wp)
		}
	}
	invCases := []struct{ c, p int }{
		{0x000, 12345}, {0x123, 6789}, {0x7ef, 244823039}, {0xfff, 555},
	}
	for _, ic := range invCases {
		inv := NewAutPL(ic.c, ic.p).Inv()
		wc := oracleUint(t, fmt.Sprintf("(mmgroup.AutPL(%d, %d)**-1).cocode", ic.c, ic.p))
		wp := oracleUint(t, fmt.Sprintf("(mmgroup.AutPL(%d, %d)**-1).perm_num", ic.c, ic.p))
		if uint64(inv.Cocode()) != wc || uint64(inv.PermNum()) != wp {
			t.Errorf("AutPL(%#x,%d).Inv = (%#x,%d), want (%#x,%d)", ic.c, ic.p, inv.Cocode(), inv.PermNum(), wc, wp)
		}
	}
}

func TestAutPLPerm(t *testing.T) {
	t.Parallel()
	for _, num := range []int{0, 1, 12345, 1000000, 244823039} {
		a := NewAutPL(0, num)
		eqInts(t, fmt.Sprintf("AutPL(0,%d).Perm", num), intsToInts(a.Perm()), oracleInts(t, fmt.Sprintf("mmgroup.AutPL(0, %d).perm", num)))
		if got, want := uint64(a.PermNum()), oracleUint(t, fmt.Sprintf("mmgroup.AutPL(0, %d).perm_num", num)); got != want {
			t.Errorf("AutPL(0,%d).PermNum = %d, want %d", num, got, want)
		}
		if got, want := int64(a.Parity().Int()), oracleInt(t, fmt.Sprintf("mmgroup.AutPL(0, %d).parity", num)); got != want {
			t.Errorf("AutPL(0,%d).Parity = %d, want %d", num, got, want)
		}
	}
}

func TestAutPLAction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v, c, p int
	}{
		{0x1234, 0x000, 12345},
		{0x0567, 0x123, 6789},
		{0x1abc, 0x7ef, 100000},
		{0x0800, 0xfff, 244823039},
	}
	for _, tc := range cases {
		a := NewAutPL(tc.c, tc.p)
		if got, want := uint64(NewPLoop(tc.v).Apply(a).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.PLoop(%d) * mmgroup.AutPL(%d, %d)).ord", tc.v, tc.c, tc.p)); got != want {
			t.Errorf("PLoop(%#x).Apply(AutPL(%#x,%d)) = %d, want %d", tc.v, tc.c, tc.p, got, want)
		}
		cc := tc.v & 0xfff
		if got, want := uint64(NewCocode(cc).Apply(a).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.Cocode(%d) * mmgroup.AutPL(%d, %d)).ord", cc, tc.c, tc.p)); got != want {
			t.Errorf("Cocode(%#x).Apply(AutPL(%#x,%d)) = %d, want %d", cc, tc.c, tc.p, got, want)
		}
	}
}

func TestOctadStruct(t *testing.T) {
	t.Parallel()
	for _, no := range []int{0, 1, 100, 500, 758} {
		o := NewOctad(no)
		if got, want := uint64(o.GCode()), oracleUint(t, fmt.Sprintf("mmgroup.Octad(%d).gcode", no)); got != want {
			t.Errorf("Octad(%d).GCode = %d, want %d", no, got, want)
		}
		if got, want := int64(o.Octad()), oracleInt(t, fmt.Sprintf("mmgroup.Octad(%d).octad", no)); got != want {
			t.Errorf("Octad(%d).Octad = %d, want %d", no, got, want)
		}
	}
}

func TestParityType(t *testing.T) {
	t.Parallel()
	p0 := NewParity(0)
	p1 := NewParity(1)
	if p0.Int() != 0 || p1.Int() != 1 {
		t.Fatalf("Parity.Int: got %d,%d want 0,1", p0.Int(), p1.Int())
	}
	if !p0.Equal(NewParity(0)) || p0.Equal(p1) {
		t.Fatalf("Parity.Equal failed")
	}
	if p0.Add(p1).Int() != 1 || p1.Add(p1).Int() != 0 {
		t.Fatalf("Parity.Add failed")
	}
}

func TestPLoopNonAssociativity(t *testing.T) {
	t.Parallel()
	// These triples have a nonzero Parker-loop associator (oracle-
	// confirmed via mat24.ploop_assoc), so the body that checks
	// (a*b)*c != a*(b*c) actually executes. The associator is a sign
	// flip, so (a*b)*c and a*(b*c) differ by negation and thus have
	// distinct Ord values.
	triples := [][3]int{{0x111, 0x222, 0x0f0f}, {0x111, 0x444, 0x033}, {0x111, 0x01f, 0x3e0}}
	for _, tr := range triples {
		a, b, c := NewPLoop(tr[0]), NewPLoop(tr[1]), NewPLoop(tr[2])
		assoc := a.Assoc(b, c)
		if assoc != 0 {
			lhs := a.Mul(b).Mul(c)
			rhs := a.Mul(b.Mul(c))
			if lhs.Ord() == rhs.Ord() {
				t.Errorf("PLoop assoc(%#x,%#x,%#x)=%d but (a*b)*c == a*(b*c)", tr[0], tr[1], tr[2], assoc)
			}
		}
	}
}

func TestGCodeUntested(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x001, 0x234, 0x800, 0xabc} {
		g := NewGCode(v)
		if got, want := uint64(g.Invert().Ord()), oracleUint(t, fmt.Sprintf("(~mmgroup.GCode(%d)).ord", v)); got != want {
			t.Errorf("GCode(%#x).Invert = %d, want %d", v, got, want)
		}
	}
	pairs := [][2]int{{0x234, 0xabc}, {0x001, 0x800}}
	for _, pr := range pairs {
		if got, want := uint64(NewGCode(pr[0]).And(NewGCode(pr[1])).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.GCode(%d) & mmgroup.GCode(%d)).ord", pr[0], pr[1])); got != want {
			t.Errorf("GCode(%#x).And(%#x) = %d, want %d", pr[0], pr[1], got, want)
		}
	}
	for _, no := range []int{0, 1, 100, 500} {
		g := NewGCode(int(NewOctad(no).GCode()))
		if got, want := int64(g.Octad()), oracleInt(t, fmt.Sprintf("mmgroup.GCode(mmgroup.Octad(%d).gcode).octad", no)); got != want {
			t.Errorf("GCode.Octad(%d) = %d, want %d", no, got, want)
		}
	}
}

func TestPLoopSplitOctad(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x0001, 0x0800, 0x1234} {
		p := NewPLoop(v)
		es, eo, pp := p.SplitOctad()
		ws := oracleInt(t, fmt.Sprintf("mmgroup.PLoop(%d).split_octad()[0]", v))
		wo := oracleInt(t, fmt.Sprintf("mmgroup.PLoop(%d).split_octad()[1]", v))
		wp := oracleUint(t, fmt.Sprintf("mmgroup.PLoop(%d).split_octad()[2].ord", v))
		if int64(es) != ws || int64(eo) != wo || uint64(pp.Ord()) != wp {
			t.Errorf("PLoop(%#x).SplitOctad = (%d,%d,%d), want (%d,%d,%d)", v, es, eo, pp.Ord(), ws, wo, wp)
		}
	}
}

func TestAutPLPowEqual(t *testing.T) {
	t.Parallel()
	a := NewAutPL(0x123, 6789)
	a2 := a.Pow(2)
	aa := a.Mul(a)
	if !a2.Equal(aa) {
		t.Errorf("AutPL.Pow(2) != AutPL.Mul(self)")
	}
	a0 := a.Pow(0)
	id := NewAutPL(0, 0)
	if !a0.Equal(id) {
		t.Errorf("AutPL.Pow(0) != identity")
	}
}

func TestPLoopZ(t *testing.T) {
	t.Parallel()
	cases := []struct{ e1, eo int }{
		{0, 0},
		{1, 0},
		{0, 1},
		{1, 1},
		{2, 3},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.e1, c.eo), func(t *testing.T) {
			if got, want := uint64(PLoopZ(c.e1, c.eo).Ord()), oracleUint(t, fmt.Sprintf("mmgroup.PLoopZ(%d, %d).ord", c.e1, c.eo)); got != want {
				t.Errorf("PLoopZ(%d,%d).Ord = %d, want %d", c.e1, c.eo, got, want)
			}
		})
	}
}

func TestGCodeTheta(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0x000, 0x001, 0x234, 0x800, 0xabc} {
		t.Run(fmt.Sprintf("%#x", v), func(t *testing.T) {
			if got, want := uint64(NewGCode(v).Theta().Ord()), oracleUint(t, fmt.Sprintf("mmgroup.GCode(%d).theta().ord", v)); got != want {
				t.Errorf("GCode(%#x).Theta = %d, want %d", v, got, want)
			}
		})
	}
}

func TestGCodeThetaWith(t *testing.T) {
	t.Parallel()
	pairs := [][2]int{{0x234, 0xabc}, {0x001, 0x002}, {0x800, 0xfff}, {0x123, 0x456}}
	for _, pr := range pairs {
		a, b := pr[0], pr[1]
		t.Run(fmt.Sprintf("%#x_%#x", a, b), func(t *testing.T) {
			if got, want := int64(NewGCode(a).ThetaWith(NewGCode(b)).Int()), oracleInt(t, fmt.Sprintf("int(mmgroup.GCode(%d).theta(mmgroup.GCode(%d)))", a, b)); got != want {
				t.Errorf("GCode(%#x).ThetaWith(%#x) = %d, want %d", a, b, got, want)
			}
		})
	}
}

func TestGCodeApply(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v, c, p int
	}{
		{0x234, 0x000, 12345},
		{0x800, 0x123, 6789},
		{0xabc, 0x7ef, 100000},
		{0x001, 0xfff, 244823039},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%#x_%#x_%d", tc.v, tc.c, tc.p), func(t *testing.T) {
			a := NewAutPL(tc.c, tc.p)
			if got, want := uint64(NewGCode(tc.v).Apply(a).Ord()), oracleUint(t, fmt.Sprintf("(mmgroup.GCode(%d) * mmgroup.AutPL(%d, %d)).ord", tc.v, tc.c, tc.p)); got != want {
				t.Errorf("GCode(%#x).Apply(AutPL(%#x,%d)) = %d, want %d", tc.v, tc.c, tc.p, got, want)
			}
		})
	}
}

func TestAutPLCheck(t *testing.T) {
	t.Parallel()
	cases := []struct{ c, p int }{
		{0x000, 0},
		{0x123, 6789},
		{0x7ef, 244823039},
		{0xfff, 555},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%#x_%d", tc.c, tc.p), func(t *testing.T) {
			chk := NewAutPL(tc.c, tc.p).Check()
			wc := oracleUint(t, fmt.Sprintf("mmgroup.AutPL(%d, %d).check().cocode", tc.c, tc.p))
			wp := oracleUint(t, fmt.Sprintf("mmgroup.AutPL(%d, %d).check().perm_num", tc.c, tc.p))
			if uint64(chk.Cocode()) != wc || uint64(chk.PermNum()) != wp {
				t.Errorf("AutPL(%#x,%d).Check = (%#x,%d), want (%#x,%d)", tc.c, tc.p, chk.Cocode(), chk.PermNum(), wc, wp)
			}
		})
	}
}

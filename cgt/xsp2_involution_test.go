package cgt

import "testing"

// Tests for the involution machinery of G_{x0}
// (xsp2_involution.go), grounded in the canonical
// mmgroup tests
//   mmgroup/tests/test_involutions/test_display_characters.py
//     (test_xsp2co1_elem_involution_class)
//   mmgroup/tests/test_involutions/test_involution_Gx0.py
//     (test_std_rep)
//   mmgroup/tests/test_clifford/test_xsp2_conjugate.py
//     (test_xsp2_conjugate)
//   mmgroup/tests/test_orders/test_order.py
//     (test_chi_powers)
//
// The involution samples below are the order-<=2
// representatives of the classes of square roots of
// Q_{x0} in G_{x0}, taken from
//   mmgroup/tests/test_involutions/involution_samples.py
// together with two order-4 samples (x_0ae0h, y_0ae0h)
// used by the class test. Expected involution-class
// numbers and standard representatives were obtained
// from the C reference via `goof mmgroup.py`.

// xspFromMM converts a reduced G_x0 monster element to an
// Xsp2Co1. A reduced G_x0 word uses only the generators
// d, p, x, y, l (no inverse or triality tags), so
// atomsFromWord yields a valid G_x0 atom list; ok is
// false only if a stray non-G_x0 tag appears, in which
// case the element is skipped by the caller.
func xspFromMM(g *MM) (gx *Xsp2Co1, ok bool) {
	defer func() {
		if recover() != nil {
			gx, ok = nil, false
		}
	}()
	return NewXsp2Co1(atomsFromWord(g.Mmdata())...), true
}

// involSamples are the involution-class representatives
// (order-1 and order-2, plus two order-4 elements) with
// their xsp2co1_elem_involution_class numbers.
var involSamples = []struct {
	name  string
	atoms []XspAtom
	class int32
	order int
}{
	{"1", nil, 0x1011, 1},
	{"x_1000h", []XspAtom{{"x", 0x1000}}, 0x3022, 2},
	{"x_80fh", []XspAtom{{"x", 0x80f}}, 0x21, 2},
	{"x_800h", []XspAtom{{"x", 0x800}}, 0x22, 2},
	{"y_80fh", []XspAtom{{"y", 0x80f}}, 0x1121, 2},
	{"y_0fh", []XspAtom{{"y", 0xf}}, 0x1122, 2},
	{"y_80fh*d_3h", []XspAtom{{"y", 0x80f}, {"d", 0x3}}, 0x122, 2},
	{"y_0ae0h*d_20h", []XspAtom{{"y", 0xae0}, {"d", 0x20}}, 0x322, 2},
	{"x_0ae0h", []XspAtom{{"x", 0xae0}}, 0x2041, 4},
	{"y_0ae0h", []XspAtom{{"y", 0xae0}}, 0x341, 4},
}

// TestXsp2Co1InvolutionClass mirrors
// test_xsp2co1_elem_involution_class. For each sample it
// checks that xsp2co1_elem_involution_class returns the
// expected class number and that the class is invariant
// under conjugation by random elements of G_{x0}. This
// exercises xsp2co1InvolutionInvariants,
// xsp2co1Leech2CountType2 and the trace classifier across
// all involution-class shapes (dim I_1 = 0,1,8,9,12).
func TestXsp2Co1InvolutionClass(t *testing.T) {
	t.Parallel()
	for _, s := range involSamples {
		g := NewXsp2Co1(s.atoms...)
		if got := g.Order(); got != s.order {
			t.Errorf("%s: order=%d want %d", s.name, got, s.order)
		}
		if got := xsp2co1ElemInvolutionClass(g.data[:]); got != s.class {
			t.Errorf("%s: involution class=%#x want %#x", s.name, got, s.class)
		}
		// The class is a conjugacy invariant: conjugating by
		// a random G_x0 element must not change it.
		for i := 0; i < 25; i++ {
			c, ok := xspFromMM(MMRandIn(SubGx0))
			if !ok {
				continue
			}
			h := c.Inv().Mul(g).Mul(c)
			if got := xsp2co1ElemInvolutionClass(h.data[:]); got != s.class {
				t.Fatalf("%s: class changed under conjugation: %#x want %#x", s.name, got, s.class)
			}
		}
	}
}

// TestXsp2Co1ConjElem mirrors the core relation of
// test_xsp2_conjugate: for an extraspecial element x in
// Q_{x0} and an element g of G_{x0}, the group
// conjugation g^{-1} x g (computed by xsp2co1ConjElem)
// agrees with both g.XspConjugate(x) and the direct
// product g^{-1} * x * g. The generators l (=xi) and
// some random G_x0 elements are used as g, as in the
// mmgroup create_conjugate_data().
func TestXsp2Co1ConjElem(t *testing.T) {
	t.Parallel()
	gs := []*Xsp2Co1{
		NewXsp2Co1(XspAtom{"l", 1}),
		NewXsp2Co1(XspAtom{"l", 2}),
	}
	for i := 0; i < 6; i++ {
		if c, ok := xspFromMM(MMRandIn(SubGx0)); ok {
			gs = append(gs, c)
		}
	}
	for gi, g := range gs {
		for i := 0; i < 24; i++ {
			x := uint32(1) << uint(i)
			xg := Xsp2FromXsp(x)
			var res Xsp2Co1
			// xsp2co1ConjElem(elem1, elem2, elem3): elem3 = elem2^-1 elem1 elem2.
			xsp2co1ConjElem(xg.data[:], g.data[:], res.data[:])
			got := res.AsXsp()
			wantXsp := g.XspConjugate([]uint32{x})[0]
			wantMul := g.Inv().Mul(xg).Mul(g).AsXsp()
			if got != wantXsp {
				t.Errorf("g#%d: xsp2co1ConjElem(x=%#x)=%#x, XspConjugate=%#x", gi, x, got, wantXsp)
			}
			if got != wantMul {
				t.Errorf("g#%d: xsp2co1ConjElem(x=%#x)=%#x, g^-1 x g=%#x", gi, x, got, wantMul)
			}
		}
	}
}

// stdInvol is the I value and standard representative
// (as a reduced G_x0 word) that
// xsp2co1_elem_conjugate_involution maps a class to,
// from the C reference. I=0 identity, I=1 the 2A
// involution x_{[2,3]} = 0x10000200, I=2 the central 2B
// involution z = x_1000h = 0x30001000.
var stdInvolReps = []struct {
	name  string
	atoms []XspAtom
	i     int
	std   []uint32
}{
	{"1", nil, 0, []uint32{}},
	{"x_1000h", []XspAtom{{"x", 0x1000}}, 2, []uint32{0x30001000}},
	{"x_80fh", []XspAtom{{"x", 0x80f}}, 1, []uint32{0x10000200}},
	{"x_800h", []XspAtom{{"x", 0x800}}, 2, []uint32{0x30001000}},
	{"y_80fh", []XspAtom{{"y", 0x80f}}, 1, []uint32{0x10000200}},
	{"y_0fh", []XspAtom{{"y", 0xf}}, 2, []uint32{0x30001000}},
	{"y_80fh*d_3h", []XspAtom{{"y", 0x80f}, {"d", 0x3}}, 2, []uint32{0x30001000}},
	// Dim-12 class (Co_1 class 2C, invariant-space
	// dimension dim (im(A-1))^+ = 12). It reaches the
	// xsp2co1InvolutionOrthogonal column-cap path that
	// previously read out of bounds; it is a 2B
	// involution, so I=2 and the standard rep is z.
	{"y_0ae0h*d_20h", []XspAtom{{"y", 0xae0}, {"d", 0x20}}, 2, []uint32{0x30001000}},
}

// conjInvolStdRep runs the low-level conjugate-to-standard
// form path (xsp2co1ElemConjugateInvolution to obtain the
// conjugating monster word, then xsp2co1ConjugateElem to
// apply it), returning the class indicator I and the
// standard representative g^a. This mirrors mmgroup's
// conjugate_involution_in_Gx0, which conjugates with
// xsp2co1_conjugate_elem rather than re-wrapping the
// (generally non-G_x0) conjugating word.
func conjInvolStdRep(t *testing.T, g *Xsp2Co1) (int, *Xsp2Co1) {
	t.Helper()
	var a [15]uint32
	res := xsp2co1ElemConjugateInvolution(g.data[:], a[:])
	if res < 0 {
		t.Fatalf("xsp2co1ElemConjugateInvolution failed: %d", res)
	}
	length := int(res & 0xff)
	out := &Xsp2Co1{}
	xsp2co1CopyElem(g.data[:], out.data[:])
	if err := xsp2co1ConjugateElem(out.data[:], a[:length]); err != nil {
		t.Fatalf("xsp2co1ConjugateElem failed: %v", err)
	}
	return int(res >> 8), out
}

// TestXsp2Co1ConjugateInvolution mirrors test_std_rep for
// the involution classes whose invariant space has
// dimension <= 12. It checks that the conjugate-to-standard
// form path maps each sample, and every random G_x0
// conjugate of it, to the same fixed representative of its
// class with the same class indicator I. This exercises
// xsp2co1ElemConjugateInvolution, xsp2co1ElemConjGx0ToQx0,
// xsp2co1ElemFindType4, xsp2co1InvolutionFindType4,
// xsp2co1InvolutionOrthogonal (dims 9 and 12),
// xsp2co1ConjugateElem and genLeech2ReduceType2/Type4. The
// dim-12 sample y_0ae0h*d_20h exercises the
// xsp2co1InvolutionOrthogonal column-cap path (see the
// equivalence argument and C-UB note at that function).
func TestXsp2Co1ConjugateInvolution(t *testing.T) {
	t.Parallel()
	for _, s := range stdInvolReps {
		g := NewXsp2Co1(s.atoms...)
		stdWant := NewXsp2Co1(atomsFromWord(s.std)...)
		i0, std0 := conjInvolStdRep(t, g)
		if i0 != s.i {
			t.Errorf("%s: I=%d want %d", s.name, i0, s.i)
		}
		if !std0.Equal(stdWant) {
			t.Errorf("%s: standard rep=%v want %v", s.name, std0.Mmdata(), s.std)
		}
		// Every random G_x0 conjugate reduces to the same
		// standard representative of the class.
		for i := 0; i < 20; i++ {
			c, ok := xspFromMM(MMRandIn(SubGx0))
			if !ok {
				continue
			}
			gc := c.Inv().Mul(g).Mul(c)
			ii, stdc := conjInvolStdRep(t, gc)
			if ii != s.i {
				t.Fatalf("%s conjugate %d: I=%d want %d", s.name, i, ii, s.i)
			}
			if !stdc.Equal(stdWant) {
				t.Fatalf("%s conjugate %d: standard rep=%v want %v", s.name, i, stdc.Mmdata(), s.std)
			}
		}
	}
}

// stdRepMM returns the monster standard representative z
// that ConjugateInvolutionType maps an involution of type
// it to: identity for it=0, the 2A involution x_{2,3} =
// d_200h for it=1, and the central 2B involution x_1000h
// for it=2. These match the oracle
// MM.conjugate_involution (h^-1 g h).
func stdRepMM(t *testing.T, it int) *MM {
	t.Helper()
	switch it {
	case 0:
		return MMIdentity()
	case 1:
		return MMGen("d", 0x200)
	case 2:
		return MMGen("x", 0x1000)
	default:
		t.Fatalf("unexpected involution type %d", it)
		return nil
	}
}

// publicInvolSamples are the order-<=2 entries of
// involSamples (the order-4 elements are not involutions
// and are excluded) with the involution type returned by
// the public ConjugateInvolutionType wrapper. it=1 are the
// 2A classes, it=2 the 2B classes, it=0 the identity.
var publicInvolSamples = []struct {
	name  string
	atoms []XspAtom
	it    int
}{
	{"1", nil, 0},
	{"x_1000h", []XspAtom{{"x", 0x1000}}, 2},
	{"x_80fh", []XspAtom{{"x", 0x80f}}, 1},
	{"x_800h", []XspAtom{{"x", 0x800}}, 2},
	{"y_80fh", []XspAtom{{"y", 0x80f}}, 1},
	{"y_0fh", []XspAtom{{"y", 0xf}}, 2},
	{"y_80fh*d_3h", []XspAtom{{"y", 0x80f}, {"d", 0x3}}, 2},
	{"y_0ae0h*d_20h", []XspAtom{{"y", 0xae0}, {"d", 0x20}}, 2},
}

// TestConjugateInvolutionTypeSamples covers the public
// ConjugateInvolutionType wrapper across the full
// involution-class matrix (and, through it,
// Xsp2Co1.ConjugateInvolution after the bug-2 retype to
// return a monster *MM conjugator). The existing
// TestConjugateInvolutionType in misc_test.go only
// exercises the central 2B element x_1000h conjugated by
// G_x0 atoms (it=2 with a G_x0 conjugator); this test adds
// the 2A classes (it=1), the other 2B classes, and the
// triality-conjugator regression. Every sample is a G_x0
// element, so the fast checkInGx0 path is taken and the
// slow trial loop is never entered.
//
// For each sample it checks the returned involution type
// against the oracle-derived expected value and verifies
// h^-1 g h = z in the monster, where z is the standard
// representative for that type. It pins x_800h to it=2
// with a single triality (t) atom conjugator, which is
// exactly the case that panicked before the fix (the
// conjugator is not in G_x0, so the old wrapper's
// xsp2co1SetElemWord re-wrap rejected it). It is
// deliberately not Short-gated: the fast path is cheap.
func TestConjugateInvolutionTypeSamples(t *testing.T) {
	t.Parallel()
	for _, s := range publicInvolSamples {
		gx := NewXsp2Co1(s.atoms...)
		g := &MM{data: gx.Mmdata()}
		it, h := ConjugateInvolutionType(g)
		if it != s.it {
			t.Errorf("%s: it=%d want %d", s.name, it, s.it)
		}
		z := stdRepMM(t, it)
		if got := h.Inv().Mul(g).Mul(h); !got.Equal(z) {
			t.Errorf("%s: h^-1 g h = %v want %v", s.name, got, z)
		}
	}
	// x_800h is the canonical regression case: it=2 with a
	// single triality-atom conjugator (the old wrapper
	// panicked re-wrapping it as Xsp2Co1).
	g := &MM{data: NewXsp2Co1(XspAtom{"x", 0x800}).Mmdata()}
	it, h := ConjugateInvolutionType(g)
	if it != 2 {
		t.Errorf("x_800h: it=%d want 2", it)
	}
	w := h.Mmdata()
	if len(w) != 1 || (w[0]>>28)&7 != 5 {
		t.Errorf("x_800h: conjugator=%v want a single triality (t) atom", w)
	}
}

// g2ReproducerWord is the reduced word of the 2B
// involution from T5_BUG_REPORT.md that exposed both G9
// bugs. It is not in G_x0 (despite the report's disproven
// premise to the contrary: g2*z has order 66, which is
// incompatible with z being central in a common G_x0), so
// conjugateInvolution must route it through the slow trial
// loop. The oracle's mm_conjugate_involution(g2,
// check=False, ntrials=1) returns it=2.
const g2ReproducerWord = "M<y_580h*x_105fh*d_0ea4h*p_207694242*l_2*p_2956800*l_1*p_12998960*l_1*t_1*l_2*p_2597760*l_1*p_12571672*l_1*t_1*l_2*p_1858560*l_1*p_3216*l_2*p_1127040*t_2*l_2*p_2956800*l_1*p_53382101*t_1*l_2*p_2344320*l_2*p_151120*t_1*l_1*p_1415040*l_1*p_10708992*t_2*l_2*p_2956800*l_1*p_64016138>"

// TestConjugateInvolutionReproducer is the G9 acceptance
// case: the T5_BUG_REPORT.md reproducer, a 2B involution
// outside G_x0, must drive conjugateInvolution's slow trial
// loop to it=2 with h^-1 g2 h = z (z = x_1000h). A single
// trial costs one HalfOrder (~10s), so the test is gated
// behind testing.Short per Q-s and skipped by go test
// -short.
func TestConjugateInvolutionReproducer(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: one HalfOrder per trial; run without -short")
	}
	t.Parallel()
	g2 := mustMM(t, g2ReproducerWord)
	g2.Reduce()
	if !g2.Mul(g2).Equal(MMIdentity()) {
		t.Fatalf("reproducer is not an involution")
	}
	if g2.InGx0() {
		t.Fatalf("reproducer unexpectedly in G_x0; should exercise the trial loop")
	}
	it, h, ok := conjugateInvolution(g2, false, 1)
	if !ok {
		t.Fatalf("conjugateInvolution(g2, false, 1) failed to find a conjugator")
	}
	if it != 2 {
		t.Errorf("it=%d want 2", it)
	}
	z := MMGen("x", 0x1000)
	if got := h.Inv().Mul(g2).Mul(h); !got.Equal(z) {
		t.Errorf("h^-1 g2 h = %v want %v", got, z)
	}
}

// TestChiPowers mirrors test_chi_powers for involution
// classes that the conjugate-to-standard path handles. It
// checks that ChiPowers returns the element's order, that
// the character map is keyed exactly by the divisors of
// the order, that the sqrt(-1) character distinguishes 2A
// (4371) from 2B (275), and that the returned h is a valid
// monster element. The standard 2A (d_[2,3]) and 2B
// (x_1000h) involutions and the identity are used.
func TestChiPowers(t *testing.T) {
	t.Parallel()
	type tc struct {
		name      string
		g         *MM
		order     int
		chi       ChiMap
		chiSqrtM1 int // chi at order/2 for even order; ignored if order odd
	}
	cases := []tc{
		{"identity", MMIdentity(), 1, ChiMap{1: 196883}, 0},
		{"x_1000h (2B)", MMGen("x", 0x1000), 2, ChiMap{1: 275, 2: 196883}, 275},
		{"d_[2,3] (2A)", MMGen("d", 0xc), 2, ChiMap{1: 4371, 2: 196883}, 4371},
	}
	for _, c := range cases {
		order, chi, h := c.g.ChiPowers(0, 100)
		if order != c.order {
			t.Errorf("%s: order=%d want %d", c.name, order, c.order)
		}
		if order != c.g.Order() {
			t.Errorf("%s: ChiPowers order=%d != Order()=%d", c.name, order, c.g.Order())
		}
		if !chiMapEq(chi, c.chi) {
			t.Errorf("%s: chi=%v want %v", c.name, chi, c.chi)
		}
		// Keys must be exactly the divisors of the order.
		if !sameKeys(chi, divisorsOf(order)) {
			t.Errorf("%s: chi keys=%v want divisors of %d", c.name, keysOf(chi), order)
		}
		if c.order%2 == 0 {
			got, ok := chi[order/2]
			if !ok || got != c.chiSqrtM1 {
				t.Errorf("%s: chi[order/2]=%d (ok=%v) want %d", c.name, got, ok, c.chiSqrtM1)
			}
			// The sqrt(-1) character is one of the involution
			// characters 275 (2B) or 4371 (2A), per test_chi_powers.
			if got != 275 && got != 4371 {
				t.Errorf("%s: chi[order/2]=%d not an involution character", c.name, got)
			}
		}
		if h == nil {
			t.Errorf("%s: ChiPowers returned nil h", c.name)
		} else if h.Order() <= 0 {
			t.Errorf("%s: ChiPowers h has non-positive order %d", c.name, h.Order())
		}
	}
}

func chiMapEq(a, b ChiMap) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func divisorsOf(n int) []int {
	var d []int
	for i := 1; i <= n; i++ {
		if n%i == 0 {
			d = append(d, i)
		}
	}
	return d
}

func keysOf(m ChiMap) []int {
	var k []int
	for key := range m {
		k = append(k, key)
	}
	return k
}

func sameKeys(m ChiMap, keys []int) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

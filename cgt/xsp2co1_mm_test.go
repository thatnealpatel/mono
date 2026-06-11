package cgt

import (
	"testing"

	"patel.codes/cgt/xsp2co1"
)

// mm-level tests for the public involution wrappers
// over G_{x0}, grounded in the canonical mmgroup tests
//   mmgroup/tests/test_involutions/test_involution_Gx0.py
//     (test_std_rep)
//   mmgroup/tests/test_orders/test_order.py
//     (test_chi_powers)
//
// These exercise the *MM-returning surface
// (ConjugateInvolutionType, the slow trial-loop
// reproducer, and ChiPowers), which is the mm-side
// resolution of Q-r: xsp2co1.Xsp2Co1.ConjugateInvolution
// returns an xsp2co1.Word; flat cgt wraps it as *MM. The
// white-box tests of the algorithm itself live in
// package xsp2co1.

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

// publicInvolSamples are the order-<=2 involution-class
// representatives with the involution type returned by
// the public ConjugateInvolutionType wrapper. it=1 are
// the 2A classes, it=2 the 2B classes, it=0 the identity.
var publicInvolSamples = []struct {
	name  string
	atoms []xsp2co1.XspAtom
	it    int
}{
	{"1", nil, 0},
	{"x_1000h", []xsp2co1.XspAtom{{Tag: "x", I: 0x1000}}, 2},
	{"x_80fh", []xsp2co1.XspAtom{{Tag: "x", I: 0x80f}}, 1},
	{"x_800h", []xsp2co1.XspAtom{{Tag: "x", I: 0x800}}, 2},
	{"y_80fh", []xsp2co1.XspAtom{{Tag: "y", I: 0x80f}}, 1},
	{"y_0fh", []xsp2co1.XspAtom{{Tag: "y", I: 0xf}}, 2},
	{"y_80fh*d_3h", []xsp2co1.XspAtom{{Tag: "y", I: 0x80f}, {Tag: "d", I: 0x3}}, 2},
	{"y_0ae0h*d_20h", []xsp2co1.XspAtom{{Tag: "y", I: 0xae0}, {Tag: "d", I: 0x20}}, 2},
}

// TestConjugateInvolutionTypeSamples covers the public
// ConjugateInvolutionType wrapper across the full
// involution-class matrix (and, through it,
// Xsp2Co1.ConjugateInvolution after the bug-2 retype to
// return a monster word). The existing
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
		gx := xsp2co1.NewXsp2Co1(s.atoms...)
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
	g := &MM{data: xsp2co1.NewXsp2Co1(xsp2co1.XspAtom{Tag: "x", I: 0x800}).Mmdata()}
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

package xsp2co1

import (
	"math/rand/v2"
	"testing"

	"patel.codes/cgt/mat24"
)

// White-box tests for the involution machinery of
// G_{x0} (xsp2_involution.go), grounded in the
// canonical mmgroup tests
//   mmgroup/tests/test_involutions/test_display_characters.py
//     (test_xsp2co1_elem_involution_class)
//   mmgroup/tests/test_involutions/test_involution_Gx0.py
//     (test_std_rep)
//   mmgroup/tests/test_clifford/test_xsp2_conjugate.py
//     (test_xsp2_conjugate)
//
// These tests reach unexported internals (the elem
// buffer g.data, xsp2co1ConjElem, the involution
// invariant/conjugation helpers) and therefore live in
// package xsp2co1. The mm-level wrapper tests
// (ConjugateInvolutionType, the slow reproducer, and
// ChiPowers) live in flat cgt, where *MM exists.
//
// The involution samples below are the order-<=2
// representatives of the classes of square roots of
// Q_{x0} in G_{x0}, taken from
//   mmgroup/tests/test_involutions/involution_samples.py
// together with two order-4 samples (x_0ae0h, y_0ae0h)
// used by the class test. Expected involution-class
// numbers and standard representatives were obtained
// from the C reference via `goof mmgroup.py`.

// randGx0Elem returns a random element of G_{x0},
// built as a short product of random d, p, x, y and l
// generator atoms. It replaces the former
// xspFromMM(MMRandIn(SubGx0)) helper: the involution
// invariance tests only need varied G_x0 elements (any
// word in the G_x0 generators is one), and building it
// here from XspAtoms keeps xsp2co1 free of an upward
// mm import.
func randGx0Elem() *Xsp2Co1 {
	n := 4 + rand.IntN(4)
	atoms := make([]XspAtom, 0, n)
	for i := 0; i < n; i++ {
		switch rand.IntN(5) {
		case 0:
			atoms = append(atoms, XspAtom{"d", rand.IntN(0x1000)})
		case 1:
			atoms = append(atoms, XspAtom{"p", int(mat24.M24numRandLocal(0, rand.Uint32()))})
		case 2:
			atoms = append(atoms, XspAtom{"x", rand.IntN(0x2000)})
		case 3:
			atoms = append(atoms, XspAtom{"y", rand.IntN(0x2000)})
		case 4:
			atoms = append(atoms, XspAtom{"l", 1 + rand.IntN(2)})
		}
	}
	return NewXsp2Co1(atoms...)
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
			c := randGx0Elem()
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
		gs = append(gs, randGx0Elem())
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

// stdInvolReps is the I value and standard representative
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
			c := randGx0Elem()
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

// atomsFromWord converts a reduced G_x0 monster word
// (tags d, p, x, y, l only) to the XspAtom list that
// NewXsp2Co1 consumes. It is the package-local inverse
// of XspAtom encoding used by the standard-rep test.
func atomsFromWord(w []uint32) []XspAtom {
	letters := map[uint32]string{1: "d", 2: "p", 3: "x", 4: "y", 5: "t", 6: "l"}
	out := make([]XspAtom, 0, len(w))
	for _, a := range w {
		letter, ok := letters[(a>>28)&7]
		if !ok {
			continue
		}
		sign := ""
		if a&0x80000000 != 0 {
			sign = "-"
		}
		out = append(out, XspAtom{Tag: sign + letter, I: int(a & 0xfffffff)})
	}
	return out
}

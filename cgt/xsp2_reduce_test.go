package cgt

import "testing"

// Tests for the type-2 Leech-lattice-mod-2 reducer
// (xsp2_reduce.go), grounded in the canonical mmgroup
// tests
//   mmgroup/tests/test_gen_xi/test_reduce.py
//     (test_reduce_type_2, test_reduce_type_2_ortho)
//   mmgroup/tests/test_gen_xi/test_start_reduce.py
//     (test_start_type24)
//
// Each table entry is a concrete vector together with
// the reduce word and result obtained from the C
// reference (gen_leech2_reduce_type2,
// gen_leech2_reduce_type2_ortho, gen_leech2_start_type24)
// via `goof mmgroup.py`. We assert both that Go's word
// is byte-identical to the C word and that it satisfies
// the mathematical invariant the mmgroup tests check
// (gen_leech2_op_word maps v to the standard vector).

const (
	leechBeta      = 0x200    // standard type-2 vector beta = e_2 - e_3
	leechBetaOrtho = 0x800200 // e_2 + e_3 (image of an orthogonal type-2 vector)
)

func u32SliceEq(a, b []uint32) bool {
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

// TestGenLeech2ReduceType2 mirrors test_reduce_type_2.
// For each type-2 vector v of subtype 0x20, 0x21 or
// 0x22, gen_leech2_reduce_type2 produces a word g with
// v*g == beta (mod the sign bit 0x1000000), which is the
// assertion `w & 0xffffff == BETA` in the mmgroup test.
func TestGenLeech2ReduceType2(t *testing.T) {
	t.Parallel()
	type tc struct {
		v       uint32
		subtype uint32
		word    []uint32
		w       uint32
	}
	// Vectors from test_reduce.py type2_testdata: the four
	// explicit vectors, plus oracle-sampled vectors of each
	// short subtype (0x20, 0x21, 0x22) exercising the
	// permutation branches.
	cases := []tc{
		{0x200, 0x20, []uint32{}, 0x200},
		{0x1000200, 0x20, []uint32{}, 0x1000200},
		{0x800200, 0x20, []uint32{0xc0000200}, 0x1000200},
		{0x1800200, 0x20, []uint32{0xc0000200}, 0x200},
		{0xe48b3, 0x21, []uint32{0xe0000001, 0xa0154500, 0xe0000001, 0xa0a2a080, 0xc0000200}, 0x1000200},
		{0xe5f558, 0x22, []uint32{0xa14ce680, 0xe0000002, 0xa0002580, 0xc0000200}, 0x200},
		{0x420217, 0x22, []uint32{0xa01c5c00, 0xe0000002, 0xa0007080}, 0x200},
		{0x414eef, 0x21, []uint32{0xe0000002, 0xa0a2be80, 0xe0000002, 0xa0003c00, 0xc0000200}, 0x200},
		{0xb35470, 0x22, []uint32{0xa0a46280, 0xe0000002, 0xa0a2af80}, 0x200},
		{0xa36628, 0x22, []uint32{0xa0ab7200, 0xe0000001, 0xa0a2a440}, 0x200},
		{0xd0ec01, 0x21, []uint32{0xe0000002, 0xa0071700, 0xe0000002, 0xa0a27ec0}, 0x1000200},
		{0xb55f9a, 0x21, []uint32{0xe0000001, 0xa1e88e00, 0xe0000001, 0xa006edc0, 0xc0000200}, 0x1000200},
		{0x8002f4, 0x20, []uint32{0xa0012840, 0xc0000200}, 0x1000200},
		{0x31a, 0x20, []uint32{0xa003ad40}, 0x200},
		{0xb6, 0x20, []uint32{0xa003e580}, 0x200},
		{0x800481, 0x20, []uint32{0xa000ff00, 0xc0000200}, 0x1000200},
	}
	for _, c := range cases {
		if got := Leech2Subtype(c.v); got != c.subtype {
			t.Errorf("Leech2Subtype(%#x)=%#x want %#x", c.v, got, c.subtype)
		}
		var a [6]uint32
		l := genLeech2ReduceType2(c.v, a[:])
		if l < 0 {
			t.Errorf("genLeech2ReduceType2(%#x) failed, l=%d", c.v, l)
			continue
		}
		got := a[:l]
		if !u32SliceEq(got, c.word) {
			t.Errorf("genLeech2ReduceType2(%#x) word=%v want %v", c.v, got, c.word)
		}
		w := Leech2OpWord(c.v, got)
		if w != c.w {
			t.Errorf("Leech2OpWord(%#x, reduceWord)=%#x want %#x", c.v, w, c.w)
		}
		// Mathematical invariant from the mmgroup test:
		// the reduced vector equals beta up to the sign bit.
		if w&0xffffff != leechBeta {
			t.Errorf("genLeech2ReduceType2(%#x): reduced to %#x, want beta=%#x", c.v, w&0xffffff, leechBeta)
		}
	}
}

// TestGenLeech2ReduceType2Ortho mirrors
// test_reduce_type_2_ortho. For each type-2 vector v
// orthogonal to beta (i.e. v^beta is of type 4),
// gen_leech2_reduce_type2_ortho produces a word g with
// v*g == e_2+e_3 (mod the sign bit) and beta*g == beta,
// i.e. g fixes beta. These are the two assertions
// `w & 0xffffff == 0x800200` and `b == BETA` in mmgroup.
func TestGenLeech2ReduceType2Ortho(t *testing.T) {
	t.Parallel()
	type tc struct {
		v       uint32
		subtype uint32
		word    []uint32
		w       uint32
		b       uint32
	}
	cases := []tc{
		{0x800200, 0x20, []uint32{}, 0x800200, 0x200},
		{0x1800200, 0x20, []uint32{}, 0x1800200, 0x200},
		{0x982f65, 0x21, []uint32{0xe0000001, 0xad45f830, 0xe0000001, 0xa42dec00, 0xe0000002}, 0x1800200, 0x200},
		{0x448fbe, 0x21, []uint32{0xe0000002, 0xa0b70c00, 0xe0000002, 0xa03d3b00, 0xe0000001}, 0x800200, 0x200},
		{0x983dc6, 0x21, []uint32{0xe0000001, 0xa0222900, 0xe0000001, 0xa03d3b00, 0xe0000001}, 0x800200, 0x200},
		{0xcd5ffc, 0x21, []uint32{0xe0000002, 0xa9994680, 0xe0000002, 0xa38b8000, 0xe0000002}, 0x800200, 0x200},
		{0x9822b7, 0x22, []uint32{0xa3c1a400, 0xe0000001, 0xa412da00, 0xe0000001}, 0x1800200, 0x200},
		{0xca6118, 0x22, []uint32{0xad1d5d00, 0xe0000001, 0xa3990900, 0xe0000002}, 0x800200, 0x200},
		{0x94b79f, 0x22, []uint32{0xa28a5500, 0xe0000002, 0xa07a2380, 0xe0000001}, 0x800200, 0x200},
		{0x18523f, 0x22, []uint32{0xa4641000, 0xe0000001, 0xa4c2cf00, 0xe0000001}, 0x1800200, 0x200},
		{0x7b7, 0x20, []uint32{0xa130dd00, 0xe0000001}, 0x800200, 0x200},
		{0x15d, 0x20, []uint32{0xa0584d00, 0xe0000001}, 0x800200, 0x200},
		{0x8000d1, 0x20, []uint32{0xa47f2200, 0xe0000002}, 0x1800200, 0x200},
		{0x255, 0x20, []uint32{0xa5f35980, 0xe0000001}, 0x800200, 0x200},
	}
	for _, c := range cases {
		if Leech2Type(c.v^leechBeta) != 4 {
			t.Errorf("vector %#x is not orthogonal to beta (v^beta has type %d)", c.v, Leech2Type(c.v^leechBeta))
		}
		if got := Leech2Subtype(c.v); got != c.subtype {
			t.Errorf("Leech2Subtype(%#x)=%#x want %#x", c.v, got, c.subtype)
		}
		var a [6]uint32
		l := genLeech2ReduceType2Ortho(c.v, a[:])
		if l < 0 {
			t.Errorf("genLeech2ReduceType2Ortho(%#x) failed, l=%d", c.v, l)
			continue
		}
		got := a[:l]
		if !u32SliceEq(got, c.word) {
			t.Errorf("genLeech2ReduceType2Ortho(%#x) word=%v want %v", c.v, got, c.word)
		}
		w := Leech2OpWord(c.v, got)
		if w != c.w {
			t.Errorf("Leech2OpWord(%#x, reduceWord)=%#x want %#x", c.v, w, c.w)
		}
		if w&0xffffff != leechBetaOrtho {
			t.Errorf("genLeech2ReduceType2Ortho(%#x): reduced to %#x, want %#x", c.v, w&0xffffff, leechBetaOrtho)
		}
		// The word must fix beta.
		b := Leech2OpWord(leechBeta, got)
		if b != c.b {
			t.Errorf("Leech2OpWord(beta, reduceWord)=%#x want %#x (word must fix beta)", b, c.b)
		}
	}
}

// TestGenLeech2StartType24 mirrors test_start_type24.
// gen_leech2_start_type24 returns the subtype of a
// type-2 vector v provided v+beta is of type 4 (0 in
// the special case v=beta+Omega), and a negative value
// for illegal input. The cases cover the explicit
// vectors of test_start_reduce.py plus single-bit and
// complemented single-bit vectors that exercise the
// negative-return branches.
func TestGenLeech2StartType24(t *testing.T) {
	t.Parallel()
	type tc struct {
		v    uint32
		want int32
	}
	cases := []tc{
		{0x0, -1},
		{0x800, 33},
		{0xc, 32},
		{0x200, -1},
		{0x800200, 0},
		{0x1, 32},
		{0xfffffe, -1},
		{0x2, 32},
		{0xfffffd, -1},
		{0x4, 32},
		{0xfffffb, -1},
		{0x8, 32},
		{0xfffff7, -1},
		{0x10, 32},
		{0xffffef, -1},
		{0x20, 32},
		{0xffffdf, -1},
		{0x40, 32},
		{0xffffbf, -1},
		{0x80, 32},
		{0xffff7f, -1},
		{0x100, -1},
		{0xfffeff, -1},
		{0x400, -1},
		{0xfffbff, -1},
		{0x1000, -1},
		{0xffefff, -1},
		{0x2000, -1},
		{0xffdfff, -1},
		{0x4000, -1},
		{0xffbfff, -1},
		{0x8000, -1},
		{0xff7fff, -1},
		{0x10000, 34},
		{0xfeffff, -1},
		{0x20000, 34},
		{0xfdffff, -1},
		{0x40000, 34},
		{0xfbffff, -1},
		{0x80000, 34},
		{0xf7ffff, -1},
		{0x100000, -1},
		{0xefffff, -1},
		{0x200000, -1},
		{0xdfffff, -1},
		{0x400000, 34},
		{0xbfffff, -1},
		{0x800000, -1},
		{0x7fffff, -1},
	}
	for _, c := range cases {
		if got := genLeech2StartType24(c.v); got != c.want {
			t.Errorf("genLeech2StartType24(%#x)=%d want %d", c.v, got, c.want)
		}
	}
}

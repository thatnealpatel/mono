package monster

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
	"patel.codes/cgt/leech"
	"patel.codes/cgt/xsp2co1"
)

func oracleLeech(t *testing.T, setup, expr string) []int64 {
	t.Helper()
	script := fmt.Sprintf("import json,mmgroup,numpy as np\n%s\nprint(json.dumps(%s))", setup, expr)
	out, err := oraclepkg.Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	var v []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("oracleLeech(%q): %v", expr, err)
	}
	return v
}

func u32List(v []uint32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%d", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func u64Eq(a []uint64, b []int64) bool {
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

func u32Eq(a []uint32, b []int64) bool {
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

func xspPy(atoms []xsp2co1.XspAtom) string {
	parts := make([]string, len(atoms))
	for i, a := range atoms {
		parts[i] = fmt.Sprintf("(%q,%d)", a.Tag, a.I)
	}
	return "mmgroup.Xsp2_Co1([" + strings.Join(parts, ",") + "])"
}

func TestXLeech2Ord(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 0x800000, 0x1000, 0x800001, 0x1fffff, 0x3ffffff} {
		got := leech.NewXLeech2FromInt(v).Ord()
		want := oracleUint(t, fmt.Sprintf("mmgroup.XLeech2(%d).ord", v))
		if uint64(got) != want {
			t.Errorf("XLeech2(%#x).Ord()=%#x want %#x", v, got, want)
		}
	}
}

func TestXLeech2Type(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 0x800000, 0x800800, 0x1000, 0x200} {
		x := leech.NewXLeech2FromInt(v)
		gotT := x.Type()
		wantT := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_type(%d)", v))
		if uint64(gotT) != wantT {
			t.Errorf("XLeech2(%#x).Type()=%#x want %#x", v, gotT, wantT)
		}
		gotS := x.Subtype()
		wantS := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_subtype(%d)", v))
		if uint64(gotS) != wantS {
			t.Errorf("XLeech2(%#x).Subtype()=%#x want %#x", v, gotS, wantS)
		}
	}
}

func bytesEq(a []byte, b []int64) bool {
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

func TestXLeech2Bitvector(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{1, 0x800000, 0x123456, 0x1000, 0xabcdef, 0x1800001} {
		got := leech.NewXLeech2FromInt(v).Bitvector()
		want := oracleInts(t, fmt.Sprintf("[int(x) for x in mmgroup.XLeech2(%d).as_Leech2_bitvector()]", v))
		if !bytesEq(got, want) {
			t.Errorf("XLeech2(%#x).Bitvector()=%v want %v", v, got, want)
		}
	}
}

func TestLeech2Scalprod(t *testing.T) {
	t.Parallel()
	pairs := [][2]uint32{{1, 0x1000}, {0x800001, 0x100}, {0x1000, 0x1000}, {0x123, 0x456000}, {0x800000, 0x800800}}
	for _, p := range pairs {
		got := leech.Leech2Scalprod(p[0], p[1])
		want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_scalprod(%d,%d)", p[0], p[1]))
		if uint64(got) != want {
			t.Errorf("Leech2Scalprod(%#x,%#x)=%d want %d", p[0], p[1], got, want)
		}
	}
}

func TestLeechMod3Short(t *testing.T) {
	t.Parallel()
	for _, x2 := range []uint32{0x10001, 0x10020, 0x10022, 0x20000, 0x30000} {
		x3 := leech.Leech2To3Short(x2)
		wantX3 := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2to3_short(%d)", x2))
		if uint64(x3) != wantX3 {
			t.Errorf("Leech2To3Short(%#x)=%#x want %#x", x2, x3, wantX3)
		}
		back := leech.Leech3To2Short(x3)
		wantBack := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech3to2_short(%d)", wantX3))
		if uint64(back) != wantBack {
			t.Errorf("Leech3To2Short(%#x)=%#x want %#x", x3, back, wantBack)
		}
	}
}

const leechBasis = "from mmgroup.clifford12 import leech2_matrix_basis, leech2_matrix_radical\n" +
	"def basis(v2):\n" +
	" a=np.array(v2,dtype=np.uint32); o=np.zeros(24,dtype=np.uint64); k=int(leech2_matrix_basis(a,len(a),o,24)); return [int(x) for x in o[:k]]\n" +
	"def radical(v2):\n" +
	" a=np.array(v2,dtype=np.uint32); o=np.zeros(24,dtype=np.uint64); k=int(leech2_matrix_radical(a,len(a),o,24)); return [int(x) for x in o[:k]]"

func TestLeech2MatrixBasis(t *testing.T) {
	t.Parallel()
	cases := [][]uint32{
		{1, 2, 4},
		{0x800000, 0x1000, 0x200, 0x800000},
		{0x10001, 0x20002, 0x40004, 0x80008},
		{1, 1, 1, 2, 4},
	}
	for _, v2 := range cases {
		want := oracleLeech(t, leechBasis, fmt.Sprintf("basis(%s)", u32List(v2)))
		if got := leech.Leech2MatrixBasis(v2); !u64Eq(got, want) {
			t.Errorf("Leech2MatrixBasis(%v)=%v want %v", v2, got, want)
		}
	}
}

func TestLeech2MatrixRadical(t *testing.T) {
	t.Parallel()
	cases := [][]uint32{
		{1, 2, 4},
		{0x800000, 0x1000, 0x200},
		{0x10001, 0x20002, 0x40004, 0x80008, 0x100010},
		{0x123, 0x456, 0x789, 0xabc},
	}
	for _, v2 := range cases {
		want := oracleLeech(t, leechBasis, fmt.Sprintf("radical(%s)", u32List(v2)))
		if got := leech.Leech2MatrixRadical(v2); !u64Eq(got, want) {
			t.Errorf("Leech2MatrixRadical(%v)=%v want %v", v2, got, want)
		}
	}
}

func TestXsp2AsXsp(t *testing.T) {
	t.Parallel()
	cases := [][]xsp2co1.XspAtom{
		{{Tag: "x", I: 1}},
		{{Tag: "d", I: 0x456}},
		{{Tag: "x", I: 0x1abc}, {Tag: "d", I: 0x555}},
		{{Tag: "x", I: 1}, {Tag: "d", I: 1}},
	}
	for _, atoms := range cases {
		got := xsp2co1.NewXsp2Co1(atoms...).AsXsp()
		want := oracleUint(t, xspPy(atoms)+".as_xsp()")
		if uint64(got) != want {
			t.Errorf("Xsp2Co1(%v).AsXsp()=%#x want %#x", atoms, got, want)
		}
	}
}

func TestXsp2Order(t *testing.T) {
	t.Parallel()
	cases := [][]xsp2co1.XspAtom{
		{{Tag: "x", I: 0x1abc}, {Tag: "y", I: 0x3}, {Tag: "d", I: 0x4}},
		{{Tag: "l", I: 1}},
		{{Tag: "l", I: 2}},
		{{Tag: "p", I: 187654344}},
		{{Tag: "d", I: 0xd79}, {Tag: "p", I: 205334671}},
	}
	for _, atoms := range cases {
		got := int64(xsp2co1.NewXsp2Co1(atoms...).Order())
		want := oracleInt(t, xspPy(atoms)+".order()")
		if got != want {
			t.Errorf("Xsp2Co1(%v).Order()=%d want %d", atoms, got, want)
		}
	}
}

func TestXsp2XspConjugate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		atoms []xsp2co1.XspAtom
		v     []uint32
	}{
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, []uint32{1, 2, 4, 0x800000}},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 2}}, []uint32{1, 0x1000, 0x100}},
		{[]xsp2co1.XspAtom{{Tag: "d", I: 0x124}}, []uint32{0x1f24, 0x555, 0x1abc}},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}, []uint32{1, 0x800001, 0x123456}},
	}
	for _, c := range cases {
		got := xsp2co1.NewXsp2Co1(c.atoms...).XspConjugate(c.v)
		want := oracleInts(t, xspPy(c.atoms)+".xsp_conjugate("+u32List(c.v)+")")
		if !u32Eq(got, want) {
			t.Errorf("Xsp2Co1(%v).XspConjugate(%v)=%v want %v", c.atoms, c.v, got, want)
		}
	}
}

func TestXsp2FromXspRoundTrip(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{1, 0x1000, 0x800001, 0x123, 0x1abc000} {
		got := xsp2co1.Xsp2FromXsp(x).AsXsp()
		want := oracleUint(t, fmt.Sprintf("mmgroup.Xsp2_Co1.group.from_xsp(%d).as_xsp()", x))
		if uint64(got) != want {
			t.Errorf("xsp2co1.Xsp2FromXsp(%#x).AsXsp()=%#x want %#x", x, got, want)
		}
	}
}

func TestXsp2Mul(t *testing.T) {
	t.Parallel()
	cases := []struct{ a, b []xsp2co1.XspAtom }{
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1abc}}, []xsp2co1.XspAtom{{Tag: "d", I: 0x555}}},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, []xsp2co1.XspAtom{{Tag: "l", I: 2}}},
		{[]xsp2co1.XspAtom{{Tag: "d", I: 0x124}}, []xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}},
	}
	for _, c := range cases {
		ga := xsp2co1.NewXsp2Co1(c.a...)
		gb := xsp2co1.NewXsp2Co1(c.b...)
		gotOrd := int64(ga.Mul(gb).Order())
		wantOrd := oracleInt(t, fmt.Sprintf("(%s * %s).order()", xspPy(c.a), xspPy(c.b)))
		if gotOrd != wantOrd {
			t.Errorf("Xsp2Mul(%v,%v).Order()=%d want %d", c.a, c.b, gotOrd, wantOrd)
		}
		if !ga.Mul(gb).Mul(gb.Inv()).Equal(ga) {
			t.Errorf("Xsp2 (a*b)*b^-1 != a for %v, %v", c.a, c.b)
		}
		if !ga.Mul(ga.Inv()).Equal(xsp2co1.Xsp2Co1Identity()) {
			t.Errorf("Xsp2 a*a^-1 != 1 for %v", c.a)
		}
	}
}

func TestLeech2OpWord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		atoms []xsp2co1.XspAtom
		x     uint32
	}{
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 0x800000},
		{[]xsp2co1.XspAtom{{Tag: "d", I: 0x124}}, 0x1f24},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}, 0x123456},
		{[]xsp2co1.XspAtom{{Tag: "y", I: 0x1d79}}, 1},
	}
	for _, c := range cases {
		g := xsp2co1.NewXsp2Co1(c.atoms...)
		got := leech.Leech2OpWord(c.x, g.Mmdata())
		setup := "g=" + xspPy(c.atoms) + ".mmdata"
		want := oracleLeech(t, setup, fmt.Sprintf(
			"[int(mmgroup.generators.gen_leech2_op_word(%d,g,len(g)))]", c.x))
		if int64(got) != want[0] {
			t.Errorf("Leech2OpWord(%#x, %v)=%#x want %#x", c.x, c.atoms, got, want[0])
		}
	}
}

func TestXsp2Pow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		atoms []xsp2co1.XspAtom
		e     int
	}{
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 0},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 1},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 2},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 3},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, -1},
		{[]xsp2co1.XspAtom{{Tag: "l", I: 2}}, 5},
		{[]xsp2co1.XspAtom{{Tag: "d", I: 0x124}}, 2},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}, 3},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}, -2},
	}
	for _, c := range cases {
		g := xsp2co1.NewXsp2Co1(c.atoms...)
		got := g.Pow(c.e)
		wantOrd := oracleInt(t, fmt.Sprintf("(%s**%d).order()", xspPy(c.atoms), c.e))
		if int64(got.Order()) != wantOrd {
			t.Errorf("Xsp2Co1(%v).Pow(%d).Order()=%d want %d", c.atoms, c.e, got.Order(), wantOrd)
		}
		if c.e == 0 {
			if !got.Equal(xsp2co1.Xsp2Co1Identity()) {
				t.Errorf("Xsp2Co1(%v).Pow(0) != identity", c.atoms)
			}
		}
		if c.e == 1 {
			if !got.Equal(g) {
				t.Errorf("Xsp2Co1(%v).Pow(1) != self", c.atoms)
			}
		}
		if c.e >= 2 {
			manual := xsp2co1.Xsp2Co1Identity()
			for i := 0; i < c.e; i++ {
				manual = manual.Mul(g)
			}
			if !got.Equal(manual) {
				t.Errorf("Xsp2Co1(%v).Pow(%d) != manual multiplication", c.atoms, c.e)
			}
		}
		if c.e == -1 {
			if !got.Equal(g.Inv()) {
				t.Errorf("Xsp2Co1(%v).Pow(-1) != Inv()", c.atoms)
			}
		}
	}
}

func TestLeech3OpVectorWord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		atoms []xsp2co1.XspAtom
		x     uint32
	}{
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 0x800000},
		{[]xsp2co1.XspAtom{{Tag: "d", I: 0x124}}, 0x1f24},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}}, 0x123456},
		// Type-2 (short) inputs: gen_leech2to3_short maps these to
		// nonzero mod-3 vectors, so the word actually acts on a
		// short Leech vector rather than the trivial x3=0 case.
		// 0x200, 0x100, 0x10020 are type 2 (oracle-confirmed); the
		// l- and p-generators move them to distinct mod-3 vectors.
		{[]xsp2co1.XspAtom{{Tag: "l", I: 1}}, 0x200},
		{[]xsp2co1.XspAtom{{Tag: "p", I: 187654344}}, 0x100},
		{[]xsp2co1.XspAtom{{Tag: "x", I: 0x1123}, {Tag: "d", I: 0xd79}}, 0x10020},
	}
	for _, c := range cases {
		g := xsp2co1.NewXsp2Co1(c.atoms...)
		x3 := leech.Leech2To3Short(c.x)
		got := leech.Leech3OpVectorWord(x3, g.Mmdata())
		setup := "import numpy as np\ng=" + xspPy(c.atoms) + ".mmdata"
		want := oracleLeech(t, setup, fmt.Sprintf(
			"[int(mmgroup.generators.gen_leech3_op_vector_word(mmgroup.generators.gen_leech2to3_short(%d),g,len(g)))]", c.x))
		if int64(got) != want[0] {
			t.Errorf("Leech3OpVectorWord(%#x, %v)=%#x want %#x", c.x, c.atoms, got, want[0])
		}
	}
}

func TestLeech2Pow(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{1, 0x1000, 0x800001, 0x123456, 0x1f24, 0x100} {
		for _, e := range []uint8{0, 1, 2, 3, 4, 5} {
			got := leech.Leech2Pow(x, e)
			want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_pow(%d,%d)", x, e))
			if uint64(got) != want {
				t.Errorf("Leech2Pow(%#x,%d)=%#x want %#x", x, e, got, want)
			}
		}
	}
}

func TestLeech2OpAtom(t *testing.T) {
	t.Parallel()
	cases := []struct {
		x uint32
		g uint32
	}{
		{1, xAtom(0x1123)},
		{0x1f24, deltaAtom(0x124)},
		{0x123456, permAtom(187654344)},
		{0x800001, 0x40000000 | 0x1d79},
		{0x1000, 0x60000000 | 1},
	}
	for _, c := range cases {
		got := leech.Leech2OpAtom(c.x, c.g)
		want := oracleUint(t, fmt.Sprintf("int(mmgroup.generators.gen_leech2_op_atom(%d,%d))", c.x, c.g))
		if uint64(got) != want {
			t.Errorf("Leech2OpAtom(%#x,%#x)=%#x want %#x", c.x, c.g, got, want)
		}
	}
}

package generator

import (
	"fmt"
	"testing"
)

func TestGenXiLeech(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{1, 0x1000, 2, 0x100, 0x800001} {
		x1 := XiOpXi(x, 1)
		x2 := XiOpXi(x, 2)
		want1 := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_op_xi(%d,1)", x))
		want2 := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_op_xi(%d,2)", x))
		if uint64(x1) != want1 {
			t.Errorf("XiOpXi(%#x,1)=%#x want %#x", x, x1, want1)
		}
		if uint64(x2) != want2 {
			t.Errorf("XiOpXi(%#x,2)=%#x want %#x", x, x2, want2)
		}
		if got := XiOpXi(x1, 1); got != x2 {
			t.Errorf("XiOpXi(XiOpXi(%#x,1),1)=%#x want %#x", x, got, x2)
		}
		if got := XiOpXi(x2, 1); got != x {
			t.Errorf("XiOpXi(XiOpXi(%#x,2),1)=%#x want %#x", x, got, x)
		}
		if got := XiOpXi(x1, 2); got != x {
			t.Errorf("XiOpXi(XiOpXi(%#x,1),2)=%#x want %#x", x, got, x)
		}
	}
}

func TestGenXiShort(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{0x10001, 0x10020, 0x10022, 0x20000, 0x30000} {
		xl := XiShortToLeech(x)
		wantLeech := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_short_to_leech(%d)", x))
		if uint64(xl) != wantLeech {
			t.Errorf("XiShortToLeech(%#x)=%#x want %#x", x, xl, wantLeech)
		}
		xs := XiLeechToShort(xl)
		wantShort := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_leech_to_short(%d)", wantLeech))
		if uint64(xs) != wantShort {
			t.Errorf("XiLeechToShort(%#x)=%#x want %#x", xl, xs, wantShort)
		}
		for exp := 1; exp <= 2; exp++ {
			got := XiOpXiShort(x, exp)
			want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_op_xi_short(%d,%d)", x, exp))
			if uint64(got) != want {
				t.Errorf("XiOpXiShort(%#x,%d)=%#x want %#x", x, exp, got, want)
			}
		}
	}
}

func TestGenXiRef(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{1, 0x1000, 2, 0x100, 0x800001} {
		v := x >> 12
		c := x & 0xfff
		checks := []struct {
			name string
			got  uint32
			expr string
		}{
			{"XiGGray", XiGGray(v), fmt.Sprintf("mmgroup.generators.gen_xi_g_gray(%d)", v)},
			{"XiW2Gray", XiW2Gray(v), fmt.Sprintf("mmgroup.generators.gen_xi_w2_gray(%d)", v)},
			{"XiGCocode", XiGCocode(c), fmt.Sprintf("mmgroup.generators.gen_xi_g_cocode(%d)", c)},
			{"XiW2Cocode", XiW2Cocode(c), fmt.Sprintf("mmgroup.generators.gen_xi_w2_cocode(%d)", c)},
		}
		for _, ck := range checks {
			want := oracleUint(t, ck.expr)
			if uint64(ck.got) != want {
				t.Errorf("%s(x=%#x)=%#x want %#x", ck.name, x, ck.got, want)
			}
		}
	}
}

func TestLeech2Type(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{0, 0x800000, 0x800800, 0x1000, 0x200} {
		got := Leech2Subtype(x)
		want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_subtype(%d)", x))
		if uint64(got) != want {
			t.Errorf("Leech2Subtype(%#x)=%#x want %#x", x, got, want)
		}
		gotT := Leech2Type(x)
		wantT := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_type(%d)", x))
		if uint64(gotT) != wantT {
			t.Errorf("Leech2Type(%#x)=%#x want %#x", x, gotT, wantT)
		}
		gotC := Leech2CoarseSubtype(x)
		wantC := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_coarse_subtype(%d)", x))
		if uint64(gotC) != wantC {
			t.Errorf("Leech2CoarseSubtype(%#x)=%#x want %#x", x, gotC, wantC)
		}
		got2 := Leech2Type2(x)
		want2 := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_type2(%d)", x))
		if uint64(got2) != want2 {
			t.Errorf("Leech2Type2(%#x)=%#x want %#x", x, got2, want2)
		}
	}
}

func TestLeech2Mul(t *testing.T) {
	t.Parallel()
	pairs := [][2]uint32{{1, 0x1000}, {0x800001, 0x100}, {2, 2}, {0x1000, 0x800000}, {0x100, 0x10001}}
	for _, p := range pairs {
		got := Leech2Mul(p[0], p[1])
		want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_leech2_mul(%d,%d)", p[0], p[1]))
		if uint64(got) != want {
			t.Errorf("Leech2Mul(%#x,%#x)=%#x want %#x", p[0], p[1], got, want)
		}
	}
}

func TestGenXiOpXiNoSign(t *testing.T) {
	t.Parallel()
	for _, x := range []uint32{1, 0x1000, 2, 0x100, 0x800001, 0x123456} {
		for exp := 1; exp <= 2; exp++ {
			got := XiOpXiNoSign(x, exp)
			want := oracleUint(t, fmt.Sprintf("mmgroup.generators.gen_xi_op_xi_nosign(%d,%d)", x, exp))
			if uint64(got) != want {
				t.Errorf("XiOpXiNoSign(%#x,%d)=%#x want %#x", x, exp, got, want)
			}
		}
	}
}

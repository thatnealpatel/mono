package monster

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"patel.codes/cgt/reduce"
)

func tagCode(c byte) Tag {
	switch c {
	case 'A':
		return TagA
	case 'B':
		return TagB
	case 'C':
		return TagC
	case 'T':
		return TagT
	case 'X':
		return TagX
	case 'Z':
		return TagZ
	case 'Y':
		return TagY
	}
	panic("bad tag " + string(c))
}

func parseTuples(t *testing.T, py string) []Tuple {
	t.Helper()
	s := strings.TrimSpace(py)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []Tuple
	for _, part := range strings.Split(s, "),") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "()")
		fields := strings.Split(part, ",")
		var tup Tuple
		off := 0
		first := strings.TrimSpace(fields[0])
		if !strings.Contains(first, "'") {
			n, err := strconv.Atoi(first)
			if err != nil {
				t.Fatalf("parseTuples factor %q: %v", first, err)
			}
			tup.Factor = n
			off = 1
		} else {
			tup.Factor = 1
		}
		tup.Tag = tagCode(strings.Trim(strings.TrimSpace(fields[off]), "'")[0])
		i0, err := strconv.ParseInt(strings.TrimSpace(fields[off+1]), 0, 64)
		if err != nil {
			t.Fatalf("parseTuples i0: %v", err)
		}
		i1, err := strconv.ParseInt(strings.TrimSpace(fields[off+2]), 0, 64)
		if err != nil {
			t.Fatalf("parseTuples i1: %v", err)
		}
		tup.I0 = int(i0)
		tup.I1 = int(i1)
		out = append(out, tup)
	}
	return out
}

func TestMMVSize(t *testing.T) {
	t.Parallel()
	for _, p := range []int{3, 7, 15, 31, 127} {
		want := int(oracleInt(t, fmt.Sprintf("mmgroup.mm_op.mm_aux_mmv_size(%d)", p)))
		if got := MMVSize(p); got != want {
			t.Fatalf("MMVSize(%d) = %d, want %d", p, got, want)
		}
	}
}

func TestCharacteristics(t *testing.T) {
	t.Parallel()
	want := oracleInts(t, "list(mmgroup.mm_op.characteristics())")
	got := Characteristics()
	if len(got) != len(want) {
		t.Fatalf("Characteristics() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if int64(got[i]) != want[i] {
			t.Fatalf("Characteristics()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestScalprod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p      int
		v1, v2 string
	}{
		{3, "[('A',2,3),(2,'X',5,7)]", "[('A',2,3),('T',9,4)]"},
		{7, "[('B',1,2),('C',3,4)]", "[('B',1,2),('Z',10,5)]"},
		{15, "[('T',100,13),('X',0x444,3)]", "[('T',100,13),('Y',0x12,7)]"},
		{31, "[('A',0,0),('A',5,5)]", "[('A',0,0),('B',5,6)]"},
		// B/C-weight regression cases: v1 and v2 share the same
		// ('B',3,1) and ('C',5,2) off-diagonal coordinates with
		// distinct factors, so the B/C block is the only nonzero
		// overlap (the trailing A/T entries are single-sided and
		// contribute 0). The B/C contribution is (b1*b2 + c1*c2)
		// mod p = (2*2 + 3*2) mod p = 10 mod p. Internal storage
		// holds both triangles, so the raw row sum is 20 and the
		// (p+1)/2 weight halves it back to 10. These exercise the
		// weight at every supported modulus; the old hardcoded
		// weight 4 yields 80 mod p, which agrees only at p=7
		// (80 == 3 == 10 mod 7) and diverges for every other p.
		{3, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"},   // -> 10 mod 3 = 1
		{7, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"},   // -> 10 mod 7 = 3
		{15, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"},  // -> 10 mod 15 = 10
		{31, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"},  // -> 10 mod 31 = 10
		{127, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"}, // -> 10 mod 127 = 10
		{255, "[(2,'B',3,1),(3,'C',5,2),('A',1,1)]", "[(2,'B',3,1),(2,'C',5,2),('T',7,9)]"}, // -> 10 mod 255 = 10
	}
	for _, c := range cases {
		expr := fmt.Sprintf("mmgroup.mmv_scalprod(mmgroup.MMVector(%d,%s),mmgroup.MMVector(%d,%s))", c.p, c.v1, c.p, c.v2)
		want := int(oracleInt(t, expr))
		v1 := NewVector(c.p, parseTuples(t, c.v1))
		v2 := NewVector(c.p, parseTuples(t, c.v2))
		if got := Scalprod(v1, v2); got != want {
			t.Fatalf("Scalprod(p=%d) = %d, want %d", c.p, got, want)
		}
	}
}

func TestAsBytesEntry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p int
		v string
	}{
		{3, "[('A',2,3),(2,'X',5,7),('T',9,4)]"},
		{15, "[('B',1,2),('C',3,4),('Z',10,5)]"},
		{127, "[('A',0,0),('Y',0x12,7),('T',700,40)]"},
	}
	for _, c := range cases {
		for _, e := range []int{0, 24, 300, 852, 49428} {
			expr := fmt.Sprintf("int(mmgroup.MMVector(%d,%s).as_bytes()[%d])", c.p, c.v, e)
			want := int(oracleInt(t, expr))
			v := NewVector(c.p, parseTuples(t, c.v))
			if got := v.Entry(e); got != want {
				t.Fatalf("Entry(p=%d, i=%d) = %d, want %d", c.p, e, got, want)
			}
		}
	}
}

func TestOpPiDeltaPerm(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p     int
		v     string
		delta int
		pi    int
	}{
		{3, "[('X',3,6),('T',5,7)]", 0x837, 217821225},
		{15, "[('A',2,3),('Z',10,5)]", 0x123, 12745645},
		{127, "[('B',1,2),('Y',0x12,7)]", 0, 999},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0([('d',%d),('p',%d)])).as_bytes()]", c.p, c.v, c.delta, c.pi)
		want := oracleInts(t, expr)
		v := NewVector(c.p, parseTuples(t, c.v))
		g := []uint32{deltaAtom(c.delta), permAtom(c.pi)}
		got := v.Mul(g).AsBytes()
		assertBytes(t, "OpPi", c.p, got, want)
	}
}

func TestOpXY(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p         int
		v         string
		f, e, eps int
	}{
		{3, "[('X',3,6),('Z',0,0)]", 567, 1237, 0x12},
		{15, "[('T',100,13),('Y',5,3)]", 0, 1111, 0},
		{31, "[('A',2,3),('B',1,2)]", 34, 0, 0x800},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0([('y',%d),('x',%d),('d',%d)])).as_bytes()]", c.p, c.v, c.f, c.e, c.eps)
		want := oracleInts(t, expr)
		v := NewVector(c.p, parseTuples(t, c.v))
		dst := make([]uint64, len(v.Data()))
		OpXY(c.p, v.Data(), c.f, c.e, c.eps, dst)
		assertBytes(t, "OpXY", c.p, bytesOf(c.p, dst), want)
	}
}

func TestOpOmega(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p int
		v string
		x int
	}{
		{3, "[('A',2,3),('X',5,7)]", 0x800},
		{15, "[('T',100,13),('Z',10,5)]", 0x1000},
		{127, "[('B',1,2),('Y',0x12,7)]", 0x1800},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0([('x',%d)])).as_bytes()]", c.p, c.v, c.x)
		want := oracleInts(t, expr)
		v := NewVector(c.p, parseTuples(t, c.v))
		OpOmega(c.p, v.Data(), c.x)
		assertBytes(t, "OpOmega", c.p, bytesOf(c.p, v.Data()), want)
	}
}

func TestOpWord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p int
		v string
		g string
	}{
		{3, "[('X',3,6),('T',5,7)]", "[('d',0x837),('p',217821225)]"},
		{15, "[('A',2,3),('Z',10,5)]", "[('x',0x123),('d',0x456)]"},
		{127, "[('B',1,2),('Y',0x12,7)]", "[('t',1)]"},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0(%s)).as_bytes()]", c.p, c.v, c.g)
		want := oracleInts(t, expr)
		v := NewVector(c.p, parseTuples(t, c.v))
		g := parseMmWord(t, c.g)
		work := make([]uint64, len(v.Data()))
		if err := OpWord(c.p, v.Data(), g, len(g), 1, work); err != nil {
			t.Fatalf("OpWord(p=%d): %v", c.p, err)
		}
		assertBytes(t, "OpWord", c.p, bytesOf(c.p, work), want)
	}
}

func parseMmWord(t *testing.T, py string) []uint32 {
	t.Helper()
	s := oracleInts(t, fmt.Sprintf("[int(x) for x in mmgroup.MM0(%s).mmdata]", py))
	out := make([]uint32, len(s))
	for i, v := range s {
		out[i] = uint32(v)
	}
	return out
}

func TestOpWordABC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p  int
		v  string
		gx int
		gd int
	}{
		{3, "[('A',2,3),('B',1,2)]", 1237, 0x837},
		{15, "[('C',3,4),('A',0,0)]", 99, 0x123},
		{127, "[('A',5,6),('B',7,8)]", 1, 0},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0([('x',%d),('d',%d)])).as_bytes()[:852]]", c.p, c.v, c.gx, c.gd)
		want := oracleInts(t, expr)
		v := NewVector(c.p, parseTuples(t, c.v))
		g := []uint32{xAtom(c.gx), deltaAtom(c.gd)}
		dst := make([]uint64, len(v.Data()))
		if err := OpWordABC(c.p, v.Data(), g, len(g), dst); err != nil {
			t.Fatalf("OpWordABC(p=%d): %v", c.p, err)
		}
		assertBytes(t, "OpWordABC", c.p, bytesOf(c.p, dst)[:852], want)
	}
}

func TestMulStdAxisLinear(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p      int
		v1, v2 string
	}{
		{15, "'A_18_2'", "'A_11_3+A_15_2+A_18_2+A_21_0+A_21_5'"},
		{15, "'X_18_2'", "'A_17_13+A_18_2+A_19_0'"},
		{3, "'A_2_2+A_3_3-A_2_3+2*B_2_3'", "'A_18_2'"},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("bool((lambda V:(lambda f:f(V(%s)+V(%s))==f(V(%s))+f(V(%s)))(lambda v:(lambda w:(mmgroup.mm_op.mm_op_mul_std_axis(%d,w.data),w)[1])(v.copy())))(mmgroup.MMV(%d)))", c.v1, c.v2, c.v1, c.v2, c.p, c.p)
		if !oracleBool(t, expr) {
			t.Fatalf("oracle: std axis not linear for p=%d", c.p)
		}
		v1 := mustParseVector(t, c.p, strings.Trim(c.v1, "'"))
		v2 := mustParseVector(t, c.p, strings.Trim(c.v2, "'"))
		w1 := v1.Copy()
		MulStdAxis(c.p, w1.Data())
		w2 := v2.Copy()
		MulStdAxis(c.p, w2.Data())
		w3 := v1.Add(v2).Copy()
		MulStdAxis(c.p, w3.Data())
		if !w3.Equal(w1.Add(w2)) {
			t.Fatalf("MulStdAxis not linear for p=%d", c.p)
		}
		wantBytes := oracleInts(t, fmt.Sprintf("[int(x) for x in (lambda v:(mmgroup.mm_op.mm_op_mul_std_axis(%d,v.data),v)[1])(mmgroup.MMV(%d)(%s).copy()).as_bytes()]", c.p, c.p, c.v1))
		assertBytes(t, "MulStdAxis value", c.p, w1.AsBytes(), wantBytes)
	}
}

func TestPrepPi64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		delta int
		pi    int
	}{
		{0, 0},
		{0x123, 4567},
		{0x7ff, 89012},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (lambda a:(mmgroup.mm_op.mm_sub_test_prep_pi_64(%d,%d,a),a)[1])(__import__('numpy').zeros(759*7,dtype='uint32'))]", c.delta, c.pi)
		want := oracleInts(t, expr)
		out := make([]uint32, 759*7)
		subTestPrepPi64(c.delta, c.pi, out)
		if len(out) != len(want) {
			t.Fatalf("PrepPi64 len = %d, want %d", len(out), len(want))
		}
		for i := range want {
			if int64(out[i]) != want[i] {
				t.Fatalf("PrepPi64(delta=%#x,pi=%d)[%d] = %d, want %d", c.delta, c.pi, i, out[i], want[i])
			}
		}
	}
}

func TestSubTestPrepXY(t *testing.T) {
	t.Parallel()
	cases := []struct {
		f, e, eps, mode int
	}{
		{0, 0, 0, 0},
		{567, 1237, 0x12, 0},
		{34, 0, 0x800, 1},
		// mode 2 writes op_xy.sign_XYZ (2048 entries); mode 3
		// writes op_xy.s_T (759 entries, rest of the 2048-wide
		// buffer stays zero). See mm_tables.c:389-404. Both modes
		// emit substantial nonzero data for these params.
		{567, 1237, 0x12, 2},
		{567, 1237, 0x12, 3},
		{34, 0, 0x800, 2},
		{34, 0, 0x800, 3},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("[int(x) for x in (lambda a:(mmgroup.mm_op.mm_sub_test_prep_xy(%d,%d,%d,%d,a),a)[1])(__import__('numpy').zeros(2048,dtype='uint32'))]", c.f, c.e, c.eps, c.mode)
		want := oracleInts(t, expr)
		out := make([]uint32, 2048)
		subTestPrepXY(c.f, c.e, c.eps, c.mode, out)
		for i := range want {
			if int64(out[i]) != want[i] {
				t.Fatalf("subTestPrepXY(%d,%d,%d,%d)[%d] = %d, want %d", c.f, c.e, c.eps, c.mode, i, out[i], want[i])
			}
		}
	}
}

func TestTableXi(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stage, e, j, col int
	}{
		{0, 0, 0, 0},
		{2, 1, 5, 0},
		{4, 0, 10, 1},
	}
	for _, c := range cases {
		expr := fmt.Sprintf("int(mmgroup.mm_op.mm_sub_get_table_xi(%d,%d,%d,%d))", c.stage, c.e, c.j, c.col)
		want := oracleUint(t, expr)
		if got := uint64(getTableXi(c.stage, c.e, c.j, c.col)); got != want {
			t.Fatalf("getTableXi(%d,%d,%d,%d) = %#x, want %#x", c.stage, c.e, c.j, c.col, got, want)
		}
	}
}

func deltaAtom(d int) uint32 { return 0x10000000 | uint32(d&0xfffffff) }
func permAtom(p int) uint32  { return 0x20000000 | uint32(p&0xfffffff) }
func xAtom(x int) uint32     { return 0x30000000 | uint32(x&0xfffffff) }

func bytesOf(p int, data []uint64) []uint8 {
	return (&MMVector{p: p, data: data}).AsBytes()
}

func assertBytes(t *testing.T, name string, p int, got []uint8, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s(p=%d) len = %d, want %d", name, p, len(got), len(want))
	}
	for i := range want {
		if int64(got[i]) != want[i] {
			t.Fatalf("%s(p=%d) byte[%d] = %d, want %d", name, p, i, got[i], want[i])
		}
	}
}

func TestOpNormA(t *testing.T) {
	t.Parallel()
	// mm_op_norm_A is only defined for p in {3,15};
	// it returns the -1 error sentinel for p=7,31.
	cases := []struct {
		p int
		v string
	}{
		{3, "[('A',2,3),('A',0,0),('A',5,5)]"},
		{15, "[('A',0,0),('A',11,3),('A',18,2)]"},
	}
	for _, c := range cases {
		v := NewVector(c.p, parseTuples(t, c.v))
		got := OpNormA(c.p, v.Data())
		want := int(oracleInt(t, fmt.Sprintf("int(mmgroup.mm_op.mm_op_norm_A(%d,mmgroup.MMVector(%d,%s).data))", c.p, c.p, c.v)))
		if got != want {
			t.Fatalf("OpNormA(p=%d,%s)=%d want %d", c.p, c.v, got, want)
		}
	}
}

func TestOpCheckzero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p int
		v string
	}{
		{3, "[('A',2,3),('X',5,7)]"},
		{7, "[('B',1,2),('C',3,4)]"},
		{15, "[('T',100,13),('X',0x444,3)]"},
	}
	for _, c := range cases {
		v := NewVector(c.p, parseTuples(t, c.v))
		got := OpCheckzero(c.p, v.Data())
		// C mm_op_checkzero returns 1 when non-zero,
		// 0 when zero; Go's OpCheckzero returns true
		// when zero. The semantics are inverted, so
		// negate the oracle result.
		want := !oracleBool(t, fmt.Sprintf("bool(mmgroup.mm_op.mm_op_checkzero(%d,mmgroup.MMVector(%d,%s).data))", c.p, c.p, c.v))
		if got != want {
			t.Fatalf("OpCheckzero(p=%d,%s)=%v want %v", c.p, c.v, got, want)
		}
		z := ZeroVector(c.p)
		gotZ := OpCheckzero(c.p, z.Data())
		wantZ := !oracleBool(t, fmt.Sprintf("bool(mmgroup.mm_op.mm_op_checkzero(%d,mmgroup.MMVector(%d).data))", c.p, c.p))
		if gotZ != wantZ {
			t.Fatalf("OpCheckzero(p=%d,zero)=%v want %v", c.p, gotZ, wantZ)
		}
	}
}

func TestGetOffsetTableXi(t *testing.T) {
	t.Parallel()
	for stage := 0; stage < 5; stage++ {
		for e := 0; e < 2; e++ {
			for dir := 0; dir < 2; dir++ {
				got := getOffsetTableXi(stage, e, dir)
				want := oracleUint(t, fmt.Sprintf("int(mmgroup.mm_op.mm_sub_get_offset_table_xi(%d,%d,%d))", stage, e, dir))
				if uint64(got) != want {
					t.Fatalf("getOffsetTableXi(%d,%d,%d)=%#x want %#x", stage, e, dir, got, want)
				}
			}
		}
	}
}

func TestMMVectorAtSet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p             int
		tag           Tag
		tc            string
		i0, i1, value int
	}{
		{3, TagA, "A", 2, 3, 1},
		{7, TagB, "B", 1, 2, 3},
		{15, TagX, "X", 0x444, 3, 4},
		{31, TagT, "T", 100, 13, 5},
	}
	for _, c := range cases {
		v := ZeroVector(c.p)
		v.Set(c.tag, c.i0, c.i1, c.value)
		got := v.At(c.tag, c.i0, c.i1)
		want := int(oracleInt(t, fmt.Sprintf("int((lambda v:(v.__setitem__((%q,%d,%d),%d),v)[1])(mmgroup.MMVector(%d))[%q,%d,%d])", c.tc, c.i0, c.i1, c.value, c.p, c.tc, c.i0, c.i1)))
		if got != want {
			t.Fatalf("At after Set(%s,%d,%d,%d)=%d want %d", c.tc, c.i0, c.i1, c.value, got, want)
		}
		if got != c.value {
			t.Fatalf("At after Set(%s,%d,%d,%d)=%d, value not recovered", c.tc, c.i0, c.i1, c.value, got)
		}
	}
}

func TestMMVectorCountShort(t *testing.T) {
	t.Parallel()
	cases := []string{
		"[('A',2,3),('X',5,7),('B',1,2),('T',100,13)]",
		"[('B',1,2),('C',3,4),('X',0x444,3)]",
		"[('T',9,4),('X',0x123,5),('B',7,8),('C',0,1)]",
	}
	for _, v := range cases {
		vec := NewVector(15, parseTuples(t, v))
		got := vec.CountShort()
		want := oracleInts(t, fmt.Sprintf("[int(x) for x in mmgroup.MMVector(15,%s).count_short()]", v))
		if len(got) != len(want) {
			t.Fatalf("CountShort(%s) len=%d want %d", v, len(got), len(want))
		}
		for i := range want {
			if int64(got[i]) != want[i] {
				t.Fatalf("CountShort(%s)[%d]=%d want %d", v, i, got[i], want[i])
			}
		}
	}
}

// opAllCocodeWords are oracle-pinned regression words for the
// genOpAllCocode odd-cocode defect (Ow1/Ow2). genOpAllCocode
// was missing phase 2 of C mat24_op_all_cocode: for odd delta
// (delta & 0x800) bit 0 of every row's sign byte must be XORed
// with bit 12 of MAT24_THETA_TABLE[row]. The omission corrupts
// the tag-X sign of the 1288 rows whose theta bit 12 is set for
// any odd-delta cocode atom.
//
// Each word is a list of raw mmgroup atoms; an atom with tag d
// (0x1_______) and value & 0x800 set is an odd cocode element
// and triggers the formerly-defective path.
//
// Bug 3 (delta-then-xi divergence) was a TEST bug, not a
// genOpXi defect. Ow1's spec flagged an eighth pin,
// [0x30000297, 0x10000c93, 0x60000002] (x_0x297 d_0xc93 xi^2),
// as diverging from the oracle both before AND after the cocode
// fix, and the same divergence appeared for even deltas like
// [0x10000093, 0x60000002]. An intermediate analysis (Xi1)
// wrongly called this a false positive; external-oracle
// re-localization (Xi3) traced it to this test, which read the
// scratch `work` buffer via bytesOf(p, work) instead of the
// result buffer v. genOpWord ping-pongs v/work and always copies
// the result back into v after an odd number of swaps; the work
// buffer only coincidentally holds the answer for odd-swap words.
// Even-swap words (e.g. d_0x093 * xi^2, an even number of swaps)
// leave a stale value in work, so reading it diverged from the
// oracle. The fix reads v.AsBytes() (TestOpWordAllCocodeOracle).
// The eighth pin and both even-swap reproducers are now pinned
// below as odd-swap/even-swap regressions, alongside bare-xi
// controls. See the Ow2 finding.
var opAllCocodeWords = [][]uint32{
	{0x10000800},             // d_0x800: minimal odd-delta reproducer
	{0x10000fff},             // d_0xfff: all cocode bits set
	{0x10000c93},             // d_0xc93: the original defect's delta
	{0x10000801},             // d_0x801: odd + bit 0
	{0x10000001},             // d_0x001: even delta (control, no bug expected)
	{0x30000001, 0x10000800}, // x_1 * d_0x800: combined tag-X / odd-delta path
	{0x10000e03, 0x10000fd9, 0x10000941, 0x60000002, 0x30001a97}, // original 5-atom defect word
	{0x10000093, 0x60000001},             // d_0x093 * xi^1: even-swap test-buffer regression
	{0x10000093, 0x60000002},             // d_0x093 * xi^2: even-swap
	{0x30000297, 0x10000c93, 0x60000002}, // x_0x297 * d_0xc93 * xi^2: Ow1's 8th pin, even-swap
	{0x60000001},                         // xi^1 alone: odd-swap control
	{0x60000002},                         // xi^2 alone: odd-swap control
}

// opAllCocodeProbe is the fixed input vector the regression
// words act on. It touches every tag, and critically carries
// tag-X mass on rows 17 and 18 (both have MAT24_THETA_TABLE
// bit 12 set), so the genOpAllCocode phase-2 sign flip is
// actually exercised. A probe whose only tag-X mass sat on a
// bit-12-clear row (e.g. row 5) would pass even with the bug.
const opAllCocodeProbe = "[('A',2,3),('X',17,7),('X',18,4),('X',5,7),('T',9,4),('B',1,2),('Z',10,5),('Y',0x12,3)]"

// TestOpWordAllCocodeOracle pins genOpAllCocode against the
// mmgroup oracle: for each regression word, OpWord applied to
// the fixed probe vector at p = 15 must reproduce the oracle
// action MMVector(15, probe) * MM0(word) byte-for-byte. Before
// the Ow2 fix the odd-delta words diverged in the tag-X block.
func TestOpWordAllCocodeOracle(t *testing.T) {
	t.Parallel()
	const p = 15
	for wi, w := range opAllCocodeWords {
		atoms := u32List(w)
		expr := fmt.Sprintf("[int(x) for x in (mmgroup.MMVector(%d,%s)*mmgroup.MM0('a',__import__('numpy').array(%s,dtype='uint32'))).as_bytes()]",
			p, opAllCocodeProbe, atoms)
		want := oracleInts(t, expr)
		v := NewVector(p, parseTuples(t, opAllCocodeProbe))
		work := make([]uint64, len(v.Data()))
		if err := OpWord(p, v.Data(), w, len(w), 1, work); err != nil {
			t.Fatalf("OpWord(word %d=%v): %v", wi, w, err)
		}
		assertBytes(t, fmt.Sprintf("OpWordAllCocode[%d]", wi), p, v.AsBytes(), want)
	}
}

// TestOpWordReduceAgrees is the action-faithfulness regression
// from Ow1: a faithful action must map equal monster elements
// to equal vectors. For each odd-delta regression word w,
// OpWord(w) and OpWord(ReduceWord(w)) must agree on a fixed
// probe at p = 15. The genOpAllCocode defect broke exactly this
// (OpWord(w) != OpWord(ReduceWord(w)) for the same element).
func TestOpWordReduceAgrees(t *testing.T) {
	t.Parallel()
	const p = 15
	probe := RandVectorSeed(p, 1000)
	for wi, w := range opAllCocodeWords {
		wr, n := reduce.ReduceWord(w)
		if n < 0 {
			t.Fatalf("ReduceWord(word %d=%v) failed: n=%d", wi, w, n)
		}
		got := applyWord(t, probe, p, w)
		gotR := applyWord(t, probe, p, wr)
		if !got.Equal(gotR) {
			t.Fatalf("OpWord disagrees with OpWord(ReduceWord) on word %d\n  w =%v\n  wr=%v", wi, w, wr)
		}
	}
}

// TestOpWordCocodeHomomorphism checks the homomorphism property
// of the OpWord action on the regression words: applying the
// whole word in one OpWord call equals applying its atoms
// sequentially, one OpWord call per atom. The action is a group
// homomorphism, so the two must produce identical canonical
// vectors. (Each atom is applied as its own complete OpWord, so
// the word iterator's per-subword theta context is preserved.)
func TestOpWordCocodeHomomorphism(t *testing.T) {
	t.Parallel()
	const p = 15
	probe := RandVectorSeed(p, 2000)
	for wi, w := range opAllCocodeWords {
		whole := applyWord(t, probe, p, w)
		seq := probe.Copy()
		for _, atom := range w {
			seq = applyWord(t, seq, p, []uint32{atom})
		}
		if !whole.Equal(seq) {
			t.Fatalf("OpWord not homomorphic on word %d\n  w=%v", wi, w)
		}
	}
}

// applyWord returns a copy of v acted on by the word g at
// modulus p via a single OpWord call. Comparison of the result
// must go through MMVector.Equal/AsBytes, never raw Data().
func applyWord(t *testing.T, v *MMVector, p int, g []uint32) *MMVector {
	t.Helper()
	out := v.Copy()
	work := make([]uint64, len(out.Data()))
	if err := OpWord(p, out.Data(), g, len(g), 1, work); err != nil {
		t.Fatalf("OpWord(%v): %v", g, err)
	}
	return out
}

package mat24

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestMat24OrderConst(t *testing.T) {
	t.Parallel()
	want := oracleUint(t, "mat24.MAT24_ORDER")
	if uint64(Mat24Order) != want {
		t.Fatalf("Mat24Order = %d want %d", Mat24Order, want)
	}
}

func bytesToInts(b []byte) []int64 {
	r := make([]int64, len(b))
	for i, x := range b {
		r[i] = int64(x)
	}
	return r
}

func u32sToInts(u []uint32) []int64 {
	r := make([]int64, len(u))
	for i, x := range u {
		r[i] = int64(x)
	}
	return r
}

func u16sToInts(u []uint16) []int64 {
	r := make([]int64, len(u))
	for i, x := range u {
		r[i] = int64(x)
	}
	return r
}

func pyList(b []byte) string {
	s := "["
	for i, x := range b {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%d", x)
	}
	return s + "]"
}

func eqInts(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %v want %v", label, got, want)
	}
}

func TestGcodeToVect(t *testing.T) {
	t.Parallel()
	for _, n := range []uint32{0, 1, 0x800, 0xabc, 0xfff} {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			got := uint64(GcodeToVect(n))
			want := oracleUint(t, fmt.Sprintf("mat24.gcode_to_vect(%d)", n))
			if got != want {
				t.Fatalf("GcodeToVect(%d) = %d want %d", n, got, want)
			}
		})
	}
}

func TestGcode(t *testing.T) {
	t.Parallel()
	for _, n := range []uint32{0, 1, 0x123, 0x800, 0xfff} {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			vect := GcodeToVect(n)
			if got, want := uint64(VectToGcode(vect)), uint64(n); got != want {
				t.Fatalf("VectToGcode roundtrip = %d want %d", got, want)
			}
			if got, want := uint64(Bw24(vect)), oracleUint(t, fmt.Sprintf("mat24.bw24(mat24.gcode_to_vect(%d))", n)); got != want {
				t.Fatalf("Bw24 = %d want %d", got, want)
			}
			if got, want := uint64(GcodeWeight(n)), oracleUint(t, fmt.Sprintf("mat24.gcode_weight(%d)", n)); got != want {
				t.Fatalf("GcodeWeight = %d want %d", got, want)
			}
			eqInts(t, "GcodeToBitList", bytesToInts(GcodeToBitList(n)),
				oracleInts(t, fmt.Sprintf("mat24.gcode_to_bit_list(%d)", n)))
		})
	}
}

func TestBw24(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 1, 0xffffff, 0x801, 0xdb1235} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			if got, want := uint64(Bw24(v)), oracleUint(t, fmt.Sprintf("mat24.bw24(%d)", v)); got != want {
				t.Fatalf("Bw24(%d) = %d want %d", v, got, want)
			}
		})
	}
}

func TestVectToBitList(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 1, 0xff, 0x800001, 0xffffff} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			ln, lst := VectToBitList(v)
			wln := oracleInt(t, fmt.Sprintf("mat24.vect_to_bit_list(%d)[0]", v))
			if int64(ln) != wln {
				t.Fatalf("VectToBitList(%d) len = %d want %d", v, ln, wln)
			}
			eqInts(t, "VectToBitList", bytesToInts(lst),
				oracleInts(t, fmt.Sprintf("mat24.vect_to_bit_list(%d)[1]", v)))
		})
	}
}

func TestLsbit24(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{1, 2, 0x80000, 0, 0xffffff} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			if got, want := uint64(Lsbit24(v)), oracleUint(t, fmt.Sprintf("mat24.lsbit24(%d)", v)); got != want {
				t.Fatalf("Lsbit24(%d) = %d want %d", v, got, want)
			}
		})
	}
}

func TestSpread(t *testing.T) {
	t.Parallel()
	cases := []struct{ x, mask uint32 }{
		{7, 0},
		{0x311111, 0x101ffe},
		{0xffffff, 0xffffff},
		{0x123456, 0xff00ff},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.x, c.mask), func(t *testing.T) {
			ex := ExtractB24(c.x, c.mask)
			if got, want := uint64(ex), oracleUint(t, fmt.Sprintf("mat24.extract_b24(%d, %d)", c.x, c.mask)); got != want {
				t.Fatalf("ExtractB24 = %d want %d", got, want)
			}
			if got, want := uint64(SpreadB24(ex, c.mask)), oracleUint(t, fmt.Sprintf("mat24.spread_b24(%d, %d)", ex, c.mask)); got != want {
				t.Fatalf("SpreadB24 = %d want %d", got, want)
			}
		})
	}
}

func TestVintern(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 1, 0xabcdef, 0xffffff, 0x55} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			ve := VectToVintern(v)
			if got, want := uint64(ve), oracleUint(t, fmt.Sprintf("mat24.vect_to_vintern(%d)", v)); got != want {
				t.Fatalf("VectToVintern = %d want %d", got, want)
			}
			if got, want := uint64(VinternToVect(ve)), oracleUint(t, fmt.Sprintf("mat24.vintern_to_vect(%d)", ve)); got != want {
				t.Fatalf("VinternToVect = %d want %d", got, want)
			}
		})
	}
}

func TestCocode(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{1, 0x401, 0xd07, 0x3, 0xfff} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			c := VectToCocode(v)
			if got, want := uint64(c), oracleUint(t, fmt.Sprintf("mat24.vect_to_cocode(%d)", v)); got != want {
				t.Fatalf("VectToCocode = %d want %d", got, want)
			}
			if got, want := uint64(CocodeToVect(c)), oracleUint(t, fmt.Sprintf("mat24.cocode_to_vect(%d)", c)); got != want {
				t.Fatalf("CocodeToVect = %d want %d", got, want)
			}
		})
	}
}

func TestSyndrome(t *testing.T) {
	t.Parallel()
	cases := []struct{ v, t uint32 }{
		{2, 3},
		{0x401, 0},
		{0xd07, 0},
		{0x80, 24},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.v, c.t), func(t *testing.T) {
			if got, want := uint64(Syndrome(c.v, c.t)), oracleUint(t, fmt.Sprintf("mat24.syndrome(%d, %d)", c.v, c.t)); got != want {
				t.Fatalf("Syndrome = %d want %d", got, want)
			}
		})
	}
}

func TestCocodeSyndrome(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{0, 1, 0x401, 0x55, 0xabc} {
		t.Run(fmt.Sprintf("%d", c), func(t *testing.T) {
			if got, want := uint64(CocodeSyndrome(c, 0)), oracleUint(t, fmt.Sprintf("mat24.cocode_syndrome(%d, 0)", c)); got != want {
				t.Fatalf("CocodeSyndrome = %d want %d", got, want)
			}
		})
	}
}

func TestCocodeToBitList(t *testing.T) {
	t.Parallel()
	cases := []struct{ c, t uint32 }{
		{1, 0},
		{0x55, 3},
		{0xabc, 5},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.c, c.t), func(t *testing.T) {
			eqInts(t, "CocodeToBitList", bytesToInts(CocodeToBitList(c.c, c.t)),
				oracleInts(t, fmt.Sprintf("mat24.cocode_to_bit_list(%d, %d)", c.c, c.t)))
		})
	}
}

func TestCocodeToSextet(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{0x55, 0x155, 0x2aa} {
		t.Run(fmt.Sprintf("valid_%#x", c), func(t *testing.T) {
			if CocodeWeight(c) != 4 {
				t.Fatalf("cocode %#x has weight %d, not 4 — bad test value", c, CocodeWeight(c))
			}
			eqInts(t, "CocodeToSextet", bytesToInts(CocodeToSextet(c)),
				oracleInts(t, fmt.Sprintf("mat24.cocode_to_sextet(%d)", c)))
		})
	}
	for _, c := range []uint32{1, 0xabc} {
		t.Run(fmt.Sprintf("panic_%#x", c), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("CocodeToSextet(%#x) did not panic on non-tetrad input", c)
				}
			}()
			CocodeToSextet(c)
		})
	}
}

func TestAllSyndromes(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0x401 ^ 2, 0x80 ^ 0xf, 1, 0xff0, 0x33} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			got := u32sToInts(AllSyndromes(v))
			eqInts(t, "AllSyndromes", got,
				oracleInts(t, fmt.Sprintf("mat24.all_syndromes(%d)", v)))
			c := VectToCocode(v)
			eqInts(t, "CocodeAllSyndromes", u32sToInts(CocodeAllSyndromes(c)),
				oracleInts(t, fmt.Sprintf("mat24.cocode_all_syndromes(%d)", c)))
		})
	}
}

func TestCocodeWeight(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{0, 1, 0xabc, 0x55, 0xfff} {
		t.Run(fmt.Sprintf("%d", c), func(t *testing.T) {
			if got, want := uint64(CocodeWeight(c)), oracleUint(t, fmt.Sprintf("mat24.cocode_weight(%d)", c)); got != want {
				t.Fatalf("CocodeWeight = %d want %d", got, want)
			}
		})
	}
}

func TestVectType(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 0xff, 0xffffff, 0xdb1235, 0x123456} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			if got, want := uint64(VectType(v)), oracleUint(t, fmt.Sprintf("mat24.vect_type(%d)", v)); got != want {
				t.Fatalf("VectType(%d) = %d want %d", v, got, want)
			}
		})
	}
}

func TestOctads(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{3, 5, 6, 7} {
		t.Run(fmt.Sprintf("%d", c), func(t *testing.T) {
			v := GcodeToVect(c)
			oct := GcodeToOctad(c, 1)
			if got, want := uint64(oct), oracleUint(t, fmt.Sprintf("mat24.gcode_to_octad(%d)", c)); got != want {
				t.Fatalf("GcodeToOctad = %d want %d", got, want)
			}
			if got, want := uint64(VectToOctad(v, 1)), oracleUint(t, fmt.Sprintf("mat24.vect_to_octad(%d)", v)); got != want {
				t.Fatalf("VectToOctad = %d want %d", got, want)
			}
			if got, want := uint64(OctadToGcode(oct)), oracleUint(t, fmt.Sprintf("mat24.octad_to_gcode(%d)", oct)); got != want {
				t.Fatalf("OctadToGcode = %d want %d", got, want)
			}
			if got, want := uint64(OctadToVect(oct)), oracleUint(t, fmt.Sprintf("mat24.octad_to_vect(%d)", oct)); got != want {
				t.Fatalf("OctadToVect = %d want %d", got, want)
			}
		})
	}
	for _, c := range []uint32{1, 0x800, 0x903, 0xfff} {
		t.Run(fmt.Sprintf("non_octad_%d", c), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("GcodeToOctad(%d) should panic for non-octad", c)
				}
			}()
			GcodeToOctad(c, 1)
		})
	}
}

func TestGcodeToOctadNonStrict(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{1, 0x800, 0x903, 0xfff} {
		v := GcodeToVect(c)
		if Bw24(v) == 16 {
			got := GcodeToOctad(c, 0)
			want := oracleUint(t, fmt.Sprintf("mat24.gcode_to_octad(%d, 0)", c))
			if uint64(got) != want {
				t.Errorf("GcodeToOctad(%d,false)=%d want %d", c, got, want)
			}
		}
	}
}

func TestSuboctads(t *testing.T) {
	t.Parallel()
	for _, oct := range []uint32{0, 1, 100, 758} {
		t.Run(fmt.Sprintf("%d", oct), func(t *testing.T) {
			g := OctadToGcode(oct)
			v := OctadToVect(oct)
			for _, usub := range []uint32{0, 1, 0x2a, 0x3f} {
				c := SuboctadToCocode(usub, oct)
				if got, want := uint64(c), oracleUint(t, fmt.Sprintf("mat24.suboctad_to_cocode(%d, %d)", usub, oct)); got != want {
					t.Fatalf("SuboctadToCocode(%d,%d) = %d want %d", usub, oct, got, want)
				}
				back := CocodeToSuboctad(c, g, 0)
				if got, want := uint64(back), oracleUint(t, fmt.Sprintf("mat24.cocode_to_suboctad(%d, %d, 0)", c, g)); got != want {
					t.Fatalf("CocodeToSuboctad = %d want %d", got, want)
				}
				if got, want := uint64(SuboctadWeight(usub)), oracleUint(t, fmt.Sprintf("mat24.suboctad_weight(%d)", usub)); got != want {
					t.Fatalf("SuboctadWeight = %d want %d", got, want)
				}
			}
			_ = v
		})
	}
}

func TestSuboctadScalarProd(t *testing.T) {
	t.Parallel()
	cases := []struct{ a, b uint32 }{{0, 0}, {0x2a, 0x15}, {0x3f, 0x3f}, {7, 0x38}}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.a, c.b), func(t *testing.T) {
			if got, want := uint64(SuboctadScalarProd(c.a, c.b)), oracleUint(t, fmt.Sprintf("mat24.suboctad_scalar_prod(%d, %d)", c.a, c.b)); got != want {
				t.Fatalf("SuboctadScalarProd = %d want %d", got, want)
			}
		})
	}
}

func TestScalarProd(t *testing.T) {
	t.Parallel()
	cases := []struct{ v, c uint32 }{{0, 0}, {0xfff, 0x555}, {0x800, 1}, {0xabc, 0xdef}}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.v, c.c), func(t *testing.T) {
			if got, want := uint64(ScalarProd(c.v, c.c)), oracleUint(t, fmt.Sprintf("mat24.scalar_prod(%d, %d)", c.v, c.c)); got != want {
				t.Fatalf("ScalarProd = %d want %d", got, want)
			}
		})
	}
}

func TestIntersectOctadTetrad(t *testing.T) {
	t.Parallel()
	for _, oct := range []uint32{0, 50, 200} {
		t.Run(fmt.Sprintf("%d", oct), func(t *testing.T) {
			o := OctadToVect(oct)
			ln, bits := VectToBitList(o)
			if ln < 4 {
				t.Fatalf("octad has too few bits")
			}
			v2 := uint32(1)<<bits[0] | uint32(1)<<bits[1] | uint32(1)<<bits[2]
			got := IntersectOctadTetrad(o, v2)
			want := oracleUint(t, fmt.Sprintf("mat24.intersect_octad_tetrad(%d, %d)", o, v2))
			if uint64(got) != want {
				t.Fatalf("IntersectOctadTetrad = %d want %d", got, want)
			}
		})
	}
}

func TestPloopTheta(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 1, 0x401, 0xfff, 0x123} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			if got, want := uint64(PloopTheta(v)), oracleUint(t, fmt.Sprintf("mat24.ploop_theta(%d)", v)); got != want {
				t.Fatalf("PloopTheta = %d want %d", got, want)
			}
		})
	}
}

func TestParkerLoop(t *testing.T) {
	t.Parallel()
	cases := []struct{ v1, v2, v3 uint32 }{
		{0x111, 0x222, 0x444},
		{0x1abc, 0x1def, 0x123},
		{0x800, 0x1800, 0x401},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d_%d", c.v1, c.v2, c.v3), func(t *testing.T) {
			if got, want := uint64(PloopCocycle(c.v1, c.v2)), oracleUint(t, fmt.Sprintf("mat24.ploop_cocycle(%d, %d)", c.v1, c.v2)); got != want {
				t.Fatalf("PloopCocycle = %d want %d", got, want)
			}
			if got, want := uint64(MulPloop(c.v1, c.v2)), oracleUint(t, fmt.Sprintf("mat24.mul_ploop(%d, %d)", c.v1, c.v2)); got != want {
				t.Fatalf("MulPloop = %d want %d", got, want)
			}
			if got, want := uint64(PloopComm(c.v1, c.v2)), oracleUint(t, fmt.Sprintf("mat24.ploop_comm(%d, %d)", c.v1, c.v2)); got != want {
				t.Fatalf("PloopComm = %d want %d", got, want)
			}
			if got, want := uint64(PloopCap(c.v1, c.v2)), oracleUint(t, fmt.Sprintf("mat24.ploop_cap(%d, %d)", c.v1, c.v2)); got != want {
				t.Fatalf("PloopCap = %d want %d", got, want)
			}
			if got, want := uint64(PloopAssoc(c.v1, c.v2, c.v3)), oracleUint(t, fmt.Sprintf("mat24.ploop_assoc(%d, %d, %d)", c.v1, c.v2, c.v3)); got != want {
				t.Fatalf("PloopAssoc = %d want %d", got, want)
			}
			for _, e := range []uint32{0, 1, 2, 3, 4, 5} {
				if got, want := uint64(PowPloop(c.v1, e)), oracleUint(t, fmt.Sprintf("mat24.pow_ploop(%d, %d)", c.v1, e)); got != want {
					t.Fatalf("PowPloop(%d,%d) = %d want %d", c.v1, e, got, want)
				}
			}
		})
	}
}

func TestPloopSolve(t *testing.T) {
	t.Parallel()
	arrays := [][]uint32{
		{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048},
		{0x1001, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048},
	}
	for i, a := range arrays {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			py := "["
			for j, x := range a {
				if j > 0 {
					py += ","
				}
				py += fmt.Sprintf("%d", x)
			}
			py += "]"
			got := uint64(PloopSolve(a))
			want := oracleUint(t, fmt.Sprintf("mat24.ploop_solve(%s)", py))
			c := uint32(got) & 0xfff
			for _, v := range a {
				if ScalarProd(c, v) != (v>>12)&1 {
					t.Fatalf("ploop_solve constraint failed for v=%d", v)
				}
			}
			if uint64(uint32(got)&0xfff) != want&0xfff {
				t.Fatalf("PloopSolve low bits = %d want %d", uint32(got)&0xfff, want&0xfff)
			}
		})
	}
}

func TestMat24Num(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 244823040 - 1, 1} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			p := M24numToPerm(k)
			eqInts(t, "M24numToPerm", bytesToInts(p),
				oracleInts(t, fmt.Sprintf("list(mat24.m24num_to_perm(%d))", k)))
			if got, want := uint64(PermToM24num(p)), uint64(k); got != want {
				t.Fatalf("PermToM24num roundtrip = %d want %d", got, want)
			}
		})
	}
}

func TestMat24Lex(t *testing.T) {
	t.Parallel()
	for _, n := range []uint32{0, 1, 2, 244823040 - 1, 123456789} {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			p := M24numToPerm(n)
			eqInts(t, "m24num_to_perm", bytesToInts(p),
				oracleInts(t, fmt.Sprintf("list(mat24.m24num_to_perm(%d))", n)))
			if got, want := uint64(PermToM24num(p)), oracleUint(t, fmt.Sprintf("mat24.perm_to_m24num(mat24.m24num_to_perm(%d))", n)); got != want {
				t.Fatalf("perm_to_m24num = %d want %d", got, want)
			}
		})
	}
}

func TestPermCheck(t *testing.T) {
	t.Parallel()
	good := M24numToPerm(115873693)
	if err := PermCheck(good); err != nil {
		t.Fatalf("PermCheck(valid) returned error: %v", err)
	}
	bad := append([]byte(nil), good...)
	bad[0], bad[1] = bad[1], bad[0]
	if err := PermCheck(bad); err == nil {
		t.Fatalf("PermCheck(invalid) returned nil error")
	}
}

func TestMatGroup(t *testing.T) {
	t.Parallel()
	cases := []struct{ k1, k2 uint32 }{
		{115873693, 12345},
		{0, 244823040 - 1},
		{1000000, 2000000},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.k1, c.k2), func(t *testing.T) {
			p1 := M24numToPerm(c.k1)
			p2 := M24numToPerm(c.k2)
			p12 := MulPerm(p1, p2)
			eqInts(t, "MulPerm", bytesToInts(p12),
				oracleInts(t, fmt.Sprintf("list(mat24.mul_perm(mat24.m24num_to_perm(%d), mat24.m24num_to_perm(%d)))", c.k1, c.k2)))
			eqInts(t, "InvPerm", bytesToInts(InvPerm(p2)),
				oracleInts(t, fmt.Sprintf("list(mat24.inv_perm(mat24.m24num_to_perm(%d)))", c.k2)))
		})
	}
}

func TestOpVectPerm(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v uint32
		k uint32
	}{
		{0xdb1235, 115873693},
		{0xffffff, 244823040 - 1},
		{0x123456, 1000000},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.v, c.k), func(t *testing.T) {
			p := M24numToPerm(c.k)
			if got, want := uint64(OpVectPerm(c.v, p)), oracleUint(t, fmt.Sprintf("mat24.op_vect_perm(%d, mat24.m24num_to_perm(%d))", c.v, c.k)); got != want {
				t.Fatalf("OpVectPerm = %d want %d", got, want)
			}
		})
	}
}

func TestMatrixOps(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 1000000} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			p := M24numToPerm(k)
			m := PermToMatrix(p)
			eqInts(t, "PermToMatrix", u32sToInts(m),
				oracleInts(t, fmt.Sprintf("list(mat24.perm_to_matrix(mat24.m24num_to_perm(%d)))", k)))
			eqInts(t, "MatrixToPerm", bytesToInts(MatrixToPerm(m)),
				oracleInts(t, fmt.Sprintf("list(mat24.matrix_to_perm(mat24.perm_to_matrix(mat24.m24num_to_perm(%d))))", k)))
			vc := uint32(0xabc)
			if got, want := uint64(OpGcodeMatrix(vc, m)), oracleUint(t, fmt.Sprintf("mat24.op_gcode_matrix(%d, mat24.perm_to_matrix(mat24.m24num_to_perm(%d)))", vc, k)); got != want {
				t.Fatalf("OpGcodeMatrix = %d want %d", got, want)
			}
			if got, want := uint64(OpGcodePerm(vc, p)), oracleUint(t, fmt.Sprintf("mat24.op_gcode_perm(%d, mat24.m24num_to_perm(%d))", vc, k)); got != want {
				t.Fatalf("OpGcodePerm = %d want %d", got, want)
			}
			if got, want := uint64(OpCocodePerm(vc, p)), oracleUint(t, fmt.Sprintf("mat24.op_cocode_perm(%d, mat24.m24num_to_perm(%d))", vc, k)); got != want {
				t.Fatalf("OpCocodePerm = %d want %d", got, want)
			}
		})
	}
}

func TestAutpl(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c uint32
		k uint32
	}{
		{0, 0},
		{0x55, 567234},
		{0x356, 1000000},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d_%d", tc.c, tc.k), func(t *testing.T) {
			p := M24numToPerm(tc.k)
			a := PermToAutpl(tc.c, p)
			eqInts(t, "PermToAutpl", u32sToInts(a),
				oracleInts(t, fmt.Sprintf("list(mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d)))", tc.c, tc.k)))
			if got, want := uint64(AutplToCocode(a)), uint64(tc.c); got != want {
				t.Fatalf("AutplToCocode = %d want %d", got, want)
			}
			eqInts(t, "AutplToPerm", bytesToInts(AutplToPerm(a)), bytesToInts(p))
			eqInts(t, "CocodeToAutpl", u32sToInts(CocodeToAutpl(tc.c)),
				oracleInts(t, fmt.Sprintf("list(mat24.cocode_to_autpl(%d))", tc.c)))
			for _, v := range []uint32{0x111, 0x222, 0x1abc} {
				if got, want := uint64(OpPloopAutpl(v, a)), oracleUint(t, fmt.Sprintf("mat24.op_ploop_autpl(%d, mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d)))", v, tc.c, tc.k)); got != want {
					t.Fatalf("OpPloopAutpl(%d) = %d want %d", v, got, want)
				}
			}
			ai := InvAutpl(a)
			eqInts(t, "InvAutpl", u32sToInts(ai),
				oracleInts(t, fmt.Sprintf("list(mat24.inv_autpl(mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d))))", tc.c, tc.k)))
		})
	}
}

func TestMulAutpl(t *testing.T) {
	t.Parallel()
	c1, k1 := uint32(0x55), uint32(567234)
	c2, k2 := uint32(0x356), uint32(1000000)
	a1 := fmt.Sprintf("mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d))", c1, k1)
	a2 := fmt.Sprintf("mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d))", c2, k2)
	m1 := PermToAutpl(c1, M24numToPerm(k1))
	m2 := PermToAutpl(c2, M24numToPerm(k2))
	eqInts(t, "MulAutpl", u32sToInts(MulAutpl(m1, m2)),
		oracleInts(t, fmt.Sprintf("list(mat24.mul_autpl(%s, %s))", a1, a2)))
}

func TestPermToIautpl(t *testing.T) {
	t.Parallel()
	c, k := uint32(0x356), uint32(1000000)
	p := M24numToPerm(k)
	pi, ai := PermToIautpl(c, p)
	eqInts(t, "PermToIautpl perm", bytesToInts(pi),
		oracleInts(t, fmt.Sprintf("list(mat24.perm_to_iautpl(%d, mat24.m24num_to_perm(%d))[0])", c, k)))
	eqInts(t, "PermToIautpl autpl", u32sToInts(ai),
		oracleInts(t, fmt.Sprintf("list(mat24.perm_to_iautpl(%d, mat24.m24num_to_perm(%d))[1])", c, k)))
}

func TestOpAllAutpl(t *testing.T) {
	t.Parallel()
	c, k := uint32(1), uint32(0)
	m := PermToAutpl(c, M24numToPerm(k))
	got := u16sToInts(OpAllAutpl(m))
	want := oracleInts(t, fmt.Sprintf("mat24.op_all_autpl(mat24.perm_to_autpl(%d, mat24.m24num_to_perm(%d)))", c, k))
	if len(got) != 2048 {
		t.Fatalf("OpAllAutpl len = %d want 2048", len(got))
	}
	eqInts(t, "OpAllAutpl", got, want)
}

func TestOpAllCocode(t *testing.T) {
	t.Parallel()
	for _, c := range []uint32{0, 1, 0xabc} {
		t.Run(fmt.Sprintf("%d", c), func(t *testing.T) {
			got := bytesToInts(OpAllCocode(c))
			want := oracleInts(t, fmt.Sprintf("mat24.op_all_cocode(%d)", c))
			if len(got) != 2048 {
				t.Fatalf("OpAllCocode len = %d want 2048", len(got))
			}
			eqInts(t, "OpAllCocode", got, want)
		})
	}
}

func TestHeptadCompleter(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 1000000} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			full := M24numToPerm(k)
			in := make([]byte, 24)
			for i := range in {
				in[i] = 24
			}
			copy(in[:6], full[:6])
			in[8] = full[8]
			out, err := PermCompleteHeptad(in)
			if err != nil {
				t.Fatalf("PermCompleteHeptad error: %v", err)
			}
			eqInts(t, "PermCompleteHeptad", bytesToInts(out), bytesToInts(full))
		})
	}
}

func TestPermCompleteOctad(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 1000000} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			full := M24numToPerm(k)
			hexad := make([]byte, 8)
			for i := range hexad {
				hexad[i] = 24
			}
			copy(hexad[:6], full[:6])
			out, err := PermCompleteOctad(hexad)
			if err != nil {
				t.Fatalf("PermCompleteOctad error: %v", err)
			}
			py := fmt.Sprintf("(lambda p: (mat24.perm_complete_octad(p), p)[1])(%s)", pyOctadIn(full[:6]))
			eqInts(t, "PermCompleteOctad", bytesToInts(out[:8]), oracleInts(t, py))
		})
	}
}

func pyOctadIn(first6 []byte) string {
	b := append([]byte(nil), first6...)
	s := "["
	for i, x := range b {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%d", x)
	}
	s += ",None,None]"
	return s
}

func TestPermFromHeptads(t *testing.T) {
	t.Parallel()
	p1 := M24numToPerm(115873693)
	p2 := M24numToPerm(1000000)
	h1 := []byte{p1[0], p1[1], p1[2], p1[3], p1[4], p1[5], p1[8]}
	h2 := []byte{p2[0], p2[1], p2[2], p2[3], p2[4], p2[5], p2[8]}
	got := PermFromHeptads(h1, h2)
	want := oracleInts(t, fmt.Sprintf("list(mat24.perm_from_heptads(%s, %s))", pyList(h1), pyList(h2)))
	eqInts(t, "PermFromHeptads", bytesToInts(got), want)
}

func TestPermFromMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		h1, h2 []byte
	}{
		{[]byte{0, 1, 2, 3, 4}, []byte{0, 1, 2, 3, 4}},
		{[]byte{0, 1, 2, 3, 4, 5, 6}, []byte{0, 1, 2, 3, 4, 5, 6}},
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			res, perm, err := PermFromMap(c.h1, c.h2)
			if err != nil {
				t.Fatalf("PermFromMap: %v", err)
			}
			wres := oracleInt(t, fmt.Sprintf("mat24.perm_from_map(%s, %s)[0]", pyList(c.h1), pyList(c.h2)))
			if int64(res) != wres {
				t.Fatalf("PermFromMap res = %d want %d", res, wres)
			}
			eqInts(t, "PermFromMap perm", bytesToInts(perm),
				oracleInts(t, fmt.Sprintf("list(mat24.perm_from_map(%s, %s)[1])", pyList(c.h1), pyList(c.h2))))
		})
	}
}

func TestPermFromMapRejection(t *testing.T) {
	t.Parallel()
	bad := []byte{0, 0, 2, 3, 4}
	if _, _, err := PermFromMap(bad, bad); err == nil {
		t.Errorf("PermFromMap(duplicate): want error, got nil")
	}
	nomap := []byte{0, 1, 2, 3, 4, 5, 6}
	dest := []byte{1, 2, 3, 4, 5, 6, 7}
	if _, _, err := PermFromMap(nomap, dest); err == nil {
		t.Errorf("PermFromMap(no-completion): want error, got nil")
	}
	// h1={0..6} lies in octad {0..7}; h2={0..5,8}
	// is umbral (no octad contains it). M24 preserves
	// octads, so no permutation realizes this map.
	// C returns (res=0, stale identity); Go returns
	// (0, identity, errNoM24Perm).
	h1 := []byte{0, 1, 2, 3, 4, 5, 6}
	h2 := []byte{0, 1, 2, 3, 4, 5, 8}
	res, perm, err := PermFromMap(h1, h2)
	if !errors.Is(err, errNoM24Perm) {
		t.Fatalf("PermFromMap(octad→umbral): want errNoM24Perm, got %v", err)
	}
	if res != 0 {
		t.Errorf("PermFromMap(octad→umbral): res = %d, want 0", res)
	}
	for i, v := range perm {
		if v != byte(i) {
			t.Fatalf("PermFromMap(octad→umbral): perm[%d] = %d, want identity", i, v)
		}
	}
}

func TestPermFromDodecads(t *testing.T) {
	t.Parallel()
	d1 := oracleInts(t, "list(mat24.vect_to_bit_list(mat24.gcode_to_vect([g for g in range(0x1000) if mat24.gcode_weight(g)==3][0]))[1])[:12]")
	d2 := oracleInts(t, "list(mat24.vect_to_bit_list(mat24.gcode_to_vect([g for g in range(0x1000) if mat24.gcode_weight(g)==3][3]))[1])[:12]")
	b1 := intsToBytes(d1[:9])
	b2 := intsToBytes(d2[:9])
	got := PermFromDodecads(b1, b2)
	want := oracleInts(t, fmt.Sprintf("list(mat24.perm_from_dodecads(%s, %s))", pyList(b1), pyList(b2)))
	eqInts(t, "PermFromDodecads", bytesToInts(got), want)
}

func intsToBytes(v []int64) []byte {
	r := make([]byte, len(v))
	for i, x := range v {
		r[i] = byte(x)
	}
	return r
}

func TestMat24Rand(t *testing.T) {
	t.Parallel()
	for _, mode := range []uint32{0, 1, 2, 4, 64} {
		t.Run(fmt.Sprintf("mode%d", mode), func(t *testing.T) {
			if got, want := uint64(CompleteRandMode(mode)), oracleUint(t, fmt.Sprintf("mat24.complete_rand_mode(%d)", mode)); got != want {
				t.Fatalf("CompleteRandMode(%d) = %d want %d", mode, got, want)
			}
			for _, r := range []uint32{0, 12345, 987654} {
				p := PermRandLocal(mode, r)
				eqInts(t, "PermRandLocal", bytesToInts(p),
					oracleInts(t, fmt.Sprintf("list(mat24.perm_rand_local(%d, %d))", mode, r)))
				if got, want := PermInLocal(p), oracleUint(t, fmt.Sprintf("mat24.perm_in_local(mat24.perm_rand_local(%d, %d))", mode, r)); uint64(got) != want {
					t.Fatalf("PermInLocal = %d want %d", got, want)
				}
				if got, want := int64(M24numRandLocal(mode, r)), oracleInt(t, fmt.Sprintf("mat24.m24num_rand_local(%d, %d)", mode, r)); got != want {
					t.Fatalf("M24numRandLocal = %d want %d", got, want)
				}
			}
		})
	}
}

func TestCocodeAsSubdodecad(t *testing.T) {
	t.Parallel()
	dod := oracleInt(t, "[g for g in range(0x1000) if mat24.gcode_weight(g)==3][0]")
	d := uint32(dod)
	cocodes := oracleInts(t, fmt.Sprintf("[c for c in range(0x1000) if mat24.scalar_prod(%d ^ 0x800, c) == 0][:4]", d))
	for _, ci := range cocodes {
		c := uint32(ci)
		t.Run(fmt.Sprintf("%d", c), func(t *testing.T) {
			got := CocodeAsSubdodecad(c, d, 24)
			want := oracleUint(t, fmt.Sprintf("mat24.cocode_as_subdodecad(%d, %d, 24)", c, d))
			if uint64(got) != want {
				t.Fatalf("CocodeAsSubdodecad(%d,%d) = %d want %d", c, d, got, want)
			}
		})
	}
}

func TestVectToList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v      uint32
		maxLen int
	}{
		{0xff, 8},
		{0xff, 4},
		{0, 8},
		{0x800001, 24},
		{0xffffff, 24},
		{0xabc, 12},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.v, c.maxLen), func(t *testing.T) {
			got := VectToList(c.v, c.maxLen)
			eqInts(t, "VectToList", bytesToInts(got),
				oracleInts(t, fmt.Sprintf("list(mat24.vect_to_list(%d, %d))", c.v, c.maxLen)))
		})
	}
}

func TestOctadEntries(t *testing.T) {
	t.Parallel()
	for _, oct := range []uint32{0, 1, 100, 758} {
		t.Run(fmt.Sprintf("%d", oct), func(t *testing.T) {
			got := OctadEntries(oct)
			eqInts(t, "OctadEntries", bytesToInts(got[:]),
				oracleInts(t, fmt.Sprintf("list(mat24.octad_entries(%d))", oct)))
		})
	}
}

func TestPermToNet(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 1000000} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			p := M24numToPerm(k)
			got := PermToNet(p)
			eqInts(t, "PermToNet", u32sToInts(got[:]),
				oracleInts(t, fmt.Sprintf("list(mat24.perm_to_net(mat24.m24num_to_perm(%d)))", k)))
		})
	}
}

func TestMatrixFromModOmega(t *testing.T) {
	t.Parallel()
	for _, k := range []uint32{0, 115873693, 1000000} {
		t.Run(fmt.Sprintf("%d", k), func(t *testing.T) {
			p := M24numToPerm(k)
			m := PermToMatrix(p)
			MatrixFromModOmega(m)
			want := oracleInts(t, fmt.Sprintf("(lambda m: (mat24.matrix_from_mod_omega(m), list(m))[1])(mat24.perm_to_matrix(mat24.m24num_to_perm(%d)))", k))
			eqInts(t, "MatrixFromModOmega", u32sToInts(m), want)
		})
	}
}

// DIAGNOSTIC tests for the PermFromMap n=7 "bug".
//
// ROOT CAUSE: TestPermFromMap case 1 fixture
//   [0,1,2,3,4,5,6] -> [0,1,2,3,4,5,8]
// is mathematically UNSOLVABLE. h1={0..6} lies in the
// octad {0..7} (its syndrome within that octad is the
// singleton {7}); h2={0,1,2,3,4,5,8} is umbral (minimum
// syndrome weight 3, contained in no octad). M24 acts on
// the Steiner system S(5,8,24) and preserves octads, so it
// cannot map an octad-contained heptad to an umbral heptad.
//
// The C function mat24_perm_from_map returns res=0 meaning
// "no M24 permutation exists" (mat24_functions.c:1936). The
// Cython wrapper perm_from_map (mat24fast.pyx:395) guards on
// `if res < 0:` but res is uint32_t, so that check is dead
// code: it never raises, and returns (0, <stale p_out>),
// where p_out is the identity left over from the internal
// perm_from_heptads success. The Go port correctly returns
// an error for res=0. The FIX belongs in the test fixture,
// not in mat24.go.

// TestDiagOracleFromMap pins the fact that the case-1
// fixture is unsolvable: the oracle returns res=0 and the
// returned permutation does NOT realize h1 -> h2.
func TestDiagOracleFromMap(t *testing.T) {
	t.Parallel()
	res := oracleInt(t, "mat24.perm_from_map([0,1,2,3,4,5,6], [0,1,2,3,4,5,8])[0]")
	if res != 0 {
		t.Fatalf("expected oracle res=0 (unsolvable), got %d", res)
	}
	// res=0 is FAILURE; the returned perm is stale and does
	// not map h1 -> h2.
	mapsOK := oracleBool(t,
		"(lambda p: all(p[a]==b for a,b in zip([0,1,2,3,4,5,6],[0,1,2,3,4,5,8])))"+
			"(mat24.perm_from_map([0,1,2,3,4,5,6], [0,1,2,3,4,5,8])[1])")
	if mapsOK {
		t.Fatalf("oracle perm unexpectedly realizes the unsolvable map")
	}

	// Contrast: the solvable map [..6]->[..6] gives res=3.
	if resOK := oracleInt(t, "mat24.perm_from_map([0,1,2,3,4,5,6], [0,1,2,3,4,5,6])[0]"); resOK == 0 {
		t.Fatalf("solvable map [..6]->[..6] gave res=0")
	}

	// Mathematical root cause, grounded in the oracle:
	// h1={0..6} completes to octad (syndrome is a singleton);
	// h2={0,1,2,3,4,5,8} is umbral (min syndrome weight 3).
	if !oracleBool(t, "mat24.bw24(mat24.syndrome(0b1111111, 0)) <= 1") {
		t.Fatalf("expected h1={0..6} to lie in an octad")
	}
	if w := oracleInt(t, "mat24.bw24(mat24.syndrome(0b100000000111111, 0))"); w <= 1 {
		t.Fatalf("expected h2={0,1,2,3,4,5,8} to be umbral, got syndrome weight %d", w)
	}

	// Go agrees: PermFromMap must return an error here.
	if _, _, err := PermFromMap([]byte{0, 1, 2, 3, 4, 5, 6}, []byte{0, 1, 2, 3, 4, 5, 8}); err == nil {
		t.Fatalf("Go PermFromMap should error on the unsolvable case-1 map")
	}
}

// TestDiagN7Solvable confirms the n=7 path is NOT
// broken: a genuinely solvable n=7 map (and an n=7
// umbral->umbral map) must agree with the oracle.
func TestDiagN7Solvable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		h1, h2 []byte
	}{
		// Identity n=7 (h1 in octad -> h2 in octad): res=3.
		{"identity7", []byte{0, 1, 2, 3, 4, 5, 6}, []byte{0, 1, 2, 3, 4, 5, 6}},
		// Umbral heptad -> umbral heptad: the i16!=25 path.
		// Take perm p = some M24 element, map h1 -> p(h1).
		{"umbral_fixed", []byte{0, 1, 2, 3, 4, 5, 8}, []byte{0, 1, 2, 3, 4, 5, 8}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, perm, err := PermFromMap(c.h1, c.h2)
			wres := oracleInt(t, fmt.Sprintf(
				"mat24.perm_from_map(%s, %s)[0]", pyList(c.h1), pyList(c.h2)))
			t.Logf("Go: res=%d err=%v", res, err)
			t.Logf("oracle res=%d", wres)
			if wres > 0 {
				// Oracle says solvable: Go must succeed and match.
				if err != nil {
					t.Fatalf("Go errored but oracle solvable (res=%d): %v", wres, err)
				}
				if int64(res) != wres {
					t.Fatalf("res mismatch: Go=%d oracle=%d", res, wres)
				}
				want := oracleInts(t, fmt.Sprintf(
					"list(mat24.perm_from_map(%s, %s)[1])", pyList(c.h1), pyList(c.h2)))
				eqInts(t, "perm", bytesToInts(perm), want)
			} else {
				// Oracle says unsolvable (res=0): Go must error.
				if err == nil {
					t.Fatalf("Go succeeded but oracle unsolvable (res=0)")
				}
			}
		})
	}
}

// TestDiagN7UmbralReal builds a real solvable umbral
// n=7 map via an actual M24 element so the i16!=25
// branch is exercised on a SUCCESS case.
func TestDiagN7UmbralReal(t *testing.T) {
	t.Parallel()
	// h1 = umbral heptad [0,1,2,3,4,5,8].
	h1 := []byte{0, 1, 2, 3, 4, 5, 8}
	// Apply M24 element number 12345 to get its image.
	p := M24numToPerm(12345)
	h2 := make([]byte, 7)
	for i, x := range h1 {
		h2[i] = p[x]
	}
	t.Logf("h1=%v h2=%v (via M24num 12345)", h1, h2)
	res, perm, err := PermFromMap(h1, h2)
	if err != nil {
		t.Fatalf("Go PermFromMap errored on solvable umbral map: %v", err)
	}
	wres := oracleInt(t, fmt.Sprintf(
		"mat24.perm_from_map(%s, %s)[0]", pyList(h1), pyList(h2)))
	if int64(res) != wres {
		t.Fatalf("res mismatch: Go=%d oracle=%d", res, wres)
	}
	want := oracleInts(t, fmt.Sprintf(
		"list(mat24.perm_from_map(%s, %s)[1])", pyList(h1), pyList(h2)))
	eqInts(t, "perm (umbral real)", bytesToInts(perm), want)
	t.Logf("Go res=%d perm=%v (matches oracle)", res, perm)
}

// TestDiagHeptadComplete checks Go permCompleteHeptad
// against the C oracle mat24.perm_complete_heptad on
// the heptad [0,1,2,3,4,5,_,_,8] that PermFromMap(case 1)
// constructs.
func TestDiagHeptadComplete(t *testing.T) {
	t.Parallel()
	// The heptad PermFromMap builds for case 1 has
	// images at indices 0,1,2,3,4,5,8 = 0,1,2,3,4,5,8.
	var p [24]byte
	for i := range p {
		p[i] = 0xff
	}
	p[0], p[1], p[2], p[3], p[4], p[5], p[8] = 0, 1, 2, 3, 4, 5, 8
	pin := p // copy for Go
	errCode := permCompleteHeptad(pin[:])
	t.Logf("Go permCompleteHeptad err=%#x", errCode)
	t.Logf("Go result = %v", pin)

	// Python: build the 24-list with None at unset positions.
	pyExpr := fmt.Sprintf(
		"(lambda L: (mat24.perm_complete_heptad(L), L)[1])([%s])",
		heptadPyList(p[:]))
	want := oracleInts(t, pyExpr)
	t.Logf("Py result = %v", want)
	eqInts(t, "permCompleteHeptad [0,1,2,3,4,5,_,_,8]", bytesToInts(pin[:]), want)
}

// heptadPyList formats a 24-element array, 0xff -> None.
func heptadPyList(b []byte) string {
	s := ""
	for i, x := range b {
		if i > 0 {
			s += ","
		}
		if x == 0xff {
			s += "None"
		} else {
			s += fmt.Sprintf("%d", x)
		}
	}
	return s
}

// TestDiagPermFromHeptads checks Go permFromHeptads
// against the C oracle on the identical-heptad pair
// [0,1,2,3,4,5,8] that PermFromMap(case 1) constructs.
// Both yield the identity: the sub-function is correct,
// which is WHY the case-1 final h1->h2 check fails (the
// constructed heptad lost the 6->8 constraint because no
// M24 permutation can honor it).
func TestDiagPermFromHeptads(t *testing.T) {
	t.Parallel()
	h1 := []byte{0, 1, 2, 3, 4, 5, 8}
	h2 := []byte{0, 1, 2, 3, 4, 5, 8}
	var out [24]byte
	if rc := permFromHeptads(h1, h2, out[:]); rc == 0xffffffff {
		t.Fatalf("Go permFromHeptads returned -1 but the heptad is valid")
	}
	want := oracleInts(t, fmt.Sprintf(
		"list(mat24.perm_from_heptads(%s, %s))", pyList(h1), pyList(h2)))
	eqInts(t, "permFromHeptads", bytesToInts(out[:]), want)
}

func TestM24numRandAdjustXY(t *testing.T) {
	t.Parallel()
	cases := []struct{ mode, v uint32 }{
		{0, 0},
		{1, 0x123},
		{2, 0x800},
		{4, 0xfff},
		{64, 0x800},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%d", c.mode, c.v), func(t *testing.T) {
			if got, want := uint64(M24numRandAdjustXY(c.mode, c.v)), oracleUint(t, fmt.Sprintf("mat24.m24num_rand_adjust_xy(%d, %d)", c.mode, c.v)); got != want {
				t.Fatalf("M24numRandAdjustXY(%d,%d) = %d want %d", c.mode, c.v, got, want)
			}
		})
	}
}

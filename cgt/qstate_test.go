package cgt

import (
	"math"
	"math/bits"
	"math/cmplx"
	"testing"
)

const eps = 1e-8

func evalAQ(ncols int, data []uint64, v uint64) (uint64, int) {
	v &= (uint64(1) << len(data)) - 1
	var a, q, diag uint64
	for i, d := range data {
		d &= -((v >> i) & 1)
		a ^= d
		d = (d >> ncols) & v
		diag += (d >> i) & 1
		q ^= d & ((uint64(1) << i) - 1)
	}
	for _, sh := range []uint{32, 16, 8, 4, 2, 1} {
		q ^= q >> sh
	}
	a &= (uint64(1) << ncols) - 1
	return a, int((2*q + diag) & 3)
}

func oracleComplex(rows, cols int, factor [2]int, data []uint64) []complex128 {
	ncols := rows + cols
	e, phi := factor[0], factor[1]
	f := complex(math.Pow(2.0, 0.5*float64(e-phi)), 0)
	base := complex(1, 1)
	for i := 0; i < phi; i++ {
		f *= base
	}
	phases := make([]complex128, 4)
	for i := 0; i < 4; i++ {
		phases[i] = f
		f *= complex(0, 1)
	}
	a := make([]complex128, 1<<ncols)
	for v := uint64(1); v < uint64(1)<<len(data); v += 2 {
		idx, q := evalAQ(ncols, data, v)
		a[idx] += phases[q]
	}
	return a
}

func parity(v uint64) int {
	return bits.OnesCount64(v) & 1
}

func cmpComplex(t *testing.T, name string, want, got []complex128) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: length mismatch want %d got %d", name, len(want), len(got))
	}
	var diff float64
	for i := range want {
		d := cmplx.Abs(want[i] - got[i])
		if d > diff {
			diff = d
		}
	}
	if diff > eps {
		t.Fatalf("%s: max abs error %g exceeds %g", name, diff, eps)
	}
}

type tcase struct {
	rows, cols int
	factor     [2]int
	data       []uint64
}

func (c tcase) state() *QState {
	q := NewQState(c.rows, c.cols, c.data, 0)
	return q.MulScalar(c.factor[0], c.factor[1])
}

func (c tcase) oracle() []complex128 {
	return oracleComplex(c.rows, c.cols, c.factor, c.data)
}

var baseCases = []tcase{
	{0, 0, [2]int{1, 0}, nil},
	{0, 4, [2]int{1, 4}, []uint64{0, 8, 4, 2, 1}},
	{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}},
	{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}},
	{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
}

func TestMatrix(t *testing.T) {
	for i, c := range baseCases {
		got := c.state().Matrix()
		cmpComplex(t, "matrix", c.oracle(), got)
		_ = i
	}
}

func gateNotRef(c []complex128, v uint64) []complex128 {
	v &= uint64(len(c)) - 1
	out := make([]complex128, len(c))
	for i, x := range c {
		out[uint64(i)^v] = x
	}
	return out
}

func TestGateNot(t *testing.T) {
	cases := []struct {
		c tcase
		v uint64
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b000}}, 0b11},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 0b11},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 0b101},
		{tcase{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}}, 0b10},
	}
	for _, tc := range cases {
		want := gateNotRef(tc.c.oracle(), tc.v)
		got := tc.c.state().GateNot(tc.v).Matrix()
		cmpComplex(t, "gate_not", want, got)
	}
}

func gateCtrlNotRef(c []complex128, vc, v uint64) []complex128 {
	vc &= uint64(len(c)) - 1
	v &= uint64(len(c)) - 1
	out := make([]complex128, len(c))
	for i, x := range c {
		out[uint64(i)^(v*uint64(parity(vc&uint64(i))))] += x
	}
	return out
}

func TestGateCtrlNot(t *testing.T) {
	cases := []struct {
		c      tcase
		vc, v  uint64
	}{
		{tcase{0, 1, [2]int{0, 0}, []uint64{0b0}}, 1, 0},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 0b01, 0b10},
		{tcase{0, 4, [2]int{1, 4}, []uint64{0, 8, 4, 2, 1}}, 0b10, 0b01},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 0b100, 0b010},
	}
	for _, tc := range cases {
		want := gateCtrlNotRef(tc.c.oracle(), tc.vc, tc.v)
		got := tc.c.state().GateCtrlNot(tc.vc, tc.v).Matrix()
		cmpComplex(t, "gate_ctrl_not", want, got)
	}
}

func gatePhiRef(c []complex128, v uint64, phi int) []complex128 {
	v &= uint64(len(c)) - 1
	mult := []complex128{1, complex(0, 1), -1, complex(0, -1)}
	f := mult[phi&3]
	out := make([]complex128, len(c))
	for i, x := range c {
		if parity(uint64(i)&v) != 0 {
			out[i] = x * f
		} else {
			out[i] = x
		}
	}
	return out
}

func TestGatePhi(t *testing.T) {
	cases := []struct {
		c   tcase
		v   uint64
		phi int
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 0b11, 1},
		{tcase{0, 4, [2]int{1, 4}, []uint64{0, 8, 4, 2, 1}}, 0b01, 2},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 0b10, 3},
		{tcase{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}}, 0b101, 1},
	}
	for _, tc := range cases {
		want := gatePhiRef(tc.c.oracle(), tc.v, tc.phi)
		got := tc.c.state().GatePhi(tc.v, tc.phi).Matrix()
		cmpComplex(t, "gate_phi", want, got)
	}
}

func gateCtrlPhiRef(c []complex128, v1, v2 uint64) []complex128 {
	v1 &= uint64(len(c)) - 1
	v2 &= uint64(len(c)) - 1
	out := make([]complex128, len(c))
	for i, x := range c {
		if parity(uint64(i)&v1)*parity(uint64(i)&v2) != 0 {
			out[i] = -x
		} else {
			out[i] = x
		}
	}
	return out
}

func TestGateCtrlPhi(t *testing.T) {
	cases := []struct {
		c      tcase
		v1, v2 uint64
	}{
		{tcase{0, 1, [2]int{0, 0}, []uint64{0b0}}, 1, 1},
		{tcase{0, 2, [2]int{0, 0}, []uint64{0b11}}, 0b10, 0b11},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 0b11, 0b01},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 0b101, 0b010},
	}
	for _, tc := range cases {
		want := gateCtrlPhiRef(tc.c.oracle(), tc.v1, tc.v2)
		got := tc.c.state().GateCtrlPhi(tc.v1, tc.v2).Matrix()
		cmpComplex(t, "gate_ctrl_phi", want, got)
	}
}

func gateHOne(c []complex128, d uint64) []complex128 {
	out := make([]complex128, len(c))
	for i := range c {
		f := complex(1, 0)
		if uint64(i)&d != 0 {
			f = complex(-1, 0)
		}
		out[i] = f*c[i] + c[uint64(i)^d]
	}
	return out
}

func gateHRef(c []complex128, v uint64) []complex128 {
	exp := 0.0
	d := uint64(1)
	for d < uint64(len(c)) {
		if v&d != 0 {
			c = gateHOne(c, d)
			exp -= 0.5
		}
		d <<= 1
	}
	scale := complex(math.Pow(2.0, exp), 0)
	out := make([]complex128, len(c))
	for i, x := range c {
		out[i] = x * scale
	}
	return out
}

func TestGateH(t *testing.T) {
	cases := []struct {
		c tcase
		v uint64
	}{
		{tcase{0, 1, [2]int{0, 0}, []uint64{0b0}}, 1},
		{tcase{0, 2, [2]int{0, 0}, []uint64{0b11}}, 0b11},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 0b01},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 0b10},
	}
	for _, tc := range cases {
		want := gateHRef(tc.c.oracle(), tc.v)
		got := tc.c.state().GateH(tc.v).Matrix()
		cmpComplex(t, "gate_h", want, got)
	}
}

func rotIndex(index uint64, rot, nrot, n0 int) uint64 {
	if nrot <= 1 {
		return index
	}
	mask := ((uint64(1) << nrot) - 1) << n0
	i0, i1 := index&^mask, index&mask
	r := ((rot % nrot) + nrot) % nrot
	i1 = (i1 << r) + (i1 >> (nrot - r))
	return i0 + (i1 & mask)
}

func rotRef(c []complex128, rot, nrot, n0 int) []complex128 {
	out := make([]complex128, len(c))
	copy(out, c)
	for i, x := range c {
		out[rotIndex(uint64(i), rot, nrot, n0)] = x
	}
	return out
}

func TestRotBits(t *testing.T) {
	cases := []struct {
		c              tcase
		rot, nrot, n0  int
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b000}}, 3, 2, 1},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 2, 1, 1},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 1, 3, 0},
		{tcase{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}}, -1, 2, 2},
	}
	for _, tc := range cases {
		want := rotRef(tc.c.oracle(), tc.rot, tc.nrot, tc.n0)
		got := tc.c.state().RotBits(tc.rot, tc.nrot, tc.n0).Matrix()
		cmpComplex(t, "rot_bits", want, got)
	}
}

func xchIndex(index uint64, sh int, mask uint64) uint64 {
	diff := mask & (index ^ (index >> sh))
	return index ^ diff ^ (diff << sh)
}

func xchRef(c []complex128, sh int, mask uint64) []complex128 {
	out := make([]complex128, len(c))
	copy(out, c)
	for i, x := range c {
		out[xchIndex(uint64(i), sh, mask)] = x
	}
	return out
}

func TestXchBits(t *testing.T) {
	cases := []struct {
		c    tcase
		sh   int
		mask uint64
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b000}}, 2, 0b11},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 1, 0b101},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 1, 0b01},
		{tcase{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}}, 2, 0b001},
	}
	for _, tc := range cases {
		want := xchRef(tc.c.oracle(), tc.sh, tc.mask)
		got := tc.c.state().XchBits(tc.sh, tc.mask).Matrix()
		cmpComplex(t, "xch_bits", want, got)
	}
}

func extendRef(c []complex128, j, nqb int, zero bool) []complex128 {
	out := make([]complex128, len(c)<<nqb)
	mask := uint64(1<<j) - 1
	for i, x := range c {
		out[(uint64(i)&mask)+((uint64(i)&^mask)<<nqb)] = x
	}
	if !zero {
		step := uint64(1) << j
		block := uint64(1) << (nqb + j)
		for i0 := uint64(0); i0 < uint64(len(out)); i0 += block {
			for i1 := uint64(0); i1 < step; i1++ {
				i := i0 + i1
				x := out[i]
				for k := i; k < i+block; k += step {
					out[k] = x
				}
			}
		}
	}
	return out
}

func TestExtend(t *testing.T) {
	cases := []struct {
		c      tcase
		j, nqb int
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b000}}, 2, 2},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 1, 3},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 2, 1},
		{tcase{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}}, 0, 2},
	}
	for _, tc := range cases {
		want := extendRef(tc.c.oracle(), tc.j, tc.nqb, false)
		got := tc.c.state().Extend(tc.j, tc.nqb).Matrix()
		cmpComplex(t, "extend", want, got)

		wantZero := extendRef(tc.c.oracle(), tc.j, tc.nqb, true)
		gotZero := tc.c.state().ExtendZero(tc.j, tc.nqb).Matrix()
		cmpComplex(t, "extend_zero", wantZero, gotZero)
	}
}

func restrictRef(c []complex128, j, nqb int, zero bool) []complex128 {
	if !zero {
		mask := uint64(1<<j) - 1
		out := make([]complex128, len(c)>>nqb)
		for i := range out {
			out[i] = c[(uint64(i)&mask)+((uint64(i)&^mask)<<nqb)]
		}
		return out
	}
	mask := ((uint64(1) << nqb) - 1) << j
	out := make([]complex128, len(c))
	copy(out, c)
	for i := range out {
		if uint64(i)&mask != 0 {
			out[i] = 0
		}
	}
	return out
}

func sumupRef(c []complex128, j, nqb int) []complex128 {
	outer := len(c) >> (nqb + j)
	out := make([]complex128, outer<<j)
	jw := 1 << j
	for o := 0; o < outer; o++ {
		for k := 0; k < jw; k++ {
			var s complex128
			for m := 0; m < (1 << nqb); m++ {
				s += c[(o<<(nqb+j))+(m<<j)+k]
			}
			out[(o<<j)+k] = s
		}
	}
	return out
}

func TestRestrict(t *testing.T) {
	cases := []struct {
		c      tcase
		j, nqb int
	}{
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b0000}}, 2, 2},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b110_1001, 0b101_0111, 0b011_0100}}, 1, 3},
		{tcase{0, 5, [2]int{1, 4}, []uint64{0, 16, 8, 4, 2, 1}}, 1, 2},
		{tcase{0, 6, [2]int{1, 4}, []uint64{0, 32, 16, 8, 4, 2}}, 0, 3},
	}
	for _, tc := range cases {
		want := restrictRef(tc.c.oracle(), tc.j, tc.nqb, false)
		got := tc.c.state().Restrict(tc.j, tc.nqb).Matrix()
		cmpComplex(t, "restrict", want, got)

		wantZero := restrictRef(tc.c.oracle(), tc.j, tc.nqb, true)
		gotZero := tc.c.state().RestrictZero(tc.j, tc.nqb).Matrix()
		cmpComplex(t, "restrict_zero", wantZero, gotZero)

		wantSum := sumupRef(tc.c.oracle(), tc.j, tc.nqb)
		gotSum := tc.c.state().Sumup(tc.j, tc.nqb).Matrix()
		cmpComplex(t, "sumup", wantSum, gotSum)
	}
}

func matMulRef(a, b []complex128, ra, ca, cb int) []complex128 {
	out := make([]complex128, (1<<ra)*(1<<cb))
	A, B, C := 1<<ra, 1<<ca, 1<<cb
	for i := 0; i < A; i++ {
		for j := 0; j < C; j++ {
			var s complex128
			for k := 0; k < B; k++ {
				s += a[i*B+k] * b[k*C+j]
			}
			out[i*C+j] = s
		}
	}
	return out
}

func TestMatMul(t *testing.T) {
	cases := []struct {
		a, b tcase
	}{
		{tcase{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
			tcase{2, 2, [2]int{0, 0}, []uint64{0b101_01_11, 0b110_10_01, 0b011_01_00}}},
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}},
			tcase{1, 1, [2]int{0, 0}, []uint64{0b10_1, 0b01_1}}},
		{tcase{2, 1, [2]int{0, 0}, []uint64{0b000_10, 0b010_01, 0b100_01}},
			tcase{1, 2, [2]int{0, 0}, []uint64{0b00_001, 0b01_010, 0b10_100}}},
	}
	for _, tc := range cases {
		ca := oracleComplex(tc.a.rows, tc.a.cols, tc.a.factor, tc.a.data)
		cb := oracleComplex(tc.b.rows, tc.b.cols, tc.b.factor, tc.b.data)
		want := matMulRef(ca, cb, tc.a.rows, tc.a.cols, tc.b.cols)
		got := tc.a.state().MatMul(tc.b.state()).Matrix()
		cmpComplex(t, "matmul", want, got)
	}
}

func TestMulElementwise(t *testing.T) {
	cases := []struct {
		a, b tcase
	}{
		{tcase{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
			tcase{2, 2, [2]int{1, 0}, []uint64{0b101_01_11, 0b011_01_00}}},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0101, 0b00_0011}},
			tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0111, 0b00_1001}}},
		{tcase{2, 1, [2]int{0, 0}, []uint64{0b000_10, 0b010_01, 0b100_01}},
			tcase{2, 1, [2]int{-3, 4}, []uint64{0b100_01, 0b010_01}}},
	}
	for _, tc := range cases {
		ca := tc.a.oracle()
		cb := tc.b.oracle()
		want := make([]complex128, len(ca))
		for i := range ca {
			want[i] = ca[i] * cb[i]
		}
		got := tc.a.state().Mul(tc.b.state()).Matrix()
		cmpComplex(t, "mul", want, got)
	}
}

func TestProduct(t *testing.T) {
	cases := []struct {
		a, b     tcase
		nqb, nc  int
	}{
		{tcase{0, 0, [2]int{1, 0}, nil}, tcase{0, 0, [2]int{1, 0}, nil}, 0, 0},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0101, 0b00_0011}},
			tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0111, 0b00_1001}}, 2, 0},
		{tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0101, 0b00_0011}},
			tcase{0, 4, [2]int{0, 0}, []uint64{0b00_0111, 0b00_1001}}, 2, 1},
		{tcase{0, 3, [2]int{0, 0}, []uint64{0b0_001, 0b0_010}},
			tcase{0, 3, [2]int{0, 0}, []uint64{0b0_011, 0b0_101}}, 1, 0},
	}
	for _, tc := range cases {
		ca := tc.a.oracle()
		cb := tc.b.oracle()
		want := productRef(ca, cb, tc.nqb, tc.nc)
		got := FlatProduct(tc.a.state(), tc.b.state(), tc.nqb, tc.nc).Matrix()
		cmpComplex(t, "product", want, got)
	}
}

func productRef(a, b []complex128, nqb, nc int) []complex128 {
	nb := nqb - nc
	cw := 1 << nc
	bw := 1 << nb
	la := len(a) / (cw * bw)
	lb := len(b) / (cw * bw)
	out := make([]complex128, cw*la*lb)
	for k := 0; k < cw; k++ {
		for i := 0; i < la; i++ {
			for j := 0; j < lb; j++ {
				var s complex128
				for m := 0; m < bw; m++ {
					ai := k*bw*la + m*la + i
					bj := k*bw*lb + m*lb + j
					s += a[ai] * b[bj]
				}
				out[k*la*lb+i*lb+j] = s
			}
		}
	}
	return out
}

func TestReduceMatrix(t *testing.T) {
	for _, c := range baseCases {
		want := c.oracle()
		q := c.state()
		_ = q.ReduceMatrix()
		got := q.Matrix()
		cmpComplex(t, "reduce_matrix", want, got)
	}
}

func TestReduceEchelon(t *testing.T) {
	for _, c := range baseCases {
		want := c.oracle()
		cmpComplex(t, "echelon", want, c.state().Echelon().Matrix())
		cmpComplex(t, "reduce", want, c.state().Reduce().Matrix())
	}
}

func TestConjugateTranspose(t *testing.T) {
	for _, c := range baseCases {
		base := c.oracle()
		conjWant := make([]complex128, len(base))
		for i, x := range base {
			conjWant[i] = cmplx.Conj(x)
		}
		cmpComplex(t, "conj", conjWant, c.state().Conjugate().Matrix())

		rows, cols := c.rows, c.cols
		R, C := 1<<rows, 1<<cols
		tWant := make([]complex128, len(base))
		for i := 0; i < R; i++ {
			for j := 0; j < C; j++ {
				tWant[j*R+i] = base[i*C+j]
			}
		}
		cmpComplex(t, "transpose", tWant, c.state().T().Matrix())
	}
}

func TestToSigns(t *testing.T) {
	cases := []tcase{
		{0, 4, [2]int{0, 0}, []uint64{0, 8, 4, 2, 1}},
		{0, 5, [2]int{0, 0}, []uint64{0, 16, 8, 4, 2, 1}},
		{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
		{2, 0, [2]int{0, 0}, []uint64{0b000_10, 0b010_01, 0b100_01}},
	}
	for _, c := range cases {
		a := c.oracle()
		want := make([]uint64, (len(a)+31)/32)
		for k, x := range a {
			var code uint64
			if real(x) > eps {
				code = 1
			} else if real(x) < -eps {
				code = 3
			}
			want[k>>5] |= code << uint((k&31)<<1)
		}
		got := c.state().ToSigns()
		if len(got) != len(want) {
			t.Fatalf("to_signs: length want %d got %d", len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("to_signs word %d: want %x got %x", i, want[i], got[i])
			}
		}
	}
}

func TestCompareSigns(t *testing.T) {
	for _, c := range baseCases[1:] {
		q := c.state()
		bm := q.ToSigns()
		if !q.CompareSigns(bm) {
			t.Fatalf("compare_signs: expected match for own bitmap")
		}
		if len(bm) > 0 {
			bad := make([]uint64, len(bm))
			copy(bad, bm)
			bad[0] ^= 1
			if q.CompareSigns(bad) {
				t.Fatalf("compare_signs: expected mismatch for perturbed bitmap")
			}
		}
	}
}

func unitOracle(nqb int) []complex128 {
	n := 1 << nqb
	out := make([]complex128, n*n)
	for i := 0; i < n; i++ {
		out[i*n+i] = 1
	}
	return out
}

func TestUnitMatrix(t *testing.T) {
	for _, nqb := range []int{0, 1, 2, 3} {
		want := unitOracle(nqb)
		got := UnitMatrix(nqb).Matrix()
		cmpComplex(t, "unit", want, got)
	}
}

func TestPower(t *testing.T) {
	cases := []struct {
		c tcase
		e int
	}{
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, 0},
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, 2},
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, 3},
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, -1},
	}
	for _, tc := range cases {
		base := tc.c.oracle()
		n := 1 << tc.c.rows
		want := matPowRef(base, n, tc.e)
		got := tc.c.state().Power(tc.e).Matrix()
		cmpComplex(t, "power", want, got)
	}
}

func matPowRef(m []complex128, n, e int) []complex128 {
	id := make([]complex128, n*n)
	for i := 0; i < n; i++ {
		id[i*n+i] = 1
	}
	if e == 0 {
		return id
	}
	base := m
	if e < 0 {
		base = matInvRef(m, n)
		e = -e
	}
	acc := id
	for i := 0; i < e; i++ {
		acc = matMulRef(acc, base, intLog2(n), intLog2(n), intLog2(n))
	}
	return acc
}

func intLog2(n int) int {
	return bits.TrailingZeros(uint(n))
}

func matInvRef(m []complex128, n int) []complex128 {
	a := make([][]complex128, n)
	for i := 0; i < n; i++ {
		a[i] = make([]complex128, 2*n)
		copy(a[i], m[i*n:(i+1)*n])
		a[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		piv := col
		for r := col; r < n; r++ {
			if cmplx.Abs(a[r][col]) > cmplx.Abs(a[piv][col]) {
				piv = r
			}
		}
		a[col], a[piv] = a[piv], a[col]
		d := a[col][col]
		for k := 0; k < 2*n; k++ {
			a[col][k] /= d
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := a[r][col]
			for k := 0; k < 2*n; k++ {
				a[r][k] -= f * a[col][k]
			}
		}
	}
	out := make([]complex128, n*n)
	for i := 0; i < n; i++ {
		copy(out[i*n:(i+1)*n], a[i][n:])
	}
	return out
}

func TestTrace(t *testing.T) {
	cases := []tcase{
		{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}},
		{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
		{2, 2, [2]int{0, 0}, []uint64{0b101_01_11, 0b110_10_01, 0b011_01_00}},
	}
	for _, c := range cases {
		m := c.oracle()
		n := 1 << c.rows
		var want complex128
		for i := 0; i < n; i++ {
			want += m[i*n+i]
		}
		got := c.state().Trace()
		if cmplx.Abs(want-got) > eps {
			t.Fatalf("trace: want %v got %v", want, got)
		}
	}
}

func TestPauliVectorMul(t *testing.T) {
	cases := []struct {
		nqb    int
		v1, v2 uint64
	}{
		{0, 2, 3},
		{2, 0x1, 0x10},
		{2, 0x5, 0xa},
		{1, 0x3, 0x9},
	}
	for _, tc := range cases {
		p1 := PauliMatrix(tc.nqb, tc.v1)
		p2 := PauliMatrix(tc.nqb, tc.v2)
		v3 := PauliVectorMul(tc.nqb, tc.v1, tc.v2)
		want := p1.MatMul(p2).PauliVector()
		if v3 != want {
			t.Fatalf("pauli_vector_mul nqb=%d: want %x got %x", tc.nqb, want, v3)
		}
	}
}

func TestPauliVectorExp(t *testing.T) {
	cases := []struct {
		nqb int
		v   uint64
	}{
		{1, 0x3},
		{2, 0x5},
		{2, 0xa},
	}
	for _, tc := range cases {
		unit := UnitMatrix(tc.nqb)
		p := PauliMatrix(tc.nqb, tc.v)
		refs := []*QState{unit, p, p.MatMul(p), p.H()}
		for e, rm := range refs {
			want := rm.PauliVector()
			got := PauliVectorExp(tc.nqb, tc.v, e)
			if got != want {
				t.Fatalf("pauli_vector_exp nqb=%d e=%d: want %x got %x", tc.nqb, e, want, got)
			}
		}
	}
}

func TestPauliConjugate(t *testing.T) {
	cases := []struct {
		c tcase
		v []uint64
	}{
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, []uint64{0, 1, 2, 3}},
		{tcase{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}}, []uint64{0x1, 0x4, 0xa}},
		{tcase{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}}, []uint64{0x2, 0x3}},
	}
	for _, tc := range cases {
		m := tc.c.state()
		n := tc.c.rows
		mi := m.Inv()
		p := make([]*QState, len(tc.v))
		for i, x := range tc.v {
			p[i] = PauliMatrix(n, x)
		}
		want := make([]uint64, len(tc.v))
		for i, x := range p {
			want[i] = m.MatMul(x).MatMul(mi).PauliVector()
		}
		got := m.PauliConjugate(tc.v, true)
		if len(got) != len(want) {
			t.Fatalf("pauli_conjugate: length want %d got %d", len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("pauli_conjugate[%d]: want %x got %x", i, want[i], got[i])
			}
		}
		mask := (uint64(1) << (2 * n)) - 1
		noarg := m.PauliConjugate(tc.v, false)
		for i := range want {
			if noarg[i] != (want[i] & mask) {
				t.Fatalf("pauli_conjugate noarg[%d]: want %x got %x", i, want[i]&mask, noarg[i])
			}
		}
	}
}

func TestToSymplectic(t *testing.T) {
	cases := []tcase{
		{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}},
		{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
		{2, 2, [2]int{0, 0}, []uint64{0b101_01_11, 0b110_10_01, 0b011_01_00}},
	}
	for _, c := range cases {
		m := c.state()
		n := c.rows
		mi := m.Inv()
		mask := (uint64(1) << (2 * n)) - 1
		want := make([]uint64, 2*n)
		for i := 0; i < 2*n; i++ {
			p := PauliMatrix(n, uint64(1)<<i)
			want[i] = m.MatMul(p).MatMul(mi).PauliVector() & mask
		}
		got := m.ToSymplectic()
		if len(got) != len(want) {
			t.Fatalf("to_symplectic: length want %d got %d", len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("to_symplectic[%d]: want %x got %x", i, want[i], got[i])
			}
		}
	}
}

func TestOrder(t *testing.T) {
	cases := []tcase{
		{1, 1, [2]int{0, 0}, []uint64{0b00_1, 0b11_1}},
		{2, 2, [2]int{0, 0}, []uint64{0b110_10_01, 0b101_01_11, 0b011_01_00}},
	}
	for _, c := range cases {
		m := c.state()
		order := m.Order(1 << 20)
		if order <= 0 {
			t.Fatalf("order: expected positive, got %d", order)
		}
		if !m.Power(order).Equal(UnitMatrix(c.rows)) {
			t.Fatalf("order: m^%d is not unit", order)
		}
	}
}

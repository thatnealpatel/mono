// Code partly generated from the precomputed order
// vector v_1 of the representation rho_15. The big
// ORDER_VECTOR table is a verbatim copy of the C
// table MM_ORDER_VECTOR (file mm_order_vector.c,
// table ORDER_VECTOR_DATA); DO NOT EDIT the table.
//
// The remaining code ports the order-vector reduction
// engine of mm_reduce.c: it reduces a 2A axis to the
// standard axis v^+ and returns the transforming word.

package cgt

//go:generate go run -C _gen . -out ../monster_order_gen.go

import "math/bits"

//////////////////////////////////////////////////
// Part 1: precomputed order vector v_1 mod 15
//////////////////////////////////////////////////

// Region offsets into orderVectorTable (in uint32
// units). C macros OFS_ABC, OFS_T, OFS_X.
const (
	ovOfsABC = 0
	ovOfsT   = 72 * 3
	ovOfsX   = ovOfsT + 759*8
)

// Destination word offsets into the p=15 vector
// (uint64 units). C macros MM_OP15_OFS_{A,T,X} =
// MM_AUX_OFS_{A,T,X} >> 4.
const (
	ovDestA = mmAuxOfsA >> 4
	ovDestT = mmAuxOfsT >> 4
	ovDestX = mmAuxOfsX >> 4
)

// ovPair packs two uint32 table words into one
// uint64 SWAR word. C macro pair(h, l).
func ovPair(h, l uint32) uint64 {
	return (uint64(h) << 32) | uint64(l)
}

// ovLoad24 unpacks n rows of a 24-coordinate region
// (tags A, B, C, X). Each row is 3 table words ->
// 2 destination words. C function load24.
func ovLoad24(srcOfs int, dst []uint64, n int) {
	src := orderVectorTable[srcOfs:]
	for i := 0; i < n; i++ {
		dst[0] = ovPair(src[1], src[0])
		dst[1] = ovPair(0, src[2])
		src = src[3:]
		dst = dst[2:]
	}
}

// ovLoad64 unpacks n rows of a 64-coordinate region
// (tag T). Each row is 8 table words -> 4 destination
// words (two pair-steps). C function load64.
func ovLoad64(srcOfs int, dst []uint64, n int) {
	src := orderVectorTable[srcOfs:]
	for i := 0; i < 2*n; i++ {
		dst[0] = ovPair(src[1], src[0])
		dst[1] = ovPair(src[3], src[2])
		src = src[4:]
		dst = dst[2:]
	}
}

// loadOrderVector returns the precomputed order
// vector v_1 of the representation mod 15. C
// mm_order_load_vector.
func loadOrderVector() *MMVector {
	v := ZeroVector(15)
	ovLoad24(ovOfsABC, v.data[ovDestA:], 72)
	ovLoad64(ovOfsT, v.data[ovDestT:], 759)
	ovLoad24(ovOfsX, v.data[ovDestX:], 3*2048)
	return v
}

//////////////////////////////////////////////////
// Part 2: helpers for the Leech lattice mod 3 -> mod 2
// map used by the axis-type oracle (gen_leech3to2).
//////////////////////////////////////////////////

// parity24 returns the parity of the low 24 bits of v.
// C macro parity24.
func parity24(v uint32) uint32 {
	return uint32(bits.OnesCount32(v&0xffffff) & 1)
}

// genLeech3To2 maps a vector v3 in Lambda/3Lambda
// (Leech lattice mod 3 encoding) to the corresponding
// type-2, type-3 or type-4 vector in Lambda/2Lambda
// (Leech lattice encoding). The result has the type
// in bits 24..26. It returns 0 for the zero vector and
// the sentinel ^uint64(0) if v3 has no admissible type.
// C function gen_leech3to2.
func genLeech3To2(v3 uint64) uint64 {
	var gcodev, cocodev, h, w, w1, x1, syn, t, res uint32
	omega := uint32(0)
	vtype := ^uint64(0)
	v3 = short3Reduce(v3)
	h = uint32(((v3 >> 24) | v3) & 0xffffff)
	w = Bw24(h)
	switch w {
	case 22:
		syn = cocodeSyndrome(vintern(uint32(v3)), 0)
		gcodev = (uint32(v3) ^ syn) & 0xffffff
		t = h & syn
		cocodev = t | (0xffffff &^ h)
		if t == 0 || t&(t-1) != 0 {
			return vtype
		}
		vtype = 4
	case 19:
		w1 = Bw24(uint32(v3))
		if w1&1 != 0 {
			x1 = uint32(v3) & 0xffffff
		} else {
			x1 = uint32(v3>>24) & 0xffffff
		}
		syn = cocodeSyndrome(vintern(x1), 0)
		cocodev = ^h & 0xffffff
		if syn&h != 0 {
			syn = cocodev
		}
		gcodev = (x1 ^ syn) & 0xffffff
		vtype = 4
	case 16:
		w1 = Bw24(uint32(v3))
		if w1&1 != 0 {
			return vtype
		}
		gcodev = h
		omega = w1 >> 1
		cocodev = uint32(v3) & 0xfffffff
		vtype = 4
	case 13, 10:
		syn = cocodeSyndrome(vintern(h&0xffffff), 24)
		if syn&0xff000000 != 0 {
			return vtype
		}
		if h&syn != syn {
			return vtype
		}
		gcodev = h ^ syn
		cocodev = syn | (uint32(v3) &^ syn & 0xffffff)
		w1 = Bw24(cocodev)
		if w1&1 != 0 {
			return vtype
		}
		omega = (w1 >> 1) + parity24(syn&uint32(v3)) + w
		vtype = 4
	case 7:
		syn = cocodeSyndrome(vintern(h), 0)
		if syn&(syn-1) != 0 {
			return vtype
		}
		gcodev = h ^ syn
		cocodev = uint32(v3) & 0xffffff
		w1 = Bw24(cocodev)
		cocodev |= (0 - (w1 & 1)) & syn
		omega = ((w1 + 1) >> 1) + 1
		vtype = 4
	case 4:
		gcodev = 0
		cocodev = h
		omega = parity24(uint32(v3))
		vtype = 4
	case 1:
		gcodev, cocodev = 0, 0
		omega = 1
		vtype = 4
	case 24:
		cocodev = cocodeSyndrome(vintern(uint32(v3)), 0)
		gcodev = (uint32(v3) ^ cocodev) & 0xffffff
		if cocodev == 0 || cocodev&(cocodev-1) != 0 {
			return vtype
		}
		vtype = 3
	case 21:
		syn = cocodeSyndrome(vintern(uint32(v3)), 0)
		gcodev = (uint32(v3) ^ syn) & 0xffffff
		cocodev = 0xffffff &^ h
		if syn&cocodev != syn {
			return vtype
		}
		vtype = 3
	case 12:
		gcodev = h
		syn = cocodeSyndrome(vintern(h), 0)
		_ = syn
		cocodev = uint32(v3) & 0xffffff
		w1 = Bw24(cocodev)
		if w1&1 != 0 {
			return vtype
		}
		omega = (w1 >> 1) + 1
		vtype = 3
	case 9:
		syn = cocodeSyndrome(vintern(h), 0)
		if h&syn != syn {
			return vtype
		}
		gcodev = h ^ syn
		cocodev = syn | (uint32(v3) &^ syn & 0xffffff)
		w1 = Bw24(cocodev)
		if w1&1 != 0 {
			return vtype
		}
		omega = (w1 >> 1) + parity24(syn&uint32(v3))
		vtype = 3
	case 23:
		cocodev = ^h & 0xffffff
		if cocodev == 0 || cocodev&(cocodev-1) != 0 {
			return vtype
		}
		w1 = (0 - parity24(uint32(v3))) & 24
		gcodev = uint32(v3>>w1) & 0xffffff
		vtype = 2
	case 8:
		w1 = Bw24(uint32(v3))
		if w1&1 != 0 {
			return vtype
		}
		gcodev = h
		cocodev = uint32(v3) & 0xffffff
		omega = w1 >> 1
		vtype = 2
	case 2:
		cocodev = uint32(v3|(v3>>24)) & 0xffffff
		gcodev = 0
		omega = Bw24(uint32(v3)) ^ 1
		vtype = 2
	case 0:
		return 0
	default:
		return vtype
	}
	gcodev = vectToGcodeRaw(gcodev)
	if gcodev&0xfffff000 != 0 {
		return ^uint64(0)
	}
	cocodev = VectToCocode(cocodev)
	cocodev ^= uint32(mat24ThetaTable[gcodev&0x7ff]) & 0xfff
	gcodev ^= (omega & 1) << 11
	res = (gcodev << 12) ^ cocodev
	if w >= 19 {
		w1 = (uint32(vtype) ^ parity12(res&(res>>12))) & 1
		res ^= (0 - w1) & 0x800000
	}
	return uint64(res) | (vtype << 24)
}

// genLeech3To2Type4 maps a type-4 vector v3 in
// Lambda/3Lambda to the corresponding type-4 vector in
// Lambda/2Lambda, returning 0 if v3 is not of type 4.
// C function gen_leech3to2_type4.
func genLeech3To2Type4(v3 uint64) uint32 {
	res := genLeech3To2(v3)
	if res>>24 == 4 {
		return uint32(res) & 0xffffff
	}
	return 0
}

//////////////////////////////////////////////////
// Part 3: 2A axis type oracle (mm_reduce_2A_axis_type).
//////////////////////////////////////////////////

// ovAxesTypes maps mm_op15_norm_A mod 15 to a coarse
// 2A axis type. C table axes_types.
var ovAxesTypes = [16]uint8{
	0, 0, 0x82, 0x43, 0xF2, 0x63, 0, 0,
	0xF0, 0, 0xC3, 0, 0, 0x42, 0xF0, 0,
}

// reduce2AAxisType returns the 2A axis type of the
// monster-rep vector v (mod 15) encoded as
// n*2^28 + k*2^24 + leech2, where n is the class
// number and k the class letter. C function
// mm_reduce_2A_axis_type. It reads the tag-A part of
// v only.
func reduce2AAxisType(v []uint64) uint32 {
	norm := uint32(OpNormA(15, v))
	res := uint32(ovAxesTypes[norm&15])
	if res < 0x0F0 {
		return res << 24
	}
	r := uint64(evalARankMod3(v, res&0xf))
	rank := uint32(r >> 48)
	v3 := r & 0xffffffffffff
	switch norm {
	case 4:
		if rank == 2 {
			return 0xA1000000
		}
		if rank == 23 {
			v2 := Leech3To2Short(v3) & 0xffffff
			valA := mmEvalA(v, v2)
			switch valA {
			case 4:
				return 0x21000000 + v2
			case 7:
				return 0x61000000 + v2
			}
		}
	case 8:
		if rank == 8 {
			return 0x22000000
		}
		if rank == 24 {
			return 0xA2000000
		}
	case 14:
		if rank == 8 {
			return 0x66000000
		}
		if rank == 23 {
			v2 := genLeech3To2Type4(v3) & 0xffffff
			return 0x41000000 + v2
		}
	}
	return 0
}

// mmEvalA evaluates v2 * A * v2^T mod 15 for the
// tag-A matrix A of v and short Leech-mod-2 vector v2.
// C mm_op15_eval_A (no triality, i.e. exponent 0).
func mmEvalA(v []uint64, v2 uint32) uint32 {
	return uint32(evalA15(v, v2))
}

// evalARankMod3 returns (rank << 48) + w for the
// tag-A matrix of v minus d*I over GF(3); w is a
// kernel vector (Leech mod 3 encoding) when the
// corank is 1. C mm_op15_eval_A_rank_mod3.
func evalARankMod3(v []uint64, d uint32) int64 {
	a := make([]uint64, 24*3)
	OpLoadLeech3Matrix(15, v, a)
	return leech3matrixRank(a, d)
}

//////////////////////////////////////////////////
// Part 4: find short vectors of given absolute value
// (mm_op15_eval_X_find_abs).
//////////////////////////////////////////////////

// ovMaxShortArray is the capacity of the short-vector
// list in an axis analysis. C macro MAX_SHORT_ARRAY.
const ovMaxShortArray = 892

// ovAbs15 returns the absolute value mod 15 of a
// reduced 4-bit field value e (0..14): e if e <= 7
// else 15 - e.
func ovAbs15(e uint8) uint8 {
	if e > 7 {
		return 15 - e
	}
	return e
}

// findShortAll collects, in Leech lattice encoding,
// the coordinates of the monomial part (tags B, C, T,
// X) of v whose absolute value mod 15 equals value0,
// followed by those equal to value1. Entries matching
// value1 carry bit 24 set. It writes at most n entries
// to out and returns the number written. C function
// find_short_all / mm_op15_eval_X_find_abs.
func findShortAll(v []uint64, out []uint32, n int, value0, value1 uint32) int {
	if value0 > 7 {
		value0 = 0
	}
	if value0 == 0 {
		return 0
	}
	if value1 > 7 || value1 == value0 {
		value1 = 0
	}
	a0 := uint8(value0)
	a1 := uint8(value1)
	hasV1 := value1 != 0

	// pstart grows forward; pend shrinks backward, as
	// in the C buffer-splitting scheme.
	pstart := 0
	pend := n

	emit := func(idx uint32) bool {
		e := ovAbs15(getMMV(15, v, idx))
		if e == a0 {
			if pstart >= pend {
				return false
			}
			out[pstart] = idx
			pstart++
		}
		if hasV1 && e == a1 {
			if pstart >= pend {
				return false
			}
			pend--
			out[pend] = 0x1000000 + idx
		}
		return true
	}

	// Tags B and C: strict lower triangle (col < row)
	// of the symmetric 24x24 matrix, row-major. Each
	// row occupies 32 internal indices.
	for _, base := range []int{mmAuxOfsB, mmAuxOfsC} {
		for row := 1; row < 24; row++ {
			for col := 0; col < row; col++ {
				if !emit(uint32(base + row*32 + col)) {
					goto convert
				}
			}
		}
	}

	// Tags T and X: contiguous internal index range.
	for idx := mmAuxOfsT; idx < mmAuxOfsZ; idx++ {
		if !emit(uint32(idx)) {
			goto convert
		}
	}

convert:
	// Reverse the value1 segment and move it directly
	// after the value0 segment.
	rev := n - pend
	for j := 0; j < rev/2; j++ {
		out[pend+j], out[pend+rev-1-j] = out[pend+rev-1-j], out[pend+j]
	}
	for j := 0; j < rev; j++ {
		out[pstart+j] = out[pend+j]
	}
	total := pstart + rev

	for j := 0; j < total; j++ {
		leech2 := mmAuxIndexInternToLeech2(out[j] & 0xffffff)
		out[j] = (out[j] & 0xff000000) | (leech2 & 0xffffff)
	}
	return total
}

//////////////////////////////////////////////////
// Part 5: analyze a 2A axis (analyze_axis and the
// get_short / get_span / get_radical helpers).
//////////////////////////////////////////////////

// ovAxesReduce stores the result of analyzeAxis. C
// struct axes_reduce_t.
type ovAxesReduce struct {
	axisType   uint32
	targetAxes [2]uint32
	vLeech2    []uint32 // length nLeech2
}

// getShort fills p.vLeech2 with the coordinates of the
// monomial part of v whose absolute value is value0
// (and value1, if nonzero). C function get_short.
func getShort(v []uint64, value0, value1 uint32, p *ovAxesReduce) {
	buf := make([]uint32, ovMaxShortArray)
	n := findShortAll(v, buf, ovMaxShortArray, value0, value1)
	p.vLeech2 = buf[:n]
}

// getSpan replaces p.vLeech2 with the (capped) linear
// span of the short vectors of value value. C function
// get_span.
func getSpan(v []uint64, value uint32, p *ovAxesReduce) {
	getShort(v, value, 0, p)
	basis := Leech2MatrixBasis(p.vLeech2)
	dim := len(basis)
	if dim > 10 {
		dim = 10
	}
	p.vLeech2 = leech2MatrixExpand(basis[:dim])
}

// getRadical replaces p.vLeech2 with the (capped)
// radical of the span of the short vectors of value
// value. C function get_radical.
func getRadical(v []uint64, value uint32, p *ovAxesReduce) {
	getShort(v, value, 0, p)
	basis := Leech2MatrixRadical(p.vLeech2)
	dim := len(basis)
	if dim > 10 {
		dim = 10
	}
	p.vLeech2 = leech2MatrixExpand(basis[:dim])
}

// leech2MatrixExpand lists all 2^dim vectors of the
// subspace spanned by basis, in Leech lattice
// encoding. C function leech2_matrix_expand.
func leech2MatrixExpand(basis []uint64) []uint32 {
	out := make([]uint32, 1<<len(basis))
	out[0] = 0
	length := 1
	for i := len(basis) - 1; i >= 0; i-- {
		w := uint32(basis[i]) & 0xffffff
		for j := 0; j < length; j++ {
			out[length+j] = w ^ out[j]
		}
		length += length
	}
	return out[:length]
}

// xorEntries adds value to every vector in p.vLeech2.
// C function xor_entries.
func xorEntries(p *ovAxesReduce, value uint32) {
	for i := range p.vLeech2 {
		p.vLeech2[i] ^= value
	}
}

// analyzeAxis analyzes the 2A axis v and fills p with
// its type, the feasible target types, and the list
// U(v) of Leech-mod-2 vectors. It returns 0 on success
// and a negative value on error. C function
// analyze_axis.
func analyzeAxis(v []uint64, p *ovAxesReduce) int {
	vt := reduce2AAxisType(v)
	p.axisType = vt >> 24
	p.targetAxes[0] = 0xffffffff
	p.targetAxes[1] = 0xffffffff
	p.vLeech2 = nil
	if vt == 0 {
		return -2
	}
	vt &= 0xffffff
	switch p.axisType {
	case 0xC3:
		getRadical(v, 7, p)
		p.targetAxes[0] = 0x42
		p.targetAxes[1] = 0x61
		return 0
	case 0xA2:
		getRadical(v, 4, p)
		p.targetAxes[0] = 0x42
		p.targetAxes[1] = 0x43
		return 0
	case 0xA1:
		getShort(v, 3, 1, p)
		vt = p.vLeech2[0]
		xorEntries(p, vt)
		p.vLeech2[0] = vt | 0x2000000
		p.targetAxes[0] = 0x61
		return 0
	case 0x82:
		getShort(v, 1, 0, p)
		vt = p.vLeech2[0]
		vt1 := uint32(0)
		for j := range p.vLeech2 {
			vt2 := vt ^ p.vLeech2[j]
			if Leech2Type(vt2) == 4 {
				vt1 = vt2
			}
			p.vLeech2[j] |= 0x2000000
		}
		if vt1 != 0 {
			p.vLeech2 = append(p.vLeech2, vt1)
		}
		p.targetAxes[0] = 0x41
		return 0
	case 0x66:
		getRadical(v, 7, p)
		p.targetAxes[0] = 0x43
		return 0
	case 0x63:
		getSpan(v, 3, p)
		p.targetAxes[0] = 0x41
		return 0
	case 0x61:
		getShort(v, 5, 0, p)
		xorEntries(p, vt)
		p.vLeech2 = append(p.vLeech2, p.vLeech2[0])
		p.vLeech2[0] = vt | 0x2000000
		p.targetAxes[0] = 0x41
		return 0
	case 0x43, 0x42:
		getRadical(v, 1, p)
		p.targetAxes[0] = 0x22
		return 0
	case 0x41:
		p.vLeech2 = []uint32{vt}
		p.targetAxes[0] = 0x21
		return 0
	case 0x22:
		getSpan(v, 4, p)
		p.targetAxes[0] = 0x21
		return 0
	case 0x21:
		p.vLeech2 = []uint32{vt}
		return 0
	default:
		p.axisType = 0
		return -3
	}
}

//////////////////////////////////////////////////
// Part 6: select a type-4 vector from a list
// (mm_reduce_find_type4 / find_type4).
//////////////////////////////////////////////////

// reduceFindType4 returns a type-4 vector from the
// list v according to the ordering
// [48, 40, (42,44), 46, 43]. If v2 != 0 only vectors w
// with type(w)=4 and type(w+v2)=2 are accepted. It
// returns 0 if none is found. The list v is destroyed.
// C function mm_reduce_find_type4.
func reduceFindType4(v []uint32, v2 uint32) uint32 {
	n := len(v)
	v2 &= 0xffffff
	for i := 0; i < n; i++ {
		v[i] &= 0xffffff
	}
	noSub2 := v2 == 0
	var part [6]uint32
	part[5] = uint32(n)

	i, j := 0, n
	i = condSort(v, i, j, func(x uint32) bool { return x&0x800 != 0 })
	part[4] = uint32(i)
	j = i
	i = condSort(v, 0, j, func(x uint32) bool {
		return uint32(mat24ThetaTable[(x>>12)&0x7ff])&0x1000 != 0
	})
	part[3] = uint32(i)
	j = i
	i = condSort(v, 0, j, func(x uint32) bool { return x&0x7ff000 != 0 })
	part[2] = uint32(i)

	for k := 0; k < int(part[2]); k++ {
		if v[k] == 0x800000 {
			v[0], v[k] = v[k], v[0]
			part[1] = 1
			break
		}
	}

	for k := 0; k < 5; k++ {
		lo := int(part[k])
		hi := int(part[k+1])
		sortU32(v[lo:hi])
		for m := lo; m < hi; m++ {
			if Leech2Type(v[m]) == 4 && (noSub2 || Leech2Type2(v[m]^v2) != 0) {
				return v[m]
			}
		}
	}
	return 0
}

// condSort partitions v[i:j] in place: entries failing
// cond are moved to the front, entries satisfying cond
// to the back, scanning inward from both ends. It
// returns the new split index (the count of failing
// entries). C macro bitvector_cond_sort.
func condSort(v []uint32, i, j int, cond func(uint32) bool) int {
	for {
		for i < j && !cond(v[i]) {
			i++
		}
		for i < j && cond(v[j-1]) {
			j--
		}
		if i >= j {
			return i
		}
		j--
		v[i], v[j] = v[j], v[i]
		i++
	}
}

// sortU32 sorts a slice of uint32 ascending. C
// function bitvector32_sort (an insertion sort).
func sortU32(a []uint32) {
	for i := 1; i < len(a); i++ {
		x := a[i]
		k := i
		for k > 0 && a[k-1] > x {
			a[k] = a[k-1]
			k--
		}
		a[k] = x
	}
}

// findType4 returns a type-4 vector from p.vLeech2,
// ignoring an initial run of entries with bit 25 set.
// C function find_type4.
func findType4(p *ovAxesReduce, v2 uint32) uint32 {
	v := p.vLeech2
	for len(v) > 0 && v[0]&0x2000000 != 0 {
		v = v[1:]
	}
	return reduceFindType4(v, v2)
}

//////////////////////////////////////////////////
// Part 7: transform helpers (v_leech2_adjust_sign,
// transform_v4).
//////////////////////////////////////////////////

// vLeech2AdjustSign returns the element v2 of Q_x0
// (Leech lattice encoding) with its sign bit (bit 24)
// set to match the sign of the axis v. C function
// v_leech2_adjust_sign.
func vLeech2AdjustSign(v []uint64, v2 uint32) uint32 {
	ind := IndexLeech2ToSparse(v2 & 0xffffff)
	sp := []uint32{ind}
	mmvExtractSparse(15, v, sp, 1)
	signBit := uint32(0)
	if sp[0]&15 == 2 {
		signBit = 1
	}
	return (v2 & 0xffffff) | (signBit << 24)
}

// transformV4 transforms the axis v by mapping the
// type-4 vector v4 to Omega, then applying tau^e for
// e in {1,2} until the axis type is one of
// targetAxes. It stores the transforming word in r and
// returns its length, or a negative value on failure.
// C function transform_v4.
func transformV4(v []uint64, v4 uint32, targetAxes [2]uint32, r []uint32, work []uint64) int {
	lenR := genLeech2ReduceType4(v4, r)
	if lenR < 0 {
		return -1000 + lenR
	}
	if err := OpWord(15, v, r, lenR, 1, work); err != nil {
		panic("cgt: transformV4 OpWord: " + err.Error())
	}
	if targetAxes[0]&0xffffff00 != 0 {
		return lenR
	}
	for e := 1; e < 3; e++ {
		OpTA(15, v, e, work)
		axType := reduce2AAxisType(work) >> 24
		for j := 0; j < 2; j++ {
			if axType == targetAxes[j] {
				r[lenR] = 0xD0000003 - uint32(e)
				if err := OpWord(15, v, r[lenR:], 1, 1, work); err != nil {
					panic("cgt: transformV4 OpWord: " + err.Error())
				}
				lenR++
				return lenR
			}
		}
	}
	return -10
}

//////////////////////////////////////////////////
// Part 8: reduce a 2A axis to v^+ (reduce_v_axis,
// mm_reduce_vector_vp).
//////////////////////////////////////////////////

// reduceVAxisFinal appends the closing atoms of the
// reduction once the axis has reached type 2A. C
// function reduce_v_axis_final.
func reduceVAxisFinal(vt uint32, r []uint32, lenR int, stdAxis bool) int {
	if !stdAxis {
		r[lenR] = 0x84000000 + (vt & 0x1ffffff)
		lenR++
		return lenR
	}
	lenR1 := genLeech2ReduceType2(vt, r[lenR:])
	if lenR1 < 0 {
		return -10003
	}
	vt = Leech2OpWord(vt, r[lenR:lenR+lenR1])
	lenR += lenR1
	if vt&0x1000000 != 0 {
		r[lenR] = 0xB0000200
		lenR++
	}
	r[lenR] = 0x84000200
	lenR++
	return lenR
}

// reduceVAxis reduces the 2A axis v (or the element vt
// of Q_x0, if nonzero) to the standard axis v^+. It
// stores the transforming word in r and returns its
// length, or a negative value on error. C function
// reduce_v_axis. The work buffer must have the size of
// a p=15 vector.
func reduceVAxis(vt uint32, v []uint64, r []uint32, stdAxis bool, work []uint64) int {
	lenR := 0

	if vt != 0 {
		return reduceVAxisFinal(vt, r, 0, stdAxis)
	}

	var i int
	for i = 0; i < 5; i++ {
		var ax ovAxesReduce
		if status := analyzeAxis(v, &ax); status < 0 {
			return -11000 + status
		}
		targetAxes := ax.targetAxes
		axType := ax.axisType

		if axType == 0x21 {
			vt = ax.vLeech2[0]
			vt = vLeech2AdjustSign(v, vt)
			return reduceVAxisFinal(vt, r, lenR, stdAxis)
		}

		v4 := findType4(&ax, 0)
		lenR1 := transformV4(v, v4, targetAxes, r[lenR:], work)
		if lenR1 < 0 {
			return -13000 + lenR1
		}
		lenR += lenR1
	}
	return -12000 - i
}

// ovMarkVPDone marks a successful v^+ reduction in
// r[0]. C macro MM_REDUCE_MARK_VP_DONE.
const ovMarkVPDone = 0x8FED5500

// ovMarkError marks a failed reduction in r[0]. C
// macro MM_REDUCE_MARK_ERROR.
const ovMarkError = 0x7FFFFF00

// mmReduceVectorVP computes a word h with v.h == v^+
// for the 2A axis encoded by (vt, v), storing h in r
// (prefixed by a marker atom) and returning the word
// length. A negative return value indicates a fatal
// error. C function mm_reduce_vector_vp.
func mmReduceVectorVP(vt uint32, v []uint64, mode int, r []uint32, work []uint64) int {
	r[0] = 0
	res := reduceVAxis(vt, v, r[1:], mode&1 != 0, work)
	if res > 0 && res <= 40 {
		res++
		r[0] = ovMarkVPDone + uint32(res)
		r[res] = ovChecksum(r[:res])
		return res
	}
	if res >= 0 {
		res = -10000
	}
	r[0] = ovMarkError
	r[1] = uint32(-res)
	return res
}

// ovChecksum sums the entries of r. C function
// checksum.
func ovChecksum(r []uint32) uint32 {
	var sum uint32
	for _, x := range r {
		sum += x
	}
	return sum
}

//////////////////////////////////////////////////
// Part 9: rebase a 2A axis (axis.rebase_axis).
//////////////////////////////////////////////////

// rebaseAxis returns a G_x0 element g0 with
// v^+ . g0 == v15, or nil on failure. It mirrors
// axis.rebase_axis, which uses the order-vector
// reducer.
func rebaseAxis(v15 *MMVector) *MM {
	if v15.p != 15 {
		panic("cgt: rebaseAxis supported for p = 15 only")
	}
	v := v15.Copy()
	work := ZeroVector(15)
	r := make([]uint32, 256)
	lG := mmReduceVectorVP(0, v.data, 1, r, work.data)
	if lG < 0 || lG >= 128 {
		return nil
	}
	g0 := (&MM{data: append([]uint32(nil), r[:lG]...)}).Inv()

	// Verify v^+ . g0 == v15, as axis.rebase_axis does;
	// return nil on mismatch.
	base, err := ParseVector(15, axisV15)
	if err != nil {
		panic("cgt: rebaseAxis: " + err.Error())
	}
	if !base.Mul(g0.Mmdata()).Equal(v15) {
		return nil
	}
	return g0
}

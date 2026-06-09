package cgt

// This file ports the group operations and the
// supporting tables from mm_tables.c, mm_tables_xi.c
// and the per-modulus mm*_op_*.c files. The
// per-modulus C uses SWAR bit tricks; here the
// arithmetic is done field-generically so that one
// implementation serves every modulus.

// fieldWidth returns the bit width of one field for
// modulus p.
func fieldWidth(p int) uint { return uint(1) << logFieldBits(p) }

// getField returns field j of word w for modulus p,
// reduced modulo p.
func getField(p int, w uint64, j uint, fw uint) int {
	v := (w >> (j * fw)) & uint64(p)
	if v == uint64(p) {
		return 0
	}
	return int(v)
}

// opVectorAdd computes mv1 += mv2 over the data
// region. C mm_op*_vector_add.
func opVectorAdd(p int, mv1, mv2 []uint64) {
	n := MMVSize(p)
	fw := fieldWidth(p)
	nf := uint(64) / fw
	for i := 0; i < n; i++ {
		a := mv1[i]
		b := mv2[i]
		var out uint64
		for j := uint(0); j < nf; j++ {
			va := getField(p, a, j, fw)
			vb := getField(p, b, j, fw)
			s := (va + vb) % p
			out |= uint64(s) << (j * fw)
		}
		mv1[i] = out
	}
}

// opScalarMul computes mv *= factor (0 <= factor <
// p). C mm_op*_scalar_mul.
func opScalarMul(p, factor int, mv []uint64) {
	n := MMVSize(p)
	fw := fieldWidth(p)
	nf := uint(64) / fw
	f := factor % p
	if f < 0 {
		f += p
	}
	for i := 0; i < n; i++ {
		a := mv[i]
		var out uint64
		for j := uint(0); j < nf; j++ {
			v := getField(p, a, j, fw)
			out |= uint64((v*f)%p) << (j * fw)
		}
		mv[i] = out
	}
}

// OpCheckzero reports whether vector v modulo p is
// zero (treating an all-ones field as zero). C
// mm_op_checkzero (returns 0 if zero); here true
// means zero.
//
// OpCheckzero panics if p is not a supported modulus.
func OpCheckzero(p int, v []uint64) bool {
	checkP(p)
	n := MMVSize(p)
	fw := fieldWidth(p)
	nf := uint(64) / fw
	for i := 0; i < n; i++ {
		w := v[i]
		for j := uint(0); j < nf; j++ {
			if getField(p, w, j, fw) != 0 {
				return false
			}
		}
	}
	return true
}

/**********************************************************************
 * Scalar product (mm*_op_scalprod.c)
 **********************************************************************/

// scalprodRows dots nRows rows of 32 entries each,
// starting at uint64 index base, returning the
// unreduced sum.
func scalprodRows(p int, v1, v2 []uint64, base, nRows int) int {
	fw := fieldWidth(p)
	nf := uint(64) / fw
	wordsPerRow := 32 / int(nf) // uint64 words per 32-entry row
	sum := 0
	for r := 0; r < nRows; r++ {
		off := base + r*wordsPerRow
		for w := 0; w < wordsPerRow; w++ {
			a := v1[off+w]
			b := v2[off+w]
			for j := uint(0); j < nf; j++ {
				sum += getField(p, a, j, fw) * getField(p, b, j, fw)
			}
		}
	}
	return sum
}

// ofsWords returns an internal offset converted to
// uint64-word units for modulus p.
func ofsWords(p, ofs int) int { return ofs >> logIntFields(p) }

// Scalprod returns the scalar product of v1 and v2. C
// mm_op_scalprod via mmv_scalprod.
//
// Scalprod panics if the moduli differ.
func Scalprod(v1, v2 *MMVector) int {
	if v1.p != v2.p {
		panic("cgt: Scalprod requires equal moduli")
	}
	p := v1.p
	a := v1.data
	b := v2.data
	res := scalprodRows(p, a, b, ofsWords(p, mmAuxOfsT), 2*759)
	res += scalprodRows(p, a, b, ofsWords(p, mmAuxOfsA), 24)
	res += 4 * scalprodRows(p, a, b, ofsWords(p, mmAuxOfsB), 48)
	res += scalprodRows(p, a, b, ofsWords(p, mmAuxOfsX), 6144)
	return res % p
}

// ScalprodInd computes the scalar product of v1 and
// v2 using the precomputed nonzero-index array ind. C
// mm_op_scalprod_ind. Here we ignore ind and compute
// directly (the result is identical).
func ScalprodInd(p int, v1, v2 []uint64, ind []uint16) int {
	a := &MMVector{p: p, data: v1}
	b := &MMVector{p: p, data: v2}
	return Scalprod(a, b)
}

/**********************************************************************
 * xi tables (mm_tables_xi.c)
 **********************************************************************/

// xiPermTables indexes the perm tables as [stage][e].
var xiPermTables = [5][2][]uint16{
	{xiPerm00[:], xiPerm01[:]},
	{xiPerm10[:], xiPerm11[:]},
	{xiPerm20[:], xiPerm21[:]},
	{xiPerm30[:], xiPerm31[:]},
	{xiPerm40[:], xiPerm41[:]},
}

// xiSignTables indexes the sign tables as [stage][e].
var xiSignTables = [5][2][]uint32{
	{xiSign00[:], xiSign01[:]},
	{xiSign10[:], xiSign11[:]},
	{xiSign20[:], xiSign21[:]},
	{xiSign30[:], xiSign31[:]},
	{xiSign40[:], xiSign41[:]},
}

// xiOffsetTable is the xi offset table. C
// MM_SUB_OFFSET_TABLE_XI[5][2][2].
var xiOffsetTable = [5][2][2]uint32{
	{{0x00000300, 0x00000300}, {0x00000300, 0x00000300}},
	{{0x00000cc0, 0x00000cc0}, {0x00000cc0, 0x00000cc0}},
	{{0x000066c0, 0x0000c6c0}, {0x000066c0, 0x000146c0}},
	{{0x0000c6c0, 0x000146c0}, {0x000146c0, 0x0000c6c0}},
	{{0x000146c0, 0x000066c0}, {0x0000c6c0, 0x000066c0}},
}

// GetTableXi returns entry j of the perm table (col
// == 0) or sign table (col != 0) for stage and e. C
// mm_sub_get_table_xi.
func GetTableXi(stage, e, j, col int) uint32 {
	if col != 0 {
		return xiSignTables[stage][e][j]
	}
	return uint32(xiPermTables[stage][e][j])
}

// GetOffsetTableXi returns the xi offset for stage,
// e, dir. C mm_sub_get_offset_table_xi.
func GetOffsetTableXi(stage, e, dir int) uint32 {
	return xiOffsetTable[stage][e][dir]
}

/**********************************************************************
 * Preparation tables for pi and xy (mm_tables.c)
 **********************************************************************/

// subOpPi64 mirrors the C struct mm_sub_op_pi64_type.
type subOpPi64 struct {
	preimage uint16
	perm     [6]uint8
}

// mmSubPermTable maps a pair (i,j) of an octad to a
// suboctad number. C table MM_SUB_PERM64_TABLE.
var mmSubPermTable = [64]uint8{
	0x00, 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x3f,
	0x01, 0x00, 0x03, 0x05, 0x09, 0x11, 0x21, 0x3e,
	0x02, 0x03, 0x00, 0x06, 0x0a, 0x12, 0x22, 0x3d,
	0x04, 0x05, 0x06, 0x00, 0x0c, 0x14, 0x24, 0x3b,
	0x08, 0x09, 0x0a, 0x0c, 0x00, 0x18, 0x28, 0x37,
	0x10, 0x11, 0x12, 0x14, 0x18, 0x00, 0x30, 0x2f,
	0x20, 0x21, 0x22, 0x24, 0x28, 0x30, 0x00, 0x1f,
	0x3f, 0x3e, 0x3d, 0x3b, 0x37, 0x2f, 0x1f, 0x00,
}

// mmSuboctadTable maps an 8-bit octad subset mask to
// a suboctad number. C table MM_SUBOCTAD_TABLE.
var mmSuboctadTable = [128]uint8{
	0x00, 0x3f, 0x3e, 0x01, 0x3d, 0x02, 0x03, 0x3c,
	0x3b, 0x04, 0x05, 0x3a, 0x06, 0x39, 0x38, 0x07,
	0x37, 0x08, 0x09, 0x36, 0x0a, 0x35, 0x34, 0x0b,
	0x0c, 0x33, 0x32, 0x0d, 0x31, 0x0e, 0x0f, 0x30,
	0x2f, 0x10, 0x11, 0x2e, 0x12, 0x2d, 0x2c, 0x13,
	0x14, 0x2b, 0x2a, 0x15, 0x29, 0x16, 0x17, 0x28,
	0x18, 0x27, 0x26, 0x19, 0x25, 0x1a, 0x1b, 0x24,
	0x23, 0x1c, 0x1d, 0x22, 0x1e, 0x21, 0x20, 0x1f,
	0x1f, 0x20, 0x21, 0x1e, 0x22, 0x1d, 0x1c, 0x23,
	0x24, 0x1b, 0x1a, 0x25, 0x19, 0x26, 0x27, 0x18,
	0x28, 0x17, 0x16, 0x29, 0x15, 0x2a, 0x2b, 0x14,
	0x13, 0x2c, 0x2d, 0x12, 0x2e, 0x11, 0x10, 0x2f,
	0x30, 0x0f, 0x0e, 0x31, 0x0d, 0x32, 0x33, 0x0c,
	0x0b, 0x34, 0x35, 0x0a, 0x36, 0x09, 0x08, 0x37,
	0x07, 0x38, 0x39, 0x06, 0x3a, 0x05, 0x04, 0x3b,
	0x3c, 0x03, 0x02, 0x3d, 0x01, 0x3e, 0x3f, 0x00,
}

// mat24Order is the order of the Mathieu group M24. C
// MAT24_ORDER.
const mat24Order = 244823040

// subPrepPi64 computes the 759-entry tag-T preparation
// table for the operation x_eps x_pi. It mirrors the
// tbl_perm64 portion of C mm_sub_prep_pi.
func subPrepPi64(eps, pi uint32) []subOpPi64 {
	out := make([]subOpPi64, 759)

	// perm and inv_perm of pi, plus the big autpl table.
	perm := M24numToPerm(pi % mat24Order)
	invPerm, repAutpl := PermToIautpl(eps&0xfff, perm)
	p24big := OpAllAutpl(repAutpl) // first 2048 entries

	var pInv [24]uint8
	for i := 0; i < 24; i++ {
		pInv[i] = uint8(invPerm[i])
	}
	epsMasked := eps & 0x800
	p0base := 0
	for i := uint32(0); i < 759; i++ {
		var src uint32
		{
			dest := uint32(mat24OctDecTable[i]) // octad_to_gcode
			src = ((dest & epsMasked) << 1) ^ uint32(p24big[dest&0x7ff])
			sign := src & 0x1000
			src &= 0xfff
			src = gcodeToOctadFast(src)
			out[i].preimage = uint16(src | sign)
		}
		{
			var qi [24]uint8
			p1 := mat24OctadElementTable[src<<3:]
			qi[p1[0]] = 0
			qi[p1[1]] = 1
			qi[p1[2]] = 2
			qi[p1[3]] = 4
			qi[p1[4]] = 8
			qi[p1[5]] = 16
			qi[p1[6]] = 32
			qi[p1[7]] = 63
			p0 := mat24OctadElementTable[p0base:]
			q0 := qi[pInv[p0[0]]]
			acc := qi[pInv[p0[1]]] ^ q0
			out[i].perm[0] = acc
			acc ^= qi[pInv[p0[2]]] ^ q0
			out[i].perm[1] = acc
			acc ^= qi[pInv[p0[3]]] ^ q0
			out[i].perm[2] = acc
			acc ^= qi[pInv[p0[4]]] ^ q0
			out[i].perm[3] = acc
			acc ^= qi[pInv[p0[5]]] ^ q0
			out[i].perm[4] = acc
			acc ^= qi[pInv[p0[6]]] ^ q0
			out[i].perm[5] = acc
		}
		p0base += 8
	}
	return out
}

// gcodeToOctadFast converts a Golay code number to an
// octad number without validation. C macro
// mat24_def_gcode_to_octad.
func gcodeToOctadFast(v uint32) uint32 {
	return uint32(mat24OctEncTable[v&0x7ff]) >> 1
}

// SubTestPrepPi64 writes the 759*(1+6) tag-T
// preparation table to out (length 759*7). C
// mm_sub_test_prep_pi_64.
func SubTestPrepPi64(delta, pi int, out []uint32) {
	tbl := subPrepPi64(uint32(delta), uint32(pi))
	o := 0
	for i := 0; i < 759; i++ {
		out[o] = uint32(tbl[i].preimage)
		for j := 0; j < 6; j++ {
			out[o+j+1] = uint32(tbl[i].perm[j])
		}
		o += 7
	}
}

// subOpXY mirrors the relevant fields of the C struct
// mm_sub_op_xy_type.
type subOpXY struct {
	f       uint32
	e       uint32
	eps     uint32
	fI      uint32
	efI     uint32
	linI    [3]uint32
	linD    [3]uint32
	signXYZ [2048]uint8
	sT      [759]uint16
}

// toSuboctad mirrors the C macro to_suboctad: it
// projects vector v onto the octad given by po and
// returns the 6-bit suboctad number.
func toSuboctad(v uint32, po []uint8) uint8 {
	idx := ((v >> po[0]) & 1) +
		(((v >> po[1]) & 1) << 1) +
		(((v >> po[2]) & 1) << 2) +
		(((v >> po[3]) & 1) << 3) +
		(((v >> po[4]) & 1) << 4) +
		(((v >> po[5]) & 1) << 5) +
		(((v >> po[6]) & 1) << 6)
	return mmSuboctadTable[idx]
}

// pwrMap returns bit 12 of the theta table entry for
// d. C macro PwrMap.
func pwrMap(d uint32) uint32 {
	return (uint32(mat24ThetaTable[d&0x7ff]) >> 12) & 1
}

// pwrMapH returns the full theta table entry for d. C
// macro PwrMapH.
func pwrMapH(d uint32) uint32 { return uint32(mat24ThetaTable[d]) }

// subPrepXY computes the operation tables for
// y_f x_e x_eps. C mm_sub_prep_xy.
func subPrepXY(f, e, eps uint32) *subOpXY {
	op := &subOpXY{}
	f &= 0x1fff
	e &= 0x1fff
	eps &= 0xfff
	op.f = f
	op.e = e
	op.eps = eps

	op.linI[0] = GcodeToVect(e)
	v := GcodeToVect(f)
	op.linI[1] = v
	op.linI[2] = v
	op.fI = v
	op.efI = op.linI[0] ^ op.fI

	// sign_XYZ
	{
		ld0 := eps ^ PloopCap(e, f) ^ PloopTheta(f)
		ld2 := eps ^ PloopTheta(e)
		ld1 := ld2 ^ PloopTheta(f)
		pXYZ := &op.signXYZ
		pXYZ[0] = uint8(
			(pwrMap(f) ^ pwrMap(e^f) ^ (f >> 12)) ^
				((PloopCocycle(f, e) ^ ((e ^ f) >> 12)) << 1) ^
				(((pwrMap(f) ^ (e >> 12) ^ (e >> 11)) & 1) << 2),
		)
		for li := uint32(0); li < 11; li++ {
			i := uint32(1) << li
			vv := ((ld0 >> li) & 1) + (((ld1 >> li) & 1) << 1) +
				(((ld2 >> li) & 1) << 2)
			for j := uint32(0); j < i; j++ {
				pXYZ[i+j] = pXYZ[j] ^ uint8(vv)
			}
		}
		e1 := e & 0x7ff
		ef1 := (e ^ f) & 0x7ff
		eps1 := ((eps & 0x800) ^ 0x800) << 1
		for d := uint32(0); d < 2048; d++ {
			pXYZ[d] ^= uint8(
				((0 - (pwrMapH(d^ef1) & 0x1000)) >> 12) ^
					((0 - (pwrMapH(d^e1) & 0x1000)) >> 11) ^
					((pwrMapH(d) & eps1) >> 12),
			)
		}
		op.linD[0] = e1 ^ ef1
		op.linD[1] = ef1
		op.linD[2] = e1
	}

	// s_T
	{
		vf := op.linI[1]
		vef := vf ^ op.linI[0]
		signE := pwrMap(e)
		p0 := 0
		for oct := uint32(0); oct < 759; oct++ {
			d := uint32(mat24OctDecTable[oct]) // octad_to_gcode
			pOct := mat24OctadElementTable[p0:]
			res := uint32(toSuboctad(vf, pOct))
			res ^= uint32(toSuboctad(vef, pOct)) << 8
			sign := d & eps
			sign = parity12(sign)
			sign ^= signE ^ pwrMap(d^e)
			op.sT[oct] = uint16(res + (sign << 14) + ((eps & 0x800) << (15 - 11)))
			p0 += 8
		}
	}
	return op
}

// SubTestPrepXY writes components (selected by mode)
// of the xy preparation tables to out. C
// mm_sub_test_prep_xy.
func SubTestPrepXY(f, e, eps, mode int, out []uint32) {
	op := subPrepXY(uint32(f), uint32(e), uint32(eps))
	switch mode {
	case 1:
		for i := 0; i < 3; i++ {
			out[i] = op.linI[i]
			out[i+3] = op.linD[i]
		}
	case 2:
		for i := 0; i < 2048; i++ {
			out[i] = uint32(op.signXYZ[i])
		}
	case 3:
		for i := 0; i < 759; i++ {
			out[i] = uint32(op.sT[i])
		}
	}
}

// tableOctadToStdAxOp supports Griess multiplication
// by the standard axis. C table
// TABLE_OCTAD_TO_STD_AX_OP.
var tableOctadToStdAxOp = [24]uint64{
	0x000000003000c0cf, 0x0000000000000000,
	0x0000000000000000, 0x0000000000000000,
	0x8a208a208a208000, 0x8a208a208a208a20,
	0x55554a208a208a20, 0x5555555555555555,
	0x5555555555555555, 0x5555555555555555,
	0x5555555555555555, 0x0000955555555555,
	0x0200200220020020, 0x0020020080020080,
	0x0080002008002002, 0x0082000808002008,
	0x0200008008080008, 0x5555400808000820,
	0x5555555555555555, 0x5555555555555555,
	0x5555555555555555, 0x5555555555555555,
	0x5555555555555555, 0x0000155555555555,
}

// subTableOctadToStdAxOp returns the 2-bit table
// entry for octad o, or -1 if o >= 759. C
// mm_sub_table_octad_to_std_ax_op.
func subTableOctadToStdAxOp(o uint32) int {
	if o >= 759 {
		return -1
	}
	return int((tableOctadToStdAxOp[o>>5] >> (2 * (o & 31))) & 3)
}

/**********************************************************************
 * Group operations on a vector
 **********************************************************************/

// onesFieldMask returns a word whose first nFields
// fields each hold the value p (the all-ones field
// pattern used to negate entries mod p = 2^k-1).
func onesFieldMask(p int, nFields, fw uint) uint64 {
	var m uint64
	for j := uint(0); j < nFields; j++ {
		m |= uint64(p) << (j * fw)
	}
	return m
}

// negateRowsXYZ negates the 24 entries of each of the
// nRows rows of tags X/Z/Y starting at word index
// base (XOR with the all-ones field pattern, the
// mod-2^k-1 negation).
func negateRowsXYZ(p int, v []uint64, base, nRows int) {
	fw := fieldWidth(p)
	nf := uint(64) / fw
	per := rowsPer24(p)
	for r := 0; r < nRows; r++ {
		off := base + r*per
		got := uint(0)
		for w := 0; w < per; w++ {
			rem := uint(24) - got
			if rem > nf {
				rem = nf
			}
			v[off+w] ^= onesFieldMask(p, rem, fw)
			got += nf
		}
	}
}

// OpOmega applies the generator x_d (d in {0, 0x800,
// 0x1000, 0x1800}, i.e. x_1, x_Omega, x_-1,
// x_-Omega) to v in place. C mm_op*_omega. It negates
// whole X/Z/Y blocks selected by d.
func OpOmega(p int, v []uint64, x int) {
	d := uint32(x) & 0x1800
	if d == 0 {
		return
	}
	base := ofsWords(p, mmAuxOfsX)
	per := rowsPer24(p)
	blockRows := 2048
	sh := uint32(0x01120200) >> ((d >> 11) << 3)
	for i0 := uint(0); i0 < 8; i0 += 4 {
		k := (sh >> i0) & 0xf
		blockBase := base + int(k)*blockRows*per
		negateRowsXYZ(p, v, blockBase, blockRows)
	}
}

// OpPi applies x_delta * x_pi to src, storing the
// result in dst. It is a monomial operation built
// from mm_sub_prep_pi: tags X, Z, Y and the diagonal
// A are permuted as rows of 24 with signs, and tag T
// is permuted as rows of 64. C mm{p}_op_pi.
//
// OpPi panics if p is not a supported modulus.
func OpPi(p int, src []uint64, delta, pi int, dst []uint64) {
	genOpPi(p, src, delta, pi, dst)
}

// OpXY applies y_f * x_e * x_eps to src, storing the
// result in dst. It is monomial, built from
// mm_sub_prep_xy and the tag-T permutation tables.
// C mm{p}_op_xy.
//
// OpXY panics if p is not a supported modulus.
func OpXY(p int, src []uint64, f, e, eps int, dst []uint64) {
	genOpXY(p, src, f, e, eps, dst)
}

// OpWord applies g^e to v in place, using work as
// scratch. C mm_op*_word. It runs the mm_group word
// iterator and, per group element, dispatches the
// xi, tau, xy and pi/delta blocks.
//
// OpWord panics if p is not a supported modulus.
func OpWord(p int, v []uint64, g []uint32, length, e int, work []uint64) error {
	return genOpWord(p, v, g, length, e, work)
}

// OpWordTagA applies g^e to the tag-A part of v in
// place. C mm_op*_word_tag_A.
//
// OpWordTagA panics if p is not a supported modulus.
// It returns a non-nil error if the word contains a
// nonzero tau power (which does not fix tag A).
func OpWordTagA(p int, v []uint64, g []uint32, length, e int) error {
	return genOpWordTagA(p, v, g, length, e)
}

// OpWordABC computes the tags A, B, C of v * g into
// dst. C mm_op*_word_ABC. PrepareOpABC (below)
// supplies its word-preprocessing front end.
//
// OpWordABC panics: the general path is blocked on the
// gen_leech2 subframe machinery (gen_leech2_prefix_Gx0,
// gen_leech2_map_std_subframe, the extract_BC subframe
// extraction) and the tag-ABC tau/delta restricted ops
// (mm_op*_t_ABC, mm_op*_delta_tag_ABC), none of which
// are ported. Its word front end (PrepareOpABC) and the
// tag-ABC xy/pi/xi ops are available.
func OpWordABC(p int, src []uint64, g []uint32, length int, dst []uint64) error {
	return genOpWordABC(p, src, g, length, dst)
}

// PrepareOpABC preprocesses the word g (of length
// length) for OpWordABC, writing the result to out
// (which must hold at least 11 atoms). C
// mm_group_prepare_op_ABC.
//
// It returns a negative value on failure. On success
// the low 8 bits hold the output word length; bit 8
// (0x100) is set iff g lies in N_0, in which case a
// terminating zero atom is appended and out holds a
// word of at most 5 atoms with tags t,y,x,d,p in that
// order. If bit 8 is clear, out holds a word in the
// generators of G_{x0}, possibly followed by a single
// tag-t atom.
//
// Every prefix of g must lie in G_{x0} * N_0.
func PrepareOpABC(g []uint32, length int, out []uint32) int {
	g = g[:length]
	hasT := false   // a tag-t atom has occurred
	hasL := false   // a tag-l atom has occurred
	reduce := false // g must be reduced

	for _, atom := range g {
		// If a tag-t atom has previously occurred then
		// a tag-l atom forces a reduction.
		if hasT {
			reduce = true
		}
		switch (atom >> 28) & 7 {
		case 5: // tag t
			if atom&0xfffffff != 0 {
				hasT = true
			}
		case 6: // tag l
			if atom&0xfffffff != 0 {
				hasL = true
			}
		case 7: // illegal tag
			return -1001
		}
	}

	if !hasL {
		// No tag l: compute the whole word in N_0.
		var a N0Elem
		if int(nMulWordScan(&a, g)) < length {
			return -1002
		}
		lenA := nToWord(&a, out)
		return int(lenA) + 0x100
	}

	if !reduce && length <= 11 {
		// Return g unchanged.
		copy(out[:length], g)
		return length
	}

	// Reduce g into a product elem * gn with elem in
	// G_{x0} and gn in N_0.
	var elem [26]uint64
	pos := xsp2co1SetElemWordScan(elem[:], g, false)
	if pos < 0 || pos > length {
		return -0x1009
	}
	var gn N0Elem
	scan := int(nMulWordScan(&gn, g[pos:]))
	if pos+scan != length {
		return -1003
	}
	// Here g = elem * gn.

	if xsp2co1ElemSubtype(elem[:]) == 0x48 {
		// elem is in N_x0: store g as an element of N_x0.
		if xsp2co1ElemToN0(elem[:], out[:5]) != nil {
			return -1004
		}
		nMulElement((*N0Elem)(out[:5]), &gn, (*N0Elem)(out[:5]))
		lenA := nToWord((*N0Elem)(out[:5]), out)
		return int(lenA) + 0x100
	}

	// Otherwise store a word equal to g. Split off the
	// tag-t power so that gn lands in N_x0.
	e := nRightCosetNx0(gn[:])
	lenA := int(nToWord(&gn, gn[:]))
	if xsp2co1MulElemWord(elem[:], gn[:lenA]) != nil {
		return -1005
	}
	lenA = xsp2co1ElemToWord(elem[:], out)
	if lenA < 0 {
		return -1006
	}
	if lenA > 10 {
		return -1007
	}
	if e != 0 {
		out[lenA] = 0x50000000 + e
		lenA++
	}
	return lenA
}

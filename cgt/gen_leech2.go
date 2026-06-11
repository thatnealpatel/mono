package cgt

import (
	"errors"

	"patel.codes/cgt/generator"
)

// gen_leech2.go holds the field-independent helpers
// that the generated word-ABC operation (genOpWordABC
// in mm_op_p_gen.go) needs but that are not part of
// the per-modulus SWAR template: the G_{x0} prefix
// scan, the standard-subframe map, the tag-B/C
// extraction and the tag-ABC odd-cocode negation.
//
// Provenance (C sources reproduced here):
//
//	mmgroup/src/mmgroup/dev/generators/gen_leech.ske
//	    gen_leech2_prefix_Gx0   (~line 419)
//	    gen_leech2_map_std_subframe (~line 924)
//	mmgroup/src/mmgroup/dev/c_files/gen_leech.c
//	    TABLE_IN_MAP_STD_SUBFRAME, TABLE_OP_MAP_STD_SUBFRAME
//	mmgroup/src/mmgroup/dev/c_files/mm15_op_word.c
//	    extract_BC              (~line 180)
//	mmgroup/src/mmgroup/dev/c_files/mm15_op_pi.c
//	    mm_op15_delta_tag_ABC   (~line 1087)

// genLeech2PrefixGx0 returns the length of the longest
// prefix of g[:length] every prefix of which lies in
// G_{x0}. A word atom breaks the prefix iff its tag is
// 7 (illegal) or it is a tag-t atom with a power not
// divisible by 3. C gen_leech2_prefix_Gx0.
func genLeech2PrefixGx0(g []uint32, length int) int {
	for i := 0; i < length; i++ {
		tag := (g[i] >> 28) & 7
		if tag == 7 || (tag == 5 && (g[i]&0xfffffff)%3 != 0) {
			return i
		}
	}
	return length
}

// genLeech2MapStdSubframe computes the images of the
// standard-subframe generators of Q_{x0} under
// conjugation by g[:length] and stores the 24 results
// (x_Omega^g, x_{0,1}^g, ..., x_{0,23}^g) in a, in
// Leech lattice encoding. It returns the number of
// atoms of g that were processed, or a negative value
// on failure. C gen_leech2_map_std_subframe.
//
// a must have at least 24 entries.
func genLeech2MapStdSubframe(g []uint32, length int, a []uint32) int {
	length = genLeech2PrefixGx0(g, length)

	// q holds the 12 generators 1<<i (i<11) and Omega
	// (0x800000); conjugate them all by g at once.
	var q [12]uint32
	for i := 0; i < 11; i++ {
		q[i] = 1 << uint(i)
	}
	q[11] = 0x800000
	if genLeech2OpWordMany(q[:], g[:length]) != length {
		return -1
	}
	for i := 0; i < 11; i++ {
		a[tableInMapStdSubframe[i]] = q[i]
	}

	// Recover the remaining short cocode images as
	// products of already-known ones.
	for i := 0; i < 3*nOpsMapStdSubframe; i += 3 {
		op1 := a[tableOpMapStdSubframe[i]]
		op2 := a[tableOpMapStdSubframe[i+1]]
		a[tableOpMapStdSubframe[i+2]] = generator.Leech2Mul(op1, op2)
	}

	a[0] = q[11]
	return length
}

// nOpsMapStdSubframe is the number of recovery
// multiplications in the subframe map. C
// N_OPS_MAP_STD_SUBFRAME.
const nOpsMapStdSubframe = 51

// tableInMapStdSubframe maps the 11 directly conjugated
// generators to their destination slots in a. C
// TABLE_IN_MAP_STD_SUBFRAME / ShortCocode_InTable.
var tableInMapStdSubframe = [11]uint8{
	0x04, 0x08, 0x0c, 0x12, 0x0a, 0x09, 0x06, 0x05,
	0x03, 0x01, 0x02,
}

// tableOpMapStdSubframe lists the recovery
// multiplications: for each triple (i, j, k) the slot
// a[k] is set to a[i] * a[j] in Q_{x0}. C
// TABLE_OP_MAP_STD_SUBFRAME / ShortCocode_OpTable.
var tableOpMapStdSubframe = [3 * nOpsMapStdSubframe]uint8{
	0x08, 0x04, 0x10, 0x10, 0x0c, 0x14, 0x10, 0x12,
	0x10, 0x12, 0x0c, 0x0c, 0x0c, 0x08, 0x08, 0x0c,
	0x04, 0x0c, 0x08, 0x04, 0x04, 0x05, 0x0a, 0x12,
	0x12, 0x10, 0x12, 0x05, 0x09, 0x0d, 0x09, 0x0a,
	0x0b, 0x0b, 0x08, 0x0b, 0x0d, 0x0a, 0x17, 0x0d,
	0x0c, 0x0d, 0x17, 0x14, 0x17, 0x01, 0x0d, 0x0d,
	0x03, 0x17, 0x17, 0x05, 0x06, 0x07, 0x05, 0x04,
	0x05, 0x06, 0x0a, 0x0e, 0x0e, 0x0c, 0x0e, 0x03,
	0x0e, 0x0e, 0x07, 0x0a, 0x16, 0x16, 0x14, 0x16,
	0x01, 0x16, 0x16, 0x06, 0x09, 0x15, 0x06, 0x04,
	0x06, 0x07, 0x09, 0x11, 0x09, 0x08, 0x09, 0x07,
	0x04, 0x07, 0x02, 0x04, 0x04, 0x15, 0x0a, 0x13,
	0x15, 0x14, 0x15, 0x13, 0x10, 0x13, 0x02, 0x14,
	0x14, 0x01, 0x13, 0x13, 0x11, 0x0a, 0x0f, 0x0a,
	0x08, 0x0a, 0x11, 0x10, 0x11, 0x0f, 0x0c, 0x0f,
	0x02, 0x08, 0x08, 0x02, 0x0c, 0x0c, 0x02, 0x10,
	0x10, 0x03, 0x11, 0x11, 0x01, 0x03, 0x00, 0x02,
	0x01, 0x01, 0x02, 0x03, 0x02, 0x01, 0x03, 0x03,
	0x00, 0x0f, 0x0f, 0x00, 0x12, 0x12, 0x00, 0x15,
	0x15,
}

// genExtractBC reconstructs the tags B and C of v_out
// from the standard subframe a (24 Leech-encoded
// vectors) and the source vector v_in at modulus p. C
// extract_BC (mm{p}_op_word.c). It returns an error if
// any subframe vector fails to map to a short sparse
// index.
//
// For each pair i<j of subframe vectors the function
// reads the corresponding short coordinate of v_in
// (applying the Leech sign), stores it symmetrically in
// tag B at (i,j) and (j,i), then forms the product with
// a[0] (the Omega image) to obtain the matching tag-C
// coordinate. The walk a[j] = a[i+1]*a[j] advances the
// row generators in place, exactly as in C.
func genExtractBC(p int, vIn []uint64, a []uint32, vOut []uint64) error {
	s := genSwarFor(p)
	bs := s.genTagBOfs()
	cs := s.genTagCOfs()
	ts := s.genTagTOfs()

	// Zero tags B and C of the output (the diagonal and
	// any entry not written below must be zero).
	for i := bs; i < ts; i++ {
		vOut[i] = 0
	}

	for i := 0; i < 24; i++ {
		for j := i + 1; j < 24; j++ {
			c := a[j]
			val, ok := genSubframeValue(s, p, vIn, c)
			if !ok {
				return errExtractBC
			}
			genWriteEntry24(s, vOut, bs, i, j, val)
			genWriteEntry24(s, vOut, bs, j, i, val)

			c = generator.Leech2Mul(a[0], a[j])
			val, ok = genSubframeValue(s, p, vIn, c)
			if !ok {
				return errExtractBC
			}
			genWriteEntry24(s, vOut, cs, i, j, val)
			genWriteEntry24(s, vOut, cs, j, i, val)

			if j > i+1 {
				a[j] = generator.Leech2Mul(a[i+1], a[j])
			}
		}
	}
	return nil
}

// genSubframeValue reads the coordinate of v_in indexed
// by the Leech vector c (with its sign), reduced modulo
// p. It reports false if c does not map to a short
// sparse index. C extract_BC inner body.
func genSubframeValue(s *genSwar, p int, vIn []uint64, c uint32) (int, bool) {
	sparse := IndexLeech2ToSparse(c)
	if sparse == 0 {
		return 0, false
	}
	// Fold the Leech sign (bit 24 of c) into the low
	// bits of the sparse index: when set the extracted
	// value is XORed with p, i.e. negated modulo p.
	sparse |= (0 - ((c >> 24) & 1)) & uint32(p)
	v := int(mmvGetSparse(p, vIn, sparse) & uint32(p))
	return v, true
}

// errExtractBC reports a subframe vector that is not
// short (C extract_BC returning -1).
var errExtractBC = errors.New("cgt: extract_BC: subframe vector not short")

// genOpDeltaTagABC negates tag C of v in place when the
// cocode element d is odd (bit 0x800 set) and mode is
// zero; otherwise it does nothing. C
// mm{p}_op_delta_tag_ABC. This is the
// identity-permutation odd-delta case of genOpPiTagABC:
// no row permutation, only the tag-C sign flip.
func genOpDeltaTagABC(p int, v []uint64, d, mode int) {
	if mode != 0 || d&0x800 == 0 {
		return
	}
	s := genSwarFor(p)
	cs := s.genTagCOfs()
	for r := 0; r < 24; r++ {
		s.genNegateRow24(v, cs+r*s.wordsPer24)
	}
}

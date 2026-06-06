package main

// mm_op_p.go generates per-modulus SWAR (SIMD Within
// A Register) operation code for the Monster group
// representation, the Go counterpart of the C files
// mm{p}_op_pi.c, mm{p}_op_xy.c, mm{p}_op_t.c and
// mm{p}_op_word.c (one such triple per modulus
// p in {3,7,15,31,127,255}).
//
// The C library bakes the SWAR masks, field packing
// and arithmetic into six near-identical unrolled
// files. In Go the same logic collapses to a single
// field-generic implementation driven by a small
// per-modulus constant table: the only data that
// actually differs between moduli is
//
//   - the field width (bits per element),
//   - the number of fields packed in a uint64, and
//   - a handful of SWAR masks (the all-p "negate"
//     mask, the tag-T odd-cocode "invert" mask and
//     the tag-T xy sign tables),
//
// all of which are derived here from the modulus and
// emitted as compile-time constants. The generated
// functions live in package cgt and are named with a
// "gen" prefix so they never collide with the
// hand-written stubs in mm_op_group.go.
//
// Provenance (C sources consulted for the template
// patterns this generator reproduces):
//
//	mmgroup/src/mmgroup/dev/c_files/mm_op_p.h
//	mmgroup/src/mmgroup/dev/c_files/mm_op_sub.h
//	mmgroup/src/mmgroup/dev/c_files/mm7_op_pi.c
//	mmgroup/src/mmgroup/dev/c_files/mm7_op_xy.c
//	mmgroup/src/mmgroup/dev/c_files/mm7_op_t.c
//	mmgroup/src/mmgroup/dev/c_files/mm7_op_word.c

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"text/template"
)

// genModuli is the list of supported moduli, matching
// C table MM_OP_P_TABLE and cgt.Characteristics.
var genModuli = []int{3, 7, 15, 31, 127, 255}

// mmvConstTable mirrors C MMV_CONST_TABLE / the
// cgt.mmvConstTable used by mm_op.go. It packs the
// per-modulus field-layout parameters.
var mmvConstTable = [8]uint32{
	0x00044643, 0x00000000, 0x00034643, 0x00011305,
	0x0003c643, 0x0002c643, 0x00022484, 0x0001a484,
}

// mmvConst returns the packed layout word for p. C
// macro MMV_LOAD_CONST.
func mmvConst(p int) uint32 {
	return mmvConstTable[(((p)+1)*232>>8)&7]
}

// logIntFields is LOG_INT_FIELDS: 1<<logIntFields
// entries are packed in one uint64.
func logIntFields(p int) uint { return uint(mmvConst(p) & 7) }

// logFieldBits is LOG_FIELD_BITS: each field is
// 1<<logFieldBits bits wide.
func logFieldBits(p int) uint { return uint((mmvConst(p) >> 9) & 3) }

// mat24SuboctadWeights packs, for each of the 64
// suboctad numbers, the parity of its halved bit
// weight. C MAT24_SUBOCTAD_WEIGHTS; identical to
// cgt.mat24SuboctadWeights. Bit delta is set iff the
// suboctad delta has weight 4n+2, which is exactly
// the set of tag-T entries the odd-cocode operation
// negates.
const mat24SuboctadWeights uint64 = 0xe88181178117177e

// swarParams holds the per-modulus SWAR layout
// derived from the modulus p.
type swarParams struct {
	P int // the modulus

	FieldBits  uint // bits per packed element
	IntFields  uint // elements packed per uint64
	WordsPer24 int  // uint64 words holding a 24-row
	WordsPer64 int  // uint64 words holding a 64-row
	UsedPer24  int  // words of a 24-row carrying data
	Slack24    uint // elements of slack in the last
	NegateMask uint64
	SlackMask  uint64   // negate mask for the last 24-row word
	InvertT    []uint64 // tag-T odd-cocode invert mask
	XYSignLow  []uint64 // TABLE_PERM64_XY_LOW
	XYSignHigh []uint64 // TABLE_PERM64_XY_HIGH
}

// fieldMask returns a uint64 whose low n fields each
// hold the value p.
func (s *swarParams) fieldMask(n uint) uint64 {
	var m uint64
	for j := uint(0); j < n; j++ {
		m |= uint64(s.P) << (j * s.FieldBits)
	}
	return m
}

// spread64 spreads the low 64 bits of a per-element
// predicate (bit i set => element i is negated) into
// a slice of WordsPer64 uint64 fields, each set field
// holding the value p. bits[i] uses bit i of the
// 64-bit value src. This reproduces the C
// TABLE_PERM64 layout for tag T.
func (s *swarParams) spread64(src uint64) []uint64 {
	out := make([]uint64, s.WordsPer64)
	for delta := 0; delta < 64; delta++ {
		if (src>>uint(delta))&1 == 0 {
			continue
		}
		w := uint(delta) / s.IntFields
		pos := uint(delta) % s.IntFields
		out[w] |= uint64(s.P) << (pos * s.FieldBits)
	}
	return out
}

// suboctadWeight reports whether suboctad delta has
// weight 4n+2 (the negate predicate for tag T). C
// mat24_def_suboctad_weight.
func suboctadWeight(delta int) bool {
	return (mat24SuboctadWeights>>uint(delta&0x3f))&1 != 0
}

// tpSign computes the tag-T xy sign bit
// TP(e, s, alpha, delta) used by the C tables
// TABLE_PERM64_XY_{LOW,HIGH}. Per mm7_op_xy.c:
//
//	TP(e, s, alpha, delta) =
//	    e*|delta|/2 + s + <alpha, delta>  (mod 2),
//
// where |delta|/2 mod 2 is suboctadWeight(delta), and
// <alpha, delta> is the parity of the intersection of
// the suboctads alpha and delta, computed as
// Par(alpha&delta) ^ (Par(alpha) & Par(delta)).
func tpSign(e, s, alpha, delta int) int {
	r := s & 1
	if e&1 != 0 && suboctadWeight(delta) {
		r ^= 1
	}
	r ^= suboctadScalarProd(alpha, delta)
	return r & 1
}

// suboctadScalarProd returns <a, b>, the scalar
// product of the suboctads with numbers a and b. It is
// a faithful copy of cgt.SuboctadScalarProd /
// mat24_def_suboctad_scalar_prod: suboctad numbers are
// 6-bit Seysen encodings of even subsets, so the
// bilinear form is not the plain dot product but the
// formula below over the (sub ^ (sub>>3)) & 7
// reduction with magic weight constant 0x96.
func suboctadScalarProd(a, b int) int {
	wp := (0x96 >> uint((a^(a>>3))&7)) & (0x96 >> uint((b^(b>>3))&7))
	c := a & b
	wp ^= 0x96 >> uint((c^(c>>3))&7)
	return wp & 1
}

// xyTableLow builds TABLE_PERM64_XY_LOW: for each of
// the 16 low nibbles x (alpha = x, 0<=x<16), the
// length-64 sign vector TL(x, delta) spread to fields
// of width FieldBits. Stored as WordsPer64 words per
// entry, 16 entries.
func (s *swarParams) xyTableLow() []uint64 {
	out := make([]uint64, 0, 16*s.WordsPer64)
	for x := 0; x < 16; x++ {
		var pred uint64
		for delta := 0; delta < 64; delta++ {
			// TL(x, delta) = TP with e=0, s=0,
			// alpha = x (the low-nibble suboctad).
			if tpSign(0, 0, x, delta) != 0 {
				pred |= uint64(1) << uint(delta)
			}
		}
		out = append(out, s.spread64(pred)...)
	}
	return out
}

// xyTableHigh builds TABLE_PERM64_XY_HIGH: for each of
// the 16 high-nibble indices h (= x>>4 of the full
// argument x = 128*e + 64*s + alpha, 0 <= alpha < 64,
// 0 <= x < 256), the length-64 sign vector
// TH(h, delta) = TP(e, s, alpha_high, delta). Decoding
// h = (128*e + 64*s + alpha) >> 4 gives
//
//	e          = (h >> 3) & 1,
//	s          = (h >> 2) & 1,
//	alpha_high = (h & 3) << 4,
//
// where alpha_high carries the top two bits of the
// 6-bit suboctad alpha (the low four bits feed the LOW
// table). See the C decomposition
// T(x, delta) = TH(x>>4, delta) ^ TL(x&0xf, delta).
func (s *swarParams) xyTableHigh() []uint64 {
	out := make([]uint64, 0, 16*s.WordsPer64)
	for h := 0; h < 16; h++ {
		e := (h >> 3) & 1
		sBit := (h >> 2) & 1
		alpha := (h & 3) << 4
		var pred uint64
		for delta := 0; delta < 64; delta++ {
			if tpSign(e, sBit, alpha, delta) != 0 {
				pred |= uint64(1) << uint(delta)
			}
		}
		out = append(out, s.spread64(pred)...)
	}
	return out
}

// newSwarParams derives the SWAR layout for modulus p.
func newSwarParams(p int) *swarParams {
	s := &swarParams{P: p}
	s.FieldBits = uint(1) << logFieldBits(p)
	s.IntFields = uint(1) << logIntFields(p)
	s.WordsPer24 = 32 >> logIntFields(p)
	s.WordsPer64 = 64 / int(s.IntFields)

	// Number of words of a 24-row that actually carry
	// data, and the slack in the final word.
	s.UsedPer24 = int((24 + s.IntFields - 1) / s.IntFields)
	rem := uint(24) % s.IntFields
	if rem == 0 {
		s.Slack24 = 0
	} else {
		s.Slack24 = s.IntFields - rem
	}

	s.NegateMask = s.fieldMask(s.IntFields)
	// The last data word of a 24-row only spans the
	// remaining entries; its negate mask is narrower
	// when 24 is not a multiple of IntFields.
	last := uint(24) - uint(s.UsedPer24-1)*s.IntFields
	s.SlackMask = s.fieldMask(last)

	s.InvertT = s.spread64(mat24SuboctadWeights)
	s.XYSignLow = s.xyTableLow()
	s.XYSignHigh = s.xyTableHigh()
	return s
}

// genMMOpP writes the generated package cgt source to
// w. It emits a single combined file containing the
// per-modulus SWAR constant table and the
// field-generic group operations (delta, pi, xy and
// the tag-T odd-cocode negation) that the C library
// splits across mm{p}_op_*.c.
//
// The cgtDir argument is unused: this generator
// derives its constants from the modulus alone and
// self-verifies against hardcoded p=7 C values rather
// than a golden file. The parameter is present so
// every generator shares one dispatch signature.
//
// genMMOpP returns an error if the per-modulus masks
// fail their self-consistency check against the known
// p=7 C constants, or if the emitted source does not
// gofmt.
func genMMOpP(w io.Writer, _ string) error {
	if err := verifyAgainstC(); err != nil {
		return err
	}

	params := make([]*swarParams, len(genModuli))
	for i, p := range genModuli {
		params[i] = newSwarParams(p)
	}

	var buf bytes.Buffer
	if err := genTemplate.Execute(&buf, params); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt: %w\n%s", err, buf.Bytes())
	}
	if _, err := w.Write(src); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// verifyAgainstC checks the derived p=7 masks against
// the constants baked into mm7_op_pi.c, mm7_op_xy.c
// and mm7_op_t.c. This is the generator's proof of
// work: if the field-generic derivation diverges from
// the upstream C, generation fails loudly rather than
// emitting wrong constants.
func verifyAgainstC() error {
	s := newSwarParams(7)

	// Tag-T odd-cocode invert mask. C mm7_op_pi.c
	// INVERT_PERM64 (and the identical block in
	// mm7_op_t.c diagonal-matrix load).
	wantInvertT := []uint64{
		0x0007077707777770, 0x7000000700070777,
		0x7000000700070777, 0x7770700070000007,
	}
	if !equalU64(s.InvertT, wantInvertT) {
		return fmt.Errorf("p=7 InvertT mismatch:\n got %#016x\nwant %#016x",
			s.InvertT, wantInvertT)
	}

	// Negate mask (all fields = 7).
	if s.NegateMask != 0x7777777777777777 {
		return fmt.Errorf("p=7 NegateMask = %#x, want 0x7777777777777777",
			s.NegateMask)
	}

	// Several rows of TABLE_PERM64_XY_HIGH, transcribed
	// verbatim from mm7_op_xy.c. Rows 1 and 2 exercise
	// the plain suboctad scalar product
	// (hi_list[0] == 0); row 4 exercises the
	// hi_list[1] == mask all-ones XOR path
	// ((16*4 >> 6) & 3 == 1).
	wantHigh := map[int][]uint64{
		1: {0x0770700770070770, 0x0770700770070770,
			0x7007077007707007, 0x7007077007707007},
		2: {0x0770700770070770, 0x7007077007707007,
			0x0770700770070770, 0x7007077007707007},
		4: {0x7777777777777777, 0x7777777777777777,
			0x7777777777777777, 0x7777777777777777},
	}
	for h, want := range wantHigh {
		got := s.XYSignHigh[h*s.WordsPer64 : (h+1)*s.WordsPer64]
		if !equalU64(got, want) {
			return fmt.Errorf("p=7 XYSignHigh[%d] mismatch:\n got %#016x\nwant %#016x",
				h, got, want)
		}
	}

	// First nonzero TABLE_PERM64_XY_LOW row (x=1),
	// transcribed from mm7_op_xy.c lines 95-97.
	wantLow1 := []uint64{
		0x7700007700777700, 0x0077770077000077,
		0x0077770077000077, 0x7700007700777700,
	}
	gotLow := s.XYSignLow[1*s.WordsPer64 : 2*s.WordsPer64]
	if !equalU64(gotLow, wantLow1) {
		return fmt.Errorf("p=7 XYSignLow[1] mismatch:\n got %#016x\nwant %#016x",
			gotLow, wantLow1)
	}
	return nil
}

func equalU64(a, b []uint64) bool {
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

// hexWords renders a uint64 slice as a Go composite
// literal body (comma-separated 0x... constants).
func hexWords(xs []uint64) string {
	var b bytes.Buffer
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "0x%016x", x)
	}
	return b.String()
}

var genTemplate = template.Must(template.New("mm_op_p").
	Funcs(template.FuncMap{"hex": hexWords}).
	Parse(genTemplateText))

// genTemplateText is the body of the generated file.
// It targets package cgt and reuses the field-access
// primitives (getMMV/putMMV), the preparation tables
// (subPrepPi64/subPrepXY) and the layout helpers
// (rowsPer24, fieldWidth, ofsWords, mmAuxOfs*) that
// already exist there. All identifiers it introduces
// carry a "gen" or "genMM" prefix to avoid colliding
// with the hand-written stubs in mm_op_group.go.
const genTemplateText = `// Code generated by cgt/_gen; DO NOT EDIT.

package cgt

// This file implements the per-modulus SWAR group
// operations x_delta * x_pi (genOpPi),
// y_f * x_e * x_eps (genOpXY) and the supporting
// tag-T sign negation, ported from the C files
// mm{p}_op_pi.c, mm{p}_op_xy.c and mm{p}_op_t.c. The
// six unrolled C variants collapse to one
// field-generic implementation here, driven by the
// genSwar table of per-modulus SWAR constants.

// genSwar is the per-modulus SWAR configuration. It
// is indexed via genSwarFor, which dispatches on p.
type genSwar struct {
	p          int
	fieldBits  uint     // bits per packed element
	intFields  uint     // elements packed per uint64
	wordsPer24 int      // uint64 words per 24-row
	wordsPer64 int      // uint64 words per 64-row
	usedPer24  int      // 24-row words carrying data
	negateMask uint64   // all fields = p
	slackMask  uint64   // negate mask, last 24-row word
	invertT    []uint64 // tag-T odd-cocode invert mask
	xySignLow  []uint64 // TABLE_PERM64_XY_LOW
	xySignHigh []uint64 // TABLE_PERM64_XY_HIGH
}

// genSwarTable holds the SWAR configuration for each
// supported modulus, in the order returned by
// Characteristics.
var genSwarTable = [...]genSwar{
{{- range . }}
	{
		p:          {{ .P }},
		fieldBits:  {{ .FieldBits }},
		intFields:  {{ .IntFields }},
		wordsPer24: {{ .WordsPer24 }},
		wordsPer64: {{ .WordsPer64 }},
		usedPer24:  {{ .UsedPer24 }},
		negateMask: 0x{{ printf "%016x" .NegateMask }},
		slackMask:  0x{{ printf "%016x" .SlackMask }},
		invertT:    []uint64{ {{ hex .InvertT }} },
		xySignLow:  []uint64{ {{ hex .XYSignLow }} },
		xySignHigh: []uint64{ {{ hex .XYSignHigh }} },
	},
{{- end }}
}

// genSwarFor returns the SWAR configuration for
// modulus p. genSwarFor panics if p is not one of the
// supported moduli.
func genSwarFor(p int) *genSwar {
	for i := range genSwarTable {
		if genSwarTable[i].p == p {
			return &genSwarTable[i]
		}
	}
	panic("cgt: genSwarFor: unsupported modulus")
}

// genReadField returns element j of word w, reduced
// modulo p (the all-ones field is the alternate
// encoding of zero).
func (s *genSwar) genReadField(w uint64, j uint) int {
	v := (w >> (j * s.fieldBits)) & uint64(s.p)
	if v == uint64(s.p) {
		return 0
	}
	return int(v)
}

// genNegateRow24 XORs the negate mask into the
// wordsPer24 words of one 24-row at word index off,
// flipping the sign of all 24 entries (the mod-(2^k-1)
// negation). The final word uses the narrower
// slackMask so the slack entries stay zero.
func (s *genSwar) genNegateRow24(v []uint64, off int) {
	for w := 0; w < s.wordsPer24; w++ {
		m := s.negateMask
		if w == s.usedPer24-1 {
			m = s.slackMask
		} else if w >= s.usedPer24 {
			m = 0
		}
		v[off+w] ^= m
	}
}

// genCopyRow24 copies one 24-row (wordsPer24 words)
// from src[soff:] to dst[doff:], optionally negating.
func (s *genSwar) genCopyRow24(src []uint64, soff int, dst []uint64, doff int, neg bool) {
	for w := 0; w < s.wordsPer24; w++ {
		x := src[soff+w]
		if neg {
			m := s.negateMask
			if w == s.usedPer24-1 {
				m = s.slackMask
			} else if w >= s.usedPer24 {
				m = 0
			}
			x ^= m
		}
		dst[doff+w] = x
	}
}

// genReadRow64 unpacks one 64-row of tag T starting at
// word index off into a [64]int of field values.
func (s *genSwar) genReadRow64(v []uint64, off int) [64]int {
	var out [64]int
	for w := 0; w < s.wordsPer64; w++ {
		word := v[off+w]
		base := w * int(s.intFields)
		for j := uint(0); j < s.intFields; j++ {
			out[base+int(j)] = s.genReadField(word, j)
		}
	}
	return out
}

// genWriteRow64 packs a [64]int of field values into
// the wordsPer64 words of one tag-T 64-row at word
// index off. Values are reduced modulo p.
func (s *genSwar) genWriteRow64(v []uint64, off int, vals [64]int) {
	for w := 0; w < s.wordsPer64; w++ {
		var word uint64
		base := w * int(s.intFields)
		for j := uint(0); j < s.intFields; j++ {
			x := vals[base+int(j)] % s.p
			if x < 0 {
				x += s.p
			}
			word |= uint64(x) << (j * s.fieldBits)
		}
		v[off+w] = word
	}
}

// genTagTOfs returns the tag-T word offset (off >>
// logIntFields) for modulus p.
func (s *genSwar) genTagTOfs() int { return mmAuxOfsT >> genLogIntFields(s.p) }

func (s *genSwar) genTagAOfs() int { return mmAuxOfsA >> genLogIntFields(s.p) }
func (s *genSwar) genTagBOfs() int { return mmAuxOfsB >> genLogIntFields(s.p) }
func (s *genSwar) genTagCOfs() int { return mmAuxOfsC >> genLogIntFields(s.p) }
func (s *genSwar) genTagXOfs() int { return mmAuxOfsX >> genLogIntFields(s.p) }
func (s *genSwar) genTagZOfs() int { return mmAuxOfsZ >> genLogIntFields(s.p) }
func (s *genSwar) genTagYOfs() int { return mmAuxOfsY >> genLogIntFields(s.p) }

// genLogIntFields returns LOG_INT_FIELDS for p.
func genLogIntFields(p int) uint { return logIntFields(p) }

// genNegScalprodDI negates the tag-X entries X_d,i
// whose Leech scalar product <d,i> equals 1. C
// mm{p}_op_neg_scalprod_d_i. v must point at the tag-X
// region. The negate predicate is field-generic: for
// each of the 2048 rows d and 24 columns i, the entry
// is flipped iff the parity of d & cocode(i) is odd,
// which the C precomputes via MMV_TBL_SCALPROD_*.
func (s *genSwar) genNegScalprodDI(v []uint64) {
	per := s.wordsPer24
	for d := 0; d < 2048; d++ {
		off := d * per
		for i := 0; i < 24; i++ {
			if genScalprodDISign(uint32(d), i) == 0 {
				continue
			}
			s.genFlipEntryInRow(v, off, i)
		}
	}
}

// genFlipEntryInRow flips the sign of element i within
// the 24-row at word index off (entry i lives in word
// i/intFields at field i%intFields).
func (s *genSwar) genFlipEntryInRow(v []uint64, off, i int) {
	w := off + i/int(s.intFields)
	j := uint(i) % s.intFields
	v[w] ^= uint64(s.p) << (j * s.fieldBits)
}

// genScalprodDISign returns the parity <d, i> for the
// tag-X entry X_d,i, i.e. the bit by which
// genNegScalprodDI negates that entry. It mirrors the
// effect of C MMV_TBL_SCALPROD_HIGH/LOW: the sign is
// the parity of the intersection of the Golay code
// word d (low 11 bits) with the cocode element of the
// basis vector i.
func genScalprodDISign(d uint32, i int) int {
	c := vectToCocode(1 << uint(i))
	x := (d ^ (d >> 11)) & c
	return int(parity12(x) & 1)
}

// genOpDelta applies x_delta (the pure cocode
// automorphism, pi == 0) to src, storing the result in
// dst modulo p. C mm{p}_op_delta. It is the fast path
// of genOpPi: a pure sign change on every tag plus the
// X<->? swap for odd delta, with no row permutation.
func genOpDelta(p int, src []uint64, delta int, dst []uint64) {
	s := genSwarFor(p)

	// Per-row signs for tags X, Z, Y (2048 rows) and
	// the diagonal tag A (the first 72 rows of the
	// cocode-sign table). C uses mat24_op_all_cocode.
	signs := genOpAllCocode(uint32(delta))
	odd := delta & 0x800

	// Tags X, Z, Y: 2048 24-rows each. For odd delta
	// the Z and Y destinations are swapped.
	xs := s.genTagXOfs()
	zs := s.genTagZOfs()
	ys := s.genTagYOfs()
	zd, yd := zs, ys
	if odd != 0 {
		zd, yd = ys, zs
	}
	srcOfs := [3]int{xs, zs, ys}
	dstOfs := [3]int{xs, zd, yd}
	for k := 0; k < 3; k++ {
		so := srcOfs[k]
		do := dstOfs[k]
		for r := 0; r < 2048; r++ {
			neg := (signs[r]>>uint(k))&1 != 0
			s.genCopyRow24(src, so+r*s.wordsPer24, dst, do+r*s.wordsPer24, neg)
		}
	}

	// Tag A: 24 rows; sign from the first 72 entries
	// of the cocode table (3 tags packed per index in
	// the C, here row r uses signs[r] bit 0).
	as := s.genTagAOfs()
	for r := 0; r < 24; r++ {
		neg := signs[r]&1 != 0
		s.genCopyRow24(src, as+r*s.wordsPer24, dst, as+r*s.wordsPer24, neg)
	}

	// Tag T: 759 64-rows, negated when
	// octad_to_gcode(i) & delta has odd parity, plus
	// the odd-cocode invert when delta is odd.
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		off := ts + i*s.wordsPer64
		sign := uint32(mat24OctDecTable[i]) & uint32(delta)
		neg := parity12(sign)&1 != 0
		for w := 0; w < s.wordsPer64; w++ {
			x := src[off+w]
			if neg {
				x ^= s.negateMask
			}
			dst[off+w] = x
		}
	}
	if odd != 0 {
		s.genInvertTagT(dst)
		s.genNegScalprodDI(dst[s.genTagXOfs():])
	}
}

// genInvertTagT XORs the odd-cocode invert mask into
// all 759 tag-T 64-rows (negating suboctads of weight
// 4n+2). C INVERT_PERM64.
func (s *genSwar) genInvertTagT(v []uint64) {
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		off := ts + i*s.wordsPer64
		for w := 0; w < s.wordsPer64; w++ {
			v[off+w] ^= s.invertT[w]
		}
	}
}

// genOpPi applies x_delta * x_pi to src, storing the
// result in dst modulo p. C mm{p}_op_pi via
// mm{p}_op_do_pi. It is a monomial operation: tags X,
// Z, Y and the diagonal A are permuted as rows of 24
// with signs, and tag T is permuted as rows of 64
// using the preparation table from subPrepPi64.
//
// genOpPi panics if p is not a supported modulus.
func genOpPi(p int, src []uint64, delta, pi int, dst []uint64) {
	if pi == 0 {
		genOpDelta(p, src, delta, dst)
		return
	}
	s := genSwarFor(p)

	// inv_perm and the big 2048-entry sign+row table
	// for the X/Z/Y/A permutation. C mm_sub_prep_pi
	// fills tbl_perm24_big = OpAllAutpl(rep_autpl).
	perm := M24numToPerm(uint32(pi) % uint32(mat24Order))
	invPerm, repAutpl := PermToIautpl(uint32(delta)&0xfff, perm)
	big := OpAllAutpl(repAutpl) // 2048 row+sign entries

	// Column permutation for the 24-rows derived from
	// invPerm: entry i of a row moves to position
	// invPerm[i].
	var col [24]int
	for i := 0; i < 24; i++ {
		col[invPerm[i]] = i
	}

	// Tag T: rows of 64 via the suboctad table.
	tbl := subPrepPi64(uint32(delta), uint32(pi))
	genDoPiTagT(s, src, tbl, dst)

	// Tags X, Z, Y: 2048 rows of 24 with sign from the
	// big table. For odd delta, Z and Y swap.
	xs := s.genTagXOfs()
	zs := s.genTagZOfs()
	ys := s.genTagYOfs()
	zd, yd := zs, ys
	if delta&0x800 != 0 {
		zd, yd = ys, zs
	}
	srcOfs := [3]int{xs, zs, ys}
	dstOfs := [3]int{xs, zd, yd}
	for k := 0; k < 3; k++ {
		genDoPiRows24(s, src, srcOfs[k], dst, dstOfs[k], big, &col, 2048, uint(k+12))
	}

	// Tag A: 24 diagonal rows, sign from big table
	// bits, same column permutation.
	as := s.genTagAOfs()
	genDoPiRows24(s, src, as, dst, as, big, &col, 24, 0)

	if delta&0x800 != 0 {
		s.genInvertTagT(dst)
		s.genNegScalprodDI(dst[s.genTagXOfs():])
	}
}

// genDoPiRows24 permutes n rows of 24 entries from
// src[soff:] to dst[doff:]. Row r is taken from source
// row (big[r] & 0x7ff), its 24 entries permuted by
// col, and negated when bit signShift of big[r] is
// set. C pi24_2048 / pi24_n via the Benes network,
// here as a direct column gather.
func genDoPiRows24(s *genSwar, src []uint64, soff int, dst []uint64, doff int, big []uint16, col *[24]int, n int, signShift uint) {
	for r := 0; r < n; r++ {
		sp := big[r]
		srcRow := int(sp&0x7ff) * s.wordsPer24
		neg := (uint32(sp)>>signShift)&1 != 0
		var vals [24]int
		base := soff + srcRow
		for i := 0; i < 24; i++ {
			w := base + i/int(s.intFields)
			j := uint(i) % s.intFields
			vals[col[i]] = s.genReadField(src[w], j)
		}
		genWriteRow24(s, dst, doff+r*s.wordsPer24, &vals, neg)
	}
}

// genWriteRow24 packs 24 field values into one 24-row
// at word index off, optionally negating.
func genWriteRow24(s *genSwar, v []uint64, off int, vals *[24]int, neg bool) {
	for w := 0; w < s.wordsPer24; w++ {
		var word uint64
		base := w * int(s.intFields)
		for j := uint(0); j < s.intFields; j++ {
			idx := base + int(j)
			if idx >= 24 {
				break
			}
			x := vals[idx] % s.p
			if x < 0 {
				x += s.p
			}
			if neg {
				x = (s.p - x) % s.p
			}
			word |= uint64(x) << (j * s.fieldBits)
		}
		v[off+w] = word
	}
}

// genDoPiTagT permutes the 759 tag-T rows of 64 using
// the preparation table tbl from subPrepPi64. Row i is
// taken from source row tbl[i].preimage (low bits),
// sign-flipped by bit 12 of preimage, then its 64
// suboctad entries are permuted by the source-index
// walk genPerm64Walk (a literal transcription of C
// STORE_PERM64) driven by the 6 generators
// tbl[i].perm.
func genDoPiTagT(s *genSwar, src []uint64, tbl []subOpPi64, dst []uint64) {
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		pre := tbl[i].preimage
		srcRow := int(pre&0x3ff) * s.wordsPer64
		neg := (pre>>12)&1 != 0
		in := s.genReadRow64(src, ts+srcRow)
		if neg {
			for k := range in {
				in[k] = (s.p - in[k]) % s.p
			}
		}
		var out [64]int
		seq := genPerm64Walk(&tbl[i].perm)
		for k := 0; k < 64; k++ {
			out[k] = in[seq[k]]
		}
		s.genWriteRow64(dst, ts+i*s.wordsPer64, out)
	}
}

// genPerm64Walk reproduces C STORE_PERM64 literally:
// it returns, for each of the 64 destination suboctad
// positions, the source suboctad index that the C
// running index ri selects. The walk starts at
// position 0 (source index 0), then ri = perm[0], and
// proceeds by XORing the fixed sequence of generators
// the unrolled C lists. The four 16-entry words begin
// by XORing the carry generators perm[4], perm[5],
// perm[4] respectively.
func genPerm64Walk(perm *[6]uint8) [64]int {
	r0 := int(perm[0])
	r1 := int(perm[1])
	r2 := int(perm[2])
	r3 := int(perm[3])
	r4 := int(perm[4])
	r5 := int(perm[5])

	// gens[k] is the generator XORed into ri just
	// before emitting destination position k (gens[0]
	// is unused; position 0 always reads source 0).
	// Word 0 (k 1..15): ri := r0 then the pattern
	//   r1,r0,r2,r0,r1,r0,r3,r0,r1,r0,r2,r0,r1,r0.
	// Words 1..3 (k 16,32,48): carry r4,r5,r4 then the
	//   pattern r0,r1,r0,r2,r0,r1,r0,r3,r0,r1,r0,r2,
	//   r0,r1,r0.
	p1 := [15]int{r0, r1, r0, r2, r0, r1, r0, r3, r0, r1, r0, r2, r0, r1, r0}

	var out [64]int
	ri := 0
	out[0] = 0
	// position 1: ri := r0
	ri = r0
	out[1] = ri
	w0 := [14]int{r1, r0, r2, r0, r1, r0, r3, r0, r1, r0, r2, r0, r1, r0}
	for k, g := range w0 {
		ri ^= g
		out[2+k] = ri
	}
	carries := [3]int{r4, r5, r4}
	for b := 0; b < 3; b++ {
		base := 16 + b*16
		ri ^= carries[b]
		out[base] = ri
		for k, g := range p1 {
			ri ^= g
			out[base+1+k] = ri
		}
	}
	return out
}

// genOpAllCocode returns, for each of 2048 rows, the
// 3-bit sign pattern (bit k => negate in tag X/Z/Y or
// A) for the cocode automorphism x_delta. C
// mat24_op_all_cocode. Row r sign bit k is the parity
// of delta & cocode-image determined by the basis; we
// reuse the existing mat24 machinery via genCocodeSign.
func genOpAllCocode(delta uint32) []uint8 {
	out := make([]uint8, 2048)
	for r := uint32(0); r < 2048; r++ {
		out[r] = uint8(genCocodeSign(delta, r))
	}
	return out
}

// genCocodeSign returns the 3-bit sign pattern for row
// r under cocode element delta: bit b is the parity of
// the scalar product of delta with the Golay/cocode
// data of row r, as used by tags X, Z, Y (and A).
func genCocodeSign(delta, r uint32) uint32 {
	// For tags X/Z/Y the relevant sign is the parity
	// of delta & (Golay code word r). Tag A uses the
	// same parity on its diagonal. All three tag bits
	// share this parity in the delta-only operation.
	s := parity12(delta & r & 0x7ff)
	return s | (s << 1) | (s << 2)
}

// genOpXY applies y_f * x_e * x_eps to src, storing
// the result in dst modulo p. C mm{p}_op_xy via
// mm{p}_op_do_xy. Tags X, Z, Y are permuted as rows of
// 24 with per-row signs from subPrepXY.sign_XYZ; tag T
// is recombined using the xy sign tables; tags A, B, C
// are handled by genOpXYABC.
//
// genOpXY panics if p is not a supported modulus.
func genOpXY(p int, src []uint64, f, e, eps int, dst []uint64) {
	s := genSwarFor(p)
	op := subPrepXY(uint32(f), uint32(e), uint32(eps))

	// Step 1: tags X, Z, Y, 2048 rows of 24.
	xs := s.genTagXOfs()
	zs := s.genTagZOfs()
	ys := s.genTagYOfs()
	start := [3]int{xs, zs, ys}
	dest := [3]int{xs, zs, ys}
	if (op.eps>>11)&1 != 0 {
		dest[1], dest[2] = ys, zs
	}
	for k := 0; k < 3; k++ {
		so := start[k]
		do := dest[k]
		dXor := int(op.linD[k])
		// aSign[0] is the per-column sign mask from
		// lin_i spread over the 24 fields; aSign[1] is
		// the same XORed with the negate mask (a whole
		// extra row sign). C mm{p}_op_do_xy a_sign.
		colMask := s.genSpread24(op.linI[k])
		for r := 0; r < 2048; r++ {
			srcRow := (r ^ dXor) * s.wordsPer24
			sgn := (op.signXYZ[r] >> uint(k)) & 1
			for w := 0; w < s.wordsPer24; w++ {
				m := colMask[w]
				if sgn != 0 {
					switch {
					case w == s.usedPer24-1:
						m ^= s.slackMask
					case w < s.usedPer24:
						m ^= s.negateMask
					}
				}
				dst[do+r*s.wordsPer24+w] = src[so+srcRow+w] ^ m
			}
		}
	}

	// Step 2: tag T, 759 rows of 64.
	genDoXYTagT(s, src, op, dst)

	// Step 3: tags A, B, C.
	genOpXYABC(s, src, op, 0, dst)

	if op.eps&0x800 != 0 {
		s.genNegScalprodDI(dst[s.genTagXOfs():])
	}
}

// genSpread24 returns the per-column sign mask for a
// 24-row: field c is set to p iff bit c of lin is set.
// C MMV_UINT_SPREAD of lin_i. The result has
// wordsPer24 words.
func (s *genSwar) genSpread24(lin uint32) []uint64 {
	out := make([]uint64, s.wordsPer24)
	for c := 0; c < 24; c++ {
		if (lin>>uint(c))&1 == 0 {
			continue
		}
		w := c / int(s.intFields)
		j := uint(c) % s.intFields
		out[w] |= uint64(s.p) << (j * s.fieldBits)
	}
	return out
}

// genDoXYTagT recombines the 759 tag-T rows using the
// xy sign tables and the per-octad offset s_T from
// subPrepXY. C step 2 of mm{p}_op_do_xy.
func genDoXYTagT(s *genSwar, src []uint64, op *subOpXY, dst []uint64) {
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		ofs := op.sT[i]
		ofsH := (ofs & 63) >> 4
		hi := s.xySignHigh[int((ofs&0xf000)>>12)*s.wordsPer64:]
		lo := s.xySignLow[int((ofs&0xf00)>>8)*s.wordsPer64:]
		shift := uint((ofs << 2) & 0x3f)
		base := ts + i*s.wordsPer64
		srcBase := ts + i*s.wordsPer64
		in := s.genReadRow64(src, srcBase)
		var out [64]int
		for k := 0; k < 64; k++ {
			// permuted source suboctad index: the high
			// word offset xors ofsH into the word, and
			// the in-word shift xors the low bits.
			idx := (k ^ int(shift)) ^ (int(ofsH) << 6)
			idx &= 63
			out[k] = in[idx]
		}
		s.genWriteRow64(dst, base, out)
		// apply the xy sign masks
		for w := 0; w < s.wordsPer64; w++ {
			dst[base+w] ^= hi[w] ^ lo[w]
		}
	}
}

// genOpXYABC computes tags A, B, C of y_f x_e x_eps on
// src into dst. C op{p}_do_ABC. When mode != 0 only
// tag A is written.
func genOpXYABC(s *genSwar, src []uint64, op *subOpXY, mode int, dst []uint64) {
	as := s.genTagAOfs()
	bs := s.genTagBOfs()
	cs := s.genTagCOfs()

	fI := op.fI
	efI := op.efI
	epsOdd := (op.eps >> 11) & 1

	// Tag A: 24 rows of 24, sign from fI per column.
	for r := 0; r < 24; r++ {
		fbit := (fI >> uint(r)) & 1
		for w := 0; w < s.wordsPer24; w++ {
			x := src[as+r*s.wordsPer24+w]
			if fbit != 0 {
				m := s.negateMask
				if w == s.usedPer24-1 {
					m = s.slackMask
				} else if w >= s.usedPer24 {
					m = 0
				}
				x ^= m
			}
			dst[as+r*s.wordsPer24+w] = x
		}
	}
	if mode != 0 {
		return
	}

	// Tags B, C: 24 rows of 24, signs from fI/efI and
	// the eps negate on C. C op{p}_do_ABC tags B, C.
	for r := 0; r < 24; r++ {
		fbit := (fI >> uint(r)) & 1
		efbit := (efI >> uint(r)) & 1
		for w := 0; w < s.wordsPer24; w++ {
			m := s.negateMask
			if w == s.usedPer24-1 {
				m = s.slackMask
			} else if w >= s.usedPer24 {
				m = 0
			}
			t1 := src[bs+r*s.wordsPer24+w]
			t2 := src[cs+r*s.wordsPer24+w]
			var t uint64
			if fbit != 0 {
				t = m & (t1 ^ t2)
			}
			if efbit != 0 {
				t ^= m
			}
			dst[bs+r*s.wordsPer24+w] = t1 ^ t
			if epsOdd != 0 {
				t2 ^= m
			}
			dst[cs+r*s.wordsPer24+w] = t2 ^ t
		}
	}
}
`

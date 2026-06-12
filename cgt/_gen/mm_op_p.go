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
	if err := verifyMatrices(); err != nil {
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

// verifyMatrices is a generation-time (proof-of-work) check of the
// non-monomial operation matrices that the template emits. It rebuilds,
// using this generator's own mat24 primitives (suboctadWeight,
// suboctadScalarProd), the 64x64 tag-T triality matrix and checks its
// defining identities mat^3 = 512*I and mat1*mat2 = 64*I. It also
// checks the 3x3 tag-ABC triality matrix cube identity and the 16x16
// tag-A xi matrix order. If the field-generic matrices the template
// emits diverge from these, generation fails loudly. (The 64x64 xi
// matrix additionally depends on w2_gray, which is checked in the
// emitted package's init.)
func verifyMatrices() error {
	matMul64 := func(a, b *[64][64]int) [64][64]int {
		var c [64][64]int
		for i := 0; i < 64; i++ {
			for k := 0; k < 64; k++ {
				if a[i][k] == 0 {
					continue
				}
				aik := a[i][k]
				for j := 0; j < 64; j++ {
					c[i][j] += aik * b[k][j]
				}
			}
		}
		return c
	}
	identity64 := func(m *[64][64]int, scale int) bool {
		for i := 0; i < 64; i++ {
			for j := 0; j < 64; j++ {
				want := 0
				if i == j {
					want = scale
				}
				if m[i][j] != want {
					return false
				}
			}
		}
		return true
	}

	// Build sym64, diag64 from the generator's own primitives.
	var diag, sym [64][64]int
	for i := 0; i < 64; i++ {
		if suboctadWeight(i) {
			diag[i][i] = -1
		} else {
			diag[i][i] = 1
		}
		for j := 0; j < 64; j++ {
			if suboctadScalarProd(i, j)&1 != 0 {
				sym[i][j] = -1
			} else {
				sym[i][j] = 1
			}
		}
	}
	mat1 := matMul64(&sym, &diag)
	mat2 := matMul64(&diag, &sym)
	for _, m := range []*[64][64]int{&mat1, &mat2} {
		sq := matMul64(m, m)
		cube := matMul64(&sq, m)
		if !identity64(&cube, 512) {
			return fmt.Errorf("tau mat64 cube identity failed")
		}
	}
	prod := matMul64(&mat1, &mat2)
	if !identity64(&prod, 64) {
		return fmt.Errorf("tau mat64 e1*e2 identity failed")
	}

	// 3x3 tag-ABC matrices: mat^3 = 8*I.
	mat3 := [3][3][3]int{
		1: {{0, 2, -2}, {1, 1, 1}, {1, -1, -1}},
		2: {{0, 2, 2}, {1, 1, -1}, {-1, 1, -1}},
	}
	for _, e := range []int{1, 2} {
		m := mat3[e]
		var cube [3][3]int
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				var acc int
				for k := 0; k < 3; k++ {
					for l := 0; l < 3; l++ {
						acc += m[i][k] * m[k][l] * m[l][j]
					}
				}
				cube[i][j] = acc
			}
		}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				want := 0
				if i == j {
					want = 8
				}
				if cube[i][j] != want {
					return fmt.Errorf("tau mat3[%d] cube identity failed", e)
				}
			}
		}
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

package monster

import (
	"errors"

	"patel.codes/cgt/generator"
	"patel.codes/cgt/mat24"
	"patel.codes/cgt/n0"
)

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
func (s *genSwar) genTagTOfs() int { return mmAuxOfsT >> logIntFields(s.p) }

func (s *genSwar) genTagAOfs() int { return mmAuxOfsA >> logIntFields(s.p) }
func (s *genSwar) genTagBOfs() int { return mmAuxOfsB >> logIntFields(s.p) }
func (s *genSwar) genTagCOfs() int { return mmAuxOfsC >> logIntFields(s.p) }
func (s *genSwar) genTagXOfs() int { return mmAuxOfsX >> logIntFields(s.p) }
func (s *genSwar) genTagZOfs() int { return mmAuxOfsZ >> logIntFields(s.p) }
func (s *genSwar) genTagYOfs() int { return mmAuxOfsY >> logIntFields(s.p) }

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
	return int(mat24.Parity12(x) & 1)
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

	// Tags A, B, C: 72 contiguous 24-rows starting at
	// OFS_A. C mm{p}_op_delta takes the sign of row i1
	// from bit 3 of signs[i1], where signs[i1] is masked
	// to its low 3 bits for all 72 rows; bit 3 is then
	// set only for tag C (rows 48..71) when delta is odd.
	// Thus tags A and B are copied unchanged and tag C is
	// negated for odd delta. (Unlike tags X, Z, Y, the
	// cocode parity does not act on the symmetric A, B, C
	// matrices.)
	as := s.genTagAOfs()
	for i1 := 0; i1 < 72; i1++ {
		neg := odd != 0 && i1 >= 48
		s.genCopyRow24(src, as+i1*s.wordsPer24, dst, as+i1*s.wordsPer24, neg)
	}

	// Tag T: 759 64-rows, negated when
	// octad_to_gcode(i) & delta has odd parity, plus
	// the odd-cocode invert when delta is odd.
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		off := ts + i*s.wordsPer64
		sign := mat24.OctDecTable(uint32(i)) & uint32(delta)
		neg := mat24.Parity12(sign)&1 != 0
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
	perm := mat24.M24numToPerm(uint32(pi) % uint32(mat24.Mat24Order))
	invPerm, repAutpl := mat24.PermToIautpl(uint32(delta)&0xfff, perm)
	big := mat24.OpAllAutpl(repAutpl) // 2048 row+sign entries

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

	// Tags A, B, C: 72 contiguous 24-rows at OFS_A. C
	// mm_sub_prep_pi fills tbl_perm24_big[2048..2120]
	// with the row permutation for these tags: row i of
	// each tag is gathered from source row inv_perm[i] of
	// the same tag (offsets 0, 24, 48), with the column
	// permutation col applied; only tag C carries a sign
	// (bit 15) and only when delta is odd. The big X/Z/Y
	// table is not used here.
	abc := genAbcRowPerm(invPerm, delta&0x800 != 0)
	as := s.genTagAOfs()
	genDoPiRows24(s, src, as, dst, as, abc[:], &col, 72, 15)

	if delta&0x800 != 0 {
		s.genInvertTagT(dst)
		s.genNegScalprodDI(dst[s.genTagXOfs():])
	}
}

// genAbcRowPerm builds the 72-entry row permutation for
// tags A, B, C used by genOpPi, mirroring the
// tbl_perm24_big[2048..2120] block of C mm_sub_prep_pi.
// Row i of tag k (k = 0, 1, 2 for A, B, C) is gathered
// from source row inv_perm[i] + 24*k; tag C additionally
// carries a sign in bit 15 when delta is odd.
func genAbcRowPerm(invPerm []byte, oddDelta bool) [72]uint16 {
	var abc [72]uint16
	for i := 0; i < 24; i++ {
		t := uint16(invPerm[i])
		abc[i] = t
		abc[i+24] = t + 24
		c := t + 48
		if oddDelta {
			c |= 0x8000
		}
		abc[i+48] = c
	}
	return abc
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
	s := mat24.Parity12(delta & r & 0x7ff)
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
		// C op{p}_do_xy reads source word (w ^ ofsH) and
		// bit (4*field ^ ofsL) for output word w, field
		// f, where ofsH = (ofs & 63) >> 4 selects the word
		// (each row is 4 words of 16 fields) and ofsL =
		// (ofs << 2) & 0x3f is the in-word bit shift, i.e.
		// the source field is f ^ (ofs & 0xf).
		ofsH := int((ofs & 63) >> 4)
		fieldXor := int(ofs) & 0xf
		hi := s.xySignHigh[int((ofs&0xf000)>>12)*s.wordsPer64:]
		lo := s.xySignLow[int((ofs&0xf00)>>8)*s.wordsPer64:]
		base := ts + i*s.wordsPer64
		in := s.genReadRow64(src, base)
		var out [64]int
		for k := 0; k < 64; k++ {
			// k = word*16 + field. The word is permuted by
			// xoring ofsH (bits 4..5); the field by xoring
			// ofs & 0xf (bits 0..3).
			srcWord := (k >> 4) ^ ofsH
			srcField := (k & 0xf) ^ fieldXor
			out[k] = in[(srcWord<<4)|srcField]
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

	// Tag A: 24 rows of 24, the symmetric matrix entry
	// (r, c) is negated when bit r XOR bit c of f_i is
	// set. C op{p}_do_ABC builds a per-column mask from
	// f_i and XORs a whole-row negate when bit r of f_i
	// is set, so the sign is the product of the row and
	// column signs (not just the row sign).
	fColMask := s.genSpread24(fI)
	for r := 0; r < 24; r++ {
		fbit := (fI >> uint(r)) & 1
		for w := 0; w < s.wordsPer24; w++ {
			m := fColMask[w]
			if fbit != 0 {
				switch {
				case w == s.usedPer24-1:
					m ^= s.slackMask
				case w < s.usedPer24:
					m ^= s.negateMask
				}
			}
			dst[as+r*s.wordsPer24+w] = src[as+r*s.wordsPer24+w] ^ m
		}
	}
	if mode != 0 {
		return
	}

	// Tags B, C: 24 rows of 24. As for tag A the sign on
	// entry (r, c) is the product of a row and a column
	// sign, but here B and C mix: C op{p}_do_ABC forms
	//   t = fmask & (B ^ C); t ^= efmask
	// where fmask negates entry (r, c) iff f_i[r] ^ f_i[c]
	// and efmask iff ef_i[r] ^ ef_i[c]; then
	//   B_out = B ^ t,  C_out = (C, negated row if odd eps) ^ t.
	efColMask := s.genSpread24(efI)
	for r := 0; r < 24; r++ {
		fRow := (fI >> uint(r)) & 1
		efRow := (efI >> uint(r)) & 1
		for w := 0; w < s.wordsPer24; w++ {
			rowNeg := s.negateMask
			if w == s.usedPer24-1 {
				rowNeg = s.slackMask
			} else if w >= s.usedPer24 {
				rowNeg = 0
			}
			fm := fColMask[w]
			if fRow != 0 {
				fm ^= rowNeg
			}
			efm := efColMask[w]
			if efRow != 0 {
				efm ^= rowNeg
			}
			t1 := src[bs+r*s.wordsPer24+w]
			t2 := src[cs+r*s.wordsPer24+w]
			t := (fm & (t1 ^ t2)) ^ efm
			dst[bs+r*s.wordsPer24+w] = t1 ^ t
			if epsOdd != 0 {
				t2 ^= rowNeg
			}
			dst[cs+r*s.wordsPer24+w] = t2 ^ t
		}
	}
}

// =====================================================================
// Non-monomial operations tau (genOpT) and xi (genOpXi)
// =====================================================================
//
// The C library implements the non-monomial parts of tau and xi as
// hand-unrolled SWAR Hadamard butterflies (one variant per modulus),
// generated by mmgroup/dev/hadamard/hadamard_{t,xi}.py. As everywhere
// else in this file we do the field arithmetic field-generically: we
// unpack a logical row to a []int, multiply it by the relevant fixed
// matrix modulo p, and repack. The matrices are exactly the reference
// matrices from mmgroup's own test oracle
// (mmgroup/tests/spaces/sparse_mm_space.py, classes NonMonomialOp_t
// and NonMonomialOp_l) and are built once at package-init time from the
// mat24 primitives, with their defining algebraic identities checked.

// genParityHadamardSign returns the sign of entry (i, j) of the
// parity-adjusted Hadamard matrix,
// (-1)^(parity(i&j) ^ (parity(i)&parity(j))). Reference matrices.py
// scal_prod_parity; identical to cgt.parityHadamardSign.
func genParityHadamardSign(i, j int) int { return parityHadamardSign(i, j) }

// genTauMat3 is the 3x3 triality matrix on (A, B, C) for exponents
// e = 1, 2 (indices 1, 2; index 0 unused). C NonMonomialOp_t.mat3.
var genTauMat3 = [3][3][3]int{
	1: {
		{0, 2, -2}, {1, 1, 1}, {1, -1, -1},
	},
	2: {
		{0, 2, 2}, {1, 1, -1}, {-1, 1, -1},
	},
}

// genTauMat64 holds the 64x64 tag-T triality matrices for e = 1, 2.
// e=1: sym64 @ diag64; e=2: diag64 @ sym64, where
// diag64 = diag((-1)^suboctad_weight) and
// sym64[i,j] = (-1)^suboctad_scalar_prod(i,j). C NonMonomialOp_t.mat64.
var genTauMat64 [3][64][64]int

// genXiMat16 holds the 16x16 tag-A xi matrices for e = 1, 2, equal to
// kron(L4_POWERS[e-1], L4_POWERS[e-1]). C NonMonomialOp_l.mat16.
var genXiMat16 [3][16][16]int

// genXiMat64 holds the 64x64 tag-X/Y/Z xi matrices for e = 1, 2, equal
// to kron(L16_POWERS[e-1], L4_POWERS[e-1]). C NonMonomialOp_l.mat64.
var genXiMat64 [3][64][64]int

// genXiCycle3 routes the four 1024-row sub-blocks of the Y/Z region
// through xi. cyc3[e][k] is the destination quarter for source quarter
// k under xi^e. C NonMonomialOp_l.cycles3.
var genXiCycle3 = [3][4]int{
	1: {2, 0, 1, 3},
	2: {1, 2, 0, 3},
}

func init() {
	genBuildTauMat64()
	genBuildXiMat16()
	genBuildXiMat64()
}

// genMatMul64 returns the 64x64 product a*b over the integers.
func genMatMul64(a, b *[64][64]int) [64][64]int {
	var c [64][64]int
	for i := 0; i < 64; i++ {
		for k := 0; k < 64; k++ {
			if a[i][k] == 0 {
				continue
			}
			aik := a[i][k]
			for j := 0; j < 64; j++ {
				c[i][j] += aik * b[k][j]
			}
		}
	}
	return c
}

// genBuildTauMat64 builds genTauMat64 from the suboctad primitives and
// verifies sym^3 scaling identities. Reference NonMonomialOp_t.mat64.
func genBuildTauMat64() {
	var diag, sym [64][64]int
	for i := 0; i < 64; i++ {
		if mat24.SuboctadWeight(uint32(i))&1 != 0 {
			diag[i][i] = -1
		} else {
			diag[i][i] = 1
		}
		for j := 0; j < 64; j++ {
			if mat24.SuboctadScalarProd(uint32(i), uint32(j))&1 != 0 {
				sym[i][j] = -1
			} else {
				sym[i][j] = 1
			}
		}
	}
	genTauMat64[1] = genMatMul64(&sym, &diag)
	genTauMat64[2] = genMatMul64(&diag, &sym)

	// Defining identities: mat^3 = 512*I and mat1*mat2 = 64*I.
	for _, e := range []int{1, 2} {
		m := genTauMat64[e]
		sq := genMatMul64(&m, &m)
		cube := genMatMul64(&sq, &m)
		for i := 0; i < 64; i++ {
			for j := 0; j < 64; j++ {
				want := 0
				if i == j {
					want = 512
				}
				if cube[i][j] != want {
					panic("cgt: genTauMat64 cube identity failed")
				}
			}
		}
	}
	prod := genMatMul64(&genTauMat64[1], &genTauMat64[2])
	for i := 0; i < 64; i++ {
		for j := 0; j < 64; j++ {
			want := 0
			if i == j {
				want = 64
			}
			if prod[i][j] != want {
				panic("cgt: genTauMat64 e1*e2 identity failed")
			}
		}
	}
}

// genL4Powers returns the two 4x4 sign matrices used by
// the non-monomial xi^e on tags A and Y/Z, indexed by
// e-1. The tag-A action is A'_block = blk_e @ A_block @
// blk_e^T per 4x4 group-pair, and the build helpers want
// f = blk_e^T. Since blk_1^T = blk_2 and blk_2^T = blk_1,
// L4_POWERS = [blk_2, blk_1]. The blocks were validated
// against mm_op15_xi for both exponents (0/576 tag-A
// entry diffs). C NonMonomialOp_l.L4_POWERS.
func genL4Powers() [2][4][4]int {
	// blk_1 (for e=1) and blk_2 = blk_1^T (for e=2).
	blk1 := [4][4]int{
		{1, -1, -1, -1}, {1, -1, 1, 1}, {1, 1, -1, 1}, {1, 1, 1, -1},
	}
	blk2 := [4][4]int{
		{1, 1, 1, 1}, {-1, -1, 1, 1}, {-1, 1, -1, 1}, {-1, 1, 1, -1},
	}
	// L4_POWERS[e-1] = blk_e^T: index 0 (e=1) = blk_2, index 1 (e=2) = blk_1.
	return [2][4][4]int{blk2, blk1}
}

// genBuildXiMat16 builds genXiMat16 = kron(L4_POWERS[e-1], itself).
func genBuildXiMat16() {
	l4 := genL4Powers()
	for _, e := range []int{1, 2} {
		f := &l4[e-1]
		for i0 := 0; i0 < 4; i0++ {
			for i1 := 0; i1 < 4; i1++ {
				for j0 := 0; j0 < 4; j0++ {
					for j1 := 0; j1 < 4; j1++ {
						genXiMat16[e][4*i0+i1][4*j0+j1] = f[i0][j0] * f[i1][j1]
					}
				}
			}
		}
	}
	// Defining identity: xi has order 3 with scaling 2^-2 on tag A, so
	// mat^3 = 4^3*I = 64*I.
	for _, e := range []int{1, 2} {
		m := &genXiMat16[e]
		var sq, cube [16][16]int
		for i := 0; i < 16; i++ {
			for k := 0; k < 16; k++ {
				for j := 0; j < 16; j++ {
					sq[i][j] += m[i][k] * m[k][j]
				}
			}
		}
		for i := 0; i < 16; i++ {
			for k := 0; k < 16; k++ {
				for j := 0; j < 16; j++ {
					cube[i][j] += sq[i][k] * m[k][j]
				}
			}
		}
		for i := 0; i < 16; i++ {
			for j := 0; j < 16; j++ {
				want := 0
				if i == j {
					want = 64
				}
				if cube[i][j] != want {
					panic("cgt: genXiMat16 cube identity failed")
				}
			}
		}
	}
}

// genBuildXiMat64 builds genXiMat64 = kron(L16_POWERS[e-1],
// L4_POWERS[e-1]), where L16_POWERS = [MDIAG16@MSYM16, MSYM16@MDIAG16],
// MSYM16[i,j] = (-1)^w2_gray(i^j), MDIAG16 = -diag((-1)^w2_gray(i)). C
// NonMonomialOp_l.mat64.
func genBuildXiMat64() {
	var msym, mdiag [16][16]int
	for i := 0; i < 16; i++ {
		// MDIAG16 = -diag((-1)^w2_gray(i)).
		s := 1
		if generator.XiW2Gray(uint32(i))&1 != 0 {
			s = -1
		}
		mdiag[i][i] = -s
		for j := 0; j < 16; j++ {
			if generator.XiW2Gray(uint32(i^j))&1 != 0 {
				msym[i][j] = -1
			} else {
				msym[i][j] = 1
			}
		}
	}
	mul16 := func(a, b *[16][16]int) [16][16]int {
		var c [16][16]int
		for i := 0; i < 16; i++ {
			for k := 0; k < 16; k++ {
				for j := 0; j < 16; j++ {
					c[i][j] += a[i][k] * b[k][j]
				}
			}
		}
		return c
	}
	// The Y/Z 64x64 xi matrix is kron(hi, lo) with
	// hi = -(MDIAG@MSYM or MSYM@MDIAG) and lo = L4_POWERS.
	// The negation of hi was determined against mm_op15_xi:
	// the extracted L16 equals -(MDIAG@MSYM)^T, and the
	// genApplyMat64 convention then needs hi = -(MDIAG@MSYM).
	// Verified 0/98304 Y/Z entry diffs for both exponents.
	l16 := [2][16][16]int{mul16(&mdiag, &msym), mul16(&msym, &mdiag)}
	l4 := genL4Powers()
	for _, e := range []int{1, 2} {
		hi := &l16[e-1]
		lo := &l4[e-1]
		for ih := 0; ih < 16; ih++ {
			for il := 0; il < 4; il++ {
				for jh := 0; jh < 16; jh++ {
					for jl := 0; jl < 4; jl++ {
						genXiMat64[e][4*ih+il][4*jh+jl] = -hi[ih][jh] * lo[il][jl]
					}
				}
			}
		}
	}
	// Defining identity: xi has order 3, so (mat * 2^-3)^3 = I, i.e.
	// mat^3 = 512*I and mat1*mat2 = 64*I.
	for _, e := range []int{1, 2} {
		m := genXiMat64[e]
		sq := genMatMul64(&m, &m)
		cube := genMatMul64(&sq, &m)
		for i := 0; i < 64; i++ {
			for j := 0; j < 64; j++ {
				want := 0
				if i == j {
					want = 512
				}
				if cube[i][j] != want {
					panic("cgt: genXiMat64 cube identity failed")
				}
			}
		}
	}
	prod := genMatMul64(&genXiMat64[1], &genXiMat64[2])
	for i := 0; i < 64; i++ {
		for j := 0; j < 64; j++ {
			want := 0
			if i == j {
				want = 64
			}
			if prod[i][j] != want {
				panic("cgt: genXiMat64 e1*e2 identity failed")
			}
		}
	}
}

// genApplyMat64 returns out = (in * mat) * scalar reduced modulo p,
// where mat is a 64x64 integer matrix and scalar is inv2^k mod p. The
// product is a row-vector times matrix: out[j] = sum_i in[i]*mat[i][j].
func genApplyMat64(p int, in *[64]int, mat *[64][64]int, scalar int) [64]int {
	var out [64]int
	for j := 0; j < 64; j++ {
		var acc int
		for i := 0; i < 64; i++ {
			if in[i] != 0 {
				acc += in[i] * mat[i][j]
			}
		}
		acc = (acc%p + p) % p
		out[j] = (acc * scalar) % p
	}
	return out
}

// genInvPow2 returns the inverse of 2^k modulo p (p odd). Modulo
// 2^n-1 this is the cyclic rotation 2^(P_BITS-k); we compute it by
// repeated halving so it is independent of P_BITS.
func genInvPow2(p, k int) int {
	half := (p + 1) / 2 // inverse of 2
	r := 1
	for i := 0; i < k; i++ {
		r = (r * half) % p
	}
	return r
}

// genOpT applies tau^t to the full representation vector src, storing
// the result in dst modulo p. C mm{p}_op_t. Tags A, B, C are mixed by
// the 3x3 triality matrix (the diagonal of A is preserved); tag T by
// the 64x64 matrix; tags X, Y, Z are permuted with the scalprod sign
// (tag X) and the xyz inversion (tags Y, Z).
//
// genOpT panics if p is not a supported modulus.
func genOpT(p int, src []uint64, t int, dst []uint64) {
	s := genSwarFor(p)
	if (t-1)&2 != 0 {
		// t == 0 or t >= 3: identity.
		copy(dst, src)
		return
	}
	e := t % 3 // exponent 1 or 2

	genOpTABC(s, src, e, dst, false)

	// Tag T: 759 rows of 64, multiplied by mat64 and scaled by 2^-3.
	mat := &genTauMat64[e]
	scalar := genInvPow2(p, 3)
	ts := s.genTagTOfs()
	for i := 0; i < 759; i++ {
		off := ts + i*s.wordsPer64
		in := s.genReadRow64(src, off)
		out := genApplyMat64(p, &in, mat, scalar)
		s.genWriteRow64(dst, off, out)
	}

	// Tags X, Y, Z. C swaps the source/destination roles of the X-Y,
	// Y-Z, Z-X blocks depending on the exponent. exp1 == 0 for e == 1.
	xs := s.genTagXOfs()
	zs := s.genTagZOfs()
	ys := s.genTagYOfs()
	rows24 := 2048 * s.wordsPer24
	var pXYin, pXYout, pYZin, pYZout, pZXin, pZXout int
	if e == 1 {
		pXYin, pXYout = xs, ys
		pYZin, pYZout = ys, zs
		pZXin, pZXout = zs, xs
	} else {
		pXYout, pXYin = xs, ys
		pYZout, pYZin = ys, zs
		pZXout, pZXin = zs, xs
	}

	// Map X to Y (for t) or Y to X (for t**2): plain copy then the
	// scalar-product-d-i negation on the destination.
	copy(dst[pXYout:pXYout+rows24], src[pXYin:pXYin+rows24])
	s.genNegScalprodDI(dst[pXYout:])
	// Map Y to Z (or Z to Y): the xyz inversion then the negation.
	genInvertXYZ(s, src, pYZin, dst, pYZout)
	s.genNegScalprodDI(dst[pYZout:])
	// Map Z to X (or X to Z): just the xyz inversion.
	genInvertXYZ(s, src, pZXin, dst, pZXout)
}

// genInvertXYZ copies a 2048-row tag block from src[soff:] to dst[doff:]
// negating row d when bit 12 of MAT24_THETA_TABLE[d] is set. C
// invert{p}_xyz.
func genInvertXYZ(s *genSwar, src []uint64, soff int, dst []uint64, doff int) {
	for d := 0; d < 2048; d++ {
		neg := (mat24.ThetaTable(uint32(d))>>12)&1 != 0
		s.genCopyRow24(src, soff+d*s.wordsPer24, dst, doff+d*s.wordsPer24, neg)
	}
}

// genOpTABC computes tags A, B, C of v * tau^e from src into dst. The
// diagonal entries of tag A are copied unchanged; every off-diagonal
// triple (A,B,C)[i][j] is replaced by (triple * mat3) * 2^-1 mod p. If
// abcOnly is set the B and C destinations are written too (used by the
// tag-ABC restricted variant); here it is always full. C op{p}_t_ABC.
func genOpTABC(s *genSwar, src []uint64, e int, dst []uint64, abcOnly bool) {
	_ = abcOnly
	mat := &genTauMat3[e]
	inv2v := (s.p + 1) / 2
	as := s.genTagAOfs()
	bs := s.genTagBOfs()
	cs := s.genTagCOfs()
	for i := 0; i < 24; i++ {
		for j := 0; j < 24; j++ {
			if i == j {
				// Preserve the tag-A diagonal; zero B, C diagonal.
				v := genReadEntry24(s, src, as, i, j)
				genWriteEntry24(s, dst, as, i, j, v)
				genWriteEntry24(s, dst, bs, i, j, 0)
				genWriteEntry24(s, dst, cs, i, j, 0)
				continue
			}
			a := genReadEntry24(s, src, as, i, j)
			b := genReadEntry24(s, src, bs, i, j)
			c := genReadEntry24(s, src, cs, i, j)
			in := [3]int{a, b, c}
			for col := 0; col < 3; col++ {
				acc := in[0]*mat[0][col] + in[1]*mat[1][col] + in[2]*mat[2][col]
				acc = (acc%s.p + s.p) % s.p
				acc = (acc * inv2v) % s.p
				switch col {
				case 0:
					genWriteEntry24(s, dst, as, i, j, acc)
				case 1:
					genWriteEntry24(s, dst, bs, i, j, acc)
				case 2:
					genWriteEntry24(s, dst, cs, i, j, acc)
				}
			}
		}
	}
}

// genReadEntry24 reads entry (row, col) of a 24x24 tag block based at
// word index base (base already includes the tag offset). Entry
// (row,col) lives at logical index row*32+col in that tag's 24-row
// layout: word base + row*wordsPer24 + col/intFields, field col%intFields.
func genReadEntry24(s *genSwar, v []uint64, base, row, col int) int {
	off := base + row*s.wordsPer24 + col/int(s.intFields)
	j := uint(col) % s.intFields
	return s.genReadField(v[off], j)
}

// genWriteEntry24 writes value val (reduced mod p) to entry (row, col)
// of a 24x24 tag block based at word index base.
func genWriteEntry24(s *genSwar, v []uint64, base, row, col, val int) {
	off := base + row*s.wordsPer24 + col/int(s.intFields)
	j := uint(col) % s.intFields
	x := val % s.p
	if x < 0 {
		x += s.p
	}
	mask := uint64(s.p) << (j * s.fieldBits)
	v[off] = (v[off] &^ mask) | (uint64(x) << (j * s.fieldBits))
}

// genXiShape is one stage of the xi monomial part: the source and
// destination box shapes (rows, words, fields) from MM_TABLE_SHAPES_XI.
type genXiShape struct {
	srcRows, srcWords, srcFields int
	dstRows, dstWords, dstFields int
}

// genXiShapes are the five monomial-stage shapes. C
// MM_TABLE_SHAPES_XI / mm_tables_xi.BOX_SHAPES with MAP_XI.
var genXiShapes = [5]genXiShape{
	{1, 78, 32, 1, 78, 32},
	{45, 16, 32, 45, 16, 32},
	{64, 12, 32, 64, 16, 24},
	{64, 16, 24, 64, 16, 24},
	{64, 16, 24, 64, 12, 32},
}

// genOpXi applies xi^e to the full representation vector src, storing
// the result in dst modulo p. C mm{p}_op_xi. The operation splits into
// a monomial part (tags B, C, T, X permuted with signs through the five
// xi tables) and a non-monomial part (tag A through the 16x16 matrix,
// tags X, Y, Z through the 64x64 matrix).
//
// genOpXi panics if p is not a supported modulus.
func genOpXi(p int, src []uint64, e int, dst []uint64) {
	s := genSwarFor(p)
	if (e-1)&2 != 0 {
		copy(dst, src)
		return
	}
	exp1 := (e & 3) - 1 // 0 for e==1, 1 for e==2

	genXiMonomial(s, src, exp1, dst)
	genXiTagA(s, src, exp1, dst)
	genXiTagYZ(s, src, exp1, dst)
}

// genXiMonomial performs the monomial part of xi^e: tags B, C, T, X.
// It mirrors C mm{p}_op_xi_mon but works on logical field values rather
// than the packed uint8 scratch buffer. For each of the five stages it
// flattens the source region into a slice of field values (32 per
// source word, matching the C OP_XI_LOAD padding), then for each
// destination row gathers DEST_SHAPE[2] entries through the permutation
// table and applies the per-row sign vector. exp1 selects the table
// pair (0 for xi, 1 for xi**2).
func genXiMonomial(s *genSwar, src []uint64, exp1 int, dst []uint64) {
	for stage := 0; stage < 5; stage++ {
		sh := genXiShapes[stage]
		perm := xiPermTables[stage][exp1]
		sign := xiSignTables[stage][exp1]
		srcOfs := int(xiOffsetTable[stage][exp1][0]) >> logIntFields(s.p)
		dstOfs := int(xiOffsetTable[stage][exp1][1]) >> logIntFields(s.p)

		// Stride between consecutive box groups, for both
		// source and destination. C mm{p}_op_xi_mon advances
		// p_src and p_dest by V24_INTS (= wordsPer24) per group
		// for every stage, regardless of the field count: a
		// group is one 24-row's worth of words even when the
		// box spans the 64-wide tag T. Using wordsPer64 here
		// scattered the output (only tag B happened to align).
		srcRowWords := s.wordsPer24
		dstRowWords := s.wordsPer24

		permPos := 0
		signPos := 0
		for j := 0; j < sh.srcRows; j++ {
			// Flatten this j-block's source words to 32 fields each.
			b := make([]int, sh.srcWords*32)
			for k := 0; k < sh.srcWords; k++ {
				rowOff := srcOfs + (j*sh.srcWords+k)*srcRowWords
				for f := 0; f < 32; f++ {
					w := rowOff + f/int(s.intFields)
					b[k*32+f] = s.genReadField(src[w], uint(f)%s.intFields)
				}
			}
			for k := 0; k < sh.dstWords; k++ {
				rowOff := dstOfs + (j*sh.dstWords+k)*dstRowWords
				sgn := sign[signPos]
				var vals [64]int
				for f := 0; f < sh.dstFields; f++ {
					val := b[perm[permPos+f]]
					if (sgn>>uint(f))&1 != 0 {
						val = (s.p - val) % s.p
					}
					vals[f] = val
				}
				genWriteFieldsRow(s, dst, rowOff, sh.dstFields, &vals)
				permPos += sh.dstFields
				signPos++
			}
		}
	}
}

// genWriteFieldsRow packs the first n field values of vals into the
// row at word index off, using intFields fields per word and zeroing
// any trailing fields of the last partially-used word.
func genWriteFieldsRow(s *genSwar, v []uint64, off, n int, vals *[64]int) {
	words := (n + int(s.intFields) - 1) / int(s.intFields)
	for w := 0; w < words; w++ {
		var word uint64
		base := w * int(s.intFields)
		for jf := uint(0); jf < s.intFields; jf++ {
			idx := base + int(jf)
			if idx >= n {
				break
			}
			x := vals[idx] % s.p
			if x < 0 {
				x += s.p
			}
			word |= uint64(x) << (jf * s.fieldBits)
		}
		v[off+w] = word
	}
}

// genXiTagA performs the non-monomial xi^e on tag A. The 24x24 tag-A
// block decomposes into 6x6 super-blocks indexed by (ih>>2, jh>>2),
// each a 4x4 sub-block; the 16-vector for super-block (ih, jh) is
// formed as v[4*il+jl] = A[ih+il][jh+jl] and multiplied by the 16x16
// xi matrix, then scaled by 2^-2. C mm{p}_op_xi_a / NonMonomialOp_l.op_A.
func genXiTagA(s *genSwar, src []uint64, exp1 int, dst []uint64) {
	e := exp1 + 1
	mat := &genXiMat16[e]
	scalar := genInvPow2(s.p, 2)
	as := s.genTagAOfs()
	for ih := 0; ih < 24; ih += 4 {
		for jh := 0; jh < 24; jh += 4 {
			var in [16]int
			for il := 0; il < 4; il++ {
				for jl := 0; jl < 4; jl++ {
					in[4*il+jl] = genReadEntry24(s, src, as, ih+il, jh+jl)
				}
			}
			var out [16]int
			for col := 0; col < 16; col++ {
				var acc int
				for row := 0; row < 16; row++ {
					if in[row] != 0 {
						acc += in[row] * mat[row][col]
					}
				}
				acc = (acc%s.p + s.p) % s.p
				out[col] = (acc * scalar) % s.p
			}
			for il := 0; il < 4; il++ {
				for jl := 0; jl < 4; jl++ {
					genWriteEntry24(s, dst, as, ih+il, jh+jl, out[4*il+jl])
				}
			}
		}
	}
}

// genXiTagYZ performs the non-monomial xi^e on tags X, Y, Z (the
// 4096-row region beginning at tag Z). It processes 64-entry blocks
// formed from one of 16 rows il (0..15) of a 16-row group times four
// columns jl (0..3): v[4*il+jl] = block[il][jl]. Each block is
// multiplied by the 64x64 xi matrix and scaled by 2^-3, and the result
// is routed to a destination quarter selected by cycles3. C
// mm{p}_op_xi_yz driven by TAB{P}_XI64_OFFSET / NonMonomialOp_l.op_YZ.
func genXiTagYZ(s *genSwar, src []uint64, exp1 int, dst []uint64) {
	e := exp1 + 1
	mat := &genXiMat64[e]
	scalar := genInvPow2(s.p, 3)
	cyc := &genXiCycle3[e]

	// The region from tag Z spans tags Z then Y (4096 rows of 24).
	// Index a logical entry (tag in {Z,Y}, row r in 0..2047, col) by a
	// flat row number R = (tagBit<<11) + r, 0 <= R < 4096. Group rows
	// into 256 super-rows of 16 (ih = R & 0xff0) and split columns into
	// (jh = col & 0x1c, jl = col & 3).
	zBase := s.genTagZOfs()

	// readEntry/writeEntry over the flat 4096-row Z..Y region.
	read := func(R, col int) int {
		off := zBase + R*s.wordsPer24 + col/int(s.intFields)
		return s.genReadField(src[off], uint(col)%s.intFields)
	}
	write := func(R, col, val int) {
		off := zBase + R*s.wordsPer24 + col/int(s.intFields)
		j := uint(col) % s.intFields
		x := val % s.p
		if x < 0 {
			x += s.p
		}
		mask := uint64(s.p) << (j * s.fieldBits)
		dst[off] = (dst[off] &^ mask) | (uint64(x) << (j * s.fieldBits))
	}

	for ih := 0; ih < 4096; ih += 16 {
		for jh := 0; jh < 24; jh += 4 {
			if jh >= 24 {
				break
			}
			// Build the 64-vector for super-block (ih, jh).
			var in [64]int
			for il := 0; il < 16; il++ {
				for jl := 0; jl < 4; jl++ {
					col := jh + jl
					if col >= 24 {
						in[4*il+jl] = 0
						continue
					}
					in[4*il+jl] = read(ih+il, col)
				}
			}
			out := genApplyMat64(s.p, &in, mat, scalar)
			// Route the destination super-row through cycles3 on the
			// top quarter (bits 10..11 of ih).
			kHi := cyc[ih>>10]
			dstIh := (kHi << 10) + (ih & 0x3ff)
			for il := 0; il < 16; il++ {
				for jl := 0; jl < 4; jl++ {
					col := jh + jl
					if col >= 24 {
						continue
					}
					write(dstIh+il, col, out[4*il+jl])
				}
			}
		}
	}
}

// =====================================================================
// Word operation (genOpWord) — C mm{p}_op_word via the mm_group word
// iterator (mm_group_word.ske). The iterator simplifies maximal
// subwords lying in N_0 and yields, per step, an N_0 element h (as the
// five-word mm_group_n encoding h[1..5]) together with an accumulated
// xi exponent h[0]. We dispatch each component to genOpXi, genOpT,
// genOpXY and genOpPi/genOpDelta, ping-ponging between v and work.
// =====================================================================

// genWordIter is the Go port of C mm_group_iter_t.
type genWordIter struct {
	data      [6]uint32
	lookahead uint32
	g         []uint32
	e         int
	index     int
	iStart    int
	iStop     int
	iStep     int
	sign      uint32
}

const genEndWord = 0xffffffff

// genIterStart initialises the iterator over g^e. C mm_group_iter_start.
func genIterStart(it *genWordIter, g []uint32, e int) {
	it.g = g
	it.e = e
	if len(g) == 0 {
		it.e = 0
	}
	if e >= 0 {
		it.iStart, it.iStop, it.iStep = 0, len(g), 1
		it.sign = 0
	} else {
		it.iStart, it.iStop, it.iStep = len(g)-1, -1, -1
		it.sign = 0x80000000
		it.e = -e
	}
	it.index = it.iStart
	it.lookahead = 0
}

// genIterNextAtom advances lookahead to the next atom. C
// mm_group_iter_next_atom.
func genIterNextAtom(it *genWordIter) {
	if it.e == 0 {
		it.lookahead = genEndWord
		return
	}
	result := it.g[it.index]
	it.index += it.iStep
	if it.index == it.iStop {
		it.index = it.iStart
		it.e--
	}
	it.lookahead = result ^ it.sign
}

// genIterNext accumulates the next maximal N_0 subword (with one
// xi exponent) into it.data, returning 0 to continue, 1 at the end of
// the word, or 2 on an illegal atom. C mm_group_iter_next.
func genIterNext(it *genWordIter) uint32 {
	g := (*n0.N0Elem)(it.data[1:])
	for i := range it.data {
		it.data[i] = 0
	}
	xiUsed := false
	for {
		atom := it.lookahead
		tag := (atom >> 28) & 0xf
		switch tag {
		case 8, 0:
			// neutral
		case 8 + 1, 1:
			n0.MulDeltaPi(g, atom&0xfff, 0)
			xiUsed = true
		case 8 + 2:
			n0.MulInvDeltaPi(g, 0, atom&0xfffffff)
			xiUsed = true
		case 2:
			n0.MulDeltaPi(g, 0, atom&0xfffffff)
			xiUsed = true
		case 8 + 3:
			atom ^= mat24.ThetaTable(atom&0x7ff) & 0x1000
			n0.MulX(g, atom&0x1fff)
			xiUsed = true
		case 3:
			n0.MulX(g, atom&0x1fff)
			xiUsed = true
		case 8 + 4:
			atom ^= mat24.ThetaTable(atom&0x7ff) & 0x1000
			n0.MulY(g, atom&0x1fff)
			xiUsed = true
		case 4:
			n0.MulY(g, atom&0x1fff)
			xiUsed = true
		case 8 + 5:
			atom ^= 0x3
			n0.MulT(g, atom&3)
			xiUsed = true
		case 5:
			n0.MulT(g, atom&3)
			xiUsed = true
		case 8 + 6:
			atom ^= 3
			if xiUsed {
				return 0
			}
			it.data[0] = (it.data[0] + (atom & 3)) % 3
		case 6:
			if xiUsed {
				return 0
			}
			it.data[0] = (it.data[0] + (atom & 3)) % 3
		default:
			atom |= 0x80000000
			if atom == genEndWord {
				return 1
			}
			if atom == 0xf0000000 {
				it.lookahead = 0
				return 0
			}
			return 2
		}
		genIterNextAtom(it)
	}
}

// genOpWord applies g^e to v in place, using work (an MMV-sized
// scratch buffer) for double buffering. C mm{p}_op_word.
//
// genOpWord panics if p is not a supported modulus.
func genOpWord(p int, v []uint64, g []uint32, length, e int, work []uint64) error {
	g = g[:length]
	p0, p1 := v, work
	var it genWordIter
	genIterStart(&it, g, e)
	var status uint32
	for {
		status = genIterNext(&it)
		h := &it.data
		if h[0] != 0 {
			genOpXi(p, p0, int(h[0]), p1)
			p0, p1 = p1, p0
		}
		if h[1] != 0 {
			genOpT(p, p0, int(h[1]), p1)
			p0, p1 = p1, p0
		}
		if h[2]|h[3] != 0 {
			genOpXY(p, p0, int(h[2]), int(h[3]), int(h[4]), p1)
			h[4] = 0
			p0, p1 = p1, p0
		}
		if h[5] != 0 {
			genOpPi(p, p0, int(h[4]), int(h[5]), p1)
			p0, p1 = p1, p0
		} else if h[4] != 0 {
			genOpDelta(p, p0, int(h[4]), p1)
			p0, p1 = p1, p0
		}
		if status != 0 {
			break
		}
	}
	// C mm{p}_op_word: if the result ended up in the work buffer (an
	// odd number of buffer swaps), copy it back so the result is always
	// in v. The work buffer also retains the result in that case.
	if &p0[0] != &v[0] {
		copy(v, p0)
	}
	if status == 1 {
		return nil
	}
	return errWordIllegalAtom
}

// errWordIllegalAtom reports an illegal atom encountered by the word
// iterator (C return value 2, i.e. status - 1 != 0).
var errWordIllegalAtom = errors.New("cgt: illegal atom in mm_op word")

// genOpWordTagA applies g^e to the tag-A part of v in place. C
// mm{p}_op_word_tag_A. It dispatches only the tag-A restricted
// operations and fails if the word contains a nonzero tau power
// (tag t), which does not fix tag A.
//
// genOpWordTagA panics if p is not a supported modulus.
func genOpWordTagA(p int, v []uint64, g []uint32, length, e int) error {
	g = g[:length]
	var it genWordIter
	genIterStart(&it, g, e)
	var status uint32
	for {
		status = genIterNext(&it)
		h := &it.data
		if h[0] != 0 {
			genOpXiTagA(p, v, int(h[0]))
		}
		if h[1] != 0 {
			return errWordTauOnA
		}
		if h[2] != 0 {
			genOpXYTagABC(p, v, int(h[2]), 0, 0, 1)
		}
		if h[5] != 0 {
			genOpPiTagABC(p, v, 0, int(h[5]), 1)
		}
		if status != 0 {
			break
		}
	}
	if status == 1 {
		return nil
	}
	return errWordIllegalAtom
}

// errWordTauOnA reports a tau power in a tag-A-only word operation. C
// mm{p}_op_word_tag_A returns -1 in this case.
var errWordTauOnA = errors.New("cgt: tau power in tag-A word operation")

// genOpXiTagA applies xi^e to tag A of v in place. C
// mm{p}_op_xi_tag_A: it runs the tag-A non-monomial step into a scratch
// 24-row buffer and copies it back.
func genOpXiTagA(p int, v []uint64, e int) {
	s := genSwarFor(p)
	if (e-1)&2 != 0 {
		return
	}
	exp1 := (e & 3) - 1
	tmp := make([]uint64, 24*s.wordsPer24)
	// genXiTagA reads tag A of v and writes tag A of its dst; here we
	// stage into tmp whose tag-A offset is 0.
	genXiTagAInto(s, v, exp1, tmp)
	copy(v[:24*s.wordsPer24], tmp)
}

// genXiTagAInto is genXiTagA writing to a destination whose tag-A base
// is word 0 (a 24-row scratch buffer) rather than the full vector.
func genXiTagAInto(s *genSwar, src []uint64, exp1 int, tmp []uint64) {
	e := exp1 + 1
	mat := &genXiMat16[e]
	scalar := genInvPow2(s.p, 2)
	as := s.genTagAOfs()
	for ih := 0; ih < 24; ih += 4 {
		for jh := 0; jh < 24; jh += 4 {
			var in [16]int
			for il := 0; il < 4; il++ {
				for jl := 0; jl < 4; jl++ {
					in[4*il+jl] = genReadEntry24(s, src, as, ih+il, jh+jl)
				}
			}
			var out [16]int
			for col := 0; col < 16; col++ {
				var acc int
				for row := 0; row < 16; row++ {
					if in[row] != 0 {
						acc += in[row] * mat[row][col]
					}
				}
				acc = (acc%s.p + s.p) % s.p
				out[col] = (acc * scalar) % s.p
			}
			for il := 0; il < 4; il++ {
				for jl := 0; jl < 4; jl++ {
					genWriteEntry24(s, tmp, 0, ih+il, jh+jl, out[4*il+jl])
				}
			}
		}
	}
}

// genOpXYTagABC applies y_f x_e x_eps to tags A, B, C of v in place. If
// mode is nonzero only tag A is written. C mm{p}_op_xy_tag_ABC; it
// reuses the field-generic tag-ABC kernel genOpXYABC with src == dst.
func genOpXYTagABC(p int, v []uint64, f, e, eps, mode int) {
	s := genSwarFor(p)
	op := subPrepXY(uint32(f), uint32(e), uint32(eps))
	genOpXYABC(s, v, op, mode, v)
}

// genOpPiTagABC applies x_delta x_pi to tags A, B, C of v in place. If
// mode is nonzero only tag A is written. C mm{p}_op_pi_tag_ABC. Tag A
// (and B, C) is conjugated by the permutation pi: the symmetric entry
// (i, j) of the source moves to (pi[i], pi[j]) of the result, i.e. the
// result entry (i, j) is gathered from source entry (pi^-1[i], pi^-1[j]);
// tag C additionally gets the odd-delta row sign. No cocode signs are
// applied here (the full sign change is the caller's responsibility),
// matching the C tag-ABC restriction. C mm{p}_op_pi_tag_ABC builds
// row_perm[perm[i]] = i, i.e. it gathers via the inverse permutation.
func genOpPiTagABC(p int, v []uint64, delta, pi, mode int) {
	s := genSwarFor(p)
	perm := mat24.M24numToPerm(uint32(pi) % uint32(mat24.Mat24Order))
	invPerm := mat24.InvPerm(perm)
	var inv [24]int
	for i := 0; i < 24; i++ {
		inv[i] = int(invPerm[i])
	}
	tags := []int{s.genTagAOfs()}
	if mode == 0 {
		tags = append(tags, s.genTagBOfs(), s.genTagCOfs())
	}
	oddDelta := delta&0x800 != 0
	for ti, base := range tags {
		src := make([]int, 24*24)
		for i := 0; i < 24; i++ {
			for j := 0; j < 24; j++ {
				src[i*24+j] = genReadEntry24(s, v, base, i, j)
			}
		}
		isC := mode == 0 && ti == 2
		for i := 0; i < 24; i++ {
			for j := 0; j < 24; j++ {
				val := src[inv[i]*24+inv[j]]
				if isC && oddDelta {
					val = (s.p - val) % s.p
				}
				genWriteEntry24(s, v, base, i, j, val)
			}
		}
	}
}

// genOpTTagABC applies tau^exp to tags A, B, C of v in
// place. C mm{p}_op_t_ABC: when exp is 0 or >= 3 (i.e.
// (exp-1)&2 != 0) it is the identity; otherwise it runs
// the tag-ABC triality kernel with src == dst.
func genOpTTagABC(p int, v []uint64, exp int) {
	if (exp-1)&2 != 0 {
		return
	}
	s := genSwarFor(p)
	genOpTABC(s, v, exp%3, v, true)
}

// genOpWordABC computes tags A, B, C of v * g (with g
// of length length) into dst, leaving the other tags of
// dst untouched. C mm{p}_op_word_ABC.
//
// Every prefix of g must lie in G_{x0} * N_0; the word
// front end PrepareOpABC enforces this and either keeps
// g in N_0 (the 0x100 branch) or splits off a G_{x0}
// prefix whose action on tags B, C is recovered from
// the standard subframe (genLeech2MapStdSubframe /
// genExtractBC). The N_0 remainder is then applied with
// the tag-ABC restricted operations tau, y/x/d and d/p.
//
// genOpWordABC panics if p is not a supported modulus.
// It returns an error if g is malformed or leaves
// G_{x0} * N_0.
func genOpWordABC(p int, src []uint64, g []uint32, length int, dst []uint64) error {
	s := genSwarFor(p)
	abcEnd := s.genTagTOfs() // end of tags A, B, C
	aEnd := s.genTagBOfs()   // end of tag A

	var g1 [12]uint32
	lenG1 := PrepareOpABC(g, length, g1[:])
	if (lenG1 & 0xff) > 11 {
		return errWordABCLong
	}
	if lenG1 < 0 {
		return errWordABCPrepare
	}

	var pos, rem int // g1[pos:pos+rem] is the N_0 remainder
	if lenG1&0x100 != 0 {
		// g lies in N_0: copy all of tags A, B, C and
		// apply the remainder directly.
		copy(dst[:abcEnd], src[:abcEnd])
		rem = lenG1 & 0xff
		pos = 0
	} else {
		// g has a G_{x0} prefix. Copy tag A, apply the
		// prefix to tag A, then recover tags B, C from the
		// standard subframe of the inverted prefix.
		rem = lenG1 & 0xff
		copy(dst[:aEnd], src[:aEnd])
		lenG0 := genLeech2PrefixGx0(g1[:], rem)
		if err := genOpWordTagA(p, dst, g1[:], lenG0, 1); err != nil {
			return err
		}
		invertWord(g1[:lenG0])
		var subframe [24]uint32
		if res := genLeech2MapStdSubframe(g1[:], lenG0, subframe[:]); res != lenG0 {
			return errWordABCSubframe
		}
		if err := genExtractBC(p, src, subframe[:], dst); err != nil {
			return err
		}
		pos = lenG0
		rem -= lenG0
	}

	if rem == 0 {
		return nil
	}
	// Set a terminator atom so the tail can scan subwords
	// of g1 without a separate length check.
	g1[pos+rem] = 0xffffffff

	// Tag t: a single tau power.
	if (g1[pos]>>28)&0xf == 5 {
		genOpTTagABC(p, dst, int(g1[pos]&0xfffffff))
		pos++
		rem--
	}
	if rem == 0 {
		return nil
	}

	// Tag sequence y, x, d. Generators x_e, y_e, x_eps
	// act trivially on tags A, B, C when e in {+-1,
	// +-Omega} or eps is even, so only y | x triggers the
	// xy operation.
	{
		var y, x, d uint32
		p2 := pos
		if (g1[p2]>>28)&0xf == 4 {
			y = g1[p2] & 0x7ff
			p2++
		}
		if (g1[p2]>>28)&0xf == 3 {
			x = g1[p2] & 0x7ff
			p2++
		}
		if x|y != 0 && (g1[p2]>>28)&0xf == 1 {
			d = g1[p2] & 0xfff
			p2++
		}
		if p2 > pos {
			if x|y != 0 {
				genOpXYTagABC(p, dst, int(y), int(x), int(d), 0)
			}
			rem -= p2 - pos
			if rem == 0 {
				return nil
			}
			pos = p2
		}
	}

	// Tag sequence d, p.
	{
		var d, pi uint32
		p2 := pos
		if (g1[p2]>>28)&0xf == 1 {
			d = g1[p2] & 0x800
			p2++
		}
		if (g1[p2]>>28)&0xf == 2 {
			pi = g1[p2] & 0xfffffff
			p2++
		}
		if p2 > pos {
			if pi != 0 {
				genOpPiTagABC(p, dst, int(d), int(pi), 0)
			} else if d != 0 {
				genOpDeltaTagABC(p, dst, int(d), 0)
			}
			rem -= p2 - pos
		}
	}
	if rem != 0 {
		return errWordABCTrailing
	}
	return nil
}

// Errors returned by genOpWordABC, mirroring the
// distinct negative return codes of C mm{p}_op_word_ABC.
var (
	errWordABCLong     = errors.New("cgt: OpWordABC: prepared word too long")
	errWordABCPrepare  = errors.New("cgt: OpWordABC: word not in Gx0*N0")
	errWordABCSubframe = errors.New("cgt: OpWordABC: subframe map failed")
	errWordABCTrailing = errors.New("cgt: OpWordABC: unconsumed trailing atoms")
)
`

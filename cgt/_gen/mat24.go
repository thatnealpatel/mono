package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// This file computes the 15 mat24 tables from
// first principles — the Golay code basis and the
// definitions in mmgroup's pure-python table
// generator (mmgroup/src/mmgroup/dev/mat24) — and
// renders them as package-cgt Go source.
//
// Nothing here parses C: the only seed is the
// hardcoded Golay code generator matrix
// golayBasisSeed. Every other table is derived
// from it by GF(2) linear algebra and the same
// constructions used upstream.
//
// genMat24Tables also verifies each computed table
// against the hand-checked golden values currently
// in <cgtDir>/mat24_gen.go and fails if any entry
// disagrees, so the generator is both a producer and
// a verifier.
//
// The shared rendering infrastructure (cType,
// cTable, writeHeader, writeHexTable, valsPerLine)
// lives in emit.go and is reused by the xi table
// generator in mm_op_xi.go.

// genMat24Tables computes every mat24 table from
// the Golay code basis, verifies it against the
// golden file in cgtDir, and writes the gofmt-clean
// Go source — one var per table — to w.
//
// genMat24Tables returns an error if a computed
// table fails verification, if the golden file
// cannot be read or parsed, or if formatting fails.
func genMat24Tables(w io.Writer, cgtDir string) error {
	tables, err := computeMat24Tables()
	if err != nil {
		return err
	}
	if err := verifyMat24Tables(tables, cgtDir); err != nil {
		return err
	}

	var buf bytes.Buffer
	writeHeader(&buf)
	for _, t := range tables {
		writeTable(&buf, t)
	}

	out, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("format generated source: %w", err)
	}
	if _, err := w.Write(out); err != nil {
		return err
	}
	return nil
}

// writeTable renders one computed mat24 table: a doc
// comment naming the entry count, then the var
// declaration produced by writeHexTable.
func writeTable(buf *bytes.Buffer, t cTable) {
	fmt.Fprintf(buf, "\n// %s has %d entries.\n", t.goName, len(t.values))
	writeHexTable(buf, t.goName, "...", t.typ.goType, t.typ.width, valsPerLine, t.values)
}

//////////////////////////////////////////////////
// The Golay code seed
//////////////////////////////////////////////////

// golayBasisSeed is the 24-row generator matrix of
// the extended binary Golay code as used by
// mmgroup. Row i is a vector in GF(2)^24 with bit
// j (valence 2**j) set iff component j is one.
//
// Rows 0..11 span a transversal of the Golay
// cocode; rows 12..23 span the Golay code itself.
// This is the single mathematical constant from
// which every other mat24 table is derived; it is
// produced upstream by HexacodeToGolay.basis().
var golayBasisSeed = [24]uint32{
	0x00000110, 0x00001010, 0x00010010, 0x00100010,
	0x00000a00, 0x00000c00, 0x000000a0, 0x000000c0,
	0x0000000a, 0x0000000c, 0x00111111, 0x00000001,
	0x00fff0f0, 0x00ff0ff0, 0x00f0fff0, 0x000ffff0,
	0x00cccc00, 0x00aaaa00, 0x006ac0c0, 0x00c6a0a0,
	0x00a6c00c, 0x006ca00a, 0x0011111e, 0x00ffffff,
}

//////////////////////////////////////////////////
// GF(2) bit-vector / bit-matrix primitives
//
// A bit vector is an integer; bit i has valence
// 2**i. A bit matrix is a slice of row vectors.
// These mirror mmgroup.bitfunctions.
//////////////////////////////////////////////////

// bw24 returns the bit weight of the low 24 bits.
func bw24(v uint32) int {
	return bits.OnesCount32(v & 0xffffff)
}

// bitWeight returns the bit weight of all bits.
func bitWeight(v uint32) int {
	return bits.OnesCount32(v)
}

// bitParity returns the parity of all bits.
func bitParity(v uint32) int {
	return bits.OnesCount32(v) & 1
}

// v2 returns the 2-adic valuation of v: the index
// of its least significant set bit.
//
// v2 panics if v == 0, matching the upstream
// ZeroDivisionError contract.
func v2(v uint32) int {
	if v == 0 {
		panic("v2: argument is zero")
	}
	return bits.TrailingZeros32(v)
}

// bits2List returns the ascending positions of the
// set bits of v.
func bits2List(v uint32) []int {
	var l []int
	for v != 0 {
		i := bits.TrailingZeros32(v)
		l = append(l, i)
		v &= v - 1
	}
	return l
}

// basisSpanned returns the 2**len(basis) vectors
// spanned by basis, with entry index sum(b_i*2**i)
// holding sum(b_i*basis[i]). This is the linear
// table layout used by both bit_mat_basis_spanned
// and lin_table upstream.
func basisSpanned(basis []uint32) []uint32 {
	sp := make([]uint32, 1, 1<<len(basis))
	for _, x := range basis {
		n := len(sp)
		for i := 0; i < n; i++ {
			sp = append(sp, sp[i]^x)
		}
	}
	return sp
}

// bitMatInverse returns the inverse of the square
// bit matrix a over GF(2).
//
// a must be square: its column count (the bit
// length of the OR of all rows) must equal len(a).
// bitMatInverse panics if a is not square or is
// singular.
func bitMatInverse(a []uint32) []uint32 {
	var orAll uint32
	for _, x := range a {
		orAll |= x
	}
	ncols := bits.Len32(orAll)
	if ncols != len(a) {
		panic(fmt.Sprintf("bitMatInverse: not square (%d rows, %d cols)",
			len(a), ncols))
	}

	// Augment each row with an identity column block
	// above bit ncols, then reduce to identity.
	hicol := uint64(1) << ncols
	ah := make([]uint64, ncols)
	for i, x := range a {
		ah[i] = uint64(x) | (hicol << i)
	}
	perm := make([]int, ncols)
	for i := range perm {
		perm[i] = -1
	}
	for i := 0; i < ncols; i++ {
		piv := bits.TrailingZeros64(ah[i])
		if piv >= ncols {
			panic("bitMatInverse: singular matrix")
		}
		perm[piv] = i
		msk := uint64(1) << piv
		for j := 0; j < ncols; j++ {
			if j != i && ah[j]&msk != 0 {
				ah[j] ^= ah[i]
			}
		}
	}
	inv := make([]uint32, ncols)
	for i := 0; i < ncols; i++ {
		inv[i] = uint32(ah[perm[i]] >> ncols)
	}
	return inv
}

//////////////////////////////////////////////////
// Lsbit (de Bruijn) table
//////////////////////////////////////////////////

// deBruijnConst is the de Bruijn sequence used to
// index the lsbit lookup table; see
// Lsbit24Function in mmgroup.
const deBruijnConst = 0x077cb531

// makeLsbitTable builds the 32-entry de Bruijn
// lookup table that maps a power-of-two times the
// constant (shifted) to its bit index, returning
// 24 for the all-zero low-24-bit input.
func makeLsbitTable() []uint64 {
	tab := make([]uint64, 32)
	for i := 0; i < 32; i++ {
		index := (uint32(deBruijnConst) << uint(i) >> 26) & 0x1f
		v := i
		if v > 24 {
			v = 24
		}
		tab[index] = uint64(v)
	}
	return tab
}

//////////////////////////////////////////////////
// Encoding / decoding tables and conversions
//////////////////////////////////////////////////

// encodingTables splits an 24-row basis into three
// 8-row blocks and returns the spanned tables for
// each block. With basis == recipBasis these are
// the enc tables (vect -> internal); with basis ==
// golayBasis they are the dec tables (internal ->
// vect).
func encodingTables(basis []uint32) (t0, t1, t2 []uint32) {
	t0 = basisSpanned(basis[0:8])
	t1 = basisSpanned(basis[8:16])
	t2 = basisSpanned(basis[16:24])
	return
}

// mat24 holds every derived table plus the working
// vectors needed to derive later tables, all
// computed once from the Golay basis.
type mat24 struct {
	basis      []uint32 // 24 Golay basis rows
	recipBasis []uint32 // 24-row reciprocal basis

	enc0, enc1, enc2 []uint32
	dec0, dec1, dec2 []uint32
}

// vectToVintern maps a GF(2)^24 vector to internal
// representation via the encoding tables.
func (m *mat24) vectToVintern(v uint32) uint32 {
	return m.enc0[v&0xff] ^ m.enc1[(v>>8)&0xff] ^ m.enc2[(v>>16)&0xff]
}

// vectToCocode maps a GF(2)^24 vector to a Golay
// cocode word.
func (m *mat24) vectToCocode(v uint32) uint32 {
	return m.vectToVintern(v) & 0xfff
}

// gcodeToVect maps a Golay code word (gcode
// representation) to its GF(2)^24 vector.
func (m *mat24) gcodeToVect(v uint32) uint32 {
	return m.dec1[(v<<4)&0xf0] ^ m.dec2[(v>>4)&0xff]
}

//////////////////////////////////////////////////
// Syndrome table
//////////////////////////////////////////////////

// makeSyndromeTable builds the 2048-entry syndrome
// lookup. Entry c&0x7ff holds the unique cocode
// word of weight 1 or 3 equivalent to cocode word
// number c, packed as i + (j<<5) + (k<<10) with
// j=k=24 for weight 1. Bit 15 of an even entry is
// set iff that cocode word has weight 2.
//
// recipBasis is the reciprocal basis; its low 12
// rows must each have bit 11 set (odd parity in the
// internal representation). makeSyndromeTable
// panics if that precondition fails.
func makeSyndromeTable(recipBasis []uint32) []uint64 {
	const c1 = (24 << 5) | (24 << 10)
	table := make([]uint64, 0x800)
	rb := make([]uint32, 24)
	for i := 0; i < 24; i++ {
		if recipBasis[i]&0x800 == 0 {
			panic(fmt.Sprintf("makeSyndromeTable: recip row %d lacks bit 11", i))
		}
		rb[i] = recipBasis[i] & 0x7ff
	}
	for i := 0; i < 24; i++ {
		bi := rb[i]
		table[bi] ^= uint64(i) | c1
		for j := i + 1; j < 24; j++ {
			bj := bi ^ rb[j]
			table[bj] ^= 0x8000
			for k := j + 1; k < 24; k++ {
				bk := bj ^ rb[k]
				table[bk] ^= uint64(i) | (uint64(j) << 5) | (uint64(k) << 10)
			}
		}
	}
	for _, e := range table {
		if e&0x7fff == 0 {
			panic("makeSyndromeTable: zero low syndrome entry")
		}
	}
	return table
}

//////////////////////////////////////////////////
// Octad tables
//////////////////////////////////////////////////

// oddOctadsDict reorders the first three set bits
// of an odd MOG column when listing an octad's
// elements, matching ODD_OCTADS_DICT upstream.
var oddOctadsDict = map[uint32][]int{
	7: {0, 1, 2}, 11: {1, 0, 2}, 13: {1, 2, 0}, 14: {2, 1, 0},
}

// octadToBitList returns the ordered element list
// of a weight-8 octad vector, applying the upstream
// ODD_OCTADS_SPECIAL == 1 canonicalization: if the
// lowest MOG column (bits 0..3) has even weight the
// elements are listed in ascending order, otherwise
// the odd column's first three elements are
// reordered per oddOctadsDict and moved to the
// front.
//
// octadToBitList panics if no odd column matches,
// which cannot happen for a valid octad.
func octadToBitList(vector uint32) []int {
	if bitWeight(vector&15)&1 == 0 {
		return bits2List(vector)
	}
	for i := 0; i < 24; i += 4 {
		v3 := vector & (15 << uint(i))
		key := v3 >> uint(i)
		seq, ok := oddOctadsDict[key]
		if !ok {
			continue
		}
		first := bits2List(v3)
		last := bits2List(vector &^ v3)
		out := []int{first[seq[0]], first[seq[1]], first[seq[2]]}
		return append(out, last...)
	}
	panic("octadToBitList: no odd MOG column matched")
}

// makeOctadTables builds the three octad tables
// from the 11-row Golay code basis B (basis[12:23],
// i.e. the code basis minus its all-ones vector).
//
// It returns:
//   - octEnc: 2048 entries; octEnc[gcode] is
//     ((weight-8)>>3) + 2*octad for octads and
//     their complements, else 0xffff;
//   - octDec: 759 entries; octDec[o] is the gcode
//     of octad o, with bit 11 set for the
//     complement (weight-16) case;
//   - octElem: 759*8 entries; the 8 element
//     positions of each octad in canonical order.
//
// makeOctadTables panics if the octad count is not
// exactly 759.
func makeOctadTables(b []uint32) (octEnc, octDec []uint16, octElem []uint8) {
	codewords := basisSpanned(b) // 2**11 = 2048 entries
	octDec = make([]uint16, 759)
	octElem = make([]uint8, 759*8)
	octEnc = make([]uint16, 2048)
	for i := range octEnc {
		octEnc[i] = 0xffff
	}
	octad := 0
	for gcode, vector := range codewords {
		weight := bw24(vector)
		if weight != 8 && weight != 16 {
			continue
		}
		octDec[octad] = uint16(gcode) + uint16((weight&16)<<7)
		octEnc[gcode] = uint16((weight-8)>>3) + uint16(2*octad)
		octVector := vector
		if weight == 16 {
			octVector = vector ^ 0xffffff
		}
		blist := octadToBitList(octVector)
		for j := 0; j < 8; j++ {
			octElem[8*octad+j] = uint8(blist[j])
		}
		octad++
	}
	if octad != 759 {
		panic(fmt.Sprintf("makeOctadTables: got %d octads, want 759", octad))
	}
	return octEnc, octDec, octElem
}

// makeOctadIndexTable builds the 256-entry (64*4)
// suboctad index table. For suboctad number i the
// four entries 4*i..4*i+3 are the octad-element
// indices whose XOR yields the cocode vector of
// suboctad i; the upstream layout pads short lists
// with zeros.
func makeOctadIndexTable() []uint8 {
	bl := make([]uint8, 0, 256)
	for i := uint32(0); i < 64; i++ {
		j := (i << 1) + uint32(bitParity(i))
		if bitWeight(j) > 4 {
			j ^= 0xff
		}
		blist := bits2List(j)
		for len(blist) < 4 {
			blist = append(blist, 0, 0)
		}
		for k := 0; k < 4; k++ {
			bl = append(bl, uint8(blist[k]))
		}
	}
	return bl
}

//////////////////////////////////////////////////
// Parker loop theta table
//////////////////////////////////////////////////

// splitColor maps the low three bits of a MOG
// column entry to its colored part, mirroring the
// color[] table in split_golay_codevector upstream.
var splitColor = [8]uint32{0, 6, 5, 3, 3, 5, 6, 0}

// splitGolayCodevector splits a Golay code vector
// into its blackwhite and colored parts (summing to
// v). The colored part has 0 or 2 set bits in the
// low three rows of each MOG column and none in row
// 0.
func splitGolayCodevector(v uint32) (bw, col uint32) {
	col = 0
	for i := uint(1); i < 25; i += 4 {
		col |= splitColor[(v>>i)&7] << i
	}
	return v ^ col, col
}

// thetaToBasisVector returns theta(v) for a Golay
// code basis vector v in vect representation, by
// the cases of Lemma 3.9 (Seysen) as implemented in
// theta_to_basis_vector upstream.
//
// thetaToBasisVector panics if v is neither purely
// grey nor a single colored weight-8 vector, which
// cannot occur for the standard code basis.
func thetaToBasisVector(v uint32) uint32 {
	bw, col := splitGolayCodevector(v)
	if col != 0 && bw == 0 {
		if bw24(col) != 8 {
			panic("thetaToBasisVector: colored part not weight 8")
		}
		return ((col >> 1) | (col >> 2) | (col >> 3)) & 0x111111
	}
	if col != 0 {
		panic("thetaToBasisVector: mixed grey/colored basis vector")
	}
	bw &= 0x111111
	switch bw24(bw) {
	case 2:
		return bw ^ 0x111111
	case 3:
		return 0x111111
	case 4:
		return bw
	default:
		return 0
	}
}

// makeThetaTable builds the 2048-entry augmented
// Parker loop theta table. The quadratic form theta
// is defined on the code basis by thetaToBasisVector
// and extended via the bilinear form B(x,y) = x ∩ y;
// bits 14..12 of entry i carry the bit weight of
// gcode word i divided by four.
//
// makeThetaTable panics if gcode 0x800 (the all-ones
// code word) does not map to 0xffffff, an internal
// consistency check from upstream.
func (m *mat24) makeThetaTable() []uint64 {
	if m.gcodeToVect(0x800) != 0xffffff {
		panic("makeThetaTable: gcode 0x800 is not all-ones")
	}
	table := make([]uint64, 0x800)

	codeBasis := m.basis[12:24]
	for i := 0; i < 11; i++ {
		theta := thetaToBasisVector(codeBasis[i])
		table[1<<uint(i)] = uint64(m.vectToCocode(theta))
	}
	for i := uint32(0); i < 0x800; i++ {
		if i&(i-1) == 0 {
			continue // skip 0 and powers of two
		}
		i0 := uint32(1) << uint(v2(i))
		i1 := i ^ i0
		capVec := m.gcodeToVect(i0) & m.gcodeToVect(i1)
		cc := m.vectToCocode(capVec)
		table[i] = table[i0] ^ table[i1] ^ uint64(cc)
	}
	for i := uint32(0); i < 0x800; i++ {
		w := bw24(m.gcodeToVect(i))
		switch w {
		case 0, 8, 12, 16:
		default:
			panic(fmt.Sprintf("makeThetaTable: bad code weight %d", w))
		}
		table[i] |= uint64(w>>2) << 12
	}
	return table
}

//////////////////////////////////////////////////
// Driver: compute every table in golden-file order
//////////////////////////////////////////////////

// computeMat24Tables derives all 15 mat24 tables
// from the Golay basis seed and returns them in the
// order they appear in the golden file.
func computeMat24Tables() ([]cTable, error) {
	m := &mat24{}
	m.basis = golayBasisSeed[:]
	m.recipBasis = bitMatInverse(m.basis)

	m.enc0, m.enc1, m.enc2 = encodingTables(m.recipBasis)
	m.dec0, m.dec1, m.dec2 = encodingTables(m.basis)

	lsbit := makeLsbitTable()
	syndrome := makeSyndromeTable(m.recipBasis)
	octEnc, octDec, octElem := makeOctadTables(m.basis[12:23])
	theta := m.makeThetaTable()
	octIndex := makeOctadIndexTable()

	// recip_basis_c pads the 24-row reciprocal basis
	// to 32 entries with zeros so recip_basis[i&31]
	// is always in range in the consuming C/Go code.
	recipPadded := make([]uint32, 32)
	copy(recipPadded, m.recipBasis)

	u8 := cType{"uint8", 2}
	u16 := cType{"uint16", 4}
	u32 := cType{"uint32", 8}

	return []cTable{
		{"mat24LsbitTable", u8, lsbit},
		{"mat24EncTable0", u32, u32vals(m.enc0)},
		{"mat24EncTable1", u32, u32vals(m.enc1)},
		{"mat24EncTable2", u32, u32vals(m.enc2)},
		{"mat24DecTable0", u32, u32vals(m.dec0)},
		{"mat24DecTable1", u32, u32vals(m.dec1)},
		{"mat24DecTable2", u32, u32vals(m.dec2)},
		{"mat24Basis", u32, u32vals(m.basis)},
		{"mat24RecipBasis", u32, u32vals(recipPadded)},
		{"mat24SyndromeTable", u16, syndrome},
		{"mat24OctDecTable", u16, u16vals(octDec)},
		{"mat24OctEncTable", u16, u16vals(octEnc)},
		{"mat24ThetaTable", u16, theta},
		{"mat24OctadElementTable", u8, u8vals(octElem)},
		{"mat24OctadIndexTable", u8, u8vals(octIndex)},
	}, nil
}

// u32vals/u16vals/u8vals widen a typed table to the
// uint64 element form carried by cTable.
func u32vals(xs []uint32) []uint64 {
	out := make([]uint64, len(xs))
	for i, x := range xs {
		out[i] = uint64(x)
	}
	return out
}

func u16vals(xs []uint16) []uint64 {
	out := make([]uint64, len(xs))
	for i, x := range xs {
		out[i] = uint64(x)
	}
	return out
}

func u8vals(xs []uint8) []uint64 {
	out := make([]uint64, len(xs))
	for i, x := range xs {
		out[i] = uint64(x)
	}
	return out
}

//////////////////////////////////////////////////
// Verification against the golden file
//////////////////////////////////////////////////

// goldenVarRe matches the head of a golden var
// declaration, capturing the var name.
var goldenVarRe = regexp.MustCompile(
	`var\s+(mat24[A-Za-z0-9]+)\s*=\s*\[\.\.\.\][A-Za-z0-9]+\{`)

// goldenHexRe matches one hex literal in a golden
// table body.
var goldenHexRe = regexp.MustCompile(`0x[0-9a-fA-F]+`)

// verifyMat24Tables checks every computed table
// against the corresponding var in the golden file
// <cgtDir>/mat24_gen.go.
//
// verifyMat24Tables returns an error if the golden
// file is missing or unparsable, if a computed
// table is absent from the golden file, or if any
// length or element disagrees.
func verifyMat24Tables(tables []cTable, cgtDir string) error {
	path := filepath.Join(cgtDir, "mat24_gen.go")
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read golden %s: %w", path, err)
	}
	golden, err := parseGoldenTables(string(src))
	if err != nil {
		return err
	}

	for _, t := range tables {
		want, ok := golden[t.goName]
		if !ok {
			return fmt.Errorf("verify: golden file has no table %s", t.goName)
		}
		if len(want) != len(t.values) {
			return fmt.Errorf("verify %s: length %d, golden %d",
				t.goName, len(t.values), len(want))
		}
		for i := range t.values {
			if t.values[i] != want[i] {
				return fmt.Errorf(
					"verify %s: index %d computed 0x%x, golden 0x%x",
					t.goName, i, t.values[i], want[i])
			}
		}
	}
	return nil
}

// parseGoldenTables extracts every mat24 table var
// and its hex-literal element values from the
// golden Go source.
//
// parseGoldenTables returns an error if a literal
// fails to parse as an unsigned integer.
func parseGoldenTables(src string) (map[string][]uint64, error) {
	heads := goldenVarRe.FindAllStringSubmatchIndex(src, -1)
	out := make(map[string][]uint64, len(heads))
	for h, head := range heads {
		name := src[head[2]:head[3]]
		// The body runs from the end of this match to
		// the start of the next var head (or EOF).
		bodyStart := head[1]
		bodyEnd := len(src)
		if h+1 < len(heads) {
			bodyEnd = heads[h+1][0]
		}
		body := src[bodyStart:bodyEnd]

		var vals []uint64
		for _, lit := range goldenHexRe.FindAllString(body, -1) {
			v, err := strconv.ParseUint(lit[2:], 16, 64)
			if err != nil {
				return nil, fmt.Errorf("parse golden literal %q in %s: %w",
					lit, name, err)
			}
			vals = append(vals, v)
		}
		out[name] = vals
	}
	return out, nil
}

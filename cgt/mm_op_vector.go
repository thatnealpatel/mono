package cgt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// newData allocates the backing slice for a vector
// modulo p, with the trailing guard entry set. The
// data region (first MMVSize(p) entries) is zero.
func newData(p int) []uint64 {
	n := MMVSize(p)
	d := make([]uint64, n+1)
	d[n] = protectOverflow
	return d
}

// checkP panics if p is not a supported modulus.
func checkP(p int) {
	if mmAuxBadP(p) {
		panic(fmt.Sprintf("cgt: bad modulus p = %d", p))
	}
}

// ZeroVector returns the zero vector modulo p.
//
// ZeroVector panics if p is not a supported modulus.
func ZeroVector(p int) *MMVector {
	checkP(p)
	return &MMVector{p: p, data: newData(p)}
}

// MMV returns a constructor for basis vectors of the
// representation modulo p, mirroring the Python MMV.
//
// MMV panics if p is not a supported modulus.
func MMV(p int) func(tag Tag, i0, i1 int) *MMVector {
	checkP(p)
	return func(tag Tag, i0, i1 int) *MMVector {
		return BasisVector(p, tag, i0, i1)
	}
}

// tupleToSparse converts a single tuple to its
// one-entry sparse representation, following the
// Python tuple_to_sparse helpers. Returns the sparse
// words to add. Most tuples yield one word; tags I/J
// yield four and U yields 24.
func tupleToSparse(p int, t Tuple) []uint32 {
	sc := func(f int) uint32 {
		r := f % p
		if r < 0 {
			r += p
		}
		return uint32(r)
	}
	i0 := uint32(t.I0)
	i1 := uint32(t.I1)
	switch t.Tag {
	case TagA:
		if i0 < i1 {
			i0, i1 = i1, i0
		}
		return []uint32{mmSpaceTagA + (i0 << 14) + (i1 << 8) + sc(t.Factor)}
	case TagB, TagC:
		if i0 == i1 {
			panic("cgt: equal indices illegal for tag B/C")
		}
		if i0 < i1 {
			i0, i1 = i1, i0
		}
		off := uint32(mmSpaceTagB)
		if t.Tag == TagC {
			off = mmSpaceTagC
		}
		return []uint32{off + (i0 << 14) + (i1 << 8) + sc(t.Factor)}
	case TagT:
		return []uint32{mmSpaceTagT + i0*0x4000 + i1*0x100 + sc(t.Factor)}
	case TagX, TagZ, TagY:
		off := map[Tag]uint32{TagX: mmSpaceTagX, TagZ: mmSpaceTagZ, TagY: mmSpaceTagY}[t.Tag]
		return []uint32{off + i0*0x4000 + i1*0x100 + sc(t.Factor)}
	default:
		panic(fmt.Sprintf("cgt: bad tag %d", t.Tag))
	}
}

// NewVector returns the linear combination of the
// basis vectors described by tuples, modulo p.
//
// NewVector panics if p is not a supported modulus or
// any tuple is malformed.
func NewVector(p int, tuples []Tuple) *MMVector {
	checkP(p)
	v := &MMVector{p: p, data: newData(p)}
	var sp []uint32
	for _, t := range tuples {
		sp = append(sp, tupleToSparse(p, t)...)
	}
	if len(sp) > 0 {
		mmvAddSparse(p, sp, len(sp), v.data)
	}
	return v
}

// BasisVector returns the basis vector (tag, i0, i1)
// modulo p.
//
// BasisVector panics if p is not a supported modulus
// or the tuple is malformed.
func BasisVector(p int, tag Tag, i0, i1 int) *MMVector {
	return NewVector(p, []Tuple{{Factor: 1, Tag: tag, I0: i0, I1: i1}})
}

// NewVectorA returns the basis vector ('A', i0, i1)
// modulo p.
//
// NewVectorA panics if p is not a supported modulus.
func NewVectorA(p, i0, i1 int) *MMVector {
	return BasisVector(p, TagA, i0, i1)
}

// NewVectorJ returns the axis ('J', i0, i1) modulo p:
// (A,i0,i0)+(A,i1,i1)-(A,i0,i1)+2(B,i0,i1). C/Python
// special tag 'J'.
//
// NewVectorJ panics if p is not a supported modulus
// or i0 == i1.
func NewVectorJ(p, i0, i1 int) *MMVector {
	return newVectorIJ(p, -1, i0, i1)
}

// newVectorIJ builds the I/J special-tag axes. For
// tag I sign=1, for tag J sign=-1. C/Python
// gen_unit_IJ.
func newVectorIJ(p, sign, i0, i1 int) *MMVector {
	checkP(p)
	if i0 == i1 {
		panic("cgt: equal indices illegal for tag I/J")
	}
	// b = ('B', -2*sign, i0, i1); reorder via B rule.
	b := tupleToSparse(p, Tuple{Factor: -2 * sign, Tag: TagB, I0: i0, I1: i1})[0]
	j0 := int((b >> 14) & 0x1f)
	j1 := int((b >> 8) & 0x1f)
	tuples := []Tuple{
		{Factor: 1, Tag: TagA, I0: j0, I1: j0},
		{Factor: 1, Tag: TagA, I0: j1, I1: j1},
		{Factor: -1, Tag: TagA, I0: j0, I1: j1},
	}
	v := NewVector(p, tuples)
	bSp := []uint32{b}
	mmvAddSparse(p, bSp, 1, v.data)
	return v
}

// FromBytes constructs a vector modulo p from the
// external byte array b (length 196884). Entries are
// reduced modulo p. C/Python from_bytes.
//
// FromBytes panics if p is not a supported modulus or
// len(b) != 196884.
func FromBytes(p int, b []uint8) *MMVector {
	checkP(p)
	if len(b) != mmAuxXlenV {
		panic("cgt: byte vector must have length 196884")
	}
	bb := make([]uint8, mmAuxXlenV)
	for i, x := range b {
		bb[i] = uint8(int(x) % p)
	}
	v := &MMVector{p: p, data: newData(p)}
	bytesToMMV(p, bb, v.data)
	return v
}

// FromSparse constructs a vector modulo p from a
// sparse-representation array.
//
// FromSparse panics if p is not a supported modulus.
func FromSparse(p int, sparse []uint32) *MMVector {
	checkP(p)
	v := &MMVector{p: p, data: newData(p)}
	if len(sparse) > 0 {
		sp := make([]uint32, len(sparse))
		copy(sp, sparse)
		mmvAddSparse(p, sp, len(sp), v.data)
	}
	return v
}

// RandVector is not implemented: the deterministic
// oracle does not exercise it and the C version
// depends on gen_random.c's RNG state.
//
// RandVector always panics.
func RandVector(p int) *MMVector {
	checkP(p)
	panic("cgt: RandVector not implemented (no deterministic oracle)")
}

// P returns the modulus of the vector.
func (v *MMVector) P() int { return v.p }

// Data returns the backing slice (including the guard
// entry). The slice is not copied.
func (v *MMVector) Data() []uint64 { return v.data }

// Copy returns a deep copy of the vector.
func (v *MMVector) Copy() *MMVector {
	d := make([]uint64, len(v.data))
	copy(d, v.data)
	return &MMVector{p: v.p, data: d}
}

// errMMVCheck reports a vector validation failure.
var errMMVCheck = errors.New("cgt: MM vector check failed")

// Check validates the vector, reducing it as a side
// effect. It returns an error if a structural problem
// is found. C/Python check.
func (v *MMVector) Check() error {
	res := checkMMV(v.p, v.data)
	if res == 0 {
		return nil
	}
	msgs := map[int]string{
		-1: "bad input value p",
		-2: "a one bit outside a field has been found",
		-3: "a subfield has an illegal nonzero entry at index >= 24",
		-4: "illegal nonzero diagonal entry",
		-5: "symmetric part of vector is not symmetric",
	}
	if m, ok := msgs[res]; ok {
		return fmt.Errorf("%w: %s", errMMVCheck, m)
	}
	return fmt.Errorf("%w: code %d", errMMVCheck, res)
}

// checkSameP panics if other has a different modulus.
func (v *MMVector) checkSameP(other *MMVector) {
	if v.p != other.p {
		panic("cgt: cannot combine MM vectors modulo different p")
	}
}

// Add returns v + other (a new vector).
//
// Add panics if the moduli differ.
func (v *MMVector) Add(other *MMVector) *MMVector {
	v.checkSameP(other)
	r := v.Copy()
	opVectorAdd(r.p, r.data, other.data)
	return r
}

// Sub returns v - other (a new vector).
//
// Sub panics if the moduli differ.
func (v *MMVector) Sub(other *MMVector) *MMVector {
	v.checkSameP(other)
	r := other.Copy()
	opScalarMul(r.p, r.p-1, r.data) // negate
	opVectorAdd(r.p, r.data, v.data)
	return r
}

// MulScalar returns a*v (a new vector).
func (v *MMVector) MulScalar(a int) *MMVector {
	r := v.Copy()
	m := a % v.p
	if m < 0 {
		m += v.p
	}
	opScalarMul(r.p, m, r.data)
	return r
}

// Hash returns a hash value of the vector. C/Python
// hash.
func (v *MMVector) Hash() uint64 {
	return Hash(v.p, v.data, 0)
}

// Equal reports whether v and other are equal,
// reducing both first.
func (v *MMVector) Equal(other *MMVector) bool {
	if v.p != other.p {
		return false
	}
	reduceMMV(v.p, v.data)
	reduceMMV(other.p, other.data)
	n := MMVSize(v.p)
	for i := 0; i < n; i++ {
		if v.data[i] != other.data[i] {
			return false
		}
	}
	return true
}

// AsBytes returns the external byte representation
// (length 196884), reduced modulo p.
func (v *MMVector) AsBytes() []uint8 {
	b := make([]uint8, mmAuxXlenV)
	mmvToBytes(v.p, v.data, b)
	return b
}

// AsSparse returns the nonzero entries in sparse
// representation.
func (v *MMVector) AsSparse() []uint32 {
	sp := make([]uint32, mmAuxXlenV)
	length := mmvToSparse(v.p, v.data, sp)
	if length < 0 {
		panic(fmt.Sprintf("cgt: AsSparse failed with code %d", length))
	}
	return sp[:length]
}

// AsTuples returns the nonzero entries as tuples
// (factor, tag, i0, i1). C/Python sparse_to_tuples.
func (v *MMVector) AsTuples() []Tuple {
	sp := v.AsSparse()
	out := make([]Tuple, 0, len(sp))
	for _, i := range sp {
		if i&0xe000000 == 0 {
			continue
		}
		out = append(out, Tuple{
			Factor: int(i & 0xff),
			Tag:    Tag((i >> 25) & 7),
			I0:     int((i >> 14) & 0x7ff),
			I1:     int((i >> 8) & 0x3f),
		})
	}
	return out
}

// At returns the coordinate of basis vector
// (tag, i0, i1).
func (v *MMVector) At(tag Tag, i0, i1 int) int {
	sp := tupleToSparse(v.p, Tuple{Factor: 0, Tag: tag, I0: i0, I1: i1})[0]
	return int(mmvGetSparse(v.p, v.data, sp) & uint32(v.p))
}

// Set sets the coordinate of basis vector
// (tag, i0, i1) to value.
func (v *MMVector) Set(tag Tag, i0, i1, value int) {
	m := value % v.p
	if m < 0 {
		m += v.p
	}
	sp := tupleToSparse(v.p, Tuple{Factor: m, Tag: tag, I0: i0, I1: i1})[0]
	sp1 := []uint32{sp}
	mmvSetSparse(v.p, v.data, sp1, 1)
}

// Entry returns the i-th coordinate in linear
// (external) order. C/Python v['E', i].
func (v *MMVector) Entry(i int) int {
	sp := IndexExternToSparse(i)
	return int(mmvGetSparse(v.p, v.data, sp) & uint32(v.p))
}

// GetSparse updates the supplied sparse labels with
// the coordinates of v and returns them. C
// getitems_sparse.
func (v *MMVector) GetSparse(sparse []uint32) []uint32 {
	out := make([]uint32, len(sparse))
	copy(out, sparse)
	if len(out) > 0 {
		mmvExtractSparse(v.p, v.data, out, len(out))
	}
	return out
}

// Mul returns v * g, where g is a word of Monster
// generator atoms. The vector is not modified. C/
// Python imul_group_word via mm_op_word.
//
// Mul panics if the underlying OpWord fails.
func (v *MMVector) Mul(g []uint32) *MMVector {
	return v.MulExp(g, 1, false)
}

// MulExp returns v * g^e. If breakG is set, each
// factor g is applied separately. C/Python
// vector_mul_exp via mm_op_word.
func (v *MMVector) MulExp(g []uint32, e int, breakG bool) *MMVector {
	r := v.Copy()
	gg := g
	length := len(g)
	if breakG {
		gg = make([]uint32, length+1)
		copy(gg, g)
		gg[length] = 0x70000000
		length++
	}
	work := make([]uint64, len(r.data))
	if err := OpWord(r.p, r.data, gg, length, e, work); err != nil {
		panic(err.Error())
	}
	return r
}

// Projection returns the projection of v onto the
// span of the basis vectors named by tuples: a new
// vector whose coordinate at each named basis vector
// equals that of v and which is zero elsewhere.
func (v *MMVector) Projection(tuples []Tuple) *MMVector {
	out := ZeroVector(v.p)
	for _, t := range tuples {
		val := v.At(t.Tag, t.I0, t.I1)
		out.Set(t.Tag, t.I0, t.I1, val)
	}
	return out
}

// ParseVector parses a vector expression such as
// "A_2_2+A_3_3-A_2_3+2*B_2_3" modulo p, mirroring the
// mmgroup string syntax (decimal indices, hex with a
// trailing 'h' or a 0x prefix, operators + - *).
func ParseVector(p int, s string) (*MMVector, error) {
	if mmAuxBadP(p) {
		return nil, fmt.Errorf("cgt: bad modulus p = %d", p)
	}
	pr := &vecParser{p: p, s: s}
	v, err := pr.parseExpr()
	if err != nil {
		return nil, err
	}
	pr.skipSpace()
	if pr.pos != len(pr.s) {
		return nil, fmt.Errorf("cgt: trailing input in vector %q at %d", s, pr.pos)
	}
	return v, nil
}

// vecParser is a recursive-descent parser for the
// vector expression grammar.
type vecParser struct {
	p   int
	s   string
	pos int
}

func (pr *vecParser) skipSpace() {
	for pr.pos < len(pr.s) && pr.s[pr.pos] == ' ' {
		pr.pos++
	}
}

// parseExpr handles + and - at the lowest precedence.
func (pr *vecParser) parseExpr() (*MMVector, error) {
	left, err := pr.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		pr.skipSpace()
		if pr.pos >= len(pr.s) {
			return left, nil
		}
		op := pr.s[pr.pos]
		if op != '+' && op != '-' {
			return left, nil
		}
		pr.pos++
		right, err := pr.parseTerm()
		if err != nil {
			return nil, err
		}
		if op == '+' {
			left = left.Add(right)
		} else {
			left = left.Sub(right)
		}
	}
}

// parseTerm handles * (scalar times vector).
func (pr *vecParser) parseTerm() (*MMVector, error) {
	pr.skipSpace()
	// Optional leading unary minus.
	neg := false
	for pr.pos < len(pr.s) && (pr.s[pr.pos] == '+' || pr.s[pr.pos] == '-') {
		if pr.s[pr.pos] == '-' {
			neg = !neg
		}
		pr.pos++
		pr.skipSpace()
	}
	// A term is either <int> '*' <atom> or <atom>.
	if n, ok := pr.tryInt(); ok {
		pr.skipSpace()
		if pr.pos < len(pr.s) && pr.s[pr.pos] == '*' {
			pr.pos++
			v, err := pr.parseAtom()
			if err != nil {
				return nil, err
			}
			f := n
			if neg {
				f = -f
			}
			return v.MulScalar(f), nil
		}
		return nil, fmt.Errorf("cgt: bare integer in vector %q", pr.s)
	}
	v, err := pr.parseAtom()
	if err != nil {
		return nil, err
	}
	if neg {
		return v.MulScalar(-1), nil
	}
	return v, nil
}

// tryInt parses a leading decimal integer if present.
func (pr *vecParser) tryInt() (int, bool) {
	start := pr.pos
	for pr.pos < len(pr.s) && pr.s[pr.pos] >= '0' && pr.s[pr.pos] <= '9' {
		pr.pos++
	}
	if pr.pos == start {
		return 0, false
	}
	// A digit run immediately followed by '_' or a
	// letter is an atom index, not a coefficient.
	if pr.pos < len(pr.s) && (pr.s[pr.pos] == '_') {
		pr.pos = start
		return 0, false
	}
	n, err := strconv.Atoi(pr.s[start:pr.pos])
	if err != nil {
		pr.pos = start
		return 0, false
	}
	return n, true
}

// parseAtom parses an atom like A_2_3 or X_12h_2.
func (pr *vecParser) parseAtom() (*MMVector, error) {
	pr.skipSpace()
	if pr.pos >= len(pr.s) {
		return nil, fmt.Errorf("cgt: expected atom in %q", pr.s)
	}
	tagCh := pr.s[pr.pos]
	tag, ok := parseTagLetter(tagCh)
	if !ok {
		return nil, fmt.Errorf("cgt: bad tag %q in %q", string(tagCh), pr.s)
	}
	pr.pos++
	if pr.pos >= len(pr.s) || pr.s[pr.pos] != '_' {
		return nil, fmt.Errorf("cgt: expected '_' after tag in %q", pr.s)
	}
	// Collect the underscore-separated index fields.
	rest := pr.s[pr.pos+1:]
	end := 0
	for end < len(rest) {
		c := rest[end]
		if c == '_' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') ||
			c == 'x' || c == 'h' || c == 'H' {
			end++
			continue
		}
		break
	}
	field := rest[:end]
	pr.pos += 1 + end
	parts := strings.Split(field, "_")
	if len(parts) != 2 {
		return nil, fmt.Errorf("cgt: tag needs two indices in %q", pr.s)
	}
	i0, err := parseIndex(parts[0])
	if err != nil {
		return nil, err
	}
	i1, err := parseIndex(parts[1])
	if err != nil {
		return nil, err
	}
	return BasisVector(pr.p, tag, i0, i1), nil
}

// parseTagLetter maps a tag letter to a Tag.
func parseTagLetter(c byte) (Tag, bool) {
	switch c {
	case 'A':
		return TagA, true
	case 'B':
		return TagB, true
	case 'C':
		return TagC, true
	case 'T':
		return TagT, true
	case 'X':
		return TagX, true
	case 'Z':
		return TagZ, true
	case 'Y':
		return TagY, true
	}
	return 0, false
}

// parseIndex parses an index field: decimal by
// default, hex if it has a 0x prefix or trailing 'h'.
func parseIndex(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("cgt: empty index")
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, err := strconv.ParseInt(s[2:], 16, 64)
		return int(n), err
	}
	if c := s[len(s)-1]; c == 'h' || c == 'H' {
		n, err := strconv.ParseInt(s[:len(s)-1], 16, 64)
		return int(n), err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return int(n), err
}

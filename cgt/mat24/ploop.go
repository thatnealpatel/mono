package mat24

import "fmt"

// Parity models an element of GF(2), the parity
// of a bit vector.
type Parity int

// NewParity returns the parity of v.
func NewParity(v int) Parity { return Parity(v & 1) }

// Int returns 0 for even and 1 for odd.
func (p Parity) Int() int { return int(p) & 1 }

// Sign returns (-1)**p, i.e. 1 if even and -1 if
// odd.
func (p Parity) Sign() int { return 1 - 2*p.Int() }

// Equal reports whether p and other have the same
// parity.
func (p Parity) Equal(other Parity) bool { return p.Int() == other.Int() }

// Add returns the sum of p and other in GF(2).
func (p Parity) Add(other Parity) Parity { return p ^ other }

// gcodeFromVect24 corrects bit vector v to the
// nearest Golay code word and returns its Golay
// code number.
func gcodeFromVect24(v uint32) uint16 {
	v ^= Syndrome(v, 24)
	return uint16(VectToGcode(v))
}

// GCode models a code word of the binary Golay
// code, numbered from 0 to 0xfff.
type GCode struct {
	value uint16
}

// NewGCode builds a Golay code word from obj.
//
// NewGCode accepts an int (code word number), a
// GCode, a PLoop (sign dropped), or a []int of
// bit positions (corrected to the nearest code
// word). NewGCode panics on any other type.
func NewGCode(obj any) GCode {
	switch v := obj.(type) {
	case int:
		return GCodeFromInt(v)
	case GCode:
		return GCode{v.value & 0xfff}
	case PLoop:
		return GCode{v.value & 0xfff}
	case []int:
		return GCodeFromBitList(v)
	default:
		panic(fmt.Sprintf("NewGCode: unsupported type %T", obj))
	}
}

// GCodeFromInt builds a Golay code word from its
// code word number n, reduced to 0 <= n < 0x1000.
func GCodeFromInt(n int) GCode { return GCode{uint16(n) & 0xfff} }

// GCodeFromBitList builds a Golay code word from a
// list of bit positions, corrected to the nearest
// code word.
func GCodeFromBitList(bits []int) GCode {
	var vect uint32
	for _, x := range bits {
		vect ^= 1 << uint(x)
	}
	return GCode{gcodeFromVect24(vect)}
}

// Ord returns the Golay code word number, with
// 0 <= n < 0x1000.
func (g GCode) Ord() uint16 { return g.value & 0xfff }

// Len returns the bit weight of the code word.
func (g GCode) Len() int { return int(GcodeWeight(uint32(g.value))) << 2 }

// Vector returns the bit vector of the code word
// as a 24-bit number.
func (g GCode) Vector() uint32 { return GcodeToVect(uint32(g.value)) }

// BitList returns the ascending positions of the
// set bits of the code word.
func (g GCode) BitList() []int {
	bl := GcodeToBitList(uint32(g.value))
	out := make([]int, len(bl))
	for i, b := range bl {
		out[i] = int(b)
	}
	return out
}

// Octad returns the octad number of the code
// word. Complements of octads are accepted.
//
// Octad panics if the code word is not an octad.
func (g GCode) Octad() int { return int(GcodeToOctad(uint32(g.value), 0)) }

// Add returns the sum g + other of code words.
func (g GCode) Add(other GCode) GCode {
	return GCode{(g.value ^ other.value) & 0xfff}
}

// And returns the bitwise intersection of g and
// other as a cocode word.
func (g GCode) And(other GCode) Cocode {
	return Cocode{uint16(PloopCap(uint32(g.value), uint32(other.value))) & 0xfff}
}

// ScalarProd returns the scalar product of g and
// cocode word c as a parity.
func (g GCode) ScalarProd(c Cocode) Parity {
	return NewParity(int(ScalarProd(uint32(g.value), uint32(c.value))))
}

// Theta returns the Parker loop cocycle theta(g)
// as a cocode word.
func (g GCode) Theta() Cocode {
	return Cocode{uint16(PloopTheta(uint32(g.value)))}
}

// ThetaWith returns the cocycle theta(g, other)
// as a parity.
func (g GCode) ThetaWith(other GCode) Parity {
	th := PloopTheta(uint32(g.value))
	return NewParity(int(ScalarProd(uint32(other.value), th)))
}

// Invert returns the bitwise complement of the
// code word.
func (g GCode) Invert() GCode { return GCode{g.value ^ 0x800} }

// Apply returns the image of g under the Parker
// loop automorphism a.
func (g GCode) Apply(a *AutPL) GCode {
	return GCode{uint16(OpGcodePerm(uint32(g.value), bytePerm(a.perm))) & 0xfff}
}

// cocodeFromVect24 returns the cocode number of
// bit vector v.
func cocodeFromVect24(v uint32) uint16 {
	return uint16(VectToCocode(v))
}

// Cocode models an element of the cocode of the
// Golay code, numbered from 0 to 0xfff.
type Cocode struct {
	value uint16
}

// NewCocode builds a cocode element from obj.
//
// NewCocode accepts an int (cocode number), a
// Cocode, or a []int of bit positions. NewCocode
// panics on any other type.
func NewCocode(obj any) Cocode {
	switch v := obj.(type) {
	case int:
		return CocodeFromInt(v)
	case Cocode:
		return Cocode{v.value}
	case []int:
		return CocodeFromBitList(v)
	default:
		panic(fmt.Sprintf("NewCocode: unsupported type %T", obj))
	}
}

// CocodeFromInt builds a cocode element from its
// number n, reduced to 0 <= n < 0x1000.
func CocodeFromInt(n int) Cocode { return Cocode{uint16(n) & 0xfff} }

// CocodeFromBitList builds a cocode element from a
// list of bit positions.
func CocodeFromBitList(bits []int) Cocode {
	var vect uint32
	for _, x := range bits {
		vect ^= 1 << uint(x)
	}
	return Cocode{cocodeFromVect24(vect)}
}

// Ord returns the cocode element number, with
// 0 <= n < 0x1000.
func (c Cocode) Ord() uint16 { return c.value & 0xfff }

// Len returns the bit weight of the shortest
// representative of the cocode element.
func (c Cocode) Len() int { return int(CocodeWeight(uint32(c.value))) }

// Parity returns the parity of the cocode
// element.
func (c Cocode) Parity() Parity { return NewParity(int((c.value >> 11) & 1)) }

// Add returns the sum c + other of cocode
// elements.
func (c Cocode) Add(other Cocode) Cocode {
	return Cocode{(c.value ^ other.value) & 0xfff}
}

// Syndrome returns the syndrome of the cocode
// element as a bit vector. If the syndrome is not
// unique the one containing bit i is returned.
//
// Syndrome panics if i is out of range.
func (c Cocode) Syndrome(i int) uint32 {
	return CocodeSyndrome(uint32(c.value), uint32(i))
}

// SyndromeList returns the syndrome of the cocode
// element as ascending bit positions. See
// Syndrome for the meaning of i.
//
// SyndromeList panics if i is out of range.
func (c Cocode) SyndromeList(i int) []int {
	bl := CocodeToBitList(uint32(c.value), uint32(i))
	out := make([]int, len(bl))
	for j, b := range bl {
		out[j] = int(b)
	}
	return out
}

// AllSyndromes returns all minimum-weight
// syndromes of the cocode element as bit vectors.
func (c Cocode) AllSyndromes() []uint32 {
	return CocodeAllSyndromes(uint32(c.value))
}

// Apply returns the image of c under the Parker
// loop automorphism a.
func (c Cocode) Apply(a *AutPL) Cocode {
	return Cocode{uint16(OpCocodePerm(uint32(c.value), bytePerm(a.perm))) & 0xfff}
}

// PLoop models an element of the Parker loop,
// numbered from 0 to 0x1fff.
type PLoop struct {
	value uint16
}

// NewPLoop builds a Parker loop element from obj.
//
// NewPLoop accepts an int (element number), a
// PLoop, a GCode (positive element), or a []int
// of bit positions (corrected to the nearest code
// word, positive). NewPLoop panics on any other
// type.
func NewPLoop(obj any) PLoop {
	switch v := obj.(type) {
	case int:
		return PLoopFromInt(v)
	case PLoop:
		return PLoop{v.value & 0x1fff}
	case GCode:
		return PLoop{v.value & 0xfff}
	case []int:
		return PLoopFromBitList(v)
	default:
		panic(fmt.Sprintf("NewPLoop: unsupported type %T", obj))
	}
}

// PLoopFromInt builds a Parker loop element from
// its number n, reduced to 0 <= n < 0x2000.
func PLoopFromInt(n int) PLoop { return PLoop{uint16(n) & 0x1fff} }

// PLoopFromBitList builds a positive Parker loop
// element from a list of bit positions, corrected
// to the nearest code word.
func PLoopFromBitList(bits []int) PLoop {
	var vect uint32
	for _, x := range bits {
		vect ^= 1 << uint(x)
	}
	return PLoop{gcodeFromVect24(vect)}
}

// PLoopZ returns the central element
// (-1)**e1 * Omega**eo of the Parker loop.
func PLoopZ(e1, eo int) PLoop {
	return PLoop{uint16(((e1 & 1) << 12) | ((eo & 1) << 11))}
}

// Ord returns the Parker loop element number,
// with 0 <= n < 0x2000.
func (p PLoop) Ord() uint16 { return p.value & 0x1fff }

// Sign returns 1 for a positive and -1 for a
// negative element.
func (p PLoop) Sign() int { return 1 - int((p.value>>11)&2) }

// Len returns the bit weight of the underlying
// Golay code word.
func (p PLoop) Len() int { return int(GcodeWeight(uint32(p.value))) << 2 }

// GCode returns the Golay code word of the
// element, dropping its sign.
func (p PLoop) GCode() GCode { return GCode{p.value & 0xfff} }

// Theta returns the Parker loop cocycle theta(p)
// as a cocode word.
func (p PLoop) Theta() Cocode {
	return Cocode{uint16(PloopTheta(uint32(p.value)))}
}

// Mul returns the Parker loop product p * other.
func (p PLoop) Mul(other PLoop) PLoop {
	return PLoop{uint16(MulPloop(uint32(p.value), uint32(other.value))) & 0x1fff}
}

// Pow returns the Parker loop power p**e.
func (p PLoop) Pow(e int) PLoop {
	return PLoop{uint16(PowPloop(uint32(p.value), uint32(e&3))) & 0x1fff}
}

// Neg returns the negative -p of the element.
func (p PLoop) Neg() PLoop { return PLoop{p.value ^ 0x1000} }

// Invert returns p * Omega, the bitwise
// complement of the code word, with the sign
// unchanged.
func (p PLoop) Invert() PLoop { return PLoop{p.value ^ 0x800} }

// Abs returns the positive element of {p, -p}.
func (p PLoop) Abs() PLoop { return PLoop{p.value & 0xfff} }

// Cap returns the bitwise intersection of the
// underlying code words of p and other as a
// cocode word.
func (p PLoop) Cap(other PLoop) Cocode {
	return Cocode{uint16(PloopCap(uint32(p.value), uint32(other.value))) & 0xfff}
}

// Comm returns the Parker loop commutator bit of
// p and other.
func (p PLoop) Comm(other PLoop) int {
	return int(PloopComm(uint32(p.value), uint32(other.value)))
}

// Assoc returns the Parker loop associator bit of
// p, b, and c.
func (p PLoop) Assoc(b, c PLoop) int {
	return int(PloopAssoc(uint32(p.value), uint32(b.value), uint32(c.value)))
}

// Split splits sign and Omega from p, returning
// (es, eo, v) with p = (-1)**es * Omega**eo * v
// and 0 <= v.Ord() < 0x800.
func (p PLoop) Split() (int, int, PLoop) {
	v := uint32(p.value)
	return int((v >> 12) & 1), int((v >> 11) & 1), PLoop{uint16(v & 0x7ff)}
}

// SplitOctad splits p into a central element and
// an octad, returning (es, eo, o) with
// p = (-1)**es * Omega**eo * o, where o is the
// neutral element or a positive octad.
//
// SplitOctad panics if p is not Omega**eo times
// a central element times an octad.
func (p PLoop) SplitOctad() (int, int, PLoop) {
	v := uint32(p.value)
	es := int((v >> 12) & 1)
	eo := 0
	w := GcodeWeight(v)
	if w > 3 {
		v ^= 0x800
		eo = 1
		w = 6 - w
	}
	if w <= 2 {
		return es, eo, PLoop{uint16(v & 0xfff)}
	}
	panic("SplitOctad: cannot convert Golay code word to octad")
}

// Apply returns the image of p under the Parker
// loop automorphism a.
func (p PLoop) Apply(a *AutPL) PLoop {
	return PLoop{uint16(OpPloopAutpl(uint32(p.value), a.rep)) & 0x1fff}
}

// Octad models a (signed) octad as a Parker loop
// element.
type Octad struct {
	value uint16
}

// NewOctad returns the positive octad with number
// o as a Parker loop element.
//
// NewOctad panics if o is out of range.
func NewOctad(o int) Octad {
	if o < 0 || o >= 759 {
		panic("NewOctad: octad number out of range")
	}
	return Octad{uint16(OctadToGcode(uint32(o))) & 0xfff}
}

// Octad returns the octad number of the element.
//
// Octad panics if the element is not an octad.
func (o Octad) Octad() int { return int(GcodeToOctad(uint32(o.value), 0)) }

// GCode returns the Golay code word number of the
// octad.
func (o Octad) GCode() uint16 { return o.value & 0xfff }

// bytePerm converts a permutation given as []int
// to the []byte form used by the mat24 helpers.
func bytePerm(p []int) []byte {
	// C uses uint8_t p[32]. The syndrome table
	// stores 24 for unused bit-positions (weight-1
	// cocode words), so OpCocodePerm indexes p[24];
	// the zero padding neutralizes the XOR.
	n := len(p)
	if n < 32 {
		n = 32
	}
	out := make([]byte, n)
	for i, x := range p {
		out[i] = byte(x)
	}
	return out
}

// intPerm converts a permutation given as []byte
// to []int.
func intPerm(p []byte) []int {
	out := make([]int, len(p))
	for i, x := range p {
		out[i] = int(x)
	}
	return out
}

// AutPL models a standard automorphism of the
// Parker loop, an element of 2^12.M24.
type AutPL struct {
	cocode  uint16
	permNum uint32
	perm    []int
	rep     []uint32
}

// NewAutPL builds a Parker loop automorphism from
// a cocode element d and a permutation p.
//
// d may be an int (cocode number) or a Cocode. p
// may be an int (M24 permutation number), a []int
// permutation list, or a map[int]int partial map
// that extends to a unique M24 permutation.
//
// NewAutPL panics on an unsupported argument type,
// an out-of-range permutation number, or a map
// that does not extend to a unique M24
// permutation.
func NewAutPL(d, p any) *AutPL {
	var cocode uint16
	switch v := d.(type) {
	case int:
		cocode = uint16(v) & 0xfff
	case Cocode:
		cocode = v.value & 0xfff
	default:
		panic(fmt.Sprintf("NewAutPL: unsupported cocode type %T", d))
	}

	var permNum uint32
	switch v := p.(type) {
	case int:
		if v < 0 || uint32(v) >= Mat24Order {
			panic("NewAutPL: permutation number out of range")
		}
		permNum = uint32(v)
	case []int:
		pb := bytePerm(v)
		if err := PermCheck(pb); err != nil {
			panic("NewAutPL: permutation is not in M24")
		}
		permNum = PermToM24num(pb)
	case map[int]int:
		h1 := make([]byte, 0, len(v))
		h2 := make([]byte, 0, len(v))
		for k, val := range v {
			h1 = append(h1, byte(k))
			h2 = append(h2, byte(val))
		}
		res, perm, err := PermFromMap(h1, h2)
		if err != nil || res < 1 {
			panic("NewAutPL: permutation is not in M24")
		}
		if res > 1 {
			panic("NewAutPL: permutation in M24 is not unique")
		}
		permNum = PermToM24num(perm)
	default:
		panic(fmt.Sprintf("NewAutPL: unsupported permutation type %T", p))
	}

	return newAutPLFromNumbers(cocode, permNum)
}

// AutPLFromCocodePermNum builds a Parker loop
// automorphism from a cocode element number and an
// M24 permutation number.
//
// AutPLFromCocodePermNum panics if permNum is out
// of range.
func AutPLFromCocodePermNum(cocode int, permNum uint32) *AutPL {
	if permNum >= Mat24Order {
		panic("AutPLFromCocodePermNum: permutation number out of range")
	}
	return newAutPLFromNumbers(uint16(cocode)&0xfff, permNum)
}

// newAutPLFromNumbers builds an AutPL from its
// cocode element and M24 permutation number,
// computing the permutation and the rep matrix.
func newAutPLFromNumbers(cocode uint16, permNum uint32) *AutPL {
	perm := M24numToPerm(permNum)
	rep := PermToAutpl(uint32(cocode), perm)
	return &AutPL{
		cocode:  cocode,
		permNum: permNum,
		perm:    intPerm(perm),
		rep:     rep,
	}
}

// newAutPLFromRep builds an AutPL from its rep
// matrix, computing the permutation, permutation
// number, and cocode element.
func newAutPLFromRep(rep []uint32) *AutPL {
	perm := AutplToPerm(rep)
	return &AutPL{
		cocode:  uint16(AutplToCocode(rep)),
		permNum: PermToM24num(perm),
		perm:    intPerm(perm),
		rep:     rep,
	}
}

// Cocode returns the cocode element number of the
// automorphism.
func (a *AutPL) Cocode() uint16 { return a.cocode }

// PermNum returns the M24 permutation number of
// the automorphism.
func (a *AutPL) PermNum() uint32 { return a.permNum }

// Perm returns the permutation of the
// automorphism as a list mapping i to perm[i].
func (a *AutPL) Perm() []int { return a.perm }

// Parity returns the parity of the automorphism,
// i.e. s where Omega maps to (-1)**s * Omega.
func (a *AutPL) Parity() Parity { return NewParity(int((a.cocode >> 11) & 1)) }

// Mul returns the group product a * other.
func (a *AutPL) Mul(other *AutPL) *AutPL {
	return newAutPLFromRep(MulAutpl(a.rep, other.rep))
}

// Inv returns the group inverse of a.
func (a *AutPL) Inv() *AutPL {
	return newAutPLFromRep(InvAutpl(a.rep))
}

// Pow returns the power a**e.
func (a *AutPL) Pow(e int) *AutPL {
	if e < 0 {
		a = a.Inv()
		e = -e
	}
	res := NewAutPL(0, 0)
	for i := 0; i < e; i++ {
		res = res.Mul(a)
	}
	return res
}

// Equal reports whether a and other are the same
// automorphism.
func (a *AutPL) Equal(other *AutPL) bool {
	return a.cocode == other.cocode && a.permNum == other.permNum
}

// Check verifies the internal consistency of a
// and returns a. Check panics on inconsistency.
func (a *AutPL) Check() *AutPL {
	perm := AutplToPerm(a.rep)
	if !eqIntByte(a.perm, perm) {
		panic("AutPL.Check: permutation mismatch")
	}
	if a.permNum != PermToM24num(perm) {
		panic("AutPL.Check: permutation number mismatch")
	}
	if a.permNum >= Mat24Order {
		panic("AutPL.Check: permutation number out of range")
	}
	if a.cocode != uint16(AutplToCocode(a.rep)) {
		panic("AutPL.Check: cocode mismatch")
	}
	return a
}

// eqIntByte reports whether the []int p equals the
// []byte q elementwise.
func eqIntByte(p []int, q []byte) bool {
	if len(p) != len(q) {
		return false
	}
	for i := range p {
		if p[i] != int(q[i]) {
			return false
		}
	}
	return true
}

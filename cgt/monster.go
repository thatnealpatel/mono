package cgt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"patel.codes/cgt/mat24"
	"patel.codes/cgt/n0"
	"patel.codes/cgt/reduce"
	"patel.codes/cgt/xsp2co1"
)

// MM is an element of the monster group,
// represented as a word of generator atoms.
//
// Each atom is a uint32 encoding a (sign, tag,
// value) triple in the standard mmgroup format:
//
//	bit 31     sign
//	bit 30..28 tag (1=d 2=p 3=x 4=y 5=t 6=l)
//	bit 27..0  value
type MM struct {
	data []uint32
}

//////////////////////////////////////////////////
// Atom-encoding helpers (mmgroup_generators.h)
//////////////////////////////////////////////////

const (
	atomTagAll = 0xf0000000
	atomData   = 0xfffffff
)

// Tag numbers shifted into bits 30..28.
const (
	tagD = 0x10000000 // 'd'
	tagP = 0x20000000 // 'p'
	tagX = 0x30000000 // 'x'
	tagY = 0x40000000 // 'y'
	tagT = 0x50000000 // 't'
	tagL = 0x60000000 // 'l'
)

// mmTagDict maps the standard tag letters to the
// shifted tag number for atom encoding.
var mmTagDict = map[byte]uint32{
	'd': tagD, 'p': tagP, 'x': tagX,
	'y': tagY, 't': tagT, 'l': tagL,
}

//////////////////////////////////////////////////
// Word algebra over the full monster (mm_group_word.c)
//////////////////////////////////////////////////

// splitWordN splits a prefix of word from the
// element g in N_0 such that word = word1 * g, with
// word1 a prefix. It returns the length of word1
// and does not modify word.
func splitWordN(word []uint32, g *n0.N0Elem) uint32 {
	length := uint32(len(word))
	status := uint32(0)
	*g = n0.N0Elem{}
	for length > 0 {
		atom := word[length-1]
		switch (atom >> 28) & 0xf {
		case 2:
			if status < 1 {
				g[4] = atom & 0xfffffff
			} else {
				return length
			}
			status = 1
			length--
		case 1:
			if status < 2 {
				g[3] = atom & 0xfff
			} else {
				return length
			}
			status = 2
			length--
		case 3:
			if status < 3 {
				g[2] = atom & 0x1fff
			} else {
				return length
			}
			status = 3
			length--
		case 4:
			if status < 4 {
				g[1] = atom & 0x1fff
			} else {
				return length
			}
			status = 4
			length--
		case 5:
			if status < 5 {
				g[0] = atom & 0xfffffff
			} else {
				return length
			}
			status = 5
			length--
		default:
			return length
		}
	}
	return 0
}

// mulWords computes w1 = w1 * w2^e, simplifying
// inside N_0, and returns the resulting length. The
// w1 slice must have capacity at least
// l1 + 2*abs(e)*len(w2). l1 is the meaningful prefix
// length of w1.
func mulWords(w1 []uint32, l1 uint32, w2 []uint32, e int) uint32 {
	l2 := len(w2)
	iStart, iStop, iStep := 0, l2, 1
	var sign uint32
	var gn n0.N0Elem
	l1 = splitWordN(w1[:l1], &gn)
	if e < 0 {
		iStart, iStop, iStep = l2-1, -1, -1
		sign = 0x80000000
		e = -e
	}
	for round := 0; round < e; round++ {
		for i := iStart; i != iStop; i += iStep {
			pending := n0.MulAtom(&gn, w2[i]^sign)
			if pending != 0 {
				n0.ReduceElement(&gn)
				l1 += n0.ToWord(&gn, w1[l1:])
				gn = n0.N0Elem{}
				if pending&0x70000000 == 0x60000000 && l1 != 0 &&
					w1[l1-1]&0x70000000 == 0x60000000 {
					exp := ((w1[l1-1] & 0xfffffff) + (pending & 3)) % 3
					if exp != 0 {
						w1[l1-1] = 0x60000000 + exp
					} else {
						l1--
					}
				} else {
					w1[l1] = pending
					l1++
				}
			}
		}
	}
	n0.ReduceElement(&gn)
	l1 += n0.ToWord(&gn, w1[l1:])
	return l1
}

// invertWord inverts the word w in place.
func invertWord(w []uint32) {
	for i := range w {
		w[i] ^= 0x80000000
	}
	for i, j := 0, len(w)-1; i < j; i, j = i+1, j-1 {
		w[i], w[j] = w[j], w[i]
	}
}

// checkWordN checks if the word w is in N_0. It
// returns:
//
//	0  w is the neutral element
//	1  w is in N_0 but not neutral
//	2  w is not in N_0
//	3  membership unknown
//
// On status 0 or 1 the N_0 element is written to
// gOut (length 5).
func checkWordN(w []uint32, gOut []uint32) uint32 {
	var g n0.N0Elem
	status := uint32(0)
	if len(w) > 0 {
		numXi := 0
		for _, a := range w {
			wv := a & 0x7fffffff
			if wv > 0x60000000 {
				numXi++
				if numXi > 1 || wv > 0x60000002 {
					return 3
				}
			}
		}
		if numXi != 0 {
			return 2
		}
		for _, a := range w {
			n0.MulAtom(&g, a)
		}
		if n0.ReduceElement(&g) != 0 {
			status = 1
		}
	}
	copy(gOut[:5], g[:])
	return status
}

// wordsEqu checks if words w1 and w2 are equal. It
// returns 0 if equal, 1 if unequal, and otherwise
// l3+2 where work holds a word w3 of length l3 with
// w1==w2 iff w3 is neutral. work must have length at
// least max(2*l1, l1+2*l2).
func wordsEqu(w1, w2, work []uint32) uint32 {
	l1, l2 := len(w1), len(w2)
	minlen := l1
	if l2 < minlen {
		minlen = l2
	}
	// Strip common prefix.
	p := 0
	for p < minlen && w1[p] == w2[p] {
		p++
	}
	w1 = w1[p:]
	w2 = w2[p:]
	l1, l2 = len(w1), len(w2)
	if l1 == 0 && l2 == 0 {
		return 0
	}
	// Strip common suffix.
	minlen = l1
	if l2 < minlen {
		minlen = l2
	}
	s := 0
	for s < minlen && w1[l1-1-s] == w2[l2-1-s] {
		s++
	}
	w1 = w1[:l1-s]
	w2 = w2[:l2-s]
	// work = reduced(w1); then work = reduced(w1 * w2^-1).
	l := mulWords(work, 0, w1, 1)
	l = mulWords(work, l, w2, -1)
	var gn [5]uint32
	status := checkWordN(work[:l], gn[:])
	if status < 3 {
		if status != 0 {
			return 1
		}
		return 0
	}
	return l + 2
}

//////////////////////////////////////////////////
// Parsing and construction
//////////////////////////////////////////////////

// stdQElements maps the special q-element names to
// their Leech-encoded value (construct_mm.py).
var stdQElements = map[string]uint32{
	"v+": 0x200, "v-": 0x1000200,
	"Omega": 0x800000, "-Omega": 0x1800000,
	"-": 0x1000000, "+": 0,
	"omega": 0x400, "-omega": 0x1000400,
}

// parseAtomValue parses an atom index value as a
// decimal number, a hex number (0x prefix), or a
// hex index with an "h"/"H" suffix.
func parseAtomValue(s string) (uint32, error) {
	if s == "" {
		return 0, errors.New("empty atom index")
	}
	if len(s) > 2 && (s[0:2] == "0x" || s[0:2] == "0X") {
		n, err := strconv.ParseUint(s[2:], 16, 64)
		return uint32(n), err
	}
	last := s[len(s)-1]
	if last == 'h' || last == 'H' {
		n, err := strconv.ParseUint(s[:len(s)-1], 16, 64)
		return uint32(n), err
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return uint32(n), err
}

// iterAtom appends the atom encoding of generator
// (tag, value) to out and returns it. It mirrors
// the standard-tag cases of construct_mm.py.
func iterAtom(out []uint32, tag byte, value string) ([]uint32, error) {
	switch tag {
	case 'd':
		v, err := atomIndex(value, 0xfff)
		if err != nil {
			return nil, err
		}
		return append(out, tagD+(v&0xfff)), nil
	case 'p':
		v, err := atomIndex(value, mat24.Mat24Order-1)
		if err != nil {
			return nil, err
		}
		if v >= mat24.Mat24Order {
			return nil, fmt.Errorf("tag p: bad permutation number %d", v)
		}
		return append(out, tagP+v), nil
	case 'x', 'y', 'z':
		v, err := ploopElement(tag, value)
		if err != nil {
			return nil, err
		}
		if tag == 'z' {
			pl := mat24.PowPloop(v, 3)
			out = append(out, tagX+pl)
			out = append(out, tagY+pl)
			return out, nil
		}
		return append(out, mmTagDict[tag]+v), nil
	case 't', 'l':
		v, err := atomIndex(value, 1<<28-1)
		if err != nil {
			return nil, err
		}
		e := v % 3
		if e != 0 {
			out = append(out, mmTagDict[tag]+e)
		}
		return out, nil
	case 'q':
		v, err := qElement(value)
		if err != nil {
			return nil, err
		}
		d := (v >> 12) & 0x1fff
		delta := (v ^ mat24.PloopTheta(d)) & 0xfff
		out = append(out, tagX+d)
		out = append(out, tagD+delta)
		return out, nil
	default:
		return nil, fmt.Errorf("illegal tag %q", string(tag))
	}
}

// atomIndex parses value as an integer index and
// fails if it exceeds max.
func atomIndex(value string, max uint32) (uint32, error) {
	v, err := parseAtomValue(value)
	if err != nil {
		return 0, err
	}
	if v > max {
		return 0, fmt.Errorf("atom index %d out of range", v)
	}
	return v, nil
}

// ploopElement parses the index of an x/y/z atom,
// accepting integers and the special q-names.
func ploopElement(tag byte, value string) (uint32, error) {
	if v, ok := stdQElements[value]; ok {
		if v&0xfff == 0 {
			return v >> 12, nil
		}
		return 0, fmt.Errorf("illegal value %q for tag %q", value, string(tag))
	}
	v, err := parseAtomValue(value)
	if err != nil {
		return 0, err
	}
	return v & 0x1fff, nil
}

// qElement parses the index of a q atom.
func qElement(value string) (uint32, error) {
	if v, ok := stdQElements[value]; ok {
		return v, nil
	}
	v, err := parseAtomValue(value)
	if err != nil {
		return 0, err
	}
	return v & 0x1ffffff, nil
}

// parseMMWord parses a word string such as
// "M<x_1h*y_2h>" or "x_1h*y_2h" into an atom array.
func parseMMWord(word string) ([]uint32, error) {
	s := strings.TrimSpace(word)
	// Strip an optional "name<...>" frame.
	if i := strings.IndexByte(s, '<'); i >= 0 && strings.HasSuffix(s, ">") {
		s = s[i+1 : len(s)-1]
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "1" {
		return nil, nil
	}
	var out []uint32
	for _, factor := range strings.Split(s, "*") {
		factor = strings.TrimSpace(factor)
		if factor == "" || factor == "1" {
			continue
		}
		tag := factor[0]
		rest := factor[1:]
		if len(rest) == 0 || rest[0] != '_' {
			return nil, fmt.Errorf("malformed atom %q", factor)
		}
		var err error
		out, err = iterAtom(out, tag, rest[1:])
		if err != nil {
			return nil, fmt.Errorf("atom %q: %w", factor, err)
		}
	}
	return out, nil
}

// NewMM parses a word string into a monster element.
func NewMM(word string) (*MM, error) {
	data, err := parseMMWord(word)
	if err != nil {
		return nil, err
	}
	return &MM{data: data}, nil
}

// MMIdentity returns the neutral element.
func MMIdentity() *MM {
	return &MM{}
}

// MMGen returns the generator atom with the given
// tag and index. It panics on an illegal tag.
func MMGen(tag string, i int) *MM {
	if len(tag) != 1 {
		panic("MMGen: tag must be a single letter")
	}
	out, err := iterAtom(nil, tag[0], strconv.Itoa(i))
	if err != nil {
		panic("MMGen: " + err.Error())
	}
	return &MM{data: out}
}

// Subgroup names accepted by MMRandIn.
type Subgroup string

const (
	SubM   Subgroup = "M"
	SubGx0 Subgroup = "G_x0"
	SubN0  Subgroup = "N_0"
	SubNx0 Subgroup = "N_x0"
	SubB   Subgroup = "B"
	SubQx0 Subgroup = "Q_x0"
)

// MMRand returns a random monster element of the
// form g0 t1 g1 ... ti gi with gj random in G_x0
// and tj equal to tau^{+-1}. The implementation is
// in monster_random.go.
func MMRand(rounds int) *MM {
	return mmRand(rounds)
}

// MMRandIn returns a random element of the named
// subgroup of the monster. The implementation is
// in monster_random.go.
//
// MMRandIn panics if sub is not a known subgroup
// description.
func MMRandIn(sub Subgroup) *MM {
	return mmRandIn(sub)
}

// MMFromInt reconstructs a monster element from the
// 255-bit integer produced by AsInt.
func MMFromInt(n uint64) *MM {
	g, err := reduce.ExpandInt(n)
	if err != nil {
		panic("MMFromInt: " + err.Error())
	}
	return &MM{data: g}
}

//////////////////////////////////////////////////
// String / data conversion (construct_mm.py)
//////////////////////////////////////////////////

// ihex formats x in the mmgroup hex-index style,
// e.g. 0x200 -> "200h", padding a leading zero when
// the first character is not a digit.
func ihex(x uint32) string {
	s := strconv.FormatUint(uint64(x), 16) + "h"
	if s[0] < '0' || s[0] > '9' {
		s = "0" + s
	}
	return s
}

// emitNx0Strings appends the string form of the
// reduced N_x0 element to dst and clears it.
func emitNx0Strings(dst []string, element []uint32) []string {
	n0.ReduceElement((*n0.N0Elem)(element))
	if element[1] != 0 {
		dst = append(dst, "y_"+ihex(element[1]))
	}
	if element[2] != 0 {
		dst = append(dst, "x_"+ihex(element[2]))
	}
	if element[3] != 0 {
		dst = append(dst, "d_"+ihex(element[3]))
	}
	if element[4] != 0 {
		dst = append(dst, "p_"+strconv.FormatUint(uint64(element[4]), 10))
	}
	*(*n0.N0Elem)(element) = n0.N0Elem{}
	return dst
}

// stringsFromAtoms renders an atom word as the list
// of generator strings (iter_strings_from_atoms).
func stringsFromAtoms(atoms []uint32) []string {
	var nx0 [5]uint32
	var out []string
	for _, a := range atoms {
		t := a & 0x7fffffff
		switch {
		case t < 0x50000000:
			n0.MulAtom((*n0.N0Elem)(nx0[:]), a)
		case a&0xfffffff == 0:
			// neutral t/l atom
		case t < 0x70000000:
			tag := t >> 28
			v := a & 0xfffffff
			if a&0x80000000 != 0 {
				v = (3 - v%3) % 3
			} else {
				v %= 3
			}
			if v != 0 {
				out = emitNx0Strings(out, nx0[:])
				name := "tl"[tag-5]
				out = append(out, fmt.Sprintf("%c_%d", name, v))
			}
		default:
			out = append(out, fmt.Sprintf("<Bad atom %#x>", a))
		}
	}
	return emitNx0Strings(out, nx0[:])
}

// String returns the reduced word as a string of
// generators joined by '*', or "1" for the neutral
// element.
func (g *MM) String() string {
	g.Reduce()
	parts := stringsFromAtoms(g.data)
	if len(parts) == 0 {
		return "1"
	}
	return strings.Join(parts, "*")
}

// Mmdata returns the internal atom representation of
// the (reduced) element.
func (g *MM) Mmdata() []uint32 {
	g.Reduce()
	out := make([]uint32, len(g.data))
	copy(out, g.data)
	return out
}

//////////////////////////////////////////////////
// Group operations
//////////////////////////////////////////////////

// Mul returns the product g * h. The concatenated
// word is reduced in the N_0 sense, mirroring
// MM0Group._mul for unreduced operands.
func (g *MM) Mul(h *MM) *MM {
	cat := make([]uint32, 0, len(g.data)+len(h.data))
	cat = append(cat, g.data...)
	cat = append(cat, h.data...)
	return (&MM{data: cat}).Reduce()
}

// Inv returns the inverse g^-1.
func (g *MM) Inv() *MM {
	w := make([]uint32, len(g.data))
	copy(w, g.data)
	invertWord(w)
	return (&MM{data: w}).Reduce()
}

// Pow returns g raised to the integer power e.
func (g *MM) Pow(e int) *MM {
	if e == 0 {
		return MMIdentity()
	}
	l := len(g.data)
	buf := make([]uint32, 2*absInt(e)*l+1)
	length := mulWords(buf, 0, g.data, e)
	return &MM{data: append([]uint32(nil), buf[:length]...)}
}

func absInt(e int) int {
	if e < 0 {
		return -e
	}
	return e
}

// reductionStrategy analyzes the monster word a and
// classifies which subgroup it is known to lie in:
//
//	1  a is in N_0
//	2  a is in G_x0
//	3  a is not known to be in either
//	4  a contains an opaque atom (tag 7)
//
// It mirrors C reduction_strategy: tau (tag 5) and xi
// (tag 6) atoms with exponent 0 mod 3 are ignored;
// otherwise their tag bit is accumulated. The element
// is in N_0 if it has no xi, in G_x0 if it has no tau.
func reductionStrategy(a []uint32) int {
	var accAtoms uint32
	for _, atom := range a {
		tag := (atom >> 28) & 7
		switch tag {
		case 0:
			continue
		case 5, 6:
			if (atom&0xfffffff)%3 == 0 {
				continue
			}
		case 7:
			return 4
		}
		accAtoms |= 1 << tag
	}
	if accAtoms&0x40 == 0 {
		return 1 // no xi: in N_x0
	}
	if accAtoms&0x20 == 0 {
		return 2 // no tau: in G_x0
	}
	return 3
}

// prereduce attempts to reduce the monster word a using
// simple subgroup-specific rules before the full
// axis-tracking reducer runs. It returns the (partially)
// reduced word and a status:
//
//	0  out is a fully reduced word (use it directly)
//	1  out is a partially reduced word (reduce it again)
//	2  no reduction was done (out is nil; reduce a)
//
// It mirrors C prereduce. Strategy 1 collapses an N_0
// element to standard form via n0.ToWordStd; strategy 2
// reduces a G_x0 word via reduce.ReduceWord; the
// general strategy runs the GtWord shortening engine.
func prereduce(a []uint32) ([]uint32, int) {
	switch reductionStrategy(a) {
	case 1:
		var gn n0.N0Elem
		if n0.MulWordScan(&gn, a) != uint32(len(a)) {
			return nil, 2
		}
		var buf [5]uint32
		k := n0.ToWordStd(&gn, buf[:])
		return append([]uint32(nil), buf[:k]...), 0
	case 2:
		out, n := reduce.ReduceWord(a)
		if n < 0 {
			return nil, 2
		}
		return out, 0
	default:
		gw := reduce.NewGtWord(1)
		if gw.AppendWord(a) < 0 {
			return nil, 2
		}
		status := gw.Reduce()
		if status < 0 {
			return nil, 2
		}
		out := make([]uint32, gtPrereduceBuf)
		n := gw.GtWordStore(out, gtPrereduceBuf)
		if n < 0 {
			return nil, 2
		}
		if status >= 4 {
			return append([]uint32(nil), out[:n]...), 0
		}
		return append([]uint32(nil), out[:n]...), 1
	}
}

// gtPrereduceBuf is the capacity of the prereduce output
// buffer. C macro A_BUFSIZE.
const gtPrereduceBuf = 0x1000

// reduceM is the workhorse of the canonical reducer. It
// reduces the monster word a (of length n) to a word of
// fixed maximum length using the Seysen22 method, and
// returns that word, or nil on a fatal internal error.
//
// The reduction tracks the images of the 2A axes v^+
// and v^-, and of the precomputed order vector v_1,
// under the unknown element g represented by a, then
// recovers g from those images. This is the mod-15 v1
// branch of C reduce_M (USE_ORDER_VECTOR_MOD15 = 1),
// which is output-identical to mmgroup's default path
// at the same mode. The returned word may contain
// tag-0 comment atoms acting as the neutral element.
//
// reduceM panics if a representation operation reports
// an error (an unexpected internal failure).
func reduceM(a []uint32) []uint32 {
	n := len(a)
	v := ZeroVector(15)
	work := ZeroVector(15)
	r := make([]uint32, 256)

	vp := uint32(vPlus)
	if res := mmReduceMapAxis(&vp, v.data, a, n, work.data); res < 0 {
		return nil
	}
	if res := mmReduceVectorVP(vp, v.data, 0, r, work.data); res < 0 {
		return nil
	}

	vm := uint32(vMinus)
	if res := mmReduceMapAxis(&vm, v.data, a, n, work.data); res < 0 {
		return nil
	}
	if res := mmReduceVectorVm(&vm, v.data, r, work.data); res < 0 {
		return nil
	}

	v = loadOrderVector()
	if err := OpWord(15, v.data, a, n, 1, work.data); err != nil {
		panic("cgt: reduceM OpWord: " + err.Error())
	}
	res := mmReduceVectorV1(v.data, r, work.data)
	if res < 0 {
		return nil
	}
	return append([]uint32(nil), r[:res]...)
}

// stripCommentAtoms returns a copy of w with all
// tag-0 comment atoms removed. A comment atom acts as
// the neutral element and carries no group data; the
// mod-15 axis reducer (reduceM) emits them as padding.
// They are identified by a zero low-3-bit tag in bits
// 28-30, i.e. (atom >> 28) & 0x7 == 0, which matches
// both atomTag1 (0x0) and atomTagI1 (0x8). The result
// is always a freshly allocated slice (never aliasing
// w), so callers may store it without copying.
func stripCommentAtoms(w []uint32) []uint32 {
	out := make([]uint32, 0, len(w))
	for _, atom := range w {
		if (atom>>28)&0x7 == 0 {
			continue
		}
		out = append(out, atom)
	}
	return out
}

// reduceRaw computes the canonical reduced word for the
// atom word a, before comment-atom stripping. It ports
// C mm_reduce_M (mode 0): first prereduce shortens
// elements known to lie in N_0 or G_x0 to a clean word;
// otherwise the full Seysen22 axis-tracking reducer
// (reduceM) runs. If the reducer hits an unexpected
// internal failure it falls back to the N_0-prefix
// reduction (a port of mm_group_mul_words).
//
// The returned word is byte-identical to mmgroup's
// default mm_reduce_M output (see TestMonsterMmdata's
// internal tripwire) and may therefore contain tag-0
// comment atoms when the mod-15 reducer path runs;
// Reduce strips those before they reach g.data.
func reduceRaw(a []uint32) []uint32 {
	if out, status := prereduce(a); status != 2 {
		if status == 0 {
			return out
		}
		a = out // partial: reduce again below
	}
	if r := reduceM(a); r != nil {
		return r
	}
	buf := make([]uint32, 2*len(a)+1)
	length := mulWords(buf, 0, a, 1)
	return append([]uint32(nil), buf[:length]...)
}

// Reduce reduces the element in place to a canonical
// word of fixed maximum length and returns g. The
// canonical form depends only on the value of g in the
// monster group, not on its representation as a word.
//
// The reduced word is produced by reduceRaw (a port of
// C mm_reduce_M mode 0) and then has its tag-0 comment
// atoms stripped, so g.data, Mmdata, and IsReduced all
// see a clean word. The comment atoms are neutral
// padding emitted by the mod-15 axis reducer and carry
// no group data, so removing them does not change the
// element.
func (g *MM) Reduce() *MM {
	if len(g.data) == 0 {
		return g
	}
	g.data = stripCommentAtoms(reduceRaw(g.data))
	return g
}

// Equal reduces both elements and compares the
// resulting words. When the word comparison is
// inconclusive it falls back to comparing the images
// of the order vector under the difference word.
func (g *MM) Equal(h *MM) bool {
	g.Reduce()
	h.Reduce()
	worklen := 2 * len(g.data)
	if alt := len(g.data) + 2*len(h.data); alt > worklen {
		worklen = alt
	}
	work := make([]uint32, worklen+1)
	status := wordsEqu(g.data, h.data, work)
	if status < 2 {
		return status == 0
	}
	// Inconclusive: compare order-vector images of the
	// residual word w3 = work[:status-2].
	return mmCompareViaOrderVector(work[:status-2])
}

// IsReduced reports whether the element is in
// reduced form, i.e. equals its own reduction.
func (g *MM) IsReduced() bool {
	r := (&MM{data: append([]uint32(nil), g.data...)}).Reduce()
	if len(r.data) != len(g.data) {
		return false
	}
	for i := range g.data {
		if g.data[i] != r.data[i] {
			return false
		}
	}
	return true
}

//////////////////////////////////////////////////
// Order and powering (mm_order.py)
//////////////////////////////////////////////////

// Order returns the order of the element.
func (g *MM) Order() int {
	o, _ := g.HalfOrder()
	return o
}

// HalfOrder returns the order o of the element and,
// for even o, the element raised to the o/2 power;
// otherwise it returns (o, nil). When the square
// root lies in G_x0 it is returned as a word in the
// generators of G_x0.
func (g *MM) HalfOrder() (int, *MM) {
	g.Reduce()
	o1, h := g.orderElementGx0(119)
	if o1 == 0 {
		return 0, nil
	}
	elem := xsp2co1.NewXsp2Co1(atomsFromWord(h)...)
	o2, h2 := elem.HalfOrder()
	o := o1 * o2
	if o2&1 == 0 {
		return o, &MM{data: h2.Mmdata()}
	}
	if o&1 != 0 {
		return o, nil
	}
	if o2 == 1 {
		return o, g.Pow(o1 >> 1)
	}
	q, r := (o>>1)/o1, (o>>1)%o1
	w := &MM{data: append([]uint32(nil), h...)}
	return o, w.Pow(q).Mul(g.Pow(r))
}

// orderElementGx0 returns the smallest exponent e
// such that g^e lies in G_x0, together with g^e as a
// word of generators of G_x0. It returns (0, nil) if
// e exceeds o. C mm_order_element_Gx0.
//
// G_x0 membership of g^e is not a syntactic property
// of the reduced word, so apart from the e=1 shortcut
// for a word that is already a product of G_x0
// generators, each power g^e is tested by applying it
// to the precomputed order vector v_1 and recovering
// the (would-be) G_x0 element from the image, via
// orderCheckInGx0.
func (g *MM) orderElementGx0(o int) (int, []uint32) {
	if o < 1 {
		o = 1
	}
	if o > 119 {
		o = 119
	}
	switch checkWordGx0(g.data) {
	case 0:
		// g is already a word of G_x0 generators.
		elem := xsp2co1.NewXsp2Co1(atomsFromWord(g.data)...)
		return 1, elem.Mmdata()
	case 1:
		// g is definitely not in G_x0. If we only need to
		// test e = 1 we can stop now.
		if o == 1 {
			return 0, nil
		}
	}

	// w tracks v_1 . g^i across iterations, as the C loop
	// applies mm_op15_word(w, g, ...) repeatedly.
	w := loadOrderVector()
	work := make([]uint64, MMVSize(15))
	for i := 1; i <= o; i++ {
		if err := OpWord(15, w.data, g.data, len(g.data), 1, work); err != nil {
			panic("cgt: orderElementGx0 OpWord: " + err.Error())
		}
		if h := orderCheckInGx0(w.data); h != nil {
			return i, h
		}
	}
	return 0, nil
}

//////////////////////////////////////////////////
// Subgroup membership (mm0_group.py, mm_order.py)
//////////////////////////////////////////////////

// InGx0 reports whether the element is in the
// subgroup G_x0.
func (g *MM) InGx0() bool {
	g.Reduce()
	return g.checkInGx0() != nil
}

// checkInGx0 returns a word in the generators of
// G_x0 equal to g if g is in G_x0, else nil.
func (g *MM) checkInGx0() []uint32 {
	e, h := g.orderElementGx0(1)
	if e != 1 {
		return nil
	}
	return h
}

// InNx0 reports whether the element is in the
// subgroup N_x0.
func (g *MM) InNx0() bool {
	if g.checkInGx0() == nil {
		return false
	}
	g.Reduce()
	for _, atom := range g.data {
		if (atom>>28)&7 > 4 {
			return false
		}
	}
	return true
}

// InQx0 reports whether the element is in the
// subgroup Q_x0.
func (g *MM) InQx0() bool {
	if g.checkInGx0() == nil {
		return false
	}
	g.Reduce()
	for _, atom := range g.data {
		switch atom & 0x70000000 {
		case 0x10000000, 0x30000000:
		default:
			return false
		}
	}
	return true
}

// AsInt maps the element to a 255-bit integer
// identifier. The element is reduced first.
func (g *MM) AsInt() uint64 {
	g.Reduce()
	return reduce.CompressAsInt(g.data)
}

//////////////////////////////////////////////////
// Characters (mm0_group.py)
//////////////////////////////////////////////////

// ChiGx0 computes the character tuple
// (chi_M, chi_299, chi_24, chi_4096) of the element,
// which must lie in G_x0. It panics otherwise.
func (g *MM) ChiGx0() [4]int {
	h := g.checkInGx0()
	if h == nil {
		panic("ChiGx0: element is not in the subgroup G_x0")
	}
	elem := xsp2co1.NewXsp2Co1(atomsFromWord(h)...)
	return elem.ChiGx0()
}

// ChiMap maps divisors e of the order to chi_M(g^e).
// Absent keys were not computed; use the ok idiom.
type ChiMap map[int]int

// ChiPowers returns the order, a map of chi_M(g^e)
// over divisors e (up to maxE), and an element h
// such that computing characters of h^-1 g h is
// easier. It mirrors MM.chi_powers.
func (g *MM) ChiPowers(maxE, ntrials int) (int, ChiMap, *MM) {
	o, sqrt1 := g.HalfOrder()
	// Collect the divisors e of o (and their
	// complements) for which a character may be
	// computed; the returned map omits uncomputed ones.
	divisors := map[int]bool{}
	for i := 1; i < 11; i++ {
		if o%i == 0 {
			divisors[i] = true
			divisors[o/i] = true
		}
	}
	if maxE < 1 {
		maxE = 60
	}
	chi := ChiMap{}
	h := MMIdentity()
	cur := g // we maintain cur = h^-1 g h
	iclass := 0
	if sqrt1 != nil {
		iclass, h = ConjugateInvolutionType(sqrt1)
		cur = h.Inv().Mul(g).Mul(h)
	}
	if iclass == 2 {
		chi[o>>1] = 275
	} else if iclass == 1 {
		chi[o>>1] = 4371
	}
	if cur.InGx0() {
		x := xsp2co1.NewXsp2Co1(atomsFromWord(cur.checkInGx0())...)
		for e := range divisors {
			if _, done := chi[e]; !done && e <= maxE {
				chi[e] = x.Pow(e).ChiGx0()[0]
			}
		}
	}
	return o, chi, h
}

// Simplify tries to shorten the word with the given
// number of trials, mutating g in place and returning
// it. It first reduces g; if the reduced word is
// already short (at most nine triality atoms) it
// stops there. Otherwise it runs the power-conjugation
// shortener of reduce_via_power. If the shortener
// fails to find a shorter equal word, g keeps its
// reduced value. It mirrors MM.simplify.
//
// A non-positive ntrials selects the default of 40
// trials.
func (g *MM) Simplify(ntrials int) *MM {
	g.Reduce()
	// Count triality (tag t) atoms; the word is already
	// short when there are at most nine of them.
	weight := 0
	for _, a := range g.data {
		if (a>>28)&7 == 5 {
			weight++
		}
	}
	if weight <= 9 {
		return g
	}
	if ntrials <= 0 {
		ntrials = 40
	}
	if h, ok := reduceViaPower(g, ntrials); ok && h.Equal(g) {
		g.data = h.Reduce().data
	}
	return g
}

// reduceViaPower shortens the monster element g using
// Wilson's power-conjugation method: it searches for a
// random element x0 such that g*x0 has even order with
// a square root that is a 2B involution, conjugates
// that involution to the central involution, reduces
// the conjugated element inside G_x0, and unconjugates.
// It returns the shortened element (equal to g) and
// ok=true on success, or (nil, false) if no shortening
// is found within ntrials trials. It mirrors
// reduce_via_power.
func reduceViaPower(g *MM, ntrials int) (*MM, bool) {
	for i := 0; i < ntrials; i++ {
		rounds := 1
		if r := i >> 2; r > rounds {
			rounds = r
		}
		x0 := MMRand(rounds)
		g1 := g.Mul(x0)
		o, g2 := g1.HalfOrder()
		if o&1 != 0 || g2 == nil {
			continue
		}
		it, h, ok := tryConjugateInvolution(g2)
		if !ok || it != 2 {
			continue
		}
		// g2^h = z, so g1^h = h^-1 g1 h lies in G_x0.
		hInv := h.Inv()
		g1h := hInv.Mul(g1).Mul(h)
		short := g1h.checkInGx0()
		if short == nil {
			continue
		}
		// Replace g1^h by its short G_x0 word, then
		// unconjugate and strip x0. This shortening of the
		// G_x0 part is the source of the word-length
		// reduction.
		g1New := h.Mul(&MM{data: short}).Mul(hInv)
		gNew := g1New.Mul(x0.Inv())
		gNew.Reduce()
		return gNew, true
	}
	return nil, false
}

// tryConjugateInvolution conjugates the 2B involution
// g2 to the central involution with a single trial,
// reporting failure via ok=false instead of panicking.
// It mirrors the try/except (ValueError, AssertionError)
// guarding the mm_conjugate_involution call inside
// reduce_via_power: when g2 is not actually an involution
// (an expected per-trial miss), the G_x0 fast path raises
// the "not an involution" panic, which this recovers as
// ok=false. Any other panic is a genuine bug and is
// re-raised.
func tryConjugateInvolution(g2 *MM) (it int, h *MM, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if s, isStr := r.(string); isStr && s == xsp2co1.ErrNotInvolution {
				it, h, ok = 0, nil, false
				return
			}
			panic(r)
		}
	}()
	return conjugateInvolution(g2, false, 1)
}

//////////////////////////////////////////////////
// Delegations to the representation layer
//////////////////////////////////////////////////

// atomsFromWord converts an atom word to a slice of
// XspAtom values keyed by tag letter.
func atomsFromWord(w []uint32) []xsp2co1.XspAtom {
	tagLetters := map[uint32]string{
		0x1: "d", 0x2: "p", 0x3: "x",
		0x4: "y", 0x5: "t", 0x6: "l",
	}
	out := make([]xsp2co1.XspAtom, 0, len(w))
	for _, a := range w {
		tag := (a >> 28) & 7
		letter, ok := tagLetters[tag]
		if !ok {
			continue
		}
		sign := ""
		if a&0x80000000 != 0 {
			sign = "-"
		}
		out = append(out, xsp2co1.XspAtom{Tag: sign + letter, I: int(a & 0xfffffff)})
	}
	return out
}

// checkWordGx0 classifies the word w of monster
// generators with respect to the subgroup G_x0:
//
//	0  w is in G_x0
//	1  w is not in G_x0
//	2  nothing is known about w
//
// A word built only from generators of G_x0 (tags d,
// p, x, y, l) is in G_x0. A single tau power (tag t)
// with a nonzero exponent mod 3 takes it out of G_x0;
// more than one such tau power, or any tag-7 atom,
// leaves membership undecided. C
// xsp2co1_check_word_g_x0.
func checkWordGx0(w []uint32) int {
	numT := 0
	for _, a := range w {
		switch (a >> 28) & 7 {
		case 7:
			return 2
		case 5:
			if (a&0xfffffff)%3 != 0 {
				numT++
			}
		}
	}
	if numT > 1 {
		return 2
	}
	return numT
}

// mmCompareViaOrderVector reports whether the word w
// fixes the order vector, i.e. represents the
// neutral element. It applies w to the order vector
// in the representation mod 15 and compares with the
// order vector.
func mmCompareViaOrderVector(w []uint32) bool {
	v := orderVector15()
	work := ZeroVector(15)
	if err := OpWord(15, v.data, w, len(w), 1, work.data); err != nil {
		panic("mmCompareViaOrderVector: " + err.Error())
	}
	return v.Equal(orderVector15())
}

// orderVector15 returns the precomputed order
// vector in the representation mod 15.
func orderVector15() *MMVector {
	return loadOrderVector()
}

//////////////////////////////////////////////////
// Reduce-engine boundary
//
// The following helpers represent the boundary with
// the order-vector / word-compression engine, which
// is provided by separate modules
// (mm_order_vector.c, mm_compress). They are wired
// up once those modules are ported.
//////////////////////////////////////////////////

// loadOrderVector returns the precomputed order
// vector v_1 of the representation mod 15. It is
// implemented in monster_order.go.

// reduce.CompressAsInt (GtWord.as_int) and
// reduce.ExpandInt (mm_compress_pc_expand_int) are
// implemented in package reduce (reduce/compress.go).

//////////////////////////////////////////////////
// 2A axes (mmgroup.tests.axes.axis)
//////////////////////////////////////////////////

// axisV15 is the standard 2A axis v^+ of the
// involution x_beta, beta = mat24.Cocode([2,3]), in the
// representation mod 15.
const axisV15 = "A_2_2 - A_3_2 + A_3_3 - 2*B_3_2"

// Axis models a 2A axis of the monster as the image
// of the standard axis v^+ under a group element g.
type Axis struct {
	g *MM
	v *MMVector
}

// AxisFor returns the 2A axis v^+ . g.
func AxisFor(g *MM) *Axis {
	base, err := ParseVector(15, axisV15)
	if err != nil {
		panic("AxisFor: " + err.Error())
	}
	return &Axis{
		g: g,
		v: base.Mul(g.Mmdata()),
	}
}

// Type returns the G_x0 orbit name of the axis,
// e.g. "2A", "2B", "4A".
func (a *Axis) Type() string {
	return a.v.AxisType(0)
}

// Vector returns the axis vector in the
// representation mod 15.
func (a *Axis) Vector() *MMVector {
	return a.v
}

// Mul returns the axis v^+ . (g0 * g), i.e. the
// image of this axis under g.
func (a *Axis) Mul(g *MM) *Axis {
	return &Axis{
		g: a.g.Mul(g),
		v: a.v.Mul(g.Mmdata()),
	}
}

// ReduceGx0 returns a G_x0 element mapping the
// standard axis v^+ to this axis. Python reduce_G_x0
// returns a G_x0 word.
func (a *Axis) ReduceGx0() *MM {
	g := rebaseAxis(a.v)
	if g == nil {
		panic("ReduceGx0: rebasing of axis failed")
	}
	return g
}

// Equal reports whether two axes have the same axis
// vector.
func (a *Axis) Equal(b *Axis) bool {
	return a.v.Equal(b.v)
}

// rebaseAxis returns a G_x0 element g0 with
// v^+ . g0 == v15, or nil on failure. It is
// implemented in monster_order.go.

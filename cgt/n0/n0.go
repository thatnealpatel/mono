// Package n0 implements the word algebra of the
// monster subgroup N_0 (the C file mm_group_n.c).
//
// An element of N_0 is held as a [5]uint32 with index
// layout t, y, x, d, pi (the iT..iPi constants). The
// package depends only on mat24 (and stdlib), so it
// sits low in the cgt DAG and is shared by the mm and
// xsp2co1 layers.
package n0

import "patel.codes/cgt/mat24"

// N0Elem is an element of the subgroup N_0, held as
// the five components t, y, x, d, pi (indexed by
// iT..iPi). The neutral element is the zero value.
type N0Elem [5]uint32

const (
	iT  = 0 // tau exponent
	iY  = 1 // y_e Parker loop elt
	iX  = 2 // x_e Parker loop elt
	iD  = 3 // x_delta cocode elt
	iPi = 4 // pi M24 number
)

// theta1000 returns bit 0x1000 of the Parker-loop
// theta of v (low 11 bits index the table). This
// matches the C idiom MAT24_THETA_TABLE[v&0x7ff] &
// 0x1000.
func theta1000(v uint32) uint32 {
	return uint32(mat24.ThetaTable(v&0x7ff)) & 0x1000
}

// nPlInvAutpl returns f with
// f = (x_delta x_pi)^-1 e (x_delta x_pi) for e in
// PL, delta in C*, pi in mat24.AutPL. Mirrors C
// mm_group_op_pl_inv_autpl.
func nPlInvAutpl(e, delta, pi uint32) uint32 {
	e &= 0x1fff
	if pi == 0 || pi >= mat24.Mat24Order {
		return e ^ (mat24.ScalarProd(e, delta) << 12)
	}
	perm := mat24.M24numToPerm(pi)
	_, invAutpl := mat24.PermToIautpl(delta, perm)
	return mat24.OpPloopAutpl(e, invAutpl)
}

// MulDeltaPi puts g = g * x_delta x_pi.
func MulDeltaPi(g *N0Elem, delta, pi uint32) {
	if pi >= mat24.Mat24Order {
		pi = 0
	}
	delta &= 0xfff
	switch {
	case g[iPi] == 0:
		g[iPi] = pi
		g[iD] ^= delta
	case pi == 0:
		perm := mat24.M24numToPerm(g[iPi])
		invPerm := mat24.InvPerm(perm)
		delta = mat24.OpCocodePerm(delta, invPerm)
		g[iD] ^= delta
	default:
		perm1 := mat24.M24numToPerm(g[iPi])
		aut1 := mat24.PermToAutpl(g[iD], perm1)
		perm2 := mat24.M24numToPerm(pi)
		aut2 := mat24.PermToAutpl(delta, perm2)
		aut3 := mat24.MulAutpl(aut1, aut2)
		g[iD] = mat24.AutplToCocode(aut3)
		perm1 = mat24.AutplToPerm(aut3)
		g[iPi] = mat24.PermToM24num(perm1)
	}
}

// MulInvDeltaPi puts g = g * (x_delta x_pi)^-1.
func MulInvDeltaPi(g *N0Elem, delta, pi uint32) {
	if pi >= mat24.Mat24Order {
		pi = 0
	}
	delta &= 0xfff
	if pi == 0 {
		if g[iPi] != 0 {
			perm := mat24.M24numToPerm(g[iPi])
			invPerm := mat24.InvPerm(perm)
			delta = mat24.OpCocodePerm(delta, invPerm)
		}
		g[iD] ^= delta
		return
	}
	perm2 := mat24.M24numToPerm(pi)
	invPerm2, aut2 := mat24.PermToIautpl(delta, perm2)
	_ = invPerm2
	var perm1 []byte
	if g[iPi] == 0 {
		g[iD] ^= mat24.AutplToCocode(aut2)
		perm1 = mat24.AutplToPerm(aut2)
	} else {
		perm1 = mat24.M24numToPerm(g[iPi])
		aut1 := mat24.PermToAutpl(g[iD], perm1)
		aut3 := mat24.MulAutpl(aut1, aut2)
		g[iD] = mat24.AutplToCocode(aut3)
		perm1 = mat24.AutplToPerm(aut3)
	}
	g[iPi] = mat24.PermToM24num(perm1)
}

// MulX puts g = g * x_e.
func MulX(g *N0Elem, e uint32) {
	e = nPlInvAutpl(e, g[iD], g[iPi])
	g[iD] ^= mat24.PloopCap(g[iX], e)
	g[iX] ^= e ^ (mat24.PloopCocycle(g[iX], e) << 12)
}

// MulY puts g = g * y_f.
func MulY(g *N0Elem, f uint32) {
	f = nPlInvAutpl(f, g[iD], g[iPi])
	signC := mat24.PloopComm(g[iX], f)
	signY := mat24.PloopCocycle(g[iY], f) ^ signC
	signX := mat24.PloopAssoc(g[iX], g[iY], f) ^ signC
	if g[iD]&0x800 != 0 { // g1 is odd
		signX ^= mat24.PloopCocycle(g[iX], f)
		f ^= theta1000(f)
		g[iD] ^= mat24.PloopCap(g[iY], f)
		g[iX] ^= f ^ (signX << 12)
	} else { // g1 is even
		g[iD] ^= mat24.PloopCap(g[iX]^g[iY], f)
		g[iX] ^= signX << 12
	}
	g[iY] ^= f ^ (signY << 12)
}

// MulT puts g = g * tau^t.
func MulT(g *N0Elem, t uint32) {
	t %= 3
	if t == 0 {
		return
	}
	t = (t ^ (g[iD] >> 11)) & 1
	var a1, b1 uint32
	if t != 0 { // (-1)^parity(g1) * t = 1 (mod 3)
		a1 = g[iY]
		a1 ^= theta1000(a1)
		b1 = g[iX] ^ a1
		a1 ^= mat24.PloopComm(g[iX], g[iY]) << 12
		b1 ^= mat24.PloopCocycle(g[iX], g[iY]) << 12
	} else { // (-1)^parity(g1) * t = 2 (mod 3)
		b1 = g[iX]
		b1 ^= theta1000(b1)
		a1 = g[iY] ^ b1
		b1 ^= mat24.PloopComm(g[iX], g[iY]) << 12
		a1 ^= mat24.PloopCocycle(g[iX], g[iY]) << 12
	}
	t = g[iT] + 3 - t
	g[iT] = ((t + (t >> 2)) & 3) - 1
	g[iY] = b1
	g[iX] = a1
}

// Mul puts g = g * g1 for g1 in N_0.
func Mul(g, g1 *N0Elem) {
	MulT(g, g1[iT])
	MulY(g, g1[iY])
	MulX(g, g1[iX])
	MulDeltaPi(g, g1[iD], g1[iPi])
}

// mulInv puts g = g * g1^-1 for g1 in N_0.
func mulInv(g, g1 *N0Elem) {
	MulInvDeltaPi(g, g1[iD], g1[iPi])
	MulX(g, g1[iX]^theta1000(g1[iX]))
	MulY(g, g1[iY]^theta1000(g1[iY]))
	MulT(g, 3-g1[iT])
}

// MulElement puts g3 = g1 * g2 for elements of
// N_0. The arguments may overlap.
func MulElement(g1, g2, g3 *N0Elem) {
	g := *g1
	Mul(&g, g2)
	*g3 = g
}

// InvElement puts g2 = g1^-1 for g1 in N_0.
func InvElement(g1, g2 *N0Elem) {
	var g N0Elem
	mulInv(&g, g1)
	*g2 = g
}

// ConjugateElement puts g3 = g2^-1 g1 g2 for
// elements of N_0. The arguments may overlap.
func ConjugateElement(g1, g2, g3 *N0Elem) {
	var g N0Elem
	mulInv(&g, g2)
	Mul(&g, g1)
	Mul(&g, g2)
	*g3 = g
}

// Shifted atom tags, i.e. the top nibble of each
// atom tag, used by the nMulWordScanCore switch. The
// values are the literal nibbles (tag number, with
// bit 3 set for the inverse forms); n0 keeps them
// here rather than importing the flat-cgt atomTag
// constants so the package depends only on mat24.
const (
	tagShift1  = 0  // atomTag1 >> 28
	tagShiftI1 = 8  // atomTagI1 >> 28
	tagShiftD  = 1  // atomTagD >> 28
	tagShiftID = 9  // atomTagID >> 28
	tagShiftP  = 2  // atomTagP >> 28
	tagShiftIP = 10 // atomTagIP >> 28
	tagShiftX  = 3  // atomTagX >> 28
	tagShiftIX = 11 // atomTagIX >> 28
	tagShiftY  = 4  // atomTagY >> 28
	tagShiftIY = 12 // atomTagIY >> 28
	tagShiftT  = 5  // atomTagT >> 28
	tagShiftIT = 13 // atomTagIT >> 28
	tagShiftL  = 6  // atomTagL >> 28
	tagShiftIL = 14 // atomTagIL >> 28
)

// nMulWordScanCore is the workhorse C
// _mul_word_scan. If index is true it multiplies g
// by the longest N_0 prefix of w and returns the
// number of atoms processed. Otherwise it returns
// the first unprocessed (possibly simplified) atom,
// or 0 if all atoms were processed.
func nMulWordScanCore(g *N0Elem, w []uint32, index bool) uint32 {
	n := uint32(len(w))
	for i := uint32(0); i < n; i++ {
		atom := w[i]
		tag := (atom >> 28) & 0xf
		op := atom & 0xfffffff
		switch tag {
		case tagShiftI1, tagShift1:
		case tagShiftID, tagShiftD:
			MulDeltaPi(g, op&0xfff, 0)
		case tagShiftIP:
			MulInvDeltaPi(g, 0, op)
		case tagShiftP:
			MulDeltaPi(g, 0, op)
		case tagShiftIX:
			op ^= theta1000(op)
			fallthrough
		case tagShiftX:
			MulX(g, op&0x1fff)
		case tagShiftIY:
			op ^= theta1000(op)
			fallthrough
		case tagShiftY:
			MulY(g, op&0x1fff)
		case tagShiftIT:
			op ^= 3
			fallthrough
		case tagShiftT:
			MulT(g, op&3)
		case tagShiftIL:
			op ^= 3
			fallthrough
		case tagShiftL:
			if (op+1)&2 != 0 {
				if index {
					return i
				}
				return 0x60000000 + (op & 3)
			}
		default:
			if index {
				return i
			}
			return atom
		}
	}
	if index {
		return n
	}
	return 0
}

// MulWordScan multiplies g in N_0 by the longest
// N_0 prefix of w and returns its length.
func MulWordScan(g *N0Elem, w []uint32) uint32 {
	return nMulWordScanCore(g, w, true)
}

// MulAtom puts g = g * atom for an atom that is a
// generator of N_0. It returns 0 on success and
// the (possibly simplified) atom on failure.
func MulAtom(g *N0Elem, atom uint32) uint32 {
	return nMulWordScanCore(g, []uint32{atom}, false)
}

// ker tables for ReduceElement.
var kerTableXy = [4]uint16{0, 0x1800, 0x800, 0x1000}

// KerTableYx is the kernel table used to fold the y
// component into x when reducing an N_0 element (the
// C mm_group_n reduce kernel). It is exported because
// the mm-side compress reducer shares it.
var KerTableYx = [4]uint16{0, 0x1000, 0x1800, 0x800}

// ReduceElement reduces g in N_0 to standard form
// and returns 0 iff g is the neutral element.
func ReduceElement(g *N0Elem) uint32 {
	g[0] %= 3
	g[1] &= 0x1fff
	g[2] &= 0x1fff
	g[3] &= 0xfff
	if ((g[1]&0x7ff)+0x7ff)&((g[2]&0x7ff)-1)&0x800 != 0 {
		g[1] ^= uint32(kerTableXy[g[2]>>11])
		g[2] = 0
	} else {
		g[2] ^= uint32(KerTableYx[g[1]>>11])
		g[1] &= 0x7ff
	}
	return g[0] | g[1] | g[2] | g[3] | g[4]
}

// ToWord reduces g in N_0 and converts it to a
// word of generator atoms in w (up to 5). It
// returns the word length. w may alias g.
func ToWord(g *N0Elem, w []uint32) uint32 {
	ReduceElement(g)
	var n uint32
	if g[0] != 0 {
		w[n] = (g[0] & 0xfffffff) | tagT
		n++
	}
	if g[1] != 0 {
		w[n] = (g[1] & 0x1fff) | tagY
		n++
	}
	if g[2] != 0 {
		w[n] = (g[2] & 0x1fff) | tagX
		n++
	}
	if g[3] != 0 {
		w[n] = (g[3] & 0xfff) | tagD
		n++
	}
	if g[4] != 0 {
		w[n] = (g[4] & 0xfffffff) | tagP
		n++
	}
	return n
}

// Atom tag numbers shifted into bits 30..28, used to
// encode N_0 components as generator atoms. n0 keeps
// its own copy (rather than importing flat cgt) to
// preserve its mat24-only dependency.
const (
	tagD = 0x10000000 // 'd'
	tagP = 0x20000000 // 'p'
	tagX = 0x30000000 // 'x'
	tagY = 0x40000000 // 'y'
	tagT = 0x50000000 // 't'
)

// ToWordStd reduces g in N_0 and converts it to a
// word of generator atoms in w (up to 5), in the
// standard generator order x, d, y, p, t used by the
// reducer. Unlike ToWord (order t, y, x, d, p), the
// y component is folded into x/d via a right-coset
// step so the tau power becomes the last atom. It
// returns the word length. w may alias g. C function
// mm_group_n_to_word_std.
func ToWordStd(g *N0Elem, w []uint32) uint32 {
	ReduceElement(g)
	h := *g
	var out [5]uint32
	// 't' part becomes the last generator; remove it from h.
	out[4] = RightCosetNx0(h[:])
	// 'p' part precedes 't'; remove it from h.
	out[3] = h[iPi]
	h[iPi] = 0
	// 'y' part precedes 'p'; fold it into x/d via mul_y.
	y := h[iY] & 0x7ff
	out[2] = y
	y ^= theta1000(y)
	MulY(&h, y)
	ReduceElement(&h)
	// 'x' and 'd' parts come first.
	out[0] = h[iX]
	out[1] = h[iD]
	var n uint32
	if out[0] != 0 {
		w[n] = (out[0] & 0x1fff) | tagX
		n++
	}
	if out[1] != 0 {
		w[n] = (out[1] & 0xfff) | tagD
		n++
	}
	if out[2] != 0 {
		w[n] = (out[2] & 0x1fff) | tagY
		n++
	}
	if out[3] != 0 {
		w[n] = (out[3] & 0xfffffff) | tagP
		n++
	}
	if out[4] != 0 {
		w[n] = (out[4] & 0xfffffff) | tagT
		n++
	}
	return n
}

// RightCosetNx0 changes g in N_0 to an element g'
// of N_x0 and returns e with g = g' * tau^e.
func RightCosetNx0(g []uint32) uint32 {
	ng := (*N0Elem)(g)
	ReduceElement(ng)
	e := g[0]
	if e != 0 && g[3]&0x800 != 0 {
		e = 3 - e
	}
	MulT(ng, 3-e)
	return e
}

// ConjToQx0 tries to find e (0..2) and q in Q_x0
// with g = tau^-e q tau^e. On success it returns q
// in bits 24..0 (Leech encoding) and e in bits
// 26..25. On failure it returns -1.
func ConjToQx0(g []uint32) int32 {
	var t2 N0Elem
	t2[iT] = 2
	g1 := *(*N0Elem)(g)
	ReduceElement(&g1)
	if g1[iPi] != 0 {
		return -1
	}
	e := uint32(0)
	for {
		if (g1[iY] | g1[iT]) == 0 {
			x := g1[iX] & 0x1fff
			x = (x << 12) ^ (uint32(mat24.ThetaTable(x&0x7ff)) & 0xfff)
			x ^= g1[iD] & 0xfff
			return int32(x + (e << 25))
		}
		if e >= 2 {
			return -1
		}
		ConjugateElement(&g1, &t2, &g1)
		ReduceElement(&g1)
		e++
	}
}

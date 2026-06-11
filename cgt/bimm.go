package cgt

// This file ports mmgroup's Bimonster construction:
//   - mmgroup/dev/clifford12/xsp2co1_map.ske
//     (xsp2co1_elem_from_mapping and helpers)
//   - mmgroup/bimm/p3_to_mm.py
//     (Norton's generators, Point/Star mappings,
//      AutP3 -> Monster embedding)
//   - mmgroup/bimm/bimm.py
//     (class BiMM, P3_BiMM, AutP3_BiMM)

import "sync"

// BiMM is an element of the Bimonster M wr 2, of
// structure (M x M).2. It is the pair (m1, m2) of
// Monster elements followed by alpha^alpha, where
// the involution alpha (alpha = 1) swaps the two
// Monster factors.
type BiMM struct {
	m1    *MM
	m2    *MM
	alpha int
}

/*************************************************************************
*** xsp2co1_elem_from_mapping (xsp2co1_map.ske)
*************************************************************************/

// parity64Tab encodes the bit parity of x in
// (parity64Tab >> x) & 1 for 0 <= x < 64. This is
// the PARITY64 constant from xsp2co1_map.ske.
const parity64Tab = uint64(0x6996966996696996)

// xsp2co1Co1GetMapping computes the element g of
// G_{x0} that maps m1[j] to m2[j] (Leech lattice
// encoding) as the image mOut[j] of the standard
// basis. It returns the odd part o of the order of
// g, or a negative value on error.
//
// This is a port of xsp2co1_Co1_get_mapping.
func xsp2co1Co1GetMapping(m1, m2, mOut []uint32) int {
	var m [24]uint64
	// Store columns 0..23 of m1[i] in columns 24..47,
	// columns 0..23 of m2[i] in columns 0..23, and the
	// XOR of column 24 of m1[i], m2[i] in column 48.
	for row := 0; row < 24; row++ {
		m[row] = ((uint64(m2[row]) & 0x1ffffff) << 24) ^
			(uint64(m1[row]) & 0xffffff) ^
			((uint64(m1[row]) & 0x1000000) << 24)
	}

	// Check that scalar products and types (mod 2)
	// agree between m1 and m2; abort otherwise.
	var acc uint64
	for k1 := 0; k1 < 24; k1++ {
		v := m[k1]
		sign := v & (v >> 12)
		sign ^= sign >> 24
		sign ^= sign >> 6
		acc |= parity64Tab >> (sign & 0x3f)
		for k2 := k1 + 1; k2 < 24; k2++ {
			w := m[k2]
			sign = (v & (w >> 12)) ^ (w & (v >> 12))
			sign ^= sign >> 24
			sign ^= sign >> 6
			acc |= parity64Tab >> (sign & 0x3f)
		}
	}
	if acc&1 != 0 {
		return -2
	}

	// Echelonize columns 0..23, adding extraspecial
	// elements (carrying signs in columns 48..59).
	var k1 int
	for col := 0; col < 24; col++ {
		colMask := uint64(1) << uint(col)
		k1 = 23
		for ; k1 >= col; k1-- {
			if m[k1]&colMask != 0 {
				v := m[k1]
				for k2 := k1 - 1; k2 >= 0; k2-- {
					if m[k2]&colMask != 0 {
						sign := v & (m[k2] >> 12)
						sign ^= sign >> 24
						m[k2] ^= v ^ (sign << 48)
					}
				}
				m[k1] = m[col]
				m[col] = v
				break
			}
		}
		if k1 < col {
			return -1
		}
	}

	// Copy the image (with sign) of each basis vector.
	for row := 0; row < 24; row++ {
		sign := ((m[row] >> 48) ^ (m[row] >> 54)) & 0x3f
		sign = (parity64Tab >> sign) & 1
		m[row] = ((m[row] >> 24) & 0xffffff) | (sign << 24)
		mOut[row] = uint32(m[row])
	}

	var bm [24]uint64
	copy(bm[:], m[:])
	return xsp2co1OddOrderBitmatrix(bm[:])
}

// xsp2co1Co1MatrixToWord computes a word g of
// generators of G_{x0} that acts on Q_{x0} as the
// automorphism described by the 24x25 bit matrix m
// (row m[i] is the image of the i-th basis vector,
// Leech lattice encoding). It returns the word
// length, or a negative value on failure. The
// buffer g must have length at least 10.
//
// This is a port of xsp2co1_Co1_matrix_to_word.
func xsp2co1Co1MatrixToWord(m, g []uint32) int {
	var a [24]uint32
	// g' maps the image of Omega (m[23]) to +-Omega.
	res := genLeech2ReduceType4(m[23], g)
	if res < 0 {
		return res
	}
	length := res
	copy(a[:], m[:24])
	if genLeech2OpWordMany(a[:], g[:length]) != length {
		return -100001
	}

	// Compute a permutation in M24 from the bit matrix
	// a[12:23, 12:23].
	var aPi [12]uint32
	for i := 0; i < 11; i++ {
		aPi[i] = (a[i+12] >> 12) & 0x7ff
	}
	aPi[11] = 0
	MatrixFromModOmega(aPi[:])
	perm := MatrixToPerm(aPi[:])
	if PermCheck(perm) != nil {
		return -100002
	}
	pi := PermToM24num(perm)
	if pi != 0 {
		g[length] = atomTagIP + pi
		if genLeech2OpWordMany(a[:], g[length:length+1]) != 1 {
			return -100003
		}
		length++
	}

	// Compute a y_d generator from the image a[11] of
	// an odd Golay cocode vector.
	y := (a[11] >> 12) & 0x7ff
	if y != 0 {
		g[length] = atomTagIY + y
		if genLeech2OpWordMany(a[:], g[length:length+1]) != 1 {
			return -100004
		}
		length++
	}

	// a[i] must equal the i-th basis vector up to sign.
	var accU, q uint32
	for i := 0; i < 24; i++ {
		accU |= a[i] ^ (uint32(1) << uint(i))
		q ^= ((a[i] >> 24) & 1) << uint(i)
	}
	if accU&0xffffff != 0 {
		return -100004
	}

	// Convert the residual x' to generators.
	x := q & 0xfff
	q = (q >> 12) & 0xfff
	q ^= PloopTheta(x&0x7ff) & 0x7ff
	if q != 0 {
		g[length] = atomTagID + q
		length++
	}
	g[length] = atomTagIX + x
	length++

	invertWord(g[:length])
	return length
}

// chi244096 returns the character of the
// representation rho_24 (x) rho_4096 of the element
// elem. The second result is false on failure
// (matching the CHI_24_4096_BAD path in C).
func chi244096(elem []uint64) (int32, bool) {
	var atrace [4]int32
	if !tracesSmallOK(elem, atrace[:]) {
		return 0, false
	}
	return atrace[0] * atrace[2], true
}

// xsp2co1ElemFromMapping computes g in G_{x0} from
// its action m1[j] -> m2[j] on Q_{x0} (Leech lattice
// encoding). On success it stores g as a word of
// generators in the buffer g (length >= 10) and
// returns (length | order<<8 | notZero<<16). It
// returns a negative value on failure.
//
// g is determined up to sign; the function picks the
// representative of odd order if one exists, else the
// one with non-negative character chi(g^o).
//
// This is a port of xsp2co1_elem_from_mapping.
func xsp2co1ElemFromMapping(m1, m2, g []uint32) int {
	var m [24]uint32
	o := xsp2co1Co1GetMapping(m1, m2, m[:])
	if o < 0 {
		return o
	}
	length := xsp2co1Co1MatrixToWord(m[:], g)
	if length < 0 {
		return length
	}
	var elem [26]uint64
	if err := xsp2co1SetElemWord(elem[:], g[:length]); err != nil {
		return -100006
	}
	xsp2co1PowerElem(elem[:], int64(o), elem[:])
	chi, ok := chi244096(elem[:])
	if !ok {
		return -100007
	}
	if chi < 0 {
		g[0] ^= 0x1000
		xsp2co1NegElem(elem[:])
	}
	oo := o
	i := 0
	for ; i < 7; i++ {
		if xsp2co1IsUnitElem(elem[:]) {
			break
		}
		xsp2co1MulElem(elem[:], elem[:], elem[:])
		oo <<= 1
	}
	if i == 7 {
		return -100008
	}
	notZero := 0
	if chi != 0 {
		notZero = 1
	}
	return (notZero << 16) + (oo << 8) + length
}

/*************************************************************************
*** Points and Stars in the Monster (p3_to_mm.py)
*************************************************************************/

// dictPointMOG maps points of P3 to MOG positions
// (Norton, [Nor02] page 80).
var dictPointMOG = map[int]int{
	2: 13, 6: 17, 7: 21,
	8: 14, 11: 18, 4: 22,
	12: 15, 10: 19, 5: 23,
}

// dictPointMOGColumn maps pairs P_0 P_i to MOG
// columns for i in {1,3,9}.
var dictPointMOGColumn = map[int]int{1: 0, 3: 4, 9: 8}

// dictLineMOG maps 'stars' to MOG positions.
var dictLineMOG = map[int]int{
	1: 12, 3: 16, 9: 20,
	12: 1, 11: 5, 7: 9,
	8: 2, 6: 6, 5: 10,
	2: 3, 10: 7, 4: 11,
}

// makeP returns the element x_x x_delta of Q_{x0}
// (in Leech lattice encoding) as an XLeech2, where x
// is a Parker loop element and delta a cocode word.
// It mirrors make_P = XLeech2(PLoop(x), Cocode(delta)).
func makeP(x PLoop, delta Cocode) XLeech2 {
	d := uint32(x.Ord())
	v := ((d << 12) ^ PloopTheta(d)) ^ uint32(delta.Ord())
	return xleech2FromInt(v)
}

// computeP0 returns the image of the pair P_0 P_x in
// Q_{x0} (port of compute_P0).
func computeP0(x int) XLeech2 {
	if x == 0 {
		return xleech2FromInt(0)
	}
	if x == 1 || x == 3 || x == 9 {
		c := dictPointMOGColumn[x]
		octad := []int{0, 4, 8, 12, 16, 20, c + 1, c + 2, c + 3}
		return makeP(NewPLoop(octad), NewCocode(0))
	}
	return makeP(NewPLoop(0), NewCocode([]int{dictPointMOG[x]}))
}

// computeStarP3 returns the image of the 'star'
// P_i^* in Q_{x0} (port of compute_StarP3).
func computeStarP3(i int) XLeech2 {
	if i == 0 {
		return makeP(NewPLoop(0).Invert(), NewCocode([]int{0, 1, 2, 3}))
	}
	if i == 1 || i == 3 || i == 9 {
		return makeP(NewPLoop(0), NewCocode([]int{1, 2, 3, 4, 8, dictPointMOGColumn[i]}))
	}
	octad := []int{0, 4, 8, dictPointMOG[i]}
	for _, y := range P3Incidences(i) {
		octad = append(octad, dictLineMOG[y.Ord()%13])
	}
	point := dictPointMOG[i]
	col := point & (-4)
	var cocode []int
	for j := col + 1; j < col+4; j++ {
		if j != point {
			cocode = append(cocode, j)
		}
	}
	return makeP(NewPLoop(octad), NewCocode(cocode))
}

// p3Precompute holds the precomputed Point/Star
// dictionaries (P0_DICT, PSTAR_DICT).
var (
	p3PrecomputeOnce sync.Once
	p0Dict           [13]XLeech2
	pstarDict        [13]XLeech2
)

func p3Precompute() {
	p3PrecomputeOnce.Do(func() {
		for x := 0; x < 13; x++ {
			p0Dict[x] = computeP0(x)
			pstarDict[x] = computeStarP3(x)
		}
	})
}

// pointP3 maps the product of the 'points' in x
// (even length, entries mod 13) into Q_{x0}, as a
// vector in Leech lattice encoding. Port of PointP3.
func pointP3(x []int) uint32 {
	p3Precompute()
	if len(x)&1 != 0 {
		panic("cgt: pointP3 requires an even number of points")
	}
	p := uint32(0)
	for _, xi := range x {
		p = Leech2Mul(p, p0Dict[((xi%13)+13)%13].Ord())
	}
	return p
}

// starP3Product maps the product of the 'stars' in x
// (entries mod 13) into Q_{x0}. Port of StarP3 for a
// list argument.
func starP3Product(x []int) uint32 {
	p3Precompute()
	p := uint32(0)
	for _, xi := range x {
		p = Leech2Mul(p, pstarDict[((xi%13)+13)%13].Ord())
	}
	return p
}

// mmFromLeech2 builds the Monster element x_d x_delta
// from the element e of Q_{x0} in Leech lattice
// encoding (the full 25-bit value including sign). It
// mirrors MM(XLeech2): d = (e>>12)&0x1fff,
// delta = (e ^ ploop_theta(d)) & 0xfff.
func mmFromLeech2(e uint32) *MM {
	d := (e >> 12) & 0x1fff
	delta := (e ^ PloopTheta(d)) & 0xfff
	return &MM{data: []uint32{atomTagX + d, atomTagD + delta}}
}

/*************************************************************************
*** AutP3 -> Monster (p3_to_mm.py)
*************************************************************************/

// mmFromPerm maps the AutP3 with point permutation
// perm (a length-13 list) into G_{x0}. It returns
// (g, order, special) where g is the word of monster
// atoms, order is the (odd-if-possible) order, and
// special reports whether g could be distinguished
// from -g. Port of MM_from_perm.
func mmFromPerm(perm []int) (g []uint32, order int, special bool) {
	aSrc := make([]uint32, 24)
	aDest := make([]uint32, 24)
	var a [10]uint32
	pi0 := perm[0]
	for i := 0; i < 12; i++ {
		dSrc := []int{0, i + 1}
		dDest := []int{pi0, perm[i+1]}
		aSrc[i] = pointP3(dSrc)
		aSrc[i+12] = starP3Product(dSrc)
		aDest[i] = pointP3(dDest)
		aDest[i+12] = starP3Product(dDest)
	}
	res := xsp2co1ElemFromMapping(aSrc, aDest, a[:])
	if res < 0 {
		panic("cgt: xsp2co1_elem_from_mapping failed in mmFromPerm")
	}
	length := res & 0xff
	g = append([]uint32(nil), a[:length]...)
	order = (res >> 8) & 0xff
	special = (res>>16)&1 != 0
	return g, order, special
}

// precomputedAutP3 stores a fixed embedding of AutP3
// into G_{x0}, mirroring class Precomputed_AutP3.
// It memoizes transversal elements and their images.
type precomputedAutP3 struct {
	mu sync.Mutex

	// transversal[t] holds an AutP3 point permutation
	// (length 13) for index t, or nil if unused; the
	// inverse perm and the in-use flag are tracked
	// separately. Index 1 is the identity.
	transversal [][]int
	tInv        [][]int
	tUsed       []bool

	// data stores the image words; ind[t] is the index
	// into data of the image of transversal[t], or 0 if
	// not yet computed. numMM is the running count of
	// stored images (entry 0 wasted, entry 1 identity).
	data  [][]uint32
	ind   []int
	numMM int

	goodOrders map[int]int  // order -> sign correction
	badOrders  map[int]bool // orders where +-g is ambiguous
}

const precompMaxInd = 2 * 13 * 13

var (
	precompAutP3     *precomputedAutP3
	precompAutP3Once sync.Once
)

func getPrecompAutP3() *precomputedAutP3 {
	precompAutP3Once.Do(func() {
		p := &precomputedAutP3{
			transversal: make([][]int, precompMaxInd),
			tInv:        make([][]int, precompMaxInd),
			tUsed:       make([]bool, precompMaxInd),
			// data is sized to precompMaxInd (>= the 1+192
			// of mmgroup) so that storing one image per
			// distinct transversal index can never overflow.
			data:       make([][]uint32, precompMaxInd),
			ind:        make([]int, precompMaxInd),
			goodOrders: map[int]int{1: 1, 3: 1, 13: 1},
			badOrders:  map[int]bool{},
			numMM:      2,
		}
		// Identity at index 1.
		p.transversal[1] = intRange(13)
		p.tUsed[1] = true
		p.ind[1] = 1
		precompAutP3 = p
	})
	return precompAutP3
}

// splitTransversal splits g into f1*f2 where f1 fixes
// points 0 and 1 and f2 is in a transversal of the
// stabilizer of those points. It returns indices h1,
// h2 into the transversal. Port of _split_transveral.
func (p *precomputedAutP3) splitTransversal(g *AutP3) (int, int) {
	perm := g.Perm()
	checkPermP3(perm)
	h2 := 13*perm[0] + perm[1]
	if !p.tUsed[h2] {
		p.transversal[h2] = append([]int(nil), perm...)
		p.tInv[h2] = invertPermP3(perm)
		p.tUsed[h2] = true
		return 1, h2
	}
	f1 := mulPermP3(perm, p.tInv[h2])
	h1 := 169 + 13*f1[2] + f1[5]
	if !p.tUsed[h1] {
		p.transversal[h1] = append([]int(nil), f1...)
		p.tUsed[h1] = true
	}
	return h1, h2
}

// splitIntoGoodOrders splits h into h1*h2 where both
// factors have orders in goodOrders, choosing h1 at
// random. Port of _split_into_good_orders.
func (p *precomputedAutP3) splitIntoGoodOrders(h *AutP3) (*AutP3, *AutP3) {
	for {
		h1 := NewAutP3Rand()
		h2 := h1.Inv().Mul(h)
		if _, ok1 := p.goodOrders[h1.Order()]; ok1 {
			if _, ok2 := p.goodOrders[h2.Order()]; ok2 {
				return h1, h2
			}
		}
	}
}

// computeImageInMM maps the AutP3 element h into
// G_{x0}, resolving signs via character theory and
// recording sign data for the order of h. Port of
// compute_image_in_MM.
func (p *precomputedAutP3) computeImageInMM(h *AutP3) []uint32 {
	order := h.Order()
	if sign, ok := p.goodOrders[order]; ok {
		g, _, _ := mmFromPerm(h.Perm())
		gm := &MM{data: g}
		if sign == 0 {
			gm = gm.Mul(MMGen("x", 0x1000))
		}
		return gm.Mmdata()
	}
	h1, h2 := p.splitIntoGoodOrders(h)
	g1, _, _ := mmFromPerm(h1.Perm())
	g2, _, _ := mmFromPerm(h2.Perm())
	gProd := (&MM{data: g1}).Mul(&MM{data: g2})
	gData := gProd.Mmdata()
	if p.badOrders[order] {
		return gData
	}
	g1Full, _, special := mmFromPerm(h.Perm())
	g1m := &MM{data: g1Full}
	if !special {
		p.badOrders[order] = true
	} else {
		if g1m.Equal(gProd) {
			p.goodOrders[order] = 1
		} else {
			p.goodOrders[order] = 0
		}
	}
	return gData
}

// mapToMM maps the transversal element with index t
// into G_{x0}, memoizing the result. Port of
// map_to_MM.
func (p *precomputedAutP3) mapToMM(t int) []uint32 {
	if idx := p.ind[t]; idx != 0 {
		return p.data[idx]
	}
	h := NewAutP3(append([]int(nil), p.transversal[t]...))
	gData := p.computeImageInMM(h)
	p.data[p.numMM] = gData
	p.ind[t] = p.numMM
	p.numMM++
	return gData
}

// asMM maps h into the Monster (port of as_MM).
func (p *precomputedAutP3) asMM(h *AutP3) *MM {
	p.mu.Lock()
	defer p.mu.Unlock()
	h1, h2 := p.splitTransversal(h)
	d1 := p.mapToMM(h1)
	d2 := p.mapToMM(h2)
	out := make([]uint32, 0, len(d1)+len(d2))
	out = append(out, d1...)
	out = append(out, d2...)
	return &MM{data: out}
}

// autP3MM embeds the AutP3 element h into the
// Monster (port of AutP3_MM).
func autP3MM(h *AutP3) *MM {
	return getPrecompAutP3().asMM(h)
}

/*************************************************************************
*** Norton's generators (p3_to_mm.py)
*************************************************************************/

// expXNorton is Norton's exponent for generator x:
// MM('t', 1)**EXP_X, set to 1 in mmgroup.
const expXNorton = 1

// nortonGens holds the precomputed images of Norton's
// generators (s, t, u, v, x) in the Monster.
type nortonGens struct {
	s, t, u, v, x *MM
}

var (
	nortonGensVal  *nortonGens
	nortonGensOnce sync.Once
)

func nortonGenerators() *nortonGens {
	nortonGensOnce.Do(func() {
		sAut := NewAutP3(map[int]int{1: 2, 2: 5, 5: 9, 9: 8, 8: 7, 7: 1})
		s := autP3MM(sAut)
		tAut := NewAutP3(map[int]int{0: 12, 12: 3, 3: 0, 1: 2, 2: 4, 4: 1})
		t := autP3MM(tAut)
		// u = s t s^2 t^2
		u := s.Mul(t).Mul(s.Pow(2)).Mul(t.Pow(2))
		// v = MM(PointP3(range(1,13)))
		pts := make([]int, 12)
		for i := range pts {
			pts[i] = i + 1
		}
		v := mmFromLeech2(pointP3(pts))
		x := MMGen("t", expXNorton)
		nortonGensVal = &nortonGens{s: s, t: t, u: u, v: v, x: x}
	})
	return nortonGensVal
}

/*************************************************************************
*** class BiMM (bimm.py)
*************************************************************************/

// gcdInt returns the greatest common divisor of a
// and b (both positive).
func gcdInt(a, b int) int {
	for b > 0 {
		a, b = b, a%b
	}
	return a
}

// NewBiMM returns the Bimonster element
// (m1, m2) * alpha^e. The exponent e is reduced
// modulo 2.
func NewBiMM(m1, m2 *MM, e int) *BiMM {
	return &BiMM{m1: m1, m2: m2, alpha: e & 1}
}

// BiMMIdentity returns the neutral element of the
// Bimonster.
func BiMMIdentity() *BiMM {
	return &BiMM{m1: MMIdentity(), m2: MMIdentity(), alpha: 0}
}

// reduce reduces the two Monster components and the
// involution exponent of b in place.
func (b *BiMM) reduce() {
	b.m1.Reduce()
	b.m2.Reduce()
	b.alpha &= 1
}

// Mul returns the Bimonster product b * other. When
// b has alpha set, the two Monster factors of other
// are swapped before multiplying.
func (b *BiMM) Mul(other *BiMM) *BiMM {
	var m1, m2 *MM
	if b.alpha&1 != 0 {
		m1 = b.m1.Mul(other.m2)
		m2 = b.m2.Mul(other.m1)
	} else {
		m1 = b.m1.Mul(other.m1)
		m2 = b.m2.Mul(other.m2)
	}
	return &BiMM{m1: m1, m2: m2, alpha: (b.alpha ^ other.alpha) & 1}
}

// Inv returns the inverse of b.
func (b *BiMM) Inv() *BiMM {
	var m1, m2 *MM
	if b.alpha&1 != 0 {
		m1, m2 = b.m2, b.m1
	} else {
		m1, m2 = b.m1, b.m2
	}
	return &BiMM{m1: m1.Inv(), m2: m2.Inv(), alpha: b.alpha & 1}
}

// Pow returns b raised to the integer power e.
func (b *BiMM) Pow(e int) *BiMM {
	if e < 0 {
		return b.Inv().Pow(-e)
	}
	result := BiMMIdentity()
	base := b
	for e > 0 {
		if e&1 == 1 {
			result = result.Mul(base)
		}
		base = base.Mul(base)
		e >>= 1
	}
	return result
}

// Orders returns the orders of the two Monster
// components and the parity factor (1 or 2). When
// alpha is set, b is first squared (collapsing it
// into M x M) and the parity factor is 2.
func (b *BiMM) Orders() (int, int, int) {
	b.reduce()
	if b.alpha&1 != 0 {
		a := b.Mul(b)
		return a.m1.Order(), a.m2.Order(), 2
	}
	return b.m1.Order(), b.m2.Order(), 1
}

// Order returns the order of b in the Bimonster.
func (b *BiMM) Order() int {
	o1, o2, s := b.Orders()
	if o1 == 0 || o2 == 0 {
		panic("cgt: BiMM.Order: a Monster component order is 0 " +
			"(order exceeded the search bound in orderElementGx0)")
	}
	return s * o1 * o2 / gcdInt(o1, o2)
}

// Equal reports whether b equals other (after
// reducing both).
func (b *BiMM) Equal(other *BiMM) bool {
	b.reduce()
	other.reduce()
	return b.alpha == other.alpha &&
		b.m1.Equal(other.m1) && b.m2.Equal(other.m2)
}

// Decompose returns (m1, m2, e) with b equal to
// (m1, m2) * alpha^e and e in {0, 1}.
func (b *BiMM) Decompose() (*MM, *MM, int) {
	b.reduce()
	return b.m1, b.m2, b.alpha
}

/*************************************************************************
*** Points/lines and automorphisms into the Bimonster (bimm.py)
*************************************************************************/

// plData[node] holds the two Monster atom words for
// the image of P3_node(node) in the Bimonster; ALPHA
// is implied. Set by bimmPrecompute.
var (
	plData          [26][2][]uint32
	bimmPrecomputeO sync.Once
)

// bimmPrecompute computes plData, mapping point P_i
// to (P_0..P_12 P_i)*ALPHA and line L_i to
// ALPHA*(L_i, L_i^-1). Port of
// precompute_points_lines_list.
func bimmPrecompute() {
	bimmPrecomputeO.Do(func() {
		gens := nortonGenerators()
		alpha := &BiMM{m1: MMIdentity(), m2: MMIdentity(), alpha: 1}
		var vals [26]*BiMM
		for i := 0; i < 13; i++ {
			// P_i = MM(PointP3([0..12] + [i]))
			pts := make([]int, 0, 14)
			for j := 0; j < 13; j++ {
				pts = append(pts, j)
			}
			pts = append(pts, i)
			e := pointP3(pts)
			vals[i] = &BiMM{m1: mmFromLeech2(e), m2: mmFromLeech2(e), alpha: 1}
		}
		for i := 0; i < 13; i++ {
			// L_i = (v*x) ** (u**i)
			ui := gens.u.Pow(i)
			vx := gens.v.Mul(gens.x)
			lI := ui.Inv().Mul(vx).Mul(ui)
			bm := alpha.Mul(&BiMM{m1: lI, m2: lI.Inv(), alpha: 0})
			vals[13+i] = bm
		}
		for i := 0; i < 26; i++ {
			m1, m2, _ := vals[i].Decompose()
			plData[i][0] = m1.Mmdata()
			plData[i][1] = m2.Mmdata()
		}
	})
}

// P3BiMM maps a word of generators of IncP3 into the
// Bimonster. Each entry of word is a P3 node ordinal
// (point 0..12 or line 13..25). Port of P3_BiMM.
func P3BiMM(word []int) *BiMM {
	bimmPrecompute()
	pl := make([]int, len(word))
	for i, w := range word {
		pl[i] = NewP3Node(w).Ord()
	}
	var data0, data1 []uint32
	n := len(pl)
	for i := 0; i < n&^1; i += 2 {
		k := pl[i]
		data0 = append(data0, plData[k][0]...)
		data1 = append(data1, plData[k][1]...)
		k = pl[i+1]
		data0 = append(data0, plData[k][1]...)
		data1 = append(data1, plData[k][0]...)
	}
	al := n & 1
	if al != 0 {
		k := pl[n-1]
		data0 = append(data0, plData[k][0]...)
		data1 = append(data1, plData[k][1]...)
	}
	return &BiMM{m1: &MM{data: data0}, m2: &MM{data: data1}, alpha: al}
}

// AutP3BiMM maps an automorphism of P3 into the
// Bimonster. Port of AutP3_BiMM.
func AutP3BiMM(g *AutP3) *BiMM {
	gm := autP3MM(g)
	return &BiMM{m1: gm, m2: &MM{data: gm.Mmdata()}, alpha: 0}
}

// BiMMCoxeterExp returns the Coxeter exponent of the
// two P3 generators x1 and x2 (node ordinals): 1 if
// equal, 3 for an incident point/line pair whose
// ordinals sum (mod 13) to 0, 1, 3, or 9, and 2
// otherwise. This is the off-diagonal entry of the
// Coxeter matrix of the group IncP3.
func BiMMCoxeterExp(x1, x2 int) int {
	mi, ma := x1, x2
	if mi > ma {
		mi, ma = ma, mi
	}
	if mi < 13 && ma >= 13 {
		switch (mi + ma) % 13 {
		case 0, 1, 3, 9:
			return 3
		}
	}
	if mi != ma {
		return 2
	}
	return 1
}

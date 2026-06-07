package cgt

// Involution analysis for elements of G_{x0}
// (involutions.c, xsp2co1_traces.c) and the
// subset of the N_0 group machinery (mm_group_n.c)
// needed to conjugate an involution to a standard
// form. Ported here alongside xsp2.go.

// The N_0 word algebra (mm_group_n.c) lives in
// monster.go: the N0Elem type, nMul, nMulElement,
// nConjugateElement, nMulWordScan, nReduceElement,
// nToWord, nConjToQx0 and the index constants
// iT..iPi are reused here.

/*************************************************************************
*** Conversion between G_x0 and N_0
*************************************************************************/

// xsp2co1ElemToN0 converts elem to an element of
// N_0 in g[5]. It returns an error if elem is not
// in N_0.
func xsp2co1ElemToN0(elem []uint64, g []uint32) error {
	var a [10]uint32
	lenA := xsp2co1ElemToWord(elem, a[:])
	ng := (*N0Elem)(g)
	*ng = N0Elem{}
	if int(nMulWordScan(ng, a[:lenA])) < lenA {
		return errNotInGx0
	}
	return nil
}

// xsp2co1ElemFromN0 converts the N_0 element g to
// G_x0 representation in elem. It returns an error
// if g is not in G_{x0}.
func xsp2co1ElemFromN0(elem []uint64, g []uint32) error {
	var g1 [5]uint32
	nReduceElement((*N0Elem)(g))
	if g[0] != 0 {
		return errNotInGx0
	}
	lenG := nToWord((*N0Elem)(g), g1[:])
	return xsp2co1SetElemWord(elem, g1[:lenG])
}

/*************************************************************************
*** Conjugate an element of G_x0 by a word in the monster
*************************************************************************/

// xsp2co1ConjugateElem replaces elem by
// a^{-1} elem a for the word a of monster
// generators, where membership in G_{x0} holds
// for every prefix conjugation. It returns an
// error on failure.
func xsp2co1ConjugateElem(elem []uint64, a []uint32) error {
	var elemA [26]uint64
	aPending := false
	for len(a) > 0 {
		k := xsp2co1SetElemWordScan(elemA[:], a, aPending)
		if k > 0 {
			aPending = true
			a = a[k:]
			if len(a) == 0 {
				break
			}
		}
		var aN N0Elem
		k = int(nMulWordScan(&aN, a))
		if k == 0 {
			return errNotInGx0
		}
		a = a[k:]
		if nReduceElement(&aN) == 0 {
			continue
		}
		if aN[0] == 0 {
			var aNword [5]uint32
			lenANword := int(nToWord(&aN, aNword[:]))
			if xsp2co1SetElemWordScan(elemA[:], aNword[:lenANword], aPending) != lenANword {
				return errNotInGx0
			}
			aPending = true
			continue
		}
		if aPending {
			xsp2co1ConjElem(elem, elemA[:], elem)
			aPending = false
		}
		var eN N0Elem
		if err := xsp2co1ElemToN0(elem, eN[:]); err != nil {
			return err
		}
		nConjugateElement(&eN, &aN, &eN)
		if err := xsp2co1ElemFromN0(elem, eN[:]); err != nil {
			return err
		}
	}
	if aPending {
		xsp2co1ConjElem(elem, elemA[:], elem)
	}
	return nil
}

/*************************************************************************
*** Involution invariants (involutions.c)
*************************************************************************/

// squareMat24Nonzero returns the low 24 bits of
// the square of the 24x24 bit matrix m (0 iff the
// square is zero).
func squareMat24Nonzero(m []uint64) uint64 {
	var result uint64
	for i := 0; i < 24; i++ {
		mi := m[i]
		var mo uint64
		for j := 0; j < 24; j++ {
			mo ^= (0 - ((mi >> uint(j)) & 1)) & m[j]
		}
		result |= mo
	}
	return result & 0xffffff
}

// leechTypeMod2 returns the type modulo 2 of a
// Leech-lattice-mod-2 vector v.
func leechTypeMod2(v uint64) uint64 {
	x := v
	x &= x >> 12
	return uint64(parity12(uint32(x)))
}

// xsp2co1InvolutionInvariants computes invariant
// spaces for an involution elem of G_{x0}. The
// 12-row output is stored in invar; the function
// returns the dimension k, or a negative value on
// error.
func xsp2co1InvolutionInvariants(elem, invar []uint64) int32 {
	var data [40]uint64
	pa := data[16:]
	invar[0] = 0x8000000
	for i := 1; i < 12; i++ {
		invar[i] = 0
	}
	for i := 0; i < 24; i++ {
		pa[i] = uint64(1) << uint(i)
	}
	xsp2co1XspecialConjugate(elem, pa[:24], false)
	for i := 0; i < 24; i++ {
		pa[i] &= 0xffffff
		pa[i] ^= 0x100000001 << uint(i)
	}
	if squareMat24Nonzero(pa) != 0 {
		return -1
	}
	n := bm64EchelonH(pa, 24, 24, 24)

	if n == 0 {
		invar[0] = uint64(xsp2co1XspecialVector(elem)) & 0xffffff
		if invar[0] == 0 {
			return 0
		}
		invar[0] |= (leechTypeMod2(invar[0]) << 24) | 0x4000000
		return 1
	}

	if n == 8 {
		bm64RotBits(pa[8:], 16, 32, 64, 0)
		xsp2co1XspecialConjugate(elem, pa[8:8+16], true)
		bm64EchelonH(pa[8:], 16, 25, 1)
		t1 := (pa[8] >> 24) & 1
		for i := 0; i < 8; i++ {
			invar[int(t1)+i] = pa[i]
		}
		if t1 == 0 {
			goto finalEchelonize
		}
		leech2MatrixOrthogonal(pa[8:], data[:], 16)
		invar[0] = data[0] & 0xffffff
		invar[0] |= 0x4000000
		t0 := leechTypeMod2(invar[0])
		invar[0] |= t0 << 24
		for i := 1; i < 9; i++ {
			tt := t0 ^ leechTypeMod2(invar[0]^invar[i])
			invar[i] |= tt << 24
		}
		n = 9
	} else if n == 12 {
		for i := 0; i < 12; i++ {
			data[i] = pa[i]
		}
		xsp2co1XspecialConjugate(elem, data[:16], true)
		for i := 0; i < 12; i++ {
			invar[i] = (pa[i] & 0xffffff00ffffff) | (data[i] & 0x1000000) | (leechTypeMod2(pa[i]) << 25)
		}
	} else {
		return -2
	}

finalEchelonize:
	bm64XchBits(invar, n, 12, 0x800)
	bm64RotBits(invar, n, 1, 12, 0)
	bm64EchelonH(invar, n, 27, 27)
	bm64RotBits(invar, n, 11, 12, 0)
	bm64XchBits(invar, n, 12, 0x800)
	invar[0] &= ((invar[0] & 0x4000000) << 2) - 1
	return int32(n)
}

// xsp2co1InvolutionOrthogonal computes the
// orthogonal complement of the linear form in
// column col+25 of invar under the Wall
// parametrization. It returns the vector v, or a
// negative value on error.
func xsp2co1InvolutionOrthogonal(invar []uint64, col uint32) int32 {
	var M [12]uint64
	var T [24]uint64
	if col > 1 {
		return -1
	}
	if invar[0]&0x8000000 != 0 {
		return -1
	}
	col += 24
	n := 12
	for n > 0 && invar[n-1] == 0 {
		n--
	}
	pA := invar
	if pA[0]&0x4000000 != 0 {
		pA = pA[1:]
		n--
	}
	if n == 0 {
		return 0
	}
	var v uint64
	for i := 0; i < n; i++ {
		v |= ((pA[i] >> col) & 1) << uint(i)
	}
	if v == 0 {
		return 0
	}
	for i := 0; i < n; i++ {
		M[i] = pA[i]
	}
	bm64RotBits(M[:], n, 32, 64, 0)
	bm64T(M[:], n, 24, T[:])
	bm64RotBits(M[:], n, 32, 64, 0)
	bm64RotBits(M[:], n, 12, 24, 0)
	bm64Mul(M[:], T[:], n, 24, M[:])
	if !bm64Inv(M[:], n) {
		return -1
	}
	vv := []uint64{v}
	bm64Mul(vv, M[:], 1, n, vv)
	bm64Mul(vv, pA, 1, 24, vv)
	return int32(vv[0] & 0xffffff)
}

/*************************************************************************
*** Find type-4 vector in involution invariants
*************************************************************************/

// expandAffine lists all vectors aa + sum m_i *
// a[k-1-i] for 0 <= m < 2^k into b.
func expandAffine(a []uint64, k int, aa uint64, b []uint32) {
	b[0] = uint32(aa)
	exp := 1
	for i := 0; i < k; i++ {
		v := uint32(a[k-i-1])
		for j := 0; j < exp; j++ {
			b[exp+j] = v ^ b[j]
		}
		exp <<= 1
	}
}

// subFindType searches the set {a0[i0] + a1[i1]}
// for a nice type-4 vector. It returns such a
// vector or 0. a is destroyed.
func subFindType(a []uint32, n0, n1 int, guide uint32) uint32 {
	subtypes := [6]uint8{0x48, 0x40, 0x42, 0x44, 0x46, 0x43}
	n2 := n0 + n1
	for i0 := 0; i0 < n2; i0++ {
		a[i0] &= 0xffffff
	}
	if guide == 0xffffffff {
		for i0 := 0; i0 < n0; i0++ {
			for i1 := n0; i1 < n2; i1++ {
				v := a[i0] ^ a[i1]
				if Leech2Type(v) == 4 {
					return v
				}
			}
		}
		return 0
	}
	guide &= 0xffffff
	guideType := Leech2Type(guide)
	if guideType == 4 {
		for i0 := 0; i0 < n0; i0++ {
			v := a[i0] ^ guide
			for i1 := n0; i1 < n0+n1; i1++ {
				if v == a[i1] {
					return a[i0] ^ a[i1]
				}
			}
		}
	}
	var w [0x80]uint32
	if guideType == 2 {
		for i0 := 0; i0 < n0; i0++ {
			for i1 := n0; i1 < n2; i1++ {
				v := a[i0] ^ a[i1]
				if Leech2Type2(v^guide) != 0 {
					w[Leech2Subtype(v)&0x7f] = v
				}
			}
		}
		for i0 := 0; i0 < 6; i0++ {
			if v := w[subtypes[i0]]; v != 0 {
				return v
			}
		}
	}
	for i := range w {
		w[i] = 0
	}
	for i0 := 0; i0 < n0; i0++ {
		for i1 := n0; i1 < n2; i1++ {
			v := a[i0] ^ a[i1]
			w[Leech2Subtype(v)&0x7f] = v
		}
	}
	for i0 := 0; i0 < 6; i0++ {
		if v := w[subtypes[i0]]; v != 0 {
			return v
		}
	}
	return 0
}

// xsp2co1InvolutionFindType4 returns a type-4
// vector in the space I_1 computed by
// xsp2co1InvolutionInvariants (dim I_1 = 8 case),
// or 0.
func xsp2co1InvolutionFindType4(invar []uint64, guide uint32) int32 {
	if invar[0]&0x8000000 != 0 {
		return -901
	}
	n := 12
	for n > 0 && (invar[n-1]&0xffffff) == 0 {
		n--
	}
	pInv := invar
	for n > 0 && (pInv[0]&0xf000000) != 0 {
		pInv = pInv[1:]
		n--
	}
	if n > 8 {
		return int32(-1000 - n)
	}
	var a [48]uint32
	n1 := n
	if n1 > 4 {
		n1 = 4
	}
	n0 := n - n1
	e0 := 1 << uint(n0)
	expandAffine(pInv, n0, 0, a[:])
	expandAffine(pInv[n0:], n1, 0, a[e0:])
	return int32(subFindType(a[:], e0, 1<<uint(n1), guide))
}

// xsp2co1InvolutionFindCoset8 finds a type-4
// vector written as a sum of two type-2 vectors
// in I_1^+ \ I_1 (dim I_1 = 8, dim I_1^+ = 9), or
// 0.
func xsp2co1InvolutionFindCoset8(invar []uint64, guide uint32) int32 {
	var a0, a1, a2 [16]uint32
	if invar[0]&0x8000000 != 0 {
		return -901
	}
	if invar[0]&0xf000000 == 0 {
		return -951
	}
	if invar[1]&0xf000000 != 0 {
		return -952
	}
	if invar[9]&0xffffff != 0 {
		return -953
	}
	expandAffine(invar[1:], 4, invar[0], a0[:])
	expandAffine(invar[5:], 4, 0, a1[:])
	i2 := 0
	for i0 := 0; i0 < 16; i0++ {
		for i1 := 0; i1 < 16; i1++ {
			v := a0[i0] ^ a1[i1]
			if Leech2Type2(v) != 0 && i2 < 16 {
				a2[i2] = v
				i2++
			}
		}
	}
	if i2 > 0 {
		i2--
	}
	return int32(subFindType(a2[:], 1, i2, guide))
}

// xsp2co1ElemFindType4 tries to find a type-4
// vector v such that conjugating elem by an
// element mapping v to Omega yields an element of
// N_{x0}. It returns v, or a negative value.
func xsp2co1ElemFindType4(elem []uint64, guide uint32) int32 {
	v := xsp2co1XspecialVector(elem)
	if v >= 0 {
		return 0x800000
	}
	var invar [12]uint64
	n := xsp2co1InvolutionInvariants(elem, invar[:])
	if n < 0 {
		return n
	}
	switch n {
	case 8:
		vv := xsp2co1InvolutionFindType4(invar[:], guide)
		if vv != 0 {
			return vv
		}
		return -1
	case 9:
		vv := xsp2co1InvolutionOrthogonal(invar[:], 0)
		t := Leech2Type(uint32(vv))
		if t == 4 {
			return vv
		}
		if t == 0 {
			vv = xsp2co1InvolutionFindCoset8(invar[:], guide)
			if vv > 0 {
				return vv
			}
		}
		vv = xsp2co1InvolutionFindType4(invar[:], guide)
		if vv != 0 {
			return vv
		}
		return -1
	case 12:
		vv := xsp2co1InvolutionOrthogonal(invar[:], 1)
		t := Leech2Type(uint32(vv))
		if t == 4 {
			return vv
		}
		return -1
	default:
		return -1
	}
}

/*************************************************************************
*** Map an involution in G_x0 to Q_x0 and to a standard form
*************************************************************************/

// xsp2co1ElemConjGx0ToQx0 finds an element h of
// the monster (stored as a word in a) with
// h^{-1} g h = q in Q_{x0}. It returns q in bits
// 24..0 and the number of atoms in bits 27,26,25,
// or a negative value on failure.
func xsp2co1ElemConjGx0ToQx0(elem []uint64, a []uint32, baby bool) int32 {
	var elem1 [26]uint64
	var eN [5]uint32
	v := xsp2co1ElemFindType4(elem, 0)
	if v < 0 {
		return v
	}
	var length int
	if baby {
		length = genLeech2ReduceType2Ortho(uint32(v), a)
	} else {
		length = genLeech2ReduceType4(uint32(v), a)
	}
	if length < 0 {
		return -1
	}
	xsp2co1CopyElem(elem, elem1[:])
	if err := xsp2co1ConjugateElem(elem1[:], a[:length]); err != nil {
		return -1
	}
	if err := xsp2co1ElemToN0(elem1[:], eN[:]); err != nil {
		return -1
	}
	v = nConjToQx0(eN[:])
	if v < 0 {
		return -1
	}
	if v&0x6000000 != 0 {
		a[length] = 0xd0000000 + ((uint32(v) >> 25) & 3)
		length++
	}
	v &= 0x1ffffff
	return v | int32((length&7)<<25)
}

// xsp2co1ElemConjugateInvolution conjugates the
// involution elem to a standard form, storing the
// conjugating monster word in a. It returns
// 0x100*I + len(a) where I is 0 (identity), 1 (2A)
// or 2 (2B), or a negative value on failure.
func xsp2co1ElemConjugateInvolution(elem []uint64, a []uint32) int32 {
	v := xsp2co1ElemConjGx0ToQx0(elem, a, false)
	if v < 0 {
		return v
	}
	length := int(v >> 25)
	v &= 0x1ffffff
	if v == 0 {
		return 0
	}
	if v == 0x1000000 {
		return int32(0x200 + length)
	}
	switch Leech2Type(uint32(v)) {
	case 2:
		l1 := genLeech2ReduceType2(uint32(v), a[length:])
		if l1 < 0 {
			return -1
		}
		v = int32(Leech2OpWord(uint32(v), a[length:length+l1]))
		length += l1
		if v&0x1000000 != 0 {
			a[length] = 0xb0000200
			v = int32(Leech2OpAtom(uint32(v), a[length]))
			length++
		}
		if v != 0x200 {
			return -1
		}
		return int32(0x100 + length)
	case 4:
	default:
		return -1
	}
	l1 := genLeech2ReduceType4(uint32(v), a[length:])
	if l1 < 0 {
		return -1
	}
	v = int32(Leech2OpWord(uint32(v), a[length:length+l1]))
	if v & ^int32(0x1000000) != 0x800000 {
		return -1
	}
	length += l1
	a[length] = 0xd0000002 - ((uint32(v) >> 24) & 1)
	length++
	return int32(0x200 + length)
}

/*************************************************************************
*** Public involution methods
*************************************************************************/

// InGx0 reports whether g lies in the subgroup
// G_{x0}. Elements of this type are always in
// G_{x0} by construction.
func (g *Xsp2Co1) InGx0() bool {
	return true
}

// ConjugateInvolution returns (I, h) where h is
// an element conjugating the involution g to the
// standard representative z (I = 0 for the
// identity, 1 for a 2A and 2 for a 2B
// involution). ConjugateInvolution panics if g is
// not an involution or the conjugating element is
// not in G_{x0}.
func (g *Xsp2Co1) ConjugateInvolution() (int, *Xsp2Co1) {
	var a [15]uint32
	res := xsp2co1ElemConjugateInvolution(g.data[:], a[:])
	if res < 0 {
		panic("cgt: element is not an involution")
	}
	length := int(res & 0xff)
	out := &Xsp2Co1{}
	if err := xsp2co1SetElemWord(out.data[:], a[:length]); err != nil {
		panic(err.Error())
	}
	return int(res >> 8), out
}

/*************************************************************************
*** Fast trace via the involution class table (xsp2co1_traces.c)
*************************************************************************/

// xsp2co1ElemInvolutionClass returns class
// information for elem if it maps to an involution
// in Co_1, or 0 otherwise.
func xsp2co1ElemInvolutionClass(elem []uint64) int32 {
	var invar [12]uint64
	vTypes := [4]uint16{0x22, 0, 0x21, 0x2041}
	n := xsp2co1InvolutionInvariants(elem, invar[:])
	if n < 0 {
		return 0
	}
	switch n {
	case 0:
		if xsp2co1IsUnitElem(elem) {
			return 0x1011
		}
		return 0x3022
	case 1:
		v := xsp2co1XspecialVector(elem)
		if v < 0 {
			return 0
		}
		return int32(vTypes[Leech2Type(uint32(v))&3])
	case 8:
		var traces [4]int32
		if !tracesSmallOK(elem, traces[:]) {
			return 0
		}
		if traces[2] > 0 {
			return 0x1121
		}
		return 0x1122
	case 9:
		t := (invar[1] >> 24) & 1
		n2 := xsp2co1Leech2CountType2(invar[:], 9)
		switch n2 {
		case 0:
			if t != 0 {
				return 0x143
			}
			return 0x2143
		case 2:
			return 0x142
		case 16:
			if t != 0 {
				return 0x141
			}
			return 0x122
		default:
			return 0
		}
	case 12:
		inv0 := (invar[0] >> 24) & 3
		t := (invar[1] >> 24) & 1
		if inv0&2 != 0 {
			if t != 0 {
				invar[1] = 0
				n2 := xsp2co1Leech2CountType2(invar[1:], 11)
				switch n2 {
				case 120:
					return 0x344
				case 132:
					return 0x2382
				case 136:
					return 0x343
				case 152:
					return 0x342
				default:
					return 0
				}
			}
			if inv0&1 != 0 {
				return 0x322
			}
			return 0x341
		}
		if inv0&1 != 0 {
			return 0x244
		}
		return 0x2244
	default:
		return 0
	}
}

// tracesSmallOK is xsp2co1TracesSmall returning
// false on the overflow error path instead of
// panicking, for the involution classifier.
func tracesSmallOK(elem []uint64, ptrace []int32) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	xsp2co1TracesSmall(elem, ptrace)
	return true
}

// chi98280Keys / chi98280Data are the precomputed
// character table for rho_98280 keyed by class.
var chi98280Keys = [20]uint16{
	0x21, 0x22, 0x122, 0x141, 0x142, 0x143, 0x244, 0x322,
	0x341, 0x342, 0x343, 0x344, 0x1011, 0x1121, 0x1122, 0x2041,
	0x2143, 0x2244, 0x2382, 0x3022,
}
var chi98280Data = [20]int32{
	4072, -24, 232, 232, 8, -24, 0, 264,
	264, 40, 8, -24, 98280, 2280, 2280, -24,
	-24, 0, 0, 98280,
}

// trace98280Fast returns the character of
// rho_98280 from the precomputed table, with ok
// false if the class is not tabulated.
func trace98280Fast(elem []uint64) (int32, bool) {
	cl := xsp2co1ElemInvolutionClass(elem)
	if cl > 0 {
		for i := 0; i < 20; i++ {
			if cl == int32(chi98280Keys[i]) {
				return chi98280Data[i], true
			}
		}
	}
	return 0, false
}

// xsp2co1TracesFast computes the characters
// rho_24, rho_576, rho_4096, rho_98280 of elem
// into ptrace, using the fast table for the hard
// rho_98280 cases.
func xsp2co1TracesFast(elem []uint64, ptrace []int32) {
	xsp2co1TracesSmall(elem, ptrace)
	ptrace[3] = xsp2co1Trace98280(elem, trace98280Fast)
}

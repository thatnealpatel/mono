// Package leech implements the Leech lattice mod 2
// (Q_{x0}) layer of the monster's G_{x0} machinery:
// the XLeech2 element type, its pure (mm-free)
// constructors, the lattice mod-2/mod-3 operations,
// the Leech2OpAtom/Leech2OpWord family describing the
// action of G_{x0} on Q_{x0}, the bm64 basis/radical
// composites, and the reduction and orbit drivers.
//
// The mm-coupled XLeech2 constructors (those that
// take or yield a monster element, parse a named
// q-element, or index the short-vector table) live in
// the flat cgt package, which imports leech.
package leech

import (
	"math/bits"
	"math/rand/v2"

	"patel.codes/cgt/generator"
	"patel.codes/cgt/mat24"
	"patel.codes/cgt/swar"
)

// XLeech2 is an element of the group Q_{x0},
// equivalently a vector of the Leech lattice
// mod 2, in Leech lattice encoding.
type XLeech2 struct {
	ord uint32
}

// NewXLeech2 returns the neutral (zero/identity)
// element of Q_{x0}. It mirrors the Python
// XLeech2() with no arguments.
func NewXLeech2() *XLeech2 {
	return &XLeech2{ord: 0}
}

// NewXLeech2FromInt creates an XLeech2 from a value
// in Leech lattice encoding. Only bits 0..24 are
// retained. It mirrors XLeech2(int).
func NewXLeech2FromInt(v uint32) *XLeech2 {
	x := XLeech2FromInt(v)
	return &x
}

// XLeech2FromInt builds an XLeech2 value (not a
// pointer) from a Leech-lattice-encoded integer,
// retaining only bits 0..24. It is the value-returning
// workhorse behind the public constructors, exported
// so the flat-cgt mm-coupled constructors can build an
// XLeech2 without reaching into unexported state.
func XLeech2FromInt(v uint32) XLeech2 {
	return XLeech2{ord: v & 0x1ffffff}
}

// NewXLeech2Copy returns a copy of x. It mirrors
// the Python XLeech2(XLeech2) deep-copy path.
//
// NewXLeech2Copy panics if x is nil.
func NewXLeech2Copy(x *XLeech2) *XLeech2 {
	if x == nil {
		panic("NewXLeech2Copy: nil XLeech2")
	}
	c := *x
	return &c
}

// NewXLeech2FromPLoop returns the element
// x_d * x_delta of Q_{x0}, where d is the Parker
// loop element and c the Golay cocode element. c
// may be nil, denoting the trivial cocode element.
// It mirrors XLeech2(mat24.PLoop, mat24.Cocode).
//
// NewXLeech2FromPLoop panics if d is nil.
func NewXLeech2FromPLoop(d *mat24.PLoop, c *mat24.Cocode) *XLeech2 {
	if d == nil {
		panic("NewXLeech2FromPLoop: nil PLoop")
	}
	pv := uint32(d.Ord()) & 0x1fff
	val := (pv << 12) ^ mat24.PloopTheta(pv)
	if c != nil {
		val ^= uint32(c.Ord()) & 0xfff
	}
	x := XLeech2FromInt(val)
	return &x
}

// NewXLeech2Random returns a uniformly random
// element of Q_{x0}, in Leech lattice encoding. It
// mirrors XLeech2('r').
func NewXLeech2Random() *XLeech2 {
	x := XLeech2FromInt(uint32(rand.IntN(0x2000000)))
	return &x
}

// Ord returns the number of the element, a value
// with 0 <= ord < 0x2000000.
func (x XLeech2) Ord() uint32 {
	return x.ord
}

// Type returns the type of the corresponding
// Leech lattice mod 2 vector (0, 2, 3, or 4).
func (x XLeech2) Type() uint32 {
	return generator.Leech2Type(x.ord)
}

// Subtype returns the packed subtype (same as
// gen_leech2_subtype). Python .subtype unpacks to
// a tuple.
func (x XLeech2) Subtype() uint32 {
	return generator.Leech2Subtype(x.ord)
}

// Bitvector returns the 24 coordinates of the
// vector in the Leech lattice mod 2, as a 24-byte
// slice of 0/1 values.
func (x XLeech2) Bitvector() []byte {
	v := x.ord & 0xffffff
	out := make([]byte, 24)
	for i := uint32(0); i < 24; i++ {
		out[i] = byte((v >> i) & 1)
	}
	return out
}

// Leech2Scalprod returns the scalar product of a
// and b in the Leech lattice mod 2, 0 or 1. Both
// are in Leech lattice encoding.
func Leech2Scalprod(a, b uint32) uint32 {
	scalar := (((a >> 12) & b) ^ ((b >> 12) & a)) & 0xfff
	return mat24.Parity12(scalar)
}

// Short3Reduce reduces every coordinate of a
// Leech-mod-3 vector to 0, 1, or 2.
func Short3Reduce(v3 uint64) uint64 {
	a := (v3 & (v3 >> 24)) & 0xffffff
	v3 ^= a | (a << 24)
	return v3 & 0xffffffffffff
}

// Leech2To3Short maps a short vector (type 2)
// from Lambda/2 to Lambda/3. The result is unique
// up to sign. Returns 0 if v2 is not short.
func Leech2To3Short(x uint32) uint64 {
	v2 := uint64(x)
	gcodev := uint64(mat24.GcodeToVect(x >> 12))
	theta := uint64(mat24.ThetaTable((x >> 12) & 0x7ff))
	// w = weight(code word gcodev) / 4
	w := uint64(0) - ((v2 >> 23) & 1)
	w = (((theta >> 12) & 7) ^ w) + (w & 7)

	if v2&0x800 != 0 { // case odd cocode
		cocodev := uint64(mat24.CocodeSyndromeRaw(uint32(v2^theta), 0))
		if cocodev&(cocodev-1) != 0 {
			return 0
		}
		scalar := (v2 >> 12) & v2
		scalar = uint64(mat24.Parity12(uint32(scalar)))
		if scalar&1 != 0 {
			return 0
		}
		result := (gcodev ^ ((gcodev ^ 0xffffff) << 24)) &^ (cocodev | (cocodev << 24))
		return result
	}
	// even cocode: v2[11..0] = cocode word (cocode rep)
	v2 ^= theta
	switch w {
	case 4:
		gcodev ^= 0xffffff
		fallthrough
	case 2:
		cocodev := uint64(mat24.CocodeSyndromeRaw(uint32(v2), mat24.Lsbit24(uint32(gcodev))))
		cW := uint64(mat24.Bw24(uint32(cocodev)))
		if (cocodev&gcodev) != cocodev || (cW^2^w)&3 != 0 {
			return 0
		}
		return (gcodev &^ cocodev) | (cocodev << 24)
	case 3:
		return 0
	default: // case 0 or 6 only
		cList := mat24.CocodeToBitList(uint32(v2), 0)
		if len(cList) != 2 {
			return 0
		}
		return (uint64(1) << cList[0]) + (uint64(1) << (uint64(cList[1]) + 24 - 4*w))
	}
}

// Leech3To2Short maps a short vector (type 2)
// from Lambda/3 to Lambda/2. The result is
// unique. Returns 0 if v3 is not short. Inverse
// of Leech2To3Short.
func Leech3To2Short(x uint64) uint32 {
	v3 := Short3Reduce(x)
	w1 := mat24.Bw24(uint32(v3))
	w2 := mat24.Bw24(uint32(v3 >> 24))
	var gcodev, cocodev uint32
	switch w1 + w2 {
	case 23:
		cocodev = ^uint32(v3|(v3>>24)) & 0xffffff
		if cocodev == 0 || cocodev&(cocodev-1) != 0 {
			return 0
		}
		gcodev = uint32(v3>>((0-(w1&1))&24)) & 0xffffff
		if (w1+1)&4 != 0 {
			gcodev ^= 0xffffff
		}
	case 8:
		if w1&1 != 0 {
			return 0
		}
		gcodev = uint32(v3|(v3>>24)) & 0xffffff
		cocodev = uint32(v3) & 0xffffff
		if w1&2 != 0 {
			gcodev ^= 0xffffff
		}
	case 2:
		cocodev = uint32(v3|(v3>>24)) & 0xffffff
		if w1&1 != 0 {
			gcodev = 0
		} else {
			gcodev = 0xffffff
		}
	default:
		return 0
	}
	gc, ok := VectToGcodeRaw(gcodev)
	if !ok || gc&0xfffff000 != 0 {
		return 0
	}
	theta := uint32(mat24.ThetaTable(gc&0x7ff)) & 0xfff
	coc := mat24.VectToCocode(cocodev)
	return (gc << 12) ^ theta ^ coc
}

// VectToGcodeRaw returns the Golay code number of
// vector v and true, or (0, false) if v is not a
// code word (the non-panicking analogue of
// mat24.VectToGcode, matching C mat24_vect_to_gcode).
func VectToGcodeRaw(v uint32) (uint32, bool) {
	cn := mat24.Vintern(v)
	if cn&0xfff != 0 {
		return 0, false
	}
	return cn >> 12, true
}

// bm64RestoreCapH copies the used rows of ma back
// into m, placing rows marked in rowsCap last.
// Returns (number of cap rows, true), or
// (0, false) on failure.
func bm64RestoreCapH(ma, m []uint64, rowsUsed, rowsCap uint64, maxRows int) (int, bool) {
	iMax := bits.Len64(rowsUsed)
	nUsed := bits.OnesCount64(rowsUsed)
	rowsCap &= rowsUsed
	nCap := bits.OnesCount64(rowsCap)
	rowH := nUsed - nCap
	if maxRows < nUsed {
		return 0, false
	}
	row := 0
	for i := 0; i < iMax; i++ {
		if (rowsUsed>>uint(i))&1 != 0 {
			if (rowsCap>>uint(i))&1 != 0 {
				m[rowH] = ma[i]
				rowH++
			} else {
				m[row] = ma[i]
				row++
			}
		}
	}
	return nCap, true
}

// bm64CapH computes the intersection of the row
// spaces of the echelonized matrices m1 (i1 rows)
// and m2 (i2 rows), over columns j0-1,...,j0-n.
// It rearranges m1 and m2 so the last nCap rows
// hold the intersection, and returns (nCap, true)
// (or (0, false) on failure).
func bm64CapH(m1, m2 []uint64, i1, i2, j0, n int) (int, bool) {
	var m1a, m2a [64]uint64
	var rowsUsed1, rowsUsed2, rowsEqu uint64
	var v1, v2 uint64

	if j0 > 64 {
		j0 = 64
	}
	if n > j0 {
		n = j0
	}
	if n == 0 {
		return 0, true
	}

	mask := (((uint64(1) + 1) << uint(n-1)) - 1) << uint(j0-n)
	for i1 > 0 && m1[i1-1]&mask == 0 {
		i1--
	}
	for i2 > 0 && m2[i2-1]&mask == 0 {
		i2--
	}
	if i1 == 0 || i2 == 0 {
		return 0, true
	}

	rowPos1, rowPos2, rowPos := 0, 0, 0
	for col := j0 - 1; col >= j0-n; col-- {
		var b1, b2 uint64
		if rowPos1 < i1 {
			b1 = (m1[rowPos1] >> uint(col)) & 1
		}
		if rowPos2 < i2 {
			b2 = (m2[rowPos2] >> uint(col)) & 1
		}
		rowsUsed1 |= b1 << uint(rowPos)
		rowsUsed2 |= b2 << uint(rowPos)

		if b1 != 0 {
			if b2 != 0 {
				m1a[rowPos] = m1[rowPos1]
				rowPos1++
				m2a[rowPos] = m2[rowPos2]
				rowPos2++
				rowsEqu |= uint64(1) << uint(rowPos)
				rowPos++
			} else {
				v1 = m1[rowPos1]
				rowPos1++
				m1a[rowPos] = v1
				for row := rowPos - 1; row >= 0; row-- {
					m1a[row] ^= v1 & (0 - ((rowsEqu >> uint(row)) & (m2a[row] >> uint(col)) & 1))
				}
				rowPos++
			}
		} else {
			if b2 != 0 {
				v2 = m2[rowPos2]
				rowPos2++
				m2a[rowPos] = v2
				for row := rowPos - 1; row >= 0; row-- {
					m2a[row] ^= v2 & (0 - ((rowsEqu >> uint(row)) & (m1a[row] >> uint(col)) & 1))
				}
				rowPos++
			} else {
				row := rowPos - 1
				for ; row >= 0; row-- {
					if ((m1a[row]^m2a[row])>>uint(col))&(rowsEqu>>uint(row))&1 != 0 {
						rowsEqu &^= uint64(1) << uint(row)
						v1 = m1a[row]
						v2 = m2a[row]
						break
					}
				}
				for row--; row >= 0; row-- {
					if ((m1a[row]^m2a[row])>>uint(col))&(rowsEqu>>uint(row))&1 != 0 {
						m1a[row] ^= v1
						m2a[row] ^= v2
					}
				}
			}
		}
	}

	if _, ok := bm64RestoreCapH(m1a[:], m1, rowsUsed1, rowsEqu, i1); !ok {
		return 0, false
	}
	return bm64RestoreCapH(m2a[:], m2, rowsUsed2, rowsEqu, i2)
}

// getLeech2Basis computes a basis of the space
// generated by v2[0..n-1] in basis[0..k-1] and
// returns k, the dimension. d is an upper bound
// for the dimension.
func getLeech2Basis(v2 []uint32, basis []uint64, d uint32) uint32 {
	var pos [24]uint8
	k := uint32(0)
	for i := range v2 {
		w := v2[i] & 0xffffff
		for j := uint32(0); j < k; j++ {
			w ^= (0 - ((w >> pos[j]) & 1)) & uint32(basis[j])
		}
		if w == 0 {
			continue
		}
		j := w & (0 - w)
		pos[k] = uint8(mat24.Lsbit24Pwr2(j))
		basis[k] = uint64(w)
		k++
		if k >= d {
			break
		}
	}
	return k
}

// Leech2MatrixBasis returns a basis of the
// subspace of the Leech lattice mod 2 generated
// by the vectors in v2, echelonized in a special
// column order (Omega first).
func Leech2MatrixBasis(v2 []uint32) []uint64 {
	basis := make([]uint64, 24)
	dim := int(getLeech2Basis(v2, basis, 24))
	swar.Bm64XchBits(basis, dim, 12, 0x800)
	swar.Bm64RotBits(basis, dim, 1, 12, 0)
	swar.Bm64EchelonH(basis, dim, 24, 24)
	swar.Bm64RotBits(basis, dim, 11, 12, 0)
	swar.Bm64XchBits(basis, dim, 12, 0x800)
	return basis[:dim]
}

// Leech2MatrixOrthogonal computes a basis b of
// the Leech lattice mod 2 such that b[m..23] is a
// basis of the orthogonal complement of the space
// spanned by a[0..k-1]. It returns (m, true), or
// (0, false) if k > 24. a has k rows; b must have
// 24 rows.
func Leech2MatrixOrthogonal(a, b []uint64, k int) (int, bool) {
	if k > 24 {
		return 0, false
	}
	// Bl = A^T
	swar.Bm64T(a, k, 24, b)
	// Bh = Q * A^T: exchange row i of A^T with row i+12
	for i := 0; i < 12; i++ {
		x := b[i]
		b[i] = b[i+12] << 24
		b[i+12] = x << 24
	}
	// Store the unit matrix in Bl
	for i := 0; i < 24; i++ {
		b[i] |= 1 << uint(i)
	}
	m := swar.Bm64EchelonL(b, 24, 24, k)
	for i := 0; i < 24; i++ {
		b[i] &= 0xffffff
	}
	return m, true
}

// Leech2MatrixRadical returns a basis of the
// radical of the subspace of the Leech lattice
// mod 2 generated by the vectors in v2. The
// radical is the intersection of that space with
// its orthogonal complement.
func Leech2MatrixRadical(v2 []uint32) []uint64 {
	var span, ortho [24]uint64
	basis := make([]uint64, 24)
	dim := int(getLeech2Basis(v2, span[:], 24))
	if _, ok := Leech2MatrixOrthogonal(span[:], ortho[:], dim); !ok {
		return basis[:0]
	}
	swar.Bm64EchelonH(span[:], dim, 24, 24)
	swar.Bm64EchelonH(ortho[dim:], 24-dim, 24, 24)
	res, ok := bm64CapH(span[:], ortho[dim:], dim, 24-dim, 24, 24)
	if !ok {
		return basis[:0]
	}
	for i := 0; i < res; i++ {
		basis[i] = span[i+dim-res]
	}
	swar.Bm64XchBits(basis, res, 12, 0x800)
	swar.Bm64RotBits(basis, res, 1, 12, 0)
	swar.Bm64EchelonH(basis, res, 24, 24)
	swar.Bm64RotBits(basis, res, 11, 12, 0)
	swar.Bm64XchBits(basis, res, 12, 0x800)
	return basis[:res]
}

// Leech3OpPi performs x_pi on a Leech-mod-3
// vector v3 using permutation perm.
func Leech3OpPi(v3 uint64, perm []byte) uint64 {
	var w3 uint64
	for i := uint(0); i < 24; i++ {
		w3 |= ((v3 >> i) & 0x1000001) << perm[i]
	}
	return w3
}

// Leech3OpY performs y_d on a Leech-mod-3 vector
// v3, with d an element of the Parker loop.
func Leech3OpY(v3 uint64, d uint32) uint64 {
	v := uint64(mat24.GcodeToVect(d))
	return v3 ^ (v | (v << 24))
}

// Leech3OpXi performs xi^e on a Leech-mod-3
// vector v3.
func Leech3OpXi(v3 uint64, e uint32) uint64 {
	e %= 3
	if e == 0 {
		return v3
	}
	ee1 := uint64(0) - uint64((e-1)&1)
	v3 ^= 0x111111111111 &^ ee1
	// multiply x with the 4x4 Hadamard-like matrix
	a := (v3 & 0xaaaaaa555555) ^ ((v3 >> 23) & 0xaaaaaa) ^ ((v3 & 0xaaaaaa) << 23)
	a ^= 0xcccccc000000
	b := (a >> 2) & 0x333333333333
	a &= 0x333333333333
	// 1st Hadamard step
	t := a + b
	b = a + (b ^ 0x333333333333)
	a = t & 0x444444444444
	a = t - a + (a >> 2)
	t = b & 0x444444444444
	b = b - t + (t >> 2)
	// exchange high and low part of b
	b = ((b >> 24) & 0xffffff) + ((b & 0xffffff) << 24)
	// 2nd Hadamard step
	t = a + b
	b = a + (b ^ 0x333333333333)
	a = t & 0x444444444444
	a = t - a + (a >> 2)
	t = b & 0x444444444444
	b = b - t + (t >> 2)
	// unite a and b
	a = a ^ (b << 2)
	a ^= 0xcccccc000000
	a = (a & 0xaaaaaa555555) ^ ((a >> 23) & 0xaaaaaa) ^ ((a & 0xaaaaaa) << 23)
	a ^= 0x111111111111 & ee1
	return a
}

// Leech3OpVectorWord returns the vector v3 . g
// for a Leech-mod-3 vector v3 and an element g of
// G_{x0} given as a word of generators. Returns
// 0xffff000000000000 if g contains an illegal
// generator.
func Leech3OpVectorWord(v3 uint64, g []uint32) uint64 {
	perm := make([]byte, 24)
	permI := make([]byte, 24)
	for i := range g {
		v := g[i]
		tag := v >> 28
		v &= 0xfffffff
		switch tag {
		case 8, 0, 8 + 1, 1, 8 + 3, 3:
			// no operation
		case 8 + 2:
			copy(perm, mat24.M24numToPermSafe(v))
			copy(permI, mat24.InvPerm(perm))
			v3 = Leech3OpPi(v3, permI)
		case 2:
			copy(perm, mat24.M24numToPermSafe(v))
			v3 = Leech3OpPi(v3, perm)
		case 8 + 4, 4:
			v3 = Leech3OpY(v3, v&0x1fff)
		case 8 + 5, 5:
			if (v+1)&2 != 0 {
				return 0xffff000000000000
			}
		case 8 + 6:
			v ^= 3
			fallthrough
		case 6:
			if (v+1)&2 != 0 {
				v3 = Leech3OpXi(v3, v&3)
			}
		default:
			return 0xffff000000000000
		}
	}
	return Short3Reduce(v3)
}

// Leech2Pow returns the power x1**e of x1 in
// Q_{x0}, in Leech lattice encoding.
func Leech2Pow(x uint32, e uint8) uint32 {
	var scalar uint32
	x &= 0x1ffffff
	if e&2 != 0 {
		scalar = (x >> 12) & x
		scalar = mat24.Parity12(scalar)
		scalar <<= 24
	}
	if e&1 != 0 {
		return x ^ scalar
	}
	return scalar
}

// opXDDelta performs q0 x_d x_delta on Q_{x0}.
func opXDDelta(q0, d, delta uint32) uint32 {
	delta ^= uint32(mat24.ThetaTable(d & 0x7ff))
	s := ((q0 >> 12) & delta) ^ (q0 & d)
	s = mat24.Parity12(s)
	return q0 ^ (s << 24)
}

// opDeltaPi performs the conjugation q0 ->
// q0^(x_delta x_pi) on Q_{x0}, using the encoded
// permutation perm and automorphism autpl.
func opDeltaPi(q0 uint32, perm []byte, autpl []uint32) uint32 {
	xd := (q0 >> 12) & 0x1fff
	xdelta := (q0 ^ uint32(mat24.ThetaTable((q0>>12)&0x7ff))) & 0xfff
	xd = mat24.OpPloopAutpl(xd, autpl)
	xdelta = mat24.OpCocodePerm(xdelta, perm)
	return (xd << 12) ^ xdelta ^ (uint32(mat24.ThetaTable(xd&0x7ff)) & 0xfff)
}

// opY performs q0 -> q0^(y_d) on Q_{x0}, with d
// an element of the Parker loop.
func opY(q0, d uint32) uint32 {
	odd := 0 - ((q0 >> 11) & 1)
	thetaQ0 := uint32(mat24.ThetaTable((q0 >> 12) & 0x7ff))
	thetaY := uint32(mat24.ThetaTable(d & 0x7ff))
	s := (thetaQ0 & d) ^ (^odd & q0 & d)
	s = mat24.Parity12(s)
	o := (thetaY & (q0 >> 12)) ^ (q0 & d)
	o ^= (thetaY >> 12) & 1 & odd
	o = mat24.Parity12(o)
	eps := thetaQ0 ^ (thetaY & ^odd) ^ uint32(mat24.ThetaTable(((q0>>12)^d)&0x7ff))
	q0 ^= (eps & 0xfff) ^ ((d << 12) & 0x1fff000 & odd)
	q0 ^= (s << 24) ^ (o << 23)
	return q0
}

// imgOmegaTable is IMG_OMEGA.
var imgOmegaTable = [2][4]uint8{{0, 2, 3, 1}, {0, 3, 1, 2}}

// imgOmega returns the image of q under the
// triality element tau^e. q must be one of +-1,
// +-Omega in Leech lattice encoding; e is 1 or 2.
func imgOmega(q, e uint32) uint32 {
	return uint32(imgOmegaTable[e-1][(q>>23)&3]) << 23
}

// Leech2OpAtom returns g^{-1} q0 g for q0 in
// Q_{x0} and a single generator atom g of
// G_{x0}. q0 and the result are in Leech lattice
// encoding. Returns 0xffffffff if the atom is
// illegal.
func Leech2OpAtom(x, g uint32) uint32 {
	q0 := x & 0x1ffffff
	perm := make([]byte, 24)
	var autpl []uint32
	v := g
	tag := v & 0xf0000000
	v &= 0xfffffff
	var y uint32
	switch tag {
	case 0x00000000, 0x80000000: // 1, I1
		// unit
	case 0x10000000, 0x90000000: // d, Id
		q0 = opXDDelta(q0, 0, v&0xfff)
	case 0xa0000000: // Ip
		if v == 0 {
			break
		}
		iPerm := mat24.M24numToPermSafe(v)
		perm, autpl = mat24.PermToIautpl(0, iPerm)
		q0 = opDeltaPi(q0, perm, autpl)
	case 0x20000000: // p
		if v == 0 {
			break
		}
		copy(perm, mat24.M24numToPermSafe(v))
		autpl = mat24.PermToAutpl(0, perm)
		q0 = opDeltaPi(q0, perm, autpl)
	case 0x30000000, 0xb0000000: // x, Ix
		q0 = opXDDelta(q0, v&0xfff, 0)
	case 0xc0000000: // Iy
		y ^= uint32(mat24.ThetaTable(v&0x7ff)) & 0x1000
		y ^= v & 0x1fff
		q0 = opY(q0, y&0x1fff)
	case 0x40000000: // y
		y ^= v & 0x1fff
		q0 = opY(q0, y&0x1fff)
	case 0xd0000000: // It
		v ^= 3
		fallthrough
	case 0x50000000: // t
		if (v+1)&2 != 0 {
			if q0&0x7ff800 != 0 {
				return 0xffffffff
			}
			q0 = imgOmega(q0, v&3) ^ (q0 & 0x7ff)
		}
	case 0xe0000000: // Il
		v ^= 3
		fallthrough
	case 0x60000000: // l
		q0 = generator.XiOpXi(q0, int(v&3))
	default:
		return 0xffffffff
	}
	return q0
}

// Leech2OpWord returns g^{-1} q0 g for q0 in
// Q_{x0} (Leech lattice encoding) and a word g of
// generators of G_{x0}. Leech2OpWord panics if an
// atom of g is illegal.
func Leech2OpWord(x uint32, g []uint32) uint32 {
	q0 := x & 0x1ffffff
	for _, atom := range g {
		q0 = Leech2OpAtom(q0, atom)
		if q0 == 0xffffffff {
			panic("leech: illegal atom in Leech2OpWord")
		}
	}
	return q0
}

// GenLeech2OpWordMany applies the word g to every
// entry of q in place, returning the number of
// atoms successfully applied to all entries.
func GenLeech2OpWordMany(q []uint32, g []uint32) int {
	for j := range q {
		q[j] &= 0x1ffffff
	}
	next := make([]uint32, len(q))
	for i, atom := range g {
		for j, qv := range q {
			r := Leech2OpAtom(qv, atom)
			if r == 0xffffffff {
				return i
			}
			next[j] = r
		}
		copy(q, next)
	}
	return len(g)
}

// opPermNoSign applies the permutation pi to the Leech
// lattice mod 2 vector v, ignoring the sign. C function
// op_perm_nosign.
func opPermNoSign(v uint32, pi []byte) uint32 {
	xd := (v >> 12) & 0xfff
	xdelta := (v ^ uint32(mat24.ThetaTable(xd&0x7ff))) & 0xfff
	xd = mat24.OpGcodePerm(xd, pi)
	xdelta = mat24.OpCocodePerm(xdelta, pi)
	return (xd << 12) ^ xdelta ^ (uint32(mat24.ThetaTable(xd&0x7ff)) & 0xfff)
}

// opYNoSign applies x_d to the Leech lattice mod 2
// vector q0, ignoring the sign. C function op_y_nosign.
func opYNoSign(q0, d uint32) uint32 {
	odd := 0 - ((q0 >> 11) & 1)
	thetaQ0 := uint32(mat24.ThetaTable((q0 >> 12) & 0x7ff))
	thetaY := uint32(mat24.ThetaTable(d & 0x7ff))
	o := (thetaY & (q0 >> 12)) ^ (q0 & d)
	o ^= (thetaY >> 12) & 1 & odd
	o = mat24.Parity12(o)
	eps := thetaQ0 ^ (thetaY & ^odd) ^ uint32(mat24.ThetaTable(((q0>>12)^d)&0x7ff))
	q0 ^= (eps & 0xfff) ^ ((d << 12) & 0xfff000 & odd)
	q0 ^= o << 23
	return q0
}

// GenLeech2OpWordLeech2Many applies the word g (or its
// inverse if back) to every entry of a in place,
// ignoring the sign of each Leech-mod-2 vector. It
// returns 0 on success or a negative value if any
// generator in g is not in G_x0. C function
// gen_leech2_op_word_leech2_many.
//
// Unlike GenLeech2OpWordMany, this is the sign-free
// operation: tags x, d and the identity are ignored (they
// only change signs), tags p/y/l act via the nosign
// helpers, and a nonzero tau (tag t) or opaque atom
// (tag 7) makes the word leave G_x0.
func GenLeech2OpWordLeech2Many(a []uint32, g []uint32, back bool) int {
	step := 1
	imask := uint32(0)
	idx := 0
	if back {
		step = -1
		imask = 0x80000000
		idx = len(g) - 1
	}
	for n := len(g); n > 0; n-- {
		v := g[idx] ^ imask
		idx += step
		tag := v & 0xf0000000
		v &= 0xfffffff
		switch tag {
		case 0xa0000000: // Ip
			perm := mat24.M24numToPermSafe(v)
			perm = mat24.InvPerm(perm)
			for j := range a {
				a[j] = opPermNoSign(a[j], perm)
			}
		case 0x20000000: // p
			perm := mat24.M24numToPermSafe(v)
			for j := range a {
				a[j] = opPermNoSign(a[j], perm)
			}
		case 0xc0000000: // Iy
			y := uint32(mat24.ThetaTable(v&0x7ff)) & 0x1000
			y ^= v & 0x1fff
			for j := range a {
				a[j] = opYNoSign(a[j], y&0x1fff)
			}
		case 0x40000000: // y
			y := v & 0x1fff
			for j := range a {
				a[j] = opYNoSign(a[j], y&0x1fff)
			}
		case 0xe0000000, 0x60000000: // Il, l
			if tag == 0xe0000000 {
				v ^= 3
			}
			for j := range a {
				a[j] = generator.XiOpXiNoSign(a[j], int(v))
			}
		case 0xd0000000, 0x50000000: // It, t
			if (v-1)&2 == 0 {
				return -1
			}
		case 0x70000000, 0xf0000000:
			if v != 0 {
				return -1
			}
		default: // 1, I1, d, Id, x, Ix: no effect on sign-free image
		}
	}
	return 0
}

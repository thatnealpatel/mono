package cgt

import (
	"errors"
	"fmt"
)

// errNoM24Perm indicates no M24 permutation
// completes the given partial map.
var errNoM24Perm = errors.New("cgt: no M24 permutation exists for this map")

// Mat24Order is the order of the Mathieu
// group M24.
const Mat24Order = 244823040

// mat24SuboctadWeights packs the halved
// suboctad weights parity (C macro
// MAT24_SUBOCTAD_WEIGHTS).
const mat24SuboctadWeights uint64 = 0xe88181178117177e

// lsbit24 returns the position of the least
// significant set bit of v, or 24 if the low
// 24 bits are zero.
func lsbit24(v uint32) uint32 {
	return uint32(mat24LsbitTable[(0x077cb531*(v&(0-v))>>26)&0x1f])
}

// lsbit24pwr2 is lsbit24 for a power of two.
func lsbit24pwr2(v uint32) uint32 {
	return uint32(mat24LsbitTable[(v*0x077cb531>>26)&0x1f])
}

// Parity12 returns the bit parity of the low
// 12 bits of v.
func Parity12(v uint32) uint32 {
	return uint32((uint64(0x6996966996696996) >> ((v ^ (v >> 6)) & 0x3f)) & 1)
}

// SynFromTable expands a MAT24_SYNDROME_TABLE
// entry to a syndrome bit vector.
func SynFromTable(t uint32) uint32 {
	return (1 << (t & 31)) ^ (1 << ((t >> 5) & 31)) ^ (1 << ((t >> 10) & 31))
}

// Vintern computes the cocode/Vintern image of
// a vector (combined ENC tables).
func Vintern(v uint32) uint32 {
	return mat24EncTable0[v&0xff] ^
		mat24EncTable1[(v>>8)&0xff] ^
		mat24EncTable2[(v>>16)&0xff]
}

// GcodeToVectInternal expands a Golay code
// number to a vector (combined DEC tables).
func GcodeToVectInternal(v uint32) uint32 {
	return mat24DecTable1[(v<<4)&0xf0] ^ mat24DecTable2[(v>>4)&0xff]
}

// oddSyn returns syndrome(x, 24) for an odd
// parity vector x.
func oddSyn(x uint32) uint32 {
	x = Vintern(x)
	x = uint32(mat24SyndromeTable[x&0x7ff])
	return SynFromTable(x)
}

// GcodeToVect converts a Golay code number to a
// bit vector in GF(2)^24.
func GcodeToVect(v uint32) uint32 {
	return GcodeToVectInternal(v)
}

// VectToGcode returns the Golay code number of
// vector v.
//
// VectToGcode panics if v is not a Golay code
// word.
func VectToGcode(v uint32) uint32 {
	cn := Vintern(v)
	if cn&0xfff != 0 {
		panic("VectToGcode: vector is not a Golay code word")
	}
	return cn >> 12
}

// Bw24 returns the bit weight of the low 24
// bits of v.
func Bw24(v uint32) uint32 {
	v = (v & 0x555555) + ((v & 0xaaaaaa) >> 1)
	v = (v & 0x333333) + ((v & 0xcccccc) >> 2)
	v = (v + (v >> 4)) & 0xf0f0f
	return (v + (v >> 8) + (v >> 16)) & 0x1f
}

// GcodeWeight returns the bit weight of Golay
// code word v divided by 4.
func GcodeWeight(v uint32) uint32 {
	t := 0 - ((v >> 11) & 1)
	return (((uint32(mat24ThetaTable[v&0x7ff]) >> 12) & 7) ^ t) + (t & 7)
}

// VectToBitList returns the bit weight w of v
// and a 24-byte list. The first w entries are
// the ascending set-bit positions; the rest are
// the ascending clear-bit positions.
func VectToBitList(v uint32) (int, []byte) {
	out := make([]byte, 24)
	w := Bw24(v)
	v <<= 3 // bit 0 is now at position 3
	j := w  // write to pos 0 if clear, pos w if set
	for i := uint32(0); i < 24; i++ {
		o := (v >> i) & 8
		out[(j>>o)&0x1f] = byte(i)
		j += 1 << o
	}
	return int(w), out
}

// GcodeToBitList returns the ascending set-bit
// positions of Golay code word v.
func GcodeToBitList(v uint32) []byte {
	vect := GcodeToVectInternal(v)
	w, out := VectToBitList(vect)
	return out[:w]
}

// Lsbit24 returns the position of the least
// significant set bit of v, or 24 if the low 24
// bits are zero.
func Lsbit24(v uint32) uint32 {
	return lsbit24(v)
}

// ExtractB24 gathers the bits of v selected by
// mask into the low bits of the result.
func ExtractB24(v, mask uint32) uint32 {
	var res, sh uint32
	v &= mask
	for i := uint32(0); i < 24; i++ {
		res |= ((v >> i) & 1) << sh
		sh += (mask >> i) & 1
	}
	return res
}

// SpreadB24 scatters the low bits of v to the
// positions selected by mask.
func SpreadB24(v, mask uint32) uint32 {
	var res, sh uint32
	for i := uint32(0); i < 24; i++ {
		res |= (((v >> sh) & 1) << i) & mask
		sh += (mask >> i) & 1
	}
	return res
}

// VectToVintern converts v from vector to
// Vintern representation.
func VectToVintern(v uint32) uint32 {
	return Vintern(v)
}

// VinternToVect converts v from Vintern to
// vector representation.
func VinternToVect(v uint32) uint32 {
	return mat24DecTable0[v&0xff] ^
		mat24DecTable1[(v>>8)&0xff] ^
		mat24DecTable2[(v>>16)&0xff]
}

// VectToCocode returns the cocode element of
// vector v.
func VectToCocode(v uint32) uint32 {
	return Vintern(v) & 0xfff
}

// CocodeToVect returns one preimage in
// GF(2)^24 of cocode element c.
func CocodeToVect(c uint32) uint32 {
	return VinternToVect(c)
}

// Syndrome returns a minimum-weight Golay code
// syndrome of v. If the minimum weight is 4 the
// tetrad containing bit tetrad is returned.
//
// Syndrome panics if tetrad is out of range for
// the syndrome weight.
func Syndrome(v, tetrad uint32) uint32 {
	return CocodeSyndrome(Vintern(v), tetrad)
}

// CocodeSyndrome returns a minimum-weight Golay
// code syndrome of cocode element c. See
// Syndrome for the meaning of tetrad.
//
// CocodeSyndrome panics if tetrad is out of
// range for the syndrome weight.
func CocodeSyndrome(c, tetrad uint32) uint32 {
	r := CocodeSyndromeRaw(c, tetrad)
	if r == 0xffffffff {
		panic("CocodeSyndrome: tetrad out of range")
	}
	return r
}

// CocodeSyndromeRaw returns the syndrome or
// 0xffffffff on failure.
func CocodeSyndromeRaw(c1, tetrad uint32) uint32 {
	if tetrad > 24 {
		return 0xffffffff
	}
	bad := (tetrad + 8) >> 5 // bad = (tetrad >= 24)
	tetrad -= bad            // change 24 to 23
	y := 0 - (((c1 >> 11) + 1) & 1)
	bad &= y
	c1 ^= mat24RecipBasis[tetrad&31] & y
	y &= 1 << tetrad
	syn := uint32(mat24SyndromeTable[c1&0x7ff])
	syn = (1 << (syn & 31)) | (1 << ((syn >> 5) & 31)) | (1 << ((syn >> 10) & 31))
	bad &= ((syn & (y | 0x1000000)) - 1) >> 25
	syn ^= y
	return (syn & 0xffffff) | (0 - (bad & 1))
}

// CocodeToBitList returns the ascending bit
// positions of the syndrome of cocode element c.
// See Syndrome for the meaning of tetrad.
//
// CocodeToBitList panics if tetrad is out of
// range for the syndrome weight.
func CocodeToBitList(c, tetrad uint32) []byte {
	out := make([]byte, 4)
	if tetrad > 24 {
		panic("CocodeToBitList: tetrad out of range")
	}
	var syn, length, i, tmp uint32
	if c&0x800 == 0 { // even cocode word
		bad := uint32(0)
		if tetrad == 24 {
			bad = 1
		}
		tetrad -= bad
		c ^= mat24RecipBasis[tetrad&31]
		syn = uint32(mat24SyndromeTable[c&0x7ff])
		var a [6]uint32
		a[3], a[4], a[5] = 24, 24, 24
		a[0] = syn & 31
		a[1] = (syn >> 5) & 31
		length = 4
		if a[1] == 24 {
			length = 2
		}
		a[2] = (syn >> 10) & 31
		a[length-1] = tetrad
		i = length - 1
		for i > 0 && a[i] < a[i-1] {
			tmp = a[i]
			a[i] = a[i-1]
			a[i-1] = tmp
			i--
		}
		if i > 0 && a[i] == a[i-1] {
			a[i-1] = a[i+1]
			a[i] = a[i+2]
			length -= 2
		}
		out[0] = byte(a[0])
		out[1] = byte(a[1])
		out[2] = byte(a[2])
		out[3] = byte(a[3])
		if bad != 0 && length == 4 {
			panic("CocodeToBitList: tetrad out of range")
		}
		return out[:length]
	}
	// odd cocode word
	syn = uint32(mat24SyndromeTable[c&0x7ff])
	out[0] = byte(syn & 31)
	out[1] = byte((syn >> 5) & 31)
	length = 3
	if out[1] == 24 {
		length = 1
	}
	out[2] = byte((syn >> 10) & 31)
	out[3] = 24
	return out[:length]
}

// CocodeToSextet stores the six tetrads of the
// sextet of cocode element c as 24 bit
// positions, six groups of four in lexical
// order.
//
// CocodeToSextet panics if c has minimum weight
// other than 4.
func CocodeToSextet(c uint32) []byte {
	out := make([]byte, 24)
	if c&0x800 != 0 {
		panic("CocodeToSextet: cocode element is not a tetrad")
	}
	c2 := c ^ mat24RecipBasis[0]
	syn := uint32(mat24SyndromeTable[c2&0x7ff])
	if syn&31 == 0 {
		panic("CocodeToSextet: cocode element is not a tetrad")
	}
	out[0] = 0
	out[1] = byte(syn & 31)
	out[2] = byte((syn >> 5) & 31)
	out[3] = byte((syn >> 10) & 31)
	v := uint32(0xffffff)
	for k := uint32(4); k < 24; k += 4 {
		v ^= (1 << out[k-4]) ^ (1 << out[k-3]) ^ (1 << out[k-2]) ^ (1 << out[k-1])
		out[k] = byte(lsbit24(v))
		c2 = c ^ mat24RecipBasis[out[k]]
		syn = uint32(mat24SyndromeTable[c2&0x7ff])
		out[k+1] = byte(syn & 31)
		out[k+2] = byte((syn >> 5) & 31)
		out[k+3] = byte((syn >> 10) & 31)
	}
	return out
}

// AllSyndromes returns all minimum-weight Golay
// code syndromes of vector v as bit vectors.
// The result has length 1 or 6.
func AllSyndromes(v uint32) []uint32 {
	return CocodeAllSyndromes(Vintern(v))
}

// CocodeAllSyndromes returns all minimum-weight
// Golay code syndromes of cocode element c as
// bit vectors. The result has length 1 or 6.
func CocodeAllSyndromes(c uint32) []uint32 {
	out := make([]uint32, 6)
	syn := uint32(mat24SyndromeTable[c&0x7ff])
	out[0] = (1 << (syn & 31)) ^ (1 << ((syn >> 5) & 31)) ^ (1 << ((syn >> 10) & 31))
	if c&0x800 != 0 {
		return out[:1]
	}
	out[0] ^= 1
	if syn>>15 != 0 || out[0] == 0 {
		return out[:1]
	}
	remain := 0xffffff & ^out[0]
	for i := 1; i < 6; i++ {
		next := remain & (0 - remain)
		b := mat24RecipBasis[lsbit24pwr2(next)]
		syn = uint32(mat24SyndromeTable[(c^b)&0x7ff])
		out[i] = (1 << (syn & 31)) ^ (1 << ((syn >> 5) & 31)) ^ (1 << ((syn >> 10) & 31)) ^ next
		remain &= ^out[i]
	}
	return out
}

// CocodeWeight returns the minimum possible bit
// weight of cocode element c.
func CocodeWeight(c uint32) uint32 {
	syn := uint32(mat24SyndromeTable[c&0x7ff])
	if c&0x800 != 0 {
		return 3 - ((((syn & 0x7fff) + 0x2000) >> 15) << 1)
	}
	mask := (0 - (c & 0xfff)) >> 16
	return (4 - ((syn >> 15) << 1)) & mask
}

// VectType returns the M24-orbit type of vector
// v encoded as 32*c + w, with w the weight and
// c the type code (0 special, 1 umbral, 2
// transversal, 3 extraspecial, 4 penumbral).
func VectType(v uint32) uint32 {
	vt := func(weight, typ uint32) uint32 { return (typ << 5) + weight }
	w := Bw24(v)
	if w == 12 {
		synd := AllSyndromes(v)
		if len(synd) == 1 {
			if synd[0] != 0 {
				return vt(12, 4)
			}
			return vt(12, 1)
		}
		dodecadeTypes := [7]uint32{vt(12, 2), vt(12, 0), 255, vt(12, 3), 255, 255, 255}
		n := uint32(0)
		for i := 0; i < 6; i++ {
			if v&synd[i] == synd[i] {
				n++
			}
		}
		return dodecadeTypes[n]
	}
	w1 := w
	if w > 12 {
		w1 = 24 - w
	}
	if w1 <= 5 {
		return vt(w, 0)
	}
	syn := Syndrome(v, 0)
	wSyn := Bw24(syn)
	if wSyn&3 != 0 {
		wSyn += Bw24(syn^v) & 4
	}
	const aBad0, aBad1 = 0xff, 0xff
	aWsyn := [6][3][2]uint32{
		{{2, 0}, {4, 1}, {aBad0, aBad1}}, // hexad
		{{1, 0}, {3, 1}, {aBad0, aBad1}}, // heptad
		{{0, 0}, {4, 1}, {2, 2}},         // octad
		{{1, 0}, {7, 1}, {3, 2}},         // nonad
		{{2, 0}, {6, 1}, {4, 2}},         // decad
		{{3, 0}, {5, 1}, {7, 2}},         // undecad
	}
	pSyn := aWsyn[w1-6]
	for i := 0; i < 3; i++ {
		if pSyn[i][0] == wSyn {
			return vt(w, pSyn[i][1])
		}
	}
	return 255
}

// gcodeToOctad returns the octad number or
// 0xffffffff on failure.
func gcodeToOctad(v1, strict uint32) uint32 {
	y := uint32(mat24OctEncTable[v1&0x7ff])
	if y&0x8000 != 0 || (y^(v1>>11))&1&strict != 0 {
		return 0xffffffff
	}
	return y >> 1
}

// GcodeToOctad returns the octad number of Golay
// code word v. If strict is odd a complemented
// octad is rejected.
//
// GcodeToOctad panics if v is not a (possibly
// complemented) octad, or is complemented when
// strict is odd.
func GcodeToOctad(v uint32, strict uint8) uint32 {
	r := gcodeToOctad(v, uint32(strict))
	if r == 0xffffffff {
		panic("GcodeToOctad: not an octad")
	}
	return r
}

// VectToOctad returns the octad number of vector
// v. If strict is odd a complemented octad is
// rejected.
//
// VectToOctad panics if v is not a (possibly
// complemented) octad, or is complemented when
// strict is odd.
func VectToOctad(v uint32, strict uint8) uint32 {
	gc := Vintern(v)
	if gc&0xfff != 0 {
		panic("VectToOctad: not a Golay code word")
	}
	gc >>= 12
	r := gcodeToOctad(gc, uint32(strict))
	if r == 0xffffffff {
		panic("VectToOctad: not an octad")
	}
	return r
}

// OctadToGcode converts octad number octad to a
// Golay code number.
//
// OctadToGcode panics if octad >= 759.
func OctadToGcode(octad uint32) uint32 {
	if octad >= 759 {
		panic("OctadToGcode: octad out of range")
	}
	return uint32(mat24OctDecTable[octad]) & 0xfff
}

// OctadToVect converts octad number octad to a
// bit vector in GF(2)^24.
//
// OctadToVect panics if octad >= 759.
func OctadToVect(octad uint32) uint32 {
	if octad >= 759 {
		panic("OctadToVect: octad out of range")
	}
	u := uint32(mat24OctDecTable[octad]) & 0xfff
	return mat24DecTable1[(u<<4)&0xf0] ^ mat24DecTable2[(u>>4)&0xff]
}

// CocodeToSuboctad converts cocode element c and
// octad v (in gcode) to (o<<6)+sub, the octad
// number and suboctad number. If strict is set,
// (v,c) must give a short Leech lattice vector.
//
// CocodeToSuboctad panics if v is not an octad
// or c is not an even subset of it.
func CocodeToSuboctad(c, v, strict uint32) uint32 {
	r := cocodeToSuboctad(c, v, strict)
	if r == 0xffffffff {
		panic("CocodeToSuboctad: invalid (cocode, octad) pair")
	}
	return r
}

func cocodeToSuboctad(c1, v1, strict uint32) uint32 {
	var res uint32
	status := c1 >> 11
	octWeight := uint32(mat24OctEncTable[v1&0x7ff])
	status |= octWeight >> 15
	oct := octWeight >> 1
	if status&1 != 0 {
		return 0xffffffff
	}
	pOctad := mat24OctadElementTable[oct<<3:]

	j := uint32(pOctad[0])
	mask := uint32(1) << j
	syn := mask
	t := uint32(mat24SyndromeTable[(c1^mat24RecipBasis[j])&0x7ff])
	syn ^= SynFromTable(t)
	j = uint32(pOctad[7])
	mask |= 1 << j
	csyn := syn ^ (0 - ((syn >> j) & 1))
	j = uint32(pOctad[1])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 0
	j = uint32(pOctad[2])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 1
	j = uint32(pOctad[3])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 2
	j = uint32(pOctad[4])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 3
	j = uint32(pOctad[5])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 4
	j = uint32(pOctad[6])
	mask |= 1 << j
	res ^= ((csyn >> j) & 1) << 5
	status = (octWeight ^ suboctadWeight(res) ^ (v1 >> 11)) & strict & 1
	status |= syn & ^mask
	if status != 0 {
		return 0xffffffff
	}
	return (oct << 6) + res
}

// SuboctadToCocode converts suboctad sub of
// octad to a cocode element.
//
// SuboctadToCocode panics if octad >= 759.
func SuboctadToCocode(sub, octad uint32) uint32 {
	if octad >= 759 {
		panic("SuboctadToCocode: octad out of range")
	}
	pOctad := mat24OctadElementTable[octad<<3:]
	pSub := mat24OctadIndexTable[(sub&0x3f)<<2:]
	c := mat24RecipBasis[pOctad[pSub[0]]] ^
		mat24RecipBasis[pOctad[pSub[1]]] ^
		mat24RecipBasis[pOctad[pSub[2]]] ^
		mat24RecipBasis[pOctad[pSub[3]]]
	return c & 0xfff
}

func suboctadWeight(sub uint32) uint32 {
	return uint32((mat24SuboctadWeights >> (sub & 0x3f)) & 1)
}

// SuboctadWeight returns the parity of the
// halved bit weight of suboctad sub.
func SuboctadWeight(sub uint32) uint32 {
	return suboctadWeight(sub)
}

// SuboctadScalarProd returns the scalar product
// of suboctads sub1 and sub2.
func SuboctadScalarProd(sub1, sub2 uint32) uint32 {
	wp := (uint32(0x96) >> ((sub1 ^ (sub1 >> 3)) & 7)) & (uint32(0x96) >> ((sub2 ^ (sub2 >> 3)) & 7))
	sub1 &= sub2
	wp ^= uint32(0x96) >> ((sub1 ^ (sub1 >> 3)) & 7)
	return wp & 1
}

// ScalarProd returns the scalar product of Golay
// code vector v (gcode) and cocode vector c.
func ScalarProd(v, c uint32) uint32 {
	v &= c
	return Parity12(v)
}

// IntersectOctadTetrad returns an octad
// containing v2 and meeting the octad of v1 in a
// tetrad, as a vector, or 0 if none exists.
//
// IntersectOctadTetrad panics if v1 is not a
// (possibly complemented) octad.
func IntersectOctadTetrad(v1, v2 uint32) uint32 {
	var o uint32
	{
		gc := Vintern(v1)
		if gc&0xfff != 0 {
			panic("IntersectOctadTetrad: v1 is not an octad")
		}
		gc >>= 12
		y := uint32(mat24OctEncTable[gc&0x7ff])
		if y&0x8000 != 0 {
			panic("IntersectOctadTetrad: v1 is not an octad")
		}
		v1 ^= 0 - ((y ^ (gc >> 11)) & 1)
		o = v1 & 0xffffff
	}
	v2 &= 0xffffff
	sub := v2 & o
	w := Bw24(sub)
	wUp := Bw24(v2 & ^o)
	if w > 4 {
		return 0
	}
	if w == 0 || (w == 2 && wUp <= 2) {
		b := o & ^sub
		sub |= b & (0 - b)
		w++
	}
	var up uint32
	wUp = 0
	pool := v2 & ^o
	for w+wUp < 4 && pool != 0 {
		b := pool & (0 - pool)
		up |= b
		pool &= ^b
		wUp++
	}
	pool = ^o & ^up & 0xffffff
	for w+wUp < 4 && pool != 0 {
		b := pool & (0 - pool)
		up |= b
		pool &= ^b
		wUp++
	}
	up |= sub
	syndromes := CocodeAllSyndromes(Vintern(up))
	for _, s := range syndromes {
		res := up ^ s
		sub = res & o
		if res&v2 == v2 && Bw24(sub) == 4 {
			return res
		}
	}
	return 0
}

// CocodeAsSubdodecad returns a bit vector
// equivalent to cocode element c that is a
// subset of dodecad v (gcode). If single < 24
// that bit may be set to absorb an odd scalar
// product.
//
// CocodeAsSubdodecad panics if v is not a
// dodecad or the representation fails.
func CocodeAsSubdodecad(c, v, single uint32) uint32 {
	r := cocodeAsSubdodecad(c, v, single)
	if r == 0xffffffff {
		panic("CocodeAsSubdodecad: not representable as subdodecad")
	}
	return r
}

func cocodeAsSubdodecad(c1, v1, uSingle uint32) uint32 {
	pos := [20]uint8{0, 1, 0, 2, 0, 3, 0, 4, 1, 2, 1, 3, 1, 4, 2, 3, 2, 4, 3, 4}
	var single uint32

	if mat24ThetaTable[v1&0x7ff]&0x1000 == 0 {
		return 0xffffffff
	}
	vect1 := GcodeToVectInternal(v1)

	if ScalarProd(v1^0x800, c1) != 0 {
		single = 1 << uSingle
		if uSingle >= 24 || single&vect1 != 0 {
			return 0xffffffff
		}
		c1 ^= mat24RecipBasis[uSingle] & 0xfff
	}

	lsb := lsbit24(vect1)
	syn := CocodeSyndromeRaw(c1, lsb)
	if syn&0xff000000 != 0 {
		return 0xffffffff
	}
	res := syn & vect1
	syn = syn & ^vect1

	if syn != 0 {
		var b [24]uint8
		var coc [5]uint32
		var c uint32
		bw, blist := VectToBitList(vect1)
		_ = bw
		copy(b[:], blist)
		u0 := VectToCocode(syn) & 0x7ff
		for i := 0; i <= 4; i++ {
			coc[i] = mat24RecipBasis[b[i]&0x1f] & 0x7ff
			u0 ^= coc[i]
			c ^= 1 << b[i]
		}
		for i := 0; i < 20; i += 2 {
			p0 := pos[i]
			p1 := pos[i+1]
			u1 := u0 ^ coc[p0] ^ coc[p1]
			tab := uint32(mat24SyndromeTable[u1])
			syn1 := SynFromTable(tab)
			if syn1&vect1 == syn1 {
				c6 := c ^ syn1 ^ (1 << b[p0]) ^ (1 << b[p1])
				res ^= c6
				goto done
			}
		}
		return 0xffffffff
	}
done:
	w := Bw24(res)
	if w > 6 || (w == 6 && res&(1<<lsb) == 0) {
		res ^= vect1
	}
	return res ^ single
}

// PloopTheta returns the Parker loop theta
// function of v as a cocode element.
func PloopTheta(v uint32) uint32 {
	return uint32(mat24ThetaTable[v&0x7ff]) & 0xfff
}

// PloopCocycle returns the Parker loop cocycle
// of v1 and v2.
func PloopCocycle(v1, v2 uint32) uint32 {
	s := uint32(mat24ThetaTable[v1&0x7ff]) & v2 & 0xfff
	return Parity12(s)
}

// MulPloop returns the Parker loop product of
// v1 and v2.
func MulPloop(v1, v2 uint32) uint32 {
	return v1 ^ v2 ^ (PloopCocycle(v1, v2) << 12)
}

// PowPloop returns the Parker loop power v^exp.
func PowPloop(v, exp uint32) uint32 {
	return (v & (0 - (exp & 1))) ^
		(uint32(mat24ThetaTable[v&0x7ff]) & ((exp & 2) << 11))
}

// PloopComm returns the Parker loop commutator
// bit of v1 and v2.
func PloopComm(v1, v2 uint32) uint32 {
	r := (uint32(mat24ThetaTable[v1&0x7ff]) & v2) ^
		(uint32(mat24ThetaTable[v2&0x7ff]) & v1)
	return Parity12(r)
}

// PloopCap returns the intersection of Golay
// code words v1 and v2 as a cocode word.
func PloopCap(v1, v2 uint32) uint32 {
	v1 &= 0x7ff
	v2 &= 0x7ff
	return (uint32(mat24ThetaTable[v1]) ^ uint32(mat24ThetaTable[v2]) ^
		uint32(mat24ThetaTable[v1^v2])) & 0xfff
}

// PloopAssoc returns the Parker loop associator
// bit of v1, v2, and v3.
func PloopAssoc(v1, v2, v3 uint32) uint32 {
	r := (uint32(mat24ThetaTable[v1&0x7ff]) & v3) ^
		(uint32(mat24ThetaTable[v2&0x7ff]) & v3) ^
		(uint32(mat24ThetaTable[(v1^v2)&0x7ff]) & v3)
	return Parity12(r)
}

// PloopSolve returns a cocode element (low 12
// bits) that makes every Parker loop element in
// a positive, and the rank k in bits 16..31.
// The slice a is modified in place.
//
// PloopSolve sets bit 12 of the result if no
// such cocode element exists.
func PloopSolve(a []uint32) uint32 {
	uLen := uint32(len(a))
	var pivCol [13]uint32
	var nrows uint32
	for col := uint32(0); col <= 12; col++ {
		mask := uint32(1) << col
		for row := nrows; row < uLen; row++ {
			if a[row]&mask != 0 {
				piv := a[row]
				a[row] = a[nrows]
				for r := uint32(0); r < uLen; r++ {
					a[r] ^= piv & (0 - ((a[r] >> col) & 1))
				}
				a[nrows] = piv
				pivCol[nrows] = col
				nrows++
				break
			}
		}
	}
	var res uint32
	for row := uint32(0); row < nrows; row++ {
		res |= ((a[row] >> 12) & 1) << pivCol[row]
	}
	return res + (nrows << 16)
}

// permCompleteHeptad completes p[0,1,2,3,4,5,8]
// to a full M24 permutation in place. It returns
// a nonzero value if the heptad is infeasible.
func permCompleteHeptad(p []byte) uint32 {
	err := (uint32(p[0]) + 8) | (uint32(p[1]) + 8) | (uint32(p[2]) + 8) |
		(uint32(p[3]) + 8) | (uint32(p[4]) + 8) | (uint32(p[5]) + 8) | (uint32(p[8]) + 8)
	err &= 0xffffffe0
	s1 := uint32(1) << p[1]
	s5 := uint32(1) << p[5]
	s015 := (uint32(1) << p[0]) ^ s1 ^ s5
	s3 := uint32(1) << p[3]
	s4 := uint32(1) << p[4]
	s8 := uint32(1) << p[8]
	s01234 := s015 ^ s5 ^ (uint32(1) << p[2]) ^ s3 ^ s4
	s567 := oddSyn(s01234)
	err |= s01234 & s567
	err |= (s01234 | s567) & s8
	err |= s5 ^ (s5 & s567)
	s67 := s567 & ^s5
	s9AB := oddSyn(s01234 ^ s4 ^ s8)
	s9CD := oddSyn(s015 ^ s4 ^ s8)
	s9 := s9AB & s9CD
	p[9] = byte(lsbit24pwr2(s9))
	s6GH := oddSyn(s1 ^ s3 ^ s5 ^ s8 ^ s9)
	s6 := s67 & s6GH
	p[6] = byte(lsbit24pwr2(s6))
	p[7] = byte(lsbit24pwr2(s67 & ^s6GH))
	sACE := oddSyn(s01234 ^ s1 ^ s3 ^ s6 ^ s8)
	p[10] = byte(lsbit24pwr2(s9AB & sACE))
	p[11] = byte(lsbit24pwr2(s9AB & ^sACE & ^s9))
	p[12] = byte(lsbit24pwr2(s9CD & sACE))
	sD := s9CD & ^sACE & ^s9
	p[13] = byte(lsbit24pwr2(sD))
	p[14] = byte(lsbit24pwr2(sACE & ^s9AB & ^s9CD))
	sFGI := oddSyn(s015 ^ s6 ^ sD)
	sG := s6GH & sFGI
	p[16] = byte(lsbit24pwr2(sG))
	p[17] = byte(lsbit24pwr2(s6GH & ^s6 & ^sFGI))
	sFJK := oddSyn(s015 ^ s3 ^ s8)
	p[15] = byte(lsbit24pwr2(sFGI & sFJK))
	p[18] = byte(lsbit24pwr2(sFGI & ^sG & ^sFJK))
	sJLM := oddSyn(s015 ^ s1 ^ s3 ^ s6 ^ sG)
	p[19] = byte(lsbit24pwr2(sFJK & sJLM))
	p[20] = byte(lsbit24pwr2(sFJK & ^sFGI & ^sJLM))
	sALN := oddSyn(s015 ^ s6 ^ s8)
	p[21] = byte(lsbit24pwr2(sALN & sJLM))
	p[22] = byte(lsbit24pwr2(sJLM & ^sALN & ^sFJK))
	p[23] = byte(lsbit24pwr2(sALN & ^sACE & ^sJLM))
	return err
}

// PermCompleteHeptad completes the images at
// indices 0,1,2,3,4,5,8 of p to a full M24
// permutation. The input p must have length 24.
//
// PermCompleteHeptad returns an error if the
// heptad is infeasible.
func PermCompleteHeptad(p []byte) ([]byte, error) {
	out := make([]byte, 24)
	copy(out, p)
	if permCompleteHeptad(out) != 0 {
		return nil, fmt.Errorf("PermCompleteHeptad: infeasible heptad")
	}
	return out, nil
}

// permCheck returns 0 if p is in M24.
func permCheck(p []byte) uint32 {
	var p2 [24]byte
	copy(p2[:9], p[:9])
	if permCompleteHeptad(p2[:]) != 0 {
		return 1
	}
	for i := 0; i < 24; i++ {
		if p2[i] != p[i] {
			return 1
		}
	}
	return 0
}

// PermCheck reports whether the mapping i ->
// p[i] is a permutation in M24.
//
// PermCheck returns an error if p is not in M24.
func PermCheck(p []byte) error {
	if permCheck(p) != 0 {
		return fmt.Errorf("PermCheck: permutation not in M24")
	}
	return nil
}

// permCompleteOctad completes p[0..5] to an
// octad p[0..7]. It returns 0xffffffff on
// failure.
func permCompleteOctad(p []byte) uint32 {
	err := (uint32(p[0]) + 8) | (uint32(p[1]) + 8) | (uint32(p[2]) + 8) |
		(uint32(p[3]) + 8) | (uint32(p[4]) + 8) | (uint32(p[5]) + 8)
	err &= 0xffffffe0
	s1 := uint32(1) << p[1]
	s5 := uint32(1) << p[5]
	s015 := (uint32(1) << p[0]) ^ s1 ^ s5
	s3 := uint32(1) << p[3]
	s4 := uint32(1) << p[4]
	s01234 := s015 ^ s5 ^ (uint32(1) << p[2]) ^ s3 ^ s4
	s567 := oddSyn(s01234)
	s8 := 0xffffff ^ (s01234 | s567)
	s8 = s8 & (0 - s8)
	err |= s01234 & s567
	err |= s5 ^ (s5 & s567)
	s67 := s567 & ^s5
	s9AB := oddSyn(s01234 ^ s4 ^ s8)
	s9CD := oddSyn(s015 ^ s4 ^ s8)
	s9 := s9AB & s9CD
	s6GH := oddSyn(s1 ^ s3 ^ s5 ^ s8 ^ s9)
	s6 := s67 & s6GH
	p[6] = byte(lsbit24pwr2(s6))
	p[7] = byte(lsbit24pwr2(s67 & ^s6GH))
	if err != 0 {
		return 0xffffffff
	}
	return 0
}

// PermCompleteOctad completes the images at
// indices 0..5 of p to an octad p[0..7]. The
// input p must have length at least 6.
//
// PermCompleteOctad returns an error if the six
// points are not a subset of an octad.
func PermCompleteOctad(p []byte) ([]byte, error) {
	out := make([]byte, 24)
	copy(out, p)
	if permCompleteOctad(out) == 0xffffffff {
		return nil, fmt.Errorf("PermCompleteOctad: points not in an octad")
	}
	return out, nil
}

// permFromHeptads completes the mapping h1 -> h2
// of umbral heptads to a full M24 permutation in
// p. It returns 0xffffffff on failure.
func permFromHeptads(h1, h2, p []byte) uint32 {
	var p1, p2 [24]byte
	var v, y uint32

	v = 0
	for i := 0; i < 7; i++ {
		v |= 1 << (h1[i] & 31)
	}
	y = Vintern(v)
	y = uint32(mat24SyndromeTable[y&0x7ff])
	y = SynFromTable(y)
	v &= y
	v = lsbit24(v)
	y = 0
	for i := uint32(0); i < 7; i++ {
		if uint32(h1[i]) == v {
			y |= i
		}
	}
	copy(p1[:7], h1[:7])
	copy(p2[:7], h2[:7])
	p1[8] = p1[y]
	p1[y] = p1[6]
	p2[8] = p2[y]
	p2[y] = p2[6]
	if permCompleteHeptad(p1[:])|permCompleteHeptad(p2[:]) != 0 {
		return 0xffffffff
	}
	for i := 0; i < 24; i++ {
		p[p1[i]] = p2[i]
	}
	return 0
}

// PermFromHeptads completes the mapping h1 -> h2
// of umbral heptads to a permutation in M24.
//
// PermFromHeptads panics if h1, h2 are not
// feasible umbral heptads.
func PermFromHeptads(h1, h2 []byte) []byte {
	out := make([]byte, 24)
	if permFromHeptads(h1, h2, out) == 0xffffffff {
		panic("PermFromHeptads: infeasible heptad mapping")
	}
	return out
}

// insertsortI8 sorts a1 in place and applies the
// same permutation to a2.
func insertsortI8(a1, a2 []byte, n int) {
	for i := 1; i < n; i++ {
		temp1, temp2 := a1[i], a2[i]
		j := i
		for ; j >= 1 && a1[j-1] > temp1; j-- {
			a1[j] = a1[j-1]
			a2[j] = a2[j-1]
		}
		a1[j] = temp1
		a2[j] = temp2
	}
}

// extendUmbralHexad appends one entry to h1 and
// h2 so they become matching umbral heptads.
func extendUmbralHexad(h1, h2 []byte, bm1, bm2 uint32) {
	src := lsbit24(^bm1)
	h1[6] = byte(src)
	bm1 |= 1 << src
	bm1 &= mat24Syndrome(bm1, 0)
	var img uint32 = 24
	for j := 0; j < 6; j++ {
		if uint32(1)<<h1[j] == bm1 {
			img = uint32(h2[j])
		}
	}
	bm2 &= ^(uint32(1) << img)
	bm2 = mat24Syndrome(bm2, 0)
	h2[6] = byte(lsbit24(bm2))
}

// mat24Syndrome is the internal syndrome helper
// returning 0xffffffff on failure (no panic).
func mat24Syndrome(v, tetrad uint32) uint32 {
	return CocodeSyndromeRaw(Vintern(v), tetrad)
}

// PermFromMap returns the completion type (1, 2,
// or 3) and the M24 permutation extending the
// mapping h1[i] -> h2[i].
//
// PermFromMap returns an error if the mapping is
// illegal (duplicate or out-of-range entries) or
// no M24 permutation completes it.
func PermFromMap(h1, h2 []byte) (int, []byte, error) {
	n := uint32(len(h1))
	pOut := make([]byte, 24)
	var p1, p2 [32]byte
	var bm1, bm2 uint32

	err := 0 - boolToU32(n > 24)
	if err == 0 {
		for i := uint32(0); i < n; i++ {
			err |= (uint32(h1[i]) + 8) | (uint32(h2[i]) + 8)
			bm1 |= 1 << h1[i]
			bm2 |= 1 << h2[i]
		}
	}
	if err&0xffffffe0 != 0 || Bw24(bm1) != n || Bw24(bm2) != n {
		return 0, nil, fmt.Errorf("PermFromMap: illegal mapping")
	}

	if n < 5 {
		_, b1 := VectToBitList(bm1)
		_, b2 := VectToBitList(bm2)
		copy(p1[:24], b1)
		copy(p2[:24], b2)
	}
	for i := uint32(0); i < n; i++ {
		p1[i] = h1[i]
		p2[i] = h2[i]
	}
	if n < 9 {
		insertsortI8(p1[:], p2[:], int(n))
	}

	bm1, bm2 = 0, 0
	for i := 0; i < 5; i++ {
		bm1 |= 1 << p1[i]
	}
	for i := 0; i < 5; i++ {
		bm2 |= 1 << p2[i]
	}
	o1 := bm1 | mat24Syndrome(bm1, 0)
	o2 := bm2 | mat24Syndrome(bm2, 0)

	i8 := uint32(24)
	i16 := uint32(25)
	for i := uint32(5); i < n; i++ {
		if (uint32(1)<<p1[i])&o1 != 0 {
			if i8 == 24 {
				i8 = i
			}
		} else {
			if i16 == 25 {
				i16 = i
			}
		}
	}

	var res int
	if i16 == 25 {
		res = 3
	} else {
		res = 1
		if i8 == 24 && n < 7 {
			res = 2
		}
	}

	if i8 == 24 {
		if n == 6 {
			bm1 |= 1 << p1[5]
			bm2 |= 1 << p2[5]
			extendUmbralHexad(p1[:], p2[:], bm1, bm2)
			i8 = 5
			i16 = 6
		} else if n < 6 {
			bm1 ^= o1
			bm2 ^= o2
			p1[24] = byte(lsbit24(bm1))
			p2[24] = byte(lsbit24(bm2))
		} else {
			i8 = 5
			i16 = 6
		}
	}

	if i16 == 25 {
		o1 = ^o1
		o2 = ^o2
		p1[25] = byte(lsbit24(o1))
		p2[25] = byte(lsbit24(o2))
	}
	t := p1[i16]
	p1[5] = p1[i8]
	p1[6] = t
	t = p2[i16]
	p2[5] = p2[i8]
	p2[6] = t

	if permFromHeptads(p1[:], p2[:], pOut) == 0xffffffff {
		return 0, nil, errNoM24Perm
	}

	var checkErr uint32
	for i := uint32(0); i < n; i++ {
		checkErr |= uint32(pOut[h1[i]]) ^ uint32(h2[i])
	}
	if checkErr != 0 {
		// C returns (res=0, stale pOut) here. The Cython
		// wrapper fails to detect res=0 as failure (dead
		// uint32 < 0 check) and returns (0, identity).
		// We return the identity too for C compatibility,
		// but signal the failure via errNoM24Perm so the
		// caller can distinguish this from a hard error.
		for i := range pOut {
			pOut[i] = byte(i)
		}
		return 0, pOut, errNoM24Perm
	}
	return res, pOut, nil
}

func boolToU32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

const (
	m24Sh      = 58
	m24Factor  = 24 * ((1<<m24Sh)/Mat24Order + 1)
	m24DField1 = 0x555555555555
)

// m24numToPerm computes the M24 permutation with
// number m24 into pOut and returns 0xffffffff if
// m24 is out of range.
func m24numToPerm(m24 uint32, pOut []byte) uint32 {
	var p1 [32]byte
	res := 0 - boolToU32(m24 >= Mat24Order)
	m24 &= ^res

	n1 := uint64(m24Factor) * uint64(m24)
	k := uint32(n1 >> m24Sh)
	pOut[0] = byte(k)
	n1 -= uint64(k) << m24Sh
	bitmap := uint32(1) << k
	d := uint64(m24DField1) << (2 * k)

	var last, j uint32
	steps := [3]uint64{23, 22, 21}
	for si, f := range steps {
		n1 = f * n1
		k = uint32(n1 >> m24Sh)
		n1 &= (1 << m24Sh) - 1
		j = k + uint32((d>>(2*k))&3)
		pOut[24-int(f)] = byte(j)
		mask := (uint64(1) << (2 * k)) - 1
		if si < 2 {
			d = (((d + m24DField1) >> 2) & ^mask) + (d & mask)
		} else {
			d = ((d >> 2) & ^mask) + (d & mask)
			last = k
		}
		bitmap |= 1 << j
	}

	n1 = 20 * n1
	k = uint32(n1 >> m24Sh)
	n1 &= (1 << m24Sh) - 1
	j = k + uint32((d>>(2*k))&3) + boolToU32(k >= last)
	pOut[4] = byte(j)
	bitmap |= 1 << pOut[4]

	cocode := Vintern(bitmap)
	syn := uint32(mat24SyndromeTable[cocode&0x7ff])

	k = uint32((3 * n1) >> m24Sh)
	j = (syn >> (5 * k)) & 31
	pOut[5] = byte(j)

	bitmap |= SynFromTable(syn)
	bitmap ^= 0xffffff

	j = 0
	for i := uint32(0); i < 24; i++ {
		p1[j] = byte(i)
		j += (bitmap >> i) & 1
	}

	j = uint32(p1[m24&15])
	pOut[8] = byte(j)

	permCompleteHeptad(pOut)
	return res
}

// M24numToPerm returns the M24 permutation with
// number num as a 24-byte mapping.
//
// M24numToPerm panics if num >= Mat24Order.
func M24numToPerm(num uint32) []byte {
	if num >= Mat24Order {
		panic("M24numToPerm: number out of range")
	}
	out := make([]byte, 24)
	m24numToPerm(num, out)
	return out
}

// m24numToPermSafe is like M24numToPerm but, instead
// of panicking, substitutes the identity permutation
// (number 0) when num is out of range. This matches
// the C macro mat24_m24num_to_perm, which masks an
// illegal permutation number to 0; the C word
// operations apply it while ignoring the error flag,
// so an out-of-range permutation atom acts as the
// identity.
func m24numToPermSafe(num uint32) []byte {
	out := make([]byte, 24)
	m24numToPerm(num, out)
	return out
}

// PermToM24num returns the number of permutation
// p in M24. The result is garbage if p is not in
// M24.
func PermToM24num(p []byte) uint32 {
	var d uint64
	var n, k, bitmap, last, syn, syn1 uint32

	n = uint32(p[0])
	k = n
	bitmap = 1 << k
	d = uint64(m24DField1) << (2 * k)

	steps := [3]uint32{23, 22, 21}
	for si, f := range steps {
		k = uint32(p[24-int(f)])
		n = f*n + k - uint32((d>>(2*k))&3)
		bitmap |= 1 << k
		if si < 2 {
			d += uint64(m24DField1) << (2 * k)
		} else {
			last = k
		}
	}

	k = uint32(p[4])
	bitmap |= 1 << k
	n = 20*n + k - uint32((d>>(2*k))&3) - boolToU32(k >= last)

	cocode := Vintern(bitmap)
	syn = uint32(mat24SyndromeTable[cocode&0x7ff])

	syn1 = (syn >> 5) & 31
	k = boolToU32(uint32(p[5]) > syn1) + boolToU32(uint32(p[5]) >= syn1)
	n = 3*n + k

	bitmap |= (1 << (syn & 31)) | (1 << syn1) | (1 << ((syn >> 10) & 31))

	k = ((1 << p[8]) - 1) & bitmap
	k = Bw24(k)

	n = 16*n + uint32(p[8]) - k
	return n & (boolToU32(n >= Mat24Order) - 1)
}

// permToMatrix converts permutation p1 to a
// 12x12 bit matrix in m_out.
func permToMatrix(p1 []byte, mOut []uint32) {
	var a [24]uint32
	for i := 0; i < 24; i++ {
		a[i] = mat24RecipBasis[p1[i]&0x1f] >> 12
	}
	var r [2]uint32
	mOut[9] = a[3] ^ a[1]
	mOut[8] = a[3] ^ a[2]
	mOut[10] = mOut[8] ^ a[1]
	mOut[10] = a[4] ^ mOut[10]
	mOut[10] = a[8] ^ mOut[10]
	mOut[10] = a[12] ^ mOut[10]
	mOut[10] = a[16] ^ mOut[10]
	mOut[10] = a[20] ^ mOut[10]
	mOut[7] = a[7] ^ a[5]
	mOut[0] = a[5] ^ a[4]
	mOut[6] = a[7] ^ a[6]
	mOut[0] = mOut[6] ^ mOut[0]
	mOut[1] = a[9] ^ a[8]
	mOut[5] = a[11] ^ a[9]
	mOut[4] = a[11] ^ a[10]
	mOut[1] = mOut[4] ^ mOut[1]
	mOut[1] = mOut[1] ^ mOut[0]
	mOut[2] = a[13] ^ a[12]
	r[0] = a[15] ^ a[13]
	mOut[5] = r[0] ^ mOut[5]
	mOut[9] = r[0] ^ mOut[9]
	mOut[7] = r[0] ^ mOut[7]
	r[0] = a[15] ^ a[14]
	mOut[2] = r[0] ^ mOut[2]
	mOut[4] = r[0] ^ mOut[4]
	mOut[8] = r[0] ^ mOut[8]
	mOut[6] = r[0] ^ mOut[6]
	mOut[0] = mOut[2] ^ mOut[0]
	mOut[2] = mOut[2] ^ mOut[1]
	r[0] = a[18] ^ a[17]
	mOut[8] = r[0] ^ mOut[8]
	mOut[7] = r[0] ^ mOut[7]
	r[0] = a[19] ^ a[17]
	mOut[6] = r[0] ^ mOut[6]
	mOut[5] = r[0] ^ mOut[5]
	r[0] = a[19] ^ a[18]
	mOut[9] = r[0] ^ mOut[9]
	mOut[4] = r[0] ^ mOut[4]
	mOut[3] = a[17] ^ a[16]
	mOut[3] = r[0] ^ mOut[3]
	mOut[0] = mOut[3] ^ mOut[0]
	mOut[1] = mOut[3] ^ mOut[1]
	mOut[3] = mOut[3] ^ mOut[2]
	r[0] = a[23] ^ a[22]
	mOut[7] = r[0] ^ mOut[7]
	mOut[4] = r[0] ^ mOut[4]
	r[1] = a[22] ^ a[21]
	mOut[9] = r[1] ^ mOut[9]
	mOut[6] = r[1] ^ mOut[6]
	r[1] = a[23] ^ a[21]
	mOut[8] = r[1] ^ mOut[8]
	mOut[5] = r[1] ^ mOut[5]
	r[1] = a[21] ^ a[20]
	r[1] = r[0] ^ r[1]
	mOut[0] = r[1] ^ mOut[0]
	mOut[1] = r[1] ^ mOut[1]
	mOut[2] = r[1] ^ mOut[2]
	mOut[11] = 0x800
}

// PermToMatrix converts permutation p to the
// 12x12 bit matrix acting on Golay code words by
// right multiplication.
func PermToMatrix(p []byte) []uint32 {
	out := make([]uint32, 12)
	permToMatrix(p, out)
	return out
}

// matrixToPerm converts bit matrix m1 to a
// permutation in pOut.
func matrixToPerm(m1 []uint32, pOut []byte) {
	var ba [14]uint32
	var t [11]uint32
	for i := 0; i < 12; i++ {
		ba[i] = mat24DecTable1[(m1[i]<<4)&0xf0] ^ mat24DecTable2[(m1[i]>>4)&0xff]
	}
	ba[12] = ba[4] ^ ba[7] ^ ba[9]
	ba[13] = ba[4] ^ ba[5] ^ ba[6] ^ ba[8]
	t[0] = ba[10]
	t[1] = ^ba[10]
	t[2] = ba[12] & ba[13]
	t[3] = ba[12] & ^ba[13]
	t[4] = ba[13] & ^ba[12]
	t[5] = ba[11] & ^(ba[0] | ba[1])
	t[6] = ba[0] & ba[1] & ba[2] & ba[3]
	t[7] = ba[1] & ^ba[0]
	t[8] = ba[2] & ^ba[1]
	t[9] = ba[3] & ^ba[2]
	t[10] = ba[0] & ^ba[3]
	idx := 0
	sel0 := [6]uint32{0x1, 0, 0, 0, 0, 0}
	const sel1 = 0x224433
	const sel2 = 0x332244
	const sel3 = 0x443322
	for i := uint32(0); i < 6; i++ {
		kk := t[i+5]
		w := t[(sel0[i]>>(i<<2))&0xf] & kk
		pOut[idx] = byte(lsbit24(w))
		idx++
		w = t[(uint32(sel1)>>(i<<2))&0xf] & kk
		pOut[idx] = byte(lsbit24(w))
		idx++
		w = t[(uint32(sel2)>>(i<<2))&0xf] & kk
		pOut[idx] = byte(lsbit24(w))
		idx++
		w = t[(uint32(sel3)>>(i<<2))&0xf] & kk
		pOut[idx] = byte(lsbit24(w))
		idx++
	}
}

// MatrixToPerm converts the bit matrix m to a
// permutation in M24.
func MatrixToPerm(m []uint32) []byte {
	out := make([]byte, 24)
	matrixToPerm(m, out)
	return out
}

// matrixFromModOmega completes an 11x11 bit
// matrix to a 12x12 matrix in place.
func matrixFromModOmega(m1 []uint32) {
	weights := uint32(0xff0) << 11
	m1[11] &= ^uint32(0xfff)
	for i := 0; i < 12; i++ {
		m1[i] ^= ((uint32(mat24ThetaTable[m1[i]&0x7ff]) >> 2) ^ (weights >> i)) & 0x800
	}
}

// MatrixFromModOmega completes the matrix m,
// known modulo Omega, in place.
func MatrixFromModOmega(m []uint32) {
	matrixFromModOmega(m)
}

// dodecadToHeptad computes a unique umbral
// heptad hOut from dodecad d1. It returns
// 0xffffffff on failure.
func dodecadToHeptad(d1, hOut []byte) uint32 {
	var a [8]byte
	var sextet [24]byte
	s5 := (uint32(1) << d1[0]) ^ (uint32(1) << d1[1]) ^ (uint32(1) << d1[2]) ^
		(uint32(1) << d1[3]) ^ (uint32(1) << d1[4])
	syn := oddSyn(s5)
	rem := s5 ^ (uint32(1) << d1[5]) ^ (uint32(1) << d1[6]) ^
		(uint32(1) << d1[7]) ^ (uint32(1) << d1[8])
	t := oddSyn(rem)
	all12 := t ^ rem
	if Bw24(all12) != 12 {
		return 0xffffffff
	}
	hOut[0], a[0] = d1[0], d1[0]
	hOut[1], a[1] = d1[1], d1[1]
	hOut[2], a[2] = d1[2], d1[2]
	hOut[3], a[3] = d1[3], d1[3]
	hOut[4], a[4] = d1[4], d1[4]
	t = syn & all12
	hOut[5] = byte(lsbit24(t))
	a[5] = hOut[5]
	if permCompleteOctad(a[:]) == 0xffffffff {
		return 0xffffffff
	}
	t = (uint32(1) << a[0]) ^ (uint32(1) << a[1]) ^ (uint32(1) << a[2]) ^ (uint32(1) << a[6])
	t = Vintern(t)
	if cocodeToSextet(t, sextet[:]) == 0xffffffff {
		return 0xffffffff
	}
	rem = ^(all12 | syn)
	for i := 0; i < 24; i += 4 {
		t = ((uint32(1) << sextet[i]) ^ (uint32(1) << sextet[i+1]) ^
			(uint32(1) << sextet[i+2]) ^ (uint32(1) << sextet[i+3])) & rem
		if t != 0 && t&(t-1) == 0 {
			hOut[6] = byte(lsbit24pwr2(t))
			return 0
		}
	}
	return 0xffffffff
}

// cocodeToSextet is the no-panic internal
// version of CocodeToSextet.
func cocodeToSextet(c1 uint32, out []byte) uint32 {
	if c1&0x800 != 0 {
		return 0xffffffff
	}
	c2 := c1 ^ mat24RecipBasis[0]
	syn := uint32(mat24SyndromeTable[c2&0x7ff])
	if syn&31 == 0 {
		return 0xffffffff
	}
	out[0] = 0
	out[1] = byte(syn & 31)
	out[2] = byte((syn >> 5) & 31)
	out[3] = byte((syn >> 10) & 31)
	v := uint32(0xffffff)
	for k := uint32(4); k < 24; k += 4 {
		v ^= (1 << out[k-4]) ^ (1 << out[k-3]) ^ (1 << out[k-2]) ^ (1 << out[k-1])
		out[k] = byte(lsbit24(v))
		c2 = c1 ^ mat24RecipBasis[out[k]]
		syn = uint32(mat24SyndromeTable[c2&0x7ff])
		out[k+1] = byte(syn & 31)
		out[k+2] = byte((syn >> 5) & 31)
		out[k+3] = byte((syn >> 10) & 31)
	}
	return 0
}

// PermFromDodecads returns the unique M24
// permutation mapping dodecad d1 to dodecad d2
// (fixing the first five points each).
//
// PermFromDodecads panics if the inputs are not
// valid dodecad subsets.
func PermFromDodecads(d1, d2 []byte) []byte {
	out := make([]byte, 24)
	var h1, h2 [8]byte
	if dodecadToHeptad(d1, h1[:]) == 0xffffffff ||
		dodecadToHeptad(d2, h2[:]) == 0xffffffff ||
		permFromHeptads(h1[:], h2[:], out) == 0xffffffff {
		panic("PermFromDodecads: invalid dodecad mapping")
	}
	return out
}

// OpVectPerm applies permutation p to bit vector
// v, mapping bit i to bit p[i].
func OpVectPerm(v uint32, p []byte) uint32 {
	var w uint32
	for i := uint32(0); i < 24; i++ {
		w |= ((v >> i) & 1) << p[i]
	}
	return w
}

// OpGcodeMatrix returns the product v*m of Golay
// code word v (gcode) and bit matrix m.
func OpGcodeMatrix(v uint32, m []uint32) uint32 {
	var w uint32
	for i := uint32(0); i < 12; i++ {
		w ^= m[i] & (0 - ((v >> i) & 1))
	}
	return w
}

// OpGcodePerm applies permutation p to Golay
// code word v (gcode) and returns the result in
// gcode representation.
func OpGcodePerm(v uint32, p []byte) uint32 {
	v = GcodeToVectInternal(v)
	var w uint32
	for i := uint32(0); i < 24; i++ {
		w |= ((v >> i) & 1) << p[i]
	}
	v = Vintern(w)
	return v >> 12
}

// OpCocodePerm applies permutation p to cocode
// element c and returns the result in cocode
// representation. p is padded to 32 internally;
// see bytePerm (ploop.go) for why index 24 is
// reachable from the syndrome table.
func OpCocodePerm(c uint32, p []byte) uint32 {
	var pp [32]byte
	copy(pp[:], p)
	res := 0 - (((c >> 11) + 1) & 1)
	c ^= mat24RecipBasis[0] & res
	res &= mat24RecipBasis[pp[0]&31]
	c = uint32(mat24SyndromeTable[c&0x7ff])
	res ^= mat24RecipBasis[pp[c&31]&31] ^
		mat24RecipBasis[pp[(c>>5)&31]&31] ^
		mat24RecipBasis[pp[(c>>10)&31]&31]
	return res & 0xfff
}

// MulPerm returns the product p1*p2, i.e. the
// mapping i -> p2[p1[i]].
func MulPerm(p1, p2 []byte) []byte {
	out := make([]byte, 24)
	for i := 0; i < 24; i++ {
		out[i] = p2[p1[i]&31]
	}
	return out
}

// InvPerm returns the inverse of permutation p.
func InvPerm(p []byte) []byte {
	out := make([]byte, 24)
	for i := 0; i < 24; i++ {
		out[p[i]&31] = byte(i)
	}
	return out
}

// bitvmultransp computes v_i = v_i *
// transpose(m_i) for i = 0..4, packed 12 bits
// per slot. nrows is the number of m2 rows used.
func bitvmultransp(v uint64, m2 []uint64, nrows int) uint64 {
	if nrows == 5 {
		var t0, t1, t2 uint64
		t0 = m2[0] & v
		t0 = (t0 ^ (t0 >> 1)) & 0x555555555555555
		t1 = m2[1] & v
		t1 = ((t1 << 1) ^ t1) & 0xaaaaaaaaaaaaaaa
		t0 |= t1
		t0 = (t0 ^ (t0 >> 2)) & 0x333333333333333
		t1 = m2[2] & v
		t1 = (t1 ^ (t1 >> 1)) & 0x555555555555555
		t2 = m2[3] & v
		t2 = ((t2 << 1) ^ t2) & 0xaaaaaaaaaaaaaaa
		t1 |= t2
		t1 = ((t1 << 2) ^ t1) & 0xccccccccccccccc
		t0 |= t1
		t0 = (t0 ^ (t0 >> 4) ^ (t0 >> 8)) & 0xf00f00f00f00f
		t1 = m2[4] & v
		t1 = (t1 ^ (t1 >> 1)) & 0x555555555555555
		t1 = (t1 ^ (t1 >> 2)) & 0x333333333333333
		t1 = ((t1 << 4) ^ t1 ^ (t1 >> 4)) & 0xf00f00f00f00f0
		t0 |= t1
		return t0
	}
	// nrows == 10
	var t0, t1, t2, t3 uint64
	t0 = m2[0] & v
	t0 = (t0 ^ (t0 >> 1)) & 0x555555555555555
	t1 = m2[1] & v
	t1 = ((t1 << 1) ^ t1) & 0xaaaaaaaaaaaaaaa
	t0 |= t1
	t0 = (t0 ^ (t0 >> 2)) & 0x333333333333333
	t1 = m2[2] & v
	t1 = (t1 ^ (t1 >> 1)) & 0x555555555555555
	t2 = m2[3] & v
	t2 = ((t2 << 1) ^ t2) & 0xaaaaaaaaaaaaaaa
	t1 |= t2
	t1 = ((t1 << 2) ^ t1) & 0xccccccccccccccc
	t0 |= t1
	t0 = (t0 ^ (t0 >> 4) ^ (t0 >> 8)) & 0xf00f00f00f00f
	t1 = m2[4] & v
	t1 = (t1 ^ (t1 >> 1)) & 0x555555555555555
	t2 = m2[5] & v
	t2 = ((t2 << 1) ^ t2) & 0xaaaaaaaaaaaaaaa
	t1 |= t2
	t1 = (t1 ^ (t1 >> 2)) & 0x333333333333333
	t2 = m2[6] & v
	t2 = (t2 ^ (t2 >> 1)) & 0x555555555555555
	t3 = m2[7] & v
	t3 = ((t3 << 1) ^ t3) & 0xaaaaaaaaaaaaaaa
	t2 |= t3
	t2 = ((t2 << 2) ^ t2) & 0xccccccccccccccc
	t1 |= t2
	t1 = ((t1 << 4) ^ t1 ^ (t1 >> 4)) & 0xf00f00f00f00f0
	t0 |= t1
	t1 = m2[8] & v
	t1 = (t1 ^ (t1 >> 1)) & 0x555555555555555
	t2 = m2[9] & v
	t2 = ((t2 << 1) ^ t2) & 0xaaaaaaaaaaaaaaa
	t1 |= t2
	t1 = (t1 ^ (t1 >> 2)) & 0x333333333333333
	t1 = ((t1 << 8) ^ (t1 << 4) ^ t1) & 0xf00f00f00f00f00
	t0 |= t1
	return t0
}

// autplSetQform computes the quadratic form of
// automorphism m_io in place, storing q[i,j] in
// bit 13+j of m_io[i].
func autplSetQform(mIO []uint32) {
	var m2 [10]uint64
	var v uint64
	for i := 0; i < 10; i++ {
		v = uint64(mIO[i] & 0x7ff)
		v ^= v << 12
		v ^= v << 24
		m2[i] = v ^ (v << 48)
	}

	v = uint64(mat24ThetaTable[mIO[1]&0x7ff]) & 0x7ff
	v ^= (uint64(mat24ThetaTable[mIO[2]&0x7ff]) & 0x7ff) << 12
	v ^= (uint64(mat24ThetaTable[mIO[3]&0x7ff]) & 0x7ff) << 24
	v ^= (uint64(mat24ThetaTable[mIO[4]&0x7ff]) & 0x7ff) << 36
	v ^= (uint64(mat24ThetaTable[mIO[5]&0x7ff]) & 0x7ff) << 48

	v = bitvmultransp(v, m2[:], 5)
	v ^= 0xf00f00700b00d

	mIO[1] &= 0x1fff
	mIO[1] ^= uint32((v << 13) & 0x2000)
	mIO[2] &= 0x1fff
	mIO[2] ^= uint32((v << 1) & 0x6000)
	mIO[3] &= 0x1fff
	mIO[3] ^= uint32((v >> 11) & 0xe000)
	mIO[4] &= 0x1fff
	mIO[4] ^= uint32((v >> 23) & 0x1e000)
	mIO[5] &= 0x1fff
	mIO[5] ^= uint32((v >> 35) & 0x3e000)

	v = uint64(mat24ThetaTable[mIO[6]&0x7ff]) & 0x7ff
	v ^= (uint64(mat24ThetaTable[mIO[7]&0x7ff]) & 0x7ff) << 12
	v ^= (uint64(mat24ThetaTable[mIO[8]&0x7ff]) & 0x7ff) << 24
	v ^= (uint64(mat24ThetaTable[mIO[9]&0x7ff]) & 0x7ff) << 36
	v ^= (uint64(mat24ThetaTable[mIO[10]&0x7ff]) & 0x7ff) << 48

	v = bitvmultransp(v, m2[:], 10)
	v ^= 0x40140100e00e

	mIO[6] &= 0x1fff
	mIO[6] ^= uint32((v << 13) & 0x7e000)
	mIO[7] &= 0x1fff
	mIO[7] ^= uint32((v << 1) & 0xfe000)
	mIO[8] &= 0x1fff
	mIO[8] ^= uint32((v >> 11) & 0x1fe000)
	mIO[9] &= 0x1fff
	mIO[9] ^= uint32((v >> 23) & 0x3fe000)
	mIO[10] &= 0x1fff
	mIO[10] ^= uint32((v >> 35) & 0x7fe000)

	mIO[0] &= 0x1fff
	mIO[11] &= 0x1fff
}

// permToAutpl builds the Parker loop
// automorphism AutPL(c1, p) into m_out.
func permToAutpl(c1 uint32, p1 []byte, mOut []uint32) {
	permToMatrix(p1, mOut)
	for i := 0; i < 12; i++ {
		mOut[i] ^= (c1 << (12 - i)) & 0x1000
	}
	autplSetQform(mOut)
}

// PermToAutpl returns the Parker loop
// automorphism AutPL(c, p) as 12 uint32 entries.
func PermToAutpl(c uint32, p []byte) []uint32 {
	out := make([]uint32, 12)
	permToAutpl(c, p, out)
	return out
}

// CocodeToAutpl returns the diagonal Parker loop
// automorphism for cocode element c.
func CocodeToAutpl(c uint32) []uint32 {
	out := make([]uint32, 12)
	for i := 0; i < 12; i++ {
		out[i] = (uint32(1) << i) + ((c << (12 - i)) & 0x1000)
	}
	return out
}

// AutplToPerm extracts the M24 permutation of
// automorphism m.
func AutplToPerm(m []uint32) []byte {
	out := make([]byte, 24)
	matrixToPerm(m, out)
	return out
}

// AutplToCocode extracts the cocode element of
// automorphism m.
func AutplToCocode(m []uint32) uint32 {
	var v uint32
	for i := 0; i < 12; i++ {
		v += (m[i] >> (12 - i)) & (uint32(1) << i)
	}
	return v
}

// opPloopAutpl applies automorphism m1 to Parker
// loop element v1 and returns the result.
func opPloopAutpl(v1 uint32, m1 []uint32) uint32 {
	t := (v1 & 0x1000) ^
		(m1[0] & (0 - (v1 & 1))) ^ (m1[1] & (0 - ((v1 >> 1) & 1))) ^
		(m1[2] & (0 - ((v1 >> 2) & 1))) ^ (m1[3] & (0 - ((v1 >> 3) & 1))) ^
		(m1[4] & (0 - ((v1 >> 4) & 1))) ^ (m1[5] & (0 - ((v1 >> 5) & 1))) ^
		(m1[6] & (0 - ((v1 >> 6) & 1))) ^ (m1[7] & (0 - ((v1 >> 7) & 1))) ^
		(m1[8] & (0 - ((v1 >> 8) & 1))) ^ (m1[9] & (0 - ((v1 >> 9) & 1))) ^
		(m1[10] & (0 - ((v1 >> 10) & 1))) ^ (m1[11] & (0 - ((v1 >> 11) & 1)))
	v1 = (t >> 13) & v1
	v1 ^= v1 >> 6
	v1 ^= v1 >> 3
	v1 = (0x96 >> (v1 & 7)) & 1
	return (t & 0x1fff) ^ (v1 << 12)
}

// OpPloopAutpl applies Parker loop automorphism
// m to Parker loop element v.
func OpPloopAutpl(v uint32, m []uint32) uint32 {
	return opPloopAutpl(v, m)
}

// MulAutpl returns the product m1*m2 of Parker
// loop automorphisms.
func MulAutpl(m1, m2 []uint32) []uint32 {
	var m [12]uint32
	for i := 0; i < 12; i++ {
		m[i] = opPloopAutpl(m1[i], m2)
	}
	out := make([]uint32, 12)
	copy(out, m[:])
	autplSetQform(out)
	return out
}

// InvAutpl returns the inverse of Parker loop
// automorphism m.
func InvAutpl(m []uint32) []uint32 {
	var p, pInv [32]byte
	var mi [12]uint32
	matrixToPerm(m, p[:24])
	for i := 0; i < 24; i++ {
		pInv[p[i]&31] = byte(i)
	}
	permToMatrix(pInv[:24], mi[:])
	for i := 0; i < 12; i++ {
		t := opPloopAutpl(mi[i], m)
		mi[i] ^= t & 0x1000
	}
	out := make([]uint32, 12)
	copy(out, mi[:])
	autplSetQform(out)
	return out
}

// PermToIautpl returns the inverse permutation
// and the inverse automorphism of AutPL(c, p).
func PermToIautpl(c uint32, p []byte) ([]byte, []uint32) {
	pOut := make([]byte, 24)
	mOut := make([]uint32, 12)
	var m1 [16]uint32
	var pInv [32]byte
	permToMatrix(p, m1[:12])
	for i := 0; i < 12; i++ {
		m1[i] ^= ((c >> i) & 1) << 12
	}
	autplSetQform(m1[:12])
	for i := 0; i < 24; i++ {
		pInv[p[i]&31] = byte(i)
	}
	for i := 0; i < 24; i++ {
		pOut[i] = pInv[i]
	}
	permToMatrix(pInv[:24], mOut)
	for i := 0; i < 12; i++ {
		t := opPloopAutpl(mOut[i], m1[:12])
		mOut[i] ^= t & 0x1000
	}
	autplSetQform(mOut)
	return pOut, mOut
}

// OpAllAutpl returns a 2048-entry table mapping
// every Parker loop element modulo the center
// under automorphism m, with sign data in bits
// 12..14.
func OpAllAutpl(m []uint32) []uint16 {
	aOut := make([]uint16, 0x800)
	odd := m[11] & 0x1000
	mIdx := 0
	for i := uint32(1); i < 0x800; i += i {
		ri := m[mIdx]
		mIdx++
		q := (ri >> 13) & 0x7ff
		ri = (0 - (ri & 0x1000)) ^ (ri & 0xfff) ^ ((ri & 0x800) << 3)
		d1 := 0 - ((q & 1) << 12)
		d2 := 0 - ((q & 2) << 11)
		d4 := 0 - ((q & 4) << 10)
		for j := uint32(0); j < i; j += 8 {
			qq := j & q
			qq ^= qq >> 6
			qq ^= qq >> 3
			qq = 0 - ((0xD20 << (qq & 7)) & 0x1000)
			qq ^= ri
			aOut[i+j] = uint16(qq ^ uint32(aOut[j]))
			qq ^= d1
			aOut[i+j+1] = uint16(qq ^ uint32(aOut[j+1]))
			qq ^= d2
			aOut[i+j+3] = uint16(qq ^ uint32(aOut[j+3]))
			qq ^= d1
			aOut[i+j+2] = uint16(qq ^ uint32(aOut[j+2]))
			qq ^= d4
			aOut[i+j+6] = uint16(qq ^ uint32(aOut[j+6]))
			qq ^= d1
			aOut[i+j+7] = uint16(qq ^ uint32(aOut[j+7]))
			qq ^= d2
			aOut[i+j+5] = uint16(qq ^ uint32(aOut[j+5]))
			qq ^= d1
			aOut[i+j+4] = uint16(qq ^ uint32(aOut[j+4]))
		}
	}
	if odd != 0 {
		for i := uint32(0); i < 0x800; i += 4 {
			aOut[i] ^= mat24ThetaTable[i] & 0x1000
			aOut[i+1] ^= mat24ThetaTable[i+1] & 0x1000
			aOut[i+2] ^= mat24ThetaTable[i+2] & 0x1000
			aOut[i+3] ^= mat24ThetaTable[i+3] & 0x1000
		}
	}
	return aOut
}

// OpAllCocode returns a 2048-entry sign table
// for the diagonal automorphism of cocode
// element c.
func OpAllCocode(c uint32) []byte {
	aOut := make([]byte, 0x800)
	var sh uint32
	for i := uint32(1); i < 0x800; i += i {
		ri := byte(0 - ((c >> sh) & 1))
		sh++
		aOut[i] = ri
		aOut[i+1] = ri ^ aOut[1]
		aOut[i+2] = ri ^ aOut[2]
		aOut[i+3] = ri ^ aOut[3]
		for j := uint32(4); j < i; j += 4 {
			aOut[i+j] = ri ^ aOut[j]
			aOut[i+j+1] = ri ^ aOut[j+1]
			aOut[i+j+2] = ri ^ aOut[j+2]
			aOut[i+j+3] = ri ^ aOut[j+3]
		}
	}
	if c&0x800 != 0 {
		for i := uint32(0); i < 0x800; i += 4 {
			aOut[i] ^= byte((mat24ThetaTable[i] >> 12) & 0x1)
			aOut[i+1] ^= byte((mat24ThetaTable[i+1] >> 12) & 0x1)
			aOut[i+2] ^= byte((mat24ThetaTable[i+2] >> 12) & 0x1)
			aOut[i+3] ^= byte((mat24ThetaTable[i+3] >> 12) & 0x1)
		}
	}
	return aOut
}

// M24 random subgroup flags (enum
// mat24_rand_flags).
const (
	randMaskAll = 0x7f
	rand2       = 0x01
	randO       = 0x02
	randT       = 0x04
	randS       = 0x08
	randL       = 0x10
	rand3       = 0x20
	randD       = 0x40
)

// CompleteRandMode adds all flags for subgroups
// of M24 that contain the subgroup intersection
// described by mode.
func CompleteRandMode(mode uint32) uint32 {
	subgroup := func(m, sub, of uint32) uint32 {
		if m&sub == sub {
			m |= of
		}
		return m
	}
	old := uint32(0)
	for mode != old {
		old = mode
		mode = subgroup(mode, randD, randT)
		mode = subgroup(mode, randL, randO)
		mode = subgroup(mode, randD|rand3, rand2)
		mode = subgroup(mode, randL|randT, randD)
		mode = subgroup(mode, randL|rand2, randO|randT)
		mode = subgroup(mode, randT|rand2, randO|randL)
		mode = subgroup(mode, randL|rand3, randO|randS)
		mode = subgroup(mode, randT|rand3, randO|randS)
		mode = subgroup(mode, randO|randT|randD, randL)
	}
	return mode
}

// checkInSet reports whether p fixes the set
// partition {{i..i+diff-1}} starting at start.
func checkInSet(p []byte, start, diff uint32) bool {
	var s uint32
	for i := start; i < 24; i += diff {
		for j := uint32(1); j < diff; j++ {
			s |= uint32(p[i]) ^ uint32(p[i+j])
		}
	}
	return s&(0-diff) == 0
}

// PermInLocal returns the combination of
// subgroup flags of M24 that contain permutation
// p.
//
// PermInLocal panics if p is not in M24.
func PermInLocal(p []byte) uint32 {
	r := permInLocal(p)
	if r < 0 {
		panic("PermInLocal: permutation not in M24")
	}
	return uint32(r)
}

func permInLocal(p []byte) int32 {
	var mode uint32
	if permCheck(p) != 0 {
		return -1
	}
	s := (uint32(1) << p[2]) | (uint32(1) << p[3])
	if s == 0xc {
		mode |= rand2
	}
	s |= uint32(1) << p[1]
	if s == 0xe {
		mode |= rand3
	}
	s |= (uint32(1) << p[0]) | (uint32(1) << p[4]) | (uint32(1) << p[5]) |
		(uint32(1) << p[6]) | (uint32(1) << p[7])
	if s == 0xff {
		mode |= randO
	}
	if checkInSet(p, 8, 2) && mode&randO != 0 {
		mode |= randL
	}
	if checkInSet(p, 0, 4) {
		mode |= randS
	}
	if checkInSet(p, 0, 8) {
		mode |= randT
	}
	if checkInSet(p, 0, 2) {
		mode |= randD
	}
	return int32(mode)
}

// randPi holds the state for the local random
// permutation generator.
type randPi struct {
	mode       uint32
	rand       uint32
	bitmap     uint32
	maskOctad  uint32
	maskTetrad uint32
	syn        uint32
	err        int32
	h          [7]byte
}

func randInt(pr *uint32, n uint32) uint8 {
	r := *pr / n
	i := *pr % n
	*pr = r
	return uint8(i)
}

func findBit24(pBitmap, pr *uint32, mask uint32) uint8 {
	available := ^*pBitmap & mask & 0xffffff
	w := Bw24(available)
	if w == 0 {
		return 24
	}
	b := randInt(pr, w)
	bmask := SpreadB24(1<<b, available)
	*pBitmap |= bmask
	return uint8(lsbit24(bmask))
}

func (p *randPi) addPoint(index, mask uint32) {
	if index >= 7 {
		p.err = 1
	}
	if p.err != 0 {
		return
	}
	p.h[index] = byte(findBit24(&p.bitmap, &p.rand, mask))
	if p.h[index] >= 24 {
		p.err = 1
	}
	if p.err != 0 {
		p.mode = 0
		p.maskOctad = 0
		p.maskTetrad = 0
		p.bitmap = 0xffffff
		p.syn = 0xffffff
	}
}

func completeAffTrio(h1, h2, h3 uint32) uint8 {
	return uint8(h1 ^ h2 ^ h3)
}

func completeAffLine(h1, h2, h3 uint32) uint8 {
	al := [8]uint8{0, 1, 2, 3, 4, 5, 7, 6}
	v := al[h1&7] ^ al[h2&7] ^ al[h3&7]
	return al[v]
}

func (p *randPi) findImg0() uint32 {
	mode := p.mode
	fix := uint32(0xffffff)
	if mode&randO != 0 {
		fix &= 0xff
	}
	if mode&rand3 != 0 {
		fix &= 0x0e
	}
	if mode&rand2 != 0 {
		fix &= 0x0c
	}
	return fix
}

func (p *randPi) findImg1() uint32 {
	mode := p.mode
	fix := uint32(0xffffff)
	if mode&randT != 0 || mode&randO != 0 {
		fix &= 0xff << (p.h[0] & 0xf8)
	}
	p.maskOctad = fix
	if mode&randS != 0 {
		fix &= 15 << (p.h[0] & 0xfc)
	}
	p.maskTetrad = fix
	if mode&rand3 != 0 {
		fix &= 0x0e
	}
	if mode&rand2 != 0 {
		fix &= 0x0c
	}
	if mode&randD != 0 {
		fix &= 1 << (p.h[0] ^ 1)
	}
	return fix
}

func (p *randPi) findImg2() uint32 {
	mode := p.mode
	fix := p.maskTetrad
	if mode&rand3 != 0 {
		fix &= 0x0e
	}
	return fix
}

func (p *randPi) findImg3() uint32 {
	mode := p.mode
	fix := p.maskTetrad
	h := p.h
	if mode&randD != 0 {
		fix &= 1 << (h[2] ^ 1)
	} else if mode&randT != 0 {
		fix &= 1 << completeAffTrio(uint32(h[0]), uint32(h[1]), uint32(h[2]))
	} else if mode&randL != 0 {
		fix &= 1 << completeAffLine(uint32(h[0]), uint32(h[1]), uint32(h[2]))
	}
	return fix
}

func (p *randPi) findImg4() uint32 {
	return p.maskOctad
}

func (p *randPi) findImg5() uint32 {
	mode := p.mode
	p.syn = mat24Syndrome(p.bitmap, 0)
	fix := p.syn
	h := p.h
	if mode&randD != 0 {
		fix &= 1 << (h[4] ^ 1)
	} else if mode&randT != 0 {
		fix &= 1 << completeAffTrio(uint32(h[0]), uint32(h[1]), uint32(h[4]))
	} else if mode&randL != 0 {
		fix &= 1 << completeAffLine(uint32(h[0]), uint32(h[1]), uint32(h[4]))
	}
	return fix
}

func (p *randPi) findImg6() uint32 {
	return ^p.syn & 0xffffff
}

// completePerm generates a random local
// permutation into pOut and returns -1 on
// failure.
func completePerm(mode, rand uint32, pOut []byte) int32 {
	h1 := [7]byte{3, 2, 1, 0, 5, 4, 8}
	var s randPi
	s.mode = CompleteRandMode(mode)
	s.rand = rand
	s.addPoint(0, s.findImg0())
	s.addPoint(1, s.findImg1())
	s.addPoint(2, s.findImg2())
	s.addPoint(3, s.findImg3())
	s.addPoint(4, s.findImg4())
	s.addPoint(5, s.findImg5())
	s.addPoint(6, s.findImg6())
	if s.err != 0 {
		return -1
	}
	if permFromHeptads(h1[:], s.h[:], pOut) == 0xffffffff {
		return -1
	}
	return 0
}

// permRandLocal generates a random element of
// the subgroup of M24 described by mode into
// pOut, returning -1 on failure.
func permRandLocal(mode, rand uint32, pOut []byte) int32 {
	if mode&randMaskAll == 0 {
		m24numToPerm(rand%Mat24Order, pOut)
		return 0
	}
	mode = CompleteRandMode(mode)
	return completePerm(mode, rand, pOut)
}

// PermRandLocal returns a random element of the
// subgroup of M24 described by mode, selected by
// rand.
//
// PermRandLocal panics if generation fails.
func PermRandLocal(mode, rand uint32) []byte {
	out := make([]byte, 24)
	if permRandLocal(mode, rand, out) < 0 {
		panic("PermRandLocal: generation failed")
	}
	return out
}

// M24numRandLocal returns the number of a random
// element of the subgroup of M24 described by
// mode, or -1 on failure.
func M24numRandLocal(mode, rand uint32) int32 {
	if mode&randMaskAll == 0 {
		return int32(rand % Mat24Order)
	}
	var pi [24]byte
	if permRandLocal(mode, rand, pi[:]) < 0 {
		return -1
	}
	return int32(PermToM24num(pi[:]))
}

// VectToList returns up to maxLen ascending
// set-bit positions of vector v.
func VectToList(v uint32, maxLen int) []byte {
	out := make([]byte, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		j := lsbit24(v)
		if j >= 24 {
			return out
		}
		out = append(out, byte(j))
		v ^= 1 << j
	}
	return out
}

// OctadEntries returns the 8 entries of octad in
// suboctad order.
//
// OctadEntries panics if octad >= 759.
func OctadEntries(octad uint32) [8]uint8 {
	if octad >= 759 {
		panic("OctadEntries: octad out of range")
	}
	var out [8]uint8
	copy(out[:], mat24OctadElementTable[octad<<3:(octad<<3)+8])
	return out
}

// PermToNet returns the 9-layer modified Benes
// network for permutation p.
func PermToNet(p []byte) [9]uint32 {
	var out [9]uint32
	var pp, q [32]uint8
	for i := 0; i < 24; i++ {
		pp[i] = p[i] & 31
	}
	for sh := uint32(0); sh < 3; sh++ {
		d := uint32(1) << sh
		for i := 0; i < 24; i++ {
			q[pp[i]] = uint8(i)
		}
		var done, res0, res1 uint32
		for i := uint32(0); i < 24; i++ {
			j := i
			for done&(1<<j) == 0 {
				done |= 1 << j
				j = uint32(pp[j])
				res1 |= ((j & d) >> sh) << (j & ^d)
				j = uint32(q[j^d])
				done |= 1 << j
				res0 |= ((^j & d) >> sh) << (j & ^d)
				j = j ^ d
			}
		}
		out[sh] = res0
		out[8-sh] = res1
		res0 |= res0 << d
		res1 |= res1 << d
		for i := uint32(0); i < 24; i++ {
			j := uint32(pp[i^(((res0>>i)&1)<<sh)])
			q[i] = uint8(j ^ (((res1 >> j) & 1) << sh))
		}
		for i := 0; i < 24; i++ {
			pp[i] = q[i]
		}
	}
	var res0, res1, res2 uint32
	for i := uint32(0); i < 8; i++ {
		j := uint32(pp[i]) >> 3
		j = 2*j + ((uint32(pp[i+8]) >> (3 + (j & 1))) & 1)
		j = (0x236407 >> (j << 2)) & 0xf
		res2 |= (j & 1) << i
		res1 |= ((j >> 1) & 1) << i
		res0 |= ((j >> 2) & 1) << i
	}
	out[3] = res0
	out[4] = res1
	out[5] = res2
	return out
}

// M24numRandAdjustXY clears the cocode bits of
// Parker loop element v that must vanish for the
// subgroup described by mode.
func M24numRandAdjustXY(mode, v uint32) uint32 {
	if mode&rand3 != 0 {
		v &= ^uint32(0x300)
	}
	if mode&rand2 != 0 {
		v &= ^uint32(0x200)
	}
	return v
}

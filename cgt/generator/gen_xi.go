// Package generator implements the low-level generator
// primitives of the monster's G_{x0} machinery: the xi
// operation on Leech-lattice-mod-2 vectors (gen_xi),
// the xoshiro256** RNG (rng), and the linear-algebra
// union-find over GF(2) orbit structures (ufind_lin2).
package generator

import "patel.codes/cgt/mat24"

// xiGGrayTable is MAT24_XI_G_GRAY_TABLE.
var xiGGrayTable = [64]uint8{
	0x00, 0x0e, 0x0d, 0x83, 0x0b, 0x85, 0x86, 0x88,
	0x07, 0x89, 0x8a, 0x84, 0x8c, 0x82, 0x81, 0x0f,
	0x20, 0x2e, 0x2d, 0xa3, 0x2b, 0xa5, 0xa6, 0xa8,
	0x27, 0xa9, 0xaa, 0xa4, 0xac, 0xa2, 0xa1, 0x2f,
	0x90, 0x9e, 0x9d, 0x13, 0x9b, 0x15, 0x16, 0x18,
	0x97, 0x19, 0x1a, 0x14, 0x1c, 0x12, 0x11, 0x9f,
	0x30, 0x3e, 0x3d, 0xb3, 0x3b, 0xb5, 0xb6, 0xb8,
	0x37, 0xb9, 0xba, 0xb4, 0xbc, 0xb2, 0xb1, 0x3f,
}

// xiGCocodeTable is MAT24_XI_G_COCODE_TABLE.
var xiGCocodeTable = [64]uint8{
	0x00, 0x8e, 0x8d, 0x83, 0x8b, 0x85, 0x86, 0x08,
	0x87, 0x89, 0x8a, 0x04, 0x8c, 0x02, 0x01, 0x0f,
	0xa0, 0x2e, 0x2d, 0x23, 0x2b, 0x25, 0x26, 0xa8,
	0x27, 0x29, 0x2a, 0xa4, 0x2c, 0xa2, 0xa1, 0xaf,
	0x10, 0x9e, 0x9d, 0x93, 0x9b, 0x95, 0x96, 0x18,
	0x97, 0x99, 0x9a, 0x14, 0x9c, 0x12, 0x11, 0x1f,
	0x30, 0xbe, 0xbd, 0xb3, 0xbb, 0xb5, 0xb6, 0x38,
	0xb7, 0xb9, 0xba, 0x34, 0xbc, 0x32, 0x31, 0x3f,
}

// compressGray packs the gray bits of x.
func compressGray(x uint32) uint32 {
	return (x & 0x0f) + ((x >> 6) & 0x30)
}

// expandGray unpacks the gray bits of x.
func expandGray(x uint32) uint32 {
	return (x & 0x0f) + ((x & 0x30) << 6)
}

// XiGGray computes gamma on Golay code element v.
// v is in gcode rep, the result in cocode rep.
func XiGGray(v uint32) uint32 {
	return expandGray(uint32(xiGGrayTable[compressGray(v)]))
}

// XiW2Gray computes w2 on Golay code element v
// (in gcode rep). The result is 0 or 1.
func XiW2Gray(v uint32) uint32 {
	return uint32(xiGGrayTable[compressGray(v)]) >> 7
}

// XiGCocode is a kind of inverse of gamma. For
// cocode element c it returns the unique grey
// Golay code vector v (in gcode rep) whose
// gamma is the grey part of c.
func XiGCocode(c uint32) uint32 {
	return expandGray(uint32(xiGCocodeTable[compressGray(c)]))
}

// XiW2Cocode computes w2 on cocode element v
// (in cocode rep). The result is 0 or 1.
func XiW2Cocode(c uint32) uint32 {
	return uint32(xiGCocodeTable[compressGray(c)]) >> 7
}

// XiOpXi returns xi^(-exp) x xi^exp for an
// element x of Q_{x0} in Leech lattice encoding.
func XiOpXi(x uint32, exp int) uint32 {
	e := uint32(exp)
	var tv, tc uint32
	// reduce bits 1,0 of e mod 3; no action if 0
	if (e-1)&2 != 0 {
		return x
	}
	// map e = 1, 2 to e = -1, 0
	e = (e & 3) - 2
	// tv = scalar product of gray parts of code
	// and cocode
	tv = (x >> 12) & x & 0xc0f
	tv = 0x6996 >> ((tv ^ (tv >> 10)) & 0xf)
	// xor scalar product to sign
	x ^= (tv & 1) << 24
	// tv = w2(code), g(code); tc = w2(cocode),
	// g(cocode)
	tv = uint32(xiGGrayTable[compressGray(x>>12)])
	tc = uint32(xiGCocodeTable[compressGray(x)])
	// if old e = 1: kill gray code part
	// if old e = 2: kill gray cocode part
	x &^= 0xc0f << (e & 12)
	x ^= expandGray(tv)       // g(code) to cocode
	x ^= expandGray(tc) << 12 // g(cocode) to code
	tv ^= (tc ^ tv) & e
	x ^= (tv >> 7) << 24
	return x
}

// XiOpXiNoSign is XiOpXi up to sign only.
func XiOpXiNoSign(x uint32, exp int) uint32 {
	e := uint32(exp)
	var tv, tc uint32
	if (e-1)&2 != 0 {
		return x
	}
	e = (e & 3) - 2
	tv = uint32(xiGGrayTable[compressGray(x>>12)])
	tc = uint32(xiGCocodeTable[compressGray(x)])
	x &^= 0xc0f << (e & 12)
	x ^= expandGray(tv)
	x ^= expandGray(tc) << 12
	return x
}

// XiLeechToShort converts x in Q_{x0} from Leech
// lattice encoding to short vector encoding.
//
// An invalid x is converted to 0.
func XiLeechToShort(x uint32) uint32 {
	var box, code uint32
	sign := (x >> 24) & 1
	gcodev := mat24.GcodeToVect(x >> 12)
	// transform linear to internal Leech rep
	x ^= uint32(mat24.ThetaTable((x>>12)&0x7ff)) & 0xfff
	cocodev := mat24.CocodeSyndromeRaw(x, mat24.Lsbit24(gcodev))
	// put w = weight(code word gcodev) / 4
	w := 0 - ((x >> 23) & 1)
	w = (((uint32(mat24.ThetaTable((x>>12)&0x7ff)) >> 12) & 7) ^ w) + (w & 7)
	if x&0x800 != 0 { // case odd cocode
		if cocodev&(cocodev-1) != 0 {
			return 0
		}
		scalar := (x >> 12) & x & 0xfff
		scalar = mat24.Parity12(scalar)
		if (scalar^w)&1 != 0 {
			return 0
		}
		code = ((x & 0x7ff000) >> 7) | mat24.Lsbit24(cocodev)
		box = 4 + (code >> 15)
		code &= 0x7fff
	} else { // case even cocode
		switch w {
		case 4, 2:
			code = mat24.CocodeToSuboctadRaw(x, x>>12, 1)
			if code >= 24000 {
				// 24000 = (15 + 360) * 64
				code -= 24000
				box = 3
			} else if code >= 960 {
				// 960 = 15 * 64
				code -= 960
				box = 2
			} else {
				code += 1536
				box = 1
			}
		case 3:
			return 0
		default: // can be case 0 or 6 only
			y1 := mat24.Lsbit24(cocodev)
			cocodev ^= 1 << y1
			y2 := mat24.Lsbit24(cocodev)
			if cocodev != (1<<y2) || y1 >= 24 {
				return 0
			}
			code = 384*(w&2) + (y2 << 5) + y1
			box = 1
		}
	}
	return (sign << 15) + (box << 16) + code
}

// XiShortToLeech converts x in Q_{x0} from short
// vector encoding to Leech lattice encoding.
//
// An invalid x is converted to 0.
func XiShortToLeech(x uint32) uint32 {
	sign := (x >> 15) & 1
	code := x & 0x7fff
	var gcode, cocode, octad uint32 = 0, 0, 0xffff
	switch x >> 16 {
	case 1:
		if code < 1536 {
			// 1536 = 2 * 24 * 32
			gcode = 0
			if code >= 768 {
				gcode = 1
			}
			code -= (0 - gcode) & 768
			gcode <<= 11 // gcode = code >= 768 ? 0x800 : 0
			i := code >> 5
			j := code & 31
			cocode = mat24.VectToCocode((1 << i) ^ (1 << j))
			if cocode == 0 || cocode&0x800 != 0 {
				return 0
			}
		} else if code < 2496 {
			// 2496 = 2 * 24 * 32 + 15 * 64
			octad = code - 1536
		} else {
			return 0
		}
	case 2:
		if code >= 23040 { // 23040 = 360 * 64
			return 0
		}
		octad = code + 960 // 960 = 15 * 64
	case 3:
		if code >= 24576 { // 24576 = 384 * 64
			return 0
		}
		octad = code + 24000 // 24000 = (15 + 360) * 64
	case 5:
		code += 0x8000
		fallthrough
	case 4:
		cocode = mat24.VectToCocode(1 << (code & 31))
		if cocode == 0 {
			return 0
		}
		gcode = (code >> 5) & 0x7ff
		w := ((uint32(mat24.ThetaTable(gcode)) >> 12) & 1) ^ (gcode & cocode)
		w = mat24.Parity12(w)
		gcode ^= w << 11
	default:
		return 0
	}
	if octad < 48576 {
		// 48576 = 759 * 64
		gcode = uint32(mat24.OctDecTable(octad>>6)) & 0xfff
		cocode = mat24.SuboctadToCocode(octad&0x3f, octad>>6)
		w := mat24.SuboctadWeight(octad & 0x3f)
		gcode ^= w << 11
	}
	// transform internal Leech rep to linear rep
	cocode ^= uint32(mat24.ThetaTable(gcode&0x7ff)) & 0xfff
	return (sign << 24) | (gcode << 12) | cocode
}

// XiOpXiShort returns xi^(-exp) x xi^exp for an
// element x of Q_{x0} in short vector encoding.
//
// An invalid x is returned unchanged.
func XiOpXiShort(x uint32, exp int) uint32 {
	y := XiShortToLeech(x)
	if y == 0 {
		return x
	}
	y = XiOpXi(y, exp)
	if y == 0 {
		return x
	}
	y = XiLeechToShort(y)
	if y != 0 {
		return y
	}
	return x
}

// Leech2Mul returns the product of x1 and x2 in
// Q_{x0}, both in Leech lattice encoding.
func Leech2Mul(a, b uint32) uint32 {
	result := (b >> 12) & a
	result = mat24.Parity12(result)
	return (result << 24) ^ a ^ b
}

// leech2Subtype returns gen_leech2_subtype: the
// BCD-coded value 0x10*type + subtype.
func leech2Subtype(v2 uint32) uint32 {
	tabOdd := [4]uint32{0x21, 0x31, 0x43, 0x33}
	tabEvenScalar1 := [7]uint32{0xff, 0xff, 0x34, 0x36, 0x34, 0xff, 0xff}

	theta := uint32(mat24.ThetaTable((v2 >> 12) & 0x7ff))
	coc := (v2 ^ theta) & 0xfff
	scalar := (v2 >> 12) & v2
	scalar = mat24.Parity12(scalar)

	syn := uint32(mat24.SyndromeTable(coc & 0x7ff))

	// odd cocode
	if v2&0x800 != 0 {
		cw := 3 - ((((syn & 0x7fff) + 0x2000) >> 15) << 1)
		return tabOdd[cw-1+scalar]
	}

	// w = weight(Golay code word of v2) / 4
	w := 0 - ((v2 >> 23) & 1)
	w = (((theta >> 12) & 7) ^ w) + (w & 7)

	cw := (syn >> 15) << 1

	if scalar != 0 {
		return tabEvenScalar1[w]
	}

	switch w {
	case 6:
		if coc == 0 {
			return 0x48
		}
		fallthrough
	case 0:
		cw = (4 - cw) & ((0 - coc) >> 16)
		return cw << 4
	case 3:
		return 0x46
	case 4:
		v2 ^= 0x800000
		fallthrough
	default: // case 2
		octad := mat24.GcodeToVect(v2 >> 12)
		w = SuboctadType(octad, w>>1, coc)
		return (0x44444222 >> (8 * w)) & 0xff
	}
}

// SuboctadType returns the suboctad type. octad
// is a bit vector of length 8, w is 1 for an
// octad and 0 for a complemented octad, coc an
// even cocode vector in cocode rep.
func SuboctadType(octad, w, coc uint32) uint32 {
	cw := uint32(mat24.SyndromeTable(coc&0x7ff)) >> 15
	lsb := mat24.Lsbit24(octad)
	coc ^= mat24.RecipBasis(lsb)
	syn := uint32(mat24.SyndromeTable(coc & 0x7ff))
	cocodev := mat24.SynFromTable(syn) ^ (1 << lsb)
	var sub uint32
	if octad&cocodev != cocodev {
		sub = 1
	}
	return ((^w ^ cw) & 1) + 2*sub
}

// Leech2Subtype returns gen_leech2_subtype of v2:
// 0x10*type + subtype, with v2 in Leech lattice
// encoding.
func Leech2Subtype(x uint32) uint32 {
	return leech2Subtype(x)
}

// Leech2CoarseSubtype returns a coarser version
// of Leech2Subtype as a value 0..8.
func Leech2CoarseSubtype(x uint32) uint32 {
	scalar := (x >> 12) & x
	scalar = mat24.Parity12(scalar) // norm of v2 mod 2

	if x&0x800 != 0 {
		if scalar != 0 {
			return 8
		}
		return 5
	}
	w := uint32(mat24.ThetaTable((x>>12)&0x7ff)) & 0x1000
	if w != 0 {
		if scalar != 0 {
			return 7
		}
		return 4
	}
	if x&0x7ff000 != 0 {
		if scalar != 0 {
			return 6
		}
		return 3
	}
	if x&0x7ff != 0 {
		return 2
	}
	if x&0x800000 != 0 {
		return 1
	}
	return 0
}

// Leech2Type returns the type of vector x in the
// Leech lattice mod 2 (0, 2, 3, or 4), with x in
// Leech lattice encoding.
func Leech2Type(x uint32) uint32 {
	// Return 3 if scalar product <code,cocode> odd
	scalar := (x >> 12) & x
	scalar = mat24.Parity12(scalar)
	if scalar != 0 {
		return 3
	}

	if x&0x800 != 0 { // odd cocode words
		theta := uint32(mat24.ThetaTable((x >> 12) & 0x7ff))
		syn := uint32(mat24.SyndromeTable((theta ^ x) & 0x7ff))
		w := ((syn & 0x3ff) + 0x100) & 0x400
		return 4 - (w >> 9)
	}

	// Golay code word 0 (or Omega)
	if x&0x7ff800 == 0 {
		if x&0xffffff == 0 {
			return 0
		}
		syn := uint32(mat24.SyndromeTable(x & 0x7ff))
		return 4 - ((syn >> 14) & 2)
	}

	theta := uint32(mat24.ThetaTable((x >> 12) & 0x7ff))
	if theta&0x1000 != 0 {
		return 4
	}
	w := ((theta >> 13) ^ (x >> 23)) & 1
	coc := (x ^ theta) & 0x7ff
	cw := uint32(mat24.SyndromeTable(coc&0x7ff)) >> 15
	if cw == w {
		return 4
	}
	x ^= (1 - w) << 23
	octad := mat24.GcodeToVect(x >> 12)
	lsb := mat24.Lsbit24(octad)
	coc ^= mat24.RecipBasis(lsb)
	syn := uint32(mat24.SyndromeTable(coc & 0x7ff))
	ccv := mat24.SynFromTable(syn) ^ (1 << lsb)
	if (octad&ccv)^ccv != 0 {
		return 4
	}
	return 2
}

// Leech2Type2 returns the subtype of vector x if
// x is of type 2, and 0 otherwise. x is in Leech
// lattice encoding.
func Leech2Type2(x uint32) uint32 {
	if x&0x800 != 0 { // odd cocode words
		theta := uint32(mat24.ThetaTable((x >> 12) & 0x7ff))
		syn := uint32(mat24.SyndromeTable((theta ^ x) & 0x7ff))
		if (syn & 0x3ff) < (24 << 5) {
			return 0
		}
		scalar := (x >> 12) & x & 0xfff
		scalar ^= scalar >> 6
		scalar ^= scalar >> 3
		scalar = (0x69 >> (scalar & 7)) & 1
		return 0x21 & (0 - scalar)
	}
	// Golay code word 0
	if x&0x7ff000 == 0 {
		syn := uint32(mat24.SyndromeTable(x & 0x7ff))
		return 0x20 & (0 - ((syn >> 15) & 1))
	}

	theta := uint32(mat24.ThetaTable((x >> 12) & 0x7ff))
	if theta&0x1000 != 0 {
		return 0
	}
	w := ((theta >> 13) ^ (x >> 23)) & 1
	x ^= (1 - w) << 23
	coc := (x ^ theta) & 0x7ff
	octad := mat24.GcodeToVect(x >> 12)
	if SuboctadType(octad, w, coc) != 0 {
		return 0
	}
	return 0x22
}

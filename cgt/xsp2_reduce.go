package cgt

// Reduction of Leech-lattice-mod-2 vectors to a
// standard frame (gen_leech_reduce.c) and the
// involution machinery (involutions.c,
// xsp2co1_traces.c) used by xsp2.go. These are
// ported here because the gen_leech_reduce and
// mm_group_n modules are not yet available as
// standalone packages.

// lstd is the identity list (0,...,7).
var lstd = [8]byte{0, 1, 2, 3, 4, 5, 6, 7}

// octadList is the OCTAD list (0,1,2,3,4,8,9).
var octadList = [7]byte{0, 1, 2, 3, 4, 8, 9}

/*************************************************************************
*** xi-reduction helpers
*************************************************************************/

// xiReduceOddType4 returns an exponent e such
// that xi^e maps the subtype-0x43 vector v to
// subtype 0x42 (returns e) or 0x44 (returns
// 0x100+e), or -1 if none exists.
func xiReduceOddType4(v uint32) int32 {
	coc := (v ^ uint32(mat24ThetaTable[(v>>12)&0x7ff])) & 0xfff
	tab := uint32(mat24SyndromeTable[coc&0x7ff])
	tab ^= ((tab >> 5) & 0x3ff) ^ ((tab & 0x1f) << 10)
	tab &= 0x739c
	tab += 0x739c
	tab &= 0x8420
	if tab == 0x8420 {
		return -1
	}
	scalar := (v >> 22) & 1
	exp := 2 - scalar
	var hi int32
	if tab != 0 {
		hi = 1 << 8
	}
	return hi + int32(exp)
}

// xiReduceOddType2 returns an exponent e such
// that xi^e maps the subtype-0x21 vector v to
// subtype 0x22.
func xiReduceOddType2(v uint32) int32 {
	scalar := (v >> 22) & 1
	return int32(2 - scalar)
}

// xiReduceOctad returns an exponent e such that
// v*xi^e is an even cocode element mod Omega, or
// -1 if none exists.
func xiReduceOctad(v uint32) int32 {
	if v&0x7ff800 == 0 {
		return 0
	}
	if v&0x7f080f == 0 {
		return 1
	}
	parity := uint32(0) - ((0x6996 >> (v & 0xf)) & 1)
	v ^= ((v >> 12) ^ parity) & 0xf
	if v&0x7f080f == 0 {
		return 2
	}
	return -1
}

// xiReduceDodecad returns an exponent e such that
// xi^e maps the subtype-0x46 vector v to subtype
// 0x44, or -1 if none exists.
func xiReduceDodecad(v uint32) int32 {
	vect := gcodeToVectInternal(v >> 12)
	s1 := vect | (vect >> 2)
	s1 = s1 | (s1 >> 1)
	s0 := vect & (vect >> 2)
	s0 = s0 & (s0 >> 1)
	s := (s0 | ^s1) & 0x111111
	if s == 0 {
		return -1
	}
	s *= 15
	coc := v ^ uint32(mat24ThetaTable[(v>>12)&0x7ff])
	tab := uint32(mat24SyndromeTable[(uint32(mat24RecipBasis[0])^coc)&0x7ff])
	scalar := s ^ (s >> (tab & 31)) ^ (s >> ((tab >> 5) & 31)) ^ (s >> ((tab >> 10) & 31))
	scalar &= 1
	return int32(2 - scalar)
}

/*************************************************************************
*** Permutation helpers
*************************************************************************/

// applyPerm applies a permutation pi mapping the
// entries of src to dest (length n) to the Leech
// vector v, storing the generator x_pi (tag IP)
// in pRes[0]. It returns v*x_pi, or -1 on
// failure.
func applyPerm(v uint32, src, dest []byte, n int, pRes *uint32) int32 {
	t, p, err := PermFromMap(dest[:n], src[:n])
	if err != nil || t < 1 || t > 3 {
		return -1
	}
	*pRes = 0xA0000000 + PermToM24num(p)
	pInv := InvPerm(p)
	xd := (v >> 12) & 0xfff
	xdelta := (v ^ PloopTheta(xd)) & 0xfff
	m := PermToMatrix(pInv)
	xd = OpGcodeMatrix(xd, m)
	xdelta = OpCocodePerm(xdelta, pInv)
	return int32((xd << 12) ^ xdelta ^ PloopTheta(xd))
}

// findOctadPermutation finds a permutation
// mapping the (possibly complemented) octad of v
// to the standard octad and stores it in pRes. It
// returns v*x_pi, or -1 on failure.
func findOctadPermutation(v uint32, pRes *uint32) int32 {
	var src [8]byte
	theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
	w := ((theta >> 13) ^ (v >> 23) ^ 1) & 1
	vect := gcodeToVectInternal((v ^ (w << 23)) >> 12)
	copy(src[:5], VectToList(vect, 5))
	coc := (v ^ PloopTheta(v>>12)) & 0xfff
	syn := CocodeSyndrome(coc, uint32(src[0])) & ^vect
	n := 5
	if syn != 0 {
		v5 := (uint32(1) << src[0]) | (uint32(1) << src[1]) | (uint32(1) << src[2])
		coc = VectToCocode(v5 | syn)
		tab := uint32(mat24SyndromeTable[coc&0x7ff])
		special := synFromTable(tab)
		src[3] = byte(lsbit24(special & vect))
		src[4] = byte(lsbit24(vect & ^(special | v5)))
		src[5] = byte(lsbit24(syn))
		syn &= ^(uint32(1) << src[5])
		src[6] = byte(lsbit24(syn))
		n = 7
	}
	return applyPerm(v, src[:], octadList[:], n, pRes)
}

/*************************************************************************
*** Subtype starters (gen_leech_type.c)
*************************************************************************/

// genLeech2StartType24 returns the subtype of a
// type-2 vector v with v+beta of type 4 (0 for
// v=beta+Omega), or a negative value.
func genLeech2StartType24(v uint32) int32 {
	if v&0x200000 != 0 {
		return -1
	}
	switch vtype := Leech2Type2(v); vtype {
	case 0x21:
		theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
		syn := uint32(mat24SyndromeTable[(theta^v)&0x7ff])
		if syn&0x1e == 2 {
			return -1
		}
		return 0x21
	case 0x20:
		if v&0x7fffff == 0x200 {
			if v&0x800000 != 0 {
				return 0
			}
			return -1
		}
		theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
		syn := uint32(mat24SyndromeTable[(theta^v^0x200)&0x7ff])
		if syn&0x8000 != 0 {
			return -1
		}
		return int32(vtype)
	case 0x22:
		theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
		w := ((theta >> 13) ^ (v >> 23)) & 1
		v ^= (1 - w) << 23
		coc := (v ^ theta ^ 0x200) & 0x7ff
		octad := gcodeToVectInternal(v >> 12)
		if suboctadType(octad, w, coc) != 0 {
			return 0x22
		}
		return -1
	default:
		return -1
	}
}

// genLeech2StartType4 returns the subtype of the
// type-4 vector v used for reduction (0 for
// v=Omega), or a negative value.
func genLeech2StartType4(v uint32) int32 {
	v &= 0xffffff
	if v&0x7ff800 == 0 {
		if v&0x7fffff == 0 {
			if v&0x800000 != 0 {
				return 0
			}
			return -1
		}
		coc := v & 0x7ff
		syn := uint32(mat24SyndromeTable[coc])
		if syn&0x8000 != 0 {
			return -2
		}
		syn = uint32(mat24SyndromeTable[coc^0x200])
		if syn&0x8000 != 0 {
			return 0x20
		}
		return 0x40
	}
	scalar := (v >> 12) & v & 0xfff
	scalar = parity12(scalar)
	if scalar != 0 {
		return -3
	}
	theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
	coc := (theta ^ v) & 0x7ff
	syn := uint32(mat24SyndromeTable[coc])
	if v&0x800 != 0 {
		if (syn & 0x3ff) >= (24 << 5) {
			return -2
		}
		syn = uint32(mat24SyndromeTable[coc^0x200])
		if (syn&0x3ff) >= (24<<5) && (v&0x200000) == 0 {
			return 0x21
		}
		return 0x43
	}
	if theta&0x1000 != 0 {
		return 0x46
	}
	w := ((theta >> 13) ^ (v >> 23)) & 1
	v ^= (1 - w) << 23
	octad := gcodeToVectInternal(v >> 12)
	coc = (v ^ theta) & 0x7ff
	sub := suboctadType(octad, w, coc)
	if sub == 0 {
		return -2
	}
	if suboctadType(octad, w, coc^0x200) == 0 {
		return 0x22
	}
	return int32((0x44444222 >> (8 * sub)) & 0xff)
}

/*************************************************************************
*** Reduce type-2, type-2-ortho and type-4 vectors
*************************************************************************/

// genLeech2ReduceType2 maps a type-2 vector v to
// the standard short vector beta, storing the
// word in pgOut and returning its length, or a
// negative value.
func genLeech2ReduceType2(v uint32, pgOut []uint32) int {
	end := 0
	vtype := Leech2Subtype(v)
	if (vtype >> 4) != 2 {
		if vtype>>4 != 0 {
			return 0 - int(vtype>>4)
		}
		return -1
	}
	for round := 0; round < 4; round++ {
		var exp int32
		switch vtype {
		case 0x21:
			exp = xiReduceOddType2(v)
			vtype = 0x22
		case 0x22:
			if exp = xiReduceOctad(v); exp < 0 {
				var src [4]byte
				theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
				w := ((theta >> 13) ^ (v >> 23) ^ 1) & 1
				vect := gcodeToVectInternal((v ^ (w << 23)) >> 12)
				copy(src[:4], VectToList(vect, 4))
				res := applyPerm(v, src[:], lstd[:], 4, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
				if exp = xiReduceOctad(v); exp < 0 {
					return -1
				}
			}
			vtype = 0x20
		case 0x20:
			exp = 0
			if v&0x7fffff != 0x200 {
				var src [2]byte
				tab := uint32(mat24SyndromeTable[(v^uint32(mat24RecipBasis[23]))&0x7ff]) & 0x3ff
				tab -= ((tab + 0x100) & 0x400) >> 5
				src[0] = byte(tab & 31)
				src[1] = byte((tab >> 5) & 31)
				res := applyPerm(v, src[:], lstd[2:], 2, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
			}
			if v&0x800000 != 0 {
				pgOut[end] = 0xC0000200
				v = Leech2OpAtom(v, pgOut[end])
				end++
			}
			if v&0xffffff != 0x200 {
				return -1
			}
			return end
		default:
			return -1
		}
		if exp != 0 {
			v = XiOpXi(v, int(exp))
			if v&0xfe000000 != 0 {
				return -1
			}
			pgOut[end] = 0xe0000003 - uint32(exp)
			end++
		}
	}
	return -1
}

// reduceType2Ortho maps a type-2 vector v that is
// orthogonal to beta to e_2+e_3, fixing beta. It
// stores the word in pgOut and returns its length
// or a negative value.
func reduceType2Ortho(v, vtype uint32, pgOut []uint32) int {
	end := 0
	for round := 0; round < 4; round++ {
		var exp int32
		switch vtype {
		case 0x21:
			exp = xiReduceOddType2(v)
			vtype = 0x22
		case 0x22:
			if exp = xiReduceOctad(v); exp < 0 {
				var src [8]byte
				theta := uint32(mat24ThetaTable[(v>>12)&0x7ff])
				w := ((theta >> 13) ^ (v >> 23) ^ 1) & 1
				vect := gcodeToVectInternal((v ^ (w << 23)) >> 12)
				src[2] = 2
				src[3] = 3
				var d, n int
				if vect&0x0c != 0 {
					copy(src[:2], VectToList(vect & ^uint32(0x0c), 2))
					d, n = 0, 4
				} else {
					copy(src[4:7], VectToList(vect, 3))
					v5 := (uint32(1) << src[4]) | (uint32(1) << src[5]) | (uint32(1) << src[6])
					coc := VectToCocode(v5 | 0x0c)
					tab := uint32(mat24SyndromeTable[coc&0x7ff])
					special := synFromTable(tab)
					src[7] = byte(lsbit24(special & vect))
					d, n = 2, 6
				}
				res := applyPerm(v, src[d:], lstd[d:], n, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
				if exp = xiReduceOctad(v); exp < 0 {
					return -1
				}
			}
			vtype = 0x20
		case 0x20:
			if v&0xffffff == 0x800200 {
				return end
			}
			exp = 0
			if v&0xfff != 0x200 && v&0xfff != 0x600 {
				var src [4]byte
				tab := uint32(mat24SyndromeTable[(v^uint32(mat24RecipBasis[23]))&0x7ff]) & 0x3ff
				tab -= ((tab + 0x100) & 0x400) >> 5
				src[0] = byte(tab & 31)
				src[1] = byte((tab >> 5) & 31)
				src[2] = 2
				src[3] = 3
				res := applyPerm(v, src[:], lstd[:], 4, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
			}
			exp = int32(2 - ((v >> 23) & 1))
		default:
			return -1
		}
		if exp != 0 {
			v = XiOpXi(v, int(exp))
			if v&0xfe000000 != 0 {
				return -1
			}
			pgOut[end] = 0xe0000003 - uint32(exp)
			end++
		}
	}
	return -1
}

// reduceType4 maps the type-4 vector v of subtype
// vtype to Omega, storing the word in pgOut and
// returning its length or a negative value.
func reduceType4(v, vtype uint32, pgOut []uint32) int {
	end := 0
	for round := 0; round < 5; round++ {
		coc := (v ^ PloopTheta(v>>12)) & 0xfff
		var exp int32
		switch vtype {
		case 0x48:
			return end
		case 0x40:
			if v&0x7ffbff != 0 {
				var src [4]byte
				syn := CocodeSyndrome(coc, 0)
				copy(src[:4], VectToList(syn, 4))
				res := applyPerm(v, src[:], lstd[:], 4, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
			}
			exp = int32(2 - ((v >> 23) & 1))
			vtype = 0x48
		case 0x42, 0x44:
			if exp = xiReduceOctad(v); exp < 0 {
				res := findOctadPermutation(v, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
				if exp = xiReduceOctad(v); exp < 0 {
					return -1
				}
			}
			vtype = 0x40
		case 0x46:
			if exp = xiReduceDodecad(v); exp < 0 {
				var src [4]byte
				vect := gcodeToVectInternal(v >> 12)
				copy(src[:4], VectToList(vect, 4))
				res := applyPerm(v, src[:], lstd[:], 4, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
				if exp = xiReduceDodecad(v); exp < 0 {
					return -1
				}
			}
			vtype = 0x44
		case 0x43:
			if exp = xiReduceOddType4(v); exp < 0 {
				var src [3]byte
				tab := uint32(mat24SyndromeTable[coc&0x7ff])
				src[0] = byte(tab & 31)
				src[1] = byte((tab >> 5) & 31)
				src[2] = byte((tab >> 10) & 31)
				res := applyPerm(v, src[:], lstd[1:], 3, &pgOut[end])
				if res < 0 {
					return -1
				}
				end++
				v = uint32(res)
				if exp = xiReduceOddType4(v); exp < 0 {
					return -1
				}
			}
			vtype = 0x42 + uint32((exp&0x100)>>7)
			exp &= 3
		default:
			return -1
		}
		if exp != 0 {
			v = XiOpXi(v, int(exp))
			if v&0xfe000000 != 0 {
				return -1
			}
			pgOut[end] = 0xe0000003 - uint32(exp)
			end++
		}
	}
	return -1
}

// genLeech2ReduceType4 maps the type-4 vector v
// to the standard frame Omega, storing the word
// in pgOut and returning its length or a negative
// value.
func genLeech2ReduceType4(v uint32, pgOut []uint32) int {
	vtype := genLeech2StartType4(v)
	if vtype <= 0 {
		return int(vtype)
	}
	if (vtype >> 4) == 2 {
		return reduceType2Ortho(v^0x200, uint32(vtype), pgOut)
	}
	return reduceType4(v, uint32(vtype), pgOut)
}

// genLeech2ReduceType2Ortho maps a type-2 vector
// v orthogonal to beta to e_2+e_3, fixing beta.
func genLeech2ReduceType2Ortho(v uint32, pgOut []uint32) int {
	vtype := genLeech2StartType24(v)
	if vtype <= 0 {
		return int(vtype)
	}
	return reduceType2Ortho(v, uint32(vtype), pgOut)
}

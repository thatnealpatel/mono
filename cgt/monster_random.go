package cgt

import (
	"fmt"
	"math/rand/v2"

	"patel.codes/cgt/generator"
	"patel.codes/cgt/mat24"
)

//////////////////////////////////////////////////
// Random monster elements (structures/random_mm.py,
// dev/generators/gen_random, dev/mat24/mat24_random)
//
// MMRand / MMRandIn build a random word of monster
// generators. The mmgroup reference draws its bits
// from a byte-faithful xoshiro256** generator
// (gen_random.c); this port uses math/rand/v2
// instead, matching the rest of the package, and
// reproduces only the word-construction logic. The
// subgroup-aware M24 machinery
// (mat24.M24numRandLocal, mat24.M24numRandAdjustXY) already
// lives in mat24.go.
//////////////////////////////////////////////////

// Subgroup-description flag bits, mirroring
// SUBGOUP_MAP and the comment block in random_mm.py.
// The low 7 bits (0x7f) are the M24 RAND_* flags.
const randFlagNo = 0x20000000 // experimental subgroup marker

// randMaskAll is the mask of the low 7 M24 RAND_* flags.
// A nonzero intersection selects the shorter 5-round
// word-construction strategy (random_mm._iter_rand_mm_:
// n_rounds = 5 if flags & 0x7f else 8).
const randMaskAll = 0x7f

// subgroupFlags maps a subgroup name to its flag
// mask (random_mm.SUBGOUP_MAP).
var subgroupFlags = map[Subgroup]uint32{
	"M":    0,
	"G_x0": 0x100, "G_1": 0x100,
	"N_0": 0x200, "G_2": 0x200,
	"G_3":     0x8 | randFlagNo,
	"G_4":     0x40 | randFlagNo,
	"G_5t":    0x4 | randFlagNo,
	"G_5l":    0x10 | randFlagNo,
	"G_10":    0x2 | randFlagNo,
	"B":       0x1,
	"2E_6":    0x20 | randFlagNo,
	"N_0_e":   0x1000,
	"N_x0":    0x300,
	"N_x0_e":  0x1300,
	"Q_x0":    0x4000 | randFlagNo,
	"AutPL":   0x2000 | randFlagNo,
	"AutPL_e": 0x3000 | randFlagNo,
	"quick":   0,
}

// betaLeech2 is the standard short vector BETA in
// the Leech lattice mod 2 (random_mm.BETA).
const betaLeech2 = 0x200

//////////////////////////////////////////////////
// Public entry points
//////////////////////////////////////////////////

// mmRand builds a random monster element with the
// given number of triality rounds (<= 0 selects the
// default 8). It backs MMRand.
func mmRand(rounds int) *MM {
	w := iterRandMM(0, rounds)
	return (&MM{data: w}).Reduce()
}

// mmRandIn builds a random element of the named
// subgroup. It backs MMRandIn and panics if sub is
// not a known subgroup description.
func mmRandIn(sub Subgroup) *MM {
	flags, err := parseSubgroup(sub)
	if err != nil {
		panic("MMRandIn: " + err.Error())
	}
	w := iterRandMM(flags, 0)
	return (&MM{data: w}).Reduce()
}

// parseSubgroup resolves a subgroup description,
// which may be an '&'-separated intersection of
// names, to its flag mask (_parse_group_description).
func parseSubgroup(sub Subgroup) (uint32, error) {
	var flags uint32
	start := 0
	s := string(sub)
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] != '&' {
			continue
		}
		name := trimSpace(s[start:i])
		mask, ok := subgroupFlags[Subgroup(name)]
		if !ok {
			return 0, fmt.Errorf("unknown subgroup description %q", name)
		}
		flags |= mask
		start = i + 1
	}
	return flags, nil
}

//////////////////////////////////////////////////
// Word construction (_iter_rand_mm_)
//////////////////////////////////////////////////

// embedSmallIntoLarge augments flags so that the
// small "even"/AutPL/Q_x0 subgroups carry the flags
// of the larger group containing them
// (_embded_small_into_large).
func embedSmallIntoLarge(flags uint32) uint32 {
	if flags&0x1000 != 0 {
		flags |= 0x200
	}
	if flags&0x2000 != 0 {
		flags |= 0x300
	}
	if flags&0x4000 != 0 {
		flags |= 0x300
	}
	return flags
}

// iterRandMM builds a random word in the subgroup
// described by flags. nRounds overrides the number
// of triality rounds in the standard large-subgroup
// strategy (0 selects the default). It mirrors
// _iter_rand_mm_.
func iterRandMM(flags uint32, nRounds int) []uint32 {
	flags = embedSmallIntoLarge(flags)
	w := make([]uint32, 0, 16)

	// Element of H_0 = N_x0 \cap H.
	w = appendTagsYXDP(w, flags)
	// Extend H_0 to H_1 = <<xi> \cap H_1, H_0>.
	w = appendCosetGx0(w, flags)

	// Extend H_1 to H_2 = <<tau> \cap H_2, H_1>.
	if flags&0x100 != 0 {
		// The triality element tau is not in the group.
		return w
	}
	if flags&0x208 != 0 {
		// A fixed number of generators suffices.
		w = appendTau(w, 0, 2) // tau^randint(0,2)
		if flags&0x8 != 0 {
			// G_3: there are 3 * 7 cosets.
			cd := g3Cosets[rand.IntN(7)]
			w = appendTauPow(w, cd[0]) // tau
			w = appendXiPow(w, cd[1])  // xi
		}
		return w
	}
	// Standard strategy for large subgroups.
	if nRounds <= 0 {
		if flags&randMaskAll != 0 {
			nRounds = 5
		} else {
			nRounds = 8
		}
	}
	for i := 0; i < nRounds; i++ {
		w = appendTau(w, 1, 3) // tau^randint(1,3)
		w = appendRandTagPi(w, flags)
		w = appendCosetGx0(w, flags)
	}
	return w
}

// g3Cosets holds coset representatives of
// G_3 / (G_3 \cap G_x0); a pair (e,f) means
// tau**e * xi**f (random_mm.G3_COSETS).
var g3Cosets = [7][2]uint32{
	{0, 0}, {1, 0}, {2, 0}, {1, 1}, {1, 2}, {2, 2}, {2, 2},
}

//////////////////////////////////////////////////
// Atom emitters
//////////////////////////////////////////////////

// appendTagsYXDP appends a random element of
// N_x0 \cap H as a product of atoms with tags
// y, x, d, p (_iter_tags_yxdp).
func appendTagsYXDP(w []uint32, flags uint32) []uint32 {
	// tag y
	if flags&0x6000 == 0 {
		y := mat24.M24numRandAdjustXY(flags, uint32(rand.IntN(0x2000)))
		if y != 0 {
			w = append(w, tagY+y)
		}
	}
	// tag x
	if flags&0x2000 == 0 {
		x := mat24.M24numRandAdjustXY(flags, uint32(rand.IntN(0x2000)))
		if x != 0 {
			w = append(w, tagX+x)
		}
	}
	// tag d
	d := uint32(rand.IntN(0x1000))
	if flags&0x1000 != 0 {
		d &= 0x7ff
	}
	if d != 0 {
		w = append(w, tagD+d)
	}
	// tag p
	if flags&0x4000 == 0 {
		w = appendRandTagPi(w, flags)
	}
	return w
}

// appendRandTagPi appends a single random p (pi)
// atom, subject to the subgroup flags
// (_random_tag_pi). It is a no-op when the
// permutation part is fixed (flags & 0x4000).
func appendRandTagPi(w []uint32, flags uint32) []uint32 {
	if flags&0x4000 != 0 {
		return w
	}
	pi := mat24.M24numRandLocal(flags, uint32(rand.IntN(mat24.Mat24Order)))
	if pi < 0 {
		panic("MMRand: M24numRandLocal failed")
	}
	if pi != 0 {
		w = append(w, tagP+uint32(pi))
	}
	return w
}

// appendTau appends a tau atom with exponent
// drawn uniformly from [lo, hi]. A zero exponent
// is omitted.
func appendTau(w []uint32, lo, hi uint32) []uint32 {
	e := lo + uint32(rand.IntN(int(hi-lo+1)))
	return appendTauPow(w, e)
}

// appendTauPow appends tau^e (tag t), omitting the
// neutral atom.
func appendTauPow(w []uint32, e uint32) []uint32 {
	if e%3 != 0 {
		return append(w, tagT+e%3)
	}
	return w
}

// appendXiPow appends xi^e (tag l), omitting the
// neutral atom.
func appendXiPow(w []uint32, e uint32) []uint32 {
	if e%3 != 0 {
		return append(w, tagL+e%3)
	}
	return w
}

// appendCosetGx0 appends a coset representative of
// H_1 / H_0, extending an N_x0 element to a G_x0
// element (_iter_coset_G_x0).
func appendCosetGx0(w []uint32, flags uint32) []uint32 {
	if flags&0x200 != 0 {
		// Generator xi is not in the subgroup.
		return w
	}
	relevant := flags & 0x7f
	switch {
	case relevant&0x8 != 0:
		// G_3' = G_3 \cap G_x0, with |G_3' / G_3' \cap N_x0| = 3.
		w = appendXiPow(w, uint32(rand.IntN(3)))
	case relevant&0xfe == 0:
		// H = 2.B \cap G_x0 or H = G_x0; append a word in
		// G_x0 mapping Omega to a suitable type-4 vector.
		var c uint32
		if relevant != 0 {
			c = randCo2CosetNo()
		} else {
			c = randType4Vector()
		}
		w = appendReduceType4Inv(w, c)
	default:
		// No good strategy for the remaining cases.
		for i := 0; i < 4; i++ {
			w = appendXiPow(w, 1+uint32(rand.IntN(2))) // nonzero power of xi
			w = appendRandTagPi(w, flags)
		}
	}
	return w
}

// appendReduceType4Inv appends the inverse of a
// G_x0 word that maps the type-4 vector c to the
// standard frame Omega; the inverse therefore maps
// Omega to c. It mirrors the
// gen_leech2_reduce_type4 + mm_group_invert_word
// idiom.
func appendReduceType4Inv(w []uint32, c uint32) []uint32 {
	var a [6]uint32
	n := genLeech2ReduceType4(c, a[:])
	if n < 0 {
		panic("MMRand: gen_leech2_reduce_type4 failed")
	}
	word := a[:n]
	invertWord(word)
	return append(w, word...)
}

// randType4Vector returns a uniform random type-4
// vector in the Leech lattice mod 2.
func randType4Vector() uint32 {
	for {
		c := uint32(rand.IntN(0x1000000))
		if generator.Leech2Type(c) == 4 {
			return c
		}
	}
}

// randCo2CosetNo returns a random coset
// representative of Co_2 / (Co_2 \cap N_x0): a
// type-4 vector that, xored with BETA, stays of
// type 4 (i.e. is orthogonal to BETA in the real
// Leech lattice). Mirrors _rand_Co_2_coset_No.
func randCo2CosetNo() uint32 {
	for {
		ve := 300 + rand.IntN(98579-300+1) // randint(300, 98579)
		vs := IndexExternToSparse(ve)
		// TODO(nealpatel): re-evaluate after porting;
		// IndexSparseToLeech2 returns 0 on failure.
		// Input ve ∈ [300, 98579] is always a valid
		// extern index for tags B/C/T/X, so the
		// extern→sparse→leech2 chain never fails.
		// Even if it did, v2=0 gives v4=betaLeech2
		// (type 2), which the generator.Leech2Type==4 guard
		// rejects and the loop retries. C origin
		// _rand_Co_2_coset_No (random_mm.py:194)
		// also ignores the sentinel.
		v2 := IndexSparseToLeech2(vs)
		v4 := v2 ^ betaLeech2
		if generator.Leech2Type(v4) == 4 {
			return v4
		}
	}
}

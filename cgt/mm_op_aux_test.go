package cgt

import "testing"

// Tests for the vector hash helper in mm_op_aux.go,
// grounded in the canonical mmgroup test
//   mmgroup/tests/test_mm_op/test_mm_aux.py
//     (test_mm_aux_hash)
//
// The hash values mirror mm_op.mm_aux_hash over the seeded
// random vector mm_op.mm_aux_random_mmv(seed), taken from
// the C reference via `goof mmgroup.py`. The seeded-vector
// hashes are exact because RandVectorSeed reproduces
// gen_rng_seed_no + mm_aux_random_mmv bit-for-bit.
// (MMVSize is covered separately by TestMMVSize.)

// TestHashValue checks the exact hash of a deterministic
// seeded random vector against the C reference
// (mm_aux_hash over mm_aux_random_mmv), including the skip
// bitmask that suppresses tags of "ABCTXZY".
func TestHashValue(t *testing.T) {
	t.Parallel()
	type tc struct {
		p, seed int
		skip    int
		want    uint64
	}
	cases := []tc{
		{3, 0, 0, 4106482596069746059},
		{3, 42, 0, 13708017066054123374},
		{7, 0, 0, 7219013961262465684},
		{7, 42, 0, 14876597146705413720},
		{15, 0, 0, 14806124310070432544},
		{15, 42, 0, 14343978702739038629},
		{31, 0, 0, 12749646401622857866},
		{31, 42, 0, 7814801404175945973},
		{127, 0, 0, 16871417589197860645},
		{127, 42, 0, 4297056967465021507},
		{255, 0, 0, 16634950645567936691},
		{255, 42, 0, 2217482585751475756},
		// skip bitmask cases (ignore selected tags)
		{3, 42, 1, 10483866095174937002},
		{3, 42, 0b1111111, 715500125602632940},
		{15, 42, 1, 12140385966722341387},
		{15, 42, 0b1111111, 6805158455505408492},
		{127, 42, 1, 17661395782309157234},
		{127, 42, 0b1111111, 14450652004705842668},
	}
	for _, c := range cases {
		v := RandVectorSeed(c.p, uint64(c.seed))
		if got := hash(c.p, v.Data(), c.skip); got != c.want {
			t.Errorf("hash(p=%d, seed=%d, skip=%#x)=%d want %d", c.p, c.seed, c.skip, got, c.want)
		}
		if c.skip == 0 {
			// MMVector.Hash() is the skip=0 convenience method.
			if got := v.Hash(); got != c.want {
				t.Errorf("RandVectorSeed(%d,%d).Hash()=%d want %d", c.p, c.seed, got, c.want)
			}
		}
	}
}

// TestHashEqualVectors mirrors test_mm_aux_hash: two
// vectors that are equal modulo p must hash identically,
// even when their internal representations differ. The
// value p (all field bits set) and the value 0 both
// represent 0 modulo p, so masking some fields of one
// copy to p and the corresponding fields of the other to
// 0 yields equal vectors that the hash must not
// distinguish.
func TestHashEqualVectors(t *testing.T) {
	t.Parallel()
	// FIELD_BITS per modulus, from mm_basics MM_Basics.sizes.
	fieldBits := map[int]uint{3: 2, 7: 4, 15: 4, 31: 8, 63: 8, 127: 8, 255: 8}
	for _, p := range []int{3, 7, 15, 31, 127, 255} {
		v1 := RandVectorSeed(p, 7)
		v2 := v1.Copy()
		d1 := v1.Data()
		d2 := v2.Data()
		fb := fieldBits[p]
		nFields := 64 / fb
		pmask := uint64(p)
		for j := 0; j < len(d1); j += 7 {
			field := uint(j) % nFields
			mask := pmask << (field * fb)
			d1[j] |= mask  // field -> all-ones == p == 0 (mod p)
			d2[j] &^= mask // same field -> 0
		}
		if !v1.Equal(v2) {
			t.Fatalf("p=%d: masked copies are not equal mod p", p)
		}
		if h1, h2 := v1.Hash(), v2.Hash(); h1 != h2 {
			t.Errorf("p=%d: equal vectors hash differently: %d vs %d", p, h1, h2)
		}
	}
}

package cgt

import "testing"

// Tests for the xoshiro256** generator (rng.go),
// grounded in the gen_rng_* family exercised by
//   mmgroup/tests/test_gen_xi/test_gen_rng.py
//
// Go's Rng is documented as bit-for-bit identical to the
// C reference for a given seed (gen_rng_seed_no,
// gen_rng_bytes_modp, gen_rng_modp). The expected
// streams below were produced by that C reference via
// `goof mmgroup.py`:
//   seed = np.zeros(4, dtype=np.uint64)
//   gen_rng_seed_no(seed, seedNo)
//   gen_rng_bytes_modp(15, b, 20, seed)   # then
//   gen_rng_modp(p, seed) for p in [3,100,65521,0x10000]
// The byte-faithfulness is exactly the property that
// makes RandVector oracle-testable.

func eqU8(a []uint8, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func eqU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// rngWant holds the C reference output for a seed: the
// first 20 bytes mod 15, followed by single draws mod
// 3, 100, 65521 and 2^16, consumed from the same stream
// in that order.
type rngWant struct {
	bytesModP15 []uint8
	modP        []uint32 // for p = 3, 100, 65521, 0x10000
}

var rngModPParams = []uint32{3, 100, 65521, 0x10000}

var rngCases = map[uint64]rngWant{
	0: {
		[]uint8{9, 0, 4, 3, 14, 1, 5, 11, 3, 3, 11, 0, 14, 4, 1, 8, 2, 10, 5, 12},
		[]uint32{1, 73, 65504, 3566},
	},
	1: {
		[]uint8{10, 8, 2, 5, 6, 4, 0, 7, 12, 1, 7, 1, 8, 5, 8, 9, 2, 9, 1, 7},
		[]uint32{1, 69, 9406, 1263},
	},
	42: {
		[]uint8{1, 3, 13, 0, 8, 6, 9, 5, 10, 4, 0, 13, 1, 13, 10, 3, 0, 2, 2, 14},
		[]uint32{2, 99, 50434, 21637},
	},
	12345: {
		[]uint8{11, 2, 5, 5, 4, 4, 12, 1, 14, 3, 13, 8, 4, 13, 14, 6, 11, 3, 12, 1},
		[]uint32{0, 55, 699, 10831},
	},
}

// TestRngStream verifies that NewRngSeed, BytesModP and
// ModP reproduce the C reference stream bit-for-bit. The
// draws are consumed in the same order as in the oracle
// (BytesModP(15) of length 20, then ModP for each
// modulus), exercising the seeded generator end to end.
func TestRngStream(t *testing.T) {
	t.Parallel()
	for seedNo, want := range rngCases {
		r := NewRngSeed(seedNo)
		b := make([]uint8, len(want.bytesModP15))
		r.BytesModP(15, b)
		if !eqU8(b, want.bytesModP15) {
			t.Errorf("seed %d: BytesModP(15)=%v want %v", seedNo, b, want.bytesModP15)
		}
		got := make([]uint32, len(rngModPParams))
		for i, p := range rngModPParams {
			got[i] = r.ModP(p)
		}
		if !eqU32(got, want.modP) {
			t.Errorf("seed %d: ModP(%v)=%v want %v", seedNo, rngModPParams, got, want.modP)
		}
	}
}

// TestRngBytesModPRange checks that BytesModP fills the
// buffer with values strictly below the modulus, for
// both power-of-two and non-power-of-two moduli, and
// that NewRng yields a usable (non-all-zero) generator.
func TestRngBytesModPRange(t *testing.T) {
	t.Parallel()
	r := NewRng()
	for _, p := range []int{2, 3, 15, 16, 100, 255, 256} {
		out := make([]uint8, 64)
		r.BytesModP(p, out)
		for i, x := range out {
			if int(x) >= p {
				t.Errorf("BytesModP(%d): out[%d]=%d not < %d", p, i, x, p)
			}
		}
	}
}

// TestRngSeedDeterministic checks that two generators
// seeded with the same number produce identical streams,
// and that distinct seeds diverge.
func TestRngSeedDeterministic(t *testing.T) {
	t.Parallel()
	a := NewRngSeed(99)
	b := NewRngSeed(99)
	var same, diff bool
	for i := 0; i < 32; i++ {
		if a.ModP(0) != b.ModP(0) {
			t.Fatalf("same-seed generators diverged at draw %d", i)
		}
	}
	same = true
	c := NewRngSeed(100)
	d := NewRngSeed(101)
	for i := 0; i < 32; i++ {
		if c.ModP(0) != d.ModP(0) {
			diff = true
			break
		}
	}
	if !same || !diff {
		t.Errorf("seed determinism: same=%v diff=%v", same, diff)
	}
}

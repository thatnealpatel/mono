package generator

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/bits"
)

//////////////////////////////////////////////////
// Deterministic random generator core
// (dev/generators/gen_random, c_files/gen_random.c)
//
// This is the byte-faithful xoshiro256** generator
// used by mmgroup, seeded by splitmix64. Only the
// deterministic core is ported: the MD5 entropy
// mixing, OS-specific seeding (gen_rng_seed_init /
// gen_rng_seed), and pthread/Windows locking are
// not. Go draws entropy from crypto/rand instead.
//
// The constants and state transitions match the C
// exactly, so a Rng from NewRngSeed reproduces the
// mmgroup byte stream and supports bit-exact oracle
// testing against gen_rng_bytes_modp.
//////////////////////////////////////////////////

// Rng is a xoshiro256** generator. The zero value is
// not a valid generator (xoshiro256** must not be
// seeded all-zero); construct one with NewRng or
// NewRngSeed.
type Rng struct {
	s [4]uint64
}

// NewRngSeed returns a generator seeded
// deterministically from seedNo, reproducing the
// mmgroup stream for that seed number. C
// gen_rng_seed_no.
func NewRngSeed(seedNo uint64) *Rng {
	r := &Rng{}
	r.seedNo(seedNo)
	return r
}

// seedNo fills the state with four splitmix64 outputs
// driven by seedNo. C gen_rng_seed_no.
func (r *Rng) seedNo(seedNo uint64) {
	for i := 0; i < 4; i++ {
		seedNo += 0x9e3779b97f4a7c15
		z := seedNo
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		r.s[i] = z ^ (z >> 31)
	}
}

// NewRng returns a generator seeded from system
// entropy via crypto/rand. This replaces the
// MD5/OS-specific master-seed path in the C, which is
// not ported. It panics if the system entropy source
// (crypto/rand) is unavailable.
func NewRng() *Rng {
	var b [32]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic("generator: crypto/rand failed: " + err.Error())
	}
	r := &Rng{}
	for i := 0; i < 4; i++ {
		r.s[i] = binary.LittleEndian.Uint64(b[i*8:])
	}
	// xoshiro256** degenerates from an all-zero state;
	// crypto/rand makes that astronomically unlikely,
	// but guard against it deterministically.
	if r.s[0]|r.s[1]|r.s[2]|r.s[3] == 0 {
		r.s[0] = 1
	}
	return r
}

// next advances the state and returns the next 64-bit
// output. C xoro_next.
func (r *Rng) next() uint64 {
	s := &r.s
	result := bits.RotateLeft64(s[1]*5, 7) * 9

	t := s[1] << 17

	s[2] ^= s[0]
	s[3] ^= s[1]
	s[1] ^= s[2]
	s[0] ^= s[3]

	s[2] ^= t

	s[3] = bits.RotateLeft64(s[3], 45)

	return result
}

// jump advances the state by 2^128 calls to next,
// yielding a non-overlapping subsequence. C
// xoro_jump.
func (r *Rng) jump() {
	jump := [4]uint64{
		0x180ec6d33cfd0aba, 0xd5a61266f0c9392c,
		0xa9582618e03fc9aa, 0x39abdc4529b1661c,
	}
	var s0, s1, s2, s3 uint64
	for i := 0; i < len(jump); i++ {
		for b := 0; b < 64; b++ {
			if jump[i]&(uint64(1)<<b) != 0 {
				s0 ^= r.s[0]
				s1 ^= r.s[1]
				s2 ^= r.s[2]
				s3 ^= r.s[3]
			}
			r.next()
		}
	}
	r.s[0] = s0
	r.s[1] = s1
	r.s[2] = s2
	r.s[3] = s3
}

// BytesModP fills out with len(out) uniform random
// bytes x with 0 <= x < p. It requires 1 < p <= 256
// and panics otherwise. C gen_rng_bytes_modp.
//
// The byte stream is bit-for-bit identical to the C
// for a given seed, which is what makes RandVector
// oracle-testable.
func (r *Rng) BytesModP(p int, out []uint8) {
	if p < 2 || p > 256 {
		panic("generator: BytesModP requires 1 < p <= 256")
	}
	length := len(out)
	pos := 0
	if p&(p-1) == 0 {
		// p is a power of two: mask off the low bits.
		mask := uint64(p - 1)
		for length >= 8 {
			v := r.next()
			out[pos+0] = uint8((v >> 0) & mask)
			out[pos+1] = uint8((v >> 8) & mask)
			out[pos+2] = uint8((v >> 16) & mask)
			out[pos+3] = uint8((v >> 24) & mask)
			out[pos+4] = uint8((v >> 32) & mask)
			out[pos+5] = uint8((v >> 40) & mask)
			out[pos+6] = uint8((v >> 48) & mask)
			out[pos+7] = uint8((v >> 56) & mask)
			pos += 8
			length -= 8
		}
		v := r.next()
		for length > 0 {
			out[pos] = uint8(v & mask)
			v >>= 8
			pos++
			length--
		}
		return
	}
	if p < 16 {
		// 0 < p < 16: extract seven base-p digits from
		// the top nibbles of one 60-bit draw, consuming
		// seven output bytes per draw. (The C unrolls an
		// eighth multiply that writes a dead byte at
		// out[7] before advancing by 7; that byte never
		// survives, so we omit it and stay bit-exact.)
		const lowMask = 0x0fffffffffffffff
		for length >= 7 {
			v := r.next() >> 4
			v *= uint64(p)
			out[pos+0] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+1] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+2] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+3] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+4] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+5] = uint8(v >> 60)
			v &= lowMask
			v *= uint64(p)
			out[pos+6] = uint8(v >> 60)
			pos += 7
			length -= 7
		}
		v := r.next() >> 4
		for length > 0 {
			v *= uint64(p)
			out[pos] = uint8(v >> 60)
			v &= lowMask
			pos++
			length--
		}
		return
	}
	// 16 <= p < 256: pack one byte per top byte of a
	// repeated multiply-by-p, three bytes per draw.
	const lowMask = 0x00ffffffffffffff
	for length >= 3 {
		v := r.next() >> 8
		v *= uint64(p)
		out[pos+0] = uint8(v >> 56)
		v &= lowMask
		v *= uint64(p)
		out[pos+1] = uint8(v >> 56)
		v &= lowMask
		v *= uint64(p)
		out[pos+2] = uint8(v >> 56)
		pos += 3
		length -= 3
	}
	v := r.next() >> 8
	for length > 0 {
		v *= uint64(p)
		out[pos] = uint8(v >> 56)
		v &= lowMask
		pos++
		length--
	}
}

// ModP returns a single uniform random integer x with
// 0 <= x < p, for a 32-bit modulus p. p == 0 is
// interpreted as 2^32. C gen_rng_modp.
func (r *Rng) ModP(p uint32) uint32 {
	v := r.next()
	if p&(p-1) == 0 {
		return uint32((v >> 32) & uint64(p-1))
	}
	v = (v>>32)*uint64(p) + (((v & 0xffffffff) * uint64(p)) >> 32)
	return uint32(v >> 32)
}

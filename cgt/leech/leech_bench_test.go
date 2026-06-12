package leech

import (
	"math/rand/v2"
	"testing"
)

// Baseline benchmarks for the leech2 many-op hot path
// (task Td4): applying a fixed G_x0 word to a large pool of
// Leech-lattice-mod-2 vectors. GenLeech2OpWordMany tracks
// signs; GenLeech2OpWordLeech2Many is the sign-free
// variant. Both run the per-vector Leech2OpAtom inner loop
// over the whole pool, which is the cost the I-track
// refactors are measured against.
//
// Optimization is out of scope; this only pins the
// baseline. The word and the vector pool are built
// deterministically (seeded ChaCha8, fixed type-4 seed) so
// the numbers reproduce.

// benchLeechWord is a real G_x0 word: the reduction word
// that maps a fixed type-4 vector to the standard frame
// Omega. Using a genuine reduce-word exercises the p/y/l
// atom dispatch rather than a single trivial atom.
func benchLeechWord(b *testing.B) []uint32 {
	b.Helper()
	const v4 = 0x800000 // a type-4 (frame) vector
	g := make([]uint32, 12)
	n := GenLeech2ReduceType4(v4, g)
	if n < 0 {
		b.Fatalf("GenLeech2ReduceType4(%#x) = %d", v4, n)
	}
	return g[:n]
}

// benchLeechPool returns count random Leech-mod-2 vectors
// drawn deterministically from a seeded stream, masked to
// the 25-bit domain GenLeech2OpWordMany operates on.
func benchLeechPool(count int) []uint32 {
	var seed [32]byte
	seed[0] = 0xd4
	seed[1] = 0x4c
	r := rand.New(rand.NewChaCha8(seed))
	q := make([]uint32, count)
	for i := range q {
		q[i] = r.Uint32() & 0x1ffffff
	}
	return q
}

// BenchmarkGenLeech2OpWordMany measures the sign-tracking
// many-op application over a pool of vectors. The pool is
// mutated in place, so a pristine copy is restored before
// each timed application.
func BenchmarkGenLeech2OpWordMany(b *testing.B) {
	g := benchLeechWord(b)
	const pool = 1024
	src := benchLeechPool(pool)
	q := make([]uint32, pool)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(q, src)
		if n := GenLeech2OpWordMany(q, g); n != len(g) {
			b.Fatalf("GenLeech2OpWordMany applied %d of %d atoms", n, len(g))
		}
	}
}

// BenchmarkGenLeech2OpWordLeech2Many measures the sign-free
// many-op application over a pool of vectors, mutated in
// place and restored before each timed application.
func BenchmarkGenLeech2OpWordLeech2Many(b *testing.B) {
	g := benchLeechWord(b)
	const pool = 1024
	src := benchLeechPool(pool)
	a := make([]uint32, pool)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(a, src)
		if rc := GenLeech2OpWordLeech2Many(a, g, false); rc != 0 {
			b.Fatalf("GenLeech2OpWordLeech2Many returned %d", rc)
		}
	}
}

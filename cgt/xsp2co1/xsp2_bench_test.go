package xsp2co1

import (
	"math/rand/v2"
	"testing"

	"patel.codes/cgt/mat24"
)

// Baseline benchmarks for the G_x0 group ops (task Td4).
// Xsp2Co1.Mul is the qstate12-multiply hot path: it builds
// two Clifford-group state matrices and composes them with
// QsMatmul, which dominates the per-element cost of every
// G_x0 word reduction. Xsp2Co1.HalfOrder is benchmarked
// alongside it because MM.HalfOrder bottoms out here.
//
// Optimization is out of scope; this only pins the
// baseline. Inputs are built deterministically from a
// seeded ChaCha8 stream so the numbers reproduce.

// benchRandGx0Elem builds a random element of G_{x0} as a
// short product of random d, p, x, y and l (xi) generator
// atoms, drawing from r so the sequence is deterministic.
// It mirrors randGx0Elem but takes an explicit rng for
// reproducibility.
func benchRandGx0Elem(r *rand.Rand) *Xsp2Co1 {
	n := 4 + r.IntN(4)
	atoms := make([]XspAtom, 0, n)
	for i := 0; i < n; i++ {
		switch r.IntN(5) {
		case 0:
			atoms = append(atoms, XspAtom{"d", r.IntN(0x1000)})
		case 1:
			atoms = append(atoms, XspAtom{"p", int(mat24.M24numRandLocal(0, r.Uint32()))})
		case 2:
			atoms = append(atoms, XspAtom{"x", r.IntN(0x2000)})
		case 3:
			atoms = append(atoms, XspAtom{"y", r.IntN(0x2000)})
		case 4:
			atoms = append(atoms, XspAtom{"l", 1 + r.IntN(2)})
		}
	}
	return NewXsp2Co1(atoms...)
}

// benchRng returns a deterministic ChaCha8-backed rng for
// the benchmarks in this file.
func benchRng() *rand.Rand {
	var seed [32]byte
	seed[0] = 0xd4
	return rand.New(rand.NewChaCha8(seed))
}

// BenchmarkXsp2Co1Mul measures the qstate12 multiply used
// by G_x0 group ops: Xsp2Co1.Mul converts both operands to
// QState12 Clifford matrices and composes them via
// QsMatmul. A fixed pool of random elements is multiplied
// in a rotating pattern so the timed work is realistic and
// reproducible.
func BenchmarkXsp2Co1Mul(b *testing.B) {
	r := benchRng()
	const pool = 16
	elems := make([]*Xsp2Co1, pool)
	for i := range elems {
		elems[i] = benchRandGx0Elem(r)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := elems[i%pool]
		h := elems[(i+1)%pool]
		_ = g.Mul(h)
	}
}

// BenchmarkXsp2Co1HalfOrder measures Xsp2Co1.HalfOrder over
// a fixed pool of random G_x0 elements, the routine that
// MM.HalfOrder delegates to once the element is in G_x0.
func BenchmarkXsp2Co1HalfOrder(b *testing.B) {
	r := benchRng()
	const pool = 16
	elems := make([]*Xsp2Co1, pool)
	for i := range elems {
		elems[i] = benchRandGx0Elem(r)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o, _ := elems[i%pool].HalfOrder()
		if o == 0 {
			b.Fatal("HalfOrder returned order 0")
		}
	}
}

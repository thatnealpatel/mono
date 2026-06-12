package monster

import "testing"

// Baseline benchmarks for the monster-level hot paths
// (task Td4). These pin the cost of the OpWord inner
// loops and MM.HalfOrder so the I-track refactors can be
// measured against a fixed reference. Optimization is out
// of scope here: this file only establishes the baseline.
//
// All inputs are constructed oracle-free and
// deterministically (fixed seeds / fixed word strings) so
// the numbers are reproducible across runs and machines.

// benchOpWordWords are representative monster words used by
// the OpWord benchmark. The last entry mixes every block
// the genOpWord dispatch can hit (xi via l, tau via t, xy
// via x/y, and pi/delta via p/d), which is the case the
// HalfOrder reduceViaPower loop drives repeatedly.
var benchOpWordWords = []struct {
	name string
	word string
}{
	{"pi_delta", "M<d_837h*p_217821225>"},
	{"x_y_d", "M<x_123h*y_5h*d_456h>"},
	{"tau", "M<t_1>"},
	{"mixed", "M<x_1h*y_2h*t_1*l_2*p_100>"},
}

// BenchmarkOpWord measures OpWord at p=15 (the modulus the
// G_x0 machinery runs at) applying g^1 to a random vector.
// OpWord mutates v in place and uses work as scratch, so
// each iteration restores a pristine copy of the input
// vector before timing the application.
func BenchmarkOpWord(b *testing.B) {
	const p = 15
	v0 := RandVectorSeed(p, 0x5eed)
	for _, c := range benchOpWordWords {
		mm, err := NewMM(c.word)
		if err != nil {
			b.Fatalf("NewMM(%q): %v", c.word, err)
		}
		g := mm.Mmdata()
		b.Run(c.name, func(b *testing.B) {
			v := make([]uint64, len(v0.Data()))
			work := make([]uint64, len(v0.Data()))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(v, v0.Data())
				if err := OpWord(p, v, g, len(g), 1, work); err != nil {
					b.Fatalf("OpWord: %v", err)
				}
			}
		})
	}
}

// benchHalfOrderWords are representative monster elements
// for the HalfOrder benchmark, mirroring the cases pinned
// by TestMonsterHalfOrder. The mixed t/l/p element drives
// the orderElementGx0 / Xsp2Co1.HalfOrder path that the G4
// residual flags as OpWord-dominated.
var benchHalfOrderWords = []struct {
	name string
	word string
}{
	{"d_only", "M<d_2h*d_3h>"},
	{"x_only", "M<x_1000h>"},
	{"mixed", "M<x_1h*y_2h*t_1*l_2*p_100>"},
}

// BenchmarkMMHalfOrder measures MM.HalfOrder, whose
// repeated OpWord applications dominate reduceViaPower and
// conjugateInvolution trials. HalfOrder reduces its
// receiver in place; the receiver is rebuilt from the word
// each iteration so the timed work is identical every run.
func BenchmarkMMHalfOrder(b *testing.B) {
	for _, c := range benchHalfOrderWords {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				g, err := NewMM(c.word)
				if err != nil {
					b.Fatalf("NewMM(%q): %v", c.word, err)
				}
				b.StartTimer()
				o, _ := g.HalfOrder()
				if o == 0 {
					b.Fatalf("HalfOrder(%q) returned order 0", c.word)
				}
			}
		})
	}
}

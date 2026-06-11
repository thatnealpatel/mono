package cgt

import "testing"

// TestSimplifyShortWordNoop checks that Simplify leaves
// a word with few triality atoms unchanged, mirroring
// the weight<=9 early return of MM.simplify. This path
// is cheap (no HalfOrder / conjugation work), so it
// runs in the normal suite.
//
// The shortening path of Simplify / reduceViaPower is
// not exercised here: it drives MM.HalfOrder and
// involution conjugation, each many seconds per call,
// which would balloon the suite well past its budget.
// Value-preservation of reduceViaPower is verified
// against the mmgroup oracle (reduce_via_power).
func TestSimplifyShortWordNoop(t *testing.T) {
	t.Parallel()
	g := mustMM(t, "M<t_1*l_1*t_2>")
	g.Reduce()
	before := append([]uint32(nil), g.data...)
	g.Simplify(40)
	if len(g.data) != len(before) {
		t.Fatalf("Simplify altered a short word: len %d -> %d",
			len(before), len(g.data))
	}
	for i := range before {
		if g.data[i] != before[i] {
			t.Fatalf("Simplify altered a short word at atom %d", i)
		}
	}
}

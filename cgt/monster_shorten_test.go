package cgt

import (
	"fmt"
	"testing"
)

// shortenSetup defines Python helpers that call the C
// staged reduction entry points and return their output
// arrays, for exact array-level parity against the Go
// ports.
const shortenSetup = "from mmgroup.mm_reduce import gt_word_shorten, mm_reduce_vector_shortcut\n" +
	"def shorten(g, mode):\n" +
	" a=np.array(g,dtype=np.uint32); o=np.zeros(0x1000,dtype=np.uint32)\n" +
	" k=int(gt_word_shorten(a, len(a), o, len(o), mode))\n" +
	" return [k] + ([int(x) for x in o[:k]] if k>=0 else [])\n" +
	"def shortcut(stage, mode, axis):\n" +
	" r=np.zeros(200,dtype=np.uint32)\n" +
	" k=int(mm_reduce_vector_shortcut(stage, mode, axis, r))\n" +
	" return [k] + ([int(x) for x in r[:k]] if k>=0 else [])\n"

// randomShortenWords returns reproducible random monster
// words (raw atom arrays) spanning empty, N_0, G_x0, and
// general elements.
func randomShortenWords(t *testing.T) [][]uint32 {
	t.Helper()
	var words [][]uint32
	// A handful of fixed words covering subgroup cases.
	for _, s := range []string{
		"M<t_1*l_1*l_2*t_2>",
		"M<l_1*t_1*t_2*l_2>",
		"M<l_1*t_2>",
		"M<d_2h*x_1h*y_3h>",
		"M<x_1h*y_2h*t_1*l_2*p_100*l_1*t_2>",
		"M<l_1*t_2*l_2*t_1*x_3abh*d_4h*y_5h*p_200>",
	} {
		g, err := NewMM(s)
		if err != nil {
			t.Fatalf("NewMM(%q): %v", s, err)
		}
		words = append(words, append([]uint32(nil), g.data...))
	}
	// Random words at increasing complexity.
	for rounds := 0; rounds < 5; rounds++ {
		for k := 0; k < 3; k++ {
			g := MMRand(rounds)
			words = append(words, append([]uint32(nil), g.data...))
		}
	}
	return words
}

func TestGtWordShortenOracle(t *testing.T) {
	t.Parallel()
	const maxOut = 0x1000
	for _, g := range randomShortenWords(t) {
		for _, mode := range []int{1} {
			out := make([]uint32, maxOut)
			got := gtWordShorten(append([]uint32(nil), g...), out, maxOut, mode)
			want := oracleLeech(t, shortenSetup,
				fmt.Sprintf("shorten(%s, %d)", u32List(g), mode))
			if len(want) == 0 {
				t.Fatalf("empty oracle result for %s mode %d", u32List(g), mode)
			}
			wantLen := int(int32(uint32(want[0])))
			if got != wantLen {
				t.Errorf("gtWordShorten(%s, mode=%d) len=%d want %d",
					u32List(g), mode, got, wantLen)
				continue
			}
			if got < 0 {
				continue
			}
			for i := 0; i < got; i++ {
				if out[i] != uint32(want[1+i]) {
					t.Errorf("gtWordShorten(%s, mode=%d)[%d]=%#x want %#x",
						u32List(g), mode, i, out[i], uint32(want[1+i]))
					break
				}
			}
			// The shortened word must equal g as a monster
			// element.
			gMM := &MM{data: append([]uint32(nil), g...)}
			short := &MM{data: append([]uint32(nil), out[:got]...)}
			if !gMM.Equal(short) {
				t.Errorf("gtWordShorten(%s, mode=%d) result not equal to input",
					u32List(g), mode)
			}
		}
	}
}

func TestMmReduceVectorShortcutOracle(t *testing.T) {
	t.Parallel()
	axes := []uint32{vPlus, vMinus, 0x200, 0x1000200, 0x123456, 0x1abcdef}
	for _, stage := range []uint32{1, 2} {
		for _, mode := range []uint32{0, 1} {
			for _, axis := range axes {
				r := make([]uint32, 200)
				got := mmReduceVectorShortcut(stage, mode, axis, r)
				want := oracleLeech(t, shortenSetup,
					fmt.Sprintf("shortcut(%d, %d, %d)", stage, mode, axis))
				if len(want) == 0 {
					t.Fatalf("empty oracle result for shortcut(%d,%d,%#x)",
						stage, mode, axis)
				}
				wantLen := int(int32(uint32(want[0])))
				if got != wantLen {
					t.Errorf("mmReduceVectorShortcut(%d,%d,%#x) len=%d want %d",
						stage, mode, axis, got, wantLen)
					continue
				}
				if got < 0 {
					continue
				}
				for i := 0; i < got; i++ {
					if r[i] != uint32(want[1+i]) {
						t.Errorf("mmReduceVectorShortcut(%d,%d,%#x)[%d]=%#x want %#x",
							stage, mode, axis, i, r[i], uint32(want[1+i]))
						break
					}
				}
			}
		}
	}
}

// TestMmReduceVectorShortenParity drives the full staged
// sequence (map axis -> vp -> map axis -> vm -> shorten)
// and checks that, when the shorten fast path succeeds,
// the resulting word equals the input as a monster
// element and matches the canonical reduceM output as a
// group element.
func TestMmReduceVectorShortenParity(t *testing.T) {
	t.Parallel()
	for _, g := range randomShortenWords(t) {
		a := append([]uint32(nil), g...)
		n := len(a)
		v := ZeroVector(15)
		work := ZeroVector(15)
		r := make([]uint32, 256)

		vp := uint32(vPlus)
		if res := mmReduceMapAxis(&vp, v.data, a, n, work.data); res < 0 {
			continue
		}
		if res := mmReduceVectorVP(vp, v.data, 0, r, work.data); res < 0 {
			continue
		}
		vm := uint32(vMinus)
		if res := mmReduceMapAxis(&vm, v.data, a, n, work.data); res < 0 {
			continue
		}
		if res := mmReduceVectorVm(&vm, v.data, r, work.data); res < 0 {
			continue
		}
		res := mmReduceVectorShorten(append([]uint32(nil), a...), r)
		if res < 0 {
			// Fast path declined; not an error.
			continue
		}
		short := &MM{data: append([]uint32(nil), r[:res]...)}
		gMM := &MM{data: append([]uint32(nil), g...)}
		if !gMM.Equal(short) {
			t.Errorf("mmReduceVectorShorten result not equal to input for %s",
				u32List(g))
		}
		canon := reduceM(append([]uint32(nil), g...))
		if canon == nil {
			t.Fatalf("reduceM failed for %s", u32List(g))
		}
		if !short.Equal(&MM{data: canon}) {
			t.Errorf("mmReduceVectorShorten disagrees with reduceM for %s",
				u32List(g))
		}
	}
}

package reduce

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

// Oracle-parity tests for the monster-word reduction and
// integer-encoding engine (mm_shorten.c, mm_compress.c).
// The reduce package never imports flat cgt, so the
// reduction/compression of raw atom words is the natural
// package boundary; the matching mm-level tests in flat cgt
// drive the same engines through *MM. Here we feed raw
// []uint32 words directly:
//
//   ReduceWord       <-> xsp2co1_reduce_word
//   GtWordShorten    <-> gt_word_shorten
//   CompressAsInt    <-> GtWord(...).as_int() (low 64-bit digit)
//   ExpandInt        <-> mm_compress_pc_expand_int (low digit)
//
// Words are built from a deterministic generator so the
// oracle calls are reproducible.

const reduceImports = "import json, numpy as np\n" +
	"from mmgroup.mm_reduce import GtWord, gt_word_shorten, mm_compress_pc_expand_int\n" +
	"from mmgroup.clifford12 import xsp2co1_reduce_word\n"

// reduceOracle runs a reduction oracle script and decodes
// its JSON result as a []int64. The script imports numpy
// and the mm_reduce / clifford12 names it needs and must
// print json.dumps of a list of ints in int64 range.
func reduceOracle(t *testing.T, body string) []int64 {
	t.Helper()
	out, err := oraclepkg.Cmd(reduceImports + body).CombinedOutput()
	if err != nil {
		t.Fatalf("reduce oracle failed: %v\n%s", err, out)
	}
	var v []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("reduce oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// reduceOracleU64 is the unsigned variant used by the
// compress tests, where the 64-bit identifier digit can
// exceed the signed int64 range. The script must print
// json.dumps of a list of non-negative ints.
func reduceOracleU64(t *testing.T, body string) []uint64 {
	t.Helper()
	out, err := oraclepkg.Cmd(reduceImports + body).CombinedOutput()
	if err != nil {
		t.Fatalf("reduce oracle failed: %v\n%s", err, out)
	}
	var v []uint64
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		t.Fatalf("reduce oracle: unmarshal %q: %v", strings.TrimSpace(string(out)), err)
	}
	return v
}

// lcg32 is a deterministic pseudo-random uint32 generator
// for building reproducible monster words.
type lcg32 uint64

func (s *lcg32) next() uint32 {
	*s = lcg32(uint64(*s)*6364136223846793005 + 1442695040888963407)
	return uint32(uint64(*s) >> 32)
}

// gxAtom returns a random G_x0 generator atom (tags
// d=1, p=2, x=3, y=4, l=6). No tau atom: ReduceWord only
// accepts words of G_x0 generators.
func (s *lcg32) gxAtom() uint32 {
	switch s.next() % 5 {
	case 0:
		return 0x10000000 | (s.next() & 0xfff) // d
	case 1:
		return 0x20000000 | (s.next() % 244823040) // p
	case 2:
		return 0x30000000 | (s.next() & 0x1fff) // x
	case 3:
		return 0x40000000 | (s.next() & 0x1fff) // y
	default:
		return 0x60000000 | (1 + s.next()%2) // l (xi^1 or xi^2)
	}
}

// gxWord returns a random word of n G_x0 generator atoms.
func (s *lcg32) gxWord(n int) []uint32 {
	w := make([]uint32, n)
	for i := range w {
		w[i] = s.gxAtom()
	}
	return w
}

// mmWord returns a random monster word of n atoms: G_x0
// generators interspersed with tau atoms (tag t=5).
func (s *lcg32) mmWord(n int) []uint32 {
	w := make([]uint32, n)
	for i := range w {
		if s.next()%4 == 0 {
			w[i] = 0x50000000 | (1 + s.next()%2) // t
		} else {
			w[i] = s.gxAtom()
		}
	}
	return w
}

// fittingWords returns a batch of random monster words
// whose 255-bit as_int identifier fits in 64 bits (so the
// upper three digits are zero), the domain on which the
// 64-bit-digit contract of CompressAsInt/ExpandInt holds.
// It runs a single oracle call that filters a candidate
// pool, avoiding one subprocess per rejected word. The
// returned words are paired with their oracle low digit.
func fittingWords(t *testing.T, seed lcg32, want int) ([][]uint32, []uint64) {
	t.Helper()
	s := seed
	const pool = 96
	cands := make([][]uint32, pool)
	for i := range cands {
		cands[i] = s.mmWord(2 + i%8)
	}
	lits := make([]string, len(cands))
	for i, w := range cands {
		lits[i] = npU32(w)
	}
	// Emit one [fit, low] pair per candidate, flattened. Both
	// entries are non-negative (low is a 64-bit digit, which can
	// exceed int64 max), so decode as uint64.
	body := "C=[" + strings.Join(lits, ",") + "]\n" +
		"out=[]\n" +
		"for g in C:\n" +
		" n=int(GtWord(g,1).as_int())\n" +
		" fit = 1 if (n>>64)==0 else 0\n" +
		" out += [fit, (n & ((1<<64)-1)) if fit else 0]\n" +
		"print(json.dumps(out))"
	res := reduceOracleU64(t, body)
	var words [][]uint32
	var lows []uint64
	for i := range cands {
		if res[2*i] == 0 {
			continue
		}
		words = append(words, cands[i])
		lows = append(lows, res[2*i+1])
		if len(words) >= want {
			break
		}
	}
	if len(words) == 0 {
		t.Fatal("no 64-bit-fitting words in candidate pool")
	}
	return words, lows
}

// pyU32List renders a []uint32 as a Python list literal.
func pyU32List(w []uint32) string {
	parts := make([]string, len(w))
	for i, v := range w {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// npU32 renders a numpy uint32 array constructor for w.
func npU32(w []uint32) string {
	return "np.array(" + pyU32List(w) + ",dtype=np.uint32)"
}

func TestReduceWordOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 1
	for i := 0; i < 16; i++ {
		w := s.gxWord(2 + i%8)
		got, n := ReduceWord(w)
		if n < 0 {
			t.Fatalf("ReduceWord(%v) failed: %d", w, n)
		}
		body := fmt.Sprintf(
			"g=%s\no=np.zeros(16,dtype=np.uint32)\nk=int(xsp2co1_reduce_word(g,len(g),o))\n"+
				"print(json.dumps([k]+([int(x) for x in o[:k]] if k>=0 else [])))",
			npU32(w))
		res := reduceOracle(t, body)
		wantLen := int(res[0])
		if n != wantLen {
			t.Errorf("ReduceWord(%v) len=%d want %d", w, n, wantLen)
			continue
		}
		for j := 0; j < n; j++ {
			if int64(got[j]) != res[1+j] {
				t.Errorf("ReduceWord(%v)[%d]=%#x want %#x", w, j, got[j], res[1+j])
			}
		}
	}
}

func TestGtWordShortenOracle(t *testing.T) {
	t.Parallel()
	var s lcg32 = 2
	const maxOut = 0x400
	for i := 0; i < 16; i++ {
		w := s.mmWord(2 + i%10)
		out := make([]uint32, maxOut)
		got := GtWordShorten(append([]uint32(nil), w...), out, maxOut, 1)
		body := fmt.Sprintf(
			"g=%s\no=np.zeros(%d,dtype=np.uint32)\nk=int(gt_word_shorten(g,len(g),o,len(o),1))\n"+
				"print(json.dumps([k]+([int(x) for x in o[:k]] if k>=0 else [])))",
			npU32(w), maxOut)
		res := reduceOracle(t, body)
		wantLen := int(int32(uint32(res[0])))
		if got != wantLen {
			t.Errorf("GtWordShorten(%v) len=%d want %d", w, got, wantLen)
			continue
		}
		if got < 0 {
			continue
		}
		for j := 0; j < got; j++ {
			if int64(out[j]) != res[1+j] {
				t.Errorf("GtWordShorten(%v)[%d]=%#x want %#x", w, j, out[j], res[1+j])
			}
		}
	}
}

// CompressAsInt returns only the low 64-bit digit of the
// 255-bit identifier, so it is exercised on the words whose
// full as_int() fits in 64 bits (upper three digits zero),
// the domain on which its contract holds.
func TestCompressAsIntOracle(t *testing.T) {
	t.Parallel()
	words, lows := fittingWords(t, 3, 12)
	for k, w := range words {
		got := CompressAsInt(append([]uint32(nil), w...))
		if got != lows[k] {
			t.Errorf("CompressAsInt(%v)=%#x want %#x", w, got, lows[k])
		}
	}
}

// TestExpandIntRoundTripOracle checks ExpandInt against
// mm_compress_pc_expand_int on the low-digit identifiers of
// the same 64-bit-fitting words. A single oracle call
// expands the whole batch.
func TestExpandIntRoundTripOracle(t *testing.T) {
	t.Parallel()
	_, lows := fittingWords(t, 4, 12)
	// Batch the C expansion: one call returns, per low digit, a
	// length-prefixed atom list, all flattened. (k>=0 always for
	// these in-domain digits.)
	lits := make([]string, len(lows))
	for i, n := range lows {
		lits[i] = fmt.Sprintf("%d", n)
	}
	body := "L=[" + strings.Join(lits, ",") + "]\n" +
		"out=[]\n" +
		"for low in L:\n" +
		" pn=np.array([low,0,0,0],dtype=np.uint64)\n" +
		" o=np.zeros(64,dtype=np.uint32)\n" +
		" k=int(mm_compress_pc_expand_int(pn,o,len(o)))\n" +
		" out += [k]+[int(x) for x in o[:k]]\n" +
		"print(json.dumps(out))"
	res := reduceOracleU64(t, body)
	pos := 0
	for _, low := range lows {
		wantLen := int(res[pos])
		pos++
		got, err := ExpandInt(low)
		if err != nil {
			t.Fatalf("ExpandInt(%#x): %v", low, err)
		}
		if len(got) != wantLen {
			t.Errorf("ExpandInt(%#x) len=%d want %d", low, len(got), wantLen)
			pos += wantLen
			continue
		}
		for j := 0; j < wantLen; j++ {
			if uint64(got[j]) != res[pos+j] {
				t.Errorf("ExpandInt(%#x)[%d]=%#x want %#x", low, j, got[j], res[pos+j])
			}
		}
		pos += wantLen
	}
}

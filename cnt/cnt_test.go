package cnt

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"testing"
)

func pyModInv(t *testing.T, a, p int64) int64 {
	t.Helper()
	return oracleInt(t, fmt.Sprintf(
		"__import__('mmgroup.structures.abstract_rep_space',fromlist=['mod_inv']).mod_inv(%d,%d)",
		a, p))
}

func pyGCD(t *testing.T, a, b int64) int64 {
	t.Helper()
	return oracleInt(t, fmt.Sprintf("__import__('math').gcd(%d,%d)", a, b))
}

func TestCRTReconstructRoundTrip(t *testing.T) {
	n := CRTProduct()
	cases := []int64{0, 1, -1, 12345, -54321, n/2 - 1, -(n/2 - 1)}
	for _, v := range cases {
		a7 := ((v % 7) + 7) % 7
		a31 := ((v % 31) + 31) % 31
		a127 := ((v % 127) + 127) % 127
		a255 := ((v % 255) + 255) % 255
		if got := CRTReconstruct(a7, a31, a127, a255); got != v {
			t.Errorf("CRTReconstruct residues of %d = %d, want %d", v, got, v)
		}
	}
}

func TestModInvAgainstOracle(t *testing.T) {
	cases := [][2]int64{{3, 7}, {7, 31}, {123, 1000003}, {1004895, 7}}
	for _, c := range cases {
		want := pyModInv(t, c[0], c[1])
		got, err := ModInv(c[0], c[1])
		if err != nil {
			t.Fatalf("ModInv(%d,%d): %v", c[0], c[1], err)
		}
		if got != want {
			t.Errorf("ModInv(%d,%d) = %d, want %d", c[0], c[1], got, want)
		}
	}
}

func TestModInvNotInvertible(t *testing.T) {
	if _, err := ModInv(6, 9); err == nil {
		t.Errorf("ModInv(6,9): want error, got nil")
	}
}

func TestExtGCDBezout(t *testing.T) {
	cases := [][2]int64{{240, 46}, {7027425, 7}, {123456, 789}, {17, 0}}
	for _, c := range cases {
		a, b := c[0], c[1]
		g, x, y := ExtGCD(a, b)
		if want := pyGCD(t, a, b); g != want {
			t.Errorf("ExtGCD(%d,%d) g = %d, want %d", a, b, g, want)
		}
		if a*x+b*y != g {
			t.Errorf("ExtGCD(%d,%d): %d*%d + %d*%d = %d, want %d", a, b, a, x, b, y, a*x+b*y, g)
		}
	}
}

// fibInt63Pairs returns consecutive Fibonacci pairs (a=F(k+1), b=F(k)) for
// every k with both values inside the signed 63-bit range. These are the
// worst-case inputs (maximum recursion depth) for the Euclidean algorithm.
func fibInt63Pairs() [][2]int64 {
	var pairs [][2]int64
	prev, cur := int64(1), int64(1)
	for {
		// math.MaxInt64 is the int63 ceiling; stop before overflow (which would
		// wrap to a negative int64).
		if cur > math.MaxInt64-prev {
			break
		}
		next := prev + cur
		pairs = append(pairs, [2]int64{next, cur})
		prev, cur = cur, next
	}
	return pairs
}

// TestExtGCDBigIntIdentity exercises ExtGCD on random int63 pairs and on the
// Fibonacci worst case, verifying the Bézout identity a*x + b*y == g with
// math/big arithmetic so that any int64 overflow in the native identity check
// would be caught.
func TestExtGCDBigIntIdentity(t *testing.T) {
	check := func(a, b int64) {
		g, x, y := ExtGCD(a, b)
		// a*x + b*y computed exactly in big.Int (no overflow possible).
		lhs := new(big.Int).Mul(big.NewInt(a), big.NewInt(x))
		lhs.Add(lhs, new(big.Int).Mul(big.NewInt(b), big.NewInt(y)))
		if lhs.Cmp(big.NewInt(g)) != 0 {
			t.Fatalf("ExtGCD(%d,%d) = (g=%d,x=%d,y=%d): a*x+b*y = %s, want %d",
				a, b, g, x, y, lhs.String(), g)
		}
		// g must equal gcd(|a|,|b|) up to sign; verify with big.Int GCD.
		ba := new(big.Int).Abs(big.NewInt(a))
		bb := new(big.Int).Abs(big.NewInt(b))
		want := new(big.Int).GCD(nil, nil, ba, bb)
		bg := new(big.Int).Abs(big.NewInt(g))
		if bg.Cmp(want) != 0 {
			t.Fatalf("ExtGCD(%d,%d) g = %d, |g| = %s, want gcd = %s",
				a, b, g, bg.String(), want.String())
		}
	}

	r := rand.New(rand.NewSource(0x6c6e7468))
	for range 10000 {
		a := r.Int63()
		b := r.Int63()
		check(a, b)
	}
	for _, p := range fibInt63Pairs() {
		check(p[0], p[1])
	}
}

// crtOracle computes the centered CRT reconstruction in pure Python as an
// independent ground truth for CRTReconstruct (the existing round-trip test is
// only self-consistent).
func crtOracle(t *testing.T, a7, a31, a127, a255 int64) int64 {
	t.Helper()
	expr := fmt.Sprintf(
		"(lambda M,R: (lambda n,x: x-n if x>=n//2 else x)("+
			"__import__('math').prod(M),"+
			"sum(R[i]*(__import__('math').prod(M)//M[i])*"+
			"pow((__import__('math').prod(M)//M[i])%%M[i],-1,M[i]) "+
			"for i in range(len(M)))%%__import__('math').prod(M)))"+
			"([7,31,127,255],[%d,%d,%d,%d])",
		a7, a31, a127, a255)
	return oracleInt(t, expr)
}

// TestCRTReconstructAgainstOracle cross-checks CRTReconstruct against an
// independent Python CRT implementation.
func TestCRTReconstructAgainstOracle(t *testing.T) {
	n := CRTProduct()
	cases := []int64{0, 1, -1, 2, 12345, -54321, 1000000, -1000000, n/2 - 1, -(n / 2)}
	for _, v := range cases {
		a7 := ((v % 7) + 7) % 7
		a31 := ((v % 31) + 31) % 31
		a127 := ((v % 127) + 127) % 127
		a255 := ((v % 255) + 255) % 255
		got := CRTReconstruct(a7, a31, a127, a255)
		want := crtOracle(t, a7, a31, a127, a255)
		if got != want {
			t.Errorf("CRTReconstruct(%d,%d,%d,%d) = %d, oracle = %d (v=%d)",
				a7, a31, a127, a255, got, want, v)
		}
	}
}

func TestProbablyPrimeModuli(t *testing.T) {
	cases := []struct {
		n    string
		want bool
	}{
		{"7", true}, {"31", true}, {"127", true}, {"255", false},
	}
	for _, c := range cases {
		if got := ProbablyPrime(c.n, 20); got != c.want {
			t.Errorf("ProbablyPrime(%s) = %v, want %v", c.n, got, c.want)
		}
	}
}

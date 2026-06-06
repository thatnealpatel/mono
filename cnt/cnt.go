package cnt

import (
	"errors"
	"math/big"
)

var CRTModuli = []int64{7, 31, 127, 255}

func CRTProduct() int64 {
	n := int64(1)
	for _, m := range CRTModuli {
		n *= m
	}
	return n
}

func CRTReconstruct(a7, a31, a127, a255 int64) int64 {
	residues := []int64{a7, a31, a127, a255}
	n := CRTProduct()
	var sum int64
	for idx, m := range CRTModuli {
		mi := n / m
		yi, err := ModInv(mi%m, m)
		if err != nil {
			panic("cnt: CRT moduli not pairwise coprime")
		}
		// residues[idx] * mi * yi, reduced mod n at each step to avoid overflow.
		term := (residues[idx] % n) * (mi % n) % n
		term = term * (yi % n) % n
		sum = (sum + term) % n
	}
	sum %= n
	if sum < 0 {
		sum += n
	}
	// Center into [-n/2, n/2).
	if sum >= n/2 {
		sum -= n
	}
	return sum
}

var ErrNotInvertible = errors.New("cnt: value not invertible modulo p")

func ModInv(a, p int64) (int64, error) {
	g, x, _ := ExtGCD(a, p)
	if g != 1 && g != -1 {
		return 0, ErrNotInvertible
	}
	// If g == -1, x is a Bézout coefficient for -1, so negate.
	if g < 0 {
		x = -x
	}
	x %= p
	if x < 0 {
		x += p
	}
	return x, nil
}

func ExtGCD(a, b int64) (g, x, y int64) {
	if b == 0 {
		return a, 1, 0
	}
	g, x1, y1 := ExtGCD(b, a%b)
	return g, y1, x1 - (a/b)*y1
}

func ProbablyPrime(decimal string, reps int) bool {
	n, ok := new(big.Int).SetString(decimal, 10)
	if !ok {
		panic("cnt: bad decimal")
	}
	return n.ProbablyPrime(reps)
}

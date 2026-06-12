package monster

import (
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

var oracleDriver = oraclepkg.New(oraclepkg.Mat24)

func oracle(t *testing.T, pyExpr string) string { return oracleDriver.String(t, pyExpr) }

func oracleInt(t *testing.T, pyExpr string) int64 { return oracleDriver.Int(t, pyExpr) }

func oracleUint(t *testing.T, pyExpr string) uint64 { return oracleDriver.Uint(t, pyExpr) }

func oracleInts(t *testing.T, pyExpr string) []int64 { return oracleDriver.Ints(t, pyExpr) }

func oracleBool(t *testing.T, pyExpr string) bool { return oracleDriver.Bool(t, pyExpr) }

func mustMM(t *testing.T, word string) *MM {
	t.Helper()
	g, err := NewMM(word)
	if err != nil {
		t.Fatalf("NewMM(%q): %v", word, err)
	}
	return g
}

func mustAxisFor(t *testing.T, g *MM) *Axis {
	t.Helper()
	a, err := AxisFor(g)
	if err != nil {
		t.Fatalf("AxisFor: %v", err)
	}
	return a
}

func mustParseVector(t *testing.T, p int, s string) *MMVector {
	t.Helper()
	v, err := ParseVector(p, s)
	if err != nil {
		t.Fatalf("ParseVector(%d, %q): %v", p, s, err)
	}
	return v
}

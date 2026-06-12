package mmindex

import (
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

var oracleDriver = oraclepkg.New(oraclepkg.Basic)

func oracle(t *testing.T, pyExpr string) string { return oracleDriver.String(t, pyExpr) }

func oracleInt(t *testing.T, pyExpr string) int64 { return oracleDriver.Int(t, pyExpr) }

func oracleUint(t *testing.T, pyExpr string) uint64 { return oracleDriver.Uint(t, pyExpr) }

package qstate12

import (
	"testing"

	oraclepkg "patel.codes/cgt/internal/oracle"
)

var oracleDriver = oraclepkg.New(oraclepkg.Mat24)

func oracle(t *testing.T, pyExpr string) string { return oracleDriver.String(t, pyExpr) }

func oracleInts(t *testing.T, pyExpr string) []int64 { return oracleDriver.Ints(t, pyExpr) }

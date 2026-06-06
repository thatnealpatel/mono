package cgt

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func pyCmd(script string) *exec.Cmd {
	cmd := exec.Command("python3", "-c", script)
	cmd.Env = append(cmd.Environ(),
		"PYTHONPATH=/home/neal/code/mono/mmgroup/src",
		"LD_LIBRARY_PATH=/home/neal/code/mono/mmgroup/src/mmgroup",
	)
	return cmd
}

func oracle(t *testing.T, pyExpr string) string {
	t.Helper()
	script := fmt.Sprintf("import json,mmgroup;from mmgroup import mat24;print(json.dumps(%s))", pyExpr)
	out, err := pyCmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func oracleInt(t *testing.T, pyExpr string) int64 {
	t.Helper()
	s := oracle(t, pyExpr)
	var v int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleInt(%q): unmarshal %q: %v", pyExpr, s, err)
	}
	return v
}

func oracleUint(t *testing.T, pyExpr string) uint64 {
	t.Helper()
	s := oracle(t, pyExpr)
	var v uint64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleUint(%q): unmarshal %q: %v", pyExpr, s, err)
	}
	return v
}

func oracleInts(t *testing.T, pyExpr string) []int64 {
	t.Helper()
	s := oracle(t, pyExpr)
	var v []int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracleInts(%q): unmarshal %q: %v", pyExpr, s, err)
	}
	return v
}

func oracleBool(t *testing.T, pyExpr string) bool {
	t.Helper()
	s := oracle(t, pyExpr)
	return s == "true"
}

func mustMM(t *testing.T, word string) *MM {
	t.Helper()
	g, err := NewMM(word)
	if err != nil {
		t.Fatalf("NewMM(%q): %v", word, err)
	}
	return g
}

func mustParseVector(t *testing.T, p int, s string) *MMVector {
	t.Helper()
	v, err := ParseVector(p, s)
	if err != nil {
		t.Fatalf("ParseVector(%d, %q): %v", p, s, err)
	}
	return v
}

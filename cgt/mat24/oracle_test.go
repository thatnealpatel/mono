package mat24

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
	// complete_import() forces lazy module globals
	// to bind. ploop.py binds its own (mat24, mat24.AutPL,
	// XLeech2); gcode.py separately binds mat24.PLoop into
	// its namespace (gcode.py complete_import). The
	// latter is required because mat24.GCode.__abs__ uses
	// module-global mat24.PLoop unguarded by import_pending,
	// so abs(mat24.PLoop(...)) hits NameError otherwise.
	script := fmt.Sprintf("import json,mmgroup;from mmgroup import mat24;from mmgroup.structures.ploop import complete_import as _ci_ploop;_ci_ploop();from mmgroup.structures.gcode import complete_import as _ci_gcode;_ci_gcode();print(json.dumps(%s))", pyExpr)
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

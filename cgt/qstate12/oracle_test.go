package qstate12

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
	script := fmt.Sprintf("import json,mmgroup;from mmgroup import mat24;from mmgroup.structures.ploop import complete_import as _ci_ploop;_ci_ploop();from mmgroup.structures.gcode import complete_import as _ci_gcode;_ci_gcode();print(json.dumps(%s))", pyExpr)
	out, err := pyCmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
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

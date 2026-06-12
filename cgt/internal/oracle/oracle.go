// Package oracle is the shared test-support harness for the
// mmgroup Python oracle. Per-package oracle_test.go files
// build a Driver bound to the import preamble they need and
// call its typed-decode methods: a Python expression goes in,
// a typed Go value comes out.
//
// This package exists only to be imported from _test.go files;
// it is not part of any production build.
package oracle

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// pythonPath and ldLibraryPath point at the canonical mmgroup
// build tree on this machine; the oracle invokes the upstream
// Python/Cython implementation as ground truth.
const (
	pythonPath    = "PYTHONPATH=/home/neal/code/mono/mmgroup/src"
	ldLibraryPath = "LD_LIBRARY_PATH=/home/neal/code/mono/mmgroup/src/mmgroup"
)

// Cmd returns a python3 *exec.Cmd that runs script with the
// mmgroup interpreter environment configured. Callers that
// build a bespoke multi-line driver script use Cmd directly;
// callers with a simple expression use a Driver instead.
func Cmd(script string) *exec.Cmd {
	cmd := exec.Command("python3", "-c", script)
	cmd.Env = append(cmd.Environ(), pythonPath, ldLibraryPath)
	return cmd
}

// Preamble is a Python prologue executed before the oracle
// expression. The standard variants are exported below.
type Preamble string

const (
	// Basic only imports json and mmgroup.
	Basic Preamble = "import json,mmgroup"

	// Mat24 additionally forces the lazy mat24/ploop/gcode
	// module globals to bind.
	//
	// complete_import() forces lazy module globals to bind.
	// ploop.py binds its own (mat24, mat24.AutPL, XLeech2);
	// gcode.py separately binds mat24.PLoop into its namespace
	// (gcode.py complete_import). The latter is required
	// because mat24.GCode.__abs__ uses module-global mat24.PLoop
	// unguarded by import_pending, so abs(mat24.PLoop(...)) hits
	// NameError otherwise.
	Mat24 Preamble = "import json,mmgroup;from mmgroup import mat24;" +
		"from mmgroup.structures.ploop import complete_import as _ci_ploop;_ci_ploop();" +
		"from mmgroup.structures.gcode import complete_import as _ci_gcode;_ci_gcode()"
)

// A Driver runs oracle expressions under a fixed import
// preamble and decodes the JSON result into typed Go values.
type Driver struct {
	preamble Preamble
}

// New returns a Driver bound to preamble.
func New(preamble Preamble) *Driver {
	return &Driver{preamble: preamble}
}

// String runs json.dumps(expr) under the driver's preamble and
// returns the trimmed stdout. It fails t on a nonzero exit.
func (d *Driver) String(t *testing.T, expr string) string {
	t.Helper()
	script := fmt.Sprintf("%s;print(json.dumps(%s))", d.preamble, expr)
	out, err := Cmd(script).CombinedOutput()
	if err != nil {
		t.Fatalf("python oracle failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// Int runs expr and decodes the result as an int64.
func (d *Driver) Int(t *testing.T, expr string) int64 {
	t.Helper()
	s := d.String(t, expr)
	var v int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracle Int(%q): unmarshal %q: %v", expr, s, err)
	}
	return v
}

// Uint runs expr and decodes the result as a uint64.
func (d *Driver) Uint(t *testing.T, expr string) uint64 {
	t.Helper()
	s := d.String(t, expr)
	var v uint64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracle Uint(%q): unmarshal %q: %v", expr, s, err)
	}
	return v
}

// Ints runs expr and decodes the result as an []int64.
func (d *Driver) Ints(t *testing.T, expr string) []int64 {
	t.Helper()
	s := d.String(t, expr)
	var v []int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("oracle Ints(%q): unmarshal %q: %v", expr, s, err)
	}
	return v
}

// Bool runs expr and reports whether the JSON result is true.
func (d *Driver) Bool(t *testing.T, expr string) bool {
	t.Helper()
	return d.String(t, expr) == "true"
}

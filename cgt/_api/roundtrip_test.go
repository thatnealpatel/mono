package main

import (
	"os"
	"strings"
	"testing"
)

// TestPythonYAMLRoundTripTypeKeys is a step[6] guard: it parses the
// generated python.yaml back through parsePythonYAML and asserts that the
// new typed keys (method/function return:, param type:, and the
// dispatch: ladder) survive the emit->parse round trip. It reads the live
// python.yaml produced by the walker, so it fails closed if a future emit
// change drops a key the parser expects (or vice versa).
func TestPythonYAMLRoundTripTypeKeys(t *testing.T) {
	raw, err := os.ReadFile(pythonOut)
	if err != nil {
		t.Fatalf("reading %s: %v", pythonOut, err)
	}
	man, err := parsePythonYAML(string(raw))
	if err != nil {
		t.Fatalf("parsePythonYAML: %v", err)
	}

	// Locate GCode: the canonical class exercising every typed shape
	// (return:, param type:, value/type dispatch). EXECUTION.md [8].
	var gcode *pyClassDecl
	for i := range man.Classes {
		if man.Classes[i].Name == "GCode" {
			gcode = &man.Classes[i]
			break
		}
	}
	if gcode == nil {
		t.Fatal("GCode class not parsed from python.yaml")
	}

	methods := make(map[string]pyMethod, len(gcode.Methods))
	for _, m := range gcode.Methods {
		methods[m.Name] = m
	}

	// A plain typed return survived parsing.
	if m, ok := methods["theta"]; !ok {
		t.Error("GCode.theta not parsed")
	} else if m.Return == "" {
		t.Error("GCode.theta lost its return: through the round trip")
	}

	// A dispatch ladder survived parsing with both on: and returns: per arm.
	if m, ok := methods["__and__"]; !ok {
		t.Error("GCode.__and__ not parsed")
	} else {
		if len(m.Dispatch) == 0 {
			t.Error("GCode.__and__ lost its dispatch: ladder through the round trip")
		}
		for i, d := range m.Dispatch {
			if d.On == "" {
				t.Errorf("GCode.__and__ dispatch arm %d lost on:", i)
			}
			if d.Returns == "" {
				t.Errorf("GCode.__and__ dispatch arm %d (on=%q) lost returns:", i, d.On)
			}
		}
	}

	// A param type: survived parsing on at least one method.
	foundParamType := false
	for _, m := range gcode.Methods {
		for _, p := range m.Params {
			if p.Type != "" {
				foundParamType = true
			}
		}
	}
	if !foundParamType {
		t.Error("no GCode method param recovered a type: through the round trip")
	}

	// Module-level functions: at least one return: must survive parsing
	// (676 functions were written; the typed ones must parse).
	fnReturns := 0
	for _, fn := range man.ModuleFns {
		if fn.Return != "" {
			fnReturns++
		}
	}
	if fnReturns == 0 {
		t.Error("no module function recovered a return: through the round trip")
	}

	// Aggregate parity: the parser must recover exactly as many typed
	// fields as the emitter wrote into the file. Counting the emitted keys
	// directly from the raw text (column-anchored prefixes that only the
	// step[6] emitters produce) and comparing against the parsed structs
	// proves no key is silently dropped for any shape, not just GCode.
	want := emittedKeyCounts(string(raw))
	got := parsedKeyCounts(man)
	if got != want {
		t.Errorf("typed-key counts diverged emit->parse:\n  emitted %+v\n  parsed  %+v", want, got)
	}
}

// TestCythonYAMLRoundTrip is the cython-side counterpart of
// TestPythonYAMLRoundTripTypeKeys: it parses the committed cython.yaml back
// through parseCythonYAML and asserts that every key the emitter wrote is
// recovered by the parser. It fails closed if a future emit change drops a
// key the parser expects (or vice versa).
//
// Note on shape: the plan asked for an emit->parse->emit byte-equality
// check, but emitCythonYAML consumes pxdContents (whose funcDecl carries a
// Source field driving the `# <source>` group headers and `  source:`
// lines), while parseCythonYAML returns a cythonManifest whose cythonFunc
// has no Source field and whose parser discards `source:` lines entirely. A
// parseCythonYAML -> emitCythonYAML round trip therefore cannot reproduce
// the source grouping, so byte equality is unreachable without widening the
// parser's surface (a redesign out of scope here). This test instead
// mirrors the python test's actual behavior: emit->parse key-count parity,
// which is the same fragility-class guard (proves no key, including the
// explicit `params: []` empty case, is silently dropped on parse).
func TestCythonYAMLRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(cythonOut)
	if err != nil {
		t.Fatalf("reading %s: %v", cythonOut, err)
	}
	man, err := parseCythonYAML(string(raw))
	if err != nil {
		t.Fatalf("parseCythonYAML: %v", err)
	}

	want := emittedCythonKeyCounts(string(raw))
	got := parsedCythonKeyCounts(man)
	if got != want {
		t.Errorf("cython key counts diverged emit->parse:\n  emitted %+v\n  parsed  %+v", want, got)
	}

	// The explicit empty-param case must round-trip: every `params: []`
	// line in the file must correspond to a parsed function with zero
	// params, and no other function may end up with zero params (every
	// other function has at least one `    - type:` child).
	if want.emptyParamFuncs == 0 {
		t.Fatal("expected at least one `params: []` function in cython.yaml")
	}
	emptyParsed := 0
	for _, f := range man.Funcs {
		if len(f.Params) == 0 {
			emptyParsed++
		}
	}
	if emptyParsed != want.emptyParamFuncs {
		t.Errorf("empty-param func count diverged: emitted %d `params: []`, parsed %d zero-param funcs",
			want.emptyParamFuncs, emptyParsed)
	}
}

// cythonKeyCounts tallies the key families parseCythonYAML is responsible
// for recovering from cython.yaml.
type cythonKeyCounts struct {
	funcs           int // `- name:` entries
	returns         int // `  return:` lines
	returnPtrs      int // `  return_ptr: true` lines
	emptyParamFuncs int // `  params: []` lines
	paramTypes      int // `    - type:` lines (one per param)
	paramNames      int // `      name:` lines
	paramPtrs       int // `      ptr: true` lines
	gotypes         int // `      gotype:` lines
	structs         int // `- struct:` lines
	enums           int // `- enum:` lines
	enumValues      int // `  value:` lines
}

// emittedCythonKeyCounts counts keys straight from the emitted text using
// the exact column-anchored prefixes emitCythonYAML produces.
func emittedCythonKeyCounts(raw string) cythonKeyCounts {
	var c cythonKeyCounts
	for _, ln := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(ln, "- name: "):
			c.funcs++
		case strings.HasPrefix(ln, "  return_ptr: true"):
			c.returnPtrs++
		case strings.HasPrefix(ln, "  return: "):
			c.returns++
		case ln == "  params: []":
			c.emptyParamFuncs++
		case strings.HasPrefix(ln, "    - type: "):
			c.paramTypes++
		case strings.HasPrefix(ln, "      name: "):
			c.paramNames++
		case strings.HasPrefix(ln, "      ptr: true"):
			c.paramPtrs++
		case strings.HasPrefix(ln, "      gotype: "):
			c.gotypes++
		case strings.HasPrefix(ln, "- struct: "):
			c.structs++
		case strings.HasPrefix(ln, "- enum: "):
			c.enums++
		case strings.HasPrefix(ln, "  value: "):
			c.enumValues++
		}
	}
	return c
}

// parsedCythonKeyCounts tallies the same families from the parsed manifest.
// Empty-param functions are counted from the params-less funcs; every other
// func carries at least one param, so the two partitions do not overlap.
func parsedCythonKeyCounts(man cythonManifest) cythonKeyCounts {
	var c cythonKeyCounts
	for _, f := range man.Funcs {
		c.funcs++
		// Every emitted func writes exactly one `  return:` line.
		c.returns++
		if f.ReturnPtr {
			c.returnPtrs++
		}
		if len(f.Params) == 0 {
			c.emptyParamFuncs++
		}
		for _, p := range f.Params {
			c.paramTypes++
			// emitCythonFunc writes a `      name:` line for every param
			// (including the supplement's function-pointer param, whose
			// name is non-empty), so count one per param.
			c.paramNames++
			if p.Ptr {
				c.paramPtrs++
			}
			if p.GoType != "" {
				c.gotypes++
			}
		}
	}
	c.structs = len(man.Structs)
	for _, e := range man.Enums {
		c.enums++
		// Every enum that reaches the manifest carried a value (flushEnum
		// errors otherwise), and emitCythonEnums writes a `  value:` line
		// for each such enum.
		_ = e
		c.enumValues++
	}
	return c
}

// keyCounts tallies the four typed key families across python.yaml.
type keyCounts struct {
	methodReturns int
	fnReturns     int
	methodPType   int
	fnPType       int
	dispatchArms  int
}

// emittedKeyCounts counts typed keys straight from the emitted text using
// the exact column-anchored prefixes emitMethod/emitPyFunc produce. Method
// keys sit two indent levels deeper than module-function keys, so the
// prefixes are unambiguous between the two.
func emittedKeyCounts(raw string) keyCounts {
	var c keyCounts
	for _, ln := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(ln, "      return: "):
			c.methodReturns++
		case strings.HasPrefix(ln, "  return: "):
			c.fnReturns++
		case strings.HasPrefix(ln, "          type: "):
			c.methodPType++
		case strings.HasPrefix(ln, "      type: "):
			c.fnPType++
		case strings.HasPrefix(ln, "        - on: "):
			c.dispatchArms++
		}
	}
	return c
}

// parsedKeyCounts tallies the same families from the parsed manifest.
func parsedKeyCounts(man pythonManifest) keyCounts {
	var c keyCounts
	for _, cls := range man.Classes {
		for _, m := range cls.Methods {
			if m.Return != "" {
				c.methodReturns++
			}
			c.dispatchArms += len(m.Dispatch)
			for _, p := range m.Params {
				if p.Type != "" {
					c.methodPType++
				}
			}
		}
	}
	for _, fn := range man.ModuleFns {
		if fn.Return != "" {
			c.fnReturns++
		}
		for _, p := range fn.Params {
			if p.Type != "" {
				c.fnPType++
			}
		}
	}
	return c
}

// Command api extracts the mmgroup API surface and
// emits YAML manifests for oracle-comparison tests
// and translation-parity checks against cgt.
//
// It runs as a single pipeline. The cython.yaml and
// python.yaml stages read mmgroup directly; the go.yaml
// stage is derived from the freshly written cython.yaml,
// mapping each C function to its target cgt Go package,
// exported name, and Go parameter/return types.
//
// Usage:
//
//	go run -C _api .            # run the full pipeline
//	go run -C _api . -out cython.yaml   # one stage only
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var pxdRoot = filepath.Join("..", "..", "mmgroup", "src", "mmgroup", "dev", "pxd_files")

const (
	mmgroupPythonPath  = "/home/neal/code/mono/mmgroup/src"
	mmgroupLibraryPath = "/home/neal/code/mono/mmgroup/src/mmgroup"
)

const pyPreamble = `import json, mmgroup
from mmgroup.structures.ploop import complete_import
complete_import()
from mmgroup.structures.gcode import complete_import as _gci
_gci()
`

// pyExcludeModules names the mmgroup module subtrees the inspect walker
// must not surface into python.yaml. excluded() matches a module against
// each entry as `modname == e || modname.startswith(e + ".")`, so a bare
// prefix like "mmgroup.mm" excludes the deprecated mm shim and its
// submodules without swallowing the sibling modules mm_op, mm_group,
// mm_space, or mm_reduce (none of which equal "mmgroup.mm" or start with
// "mmgroup.mm.").
//
//   - mmgroup.dev — all 55 dev.* classes (GenXi, Mat24Tables, Tables,
//     MockupTables, HadamardMatrixTable, etc.) are build-time table
//     generators or pure-Python oracle references that produce C lookup
//     tables and code — they are not part of the runtime math API. The
//     Go side regenerates equivalent tables in cgt/_gen and validates
//     against the compiled C library via oracle tests. A per-class skip
//     map was considered and rejected: blanket module exclusion is
//     self-maintaining as upstream adds new dev classes, and no dev.*
//     class is a legitimate API surface (even borderline cases like
//     GenXi are generator shells wrapping C-table emission).
//   - mmgroup.demo — demonstration scripts, not API.
//   - mmgroup.tests — test scaffolding. The two public, re-exported
//     classes living under it (Axis/BabyAxis, surfaced via mmgroup.axes)
//     are rescued by the re-export-aware identity check in the walker.
//   - mmgroup.mm{,3,7,15,31,127,255} — deprecated shims (docstring
//     "This module is deprecated; do not use in new projects!") that
//     merely re-export mm_op functions under op_*/mm_aux_* names.
//     Without excluding them the module-level function pass would emit
//     phantom duplicates of the canonical mm_op builtins.
var pyExcludeModules = []string{
	"mmgroup.generate_c",
	"mmgroup.dev",
	"mmgroup.demo",
	"mmgroup.tests",
	"mmgroup.mm",
	"mmgroup.mm3",
	"mmgroup.mm7",
	"mmgroup.mm15",
	"mmgroup.mm31",
	"mmgroup.mm127",
	"mmgroup.mm255",
}

const (
	cythonOut = "cython.yaml"
	pythonOut = "python.yaml"
	goOut     = "go.yaml"
)

var generators = map[string]func(string) error{
	cythonOut: genCython,
	pythonOut: genPython,
}

func main() {
	log.SetPrefix("api: ")
	log.SetFlags(0)

	out := flag.String("out", "", "run only this stage (cython.yaml, python.yaml, or go.yaml)")
	flag.Parse()

	if *out != "" {
		if err := runStage(filepath.Base(*out)); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Full pipeline: cython.yaml and python.yaml read mmgroup
	// directly; go.yaml is derived from the fresh cython.yaml.
	for _, stage := range []string{cythonOut, pythonOut, goOut} {
		if err := runStage(stage); err != nil {
			log.Fatal(err)
		}
	}
}

func runStage(name string) error {
	switch name {
	case goOut:
		return genGo(cythonOut, pythonOut, goOut)
	default:
		gen, ok := generators[name]
		if !ok {
			return fmt.Errorf("unknown stage %q (want cython.yaml, python.yaml, or go.yaml)", name)
		}
		return gen(name)
	}
}

func genCython(out string) error {
	entries, err := os.ReadDir(pxdRoot)
	if err != nil {
		return fmt.Errorf("reading %s: %w", pxdRoot, err)
	}
	var all pxdContents
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pxd") {
			continue
		}
		path := filepath.Join(pxdRoot, e.Name())
		c, err := parsePxd(path, e.Name())
		if err != nil {
			return err
		}
		all.Funcs = append(all.Funcs, c.Funcs...)
		all.Structs = append(all.Structs, c.Structs...)
		all.Enums = append(all.Enums, c.Enums...)
	}

	// Merge the header-only functions that have no .pxd declaration
	// (Q26): four qstate12 helpers marked %%EXPORT-plain and the one
	// %%EXPORT p symbol (xsp2co1_trace_98280) the upstream .pxd
	// generator dropped over its function-pointer parameter.
	all.Funcs = append(all.Funcs, supplementFuncs...)

	if err := os.WriteFile(out, []byte(emitCythonYAML(all)), 0o644); err != nil {
		return err
	}
	log.Printf("wrote %d functions, %d structs, %d enums to %s",
		len(all.Funcs), len(all.Structs), len(all.Enums), out)
	return nil
}

// pxdContents holds every declaration the .pxd parser extracts: C
// function signatures, the names of ctypedef struct handles (used to
// auto-populate goStruct, Q13/Q30), and the names of enum compile-time
// constants (emitted as Go constants, Q29; their values come from the
// enumValues table since the .pxd carries only the names).
type pxdContents struct {
	Funcs   []funcDecl
	Structs []string
	Enums   []string
}

type funcDecl struct {
	Source    string
	Return    string
	ReturnPtr bool
	Name      string
	Params    []param
}

type param struct {
	Type string
	Name string
	Ptr  bool
	// GoType, when non-empty, is a pre-computed Go type that
	// bypasses goTypeOf. It is used only by the manual supplement
	// table for declarators that paramRe and goTypeOf cannot express
	// (the function-pointer param of xsp2co1_trace_98280).
	GoType string
}

var funcRe = regexp.MustCompile(
	`^\s+([\w]+)\s*(\*?)\s*([\w]+)\(([^)]*)\)\s*$`,
)

var paramRe = regexp.MustCompile(
	`^\s*([\w]+)\s*(\*?)\s*([\w]+)\s*$`,
)

// structRe matches a Cython `ctypedef struct NAME:` declaration; the
// name feeds goStruct (Q13/Q30). Fields are deliberately not parsed —
// go.yaml keeps structs opaque (Q30).
var structRe = regexp.MustCompile(`^\s+ctypedef\s+struct\s+([\w]+)\s*:\s*$`)

// enumRe matches a value-less `enum: NAME` line. The .pxd never carries
// the numeric value (Cython treats it as a constant of unspecified
// value); enumValues supplies it from the C headers (Q29).
var enumRe = regexp.MustCompile(`^\s+enum:\s+([\w]+)\s*$`)

func parsePxd(path, source string) (pxdContents, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pxdContents{}, err
	}
	seen := make(map[string]bool)
	var c pxdContents
	for _, line := range strings.Split(string(data), "\n") {
		if sm := structRe.FindStringSubmatch(line); sm != nil {
			c.Structs = append(c.Structs, sm[1])
			continue
		}
		if em := enumRe.FindStringSubmatch(line); em != nil {
			c.Enums = append(c.Enums, em[1])
			continue
		}
		m := funcRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		retType, retPtr, name, paramStr := m[1], m[2] == "*", m[3], m[4]

		if seen[name] {
			continue
		}
		seen[name] = true

		params, err := parseParams(paramStr)
		if err != nil {
			return pxdContents{}, fmt.Errorf("%s: %s: %w", source, name, err)
		}
		c.Funcs = append(c.Funcs, funcDecl{
			Source:    source,
			Return:    retType,
			ReturnPtr: retPtr,
			Name:      name,
			Params:    params,
		})
	}
	return c, nil
}

func parseParams(s string) ([]param, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var params []param
	for _, p := range strings.Split(s, ",") {
		pm := paramRe.FindStringSubmatch(p)
		if pm == nil {
			return nil, fmt.Errorf("cannot parse param %q", p)
		}
		params = append(params, param{
			Type: pm[1],
			Name: pm[3],
			Ptr:  pm[2] == "*",
		})
	}
	return params, nil
}

// pxdSources lists the two compiled-Cython .pyx files whose cdef-class
// method signatures the python.yaml walker cannot read at runtime. Every
// method of a compiled cdef class (QState12, GtWord) is a method_descriptor
// on which inspect.signature() raises ValueError because mmgroup is built
// without embedsignature, so params_of emits params: []. The .pyx source is
// the only place the parameter list survives; pxdMethodOverlay parses it and
// genPython overlays it onto the walker dump's empty Params (Half 1, D1b).
//
// Paths are relative to pxdRoot (the dev/pxd_files directory). Each maps to
// the Python class names whose cdef methods it defines, used by the staleness
// check so a class that vanishes from the dump is a hard error rather than a
// silently empty overlay.
var pxdSources = []struct {
	file    string
	classes []string
}{
	{file: "clifford12.pyx", classes: []string{"QState12"}},
	{file: "mm_reduce.pyx", classes: []string{"GtWord"}},
}

// pxdClassRe matches a `cdef class NAME(...)` or `cdef class NAME:` header.
// The capture is the class name; the optional base list and trailing colon
// are discarded.
var pxdClassRe = regexp.MustCompile(`^cdef\s+class\s+([\w]+)\s*(?:\([^)]*\))?\s*:\s*$`)

// pxdDefRe matches an indented `def name(params):` line within a cdef-class
// body. The leading whitespace is exactly the class-body indent (the def
// keyword is then followed by one or more spaces — some lines use a double
// space, e.g. `def  __imul__`). Captures the method name and the raw param
// string between the parentheses. Method bodies sit at a deeper indent, so a
// nested helper def cannot match (its indent is wider than the four-space
// class-body indent).
var pxdDefRe = regexp.MustCompile(`^    def\s+([\w]+)\(([^)]*)\)\s*:\s*$`)

// pxdMethodOverlay parses the cdef-class method signatures out of the .pyx
// sources, returning class -> method -> parameter list (in inspect form,
// keeping self). genPython overlays these onto the walker dump to repair the
// params: [] the runtime inspect could not produce (D1b Half 1).
//
// Each parameter carries a Python type translated from its Cython annotation:
// integer scalars (uint32_t, int32_t, uint64_t, ...) become "int" (which
// pyScalarToGo maps to Go int), a class-typed annotation (QState12 other)
// keeps the class name (which pyClassToGo resolves), and an un-annotated
// param stays blank-typed (Type ""). Defaults are carried verbatim as a type
// hint. Every parameter's Kind is POSITIONAL_OR_KEYWORD, matching how a plain
// Cython def parameter is realized.
//
// It errors if a .pyx file is missing, if a class listed in pxdSources is
// never seen in its file, or if the file yields zero methods for it (a sign
// the .pyx format changed and the parser silently matched nothing).
func pxdMethodOverlay() (map[string]map[string][]pyParam, error) {
	overlay := map[string]map[string][]pyParam{}
	for _, src := range pxdSources {
		path := filepath.Join(pxdRoot, src.file)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading .pyx overlay source %s: %w", path, err)
		}
		methods, err := parsePxdMethods(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src.file, err)
		}
		for _, cls := range src.classes {
			m, ok := methods[cls]
			if !ok {
				return nil, fmt.Errorf("%s: cdef class %s not found (.pyx format changed?)", src.file, cls)
			}
			if len(m) == 0 {
				return nil, fmt.Errorf("%s: cdef class %s yielded zero methods (.pyx format changed?)", src.file, cls)
			}
			overlay[cls] = m
		}
	}
	return overlay, nil
}

// parsePxdMethods scans one .pyx file's text for `cdef class NAME` blocks and
// the indented `def method(params):` lines within each. A line at column 0
// (non-blank, non-indented) closes the current class block, so a method's own
// body — and any module-level code between classes — is never mistaken for a
// class method.
func parsePxdMethods(text string) (map[string]map[string][]pyParam, error) {
	out := map[string]map[string][]pyParam{}
	curClass := ""
	for _, line := range strings.Split(text, "\n") {
		if cm := pxdClassRe.FindStringSubmatch(line); cm != nil {
			curClass = cm[1]
			out[curClass] = map[string][]pyParam{}
			continue
		}
		// A non-blank line that starts at column 0 ends the current class
		// block (the next top-level statement). Indented lines stay inside it.
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			curClass = ""
			continue
		}
		if curClass == "" {
			continue
		}
		dm := pxdDefRe.FindStringSubmatch(line)
		if dm == nil {
			continue
		}
		name, paramStr := dm[1], dm[2]
		params, err := parsePyxParams(paramStr)
		if err != nil {
			return nil, fmt.Errorf("class %s: def %s: %w", curClass, name, err)
		}
		out[curClass][name] = params
	}
	return out, nil
}

// pyxScalarToPyType maps a Cython integer scalar annotation to the canonical
// Python type name the walker would have inferred (int). The Go side then maps
// "int" through pyScalarToGo. Only the scalars that appear as cdef-method
// parameter annotations in the two overlaid .pyx files are listed; an
// unrecognized annotation is treated as a class-typed param (its name passes
// through to pyClassToGo, which yields "" for a non-class), an honest gap.
var pyxScalarToPyType = map[string]string{
	"int8_t":   "int",
	"uint8_t":  "int",
	"int16_t":  "int",
	"uint16_t": "int",
	"int32_t":  "int",
	"uint32_t": "int",
	"int64_t":  "int",
	"uint64_t": "int",
	"size_t":   "int",
}

// parsePyxParams splits a Cython def parameter string into pyParam entries,
// preserving self so dropSelf works downstream. Each comma-separated field is
// one of:
//
//	NAME                     — un-annotated param (blank type)
//	NAME = DEFAULT           — un-annotated param with a default
//	CTYPE NAME               — Cython-typed param (CTYPE annotation)
//	CTYPE NAME = DEFAULT      — typed param with a default
//	*args / **kwds           — variadic; recorded with the appropriate Kind
//
// An integer-scalar CTYPE becomes Python "int"; any other two-token CTYPE
// (a class annotation like `QState12 other`) keeps the CTYPE as the Python
// type name for pyClassToGo to resolve. An un-annotated field stays blank.
func parsePyxParams(s string) ([]pyParam, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var params []pyParam
	for _, field := range strings.Split(s, ",") {
		f := strings.TrimSpace(field)
		if f == "" {
			return nil, fmt.Errorf("empty parameter in %q", s)
		}
		p := pyParam{Kind: "POSITIONAL_OR_KEYWORD"}

		// Split off a trailing default ("= value"). repr-style; carried
		// verbatim as a hint, mirroring how the walker emits defaults.
		if eq := strings.IndexByte(f, '='); eq >= 0 {
			p.Default = strings.TrimSpace(f[eq+1:])
			f = strings.TrimSpace(f[:eq])
		}

		// Variadic markers: *args / **kwds. Strip the stars for the name and
		// tag the inspect kind so hasVariadic still fires downstream.
		switch {
		case strings.HasPrefix(f, "**"):
			p.Kind = "VAR_KEYWORD"
			p.Name = strings.TrimPrefix(f, "**")
			params = append(params, p)
			continue
		case strings.HasPrefix(f, "*"):
			p.Kind = "VAR_POSITIONAL"
			p.Name = strings.TrimPrefix(f, "*")
			params = append(params, p)
			continue
		}

		// One token: an un-annotated param. Two tokens: "CTYPE NAME".
		toks := strings.Fields(f)
		switch len(toks) {
		case 1:
			p.Name = toks[0]
		case 2:
			ctype, name := toks[0], toks[1]
			p.Name = name
			if py, ok := pyxScalarToPyType[ctype]; ok {
				p.Type = py
			} else {
				// A non-scalar annotation is a class type (QState12 other);
				// keep the class name for pyClassToGo to resolve.
				p.Type = ctype
			}
		default:
			return nil, fmt.Errorf("cannot parse parameter %q", field)
		}
		params = append(params, p)
	}
	return params, nil
}

// supplementFuncs are C functions that exist in the headers under
// mmgroup/src/mmgroup/dev/headers but have no .pxd declaration, so the
// .pxd-only parser cannot see them (Gap 3, Q26). They are merged into
// genCython's output verbatim rather than by parsing the headers. Each
// entry records its clifford12.h source line and why it is pulled in
// past the .pxd boundary.
//
// The four qstate12_* helpers carry a plain `// %%EXPORT` marker (no
// suffix), meaning they are public C symbols the library deliberately
// does not wrap for Python; cgt mirrors them anyway. xsp2co1_trace_98280
// carries `// %%EXPORT p` (it IS flagged for the .pxd back end) but the
// upstream .pxd generator dropped it because it could not render its
// function-pointer parameter — an upstream miss, not curation.
var supplementFuncs = []funcDecl{
	{
		// clifford12.h:346
		Source: "clifford12.h (supplement)",
		Return: "int32_t",
		Name:   "qstate12_del_rows",
		Params: []param{
			{Type: "qstate12_type", Name: "pqs", Ptr: true},
			{Type: "uint64_t", Name: "v"},
		},
	},
	{
		// clifford12.h:349
		Source: "clifford12.h (supplement)",
		Return: "int32_t",
		Name:   "qstate12_insert_rows",
		Params: []param{
			{Type: "qstate12_type", Name: "pqs", Ptr: true},
			{Type: "uint32_t", Name: "i"},
			{Type: "uint32_t", Name: "nrows"},
		},
	},
	{
		// clifford12.h:361
		Source: "clifford12.h (supplement)",
		Return: "void",
		Name:   "qstate12_pivot",
		Params: []param{
			{Type: "qstate12_type", Name: "pqs", Ptr: true},
			{Type: "uint32_t", Name: "i"},
			{Type: "uint64_t", Name: "v"},
		},
	},
	{
		// clifford12.h:367
		Source: "clifford12.h (supplement)",
		Return: "int32_t",
		Name:   "qstate12_sum_up_kernel",
		Params: []param{
			{Type: "qstate12_type", Name: "pqs", Ptr: true},
		},
	},
	{
		// clifford12.h:1113 — int32_t xsp2co1_trace_98280(
		//   uint64_t *elem, int32_t (*f_fast)(uint64_t*))
		// The f_fast callback's Go type is hand-coded (Q27); paramRe and
		// goTypeOf cannot express a function-pointer declarator.
		Source: "clifford12.h (supplement)",
		Return: "int32_t",
		Name:   "xsp2co1_trace_98280",
		Params: []param{
			{Type: "uint64_t", Name: "elem", Ptr: true},
			{Name: "f_fast", GoType: "func([]uint64) int32"},
		},
	},
}

// enumValues supplies the numeric value of each value-less `enum: NAME`
// constant found in the .pxd files. The .pxd carries only the names, so
// the values are read from the C headers under
// mmgroup/src/mmgroup/dev/headers and recorded here (Q29). They are
// emitted as Go constants in go.yaml. MaxGtWordData and
// MmCompressTypeNentries are structural: they size the data[]/w[] arrays
// of gt_subword_type and mm_compress_type.
var enumValues = map[string]int64{
	"MAX_GT_WORD_DATA":          24,   // mm_reduce.h:246
	"MM_COMPRESS_TYPE_NENTRIES": 19,   // mm_reduce.h:104
	"QSTATE12_MAXCOLS":          64,   // clifford12.h:52
	"QSTATE12_MAXROWS":          65,   // clifford12.h:53  (QSTATE12_MAXCOLS+1)
	"QSTATE12_UNDEF_ROW":        0xff, // clifford12.h:61
}

func emitCythonYAML(c pxdContents) string {
	funcs := c.Funcs
	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].Source != funcs[j].Source {
			return funcs[i].Source < funcs[j].Source
		}
		return funcs[i].Name < funcs[j].Name
	})

	var b strings.Builder
	b.WriteString("# Auto-generated from mmgroup .pxd files. Do not edit.\n")
	b.WriteString("#\n")
	b.WriteString("# Each entry is a C function exported by mmgroup.\n")
	b.WriteString("# Use this manifest to drive oracle-comparison tests\n")
	b.WriteString("# and translation-parity checks against cgt.\n\n")

	curSource := ""
	for _, f := range funcs {
		if f.Source != curSource {
			if curSource != "" {
				b.WriteString("\n")
			}
			curSource = f.Source
			fmt.Fprintf(&b, "# %s\n", curSource)
		}
		emitCythonFunc(&b, f)
	}

	emitCythonStructs(&b, c.Structs)
	emitCythonEnums(&b, c.Enums)
	return b.String()
}

func emitCythonFunc(b *strings.Builder, f funcDecl) {
	fmt.Fprintf(b, "- name: %s\n", f.Name)
	fmt.Fprintf(b, "  source: %s\n", f.Source)
	fmt.Fprintf(b, "  return: %s\n", f.Return)
	if f.ReturnPtr {
		b.WriteString("  return_ptr: true\n")
	}
	if len(f.Params) == 0 {
		b.WriteString("  params: []\n")
		return
	}
	b.WriteString("  params:\n")
	for _, p := range f.Params {
		fmt.Fprintf(b, "    - type: %s\n", p.Type)
		fmt.Fprintf(b, "      name: %s\n", p.Name)
		if p.Ptr {
			b.WriteString("      ptr: true\n")
		}
		if p.GoType != "" {
			// Pre-computed Go type for declarators goTypeOf cannot
			// express (the xsp2co1_trace_98280 callback, Q27). Quoted:
			// Go types lead with [ or * which are YAML sigils.
			fmt.Fprintf(b, "      gotype: %q\n", p.GoType)
		}
	}
}

// emitCythonStructs records the ctypedef struct handle names so genGo
// can auto-populate goStruct (Q13/Q30). Names are sorted and
// deduplicated for deterministic output; only the names are emitted —
// field layouts stay out of the manifest (Q30).
func emitCythonStructs(b *strings.Builder, structs []string) {
	names := sortedUnique(structs)
	if len(names) == 0 {
		return
	}
	b.WriteString("\n# Opaque ctypedef struct handles (names only; fields\n")
	b.WriteString("# are a downstream concern). Used to populate goStruct.\n")
	for _, s := range names {
		fmt.Fprintf(b, "- struct: %s\n", s)
	}
}

// emitCythonEnums records each enum compile-time constant with the
// numeric value sourced from the C headers (Q29). genGo emits these as
// Go constants.
func emitCythonEnums(b *strings.Builder, enums []string) {
	names := sortedUnique(enums)
	if len(names) == 0 {
		return
	}
	b.WriteString("\n# Enum compile-time constants. Values are read from the\n")
	b.WriteString("# C headers (the .pxd carries names only).\n")
	for _, e := range names {
		v, ok := enumValues[e]
		if !ok {
			// An enum appeared in a .pxd with no value in enumValues.
			// Emit it without a value so the omission is visible rather
			// than silently dropped; genGo treats a value-less enum as
			// an error so this cannot reach go.yaml unnoticed.
			fmt.Fprintf(b, "- enum: %s\n", e)
			continue
		}
		fmt.Fprintf(b, "- enum: %s\n", e)
		fmt.Fprintf(b, "  value: %d\n", v)
	}
}

func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// mm_aux_v24_ints is a C-only internal helper (it packs/unpacks the 24
// integer coordinates of a vector in the rep_24 space) with no Python
// wrapper: mmgroup never exposes it through mm_op or any pure-Python shim,
// so its go.yaml entry carries c: but no py:. The missing py: is intentional
// — it is not a correlation bug, just a C symbol with no Python counterpart
// to correlate against (A2).

// genGo reads the cython.yaml and python.yaml manifests written earlier
// in the pipeline and emits go.yaml, the unified plan for the Go side of
// the translation. The C functions (cython.yaml) and the Python surface
// (python.yaml: classes and module-level functions) are folded into one
// per-package plan: every entry carries c: and/or py: provenance, mapping
// each operation to its target cgt package, an exported Go name, and Go
// parameter/return types (GAPS7_8 §8.5/§8.6, Q1/Q15).
func genGo(cythonPath, pythonPath, outPath string) error {
	data, err := os.ReadFile(cythonPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cythonPath, err)
	}
	man, err := parseCythonYAML(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", cythonPath, err)
	}

	pyData, err := os.ReadFile(pythonPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", pythonPath, err)
	}
	py, err := parsePythonYAML(string(pyData))
	if err != nil {
		return fmt.Errorf("%s: %w", pythonPath, err)
	}

	// The drop tables (dropClasses, dropModuleFns) must track upstream
	// exactly: a key that no longer names a live class/function is a stale
	// drop masking a real change, so fail closed before folding.
	if err := assertDropTablesFresh(py); err != nil {
		return err
	}

	// Populate goStruct from the parsed ctypedef struct names rather
	// than a hardcoded map (Q13/Q30). goTypeOf consults goStruct while
	// resolving struct-pointer parameters in groupGo, so this must run
	// before groupGo.
	populateGoStruct(man.Structs)

	pkgs, err := groupGo(man.Funcs)
	if err != nil {
		return err
	}

	// Fold the Python surface into the C-derived plan: correlate Python
	// entry points with the C functions they wrap (setting py: on the
	// existing entry) and add Python-only entries (constructors, methods,
	// pure-Python helpers) with blank parameter types (PYGEN Q16-A).
	stats, err := foldPython(pkgs, py)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outPath, []byte(emitGoYAML(pkgs, man.Enums)), 0o644); err != nil {
		return err
	}

	total := 0
	for _, p := range pkgs {
		total += len(p.Funcs)
	}
	log.Printf("wrote %d functions across %d packages and %d constants to %s",
		total, len(pkgs), len(man.Enums), outPath)
	log.Printf("python fold: %d correlated to C, %d Python-only added, %d manual TODOs",
		stats.correlated, stats.added, stats.manual)
	log.Printf("dispatch expansion (Q-d): %d ladders expanded into %d sibling methods, %d carried as honest gaps",
		stats.dispatchExpanded, stats.dispatchSiblings, stats.dispatchCarried)
	log.Printf("type supplements (D3): %d blank return/param slots filled from the Q-p/Q-q container mapping",
		stats.supplemented)
	// Residual type gaps after the Python→Go translation ([7]): surfaced to
	// the operator for final determination on how to handle (a Python type
	// with no Go counterpart yet, or a walker AST guess that was not a real
	// type). The counts match `grep -c 'return: ""'` and `grep -c 'type:
	// ""'` over the emitted go.yaml.
	log.Printf("residual type gaps: %d blank returns, %d blank param types (Python types with no Go counterpart yet — flag for operator)",
		stats.blankReturn, stats.blankParam)
	return nil
}

// populateGoStruct fills goStruct from the C ctypedef struct names
// parsed out of the .pxd files (Q13). The Go type name is the camelCase
// (unexported) form of the C name — e.g. qstate12_support_type becomes
// qstate12SupportType — matching the previously hand-maintained map. A
// pointer to one of these is a single object handle, not an array, which
// is why goTypeOf maps it to *Type rather than a slice.
func populateGoStruct(structs []string) {
	for _, c := range structs {
		goStruct[c] = lowerFirstRune(goName(c))
	}
}

// cythonManifest is everything parseCythonYAML reads back from
// cython.yaml: the C functions, the opaque struct handle names (which
// populate goStruct), and the enum constants with their header-sourced
// values (emitted as Go constants).
type cythonManifest struct {
	Funcs   []cythonFunc
	Structs []string
	Enums   []goEnum
}

type cythonFunc struct {
	Name      string
	Return    string
	ReturnPtr bool
	Params    []cythonParam
}

type cythonParam struct {
	Type string
	Name string
	Ptr  bool
	// GoType, when non-empty, is the pre-computed Go type emitted by the
	// supplement table (Q27). It bypasses goTypeOf in groupGo.
	GoType string
}

type goEnum struct {
	Name  string
	Value int64
}

// parseCythonYAML reads the subset of YAML that emitCythonYAML
// produces. It is deliberately minimal rather than a general
// parser: the schema is fixed and authored by this same program.
func parseCythonYAML(s string) (cythonManifest, error) {
	var man cythonManifest
	var cur *cythonFunc
	var curParam *cythonParam
	var curEnum *goEnum
	var enumHasValue bool
	flushParam := func() {
		if curParam != nil {
			cur.Params = append(cur.Params, *curParam)
			curParam = nil
		}
	}
	flushFunc := func() {
		if cur != nil {
			flushParam()
			man.Funcs = append(man.Funcs, *cur)
			cur = nil
		}
	}
	flushEnum := func() error {
		if curEnum != nil {
			if !enumHasValue {
				return fmt.Errorf("enum %q has no value (missing from enumValues?)", curEnum.Name)
			}
			man.Enums = append(man.Enums, *curEnum)
			curEnum = nil
			enumHasValue = false
		}
		return nil
	}
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "- name: "):
			flushFunc()
			cur = &cythonFunc{Name: strings.TrimPrefix(line, "- name: ")}
		case strings.HasPrefix(line, "  return_ptr: true"):
			if cur == nil {
				return cythonManifest{}, fmt.Errorf("return_ptr outside function: %q", line)
			}
			cur.ReturnPtr = true
		case strings.HasPrefix(line, "  return: "):
			if cur == nil {
				return cythonManifest{}, fmt.Errorf("return outside function: %q", line)
			}
			cur.Return = strings.TrimPrefix(line, "  return: ")
		case line == "  params: []":
			// Explicit empty param list. emitCythonYAML writes this line for
			// a zero-param function and then emits no `    - type:` children.
			// Parsing it explicitly makes the empty case correct by
			// construction (cur.Params stays nil) rather than relying on the
			// mere absence of child lines, and flushes any stray pending param
			// instead of silently swallowing it.
			if cur == nil {
				return cythonManifest{}, fmt.Errorf("params outside function: %q", line)
			}
			flushParam()
		case strings.HasPrefix(line, "    - type: "):
			flushParam()
			curParam = &cythonParam{Type: strings.TrimPrefix(line, "    - type: ")}
		case strings.HasPrefix(line, "      name: "):
			if curParam == nil {
				return cythonManifest{}, fmt.Errorf("param name outside param: %q", line)
			}
			curParam.Name = strings.TrimPrefix(line, "      name: ")
		case strings.HasPrefix(line, "      ptr: true"):
			if curParam == nil {
				return cythonManifest{}, fmt.Errorf("ptr outside param: %q", line)
			}
			curParam.Ptr = true
		case strings.HasPrefix(line, "      gotype: "):
			if curParam == nil {
				return cythonManifest{}, fmt.Errorf("gotype outside param: %q", line)
			}
			gt, err := strconv.Unquote(strings.TrimPrefix(line, "      gotype: "))
			if err != nil {
				return cythonManifest{}, fmt.Errorf("gotype: %w", err)
			}
			curParam.GoType = gt
		case strings.HasPrefix(line, "- struct: "):
			flushFunc()
			if err := flushEnum(); err != nil {
				return cythonManifest{}, err
			}
			man.Structs = append(man.Structs, strings.TrimPrefix(line, "- struct: "))
		case strings.HasPrefix(line, "- enum: "):
			flushFunc()
			if err := flushEnum(); err != nil {
				return cythonManifest{}, err
			}
			curEnum = &goEnum{Name: strings.TrimPrefix(line, "- enum: ")}
		case strings.HasPrefix(line, "  value: "):
			if curEnum == nil {
				return cythonManifest{}, fmt.Errorf("value outside enum: %q", line)
			}
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "  value: "), 10, 64)
			if err != nil {
				return cythonManifest{}, fmt.Errorf("enum %s value: %w", curEnum.Name, err)
			}
			curEnum.Value = v
			enumHasValue = true
		}
	}
	flushFunc()
	if err := flushEnum(); err != nil {
		return cythonManifest{}, err
	}
	return man, nil
}

// pythonManifest is everything parsePythonYAML reads back from
// python.yaml: the inspected classes (each with its MRO bases and method
// set) and the module-level functions / Cython builtins. genGo folds both
// into go.yaml (GAPS7_8 §8.5).
type pythonManifest struct {
	Classes   []pyClassDecl
	ModuleFns []pyModuleFn
}

// pyClassDecl is one class entry from python.yaml. Bases are the direct
// base class names (used only as a traceability hint here; the walker has
// already flattened inherited methods onto the class and tagged them
// Inherited, GAPS7_8 Q6).
type pyClassDecl struct {
	Name    string
	Module  string
	Bases   []string
	Methods []pyMethod
	Props   []string
}

// pyMethod is one method entry within a class. Kind is one of method,
// staticmethod, classmethod (the walker's tags); Inherited marks a method
// reached through the MRO rather than defined on the class itself. Return
// is the canonical Python return type the walker probed ("" when
// undetermined); Dispatch carries the argument-polymorphic return ladder
// when present (omitted for monomorphic methods). Calls is the
// non-authoritative C cross-reference hint (PYGEN Q17).
type pyMethod struct {
	Name      string
	Kind      string
	Inherited bool
	Params    []pyYAMLParam
	Calls     []string
	Return    string
	Dispatch  []pyDispatch
}

// pyDispatch is one arm of a method's argument-polymorphic return ladder
// as parsed from python.yaml: On is the matched argument type or value
// ("GCode", "int==2") and Returns is the canonical Python type that arm
// yields. It is the parse-side counterpart of dispatchEntry.
type pyDispatch struct {
	On      string
	Returns string
}

// pyModuleFn is one module-level function / Cython builtin entry
// (kind: function in python.yaml). It has no receiver. Return is the
// canonical Python return type the walker probed, or "" (Cython builtins
// always carry "").
type pyModuleFn struct {
	Name   string
	Module string
	Params []pyYAMLParam
	Calls  []string
	Return string
}

// pyYAMLParam is a parameter parsed from python.yaml: a name, the inspect
// parameter kind (POSITIONAL_OR_KEYWORD, VAR_POSITIONAL, ...), and the
// canonical Python type the walker inferred from an isinstance() test or
// default literal ("" when not probed).
type pyYAMLParam struct {
	Name    string
	Kind    string
	Default string
	Type    string
}

// parsePythonYAML reads the subset of YAML that emitPythonYAML produces.
// Like parseCythonYAML it is a deliberately minimal line parser, not a
// general one: the schema is fixed and authored by this same program. A
// top-level entry is a class when it carries column-2 keys (bases:,
// methods:, properties:) and a module-level function when it carries
// "  kind: function".
func parsePythonYAML(s string) (pythonManifest, error) {
	var man pythonManifest

	// Each top-level "- name:" opens a new entry. We buffer the entry's
	// raw lines and classify it on flush (a "  kind: function" line marks
	// a module-level function; otherwise it is a class).
	var entry []string
	flush := func() error {
		if len(entry) == 0 {
			return nil
		}
		isFunc := false
		for _, ln := range entry {
			if ln == "  kind: function" {
				isFunc = true
				break
			}
		}
		if isFunc {
			fn, err := parsePyModuleFn(entry)
			if err != nil {
				return err
			}
			man.ModuleFns = append(man.ModuleFns, fn)
		} else {
			cls, err := parsePyClass(entry)
			if err != nil {
				return err
			}
			man.Classes = append(man.Classes, cls)
		}
		entry = nil
		return nil
	}

	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "- name: ") {
			if err := flush(); err != nil {
				return pythonManifest{}, err
			}
		}
		entry = append(entry, line)
	}
	if err := flush(); err != nil {
		return pythonManifest{}, err
	}
	return man, nil
}

// parsePyModuleFn parses the buffered lines of a single module-level
// function entry (kind: function). The shape emitted by emitPyFunc is:
//
//   - name: NAME
//     module: MODULE
//     kind: function
//     params: [] | params:\n    - name: .../      kind: .../      default: ...
//     calls: [..]
//     raises: [..]
func parsePyModuleFn(lines []string) (pyModuleFn, error) {
	fn := pyModuleFn{}
	var curParam *pyYAMLParam
	flushParam := func() {
		if curParam != nil {
			fn.Params = append(fn.Params, *curParam)
			curParam = nil
		}
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "- name: "):
			fn.Name = strings.TrimPrefix(line, "- name: ")
		case strings.HasPrefix(line, "  module: "):
			fn.Module = strings.TrimPrefix(line, "  module: ")
		case strings.HasPrefix(line, "  return: "):
			fn.Return = strings.TrimPrefix(line, "  return: ")
		case strings.HasPrefix(line, "    - name: "):
			flushParam()
			curParam = &pyYAMLParam{Name: strings.TrimPrefix(line, "    - name: ")}
		case strings.HasPrefix(line, "      kind: "):
			if curParam != nil {
				curParam.Kind = strings.TrimPrefix(line, "      kind: ")
			}
		case strings.HasPrefix(line, "      type: "):
			if curParam != nil {
				curParam.Type = strings.TrimPrefix(line, "      type: ")
			}
		case strings.HasPrefix(line, "      default: "):
			// A default value is a Python repr() literal emitted verbatim by
			// emitPyFunc, so it may legitimately contain a colon (e.g. a dict
			// or slice repr). That is safe: this is a prefix dispatch over a
			// single physical line, and TrimPrefix captures the entire
			// remainder, so a colon inside the literal is never re-read as a
			// separate key. repr() never emits a raw newline (control chars
			// are escaped), so a default is always one line — no multi-line
			// literal can leak its tail into the next key. This invariant
			// holds for as long as the emitter writes `default: <repr>` last
			// in the param block (the only key that can carry arbitrary text).
			if curParam != nil {
				curParam.Default = strings.TrimPrefix(line, "      default: ")
			}
		case strings.HasPrefix(line, "  calls: ["):
			fn.Calls = parseYAMLInlineList(line, "  calls: ")
		}
	}
	flushParam()
	if fn.Name == "" || fn.Module == "" {
		return pyModuleFn{}, fmt.Errorf("module function entry missing name/module: %q", strings.Join(lines, "\\n"))
	}
	return fn, nil
}

// parsePyClass parses the buffered lines of a single class entry. The
// shape emitted by emitClass nests method entries one indent level
// deeper than module-level functions, so the param keys live at column 8
// (methods) rather than column 6 (module funcs).
func parsePyClass(lines []string) (pyClassDecl, error) {
	cls := pyClassDecl{}
	var curMethod *pyMethod
	var curParam *pyYAMLParam
	var curDispatch *pyDispatch
	flushParam := func() {
		if curMethod != nil && curParam != nil {
			curMethod.Params = append(curMethod.Params, *curParam)
		}
		curParam = nil
	}
	flushDispatch := func() {
		if curMethod != nil && curDispatch != nil {
			curMethod.Dispatch = append(curMethod.Dispatch, *curDispatch)
		}
		curDispatch = nil
	}
	flushMethod := func() {
		flushParam()
		flushDispatch()
		if curMethod != nil {
			cls.Methods = append(cls.Methods, *curMethod)
			curMethod = nil
		}
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "- name: "):
			cls.Name = strings.TrimPrefix(line, "- name: ")
		case strings.HasPrefix(line, "  module: "):
			cls.Module = strings.TrimPrefix(line, "  module: ")
		case strings.HasPrefix(line, "  bases: ["):
			cls.Bases = parseYAMLInlineList(line, "  bases: ")
		case strings.HasPrefix(line, "  properties: ["):
			cls.Props = parseYAMLInlineList(line, "  properties: ")
		case strings.HasPrefix(line, "    - name: "):
			flushMethod()
			curMethod = &pyMethod{Name: strings.TrimPrefix(line, "    - name: ")}
		case strings.HasPrefix(line, "      kind: "):
			if curMethod != nil {
				curMethod.Kind = strings.TrimPrefix(line, "      kind: ")
			}
		case strings.HasPrefix(line, "      return: "):
			if curMethod != nil {
				curMethod.Return = strings.TrimPrefix(line, "      return: ")
			}
		case strings.HasPrefix(line, "      inherited: true"):
			if curMethod != nil {
				curMethod.Inherited = true
			}
		case strings.HasPrefix(line, "        - name: "):
			flushParam()
			curParam = &pyYAMLParam{Name: strings.TrimPrefix(line, "        - name: ")}
		case strings.HasPrefix(line, "          kind: "):
			if curParam != nil {
				curParam.Kind = strings.TrimPrefix(line, "          kind: ")
			}
		case strings.HasPrefix(line, "          type: "):
			if curParam != nil {
				curParam.Type = strings.TrimPrefix(line, "          type: ")
			}
		case strings.HasPrefix(line, "          default: "):
			if curParam != nil {
				curParam.Default = strings.TrimPrefix(line, "          default: ")
			}
		case strings.HasPrefix(line, "        - on: "):
			flushParam()
			flushDispatch()
			curDispatch = &pyDispatch{On: strings.TrimPrefix(line, "        - on: ")}
		case strings.HasPrefix(line, "          returns: "):
			if curDispatch != nil {
				curDispatch.Returns = strings.TrimPrefix(line, "          returns: ")
			}
		case strings.HasPrefix(line, "      calls: ["):
			if curMethod != nil {
				curMethod.Calls = parseYAMLInlineList(line, "      calls: ")
			}
		}
	}
	flushMethod()
	if cls.Name == "" || cls.Module == "" {
		return pyClassDecl{}, fmt.Errorf("class entry missing name/module: %q", strings.Join(lines, "\\n"))
	}
	return cls, nil
}

// parseYAMLInlineList parses an inline-list line of the form
// "<prefix>[a, b, c]" into ["a", "b", "c"]. An empty "[]" yields nil. The
// prefix is the key including its trailing space (e.g. "  bases: ").
func parseYAMLInlineList(line, prefix string) []string {
	v := strings.TrimPrefix(line, prefix)
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// pkgRule maps a C prefix to a target Go package. Every rule simply
// strips its prefix; the remainder becomes the Go name. Rules are
// scanned in order, so longer/more-specific prefixes appear first.
// Collapsing several C prefixes into one package can make distinct C
// symbols reduce to the same identifier (e.g. bitvector32_sort and
// bitvector64_sort both reduce to "sort"); such collisions are
// resolved explicitly via renameOverrides and otherwise rejected by
// assertNoCollision.
type pkgRule struct {
	prefix string
	pkg    string
}

var pkgRules = []pkgRule{
	{prefix: "mat24_", pkg: "mat24"},
	{prefix: "mm_", pkg: "mm"},
	{prefix: "gen_", pkg: "generator"},
	{prefix: "xsp2co1_", pkg: "xsp2co1"},
	{prefix: "qstate12_", pkg: "qstate12"},
	{prefix: "gt_", pkg: "reduce"},

	// clifford12 bit-twiddling utilities collapse into "swar".
	{prefix: "bitmatrix64_", pkg: "swar"},
	{prefix: "bitvector64_", pkg: "swar"},
	{prefix: "bitvector32_", pkg: "swar"},
	{prefix: "bitvector_", pkg: "swar"},
	{prefix: "uint64_", pkg: "swar"},

	// Leech-lattice helpers collapse into "leech".
	{prefix: "leech2matrix_", pkg: "leech"},
	{prefix: "leech3matrix_", pkg: "leech"},
	{prefix: "leech2_", pkg: "leech"},
	{prefix: "leech3_", pkg: "leech"},

	// generators.pxd outliers that lack a gen_ prefix.
	{prefix: "find_", pkg: "generator"},
	{prefix: "apply_", pkg: "generator"},
}

// renameOverrides maps a C symbol to the Go name it must use, bypassing
// the computed name from goName. It resolves collisions that arise when
// collapsing width-bearing C prefixes into a single package: e.g.
// bitvector32_sort and bitvector64_sort would both reduce to "sort".
var renameOverrides = map[string]string{
	"bitvector32_sort":     "SortBV32",
	"bitvector64_sort":     "SortBV64",
	"bitvector32_heapsort": "HeapsortBV32",
	"bitvector64_heapsort": "HeapsortBV64",
	"bitvector32_copy":     "CopyBV32",
	"bitvector64_copy":     "CopyBV64",
	"bitvector32_bsearch":  "BsearchBV32",
	"bitvector64_bsearch":  "BsearchBV64",
}

// pyModuleRule maps a Python module prefix to the target Go package, the
// counterpart of pkgRules for the Python side of the manifest. It places
// each python.yaml class and each pure-Python module-level function (the
// ones whose name does not coincide with a C symbol, so packageOf cannot
// route them) into one of the eight existing go.yaml packages.
//
// Rules are scanned in order; the first whose prefix the module matches
// wins, so longer/more-specific prefixes must precede the shorter ones
// they overlap (e.g. mmgroup.mm_op before a hypothetical mmgroup.mm).
// The mapping is by mathematical domain, grounded in where the
// corresponding C functions already land under pkgRules and in the
// ratified per-class targets:
//
//   - mmgroup.bimm.* / mm_group / mm_space / mm_crt_space /
//     mm_op / structures.{mm0_group,construct_mm,mm_order,involutions,
//     random_mm,parse_atoms} / structures.abstract_{group,mm_group,
//     mm_rep_space} / tests.axes.* → mm (the Monster surface; GAP5.md §5
//     places Axis/BabyAxis here, and the bimm/group/word machinery are
//     all Monster-level operations).
//   - mmgroup.mm_reduce → reduce (the GtWord/GtSubWord reduction machinery;
//     Q-h places these in the reduce package alongside the gt_* C functions).
//   - mmgroup.clifford12 / structures.qs_matrix → qstate12 (the
//     quadratic-state-matrix kernel; GAPS7_8 §7.6 maps QState12 here).
//   - mmgroup.structures.xsp2_co1 → xsp2co1 (G_x0 elements).
//   - mmgroup.structures.xleech2 → leech (extended Leech lattice mod 2).
//   - mmgroup.structures.{autpl,cocode,gcode,ploop,parity} → mat24
//     (Golay code / Mathieu-group structures the mat24 package backs).
//   - mmgroup.general.orbit_lin2 → generator (the gen_ufind_lin2_* union-find
//     over F2, which pkgRules already routes to generator).
//   - mmgroup.bitfunctions → swar (bit-twiddling helpers, the package the
//     bit* C utilities collapse into).
//
// structures.abstract_rep_space and abstract_mm_rep_space are abstract
// vector ABCs whose concrete leaves (MMVector) live in mm, so they map to
// mm as well; their methods flatten onto the concrete mm types (Q21).
type pyModuleRule struct {
	prefix string
	pkg    string
}

var pyModuleRules = []pyModuleRule{
	// clifford12 kernel and its pure-Python matrix subclass.
	{prefix: "mmgroup.clifford12", pkg: "qstate12"},
	{prefix: "mmgroup.structures.qs_matrix", pkg: "qstate12"},

	// G_x0 / Co_1 elements.
	{prefix: "mmgroup.structures.xsp2_co1", pkg: "xsp2co1"},

	// Extended Leech lattice mod 2.
	{prefix: "mmgroup.structures.xleech2", pkg: "leech"},

	// Golay code / Mathieu-group structures backed by mat24.
	{prefix: "mmgroup.structures.autpl", pkg: "mat24"},
	{prefix: "mmgroup.structures.cocode", pkg: "mat24"},
	{prefix: "mmgroup.structures.gcode", pkg: "mat24"},
	{prefix: "mmgroup.structures.ploop", pkg: "mat24"},
	{prefix: "mmgroup.structures.parity", pkg: "mat24"},
	{prefix: "mmgroup.structures.suboctad", pkg: "mat24"},
	{prefix: "mmgroup.mat24", pkg: "mat24"},

	// Union-find over F2.
	{prefix: "mmgroup.general.orbit_lin2", pkg: "generator"},
	{prefix: "mmgroup.generators", pkg: "generator"},

	// Bit-twiddling helpers.
	{prefix: "mmgroup.bitfunctions", pkg: "swar"},

	// Everything Monster-level: bimm, the MM group/word/space/reduce
	// machinery, the abstract bases, and the axis reducers.
	{prefix: "mmgroup.bimm", pkg: "mm"},
	{prefix: "mmgroup.mm_group", pkg: "mm"},
	{prefix: "mmgroup.mm_space", pkg: "mm"},
	{prefix: "mmgroup.mm_crt_space", pkg: "mm"},
	{prefix: "mmgroup.mm_reduce", pkg: "reduce"},
	{prefix: "mmgroup.mm_op", pkg: "mm"},
	{prefix: "mmgroup.structures", pkg: "mm"},
	{prefix: "mmgroup.tests.axes", pkg: "mm"},
}

// packageOfModule routes a Python module to its Go package via the first
// matching pyModuleRule prefix. ok is false when no rule matches, which
// genGo treats as a hard error so a newly surfaced module cannot silently
// vanish from go.yaml.
func packageOfModule(module string) (pkg string, ok bool) {
	for _, r := range pyModuleRules {
		if module == r.prefix || strings.HasPrefix(module, r.prefix+".") {
			return r.pkg, true
		}
	}
	return "", false
}

type goPackage struct {
	Name  string
	Funcs []goFunc
}

// goDispatch is one arm of an argument-polymorphic method's return
// ladder in the Go plan: On is the Go-translated dispatch guard ("int",
// "int==2") and Returns is the Go-translated return type that arm yields
// ("*GCode", "*Parity"). It is the go.yaml-side counterpart of pyDispatch
// (carried verbatim through foldPython after pyTypeToGo translation).
//
// expandDispatch (Q-d, ratified) folds each fully-resolved ladder into
// per-branch Go methods: the dominant (first) arm keeps the bare method name
// and adopts its return type, and each typed arm becomes a sibling method
// named per the Q-f convention (preposition + operand: MulByMM, DivBy2,
// PowByXLeech2; the bare name for the general arm; the R prefix retained on a
// reflected operator, RMulByInt). A value arm whose return matches the
// dominant arm folds into the general body rather than spawning a method.
// Only a ladder with a blank guard (on: "") or a blank return is left
// carried — an honest gap emitGoFunc writes as a dispatch: block, the residue
// for a later type-recovery pass.
type goDispatch struct {
	On      string
	Returns string
}

type goFunc struct {
	Name string // exported Go name by default
	C    string // original C symbol, for traceability ("" if Python-only)
	Py   string // Python provenance: dotted "Class.method" for a class
	// method/constructor, dotless "module_func" for a module-level
	// function (GAPS7_8 §8.5, Q15/M3). "" if C-only.
	Return     string // "" for void
	Params     []goParam
	Unexported bool   // emit as an unexported identifier
	Recv       string // receiver type for a Python instance method (e.g.
	// "QState"); "" for a free function. Drives the Go form: a non-empty
	// Recv emits `func (r *Recv) Name(...)`, empty emits `func Name(...)`
	// (PYMETH_TO_GOFUNC kind rules).
	Manual string // non-empty marks a TODO the generator cannot resolve
	// (e.g. an overloaded/variadic __init__ form); the value is the reason.
	Note string // non-empty attaches an informational note to the entry,
	// emitted as a note: line. Unlike Manual it is not a TODO: it records a
	// resolved decision (e.g. a constructor family's sibling list, or "this
	// class is a plain data container with no constructor"). Carried by the
	// data-driven manualConstructors supplement, never by the fold.
	Calls []string // non-authoritative C cross-reference hint for a
	// Python-derived entry (PYGEN_PARAM_TYPE Q17): the resolved C matches
	// from the method/function body, surfaced for whoever fills in types.
	Dispatch []goDispatch // argument-polymorphic return ladder, when the
	// method's return type depends on the argument type/value (e.g.
	// GCode.__truediv__: int==2 → *Parity, else → *GCode). Empty for a
	// monomorphic method. When present, Return holds the principal
	// (first-arm) return and the full ladder is emitted as a dispatch:
	// block for the human to fold into per-branch Go methods.
}

type goParam struct {
	Name string
	Type string
	// Default is the Python default-value literal for this parameter,
	// carried verbatim from python.yaml (e.g. "119", "True", "'r'",
	// "<class 'mmgroup.mm_group.MM'>", "None"). "" when the parameter has
	// no default. It is a type hint for the deferred type-recovery stages:
	// the literal's runtime type is the parameter's type (an int literal
	// implies int, a quoted literal implies string, a <class 'X'> implies
	// the named type). C-derived params never carry one (the C signature
	// has no defaults). It is emitted as a hint line on the param and is
	// never round-trip-parsed back out of go.yaml.
	Default string
}

func groupGo(cfuncs []cythonFunc) ([]goPackage, error) {
	byPkg := map[string][]goFunc{}
	for _, cf := range cfuncs {
		pkg, rem, ok := packageOf(cf.Name)
		if !ok {
			return nil, fmt.Errorf("no package rule for %q", cf.Name)
		}
		name := goName(rem)
		if ov, has := renameOverrides[cf.Name]; has {
			name = ov
		}
		gf := goFunc{
			Name: name,
			C:    cf.Name,
		}
		ret, err := goTypeOf(cf.Return, cf.ReturnPtr)
		if err != nil {
			return nil, fmt.Errorf("%s: return: %w", cf.Name, err)
		}
		gf.Return = ret
		for _, p := range cf.Params {
			// A pre-computed Go type (the supplement's function-pointer
			// param, Q27) bypasses goTypeOf entirely.
			t := p.GoType
			if t == "" {
				t, err = goTypeOf(p.Type, p.Ptr)
				if err != nil {
					return nil, fmt.Errorf("%s: param %s: %w", cf.Name, p.Name, err)
				}
			}
			gf.Params = append(gf.Params, goParam{
				Name: goParamName(p.Name),
				Type: t,
			})
		}
		byPkg[pkg] = append(byPkg[pkg], gf)
	}

	var pkgs []goPackage
	for name, funcs := range byPkg {
		sort.Slice(funcs, func(i, j int) bool {
			return funcs[i].C < funcs[j].C
		})
		if err := assertNoCollision(name, funcs); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, goPackage{Name: name, Funcs: funcs})
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
	return pkgs, nil
}

func packageOf(cname string) (pkg, rem string, ok bool) {
	for _, r := range pkgRules {
		if strings.HasPrefix(cname, r.prefix) {
			return r.pkg, cname[len(r.prefix):], true
		}
	}
	return "", "", false
}

// goName converts a snake_case C remainder to an exported
// PascalCase Go identifier. All-caps and digit-bearing tokens from
// the C source (Co1, Gx0, ABC, 2A, 32bit) are preserved verbatim
// apart from upper-casing the first letter of each segment, so the
// result stays traceable to the C symbol.
func goName(rem string) string {
	var b strings.Builder
	for _, seg := range strings.Split(rem, "_") {
		if seg == "" {
			continue
		}
		b.WriteString(upperFirstRune(seg))
	}
	id := b.String()
	if id == "" {
		return "fn"
	}
	if c := id[0]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		id = "x" + id
	}
	return id
}

// constNameOverrides maps C constant symbols to exported Go names that
// goConstName would otherwise mangle. goConstName lowercases every
// segment after its leading letter, so embedded initialisms collapse
// (QSTATE12 → Qstate12). A targeted override table is used here instead
// of a general initialism dictionary: the broader goName initialism
// drift (UfindInit vs UFindInit, OpXy vs OpXY) is deferred (Q-e), and
// only the three QSTATE12 constants are reconciled now because their
// casing diverges from the struct-handle naming pattern. SortStat
// (bitvector_sort_stat) is intentionally left as-is (Q-c).
var constNameOverrides = map[string]string{
	"QSTATE12_MAXCOLS":   "QState12MaxCols",
	"QSTATE12_MAXROWS":   "QState12MaxRows",
	"QSTATE12_UNDEF_ROW": "QState12UndefRow",
}

// goConstName converts an ALL_CAPS C macro name to PascalCase
// (e.g. MAX_GT_WORD_DATA → MaxGtWordData).
func goConstName(name string) string {
	if override, ok := constNameOverrides[name]; ok {
		return override
	}
	var b strings.Builder
	for _, seg := range strings.Split(name, "_") {
		if seg == "" {
			continue
		}
		b.WriteByte(seg[0])
		if len(seg) > 1 {
			b.WriteString(strings.ToLower(seg[1:]))
		}
	}
	return b.String()
}

func upperFirstRune(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-'a'+'A') + s[1:]
	}
	return s
}

func lowerFirstRune(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'A' && c <= 'Z' {
		return string(c-'A'+'a') + s[1:]
	}
	return s
}

// goParamName derives a Go parameter identifier from a C parameter
// name. Parameters are locals, so unlike goName's exported result the
// name is lowered to camelCase; a trailing underscore disambiguates
// Go keywords (e.g. the C param "map" becomes "map_").
func goParamName(c string) string {
	n := lowerFirstRune(goName(c))
	if isGoKeyword(n) {
		return n + "_"
	}
	return n
}

func isGoKeyword(s string) bool {
	switch s {
	case "break", "case", "chan", "const", "continue", "default",
		"defer", "else", "fallthrough", "for", "func", "go", "goto",
		"if", "import", "interface", "map", "package", "range",
		"return", "select", "struct", "switch", "type", "var":
		return true
	}
	return false
}

// goScalar maps a scalar C type to its Go equivalent. size_t maps to
// uint, Go's platform-native unsigned size type and the direct
// counterpart to C's size_t for a count/length (Q11); not uintptr
// (which carries pointer-arithmetic semantics) nor uint64 (which
// over-commits to a width). On this LP64 target uint is 64-bit.
var goScalar = map[string]string{
	"void":       "",
	"int8_t":     "int8",
	"uint8_t":    "uint8",
	"int16_t":    "int16",
	"uint16_t":   "uint16",
	"int32_t":    "int32",
	"uint32_t":   "uint32",
	"int64_t":    "int64",
	"uint64_t":   "uint64",
	"size_t":     "uint",
	"double":     "float64",
	"uint_mmv_t": "uint64", // typedef uint64_t (mm_basics.h)
}

// goStruct maps the opaque mmgroup C struct handles to Go struct type
// names. Unlike scalar pointers, a pointer to one of these is a single
// object handle, not an array. It is populated at run time by
// populateGoStruct from the ctypedef struct names parsed out of the
// .pxd files (Q13), so it stays in sync with mmgroup automatically
// rather than being hand-maintained.
var goStruct = map[string]string{}

// goTypeOf maps a C type to its Go form. Every scalar pointer in
// the mmgroup C API is an array: the .pxi wrappers uniformly pass
// such arguments as &buf[0] from a typed memoryview, and none are
// single-value out-parameters. Scalar pointers therefore become
// slices; struct-handle pointers become *Type.
func goTypeOf(ctype string, ptr bool) (string, error) {
	// void * is an untyped caller-supplied buffer (e.g. gt_word_alloc's
	// p_buffer scratch arena); map it to unsafe.Pointer (Q12). Whoever
	// consumes go.yaml must arrange the unsafe import.
	if ctype == "void" && ptr {
		return "unsafe.Pointer", nil
	}
	if st, ok := goStruct[ctype]; ok {
		if ptr {
			return "*" + st, nil
		}
		return st, nil
	}
	base, ok := goScalar[ctype]
	if !ok {
		return "", fmt.Errorf("unknown C type %q", ctype)
	}
	if base == "" {
		if ptr {
			return "", fmt.Errorf("pointer to void is unsupported")
		}
		return "", nil
	}
	if ptr {
		return "[]" + base, nil
	}
	return base, nil
}

// pyScalarToGo maps the canonical Python scalar/builtin type names the
// walker emits to their Go counterparts. None/NoneType is void (the Go
// "no return", emitted as "" exactly like a C void). list and tuple have
// no faithful element-typed Go form yet (the walker records only the
// container, not the element type), so they stay un-translated and fall
// through to "" — an honest gap rather than a guessed []interface{}.
var pyScalarToGo = map[string]string{
	"int":      "int",
	"Integral": "int", // numbers.Integral ABC (gcode.py imports it for the
	// isinstance() gate on the int-dispatch dunders); the realized Python
	// value is always a plain int, so the Go type is int.
	"bool":     "bool",
	"str":      "string",
	"float":    "float64",
	"None":     "", // void
	"NoneType": "", // void
}

// pyClassToGo maps the mmgroup Python class names that have a Go struct
// counterpart to that Go type. The Go form is a pointer (*ClassName): an
// mmgroup object is a single handle, never an array, mirroring how
// goTypeOf renders a C struct-handle pointer. Only classes with a real
// Go surface are listed; a Python class with no Go counterpart yet is
// deliberately absent so pyTypeToGo returns "" for it (honest gap).
//
// PLoopIntersection is a private Cocode subclass (cocode.py: class
// PLoopIntersection(Cocode)) used only as the result of PLoop & PLoop;
// it has no public Go type of its own, so it collapses to *Cocode per
// the [7] "collapse private subtypes" rule.
//
// Dispatch-arm semantics worth recording for whoever folds the emitted
// dispatch: ladders into Go (A3); these are notes, not behavior:
//
//   - XLeech2.Mul carries a blank guard (on: "") whose Python operand type
//     is AbstractMMGroupWord, the ABC with no Go struct; the concrete Go
//     type is *MM. The blank guard is an honest gap (the ABC is not in
//     pyClassToGo), not a missing translation.
//   - XLeech2.Pow by *XLeech2 is group conjugation x^y = y^{-1}xy in Q_{x0},
//     not exponentiation; the *XLeech2 exponent is a conjugator, not a count.
//   - XLeech2.RDiv / RMul with int==0 is annihilation — both return the
//     identity XLeech2(0). Upstream Python's __rtruediv__ (xleech2.py line
//     482) dereferences other.value before the isinstance check, an upstream
//     bug; the Go port must guard the type first and must not replicate it.
//   - GCode.Div is a ladder, and it is correct as emitted: /1 is identity,
//     /2 always yields Parity(0), /4 is the power map. The leading int arm is
//     the type-assertion gate (the operand must be int), not a separate
//     return branch, so it does not widen the return set.
var pyClassToGo = map[string]string{
	"GCode":             "*GCode",
	"Cocode":            "*Cocode",
	"PLoop":             "*PLoop",
	"PLoopIntersection": "*Cocode", // collapse private subtype
	"GcVector":          "*GcVector",
	"Parity":            "*Parity",
	"AutPL":             "*AutPL",
	"XLeech2":           "*XLeech2",
	"MM":                "*MM",
	"Xsp2_Co1":          "*Xsp2Co1",
	"MMVector":          "*MMVector",
	"MMVectorCRT":       "*MMVectorCRT",
	"QStateMatrix":      "*QStateMatrix",
	"QState12":          "*QState12",
}

// pyTypeToGo translates a canonical Python type name from python.yaml to
// its Go form. It is the Python-side counterpart of goTypeOf: scalars map
// through pyScalarToGo, known mmgroup classes through pyClassToGo. An
// empty input, or any name in neither table (an un-Go-able container like
// list/tuple, an internal helper/local name the walker's AST static
// analysis mistook for a type, or a class with no Go surface yet), maps
// to "" — the same honest-gap blank the deferred type-recovery stages
// were filling before, never a guessed type. Returning "" for an unknown
// is load-bearing: it is what keeps go.yaml from asserting a type it
// cannot stand behind.
func pyTypeToGo(pyType string) string {
	if pyType == "" {
		return ""
	}
	if g, ok := pyScalarToGo[pyType]; ok {
		return g
	}
	if g, ok := pyClassToGo[pyType]; ok {
		return g
	}
	return ""
}

// pyGuardToGo translates a dispatch arm's "on:" guard from its Python
// form to the Go form for go.yaml. A pure type guard ("Integral",
// "GCode") translates as a type through pyTypeToGo. A value guard
// ("int==2", "int==4") names a concrete value of an already-Go type: the
// "TYPE==VALUE" shape is kept, translating only the TYPE half (here int
// is already Go, so it passes through unchanged). An untranslatable guard
// type yields "" so the arm records its return without asserting a guard
// type it cannot stand behind.
func pyGuardToGo(on string) string {
	if i := strings.Index(on, "=="); i >= 0 {
		typ := pyTypeToGo(on[:i])
		return typ + on[i:]
	}
	return pyTypeToGo(on)
}

// collisionKey identifies the Go identifier space an entry occupies. A
// receiver method (Recv != "") may share its method name with a method on
// a different receiver or with a free function, exactly as Go allows, so
// its key includes the receiver. A free function occupies the package
// namespace and keys on its bare name.
func collisionKey(f goFunc) string {
	if f.Recv != "" {
		return "(" + f.Recv + ")." + f.Name
	}
	return f.Name
}

func assertNoCollision(pkg string, funcs []goFunc) error {
	seen := make(map[string]string, len(funcs))
	for _, f := range funcs {
		key := collisionKey(f)
		if prev, ok := seen[key]; ok {
			return fmt.Errorf("package %s: %s and %s both map to %q",
				pkg, prev, provenance(f), key)
		}
		seen[key] = provenance(f)
	}
	return nil
}

// provenance renders an entry's origin for diagnostics: its C symbol, its
// Python entry point, or both.
func provenance(f goFunc) string {
	switch {
	case f.C != "" && f.Py != "":
		return f.C + " (py " + f.Py + ")"
	case f.C != "":
		return f.C
	case f.Py != "":
		return "py " + f.Py
	default:
		return f.Name
	}
}

// foldStats counts the outcome of folding the Python surface into the
// C-derived plan, for the genGo log line.
type foldStats struct {
	correlated int // Python entry points matched onto an existing C entry
	added      int // Python-only entries with no C counterpart
	manual     int // entries flagged as a manual TODO (overloaded __init__)
	// blankReturn / blankParam count the residual type gaps after the
	// Python→Go type translation ([7]): Python-derived entries (Py != "")
	// whose return — or whose params — still resolved to "". A residual
	// blank is a Python type with no Go counterpart yet (a list/tuple
	// container, a not-yet-ported class, or a walker AST guess that was not
	// a real type); the spec requires it be surfaced to the human operator
	// rather than hidden. These are reported, not errored: the gap is
	// expected and shrinking, not a failure.
	blankReturn int
	blankParam  int
	// dispatchExpanded counts dispatch ladders that were expanded into
	// per-branch methods (Q-d); dispatchSiblings counts the sibling methods
	// those expansions added; dispatchCarried counts ladders left unexpanded
	// as honest gaps (a blank guard or blank return in some arm).
	dispatchExpanded int
	dispatchSiblings int
	dispatchCarried  int
	// supplemented counts return/param type slots the hand-resolved
	// typeSupplements table filled over the merged plan (D3): Python-only
	// entries whose container return (or argument) the walker could not type
	// but the ratified Q-p/Q-q container mapping resolves from the source.
	supplemented int
}

// foldPython folds the Python surface (python.yaml classes and
// module-level functions) into the C-derived package plan, in place.
//
// Each Python entry point is routed to a Go package and given a Go name,
// then either correlated with an existing C entry that shares that Go name
// in that package — setting the entry's py: provenance — or appended as a
// new Python-only entry. Python-derived parameters and returns carry the
// Go type translated from python.yaml's canonical Python type ([7],
// pyTypeToGo), or "" where the Python type has no Go counterpart yet; the
// residual blanks are counted for the operator. The resolved C-name
// matches from the body are carried as a non-authoritative calls: hint
// (PYGEN Q17).
//
// Mapping of kinds to Go form follows PYMETH_TO_GOFUNC:
//
//   - method            → receiver method on the class's Go type
//   - __init__          → NewType constructor (free function); a variadic
//     or *args/**kwargs form is still emitted but flagged Manual
//   - staticmethod      → package-level free function
//   - classmethod       → package-level free function
//   - inherited methods → already flattened onto the class by the walker
//     (Q21); deduplicated by Go name across the class's method set here
func foldPython(pkgs []goPackage, py pythonManifest) (foldStats, error) {
	var stats foldStats

	// Locate each existing package by name. Every pyModuleRule target is a
	// package the C side already produced, so no package is created here; a
	// Python entry routed to an unknown package is a hard error (it means a
	// pyModuleRule and pkgRules disagree on the package set).
	pkgIdx := map[string]int{}
	for i := range pkgs {
		pkgIdx[pkgs[i].Name] = i
	}

	// Collect the Python-derived entries per package before merging, so the
	// correlation pass can see the full C surface first.
	pending := map[string][]goFunc{}
	route := func(pkgName string, gf goFunc) error {
		if _, ok := pkgIdx[pkgName]; !ok {
			return fmt.Errorf("python entry %q routed to package %q, which has no C functions (pyModuleRules/pkgRules package-set mismatch)", provenance(gf), pkgName)
		}
		pending[pkgName] = append(pending[pkgName], gf)
		return nil
	}

	// Module-level functions: the receiver-less Python analogue of the
	// package-level C functions. The ones whose name is a C symbol correlate
	// directly onto the matching go.yaml entry (GAPS7_8 §8.6).
	for _, fn := range py.ModuleFns {
		if fn.Module == pyDropModule {
			continue // pytest scaffolding, not API (kept out of go.yaml)
		}
		if dropModuleFn(fn) {
			continue // Python lazy-import bootstrap shim or a pure-Python
			// duplicate of a C-backed function (see dropModuleFn).
		}
		pkg, name := goSiteForModuleFn(fn)
		if pkg == "" {
			return foldStats{}, fmt.Errorf("no package for module function %s (module %s)", fn.Name, fn.Module)
		}
		if err := route(pkg, goFunc{
			Name:   name,
			Py:     fn.Name, // dotless: module-level free function
			Return: pyTypeToGo(fn.Return),
			Params: paramsFromPython(fn.Params),
			Calls:  cMatches(fn.Calls),
		}); err != nil {
			return foldStats{}, err
		}
	}

	// isConcreteLeaf marks a class no other non-dropped class derives from
	// (H11, Q-a). Inherited methods are flattened onto every class by the
	// walker (Q21); emitting them on a non-leaf as well as its subclass would
	// duplicate the same operation across the parent and the most-derived
	// type. Restricting inherited methods to the leaf collapses each onto the
	// one concrete type that actually realizes it. A class listed as a base
	// of a dropped class does not lose its leaf status on that account; only
	// surviving (non-dropped) subclasses keep a parent off the leaf set.
	usedAsBase := map[string]bool{}
	for _, cls := range py.Classes {
		if _, dropped := dropClasses[cls.Name]; dropped {
			continue
		}
		for _, b := range cls.Bases {
			usedAsBase[b] = true
		}
	}
	isConcreteLeaf := func(name string) bool { return !usedAsBase[name] }

	// Classes: each contributes constructors, methods, and static/class
	// functions to its package.
	for _, cls := range py.Classes {
		if reason, dropped := dropClasses[cls.Name]; dropped {
			_ = reason // documented in dropClasses; not surfaced per-entry
			continue   // not part of the Go API surface (A5)
		}
		// A class whose pyClassToGo entry collapses it onto a different Go
		// type (PLoopIntersection → *Cocode) is a private subtype: its
		// methods already live on the parent receiver, so emitting them here
		// under recv: PLoopIntersection would duplicate the parent's API on a
		// type with no Go struct (A4). The collapse is detected by the
		// mapping differing from the identity "*"+className.
		if g, ok := pyClassToGo[cls.Name]; ok && g != "*"+cls.Name {
			continue
		}
		pkg, ok := packageOfModule(cls.Module)
		if !ok {
			return foldStats{}, fmt.Errorf("no package for class %s (module %s)", cls.Name, cls.Module)
		}
		recv := goName(cls.Name) // the Go receiver type for instance methods
		leaf := isConcreteLeaf(cls.Name)
		// Dedup by Go name + receiver across the class's flattened method set
		// so a base and an override do not both emit (Q21/rule 5). order
		// records first-seen dedup keys; chosen maps each to the goFunc that
		// wins, so collisions resolve to the richer-typed copy (H11 step 3)
		// while routing stays in deterministic first-seen order.
		var order []string
		chosen := map[string]goFunc{}
		for _, m := range cls.Methods {
			// Inherited methods only land on the leaf concrete type (H11): on
			// a non-leaf the same method is re-emitted on the subclass that
			// inherits it, so skip it here to avoid the parent/child duplicate.
			if m.Inherited && !leaf {
				continue
			}
			gf, ok := goFuncForMethod(cls.Name, recv, m)
			if !ok {
				continue // a dunder with no Go surface (rare; skip quietly)
			}
			dedupKey := gf.Name + "/" + gf.Recv
			if prev, seen := chosen[dedupKey]; seen {
				// A leaf may inherit a blank-typed copy of a method a parent
				// already supplied with C-derived types; keep the richer one
				// (more non-blank param/return types) (H11 step 3).
				if richerTyping(gf, prev) {
					chosen[dedupKey] = gf
				}
				continue
			}
			chosen[dedupKey] = gf
			order = append(order, dedupKey)
		}
		for _, k := range order {
			if err := route(pkg, chosen[k]); err != nil {
				return foldStats{}, err
			}
		}
	}

	// Merge each package's pending Python entries into its C-derived plan.
	for name, pyFuncs := range pending {
		p := &pkgs[pkgIdx[name]]
		mergePython(p, pyFuncs, &stats)
	}

	// Expand each eligible dispatch ladder into per-branch Go methods (Q-d):
	// the dominant arm keeps the bare method name and gains its return type,
	// each typed arm becomes a sibling method named per Q-f, and the dispatch
	// block is dropped. Ladders with a blank guard or blank return are left
	// carried as honest gaps. Done before the constructor-family fold so the
	// sibling additions ride the same sort/collision/blank-tally passes.
	for i := range pkgs {
		expandDispatch(&pkgs[i], &stats)
	}

	// Fold the hand-resolved constructor families (A7) over the now-merged
	// plan: rewrite each overloaded __init__ principal into its resolved Go
	// form and append the sibling constructors. Done before the sort/validate
	// and blank-tally passes below so the additions are ordered, collision-
	// guarded, and counted like every other entry.
	if err := mergeManualConstructors(pkgs); err != nil {
		return foldStats{}, err
	}

	// Fill the residual blank return/param type slots the walker left for the
	// hand-resolved entries in typeSupplements (D3). Each blank is a Python
	// container the walker could not type; the supplement resolves it from the
	// source per the ratified Q-p/Q-q container mapping. Done after every
	// merge so the keyed entries exist, and before the blank tally below so
	// the filled slots drop out of the residual count.
	if err := mergeTypeSupplements(pkgs, &stats); err != nil {
		return foldStats{}, err
	}

	// Order every package's entries deterministically and re-validate the
	// (now receiver-aware) namespace.
	for i := range pkgs {
		p := &pkgs[i]
		sort.Slice(p.Funcs, func(a, b int) bool {
			return goFuncLess(p.Funcs[a], p.Funcs[b])
		})
		if err := assertNoCollision(p.Name, p.Funcs); err != nil {
			return foldStats{}, err
		}
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })

	// Tally residual type gaps over the finalized plan so the count matches
	// what emitGoFunc writes: a blank return: line appears for a Python-only
	// entry (Py != "" && C == "") with no resolved return, and a blank
	// type: line for any param that did not resolve. Entries with c:
	// provenance and a blank return are void C functions, not unresolved, so
	// they are excluded. These are the residual blanks the spec requires be
	// surfaced to the operator ([7]).
	for i := range pkgs {
		for _, f := range pkgs[i].Funcs {
			if f.Return == "" && f.Py != "" && f.C == "" {
				stats.blankReturn++
			}
			for _, p := range f.Params {
				if p.Type == "" {
					stats.blankParam++
				}
			}
		}
	}
	return stats, nil
}

// mergePython folds a package's Python-derived entries into its existing
// C-derived func list. Correlation is by bare Go name: a Python entry
// whose name matches a C entry that has no Python provenance yet annotates
// that C entry (setting py:, recv:, and the calls: hint) rather than
// producing a duplicate — this is the Python-method-wraps-C-function chain
// (GAPS7_8 §7.6). Whichever C-derived params/return the entry already has
// are kept; they are richer than the type-less Python view. A Python entry
// with no C counterpart is appended; a second Python entry that collides
// with an already-folded one (same name and receiver) is dropped, the
// first winning (Q21/rule 5).
func mergePython(p *goPackage, pyFuncs []goFunc, stats *foldStats) {
	// byName indexes only the C-derived entries, for correlation by bare
	// Go name. seen tracks every Go identifier already occupied (C entries
	// and appended Python entries) under the receiver-aware key, to dedup
	// Python-only additions.
	byName := map[string]int{}
	seen := map[string]bool{}
	for i := range p.Funcs {
		f := p.Funcs[i]
		if f.C != "" {
			byName[f.Name] = i
		}
		seen[collisionKey(f)] = true
	}

	for _, gf := range pyFuncs {
		if i, ok := byName[gf.Name]; ok && p.Funcs[i].Py == "" {
			// Correlate onto the C function this Python entry wraps.
			p.Funcs[i].Py = gf.Py
			p.Funcs[i].Recv = gf.Recv
			if len(p.Funcs[i].Calls) == 0 {
				p.Funcs[i].Calls = gf.Calls
			}
			seen[collisionKey(p.Funcs[i])] = true
			stats.correlated++
			continue
		}
		if seen[collisionKey(gf)] {
			continue // duplicate Python entry; first wins
		}
		seen[collisionKey(gf)] = true
		if gf.Manual != "" {
			stats.manual++
		} else {
			stats.added++
		}
		p.Funcs = append(p.Funcs, gf)
	}
}

// expandDispatch rewrites a package's dispatch-bearing methods into per-branch
// Go methods (Q-d ratified; Q-f naming). For each method carrying a dispatch
// ladder:
//
//   - If any arm has a blank guard (on: "") or a blank return, the ladder is
//     left carried verbatim as an honest gap: the blank means a Python type
//     with no Go counterpart yet (e.g. XLeech2.Mul's AbstractMMGroupWord ABC,
//     or a nullary arm whose return did not resolve), and expanding it would
//     manufacture a method the generator cannot stand behind.
//   - Otherwise the ladder is expanded. The first (dominant) arm gives the
//     bare method its return type and its dispatch block is dropped. Each
//     later arm becomes a sibling per dispatchSuffix, except a value arm
//     (int==N) whose return equals the dominant arm's return: that is a
//     sub-case of the general arm (e.g. GCode.Div's int==1, identity), so it
//     folds into the bare method body rather than spawning a method.
//
// Siblings inherit the parent's receiver, py provenance, params, and calls
// hint; assertNoCollision (foldPython's final pass) guards the additions.
func expandDispatch(p *goPackage, stats *foldStats) {
	var siblings []goFunc
	for i := range p.Funcs {
		f := &p.Funcs[i]
		if len(f.Dispatch) == 0 {
			continue
		}
		if !dispatchExpandable(f.Dispatch) {
			stats.dispatchCarried++
			continue
		}
		// Dominant arm: keep the bare method, adopt its return, drop the ladder.
		dominant := f.Dispatch[0]
		f.Return = dominant.Returns
		for _, arm := range f.Dispatch[1:] {
			// A value arm whose return matches the dominant arm is a sub-case
			// of the general arm (e.g. divide-by-1 identity); it folds in.
			if isValueGuard(arm.On) && arm.Returns == dominant.Returns {
				continue
			}
			sib := goFunc{
				Name:   f.Name + dispatchSuffix(arm.On),
				Py:     f.Py,
				Recv:   f.Recv,
				Return: arm.Returns,
				Params: f.Params,
				Calls:  f.Calls,
			}
			siblings = append(siblings, sib)
			stats.dispatchSiblings++
		}
		f.Dispatch = nil
		stats.dispatchExpanded++
	}
	p.Funcs = append(p.Funcs, siblings...)
}

// dispatchExpandable reports whether every arm of a dispatch ladder carries a
// non-blank guard and a non-blank return. A single blank in either field keeps
// the whole ladder carried as an honest gap (expandDispatch).
func dispatchExpandable(arms []goDispatch) bool {
	for _, a := range arms {
		if a.On == "" || a.Returns == "" {
			return false
		}
	}
	return true
}

// isValueGuard reports whether a dispatch guard is a value guard ("int==N")
// rather than a type guard ("int", "*Foo").
func isValueGuard(on string) bool {
	return strings.Contains(on, "==")
}

// dispatchSuffix derives the Q-f sibling-name suffix for a dispatch guard:
// a type guard "*Foo" yields "ByFoo", the general integer guard "int" yields
// "ByInt", and a value guard "int==N" yields "ByN". The leading R on a
// reflected operator's bare name is already in place (RMul → RMulByInt); only
// the operand suffix is added here.
func dispatchSuffix(on string) string {
	if i := strings.Index(on, "=="); i >= 0 {
		return "By" + on[i+2:]
	}
	return "By" + upperFirstRune(strings.TrimPrefix(on, "*"))
}

// manualClass is one Python class whose overloaded/variadic __init__ the
// generator could not collapse into a single Go constructor, resolved by
// hand into a constructor family (A7). The data lives here, in code, rather
// than in go.yaml: go.yaml is generated, so a manual edit there would be
// overwritten on the next run. mergeManualConstructors folds each family
// into the plan after foldPython.
//
// The fold emits exactly one constructor per class — the principal form,
// flagged Manual because the remaining overloads were dropped. This
// supplement rewrites that principal into its true (typically zero-arg
// identity) form, clearing the Manual TODO in favor of a Note that lists
// the sibling constructors, and appends those siblings as fully-typed Go
// constructors. A class A7 found needs no constructor at all (a plain data
// container, a classmethod-only type, or a module-level alias) carries an
// empty Siblings list and a Note explaining why; its principal's Manual is
// likewise cleared.
type manualClass struct {
	Pkg       string // target Go package (must already exist in the plan)
	Class     string // Python class name, for diagnostics
	Principal goFunc // the principal constructor in its resolved form
	// (zero-arg identity for a real constructor family; for a class that
	// needs none, the Note explains why and Siblings is empty). Its Name
	// must match the principal entry foldPython already emitted, when one
	// exists; PrincipalNew below governs the absent-principal case.
	PrincipalNew bool // the fold emitted no principal (the class had no
	// resolvable __init__, so foldPython produced no constructor at all);
	// append Principal as a new entry instead of rewriting an existing one.
	Siblings []goFunc // the additional named constructors for this class
}

// manualConstructors holds the per-class constructor families resolved by
// hand from the A7 research. Each entry replaces (or supplies) a class's
// principal constructor and appends its siblings; mergeManualConstructors
// guards every addition with assertNoCollision via foldPython's final pass.
//
// The "see also" Note on each principal is built from its siblings, so the
// list stays in sync with Siblings automatically (manualSeeAlso).
var manualConstructors = []manualClass{
	{
		Pkg:   "mm",
		Class: "MM",
		Principal: goFunc{
			Name:   "NewMM",
			Py:     "MM.__init__",
			Return: "*MM",
		},
		Siblings: []goFunc{
			{Name: "NewMMFromTag", Py: "MM.__init__", Return: "*MM", Note: "tag/value form", Params: []goParam{{Name: "tag", Type: "byte"}, {Name: "value", Type: "int"}}},
			{Name: "NewMMFromAtoms", Py: "MM.__init__", Return: "*MM", Note: "raw atom array", Params: []goParam{{Name: "atoms", Type: "[]uint32"}}},
			{Name: "NewMMRandom", Py: "MM.__init__", Return: "*MM", Note: "random element", Params: []goParam{{Name: "subgroup", Type: "string"}}},
			{Name: "NewMMFromString", Py: "MM.__init__", Return: "*MM", Note: "string parse", Params: []goParam{{Name: "s", Type: "string"}}},
			// TagVal is not yet a defined Go type; carry the slice element as
			// []any until the coordinate-type pass introduces it (A7).
			{Name: "NewMMFromTuples", Py: "MM.__init__", Return: "*MM", Note: "list of tuples; element type TagVal pending, []any placeholder", Params: []goParam{{Name: "tuples", Type: "[]any"}}},
			{Name: "NewMMCopy", Py: "MM.__init__", Return: "*MM", Note: "copy constructor", Params: []goParam{{Name: "g", Type: "*MM"}}},
			{Name: "NewMMFromAutPL", Py: "MM.__init__", Return: "*MM", Params: []goParam{{Name: "a", Type: "*AutPL"}}},
			{Name: "NewMMFromCocode", Py: "MM.__init__", Return: "*MM", Params: []goParam{{Name: "c", Type: "*Cocode"}}},
			{Name: "NewMMFromXLeech2", Py: "MM.__init__", Return: "*MM", Params: []goParam{{Name: "x", Type: "*XLeech2"}}},
			{Name: "NewMMFromXsp2Co1", Py: "MM.__init__", Return: "*MM", Params: []goParam{{Name: "x", Type: "*Xsp2Co1"}}},
		},
	},
	{
		Pkg:   "leech",
		Class: "XLeech2",
		Principal: goFunc{
			Name:   "NewXLeech2",
			Py:     "XLeech2.__init__",
			Return: "*XLeech2",
		},
		Siblings: []goFunc{
			{Name: "NewXLeech2FromInt", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "v", Type: "uint32"}}},
			{Name: "NewXLeech2Random", Py: "XLeech2.__init__", Return: "*XLeech2"},
			{Name: "NewXLeech2RandomType", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "vtype", Type: "int"}}},
			{Name: "NewXLeech2FromShort", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "index", Type: "int"}}},
			{Name: "NewXLeech2FromPLoop", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "d", Type: "*PLoop"}, {Name: "c", Type: "*Cocode"}}},
			{Name: "NewXLeech2Copy", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "x", Type: "*XLeech2"}}},
			{Name: "NewXLeech2FromMM", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "g", Type: "*MM"}}},
			{Name: "NewXLeech2FromBasisVector", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "tag", Type: "byte"}, {Name: "i0", Type: "int"}, {Name: "i1", Type: "int"}}},
			{Name: "NewXLeech2FromName", Py: "XLeech2.__init__", Return: "*XLeech2", Params: []goParam{{Name: "name", Type: "string"}}},
		},
	},
	{
		Pkg:          "xsp2co1",
		Class:        "Xsp2_Co1",
		PrincipalNew: true, // foldPython emitted no Xsp2_Co1.__init__ (no
		// resolvable form survived the walker), so there is no principal entry
		// to rewrite; supply the whole family fresh.
		Principal: goFunc{
			Name:   "NewXsp2Co1",
			Py:     "Xsp2_Co1.__init__",
			Return: "*Xsp2Co1",
		},
		Siblings: []goFunc{
			{Name: "NewXsp2Co1FromTag", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1", Params: []goParam{{Name: "tag", Type: "byte"}, {Name: "value", Type: "int"}}},
			{Name: "NewXsp2Co1FromAtoms", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1", Params: []goParam{{Name: "atoms", Type: "[]uint32"}}},
			{Name: "NewXsp2Co1Random", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1"},
			{Name: "NewXsp2Co1FromString", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1", Params: []goParam{{Name: "s", Type: "string"}}},
			{Name: "NewXsp2Co1Copy", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1", Params: []goParam{{Name: "g", Type: "*Xsp2Co1"}}},
			{Name: "NewXsp2Co1FromMM", Py: "Xsp2_Co1.__init__", Return: "*Xsp2Co1", Params: []goParam{{Name: "g", Type: "*MM"}}},
		},
	},
	{
		Pkg:   "reduce",
		Class: "GtWord",
		Principal: goFunc{
			Name:   "NewGtWord",
			Py:     "GtWord.__init__",
			Return: "*GtWord",
		},
		Siblings: []goFunc{
			{Name: "NewGtWordFromAtoms", Py: "GtWord.__init__", Return: "*GtWord", Params: []goParam{{Name: "atoms", Type: "[]uint32"}}},
			{Name: "NewGtWordFromMM", Py: "GtWord.__init__", Return: "*GtWord", Params: []goParam{{Name: "g", Type: "*MM"}}},
		},
	},
	{
		Pkg:   "mm",
		Class: "BiMM",
		Principal: goFunc{
			Name:   "NewBiMM",
			Py:     "BiMM.__init__",
			Return: "*BiMM",
		},
		Siblings: []goFunc{
			{Name: "NewBiMMRandom", Py: "BiMM.__init__", Return: "*BiMM"},
			{Name: "NewBiMMCopy", Py: "BiMM.__init__", Return: "*BiMM", Params: []goParam{{Name: "g", Type: "*BiMM"}}},
			{Name: "NewBiMMFromPair", Py: "BiMM.__init__", Return: "*BiMM", Params: []goParam{{Name: "m1", Type: "*MM"}, {Name: "m2", Type: "*MM"}, {Name: "alpha", Type: "int"}}},
		},
	},
	// Classes A7 found need no constructor: clear the Manual TODO and record
	// why. No Siblings; the principal entry stays but loses its constructor
	// role (GtSubWord). (Precomputed_AutP3 and standard_mm_group were dropped
	// outright via dropClasses per Q-n, so they carry no entry here.)
	{
		Pkg:   "reduce",
		Class: "GtSubWord",
		Principal: goFunc{
			Name:   "NewGtSubWord",
			Py:     "GtSubWord.__init__",
			Return: "*GtSubWord",
			Note:   "plain struct; fields set by GtWord.Subwords()",
		},
	},
}

// manualSeeAlso builds the principal's "see also" note from its sibling
// names, so the note never drifts from the Siblings list it is derived from.
func manualSeeAlso(siblings []goFunc) string {
	if len(siblings) == 0 {
		return ""
	}
	names := make([]string, len(siblings))
	for i, s := range siblings {
		names[i] = s.Name
	}
	return "constructor family; see also: " + strings.Join(names, ", ")
}

// mergeManualConstructors folds the hand-resolved constructor families
// (manualConstructors) into the plan, after foldPython has produced each
// class's single flagged principal. For each family it locates the package,
// rewrites (or, for an absent principal, appends) the principal into its
// resolved form — clearing the Manual TODO and attaching the see-also Note
// derived from the siblings — then appends the sibling constructors. It does
// not re-sort or re-validate: foldPython's caller pass already does both
// after this runs (the final assertNoCollision there guards the additions).
//
// It errors if a family targets a package the plan does not contain, names a
// principal foldPython did not emit (when PrincipalNew is false), or declares
// PrincipalNew for a class that does have a principal — each is a drift
// between this table and the fold that must fail closed, not silently skip.
func mergeManualConstructors(pkgs []goPackage) error {
	pkgIdx := map[string]int{}
	for i := range pkgs {
		pkgIdx[pkgs[i].Name] = i
	}
	for _, mc := range manualConstructors {
		pi, ok := pkgIdx[mc.Pkg]
		if !ok {
			return fmt.Errorf("manual constructors for class %s: package %q not in plan", mc.Class, mc.Pkg)
		}
		p := &pkgs[pi]

		// Locate the principal foldPython emitted (a free-function constructor,
		// Recv == "") by its Go name.
		principalIdx := -1
		for i := range p.Funcs {
			if p.Funcs[i].Name == mc.Principal.Name && p.Funcs[i].Recv == "" {
				principalIdx = i
				break
			}
		}
		switch {
		case mc.PrincipalNew && principalIdx >= 0:
			return fmt.Errorf("manual constructors for class %s: PrincipalNew set but principal %s already exists in package %s", mc.Class, mc.Principal.Name, mc.Pkg)
		case !mc.PrincipalNew && principalIdx < 0:
			return fmt.Errorf("manual constructors for class %s: principal %s not found in package %s (fold drift)", mc.Class, mc.Principal.Name, mc.Pkg)
		}

		// Build the resolved principal: the data-driven form, with its Manual
		// TODO cleared. A constructor family attaches the see-also Note; a
		// no-constructor class keeps the explanatory Note set in the table.
		principal := mc.Principal
		principal.Manual = ""
		if see := manualSeeAlso(mc.Siblings); see != "" {
			principal.Note = see
		}

		if mc.PrincipalNew {
			p.Funcs = append(p.Funcs, principal)
		} else {
			p.Funcs[principalIdx] = principal
		}
		p.Funcs = append(p.Funcs, mc.Siblings...)
	}
	return nil
}

// typeSupplement is one hand-resolved fill for the residual blank return/param
// type slots a Python-only method carries (D3). The walker leaves a slot blank
// when the Python type is a container it cannot map (a tuple, a list, a numpy
// ndarray, a heterogeneous fixed tuple); this table resolves each from the
// source per the ratified Q-p (container) and Q-q (heterogeneous-tuple →
// Result struct) mapping. Like manualConstructors the data lives in code, not
// in go.yaml (which is generated and would overwrite a manual edit there).
//
// Return, when non-empty, replaces a blank return: slot. Params, when present,
// maps a parameter name to its resolved Go type, replacing that param's blank
// type: slot. ResultStruct, when non-empty, documents the field shape of a Q-q
// Result struct named by Return for the downstream Go-source stage to
// materialize (go.yaml itself only carries the named type as a string). Source
// cites the python.yaml-backing file:line the resolution reads.
type typeSupplement struct {
	Key          string            // "Class.method": the Py provenance to match
	Return       string            // resolved Go return type; "" leaves it blank
	Params       map[string]string // param name → resolved Go type
	ResultStruct string            // field shape of a Q-q Result struct (doc only)
	Source       string            // file:line the resolution reads
}

// typeSupplements holds the per-method type fills for the three highest-traffic
// receivers (MM, MMVector, Axis), classified per the Q-p/Q-q container mapping
// (D3). mergeTypeSupplements applies each over the merged plan and errors if a
// key matches no folded Python entry (a staleness guard against the table
// drifting from python.yaml).
//
// Container mapping applied here:
//   - tuple/list of ints, dynamic           → []int
//   - tuple of ints, fixed size N           → [N]int (N cited from source)
//   - list of tuples                        → []<Recv><Method>Item alias
//   - ndarray                               → flat dtype-mapped slice (row-major)
//   - heterogeneous fixed tuple             → <GoMethod>Result struct (Q-q)
//
// Methods whose return is genuinely shape-polymorphic (__getitem__ yields an
// int or an ndarray by index shape) or an arbitrary class instance (in_space)
// are left as honest gaps: no entry here.
var typeSupplements = []typeSupplement{
	// ---- mm.MM ------------------------------------------------------------
	{
		Key:    "MM.as_Co1_bitmatrix",
		Return: "[]uint8", // 24x24 bit matrix, numpy uint8 entries 0/1, row-major flat
		Source: "structures/mm0_group.py:491",
	},
	{
		Key:    "MM.as_compressed_Co1_bitmatrix",
		Return: "[]uint32", // 1-D numpy uint32 array, length 24; bit j of entry i = (i,j)
		Source: "structures/mm0_group.py:512",
	},
	{
		Key:    "MM.as_M24_permutation",
		Return: "[]int", // list of length 24, a permutation of {0..23}
		Source: "structures/mm0_group.py:562",
	},
	{
		Key:    "MM.as_tuples",
		Return: "[]MMAsTuplesItem", // list of (tag, value) tuples (Q-p list of tuples)
		Source: "structures/abstract_mm_group.py:114 (iter_tuples_from_atoms: construct_mm.py:454)",
	},
	{
		Key:    "MM.chi_G_x0",
		Return: "[4]int", // fixed 4-tuple (chi_M, chi299, chi24, chi4096)
		Params: map[string]string{"involution": "*MM"},
		Source: "structures/mm0_group.py:432",
	},
	{
		Key:          "MM.chi_powers",
		Return:       "ChiPowersResult", // hetero triple (o, chi, h): o int, chi dict, h MM (Q-q)
		ResultStruct: "Order int; Chi map[int]int; H *MM",
		Params:       map[string]string{"maxE": "int", "ntrials": "int", "mp": "int"},
		Source:       "structures/mm0_group.py:693",
	},
	{
		Key:          "MM.conjugate_involution",
		Return:       "ConjugateInvolutionResult", // hetero pair (I, h): I int (0/1/2), h MM (Q-q)
		ResultStruct: "Class int; H *MM",
		Params:       map[string]string{"check": "bool", "ntrials": "int", "verbose": "int"},
		Source:       "structures/mm0_group.py:585",
	},
	{
		Key:          "MM.conjugate_involution_G_x0",
		Return:       "ConjugateInvolutionGX0Result", // hetero pair (iclass, a): iclass str, a MM (Q-q)
		ResultStruct: "Class string; A *MM",
		Params:       map[string]string{"guide": "int", "group": "*MM"},
		Source:       "structures/mm0_group.py:624",
	},
	{
		Key:          "MM.half_order",
		Return:       "HalfOrderResult", // hetero pair (o, h): o int, h *MM or nil (Q-q)
		ResultStruct: "Order int; H *MM",
		Params:       map[string]string{"maxOrder": "int"},
		Source:       "structures/mm0_group.py:397",
	},
	{
		Key:          "MM.half_order_chi",
		Return:       "HalfOrderChiResult", // hetero triple (o, chi, h): o int, chi [4]int or nil, h *MM (Q-q)
		ResultStruct: "Order int; Chi *[4]int; H *MM",
		Params:       map[string]string{"ntrials": "int"},
		Source:       "structures/mm0_group.py:650",
	},
	// ---- mm.MMVector ------------------------------------------------------
	{
		Key:    "MMVector.as_bytes",
		Return: "[]uint8", // 1-D numpy uint8 array, length 196884
		Source: "structures/abstract_mm_rep_space.py:259",
	},
	{
		Key:    "MMVector.as_sparse",
		Return: "[]uint32", // 1-D numpy uint32 array, sparse encoding
		Source: "structures/abstract_mm_rep_space.py:270",
	},
	{
		Key:    "MMVector.as_tuples",
		Return: "[]MMVectorAsTuplesItem", // list of (factor, tag, i0, i1) tuples (Q-p)
		Source: "structures/abstract_mm_rep_space.py:283",
	},
	{
		Key:    "MMVector.axis_type",
		Return: "string", // axis-type string, or "" when not a 2A axis (None)
		Params: map[string]string{"e": "int"},
		Source: "mm_space.py:743",
	},
	{
		Key:    "MMVector.count_short",
		Return: "[]int", // tuple of length (p+1)/2, dynamic (Q-p dynamic tuple of ints)
		Source: "mm_space.py:713",
	},
	{
		Key:    "MMVector.eval_A",
		Return: "int", // C scalar mm_op_eval_A result
		Params: map[string]string{"e": "int"},
		Source: "mm_space.py:668",
	},
	{
		Key:    "MMVector.get_sparse",
		Return: "[]uint32", // numpy uint32 array in sparse representation
		Params: map[string]string{"aSparse": "[]uint32"},
		Source: "structures/abstract_mm_rep_space.py:294",
	},
	{
		Key:    "MMVector.set_sparse",
		Params: map[string]string{"aSparse": "[]uint32"},
		Source: "structures/abstract_mm_rep_space.py:310", // in-place; void return left blank
	},
	{
		Key:    "MMVector.set_zero",
		Params: map[string]string{"p": "int"},
		Source: "structures/abstract_mm_rep_space.py:249", // in-place; void return left blank
	},
	{
		Key:    "MMVector.__rmul__",
		Return: "*MMVector", // scalar right-multiply yields a vector
		Params: map[string]string{"other": "int"},
		Source: "structures/abstract_rep_space.py:126",
	},
	{
		Key:    "MMVector.__rsub__",
		Return: "*MMVector", // right-subtract yields a vector
		Params: map[string]string{"other": "*MMVector"},
		Source: "structures/abstract_rep_space.py:195",
	},
	// ---- axis.Axis --------------------------------------------------------
	{
		Key:          "Axis.axis_type_info",
		Return:       "AxisTypeInfoResult", // hetero triple (o, chi, s): o int, chi int or nil, s str (Q-q)
		ResultStruct: "Order int; Chi *int; Class string",
		Source:       "tests/axes/axis.py:513",
	},
	{
		Key:    "Axis.copy",
		Return: "*Axis", // deep copy of the axis
		Source: "tests/axes/axis.py:454",
	},
	{
		Key:    "Axis.find_short",
		Return: "[]uint32", // numpy array of short Leech-lattice-mod-2 vectors (Leech encoding);
		// radical==1 branch yields a uint64 basis, but the dominant return is uint32
		Params: map[string]string{"value": "int", "radical": "int", "verbose": "int"},
		Source: "tests/axes/axis.py:541 (find_short: tests/axes/axis.py:179)",
	},
	{
		Key:    "Axis.__imul__",
		Return: "*Axis", // in-place multiply returns self
		Params: map[string]string{"g": "*MM"},
		Source: "tests/axes/axis.py:460",
	},
	{
		Key:          "Axis.kernel_A",
		Return:       "KernelAResult", // hetero tuple (ker, isect, M_img, M_ker): two ints, two 24-row mod-3 matrices (Q-q)
		ResultStruct: "Ker int; Isect int; MImg []uint64; MKer []uint64",
		Params:       map[string]string{"d": "int"},
		Source:       "tests/axes/axis.py:648",
	},
	{
		Key:    "Axis.leech3matrix",
		Return: "[]uint64", // 'A' part as 3*24 numpy uint64 in mod-3 matrix encoding
		Source: "tests/axes/axis.py:557",
	},
	{
		Key:    "Axis.__mul__",
		Return: "*Axis", // copy-then-multiply
		Params: map[string]string{"g": "*MM"},
		Source: "tests/axes/axis.py:468",
	},
	{
		Key:          "Axis.profile_Nxyz",
		Return:       "ProfileNxyzResult", // hetero triple (M, h, H): M matrix, h int hash, H sorted matrix (Q-q)
		ResultStruct: "M []uint64; H uint64; HSorted []uint64",
		Source:       "tests/axes/axis.py:684",
	},
	{
		Key:    "Axis.rebase",
		Return: "*Axis", // reduces the stored group element; returns self
		Source: "tests/axes/axis.py:477",
	},
	// ---- clifford12.QState12 (D3 pass 2) ----------------------------------
	{
		Key:    "QState12.__eq__",
		Return: "bool", // qstate12_equal; raises on non-QState12 operand
		Source: "dev/clifford12/clifford12.pyx:732", // other left blank: TypeError on non-QState12
	},
	{
		Key:    "QState12.__mul__",
		Return: "*QState12", // copy().__imul__; scalar-or-state, return is a state
		Source: "dev/clifford12/clifford12.pyx:804", // value polymorphic (Complex or QState12)
	},
	{
		Key:    "QState12.__imul__",
		Return: "*QState12", // in-place scalar/state multiply returns self
		Source: "dev/clifford12/clifford12.pyx:782", // value polymorphic (Complex or QState12)
	},
	{
		Key:    "QState12.__rmul__",
		Return: "*QState12", // __rmul__ = __mul__
		Source: "dev/clifford12/clifford12.pyx:807",
	},
	{
		Key:    "QState12.__truediv__",
		Return: "*QState12", // copy().__itruediv__; scalar divide
		Source: "dev/clifford12/clifford12.pyx:816", // value is a scalar (Complex); polymorphic, left blank
	},
	{
		Key:    "QState12.__itruediv__",
		Return: "*QState12", // in-place scalar divide returns self
		Source: "dev/clifford12/clifford12.pyx:809",
	},
	{
		Key:    "QState12.set_zero",
		Return: "*QState12", // zeroes the state in place, returns self
		Source: "dev/clifford12/clifford12.pyx:412",
	},
	{
		Key:    "QState12.row",
		Return: "int", // int(v): one row of the bit matrix as a packed int
		Source: "dev/clifford12/clifford12.pyx:313",
	},
	{
		Key:    "QState12.monomial_row_op",
		Return: "[]uint32", // np.zeros(nrows, uint32) returned (1-D, dynamic)
		Source: "dev/clifford12/clifford12.pyx:433",
	},
	{
		Key:    "QState12.qstate12_product",
		Return: "*QState12", // returns the product state qs1
		Source: "dev/clifford12/clifford12.pyx:842",
	},
	{
		Key:          "QState12.qstate12_prep_mul",
		Return:       "Qstate12PrepMulResult", // hetero triple (row_pos, qs1, qs2) (Q-q)
		ResultStruct: "RowPos int; Qs1 *QState12; Qs2 *QState12",
		Source:       "dev/clifford12/clifford12.pyx:882",
	},
	{
		Key:    "QState12.check_join_imaginary",
		Return: "*QState12", // result: clone when output else None (nilable state)
		Params: map[string]string{"output": "bool"},
		Source: "dev/clifford12/clifford12.pyx:980",
	},
	// QState12.matrix left blank: 2-D numpy array whose dtype (complex/float/
	// int32) is selected by the `mod` argument; shape-polymorphic return.
	// ---- structures.Cocode (D3 pass 2) ------------------------------------
	{
		Key:    "Cocode.__eq__",
		Return: "bool", // isinstance + value compare
		Source: "structures/cocode.py:167",
	},
	{
		Key:    "Cocode.__mod__",
		Return: "*Parity", // Parity(self.parity) % other
		Source: "structures/cocode.py:235", // other polymorphic, left blank
	},
	{
		Key:    "Cocode.syndrome_list",
		Return: "[]int", // mat24.cocode_to_bit_list: ordered bit positions
		Params: map[string]string{"i": "int"},
		Source: "structures/cocode.py:305",
	},
	{
		Key:    "Cocode.all_syndromes",
		Return: "[]*GcVector", // list of GcVector syndromes
		Source: "structures/cocode.py:293",
	},
	{
		Key:    "Cocode.syndromes_llist",
		Return: "[][]int", // list of lists of bit positions
		Source: "structures/cocode.py:326",
	},
	{
		Key:    "Cocode.syndrome",
		Params: map[string]string{"i": "int"}, // return already *GcVector; fill optional int selector
		Source: "structures/cocode.py:264",
	},
	// Cocode.half_weight left blank: unconditionally raises ValueError.
	// ---- structures.GCode (D3 pass 2) -------------------------------------
	{
		Key:    "GCode.__eq__",
		Return: "bool", // isinstance + value compare
		Source: "structures/gcode.py:318",
	},
	{
		Key:    "GCode.__mod__",
		Return: "*Parity", // Parity(self.parity) % other
		Source: "structures/gcode.py:404", // other polymorphic, left blank
	},
	{
		Key:          "GCode.split",
		Return:       "GCodeSplitResult", // triple (0, eo, v): eo int 0/1, v GCode (Q-q)
		ResultStruct: "Eo0 int; Eo int; V *GCode",
		Source:       "structures/gcode.py:481",
	},
	{
		Key:          "GCode.split_octad",
		Return:       "GCodeSplitOctadResult", // triple (0, eo, o): eo int 0/1, o GCode (Q-q)
		ResultStruct: "Eo0 int; Eo int; O *GCode",
		Source:       "structures/gcode.py:495",
	},
	// ---- structures.GcVector (D3 pass 2) ----------------------------------
	{
		Key:    "GcVector.__eq__",
		Return: "bool", // isinstance + value compare
		Source: "structures/gcode.py:732",
	},
	{
		Key:    "GcVector.__mod__",
		Return: "*Parity", // Parity(self.parity) % other
		Source: "structures/gcode.py:776", // other polymorphic, left blank
	},
	{
		Key:    "GcVector.all_syndromes",
		Return: "[]*GcVector", // list of GcVector syndromes
		Source: "structures/gcode.py:877",
	},
	{
		Key:    "GcVector.syndrome_list",
		Return: "[]int", // ordered bit positions of the syndrome
		Params: map[string]string{"i": "int"},
		Source: "structures/gcode.py:887",
	},
	{
		Key:    "GcVector.syndrome",
		Params: map[string]string{"i": "int"}, // return already *GcVector; fill optional int selector
		Source: "structures/gcode.py:859",
	},
	{
		Key:    "GcVector.vtype",
		Params: map[string]string{"asInt": "bool"}, // return already string; fill flag
		Source: "structures/gcode.py:913",
	},
	// ---- structures.PLoop (D3 pass 2) -------------------------------------
	{
		Key:    "PLoop.__eq__",
		Return: "bool", // isinstance + value compare
		Source: "structures/ploop.py:354",
	},
	{
		Key:    "PLoop.__mod__",
		Return: "*Parity", // inherited from GCode: Parity(self.parity) % other
		Source: "structures/gcode.py:404", // other polymorphic, left blank
	},
	{
		Key:          "PLoop.split",
		Return:       "PLoopSplitResult", // triple (es, eo, v): es/eo int 0/1, v PLoop (Q-q)
		ResultStruct: "Es int; Eo int; V *PLoop",
		Source:       "structures/ploop.py:388",
	},
	{
		Key:          "PLoop.split_octad",
		Return:       "PLoopSplitOctadResult", // triple (es, eo, o): es/eo int 0/1, o PLoop (Q-q)
		ResultStruct: "Es int; Eo int; O *PLoop",
		Source:       "structures/ploop.py:401",
	},
	// ---- structures.Parity (D3 pass 2) ------------------------------------
	{
		Key:    "Parity.__eq__",
		Return: "bool", // isinstance + value compare
		Source: "structures/parity.py:77",
	},
	// ---- structures.AutPL (D3 pass 2) -------------------------------------
	{
		Key:    "AutPL.as_tuples",
		Return: "[]AutPLAsTuplesItem", // [('d', cocode), ('p', perm_num)] (Q-p list of tuples)
		Source: "structures/autpl.py:569",
	},
	// ---- axes.BabyAxis (D3 pass 2) ----------------------------------------
	// BabyAxis subclasses Axis (tests/axes/axis.py:922). The container-shaped
	// methods Copy/FindShort/KernelA/Leech3matrix/ProfileNxyz/AxisTypeInfo are
	// inherited unchanged, so they reuse the same Q-q Result structs resolved
	// for Axis in pass 1.
	{
		Key:    "BabyAxis.copy",
		Return: "*BabyAxis", // deep copy
		Source: "tests/axes/axis.py:454 (Axis.copy)",
	},
	{
		Key:    "BabyAxis.find_short",
		Return: "[]uint32", // numpy array of short Leech-mod-2 vectors
		Params: map[string]string{"value": "int", "radical": "int", "verbose": "int"},
		Source: "tests/axes/axis.py:541 (Axis.find_short)",
	},
	{
		Key:          "BabyAxis.kernel_A",
		Return:       "KernelAResult", // shares Axis.kernel_A shape (Q-q)
		ResultStruct: "Ker int; Isect int; MImg []uint64; MKer []uint64",
		Params:       map[string]string{"d": "int"},
		Source:       "tests/axes/axis.py:648 (Axis.kernel_A)",
	},
	{
		Key:    "BabyAxis.leech3matrix",
		Return: "[]uint64", // 'A' part as 3*24 mod-3 matrix encoding
		Source: "tests/axes/axis.py:557 (Axis.leech3matrix)",
	},
	{
		Key:          "BabyAxis.profile_Nxyz",
		Return:       "ProfileNxyzResult", // shares Axis.profile_Nxyz shape (Q-q)
		ResultStruct: "M []uint64; H uint64; HSorted []uint64",
		Params:       map[string]string{"mode": "int"}, // t is tuple-or-None; left blank
		Source:       "tests/axes/axis.py:684 (Axis.profile_Nxyz)",
	},
	{
		Key:          "BabyAxis.axis_type_info",
		Return:       "AxisTypeInfoResult", // shares Axis.axis_type_info shape (Q-q)
		ResultStruct: "Order int; Chi *int; Class string",
		Source:       "tests/axes/axis.py:513 (Axis.axis_type_info)",
	},
	{
		Key:    "BabyAxis.__imul__",
		Return: "*BabyAxis", // in-place multiply by a Baby group element, returns self
		Params: map[string]string{"g": "*MM"},
		Source: "tests/axes/axis.py:1007",
	},
	{
		Key:    "BabyAxis.__mul__",
		Return: "*BabyAxis", // copy-then-multiply
		Params: map[string]string{"g": "*MM"},
		Source: "tests/axes/axis.py:1016",
	},
	{
		Key:    "BabyAxis.rebase",
		Return: "*BabyAxis", // reduces stored group element; returns self
		Source: "tests/axes/axis.py:1018",
	},
	{
		Key:    "BabyAxis.fixed_value",
		Params: map[string]string{"part": "string"}, // return already int; part is 'A'/'B'
		Source: "tests/axes/axis.py:1027",
	},
	{
		Key:    "BabyAxis.axis_type",
		Params: map[string]string{"e": "int"}, // return already string; fill triality exponent
		Source: "tests/axes/axis.py:1036",
	},
	{
		Key:    "BabyAxis.scalprod15",
		Params: map[string]string{"sparse": "bool"}, // return already int; axis operand polymorphic, left blank
		Source: "tests/axes/axis.py:745 (Axis.scalprod15)",
	},
	{
		Key:    "BabyAxis.product_class",
		Params: map[string]string{"sparse": "bool"}, // return already string; axis operand polymorphic, left blank
		Source: "tests/axes/axis.py:765 (Axis.product_class)",
	},
	{
		Key:    "BabyAxis.display_sym",
		Params: map[string]string{"text": "string", "end": "string"}, // part/mod/diff/ind polymorphic
		Source: "tests/axes/axis.py:590 (Axis.display_sym)",            // void return: prints; left blank
	},
	// BabyAxis.__getitem__ and .in_space left blank, mirroring Axis pass 1:
	// __getitem__ is index-shape polymorphic, in_space yields an arbitrary
	// space instance.
	// ---- mm_crt_space.MMVectorCRT (D3 pass 2) -----------------------------
	// MMVectorCRT subclasses AbstractMmRepVector (mm_crt_space.py:248); its
	// arithmetic dunders inherit the copy-then-imul pattern from
	// structures/abstract_rep_space.py and so return a vector of the same
	// type, mirroring MMVector's pass-1 fold.
	{
		Key:    "MMVectorCRT.__add__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:184", // other (vector) polymorphic, left blank
	},
	{
		Key:    "MMVectorCRT.__radd__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:187",
	},
	{
		Key:    "MMVectorCRT.__iadd__",
		Return: "*MMVectorCRT", // already typed by walker; entry harmless (fill-only)
		Source: "structures/abstract_rep_space.py:177",
	},
	{
		Key:    "MMVectorCRT.__sub__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:192",
	},
	{
		Key:    "MMVectorCRT.__rsub__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:195",
	},
	{
		Key:    "MMVectorCRT.__isub__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:189",
	},
	{
		Key:    "MMVectorCRT.__mul__",
		Return: "*MMVectorCRT", // scalar or group-word multiply; operand polymorphic
		Source: "structures/abstract_rep_space.py:123",
	},
	{
		Key:    "MMVectorCRT.__rmul__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:126",
	},
	{
		Key:    "MMVectorCRT.__imul__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:117", // other already typed int by walker
	},
	{
		Key:    "MMVectorCRT.__truediv__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:150",
	},
	{
		Key:    "MMVectorCRT.__itruediv__",
		Return: "*MMVectorCRT", // in-place scalar divide returns self
		Source: "structures/abstract_rep_space.py:129", // other already typed int by walker
	},
	{
		Key:    "MMVectorCRT.__neg__",
		Return: "*MMVectorCRT",
		Source: "structures/abstract_rep_space.py:171",
	},
	{
		Key:    "MMVectorCRT.__lshift__",
		Return: "*MMVectorCRT", // copy().shl(other)
		Params: map[string]string{"other": "int"},
		Source: "mm_crt_space.py:382",
	},
	{
		Key:    "MMVectorCRT.__ilshift__",
		Return: "*MMVectorCRT", // shl(other), returns self
		Params: map[string]string{"other": "int"},
		Source: "mm_crt_space.py:379",
	},
	{
		Key:    "MMVectorCRT.__rshift__",
		Return: "*MMVectorCRT",
		Params: map[string]string{"other": "int"},
		Source: "structures/abstract_rep_space.py:168",
	},
	{
		Key:    "MMVectorCRT.__irshift__",
		Return: "*MMVectorCRT", // shl(-other), returns self
		Params: map[string]string{"other": "int"},
		Source: "mm_crt_space.py:385",
	},
	{
		Key:    "MMVectorCRT.shl",
		Params: map[string]string{"sh": "int"}, // return already *MMVectorCRT
		Source: "mm_crt_space.py:344",
	},
	{
		Key:    "MMVectorCRT.__eq__",
		Return: "bool", // isinstance + space.equal_vectors
		Source: "structures/abstract_rep_space.py:104",
	},
	{
		Key:    "MMVectorCRT.as_bytes",
		Return: "[]uint8", // dense byte image (mirrors MMVector.as_bytes)
		Source: "structures/abstract_mm_rep_space.py:259",
	},
	{
		Key:    "MMVectorCRT.as_sparse",
		Return: "[]uint32", // sparse uint32 encoding (mirrors MMVector.as_sparse)
		Source: "structures/abstract_mm_rep_space.py:270",
	},
	{
		Key:    "MMVectorCRT.as_tuples",
		Return: "[]MMVectorCRTAsTuplesItem", // list of (factor, tag, i0, i1) tuples (Q-p)
		Source: "structures/abstract_mm_rep_space.py:283",
	},
	{
		Key:    "MMVectorCRT.get_sparse",
		Return: "[]uint32", // sparse uint32 array
		Params: map[string]string{"aSparse": "[]uint32"},
		Source: "structures/abstract_mm_rep_space.py:294",
	},
	{
		Key:    "MMVectorCRT.set_sparse",
		Params: map[string]string{"aSparse": "[]uint32"}, // void in-place; return left blank
		Source: "structures/abstract_mm_rep_space.py:310",
	},
	{
		Key:    "MMVectorCRT.set_zero",
		Params: map[string]string{"p": "int"}, // void in-place; return left blank
		Source: "structures/abstract_mm_rep_space.py:249",
	},
	{
		Key:    "MMVectorCRT.projection",
		Return: "*MMVectorCRT", // projection onto a subspace yields a vector
		Source: "structures/abstract_mm_rep_space.py:328", // args is variadic tuples, left blank
	},
	{
		Key:    "MMVectorCRT.raw_str",
		Return: "string", // raw_str_vector
		Source: "structures/abstract_mm_rep_space.py:347",
	},
	// MMVectorCRT.expand left blank (void, in-place CRT expansion).
	// MMVectorCRT.__getitem__/__setitem__ left blank, mirroring MMVector:
	// index-shape polymorphic.
	// ---- bimm.BiMM (D3 pass 2) --------------------------------------------
	// BiMM subclasses AbstractGroupWord (bimm/bimm.py:54); copy/__invert__/
	// __mul__/__truediv__/__pow__/as_tuples/str mirror MM's pass-1 fold.
	{
		Key:    "BiMM.copy",
		Return: "*BiMM",
		Source: "structures/abstract_group.py:75",
	},
	{
		Key:    "BiMM.__invert__",
		Return: "*BiMM",
		Source: "structures/abstract_group.py:126",
	},
	{
		Key:    "BiMM.__mul__",
		Return: "*BiMM", // operand polymorphic, left blank
		Source: "structures/abstract_group.py:80",
	},
	{
		Key:    "BiMM.__truediv__",
		Return: "*BiMM", // operand polymorphic, left blank
		Source: "structures/abstract_group.py:101",
	},
	{
		Key:    "BiMM.__rtruediv__",
		Return: "*BiMM",
		Source: "structures/abstract_group.py:101", // operand polymorphic, left blank
	},
	{
		Key:    "BiMM.__rmul__",
		Return: "*BiMM", // other already typed *Parity by walker
		Source: "structures/abstract_group.py:80",
	},
	{
		Key:    "BiMM.__pow__",
		Return: "*BiMM", // exp already typed int
		Source: "structures/abstract_group.py:130",
	},
	{
		Key:    "BiMM.__eq__",
		Return: "bool",
		Source: "structures/abstract_group.py:62", // other polymorphic, left blank
	},
	{
		Key:    "BiMM.__ne__",
		Return: "bool", // not __eq__
		Source: "structures/abstract_group.py:72", // other polymorphic, left blank
	},
	{
		Key:    "BiMM.as_tuples",
		Return: "[]BiMMAsTuplesItem", // list of (tag, value) tuples (Q-p)
		Source: "structures/abstract_group.py:189",
	},
	{
		Key:    "BiMM.str",
		Return: "string", // group.str_word
		Source: "structures/abstract_group.py:181",
	},
	{
		Key:    "BiMM.__str__",
		Return: "string", // __repr__ = str
		Source: "structures/abstract_group.py:181",
	},
	{
		Key:    "BiMM.__repr__",
		Return: "string", // __repr__ = str
		Source: "structures/abstract_group.py:181",
	},
	{
		Key:    "BiMM.__hash__",
		Return: "int", // hash of (hash(m1), hash(m2), alpha)
		Source: "bimm/bimm.py:182",
	},
	{
		Key:    "BiMM.order",
		Return: "int", // s * o1 * o2 // gcd(o1, o2)
		Source: "bimm/bimm.py:157",
	},
	{
		Key:    "BiMM.orders",
		Return: "[3]int", // (m1.order, m2.order, par): fixed 3-tuple of ints (Q-p)
		Source: "bimm/bimm.py:148",
	},
	{
		Key:          "BiMM.decompose",
		Return:       "BiMMDecomposeResult", // triple (m1, m2, e): MM, MM, int 0/1 (Q-q)
		ResultStruct: "M1 *MM; M2 *MM; E int",
		Source:       "bimm/bimm.py:169",
	},
	// BiMM.reduce left blank: in-place reduction, returns None.
	// ---- bimm.P3_node (D3 pass 2) -----------------------------------------
	{
		Key:    "P3_node.__eq__",
		Return: "bool", // isinstance + _ord compare
		Source: "bimm/inc_p3.py:140", // other polymorphic, left blank
	},
	{
		Key:    "P3_node.__ne__",
		Return: "bool", // not __eq__
		Source: "bimm/inc_p3.py:142", // other polymorphic, left blank
	},
	{
		Key:    "P3_node.__str__",
		Return: "string", // "P3<point|line N>"
		Source: "bimm/inc_p3.py:137",
	},
	{
		Key:    "P3_node.name",
		Return: "string", // "PL"[q] + str(r)
		Source: "bimm/inc_p3.py:160",
	},
	{
		Key:    "P3_node.y_name",
		Return: "string", // Y_555 notation name
		Source: "bimm/inc_p3.py:164",
	},
	// P3_node.__mul__ left blank: returns P3_node or P3_incidence by node
	// kind (shape-polymorphic). __repr__ has no class-level definition.
	// ---- mm_reduce.GtWord (D3 pass 2) -------------------------------------
	{
		Key:    "GtWord.as_int_debug_compress",
		Return: "[]uint64", // a[:j], compressed mm_compress entries
		Source: "dev/mm_reduce/mm_reduce.pyx:211",
	},
	{
		Key:          "GtWord.append_sub_part",
		Return:       "GtWordAppendSubPartResult", // (n, tail): int, uint32 word (Q-q)
		ResultStruct: "N int; Tail []uint32",
		Params:       map[string]string{"a": "[]uint32"},
		Source:       "dev/mm_reduce/mm_reduce.pyx:147",
	},
	{
		Key:    "GtWord.append",
		Params: map[string]string{"a": "[]uint32"}, // void; return left blank
		Source: "dev/mm_reduce/mm_reduce.pyx:157",
	},
	{
		Key:    "GtWord.reduce_sub",
		Params: map[string]string{"mode": "int"}, // void; return left blank
		Source: "dev/mm_reduce/mm_reduce.pyx:164",
	},
	{
		Key:    "GtWord.seek",
		Params: map[string]string{"pos": "int", "seekSet": "int"}, // void; return left blank
		Source: "dev/mm_reduce/mm_reduce.pyx:109",
	},
	{
		Key:    "GtWord.set_reduce_mode",
		Params: map[string]string{"reduceMode": "int"}, // void; return left blank
		Source: "dev/mm_reduce/mm_reduce.pyx:106",
	},
	{
		Key:    "GtWord.display_subwords",
		Params: map[string]string{"text": "string"}, // void: prints; return left blank
		Source: "dev/mm_reduce/mm_reduce.pyx:260",
	},
	{
		Key:    "GtWord.__str__",
		Return: "string", // group_name + atom strings
		Source: "dev/mm_reduce/mm_reduce.pyx:235",
	},
	{
		Key:    "GtWord.__eq__",
		Return: "bool", // object-default identity comparison
		Source: "dev/mm_reduce/mm_reduce.pyx:62", // value polymorphic, left blank
	},
	{
		Key:    "GtWord.__ne__",
		Return: "bool", // object-default identity comparison
		Source: "dev/mm_reduce/mm_reduce.pyx:62", // value polymorphic, left blank
	},
	// GtWord.reduce left blank (void). GtWord.mmdata left blank: returns a
	// uint32 array or a group element depending on the `group` argument
	// (shape-polymorphic). GtWord.subwords left blank: returns (fpos, list of
	// internal GtSubWord objects), whose element type is not an API receiver.
	// ---- structures.Xsp2_Co1_Group (D3 pass 2) ---------------------------
	{
		Key:    "Xsp2_Co1_Group.copy_word",
		Params: map[string]string{"g1": "*Xsp2Co1"}, // return already *Xsp2Co1
		Source: "structures/xsp2_co1.py:622",
	},
	{
		Key:    "Xsp2_Co1_Group.reduce",
		Return: "*Xsp2Co1Group", // returns self
		Params: map[string]string{"g1": "*Xsp2Co1"},
		Source: "structures/xsp2_co1.py:627",
	},
	{
		Key:    "Xsp2_Co1_Group.from_qs",
		Return: "*Xsp2Co1", // builds an element from a quadratic state
		Params: map[string]string{"qs": "*QState12"}, // x is an index; left blank
		Source: "structures/xsp2_co1.py:646",
	},
	{
		Key:    "Xsp2_Co1_Group.from_xsp",
		Params: map[string]string{"x": "int"}, // return already *Xsp2Co1; x is xspecial number
		Source: "structures/xsp2_co1.py:660",
	},
	{
		Key:    "Xsp2_Co1_Group.from_data",
		Params: map[string]string{"data": "[]uint32"}, // return already *Xsp2Co1
		Source: "structures/xsp2_co1.py:665",
	},
	{
		Key:    "Xsp2_Co1_Group.str_word",
		Params: map[string]string{"v1": "*Xsp2Co1"}, // return already string
		Source: "structures/xsp2_co1.py:653",
	},
	{
		Key:    "Xsp2_Co1_Group.raw_str_word",
		Params: map[string]string{"g": "*Xsp2Co1"}, // return already string
		Source: "structures/abstract_mm_group.py:229",
	},
	{
		Key:    "Xsp2_Co1_Group.as_tuples",
		Params: map[string]string{"g": "*Xsp2Co1"}, // abstract as_tuples raises; return left blank
		Source: "structures/abstract_group.py:299",
	},
	{
		Key:    "Xsp2_Co1_Group.__eq__",
		Return: "bool", // singleton group equality
		Source: "structures/abstract_mm_group.py:277", // value polymorphic, left blank
	},
	{
		Key:    "Xsp2_Co1_Group.atom",
		Source: "structures/xsp2_co1.py:609", // return already *Xsp2Co1; tag/i polymorphic, left blank
	},
	// ---- general.Orbit_Lin2 (D3 pass 2) -----------------------------------
	{
		Key:    "Orbit_Lin2.n_orbits",
		Return: "int", // gen_ufind_lin2_n_orbits
		Source: "general/orbit_lin2.py:247",
	},
	{
		Key:    "Orbit_Lin2.orbit_rep",
		Return: "int", // gen_ufind_lin2_rep_v
		Params: map[string]string{"v": "int"},
		Source: "general/orbit_lin2.py:273",
	},
	{
		Key:    "Orbit_Lin2.orbit_size",
		Return: "int", // gen_ufind_lin2_len_orbit_v
		Params: map[string]string{"v": "int"},
		Source: "general/orbit_lin2.py:283",
	},
	{
		Key:    "Orbit_Lin2.orbit",
		Return: "[]uint32", // uint32 array of the orbit
		Params: map[string]string{"v": "int"},
		Source: "general/orbit_lin2.py:286",
	},
	{
		Key:    "Orbit_Lin2.mul_v_g",
		Return: "int", // gen_ufind_lin2_mul_affine
		Params: map[string]string{"v": "int"}, // g is a group element, left blank
		Source: "general/orbit_lin2.py:332",
	},
	{
		Key:          "Orbit_Lin2.representatives",
		Return:       "Orbit_Lin2RepresentativesResult", // (reps, sizes): two uint32 arrays (Q-q)
		ResultStruct: "Reps []uint32; Sizes []uint32",
		Source:       "general/orbit_lin2.py:257",
	},
	{
		Key:    "Orbit_Lin2.stabilizer",
		Return: "*OrbitLin2", // a stabilizer subgroup
		Params: map[string]string{"v": "int", "nGen": "int", "compress": "bool"}, // map_ polymorphic, left blank
		Source: "general/orbit_lin2.py:385",
	},
	{
		Key:          "Orbit_Lin2.stabilizer_chain",
		Return:       "[]OrbitLin2StabilizerChainItem", // list of (H, v) pairs (Q-p list of tuples)
		ResultStruct: "H *OrbitLin2; V int",
		Params:       map[string]string{"vList": "[]int", "maxDescent": "bool", "nGen": "int", "compress": "bool"},
		Source:       "general/orbit_lin2.py:414",
	},
	{
		Key:    "Orbit_Lin2.compress",
		Return: "*OrbitLin2", // compressed copy
		Params: map[string]string{"orbits": "[]int"},
		Source: "general/orbit_lin2.py:533",
	},
	{
		Key:    "Orbit_Lin2.map_v_G",
		Params: map[string]string{"v": "int", "img": "int"}, // returns arbitrary group element, left blank
		Source: "general/orbit_lin2.py:295",
	},
	{
		Key:    "Orbit_Lin2.map_v_G_transform",
		Params: map[string]string{"v": "int", "img": "int"}, // obj/return arbitrary, left blank
		Source: "general/orbit_lin2.py:313",
	},
	{
		Key:    "Orbit_Lin2.rand_stabilizer",
		Params: map[string]string{"v": "int"}, // returns arbitrary group element, left blank
		Source: "general/orbit_lin2.py:375",
	},
	{
		Key:    "Orbit_Lin2.order_kernel",
		Params: map[string]string{"nGen": "int"}, // returns (o int, K arbitrary gens), left blank
		Source: "general/orbit_lin2.py:462",
	},
	{
		Key:    "Orbit_Lin2.__eq__",
		Return: "bool", // object-default identity comparison
		Source: "general/orbit_lin2.py:96", // value polymorphic, left blank
	},
	{
		Key:    "Orbit_Lin2.__ne__",
		Return: "bool", // object-default identity comparison
		Source: "general/orbit_lin2.py:96",
	},
	// Orbit_Lin2.generators/rand return arbitrary group-element objects (the
	// element type depends on the caller-supplied generator set), left blank.
	// Orbit_Lin2.map_v_word_G returns a list of (generator, sign) tuples whose
	// generator element type is arbitrary; pickle returns nested tuples of
	// data and Python callables; both left blank.
	// ---- general.Orbit_Elem2 (D3 pass 2) ----------------------------------
	{
		Key:    "Orbit_Elem2.structure_2",
		Return: "[]int", // list(reversed(exponents))
		Params: map[string]string{"fields": "[][2]int"}, // list of (m,n) int pairs
		Source: "general/orbit_lin2.py:671",
	},
	{
		Key:    "Orbit_Elem2.map_v_G",
		Params: map[string]string{"v": "int", "img": "int"}, // returns arbitrary group element, left blank
		Source: "general/orbit_lin2.py:617",
	},
	{
		Key:    "Orbit_Elem2.__eq__",
		Return: "bool", // object-default identity comparison
		Source: "general/orbit_lin2.py:546", // value polymorphic, left blank
	},
	{
		Key:    "Orbit_Elem2.__ne__",
		Return: "bool", // object-default identity comparison
		Source: "general/orbit_lin2.py:546",
	},
	// Orbit_Elem2.generators/rand/rand_kernel return arbitrary group-element
	// objects, left blank. Orbit_Elem2.map_v_word_G returns a list of
	// (generator, sign) tuples whose generator element type is arbitrary,
	// left blank.
}

// mergeTypeSupplements applies the hand-resolved typeSupplements over the
// merged plan (D3). For each supplement it locates the unique folded entry
// whose Py provenance equals the key and fills its blank return: slot (when
// Return is set) and any named param's blank type: slot. It counts each filled
// slot in stats.supplemented.
//
// It errors if a supplement key matches no folded entry — the staleness guard
// that fails closed when this table drifts from python.yaml (a method renamed
// or dropped upstream). It does not error when a slot is already non-blank:
// the supplement only ever fills, never overrides, so a slot the fold already
// typed (or a later regeneration after the type lands in go.yaml) is left
// untouched and silently skipped.
func mergeTypeSupplements(pkgs []goPackage, stats *foldStats) error {
	// Index every folded entry by its Py provenance for the staleness check
	// and the fill. A Py value is unique across the whole plan (Class.method
	// names a single method), so a flat index across packages is sufficient.
	type loc struct{ pkg, fn int }
	byPy := map[string]loc{}
	for pi := range pkgs {
		for fi := range pkgs[pi].Funcs {
			if py := pkgs[pi].Funcs[fi].Py; py != "" {
				byPy[py] = loc{pi, fi}
			}
		}
	}
	for _, sup := range typeSupplements {
		l, ok := byPy[sup.Key]
		if !ok {
			return fmt.Errorf("type supplement for %q matches no folded Python entry (stale typeSupplements vs python.yaml)", sup.Key)
		}
		f := &pkgs[l.pkg].Funcs[l.fn]
		if sup.Return != "" && f.Return == "" {
			f.Return = sup.Return
			stats.supplemented++
		}
		for name, typ := range sup.Params {
			for i := range f.Params {
				if f.Params[i].Name == name && f.Params[i].Type == "" {
					f.Params[i].Type = typ
					stats.supplemented++
				}
			}
		}
	}
	return nil
}

// pyDropModule names the one walked module whose functions are pytest
// scaffolding rather than API; genGo drops it when folding rather than
// editing the walker (which would change python.yaml). mmgroup.conftest
// exports only pytest_configure.
const pyDropModule = "mmgroup.conftest"

// dropClasses names the python.yaml classes that must be kept out of
// go.yaml because they are not part of the Go API surface. The value is the
// reason, surfaced in diagnostics. They fall into four groups (A5 triage):
//
//   - Abstract base classes: their methods flatten onto the concrete
//     receivers (the leaf types) via the walker's inherited-method
//     flattening (Q21) and Change 6's leaf collapse, so the ABC entry
//     itself carries no unique API. (A formal Go interface for these is a
//     later concern, Q-a.)
//   - Group-factory ABCs (BiMMGroup, AutP3Group): factories with no unique
//     API beyond constructing the concrete group.
//   - Parser/auxiliary internals (AtomDict, EvalNodeVisitor, TaggedAtom,
//     vsparse): private implementation helpers, not surface.
//   - Non-constructor utilities (Precomputed_AutP3, standard_mm_group):
//     a classmethod-only utility and a module-level MM0 alias, neither a
//     constructible Go type (Q-n).
//
// assertDropTablesFresh verifies every key still names a live class so a key
// cannot rot silently after upstream removes the class.
var dropClasses = map[string]string{
	// Abstract base classes (Q-a: prune now, interface later)
	"AbstractGroup":       "ABC; methods flatten onto concrete receivers",
	"AbstractGroupWord":   "ABC; methods flatten onto concrete receivers",
	"AbstractMMGroup":     "ABC; methods flatten onto concrete receivers",
	"AbstractMMGroupWord": "ABC; methods flatten onto concrete receivers",
	"AbstractMmRepSpace":  "ABC; methods flatten onto concrete receivers",
	"AbstractMmRepVector": "ABC; methods flatten onto concrete receivers",
	"AbstractRepSpace":    "ABC; methods flatten onto concrete receivers",
	"AbstractRepVector":   "ABC; methods flatten onto concrete receivers",
	// Group-factory ABCs (A5 triage: prune)
	"BiMMGroup":  "group factory for BiMM; no unique API",
	"AutP3Group": "group factory for AutP3; no unique API",
	// Parser internals (A5 triage: prune)
	"AtomDict":        "internal atom-expression parser dict",
	"EvalNodeVisitor": "internal AST walker for atom parser",
	"TaggedAtom":      "internal parsed-atom token",
	// Internal sparse-vector helper (A5 triage: prune)
	"vsparse": "internal CRT sparse vector auxiliary",
	// Non-constructor utilities ratified for drop (Q-n)
	"Precomputed_AutP3": "classmethod-only utility; no constructor (Q-n)",
	"standard_mm_group": "module-level alias for MM0; not a type (Q-n)",
}

// dropModuleFns names the module-level Python functions that must be kept
// out of go.yaml because they are not API. The value is the reason. Two
// groups: the lazy-import bootstrap shims that mmgroup's deferred-import
// machinery calls to populate a module's globals on first use (they wrap no
// C symbol and have no Go counterpart, surfacing from several modules via
// the module-level function pass), and the gen_rng_* Cython convenience
// shims plus the Orbit_Lin2 chk error helper (A5 triage). assertDropTablesFresh
// verifies every key still names a live module function.
//
// Note: make_table/invert_table/split_table are NOT listed here — pyToCAlias
// (Change 1) correlates them onto the gen_xi_* C entries, so they never
// surface as Python-only entries that would need dropping.
var dropModuleFns = map[string]string{
	// Lazy-import bootstrap shims (formerly bootstrapShimFns).
	"complete_import":           "lazy-import bootstrap shim",
	"import_mm_order_functions": "lazy-import bootstrap shim",
	"complete_import_mm_reduce": "lazy-import bootstrap shim",
	// Orbit_Lin2 helper and gen_rng_* Cython shims (A5 triage).
	"chk":                     "error-code helper for Orbit_Lin2; not API",
	"rand_bytes_modp":         "Cython shim over gen_rng_bytes_modp",
	"rand_fill_bytes_modp":    "Cython shim over gen_rng_bytes_modp (in-place)",
	"rand_gen_bitfields_modp": "Cython shim over gen_rng_bitfields_modp",
	"rand_gen_modp":           "Cython shim over gen_rng_modp",
	"rand_make_seed":          "Cython shim over gen_rng_seed_no",
}

// dropModuleFn reports whether a module-level Python function must be kept out
// of go.yaml. It drops the dropModuleFns family unconditionally, and drops
// bw24 only when it comes from mmgroup.bitfunctions: that module's bw24 is a
// trivial pure-Python duplicate of the C-backed mat24_bw24, whereas the
// mmgroup.mat24 bw24 is the Cython builtin that correlates onto (and supplies
// the py: provenance of) the mat24_bw24 entry, so it must survive. The
// sibling mmgroup.bitfunctions helpers (bitweight, bitparity, ...) are wanted,
// so only bw24 is named here rather than dropping the whole module.
func dropModuleFn(fn pyModuleFn) bool {
	if _, ok := dropModuleFns[fn.Name]; ok {
		return true
	}
	return fn.Name == "bw24" && fn.Module == "mmgroup.bitfunctions"
}

// assertDropTablesFresh verifies every key in dropClasses and dropModuleFns
// still names a live class / module function in the parsed python.yaml. A
// stale key (upstream removed the class or function, so the drop is now a
// no-op silently masking a real change) is a hard error: the drop tables
// must track upstream exactly, never carry dead entries.
func assertDropTablesFresh(py pythonManifest) error {
	classNames := make(map[string]bool, len(py.Classes))
	for _, c := range py.Classes {
		classNames[c.Name] = true
	}
	for name := range dropClasses {
		if !classNames[name] {
			return fmt.Errorf("dropClasses key %q names no class in python.yaml (stale drop table; upstream removed it?)", name)
		}
	}
	fnNames := make(map[string]bool, len(py.ModuleFns))
	for _, f := range py.ModuleFns {
		fnNames[f.Name] = true
	}
	for name := range dropModuleFns {
		if !fnNames[name] {
			return fmt.Errorf("dropModuleFns key %q names no module function in python.yaml (stale drop table; upstream removed it?)", name)
		}
	}
	return nil
}

// goFuncLess orders entries within a package for deterministic output:
// C-derived entries first (by C symbol, preserving the existing go.yaml
// order), then Python-only entries (by receiver then name).
func goFuncLess(a, b goFunc) bool {
	if (a.C != "") != (b.C != "") {
		return a.C != "" // C-derived sorts before Python-only
	}
	if a.C != "" {
		return a.C < b.C
	}
	if a.Recv != b.Recv {
		return a.Recv < b.Recv
	}
	return a.Name < b.Name
}

// pyToCAlias maps a Cython wrapper's Python-visible name to the C symbol
// it wraps when the two differ by a dropped prefix. The gen_xi_* table
// builders are exposed under their unprefixed names (make_table,
// invert_table, split_table) by mmgroup.dev.generators, so packageOf on
// the bare Python name would route them to a fresh Python-only entry
// (MakeTable, ...) instead of correlating onto the C entry (XiMakeTable,
// ...). Consulting this alias in goSiteForModuleFn makes mergePython fold
// them onto the C function, granting that entry py: provenance and
// removing the spurious Python-only duplicate (A2).
var pyToCAlias = map[string]string{
	"make_table":   "gen_xi_make_table",
	"invert_table": "gen_xi_invert_table",
	"split_table":  "gen_xi_split_table",
}

// goSiteForModuleFn determines the target package and Go name for a
// module-level Python function. A name that is itself a C symbol (it
// matches a pkgRule prefix) is routed exactly as the C side routes it, so
// it lands on — and correlates with — the same go.yaml entry. Otherwise
// it is a pure-Python helper routed by its defining module.
func goSiteForModuleFn(fn pyModuleFn) (pkg, name string) {
	// A Cython wrapper whose Python name drops a C prefix (the gen_xi_*
	// table builders) is routed by its aliased C symbol so it correlates
	// onto the C entry rather than producing a Python-only duplicate (A2).
	if cSym, ok := pyToCAlias[fn.Name]; ok {
		if p, rem, ok := packageOf(cSym); ok {
			n := goName(rem)
			if ov, has := renameOverrides[cSym]; has {
				n = ov
			}
			return p, n
		}
	}
	if p, rem, ok := packageOf(fn.Name); ok {
		n := goName(rem)
		if ov, has := renameOverrides[fn.Name]; has {
			n = ov
		}
		return p, n
	}
	if p, ok := packageOfModule(fn.Module); ok {
		return p, goName(fn.Name)
	}
	return "", ""
}

// goFuncForMethod maps one python.yaml method entry to its go.yaml form
// per PYMETH_TO_GOFUNC. ok is false for a dunder that has no Go surface.
//
// The method's canonical Python return type and its argument-polymorphic
// dispatch ladder are translated to Go (pyTypeToGo) and attached to the
// resulting goFunc, except for __init__: a constructor's return is always
// the receiver type *Recv regardless of how the Python __init__ reads (it
// returns None), so it is set directly and any Python dispatch on
// __init__ is dropped.
func goFuncForMethod(class, recv string, m pyMethod) (goFunc, bool) {
	calls := cMatches(m.Calls)
	ret := pyTypeToGo(m.Return)
	dispatch := goDispatchOf(m.Dispatch)

	// Constructor: __init__ → NewType (free function). A *args/**kwargs or
	// otherwise variadic form is emitted but flagged for manual follow-up
	// (Q19): Go has no overloading, so the principal form is NewType and
	// the variadic forms need hand-authored sibling constructors. A
	// constructor returns the receiver type, not None.
	if m.Name == "__init__" {
		gf := goFunc{
			Name:   "New" + recv,
			Py:     class + ".__init__",
			Return: "*" + recv,
			Params: paramsFromPython(dropSelf(m.Params)),
			Calls:  calls,
		}
		if hasVariadic(m.Params) {
			gf.Manual = "overloaded/variadic __init__; principal form emitted as New" + recv + ", split remaining forms by hand"
		}
		return gf, true
	}

	// Other whitelisted operator dunders → named receiver methods.
	if strings.HasPrefix(m.Name, "__") {
		op, ok := dunderGoName[m.Name]
		if !ok {
			return goFunc{}, false
		}
		return goFunc{
			Name:     op,
			Recv:     recv,
			Py:       class + "." + m.Name,
			Return:   ret,
			Params:   paramsFromPython(dropSelf(m.Params)),
			Calls:    calls,
			Dispatch: dispatch,
		}, true
	}

	name := goName(m.Name)
	switch m.Kind {
	case "staticmethod", "classmethod":
		// No receiver state → package-level free function (PYMETH §3/§4).
		return goFunc{
			Name:     name,
			Py:       class + "." + m.Name,
			Return:   ret,
			Params:   paramsFromPython(m.Params), // no self on a static/classmethod
			Calls:    calls,
			Dispatch: dispatch,
		}, true
	default: // "method"
		return goFunc{
			Name:     name,
			Recv:     recv,
			Py:       class + "." + m.Name,
			Return:   ret,
			Params:   paramsFromPython(dropSelf(m.Params)),
			Calls:    calls,
			Dispatch: dispatch,
		}, true
	}
}

// goDispatchOf translates a method's Python dispatch ladder to the Go
// plan's form: each arm's guard through pyGuardToGo and each arm's return
// through pyTypeToGo. nil maps to nil so a monomorphic method carries no
// dispatch.
func goDispatchOf(arms []pyDispatch) []goDispatch {
	if len(arms) == 0 {
		return nil
	}
	out := make([]goDispatch, 0, len(arms))
	for _, a := range arms {
		out = append(out, goDispatch{
			On:      pyGuardToGo(a.On),
			Returns: pyTypeToGo(a.Returns),
		})
	}
	return out
}

// dunderGoName maps the whitelisted operator dunders to an exported Go
// method name. Reflected operators (__rmul__, __radd__, ...) get a
// distinct R-prefixed name because mmgroup's multiplication/addition is
// non-commutative, so both argument orderings are real, separate
// operations (resolved decision). __init__ is handled separately (it is a
// constructor, not a method) and so is deliberately absent here.
var dunderGoName = map[string]string{
	"__mul__":      "Mul",
	"__rmul__":     "RMul",
	"__add__":      "Add",
	"__radd__":     "RAdd",
	"__sub__":      "Sub",
	"__rsub__":     "RSub",
	"__truediv__":  "Div",
	"__rtruediv__": "RDiv",
	"__pow__":      "Pow",
	"__mod__":      "Mod",
	"__and__":      "And",
	"__rand__":     "RAnd",
	"__lshift__":   "Lshift",
	"__rshift__":   "Rshift",
	"__neg__":      "Neg",
	"__pos__":      "Pos",
	"__abs__":      "Abs",
	"__invert__":   "Invert",
	"__eq__":       "Equal",
	"__ne__":       "NotEqual",
	"__len__":      "Len",
	"__getitem__":  "GetItem",
	"__setitem__":  "SetItem",
	"__bool__":     "Bool",
	"__repr__":     "String",
	"__hash__":     "Hash",
	"__str__":      "GoString", // not "String": __repr__ already maps to String
	"__contains__": "Contains",
	"__iadd__":     "IAdd",
	"__isub__":     "ISub",
	"__imul__":     "IMul",
	"__itruediv__": "IDiv",
	"__ilshift__":  "ILshift",
	"__irshift__":  "IRshift",
}

// dropSelf removes a leading self/cls parameter (an instance or class
// method's receiver), which has no Go counterpart.
func dropSelf(params []pyYAMLParam) []pyYAMLParam {
	if len(params) > 0 && (params[0].Name == "self" || params[0].Name == "cls") {
		return params[1:]
	}
	return params
}

// hasVariadic reports whether any parameter is *args or **kwargs, the
// mark of an overloaded Python signature with no single Go form (Q19).
func hasVariadic(params []pyYAMLParam) bool {
	for _, p := range params {
		if p.Kind == "VAR_POSITIONAL" || p.Kind == "VAR_KEYWORD" {
			return true
		}
	}
	return false
}

// paramsFromPython converts python.yaml parameters to Go parameters,
// translating each param's canonical Python type (the type: the walker
// probed from an isinstance() test or default literal) through pyTypeToGo.
// A param whose Python type is unknown or unprobed translates to "" — the
// honest gap, not a guess. The parameter name is lowered to a Go-safe
// identifier; a *args/**kwargs parameter is surfaced verbatim so the
// signature still records that an overload exists.
//
// The Python default-value literal (p.Default) is carried through onto the
// goParam as an additional type hint for whatever residual blanks remain —
// the literal's type is the parameter's type (PLAN.md §PYGEN Bug 2). It is
// retained even when the type now resolves, so the provenance of a resolved
// type stays visible.
func paramsFromPython(params []pyYAMLParam) []goParam {
	if len(params) == 0 {
		return nil
	}
	out := make([]goParam, 0, len(params))
	for _, p := range params {
		out = append(out, goParam{
			Name:    goParamName(p.Name),
			Type:    pyTypeToGo(p.Type),
			Default: p.Default,
		})
	}
	return out
}

// richerTyping reports whether a wins over b for the H11 inherited-method
// collapse: the copy with more non-blank types (across return and params) is
// the better-resolved one and should survive a same-key collision. A
// C-derived parent method (typed params, typed return) thus beats a
// blank-typed inherited copy of the same operation. Ties keep the incumbent
// (a is not strictly richer), preserving first-seen order.
func richerTyping(a, b goFunc) bool {
	return nonBlankTypes(a) > nonBlankTypes(b)
}

func nonBlankTypes(f goFunc) int {
	n := 0
	if f.Return != "" {
		n++
	}
	for _, p := range f.Params {
		if p.Type != "" {
			n++
		}
	}
	return n
}

// cMatches filters a method/function body's call list down to the names
// that are C symbols (they appear as a pkgRule-routable prefix). This is
// the non-authoritative cross-reference hint of PYGEN Q17: surfaced for
// whoever fills in the deferred types, never used as a type source.
func cMatches(calls []string) []string {
	if len(calls) == 0 {
		return nil
	}
	var out []string
	for _, c := range calls {
		// A dotted call (obj.method) is a Python attribute access, not a
		// bare C symbol; only bare names can be C entry points.
		if strings.Contains(c, ".") {
			continue
		}
		if _, _, ok := packageOf(c); ok {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func emitGoYAML(pkgs []goPackage, enums []goEnum) string {
	var b strings.Builder
	b.WriteString("# Auto-generated from cython.yaml + python.yaml. Do not edit.\n")
	b.WriteString("#\n")
	b.WriteString("# The unified plan for the Go side of the mmgroup translation.\n")
	b.WriteString("# Each operation is grouped under its target cgt package with an\n")
	b.WriteString("# exported Go name and Go parameter/return types. Provenance:\n")
	b.WriteString("#   'c'  — the originating C symbol (cross-ref cython.yaml).\n")
	b.WriteString("#   'py' — the Python entry point (cross-ref python.yaml):\n")
	b.WriteString("#          dotted 'Class.method' for a class method/constructor,\n")
	b.WriteString("#          dotless 'module_func' for a module-level function.\n")
	b.WriteString("# An entry may carry both when a Python entry point was\n")
	b.WriteString("# correlated onto the C function it wraps. 'recv' marks an\n")
	b.WriteString("# instance method; Python-derived params have an empty type\n")
	b.WriteString("# (types are deferred); 'calls' is a non-authoritative C hint.\n\n")

	for i, p := range pkgs {
		if i > 0 {
			b.WriteString("\n")
		}
		emitGoPackage(&b, p)
	}

	emitGoConstants(&b, enums)
	return b.String()
}

// emitGoConstants emits the enum compile-time constants as planned Go
// constants (Q29): the exported Go name, the numeric value sourced from
// the C headers, and the originating C symbol for traceability.
func emitGoConstants(b *strings.Builder, enums []goEnum) {
	if len(enums) == 0 {
		return
	}
	b.WriteString("\n- constants:\n")
	for _, e := range enums {
		fmt.Fprintf(b, "    - name: %s\n", goConstName(e.Name))
		fmt.Fprintf(b, "      value: %d\n", e.Value)
		fmt.Fprintf(b, "      c: %s\n", e.Name)
	}
}

func emitGoPackage(b *strings.Builder, p goPackage) {
	fmt.Fprintf(b, "- package: %s\n", p.Name)
	b.WriteString("  funcs:\n")
	for _, f := range p.Funcs {
		emitGoFunc(b, f)
	}
}

func emitGoFunc(b *strings.Builder, f goFunc) {
	fmt.Fprintf(b, "    - name: %s\n", f.Name)
	// Provenance: c: for C-derived, py: for Python-derived; an entry may
	// carry both when a Python entry point was correlated onto the C
	// function it wraps (GAPS7_8 §8.5). py: is dotted (Class.method) for a
	// class method/constructor, dotless (module_func) for a module-level
	// function — the convention that distinguishes the two without a
	// schema change (Q15/M3).
	if f.C != "" {
		fmt.Fprintf(b, "      c: %s\n", f.C)
	}
	if f.Py != "" {
		fmt.Fprintf(b, "      py: %s\n", f.Py)
	}
	// recv: marks an entry that is an instance method on a Go type rather
	// than a package-level function (PYMETH method vs static/classmethod).
	if f.Recv != "" {
		fmt.Fprintf(b, "      recv: %s\n", f.Recv)
	}
	if f.Unexported {
		b.WriteString("      unexported: true\n")
	}
	// manual: a TODO the generator could not resolve to a single Go form
	// (an overloaded/variadic __init__, Q19); the value is the reason.
	if f.Manual != "" {
		fmt.Fprintf(b, "      manual: %q\n", f.Manual)
	}
	// note: an informational, non-TODO annotation (a resolved decision such
	// as a constructor family's sibling list or a "plain data container; no
	// constructor" determination), distinct from manual: so it does not count
	// as an unresolved overload.
	if f.Note != "" {
		fmt.Fprintf(b, "      note: %q\n", f.Note)
	}
	if f.Return != "" {
		// Quoted: Go types lead with [ or * which are YAML sigils.
		fmt.Fprintf(b, "      return: %q\n", f.Return)
	} else if f.Py != "" && f.C == "" {
		// Python-only entry with no resolved return yet: emit a blank
		// return slot so every such Python entry carries a return: field for
		// the deferred type-recovery stages to fill, paralleling the blank
		// type: "" on Python-derived params (PLAN.md §PYGEN Bug 1).
		//
		// When the entry also carries c: provenance, the C signature is
		// authoritative: a blank Return there means the C function is void, so
		// no return: line is emitted at all. Gating on f.C == "" prevents the
		// blank slot (which means "unresolved") from conflating with genuine
		// void C functions that gained py: provenance through correlation.
		b.WriteString("      return: \"\"\n")
	}
	if len(f.Params) == 0 {
		b.WriteString("      params: []\n")
	} else {
		b.WriteString("      params:\n")
		for _, p := range f.Params {
			fmt.Fprintf(b, "        - name: %s\n", p.Name)
			// Python-derived params carry an empty type (PYGEN Q16-A);
			// C-derived params carry the type resolved by goTypeOf.
			fmt.Fprintf(b, "          type: %q\n", p.Type)
			// The Python default-value literal, when present, is a type
			// hint for the deferred type-recovery stages (PLAN.md §PYGEN
			// Bug 2). Emitted verbatim, mirroring how python.yaml writes
			// the same literal; only Python-derived params carry one.
			if p.Default != "" {
				fmt.Fprintf(b, "          default: %s\n", p.Default)
			}
		}
	}
	// calls: the non-authoritative C cross-reference hint for a
	// Python-derived entry (PYGEN Q17) — the C symbols seen in the body,
	// surfaced for whoever fills in the deferred types, never a binding.
	if len(f.Calls) > 0 {
		b.WriteString("      calls: [")
		b.WriteString(strings.Join(f.Calls, ", "))
		b.WriteString("]\n")
	}
	// dispatch: the argument-polymorphic return ladder, carried through
	// from python.yaml with Go-translated guards and returns ([7]). It is
	// emitted rather than expanded into N Go methods so the human operator
	// can decide which return-polymorphic coordinate types warrant
	// per-branch methods. Return above holds the principal arm; this block
	// records every arm. An arm whose guard or return did not translate
	// carries the blank, exactly as a top-level return: "" does.
	if len(f.Dispatch) > 0 {
		b.WriteString("      dispatch:\n")
		for _, d := range f.Dispatch {
			fmt.Fprintf(b, "        - on: %q\n", d.On)
			fmt.Fprintf(b, "          returns: %q\n", d.Returns)
		}
	}
}

func genPython(out string) error {
	script := buildInspectScript()

	cmd := exec.Command("python3", "-c", pyPreamble+script)
	cmd.Env = append(cmd.Environ(),
		"PYTHONPATH="+mmgroupPythonPath,
		"LD_LIBRARY_PATH="+mmgroupLibraryPath,
	)
	raw, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("python3: %s\n%s", err, ee.Stderr)
		}
		return err
	}

	var dump pyDump
	if err := json.Unmarshal(raw, &dump); err != nil {
		return fmt.Errorf("json: %w\n%s", err, raw[:min(len(raw), 200)])
	}

	// Repair the params: [] the runtime inspect could not produce for the
	// compiled cdef classes (QState12, GtWord) by overlaying the parameter
	// lists parsed from the .pyx sources (D1b Half 1).
	if err := applyPxdOverlay(&dump); err != nil {
		return err
	}

	if err := os.WriteFile(out, []byte(emitPythonYAML(dump)), 0o644); err != nil {
		return err
	}

	total := 0
	for _, c := range dump.Classes {
		total += len(c.Methods) + len(c.Properties)
	}
	log.Printf("wrote %d classes (%d members) and %d functions to %s",
		len(dump.Classes), total, len(dump.Functions), out)
	return nil
}

// applyPxdOverlay fills the empty parameter lists of the compiled cdef-class
// methods in the walker dump from the .pyx-parsed signatures (D1b Half 1).
// Every method of QState12/GtWord is a method_descriptor whose runtime
// inspect.signature() raises, so the walker emitted params: []; the .pyx is
// the only surviving source of their parameter lists.
//
// For each overlaid class found in the dump, every method whose dump Params is
// empty and whose name has a .pyx signature gets that signature (self kept, so
// dropSelf works unchanged). A non-empty dump Params is left alone: a
// pure-Python def whose signature inspect could already read (e.g. __init__)
// is authoritative over the .pyx text.
//
// Staleness: it errors if a class named in pxdSources is absent from the dump
// (the class vanished upstream, or module exclusion now hides it), or if the
// overlay matched zero empty methods across all its classes (the cdef methods
// stopped surfacing as method_descriptors, so the repair silently no-ops).
func applyPxdOverlay(dump *pyDump) error {
	overlay, err := pxdMethodOverlay()
	if err != nil {
		return err
	}

	// Index the dump's class entries by name so each overlay class can be
	// located (and confirmed present) directly.
	byName := map[string]*classDecl{}
	for i := range dump.Classes {
		byName[dump.Classes[i].Name] = &dump.Classes[i]
	}

	filled := 0
	for cls, methods := range overlay {
		cd, ok := byName[cls]
		if !ok {
			return fmt.Errorf("pxd overlay class %s not found in walker dump (upstream removed it or module exclusion now hides it?)", cls)
		}
		for i := range cd.Methods {
			m := &cd.Methods[i]
			if len(m.Params) != 0 {
				continue // inspect already read this one (pure-Python def)
			}
			ps, ok := methods[m.Name]
			if !ok {
				continue // no .pyx signature (e.g. an inherited pure-Python method)
			}
			m.Params = ps
			filled++
		}
	}
	if filled == 0 {
		return fmt.Errorf("pxd overlay filled zero methods (cdef methods no longer surface as empty-param method_descriptors? .pyx or walker changed)")
	}
	log.Printf("pxd overlay: filled params for %d cdef methods across %d classes", filled, len(overlay))
	return nil
}

// pyDump is the JSON object the inspect walker prints: the discovered
// classes plus the module-level functions and Cython builtins (the
// mmgroup.mm_op C-API surface) found by the second pass (GAPS7_8 Gap 8).
type pyDump struct {
	Classes   []classDecl `json:"classes"`
	Functions []pyFunc    `json:"functions"`
}

type classDecl struct {
	Name       string       `json:"name"`
	Module     string       `json:"module"`
	Bases      []string     `json:"bases"`
	Methods    []methodDecl `json:"methods"`
	Properties []string     `json:"properties"`
}

// pyFunc is a module-level Python function or Cython builtin. It has no
// receiver — the Python analogue of a package-level C function. Per Q15/M3
// these are emitted into python.yaml as top-level entries alongside the
// classes (no separate py_funcs: key); genGo later folds them into
// go.yaml's funcs: with a dotless py: field.
type pyFunc struct {
	Name   string    `json:"name"`
	Module string    `json:"module"`
	Params []pyParam `json:"params"`
	Calls  []string  `json:"calls"`
	Raises []string  `json:"raises"`
	// Return is the canonical Python return type name probed statically by the
	// walker, or "" when undetermined. Cython builtins always carry "" (no
	// readable source).
	Return string `json:"return_type"`
}

type methodDecl struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Inherited bool      `json:"inherited"`
	Params    []pyParam `json:"params"`
	Calls     []string  `json:"calls"`
	Raises    []string  `json:"raises"`
	// Return is the canonical Python return type name when every concrete
	// return arm agrees and there is no dispatch, else "".
	Return string `json:"return_type"`
	// Dispatch is the isinstance/value ladder for argument-polymorphic methods
	// (omitted when the method is monomorphic). NotImplemented arms are dropped.
	Dispatch []dispatchEntry `json:"dispatch"`
}

// dispatchEntry is one arm of an argument-polymorphic method's return-type
// ladder: On is the matched argument type ("GCode") or value ("int==2"), and
// Returns is the canonical Python type that arm yields.
type dispatchEntry struct {
	On      string `json:"on"`
	Returns string `json:"returns"`
}

type pyParam struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Default string `json:"default,omitempty"`
	// Type is the canonical Python type an isinstance() test in the body
	// accepts for this param, or "" when the param's type is not probed.
	Type string `json:"type,omitempty"`
}

func buildInspectScript() string {
	quoted := make([]string, len(pyExcludeModules))
	for i, m := range pyExcludeModules {
		quoted[i] = fmt.Sprintf("%q", m)
	}
	excludeList := strings.Join(quoted, ", ")

	return `
import ast, inspect, importlib, pkgutil, textwrap, sys, io, contextlib

EXCLUDE = [` + excludeList + `]

# Operator dunders that constitute the public algebra of mmgroup's
# mathematical objects (group/ring/lattice operations). These are the
# API for MM, MMVector, Cocode, PLoop, GCode, XLeech2, AutPL, so they
# must be captured even though they are underscore-prefixed. All other
# dunders (object plumbing: __reduce__, __getattribute__, ...) stay
# excluded. (GAP6.md Q5.)
DUNDER_WHITELIST = {
    "__init__",
    # binary arithmetic / group ops
    "__mul__", "__rmul__",
    "__add__", "__radd__",
    "__sub__", "__rsub__",
    "__truediv__", "__rtruediv__",
    "__pow__",
    "__mod__",
    "__and__", "__rand__",
    "__lshift__", "__rshift__",
    # unary ops
    "__neg__", "__pos__", "__abs__", "__invert__",
    # comparison
    "__eq__", "__ne__",
    # container / sizing / truth / text
    "__len__", "__getitem__", "__setitem__", "__bool__", "__repr__",
    "__hash__", "__str__", "__contains__",
    # in-place (mutating) ops on rep vectors / XLeech2
    "__iadd__", "__isub__", "__imul__", "__itruediv__",
    "__ilshift__", "__irshift__",
}

def keep_member(name):
    """Keep public members and whitelisted operator dunders; drop the
    rest of the underscore-prefixed plumbing."""
    if name.startswith('_'):
        return name in DUNDER_WHITELIST
    return True

def excluded(modname):
    return any(modname == e or modname.startswith(e + ".") for e in EXCLUDE)

def _is_instance_method(obj):
    # Pure-Python def methods are functions; Cython cdef class methods
    # are method_descriptors. Accept both, so cdef classes (QState12,
    # GtWord) and the cdef methods inherited by pure-Python subclasses
    # (QStateMatrix) are not silently dropped. (GAPS7_8 Q6.)
    return inspect.isfunction(obj) or inspect.ismethoddescriptor(obj)

def params_of(obj):
    try:
        sig = inspect.signature(obj)
    except (ValueError, TypeError):
        return []
    result = []
    for p in sig.parameters.values():
        entry = {"name": p.name, "kind": p.kind.name}
        if p.default is not p.empty:
            entry["default"] = repr(p.default)
        result.append(entry)
    return result

def calls_in(obj):
    try:
        src = textwrap.dedent(inspect.getsource(obj))
    except (OSError, TypeError):
        return [], []
    try:
        tree = ast.parse(src)
    except SyntaxError:
        return [], []
    names = []
    raises = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            f = node.func
            if isinstance(f, ast.Attribute):
                if isinstance(f.value, ast.Name):
                    names.append(f.value.id + "." + f.attr)
                else:
                    names.append(f.attr)
            elif isinstance(f, ast.Name):
                names.append(f.id)
        elif isinstance(node, ast.Raise) and node.exc is not None:
            exc = node.exc
            if isinstance(exc, ast.Call) and isinstance(exc.func, ast.Name):
                raises.append(exc.func.id)
            elif isinstance(exc, ast.Name):
                raises.append(exc.id)
    r = set(raises)
    return sorted(set(names) - r), sorted(r)

def _is_prelude_guard(stmt):
    """True for the lazy-import prelude 'if import_pending: complete_import()'
    that opens (and sometimes nests inside) many mmgroup methods. These guards
    carry no type signal and must not be mistaken for dispatch arms."""
    if not isinstance(stmt, ast.If):
        return False
    t = stmt.test
    if isinstance(t, ast.Name) and t.id == "import_pending":
        return True
    # Some bodies write 'if import_pending: complete_import()' where the call
    # is the sole statement; accept the test-name form above and also a body
    # that only calls complete_import().
    for s in stmt.body:
        if isinstance(s, ast.Expr) and isinstance(s.value, ast.Call):
            f = s.value.func
            if isinstance(f, ast.Name) and f.id == "complete_import":
                return True
    return False

def _classify_return(value, classname):
    """Map an ast.Return.value node to a canonical Python type name, or "" when
    it cannot be determined statically. None means a bare 'return' (no value).
    Returns the sentinel "<skip>" for 'return NotImplemented' so callers can
    drop that arm entirely (it is a fallback, not a real return type)."""
    if value is None:
        return "None"
    if isinstance(value, ast.Constant):
        if value.value is None:
            return "None"
        return type(value.value).__name__
    if isinstance(value, ast.Name):
        if value.id == "self":
            return classname or ""
        if value.id == "NotImplemented":
            return "<skip>"
        if value.id == "None":
            return "None"
        return ""
    if isinstance(value, ast.Call):
        f = value.func
        # A constructor call 'ClassName(...)' is a Name; an instance-method
        # call 'Cocode(self).__and__(other)' is an Attribute and is not a
        # bare construction, so it stays unresolved (conservative).
        if isinstance(f, ast.Name):
            return f.id
        return ""
    return ""

def _collect_returns(node, classname):
    """Gather the classified types of every return in a subtree, dropping the
    NotImplemented sentinel. Used to decide a single return type when the body
    has no type/value dispatch (all arms must agree)."""
    found = set()
    for n in ast.walk(node):
        if isinstance(n, ast.Return):
            t = _classify_return(n.value, classname)
            if t == "<skip>":
                continue
            found.add(t)
    return found

def _branch_return(stmts, classname):
    """The canonical return type of an if/elif branch body: the first concrete
    return found (skipping NotImplemented), descending into nested ifs since a
    branch may itself dispatch on a value (e.g. 'if other == 1: return self')."""
    for n in ast.walk(ast.Module(body=list(stmts), type_ignores=[])):
        if isinstance(n, ast.Return):
            t = _classify_return(n.value, classname)
            if t == "<skip>":
                continue
            if t:
                return t
    return ""

def _isinstance_test(test):
    """Parse 'isinstance(name, T)' or 'isinstance(name, (T1, T2, ...))' into
    (varname, [TypeName, ...]); return (None, []) for any other test. The
    enclosing parens in 'if (isinstance(x, T)):' do not change the AST."""
    if not (isinstance(test, ast.Call) and isinstance(test.func, ast.Name)
            and test.func.id == "isinstance" and len(test.args) == 2):
        return None, []
    target = test.args[0]
    if not isinstance(target, ast.Name):
        return None, []
    spec = test.args[1]
    types = []
    if isinstance(spec, ast.Name):
        types.append(spec.id)
    elif isinstance(spec, (ast.Tuple, ast.List)):
        for el in spec.elts:
            if isinstance(el, ast.Name):
                types.append(el.id)
    if not types:
        return None, []
    return target.id, types

def _value_test(test):
    """Parse 'name == <int constant>' into (varname, 'int==N'); also handle
    'name in (a, b)' as a multi-value arm. Returns (None, []) otherwise."""
    if isinstance(test, ast.Compare) and len(test.ops) == 1 \
            and isinstance(test.left, ast.Name):
        op = test.ops[0]
        comp = test.comparators[0]
        if isinstance(op, ast.Eq) and isinstance(comp, ast.Constant) \
                and isinstance(comp.value, int) and not isinstance(comp.value, bool):
            return test.left.id, [str(comp.value)]
        if isinstance(op, ast.In) and isinstance(comp, (ast.Tuple, ast.List)):
            vals = []
            for el in comp.elts:
                if isinstance(el, ast.Constant) and isinstance(el.value, int) \
                        and not isinstance(el.value, bool):
                    vals.append(str(el.value))
            if vals:
                return test.left.id, vals
    return None, []

def _walk_dispatch(stmts, classname, dispatch, param_types):
    """Walk an if/elif ladder, emitting one dispatch entry per arm whose test
    is an isinstance() (type dispatch) or a value comparison (value dispatch),
    skipping NotImplemented arms. Recurses through 'else' bodies so nested
    ladders (e.g. an isinstance arm wrapping an 'other == 1/2/4' ladder) are
    flattened. param_types accumulates the isinstance types seen per param."""
    for stmt in stmts:
        if not isinstance(stmt, ast.If):
            continue
        if _is_prelude_guard(stmt):
            # The guard carries no dispatch; still descend its else (rare).
            _walk_dispatch(stmt.orelse, classname, dispatch, param_types)
            continue
        var, itypes = _isinstance_test(stmt.test)
        if var is not None:
            param_types.setdefault(var, [])
            for t in itypes:
                if t not in param_types[var]:
                    param_types[var].append(t)
            ret = _branch_return(stmt.body, classname)
            if ret and ret != "<skip>":
                on = itypes[0] if len(itypes) == 1 else "(" + ", ".join(itypes) + ")"
                dispatch.append({"on": on, "returns": ret})
            # An isinstance arm may itself hold a value ladder; descend body.
            _walk_dispatch(stmt.body, classname, dispatch, param_types)
            _walk_dispatch(stmt.orelse, classname, dispatch, param_types)
            continue
        vvar, vvals = _value_test(stmt.test)
        if vvar is not None:
            ret = _branch_return(stmt.body, classname)
            if ret and ret != "<skip>":
                for v in vvals:
                    dispatch.append({"on": "int==" + v, "returns": ret})
            _walk_dispatch(stmt.orelse, classname, dispatch, param_types)
            continue
        # Non-dispatch if: descend both arms for nested ladders.
        _walk_dispatch(stmt.body, classname, dispatch, param_types)
        _walk_dispatch(stmt.orelse, classname, dispatch, param_types)

def types_of(obj, classname=None):
    """Static type probe for a method or function. Returns
    {"return_type": str, "dispatch": list, "param_types": dict}.

    return_type is a canonical Python type name when every concrete return
    arm agrees (and there is no dispatch), else "". dispatch is the
    isinstance/value ladder as [{"on": TypeName|"int==N", "returns": Type}].
    param_types maps each parameter probed by isinstance() to its accepted
    type names. Everything is AST-only and conservative: ambiguity yields ""
    or an omitted entry, never a guess. NotImplemented arms are dropped."""
    empty = {"return_type": "", "dispatch": [], "param_types": {}}
    try:
        src = textwrap.dedent(inspect.getsource(obj))
    except (OSError, TypeError):
        return empty
    try:
        tree = ast.parse(src)
    except SyntaxError:
        return empty
    func = None
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            func = node
            break
    if func is None:
        return empty
    body = list(func.body)
    # Drop the leading lazy-import prelude so it is not read as dispatch.
    while body and _is_prelude_guard(body[0]):
        body = body[1:]

    dispatch = []
    param_types = {}
    _walk_dispatch(body, classname, dispatch, param_types)

    # Single return type: only when there is no dispatch and every concrete
    # return arm agrees on one non-empty type.
    return_type = ""
    if not dispatch:
        rets = _collect_returns(ast.Module(body=body, type_ignores=[]), classname)
        rets.discard("")
        if len(rets) == 1:
            return_type = next(iter(rets))

    return {
        "return_type": return_type,
        "dispatch": dispatch,
        "param_types": param_types,
    }

# ---------------------------------------------------------------------------
# Runtime return-type oracle.
#
# The AST probe (types_of) resolves only what the *source* spells out:
# constructor returns and isinstance/value dispatch. Two large gaps remain.
#   1. Cython-less arity dispatch like GCode.theta(self, g2=None): the method
#      is callable with no real argument, but g2 == None is not an isinstance
#      test, so the AST leaves return_type "".
#   2. Dispatch arms whose branch returns an expression the AST cannot name
#      (e.g. GCode.__and__ with a GCode argument runs through a helper and
#      yields the private PLoopIntersection type).
# For both, the only authority is the running interpreter. The walker already
# executes inside a fully initialized mmgroup (pyPreamble runs complete_import
# for ploop and gcode), so it can build real objects and observe
# type(result).__name__ directly.
#
# Safety contract: every probe is wrapped so that a failed construction or
# call leaves the AST answer untouched -- the oracle only ever *fills* a gap
# or *overrides* with a concrete observed type, and can never crash the walk.
# All probes redirect stdout into a throwaway buffer so a stray print inside
# a method body cannot corrupt the JSON the walker writes to stdout.
# ---------------------------------------------------------------------------

# Canonical "zero / identity" constructor arguments for each class the oracle
# knows how to build. A class absent from this map is never probed (the AST
# result stands). Each value is the positional-argument tuple passed as
# cls(*args); a bare int is sugar for a 1-tuple, and an empty tuple () means
# zero-arg construction (the class's defaults supply a canonical instance).
# Validated against the live mmgroup build:
#   GCode(0x123) nonzero codeword, Cocode(0) zero coset, PLoop(1) identity,
#   GcVector(0) zero vector, Parity(0) even, AutPL(1) identity automorphism,
#   XLeech2(0x200) a short vector, MM(1) monster identity (MM(0) is rejected),
#   Xsp2_Co1(1)/Axis(1)/BabyAxis(1) identity elements/axes, MMVector(3)/
#   MMVectorCRT(3) over characteristic p=3, QState12(4, 0) 4 rows / data 0,
#   QStateMatrix(4, 4) 4 rows / 4 cols, Orbit_Lin2()/Orbit_Elem2() all-default.
ORACLE_CTOR_ARG = {
    "GCode": 0x123,
    "Cocode": 0,
    "PLoop": 1,
    "GcVector": 0,
    "Parity": 0,
    "AutPL": 1,
    "XLeech2": 0x200,
    "MM": 1,
    "Xsp2_Co1": 1,
    "Axis": 1,
    "BabyAxis": 1,
    "MMVector": 3,
    "MMVectorCRT": 3,
    "QState12": (4, 0),
    "QStateMatrix": (4, 4),
    "Orbit_Lin2": (),
    "Orbit_Elem2": (),
    "GtWord": (),
    "Xsp2_Co1_Group": (),
}

# Classes the oracle must build but which are not re-exported at the top-level
# mmgroup package. _oracle_make_named consults this table before falling back to
# getattr(mmgroup, name): GtWord lives in mmgroup.mm_reduce, not mmgroup.
ORACLE_IMPORT_PATH = {
    "GtWord": "mmgroup.mm_reduce",
}

def _oracle_ctor_args(key):
    """Normalize a registry value into the positional-argument tuple to splat
    into cls(*args). A tuple (including the empty zero-arg tuple) is returned
    as-is; any other value (an int) is wrapped into a 1-tuple."""
    v = ORACLE_CTOR_ARG[key]
    return v if isinstance(v, tuple) else (v,)

def _oracle_make_named(name):
    """Build a fresh instance of the mmgroup class called name, used both as a
    method receiver and as a dispatch-arm argument. Returns (obj, ok); ok is
    False for any class the oracle does not know or whose construction raises."""
    if name not in ORACLE_CTOR_ARG:
        return None, False
    path = ORACLE_IMPORT_PATH.get(name)
    if path is not None:
        try:
            module = importlib.import_module(path)
        except Exception:
            return None, False
        cls = getattr(module, name, None)
    else:
        cls = getattr(mmgroup, name, None)
    if cls is None:
        return None, False
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            return cls(*_oracle_ctor_args(name)), True
    except Exception:
        return None, False

def _oracle_make_receiver(cls, classname):
    """Build the receiver for a method probe from the method's true class
    object, sized by the registry argument tuple for its name. Using cls (not a
    name lookup) keeps the real class identity for re-exported classes.
    Returns (obj, ok); ok is False when classname is unknown or construction
    raises."""
    if classname not in ORACLE_CTOR_ARG:
        return None, False
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            return cls(*_oracle_ctor_args(classname)), True
    except Exception:
        return None, False

def _oracle_arm_arg(on):
    """Materialize the argument a dispatch arm matches. on is either an
    isinstance target type name (build an instance of it) or "int==N" for a
    value arm (pass the literal int N). Returns (arg, ok)."""
    if on.startswith("int=="):
        try:
            return int(on[len("int=="):]), True
        except (ValueError, TypeError):
            return None, False
    return _oracle_make_named(on)

def _oracle_call_type(bound, args):
    """Call bound(*args) with stdout muted and return type(result).__name__,
    or "" if the call raises. Never propagates an exception."""
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            r = bound(*args)
    except Exception:
        return ""
    return type(r).__name__

def _oracle_nullary_callable(bound):
    """True when bound can be invoked with no positional argument: every
    parameter past the (already-bound) self either has a default or is a
    *args/**kwargs slot. This is what lets theta(g2=None) be probed as a
    no-arg call while __truediv__(other) is not.

    A compiled cdef method (QState12, GtWord) is a method_descriptor on which
    inspect.signature raises ValueError -- mmgroup is built without
    embedsignature, so the arity is unreadable here. Rather than skip every
    such method (probe 1 would never fire for any cdef method), be optimistic
    and return True: attempt the no-argument call. Cython parses positional
    arguments before running any body code, so a genuinely non-nullary method
    raises TypeError at the call boundary, which _oracle_call_type swallows --
    the AST answer is preserved. The only methods this newly resolves are the
    genuinely nullary ones (copy, transpose, trace, ...). A TypeError from
    inspect.signature is a different failure (the object is not introspectable
    as a callable signature at all) and stays conservative (False)."""
    try:
        sig = inspect.signature(bound)
    except ValueError:
        return True
    except TypeError:
        return False
    for p in sig.parameters.values():
        if p.kind in (p.POSITIONAL_ONLY, p.POSITIONAL_OR_KEYWORD) \
                and p.default is p.empty:
            return False
    return True

def _oracle_required_positional(bound):
    """Count the required positional parameters of an (already-bound) method --
    those with no default. Returns -1 when the signature cannot be read.

    A compiled cdef method's signature raises ValueError (no embedsignature),
    so the count is unreadable. The probe-3 guard treats that -1 optimistically
    for a binary-operator dunder (whose required arity is 1 by construction):
    it attempts the self-typed call anyway, and a non-binary cdef operator
    raises TypeError at the call boundary, which _oracle_call_type swallows."""
    try:
        sig = inspect.signature(bound)
    except (TypeError, ValueError):
        return -1
    n = 0
    for p in sig.parameters.values():
        if p.kind in (p.POSITIONAL_ONLY, p.POSITIONAL_OR_KEYWORD) \
                and p.default is p.empty:
            n += 1
    return n

# Binary operator dunders for which "operate on an instance of my own class" is
# a meaningful probe. A receiver-monomorphic group/lattice op (MM.__mul__,
# PLoop.__truediv__, ...) returns the same algebra; the self-typed probe fills
# its return type when the source spells out no isinstance/value dispatch the
# AST can read. The NotImplemented guard drops the arms where same-type is the
# wrong operand. Restricting to operators keeps the probe off ordinary
# one-argument methods whose argument is not an object of the same class.
# __eq__/__ne__ are included: comparing an instance against another of its own
# class is a well-typed probe whose bool result the AST cannot name.
ORACLE_BINOP_DUNDERS = {
    "__mul__", "__rmul__", "__add__", "__radd__", "__sub__", "__rsub__",
    "__truediv__", "__rtruediv__", "__pow__", "__mod__", "__and__", "__rand__",
    "__or__", "__xor__", "__matmul__", "__ne__",
}

# Probe-4 argument table: methods whose return type the AST and probes 1-3 leave
# blank, but whose one (or few) required argument(s) the oracle can supply
# directly. Keyed by "ClassName.method_name" (Python names). Each value is a
# tuple of positional arguments; an element is one of:
#   - a literal (int, list, ...): passed through as-is
#   - "@self": build another instance of the receiver class (ORACLE_CTOR_ARG)
#   - "@ClassName": build an instance of ClassName via _oracle_make_named
#                   (which honors ORACLE_IMPORT_PATH for off-top-level classes)
ORACLE_PROBE4_ARGS = {
    # int/literal args
    "QState12.row": (0,),
    "QState12.column": (0,),
    "QState12.mul_scalar": (0,),
    "QState12.__itruediv__": (1,),
    "QState12.reshape": (2, 2),
    "QState12.sumup": (0, 0),
    "QState12.qstate12_product": ("@QState12", 0, 0),
    "QState12.qstate12_prep_mul": ("@QState12", 0),
    "Xsp2_Co1_Group.from_xsp": (0,),
    "Xsp2_Co1_Group.from_data": ([],),
    "MMVector.__truediv__": (1,),
    "MMVector.__itruediv__": (1,),
    "MMVector.__ilshift__": (1,),
    "MMVector.__imul__": (1,),
    "MMVector.__irshift__": (1,),
    "MMVector.__lshift__": (1,),
    "MMVector.__rshift__": (1,),
    "MMVector.eval_A": (0x200,),
    # same-class args ("@self")
    "Axis.__eq__": ("@self",),
    "Axis.scalprod15": ("@self",),
    "Axis.product_class": ("@self",),
    "BabyAxis.__eq__": ("@self",),
    "BabyAxis.scalprod15": ("@self",),
    "BabyAxis.product_class": ("@self",),
    "MMVector.__eq__": ("@self",),
    "MMVector.__isub__": ("@self",),
    "MMVector.__rsub__": ("@self",),
    # cross-class args ("@ClassName")
    "Axis.__imul__": ("@MM",),
    "Axis.__mul__": ("@MM",),
    "MMVector.__mul__": ("@MM",),
    "MMVector.mul_exp": ("@MM",),
    "Xsp2_Co1_Group.copy_word": ("@Xsp2_Co1",),
    "Xsp2_Co1_Group.reduce": ("@Xsp2_Co1",),
    "Xsp2_Co1_Group.str_word": ("@Xsp2_Co1",),
    "Xsp2_Co1_Group.raw_str_word": ("@Xsp2_Co1",),
    "Xsp2_Co1_Group.as_tuples": ("@Xsp2_Co1",),
}

def _oracle_probe4_arg(spec, cls, classname):
    """Materialize one probe-4 argument from its spec. "@self" builds another
    receiver-class instance; "@ClassName" builds a named instance (honoring
    ORACLE_IMPORT_PATH); any other value is a literal passed through. Returns
    (arg, ok); ok is False when a sentinel instance cannot be built."""
    if spec == "@self":
        return _oracle_make_receiver(cls, classname)
    if isinstance(spec, str) and spec.startswith("@"):
        return _oracle_make_named(spec[1:])
    return spec, True

def _oracle_probe_call(cls, classname, method_name, args):
    """Build a fresh receiver, bind method_name, and call it with args, all with
    stdout muted. Returns the canonical result type ("None" for NoneType), or ""
    if anything fails or the result is the NotImplemented sentinel. A fresh
    receiver per call means a mutating probe cannot affect any other."""
    inst, ok = _oracle_make_receiver(cls, classname)
    if not ok:
        return ""
    bound = getattr(inst, method_name, None)
    if bound is None or not callable(bound):
        return ""
    t = _oracle_call_type(bound, args)
    if not t or t == "NotImplementedType":
        return ""
    return "None" if t == "NoneType" else t

def oracle_return(cls, method_name, classname, ast_return, ast_dispatch):
    """Resolve a method's return type at runtime to fill the gaps the AST left.

    Returns (return_type, dispatch) -- a refined copy of the AST answer. Four
    independent probes run, each best-effort and each unable to crash the walk
    (every construction and call is guarded; stdout is muted throughout):

      1. Nullary return. If the method is callable with no real argument (every
         parameter past self has a default, e.g. theta(g2=None)), call it and
         record the observed type as return_type. This is independent of
         dispatch: theta both returns Cocode with no argument AND dispatches to
         Parity on a GCode argument, so both fields are filled.
      2. Dispatch arms. For every arm the AST found with shape {on, returns},
         construct the matched argument (an instance of the isinstance type, or
         the literal int of an "int==N" value arm) and call the method, filling
         or overriding the arm's return type with the observed one.
      3. Self-typed operator fallback. For a binary-operator dunder that the AST
         left with no dispatch and exactly one required argument (MM.__mul__,
         which coerces its operand through a helper the AST cannot name), call
         it with an instance of the receiver's own class and record the result
         as return_type.
      4. Per-method argument table. For a still-blank method named in
         ORACLE_PROBE4_ARGS, materialize its declared positional arguments
         (literals, "@self", or "@ClassName" sentinels), call it once, and
         record the observed type as return_type.

    The oracle wins on conflict: an observed concrete type overrides the AST.
    When the receiver class is unknown or cannot be built, the AST answer is
    returned unchanged. The NotImplemented sentinel is never recorded."""
    inst, ok = _oracle_make_receiver(cls, classname)
    if not ok:
        return ast_return, ast_dispatch
    bound = getattr(inst, method_name, None)
    if bound is None or not callable(bound):
        return ast_return, ast_dispatch

    return_type = ast_return
    dispatch = ast_dispatch

    # (1) Nullary return -- independent of any dispatch ladder.
    if _oracle_nullary_callable(bound):
        t = _oracle_probe_call(cls, classname, method_name, [])
        if t:
            return_type = t

    # (2) Resolve each dispatch arm against a freshly built argument.
    if dispatch:
        new_disp = []
        for arm in dispatch:
            on = arm.get("on", "")
            returns = arm.get("returns", "")
            arg, ok_arg = _oracle_arm_arg(on)
            if ok_arg:
                t = _oracle_probe_call(cls, classname, method_name, [arg])
                if t:
                    returns = t
            new_disp.append({"on": on, "returns": returns})
        dispatch = new_disp

    # (3) Self-typed operator fallback for monomorphic group/lattice ops the
    # AST could not read (no dispatch, one required argument). A binary-operator
    # dunder has required arity 1 by construction, so an unreadable cdef
    # signature (-1) is treated optimistically as 1: the self-typed call is
    # attempted, and a method that is not really a one-argument binop raises
    # TypeError at the call boundary (swallowed by _oracle_call_type).
    if not ast_dispatch and not return_type \
            and method_name in ORACLE_BINOP_DUNDERS \
            and _oracle_required_positional(bound) in (1, -1):
        same, ok_same = _oracle_make_receiver(cls, classname)
        if ok_same:
            t = _oracle_probe_call(cls, classname, method_name, [same])
            if t:
                return_type = t

    # (4) Per-method argument table for methods still blank that take one (or a
    # few) required non-self argument(s) the prior probes cannot guess. The
    # ORACLE_PROBE4_ARGS spec names a literal, "@self", or "@ClassName" for each
    # position; each is materialized fresh and the method called once. Same
    # safety contract as the other probes: every build/call is guarded, stdout
    # is muted, a fresh receiver is used, and NotImplemented is never recorded.
    if not return_type:
        spec = ORACLE_PROBE4_ARGS.get(classname + "." + method_name)
        if spec is not None:
            args = []
            ok_args = True
            for s in spec:
                arg, ok_arg = _oracle_probe4_arg(s, cls, classname)
                if not ok_arg:
                    ok_args = False
                    break
                args.append(arg)
            if ok_args:
                t = _oracle_probe_call(cls, classname, method_name, args)
                if t:
                    return_type = t

    return return_type, dispatch

def _annotate_params(params, param_types):
    """Stamp a 'type' field onto each param entry for which isinstance()
    revealed an accepted type. A param tested against several types takes the
    first (the ladder order is the source's declared preference); unknown
    params are left untyped. Mutates and returns the params list."""
    for p in params:
        types = param_types.get(p["name"])
        if types:
            p["type"] = types[0]
    return params

def _typed_method(mname, kind, mobj, classname, source_obj, cls):
    """Build one method dict with structure (params/calls/raises) plus the
    static type probe (return_type/dispatch/param types). source_obj is the
    underlying function whose source is read for calls/types (the same object
    used for the structural pass), while mobj supplies the signature. cls is the
    owning class object, used by the runtime oracle to instantiate a receiver.

    After the AST probe, the runtime oracle refines the result for plain
    instance methods (kind 'method'): it fills a blank return_type, fills blank
    dispatch arms, and overrides the AST where it observes a concrete type.
    classmethods and staticmethods are not probed -- they have no instance
    receiver, and some (e.g. show_basis) print, which the oracle must not run
    on the JSON-writing process."""
    c, r = calls_in(source_obj)
    t = types_of(source_obj, classname)
    return_type = t["return_type"]
    dispatch = t["dispatch"]
    if kind == "method":
        return_type, dispatch = oracle_return(
            cls, mname, classname, return_type, dispatch)
    entry = {
        "name": mname,
        "kind": kind,
        "params": _annotate_params(params_of(mobj), t["param_types"]),
        "calls": c,
        "raises": r,
        "return_type": return_type,
    }
    if dispatch:
        entry["dispatch"] = dispatch
    return entry

# Discover all mmgroup submodules.
modules = []
for importer, modname, ispkg in pkgutil.walk_packages(
    mmgroup.__path__, prefix="mmgroup."
):
    if excluded(modname):
        continue
    try:
        mod = importlib.import_module(modname)
        modules.append(mod)
    except Exception:
        pass

# Collect all classes defined in mmgroup.
seen = set()
result = []
for mod in modules:
    for cname, cls in inspect.getmembers(mod, inspect.isclass):
        if not hasattr(cls, "__module__"):
            continue
        if not cls.__module__.startswith("mmgroup"):
            continue
        # A class whose definition home is excluded may still be part of
        # the public API if it is re-exported from a non-excluded module
        # (e.g. Axis/BabyAxis are defined in mmgroup.tests.axes.axis but
        # re-exported from mmgroup.axes, which api.rst documents).
        # cls.__module__ points at the definition home, so trust the
        # re-export path instead: if the walked module mod is not
        # excluded and binds this same class object under cname, keep
        # it. The "is cls" identity test rejects same-named different
        # classes. (GAP5.md Q3; module: stays the definition home, Q4.)
        reexported = (
            not excluded(mod.__name__)
            and getattr(mod, cname, None) is cls
        )
        if excluded(cls.__module__) and not reexported:
            continue
        key = cls.__module__ + "." + cls.__qualname__
        if key in seen:
            continue
        seen.add(key)

        methods = []
        _seen_method_names = set()
        def _add_method(entry):
            # Dedup by name across both passes. Pass 1 (instance methods)
            # runs first, so its "method" kind wins; pass 2 only tags
            # static/classmethods, which pass 1 cannot have emitted (a
            # staticmethod/classmethod is neither a function nor a method
            # descriptor). (GAPS7_8 dedup note.)
            if entry["name"] in _seen_method_names:
                return
            _seen_method_names.add(entry["name"])
            methods.append(entry)
        # Pass 1: instance methods. _is_instance_method matches both
        # pure-Python functions and Cython method_descriptors, so cdef
        # classes (QState12, GtWord) and the cdef methods inherited by
        # pure-Python subclasses (QStateMatrix) survive. keep_member
        # preserves the whitelisted operator dunders. (GAPS7_8 Q6, GAP6 Q5.)
        for mname, mobj in sorted(inspect.getmembers(cls, _is_instance_method)):
            if not keep_member(mname):
                continue
            # A name that resolves to a staticmethod/classmethod is tagged
            # by pass 2 with its true kind; skip it here so it is not also
            # emitted as a plain "method". inspect.isfunction can be true
            # for a staticmethod's underlying function on some classes, so
            # guard explicitly. (GAPS7_8 walker dedup.)
            static = inspect.getattr_static(cls, mname, None)
            if isinstance(static, (staticmethod, classmethod)):
                continue
            entry = _typed_method(mname, "method", mobj, cname, mobj, cls)
            entry["inherited"] = mname not in cls.__dict__
            _add_method(entry)
        # Pass 2: re-tag staticmethod / classmethod members.
        for mname, mobj in sorted(inspect.getmembers(cls)):
            if not keep_member(mname):
                continue
            static = inspect.getattr_static(cls, mname, None)
            if isinstance(static, staticmethod):
                entry = _typed_method(
                    mname, "staticmethod", mobj, cname, static.__func__, cls)
                entry["inherited"] = mname not in cls.__dict__
                _add_method(entry)
            elif isinstance(static, classmethod):
                entry = _typed_method(
                    mname, "classmethod", mobj, cname, static.__func__, cls)
                entry["inherited"] = mname not in cls.__dict__
                _add_method(entry)
        props = sorted(
            p for p in dir(cls)
            if not p.startswith('_') and isinstance(getattr(cls, p, None), property)
        )
        if not methods and not props:
            continue
        result.append({
            "name": cname,
            "module": cls.__module__,
            "bases": [b.__name__ for b in cls.__bases__],
            "methods": methods,
            "properties": props,
        })

result.sort(key=lambda c: (c["module"], c["name"]))

# Second pass: module-level functions and Cython builtins.
# inspect.isfunction catches pure-Python defs (top-level re-exports
# like MMV, Octad); inspect.isbuiltin catches Cython module functions
# (the entire mmgroup.mm_op C-API surface: mm_op_*, mm_aux_*, ...).
# Keying on the *defining* module deduplicates a function that is both
# defined in (e.g.) mmgroup.mm_space and re-exported from the mmgroup
# top level. (GAPS7_8 Gap 8, Q7.)
seen_funcs = set()
func_result = []
for mod in modules:
    members = inspect.getmembers(mod, inspect.isfunction) \
            + inspect.getmembers(mod, inspect.isbuiltin)
    for fname, fobj in sorted(members):
        if fname.startswith('_'):
            continue
        owner = getattr(fobj, "__module__", "") or ""
        if not owner.startswith("mmgroup") or excluded(owner):
            continue
        key = owner + "." + fname
        if key in seen_funcs:
            continue
        seen_funcs.add(key)
        c, r = calls_in(fobj)          # ([], []) for Cython builtins
        t = types_of(fobj, None)       # all "" for Cython builtins (no source)
        func_result.append({
            "name": fname,
            "module": owner,           # the *defining* module, not re-exporter
            "params": _annotate_params(params_of(fobj), t["param_types"]),
            "calls": c,
            "raises": r,
            "return_type": t["return_type"],
        })
func_result.sort(key=lambda f: (f["module"], f["name"]))

print(json.dumps({"classes": result, "functions": func_result}, indent=2))
`
}

func emitPythonYAML(dump pyDump) string {
	var b strings.Builder
	b.WriteString("# Auto-generated via Python inspect + AST. Do not edit.\n")
	b.WriteString("#\n")
	b.WriteString("# Discovered by walking all mmgroup submodules.\n")
	b.WriteString("# 'calls' lists functions invoked in the method body,\n")
	b.WriteString("# cross-referenceable against cython.yaml.\n")
	b.WriteString("# Top-level entries with 'kind: function' are module-level\n")
	b.WriteString("# functions / Cython builtins (no parent class); genGo folds\n")
	b.WriteString("# them into go.yaml funcs: with a dotless py: field.\n\n")

	first := true
	for _, c := range dump.Classes {
		if !first {
			b.WriteString("\n")
		}
		first = false
		emitClass(&b, c)
	}
	// Module-level functions are emitted as top-level entries alongside
	// the classes (Q15/M3: no separate py_funcs: key, no parent class).
	for _, f := range dump.Functions {
		if !first {
			b.WriteString("\n")
		}
		first = false
		emitPyFunc(&b, f)
	}
	return b.String()
}

// emitPyFunc writes a module-level function as a top-level python.yaml
// entry. It is shaped like a class entry (column-0 name:/module:) but
// carries kind: function and no bases/methods/properties, marking it as
// a receiver-less free function (Q15/M3).
func emitPyFunc(b *strings.Builder, f pyFunc) {
	fmt.Fprintf(b, "- name: %s\n", f.Name)
	fmt.Fprintf(b, "  module: %s\n", f.Module)
	b.WriteString("  kind: function\n")
	if f.Return != "" {
		fmt.Fprintf(b, "  return: %s\n", f.Return)
	}
	if len(f.Params) == 0 {
		b.WriteString("  params: []\n")
	} else {
		b.WriteString("  params:\n")
		for _, p := range f.Params {
			fmt.Fprintf(b, "    - name: %s\n", p.Name)
			fmt.Fprintf(b, "      kind: %s\n", p.Kind)
			if p.Type != "" {
				fmt.Fprintf(b, "      type: %s\n", p.Type)
			}
			if p.Default != "" {
				fmt.Fprintf(b, "      default: %s\n", p.Default)
			}
		}
	}
	if len(f.Calls) == 0 {
		b.WriteString("  calls: []\n")
	} else {
		b.WriteString("  calls: [")
		b.WriteString(strings.Join(f.Calls, ", "))
		b.WriteString("]\n")
	}
	if len(f.Raises) > 0 {
		b.WriteString("  raises: [")
		b.WriteString(strings.Join(f.Raises, ", "))
		b.WriteString("]\n")
	}
}

func emitClass(b *strings.Builder, c classDecl) {
	fmt.Fprintf(b, "- name: %s\n", c.Name)
	fmt.Fprintf(b, "  module: %s\n", c.Module)
	b.WriteString("  bases: [")
	b.WriteString(strings.Join(c.Bases, ", "))
	b.WriteString("]\n")

	if len(c.Methods) == 0 {
		b.WriteString("  methods: []\n")
	} else {
		b.WriteString("  methods:\n")
		for _, m := range c.Methods {
			emitMethod(b, m)
		}
	}

	if len(c.Properties) == 0 {
		b.WriteString("  properties: []\n")
	} else {
		b.WriteString("  properties: [")
		b.WriteString(strings.Join(c.Properties, ", "))
		b.WriteString("]\n")
	}
}

func emitMethod(b *strings.Builder, m methodDecl) {
	fmt.Fprintf(b, "    - name: %s\n", m.Name)
	fmt.Fprintf(b, "      kind: %s\n", m.Kind)
	if m.Return != "" {
		fmt.Fprintf(b, "      return: %s\n", m.Return)
	}
	if m.Inherited {
		b.WriteString("      inherited: true\n")
	}
	if len(m.Params) == 0 {
		b.WriteString("      params: []\n")
	} else {
		b.WriteString("      params:\n")
		for _, p := range m.Params {
			fmt.Fprintf(b, "        - name: %s\n", p.Name)
			fmt.Fprintf(b, "          kind: %s\n", p.Kind)
			if p.Type != "" {
				fmt.Fprintf(b, "          type: %s\n", p.Type)
			}
			if p.Default != "" {
				fmt.Fprintf(b, "          default: %s\n", p.Default)
			}
		}
	}
	if len(m.Calls) == 0 {
		b.WriteString("      calls: []\n")
	} else {
		b.WriteString("      calls: [")
		b.WriteString(strings.Join(m.Calls, ", "))
		b.WriteString("]\n")
	}
	if len(m.Raises) > 0 {
		b.WriteString("      raises: [")
		b.WriteString(strings.Join(m.Raises, ", "))
		b.WriteString("]\n")
	}
	if len(m.Dispatch) > 0 {
		b.WriteString("      dispatch:\n")
		for _, d := range m.Dispatch {
			fmt.Fprintf(b, "        - on: %s\n", d.On)
			fmt.Fprintf(b, "          returns: %s\n", d.Returns)
		}
	}
}

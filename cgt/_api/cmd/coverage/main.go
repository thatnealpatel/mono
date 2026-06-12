// Command coverage emits the cgt translation coverage matrix as deterministic
// JSONL — one record per go.yaml entry — joining three independently-derived
// columns:
//
//	(a) implemented  — does a Go declaration with the entry's exported name
//	    (and receiver, if a method) exist in the *routed* package? Resolved by
//	    parsing the routed package's non-test .go files with go/parser and
//	    matching the declared name plus the base receiver type. Routing is
//	    strict: leech.Compress is not satisfied by an identically-named
//	    private helper in some other package.
//	(b) oracle_tested — is the entry's Go name referenced as an identifier in
//	    the routed package's *_test.go files? The tested-identifier set is
//	    re-derived from the test ASTs, never trusted from a frozen artifact.
//	(c) provenance   — C-correlated (entry carries a c: symbol), Python-only
//	    (py: but no c:), or manual-supplement (a C symbol whose cython.yaml
//	    source is a hand-added supplement). go.yaml entries always carry at
//	    least one of c:/py:, so a bare manual class never appears on the go
//	    side; the supplement class is surfaced through the correlated C
//	    return (see resolvable_void below).
//
// Two name-skew / type-gap subtleties are encoded by the join rather than
// rediscovered downstream:
//
//   - mmindex Aux-prefix skew: go.yaml specs AuxIndexCheckIntern while the Go
//     implementation and its test are IndexCheckIntern. When the verbatim name
//     misses in a package whose specs are uniformly Aux-prefixed, the matcher
//     retries with the prefix stripped and records name_skew=true. This is a
//     naming divergence, never a coverage gap.
//   - C2 void inversion: a blank go.yaml return: whose correlated C return is
//     void is a mechanically-resolvable no-return, not a genuine type gap. It
//     is flagged resolvable_void=true (the actual fill goes through the
//     generator; this tool never edits go.yaml).
//
// Output ordering is the go.yaml package/within-package order, so two runs are
// byte-identical.
//
// Usage:
//
//	go run -C _api ./cmd/coverage                 # JSONL to stdout
//	go run -C _api ./cmd/coverage -summary        # per-package tallies (JSONL)
//	go run -C _api ./cmd/coverage -spec ../go.yaml -cython ../cython.yaml
//
// Paths default to ./go.yaml and ./cython.yaml relative to the command's
// working directory (i.e. _api/), and the Go source root to ../ (the cgt
// module root); override with -spec, -cython, -root.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"patel.codes/cgt/_api/spec"
)

func main() {
	specPath := flag.String("spec", "go.yaml", "path to the go.yaml spec")
	cythonPath := flag.String("cython", "cython.yaml", "path to the cython.yaml C manifest")
	root := flag.String("root", "..", "path to the cgt module root (holds the routed package dirs)")
	summary := flag.Bool("summary", false, "emit per-package tallies instead of per-entry records")
	flag.Parse()

	if err := run(*specPath, *cythonPath, *root, *summary, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "coverage: %v\n", err)
		os.Exit(1)
	}
}

func run(specPath, cythonPath, root string, summary bool, out *os.File) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return fmt.Errorf("loading spec %s: %w", specPath, err)
	}
	creturns, err := loadCythonReturns(cythonPath)
	if err != nil {
		return fmt.Errorf("loading cython %s: %w", cythonPath, err)
	}

	idx, err := indexModule(root, s)
	if err != nil {
		return fmt.Errorf("indexing Go source under %s: %w", root, err)
	}

	records := buildRecords(s, creturns, idx)

	w := bufio.NewWriter(out)
	defer w.Flush()
	if summary {
		return emitSummary(w, records)
	}
	return emitRecords(w, records)
}

// Record is one row of the coverage matrix: a go.yaml entry joined with its
// implementation, oracle-test, and provenance status.
type Record struct {
	Package        string `json:"package"`         // go.yaml package name
	Name           string `json:"name"`            // exported Go name as specced
	Recv           string `json:"recv"`            // receiver type ("" for plain funcs)
	GoPackage      string `json:"go_package"`      // routed Go import path
	Implemented    bool   `json:"implemented"`     // column (a)
	OracleTested   bool   `json:"oracle_tested"`   // column (b)
	Provenance     string `json:"provenance"`      // column (c)
	C              string `json:"c"`               // originating C symbol ("" if none)
	Py             string `json:"py"`              // python entry point ("" if none)
	BlankReturn    bool   `json:"blank_return"`    // go.yaml return: is blank
	CReturn        string `json:"c_return"`        // correlated C return type ("" if no c)
	ResolvableVoid bool   `json:"resolvable_void"` // C2: blank return whose C return is void
	NameSkew       bool   `json:"name_skew"`       // Aux-prefix normalization applied to match
}

// buildRecords joins the spec, the C return map, and the resolved Go index into
// one record per spec entry, preserving spec order for deterministic output.
func buildRecords(s *spec.Spec, creturns map[string]cReturn, idx *moduleIndex) []Record {
	records := make([]Record, 0, len(s.AllFuncs()))
	for _, pkg := range s.Packages {
		goPkg := idx.importPath(pkg.Name)
		pi := idx.packages[pkg.Name]
		for _, fn := range pkg.Funcs {
			rec := Record{
				Package:     pkg.Name,
				Name:        fn.Name,
				Recv:        fn.Recv,
				GoPackage:   goPkg,
				Provenance:  provenance(fn, creturns),
				C:           fn.C,
				Py:          fn.Py,
				BlankReturn: fn.Return == "",
			}
			if cr, ok := creturns[fn.C]; ok && fn.C != "" {
				rec.CReturn = cr.ret
				rec.ResolvableVoid = rec.BlankReturn && cr.ret == "void"
			}
			impl := pi.implements(fn.Name, fn.Recv)
			tested := pi.tested(fn.Name)
			var skew bool
			if !impl || !tested {
				// Retry the missing column(s) under Aux-prefix stripping for
				// the mmindex name skew, recording the divergence.
				if alt := stripAux(fn.Name); alt != fn.Name {
					if !impl && pi.implements(alt, fn.Recv) {
						impl, skew = true, true
					}
					if !tested && pi.tested(alt) {
						tested, skew = true, true
					}
				}
			}
			rec.Implemented = impl
			rec.OracleTested = tested
			rec.NameSkew = skew
			records = append(records, rec)
		}
	}
	return records
}

// provenance classifies an entry's origin. C-correlated wins when a c: symbol
// is present; a supplement-sourced C symbol is reported as manual-supplement;
// otherwise a py:-only entry is Python-only.
func provenance(fn spec.Func, creturns map[string]cReturn) string {
	if fn.C != "" {
		if cr, ok := creturns[fn.C]; ok && cr.supplement {
			return "manual-supplement"
		}
		return "C-correlated"
	}
	if fn.Py != "" {
		return "Python-only"
	}
	return "manual-supplement"
}

// stripAux removes the leading "Aux" the mmindex specs carry but the Go
// implementations and tests omit. Returns the input unchanged when there is no
// such prefix.
func stripAux(name string) string {
	if rest, ok := strings.CutPrefix(name, "Aux"); ok && rest != "" {
		return rest
	}
	return name
}

// cReturn is the per-C-symbol data the join needs from cython.yaml.
type cReturn struct {
	ret        string // C return type, e.g. "void", "int32_t"
	supplement bool   // source ends in "(supplement)"
}

// loadCythonReturns parses cython.yaml (a flat top-level sequence of C function
// entries) into a name->return map. Only name/return/source are consumed; the
// params blocks are skipped structurally.
func loadCythonReturns(path string) (map[string]cReturn, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]cReturn)
	var name string
	var cur cReturn
	flush := func() {
		if name != "" {
			out[name] = cur
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level entries start at column 0 with "- name:".
		if strings.HasPrefix(line, "- name:") {
			flush()
			name = ytrim(strings.TrimPrefix(line, "- name:"))
			cur = cReturn{}
			continue
		}
		// Fields of the current entry are indented; ignore nested param keys
		// (also "return"/"source" appear there but never at 2-space depth for
		// a different meaning, so keying on the leading two-space indent is
		// safe for this flat manifest).
		if strings.HasPrefix(line, "  return:") {
			cur.ret = ytrim(strings.TrimPrefix(strings.TrimSpace(line), "return:"))
			continue
		}
		if strings.HasPrefix(line, "  source:") {
			src := ytrim(strings.TrimPrefix(strings.TrimSpace(line), "source:"))
			cur.supplement = strings.HasSuffix(src, "(supplement)")
			continue
		}
	}
	flush()
	return out, nil
}

// ytrim trims whitespace and a single pair of surrounding double quotes from a
// scalar YAML value.
func ytrim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// moduleIndex holds the resolved Go surface for every routed package, keyed by
// go.yaml package name.
type moduleIndex struct {
	packages map[string]*pkgIndex
	imports  map[string]string // go.yaml package name -> Go import path
}

// importPath returns the routed Go import path for a go.yaml package name.
func (m *moduleIndex) importPath(yamlPkg string) string {
	if p, ok := m.imports[yamlPkg]; ok {
		return p
	}
	return ""
}

// pkgIndex is the parsed declaration and test-identifier surface of one routed
// package.
type pkgIndex struct {
	// decls keys "Name" for plain functions and exported package-level
	// declarations (types, vars, consts), and "Recv.Name" for methods.
	decls map[string]bool
	// idents is every identifier referenced in the package's _test.go files.
	idents map[string]bool
}

// implements reports whether a declaration named name (with receiver recv, if
// non-empty) exists in the package.
func (p *pkgIndex) implements(name, recv string) bool {
	if p == nil {
		return false
	}
	if recv != "" {
		return p.decls[recv+"."+name]
	}
	return p.decls[name]
}

// tested reports whether name is referenced as an identifier anywhere in the
// package's test files.
func (p *pkgIndex) tested(name string) bool {
	if p == nil {
		return false
	}
	return p.idents[name]
}

// yamlToImport maps a go.yaml package name to its routed Go import path. Every
// routed package (including monster, the flat top-level package) is a
// name-identical subdirectory of the module root.
func yamlToImport(yamlPkg string) string {
	return "patel.codes/cgt/" + yamlPkg
}

// yamlToDir maps a go.yaml package name to its source directory relative to the
// module root: each routed package lives in a same-named subdirectory.
func yamlToDir(root, yamlPkg string) string {
	return filepath.Join(root, yamlPkg)
}

// indexModule parses every routed package's source directory once and records
// its declared surface (column a) and test-identifier set (column b).
func indexModule(root string, s *spec.Spec) (*moduleIndex, error) {
	m := &moduleIndex{
		packages: make(map[string]*pkgIndex, len(s.Packages)),
		imports:  make(map[string]string, len(s.Packages)),
	}
	for _, pkg := range s.Packages {
		m.imports[pkg.Name] = yamlToImport(pkg.Name)
		pi, err := indexPackageDir(yamlToDir(root, pkg.Name))
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", pkg.Name, err)
		}
		m.packages[pkg.Name] = pi
	}
	return m, nil
}

// indexPackageDir parses the .go files in dir, splitting non-test declarations
// (the implementation surface) from test-file identifiers (the oracle-test
// surface). It does not recurse into subdirectories.
func indexPackageDir(dir string) (*pkgIndex, error) {
	pi := &pkgIndex{decls: map[string]bool{}, idents: map[string]bool{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			collectIdents(file, pi.idents)
			continue
		}
		collectDecls(file, pi.decls)
	}
	return pi, nil
}

// collectDecls records the exported declared surface of a non-test file:
// top-level func/method names (methods keyed "Recv.Name") and exported
// type/var/const names.
func collectDecls(file *ast.File, decls map[string]bool) {
	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			name := decl.Name.Name
			if !ast.IsExported(name) {
				continue
			}
			if decl.Recv != nil && len(decl.Recv.List) == 1 {
				if recv := baseTypeName(decl.Recv.List[0].Type); recv != "" {
					decls[recv+"."+name] = true
				}
				continue
			}
			decls[name] = true
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(sp.Name.Name) {
						decls[sp.Name.Name] = true
					}
				case *ast.ValueSpec:
					for _, id := range sp.Names {
						if ast.IsExported(id.Name) {
							decls[id.Name] = true
						}
					}
				}
			}
		}
	}
}

// baseTypeName extracts the receiver's base type name, unwrapping a pointer and
// any generic type-parameter instantiation (e.g. *Foo[T] -> "Foo").
func baseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.IndexExpr:
		return baseTypeName(t.X)
	case *ast.IndexListExpr:
		return baseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// collectIdents records every identifier name appearing in a test file. The
// over-broad set is fine: column (b) intersects it with spec entry names, so
// non-spec identifiers are simply never queried.
func collectIdents(file *ast.File, idents map[string]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			idents[id.Name] = true
		}
		return true
	})
}

// emitRecords writes one JSON object per record, newline-delimited, in spec
// order.
func emitRecords(w *bufio.Writer, records []Record) error {
	enc := json.NewEncoder(w)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

// summaryRow is one per-package tally line.
type summaryRow struct {
	Package      string `json:"package"`
	GoPackage    string `json:"go_package"`
	Entries      int    `json:"entries"`
	Implemented  int    `json:"implemented"`
	OracleTested int    `json:"oracle_tested"`
	CCorrelated  int    `json:"c_correlated"`
	PythonOnly   int    `json:"python_only"`
	Supplement   int    `json:"manual_supplement"`
	BlankReturn  int    `json:"blank_return"`
	ResolvVoid   int    `json:"resolvable_void"`
	NameSkew     int    `json:"name_skew"`
}

// emitSummary writes per-package tallies in package-name order plus a trailing
// total row (package "(all)").
func emitSummary(w *bufio.Writer, records []Record) error {
	rows := map[string]*summaryRow{}
	var order []string
	total := &summaryRow{Package: "(all)"}
	for _, rec := range records {
		row, ok := rows[rec.Package]
		if !ok {
			row = &summaryRow{Package: rec.Package, GoPackage: rec.GoPackage}
			rows[rec.Package] = row
			order = append(order, rec.Package)
		}
		tally(row, rec)
		tally(total, rec)
	}
	sort.Strings(order)
	enc := json.NewEncoder(w)
	for _, name := range order {
		if err := enc.Encode(rows[name]); err != nil {
			return err
		}
	}
	return enc.Encode(total)
}

// tally folds one record into a summary row.
func tally(row *summaryRow, rec Record) {
	row.Entries++
	if rec.Implemented {
		row.Implemented++
	}
	if rec.OracleTested {
		row.OracleTested++
	}
	switch rec.Provenance {
	case "C-correlated":
		row.CCorrelated++
	case "Python-only":
		row.PythonOnly++
	case "manual-supplement":
		row.Supplement++
	}
	if rec.BlankReturn {
		row.BlankReturn++
	}
	if rec.ResolvableVoid {
		row.ResolvVoid++
	}
	if rec.NameSkew {
		row.NameSkew++
	}
}

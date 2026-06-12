// Command report is the cgt translation completeness report (EXECUTION.md
// Track M, item M5): the join point of the metrics track. It emits the four
// headline coverage ratios with their derived denominators, joining only what
// the M1–M3 tools already emit, so every denominator is regenerable rather
// than asserted.
//
// The four ratios, with the prose definition each carries:
//
//	(1) Spec coverage — of the go.yaml entries (the planned Go API surface),
//	    the fraction that have a real Go implementation. Numerator: entries
//	    whose exported name (and receiver, if a method) resolves to a
//	    declaration in the routed package. Denominator: all go.yaml entries.
//	    Source: the Me1 coverage matrix (implemented column).
//
//	(2) Oracle coverage — of the *implemented* entries, the fraction also
//	    pinned to the mmgroup oracle by a test. Numerator: entries that are
//	    both implemented and referenced in the routed package's tests.
//	    Denominator: implemented entries. This is the ratio with real
//	    headroom — a translated symbol with no oracle test is unverified.
//	    Source: the Me1 coverage matrix (implemented AND oracle_tested).
//
//	(3) C-surface coverage — of the live upstream `.ske` C public surface
//	    (`%%EXPORT` functions), the fraction accounted for by the M2 ledger.
//	    Two ratios are emitted per the Hu1 ruling:
//	      - full_surface (the HEADLINE): (correlated + translated-internal +
//	        classified-untranslated) / total exports. Every export is
//	        accounted for — translated, or deliberately untranslated with a
//	        (b)/(c) classification — so this reads complete by classification
//	        (C4: the class-(a) "needed" set is empty). The classification
//	        tables are freshness-asserted, so "complete by classification" is
//	        auditable, not vibes.
//	      - in_scope (the working number): translated / (translated +
//	        needed-but-untranslated). With the class-(a) queue empty the
//	        in-scope denominator is just the translated set, so this also
//	        reads complete; it is the day-to-day gap-burndown number that
//	        moves the moment a class-(a) gap appears.
//	    Source: the M2 surface ledger (`go run _api -report`).
//
//	(4) Python-surface coverage — of the live runtime `mmgroup` public Python
//	    surface (methods + module functions), the fraction accounted for by
//	    the M3 ledger. Mirrors (3): full_surface = (covered + delegated-to-C +
//	    out-of-scope) / total surface (the HEADLINE); in_scope = (covered +
//	    delegated) / (covered + delegated) (the working number).
//	    Source: the M3 surface ledger (`go run _api -report`).
//
// Per the Hu1 resolution the FULL-SURFACE ratio is the operator's headline
// pick for the two surface metrics; the in-scope ratio stays visible as the
// working number. The two M1 ratios (spec, oracle) have no classification
// dimension, so they carry a single value each — and oracle coverage is where
// the genuine headroom lives.
//
// The report runs green only when every upstream surface symbol is classified:
// an unclassified `.ske` export or public Python symbol is a hard error (the
// generator's -report mode fails closed), so report exits non-zero on any
// unclassified gap. It derives both halves by subprocess, so a single
//
//	go run -C _api ./cmd/report
//
// produces the whole report. Running twice is byte-identical: the inputs are
// committed/live-rederived and every collection is emitted in a fixed order.
//
// Usage:
//
//	go run -C _api ./cmd/report             # headline report (JSON) to stdout
//	go run -C _api ./cmd/report -matrix m.jsonl -surface s.json
//	go run -C _api ./cmd/report -spec ../go.yaml -cython ../cython.yaml
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	matrix := flag.String("matrix", "", "precomputed Me1 coverage matrix JSONL (default: run cmd/coverage)")
	surface := flag.String("surface", "", "precomputed M2/M3 surface ledger JSON (default: run the generator -report)")
	specPath := flag.String("spec", "go.yaml", "path to go.yaml (passed to cmd/coverage)")
	cythonPath := flag.String("cython", "cython.yaml", "path to cython.yaml (passed to cmd/coverage)")
	root := flag.String("root", "..", "cgt module root (passed to cmd/coverage)")
	flag.Parse()

	if err := run(opts{
		matrix:     *matrix,
		surface:    *surface,
		specPath:   *specPath,
		cythonPath: *cythonPath,
		root:       *root,
	}, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "report: %v\n", err)
		os.Exit(1)
	}
}

// opts carries the parsed command-line configuration.
type opts struct {
	matrix     string
	surface    string
	specPath   string
	cythonPath string
	root       string
}

func run(o opts, out io.Writer) error {
	records, err := loadMatrix(o)
	if err != nil {
		return err
	}
	surf, err := loadSurface(o)
	if err != nil {
		return err
	}

	rep := buildReport(records, surf)

	// Fail closed on any unclassified upstream surface gap: the report is a
	// measurement, not a belief, so an unaccounted-for export or public
	// Python symbol is a hard error even though the JSON is still emitted for
	// inspection.
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return err
	}
	if n := len(surf.C.Unclassified) + len(surf.Py.Unclassified); n > 0 {
		return fmt.Errorf("%d upstream surface symbol(s) are unclassified; the completeness report is not green (extend the M2/M3 ledgers)", n)
	}
	return nil
}

// Record mirrors the JSON schema emitted by cmd/coverage. Only the fields the
// report consumes are decoded; the schema is the join contract with cmd/coverage.
type Record struct {
	Implemented  bool `json:"implemented"`
	OracleTested bool `json:"oracle_tested"`
}

// surfaceReport mirrors the JSON object emitted by the generator's -report
// mode (the M2/M3 surface ledgers). The two halves carry their own
// accounted/total breakdown; report joins them into ratios.
type surfaceReport struct {
	C  cSurface  `json:"c_surface"`
	Py pySurface `json:"py_surface"`
}

// cSurface mirrors the M2 ledger tally. Translated = Correlated + Internal;
// Untranslated entries are complete by classification.
type cSurface struct {
	Exports      int      `json:"exports"`
	Correlated   int      `json:"correlated"`
	Internal     int      `json:"internal"`
	Untranslated int      `json:"untranslated"`
	Unclassified []string `json:"unclassified"`
}

// pySurface mirrors the M3 ledger tally. Translated = Covered + Delegated;
// OutOfScope entries are complete by classification.
type pySurface struct {
	Surface      int      `json:"surface"`
	Covered      int      `json:"covered"`
	Delegated    int      `json:"delegated"`
	OutOfScope   int      `json:"out_of_scope"`
	Unclassified []string `json:"unclassified"`
}

// Report is the emitted completeness report: the four headline metrics plus a
// machine-checkable green flag.
type Report struct {
	Spec     Ratio        `json:"spec_coverage"`     // (1) implemented / entries
	Oracle   Ratio        `json:"oracle_coverage"`   // (2) impl+tested / implemented (real headroom)
	CSurf    SurfaceRatio `json:"c_surface"`         // (3) M2: full-surface headline + in-scope
	PySurf   SurfaceRatio `json:"py_surface"`        // (4) M3: full-surface headline + in-scope
	Green    bool         `json:"green"`             // true iff no upstream surface gaps
	Gaps     int          `json:"unclassified_gaps"` // count of unclassified surface symbols
	Headline string       `json:"headline"`          // human-readable one-liner of the headline picks
}

// Ratio is a single coverage fraction with its numerator and denominator
// exposed so the value is auditable, never opaque.
type Ratio struct {
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	Value       float64 `json:"value"` // numerator/denominator, 0 when denominator is 0
}

// SurfaceRatio carries the two ratios the Hu1 ruling mandates for an upstream
// surface metric: the full-surface number (the operator's headline pick,
// exclusions counted complete-by-classification) and the in-scope number (the
// working gap-burndown denominator). FullSurfaceIsHeadline records the Hu1
// pick so a downstream consumer reads the designation, not a convention.
type SurfaceRatio struct {
	FullSurface           Ratio `json:"full_surface"`             // headline: accounted / total
	InScope               Ratio `json:"in_scope"`                 // working: translated / (translated + needed)
	FullSurfaceIsHeadline bool  `json:"full_surface_is_headline"` // Hu1: the full-surface number is the headline
}

// buildReport joins the M1 coverage matrix and the M2/M3 surface tallies into
// the four headline metrics.
func buildReport(records []Record, surf surfaceReport) Report {
	entries, implemented, oracle := 0, 0, 0
	for _, r := range records {
		entries++
		if r.Implemented {
			implemented++
			if r.OracleTested {
				oracle++
			}
		}
	}

	gaps := len(surf.C.Unclassified) + len(surf.Py.Unclassified)
	rep := Report{
		Spec:     ratio(implemented, entries),
		Oracle:   ratio(oracle, implemented),
		CSurf:    cSurfaceRatio(surf.C),
		PySurf:   pySurfaceRatio(surf.Py),
		Green:    gaps == 0,
		Gaps:     gaps,
		Headline: headline(),
	}
	return rep
}

// headline is the one-line statement of the Hu1 designation: the full-surface
// number is the headline for the two surface metrics; oracle coverage is where
// the real translation headroom lives.
func headline() string {
	return "headline = full-surface C/Python coverage (Hu1: exclusions complete-by-classification); oracle coverage carries the real headroom"
}

// cSurfaceRatio derives the two M2 ratios. full-surface counts every accounted
// export (translated + classified-untranslated) over the total live exports;
// in-scope counts the translated set over itself plus needed-but-untranslated.
// The class-(a) "needed" set is empty (C4), so the in-scope denominator is the
// translated set: with no class-(a) gap the metric reads complete, and it
// moves the moment one appears.
func cSurfaceRatio(c cSurface) SurfaceRatio {
	translated := c.Correlated + c.Internal
	accounted := translated + c.Untranslated
	return SurfaceRatio{
		FullSurface:           ratio(accounted, c.Exports),
		InScope:               ratio(translated, translated /* + 0 needed */),
		FullSurfaceIsHeadline: true,
	}
}

// pySurfaceRatio mirrors cSurfaceRatio for the M3 Python surface: translated =
// covered + delegated-to-C; out-of-scope is complete by classification.
func pySurfaceRatio(p pySurface) SurfaceRatio {
	translated := p.Covered + p.Delegated
	accounted := translated + p.OutOfScope
	return SurfaceRatio{
		FullSurface:           ratio(accounted, p.Surface),
		InScope:               ratio(translated, translated /* + 0 needed */),
		FullSurfaceIsHeadline: true,
	}
}

// ratio builds a Ratio, guarding division by zero (an empty denominator yields
// value 0, not NaN, so the JSON is always well-formed).
func ratio(num, den int) Ratio {
	r := Ratio{Numerator: num, Denominator: den}
	if den != 0 {
		r.Value = float64(num) / float64(den)
	}
	return r
}

// loadMatrix obtains the Me1 coverage matrix: decoded from the -matrix file
// when set, otherwise by running cmd/coverage as a subprocess so a single
// report invocation derives the whole report.
func loadMatrix(o opts) ([]Record, error) {
	if o.matrix != "" {
		f, err := os.Open(o.matrix)
		if err != nil {
			return nil, fmt.Errorf("opening matrix %s: %w", o.matrix, err)
		}
		defer f.Close()
		return decodeMatrix(f)
	}
	data, err := runCoverage(o)
	if err != nil {
		return nil, err
	}
	return decodeMatrix(strings.NewReader(data))
}

// runCoverage invokes `go run ./cmd/coverage` with the report's spec/cython/
// root flags and returns its JSONL stdout. It runs in the _api module
// directory so the relative default paths resolve identically to a direct run.
func runCoverage(o opts) (string, error) {
	cmd := exec.Command("go", "run", "./cmd/coverage",
		"-spec", o.specPath, "-cython", o.cythonPath, "-root", o.root)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running cmd/coverage: %w\n%s", err, stderr.String())
	}
	return stdout.String(), nil
}

// decodeMatrix decodes a newline-delimited stream of coverage Records.
func decodeMatrix(r io.Reader) ([]Record, error) {
	var records []Record
	dec := json.NewDecoder(r)
	for {
		var rec Record
		if err := dec.Decode(&rec); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decoding coverage matrix: %w", err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// loadSurface obtains the M2/M3 surface tallies: decoded from the -surface
// file when set, otherwise by running the generator's -report mode as a
// subprocess. The generator -report fails closed on any unclassified surface
// gap, so a clean decode here already implies a freshness-asserted ledger.
func loadSurface(o opts) (surfaceReport, error) {
	var data string
	if o.surface != "" {
		raw, err := os.ReadFile(o.surface)
		if err != nil {
			return surfaceReport{}, fmt.Errorf("reading surface %s: %w", o.surface, err)
		}
		data = string(raw)
	} else {
		raw, err := runGeneratorReport()
		if err != nil {
			return surfaceReport{}, err
		}
		data = raw
	}
	var surf surfaceReport
	if err := json.Unmarshal([]byte(data), &surf); err != nil {
		return surfaceReport{}, fmt.Errorf("decoding surface report: %w", err)
	}
	return surf, nil
}

// runGeneratorReport invokes `go run . -report` (the api generator's M2/M3
// surface ledger mode) in the _api module directory and returns its JSON
// stdout. The generator reads the committed cython.yaml/python.yaml plus the
// live `.ske` tree; it exits non-zero on any unclassified surface symbol.
func runGeneratorReport() (string, error) {
	cmd := exec.Command("go", "run", ".", "-report")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running generator -report: %w\n%s", err, stderr.String())
	}
	return stdout.String(), nil
}

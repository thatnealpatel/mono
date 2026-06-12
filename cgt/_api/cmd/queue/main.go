// Command queue turns the Me1 coverage matrix into a prioritized
// oracle-parity work queue and emits oraclegen skeletons for its top groups.
//
// It consumes the coverage matrix (one JSON Record per go.yaml entry, the
// output of cmd/coverage) and keeps only the rows that are implemented but
// not yet oracle-tested — the genuine gap between a translated symbol and a
// test that pins it to the mmgroup oracle. Those rows are grouped by
// (package, receiver) and the groups are emitted in a fixed priority order:
//
//   - Lead tier: leech, reduce, swar, xsp2co1 — the packages C1 identified
//     as having no oracle coverage at all. swar leads the tier even though it
//     contributes no rows yet (nothing is implemented in it), preserving the
//     C1 ordering as a standing marker for future implementer work.
//   - mmindex joins the queue only if its naming-normalized join still shows
//     a gap. Every mmindex impl-but-untested row that survives the join is a
//     name_skew artifact (the test exists under the Aux-prefix-stripped name),
//     so a group consisting entirely of name_skew rows is dropped: it is a
//     naming divergence, not a coverage gap.
//   - The remaining partially-covered packages follow, heaviest first by the
//     count of impl-but-untested entries (ties broken by package name).
//
// Within a group the entries are ordered cheapest-first for type-recovery
// work: fully-typed entries (no recovery needed) come first, then C2
// void-return inversions (resolvable_void — a blank return whose correlated C
// return is void, mechanically fillable), then genuinely-unknown blank-typed
// entries last. Type fills themselves go through the _api generator, never
// this tool.
//
// For the top groups it emits oraclegen-style test SKELETONS into a staging
// directory (never scattered into the routed packages), one file per group,
// each adopting the Td1 shared harness (patel.codes/cgt/internal/oracle) via a
// local driver. No skeleton is committed until an implementer task claims it.
//
// By default queue runs cmd/coverage itself as a subprocess, so a single
//
//	go run -C _api ./cmd/queue
//
// derives the whole queue. Pipe a precomputed matrix in with -stdin.
//
// Usage:
//
//	go run -C _api ./cmd/queue                      # group queue (JSONL) to stdout
//	go run -C _api ./cmd/queue -entries             # flat per-entry queue (JSONL)
//	go run -C _api ./cmd/queue -stdin < matrix.jsonl
//	go run -C _api ./cmd/queue -emit /tmp/goof/oracle-staging -top 4
//	go run -C _api ./cmd/queue -emit /tmp/goof/oracle-staging -top 4 -dry-run
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"patel.codes/cgt/_api/spec"
)

func main() {
	stdin := flag.Bool("stdin", false, "read the coverage matrix JSONL from stdin instead of running cmd/coverage")
	entries := flag.Bool("entries", false, "emit the flat per-entry queue instead of the per-group queue")
	emit := flag.String("emit", "", "staging directory to write skeleton files into (one per top group)")
	top := flag.Int("top", 4, "number of leading priority groups to emit skeletons for")
	dryRun := flag.Bool("dry-run", false, "with -emit, print the files that would be written rather than writing them")
	specPath := flag.String("spec", "go.yaml", "path to go.yaml (for skeleton constructor lookup and to feed cmd/coverage)")
	cythonPath := flag.String("cython", "cython.yaml", "path to cython.yaml (passed to cmd/coverage)")
	root := flag.String("root", "..", "cgt module root (passed to cmd/coverage)")
	flag.Parse()

	if err := run(opts{
		stdin:      *stdin,
		entries:    *entries,
		emit:       *emit,
		top:        *top,
		dryRun:     *dryRun,
		specPath:   *specPath,
		cythonPath: *cythonPath,
		root:       *root,
	}, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "queue: %v\n", err)
		os.Exit(1)
	}
}

// opts carries the parsed command-line configuration.
type opts struct {
	stdin      bool
	entries    bool
	emit       string
	top        int
	dryRun     bool
	specPath   string
	cythonPath string
	root       string
}

func run(o opts, in io.Reader, out io.Writer) error {
	records, err := loadMatrix(o, in)
	if err != nil {
		return err
	}

	groups := buildGroups(records)

	if o.emit != "" {
		s, err := spec.Load(o.specPath)
		if err != nil {
			return fmt.Errorf("loading spec %s: %w", o.specPath, err)
		}
		return emitSkeletons(o, groups, s, out)
	}

	w := bufio.NewWriter(out)
	defer w.Flush()
	if o.entries {
		return emitEntryQueue(w, groups)
	}
	return emitGroupQueue(w, groups)
}

// Record mirrors the JSON schema emitted by cmd/coverage. Only the fields the
// queue consumes are decoded; the schema is the join contract between the two
// commands.
type Record struct {
	Package        string `json:"package"`
	Name           string `json:"name"`
	Recv           string `json:"recv"`
	GoPackage      string `json:"go_package"`
	Implemented    bool   `json:"implemented"`
	OracleTested   bool   `json:"oracle_tested"`
	Provenance     string `json:"provenance"`
	C              string `json:"c"`
	Py             string `json:"py"`
	BlankReturn    bool   `json:"blank_return"`
	CReturn        string `json:"c_return"`
	ResolvableVoid bool   `json:"resolvable_void"`
	NameSkew       bool   `json:"name_skew"`
}

// loadMatrix obtains the coverage matrix: decoded from stdin when -stdin is
// set, otherwise by running cmd/coverage as a subprocess so a single queue
// invocation derives the whole queue.
func loadMatrix(o opts, in io.Reader) ([]Record, error) {
	var src io.Reader
	if o.stdin {
		src = in
	} else {
		data, err := runCoverage(o)
		if err != nil {
			return nil, err
		}
		src = strings.NewReader(data)
	}
	return decodeMatrix(src)
}

// runCoverage invokes `go run ./cmd/coverage` with the queue's spec/cython/root
// flags and returns its JSONL stdout. It runs in the _api module directory so
// the relative default paths resolve identically to a direct coverage run.
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

// Group is one (package, receiver) bucket of impl-but-untested entries, with
// its priority position resolved.
type Group struct {
	Package   string  `json:"package"`
	Recv      string  `json:"recv"`       // "" for free functions
	GoPackage string  `json:"go_package"` // routed import path
	Tier      int     `json:"tier"`       // 0 = lead packages, 1 = partial packages
	Weight    int     `json:"weight"`     // count of impl-but-untested entries
	Entries   []Entry `json:"entries"`    // cheapest-first within the group
}

// Entry is one impl-but-untested go.yaml entry queued for an oracle test.
type Entry struct {
	Name           string `json:"name"`
	Py             string `json:"py"`
	C              string `json:"c"`
	BlankReturn    bool   `json:"blank_return"`
	ResolvableVoid bool   `json:"resolvable_void"`
	CReturn        string `json:"c_return"`
	TypeRecovery   string `json:"type_recovery"` // "none" | "void-inversion" | "unknown-blank"
}

// leadPackages is the C1 lead tier in its canonical order. These packages had
// no oracle coverage at all; they head the queue regardless of weight, and
// swar holds its slot even with zero rows as a standing marker.
var leadPackages = []string{"leech", "reduce", "swar", "xsp2co1"}

// buildGroups filters the matrix to impl-but-untested rows, drops mmindex
// name_skew artifacts, buckets the survivors by (package, receiver), orders
// each bucket cheapest-first, and orders the buckets by the C1 priority.
func buildGroups(records []Record) []Group {
	buckets := map[string]*Group{}
	var order []string
	for _, rec := range records {
		if !rec.Implemented || rec.OracleTested {
			continue
		}
		// mmindex's surviving gaps are all name_skew artifacts: the test
		// exists under the Aux-prefix-stripped name. Skip them so a
		// purely-artifact group never enters the queue.
		if rec.NameSkew {
			continue
		}
		key := rec.Package + "\x00" + rec.Recv
		g, ok := buckets[key]
		if !ok {
			g = &Group{
				Package:   rec.Package,
				Recv:      rec.Recv,
				GoPackage: rec.GoPackage,
				Tier:      tierOf(rec.Package),
			}
			buckets[key] = g
			order = append(order, key)
		}
		g.Entries = append(g.Entries, Entry{
			Name:           rec.Name,
			Py:             rec.Py,
			C:              rec.C,
			BlankReturn:    rec.BlankReturn,
			ResolvableVoid: rec.ResolvableVoid,
			CReturn:        rec.CReturn,
			TypeRecovery:   recoveryClass(rec),
		})
	}

	groups := make([]Group, 0, len(order))
	for _, key := range order {
		g := buckets[key]
		g.Weight = len(g.Entries)
		sortEntries(g.Entries)
		groups = append(groups, *g)
	}
	sortGroups(groups)
	return groups
}

// tierOf returns 0 for a C1 lead package and 1 otherwise.
func tierOf(pkg string) int {
	for _, p := range leadPackages {
		if p == pkg {
			return 0
		}
	}
	return 1
}

// recoveryClass classifies the type-recovery cost of filling an entry's
// return type: none when already typed, void-inversion for a C2-resolvable
// blank return, unknown-blank for a genuinely-deferred type.
func recoveryClass(rec Record) string {
	if !rec.BlankReturn {
		return "none"
	}
	if rec.ResolvableVoid {
		return "void-inversion"
	}
	return "unknown-blank"
}

// recoveryRank orders the type-recovery classes cheapest-first.
func recoveryRank(class string) int {
	switch class {
	case "none":
		return 0
	case "void-inversion":
		return 1
	default: // "unknown-blank"
		return 2
	}
}

// sortEntries orders a group's entries cheapest-first by type-recovery class,
// breaking ties by name for determinism.
func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		ri, rj := recoveryRank(entries[i].TypeRecovery), recoveryRank(entries[j].TypeRecovery)
		if ri != rj {
			return ri < rj
		}
		return entries[i].Name < entries[j].Name
	})
}

// sortGroups orders the queue: lead tier in canonical C1 order, then partial
// packages heaviest-first; within a package, receiver-less funcs before
// methods, then by receiver name; both broken deterministically.
func sortGroups(groups []Group) {
	leadRank := map[string]int{}
	for i, p := range leadPackages {
		leadRank[p] = i
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}
		if a.Tier == 0 {
			if a.Package != b.Package {
				return leadRank[a.Package] < leadRank[b.Package]
			}
		} else {
			if a.Weight != b.Weight {
				return a.Weight > b.Weight
			}
			if a.Package != b.Package {
				return a.Package < b.Package
			}
		}
		// Same package: free functions before methods, then by receiver.
		return a.Recv < b.Recv
	})
}

// emitGroupQueue writes one JSON object per group in priority order.
func emitGroupQueue(w *bufio.Writer, groups []Group) error {
	enc := json.NewEncoder(w)
	for _, g := range groups {
		if err := enc.Encode(g); err != nil {
			return err
		}
	}
	return nil
}

// queueEntry is a flattened per-entry queue row carrying its group's priority
// position, for downstream consumers that want a single stream.
type queueEntry struct {
	Priority     int    `json:"priority"` // global rank across the whole queue
	Package      string `json:"package"`
	Recv         string `json:"recv"`
	GoPackage    string `json:"go_package"`
	Tier         int    `json:"tier"`
	Name         string `json:"name"`
	Py           string `json:"py"`
	C            string `json:"c"`
	TypeRecovery string `json:"type_recovery"`
}

// emitEntryQueue writes the groups flattened into a single priority-ordered
// per-entry stream, assigning each entry a global priority index.
func emitEntryQueue(w *bufio.Writer, groups []Group) error {
	enc := json.NewEncoder(w)
	priority := 0
	for _, g := range groups {
		for _, e := range g.Entries {
			if err := enc.Encode(queueEntry{
				Priority:     priority,
				Package:      g.Package,
				Recv:         g.Recv,
				GoPackage:    g.GoPackage,
				Tier:         g.Tier,
				Name:         e.Name,
				Py:           e.Py,
				C:            e.C,
				TypeRecovery: e.TypeRecovery,
			}); err != nil {
				return err
			}
			priority++
		}
	}
	return nil
}

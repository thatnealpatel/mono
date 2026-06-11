// Command specinfo queries the crystallized go.yaml API spec.
//
// go.yaml is the unified plan for the Go side of the mmgroup translation
// (one entry per planned function/method, grouped by target cgt package).
// specinfo parses it and answers structured questions about the surface:
// which entries target a package or receiver, which still carry blank
// (deferred) return types, which have value/type dispatch ladders, and the
// full detail of any single named entry.
//
// Usage:
//
//	go run -C _api ./cmd/specinfo recv MM        # entries with receiver MM
//	go run -C _api ./cmd/specinfo pkg generator  # entries in package generator
//	go run -C _api ./cmd/specinfo name NewMMFromTag
//	go run -C _api ./cmd/specinfo blanks         # entries with blank return:
//	go run -C _api ./cmd/specinfo dispatch       # entries with a dispatch block
//	go run -C _api ./cmd/specinfo packages       # list package names + counts
//
// The spec path defaults to ../go.yaml relative to the command (i.e.
// _api/go.yaml); override with -spec.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"patel.codes/cgt/_api/spec"
)

func main() {
	specPath := flag.String("spec", "go.yaml", "path to the go.yaml spec")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	s, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specinfo: loading %s: %v\n", *specPath, err)
		os.Exit(1)
	}

	cmd := args[0]
	rest := args[1:]
	if err := dispatch(s, cmd, rest); err != nil {
		fmt.Fprintf(os.Stderr, "specinfo: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `specinfo — query the go.yaml mmgroup API spec

usage: specinfo [-spec path] <query> [arg]

queries:
  recv <Receiver>   list entries whose receiver is <Receiver>
  pkg <package>     list entries in package <package>
  name <Name>       show full detail for the entry named <Name>
  blanks            list entries with a blank (deferred) return type
  dispatch          list entries that carry a dispatch: ladder
  packages          list package names with entry counts
`)
}

func dispatch(s *spec.Spec, cmd string, rest []string) error {
	switch cmd {
	case "recv":
		if len(rest) != 1 {
			return fmt.Errorf("recv needs exactly one receiver argument")
		}
		return queryRecv(s, rest[0])
	case "pkg":
		if len(rest) != 1 {
			return fmt.Errorf("pkg needs exactly one package argument")
		}
		return queryPkg(s, rest[0])
	case "name":
		if len(rest) != 1 {
			return fmt.Errorf("name needs exactly one name argument")
		}
		return queryName(s, rest[0])
	case "blanks":
		return queryBlanks(s)
	case "dispatch":
		return queryDispatch(s)
	case "packages":
		return queryPackages(s)
	default:
		return fmt.Errorf("unknown query %q (try: recv pkg name blanks dispatch packages)", cmd)
	}
}

func queryRecv(s *spec.Spec, recv string) error {
	var hits []spec.Func
	for _, fn := range s.AllFuncs() {
		if fn.Recv == recv {
			hits = append(hits, fn)
		}
	}
	if len(hits) == 0 {
		return fmt.Errorf("no entries with receiver %q", recv)
	}
	fmt.Printf("# %d entries with receiver %s\n", len(hits), recv)
	for _, fn := range hits {
		printSummary(fn)
	}
	return nil
}

func queryPkg(s *spec.Spec, pkg string) error {
	for _, p := range s.Packages {
		if p.Name == pkg {
			fmt.Printf("# package %s: %d entries\n", pkg, len(p.Funcs))
			for _, fn := range p.Funcs {
				printSummary(fn)
			}
			return nil
		}
	}
	return fmt.Errorf("no package %q (try `specinfo packages`)", pkg)
}

func queryName(s *spec.Spec, name string) error {
	var found bool
	for _, fn := range s.AllFuncs() {
		if fn.Name == name {
			printDetail(fn)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no entry named %q", name)
	}
	return nil
}

func queryBlanks(s *spec.Spec) error {
	var n int
	for _, fn := range s.AllFuncs() {
		if fn.Return == "" {
			printSummary(fn)
			n++
		}
	}
	fmt.Printf("# %d entries with blank return type\n", n)
	return nil
}

func queryDispatch(s *spec.Spec) error {
	var n int
	for _, fn := range s.AllFuncs() {
		if len(fn.Dispatch) > 0 {
			fmt.Printf("%s\n", qualify(fn))
			for _, d := range fn.Dispatch {
				on := d.On
				if on == "" {
					on = "(deferred)"
				}
				fmt.Printf("    on %-12s -> %s\n", on, d.Returns)
			}
			n++
		}
	}
	fmt.Printf("# %d entries with a dispatch ladder\n", n)
	return nil
}

func queryPackages(s *spec.Spec) error {
	names := make([]string, 0, len(s.Packages))
	counts := make(map[string]int, len(s.Packages))
	for _, p := range s.Packages {
		names = append(names, p.Name)
		counts[p.Name] = len(p.Funcs)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%-12s %d\n", name, counts[name])
	}
	if len(s.Constants) > 0 {
		fmt.Printf("%-12s %d (constants)\n", "(constants)", len(s.Constants))
	}
	return nil
}

// qualify renders the package-qualified, receiver-aware identity of an entry,
// e.g. "mm.(*MM).Order" or "generator.Perm".
func qualify(fn spec.Func) string {
	if fn.Recv != "" {
		return fmt.Sprintf("%s.(%s).%s", fn.Pkg, fn.Recv, fn.Name)
	}
	return fmt.Sprintf("%s.%s", fn.Pkg, fn.Name)
}

func printSummary(fn spec.Func) {
	ret := fn.Return
	if ret == "" {
		ret = "(blank)"
	}
	var sig []string
	for _, p := range fn.Params {
		t := p.Type
		if t == "" {
			t = "?"
		}
		sig = append(sig, p.Name+" "+t)
	}
	fmt.Printf("  %-28s (%s) %s", fn.Name, strings.Join(sig, ", "), ret)
	if fn.Py != "" {
		fmt.Printf("  [py:%s]", fn.Py)
	}
	fmt.Println()
}

func printDetail(fn spec.Func) {
	fmt.Printf("name:    %s\n", fn.Name)
	fmt.Printf("package: %s\n", fn.Pkg)
	if fn.Recv != "" {
		fmt.Printf("recv:    %s\n", fn.Recv)
	}
	if fn.C != "" {
		fmt.Printf("c:       %s\n", fn.C)
	}
	if fn.Py != "" {
		fmt.Printf("py:      %s\n", fn.Py)
	}
	ret := fn.Return
	if ret == "" {
		ret = "(blank)"
	}
	fmt.Printf("return:  %s\n", ret)
	if fn.Note != "" {
		fmt.Printf("note:    %s\n", fn.Note)
	}
	if len(fn.Params) == 0 {
		fmt.Printf("params:  (none)\n")
	} else {
		fmt.Printf("params:\n")
		for _, p := range fn.Params {
			t := p.Type
			if t == "" {
				t = "(deferred)"
			}
			line := fmt.Sprintf("  - %s %s", p.Name, t)
			if p.Default != "" {
				line += " = " + p.Default
			}
			fmt.Println(line)
		}
	}
	if len(fn.Calls) > 0 {
		fmt.Printf("calls:   %s\n", strings.Join(fn.Calls, ", "))
	}
	if len(fn.Dispatch) > 0 {
		fmt.Printf("dispatch:\n")
		for _, d := range fn.Dispatch {
			on := d.On
			if on == "" {
				on = "(deferred)"
			}
			fmt.Printf("  - on %-12s -> %s\n", on, d.Returns)
		}
	}
}

// Package main implements leandoc, an exact-name
// and BM25 search tool over the Lean sources of the
// enclosing project's dependencies and toolchain.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"patel.codes/indexing"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "-help" {
		usage()
	}

	var verbose bool
	args := os.Args[1:]
	if args[0] == "-v" {
		verbose = true
		args = args[1:]
		if len(args) == 0 {
			usage()
		}
	}
	query := strings.Join(args, " ")

	root, err := projectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "leandoc: %v\n", err)
		os.Exit(1)
	}
	ix, err := openIndex(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leandoc: index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	if err := run(os.Stdout, ix, root, query, verbose); err != nil {
		fmt.Fprintf(os.Stderr, "leandoc: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: leandoc [-v] <query...>

Exact-name lookup and BM25 search over the Lean
sources in .lake/packages/ and the toolchain of the
enclosing project, as a JSON envelope.

A single whitespace-free token is an exact lookup.
Anything else is a ranked search. Search results are
never truncated; pipe through jq.

examples:
	leandoc Nat.add_comm
	leandoc List.map
	leandoc group homomorphism

flags:
  -v   include BM25 score in output

environment:
  LEANDOC_ROOT        project root (default: nearest parent with lean-toolchain)
  LEANDOC_CACHE_DIR   index cache location (default: ~/.cache/leandoc)
`)
	os.Exit(0)
}

// openIndex opens the index for the project, building
// it first if this toolchain and manifest have not been
// indexed before.
func openIndex(root string) (*indexing.Index, error) {
	fp, err := indexing.Fingerprint(
		filepath.Join(root, "lean-toolchain"),
		filepath.Join(root, "lake-manifest.json"),
	)
	if err != nil {
		return nil, err
	}
	cache, err := cacheDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(cache, fp+".db")
	if _, err := os.Stat(path); err != nil {
		if err := buildIndex(root, path); err != nil {
			return nil, err
		}
	}
	return indexing.Open(path, LeanTokenizer{})
}

func buildIndex(root, path string) error {
	pkgs := packagesDir(root)
	tcDir, err := toolchainDir(root)
	if err != nil {
		return err
	}
	tcSrc := filepath.Join(tcDir, "src", "lean")

	var decls []Declaration
	decls, err = collect(pkgs, pkgs, decls)
	if err != nil {
		return err
	}
	for _, sub := range []string{"Init", "Std"} {
		decls, err = collect(filepath.Join(tcSrc, sub), tcSrc, decls)
		if err != nil {
			return err
		}
	}

	names, err := walkIleanNames(pkgs)
	if err != nil {
		return err
	}
	tcNames, err := walkIleanNames(filepath.Join(tcDir, "lib", "lean"))
	if err != nil {
		return err
	}
	for name, mod := range tcNames {
		if _, ok := names[name]; !ok {
			names[name] = mod
		}
	}

	b, err := indexing.Create(path, LeanTokenizer{})
	if err != nil {
		return err
	}
	indexed := make(map[string]bool, len(decls))
	for _, d := range decls {
		indexed[d.Name] = true
		err := b.Add(indexing.Record{
			Name:      d.Name,
			Kind:      d.Kind,
			Signature: d.Signature,
			Docstring: d.Docstring,
			File:      d.File,
			Line:      d.Line,
		})
		if err != nil {
			return err
		}
	}
	generated := 0
	for name, mod := range names {
		if indexed[name] {
			continue
		}
		if err := b.AddName(name, moduleToFile(mod)); err != nil {
			return err
		}
		generated++
	}
	if err := b.Close(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "leandoc: indexed %d declarations, %d generated names\n", len(decls), generated)
	return nil
}

// collect extracts declarations from every .lean file under dir, recording
// paths relative to base.
func collect(dir, base string, decls []Declaration) ([]Declaration, error) {
	return decls, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".lean") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		decls = append(decls, ExtractFile(rel, data)...)
		return nil
	})
}

func moduleToFile(mod string) string {
	return strings.ReplaceAll(mod, ".", "/") + ".lean"
}

func isIdentQuery(q string) bool {
	return !strings.ContainsAny(q, " \t\n\r")
}

// run answers query and writes the envelope to w. Exact hits from source carry
// the declaration body.
func run(w io.Writer, ix *indexing.Index, root, query string, verbose bool) error {
	var env *indexing.Envelope
	var err error
	if isIdentQuery(query) {
		env, err = ix.Lookup(query)
		if err != nil {
			return err
		}
		for i := range env.Matches {
			m := &env.Matches[i]
			if m.Line == 0 {
				continue
			}
			abs, err := resolveFile(root, m.File)
			if err != nil {
				continue
			}
			if body, err := extractBody(abs, m.Line); err == nil {
				m.Body = body
			}
		}
	} else {
		env, err = ix.Search(query)
		if err != nil {
			return err
		}
		if !verbose {
			for i := range env.Matches {
				env.Matches[i].Score = 0
			}
		}
	}
	return json.NewEncoder(w).Encode(env)
}

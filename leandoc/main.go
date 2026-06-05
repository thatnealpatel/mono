// Package main implements a fragile but
// extremely fast client for performing
// vector searches over a hundreds of
// thousands Lean source files in a given
// LEANDOC_DOT_LAKE directory.
package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"patel.codes/ranking"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	if os.Args[1] == "-h" || os.Args[1] == "-help" {
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

	if err := ensureIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "index: %v\n", err)
		os.Exit(1)
	}

	if err := search(query, verbose); err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: leandoc [-v] <query...>

BM25-ranked search over Lean 4 source code
found in $LEANDOC_DOT_LAKE/packages/ returned
as relevance sorted JSON arrays.

Results are never truncated; callers are expected
to pipe through jq or other filtering tools.

examples:
	leandoc Nat.add_comm
	leandoc List.map
	leandoc group homomorphism

flags:
  -v   include BM25 score in output

environment:
  LEANDOC_DOT_LAKE    path to .lake directory (required)
  LEANDOC_CACHE_DIR   index cache location (default: ~/.cache/leandoc)
`)
	os.Exit(0)
}

func srcDir() (string, error) {
	dotLake := os.Getenv("LEANDOC_DOT_LAKE")
	if dotLake == "" {
		return "", fmt.Errorf("LEANDOC_DOT_LAKE is not set")
	}
	return filepath.Join(dotLake, "packages"), nil
}

func cacheDir() (string, error) {
	dir := os.Getenv("LEANDOC_CACHE_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "leandoc")
	}
	return dir, os.MkdirAll(dir, 0o755)
}

func ensureIndex() error {
	cache, err := cacheDir()
	if err != nil {
		return err
	}

	bm25Path := filepath.Join(cache, "bm25.gob")
	info, err := os.Stat(bm25Path)
	if err == nil {
		src, err := srcDir()
		if err != nil {
			return err
		}
		srcInfo, err := os.Stat(src)
		if err != nil {
			return err
		}
		if info.ModTime().After(srcInfo.ModTime()) {
			return nil
		}
	}

	return buildIndex()
}

func buildIndex() error {
	src, err := srcDir()
	if err != nil {
		return err
	}

	var decls []Declaration
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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
		rel, err := filepath.Rel(src, path)
		if err != nil {
			rel = path
		}
		fileDecls := ExtractFile(rel, data)
		decls = append(decls, fileDecls...)
		return nil
	})
	if err != nil {
		return err
	}

	docs := make([]string, len(decls))
	for i := range decls {
		docs[i] = decls[i].DocText()
	}

	tok := LeanTokenizer{}
	bm := ranking.NewBM25(&ranking.BM25Params{Tokenizer: tok})
	bm.Build(docs)

	cache, err := cacheDir()
	if err != nil {
		return err
	}

	declPath := filepath.Join(cache, "decls.gob")
	bm25Path := filepath.Join(cache, "bm25.gob")

	f, err := os.Create(declPath)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(decls); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	f, err = os.Create(bm25Path)
	if err != nil {
		return err
	}
	if _, err := bm.WriteTo(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "indexed %d declarations\n", len(decls))
	return nil
}

type SearchResult struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Signature string  `json:"signature,omitempty"`
	Docstring string  `json:"docstring,omitempty"`
	File      string  `json:"file"`
	Line      int     `json:"line"`
	Score     float64 `json:"score,omitempty"`
}

func search(query string, verbose bool) error {
	cache, err := cacheDir()
	if err != nil {
		return err
	}

	declPath := filepath.Join(cache, "decls.gob")
	bm25Path := filepath.Join(cache, "bm25.gob")

	var decls []Declaration
	f, err := os.Open(declPath)
	if err != nil {
		return err
	}
	if err := gob.NewDecoder(f).Decode(&decls); err != nil {
		f.Close()
		return err
	}
	f.Close()

	tok := LeanTokenizer{}
	bm := ranking.NewBM25(&ranking.BM25Params{Tokenizer: tok})
	f, err = os.Open(bm25Path)
	if err != nil {
		return err
	}
	if _, err := bm.ReadFrom(f); err != nil {
		f.Close()
		return err
	}
	f.Close()

	results := bm.Search(query)

	out := make([]SearchResult, len(results))
	for i, r := range results {
		d := &decls[r.Index]
		out[i] = SearchResult{
			Name:      d.Name,
			Kind:      d.Kind,
			Signature: d.Signature,
			Docstring: d.Docstring,
			File:      d.File,
			Line:      d.Line,
			Score:     r.Score,
		}
	}

	if !verbose {
		for i := range out {
			out[i].Score = 0
		}
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

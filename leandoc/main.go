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
	bm := ranking.NewBM25(&ranking.BM25Params{K1: 1.2, B: 0.75, Tokenizer: tok})
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

	names, err := walkIleanNames(src)
	if err != nil {
		return err
	}

	namesPath := filepath.Join(cache, "names.gob")
	f, err = os.Create(namesPath)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(names); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "indexed %d declarations, %d compiled names\n", len(decls), len(names))
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

type Envelope struct {
	Mode       string         `json:"mode"`
	Matches    []SearchResult `json:"matches"`
	Candidates []string       `json:"candidates,omitempty"`
}

func isIdentQuery(q string) bool {
	return !strings.ContainsAny(q, " \t\n\r")
}

func buildNameIndex(decls []Declaration) map[string][]int {
	idx := make(map[string][]int)
	for i := range decls {
		idx[decls[i].Name] = append(idx[decls[i].Name], i)
	}
	return idx
}

// finalComponent returns the last dot-separated segment of a qualified name.
func finalComponent(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func moduleToFile(mod string) string {
	return strings.ReplaceAll(mod, ".", "/") + ".lean"
}

func findCandidates(query string, decls []Declaration, compiledNames map[string]string) []string {
	target := strings.ToLower(finalComponent(query))
	seen := make(map[string]bool)
	var out []string
	for i := range decls {
		if strings.EqualFold(finalComponent(decls[i].Name), target) {
			if !seen[decls[i].Name] {
				seen[decls[i].Name] = true
				out = append(out, decls[i].Name)
			}
		}
		if len(out) >= 10 {
			return out
		}
	}
	for name := range compiledNames {
		if len(out) >= 10 {
			break
		}
		if strings.EqualFold(finalComponent(name), target) && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func loadCompiledNames(cache string) (map[string]string, error) {
	namesPath := filepath.Join(cache, "names.gob")
	f, err := os.Open(namesPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var names map[string]string
	if err := gob.NewDecoder(f).Decode(&names); err != nil {
		return nil, err
	}
	return names, nil
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

	if isIdentQuery(query) {
		return searchIdent(query, decls, cache)
	}

	tok := LeanTokenizer{}
	bm := ranking.NewBM25(&ranking.BM25Params{K1: 1.2, B: 0.75, Tokenizer: tok})
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
	matches := make([]SearchResult, len(results))
	for i, r := range results {
		d := &decls[r.Index]
		matches[i] = SearchResult{
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
		for i := range matches {
			matches[i].Score = 0
		}
	}

	env := Envelope{Mode: "search", Matches: matches}
	return json.NewEncoder(os.Stdout).Encode(env)
}

func searchIdent(query string, decls []Declaration, cache string) error {
	nameIdx := buildNameIndex(decls)

	if indices, ok := nameIdx[query]; ok {
		matches := make([]SearchResult, len(indices))
		for i, idx := range indices {
			d := &decls[idx]
			matches[i] = SearchResult{
				Name:      d.Name,
				Kind:      d.Kind,
				Signature: d.Signature,
				Docstring: d.Docstring,
				File:      d.File,
				Line:      d.Line,
			}
		}
		env := Envelope{Mode: "exact", Matches: matches}
		return json.NewEncoder(os.Stdout).Encode(env)
	}

	compiledNames, err := loadCompiledNames(cache)
	if err != nil {
		return err
	}

	if mod, ok := compiledNames[query]; ok {
		matches := []SearchResult{{
			Name: query,
			Kind: "generated",
			File: moduleToFile(mod),
		}}
		env := Envelope{Mode: "exact", Matches: matches}
		return json.NewEncoder(os.Stdout).Encode(env)
	}

	candidates := findCandidates(query, decls, compiledNames)
	env := Envelope{Mode: "miss", Matches: []SearchResult{}, Candidates: candidates}
	return json.NewEncoder(os.Stdout).Encode(env)
}

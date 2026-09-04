// Command sagedoc provides exact-name lookup and BM25 search over the
// documented public namespace exported by sage.all.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode"

	"patel.codes/indexing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func cli(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		usage(stderr)
		return 0
	}

	verbose := false
	if args[0] == "-v" {
		verbose = true
		args = args[1:]
		if len(args) == 0 {
			usage(stderr)
			return 0
		}
	}
	query := strings.Join(args, " ")

	cacheRoot, err := defaultCacheRoot()
	if err != nil {
		fmt.Fprintf(stderr, "sagedoc: cache: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "sagedoc: executable: %v\n", err)
		return 1
	}
	index, err := openIndex(ctx, indexOptions{
		ConfiguredPython: os.Getenv("SAGEDOC_PYTHON"),
		CacheRoot:        cacheRoot,
		Executable:       executable,
		Diagnostics:      stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sagedoc: index: %v\n", err)
		return 1
	}

	if err := runQuery(stdout, index, query, verbose); err != nil {
		_ = index.Close()
		fmt.Fprintf(stderr, "sagedoc: query: %v\n", err)
		return 1
	}
	if err := index.Close(); err != nil {
		fmt.Fprintf(stderr, "sagedoc: close index: %v\n", err)
		return 1
	}
	return 0
}

func usage(writer io.Writer) {
	fmt.Fprint(writer, `usage: sagedoc [-v] <query...>

Exact-name lookup and BM25 search over the documented public namespace
exported by sage.all, returned as a JSON envelope.

A single whitespace-free query is an exact, case-sensitive lookup.
Anything containing whitespace is a ranked prose search.

examples:
	sagedoc GF
	sagedoc factor
	sagedoc finite field construction

flags:
  -v   include BM25 score in search output

environment:
  SAGEDOC_PYTHON      required Sage Python interpreter
  SAGEDOC_CACHE_DIR   index cache (default: ~/.cache/sagedoc)
`)
}

func isExactQuery(query string) bool {
	return strings.IndexFunc(query, unicode.IsSpace) < 0
}

func runQuery(writer io.Writer, index *indexing.Index, query string, verbose bool) error {
	var envelope *indexing.Envelope
	var err error
	if isExactQuery(query) {
		envelope, err = index.Lookup(query)
	} else {
		envelope, err = index.Search(query)
		if !verbose && envelope != nil {
			for i := range envelope.Matches {
				envelope.Matches[i].Score = 0
			}
		}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(writer).Encode(envelope)
}

// Package main implements a wrapper
// around the OEIS database and ouputs
// all results to JSON.
//
// It integrates a cacheless vector
// search using patel.codes/ranking
// for natural language query.
//
// Upstream is polled for updates at
// most once per day.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"patel.codes/ranking"
)

func main() {
	cacheDir = os.Getenv("OEIS_CACHE_DIR")
	if cacheDir == "" {
		log.Fatal("OEIS_CACHE_DIR is not set")
	}
	oeisDir = filepath.Join(cacheDir, "oeisdata.git", "seq")

	if err := ensureRepo(); err != nil {
		log.Fatal(err)
	}

	var err error
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "show":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = oeisShow(os.Args[2])
	case "search":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = oeisSearch(strings.Join(os.Args[2:], " "))
	case "match":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = oeisMatch(strings.Join(os.Args[2:], " "))
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "goof-oeis: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: oeis <command> [args]

  show <AXXXXXX>     sequence entry as JSON
  search <query>     search sequence names
  match <1,2,3,...>  find sequences containing terms
`

func oeisShow(id string) error {
	path := oeisSeqPath(id)
	if path == "" {
		return fmt.Errorf("invalid sequence ID: %q", id)
	}
	e, err := oeisParseFile(path)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(e)
}

func oeisSeqPath(id string) string {
	id = strings.ToUpper(id)
	if len(id) < 4 {
		return ""
	}
	return filepath.Join(oeisDir, id[:4], id+".seq")
}

var (
	cacheDir string
	oeisDir  string
)

const oeisRemote = "git@github.com:oeis/oeisdata.git"

func ensureRepo() error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	dir := filepath.Join(cacheDir, "oeisdata.git")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return ensureFresh(dir)
	}
	cmd := exec.Command("git", "clone", oeisRemote, dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning oeisdata: %w", err)
	}
	return nil
}

func ensureFresh(dir string) error {
	marker := filepath.Join(cacheDir, "fetched")
	if b, err := os.ReadFile(marker); err == nil {
		var ts int64
		if _, err := fmt.Sscanf(string(b), "%d", &ts); err == nil {
			if time.Since(time.Unix(ts, 0)) < 24*time.Hour {
				return nil
			}
		}
	}
	fetch := exec.Command("git", "-C", dir, "fetch")
	fetch.Stderr = os.Stderr
	if err := fetch.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "oeis: fetch failed, using cached data")
		return nil
	}
	os.WriteFile(marker, fmt.Appendf(nil, "%d\n", time.Now().Unix()), 0o644)
	local, err := gitRev(dir, "HEAD")
	if err != nil {
		return nil
	}
	remote, err := gitRev(dir, "@{u}")
	if err != nil {
		return nil
	}
	if local == remote {
		return nil
	}
	fmt.Fprintf(os.Stderr, "oeis: updating %s..%s ... ", local[:8], remote[:8])
	pull := exec.Command("git", "-C", dir, "pull", "--ff-only")
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "update failed, using cached data")
		return nil
	}
	fmt.Fprintln(os.Stderr, "done")
	return nil
}

func gitRev(dir, ref string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func oeisParseFile(path string) (oeisEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return oeisEntry{}, err
	}
	defer f.Close()

	var e oeisEntry
	var terms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 4 {
			continue
		}
		tag := line[:2]
		body := line
		if sp := strings.IndexByte(line[3:], ' '); sp >= 0 {
			body = line[3+sp+1:]
		}
		switch tag {
		case "%I":
			parts := strings.Fields(line[3:])
			if len(parts) > 0 {
				e.ID = parts[0]
			}
		case "%N":
			e.Name = body
		case "%S", "%T", "%U":
			terms = append(terms, body)
		case "%C":
			e.Comments = append(e.Comments, body)
		case "%F":
			e.Formulas = append(e.Formulas, body)
		case "%Y":
			e.Xrefs = append(e.Xrefs, body)
		case "%K":
			e.Keywords = body
		case "%o", "%p", "%t":
			e.Programs = append(e.Programs, body)
		}
	}
	e.Terms = strings.Join(terms, "")
	return e, sc.Err()
}

type oeisEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Terms    string   `json:"terms"`
	Comments []string `json:"comments,omitempty"`
	Formulas []string `json:"formulas,omitempty"`
	Xrefs    []string `json:"xrefs,omitempty"`
	Keywords string   `json:"keywords,omitempty"`
	Programs []string `json:"programs,omitempty"`
}

func oeisSearch(query string) error {
	var ids []string
	var names []string
	if err := oeisWalk(func(id, _ string, entry oeisQuick) {
		ids = append(ids, id)
		names = append(names, entry.Name)
	}); err != nil {
		return err
	}

	bm := ranking.NewBM25(nil)
	bm.Build(names)
	results := bm.Search(query)

	enc := json.NewEncoder(os.Stdout)
	for _, r := range results {
		if err := enc.Encode(struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Score float64 `json:"score"`
		}{ids[r.Index], names[r.Index], r.Score}); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "%d results\n", len(results))
	return nil
}

func oeisWalk(fn func(id, path string, q oeisQuick)) error {
	dirs, err := os.ReadDir(oeisDir)
	if err != nil {
		return err
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			defer func() { <-sem }()

			files, _ := os.ReadDir(filepath.Join(oeisDir, dir))
			for _, f := range files {
				if !strings.HasSuffix(f.Name(), ".seq") {
					continue
				}
				path := filepath.Join(oeisDir, dir, f.Name())
				id := strings.TrimSuffix(f.Name(), ".seq")
				q := oeisQuickParse(path)
				mu.Lock()
				fn(id, path, q)
				mu.Unlock()
			}
		}(d.Name())
	}
	wg.Wait()
	return nil
}

func oeisQuickParse(path string) oeisQuick {
	f, err := os.Open(path)
	if err != nil {
		return oeisQuick{}
	}
	defer f.Close()

	var q oeisQuick
	var terms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 4 {
			continue
		}
		tag := line[:2]
		body := line
		if sp := strings.IndexByte(line[3:], ' '); sp >= 0 {
			body = line[3+sp+1:]
		}
		switch tag {
		case "%N":
			q.Name = body
		case "%S", "%T", "%U":
			terms = append(terms, body)
		}
	}
	q.Terms = strings.Join(terms, "")
	return q
}

type oeisQuick struct {
	Name  string
	Terms string
}

func oeisMatch(terms string) error {
	terms = strings.TrimSpace(terms)
	if !strings.HasSuffix(terms, ",") {
		terms += ","
	}

	type result struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Terms string `json:"terms"`
	}
	var results []result

	if err := oeisWalk(func(id, _ string, entry oeisQuick) {
		if strings.Contains(entry.Terms, terms) {
			results = append(results, result{id, entry.Name, entry.Terms})
		}
	}); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	for _, r := range results {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "%d results\n", len(results))
	return nil
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"patel.codes/ranking"
)

func main() {
	log.SetFlags(0)

	cveCacheDir = os.Getenv("CVE_CACHE_DIR")
	if cveCacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			log.Fatal(err)
		}
		cveCacheDir = filepath.Join(base, "cve")
	}

	if err := ensureRepo(); err != nil {
		fmt.Fprintf(os.Stderr, "cve: %v\n", err)
		os.Exit(1)
	}

	var err error
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "search":
		fs := flag.NewFlagSet("search", flag.ExitOnError)
		year := fs.String("year", "", "filter by year")
		cna := fs.String("cna", "", "filter by CNA short name")
		state := fs.String("state", "", "filter by state (PUBLISHED, REJECTED)")
		n := fs.Int("n", 20, "max results")
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stdout, "usage: cves search [-year Y] [-cna C] [-state S] [-n N] <query>")
			os.Exit(1)
		}
		query := strings.Join(fs.Args(), " ")
		err = cmdSearch(query, *year, *cna, *state, *n)
	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stdout, "usage: cves get <CVE-ID>")
			os.Exit(1)
		}
		err = cmdGet(os.Args[2])
	case "stats":
		err = cmdStats()
	case "-h":
		fmt.Fprint(os.Stdout, usage)
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	if err != nil {
		log.Fatal(err)
	}
}

const cveRemote = "https://github.com/CVEProject/cvelistV5.git"

var (
	cveCacheDir string
	cveRepoDir  string
	usage       = `usage: cves <command> [args]

  search <query>        BM25 ranked search over title+description
  get <CVE-ID>          lookup a single record
  stats                 corpus stats

search flags:
  -year  <YYYY>         filter by CVE year
  -cna   <name>         filter by assigner (case-insensitive)
  -state <STATE>        filter by state (PUBLISHED, REJECTED)
  -n     <N>            max results (default 20)
`
)

type Record struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Year  string `json:"year"`
	CNA   string `json:"cna"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

func ensureRepo() error {
	if err := os.MkdirAll(cveCacheDir, 0o755); err != nil {
		return err
	}
	cveRepoDir = filepath.Join(cveCacheDir, "cvelistV5.git")
	if _, err := os.Stat(filepath.Join(cveRepoDir, ".git")); err == nil {
		return ensureFresh(cveRepoDir)
	}
	cmd := exec.Command("git", "clone", "--depth", "1", cveRemote, cveRepoDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning cvelistV5: %w", err)
	}
	return nil
}

func ensureFresh(dir string) error {
	marker := filepath.Join(cveCacheDir, "fetched")
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
		fmt.Fprintln(os.Stderr, "cve: fetch failed, using cached data")
		return nil
	}
	if err := os.WriteFile(marker, fmt.Appendf(nil, "%d\n", time.Now().Unix()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cve: writing marker: %v\n", err)
	}
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
	fmt.Fprintf(os.Stderr, "cve: updating %s..%s ... ", local[:8], remote[:8])
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

func parseAll() ([]Record, error) {
	procs := runtime.GOMAXPROCS(0)
	cveDir := filepath.Join(cveRepoDir, "cves")

	var paths []string
	if err := filepath.WalkDir(cveDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") && !strings.Contains(d.Name(), "delta") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	type result struct {
		rec Record
		err error
	}
	results := make([]result, len(paths))

	sem := make(chan struct{}, procs)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			rec, err := parseFile(path)
			results[i] = result{rec, err}
		}(i, p)
	}
	wg.Wait()

	var records []Record
	var parseErrors int
	for _, r := range results {
		if r.err != nil {
			parseErrors++
			continue
		}
		records = append(records, r.rec)
	}
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "skipped %d files with parse errors\n", parseErrors)
	}
	return records, nil
}

func parseFile(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var raw struct {
		CveMetadata struct {
			CveId             string `json:"cveId"`
			State             string `json:"state"`
			AssignerShortName string `json:"assignerShortName"`
		} `json:"cveMetadata"`
		Containers struct {
			CNA struct {
				Title        string `json:"title"`
				Descriptions []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"descriptions"`
			} `json:"cna"`
		} `json:"containers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Record{}, err
	}

	rec := Record{
		ID:    raw.CveMetadata.CveId,
		State: raw.CveMetadata.State,
		CNA:   raw.CveMetadata.AssignerShortName,
		Title: raw.Containers.CNA.Title,
	}

	parts := strings.SplitN(rec.ID, "-", 3)
	if len(parts) >= 2 {
		rec.Year = parts[1]
	}

	for _, d := range raw.Containers.CNA.Descriptions {
		if strings.HasPrefix(d.Lang, "en") {
			rec.Desc = stripHTML(d.Value)
			break
		}
	}

	return rec, nil
}

func stripHTML(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "br", "p", "div", "li", "tr":
				b.WriteByte(' ')
			}
		}
	}
}

func cmdSearch(query, year, cna, state string, n int) error {
	records, err := parseAll()
	if err != nil {
		return err
	}

	docs := make([]string, len(records))
	for i, r := range records {
		docs[i] = r.Title + " " + r.Desc
	}

	idx := ranking.NewBM25(nil)
	idx.Build(docs)
	results := idx.Search(query)

	type match struct {
		Record Record  `json:"record"`
		Score  float64 `json:"score"`
	}
	var matches []match
	for _, r := range results {
		rec := records[r.Index]
		if year != "" && rec.Year != year {
			continue
		}
		if cna != "" && !strings.EqualFold(rec.CNA, cna) {
			continue
		}
		if state != "" && !strings.EqualFold(rec.State, state) {
			continue
		}
		matches = append(matches, match{rec, r.Score})
		if len(matches) >= n {
			break
		}
	}

	out := struct {
		Query   string  `json:"query"`
		Results int     `json:"results"`
		Matches []match `json:"matches"`
	}{query, len(matches), matches}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func cmdGet(id string) error {
	id = strings.ToUpper(id)
	records, err := parseAll()
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.ID == id {
			return json.NewEncoder(os.Stdout).Encode(rec)
		}
	}
	return fmt.Errorf("not found: %s", id)
}

func cmdStats() error {
	records, err := parseAll()
	if err != nil {
		return err
	}
	out := struct {
		Records int `json:"records"`
	}{len(records)}
	return json.NewEncoder(os.Stdout).Encode(out)
}

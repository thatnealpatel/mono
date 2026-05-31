// Package main implements a thin client
// around the teorth/erdosproblems repo
// for metadata and fetching upstream Erdos
// problems.
//
// The LaTeX endpoint response is tokenzied
// using golang.org/x/net/html. The problem
// metadata is tokenized using the default
// tokenizer in patel.codes/ranking for vector
// search.
//
// Similarly to patel.codes/oeis, a 'fetched'
// tombstone prevents upstream from being hit
// more than once per day per problem.
//
// Set ERDOS_CACHE_DIR_CLEAN=1 to auto-clean
// stale problem cache entries.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
	"patel.codes/ranking"
)

func main() {
	log.SetFlags(0)

	erdosCacheDir = os.Getenv("ERDOS_CACHE_DIR")
	if erdosCacheDir == "" {
		log.Fatal("ERDOS_CACHE_DIR is not set")
	}

	if err := ensureRepo(); err != nil {
		fmt.Fprintf(os.Stderr, "erdos: %v\n", err)
		os.Exit(1)
	}

	var err error
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "list":
		err = cmdList()
	case "search":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stdout, "usage: erdos search <query>")
			os.Exit(0)
		}
		err = cmdSearch(strings.Join(os.Args[2:], " "))
	case "fetch":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stdout, "usage: erdos fetch <N>")
			os.Exit(0)
		}
		err = cmdFetch(os.Args[2])
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

const erdosRemote = "git@github.com:teorth/erdosproblems.git"

var (
	erdosCacheDir string
	erdosRepoDir  string
	usage         = `usage: erdos <command> [args]

  list              all problems as JSON
  search <query>    BM25 search over comments+tags
  fetch <N>         fetch problem statement from erdosproblems.com
`
)

type Problem struct {
	Number  string   `yaml:"number"  json:"number"`
	Prize   string   `yaml:"prize"   json:"prize"`
	Status  Status   `yaml:"status"  json:"status"`
	OEIS    []string `yaml:"oeis"    json:"oeis"`
	Tags    []string `yaml:"tags"    json:"tags"`
	Comment string   `yaml:"comments" json:"comment,omitempty"`
	Formal  Formal   `yaml:"formalized" json:"formalized"`
}

type Status struct {
	State      string `yaml:"state"       json:"state"`
	LastUpdate string `yaml:"last_update" json:"last_update"`
	Note       string `yaml:"note,omitempty" json:"note,omitempty"`
}

type Formal struct {
	State      string `yaml:"state"       json:"state"`
	LastUpdate string `yaml:"last_update" json:"last_update"`
}

func loadProblems() ([]Problem, error) {
	data, err := os.ReadFile(filepath.Join(erdosRepoDir, "data", "problems.yaml"))
	if err != nil {
		return nil, err
	}
	var problems []Problem
	if err := yaml.Unmarshal(data, &problems); err != nil {
		return nil, err
	}
	return problems, nil
}

func cmdList() error {
	problems, err := loadProblems()
	if err != nil {
		return err
	}
	out := struct {
		Results  int       `json:"results"`
		Problems []Problem `json:"problems"`
	}{len(problems), problems}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func cmdSearch(query string) error {
	problems, err := loadProblems()
	if err != nil {
		return err
	}

	docs := make([]string, len(problems))
	for i, p := range problems {
		docs[i] = p.Comment + " " + strings.Join(p.Tags, " ")
	}

	idx := ranking.NewBM25(nil)
	idx.Build(docs)
	results := idx.Search(query)

	type match struct {
		Problem Problem `json:"problem"`
		Score   float64 `json:"score"`
	}
	matches := make([]match, len(results))
	for i, r := range results {
		matches[i] = match{problems[r.Index], r.Score}
	}
	if len(matches) > 20 {
		matches = matches[:20]
	}

	out := struct {
		Query   string  `json:"query"`
		Results int     `json:"results"`
		Matches []match `json:"matches"`
	}{query, len(matches), matches}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func ensureRepo() error {
	if err := os.MkdirAll(erdosCacheDir, 0o755); err != nil {
		return err
	}
	erdosRepoDir = filepath.Join(erdosCacheDir, "erdosproblems.git")
	if _, err := os.Stat(filepath.Join(erdosRepoDir, ".git")); err == nil {
		return ensureFresh(erdosRepoDir)
	}
	cmd := exec.Command("git", "clone", erdosRemote, erdosRepoDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning erdosproblems: %w", err)
	}
	return nil
}

func ensureFresh(dir string) error {
	marker := filepath.Join(erdosCacheDir, "fetched")
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
		fmt.Fprintln(os.Stderr, "erdos: fetch failed, using cached data")
		return nil
	}
	if err := os.WriteFile(marker, fmt.Appendf(nil, "%d\n", time.Now().Unix()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "erdos: writing marker: %v\n", err)
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
	fmt.Fprintf(os.Stderr, "erdos: updating %s..%s ... ", local[:8], remote[:8])
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

func cacheKey(number string) string {
	return time.Now().Format("20060102") + "-" + number + ".problem"
}

func cacheClean() {
	if os.Getenv("ERDOS_CACHE_DIR_CLEAN") != "1" {
		return
	}
	today := time.Now().Format("20060102")
	entries, err := os.ReadDir(erdosCacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".problem") {
			continue
		}
		if strings.HasPrefix(name, today+"-") {
			continue
		}
		os.Remove(filepath.Join(erdosCacheDir, name))
	}
}

func cmdFetch(number string) error {
	cacheClean()
	dir := erdosCacheDir
	key := cacheKey(number)
	path := filepath.Join(dir, key)

	if data, err := os.ReadFile(path); err == nil {
		_, err = os.Stdout.Write(data)
		return err
	}

	url := "https://www.erdosproblems.com/latex/" + number
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	// Be honest about who you are.
	req.Header.Set("User-Agent", "patel.codes.erdos/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	statement, sections := parseHTML(string(body))
	if statement == "" {
		return fmt.Errorf("no problem statement found for #%s", number)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "# Erdős Problem #%s\n\n", number)
	fmt.Fprintln(&out, statement)
	for _, s := range sections {
		fmt.Fprintln(&out)
		fmt.Fprintln(&out, s)
	}

	result := out.String()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(result), 0o644); err != nil {
		return err
	}

	_, err = os.Stdout.WriteString(result)
	return err
}

func parseHTML(s string) (statement string, sections []string) {
	z := html.NewTokenizer(strings.NewReader(s))
	var target string
	var depth int
	var buf strings.Builder

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			tag := string(tn)
			if tag == "div" && hasAttr {
				id, class := divAttrs(z)
				if id == "content" && statement == "" {
					target = "statement"
					depth = 1
					buf.Reset()
					continue
				}
				if class == "problem-additional-text" && statement != "" {
					target = "additional"
					depth = 1
					buf.Reset()
					continue
				}
			}
			if target != "" {
				if tag == "div" {
					depth++
				}
				switch tag {
				case "br":
					buf.WriteByte('\n')
				case "p":
					if buf.Len() > 0 {
						buf.WriteString("\n\n")
					}
				case "i":
					buf.WriteByte('*')
				}
			}
		case html.EndTagToken:
			tn, _ := z.TagName()
			tag := string(tn)
			if target != "" && tag == "i" {
				buf.WriteByte('*')
			}
			if target != "" && tag == "div" {
				depth--
				if depth == 0 {
					text := cleanMath(buf.String())
					if text != "" {
						switch target {
						case "statement":
							statement = text
						case "additional":
							sections = append(sections, text)
						}
					}
					target = ""
				}
			}
		case html.TextToken:
			if target != "" {
				buf.Write(z.Text())
			}
		}
	}
}

func divAttrs(z *html.Tokenizer) (id, class string) {
	for {
		key, val, more := z.TagAttr()
		switch string(key) {
		case "id":
			id = string(val)
		case "class":
			class = string(val)
		}
		if !more {
			return
		}
	}
}

var reMultiNL = regexp.MustCompile(`\n{3,}`)

func cleanMath(s string) string {
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	for _, noise := range []string{
		"Back to the problem",
	} {
		s = strings.ReplaceAll(s, noise, "")
	}
	return strings.TrimSpace(s)
}

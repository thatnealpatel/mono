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
// more than once per day and each problem
// from itself being queried upstream more
// than once per day.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
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

var httpClient = &http.Client{Timeout: 30 * time.Second}

func main() {
	log.SetFlags(0)

	erdosCacheDir = os.Getenv("ERDOS_CACHE_DIR")
	if erdosCacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			log.Fatal(err)
		}
		erdosCacheDir = filepath.Join(base, "erdos")
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

var (
	erdosCacheDir string
	erdosRepoDir  string
)

const erdosRemote = "git@github.com:teorth/erdosproblems.git"

const usage = `usage: erdos <command> [args]

  list              all problems as JSON
  search <query>    BM25 search over comments+tags
  fetch <N>         fetch problem and comments from erdosproblems.com
`

type Problem struct {
	Number  string   `yaml:"number"  json:"number"`
	Prize   string   `yaml:"prize"   json:"prize"`
	Status  Status   `yaml:"status"  json:"status"`
	OEIS    []string `yaml:"oeis"    json:"oeis"`
	Tags    []string `yaml:"tags"    json:"tags"`
	Comment string   `yaml:"comments" json:"comments,omitempty"`
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
		Problem
		Score float64 `json:"score"`
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", erdosRemote, erdosRepoDir)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fetch := exec.CommandContext(ctx, "git", "-C", dir, "fetch")
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
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, err := gitRev(dir, "@{u}")
	if err != nil {
		return fmt.Errorf("git rev-parse @{u}: %w", err)
	}
	if local == remote {
		return nil
	}
	short := func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	}
	fmt.Fprintf(os.Stderr, "erdos: updating %s..%s ... ", short(local), short(remote))
	pull := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
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

func problemDir(number string) string {
	return filepath.Join(erdosCacheDir, number)
}

func problemCacheFresh(number string) bool {
	marker := filepath.Join(problemDir(number), "fetched")
	b, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	var ts int64
	if _, err := fmt.Sscanf(string(b), "%d", &ts); err != nil {
		return false
	}
	return time.Since(time.Unix(ts, 0)) < 24*time.Hour
}

func writeProblemCache(number string, prob []byte, comments []byte) error {
	dir := problemDir(number)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "problem.json"), prob, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "comments.json"), comments, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fetched"), fmt.Appendf(nil, "%d\n", time.Now().Unix()), 0o644)
}

type CachedProblem struct {
	Statement string   `json:"statement"`
	Sections  []string `json:"sections,omitempty"`
}

type FetchResult struct {
	Number    string      `json:"number"`
	Statement string      `json:"statement"`
	Sections  []string    `json:"sections,omitempty"`
	Comments  []ForumPost `json:"comments"`
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "patel.codes.erdos/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func cmdFetch(number string) error {
	dir := problemDir(number)

	if problemCacheFresh(number) {
		probData, err := os.ReadFile(filepath.Join(dir, "problem.json"))
		if err != nil {
			return err
		}
		commData, err := os.ReadFile(filepath.Join(dir, "comments.json"))
		if err != nil {
			return err
		}
		return emitFetchResult(number, probData, commData)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latexURL, err := neturl.JoinPath("https://www.erdosproblems.com", "latex", number)
	if err != nil {
		return err
	}
	latexBody, err := httpGet(ctx, latexURL)
	if err != nil {
		return err
	}

	statement, sections := parseHTML(string(latexBody))
	if statement == "" {
		return fmt.Errorf("no problem statement found for #%s", number)
	}

	prob := CachedProblem{Statement: statement, Sections: sections}
	probData, err := json.Marshal(prob)
	if err != nil {
		return err
	}

	forumURL, err := neturl.JoinPath("https://www.erdosproblems.com", "forum", "thread", number)
	if err != nil {
		return err
	}
	forumBody, err := httpGet(ctx, forumURL)
	if err != nil {
		return err
	}

	posts, err := parseForumPosts(string(forumBody))
	if err != nil {
		return err
	}
	commData, err := json.Marshal(posts)
	if err != nil {
		return err
	}

	if err := writeProblemCache(number, probData, commData); err != nil {
		fmt.Fprintf(os.Stderr, "erdos: writing cache: %v\n", err)
	}

	return emitFetchResult(number, probData, commData)
}

func emitFetchResult(number string, probData, commData []byte) error {
	var prob CachedProblem
	if err := json.Unmarshal(probData, &prob); err != nil {
		return err
	}
	var posts []ForumPost
	if err := json.Unmarshal(commData, &posts); err != nil {
		return err
	}
	result := FetchResult{
		Number:    number,
		Statement: prob.Statement,
		Sections:  prob.Sections,
		Comments:  posts,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
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

type ForumPost struct {
	ID       string      `json:"id"`
	Author   string      `json:"author"`
	Date     string      `json:"date"`
	BodyHTML string      `json:"body_html"`
	Depth    int         `json:"depth"`
	Replies  []ForumPost `json:"replies,omitempty"`
}

func parseForumPosts(s string) ([]ForumPost, error) {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return nil, fmt.Errorf("parsing forum HTML: %w", err)
	}

	var posts []ForumPost
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if isPostList(n) {
			posts = append(posts, extractPostsFromList(n)...)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return posts, nil
}

func isPostList(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "ul" {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == "class" && strings.Contains(a.Val, "post-list") {
			return true
		}
	}
	return false
}

func extractPostsFromList(ul *html.Node) []ForumPost {
	var posts []ForumPost
	for li := ul.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.Data != "li" {
			continue
		}
		if !hasClass(li, "post") {
			continue
		}
		post := extractPost(li)
		posts = append(posts, post)
	}
	return posts
}

func extractPost(li *html.Node) ForumPost {
	var p ForumPost
	p.ID = getAttr(li, "id")
	p.Depth = parseDepth(li)

	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data == "div" && hasClass(c, "post-body") {
			p.BodyHTML = renderChildren(c)
		}
		if c.Data == "div" && hasClass(c, "post-meta") {
			p.Author, p.Date = parsePostMeta(c)
		}
		if c.Data == "ul" && hasClass(c, "replies") {
			p.Replies = extractPostsFromList(c)
		}
	}
	return p
}

func parseDepth(n *html.Node) int {
	cls := getAttr(n, "class")
	for _, part := range strings.Fields(cls) {
		if strings.HasPrefix(part, "depth-") {
			var d int
			if _, err := fmt.Sscanf(part, "depth-%d", &d); err == nil {
				return d
			}
		}
	}
	return 0
}

func parsePostMeta(div *html.Node) (author, date string) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "strong" {
			author = textContent(n)
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			href := getAttr(n, "href")
			if strings.Contains(href, "#post-") {
				date = textContent(n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(div)
	return strings.TrimSpace(author), strings.TrimSpace(date)
}

func textContent(n *html.Node) string {
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return buf.String()
}

func renderChildren(n *html.Node) string {
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return buf.String()
		}
	}
	return strings.TrimSpace(buf.String())
}

func hasClass(n *html.Node, cls string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, part := range strings.Fields(a.Val) {
				if part == cls {
					return true
				}
			}
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

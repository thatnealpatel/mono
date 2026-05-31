// Package main implements a thin-wrapper
// around the WHATWG spec that enables
// deterministic traversal of the spec in
// JSON output.
//
// The upstream https://html.spec.whatwg.org
// is checked at most once per day based
// on the etag that is saved.
//
// On reasonably fast consumer hardware, the
// parsing of the entire spec is O(10ms) to
// O(100ms) obviating the need for an type of
// intermediate representation.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

var cacheDir string

func main() {
	log.SetFlags(0)

	cacheDir = os.Getenv("WHATWG_CACHE_DIR")
	if cacheDir == "" {
		log.Fatal("WHATWG_CACHE_DIR is not set")
	}

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	sub := args[0]
	rest := args[1:]

	var err error
	switch sub {
	case "section":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stdout, "usage: whatwg section <secno>")
			os.Exit(1)
		}
		err = whatwgSection(rest[0])
	case "state":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stdout, "usage: whatwg state <name-or-secno>")
			os.Exit(1)
		}
		err = whatwgState(strings.Join(rest, " "))
	case "element":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stdout, "usage: whatwg element <name>")
			os.Exit(1)
		}
		err = whatwgElement(rest[0])
	case "algo":
		fs := flag.NewFlagSet("whatwg "+sub, flag.ExitOnError)
		list := fs.Bool("list", false, "list mode")
		var reordered []string
		for _, a := range rest {
			if a == "-list" {
				reordered = append([]string{a}, reordered...)
			} else {
				reordered = append(reordered, a)
			}
		}
		fs.Parse(reordered)
		if *list {
			err = whatwgAlgoList()
		} else if fs.NArg() == 0 {
			fmt.Fprintln(os.Stdout, "usage: whatwg algo [-list] <name-or-id>")
			os.Exit(1)
		} else {
			err = whatwgAlgoShow(strings.Join(fs.Args(), " "))
		}
	case "anchor":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stdout, "usage: whatwg anchor <slug>")
			os.Exit(1)
		}
		err = whatwgAnchor(rest[0])
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const usage = `usage: whatwg <command> [args]

  section <secno>         print a section by secno (e.g. 13.2.5)
  state <name-or-secno>   print tokenizer state switch
  element <name>          element parsing properties
  algo -list              list algorithm blocks
  algo <name-or-id>       print one algorithm block
  anchor <slug>           print fragment at #slug
`

func whatwgSection(secno string) error {
	doc, err := specDoc()
	if err != nil {
		return err
	}
	start := findHeadingBySecno(doc, secno)
	if start == nil {
		return fmt.Errorf("section %q not found", secno)
	}
	return whatwgSectionJSON(secno, start)
}

const specURL = "https://html.spec.whatwg.org/"

func specFile() string { return filepath.Join(cacheDir, "spec.html") }
func etagFile() string { return filepath.Join(cacheDir, "etag") }

func ensureSpec() error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	var etag string
	if info, err := os.Stat(etagFile()); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return nil
		}
		b, _ := os.ReadFile(etagFile())
		etag = strings.TrimSpace(string(b))
	}
	resp, err := http.Head(specURL)
	if err != nil {
		if _, serr := os.Stat(specFile()); serr == nil {
			fmt.Fprintln(os.Stderr, "whatwg: update check failed, using cached spec")
			return nil
		}
		return fmt.Errorf("checking spec: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checking spec: HTTP %s", resp.Status)
	}
	remoteEtag := resp.Header.Get("ETag")
	if etag != "" && remoteEtag == etag {
		os.Chtimes(etagFile(), time.Time{}, time.Now())
		return nil
	}
	fmt.Fprintf(os.Stderr, "whatwg: updating spec ... ")
	dlResp, err := http.Get(specURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR")
		return fmt.Errorf("fetching spec: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, dlResp.Status)
		return fmt.Errorf("fetching spec: HTTP %s", dlResp.Status)
	}
	f, err := os.Create(specFile())
	if err != nil {
		return err
	}
	n, err := io.Copy(f, dlResp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(specFile())
		fmt.Fprintln(os.Stderr, "ERROR")
		return fmt.Errorf("writing spec: %w", err)
	}
	if newEtag := dlResp.Header.Get("ETag"); newEtag != "" {
		os.WriteFile(etagFile(), []byte(newEtag), 0o644)
	}
	fmt.Fprintf(os.Stderr, "OK (%d bytes)\n", n)
	return nil
}

func specDoc() (*html.Node, error) {
	specCache.Do(func() {
		specCache.err = ensureSpec()
		if specCache.err != nil {
			return
		}
		f, err := os.Open(specFile())
		if err != nil {
			specCache.err = err
			return
		}
		defer f.Close()
		specCache.doc, specCache.err = html.Parse(f)
	})
	return specCache.doc, specCache.err
}

var specCache struct {
	sync.Once
	doc *html.Node
	err error
}

func findHeadingBySecno(doc *html.Node, target string) *html.Node {
	for h := range eachHeading(doc) {
		if h.Secno == target {
			return h.node
		}
	}
	return nil
}

func eachHeading(doc *html.Node) iter.Seq[heading] {
	return func(yield func(heading) bool) {
		for n := range doc.Descendants() {
			if _, ok := headingLevel(n); !ok {
				continue
			}
			if attr(n, "id") == "" {
				continue
			}
			if !yield(extractHeading(n)) {
				return
			}
		}
	}
}

type heading struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Secno string `json:"secno,omitempty"`
	Title string `json:"title"`
	node  *html.Node
}

func headingLevel(n *html.Node) (int, bool) {
	if n == nil || n.Type != html.ElementNode {
		return 0, false
	}
	if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
		return int(n.Data[1] - '0'), true
	}
	return 0, false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func extractHeading(n *html.Node) heading {
	lvl, _ := headingLevel(n)
	h := heading{Level: lvl, ID: attr(n, "id"), node: n}
	var dfnText, trailing strings.Builder
	for c := range n.ChildNodes() {
		switch {
		case isElement(c, "span") && slices.Contains(strings.Fields(attr(c, "class")), "secno"):
			h.Secno = collapseSpace(textOf(c))
		case isElement(c, "dfn"):
			if dfnText.Len() == 0 {
				dfnText.WriteString(textOf(c))
			}
		case isElement(c, "a") && slices.Contains(strings.Fields(attr(c, "class")), "self-link"):
		case c.Type == html.TextNode:
			trailing.WriteString(c.Data)
		case c.Type == html.ElementNode:
			trailing.WriteString(textOf(c))
		}
	}
	title := collapseSpace(dfnText.String())
	if title == "" {
		title = collapseSpace(trailing.String())
	}
	h.Title = title
	return h
}

func isElement(n *html.Node, name string) bool {
	return n != nil && n.Type == html.ElementNode && n.Data == name
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func textOf(n *html.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
		}
	}
	return b.String()
}

func whatwgSectionJSON(secno string, start *html.Node) error {
	h := extractHeading(start)
	out := sectionJSON{Secno: h.Secno, Title: h.Title}
	var cw countingWriter
	bw := bufio.NewWriter(&cw)
	_ = html.Render(bw, start)
	parentLevel := h.Level
	var childStart *html.Node
	var childHeading heading
	for n := start.NextSibling; n != nil; n = n.NextSibling {
		if lvl, ok := headingLevel(n); ok {
			s := extractHeading(n).Secno
			if s != "" && !isDescendantSecno(s, secno) {
				break
			}
			if childStart != nil {
				out.Children = append(out.Children, sectionChild{
					Secno:     childHeading.Secno,
					Title:     childHeading.Title,
					SizeBytes: sectionSize(childStart, n),
				})
			}
			if lvl == parentLevel+1 {
				childStart = n
				childHeading = extractHeading(n)
			} else {
				childStart = nil
			}
		}
		_ = html.Render(bw, n)
	}
	if childStart != nil {
		out.Children = append(out.Children, sectionChild{
			Secno:     childHeading.Secno,
			Title:     childHeading.Title,
			SizeBytes: sectionSize(childStart, nil),
		})
	}
	_ = bw.Flush()
	out.SizeBytes = int(cw.n)
	return printJSON(out)
}

type sectionJSON struct {
	Secno     string         `json:"secno"`
	Title     string         `json:"title"`
	SizeBytes int            `json:"size_bytes"`
	Children  []sectionChild `json:"children"`
}

type sectionChild struct {
	Secno     string `json:"secno"`
	Title     string `json:"title"`
	SizeBytes int    `json:"size_bytes"`
}

type countingWriter struct{ n int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	return len(p), nil
}

func isDescendantSecno(child, parent string) bool {
	return child == parent || strings.HasPrefix(child, parent+".")
}

func sectionSize(start, end *html.Node) int {
	var cw countingWriter
	bw := bufio.NewWriter(&cw)
	for n := start; n != nil && n != end; n = n.NextSibling {
		_ = html.Render(bw, n)
	}
	_ = bw.Flush()
	return int(cw.n)
}

func renderHTML(n *html.Node) string {
	var b strings.Builder
	html.Render(&b, n)
	return b.String()
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func whatwgState(query string) error {
	doc, err := specDoc()
	if err != nil {
		return err
	}
	h, ok := findStateHeading(doc, query)
	if !ok {
		return fmt.Errorf("state %q not found", query)
	}
	body, isSwitch := findStateBody(h.node)
	if body == nil {
		return fmt.Errorf("state %q has no algorithm block", query)
	}
	rules := []switchRule{}
	if isSwitch {
		rules = parseSwitchDL(body)
	}
	out := struct {
		heading
		Kind  string       `json:"kind"`
		Rules []switchRule `json:"rules"`
	}{h, kindOf(isSwitch), rules}
	return printJSON(out)
}

func findStateHeading(doc *html.Node, query string) (heading, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	qTrim := strings.TrimSuffix(q, " state")
	const tokSecnoPrefix = "13.2.5."
	for h := range eachHeading(doc) {
		if !strings.HasPrefix(h.Secno, tokSecnoPrefix) {
			continue
		}
		idLower := strings.ToLower(h.ID)
		titleLower := strings.ToLower(h.Title)
		titleTrim := strings.TrimSuffix(titleLower, " state")
		if h.Secno == query ||
			idLower == q ||
			titleLower == q ||
			titleTrim == qTrim ||
			idLower == q+"-state" {
			return h, true
		}
	}
	return heading{}, false
}

func findStateBody(h *html.Node) (body *html.Node, isSwitch bool) {
	var algo *html.Node
	for n := range followingNodes(h) {
		if _, ok := headingLevel(n); ok {
			break
		}
		if isElement(n, "dl") && slices.Contains(strings.Fields(attr(n, "class")), "switch") {
			return n, true
		}
		if algo == nil && isElement(n, "div") && hasAttr(n, "data-algorithm") {
			algo = n
		}
	}
	return algo, false
}

func followingNodes(start *html.Node) iter.Seq[*html.Node] {
	return func(yield func(*html.Node) bool) {
		n := start
		for {
			if n.FirstChild != nil {
				n = n.FirstChild
			} else {
				for n != nil && n.NextSibling == nil {
					n = n.Parent
				}
				if n == nil {
					return
				}
				n = n.NextSibling
			}
			if !yield(n) {
				return
			}
		}
	}
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

type switchRule struct {
	Conditions  []string `json:"conditions"`
	Prose       string   `json:"prose"`
	Refs        []string `json:"refs,omitempty"`
	InfraRefs   []string `json:"infra_refs,omitempty"`
	ParseErrors []string `json:"parse_errors,omitempty"`
}

func parseSwitchDL(dl *html.Node) []switchRule {
	var rules []switchRule
	var pending []string
	for c := range dl.ChildNodes() {
		switch {
		case isElement(c, "dt"):
			pending = append(pending, collapseSpace(textOf(c)))
		case isElement(c, "dd"):
			rules = append(rules, switchRule{
				Conditions:  pending,
				Prose:       collapseSpace(textOf(c)),
				Refs:        hashHrefs(c),
				InfraRefs:   infraHrefs(c),
				ParseErrors: parseErrorNames(c),
			})
			pending = nil
		}
	}
	return rules
}

func hashHrefs(root *html.Node) []string {
	var out []string
	seen := map[string]bool{}
	for n := range root.Descendants() {
		if !isElement(n, "a") {
			continue
		}
		if h := attr(n, "href"); strings.HasPrefix(h, "#") {
			id := h[1:]
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func infraHrefs(root *html.Node) []string {
	const prefix = "https://infra.spec.whatwg.org/#"
	var out []string
	seen := map[string]bool{}
	for n := range root.Descendants() {
		if !isElement(n, "a") {
			continue
		}
		if h := attr(n, "href"); strings.HasPrefix(h, prefix) {
			id := strings.TrimPrefix(h, prefix)
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func parseErrorNames(root *html.Node) []string {
	var out []string
	seen := map[string]bool{}
	for n := range root.Descendants() {
		if !isElement(n, "a") {
			continue
		}
		h := attr(n, "href")
		if !strings.HasPrefix(h, "#parse-error-") {
			continue
		}
		name := strings.TrimPrefix(h, "#parse-error-")
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func kindOf(isSwitch bool) string {
	if isSwitch {
		return "switch"
	}
	return "steps"
}

func whatwgAlgoList() error {
	doc, err := specDoc()
	if err != nil {
		return err
	}
	algos := scanAlgos(doc)
	bw := bufio.NewWriter(os.Stdout)
	defer bw.Flush()
	enc := json.NewEncoder(bw)
	for _, a := range algos {
		_ = enc.Encode(a)
	}
	return nil
}

func scanAlgos(doc *html.Node) []algoRecord {
	var out []algoRecord
	var section heading
	var dfnID, dfnText string
	var haveDfn bool
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		if _, ok := headingLevel(n); ok && attr(n, "id") != "" {
			section = extractHeading(n)
			dfnID, dfnText, haveDfn = "", "", false
			continue
		}
		if isElement(n, "dfn") {
			dfnID = attr(n, "id")
			dfnText = collapseSpace(textOf(n))
			haveDfn = true
			continue
		}
		if isElement(n, "div") && hasAttr(n, "data-algorithm") {
			anchor, name := dfnID, dfnText
			if !haveDfn {
				anchor = section.ID
				name = section.Title + " (§)"
			} else if anchor == "" {
				anchor = section.ID
			}
			if anchor == "" {
				anchor = "-"
			}
			out = append(out, algoRecord{
				Secno:  section.Secno,
				Anchor: anchor,
				Name:   name,
				node:   n,
			})
		}
	}
	return out
}

type algoRecord struct {
	Secno  string `json:"secno,omitempty"`
	Anchor string `json:"anchor"`
	Name   string `json:"name"`
	node   *html.Node
}

// whatwgAlgoShow returns raw HTML:
// algorithm blocks have varied internal
// structure (nested lists, conditionals,
// prose) that resists useful decomposition.
func whatwgAlgoShow(query string) error {
	doc, err := specDoc()
	if err != nil {
		return err
	}
	algos := scanAlgos(doc)
	q := strings.ToLower(strings.TrimSpace(query))
	qTrim := strings.TrimSuffix(q, " (§)")
	for _, a := range algos {
		anchorLower := strings.ToLower(a.Anchor)
		nameLower := strings.ToLower(a.Name)
		nameTrim := strings.TrimSuffix(nameLower, " (§)")
		if anchorLower == q || nameLower == q || nameTrim == qTrim || a.Secno == query {
			return printJSON(struct {
				Secno  string `json:"secno,omitempty"`
				Anchor string `json:"anchor"`
				Name   string `json:"name"`
				HTML   string `json:"html"`
			}{a.Secno, a.Anchor, a.Name, renderHTML(a.node)})
		}
	}
	return fmt.Errorf("algorithm %q not found", query)
}

// whatwgAnchor returns raw HTML: anchors
// point to arbitrary spec elements whose
// structure varies too widely to decompose.
func whatwgAnchor(slug string) error {
	slug = strings.TrimPrefix(slug, "#")
	doc, err := specDoc()
	if err != nil {
		return err
	}
	n := findAnchor(doc, slug)
	if n == nil {
		return fmt.Errorf("anchor %q not found", slug)
	}
	return printJSON(struct {
		Anchor string `json:"anchor"`
		HTML   string `json:"html"`
	}{slug, renderHTML(n)})
}

func findAnchor(doc *html.Node, slug string) *html.Node {
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && attr(n, "id") == slug {
			return n
		}
	}
	return nil
}

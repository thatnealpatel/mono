// Package main implements a simple vector
// search over Wikipedia article titles and
// reformats the articles to Markdown; in
// addition, it provides a way to extract
// all outgoing links from a target artice.
//
// It constructs one gob-encoded index and
// one custom wire format-encoded IDF cache
// that is used to make arbitrary search
// and retrieval fast.
//
// See README for how WIKI_CACHE_DIR should
// be setup with the Wikimedia Dump Files.
package main

import (
	"compress/bzip2"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/dustin/go-wikiparse"
	"golang.org/x/net/html"
	"patel.codes/ranking"
)

var (
	wikiCacheDir string
	wikiDumpDate string
	wikiIdxPath  string
)

func main() {
	log.SetFlags(0)

	wikiCacheDir = os.Getenv("WIKI_CACHE_DIR")
	if wikiCacheDir == "" {
		log.Fatal("WIKI_CACHE_DIR is not set")
	}
	if err := wikiResolveDump(); err != nil {
		fmt.Fprintf(os.Stderr, "goof-wiki: %v\n", err)
		os.Exit(1)
	}
	wikiIdxPath = filepath.Join(wikiCacheDir, "enwiki-"+wikiDumpDate+".index")

	var err error
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "build":
		err = wikiBuild()
	case "article":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = wikiArticle(strings.Join(os.Args[2:], " "))
	case "links":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = wikiLinks(strings.Join(os.Args[2:], " "))
	case "search":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = wikiSearch(strings.Join(os.Args[2:], " "))
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "goof-wiki: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: wiki <command> [args]

  build              build article index from bz2
  article <title>    article text as markdown
  links <title>      outgoing links
  search <query>     search titles
`

func wikiBuild() error {
	f, err := os.Open(filepath.Join(wikiCacheDir, "enwiki-"+wikiDumpDate+"-pages-articles-multistream-index.txt.bz2"))
	if err != nil {
		return err
	}
	defer f.Close()
	ir := wikiparse.NewIndexReader(bzip2.NewReader(f))
	var entries []wikiEntry
	for {
		ie, err := ir.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading index stream: %w", err)
		}
		entries = append(entries, wikiEntry{Title: ie.ArticleName, Offset: ie.StreamOffset})
		if len(entries)%50000 == 0 {
			fmt.Fprintf(os.Stderr, "\rindexing %d articles...", len(entries))
		}
	}
	fmt.Fprint(os.Stderr, "\r")
	slices.SortFunc(entries, func(a, b wikiEntry) int {
		return strings.Compare(a.Title, b.Title)
	})
	out, err := os.Create(wikiIdxPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := gob.NewEncoder(out).Encode(entries); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "indexed %d articles\n", len(entries))
	return nil
}

var reWikiDump = regexp.MustCompile(`^enwiki-(\d{8})-pages-articles-multistream\.xml\.bz2$`)

func wikiResolveDump() error {
	entries, err := os.ReadDir(wikiCacheDir)
	if err != nil {
		return fmt.Errorf("reading wikipedia source dir: %w", err)
	}
	var best string
	for _, e := range entries {
		if m := reWikiDump.FindStringSubmatch(e.Name()); m != nil {
			if m[1] > best {
				best = m[1]
			}
		}
	}
	if best == "" {
		return fmt.Errorf("no enwiki dump found in %s", wikiCacheDir)
	}
	wikiDumpDate = best
	return nil
}

type wikiEntry struct {
	Title  string
	Offset int64
}

func wikiArticle(title string) error {
	page, err := wikiFetchPage(title)
	if err != nil {
		return err
	}
	page, err = wikiFollowRedirect(page)
	if err != nil {
		return err
	}
	if len(page.Revisions) == 0 {
		return fmt.Errorf("no revisions for %q", title)
	}
	n := len(page.Revisions) - 1
	fmt.Println(wikiClean(page.Revisions[n].Text))
	return nil
}

func wikiFollowRedirect(page *wikiparse.Page) (*wikiparse.Page, error) {
	if page.Redir.Title == "" {
		return page, nil
	}
	fmt.Fprintf(os.Stderr, "redirect: %s → %s\n", page.Title, page.Redir.Title)
	return wikiFetchPage(page.Redir.Title)
}

func wikiFetchPage(title string) (*wikiparse.Page, error) {
	entries, err := wikiLoadIndex()
	if err != nil {
		return nil, err
	}
	offset, ok := wikiLookup(entries, title)
	if !ok {
		return nil, fmt.Errorf("article not found: %q", title)
	}
	f, err := os.Open(filepath.Join(wikiCacheDir, "enwiki-"+wikiDumpDate+"-pages-articles-multistream.xml.bz2"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bzip2.NewReader(f))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "page" {
			continue
		}
		var page wikiparse.Page
		if err := dec.DecodeElement(&page, &se); err != nil {
			return nil, err
		}
		if page.Title == title {
			return &page, nil
		}
	}
	return nil, fmt.Errorf("article %q not in block at offset %d", title, offset)
}

func wikiLoadIndex() ([]wikiEntry, error) {
	f, err := os.Open(wikiIdxPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := wikiBuild(); err != nil {
			return nil, err
		}
		f, err = os.Open(wikiIdxPath)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var entries []wikiEntry
	if err := gob.NewDecoder(f).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func wikiLookup(entries []wikiEntry, title string) (int64, bool) {
	i, ok := slices.BinarySearchFunc(entries, title, func(e wikiEntry, t string) int {
		return strings.Compare(e.Title, t)
	})
	if ok {
		return entries[i].Offset, true
	}
	return 0, false
}

func wikiClean(text string) string {
	var maths []string
	text = reMath.ReplaceAllStringFunc(text, func(m string) string {
		inner := reNotATypo.ReplaceAllString(reMath.FindStringSubmatch(m)[1], "$1")
		maths = append(maths, inner)
		return fmt.Sprintf("WIKIMATH%dENDMATH", len(maths)-1)
	})
	text = extractMathTemplates(text, &maths)
	text = reFile.ReplaceAllString(text, "")
	text = reTable.ReplaceAllString(text, "")
	for range 5 {
		next := reTmpl.ReplaceAllString(text, "")
		if next == text {
			break
		}
		text = next
	}
	text = htmlToText(text)
	text = reHeading.ReplaceAllStringFunc(text, func(m string) string {
		parts := reHeading.FindStringSubmatch(m)
		return strings.Repeat("#", len(parts[1])) + " " + parts[2]
	})
	text = reBold.ReplaceAllString(text, "**$1**")
	text = reItalic.ReplaceAllString(text, "*$1*")
	text = reLink.ReplaceAllString(text, "$1")
	text = reExtLink.ReplaceAllString(text, "$1")
	for i, m := range maths {
		text = strings.Replace(text, fmt.Sprintf("WIKIMATH%dENDMATH", i), "$"+m+"$", 1)
	}
	text = reMultiNL.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

var (
	reMathTmpl = regexp.MustCompile(`\{\{(?:math|mvar)\|`)
	reNotATypo = regexp.MustCompile(`\{\{not a typo\|([^}]+)\}\}`)
)

func extractMathTemplates(text string, maths *[]string) string {
	for {
		loc := reMathTmpl.FindStringIndex(text)
		if loc == nil {
			break
		}
		// Find the matching closing }} with brace-depth tracking.
		depth := 0
		end := -1
		for i := loc[0]; i < len(text)-1; i++ {
			if text[i] == '{' && text[i+1] == '{' {
				depth++
				i++
			} else if text[i] == '}' && text[i+1] == '}' {
				depth--
				i++
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		if end == -1 {
			break
		}
		inner := text[loc[1] : end-2]
		*maths = append(*maths, cleanMathInner(inner))
		placeholder := fmt.Sprintf("WIKIMATH%dENDMATH", len(*maths)-1)
		text = text[:loc[0]] + placeholder + text[end:]
	}
	return text
}

var (
	reSfrac    = regexp.MustCompile(`\{\{sfrac\|([^{}|]+)\|([^{}|]+)\}\}`)
	reTmplBare = regexp.MustCompile(`\{\{([^|{}]+)\}\}`)
	reSup      = regexp.MustCompile(`<sup>([^<]*)</sup>`)
	reSub      = regexp.MustCompile(`<sub>([^<]*)</sub>`)
	reWikiIta  = regexp.MustCompile(`''([^']+)''`)
)

func cleanMathInner(s string) string {
	s = reSfrac.ReplaceAllString(s, "$1/$2")
	s = reNotATypo.ReplaceAllString(s, "$1")
	s = reTmplBare.ReplaceAllString(s, "$1")
	s = reSup.ReplaceAllString(s, "^{$1}")
	s = reSub.ReplaceAllString(s, "_{$1}")
	s = reWikiIta.ReplaceAllString(s, "$1")
	return s
}

var (
	reMath    = regexp.MustCompile(`(?s)<math[^>]*>(.*?)</math>`)
	reTmpl    = regexp.MustCompile(`\{\{(?:[^{}]|\{[^{}]*\})*\}\}`)
	reFile    = regexp.MustCompile(`(?s)\[\[(?:File|Image|Category):(?:[^\[\]]|\[\[[^\]]*\]\])*\]\]`)
	reTable   = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
	reHeading = regexp.MustCompile(`(?m)^(={2,6})\s*(.+?)\s*={2,6}\s*$`)
	reBold    = regexp.MustCompile(`'''(.+?)'''`)
	reItalic  = regexp.MustCompile(`''(.+?)''`)
	reLink    = regexp.MustCompile(`\[\[(?:[^|\]]*\|)?([^\]]+)\]\]`)
	reExtLink = regexp.MustCompile(`\[https?://[^\s\]]+\s*([^\]]*)\]`)
	reMultiNL = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	var skip int
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if wikiSkipElements[tag] {
				skip++
			}
			if skip == 0 {
				switch tag {
				case "br", "p", "div", "li", "tr", "dd", "dt":
					b.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if wikiSkipElements[string(name)] && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
}

var wikiSkipElements = map[string]bool{
	"ref": true, "gallery": true, "nowiki": true,
	"score": true, "source": true, "syntaxhighlight": true,
}

func wikiLinks(title string) error {
	page, err := wikiFetchPage(title)
	if err != nil {
		return err
	}
	if len(page.Revisions) == 0 {
		return fmt.Errorf("no revisions for %q", title)
	}
	n := len(page.Revisions) - 1
	links := wikiparse.FindLinks(page.Revisions[n].Text)
	return json.NewEncoder(os.Stdout).Encode(struct {
		Title string   `json:"title"`
		Links []string `json:"links"`
	}{page.Title, links})
}

func wikiSearch(query string) error {
	entries, err := wikiLoadIndex()
	if err != nil {
		return err
	}
	titles := make([]string, len(entries))
	for i, e := range entries {
		titles[i] = e.Title
	}

	idx := ranking.NewIDF(nil)
	cachePath := filepath.Join(wikiCacheDir, "enwiki-"+wikiDumpDate+"-idf.cache")
	if f, err := os.Open(cachePath); err == nil {
		_, err = idx.ReadFrom(f)
		f.Close()
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "wiki: building search index ...")
		idx.Build(titles)
		f, err := os.Create(cachePath)
		if err != nil {
			return err
		}
		_, err = idx.WriteTo(f)
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(cachePath)
			return err
		}
	}
	results := idx.Search(query)

	type match struct {
		Title string  `json:"title"`
		Score float64 `json:"score"`
	}
	matches := make([]match, len(results))
	for i, r := range results {
		matches[i] = match{Title: titles[r.Index], Score: r.Score}
	}
	out := struct {
		Query   string  `json:"query"`
		Results int     `json:"results"`
		Matches []match `json:"matches"`
	}{query, len(matches), matches}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(out)
}

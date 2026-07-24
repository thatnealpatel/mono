package retrieval

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"patel.codes/ranking"
)

type Wiki struct {
	dir        string
	date       string
	entries    []wikiEntry
	idf        *ranking.IDF
	maxResults int
}

func NewWiki(dir string, maxResults int) (*Wiki, error) {
	date, err := resolveWikiDump(dir)
	if err != nil {
		return nil, err
	}

	idxPath := filepath.Join(dir, "enwiki-"+date+".index")
	f, err := os.Open(idxPath)
	if err != nil {
		return nil, err
	}
	var entries []wikiEntry
	err = gob.NewDecoder(f).Decode(&entries)
	if err2 := f.Close(); err == nil {
		err = err2
	}
	if err != nil {
		return nil, fmt.Errorf("decoding wiki index %s: %w", idxPath, err)
	}

	cachePath := filepath.Join(dir, "enwiki-"+date+"-idf.cache")
	cf, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	idf := ranking.NewIDF(nil)
	_, err = idf.ReadFrom(cf)
	if err2 := cf.Close(); err == nil {
		err = err2
	}
	if err != nil {
		return nil, fmt.Errorf("reading wiki idf cache %s: %w", cachePath, err)
	}

	return &Wiki{dir: dir, date: date, entries: entries, idf: idf, maxResults: maxResults}, nil
}

type wikiEntry struct {
	Title  string
	Offset int64
}

var reWikiDump = regexp.MustCompile(`^enwiki-(\d{8})-pages-articles-multistream\.xml\.bz2$`)

func resolveWikiDump(dir string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading wiki data dir: %w", err)
	}
	var best string
	for _, e := range ents {
		if m := reWikiDump.FindStringSubmatch(e.Name()); m != nil && m[1] > best {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("no enwiki dump found in %s", dir)
	}
	return best, nil
}

func (w *Wiki) Lookup(title string) (int64, bool) {
	i, ok := slices.BinarySearchFunc(w.entries, title, func(e wikiEntry, t string) int {
		return strings.Compare(e.Title, t)
	})
	if ok {
		return w.entries[i].Offset, true
	}
	return 0, false
}

func (w *Wiki) Search(query string) WikiSearchResult {
	hits := w.idf.Search(query)
	matches := make([]WikiSearchMatch, len(hits))
	for i, h := range hits {
		matches[i] = WikiSearchMatch{Title: w.entries[h.Index].Title, Score: h.Score}
	}
	total := len(matches)
	truncated := false
	if total > w.maxResults {
		matches = matches[:w.maxResults]
		truncated = true
	}
	return WikiSearchResult{Query: query, Results: total, Truncated: truncated, Matches: matches}
}

type WikiSearchResult struct {
	Query     string            `json:"query"`
	Results   int               `json:"results"`
	Truncated bool              `json:"truncated"`
	Matches   []WikiSearchMatch `json:"matches"`
}

type WikiSearchMatch struct {
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

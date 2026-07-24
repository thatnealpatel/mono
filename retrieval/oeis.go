package retrieval

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"patel.codes/ranking"
)

type Oeis struct {
	seqDir    string
	ids       []string
	names     []string
	terms     []string
	bm        *ranking.BM25
	searchCap int
}

func NewOeis(dir string, searchCap int) (*Oeis, error) {
	seqDir := filepath.Join(dir, "oeisdata.git", "seq")
	shards, err := os.ReadDir(seqDir)
	if err != nil {
		return nil, fmt.Errorf("reading oeis seq dir: %w", err)
	}

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 16)
		ids   []string
		names []string
		terms []string
	)
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()

			sub := shard.Name()
			files, err := os.ReadDir(filepath.Join(seqDir, sub))
			if err != nil {
				return
			}
			for _, f := range files {
				if !strings.HasSuffix(f.Name(), ".seq") {
					continue
				}
				q, err := oeisQuickParse(filepath.Join(seqDir, sub, f.Name()))
				if err != nil {
					continue
				}
				id := strings.TrimSuffix(f.Name(), ".seq")
				mu.Lock()
				ids = append(ids, id)
				names = append(names, q.Name)
				terms = append(terms, q.Terms)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	bm := ranking.NewBM25(nil)
	bm.Build(names)
	return &Oeis{seqDir: seqDir, ids: ids, names: names, terms: terms, bm: bm, searchCap: searchCap}, nil
}

func oeisQuickParse(path string) (oeisQuick, error) {
	f, err := os.Open(path)
	if err != nil {
		return oeisQuick{}, err
	}
	defer f.Close()

	var q oeisQuick
	var terms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		tag, body, ok := oeisField(sc.Text())
		if !ok {
			continue
		}
		switch tag {
		case "%N":
			q.Name = body
		case "%S", "%T", "%U":
			terms = append(terms, body)
		}
	}
	q.Terms = strings.Join(terms, "")
	return q, sc.Err()
}

type oeisQuick struct {
	Name  string
	Terms string
}

func oeisField(line string) (tag, body string, ok bool) {
	if len(line) < 4 {
		return "", "", false
	}
	body = line
	if sp := strings.IndexByte(line[3:], ' '); sp >= 0 {
		body = line[3+sp+1:]
	}
	return line[:2], body, true
}

func (o *Oeis) Show(id string) (OeisEntry, error) {
	path := oeisSeqPath(o.seqDir, id)
	if path == "" {
		return OeisEntry{}, fmt.Errorf("invalid sequence ID: %q", id)
	}
	return oeisParseFile(path)
}

var reOeisID = regexp.MustCompile(`^[Aa]\d{6}$`)

func oeisSeqPath(seqDir, id string) string {
	if !reOeisID.MatchString(id) {
		return ""
	}
	id = strings.ToUpper(id)
	return filepath.Join(seqDir, id[:4], id+".seq")
}

func oeisParseFile(path string) (OeisEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return OeisEntry{}, err
	}
	defer f.Close()

	var e OeisEntry
	var terms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		tag, body, ok := oeisField(line)
		if !ok {
			continue
		}
		switch tag {
		case "%I":
			if parts := strings.Fields(line[3:]); len(parts) > 0 {
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

type OeisEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Terms    string   `json:"terms"`
	Comments []string `json:"comments,omitempty"`
	Formulas []string `json:"formulas,omitempty"`
	Xrefs    []string `json:"xrefs,omitempty"`
	Keywords string   `json:"keywords,omitempty"`
	Programs []string `json:"programs,omitempty"`
}

func (o *Oeis) Search(query string) OeisSearchResult {
	hits := o.bm.Search(query)
	matches := make([]OeisSearchMatch, len(hits))
	for i, h := range hits {
		matches[i] = OeisSearchMatch{ID: o.ids[h.Index], Name: o.names[h.Index], Score: h.Score}
	}
	total := len(matches)
	truncated := false
	if total > o.searchCap {
		matches = matches[:o.searchCap]
		truncated = true
	}
	return OeisSearchResult{Query: query, Results: total, Truncated: truncated, Matches: matches}
}

type OeisSearchResult struct {
	Query     string            `json:"query"`
	Results   int               `json:"results"`
	Truncated bool              `json:"truncated"`
	Matches   []OeisSearchMatch `json:"matches"`
}

type OeisSearchMatch struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

const oeisMaxMatches = 1000

func (o *Oeis) Match(query string) []OeisMatch {
	query = strings.TrimSpace(query)
	if !strings.HasSuffix(query, ",") {
		query += ","
	}
	var out []OeisMatch
	for i, t := range o.terms {
		if strings.Contains(t, query) {
			out = append(out, OeisMatch{ID: o.ids[i], Name: o.names[i], Terms: t})
			if len(out) >= oeisMaxMatches {
				break
			}
		}
	}
	return out
}

type OeisMatch struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Terms string `json:"terms"`
}

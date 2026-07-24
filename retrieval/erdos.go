package retrieval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"patel.codes/ranking"
)

type Erdos struct {
	problems   []ErdosProblem
	bm         *ranking.BM25
	maxMatches int
}

func NewErdos(dir string, maxMatches int) (*Erdos, error) {
	path := filepath.Join(dir, "erdosproblems.git", "data", "problems.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading erdos problems: %w", err)
	}
	var problems []ErdosProblem
	if err := yaml.Unmarshal(data, &problems); err != nil {
		return nil, fmt.Errorf("parsing erdos problems: %w", err)
	}

	docs := make([]string, len(problems))
	for i, p := range problems {
		docs[i] = p.Comment + " " + strings.Join(p.Tags, " ")
	}
	bm := ranking.NewBM25(nil)
	bm.Build(docs)
	return &Erdos{problems: problems, bm: bm, maxMatches: maxMatches}, nil
}

type ErdosProblem struct {
	Number  string      `yaml:"number" json:"number"`
	Prize   string      `yaml:"prize" json:"prize"`
	Status  ErdosStatus `yaml:"status" json:"status"`
	OEIS    []string    `yaml:"oeis" json:"oeis"`
	Tags    []string    `yaml:"tags" json:"tags"`
	Comment string      `yaml:"comments" json:"comments,omitempty"`
	Formal  ErdosFormal `yaml:"formalized" json:"formalized"`
}

type ErdosStatus struct {
	State      string `yaml:"state" json:"state"`
	LastUpdate string `yaml:"last_update" json:"last_update"`
	Note       string `yaml:"note,omitempty" json:"note,omitempty"`
}

type ErdosFormal struct {
	State      string `yaml:"state" json:"state"`
	LastUpdate string `yaml:"last_update" json:"last_update"`
}

func (e *Erdos) List() ErdosListResult {
	return ErdosListResult{Results: len(e.problems), Problems: e.problems}
}

type ErdosListResult struct {
	Results  int            `json:"results"`
	Problems []ErdosProblem `json:"problems"`
}

func (e *Erdos) Search(query string) ErdosSearchResult {
	hits := e.bm.Search(query)
	total := len(hits)
	hits = hits[:min(total, e.maxMatches)]
	matches := make([]ErdosMatch, len(hits))
	for i, h := range hits {
		matches[i] = ErdosMatch{ErdosProblem: e.problems[h.Index], Score: h.Score}
	}
	return ErdosSearchResult{Query: query, Results: total, Truncated: total > e.maxMatches, Matches: matches}
}

type ErdosSearchResult struct {
	Query     string       `json:"query"`
	Results   int          `json:"results"`
	Truncated bool         `json:"truncated"`
	Matches   []ErdosMatch `json:"matches"`
}

type ErdosMatch struct {
	ErdosProblem
	Score float64 `json:"score"`
}

package ranking

import (
	"cmp"
	"encoding/gob"
	"io"
	"math"
	"slices"
)

type IDF struct {
	tok      Tokenizer
	postings map[string][]idfPosting
	idf      map[string]float64
}

type idfPosting struct {
	Doc int32
}

type IDFParams struct {
	Tokenizer Tokenizer
}

func NewIDF(p *IDFParams) *IDF {
	var tok Tokenizer = DefaultTokenizer{}
	if p != nil && p.Tokenizer != nil {
		tok = p.Tokenizer
	}
	return &IDF{tok: tok}
}

func (idx *IDF) Build(docs []string) {
	idx.postings = map[string][]idfPosting{}
	n := float64(len(docs))
	for i, text := range docs {
		seen := map[string]bool{}
		for _, term := range idx.tok.Tokenize(text) {
			if seen[term] {
				continue
			}
			seen[term] = true
			idx.postings[term] = append(idx.postings[term], idfPosting{int32(i)})
		}
	}
	idx.idf = make(map[string]float64, len(idx.postings))
	for term, pl := range idx.postings {
		df := float64(len(pl))
		idx.idf[term] = math.Log((n-df+0.5)/(df+0.5) + 1)
	}
}

func (idx *IDF) Search(query string) []*Result {
	tokens := idx.tok.Tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	scores := map[int32]float64{}
	for _, term := range tokens {
		idf := idx.idf[term]
		for _, p := range idx.postings[term] {
			scores[p.Doc] += idf
		}
	}
	results := make([]*Result, 0, len(scores))
	for doc, score := range scores {
		results = append(results, &Result{Index: int(doc), Score: score})
	}
	slices.SortFunc(results, func(a, b *Result) int {
		return cmp.Compare(b.Score, a.Score)
	})
	return results
}

func (idx *IDF) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	err := gob.NewEncoder(cw).Encode(idfGob{
		Postings: idx.postings,
		IDF:      idx.idf,
	})
	return cw.n, err
}

func (idx *IDF) ReadFrom(r io.Reader) (int64, error) {
	cr := &countReader{r: r}
	var g idfGob
	if err := gob.NewDecoder(cr).Decode(&g); err != nil {
		return cr.n, err
	}
	idx.postings = g.Postings
	idx.idf = g.IDF
	return cr.n, nil
}

type idfGob struct {
	Postings map[string][]idfPosting
	IDF      map[string]float64
}

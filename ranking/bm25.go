package ranking

import (
	"cmp"
	"encoding/gob"
	"io"
	"math"
	"slices"
)

// NewBM25 creates a [BM25] with the
// specified p; if p is nil, then the
// defaults K1=1.2 B=0.75 are used.
//
// Degenerate parameters, such as K1=0
// and B=0, are allowed; the caller is
// responsible for non-degenerate p.
//
// NewBM25 panics if K1 or B is negative.
func NewBM25(p *BM25Params) *BM25 {
	// TODO(nealpatel): This can be a slightly
	// better API since BM25{k1: 0, b: 0} is
	// just IDF as implemented in ranking.
	bm := &BM25{
		tok: DefaultTokenizer{},
		k1:  1.2,
		b:   0.75,
	}
	if p == nil {
		return bm
	}

	if p.Tokenizer != nil {
		bm.tok = p.Tokenizer
	}
	if p.K1 < 0 {
		panic("bm25: K1 cannot be negative")
	}
	if p.B < 0 {
		panic("bm25: B cannot be negative")
	}
	bm.k1 = p.K1
	bm.b = p.B
	return bm
}

type BM25 struct {
	tok   Tokenizer
	k1    float64
	b     float64
	docs  []bm25Doc
	idf   map[string]float64
	avgdl float64
}

type BM25Params struct {
	K1        float64
	B         float64
	Tokenizer Tokenizer
}

func (bm *BM25) Build(docs []string) {
	docFreq := map[string]int{}
	n := float64(len(docs))
	bm.docs = make([]bm25Doc, len(docs))
	bm.idf = map[string]float64{}

	var totalLen float64
	for i, text := range docs {
		tokens := bm.tok.Tokenize(text)
		tf := make(map[string]float64, len(tokens))
		for _, tok := range tokens {
			tf[tok]++
		}
		for term := range tf {
			docFreq[term]++
		}
		dl := float64(len(tokens))
		bm.docs[i] = bm25Doc{TF: tf, DL: dl}
		totalLen += dl
	}

	if n > 0 {
		bm.avgdl = totalLen / n
	}

	for term, df := range docFreq {
		bm.idf[term] = math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}
}

func (bm *BM25) Search(query string) []*Result {
	tokens := bm.tok.Tokenize(query)
	if len(tokens) == 0 {
		return nil
	}

	qtf := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		qtf[tok] = true
	}

	var results []*Result
	for i, doc := range bm.docs {
		var score float64
		for term := range qtf {
			tf, ok := doc.TF[term]
			if !ok {
				continue
			}
			idf := bm.idf[term]
			denom := tf + bm.k1*(1-bm.b+bm.b*doc.DL/max(bm.avgdl, 1e-10))
			score += idf * (tf * (bm.k1 + 1)) / denom
		}
		if score > 0.001 {
			results = append(results, &Result{Index: i, Score: score})
		}
	}

	slices.SortFunc(results, func(a, b *Result) int {
		return cmp.Compare(b.Score, a.Score)
	})
	return results
}

func (bm *BM25) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	err := gob.NewEncoder(cw).Encode(bm25Gob{
		K1:    bm.k1,
		B:     bm.b,
		Docs:  bm.docs,
		IDF:   bm.idf,
		Avgdl: bm.avgdl,
	})
	return cw.n, err
}

func (bm *BM25) ReadFrom(r io.Reader) (int64, error) {
	cr := &countReader{r: r}
	var g bm25Gob
	if err := gob.NewDecoder(cr).Decode(&g); err != nil {
		return cr.n, err
	}
	bm.k1 = g.K1
	bm.b = g.B
	bm.docs = g.Docs
	bm.idf = g.IDF
	bm.avgdl = g.Avgdl
	return cr.n, nil
}

type bm25Doc struct {
	TF map[string]float64
	DL float64
}

type bm25Gob struct {
	K1    float64
	B     float64
	Docs  []bm25Doc
	IDF   map[string]float64
	Avgdl float64
}

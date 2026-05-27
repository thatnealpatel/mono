package ranking

import (
	"cmp"
	"encoding/gob"
	"io"
	"math"
	"slices"
	"strings"
)

type Tokenizer interface {
	Tokenize(text string) []string
}

type DefaultTokenizer struct{}

func (DefaultTokenizer) Tokenize(text string) []string {
	var tokens []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#*-_/\\")
		if len(w) < 2 || stopWords[w] {
			continue
		}
		tokens = append(tokens, w)
	}
	return tokens
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "in": true, "of": true,
	"to": true, "for": true, "is": true, "on": true, "and": true,
	"or": true, "with": true, "by": true, "at": true, "from": true,
	"that": true, "this": true, "it": true, "not": true, "but": true,
}

type BagOfWordsRanker interface {
	Build(docs []string)
	Search(query string) []*Result
	WriteTo(w io.Writer) error
	ReadFrom(r io.Reader) error
}

type Result struct {
	Index int
	Score float64
}

type TFIDF struct {
	tok  Tokenizer
	docs []tfidfDoc
	idf  map[string]float64
}

func NewTFIDF(p *TFIDFParams) *TFIDF {
	var tok Tokenizer = DefaultTokenizer{}
	if p != nil && p.Tokenizer != nil {
		tok = p.Tokenizer
	}
	return &TFIDF{tok: tok}
}

type TFIDFParams struct {
	Tokenizer Tokenizer
}

func (t *TFIDF) Build(docs []string) {
	docFreq := map[string]int{}
	n := float64(len(docs))
	t.docs = make([]tfidfDoc, len(docs))
	t.idf = map[string]float64{}

	for i, text := range docs {
		tf := t.termFreq(text)
		for term := range tf {
			docFreq[term]++
		}
		t.docs[i] = tfidfDoc{TF: tf}
	}

	for term, df := range docFreq {
		t.idf[term] = math.Log(1 + n/float64(df))
	}

	for i := range t.docs {
		var sum float64
		for term, freq := range t.docs[i].TF {
			w := freq * t.idf[term]
			sum += w * w
		}
		t.docs[i].Mag = math.Sqrt(sum)
	}
}

func (t *TFIDF) Search(query string) []*Result {
	qtf := t.termFreq(query)
	if len(qtf) == 0 {
		return nil
	}

	var qmag float64
	for term, freq := range qtf {
		w := freq * t.idf[term]
		qmag += w * w
	}
	qmag = math.Sqrt(qmag)
	if qmag == 0 {
		return nil
	}

	var results []*Result
	for i, doc := range t.docs {
		if doc.Mag == 0 {
			continue
		}
		var dot float64
		for term, qfreq := range qtf {
			if dfreq, ok := doc.TF[term]; ok {
				dot += (qfreq * t.idf[term]) * (dfreq * t.idf[term])
			}
		}
		score := dot / (qmag * doc.Mag)
		if score > 0.001 {
			results = append(results, &Result{Index: i, Score: score})
		}
	}

	slices.SortFunc(results, func(a, b *Result) int {
		return cmp.Compare(b.Score, a.Score)
	})
	return results
}

func (t *TFIDF) Similarity(i, j int) float64 {
	a, b := t.docs[i], t.docs[j]
	if a.Mag == 0 || b.Mag == 0 {
		return 0
	}
	var dot float64
	for term, af := range a.TF {
		if bf, ok := b.TF[term]; ok {
			dot += (af * t.idf[term]) * (bf * t.idf[term])
		}
	}
	return dot / (a.Mag * b.Mag)
}

func (t *TFIDF) WriteTo(w io.Writer) error {
	return gob.NewEncoder(w).Encode(tfidfGob{
		Docs: t.docs,
		IDF:  t.idf,
	})
}

func (t *TFIDF) ReadFrom(r io.Reader) error {
	var g tfidfGob
	if err := gob.NewDecoder(r).Decode(&g); err != nil {
		return err
	}
	t.docs = g.Docs
	t.idf = g.IDF
	return nil
}

type tfidfDoc struct {
	TF  map[string]float64
	Mag float64
}

type tfidfGob struct {
	Docs []tfidfDoc
	IDF  map[string]float64
}

func (t *TFIDF) termFreq(text string) map[string]float64 {
	tokens := t.tok.Tokenize(text)
	tf := make(map[string]float64, len(tokens))
	for _, tok := range tokens {
		tf[tok]++
	}
	return tf
}

type BM25 struct {
	tok   Tokenizer
	k1    float64
	b     float64
	docs  []bm25Doc
	idf   map[string]float64
	avgdl float64
}

func NewBM25(p *BM25Params) *BM25 {
	bm := &BM25{
		tok: DefaultTokenizer{},
		k1:  1.2,
		b:   0.75,
	}
	if p != nil {
		if p.Tokenizer != nil {
			bm.tok = p.Tokenizer
		}
		if p.K1 != 0 {
			bm.k1 = p.K1
		}
		if p.B != 0 {
			bm.b = p.B
		}
	}
	return bm
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
			score += idf * (tf * (bm.k1 + 1)) / (tf + bm.k1*(1-bm.b+bm.b*doc.DL/bm.avgdl))
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

func (bm *BM25) WriteTo(w io.Writer) error {
	return gob.NewEncoder(w).Encode(bm25Gob{
		K1:    bm.k1,
		B:     bm.b,
		Docs:  bm.docs,
		IDF:   bm.idf,
		Avgdl: bm.avgdl,
	})
}

func (bm *BM25) ReadFrom(r io.Reader) error {
	var g bm25Gob
	if err := gob.NewDecoder(r).Decode(&g); err != nil {
		return err
	}
	bm.k1 = g.K1
	bm.b = g.B
	bm.docs = g.Docs
	bm.idf = g.IDF
	bm.avgdl = g.Avgdl
	return nil
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

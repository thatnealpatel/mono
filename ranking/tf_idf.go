package ranking

import (
	"cmp"
	"encoding/gob"
	"io"
	"math"
	"slices"
)

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

func (t *TFIDF) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	err := gob.NewEncoder(cw).Encode(tfidfGob{
		Docs: t.docs,
		IDF:  t.idf,
	})
	return cw.n, err
}

func (t *TFIDF) ReadFrom(r io.Reader) (int64, error) {
	cr := &countReader{r: r}
	var g tfidfGob
	if err := gob.NewDecoder(cr).Decode(&g); err != nil {
		return cr.n, err
	}
	t.docs = g.Docs
	t.idf = g.IDF
	return cr.n, nil
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

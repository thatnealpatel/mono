// Package ranking implements various
// types of vector searchable formats
// that can be used in a variety of
// different programs.
package ranking

import (
	"io"
	"strings"
)

type Tokenizer interface {
	Tokenize(text string) []string
}

type DefaultTokenizer struct{}

func (DefaultTokenizer) Tokenize(text string) []string {
	var tokens []string
	for w := range strings.FieldsSeq(strings.ToLower(text)) {
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
	io.WriterTo
	io.ReaderFrom
}

type Result struct {
	Index int
	Score float64
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

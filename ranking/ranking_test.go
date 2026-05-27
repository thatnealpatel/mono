package ranking

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

var corpus = []string{
	"the quick brown fox jumps over the lazy dog",
	"a fast red car drives on the highway",
	"brown dogs and foxes play in the garden",
	"machine learning algorithms process large datasets",
	"natural language processing uses tokenization",
}

func TestTFIDFSearch(t *testing.T) {
	idx := NewTFIDF(nil)
	idx.Build(corpus)
	results := idx.Search("brown fox")
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Index != 0 {
		t.Fatalf("expected top result index 0, got %d", results[0].Index)
	}
}

func TestBM25Search(t *testing.T) {
	idx := NewBM25(nil)
	idx.Build(corpus)
	results := idx.Search("brown fox")
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Index != 0 {
		t.Fatalf("expected top result index 0, got %d", results[0].Index)
	}
}

func TestSearchSortedByScore(t *testing.T) {
	idx := NewTFIDF(nil)
	idx.Build(corpus)
	results := idx.Search("brown dog")
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Fatalf("results not sorted: %f > %f at index %d", results[i].Score, results[i-1].Score, i)
		}
	}
}

func TestSearchNoMatch(t *testing.T) {
	idx := NewTFIDF(nil)
	idx.Build(corpus)
	results := idx.Search("xyzzyplugh")
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestEmptyCorpus(t *testing.T) {
	for _, name := range []string{"tfidf-nil", "tfidf-empty", "bm25-nil", "bm25-empty"} {
		t.Run(name, func(t *testing.T) {
			var r BagOfWordsRanker
			switch name {
			case "tfidf-nil":
				idx := NewTFIDF(nil)
				idx.Build(nil)
				r = idx
			case "tfidf-empty":
				idx := NewTFIDF(nil)
				idx.Build([]string{})
				r = idx
			case "bm25-nil":
				idx := NewBM25(nil)
				idx.Build(nil)
				r = idx
			case "bm25-empty":
				idx := NewBM25(nil)
				idx.Build([]string{})
				r = idx
			}
			results := r.Search("anything")
			if len(results) != 0 {
				t.Fatalf("expected no results, got %d", len(results))
			}
		})
	}
}

func TestSimilarity(t *testing.T) {
	idx := NewTFIDF(nil)
	idx.Build([]string{
		"the quick brown fox",
		"the quick brown fox",
		"machine learning algorithms datasets",
	})
	identical := idx.Similarity(0, 1)
	if math.Abs(identical-1.0) > 0.001 {
		t.Fatalf("expected similarity ~1.0, got %f", identical)
	}
	unrelated := idx.Similarity(0, 2)
	if unrelated > 0.1 {
		t.Fatalf("expected similarity ~0, got %f", unrelated)
	}
}

func TestTFIDFWriteToReadFrom(t *testing.T) {
	idx := NewTFIDF(nil)
	idx.Build(corpus)
	original := idx.Search("brown fox")

	var buf bytes.Buffer
	if err := idx.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	idx2 := NewTFIDF(nil)
	if err := idx2.ReadFrom(&buf); err != nil {
		t.Fatal(err)
	}
	restored := idx2.Search("brown fox")

	if len(original) != len(restored) {
		t.Fatalf("result count mismatch: %d vs %d", len(original), len(restored))
	}
	for i := range original {
		if original[i].Index != restored[i].Index {
			t.Fatalf("index mismatch at %d: %d vs %d", i, original[i].Index, restored[i].Index)
		}
		if math.Abs(original[i].Score-restored[i].Score) > 1e-9 {
			t.Fatalf("score mismatch at %d: %f vs %f", i, original[i].Score, restored[i].Score)
		}
	}
}

func TestBM25WriteToReadFrom(t *testing.T) {
	idx := NewBM25(nil)
	idx.Build(corpus)
	original := idx.Search("brown fox")

	var buf bytes.Buffer
	if err := idx.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	idx2 := NewBM25(nil)
	if err := idx2.ReadFrom(&buf); err != nil {
		t.Fatal(err)
	}
	restored := idx2.Search("brown fox")

	if len(original) != len(restored) {
		t.Fatalf("result count mismatch: %d vs %d", len(original), len(restored))
	}
	for i := range original {
		if original[i].Index != restored[i].Index {
			t.Fatalf("index mismatch at %d: %d vs %d", i, original[i].Index, restored[i].Index)
		}
		if math.Abs(original[i].Score-restored[i].Score) > 1e-9 {
			t.Fatalf("score mismatch at %d: %f vs %f", i, original[i].Score, restored[i].Score)
		}
	}
}

type upperTokenizer struct{}

func (upperTokenizer) Tokenize(text string) []string {
	return strings.Fields(strings.ToUpper(text))
}

func TestCustomTokenizer(t *testing.T) {
	idx := NewTFIDF(&TFIDFParams{Tokenizer: upperTokenizer{}})
	idx.Build([]string{"hello world", "goodbye world"})
	results := idx.Search("HELLO")
	if len(results) == 0 {
		t.Fatal("expected results with custom tokenizer")
	}
	if results[0].Index != 0 {
		t.Fatalf("expected index 0, got %d", results[0].Index)
	}
}

func TestNilParams(t *testing.T) {
	_ = NewTFIDF(nil)
	_ = NewBM25(nil)
}

func TestBM25CustomParams(t *testing.T) {
	def := NewBM25(nil)
	def.Build(corpus)
	defResults := def.Search("brown fox")

	custom := NewBM25(&BM25Params{K1: 2.0, B: 0.5})
	custom.Build(corpus)
	customResults := custom.Search("brown fox")

	if len(defResults) == 0 || len(customResults) == 0 {
		t.Fatal("expected results from both")
	}
	if defResults[0].Score == customResults[0].Score {
		t.Fatal("expected different scores with different params")
	}
}

var (
	_ BagOfWordsRanker = (*TFIDF)(nil)
	_ BagOfWordsRanker = (*BM25)(nil)
)

package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"patel.codes/indexing"
)

func TestIsExactQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{query: "GF", want: true},
		{query: "sage.all.GF", want: true},
		{query: "finite field", want: false},
		{query: "finite\tfield", want: false},
		{query: "finite\nfield", want: false},
		{query: "unicode\u00a0space", want: false},
	}
	for _, test := range tests {
		if got := isExactQuery(test.query); got != test.want {
			t.Errorf("isExactQuery(%q) = %v, want %v", test.query, got, test.want)
		}
	}
}

func TestRunQueryEnvelopeShapes(t *testing.T) {
	index := queryTestIndex(t)

	t.Run("exact", func(t *testing.T) {
		value := runQueryJSON(t, index, "GF", false)
		if value["mode"] != "exact" {
			t.Fatalf("mode = %#v", value["mode"])
		}
		if _, ok := value["candidates"]; ok {
			t.Error("exact response contains candidates")
		}
		matches := value["matches"].([]any)
		match := matches[0].(map[string]any)
		if match["docstring"] != "Construct a finite field." {
			t.Fatalf("docstring = %#v", match["docstring"])
		}
		for _, absent := range []string{"snippet", "body", "file", "line", "score"} {
			if _, ok := match[absent]; ok {
				t.Errorf("exact response contains empty optional field %q", absent)
			}
		}
	})

	t.Run("miss", func(t *testing.T) {
		value := runQueryJSON(t, index, "gf", true)
		if value["mode"] != "miss" {
			t.Fatalf("mode = %#v", value["mode"])
		}
		if got := len(value["matches"].([]any)); got != 0 {
			t.Fatalf("matches length = %d", got)
		}
		candidates := value["candidates"].([]any)
		if len(candidates) != 1 || candidates[0] != "GF" {
			t.Fatalf("candidates = %#v", candidates)
		}
	})

	t.Run("search score visibility", func(t *testing.T) {
		plain := runQueryJSON(t, index, "finite field", false)
		plainMatch := plain["matches"].([]any)[0].(map[string]any)
		if _, ok := plainMatch["docstring"]; ok {
			t.Error("search response contains full docstring")
		}
		if _, ok := plainMatch["snippet"]; !ok {
			t.Error("search response omits snippet")
		}
		if _, ok := plainMatch["score"]; ok {
			t.Error("non-verbose search response contains score")
		}

		verbose := runQueryJSON(t, index, "finite field", true)
		verboseMatch := verbose["matches"].([]any)[0].(map[string]any)
		if score, ok := verboseMatch["score"].(float64); !ok || score <= 0 {
			t.Fatalf("verbose score = %#v, want positive number", verboseMatch["score"])
		}
	})
}

func queryTestIndex(t *testing.T) *indexing.Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	builder, err := indexing.Create(path, SageTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = builder.Abort() })
	if err := builder.Add(indexing.Record{
		Name:      "GF",
		Kind:      "object",
		Docstring: "Construct a finite field.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := builder.Close(); err != nil {
		t.Fatal(err)
	}
	index, err := indexing.Open(path, SageTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return index
}

func runQueryJSON(t *testing.T, index *indexing.Index, query string, verbose bool) map[string]any {
	t.Helper()
	var output bytes.Buffer
	if err := runQuery(&output, index, query, verbose); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatalf("decode %q: %v", output.String(), err)
	}
	return value
}

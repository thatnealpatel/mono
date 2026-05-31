package main

import (
	"slices"
	"testing"
)

func TestTokenize(t *testing.T) {
	tok := LeanTokenizer{}
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "dotted name",
			input: "List.map",
			want:  []string{"list", "map", "list.map"},
		},
		{
			name:  "camel case",
			input: "ContinuousOn",
			want:  []string{"continuouson", "continuous", "on"},
		},
		{
			name:  "underscore split",
			input: "map_append",
			want:  []string{"map", "append", "map_append"},
		},
		{
			name:  "dotted and camel",
			input: "Mathlib.Topology.ContinuousOn",
			want:  []string{"mathlib", "topology", "continuouson", "continuous", "on", "mathlib.topology.continuouson"},
		},
		{
			name:  "backtick wrapped",
			input: "`unit`",
			want:  []string{"unit"},
		},
		{
			name:  "paren wrapped",
			input: "(alpha)",
			want:  []string{"alpha"},
		},
		{
			name:  "stop words filtered",
			input: "by where let have sorry",
			want:  nil,
		},
		{
			name:  "symbolic filtered",
			input: "→ ← ↔ ∀ ∃ ∈ := ≤ ≥",
			want:  nil,
		},
		{
			name:  "short tokens filtered",
			input: "a x",
			want:  nil,
		},
		{
			name:  "mixed real signature",
			input: "theorem AccPt.nhds_inter {x : α}",
			want:  []string{"theorem", "accpt", "acc", "pt", "nhds", "inter", "nhds_inter", "accpt.nhds_inter"},
		},
		{
			name:  "all uppercase",
			input: "TFIDF BM25",
			want:  []string{"tfidf", "bm25"},
		},
		{
			name:  "underscore prefix",
			input: "_root_.Nat",
			want:  []string{"root", "nat", "root_.nat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.Tokenize(tt.input)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Tokenize(%q)\n got: %v\nwant: %v", tt.input, got, tt.want)
			}
		})
	}
}

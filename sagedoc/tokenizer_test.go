package main

import (
	"slices"
	"testing"
)

func TestSageTokenizerIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "dotted",
			text: "sage.categories.FiniteFields",
			want: []string{"sage", "categories", "finitefields", "finite", "fields", "sage.categories.finitefields"},
		},
		{
			name: "snake_case",
			text: "is_prime_power",
			want: []string{"is", "prime", "power", "is_prime_power"},
		},
		{
			name: "CamelCase",
			text: "MatrixSpace",
			want: []string{"matrixspace", "matrix", "space"},
		},
		{
			name: "acronyms",
			text: "HTTPSageAPIClient",
			want: []string{"httpsageapiclient", "http", "sage", "api", "client"},
		},
		{
			name: "surrounding punctuation and multiple words",
			text: "(sage.matrix.MatrixSpace!), [GF] {foo-bar}",
			want: []string{
				"sage", "matrix", "matrixspace", "matrix", "space", "sage.matrix.matrixspace",
				"gf", "foo-bar",
			},
		},
		{
			name: "Unicode case boundaries",
			text: "ÜberGröße.Δelta_λValue",
			want: []string{
				"übergröße", "über", "größe",
				"δelta", "λvalue", "value", "δelta_λvalue",
				"übergröße.δelta_λvalue",
			},
		},
		{
			name: "one-rune fragments omitted but complete forms retained",
			text: "x λ _y_ z.q A_B",
			want: []string{"z.q", "a_b"},
		},
		{
			name: "empty",
			text: "",
			want: nil,
		},
		{
			name: "whitespace and punctuation only",
			text: " \t ... ___ !!! ",
			want: nil,
		},
	}

	tokenizer := SageTokenizer{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := tokenizer.Tokenize(test.text)
			if !slices.Equal(got, test.want) {
				t.Fatalf("Tokenize(%q) = %#v, want %#v", test.text, got, test.want)
			}
		})
	}
}

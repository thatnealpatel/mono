package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type LeanTokenizer struct{}

func (LeanTokenizer) Tokenize(text string) []string {
	var tokens []string
	for w := range strings.FieldsSeq(text) {
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#*-_/\\@<>«»⟨⟩→←↔⇒∀∃∈∉⊆⊇∩∪")
		wLower := strings.ToLower(w)
		if !isWord(wLower) || leanStopWords[wLower] {
			continue
		}
		hasDot := strings.Contains(w, ".")
		for part := range strings.SplitSeq(w, ".") {
			part = strings.Trim(part, "_")
			partLower := strings.ToLower(part)
			if !isWord(partLower) || leanStopWords[partLower] {
				continue
			}
			hasUnderscore := strings.Contains(part, "_")
			if hasUnderscore {
				for sub := range strings.SplitSeq(part, "_") {
					subLower := strings.ToLower(sub)
					if !isWord(subLower) || leanStopWords[subLower] {
						continue
					}
					tokens = addCamelTokens(tokens, sub)
				}
				tokens = append(tokens, partLower)
			} else {
				tokens = addCamelTokens(tokens, part)
			}
		}
		if hasDot {
			tokens = append(tokens, wLower)
		}
	}
	return tokens
}

func addCamelTokens(tokens []string, s string) []string {
	camel := splitCamel(s)
	lower := strings.ToLower(s)
	if len(camel) > 1 {
		tokens = append(tokens, lower)
	}
	for _, c := range camel {
		if isWord(c) && !leanStopWords[c] {
			tokens = append(tokens, c)
		}
	}
	return tokens
}

func splitCamel(s string) []string {
	var parts []string
	var buf strings.Builder
	var prevLower bool
	for _, r := range s {
		if unicode.IsUpper(r) && prevLower && buf.Len() > 0 {
			parts = append(parts, buf.String())
			buf.Reset()
		}
		buf.WriteRune(unicode.ToLower(r))
		prevLower = unicode.IsLower(r)
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

func isWord(s string) bool {
	if utf8.RuneCountInString(s) < 2 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Hand-curated; should be replaced
// with corpus-derived frequency cutoff.
var leanStopWords = map[string]bool{
	"an": true, "the": true, "in": true, "of": true,
	"to": true, "for": true, "is": true, "and": true,
	"or": true, "with": true, "by": true, "at": true, "from": true,
	"that": true, "this": true, "it": true, "not": true, "but": true,
	"if": true, "then": true, "else": true, "do": true,
	"let": true, "have": true, "show": true, "fun": true,
	"match": true, "return": true, "where": true,
	"import": true, "open": true, "section": true,
	"end": true, "namespace": true, "variable": true,
	"sorry": true, "rfl": true,
	"set_option": true, "deriving": true,
}

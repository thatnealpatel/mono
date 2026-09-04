package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SageTokenizer exposes dotted,
// snake_case, and CamelCase identifier
// structure to indexing while leaving
// prose tokenization to SQLite's porter
// unicode61 tokenizer.
type SageTokenizer struct{}

func (SageTokenizer) Tokenize(text string) []string {
	var tokens []string
	for word := range strings.FieldsSeq(text) {
		word = strings.TrimFunc(word, surroundingPunctuation)
		if word == "" {
			continue
		}

		hasDot := strings.ContainsRune(word, '.')
		for part := range strings.SplitSeq(word, ".") {
			part = strings.Trim(part, "_")
			if part == "" {
				continue
			}
			hasUnderscore := strings.ContainsRune(part, '_')
			if hasUnderscore {
				for component := range strings.SplitSeq(part, "_") {
					tokens = appendCamelTokens(tokens, component)
				}
				if token := strings.ToLower(part); usefulToken(token) {
					tokens = append(tokens, token)
				}
			} else {
				tokens = appendCamelTokens(tokens, part)
			}
		}
		if hasDot {
			if token := strings.ToLower(word); usefulToken(token) {
				tokens = append(tokens, token)
			}
		}
	}
	return tokens
}

func surroundingPunctuation(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func appendCamelTokens(tokens []string, value string) []string {
	if value == "" {
		return tokens
	}
	parts := splitCamel(value)
	lower := strings.ToLower(value)
	if len(parts) > 1 && usefulToken(lower) {
		tokens = append(tokens, lower)
	}
	for _, part := range parts {
		if usefulToken(part) {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func splitCamel(value string) []string {
	runes := []rune(value)
	if len(runes) == 0 {
		return nil
	}
	var parts []string
	start := 0
	for i := 1; i < len(runes); i++ {
		currentUpper := unicode.IsUpper(runes[i])
		previousLowerOrDigit := unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])
		nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
		previousUpper := unicode.IsUpper(runes[i-1])
		if currentUpper && (previousLowerOrDigit || previousUpper && nextLower) {
			parts = append(parts, strings.ToLower(string(runes[start:i])))
			start = i
		}
	}
	parts = append(parts, strings.ToLower(string(runes[start:])))
	return parts
}

func usefulToken(token string) bool {
	if utf8.RuneCountInString(token) < 2 {
		return false
	}
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Declaration struct {
	Name      string
	Kind      string
	Signature string
	Docstring string
	File      string
	Line      int
}

func (d *Declaration) DocText() string {
	var buf strings.Builder
	buf.WriteString(d.Name)
	buf.WriteByte(' ')
	buf.WriteString(d.Signature)
	if d.Docstring != "" {
		buf.WriteByte(' ')
		buf.WriteString(d.Docstring)
	}
	return buf.String()
}

var declKeywords = map[string]bool{
	"theorem": true, "lemma": true, "def": true, "abbrev": true,
	"instance": true, "structure": true, "class": true,
	"inductive": true, "axiom": true, "opaque": true,
}

var declModifiers = map[string]bool{
	"private": true, "protected": true, "unsafe": true,
	"noncomputable": true, "partial": true, "meta": true, "scoped": true,
}

func ExtractFile(path string, src []byte) []Declaration {
	lines := strings.Split(string(src), "\n")
	var (
		decls   []Declaration
		nsStack []string
		docBuf  strings.Builder
		hasDoc  bool
		inDoc   bool
	)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inDoc {
			if idx := strings.Index(line, "-/"); idx >= 0 {
				docBuf.WriteString(strings.TrimSpace(line[:idx]))
				inDoc = false
				hasDoc = true
			} else {
				docBuf.WriteString(trimmed)
				docBuf.WriteByte('\n')
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/--") {
			rest := trimmed[3:]
			docBuf.Reset()
			if idx := strings.Index(rest, "-/"); idx >= 0 {
				docBuf.WriteString(strings.TrimSpace(rest[:idx]))
				hasDoc = true
			} else {
				docBuf.WriteString(strings.TrimSpace(rest))
				docBuf.WriteByte('\n')
				inDoc = true
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/-") || strings.HasPrefix(trimmed, "--") {
			continue
		}

		if strings.HasPrefix(trimmed, "namespace ") {
			ns := strings.Fields(trimmed)[1]
			nsStack = append(nsStack, ns)
			docBuf.Reset()
			hasDoc = false
			continue
		}
		if trimmed == "end" || strings.HasPrefix(trimmed, "end ") {
			if len(nsStack) > 0 {
				nsStack = nsStack[:len(nsStack)-1]
			}
			continue
		}

		if strings.HasPrefix(trimmed, "@[") ||
			strings.HasPrefix(trimmed, "section") ||
			strings.HasPrefix(trimmed, "variable") ||
			strings.HasPrefix(trimmed, "open ") ||
			strings.HasPrefix(trimmed, "set_option") ||
			strings.HasPrefix(trimmed, "module") ||
			strings.HasPrefix(trimmed, "import ") ||
			strings.HasPrefix(trimmed, "export ") ||
			strings.HasPrefix(trimmed, "public ") ||
			strings.HasPrefix(trimmed, "noncomputable section") {
			continue
		}

		if trimmed == "" {
			docBuf.Reset()
			hasDoc = false
			continue
		}

		kind, name := matchDecl(trimmed)
		if kind == "" {
			continue
		}

		fqn := qualifyName(nsStack, name)
		sig := extractSignature(lines, i)

		var doc string
		if hasDoc {
			doc = strings.TrimSpace(docBuf.String())
		}

		decls = append(decls, Declaration{
			Name:      fqn,
			Kind:      kind,
			Signature: sig,
			Docstring: doc,
			File:      path,
			Line:      i + 1,
		})
		docBuf.Reset()
		hasDoc = false
	}

	return decls
}

func matchDecl(line string) (kind, name string) {
	words := strings.Fields(line)
	i := 0
	for i < len(words) && declModifiers[words[i]] {
		i++
	}
	if i >= len(words) || !declKeywords[words[i]] {
		return "", ""
	}
	kind = words[i]
	i++
	if i < len(words) && isLeanIdent(words[i]) {
		name = words[i]
	}
	return kind, name
}

func isLeanIdent(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r) || r == '_'
}

func qualifyName(nsStack []string, name string) string {
	if len(nsStack) == 0 {
		return name
	}
	prefix := strings.Join(nsStack, ".")
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

func extractSignature(lines []string, start int) string {
	var buf strings.Builder
	for i := start; i < len(lines) && i-start < 15; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break
		}
		if i > start && len(lines[i]) > 0 && lines[i][0] != ' ' && lines[i][0] != '\t' {
			break
		}

		if idx := strings.Index(trimmed, ":="); idx >= 0 {
			before := strings.TrimSpace(trimmed[:idx])
			if before != "" {
				if buf.Len() > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(before)
			}
			return strings.TrimSpace(buf.String())
		}

		for _, marker := range []string{" where", " by"} {
			if strings.HasSuffix(trimmed, marker) || trimmed == strings.TrimPrefix(marker, " ") {
				before := strings.TrimSpace(strings.TrimSuffix(trimmed, marker))
				if before != "" && before != "by" && before != "where" {
					if buf.Len() > 0 {
						buf.WriteByte(' ')
					}
					buf.WriteString(before)
				}
				return strings.TrimSpace(buf.String())
			}
		}

		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(trimmed)
	}
	return strings.TrimSpace(buf.String())
}

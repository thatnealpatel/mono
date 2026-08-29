package markdown

import (
	"errors"
	"fmt"
	"strings"

	"patel.codes/render/internal/mathml"
)

var ErrUnclosedMath = errors.New("unclosed math delimiter")

type InlineMath struct {
	Expr  string
	Open  string
	Close string
}

func (*InlineMath) Inline() {}

func (m *InlineMath) printText(p *printer) { p.text(m.Expr) }

func (m *InlineMath) printHTML(p *printer) {
	html, err := mathml.Render(strings.TrimSpace(m.Expr), false)
	if err != nil {
		p.setError(err)
		p.text(m.Open, m.Expr, m.Close)
		return
	}
	p.html(html)
}

func (m *InlineMath) printMarkdown(p *printer) {
	p.md(m.Open, m.Expr, m.Close)
}

type DisplayMath struct {
	Position
	Expr   string
	Open   string
	Close  string
	Closed bool
}

func (*DisplayMath) Block() {}

func (m *DisplayMath) printHTML(p *printer) {
	if !m.Closed {
		p.setError(fmt.Errorf("%w: %s", ErrUnclosedMath, m.Open))
		p.text(m.Open, m.Expr)
		return
	}
	html, err := mathml.Render(strings.TrimSpace(m.Expr), true)
	if err != nil {
		p.setError(err)
		p.text(m.Open, m.Expr, m.Close)
		return
	}
	p.html(html, "\n")
}

func (m *DisplayMath) printMarkdown(p *printer) {
	p.maybeNL()
	p.md(m.Open)
	if m.Expr != "" {
		p.nl()
		for i, line := range strings.Split(m.Expr, "\n") {
			if i > 0 {
				p.nl()
			}
			p.md(line)
			p.noTrim()
		}
	}
	if m.Closed {
		p.nl()
		p.md(m.Close)
	}
}

func parseInlineMath(_ *parser, source string, start int) (Inline, int, bool) {
	if escapedAt(source, start) {
		return nil, 0, false
	}
	if source[start] == '$' {
		if (start > 0 && source[start-1] == '$') || start+1 >= len(source) || source[start+1] == '$' {
			return nil, 0, false
		}
		for end := start + 1; end < len(source); end++ {
			if source[end] != '$' || escapedAt(source, end) {
				continue
			}
			if end == start+1 || (end+1 < len(source) && source[end+1] == '$') {
				return nil, 0, false
			}
			return &InlineMath{Expr: source[start+1 : end], Open: "$", Close: "$"}, end + 1, true
		}
		return nil, 0, false
	}
	if !strings.HasPrefix(source[start:], `\(`) {
		return nil, 0, false
	}
	for end := start + 2; end+1 < len(source); end++ {
		if source[end] == '\\' && source[end+1] == ')' && !escapedAt(source, end) {
			if end == start+2 {
				return nil, 0, false
			}
			return &InlineMath{Expr: source[start+2 : end], Open: `\(`, Close: `\)`}, end + 2, true
		}
	}
	return nil, 0, false
}

func escapedAt(source string, index int) bool {
	backslashes := 0
	for index > 0 && source[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 != 0
}

type displayMathBuilder struct {
	open   string
	close  string
	text   []string
	closed bool
}

func startDisplayMath(p *parser, sourceLine line) (line, bool) {
	if !p.Math {
		return sourceLine, false
	}
	trimmed := sourceLine
	trimmed.trimSpace(0, 3, false)
	source := trimmed.string()
	var open, close string
	switch {
	case strings.HasPrefix(source, "$$"):
		open, close = "$$", "$$"
	case strings.HasPrefix(source, `\[`):
		open, close = `\[`, `\]`
	default:
		return sourceLine, false
	}

	builder := &displayMathBuilder{open: open, close: close}
	rest := source[len(open):]
	if before, ok := displayClose(rest, close); ok {
		builder.text = append(builder.text, before)
		builder.closed = true
	} else if rest != "" {
		builder.text = append(builder.text, rest)
	}
	p.addBlock(builder)
	return line{}, true
}

func (b *displayMathBuilder) extend(_ *parser, sourceLine line) (line, bool) {
	if b.closed {
		return sourceLine, false
	}
	if before, ok := displayClose(sourceLine.string(), b.close); ok {
		b.text = append(b.text, before)
		b.closed = true
		return line{}, false
	}
	b.text = append(b.text, sourceLine.string())
	return line{}, true
}

func (b *displayMathBuilder) build(p *parser) Block {
	return &DisplayMath{
		Position: p.pos(),
		Expr:     strings.Join(b.text, "\n"),
		Open:     b.open,
		Close:    b.close,
		Closed:   b.closed,
	}
}

func displayClose(source, close string) (string, bool) {
	end := strings.LastIndex(source, close)
	if end < 0 || escapedAt(source, end) || strings.TrimSpace(source[end+len(close):]) != "" {
		return "", false
	}
	return source[:end], true
}

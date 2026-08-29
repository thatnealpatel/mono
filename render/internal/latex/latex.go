package latex

import (
	"errors"
	"fmt"
	"unicode"
)

var ErrInvalid = errors.New("invalid LaTeX")

type Node interface{ Node() }

type List []Node

func (List) Node() {}

type Letter string

func (Letter) Node() {}

type Number string

func (Number) Node() {}

type Operator string

func (Operator) Node() {}

type Space struct{}

func (Space) Node() {}

type Command struct {
	Name    string
	Args    [][]Node
	OptArgs [][]Node
}

func (Command) Node() {}

type Sup struct {
	Base   Node
	Script Node
}

func (Sup) Node() {}

type Sub struct {
	Base   Node
	Script Node
}

func (Sub) Node() {}

type SubSup struct {
	Base Node
	Sub  Node
	Sup  Node
}

func (SubSup) Node() {}

type Delimited struct {
	Open  string
	Close string
	Body  []Node
}

func (Delimited) Node() {}

type Env struct {
	Name string
	Body []Node
}

func (Env) Node() {}

type parser struct {
	input []rune
	pos   int
}

func Parse(expr string) ([]Node, error) {
	p := &parser{input: []rune(expr)}
	nodes, err := p.parseUntil(0)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing %q: %v", ErrInvalid, expr, err)
	}
	return nodes, nil
}

func (p *parser) parseUntil(stop rune) ([]Node, error) {
	var nodes []Node
	for p.pos < len(p.input) {
		if stop != 0 && p.input[p.pos] == stop {
			return nodes, nil
		}
		node, err := p.parseItem()
		if err != nil {
			return nil, err
		}
		if _, ok := node.(Space); !ok {
			node, err = p.parseScripts(node)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	if stop != 0 {
		return nil, fmt.Errorf("missing %q", stop)
	}
	return nodes, nil
}

func (p *parser) parseItem() (Node, error) {
	ch := p.input[p.pos]
	switch {
	case ch == '{':
		return p.parseGroup()
	case ch == '}':
		return nil, errors.New("unexpected closing brace")
	case ch == '\\':
		return p.parseCommand()
	case unicode.IsLetter(ch):
		p.pos++
		return Letter(ch), nil
	case unicode.IsDigit(ch):
		return p.parseNumber(), nil
	case unicode.IsSpace(ch):
		p.pos++
		return Space{}, nil
	default:
		p.pos++
		return Operator(ch), nil
	}
}

func (p *parser) parseAtom() (Node, error) {
	if p.pos >= len(p.input) {
		return nil, errors.New("missing script after ^ or _")
	}
	if p.input[p.pos] == '{' {
		return p.parseGroup()
	}
	if p.input[p.pos] == '\\' {
		return p.parseCommand()
	}
	if p.input[p.pos] == '}' {
		return nil, errors.New("unexpected closing brace")
	}
	ch := p.input[p.pos]
	p.pos++
	if unicode.IsDigit(ch) {
		return Number(ch), nil
	}
	if unicode.IsLetter(ch) {
		return Letter(ch), nil
	}
	return Operator(ch), nil
}

func (p *parser) parseGroup() (Node, error) {
	p.pos++
	nodes, err := p.parseUntil('}')
	if err != nil {
		return nil, err
	}
	p.pos++
	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return List(nodes), nil
}

func (p *parser) parseCommand() (Node, error) {
	p.pos++
	if p.pos >= len(p.input) {
		return Operator(`\`), nil
	}
	if !unicode.IsLetter(p.input[p.pos]) {
		ch := p.input[p.pos]
		p.pos++
		return Operator(string([]rune{'\\', ch})), nil
	}

	start := p.pos
	for p.pos < len(p.input) && unicode.IsLetter(p.input[p.pos]) {
		p.pos++
	}
	name := `\` + string(p.input[start:p.pos])

	switch name {
	case `\left`:
		return p.parseDelimited()
	case `\right`:
		return nil, errors.New(`unexpected \right without \left`)
	case `\begin`:
		return p.parseEnv()
	case `\end`:
		return nil, errors.New(`unexpected \end without \begin`)
	case `\bigl`, `\bigr`:
		delimiter, err := p.parseDelimiter(name)
		if err != nil {
			return nil, err
		}
		return Command{Name: name, Args: [][]Node{{Operator(delimiter)}}}, nil
	case `\middle`:
		return nil, errors.New(`unexpected \middle without \left`)
	}

	nargs, known := commandArgCount(name)
	if !known {
		return Command{Name: name}, nil
	}
	cmd := Command{Name: name}
	if name == `\sqrt` && p.pos < len(p.input) && p.input[p.pos] == '[' {
		p.pos++
		arg, err := p.parseUntil(']')
		if err != nil {
			return nil, err
		}
		p.pos++
		cmd.OptArgs = append(cmd.OptArgs, arg)
	}
	for range nargs {
		arg, err := p.parseCommandArg(name)
		if err != nil {
			return nil, err
		}
		cmd.Args = append(cmd.Args, arg)
	}
	return cmd, nil
}

func (p *parser) parseDelimiter(name string) (string, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("missing delimiter after %s", name)
	}
	return p.readDelim(), nil
}

func (p *parser) parseCommandArg(name string) ([]Node, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("missing argument for %s", name)
	}
	if p.input[p.pos] == '{' {
		p.pos++
		arg, err := p.parseUntil('}')
		if err != nil {
			return nil, err
		}
		p.pos++
		return arg, nil
	}
	node, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	return []Node{node}, nil
}

func commandArgCount(name string) (int, bool) {
	switch name {
	case `\frac`, `\dfrac`, `\tfrac`, `\binom`:
		return 2, true
	case `\sqrt`, `\overline`, `\underline`, `\hat`, `\boxed`, `\xmapsto`,
		`\bar`, `\vec`, `\dot`, `\ddot`, `\tilde`, `\text`, `\textit`,
		`\textbf`, `\textmd`, `\textrm`, `\mathrm`, `\mathbf`, `\mathit`,
		`\operatorname`, `\mathcal`, `\mathbb`, `\mathfrak`, `\mod`, `\pmod`,
		`\bmod`, `\eqref`, `\label`, `\tag`, `\substack`, `\not`, `\centernot`:
		return 1, true
	default:
		return 0, false
	}
}

func (p *parser) parseNumber() Node {
	start := p.pos
	for p.pos < len(p.input) && unicode.IsDigit(p.input[p.pos]) {
		p.pos++
	}
	return Number(p.input[start:p.pos])
}

func (p *parser) parseDelimited() (Node, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, errors.New(`missing delimiter after \left`)
	}
	open := p.readDelim()
	var nodes []Node
	for p.pos < len(p.input) {
		if p.hasControlWord(`\middle`) {
			p.pos += len([]rune(`\middle`))
			delimiter, err := p.parseDelimiter(`\middle`)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, Command{Name: `\middle`, Args: [][]Node{{Operator(delimiter)}}})
			continue
		}
		if p.hasControlWord(`\right`) {
			p.pos += len([]rune(`\right`))
			p.skipSpaces()
			if p.pos >= len(p.input) {
				return nil, errors.New(`missing delimiter after \right`)
			}
			return Delimited{Open: open, Close: p.readDelim(), Body: nodes}, nil
		}
		node, err := p.parseItem()
		if err != nil {
			return nil, err
		}
		if _, ok := node.(Space); !ok {
			node, err = p.parseScripts(node)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	return nil, errors.New(`\left without matching \right`)
}

func (p *parser) readDelim() string {
	if p.pos >= len(p.input) {
		return "."
	}
	start := p.pos
	if p.input[p.pos] == '\\' && p.pos+1 < len(p.input) {
		p.pos++
		if unicode.IsLetter(p.input[p.pos]) {
			for p.pos < len(p.input) && unicode.IsLetter(p.input[p.pos]) {
				p.pos++
			}
			return string(p.input[start:p.pos])
		}
		p.pos++
		return string(p.input[start:p.pos])
	}
	p.pos++
	return string(p.input[start:p.pos])
}

func (p *parser) parseEnv() (Node, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '{' {
		return nil, errors.New(`missing environment name after \begin`)
	}
	p.pos++
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '}' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return nil, errors.New(`unterminated environment name`)
	}
	name := string(p.input[start:p.pos])
	p.pos++
	end := `\end{` + name + `}`
	var nodes []Node
	for p.pos < len(p.input) {
		if p.hasPrefix(end) {
			p.pos += len([]rune(end))
			return Env{Name: name, Body: nodes}, nil
		}
		node, err := p.parseItem()
		if err != nil {
			return nil, err
		}
		if _, ok := node.(Space); !ok {
			node, err = p.parseScripts(node)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	return nil, fmt.Errorf(`\begin{%s} without matching \end{%s}`, name, name)
}

func (p *parser) parseScripts(node Node) (Node, error) {
	saved := p.pos
	p.skipSpaces()
	if p.pos >= len(p.input) || (p.input[p.pos] != '^' && p.input[p.pos] != '_') {
		p.pos = saved
		return node, nil
	}
	first := p.input[p.pos]
	p.pos++
	script, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	afterFirst := p.pos
	p.skipSpaces()
	if p.pos < len(p.input) && ((first == '^' && p.input[p.pos] == '_') || (first == '_' && p.input[p.pos] == '^')) {
		p.pos++
		second, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		if first == '^' {
			return SubSup{Base: node, Sub: second, Sup: script}, nil
		}
		return SubSup{Base: node, Sub: script, Sup: second}, nil
	}
	p.pos = afterFirst
	if first == '^' {
		return Sup{Base: node, Script: script}, nil
	}
	return Sub{Base: node, Script: script}, nil
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.input) && unicode.IsSpace(p.input[p.pos]) {
		p.pos++
	}
}

func (p *parser) hasPrefix(s string) bool {
	prefix := []rune(s)
	return p.pos+len(prefix) <= len(p.input) && string(p.input[p.pos:p.pos+len(prefix)]) == s
}

func (p *parser) hasControlWord(s string) bool {
	word := []rune(s)
	if !p.hasPrefix(s) {
		return false
	}
	end := p.pos + len(word)
	return end == len(p.input) || !unicode.IsLetter(p.input[end])
}

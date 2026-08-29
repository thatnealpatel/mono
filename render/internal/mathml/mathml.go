package mathml

import (
	"bytes"
	"html"

	"patel.codes/render/internal/latex"
)

func Render(expr string, display bool) (string, error) {
	nodes, err := latex.Parse(expr)
	if err != nil {
		return "", err
	}
	var w writer
	if display {
		w.raw(`<math display="block">`)
	} else {
		w.raw(`<math>`)
	}
	w.nodes(nodes)
	w.raw(`</math>`)
	return w.String(), nil
}

type writer struct{ bytes.Buffer }

func (w *writer) raw(s string)  { w.WriteString(s) }
func (w *writer) text(s string) { w.WriteString(html.EscapeString(s)) }

func (w *writer) nodes(nodes []latex.Node) {
	if len(nodes) == 0 {
		w.raw("<mrow></mrow>")
		return
	}
	if len(nodes) > 1 {
		w.raw("<mrow>")
	}
	for _, node := range nodes {
		w.node(node)
	}
	if len(nodes) > 1 {
		w.raw("</mrow>")
	}
}

func (w *writer) node(node latex.Node) {
	switch node := node.(type) {
	case latex.List:
		w.nodes(node)
	case latex.Letter:
		w.element("mi", string(node))
	case latex.Number:
		w.element("mn", string(node))
	case latex.Operator:
		w.operator(string(node))
	case latex.Sup:
		w.raw("<msup>")
		w.node(node.Base)
		w.node(node.Script)
		w.raw("</msup>")
	case latex.Sub:
		w.raw("<msub>")
		w.node(node.Base)
		w.node(node.Script)
		w.raw("</msub>")
	case latex.SubSup:
		w.raw("<msubsup>")
		w.node(node.Base)
		w.node(node.Sub)
		w.node(node.Sup)
		w.raw("</msubsup>")
	case latex.Space:
		w.raw(`<mspace width="0.25em"/>`)
	case latex.Delimited:
		w.raw("<mrow>")
		w.delimiter(node.Open)
		w.nodes(node.Body)
		w.delimiter(node.Close)
		w.raw("</mrow>")
	case latex.Env:
		w.environment(node)
	case latex.Command:
		w.command(node)
	}
}

func (w *writer) element(name, text string) {
	w.raw("<" + name + ">")
	w.text(text)
	w.raw("</" + name + ">")
}

func (w *writer) operator(op string) {
	if replacement, ok := special(op); ok {
		if replacement != "" {
			w.element("mo", replacement)
		}
		return
	}
	if op == "(" || op == ")" || op == "[" || op == "]" {
		w.raw(`<mo stretchy="false">`)
		w.text(op)
		w.raw(`</mo>`)
		return
	}
	w.element("mo", op)
}

func (w *writer) delimiter(delimiter string) {
	delimiter = delimiterText(delimiter)
	w.raw("<mo>")
	if delimiter != "." {
		w.text(delimiter)
	}
	w.raw("</mo>")
}

func delimiterText(delimiter string) string {
	if replacement, ok := special(delimiter); ok {
		return replacement
	}
	if replacement, ok := namedOperator(delimiter); ok {
		return replacement
	}
	return delimiter
}

func (w *writer) command(command latex.Command) {
	switch command.Name {
	case `\frac`, `\dfrac`, `\tfrac`:
		w.raw("<mfrac>")
		w.arguments(command.Args)
		w.raw("</mfrac>")
	case `\binom`:
		w.raw(`<mrow><mo>(</mo><mfrac linethickness="0">`)
		w.arguments(command.Args)
		w.raw("</mfrac><mo>)</mo></mrow>")
	case `\sqrt`:
		if len(command.OptArgs) > 0 {
			w.raw("<mroot>")
			w.nodes(command.Args[0])
			w.nodes(command.OptArgs[0])
			w.raw("</mroot>")
		} else {
			w.raw("<msqrt>")
			w.arguments(command.Args)
			w.raw("</msqrt>")
		}
	case `\bigl`, `\bigr`:
		form := "prefix"
		if command.Name == `\bigr` {
			form = "postfix"
		}
		w.raw(`<mo fence="true" form="` + form + `" stretchy="true" minsize="1.2em" maxsize="1.2em">`)
		for _, arg := range command.Args {
			for _, node := range arg {
				if op, ok := node.(latex.Operator); ok {
					delimiter := delimiterText(string(op))
					if delimiter != "." {
						w.text(delimiter)
					}
				}
			}
		}
		w.raw("</mo>")
	case `\boxed`:
		w.raw(`<menclose notation="box"><mrow>`)
		w.arguments(command.Args)
		w.raw("</mrow></menclose>")
	case `\xmapsto`:
		w.raw(`<mover><mo stretchy="true">⟼</mo><mrow>`)
		w.arguments(command.Args)
		w.raw("</mrow></mover>")
	case `\overline`, `\bar`, `\hat`, `\vec`, `\dot`, `\ddot`, `\tilde`:
		accent := mapAccent(command.Name)
		w.raw("<mover><mrow>")
		w.arguments(command.Args)
		w.raw("</mrow>")
		w.element("mo", accent)
		w.raw("</mover>")
	case `\underline`:
		w.raw("<munder><mrow>")
		w.arguments(command.Args)
		w.raw("</mrow><mo>_</mo></munder>")
	case `\textit`, `\mathit`:
		w.raw(`<mtext mathvariant="italic">`)
		w.textArguments(command.Args)
		w.raw("</mtext>")
	case `\textbf`, `\mathbf`:
		w.raw(`<mtext mathvariant="bold">`)
		w.textArguments(command.Args)
		w.raw("</mtext>")
	case `\text`, `\textmd`, `\textrm`, `\mathrm`:
		w.raw("<mtext>")
		w.textArguments(command.Args)
		w.raw("</mtext>")
	case `\operatorname`:
		w.raw("<mo>")
		w.textArguments(command.Args)
		w.raw("</mo>")
	case `\mathbb`:
		for _, arg := range command.Args {
			w.mathbb(arg)
		}
	case `\mod`, `\bmod`:
		w.element("mo", "mod")
		w.arguments(command.Args)
	case `\gcd`, `\log`, `\min`, `\max`:
		w.element("mo", command.Name[1:])
		w.arguments(command.Args)
	case `\pmod`:
		w.raw("<mrow><mo>(</mo><mo>mod</mo>")
		w.arguments(command.Args)
		w.raw("<mo>)</mo></mrow>")
	case `\substack`:
		w.substack(command.Args[0])
	case `\eqref`:
		w.raw("<mtext>(</mtext>")
		w.argText(command.Args[0])
		w.raw("<mtext>)</mtext>")
	case `\label`, `\tag`:
		w.arguments(command.Args)
	default:
		if value, ok := greek(command.Name); ok {
			w.element("mi", value)
		} else if value, ok := namedOperator(command.Name); ok {
			w.element("mo", value)
		} else {
			w.raw("<merror><mtext>")
			w.text(command.Name)
			w.raw("</mtext></merror>")
			w.arguments(command.Args)
		}
	}
}

func (w *writer) arguments(args [][]latex.Node) {
	for _, arg := range args {
		w.nodes(arg)
	}
}

func (w *writer) textArguments(args [][]latex.Node) {
	for _, arg := range args {
		w.argText(arg)
	}
}

func (w *writer) argText(nodes []latex.Node) {
	for _, node := range nodes {
		switch node := node.(type) {
		case latex.Letter:
			w.text(string(node))
		case latex.Number:
			w.text(string(node))
		case latex.Operator:
			w.text(string(node))
		case latex.Space:
			w.text(" ")
		case latex.List:
			w.argText(node)
		default:
			w.node(node)
		}
	}
}

func (w *writer) mathbb(nodes []latex.Node) {
	for _, node := range nodes {
		switch node := node.(type) {
		case latex.Letter:
			w.raw("<mi>")
			for _, r := range node {
				w.WriteRune(doubleStruck(r))
			}
			w.raw("</mi>")
		case latex.Number:
			w.raw("<mn>")
			for _, r := range node {
				w.WriteRune(doubleStruck(r))
			}
			w.raw("</mn>")
		case latex.List:
			w.mathbb(node)
		default:
			w.node(node)
		}
	}
}

func (w *writer) environment(environment latex.Env) {
	if environment.Name != "cases" {
		w.nodes(environment.Body)
		return
	}
	w.raw(`<mrow><mo>{</mo><mtable columnalign="left left">`)
	for _, row := range splitOnLineBreak(environment.Body) {
		w.raw("<mtr>")
		for _, column := range splitOnAmpersand(row) {
			w.raw("<mtd>")
			w.nodes(column)
			w.raw("</mtd>")
		}
		w.raw("</mtr>")
	}
	w.raw("</mtable></mrow>")
}

func (w *writer) substack(nodes []latex.Node) {
	w.raw(`<mtable rowspacing="0.1em" columnalign="center">`)
	for _, row := range splitOnLineBreak(nodes) {
		w.raw("<mtr><mtd>")
		w.nodes(row)
		w.raw("</mtd></mtr>")
	}
	w.raw("</mtable>")
}

func special(command string) (string, bool) {
	switch command {
	case `\{`:
		return "{", true
	case `\}`:
		return "}", true
	case `\|`:
		return "‖", true
	case `\,`, `\;`, `\:`:
		return " ", true
	case `\!`:
		return "", true
	case `\\`:
		return "\n", true
	default:
		return "", false
	}
}

func greek(command string) (string, bool) {
	switch command {
	case `\alpha`:
		return "α", true
	case `\beta`:
		return "β", true
	case `\gamma`:
		return "γ", true
	case `\delta`:
		return "δ", true
	case `\epsilon`, `\varepsilon`:
		return "ε", true
	case `\zeta`:
		return "ζ", true
	case `\eta`:
		return "η", true
	case `\theta`:
		return "θ", true
	case `\iota`:
		return "ι", true
	case `\kappa`:
		return "κ", true
	case `\lambda`:
		return "λ", true
	case `\mu`:
		return "μ", true
	case `\nu`:
		return "ν", true
	case `\xi`:
		return "ξ", true
	case `\pi`:
		return "π", true
	case `\rho`:
		return "ρ", true
	case `\sigma`:
		return "σ", true
	case `\tau`:
		return "τ", true
	case `\upsilon`:
		return "υ", true
	case `\phi`, `\varphi`:
		return "φ", true
	case `\chi`:
		return "χ", true
	case `\psi`:
		return "ψ", true
	case `\omega`:
		return "ω", true
	case `\Gamma`:
		return "Γ", true
	case `\Delta`:
		return "Δ", true
	case `\Theta`:
		return "Θ", true
	case `\Lambda`:
		return "Λ", true
	case `\Xi`:
		return "Ξ", true
	case `\Pi`:
		return "Π", true
	case `\Sigma`:
		return "Σ", true
	case `\Phi`:
		return "Φ", true
	case `\Psi`:
		return "Ψ", true
	case `\Omega`:
		return "Ω", true
	default:
		return "", false
	}
}

func namedOperator(command string) (string, bool) {
	switch command {
	case `\pm`:
		return "±", true
	case `\mp`:
		return "∓", true
	case `\times`:
		return "×", true
	case `\div`:
		return "÷", true
	case `\cdot`:
		return "⋅", true
	case `\leq`, `\le`:
		return "≤", true
	case `\geq`, `\ge`:
		return "≥", true
	case `\neq`, `\ne`:
		return "≠", true
	case `\approx`:
		return "≈", true
	case `\equiv`:
		return "≡", true
	case `\in`:
		return "∈", true
	case `\notin`:
		return "∉", true
	case `\subset`:
		return "⊂", true
	case `\subseteq`:
		return "⊆", true
	case `\supset`:
		return "⊃", true
	case `\supseteq`:
		return "⊇", true
	case `\cup`:
		return "∪", true
	case `\bigcup`:
		return "⋃", true
	case `\cap`:
		return "∩", true
	case `\setminus`:
		return "∖", true
	case `\rightarrow`, `\to`:
		return "→", true
	case `\leftarrow`:
		return "←", true
	case `\mapsto`:
		return "↦", true
	case `\Rightarrow`:
		return "⇒", true
	case `\Longrightarrow`:
		return "⟹", true
	case `\Leftarrow`:
		return "⇐", true
	case `\iff`:
		return "⟺", true
	case `\infty`:
		return "∞", true
	case `\partial`:
		return "∂", true
	case `\nabla`:
		return "∇", true
	case `\forall`:
		return "∀", true
	case `\exists`:
		return "∃", true
	case `\emptyset`:
		return "∅", true
	case `\mid`:
		return "∣", true
	case `\lfloor`:
		return "⌊", true
	case `\rfloor`:
		return "⌋", true
	case `\lceil`:
		return "⌈", true
	case `\rceil`:
		return "⌉", true
	case `\sum`:
		return "∑", true
	case `\prod`:
		return "∏", true
	case `\int`:
		return "∫", true
	case `\ldots`, `\dots`:
		return "…", true
	case `\cdots`:
		return "⋯", true
	case `\quad`:
		return " ", true
	case `\qquad`:
		return "  ", true
	case `\gcd`:
		return "gcd", true
	case `\log`:
		return "log", true
	case `\min`:
		return "min", true
	case `\max`:
		return "max", true
	default:
		return "", false
	}
}

func mapAccent(command string) string {
	switch command {
	case `\overline`, `\bar`:
		return "¯"
	case `\hat`:
		return "^"
	case `\vec`:
		return "→"
	case `\dot`:
		return "˙"
	case `\ddot`:
		return "¨"
	default:
		return "~"
	}
}

func doubleStruck(r rune) rune {
	switch {
	case r >= 'A' && r <= 'Z':
		switch r {
		case 'C':
			return 'ℂ'
		case 'H':
			return 'ℍ'
		case 'N':
			return 'ℕ'
		case 'P':
			return 'ℙ'
		case 'Q':
			return 'ℚ'
		case 'R':
			return 'ℝ'
		case 'Z':
			return 'ℤ'
		}
		return 0x1D538 + (r - 'A')
	case r >= 'a' && r <= 'z':
		return 0x1D552 + (r - 'a')
	case r >= '0' && r <= '9':
		return 0x1D7D8 + (r - '0')
	default:
		return r
	}
}

func splitOnAmpersand(nodes []latex.Node) [][]latex.Node {
	var columns [][]latex.Node
	var current []latex.Node
	for _, node := range nodes {
		if operator, ok := node.(latex.Operator); ok && string(operator) == "&" {
			columns = append(columns, current)
			current = nil
			continue
		}
		current = append(current, node)
	}
	return append(columns, current)
}

func splitOnLineBreak(nodes []latex.Node) [][]latex.Node {
	var rows [][]latex.Node
	var current []latex.Node
	for _, node := range nodes {
		if operator, ok := node.(latex.Operator); ok && string(operator) == `\\` {
			rows = append(rows, current)
			current = nil
			continue
		}
		if _, ok := node.(latex.Space); ok && len(current) == 0 {
			continue
		}
		current = append(current, node)
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}
	return rows
}

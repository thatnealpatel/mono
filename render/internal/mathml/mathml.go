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
	w := writer{style: textStyle}
	if display {
		w.style = displayStyle
		w.raw(`<math display="block">`)
	} else {
		w.raw(`<math>`)
	}
	w.nodes(nodes)
	w.raw(`</math>`)
	return w.String(), nil
}

type writer struct {
	bytes.Buffer
	style mathStyle
}

type mathStyle uint8

const (
	displayStyle mathStyle = iota
	textStyle
	scriptStyle
	scriptScriptStyle
)

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
	for i, node := range nodes {
		if _, ok := relationBase(node); ok {
			w.relationNode(node, relationSpace(nodes, i, -1), relationSpace(nodes, i, 1))
			continue
		}
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
		if command, ok := underbraceBase(node.Base); ok {
			w.raw("<mover>")
			w.underbrace(command.Args)
			w.script(node.Script)
			w.raw("</mover>")
			return
		}
		w.raw("<msup>")
		w.node(node.Base)
		w.script(node.Script)
		w.raw("</msup>")
	case latex.Sub:
		if command, ok := underbraceBase(node.Base); ok {
			w.raw("<munder>")
			w.underbrace(command.Args)
			w.script(node.Script)
			w.raw("</munder>")
			return
		}
		w.raw("<msub>")
		w.node(node.Base)
		w.script(node.Script)
		w.raw("</msub>")
	case latex.SubSup:
		if command, ok := underbraceBase(node.Base); ok {
			w.raw("<munderover>")
			w.underbrace(command.Args)
			w.script(node.Sub)
			w.script(node.Sup)
			w.raw("</munderover>")
			return
		}
		w.raw("<msubsup>")
		w.node(node.Base)
		w.script(node.Sub)
		w.script(node.Sup)
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

func (w *writer) script(node latex.Node) {
	previous := w.style
	w.style = min(w.style+2, scriptScriptStyle)
	defer w.restoreStyle(previous)
	w.node(node)
}

func (w *writer) styleArguments(style mathStyle, args [][]latex.Node) {
	previous := w.style
	w.style = style
	defer w.restoreStyle(previous)
	w.arguments(args)
}

func (w *writer) scriptArguments(args [][]latex.Node) {
	w.styleArguments(min(w.style+2, scriptScriptStyle), args)
}

func (w *writer) fractionArguments(name string, args [][]latex.Node) {
	style := min(w.style+1, scriptScriptStyle)
	switch name {
	case `\dfrac`:
		style = textStyle
	case `\tfrac`:
		style = scriptStyle
	}
	w.styleArguments(style, args)
}

func (w *writer) restoreStyle(style mathStyle) { w.style = style }

type atomClass uint8

const (
	atomOrd atomClass = iota
	atomOperator
	atomBinary
	atomRelation
	atomOpen
	atomClose
	atomPunctuation
	atomInner
)

func relationSpace(nodes []latex.Node, index, step int) string {
	for i := index + step; i >= 0 && i < len(nodes); i += step {
		if _, ok := nodes[i].(latex.Space); ok {
			continue
		}
		class := classifyAtom(nodes[i])
		if step < 0 {
			switch class {
			case atomOrd, atomOperator, atomBinary, atomClose, atomInner:
				return "0.2778em"
			case atomPunctuation:
				return "0.1667em"
			}
			return "0em"
		}
		switch class {
		case atomOrd, atomOperator, atomBinary, atomOpen, atomInner:
			return "0.2778em"
		default:
			return "0em"
		}
	}
	return "0em"
}

func classifyAtom(node latex.Node) atomClass {
	if _, ok := relationBase(node); ok {
		return atomRelation
	}
	switch node := node.(type) {
	case latex.Sup:
		return classifyAtom(node.Base)
	case latex.Sub:
		return classifyAtom(node.Base)
	case latex.SubSup:
		return classifyAtom(node.Base)
	case latex.Delimited:
		return atomInner
	case latex.Operator:
		switch string(node) {
		case "=", "<", ">", ":":
			return atomRelation
		case "(", "[", `\{`:
			return atomOpen
		case ")", "]", `\}`:
			return atomClose
		case ",", ";":
			return atomPunctuation
		case "+", "-", "*":
			return atomBinary
		}
	case latex.Command:
		if commandIsRelation(node.Name) {
			return atomRelation
		}
		switch node.Name {
		case `\langle`, `\lfloor`, `\lceil`, `\bigl`, `\{`:
			return atomOpen
		case `\rangle`, `\rfloor`, `\rceil`, `\bigr`, `\}`:
			return atomClose
		}
	}
	return atomOrd
}

func relationBase(node latex.Node) (latex.Command, bool) {
	switch node := node.(type) {
	case latex.Command:
		return node, node.Name == `\mathrel`
	case latex.Sup:
		return relationBase(node.Base)
	case latex.Sub:
		return relationBase(node.Base)
	case latex.SubSup:
		return relationBase(node.Base)
	default:
		return latex.Command{}, false
	}
}

func (w *writer) relationNode(node latex.Node, leftSpace, rightSpace string) {
	if w.style >= scriptStyle {
		leftSpace = "0em"
		rightSpace = "0em"
	}
	if leftSpace != "0em" {
		w.raw(`<mspace width="` + leftSpace + `"/>`)
	}
	w.relationNucleus(node)
	if rightSpace != "0em" {
		w.raw(`<mspace width="` + rightSpace + `"/>`)
	}
}

func (w *writer) relationNucleus(node latex.Node) {
	switch node := node.(type) {
	case latex.Command:
		w.mathrel(node, "0em", "0em")
	case latex.Sup:
		w.raw("<msup>")
		w.relationNucleus(node.Base)
		w.script(node.Script)
		w.raw("</msup>")
	case latex.Sub:
		w.raw("<msub>")
		w.relationNucleus(node.Base)
		w.script(node.Script)
		w.raw("</msub>")
	case latex.SubSup:
		w.raw("<msubsup>")
		w.relationNucleus(node.Base)
		w.script(node.Sub)
		w.script(node.Sup)
		w.raw("</msubsup>")
	}
}

func commandIsRelation(name string) bool {
	switch name {
	case `\leq`, `\le`, `\geq`, `\ge`, `\neq`, `\ne`, `\approx`, `\equiv`, `\cong`,
		`\in`, `\notin`, `\subset`, `\subseteq`, `\supset`, `\supseteq`, `\smile`, `\mid`,
		`\not`, `\centernot`, `\uparrow`, `\downarrow`, `\updownarrow`, `\Uparrow`, `\Downarrow`, `\Updownarrow`,
		`\rightarrow`, `\to`, `\leftarrow`, `\nearrow`, `\searrow`, `\nwarrow`, `\swarrow`,
		`\longrightarrow`, `\longleftarrow`, `\leftrightarrow`, `\longleftrightarrow`,
		`\rightsquigarrow`, `\leftsquigarrow`, `\mapsto`, `\longmapsto`, `\Rightarrow`,
		`\Longrightarrow`, `\implies`, `\Leftarrow`, `\iff`:
		return true
	default:
		return false
	}
}

func underbraceBase(node latex.Node) (latex.Command, bool) {
	command, ok := node.(latex.Command)
	return command, ok && command.Name == `\underbrace`
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
		w.fractionArguments(command.Name, command.Args)
		w.raw("</mfrac>")
	case `\binom`:
		w.raw(`<mrow><mo>(</mo><mfrac linethickness="0">`)
		w.fractionArguments(`\frac`, command.Args)
		w.raw("</mfrac><mo>)</mo></mrow>")
	case `\sqrt`:
		if len(command.OptArgs) > 0 {
			w.raw("<mroot>")
			w.nodes(command.Args[0])
			w.script(latex.List(command.OptArgs[0]))
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
		w.delimiterArguments(command.Args)
		w.raw("</mo>")
	case `\Big`:
		w.raw(`<mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">`)
		w.delimiterArguments(command.Args)
		w.raw("</mo>")
	case `\middle`:
		w.raw(`<mo fence="true" stretchy="true">`)
		w.delimiterArguments(command.Args)
		w.raw("</mo>")
	case `\not`, `\centernot`:
		w.negation(command.Args)
	case `\boxed`:
		w.raw(`<menclose notation="box"><mrow>`)
		w.arguments(command.Args)
		w.raw("</mrow></menclose>")
	case `\xmapsto`:
		w.raw(`<mover><mo stretchy="true">⟼</mo><mrow>`)
		w.scriptArguments(command.Args)
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
	case `\underbrace`:
		w.underbrace(command.Args)
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
	case `\mathcal`, `\mathscr`:
		w.mathVariant(command.Args, script)
	case `\mathbb`:
		w.mathVariant(command.Args, doubleStruck)
	case `\ell`:
		w.element("mi", "ℓ")
	case `\bigsqcup`:
		w.raw(`<mo form="prefix" largeop="true" movablelimits="true">⨆</mo>`)
	case `\mathrel`:
		w.mathrel(command, "0em", "0em")
	case `\mod`, `\bmod`:
		w.element("mo", "mod")
		w.arguments(command.Args)
	case `\gcd`, `\log`, `\min`, `\max`, `\deg`, `\dim`:
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

func (w *writer) underbrace(args [][]latex.Node) {
	w.raw(`<munder accentunder="true"><mrow>`)
	for _, arg := range args {
		for _, node := range arg {
			w.node(node)
		}
	}
	w.raw(`</mrow><mo stretchy="true">⏟</mo></munder>`)
}

func (w *writer) arguments(args [][]latex.Node) {
	for _, arg := range args {
		w.nodes(arg)
	}
}

func (w *writer) delimiterArguments(args [][]latex.Node) {
	for _, arg := range args {
		for _, node := range arg {
			op, ok := node.(latex.Operator)
			if !ok {
				continue
			}
			delimiter := delimiterText(string(op))
			if delimiter != "." {
				w.text(delimiter)
			}
		}
	}
}

func (w *writer) negation(args [][]latex.Node) {
	if len(args) == 1 && len(args[0]) == 1 {
		if value, ok := operatorText(args[0][0]); ok {
			w.element("mo", negatedOperator(value))
			return
		}
	}
	w.raw(`<menclose notation="updiagonalstrike">`)
	w.arguments(args)
	w.raw(`</menclose>`)
}

func operatorText(node latex.Node) (string, bool) {
	switch node := node.(type) {
	case latex.Operator:
		return string(node), true
	case latex.Command:
		return namedOperator(node.Name)
	default:
		return "", false
	}
}

func negatedOperator(value string) string {
	switch value {
	case "=":
		return "≠"
	case "<":
		return "≮"
	case ">":
		return "≯"
	case "≤":
		return "≰"
	case "≥":
		return "≱"
	case "≅":
		return "≇"
	case "≈":
		return "≉"
	case "≡":
		return "≢"
	case "∈":
		return "∉"
	case "⊂":
		return "⊄"
	case "⊆":
		return "⊈"
	case "⊃":
		return "⊅"
	case "⊇":
		return "⊉"
	case "→":
		return "↛"
	case "←":
		return "↚"
	case "⇒":
		return "⇏"
	case "⇐":
		return "⇍"
	default:
		return value + "̸"
	}
}

func (w *writer) mathrel(command latex.Command, leftSpace, rightSpace string) {
	if w.style >= scriptStyle {
		leftSpace = "0em"
		rightSpace = "0em"
	}
	if text, ok := relationText(command.Args[0]); ok {
		w.raw(`<mo form="infix" fence="false" separator="false" stretchy="false" lspace="` + leftSpace + `" rspace="` + rightSpace + `">`)
		w.text(text)
		w.raw("</mo>")
		return
	}
	w.raw("<mrow>")
	if leftSpace != "0em" {
		w.raw(`<mspace width="` + leftSpace + `"/>`)
	}
	w.arguments(command.Args)
	if rightSpace != "0em" {
		w.raw(`<mspace width="` + rightSpace + `"/>`)
	}
	w.raw("</mrow>")
}

func relationText(nodes []latex.Node) (string, bool) {
	var text bytes.Buffer
	for _, node := range nodes {
		switch node := node.(type) {
		case latex.Letter:
			text.WriteString(string(node))
		case latex.Number:
			text.WriteString(string(node))
		case latex.Operator:
			text.WriteString(delimiterText(string(node)))
		case latex.Space:
			text.WriteByte(' ')
		case latex.List:
			value, ok := relationText(node)
			if !ok {
				return "", false
			}
			text.WriteString(value)
		case latex.Command:
			value, ok := namedOperator(node.Name)
			if !ok {
				value, ok = greek(node.Name)
			}
			if !ok {
				return "", false
			}
			text.WriteString(value)
		default:
			return "", false
		}
	}
	return text.String(), true
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

func (w *writer) mathVariant(args [][]latex.Node, transform func(rune) rune) {
	w.raw("<mrow>")
	for _, arg := range args {
		for _, node := range variantNodes(arg, transform) {
			w.node(node)
		}
	}
	w.raw("</mrow>")
}

func variantNodes(nodes []latex.Node, transform func(rune) rune) []latex.Node {
	result := make([]latex.Node, len(nodes))
	for i, node := range nodes {
		result[i] = variantNode(node, transform)
	}
	return result
}

func variantNode(node latex.Node, transform func(rune) rune) latex.Node {
	switch node := node.(type) {
	case latex.Letter:
		letters := []rune(node)
		for i, r := range letters {
			letters[i] = transform(r)
		}
		return latex.Letter(letters)
	case latex.Number:
		digits := []rune(node)
		for i, r := range digits {
			digits[i] = transform(r)
		}
		return latex.Number(digits)
	case latex.List:
		return latex.List(variantNodes(node, transform))
	case latex.Sup:
		return latex.Sup{Base: variantNode(node.Base, transform), Script: variantNode(node.Script, transform)}
	case latex.Sub:
		return latex.Sub{Base: variantNode(node.Base, transform), Script: variantNode(node.Script, transform)}
	case latex.SubSup:
		return latex.SubSup{
			Base: variantNode(node.Base, transform),
			Sub:  variantNode(node.Sub, transform),
			Sup:  variantNode(node.Sup, transform),
		}
	case latex.Delimited:
		node.Body = variantNodes(node.Body, transform)
		return node
	case latex.Env:
		node.Body = variantNodes(node.Body, transform)
		return node
	case latex.Command:
		if commandSetsMathVariant(node.Name) {
			return node
		}
		node.Args = variantArguments(node.Args, transform)
		node.OptArgs = variantArguments(node.OptArgs, transform)
		return node
	default:
		return node
	}
}

func variantArguments(args [][]latex.Node, transform func(rune) rune) [][]latex.Node {
	result := make([][]latex.Node, len(args))
	for i, arg := range args {
		result[i] = variantNodes(arg, transform)
	}
	return result
}

func commandSetsMathVariant(name string) bool {
	switch name {
	case `\mathcal`, `\mathscr`, `\mathbb`, `\mathfrak`, `\mathrm`, `\mathbf`, `\mathit`, `\operatorname`,
		`\text`, `\textit`, `\textbf`, `\textmd`, `\textrm`:
		return true
	default:
		return false
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
	case `\{`, `\lbrace`:
		return "{", true
	case `\}`, `\rbrace`:
		return "}", true
	case `\lparen`:
		return "(", true
	case `\rparen`:
		return ")", true
	case `\lbrack`:
		return "[", true
	case `\rbrack`:
		return "]", true
	case `\|`, `\Vert`, `\lVert`, `\rVert`:
		return "‖", true
	case `\vert`, `\lvert`, `\rvert`:
		return "|", true
	case `\langle`, `\lang`:
		return "⟨", true
	case `\rangle`, `\rang`:
		return "⟩", true
	case `\lt`:
		return "<", true
	case `\gt`:
		return ">", true
	case `\lgroup`:
		return "⟮", true
	case `\rgroup`:
		return "⟯", true
	case `\lmoustache`:
		return "⎰", true
	case `\rmoustache`:
		return "⎱", true
	case `\ulcorner`:
		return "⌜", true
	case `\urcorner`:
		return "⌝", true
	case `\llcorner`:
		return "⌞", true
	case `\lrcorner`:
		return "⌟", true
	case `\llbracket`:
		return "⟦", true
	case `\rrbracket`:
		return "⟧", true
	case `\lBrace`:
		return "⦃", true
	case `\rBrace`:
		return "⦄", true
	case `\backslash`:
		return `\`, true
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
	case `\circ`:
		return "∘", true
	case `\smile`:
		return "⌣", true
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
	case `\cong`:
		return "≅", true
	case `\otimes`:
		return "⊗", true
	case `\rtimes`:
		return "⋊", true
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
	case `\sqcup`:
		return "⊔", true
	case `\bigsqcup`:
		return "⨆", true
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
	case `\nearrow`:
		return "↗", true
	case `\searrow`:
		return "↘", true
	case `\nwarrow`:
		return "↖", true
	case `\swarrow`:
		return "↙", true
	case `\longrightarrow`:
		return "⟶", true
	case `\longleftarrow`:
		return "⟵", true
	case `\leftrightarrow`:
		return "↔", true
	case `\longleftrightarrow`:
		return "⟷", true
	case `\rightsquigarrow`:
		return "⇝", true
	case `\leftsquigarrow`:
		return "⇜", true
	case `\mapsto`:
		return "↦", true
	case `\longmapsto`:
		return "⟼", true
	case `\Rightarrow`:
		return "⇒", true
	case `\Longrightarrow`, `\implies`:
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
	case `\emptyset`, `\varnothing`:
		return "∅", true
	case `\mid`, `\vert`:
		return "∣", true
	case `\langle`:
		return "⟨", true
	case `\rangle`:
		return "⟩", true
	case `\lfloor`:
		return "⌊", true
	case `\rfloor`:
		return "⌋", true
	case `\lceil`:
		return "⌈", true
	case `\rceil`:
		return "⌉", true
	case `\uparrow`:
		return "↑", true
	case `\downarrow`:
		return "↓", true
	case `\updownarrow`:
		return "↕", true
	case `\Uparrow`:
		return "⇑", true
	case `\Downarrow`:
		return "⇓", true
	case `\Updownarrow`:
		return "⇕", true
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

func script(r rune) rune {
	switch r {
	case 'B':
		return 'ℬ'
	case 'E':
		return 'ℰ'
	case 'F':
		return 'ℱ'
	case 'H':
		return 'ℋ'
	case 'I':
		return 'ℐ'
	case 'L':
		return 'ℒ'
	case 'M':
		return 'ℳ'
	case 'R':
		return 'ℛ'
	case 'e':
		return 'ℯ'
	case 'g':
		return 'ℊ'
	case 'o':
		return 'ℴ'
	}
	switch {
	case r >= 'A' && r <= 'Z':
		return 0x1D49C + (r - 'A')
	case r >= 'a' && r <= 'z':
		return 0x1D4B6 + (r - 'a')
	default:
		return r
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

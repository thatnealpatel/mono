package mathml

import (
	"errors"
	"strings"
	"testing"

	"patel.codes/render/internal/latex"
)

func TestRenderSupportedCommands(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{name: "Fraction", expr: `\frac{x+1}{y}`, want: []string{"<mfrac>", "<mi>x</mi>"}},
		{name: "Root", expr: `\sqrt[3]{x}`, want: []string{"<mroot>", "<mn>3</mn>"}},
		{name: "Cases", expr: `\begin{cases}k & \text{if } k \mid n \\ 0 & \text{otherwise}\end{cases}`, want: []string{"<mtable", "<mtr>", "∣"}},
		{name: "Substack", expr: `\sum_{\substack{d \mid k \\ d > 0}}`, want: []string{"<msub>", "<mtable"}},
		{name: "Mathbb", expr: `\mathbb{N} \to \mathbb{Z}`, want: []string{"ℕ", "→", "ℤ"}},
		{name: "NamedOperator", expr: `\operatorname{Pre}(L)`, want: []string{"<mo>Pre</mo>"}},
		{name: "BigDelimiter", expr: `\bigl(x\bigr)`, want: []string{`form="prefix"`, `form="postfix"`}},
		{name: "Congruent", expr: `A \cong B`, want: []string{"<mo>≅</mo>"}},
		{name: "TensorProduct", expr: `A \otimes B`, want: []string{"<mo>⊗</mo>"}},
		{name: "Composition", expr: `f \circ g`, want: []string{"<mo>∘</mo>"}},
		{name: "Triangle", expr: `\triangle`, want: []string{`<mo lspace="0em" rspace="0em">△</mo>`}},
		{name: "VariantEmptySet", expr: `\varnothing`, want: []string{"<mo>∅</mo>"}},
		{name: "Smile", expr: `a \smile b`, want: []string{"<mo>⌣</mo>"}},
		{name: "LeftRightArrow", expr: `A \leftrightarrow B`, want: []string{"<mo>↔</mo>"}},
		{name: "LongLeftRightArrow", expr: `A \longleftrightarrow B`, want: []string{"<mo>⟷</mo>"}},
		{name: "Implies", expr: `P \implies Q`, want: []string{"<mo>⟹</mo>"}},
		{name: "Not", expr: `x \not= y`, want: []string{"<mo>≠</mo>"}},
		{name: "NotCongruent", expr: `A \not\cong B`, want: []string{"<mo>≇</mo>"}},
		{name: "CenterNot", expr: `P \centernot\implies Q`, want: []string{"<mo>⟹̸</mo>"}},
		{name: "MiddleDelimiter", expr: `\left\{x \middle| x > 0\right\}`, want: []string{`<mo fence="true" stretchy="true">|</mo>`}},
		{name: "NamedMiddleDelimiter", expr: `\left(x \middle\vert y\right)`, want: []string{`<mo fence="true" stretchy="true">|</mo>`}},
		{name: "VerticalRelation", expr: `x \vert y`, want: []string{"<mo>∣</mo>"}},
		{name: "LeftAngle", expr: `\langle x`, want: []string{"<mo>⟨</mo>"}},
		{name: "RightAngle", expr: `x \rangle`, want: []string{"<mo>⟩</mo>"}},
		{name: "AngleDelimiters", expr: `\left\langle x, y \right\rangle`, want: []string{"<mo>⟨</mo>", "<mo>⟩</mo>"}},
		{name: "DiagonalArrows", expr: `a \nearrow b \searrow c \nwarrow d \swarrow e`, want: []string{"<mo>↗</mo>", "<mo>↘</mo>", "<mo>↖</mo>", "<mo>↙</mo>"}},
		{name: "LongArrows", expr: `a \longrightarrow b \longleftarrow c`, want: []string{"<mo>⟶</mo>", "<mo>⟵</mo>"}},
		{name: "SquiggleArrows", expr: `a \rightsquigarrow b \leftsquigarrow c`, want: []string{"<mo>⇝</mo>", "<mo>⇜</mo>"}},
		{name: "Mathcal", expr: `\mathcal{F}`, want: []string{"<mi>ℱ</mi>"}},
		{name: "LongMapsto", expr: `A \longmapsto B`, want: []string{"<mo>⟼</mo>"}},
		{name: "Ell", expr: `\ell`, want: []string{"<mi>ℓ</mi>"}},
		{name: "Degree", expr: `\deg f`, want: []string{"<mo>deg</mo>"}},
		{name: "SquareCup", expr: `A \sqcup B`, want: []string{"<mo>⊔</mo>"}},
		{name: "Dimension", expr: `\dim V`, want: []string{"<mo>dim</mo>"}},
		{name: "BigSquareCup", expr: `A \bigsqcup B`, want: []string{"⨆"}},
		{name: "RightSemidirectProduct", expr: `G \rtimes H`, want: []string{"<mo>⋊</mo>"}},
		{name: "HookArrows", expr: `A \hookrightarrow B \hookleftarrow C`, want: []string{"<mo>↪</mo>", "<mo>↩</mo>"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			html, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			for _, want := range test.want {
				if got, ok := strings.Contains(html, want), true; got != ok {
					t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
				}
			}
		})
	}
}

func TestRenderCommandsWithArguments(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Mathscr", expr: `\mathscr{Fa2}`, want: `<math><mrow><mi>ℱ</mi><mi>𝒶</mi><mn>2</mn></mrow></math>`},
		{name: "MathscrOverride", expr: `\mathbb{\mathscr{A}}`, want: `<math><mrow><mrow><mi>𝒜</mi></mrow></mrow></math>`},
		{name: "WideTilde", expr: `\widetilde{ABC}`, want: `<math><mover><mrow><mi>A</mi><mi>B</mi><mi>C</mi></mrow><mo stretchy="true">~</mo></mover></math>`},
		{name: "Mathbin", expr: `a\mathbin{\triangle}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2222em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">△</mo><mspace width="0.2222em"/><mi>b</mi></mrow></math>`},
		{name: "TriangleOrdinary", expr: `a\triangle2`, want: `<math><mrow><mi>a</mi><mo lspace="0em" rspace="0em">△</mo><mn>2</mn></mrow></math>`},
		{name: "MathbinTypography", expr: `a\mathbin{x}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2222em"/><mrow><mi>x</mi></mrow><mspace width="0.2222em"/><mi>b</mi></mrow></math>`},
		{name: "LeadingMathbinCascade", expr: `\mathbin{A}\mathbin{B}x`, want: `<math><mrow><mrow><mi>A</mi></mrow><mspace width="0.2222em"/><mrow><mi>B</mi></mrow><mspace width="0.2222em"/><mi>x</mi></mrow></math>`},
		{name: "AfterOperatorMathbin", expr: `\sum\mathbin{X}b`, want: `<math><mrow><mo>∑</mo><mrow><mi>X</mi></mrow><mi>b</mi></mrow></math>`},
		{name: "AfterSpacedOperatorMathbin", expr: `\sum\quad\mathbin{X}b`, want: `<math><mrow><mo>∑</mo><mo> </mo><mrow><mi>X</mi></mrow><mi>b</mi></mrow></math>`},
		{name: "AfterThinSpacedOperatorMathbin", expr: `\sum\,\mathbin{X}b`, want: `<math><mrow><mo>∑</mo><mo> </mo><mrow><mi>X</mi></mrow><mi>b</mi></mrow></math>`},
		{name: "AfterBinaryMathbin", expr: `a\times\mathbin{X}b`, want: `<math><mrow><mi>a</mi><mo>×</mo><mrow><mi>X</mi></mrow><mi>b</mi></mrow></math>`},
		{name: "BeforeRelationMathbin", expr: `a\mathbin{X}\vert2`, want: `<math><mrow><mi>a</mi><mrow><mi>X</mi></mrow><mo>∣</mo><mn>2</mn></mrow></math>`},
		{name: "Mathrel", expr: `a\mathrel{R}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "KnownOperatorMathrel", expr: `a\mathrel{+}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">+</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "SeparatorMathrel", expr: `a\mathrel{,}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">,</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "AfterRelationMathrel", expr: `a=\mathrel{R}b`, want: `<math><mrow><mi>a</mi><mo>=</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "AfterScriptedRelationMathrel", expr: `a=_{x}\mathrel{R}b`, want: `<math><mrow><mi>a</mi><msub><mo>=</mo><mi>x</mi></msub><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "AfterSlashMathrel", expr: `a/\mathrel{R}b`, want: `<math><mrow><mi>a</mi><mo>/</mo><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "AfterColonMathrel", expr: `a:\mathrel{R}b`, want: `<math><mrow><mi>a</mi><mo>:</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "BetweenDelimitersMathrel", expr: `(\mathrel{R})`, want: `<math><mrow><mo stretchy="false">(</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mo stretchy="false">)</mo></mrow></math>`},
		{name: "BetweenCommandDelimitersMathrel", expr: `\langle\mathrel{R}\rangle`, want: `<math><mrow><mo>⟨</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mo>⟩</mo></mrow></math>`},
		{name: "BetweenBigDelimitersMathrel", expr: `\bigl(\mathrel{R}\bigr)`, want: `<math><mrow><mo fence="true" form="prefix" stretchy="true" minsize="1.2em" maxsize="1.2em">(</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mo fence="true" form="postfix" stretchy="true" minsize="1.2em" maxsize="1.2em">)</mo></mrow></math>`},
		{name: "BetweenEscapedBracesMathrel", expr: `\{\mathrel{R}\}`, want: `<math><mrow><mo>{</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mo>}</mo></mrow></math>`},
		{name: "LeadingMathrel", expr: `\mathrel{R}a`, want: `<math><mrow><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mi>a</mi></mrow></math>`},
		{name: "BeforeBinaryMathrel", expr: `\mathrel{R}+b`, want: `<math><mrow><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mspace width="0.2778em"/><mo>+</mo><mi>b</mi></mrow></math>`},
		{name: "TrailingMathrel", expr: `a\mathrel{R}`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo></mrow></math>`},
		{name: "AdjacentMathrel", expr: `a\mathrel{R}\mathrel{S}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">S</mo><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "ScriptMathrel", expr: `a_{\mathrel{R}b}`, want: `<math><msub><mi>a</mi><mrow><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mi>b</mi></mrow></msub></math>`},
		{name: "ScriptedMathrel", expr: `a\mathrel{R}_{x}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><msub><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mi>x</mi></msub><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "FractionMathrel", expr: `\frac{a\mathrel{R}b}{c}`, want: `<math><mfrac><mrow><mi>a</mi><mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo><mi>b</mi></mrow><mi>c</mi></mfrac></math>`},
		{name: "ComplexMathrel", expr: `a\mathrel{\frac12}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><mrow><mfrac><mn>1</mn><mn>2</mn></mfrac></mrow><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
		{name: "ComplexScriptedMathrel", expr: `a\mathrel{\frac12}_{x}b`, want: `<math><mrow><mi>a</mi><mspace width="0.2778em"/><msub><mrow><mfrac><mn>1</mn><mn>2</mn></mfrac></mrow><mi>x</mi></msub><mspace width="0.2778em"/><mi>b</mi></mrow></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderBigDelimiter(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Parentheses", expr: `\Big(x\Big)`, want: `<math><mrow><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">(</mo><mi>x</mi><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">)</mo></mrow></math>`},
		{name: "MiddleBar", expr: `a\Big|b`, want: `<math><mrow><mi>a</mi><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">|</mo><mi>b</mi></mrow></math>`},
		{name: "Braced", expr: `\Big{(}x\Big{)}`, want: `<math><mrow><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">(</mo><mi>x</mi><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">)</mo></mrow></math>`},
		{name: "Vertical", expr: `\Big\Vert`, want: `<math><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">‖</mo></math>`},
		{name: "Arrow", expr: `\Big\uparrow`, want: `<math><mo fence="true" stretchy="true" lspace="0em" rspace="0em" minsize="1.623em" maxsize="1.623em">↑</mo></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderBiglDelimiters(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Vertical", expr: `\bigl\Vert x\bigr\Vert`, want: `<math><mrow><mo fence="true" form="prefix" stretchy="true" minsize="1.2em" maxsize="1.2em">‖</mo><mspace width="0.25em"/><mi>x</mi><mo fence="true" form="postfix" stretchy="true" minsize="1.2em" maxsize="1.2em">‖</mo></mrow></math>`},
		{name: "Backslash", expr: `\bigl\backslash x\bigr/`, want: `<math><mrow><mo fence="true" form="prefix" stretchy="true" minsize="1.2em" maxsize="1.2em">\</mo><mspace width="0.25em"/><mi>x</mi><mo fence="true" form="postfix" stretchy="true" minsize="1.2em" maxsize="1.2em">/</mo></mrow></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderLeftRightAliases(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Brackets", expr: `\left\lbrack x\right\rbrack`, want: `<math><mrow><mo>[</mo><mrow><mspace width="0.25em"/><mi>x</mi></mrow><mo>]</mo></mrow></math>`},
		{name: "Vertical", expr: `\left\lVert x\right\rVert`, want: `<math><mrow><mo>‖</mo><mrow><mspace width="0.25em"/><mi>x</mi></mrow><mo>‖</mo></mrow></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderBigSquareCup(t *testing.T) {
	got, err := Render(`\bigsqcup_{i=1}^nA_i`, true)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `<math display="block"><mrow><msubsup><mo form="prefix" largeop="true" movablelimits="true">⨆</mo><mrow><mi>i</mi><mo>=</mo><mn>1</mn></mrow><mi>n</mi></msubsup><msub><mi>A</mi><mi>i</mi></msub></mrow></math>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnderbrace(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Bare", expr: `\underbrace{a+b}`, want: `<math><munder accentunder="true"><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mo stretchy="true">⏟</mo></munder></math>`},
		{name: "Below", expr: `\underbrace{a+b}_{n}`, want: `<math><munder><munder accentunder="true"><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mo stretchy="true">⏟</mo></munder><mi>n</mi></munder></math>`},
		{name: "Above", expr: `\underbrace{a+b}^{m}`, want: `<math><mover><munder accentunder="true"><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mo stretchy="true">⏟</mo></munder><mi>m</mi></mover></math>`},
		{name: "BelowAndAbove", expr: `\underbrace{a+b}_{n}^{m}`, want: `<math><munderover><munder accentunder="true"><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mo stretchy="true">⏟</mo></munder><mi>n</mi><mi>m</mi></munderover></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderMathcal(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "Grouped", expr: `\mathcal{Fa2}`, want: `<math><mrow><mi>ℱ</mi><mi>𝒶</mi><mn>2</mn></mrow></math>`},
		{name: "Script", expr: `x_{\mathcal{G}}`, want: `<math><msub><mi>x</mi><mrow><mi>𝒢</mi></mrow></msub></math>`},
		{name: "Operator", expr: `\mathcal{F+1}`, want: `<math><mrow><mi>ℱ</mi><mo>+</mo><mn>1</mn></mrow></math>`},
		{name: "MultipleSubscript", expr: `x_{\mathcal{AB}}`, want: `<math><msub><mi>x</mi><mrow><mi>𝒜</mi><mi>ℬ</mi></mrow></msub></math>`},
		{name: "Composite", expr: `\mathcal{F^G+\frac{H}{I}}`, want: `<math><mrow><msup><mi>ℱ</mi><mi>𝒢</mi></msup><mo>+</mo><mfrac><mi>ℋ</mi><mi>ℐ</mi></mfrac></mrow></math>`},
		{name: "OperatorOverride", expr: `\mathcal{\operatorname{Hom}}`, want: `<math><mrow><mo>Hom</mo></mrow></math>`},
		{name: "MathbbOverride", expr: `\mathcal{\mathbb{A^B}}`, want: `<math><mrow><mrow><msup><mi>𝔸</mi><mi>𝔹</mi></msup></mrow></mrow></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderMathbb(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "MultipleScript", expr: `x^{\mathbb{AB}}`, want: `<math><msup><mi>x</mi><mrow><mi>𝔸</mi><mi>𝔹</mi></mrow></msup></math>`},
		{name: "Digits", expr: `\mathbb{12}`, want: `<math><mrow><mn>𝟙𝟚</mn></mrow></math>`},
		{name: "ExpandingCommand", expr: `x^{\mathbb{\mod{A}}}`, want: `<math><msup><mi>x</mi><mrow><mo>mod</mo><mi>𝔸</mi></mrow></msup></math>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderDisplay(t *testing.T) {
	html, err := Render("x", true)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := html, `<math display="block"><mi>x</mi></math>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEscapesSourceText(t *testing.T) {
	html, err := Render(`x < y & \text{a < b & c}`, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"<mo>&lt;</mo>", "<mo>&amp;</mo>", "a &lt; b &amp; c"} {
		if got, ok := strings.Contains(html, want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
	for _, unwanted := range []string{"<mo><</mo>", "a < b & c"} {
		if got, want := strings.Contains(html, unwanted), false; got != want {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", unwanted, got, want, html)
		}
	}
}

func TestRenderUnknownCommandVisible(t *testing.T) {
	html, err := Render(`\unknown{x}`, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{`<merror><mtext>\unknown</mtext></merror>`, "<mi>x</mi>"} {
		if got, ok := strings.Contains(html, want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderRejectsMalformedLatex(t *testing.T) {
	_, err := Render(`\frac{x}`, false)
	if !errors.Is(err, latex.ErrInvalid) {
		t.Errorf("error = %v, want latex.ErrInvalid", err)
	}
}

func TestRenderEmptyGroups(t *testing.T) {
	for _, test := range []struct {
		expr string
		want string
	}{
		{expr: `x^{}`, want: `<msup><mi>x</mi><mrow></mrow></msup>`},
		{expr: `\frac{}{x}`, want: `<mfrac><mrow></mrow><mi>x</mi></mfrac>`},
	} {
		html, err := Render(test.expr, false)
		if err != nil {
			t.Fatalf("Render(%q): %v", test.expr, err)
		}
		if got, ok := strings.Contains(html, test.want), true; got != ok {
			t.Errorf("Render(%q) contains %q: got %t, want %t; HTML = %q", test.expr, test.want, got, ok, html)
		}
	}
}

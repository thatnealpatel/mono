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
		{name: "DirectSum", expr: `V \oplus W`, want: []string{"<mo>⊕</mo>"}},
		{name: "Kernel", expr: `\ker f`, want: []string{"<mo>ker</mo>"}},
		{name: "NotDivides", expr: `p \nmid n`, want: []string{"<mo>∤</mo>"}},
		{name: "Perpendicular", expr: `a \perp b`, want: []string{"<mo>⟂</mo>"}},
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

func TestRenderRoutineSymbols(t *testing.T) {
	for _, test := range []struct {
		expr string
		want string
	}{
		{expr: `\neg`, want: `<mo lspace="0em" rspace="0em">¬</mo>`},
		{expr: `\Box`, want: `<mo lspace="0em" rspace="0em">□</mo>`},
		{expr: `\Diamond`, want: `<mo lspace="0em" rspace="0em">◇</mo>`},
		{expr: `\asymp`, want: `<mo>≍</mo>`},
		{expr: `\sim`, want: `<mo>∼</mo>`},
		{expr: `\gg`, want: `<mo>≫</mo>`},
		{expr: `\ll`, want: `<mo>≪</mo>`},
		{expr: `\gtrsim`, want: `<mo>≳</mo>`},
		{expr: `\Longleftrightarrow`, want: `<mo>⟺</mo>`},
		{expr: `\sin`, want: `<mo>sin</mo>`},
		{expr: `\cos`, want: `<mo>cos</mo>`},
		{expr: `\exp`, want: `<mo>exp</mo>`},
		{expr: `\ln`, want: `<mo>ln</mo>`},
	} {
		t.Run(test.expr, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if want := "<math>" + test.want + "</math>"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestRenderRoutineCommandSpacing(t *testing.T) {
	// Relations cancel a preceding binary
	// atom; named functions cancel a
	// following one.
	for _, test := range []struct {
		commands []string
		expr     string
		want     string
		spaces   int
	}{
		{
			commands: []string{`\asymp`, `\sim`, `\gg`, `\ll`, `\gtrsim`, `\Longleftrightarrow`},
			expr:     `a\mathbin{X}%s`,
			want:     `<mi>a</mi><mrow><mi>X</mi></mrow>`,
		},
		{
			commands: []string{`\sin`, `\cos`, `\exp`, `\ln`},
			expr:     `%s\mathbin{X}b`,
			want:     `<mrow><mi>X</mi></mrow><mi>b</mi>`,
		},
		{
			commands: []string{`\sin`, `\cos`, `\exp`, `\ln`},
			expr:     `a\mathbin{X}%s`,
			want:     `<mi>a</mi><mspace width="0.2222em"/><mrow><mi>X</mi></mrow><mspace width="0.2222em"/>`,
			spaces:   2,
		},
	} {
		for _, command := range test.commands {
			t.Run(command, func(t *testing.T) {
				got, err := Render(strings.Replace(test.expr, "%s", command, 1), false)
				if err != nil {
					t.Fatalf("Render: %v", err)
				}
				if !strings.Contains(got, test.want) || strings.Count(got, `<mspace`) != test.spaces {
					t.Errorf("unexpected binary spacing around %q; got %q, want %q and %d spaces", command, got, test.want, test.spaces)
				}
			})
		}
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
		{name: "Mathsf", expr: `\mathsf{Ab1}`, want: `<math><mrow><mi>𝖠</mi><mi>𝖻</mi><mn>𝟣</mn></mrow></math>`},
		{name: "Mathbf", expr: `\mathbf{Ab1}`, want: `<math><mrow><mi>𝐀</mi><mi>𝐛</mi><mn>𝟏</mn></mrow></math>`},
		{name: "Mathit", expr: `\mathit{Ah1}`, want: `<math><mrow><mi>𝐴</mi><mi>ℎ</mi><mn>1</mn></mrow></math>`},
		{name: "Mathfrak", expr: `\mathfrak{gRa}`, want: `<math><mrow><mi>𝔤</mi><mi>ℜ</mi><mi>𝔞</mi></mrow></math>`},
		{name: "MathbfScript", expr: `\mathbf{x}_1`, want: `<math><msub><mrow><mi>𝐱</mi></mrow><mn>1</mn></msub></math>`},
		{name: "WideTilde", expr: `\widetilde{ABC}`, want: `<math><mover><mrow><mi>A</mi><mi>B</mi><mi>C</mi></mrow><mo stretchy="true">~</mo></mover></math>`},
		{name: "WideHat", expr: `\widehat{ABC}`, want: `<math><mover accent="true"><mrow><mi>A</mi><mi>B</mi><mi>C</mi></mrow><mo stretchy="true">^</mo></mover></math>`},
		{name: "WideHatScript", expr: `\widehat x^2`, want: `<math><msup><mover accent="true"><mi>x</mi><mo stretchy="true">^</mo></mover><mn>2</mn></msup></math>`},
		{name: "WideHatBinarySpacing", expr: `\widehat{a\mathbin{X}b}`, want: `<math><mover accent="true"><mrow><mi>a</mi><mspace width="0.2222em"/><mrow><mi>X</mi></mrow><mspace width="0.2222em"/><mi>b</mi></mrow><mo stretchy="true">^</mo></mover></math>`},
		{name: "Texttt", expr: `\texttt{ab {12} < & >}`, want: `<math><mtext style="font-family: monospace">ab 12 &lt; &amp; &gt;</mtext></math>`},
		{name: "TextttOverride", expr: `\mathbf{\texttt{Ab12}}`, want: `<math><mrow><mtext style="font-family: monospace">Ab12</mtext></mrow></math>`},
		{name: "Phantom", expr: `a\phantom{b+1}c`, want: `<math><mrow><mi>a</mi><mphantom><mrow><mi>b</mi><mo>+</mo><mn>1</mn></mrow></mphantom><mi>c</mi></mrow></math>`},
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

func TestRenderTriangleright(t *testing.T) {
	const operator = `<mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">▷</mo>`
	const space = `<mspace width="0.2222em"/>`
	for _, test := range []struct {
		name string
		expr string
		want string
	}{
		{name: "Infix", expr: `a\triangleright2`, want: `<mrow><mi>a</mi>` + space + operator + space + `<mn>2</mn></mrow>`},
		{name: "Leading", expr: `\triangleright2`, want: `<mrow>` + operator + `<mn>2</mn></mrow>`},
		{name: "Trailing", expr: `a\triangleright`, want: `<mrow><mi>a</mi>` + operator + `</mrow>`},
		{name: "BeforeRelation", expr: `a\triangleright=2`, want: `<mrow><mi>a</mi>` + operator + `<mo>=</mo><mn>2</mn></mrow>`},
		{name: "Script", expr: `x_{a\triangleright2}`, want: `<msub><mi>x</mi><mrow><mi>a</mi>` + operator + `<mn>2</mn></mrow></msub>`},
		{name: "Scripted", expr: `a\triangleright_x2`, want: `<mrow><mi>a</mi>` + space + `<msub>` + operator + `<mi>x</mi></msub>` + space + `<mn>2</mn></mrow>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if want := "<math>" + test.want + "</math>"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestRenderStandaloneNormDelimiters(t *testing.T) {
	const open = `<mo fence="true" form="prefix" stretchy="false" lspace="0em" rspace="0em">‖</mo>`
	const close = `<mo fence="true" form="postfix" stretchy="false" lspace="0em" rspace="0em">‖</mo>`
	const relation = `<mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">R</mo>`
	const space = `<mspace width="0.2778em"/>`
	for _, test := range []struct {
		name string
		expr string
		want string
	}{
		{name: "Norm", expr: `\lVert{x}\rVert`, want: open + `<mi>x</mi>` + close},
		{name: "OpenClass", expr: `\lVert\mathrel{R}x\rVert`, want: open + relation + space + `<mi>x</mi>` + close},
		{name: "CloseClass", expr: `\lVert{x}\mathrel{R}\rVert`, want: open + `<mi>x</mi>` + space + relation + close},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.expr, false)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if want := "<math><mrow>" + test.want + "</mrow></math>"; got != want {
				t.Errorf("got %q, want %q", got, want)
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
		{name: "NormAliases", expr: `\bigl\lVert{x}\bigr\rVert`, want: `<math><mrow><mo fence="true" form="prefix" stretchy="true" minsize="1.2em" maxsize="1.2em">‖</mo><mi>x</mi><mo fence="true" form="postfix" stretchy="true" minsize="1.2em" maxsize="1.2em">‖</mo></mrow></math>`},
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

func TestRenderMovableLimits(t *testing.T) {
	for _, op := range []struct {
		name, text, large string
	}{
		{`\limsup`, "lim\u2009sup", "false"},
		{`\liminf`, "lim\u2009inf", "false"},
		{`\bigoplus`, "⨁", "true"},
	} {
		base := `<mo form="prefix" largeop="` + op.large + `" movablelimits="true" lspace="0em" rspace="0em">` + op.text + `</mo>`
		for _, attachment := range []struct {
			source, want string
		}{
			{"", base},
			{`_n`, `<munder accentunder="false">` + base + `<mi>n</mi></munder>`},
			{`^r`, `<mover accent="false">` + base + `<mi>r</mi></mover>`},
			{`_n^r`, `<munderover accent="false" accentunder="false">` + base + `<mi>n</mi><mi>r</mi></munderover>`},
			{`^r_n`, `<munderover accent="false" accentunder="false">` + base + `<mi>n</mi><mi>r</mi></munderover>`},
		} {
			for _, display := range []bool{false, true} {
				got, err := Render(op.name+attachment.source, display)
				if err != nil {
					t.Fatal(err)
				}
				open := `<math>`
				if display {
					open = `<math display="block">`
				}
				if want := open + attachment.want + `</math>`; got != want {
					t.Errorf("Render(%q, %t) = %q, want %q", op.name+attachment.source, display, got, want)
				}
			}
		}
	}
}

func TestRenderLimitOperatorContext(t *testing.T) {
	for _, test := range []struct {
		expr         string
		thin, medium int
	}{
		{`a\limsup_n b`, 2, 0},
		{`\liminf_n=0`, 0, 0},
		{`(\liminf_n)`, 0, 0},
		{`\limsup_n\bigoplus_i`, 1, 0},
		{`\bigoplus_i\mathbin{X}b`, 1, 0},
		{`a\mathbin{X}\limsup_n`, 0, 2},
	} {
		got, err := Render(test.expr, false)
		if err != nil {
			t.Fatal(err)
		}
		// The thin Op glue is separate from normalized binary glue.
		if count := strings.Count(got, `<mspace width="0.1667em"/>`); count != test.thin {
			t.Errorf("Render(%q) has %d thin spaces, want %d: %s", test.expr, count, test.thin, got)
		}
		if count := strings.Count(got, `<mspace width="0.2222em"/>`); count != test.medium {
			t.Errorf("Render(%q) has %d medium spaces, want %d: %s", test.expr, count, test.medium, got)
		}
	}
}

func TestRenderLimitsInCompactStyles(t *testing.T) {
	for _, expr := range []string{
		`\frac{\limsup_n a}{\liminf_n b}`,
		`\frac{\bigoplus_i^r V}{W}`,
		`x_{\liminf_n a}`,
		`x_{y_{\bigoplus_i^r V}}`,
	} {
		got, err := Render(expr, true)
		if err != nil {
			t.Fatal(err)
		}
		// Placement must remain native: local fraction/script style, not the outer
		// display flag, controls movable limits.
		if !strings.Contains(got, `movablelimits="true"`) || strings.Contains(got, `displaystyle=`) || strings.Contains(got, `<mstyle`) {
			t.Errorf("local style overridden in %s", got)
		}
	}
	got, err := Render(`\sum_n^r\bigsqcup_i\oplus`, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<msubsup><mo>∑</mo>`, `<msub><mo form="prefix" largeop="true" movablelimits="true">⨆</mo>`, `<mo>⊕</mo>`} {
		if !strings.Contains(got, want) {
			t.Errorf("unrelated operator behavior changed: %s", got)
		}
	}
}

func TestRenderAlignedRows(t *testing.T) {
	for _, test := range []struct {
		body       string
		rows, cols int
	}{
		{`x&=1\\y&=2`, 2, 2},
		{`x&=1&y&=2&z&=3\\u&=4\\&=5&w&=6\\`, 3, 6},
		{`x&=1&u\\y`, 2, 4},
		{`&\\&`, 2, 2},
		{`x&\\`, 1, 2},
		{``, 0, 2},
	} {
		got, err := Render(`\begin{aligned}`+test.body+`\end{aligned}`, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, `<math><mtable class="aligned" displaystyle="true" scriptlevel="0">`) {
			t.Errorf("aligned missing native display-style table: %s", got)
		}
		rows := strings.Split(got, `<mtr>`)[1:]
		if len(rows) != test.rows {
			t.Errorf("%q has %d rows, want %d: %s", test.body, len(rows), test.rows, got)
		}
		for _, row := range rows {
			if count := strings.Count(row, `<mtd>`); count != test.cols {
				t.Errorf("%q has %d columns, want %d", row, count, test.cols)
			}
		}
	}
}

func TestRenderAlignedRHS(t *testing.T) {
	infix := `<mo form="infix" fence="false" separator="false" stretchy="false" lspace="0em" rspace="0em">`
	for _, test := range []struct {
		cell, want string
	}{
		{`=x`, `<mrow><mspace width="0.2778em"/>` + infix + `=</mo><mspace width="0.2778em"/><mi>x</mi></mrow>`},
		{`\le x`, `<mrow><mspace width="0.2778em"/>` + infix + `≤</mo><mspace width="0.2778em"/><mspace width="0.25em"/><mi>x</mi></mrow>`},
		{`+x`, `<mrow><mspace width="0.2222em"/>` + infix + `+</mo><mspace width="0.2222em"/><mi>x</mi></mrow>`},
		{`\not=x`, `<mrow><mspace width="0.2778em"/>` + infix + `≠</mo><mspace width="0.2778em"/><mi>x</mi></mrow>`},
		{`\times x`, `<mrow><mspace width="0.2222em"/>` + infix + `×</mo><mspace width="0.2222em"/><mspace width="0.25em"/><mi>x</mi></mrow>`},
		{`+`, infix + `+</mo>`},
		{`\mathbin{X}x`, `<mrow><mspace width="0.2222em"/><mrow><mi>X</mi></mrow><mspace width="0.2222em"/><mi>x</mi></mrow>`},
		{`\mathrel{R}x`, `<mrow><mspace width="0.2778em"/>` + infix + `R</mo><mspace width="0.2778em"/><mi>x</mi></mrow>`},
		{`=^rx`, `<mrow><mspace width="0.2778em"/><msup>` + infix + `=</mo><mi>r</mi></msup><mspace width="0.2778em"/><mi>x</mi></mrow>`},
		{`x`, `<mi>x</mi>`},
		{`(x)`, `<mrow><mo stretchy="false">(</mo><mi>x</mi><mo stretchy="false">)</mo></mrow>`},
		{`{=x}`, `<mrow><mo>=</mo><mi>x</mi></mrow>`},
		{``, `<mrow></mrow>`},
	} {
		got, err := Render(`\begin{aligned}a&`+test.cell+`\end{aligned}`, false)
		if err != nil {
			t.Fatal(err)
		}
		want := `<mtd>` + test.want + `</mtd>`
		if !strings.Contains(got, want) {
			t.Errorf("RHS %q: got %s, want cell %s", test.cell, got, want)
		}
	}
}

func TestRenderAlignedNesting(t *testing.T) {
	for _, test := range []struct {
		expr                string
		tables, rows, cells int
	}{
		{`\frac{\left(\begin{aligned}x&=1\\y&=2\end{aligned}\right)}{z}`, 1, 2, 4},
		{`\begin{aligned}x&=\begin{aligned}u&=1\\v&=2\end{aligned}\\y&=3\end{aligned}`, 2, 4, 8},
		{`\begin{aligned}x&=\begin{cases}u&v\\w&z\end{cases}\\{a&b}&{c\\d}\end{aligned}`, 2, 4, 8},
	} {
		got, err := Render(test.expr, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, count := range []struct {
			tag  string
			want int
		}{{`<mtable`, test.tables}, {`<mtr>`, test.rows}, {`<mtd>`, test.cells}, {`<math`, 1}} {
			if n := strings.Count(got, count.tag); n != count.want {
				t.Errorf("%q: %s count %d, want %d: %s", test.expr, count.tag, n, count.want, got)
			}
		}
	}
	got, err := Render(`x_{\begin{aligned}a&=b\mathbin{X}c\end{aligned}c\mathbin{Y}d}`, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, `<mspace width="0.2222em"/>`) != 2 {
		t.Errorf("aligned failed to enter display style or restore script style: %s", got)
	}
}

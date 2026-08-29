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

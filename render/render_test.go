package render

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderMathSyntax(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "DollarInline", source: `before $x^2$ after`, want: []string{`<p>before <math><msup>`, `<mi>x</mi>`, `</math> after</p>`}},
		{name: "ParenInline", source: `before \(\alpha + 1\) after`, want: []string{`<math>`, `<mi>α</mi>`}},
		{name: "DollarDisplay", source: `$$\frac{a}{b}$$`, want: []string{`<math display="block">`, `<mfrac>`}},
		{name: "BracketDisplay", source: "\\[\n\\sqrt{x}\n\\]", want: []string{`<math display="block">`, `<msqrt>`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			html, err := Render(test.source)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			for _, want := range test.want {
				if got, ok := strings.Contains(string(html), want), true; got != ok {
					t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
				}
			}
		})
	}
}

func TestRenderDoesNotParseMathInCode(t *testing.T) {
	source := "`$x$`\n\n```tex\n$$x$$\n\\[x\\]\n```\n\n    $y$"
	html, err := Render(source)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Count(string(html), "<math"), 0; got != want {
		t.Errorf("math element count = %d, want %d: %q", got, want, html)
	}
	for _, want := range []string{"<code>$x$</code>", "$$x$$", `\[x\]`, "$y$"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderMathInTable(t *testing.T) {
	source := "| set | relation |\n| --- | --- |\n| $|S|$ | \\(x | y\\) |"
	html, err := Render(source)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Count(string(html), "<td>"), 2; got != want {
		t.Errorf("table cell count = %d, want %d: %q", got, want, html)
	}
	if got, want := strings.Count(string(html), "<math>"), 2; got != want {
		t.Errorf("math element count = %d, want %d: %q", got, want, html)
	}
	for _, want := range []string{"<mo>|</mo>", "<mi>S</mi>", "<mi>x</mi>", "<mi>y</mi>"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderSelectedExtensions(t *testing.T) {
	source := "## H {#chosen}\n\n~~gone~~ :smile: www.example.com\n\n- [x] done\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nnote[^n]\n\n[^n]: foot\n"
	html, err := Render(source)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		`<h2 id="chosen">`, "<del>gone</del>", "😄", `<a href="http://www.example.com">`,
		`type="checkbox"`, "<table>", `class="fn"`,
	} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderPreservesAuthoredHTML(t *testing.T) {
	html, err := Render(`<aside data-note="yes">trusted</aside>`)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := string(html), "<aside data-note=\"yes\">trusted</aside>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEscapesMathText(t *testing.T) {
	html, err := Render(`$x < y & \text{a < b & c}$`)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"<mo>&lt;</mo>", "<mo>&amp;</mo>", "a &lt; b &amp; c"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderKeepsUnknownCommandsVisible(t *testing.T) {
	html, err := Render(`$\unknown x$`)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `<merror><mtext>\unknown</mtext></merror>`
	if got, ok := strings.Contains(string(html), want), true; got != ok {
		t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
	}
}

func TestRenderRejectsMalformedMath(t *testing.T) {
	for _, source := range []string{`$\left(x$`, "$$\nx", `$x}$`} {
		_, err := Render(source)
		if !errors.Is(err, ErrInvalidMath) {
			t.Errorf("Render(%q) error = %v, want ErrInvalidMath", source, err)
		}
	}
}

func TestRenderLeavesUnmatchedInlineDelimiter(t *testing.T) {
	html, err := Render(`cost is $5`)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := string(html), "<p>cost is $5</p>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderDelimitedArrowAndNamedDelimiters(t *testing.T) {
	html, err := Render(`$\left\lfloor x \rightarrow y\right\rfloor$`)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"⌊", "→", "⌋"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderIgnoresEscapedDisplayCloser(t *testing.T) {
	html, err := Render("\\[\nx \\\\]\ny\n\\]")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Count(string(html), `<math display="block">`), 1; got != want {
		t.Errorf("display math count = %d, want %d; HTML = %q", got, want, html)
	}
	if got, want := strings.Contains(string(html), "<mi>y</mi>"), true; got != want {
		t.Errorf("contains y in MathML: got %t, want %t; HTML = %q", got, want, html)
	}
}

func TestRenderTableDoesNotStartMathAtEscapedDollar(t *testing.T) {
	html, err := Render("| a | b |\n|---|---|\n| \\$x|y$ | z |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"<td>$x</td>", "<td>y$</td>"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderTablePreservesLatexEscapedPipe(t *testing.T) {
	html, err := Render("| value |\n|---|\n| $x \\| y$ foo \\| bar |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"<mo>‖</mo>", " foo | bar"} {
		if got, ok := strings.Contains(string(html), want), true; got != ok {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", want, got, ok, html)
		}
	}
}

func TestRenderTableDoesNotTreatCodeAsMath(t *testing.T) {
	html, err := Render("| `$|$` | b |\n|---|---|\n| x | y |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Contains(string(html), "<table>"), false; got != want {
		t.Errorf("contains table: got %t, want %t; HTML = %q", got, want, html)
	}
}

func TestRenderTableUnescapesPipeInCode(t *testing.T) {
	html, err := Render("| c | b |\n|---|---|\n| `$ \\| $` | y |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Contains(string(html), "<code>$ | $</code>"), true; got != want {
		t.Errorf("contains unescaped code pipe: got %t, want %t; HTML = %q", got, want, html)
	}
}

func TestRenderTableFailedBackticksDoNotHideMath(t *testing.T) {
	for _, source := range []string{
		"| v |\n|---|\n| ``$x \\| y$` |",
		"| v |\n|---|\n| \\`$x \\| y$` |",
	} {
		html, err := Render(source)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if got, want := strings.Contains(string(html), "<mo>‖</mo>"), true; got != want {
			t.Errorf("contains LaTeX double pipe: got %t, want %t; HTML = %q", got, want, html)
		}
	}
}

func TestRenderTableCodeSpanDoesNotCrossCell(t *testing.T) {
	html, err := Render("| h1 | h2 | h3 |\n|---|---|---|\n| `x | $|$` | c |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, needle := range []string{"<td>c</td>", "<math><mo>|</mo></math>"} {
		if got, want := strings.Contains(string(html), needle), true; got != want {
			t.Errorf("contains %q: got %t, want %t; HTML = %q", needle, got, want, html)
		}
	}
}

func TestRenderTableMathBacktickDoesNotStartCode(t *testing.T) {
	html, err := Render("| h1 | h2 |\n|---|---|\n| $\\text{`}$ $x \\| y$ ` | c |")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.Contains(string(html), "<mo>‖</mo>"), true; got != want {
		t.Errorf("contains LaTeX double pipe: got %t, want %t; HTML = %q", got, want, html)
	}
}

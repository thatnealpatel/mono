package latex

import (
	"errors"
	"testing"
)

func TestParseStructures(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		check func(Node) bool
	}{
		{name: "Fraction", expr: `\frac{a}{b}`, check: func(node Node) bool {
			command, ok := node.(Command)
			return ok && command.Name == `\frac` && len(command.Args) == 2
		}},
		{name: "UnbracedLetterFraction", expr: `\frac ca`, check: func(node Node) bool {
			command, ok := node.(Command)
			return ok && len(command.Args) == 2 &&
				len(command.Args[0]) == 1 && command.Args[0][0] == Letter("c") &&
				len(command.Args[1]) == 1 && command.Args[1][0] == Letter("a")
		}},
		{name: "UnbracedNumberFraction", expr: `\frac12`, check: func(node Node) bool {
			command, ok := node.(Command)
			return ok && len(command.Args) == 2 &&
				len(command.Args[0]) == 1 && command.Args[0][0] == Number("1") &&
				len(command.Args[1]) == 1 && command.Args[1][0] == Number("2")
		}},
		{name: "RootIndex", expr: `\sqrt[3]{x}`, check: func(node Node) bool {
			command, ok := node.(Command)
			return ok && len(command.Args) == 1 && len(command.OptArgs) == 1
		}},
		{name: "SubSup", expr: `\sum_{i=0}^n`, check: func(node Node) bool {
			_, ok := node.(SubSup)
			return ok
		}},
		{name: "Delimited", expr: `\left(\frac{a}{b}\right)`, check: func(node Node) bool {
			delimited, ok := node.(Delimited)
			return ok && delimited.Open == "(" && delimited.Close == ")"
		}},
		{name: "Environment", expr: `\begin{cases}a&b\\c&d\end{cases}`, check: func(node Node) bool {
			environment, ok := node.(Env)
			return ok && environment.Name == "cases"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes, err := Parse(test.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(nodes) != 1 {
				t.Fatalf("node count = %d, want 1", len(nodes))
			}
			if !test.check(nodes[0]) {
				t.Errorf("node = %#v, want %s structure", nodes[0], test.name)
			}
		})
	}
}

func TestParseUnknownCommand(t *testing.T) {
	nodes, err := Parse(`\unknown{x}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := len(nodes), 2; got != want {
		t.Fatalf("node count = %d, want %d", got, want)
	}
	command, ok := nodes[0].(Command)
	if !ok {
		t.Fatalf("node type = %T, want Command", nodes[0])
	}
	if got, want := command.Name, `\unknown`; got != want {
		t.Errorf("command name = %q, want %q", got, want)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	for _, expr := range []string{`\frac{x}`, `{x`, `x^`, `\left(x`, `\middle|`, `\begin{cases}x`, `\Big x`, `\Big\alpha`, `\left\alpha x\right)`, `\left(x\right\beta`, `\mathrel`, `\mathbin`, `\mathscr`, `\widetilde`} {
		_, err := Parse(expr)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalid", expr, err)
		}
	}
}

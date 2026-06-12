package spec

import (
	"path/filepath"
	"testing"
)

// TestParseLiveGoYAML parses the live ../go.yaml and asserts the structural
// invariants the specinfo/oraclegen tools rely on: package presence, a
// receiver method round-tripping every field, and the trailing constants
// block. It fails closed if a future emit change drops a key the parser
// expects.
func TestParseLiveGoYAML(t *testing.T) {
	s, err := Load(filepath.Join("..", "go.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Packages) == 0 {
		t.Fatal("no packages parsed")
	}

	// monster is the largest package and the canonical receiver home.
	monster := pkgByName(t, s, "monster")
	if len(monster.Funcs) == 0 {
		t.Fatal("monster package has no funcs")
	}

	// NewMMFromTag exercises typed params with defaults and a note.
	tag := funcByName(t, s, "NewMMFromTag")
	if tag.Return != "*MM" {
		t.Errorf("NewMMFromTag return = %q, want *MM", tag.Return)
	}
	if tag.Py != "MM.__init__" {
		t.Errorf("NewMMFromTag py = %q, want MM.__init__", tag.Py)
	}
	if len(tag.Params) != 2 {
		t.Fatalf("NewMMFromTag params = %d, want 2", len(tag.Params))
	}
	if tag.Params[0].Name != "tag" || tag.Params[0].Type != "byte" {
		t.Errorf("NewMMFromTag param[0] = %+v, want {tag byte}", tag.Params[0])
	}

	// XLeech2.Mul carries a dispatch ladder and a calls hint.
	mul := receiverFunc(t, s, "XLeech2", "Mul")
	if len(mul.Dispatch) != 4 {
		t.Errorf("XLeech2.Mul dispatch rungs = %d, want 4", len(mul.Dispatch))
	}
	if len(mul.Calls) == 0 {
		t.Error("XLeech2.Mul has no calls hint")
	}

	// Constants block must round-trip name/value/c.
	if len(s.Constants) == 0 {
		t.Fatal("no constants parsed")
	}
	found := false
	for _, c := range s.Constants {
		if c.Name == "QState12MaxCols" {
			found = true
			if c.Value != "64" {
				t.Errorf("QState12MaxCols value = %q, want 64", c.Value)
			}
			if c.C != "QSTATE12_MAXCOLS" {
				t.Errorf("QState12MaxCols c = %q, want QSTATE12_MAXCOLS", c.C)
			}
		}
	}
	if !found {
		t.Error("QState12MaxCols constant not found")
	}
}

func pkgByName(t *testing.T, s *Spec, name string) Package {
	t.Helper()
	for _, p := range s.Packages {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("package %q not found", name)
	return Package{}
}

func funcByName(t *testing.T, s *Spec, name string) Func {
	t.Helper()
	for _, fn := range s.AllFuncs() {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("func %q not found", name)
	return Func{}
}

func receiverFunc(t *testing.T, s *Spec, recv, name string) Func {
	t.Helper()
	for _, fn := range s.AllFuncs() {
		if fn.Recv == recv && fn.Name == name {
			return fn
		}
	}
	t.Fatalf("method %s.%s not found", recv, name)
	return Func{}
}

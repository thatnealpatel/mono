package main

import "testing"

const leanSrcFile = `module

public import Mathlib.Topology.Foo

/-! Module doc should not be captured. -/

open Topology Filter

variable {α : Type*} [TopologicalSpace α]

/-- A single-line docstring. -/
theorem foo : True := trivial

/-- Multi-line
docstring here. -/
def bar : Nat := 1

/-- Doc before attribute. -/
@[simp]
structure Baz where
  x : Nat

namespace Outer

lemma inner : True := trivial

namespace Inner

def deep := 1

end Inner

end Outer
`

func TestExtractFile(t *testing.T) {
	decls := ExtractFile("test.lean", []byte(leanSrcFile))

	type want struct {
		kind, name, doc string
		line            int
	}
	expects := []want{
		{"theorem", "foo", "A single-line docstring.", 12},
		{"def", "bar", "Multi-line\ndocstring here.", 16},
		{"structure", "Baz", "Doc before attribute.", 20},
		{"lemma", "Outer.inner", "", 25},
		{"def", "Outer.Inner.deep", "", 29},
	}

	if len(decls) != len(expects) {
		for _, d := range decls {
			t.Logf("  %s %s (line %d)", d.Kind, d.Name, d.Line)
		}
		t.Fatalf("got %d declarations, want %d", len(decls), len(expects))
	}

	for i, want := range expects {
		d := decls[i]
		t.Run(d.Kind+"/"+d.Name, func(t *testing.T) {
			if d.Kind != want.kind {
				t.Errorf("kind: got %q, want %q", d.Kind, want.kind)
			}
			if d.Name != want.name {
				t.Errorf("name: got %q, want %q", d.Name, want.name)
			}
			if d.Docstring != want.doc {
				t.Errorf("doc: got %q, want %q", d.Docstring, want.doc)
			}
			if d.Line != want.line {
				t.Errorf("line: got %d, want %d", d.Line, want.line)
			}
		})
	}
}

func TestExtractInline(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Declaration
	}{
		{
			name: "section does not affect namespace",
			src: `section Foo
theorem bar : True := trivial
end Foo`,
			want: []Declaration{
				{Kind: "theorem", Name: "bar", Line: 2},
			},
		},
		{
			name: "nested namespace",
			src: `namespace A
namespace B
def f := 1
end B
end A`,
			want: []Declaration{
				{Kind: "def", Name: "A.B.f", Line: 3},
			},
		},
		{
			name: "dotted namespace",
			src: `namespace A.B.C
def g := 2
end A.B.C`,
			want: []Declaration{
				{Kind: "def", Name: "A.B.C.g", Line: 2},
			},
		},
		{
			name: "multiline signature with :=",
			src: `def foo (x : Nat)
    (y : Nat) : Nat :=
  x + y`,
			want: []Declaration{
				{Kind: "def", Name: "foo", Signature: "def foo (x : Nat) (y : Nat) : Nat", Line: 1},
			},
		},
		{
			name: "signature ending with where",
			src: `structure Foo where
  x : Nat`,
			want: []Declaration{
				{Kind: "structure", Name: "Foo", Signature: "structure Foo", Line: 1},
			},
		},
		{
			name: "signature ending with by",
			src: `theorem foo : 1 = 1 by
  rfl`,
			want: []Declaration{
				{Kind: "theorem", Name: "foo", Signature: "theorem foo : 1 = 1", Line: 1},
			},
		},
		{
			name: "multiple modifiers",
			src:  `private noncomputable def secret : ℝ := 0`,
			want: []Declaration{
				{Kind: "def", Name: "secret", Line: 1},
			},
		},
		{
			name: "anonymous instance with brackets",
			src:  `instance [BEq α] : Decidable α := sorry`,
			want: []Declaration{
				{Kind: "instance", Name: "", Line: 1},
			},
		},
		{
			name: "blank line clears pending docstring",
			src: `/-- orphan doc -/

theorem bar : True := trivial`,
			want: []Declaration{
				{Kind: "theorem", Name: "bar", Docstring: "", Line: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFile("test.lean", []byte(tt.src))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d decls, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Kind != w.Kind {
					t.Errorf("[%d] kind: got %q, want %q", i, g.Kind, w.Kind)
				}
				if g.Name != w.Name {
					t.Errorf("[%d] name: got %q, want %q", i, g.Name, w.Name)
				}
				if w.Signature != "" && g.Signature != w.Signature {
					t.Errorf("[%d] sig: got %q, want %q", i, g.Signature, w.Signature)
				}
				if g.Docstring != w.Docstring {
					t.Errorf("[%d] doc: got %q, want %q", i, g.Docstring, w.Docstring)
				}
				if g.Line != w.Line {
					t.Errorf("[%d] line: got %d, want %d", i, g.Line, w.Line)
				}
			}
		})
	}
}

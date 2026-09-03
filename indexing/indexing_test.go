package indexing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type wordTokenizer struct{}

func (wordTokenizer) Tokenize(text string) []string {
	var out []string
	for w := range strings.FieldsSeq(strings.ToLower(text)) {
		out = append(out, strings.FieldsFunc(w, func(r rune) bool { return r == '.' || r == '_' })...)
	}
	return out
}

func build(t *testing.T, recs []Record, names map[string]string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	b, err := Create(path, wordTokenizer{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, r := range recs {
		if err := b.Add(r); err != nil {
			t.Fatalf("Add %s: %v", r.Name, err)
		}
	}
	for n, f := range names {
		if err := b.AddName(n, f); err != nil {
			t.Fatalf("AddName %s: %v", n, err)
		}
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Errorf("temporary file left behind")
	}
	ix, err := Open(path, wordTokenizer{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

var corpus = []Record{
	{Name: "Nat.add_comm", Kind: "theorem", Signature: "theorem Nat.add_comm (n m : Nat) : n + m = m + n",
		Docstring: "Addition of natural numbers is commutative.", File: "Init/Nat.lean", Line: 4},
	{Name: "Nat.add_comm", Kind: "lemma", File: "Other/Nat.lean", Line: 9},
	{Name: "Group.Hom", Kind: "structure", Signature: "structure Group.Hom (G H : Type)",
		Docstring: "A homomorphism between two groups.", File: "Alg/Hom.lean", Line: 1},
	{Name: "Ring.map", Kind: "def", Signature: "def Ring.map",
		Docstring: "Maps a ring along a group homomorphism of its additive group. Long prose follows here.",
		File:      "Alg/Ring.lean", Line: 2},
}

var generated = map[string]string{
	"Fin.sum_univ":   "Mathlib/Fin.lean",
	"Extra.add_comm": "Mathlib/Extra.lean",
}

func TestLookupExact(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Lookup("Nat.add_comm")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "exact" {
		t.Errorf("mode = %q, want exact", env.Mode)
	}
	if len(env.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(env.Matches))
	}
	m := env.Matches[0]
	if m.Kind != "theorem" || m.Line != 4 || !strings.Contains(m.Docstring, "commutative") {
		t.Errorf("unexpected first match: %+v", m)
	}
	if env.Candidates != nil {
		t.Errorf("candidates = %v, want nil", env.Candidates)
	}
}

func TestLookupGenerated(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Lookup("Fin.sum_univ")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "exact" || len(env.Matches) != 1 {
		t.Fatalf("env = %+v", env)
	}
	m := env.Matches[0]
	if m.Kind != "generated" || m.File != "Mathlib/Fin.lean" || m.Line != 0 || m.Signature != "" {
		t.Errorf("unexpected match: %+v", m)
	}
}

func TestLookupMiss(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Lookup("Bogus.Add_Comm")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "miss" {
		t.Errorf("mode = %q, want miss", env.Mode)
	}
	if env.Matches == nil || len(env.Matches) != 0 {
		t.Errorf("matches = %#v, want empty non-nil", env.Matches)
	}
	want := []string{"Nat.add_comm", "Extra.add_comm"}
	if strings.Join(env.Candidates, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v", env.Candidates, want)
	}
}

func TestLookupMissNoCandidates(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Lookup("Nothing")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "miss" || env.Candidates != nil {
		t.Errorf("env = %+v", env)
	}
}

func TestSearchRanksNameAboveProse(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Search("group homomorphism")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "search" {
		t.Errorf("mode = %q, want search", env.Mode)
	}
	if len(env.Matches) != 2 {
		t.Fatalf("matches = %d, want 2: %+v", len(env.Matches), env.Matches)
	}
	if env.Matches[0].Name != "Group.Hom" {
		t.Errorf("top = %q, want Group.Hom", env.Matches[0].Name)
	}
	if env.Matches[0].Score <= env.Matches[1].Score {
		t.Errorf("scores not descending: %v", env.Matches)
	}
	for _, m := range env.Matches {
		if m.Docstring != "" {
			t.Errorf("%s: search returned full docstring", m.Name)
		}
		if m.Snippet == "" {
			t.Errorf("%s: missing snippet", m.Name)
		}
	}
}

func TestSearchStemsProse(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Search("groups homomorphisms")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Matches) == 0 {
		t.Fatal("stemmed query matched nothing")
	}
}

func TestSearchSyntaxCharactersAreInert(t *testing.T) {
	ix := build(t, corpus, generated)
	for _, q := range []string{`(n : Nat) "quoted" OR NOT`, `a* b: c^d`, `AND OR NOT`} {
		if _, err := ix.Search(q); err != nil {
			t.Errorf("Search(%q): %v", q, err)
		}
	}
}

func TestSearchEmpty(t *testing.T) {
	ix := build(t, corpus, generated)
	env, err := ix.Search("   ")
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "search" || len(env.Matches) != 0 || env.Matches == nil {
		t.Errorf("env = %+v", env)
	}
}

func TestOpenRejectsSchemaMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	b, err := Create(path, wordTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.db.Exec("pragma user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, wordTokenizer{}); err == nil {
		t.Error("Open accepted a mismatched schema version")
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.WriteFile(a, []byte("one"), 0o644)
	os.WriteFile(b, []byte("two"), 0o644)
	fp1, err := Fingerprint(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp1) != 16 {
		t.Errorf("len = %d, want 16", len(fp1))
	}
	os.WriteFile(b, []byte("three"), 0o644)
	fp2, _ := Fingerprint(a, b)
	if fp1 == fp2 {
		t.Error("fingerprint unchanged after content change")
	}
	if _, err := Fingerprint(a, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file did not error")
	}
}

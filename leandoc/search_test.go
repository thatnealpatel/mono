package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patel.codes/ranking"
)

func TestIsIdentQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"SingleToken", "Nat.add_comm", true},
		{"Bare", "foo", true},
		{"TwoWords", "group homomorphism", false},
		{"Tab", "a\tb", false},
		{"Newline", "a\nb", false},
		{"Empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := isIdentQuery(tt.query), tt.want; got != want {
				t.Errorf("isIdentQuery(%q) = %v, want %v", tt.query, got, want)
			}
		})
	}
}

func TestBuildNameIndex(t *testing.T) {
	decls := []Declaration{
		{Name: "Foo.bar", Kind: "def"},
		{Name: "Baz", Kind: "theorem"},
		{Name: "Foo.bar", Kind: "lemma"},
	}
	idx := buildNameIndex(decls)

	if got, want := len(idx["Foo.bar"]), 2; got != want {
		t.Errorf("Foo.bar count: got %d, want %d", got, want)
	}
	if got, want := len(idx["Baz"]), 1; got != want {
		t.Errorf("Baz count: got %d, want %d", got, want)
	}
	if _, ok := idx["Missing"]; ok {
		t.Errorf("unexpected key Missing")
	}
}

func TestFinalComponent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"Dotted", "Mathlib.Data.Foo", "Foo"},
		{"Single", "Foo", "Foo"},
		{"Deep", "A.B.C.D", "D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := finalComponent(tt.in), tt.want; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestModuleToFile(t *testing.T) {
	if got, want := moduleToFile("Mathlib.Data.Fintype.BigOperators"), "Mathlib/Data/Fintype/BigOperators.lean"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindCandidates(t *testing.T) {
	decls := []Declaration{
		{Name: "Foo.bar"},
		{Name: "Baz.bar"},
		{Name: "Qux.Bar"},
	}
	compiled := map[string]string{
		"Extra.bar": "Some.Module",
	}

	t.Run("CaseInsensitiveFinalComponent", func(t *testing.T) {
		got := findCandidates("X.bar", decls, compiled)
		if len(got) != 4 {
			t.Fatalf("got %d candidates, want 4: %v", len(got), got)
		}
		for _, c := range got {
			if !strings.EqualFold(finalComponent(c), "bar") {
				t.Errorf("unexpected candidate %q", c)
			}
		}
	})

	t.Run("CapAt10", func(t *testing.T) {
		var many []Declaration
		for i := range 15 {
			many = append(many, Declaration{Name: "N" + string(rune('A'+i)) + ".zzz"})
		}
		got := findCandidates("X.zzz", many, nil)
		if got, want := len(got), 10; got != want {
			t.Errorf("got %d candidates, want %d", got, want)
		}
	})
}

func setupCache(t *testing.T, decls []Declaration, names map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LEANDOC_CACHE_DIR", dir)

	declPath := filepath.Join(dir, "decls.gob")
	f, err := os.Create(declPath)
	if err != nil {
		t.Fatalf("create decls.gob: %v", err)
	}
	if err := gob.NewEncoder(f).Encode(decls); err != nil {
		t.Fatalf("encode decls: %v", err)
	}
	f.Close()

	namesPath := filepath.Join(dir, "names.gob")
	f, err = os.Create(namesPath)
	if err != nil {
		t.Fatalf("create names.gob: %v", err)
	}
	if err := gob.NewEncoder(f).Encode(names); err != nil {
		t.Fatalf("encode names: %v", err)
	}
	f.Close()

	return dir
}

func runSearchIdent(t *testing.T, query string, decls []Declaration, names map[string]string) Envelope {
	t.Helper()
	cache := setupCache(t, decls, names)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	if err := searchIdent(query, decls, cache); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("searchIdent: %v", err)
	}
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nraw: %s", err, buf.String())
	}
	return env
}

func TestSearchIdentExactTextHit(t *testing.T) {
	decls := []Declaration{
		{Name: "Nat.add_comm", Kind: "theorem", Signature: "(n m : Nat)", File: "Init/Data/Nat.lean", Line: 42},
	}
	names := map[string]string{}

	env := runSearchIdent(t, "Nat.add_comm", decls, names)

	if got, want := env.Mode, "exact"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if got, want := len(env.Matches), 1; got != want {
		t.Fatalf("matches: got %d, want %d", got, want)
	}
	if got, want := env.Matches[0].Kind, "theorem"; got != want {
		t.Errorf("kind: got %q, want %q", got, want)
	}
	if env.Candidates != nil {
		t.Errorf("candidates should be nil on exact hit, got %v", env.Candidates)
	}
}

func TestSearchIdentMultipleDecls(t *testing.T) {
	decls := []Declaration{
		{Name: "Foo.bar", Kind: "def", File: "A.lean", Line: 1},
		{Name: "Foo.bar", Kind: "lemma", File: "B.lean", Line: 2},
	}
	env := runSearchIdent(t, "Foo.bar", decls, map[string]string{})

	if got, want := env.Mode, "exact"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if got, want := len(env.Matches), 2; got != want {
		t.Errorf("matches: got %d, want %d", got, want)
	}
}

func TestSearchIdentGeneratedHit(t *testing.T) {
	decls := []Declaration{
		{Name: "Other.thing", Kind: "def"},
	}
	names := map[string]string{
		"Fin.sum_univ_eq_sum_range": "Mathlib.Data.Fintype.BigOperators",
	}

	env := runSearchIdent(t, "Fin.sum_univ_eq_sum_range", decls, names)

	if got, want := env.Mode, "exact"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if got, want := len(env.Matches), 1; got != want {
		t.Fatalf("matches: got %d, want %d", got, want)
	}
	m := env.Matches[0]
	if got, want := m.Kind, "generated"; got != want {
		t.Errorf("kind: got %q, want %q", got, want)
	}
	if got, want := m.File, "Mathlib/Data/Fintype/BigOperators.lean"; got != want {
		t.Errorf("file: got %q, want %q", got, want)
	}
	if got, want := m.Signature, ""; got != want {
		t.Errorf("signature should be empty, got %q", got)
	}
}

func TestSearchIdentMiss(t *testing.T) {
	decls := []Declaration{
		{Name: "Foo.bar", Kind: "def"},
		{Name: "Baz.bar", Kind: "theorem"},
	}
	names := map[string]string{
		"Extra.bar": "Some.Module",
	}

	env := runSearchIdent(t, "Nonexistent.bar", decls, names)

	if got, want := env.Mode, "miss"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if got, want := len(env.Matches), 0; got != want {
		t.Errorf("matches: got %d, want %d", got, want)
	}
	if got, want := len(env.Candidates), 3; got != want {
		t.Fatalf("candidates: got %d, want %d: %v", got, want, env.Candidates)
	}
}

func TestSearchIdentMissNoCandidates(t *testing.T) {
	decls := []Declaration{
		{Name: "Foo.bar", Kind: "def"},
	}
	env := runSearchIdent(t, "CompletelyUnknown", decls, map[string]string{})

	if got, want := env.Mode, "miss"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if env.Candidates != nil {
		t.Errorf("candidates should be nil, got %v", env.Candidates)
	}
}

func TestSearchIdentMissMatchesEmptyArray(t *testing.T) {
	decls := []Declaration{{Name: "Foo.bar", Kind: "def"}}
	cache := setupCache(t, decls, map[string]string{})

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	if err := searchIdent("CompletelyUnknown", decls, cache); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("searchIdent: %v", err)
	}
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	raw := buf.String()
	if !strings.Contains(raw, `"matches":[]`) {
		t.Errorf("expected matches:[] in JSON, got: %s", raw)
	}
}

func TestSearchModeEnvelope(t *testing.T) {
	decls := []Declaration{
		{Name: "Nat.add_comm", Kind: "theorem", Signature: "(n m : Nat)", File: "Init/Data/Nat.lean", Line: 42},
	}
	cache := setupCache(t, decls, map[string]string{})

	docs := make([]string, len(decls))
	for i := range decls {
		docs[i] = decls[i].DocText()
	}
	tok := LeanTokenizer{}
	bm := ranking.NewBM25(&ranking.BM25Params{K1: 1.2, B: 0.75, Tokenizer: tok})
	bm.Build(docs)

	bm25Path := filepath.Join(cache, "bm25.gob")
	f, err := os.Create(bm25Path)
	if err != nil {
		t.Fatalf("create bm25.gob: %v", err)
	}
	if _, err := bm.WriteTo(f); err != nil {
		t.Fatalf("write bm25: %v", err)
	}
	f.Close()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	if err := search("theorem add comm", false); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("search: %v", err)
	}
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
	}
	if got, want := env.Mode, "search"; got != want {
		t.Errorf("mode: got %q, want %q", got, want)
	}
	if env.Candidates != nil {
		t.Errorf("candidates should be nil on search mode, got %v", env.Candidates)
	}
}

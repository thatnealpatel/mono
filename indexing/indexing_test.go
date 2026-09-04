package indexing

import (
	"encoding/json"
	"net/url"
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
	t.Cleanup(func() { _ = b.Abort() })
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

func TestLookupCandidateOrder(t *testing.T) {
	recs := []Record{
		{Name: "Zeta.target", Kind: "def"},
		{Name: "Alpha.target", Kind: "def"},
	}
	names := map[string]string{
		"Zoo.target":   "Zoo.lean",
		"Beta.target":  "Beta.lean",
		"Alpha.target": "AlphaGenerated.lean",
	}
	ix := build(t, recs, names)
	env, err := ix.Lookup("Missing.Target")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha.target", "Zeta.target", "Beta.target", "Zoo.target"}
	if strings.Join(env.Candidates, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want declaration-backed then generated-only in lexical order: %v", env.Candidates, want)
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

func TestEnvelopeOmitsEmptyOptionalMatchFields(t *testing.T) {
	encoded, err := json.Marshal(Envelope{
		Mode:    "exact",
		Matches: []Match{{Name: "GF", Kind: "object"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"mode":"exact","matches":[{"name":"GF","kind":"object"}]}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Stat(%q) error = %v, want not exist", path, err)
	}
}

func assertOnlyFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	want := make(map[string]bool, len(paths))
	for _, path := range paths {
		want[filepath.Clean(path)] = false
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		path = filepath.Clean(path)
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected file %q (possible SQLite path alias)", path)
			return nil
		}
		want[path] = true
		return nil
	}); err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	for path, found := range want {
		if !found {
			t.Errorf("expected file %q does not exist", path)
		}
	}
}

var literalSQLitePathComponents = []string{
	"question?mark",
	"hash#mark",
	"percent%mark",
	"escaped-slash%2Fmark",
	"space mark",
	"all ?#% marks",
}

func TestSQLiteDSNEscapesLiteralRelativePath(t *testing.T) {
	got := sqliteDSN("relative/cache ?#%20/index.db", url.Values{
		"immutable": {"1"},
		"mode":      {"ro"},
	})
	want := "file:relative/cache%20%3F%23%2520/index.db?immutable=1&mode=ro"
	if got != want {
		t.Fatalf("sqliteDSN = %q, want %q", got, want)
	}
}

func TestBuilderPublishesLiteralSQLitePaths(t *testing.T) {
	for _, component := range literalSQLitePathComponents {
		t.Run(component, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "cache-"+component)
			if err := os.Mkdir(cache, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cache, "index.db")
			b, err := Create(path, wordTokenizer{})
			if err != nil {
				t.Fatalf("Create(%q): %v", path, err)
			}
			t.Cleanup(func() { _ = b.Abort() })
			if err := b.Add(Record{Name: "Exact.path", Kind: "def"}); err != nil {
				t.Fatal(err)
			}
			var journalMode string
			if err := b.db.QueryRow("pragma journal_mode").Scan(&journalMode); err != nil {
				t.Fatal(err)
			}
			if journalMode != "off" {
				t.Errorf("journal_mode = %q, want off", journalMode)
			}
			var synchronous int
			if err := b.db.QueryRow("pragma synchronous").Scan(&synchronous); err != nil {
				t.Fatal(err)
			}
			if synchronous != 0 {
				t.Errorf("synchronous = %d, want 0", synchronous)
			}
			assertAbsent(t, path)
			if _, err := os.Stat(path + ".tmp"); err != nil {
				t.Fatalf("temporary index %q does not exist before Close: %v", path+".tmp", err)
			}
			if err := b.Close(); err != nil {
				t.Fatalf("Close(%q): %v", path, err)
			}
			assertAbsent(t, path+".tmp")

			ix, err := Open(path, wordTokenizer{})
			if err != nil {
				t.Fatalf("Open(%q): %v", path, err)
			}
			env, err := ix.Lookup("Exact.path")
			if err != nil || env.Mode != "exact" || len(env.Matches) != 1 {
				t.Errorf("Lookup from literal path: env=%+v, err=%v", env, err)
			}
			if _, err := ix.db.Exec("delete from decls"); err == nil {
				t.Error("Open returned a writable index")
			}
			if err := ix.Close(); err != nil {
				t.Fatal(err)
			}

			// In particular, '?' and '#' must
			// not truncate the URI path and
			// percent escapes must not create a
			// differently named database.
			assertOnlyFiles(t, root, path)
		})
	}
}

func TestBuilderAbortCleansLiteralSQLitePaths(t *testing.T) {
	for _, component := range literalSQLitePathComponents {
		t.Run(component, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "cache-"+component)
			if err := os.Mkdir(cache, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cache, "index.db")
			b, err := Create(path, wordTokenizer{})
			if err != nil {
				t.Fatalf("Create(%q): %v", path, err)
			}
			if err := b.Add(Record{Name: "Aborted.path", Kind: "def"}); err != nil {
				t.Fatal(err)
			}
			if err := b.Abort(); err != nil {
				t.Fatalf("Abort(%q): %v", path, err)
			}
			assertAbsent(t, path)
			assertAbsent(t, path+".tmp")
			assertOnlyFiles(t, root)
		})
	}
}

func TestBuilderAbortIdempotent(t *testing.T) {
	stages := []struct {
		name string
		run  func(*testing.T, *Builder)
	}{
		{name: "empty"},
		{name: "add", run: func(t *testing.T, b *Builder) {
			if err := b.Add(Record{Name: "Nat.add"}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "add name", run: func(t *testing.T, b *Builder) {
			if err := b.AddName("Nat.generated", "Nat.lean"); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			b, err := Create(path, wordTokenizer{})
			if err != nil {
				t.Fatal(err)
			}
			if stage.run != nil {
				stage.run(t, b)
			}
			if err := b.Abort(); err != nil {
				t.Fatalf("Abort: %v", err)
			}
			if err := b.Abort(); err != nil {
				t.Fatalf("second Abort: %v", err)
			}
			assertAbsent(t, path)
			assertAbsent(t, path+".tmp")
			if err := b.Add(Record{Name: "too.late"}); err == nil {
				t.Error("Add after Abort succeeded")
			}
			if err := b.AddName("too.late", "late.lean"); err == nil {
				t.Error("AddName after Abort succeeded")
			}
			if err := b.Close(); err == nil {
				t.Error("Close after Abort succeeded")
			}
		})
	}
}

func TestBuilderAbortAfterClosePreservesIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	b, err := Create(path, wordTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Add(Record{Name: "Nat.add", Kind: "def"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Abort(); err != nil {
		t.Fatalf("Abort after Close: %v", err)
	}
	if err := b.Abort(); err != nil {
		t.Fatalf("second Abort after Close: %v", err)
	}
	assertAbsent(t, path+".tmp")

	ix, err := Open(path, wordTokenizer{})
	if err != nil {
		t.Fatalf("Open after Abort: %v", err)
	}
	defer ix.Close()
	env, err := ix.Lookup("Nat.add")
	if err != nil || env.Mode != "exact" {
		t.Fatalf("Lookup after Abort: env=%+v, err=%v", env, err)
	}
	if err := b.Close(); err == nil {
		t.Error("second Close succeeded")
	}
}

func TestBuilderAbortAfterAddFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	b, err := Create(path, wordTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.fts.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(Record{Name: "Nat.add"}); err == nil {
		t.Fatal("Add with closed FTS statement succeeded")
	}
	if err := b.Abort(); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, path)
	assertAbsent(t, path+".tmp")
}

func TestBuilderCloseFailureCleansTemporaryFile(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.db")
		old := []byte("existing index")
		if err := os.WriteFile(path, old, 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := Create(path, wordTokenizer{})
		if err != nil {
			t.Fatal(err)
		}
		if err := b.tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err == nil {
			t.Fatal("Close after rollback succeeded")
		}
		assertAbsent(t, path+".tmp")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(old) {
			t.Errorf("existing index changed to %q", got)
		}
	})

	t.Run("optimize", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.db")
		b, err := Create(path, wordTokenizer{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.tx.Exec("drop table fts"); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err == nil {
			t.Fatal("Close without fts table succeeded")
		}
		assertAbsent(t, path)
		assertAbsent(t, path+".tmp")
	})

	t.Run("closed database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.db")
		b, err := Create(path, wordTokenizer{})
		if err != nil {
			t.Fatal(err)
		}
		if err := b.db.Close(); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err == nil {
			t.Fatal("Close with closed database succeeded")
		}
		assertAbsent(t, path)
		assertAbsent(t, path+".tmp")
	})

	t.Run("rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "index.db")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(path, "keep")
		if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := Create(path, wordTokenizer{})
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err == nil {
			t.Fatal("Close renamed database over directory")
		}
		assertAbsent(t, path+".tmp")
		if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
			t.Errorf("destination changed: contents=%q, err=%v", got, err)
		}
	})
}

func TestCreateFailureDoesNotLeaveTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "index.db")
	if _, err := Create(path, wordTokenizer{}); err == nil {
		t.Fatal("Create in missing directory succeeded")
	}
	assertAbsent(t, path)
	assertAbsent(t, path+".tmp")

	path = filepath.Join(t.TempDir(), "index.db")
	tmp := path + ".tmp"
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "keep"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(path, wordTokenizer{}); err == nil {
		t.Fatal("Create ignored stale temporary-file removal failure")
	}
	if _, err := os.Stat(filepath.Join(tmp, "keep")); err != nil {
		t.Errorf("stale temporary directory was damaged: %v", err)
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

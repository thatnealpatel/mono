package retrieval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type oeisFixture struct {
	id       string
	name     string
	terms    string
	comments []string
	keywords string
}

func writeOeisFixture(t *testing.T, dir string, seqs []oeisFixture) string {
	t.Helper()
	seqDir := filepath.Join(dir, "oeisdata.git", "seq")
	for _, s := range seqs {
		shard := filepath.Join(seqDir, s.id[:4])
		if err := os.MkdirAll(shard, 0o755); err != nil {
			t.Fatalf("mkdir shard: %v", err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%%I %s\n", s.id)
		if s.terms != "" {
			fmt.Fprintf(&b, "%%S %s %s\n", s.id, s.terms)
		}
		fmt.Fprintf(&b, "%%N %s %s\n", s.id, s.name)
		for _, c := range s.comments {
			fmt.Fprintf(&b, "%%C %s %s\n", s.id, c)
		}
		if s.keywords != "" {
			fmt.Fprintf(&b, "%%K %s %s\n", s.id, s.keywords)
		}
		if err := os.WriteFile(filepath.Join(shard, s.id+".seq"), []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write seq: %v", err)
		}
	}
	return seqDir
}

func fixtureSeqs() []oeisFixture {
	return []oeisFixture{
		{id: "A000045", name: "Fibonacci numbers", terms: "0,1,1,2,3,5,8,13,21",
			comments: []string{"A classic sequence."}, keywords: "nonn,easy"},
		{id: "A000079", name: "Powers of 2", terms: "1,2,4,8,16,32,64"},
		{id: "A000040", name: "The prime numbers", terms: "2,3,5,7,11,13"},
		{id: "A000203", name: "sum of divisors of n", terms: "1,3,4,7,6,12"},
	}
}

func TestOeisStoreShow(t *testing.T) {
	dir := t.TempDir()
	writeOeisFixture(t, dir, fixtureSeqs())
	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}

	t.Run("FullParse", func(t *testing.T) {
		e, err := store.Show("A000045")
		if err != nil {
			t.Fatalf("Show: %v", err)
		}
		if got, want := e.ID, "A000045"; got != want {
			t.Errorf("got id %q, want %q", got, want)
		}
		if got, want := e.Name, "Fibonacci numbers"; got != want {
			t.Errorf("got name %q, want %q", got, want)
		}
		if got, want := e.Terms, "0,1,1,2,3,5,8,13,21"; got != want {
			t.Errorf("got terms %q, want %q", got, want)
		}
		if got, want := strings.Join(e.Comments, "|"), "A classic sequence."; got != want {
			t.Errorf("got comments %q, want %q", got, want)
		}
		if got, want := e.Keywords, "nonn,easy"; got != want {
			t.Errorf("got keywords %q, want %q", got, want)
		}
	})

	t.Run("LowercaseID", func(t *testing.T) {
		e, err := store.Show("a000079")
		if err != nil {
			t.Fatalf("Show: %v", err)
		}
		if got, want := e.Name, "Powers of 2"; got != want {
			t.Errorf("got name %q, want %q", got, want)
		}
	})

	t.Run("ShortID", func(t *testing.T) {
		if _, err := store.Show("A00"); err == nil {
			t.Errorf("got nil error for short id, want error")
		}
	})

	t.Run("UnknownID", func(t *testing.T) {
		if _, err := store.Show("A999999"); err == nil {
			t.Errorf("got nil error for unknown id, want error")
		}
	})
}

func TestOeisStoreSearch(t *testing.T) {
	dir := t.TempDir()
	writeOeisFixture(t, dir, fixtureSeqs())
	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}

	res := store.Search("prime numbers")
	if got, want := res.Query, "prime numbers"; got != want {
		t.Errorf("got query %q, want %q", got, want)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("got no matches, want at least one")
	}
	if got, want := res.Matches[0].ID, "A000040"; got != want {
		t.Errorf("got top match %q, want %q", got, want)
	}
	if got, want := res.Results, len(res.Matches); got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if got, want := res.Truncated, false; got != want {
		t.Errorf("got truncated %t, want %t", got, want)
	}
}

func TestOeisStoreSearchCap(t *testing.T) {
	seqs := make([]oeisFixture, 70)
	for i := range seqs {
		seqs[i] = oeisFixture{
			id:    fmt.Sprintf("A%06d", i),
			name:  "sequence about primes",
			terms: "2,3,5,7,11",
		}
	}
	dir := t.TempDir()
	writeOeisFixture(t, dir, seqs)
	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}

	res := store.Search("primes")
	if got, want := len(res.Matches), 50; got != want {
		t.Errorf("got %d matches, want %d", got, want)
	}
	if got, want := res.Results, 70; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if got, want := res.Truncated, true; got != want {
		t.Errorf("got truncated %t, want %t", got, want)
	}
}

func TestOeisStoreMatch(t *testing.T) {
	dir := t.TempDir()
	writeOeisFixture(t, dir, fixtureSeqs())
	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}

	t.Run("Substring", func(t *testing.T) {
		got := store.Match("1,2,4")
		if len(got) != 1 {
			t.Fatalf("got %d matches, want 1", len(got))
		}
		if want := "A000079"; got[0].ID != want {
			t.Errorf("got id %q, want %q", got[0].ID, want)
		}
	})

	t.Run("TrailingCommaNormalized", func(t *testing.T) {
		if got := store.Match("2,3,5,"); len(got) != 2 {
			t.Errorf("got %d matches, want 2", len(got))
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		if got := store.Match("99,98,97"); len(got) != 0 {
			t.Errorf("got %d matches, want 0", len(got))
		}
	})
}

func TestOeisStoreMalformedTolerance(t *testing.T) {
	dir := t.TempDir()
	seqDir := writeOeisFixture(t, dir, fixtureSeqs())

	junk := filepath.Join(seqDir, "A000", "A000999.seq")
	if err := os.WriteFile(junk, []byte("garbage\nx\n"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seqDir, "A000", "README"), []byte("skip me"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}
	if got := store.Search("Fibonacci"); len(got.Matches) == 0 || got.Matches[0].ID != "A000045" {
		t.Errorf("got %+v, want top match A000045", got.Matches)
	}
}

func TestLoadOeisStoreMissingDir(t *testing.T) {
	if _, err := NewOeis(filepath.Join(t.TempDir(), "absent"), 50); err == nil {
		t.Errorf("got nil error for missing data dir, want error")
	}
}

func TestNewOeis(t *testing.T) {
	dir := t.TempDir()
	writeOeisFixture(t, dir, fixtureSeqs())

	store, err := NewOeis(dir, 50)
	if err != nil {
		t.Fatalf("NewOeis: %v", err)
	}
	if _, err := store.Show("A000045"); err != nil {
		t.Errorf("Show after NewOeis: %v", err)
	}
}

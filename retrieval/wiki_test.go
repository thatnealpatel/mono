package retrieval

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"patel.codes/ranking"
)

const testDumpDate = "20260501"

func writeWikiFixture(t *testing.T, dir string, entries []wikiEntry) {
	t.Helper()

	marker := filepath.Join(dir, "enwiki-"+testDumpDate+"-pages-articles-multistream.xml.bz2")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatalf("writing dump marker: %v", err)
	}

	idxPath := filepath.Join(dir, "enwiki-"+testDumpDate+".index")
	f, err := os.Create(idxPath)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}
	if err := gob.NewEncoder(f).Encode(entries); err != nil {
		t.Fatalf("encoding index: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing index: %v", err)
	}

	titles := make([]string, len(entries))
	for i, e := range entries {
		titles[i] = e.Title
	}
	idf := ranking.NewIDF(nil)
	idf.Build(titles)
	cachePath := filepath.Join(dir, "enwiki-"+testDumpDate+"-idf.cache")
	cf, err := os.Create(cachePath)
	if err != nil {
		t.Fatalf("creating idf cache: %v", err)
	}
	if _, err := idf.WriteTo(cf); err != nil {
		t.Fatalf("writing idf cache: %v", err)
	}
	if err := cf.Close(); err != nil {
		t.Fatalf("closing idf cache: %v", err)
	}
}

func fixtureEntries() []wikiEntry {
	entries := []wikiEntry{
		{Title: "Alan Turing", Offset: 10},
		{Title: "Banana bread", Offset: 20},
		{Title: "Turing award", Offset: 30},
		{Title: "Turing machine", Offset: 40},
	}
	slices.SortFunc(entries, func(a, b wikiEntry) int { return strings.Compare(a.Title, b.Title) })
	return entries
}

func TestWikiStoreLookup(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())
	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}

	t.Run("Hit", func(t *testing.T) {
		if got, ok := store.Lookup("Turing machine"); !ok || got != 40 {
			t.Errorf("got (%d, %t), want (40, true)", got, ok)
		}
	})
	t.Run("Miss", func(t *testing.T) {
		if got, ok := store.Lookup("Nonexistent"); ok || got != 0 {
			t.Errorf("got (%d, %t), want (0, false)", got, ok)
		}
	})
	t.Run("CaseExact", func(t *testing.T) {
		if _, ok := store.Lookup("turing machine"); ok {
			t.Errorf("got ok=true for wrong-case title, want false")
		}
	})
}

func TestWikiStoreSearch(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())
	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}

	res := store.Search("turing machine")
	if got, want := res.Query, "turing machine"; got != want {
		t.Errorf("got query %q, want %q", got, want)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("got no matches, want at least one")
	}
	if got, want := res.Matches[0].Title, "Turing machine"; got != want {
		t.Errorf("got top match %q, want %q", got, want)
	}
	for _, m := range res.Matches {
		if m.Title == "Banana bread" {
			t.Errorf("got %q in matches, want it absent", m.Title)
		}
	}
	if got, want := res.Results, len(res.Matches); got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
}

func TestWikiStoreSearchEmpty(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())
	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}

	res := store.Search("")
	if got, want := res.Query, ""; got != want {
		t.Errorf("got query %q, want %q", got, want)
	}
	if got, want := res.Results, 0; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if got, want := len(res.Matches), 0; got != want {
		t.Errorf("got %d matches, want %d", got, want)
	}
}

func TestWikiStoreSearchMaxResults(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())
	store, err := NewWiki(dir, 1)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}

	res := store.Search("turing")
	if got, want := len(res.Matches), 1; got != want {
		t.Errorf("got %d matches, want %d", got, want)
	}
	if got, want := res.Results, 3; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if got, want := res.Truncated, true; got != want {
		t.Errorf("got truncated %t, want %t", got, want)
	}
}

func TestWikiStoreSearchNotTruncated(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())
	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}

	res := store.Search("turing")
	if got, want := res.Truncated, false; got != want {
		t.Errorf("got truncated %t, want %t", got, want)
	}
	if got, want := res.Results, len(res.Matches); got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
}

func TestLoadWikiStoreErrors(t *testing.T) {
	t.Run("MissingDir", func(t *testing.T) {
		if _, err := NewWiki(filepath.Join(t.TempDir(), "absent"), 100); err == nil {
			t.Errorf("got nil error for missing dir, want error")
		}
	})
	t.Run("NoDump", func(t *testing.T) {
		if _, err := NewWiki(t.TempDir(), 100); err == nil {
			t.Errorf("got nil error for dir without dump, want error")
		}
	})
	t.Run("MissingIndex", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "enwiki-"+testDumpDate+"-pages-articles-multistream.xml.bz2")
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			t.Fatalf("writing marker: %v", err)
		}
		if _, err := NewWiki(dir, 100); err == nil {
			t.Errorf("got nil error for missing index, want error")
		}
	})
	t.Run("CorruptIndex", func(t *testing.T) {
		dir := t.TempDir()
		writeWikiFixture(t, dir, fixtureEntries())
		idxPath := filepath.Join(dir, "enwiki-"+testDumpDate+".index")
		if err := os.WriteFile(idxPath, []byte("not gob"), 0o644); err != nil {
			t.Fatalf("corrupting index: %v", err)
		}
		if _, err := NewWiki(dir, 100); err == nil {
			t.Errorf("got nil error for corrupt index, want error")
		}
	})
	t.Run("MissingIDFCache", func(t *testing.T) {
		dir := t.TempDir()
		writeWikiFixture(t, dir, fixtureEntries())
		cachePath := filepath.Join(dir, "enwiki-"+testDumpDate+"-idf.cache")
		if err := os.Remove(cachePath); err != nil {
			t.Fatalf("removing idf cache: %v", err)
		}
		if _, err := NewWiki(dir, 100); err == nil {
			t.Errorf("got nil error for missing idf cache, want error")
		}
	})
	t.Run("CorruptIDFCache", func(t *testing.T) {
		dir := t.TempDir()
		writeWikiFixture(t, dir, fixtureEntries())
		cachePath := filepath.Join(dir, "enwiki-"+testDumpDate+"-idf.cache")
		if err := os.WriteFile(cachePath, []byte("bad magic bytes......"), 0o644); err != nil {
			t.Fatalf("corrupting idf cache: %v", err)
		}
		if _, err := NewWiki(dir, 100); err == nil {
			t.Errorf("got nil error for corrupt idf cache, want error")
		}
	})
}

func TestNewWiki(t *testing.T) {
	dir := t.TempDir()
	writeWikiFixture(t, dir, fixtureEntries())

	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki: %v", err)
	}
	if _, ok := store.Lookup("Alan Turing"); !ok {
		t.Errorf("Lookup after NewWiki failed, want ok")
	}
}

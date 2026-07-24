package retrieval

import (
	_ "embed"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

//go:embed testdata/wiki/index_fixture.bz2
var wikiIndexFixture string

//go:embed testdata/wiki/index_unsorted.bz2
var wikiIndexUnsorted string

func writeBz2IndexFixture(t *testing.T, dir string, data string) {
	t.Helper()
	name := "enwiki-" + testDumpDate + "-pages-articles-multistream-index.txt.bz2"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		t.Fatalf("writing bz2 index: %v", err)
	}
}

func writeDumpMarker(t *testing.T, dir string) {
	t.Helper()
	marker := filepath.Join(dir, "enwiki-"+testDumpDate+"-pages-articles-multistream.xml.bz2")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatalf("writing dump marker: %v", err)
	}
}

func TestBuildWikiRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeDumpMarker(t, dir)
	writeBz2IndexFixture(t, dir, wikiIndexFixture)

	if err := BuildWiki(dir); err != nil {
		t.Fatalf("BuildWiki: %v", err)
	}

	store, err := NewWiki(dir, 100)
	if err != nil {
		t.Fatalf("NewWiki after BuildWiki: %v", err)
	}

	t.Run("Lookup", func(t *testing.T) {
		for _, e := range fixtureEntries() {
			if got, ok := store.Lookup(e.Title); !ok || got != e.Offset {
				t.Errorf("Lookup(%q) = (%d, %t), want (%d, true)", e.Title, got, ok, e.Offset)
			}
		}
	})

	t.Run("Search", func(t *testing.T) {
		res := store.Search("turing machine")
		if len(res.Matches) == 0 {
			t.Fatalf("got no matches, want at least one")
		}
		if got, want := res.Matches[0].Title, "Turing machine"; got != want {
			t.Errorf("got top match %q, want %q", got, want)
		}
	})
}

func TestBuildWikiIndexSorted(t *testing.T) {
	dir := t.TempDir()
	writeDumpMarker(t, dir)
	writeBz2IndexFixture(t, dir, wikiIndexUnsorted)

	if err := BuildWiki(dir); err != nil {
		t.Fatalf("BuildWiki: %v", err)
	}

	idxPath := filepath.Join(dir, "enwiki-"+testDumpDate+".index")
	f, err := os.Open(idxPath)
	if err != nil {
		t.Fatalf("opening index: %v", err)
	}
	defer f.Close()
	var got []wikiEntry
	if err := gob.NewDecoder(f).Decode(&got); err != nil {
		t.Fatalf("decoding index: %v", err)
	}

	if !slices.IsSortedFunc(got, func(a, b wikiEntry) int {
		return strings.Compare(a.Title, b.Title)
	}) {
		titles := make([]string, len(got))
		for i, e := range got {
			titles[i] = e.Title
		}
		t.Errorf("index not sorted: %v", titles)
	}
}

func TestBuildWikiErrors(t *testing.T) {
	t.Run("MissingDir", func(t *testing.T) {
		err := BuildWiki(filepath.Join(t.TempDir(), "absent"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("got %v, want %v", err, os.ErrNotExist)
		}
	})
	t.Run("NoDump", func(t *testing.T) {
		if err := BuildWiki(t.TempDir()); err == nil {
			t.Errorf("got nil error for dir without dump, want error")
		}
	})
	t.Run("MissingIndex", func(t *testing.T) {
		dir := t.TempDir()
		writeDumpMarker(t, dir)
		err := BuildWiki(dir)
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("got %v, want %v", err, os.ErrNotExist)
		}
	})
}

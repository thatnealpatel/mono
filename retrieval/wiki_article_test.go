package retrieval

import (
	_ "embed"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/wiki/stream1.bz2
var wikiStream1 []byte

//go:embed testdata/wiki/stream2.bz2
var wikiStream2 []byte

func newArticleStore(t *testing.T) *Wiki {
	t.Helper()
	dir := t.TempDir()
	dump := append(append([]byte(nil), wikiStream1...), wikiStream2...)
	dumpPath := filepath.Join(dir, "enwiki-"+testDumpDate+"-pages-articles-multistream.xml.bz2")
	if err := os.WriteFile(dumpPath, dump, 0o644); err != nil {
		t.Fatalf("writing dump: %v", err)
	}
	off2 := int64(len(wikiStream1))
	entries := []wikiEntry{
		{Title: "Alpha", Offset: 0},
		{Title: "Beta", Offset: 0},
		{Title: "Gamma", Offset: off2},
		{Title: "Delta", Offset: off2},
		{Title: "Redirector", Offset: off2},
		{Title: "Ghost", Offset: off2},
	}
	slices.SortFunc(entries, func(a, b wikiEntry) int { return strings.Compare(a.Title, b.Title) })
	return &Wiki{dir: dir, date: testDumpDate, entries: entries, maxResults: 100}
}

func TestWikiStoreArticle(t *testing.T) {
	store := newArticleStore(t)

	t.Run("FirstStream", func(t *testing.T) {
		got, err := store.Article("Alpha")
		if err != nil {
			t.Fatalf("Article: %v", err)
		}
		if want := "Alpha is **bold** and *italic*."; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("SecondStream", func(t *testing.T) {
		got, err := store.Article("Gamma")
		if err != nil {
			t.Fatalf("Article: %v", err)
		}
		if want := "Gamma has Delta and Alpha links."; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("Redirect", func(t *testing.T) {
		got, err := store.Article("Redirector")
		if err != nil {
			t.Fatalf("Article: %v", err)
		}
		if want := "Gamma has Delta and Alpha links."; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("NotIndexed", func(t *testing.T) {
		if _, err := store.Article("Missing"); err == nil {
			t.Errorf("got nil error for unindexed title, want error")
		}
	})
	t.Run("NotInBlock", func(t *testing.T) {
		if _, err := store.Article("Ghost"); err == nil {
			t.Errorf("got nil error for title absent from its block, want error")
		}
	})
}

func TestWikiStoreLinks(t *testing.T) {
	store := newArticleStore(t)

	t.Run("Direct", func(t *testing.T) {
		got, err := store.Links("Gamma")
		if err != nil {
			t.Fatalf("Links: %v", err)
		}
		if want := "Gamma"; got.Title != want {
			t.Errorf("got title %q, want %q", got.Title, want)
		}
		if want := []string{"Delta", "Alpha"}; !slices.Equal(got.Links, want) {
			t.Errorf("got %v, want %v", got.Links, want)
		}
	})
	t.Run("Redirect", func(t *testing.T) {
		got, err := store.Links("Redirector")
		if err != nil {
			t.Fatalf("Links: %v", err)
		}
		if want := "Gamma"; got.Title != want {
			t.Errorf("got title %q, want %q", got.Title, want)
		}
		if want := []string{"Delta", "Alpha"}; !slices.Equal(got.Links, want) {
			t.Errorf("got %v, want %v", got.Links, want)
		}
	})
	t.Run("NotIndexed", func(t *testing.T) {
		if _, err := store.Links("Missing"); err == nil {
			t.Errorf("got nil error for unindexed title, want error")
		}
	})
}

func TestWikiStoreArticleConcurrent(t *testing.T) {
	store := newArticleStore(t)
	cases := map[string]string{
		"Alpha": "Alpha is **bold** and *italic*.",
		"Gamma": "Gamma has Delta and Alpha links.",
		"Delta": "Delta content.",
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	fails := map[string][2]string{}
	for range 8 {
		for title, want := range cases {
			wg.Go(func() {
				got, err := store.Article(title)
				if err != nil {
					mu.Lock()
					fails[title] = [2]string{"error: " + err.Error(), want}
					mu.Unlock()
					return
				}
				if got != want {
					mu.Lock()
					fails[title] = [2]string{got, want}
					mu.Unlock()
				}
			})
		}
	}
	wg.Wait()
	for title, gw := range fails {
		t.Errorf("Article(%q): got %q, want %q", title, gw[0], gw[1])
	}
}

func TestWikiClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"BoldItalic", "Alpha is '''bold''' and ''italic''.", "Alpha is **bold** and *italic*."},
		{"Heading", "== History ==\nBody text.", "## History\nBody text."},
		{"PipedLink", "See [[Gamma|the letter]] here.", "See the letter here."},
		{"PlainLink", "Links to [[Delta]] only.", "Links to Delta only."},
		{"ExternalLink", "Visit [https://example.com Example] now.", "Visit Example now."},
		{"RefStripped", "Text<ref>cite</ref>more.", "Textmore."},
		{"TemplateRemoved", "Keep {{delete me}}", "Keep"},
		{"InlineMath", "Energy <math>E=mc^2</math> here.", "Energy $E=mc^2$ here."},
		{"MvarTemplate", "The variable {{mvar|n}} counts.", "The variable $n$ counts."},
		{"SyntaxHighlight", `Code:<syntaxhighlight lang="verilog">wire a;</syntaxhighlight>done.`, "Code:\n```verilog\nwire a;\n```\ndone."},
		{"SourceTag", `Code:<source lang="python">x = 1</source>done.`, "Code:\n```python\nx = 1\n```\ndone."},
		{"SyntaxHighlightNoLang", `Code:<syntaxhighlight>x = 1</syntaxhighlight>done.`, "Code:\n```\nx = 1\n```\ndone."},
		{"SyntaxHighlightInsideRef", `See<ref>Example: <syntaxhighlight lang="c">int x;</syntaxhighlight></ref> end.`, "See end."},
		{"OrphanEndTag", `Hello</syntaxhighlight>world`, "Helloworld"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wikiClean(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

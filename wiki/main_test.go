package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikiClean(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text",
			in:   "Hello world",
			want: "Hello world",
		},
		{
			name: "bold",
			in:   "'''bold text'''",
			want: "**bold text**",
		},
		{
			name: "italic",
			in:   "''italic text''",
			want: "*italic text*",
		},
		{
			name: "wikilink with display text",
			in:   "[[Target|display text]]",
			want: "display text",
		},
		{
			name: "wikilink without display text",
			in:   "[[Simple Link]]",
			want: "Simple Link",
		},
		{
			name: "heading",
			in:   "== Section Title ==",
			want: "## Section Title",
		},
		{
			name: "deeper heading",
			in:   "=== Subsection ===",
			want: "### Subsection",
		},
		{
			name: "template stripped",
			in:   "before {{cite web|url=example.com}} after",
			want: "before  after",
		},
		{
			name: "file link stripped",
			in:   "text [[File:Example.jpg|thumb|A caption]] more",
			want: "text  more",
		},
		{
			name: "category link stripped",
			in:   "text [[Category:Mathematics]] more",
			want: "text  more",
		},
		{
			name: "external link",
			in:   "see [https://example.com Example Site] for details",
			want: "see Example Site for details",
		},
		{
			name: "math preserved",
			in:   "the formula <math>x^2 + y^2</math> is well-known",
			want: "the formula $x^2 + y^2$ is well-known",
		},
		{
			name: "table stripped",
			in:   "before\n{| class=\"wikitable\"\n|-\n| cell\n|}\nafter",
			want: "before\n\nafter",
		},
		{
			name: "triple newlines collapsed",
			in:   "a\n\n\n\nb",
			want: "a\n\nb",
		},
		{
			name: "ref tags stripped",
			in:   "fact<ref>source</ref> more",
			want: "fact more",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := wikiClean(tt.in)
			if got != tt.want {
				t.Errorf("wikiClean(%q):\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHtmlToText(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text passthrough",
			in:   "no html",
			want: "no html",
		},
		{
			name: "br inserts newline",
			in:   "line1<br>line2",
			want: "line1\nline2",
		},
		{
			name: "p inserts newline",
			in:   "<p>paragraph</p>",
			want: "\nparagraph",
		},
		{
			name: "ref content skipped",
			in:   "text<ref>citation</ref> more",
			want: "text more",
		},
		{
			name: "nested ref skipped",
			in:   "a<ref>outer<ref>inner</ref></ref>b",
			want: "ab",
		},
		{
			name: "gallery skipped",
			in:   "before<gallery>images</gallery>after",
			want: "beforeafter",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToText(tt.in)
			if got != tt.want {
				t.Errorf("htmlToText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanMathInner(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "sfrac",
			in:   "{{sfrac|a|b}}",
			want: "a/b",
		},
		{
			name: "not a typo",
			in:   "{{not a typo|special}}",
			want: "special",
		},
		{
			name: "superscript",
			in:   "x<sup>2</sup>",
			want: "x^{2}",
		},
		{
			name: "subscript",
			in:   "a<sub>n</sub>",
			want: "a_{n}",
		},
		{
			name: "bare template",
			in:   "{{pi}}",
			want: "pi",
		},
		{
			name: "wiki italic",
			in:   "''variable''",
			want: "variable",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanMathInner(tt.in)
			if got != tt.want {
				t.Errorf("cleanMathInner(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWikiProofBlocks(t *testing.T) {
	wikiCacheDir = os.Getenv("WIKI_CACHE_DIR")
	if wikiCacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			t.Fatal(err)
		}
		wikiCacheDir = filepath.Join(base, "wiki")
	}
	if err := wikiResolveDump(); err != nil {
		t.Skip("no dump available:", err)
	}
	wikiIdxPath = filepath.Join(wikiCacheDir, "enwiki-"+wikiDumpDate+".index")

	page, err := wikiFetchPage("Lucas's theorem")
	if err != nil {
		t.Fatal(err)
	}
	n := len(page.Revisions) - 1
	raw := page.Revisions[n].Text

	if !strings.Contains(raw, "{{Math proof") {
		t.Fatal("raw wikitext has no {{Math proof}} templates")
	}

	cleaned := wikiClean(raw)

	if !strings.Contains(cleaned, "Combinatorial proof") {
		t.Error("combinatorial proof title was stripped")
	}
	if !strings.Contains(cleaned, "cyclic group") {
		t.Error("combinatorial proof body was stripped")
	}
	if !strings.Contains(cleaned, "generating functions") {
		t.Error("generating functions proof title was stripped")
	}
	if !strings.Contains(cleaned, "Nathan Fine") {
		t.Error("generating functions proof body was stripped")
	}
}

func TestExtractMathTemplates(t *testing.T) {
	for _, tt := range []struct {
		name      string
		in        string
		wantText  string
		wantCount int
	}{
		{
			name:      "no math templates",
			in:        "plain text",
			wantText:  "plain text",
			wantCount: 0,
		},
		{
			name:      "math template",
			in:        "the value {{math|x + y}} is",
			wantText:  "the value WIKIMATH0ENDMATH is",
			wantCount: 1,
		},
		{
			name:      "mvar template",
			in:        "variable {{mvar|n}} here",
			wantText:  "variable WIKIMATH0ENDMATH here",
			wantCount: 1,
		},
		{
			name:      "multiple templates",
			in:        "{{math|a}} and {{math|b}}",
			wantText:  "WIKIMATH0ENDMATH and WIKIMATH1ENDMATH",
			wantCount: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var maths []string
			got := extractMathTemplates(tt.in, &maths)
			if got != tt.wantText {
				t.Errorf("text = %q, want %q", got, tt.wantText)
			}
			if len(maths) != tt.wantCount {
				t.Errorf("maths count = %d, want %d", len(maths), tt.wantCount)
			}
		})
	}
}

package main

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPageTemplate also exercises the package init: a malformed pageHTML would
// panic in template.Must before this test could run.
func TestPageTemplate(t *testing.T) {
	var buf bytes.Buffer
	err := page.Execute(&buf, pageData{Root: "demo", Tree: template.HTML(`<ul><li>x</li></ul>`)})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"--paper", "<title>demo</title>", `id="tree"`, `id="content"`, `<ul><li>x</li></ul>`} {
		if !strings.Contains(out, want) {
			t.Errorf("page output missing %q", want)
		}
	}
}

func TestToHTML(t *testing.T) {
	out := toHTML([]byte("# title\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"))
	if !strings.Contains(out, "<h1") {
		t.Errorf("expected <h1>, got %q", out)
	}
	if !strings.Contains(out, "<table>") {
		t.Errorf("expected <table>, got %q", out)
	}
}

func TestSafePath(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name   string
		wantOK bool
	}{
		{"README.md", true},
		{"docs/guide.md", true},
		{"../escape.md", true}, // neutralized to within root, not rejected
		{".git/config", false},
		{"docs/.secret", false},
	}
	for _, tt := range tests {
		full, ok := safePath(root, tt.name)
		if ok != tt.wantOK {
			t.Errorf("safePath(%q) ok = %v, want %v", tt.name, ok, tt.wantOK)
		}
		if ok && !strings.HasPrefix(full, root) {
			t.Errorf("safePath(%q) = %q escaped root %q", tt.name, full, root)
		}
	}
}

func TestTreeHTML(t *testing.T) {
	root := t.TempDir()
	for rel, content := range map[string]string{
		"README.md":         "# r",
		"docs/guide.md":     "# g",
		"docs/sub/deep.md":  "# d",
		".hidden/secret.md": "# s",
		"empty/notes.txt":   "x",
	} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	body, has, err := treeHTML(root, root)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected markdown to be found")
	}
	for _, want := range []string{"README.md", "guide.md", "deep.md", "docs/", "sub/"} {
		if !strings.Contains(body, want) {
			t.Errorf("tree missing %q", want)
		}
	}
	for _, bad := range []string{".hidden", "secret.md", "empty", "notes.txt"} {
		if strings.Contains(body, bad) {
			t.Errorf("tree should not contain %q (hidden or markdown-free)", bad)
		}
	}
}

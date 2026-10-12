package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patel.codes/render"
)

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a<&.md", "# Hello\n\n$x^2$\n\n<aside>authored HTML</aside>")
	w := httptest.NewRecorder()
	newHandler(root).ServeHTTP(w, httptest.NewRequest("GET", "/a%3C%26.md", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	for _, want := range []string{
		"<!doctype html>", `<meta charset="utf-8">`, `name="viewport"`,
		"<title>a&lt;&amp;.md</title>", `<main class="markdown">`,
		"<h1", "<math><msup>", "<aside>authored HTML</aside>",
		string(render.Stylesheet()),
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestRawFilesAndListings(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"plain.txt": "plain text", "page.html": "<h1>raw HTML</h1>",
		"upper.MD": "# Not rendered", ".hidden": "visible",
		"nested/.hidden": "also visible", "folder.md/child.txt": "child",
	} {
		writeFile(t, root, name, content)
	}
	handler := newHandler(root)
	raw := http.FileServer(http.Dir(root))
	for _, target := range []string{"/plain.txt", "/page.html", "/upper.MD", "/.hidden", "/", "/nested/", "/folder.md", "/folder.md/", "/missing.md"} {
		t.Run(target, func(t *testing.T) {
			got, want := httptest.NewRecorder(), httptest.NewRecorder()
			handler.ServeHTTP(got, httptest.NewRequest("GET", target, nil))
			raw.ServeHTTP(want, httptest.NewRequest("GET", target, nil))
			if got.Code != want.Code || got.Body.String() != want.Body.String() || got.Header().Get("Content-Type") != want.Header().Get("Content-Type") || got.Header().Get("Location") != want.Header().Get("Location") {
				t.Errorf("response differs from FileServer: got %d %q, want %d %q", got.Code, got.Body.String(), want.Code, want.Body.String())
			}
			if (target == "/" || target == "/nested/") && !strings.Contains(got.Body.String(), `href=".hidden"`) {
				t.Error("listing omitted dotfile")
			}
			if target == "/missing.md" && got.Code != http.StatusNotFound {
				t.Errorf("missing file status = %d", got.Code)
			}
		})
	}
}

func TestNestedMarkdownLinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "nested/note.md", "![picture](image.png)\n\n[next](next.md)")
	writeFile(t, root, "nested/image.png", "image bytes")
	writeFile(t, root, "nested/next.md", "# Next")
	handler := newHandler(root)
	r := httptest.NewRequest("GET", "/nested/note.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	for _, want := range []string{`src="image.png"`, `href="next.md"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if r.URL.Path != "/nested/note.md" {
		t.Fatalf("request path changed: %s", r.URL.Path)
	}
	for relative, want := range map[string]string{"image.png": "image bytes", "next.md": ">Next</h1>"} {
		target, err := r.URL.Parse(relative)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", target.String(), nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Errorf("relative %s: status %d, body %q", relative, w.Code, w.Body.String())
		}
	}
}

func TestInvalidMath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bad.md", `$\left(x$`)
	w := httptest.NewRecorder()
	newHandler(root).ServeHTTP(w, httptest.NewRequest("GET", "/bad.md", nil))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestMarkdownHTTP(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "note.md", "# Hello")
	handler := newHandler(root)
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest("GET", "/note.md", nil))
	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest("HEAD", "/note.md", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD: status %d, body %q", head.Code, head.Body.String())
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Last-Modified"} {
		if head.Header().Get(header) == "" || head.Header().Get(header) != get.Header().Get(header) {
			t.Errorf("HEAD %s = %q, GET = %q", header, head.Header().Get(header), get.Header().Get(header))
		}
	}
	r := httptest.NewRequest("GET", "/note.md", nil)
	r.Header.Set("If-Modified-Since", get.Header().Get("Last-Modified"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Errorf("conditional GET: status %d, body %q", w.Code, w.Body.String())
	}
}

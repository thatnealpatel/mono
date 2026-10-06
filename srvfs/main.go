// Command srvfs serves a directory of markdown files in the browser with
// opinionated, patel.codes-flavored formatting and a simple file tree rooted
// at the given directory. Rendering is ephemeral: nothing is written to disk.
package main

import (
	"cmp"
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"patel.codes/render"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: srvfs <dir>")
		os.Exit(1)
	}

	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Fatalf("%s is not a directory", root)
	}

	http.HandleFunc("/api/render", func(w http.ResponseWriter, r *http.Request) {
		renderFile(w, r, root)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, root)
	})

	const addr = ":11111"
	log.Printf("serving %s at http://%s", root, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// serve renders the application shell (file tree + empty content
// pane) for "/", and otherwise serves a raw asset from the tree
// so relative links such as images resolve.
func serve(w http.ResponseWriter, r *http.Request, root string) {
	if r.URL.Path == "/" {
		tree, err := renderTree(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.Execute(w, pageData{Root: filepath.Base(root), Tree: tree}); err != nil {
			log.Printf("render shell: %v", err)
		}
		return
	}

	full, ok := safePath(root, r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
}

// renderFile writes the HTML for a single markdown file named by
// the "file" query parameter. The result is a fragment injected
// into the content pane.
func renderFile(w http.ResponseWriter, r *http.Request, root string) {
	name := r.URL.Query().Get("file")
	if name == "" {
		http.Error(w, "missing file param", http.StatusBadRequest)
		return
	}
	full, ok := safePath(root, name)
	if !ok || !strings.HasSuffix(full, ".md") {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}
	src, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	fragment, err := toHTML(src)
	if err != nil {
		http.Error(w, "render markdown: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.WriteString(w, string(fragment)); err != nil {
		log.Printf("write render: %v", err)
	}
}

// safePath resolves name (a URL path or query
// value) against root, rejecting any result that
// escapes root via "..". The returned path is
// cleaned and absolute.
func safePath(root, name string) (string, bool) {
	clean := filepath.Clean("/" + strings.TrimPrefix(name, "/"))
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	for seg := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(seg, ".") {
			return "", false // refuse hidden path components
		}
	}
	return full, true
}

// toHTML renders trusted local Markdown to an
// HTML fragment, including MathML. Authored
// raw HTML is preserved; this is not a
// sanitizer. Serve only trusted local files,
// not untrusted uploads.
func toHTML(src []byte) (template.HTML, error) {
	return render.Render(string(src))
}

type pageData struct {
	Root string
	Tree template.HTML
}

// renderTree builds the nested <ul> file tree rooted
// at root, listing only markdown files and the
// directories that (transitively) contain them.
func renderTree(root string) (template.HTML, error) {
	body, _, err := treeHTML(root, root)
	if err != nil {
		return "", err
	}
	if body == "" {
		return `<p class="empty">no markdown files</p>`, nil
	}
	return template.HTML(body), nil
}

func treeHTML(root, dir string) (body string, hasMarkdown bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Name(), b.Name())
	})

	var b strings.Builder
	b.WriteString("<ul>")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // skip hidden files and directories (.git, .DS_Store, ...)
		}
		if e.IsDir() {
			inner, has, err := treeHTML(root, filepath.Join(dir, e.Name()))
			if err != nil {
				return "", false, err
			}
			if !has {
				continue
			}
			hasMarkdown = true
			fmt.Fprintf(&b, `<li><span class="dir">%s/</span>%s</li>`, html.EscapeString(e.Name()), inner)
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		hasMarkdown = true
		rel, err := filepath.Rel(root, filepath.Join(dir, e.Name()))
		if err != nil {
			return "", false, err
		}
		esc := html.EscapeString(filepath.ToSlash(rel))
		fmt.Fprintf(&b, `<li><a href="#%s" data-file="%s">%s</a></li>`, esc, esc, html.EscapeString(e.Name()))
	}
	b.WriteString("</ul>")
	if !hasMarkdown {
		return "", false, nil
	}
	return b.String(), true, nil
}

//go:embed templates/page.tmpl
var pageTemplate string

var page = template.Must(template.New("page").Funcs(template.FuncMap{"stylesheet": render.Stylesheet}).Parse(pageTemplate))

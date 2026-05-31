// Command srvfs serves a directory of markdown files in the browser with
// opinionated, patel.codes-flavored formatting and a simple file tree rooted
// at the given directory. Rendering is ephemeral: nothing is written to disk.
package main

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rsc.io/markdown"
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
		render(w, r, root)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, root)
	})

	const addr = ":11111"
	log.Printf("serving %s at http://%s", root, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// serve renders the application shell (file tree + empty content pane) for "/",
// and otherwise serves a raw asset from the tree so relative links such as
// images resolve.
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

// render writes the HTML for a single markdown file named by the "file" query
// parameter. The result is a fragment injected into the content pane.
func render(w http.ResponseWriter, r *http.Request, root string) {
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.WriteString(w, toHTML(src)); err != nil {
		log.Printf("write render: %v", err)
	}
}

// safePath resolves name (a URL path or query value) against root, rejecting any
// result that escapes root via "..". The returned path is cleaned and absolute.
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

// toHTML renders markdown source to an HTML fragment. The parser enables the
// GitHub-flavored feature set common in real-world markdown; smart-punctuation
// transforms are left off so output stays faithful to the source.
func toHTML(src []byte) string {
	p := markdown.Parser{
		HeadingID:          true,
		Strikethrough:      true,
		TaskList:           true,
		Table:              true,
		AutoLinkText:       true,
		AutoLinkAssumeHTTP: true,
		Emoji:              true,
		Footnote:           true,
	}
	return markdown.ToHTML(p.Parse(string(src)))
}

type pageData struct {
	Root string
	Tree template.HTML
}

// renderTree builds the nested <ul> file tree rooted at root, listing only
// markdown files and the directories that (transitively) contain them.
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
	sort.Slice(entries, func(i, j int) bool {
		if di, dj := entries[i].IsDir(), entries[j].IsDir(); di != dj {
			return di // directories before files
		}
		return entries[i].Name() < entries[j].Name()
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

var page = template.Must(template.New("page").Parse(pageHTML))

const pageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Root}}</title>
<style>
  @import url('https://fonts.googleapis.com/css2?family=PT+Mono&display=swap');
  :root {
    --paper: #fafafa;
    --ink: #2c2b28;
    --ink-light: #6b6963;
    --ink-faint: #a8a49c;
    --accent: #f8d551;
    --font-family: "PT Mono", monospace;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; border-radius: 0; }
  body { background: var(--paper); color: var(--ink-light); font-family: var(--font-family); font-size: 19px; line-height: 1.7; }
  ::selection { background: var(--accent); color: var(--ink); }
  #tree, #content { transition: box-shadow 0.12s ease; }
  #tree.focused, #content.focused { box-shadow: inset 2px 0 0 var(--ink); }

  #tree { position: fixed; top: 2.5rem; left: 2.5rem; width: 15rem; max-height: calc(100vh - 5rem); overflow-y: auto; font-size: 0.85rem; padding-left: 0.75rem; }
  #tree h2 { color: var(--ink-faint); font-weight: normal; font-size: 0.85rem; margin-bottom: 1rem; }
  #tree ul { list-style: none; padding-left: 1rem; }
  #tree > ul { padding-left: 0; }
  #tree li { margin: 0.1rem 0; white-space: nowrap; }
  #tree .dir { color: var(--ink-faint); }
  #tree a { color: var(--ink-light); text-decoration: none; }
  #tree a:hover { color: var(--ink); }
  #tree a.active { color: var(--ink); text-decoration: underline; text-decoration-color: var(--accent); text-underline-offset: 0.2em; }

  #content { max-width: 850px; margin: 2.5rem auto 4rem 20rem; padding: 0 1.5rem; }
  #content > :first-child { margin-top: 0; }
  #content h1, #content h2, #content h3, #content h4, #content h5, #content h6 { color: var(--ink); line-height: 1.3; margin: 1.6rem 0 0.6rem; }
  #content h1 { font-size: 1.6em; }
  #content h2 { font-size: 1.3em; }
  #content h3 { font-size: 1.1em; }
  #content p, #content li { color: var(--ink-light); }
  #content ul, #content ol { padding-left: 1.5em; margin: 0.8em 0; }
  #content a { color: var(--ink); text-decoration: underline; text-decoration-color: var(--ink-faint); text-underline-offset: 0.2em; }
  #content a:hover { text-decoration-color: var(--ink); }
  #content hr { border: none; border-top: 1px solid var(--ink-faint); margin: 2rem 0; }
  #content blockquote { border-left: 2px solid var(--ink); padding-left: 1rem; margin: 1em 0; }
  #content pre { border: 1px solid var(--ink-faint); padding: 1rem; overflow-x: auto; margin: 1em 0; font-size: 0.85em; line-height: 1.5; }
  #content code { font-family: var(--font-family); font-size: 0.9em; }
  #content :not(pre) > code { border: 1px solid var(--ink-faint); padding: 0.05em 0.3em; }
  #content table { border-collapse: collapse; margin: 1em 0; }
  #content th, #content td { border: 1px solid var(--ink-faint); padding: 0.4rem 0.75rem; text-align: left; }
  #content th { color: var(--ink-faint); }
  #content img { max-width: 100%; }
  #content del { color: var(--ink-faint); }
  #empty { color: var(--ink-faint); margin-top: 30vh; }

  @media (max-width: 800px) {
    #tree { position: static; width: auto; max-height: none; margin: 1.5rem; }
    #content { margin: 1.5rem auto; }
  }
</style>
</head>
<body>
<nav id="tree">
<h2>files</h2>
{{.Tree}}
</nav>
<main id="content">
<div id="empty">select a file</div>
</main>
<script>
  const content = document.getElementById('content');
  const tree = document.getElementById('tree');
  const files = Array.from(document.querySelectorAll('#tree a'));
  let sel = -1;        // index into files of the selected entry
  let focus = 'tree';  // focused zone: 'tree' or 'content'

  function applyFocus() {
    tree.classList.toggle('focused', focus === 'tree');
    content.classList.toggle('focused', focus === 'content');
  }
  applyFocus();

  function setActive(file) {
    files.forEach(function (a) { a.classList.toggle('active', a.dataset.file === file); });
  }

  // hist is 'push' (clicks), 'replace' (keyboard), or null (history nav).
  async function load(file, hist) {
    const res = await fetch('/api/render?file=' + encodeURIComponent(file));
    if (!res.ok) {
      content.innerHTML = '<div id="empty">could not load ' + file + '</div>';
      return;
    }
    content.innerHTML = await res.text();
    setActive(file);
    if (hist === 'push') history.pushState(null, '', '#' + encodeURIComponent(file));
    else if (hist === 'replace') history.replaceState(null, '', '#' + encodeURIComponent(file));
    window.scrollTo(0, 0);
  }

  function select(i) {
    if (i < 0 || i >= files.length) return;
    sel = i;
    files[i].scrollIntoView({ block: 'nearest' });
    load(files[i].dataset.file, 'replace');
  }

  // j/k: move one file, clamped at the ends.
  function move(delta) {
    if (!files.length) return;
    select(sel < 0 ? 0 : Math.max(0, Math.min(files.length - 1, sel + delta)));
  }

  // h/l: jump to the first file of the previous/next directory group, wrapping.
  function parentOf(a) {
    const p = a.dataset.file, i = p.lastIndexOf('/');
    return i < 0 ? '' : p.slice(0, i);
  }
  const leaders = files
    .map(function (_, i) { return i; })
    .filter(function (i) { return i === 0 || parentOf(files[i]) !== parentOf(files[i - 1]); });

  function jumpParent(delta) {
    if (!leaders.length) return;
    const cur = sel < 0 ? 0 : sel;
    let g = 0;
    for (let k = 0; k < leaders.length; k++) {
      if (leaders[k] <= cur) g = k; else break;
    }
    g = (g + delta + leaders.length) % leaders.length;
    select(leaders[g]);
  }

  files.forEach(function (a, i) {
    a.addEventListener('click', function (e) {
      e.preventDefault();
      sel = i;
      focus = 'tree';
      applyFocus();
      load(a.dataset.file, 'push');
    });
  });

  const SCROLL = 64;
  document.addEventListener('keydown', function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case 'Tab':
        focus = focus === 'tree' ? 'content' : 'tree';
        applyFocus();
        break;
      case 'j':
        if (focus === 'tree') move(1); else window.scrollBy(0, SCROLL);
        break;
      case 'k':
        if (focus === 'tree') move(-1); else window.scrollBy(0, -SCROLL);
        break;
      case 'h':
        if (focus !== 'tree') return;
        jumpParent(-1);
        break;
      case 'l':
        if (focus !== 'tree') return;
        jumpParent(1);
        break;
      default:
        return;
    }
    e.preventDefault();
  });

  function syncSelFromHash(f) {
    const i = files.findIndex(function (a) { return a.dataset.file === f; });
    if (i >= 0) sel = i;
  }

  window.addEventListener('popstate', function () {
    const f = location.hash ? decodeURIComponent(location.hash.slice(1)) : '';
    if (!f) return;
    syncSelFromHash(f);
    load(f, null);
  });

  const initial = location.hash ? decodeURIComponent(location.hash.slice(1)) : '';
  if (initial) {
    syncSelFromHash(initial);
    load(initial, null);
  }
</script>
</body>
</html>
`

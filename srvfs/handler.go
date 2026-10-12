package main

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"net/http"
	"path"

	"patel.codes/render"
)

var markdownPage = template.Must(template.New("markdown").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>{{.Stylesheet}}
body { max-width: 52rem; margin: auto; padding: 1rem; }
</style>
</head>
<body>
<main class="markdown">{{.Content}}</main>
</body>
</html>
`))

func newHandler(root string) http.Handler {
	files := http.Dir(root)
	raw := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Ext(r.URL.Path) != ".md" {
			raw.ServeHTTP(w, r)
			return
		}
		file, err := files.Open(r.URL.Path)
		if err != nil {
			raw.ServeHTTP(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			raw.ServeHTTP(w, r)
			return
		}
		src, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		content, err := render.Render(string(src))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, render.ErrInvalidMath) {
				status = http.StatusUnprocessableEntity
			}
			http.Error(w, err.Error(), status)
			return
		}
		var document bytes.Buffer
		err = markdownPage.Execute(&document, struct {
			Title      string
			Stylesheet template.CSS
			Content    template.HTML
		}{info.Name(), render.Stylesheet(), content})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, info.Name(), info.ModTime(), bytes.NewReader(document.Bytes()))
	})
}

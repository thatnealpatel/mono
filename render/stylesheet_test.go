package render

import (
	"bytes"
	"html/template"
	"os"
	"strings"
	"testing"
)

func TestStylesheet(t *testing.T) {
	fixed, err := os.ReadFile("markdown.css")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Stylesheet()); got != string(fixed) {
		t.Fatal("Stylesheet must return only the embedded CSS")
	}
	for _, want := range []string{
		"color-scheme: light;", "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
		"color: #2d2d2d;", "background: #fff;", "line-height: 1.5;",
		"overflow-wrap: anywhere;", ".markdown > :first-child", ".markdown > :last-child",
		"padding-left: 2rem;", "border-left: 0.25rem solid #8a8a8a;",
		"color: #0e7187;", "color: #008000;", "background: #ffffe1;",
		".markdown pre code *", "border-collapse: collapse;", "border: 1px solid #8a8a8a;",
		`.markdown math[display="block"]`,
	} {
		if !strings.Contains(string(Stylesheet()), want) {
			t.Errorf("stylesheet missing %q", want)
		}
	}
	for _, bad := range []string{"@import", "body", ".markdown p {", "font-size:", "* { margin:"} {
		if strings.Contains(string(Stylesheet()), bad) {
			t.Errorf("stylesheet contains out-of-contract rule %q", bad)
		}
	}
}

func TestStylesheetTemplate(t *testing.T) {
	tmpl := template.Must(template.New("style").Funcs(template.FuncMap{"stylesheet": Stylesheet}).Parse(`<style>{{stylesheet}}</style>`))
	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}
	if want := "<style>" + string(Stylesheet()) + "</style>"; out.String() != want {
		t.Fatalf("CSS was not injected verbatim: %q", out.String())
	}
}

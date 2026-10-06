package render

import (
	_ "embed"
	"html/template"
)

//go:embed markdown.css
var stylesheet string

// Stylesheet returns the fixed, trusted
// document stylesheet for inline use
// in an html/template style element.
// It sets root typography and colors
// and styles content within a .markdown
// container, including MathML. It
// does not provide page layout or
// application chrome. Paragraphs retain
// native browser margins except at the
// container's first and last child
// boundaries.
//
// The returned CSS is embedded at build
// time; no caller input is marked
// trusted.
func Stylesheet() template.CSS {
	return template.CSS(stylesheet)
}

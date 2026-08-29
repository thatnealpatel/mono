// Package render converts trusted Markdown to HTML with opinionated GFM-style
// extensions and MathML. It preserves authored raw HTML and is not a sanitizer.
package render

import (
	"errors"
	"fmt"
	"html/template"

	"patel.codes/render/internal/markdown"
)

// ErrInvalidMath reports malformed LaTeX inside recognized math delimiters.
var ErrInvalidMath = errors.New("invalid Markdown math")

// Render converts trusted Markdown to HTML.
func Render(source string) (template.HTML, error) {
	parser := markdown.Parser{
		HeadingID:          true,
		Strikethrough:      true,
		TaskList:           true,
		Table:              true,
		AutoLinkText:       true,
		AutoLinkAssumeHTTP: true,
		Emoji:              true,
		Footnote:           true,
		Math:               true,
	}
	html, err := markdown.RenderHTML(parser.Parse(source))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidMath, err)
	}
	return template.HTML(html), nil
}

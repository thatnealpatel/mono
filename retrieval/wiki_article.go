package retrieval

import (
	"compress/bzip2"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dustin/go-wikiparse"
	"golang.org/x/net/html"
)

func (w *Wiki) Article(title string) (string, error) {
	page, err := w.fetchPage(title)
	if err != nil {
		return "", err
	}
	page, err = w.followRedirect(page)
	if err != nil {
		return "", err
	}
	if len(page.Revisions) == 0 {
		return "", fmt.Errorf("no revisions for %q", title)
	}
	n := len(page.Revisions) - 1
	return wikiClean(page.Revisions[n].Text), nil
}

func (w *Wiki) fetchPage(title string) (*wikiparse.Page, error) {
	offset, ok := w.Lookup(title)
	if !ok {
		return nil, fmt.Errorf("article not found: %q", title)
	}
	dump := filepath.Join(w.dir, "enwiki-"+w.date+"-pages-articles-multistream.xml.bz2")
	f, err := os.Open(dump)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bzip2.NewReader(f))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "page" {
			continue
		}
		var page wikiparse.Page
		if err := dec.DecodeElement(&page, &se); err != nil {
			return nil, err
		}
		if page.Title == title {
			return &page, nil
		}
	}
	return nil, fmt.Errorf("article %q not in block at offset %d", title, offset)
}

func (w *Wiki) followRedirect(page *wikiparse.Page) (*wikiparse.Page, error) {
	if page.Redir.Title == "" {
		return page, nil
	}
	return w.fetchPage(page.Redir.Title)
}

func wikiClean(text string) string {
	var maths []string
	text = reMath.ReplaceAllStringFunc(text, func(m string) string {
		inner := reNotATypo.ReplaceAllString(reMath.FindStringSubmatch(m)[1], "$1")
		maths = append(maths, inner)
		return fmt.Sprintf("WIKIMATH%dENDMATH", len(maths)-1)
	})
	text = extractMathTemplates(text, &maths)
	text = extractProofBlocks(text)
	text = reFile.ReplaceAllString(text, "")
	text = reTable.ReplaceAllString(text, "")
	for range 5 {
		next := reTmpl.ReplaceAllString(text, "")
		if next == text {
			break
		}
		text = next
	}
	text = htmlToText(text)
	text = reHeading.ReplaceAllStringFunc(text, func(m string) string {
		parts := reHeading.FindStringSubmatch(m)
		return strings.Repeat("#", len(parts[1])) + " " + parts[2]
	})
	text = reBold.ReplaceAllString(text, "**$1**")
	text = reItalic.ReplaceAllString(text, "*$1*")
	text = reLink.ReplaceAllString(text, "$1")
	text = reExtLink.ReplaceAllString(text, "$1")
	for i, m := range maths {
		text = strings.ReplaceAll(text, fmt.Sprintf("WIKIMATH%dENDMATH", i), "$"+m+"$")
	}
	text = reMultiNL.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

var (
	reMathTmpl = regexp.MustCompile(`\{\{(?:math|mvar)\|`)
	reNotATypo = regexp.MustCompile(`\{\{not a typo\|([^}]+)\}\}`)
)

func findClosingBraces(text string, start int) int {
	depth := 0
	for i := start; i < len(text)-1; i++ {
		if text[i] == '{' && text[i+1] == '{' {
			depth++
			i++
		} else if text[i] == '}' && text[i+1] == '}' {
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func extractMathTemplates(text string, maths *[]string) string {
	for {
		loc := reMathTmpl.FindStringIndex(text)
		if loc == nil {
			break
		}
		end := findClosingBraces(text, loc[0])
		if end == -1 {
			break
		}
		inner := text[loc[1] : end-2]
		*maths = append(*maths, cleanMathInner(inner))
		placeholder := fmt.Sprintf("WIKIMATH%dENDMATH", len(*maths)-1)
		text = text[:loc[0]] + placeholder + text[end:]
	}
	return text
}

var (
	reSfrac    = regexp.MustCompile(`\{\{sfrac\|([^{}|]+)\|([^{}|]+)\}\}`)
	reTmplBare = regexp.MustCompile(`\{\{([^|{}]+)\}\}`)
	reSup      = regexp.MustCompile(`<sup>([^<]*)</sup>`)
	reSub      = regexp.MustCompile(`<sub>([^<]*)</sub>`)
	reWikiIta  = regexp.MustCompile(`''([^']+)''`)
)

func cleanMathInner(s string) string {
	s = reSfrac.ReplaceAllString(s, "$1/$2")
	s = reNotATypo.ReplaceAllString(s, "$1")
	s = reTmplBare.ReplaceAllString(s, "$1")
	s = reSup.ReplaceAllString(s, "^{$1}")
	s = reSub.ReplaceAllString(s, "_{$1}")
	s = reWikiIta.ReplaceAllString(s, "$1")
	return s
}

var reProofTmpl = regexp.MustCompile(`\{\{[Mm]ath proof\s*\|`)

func extractProofBlocks(text string) string {
	for {
		loc := reProofTmpl.FindStringIndex(text)
		if loc == nil {
			break
		}
		end := findClosingBraces(text, loc[0])
		if end == -1 {
			break
		}
		inner := text[loc[1] : end-2]
		title, proof := parseProofParams(inner)
		var replacement string
		if title != "" {
			replacement = "\n**" + title + "**\n" + proof + "\n"
		} else {
			replacement = "\n" + proof + "\n"
		}
		text = text[:loc[0]] + replacement + text[end:]
	}
	return text
}

func parseProofParams(s string) (title, proof string) {
	var depth int
	start := 0
	for i := 0; i < len(s); i++ {
		if i < len(s)-1 {
			if s[i] == '{' && s[i+1] == '{' || s[i] == '[' && s[i+1] == '[' {
				depth++
				i++
				continue
			}
			if s[i] == '}' && s[i+1] == '}' || s[i] == ']' && s[i+1] == ']' {
				depth--
				i++
				continue
			}
		}
		if s[i] == '|' && depth == 0 {
			applyProofParam(s[start:i], &title, &proof)
			start = i + 1
		}
	}
	applyProofParam(s[start:], &title, &proof)
	return title, proof
}

func applyProofParam(param string, title, proof *string) {
	k, v, ok := strings.Cut(param, "=")
	if !ok {
		return
	}
	switch strings.TrimSpace(k) {
	case "title":
		*title = strings.TrimSpace(v)
	case "proof":
		*proof = strings.TrimSpace(v)
	}
}

var (
	reMath    = regexp.MustCompile(`(?s)<math[^>]*>(.*?)</math>`)
	reTmpl    = regexp.MustCompile(`\{\{(?:[^{}]|\{[^{}]*\})*\}\}`)
	reFile    = regexp.MustCompile(`(?s)\[\[(?:File|Image|Category):(?:[^\[\]]|\[\[[^\]]*\]\])*\]\]`)
	reTable   = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
	reHeading = regexp.MustCompile(`(?m)^(={2,6})\s*(.+?)\s*={2,6}\s*$`)
	reBold    = regexp.MustCompile(`'''(.+?)'''`)
	reItalic  = regexp.MustCompile(`''(.+?)''`)
	reLink    = regexp.MustCompile(`\[\[(?:[^|\]]*\|)?([^\]]+)\]\]`)
	reExtLink = regexp.MustCompile(`\[https?://[^\s\]]+\s*([^\]]*)\]`)
	reMultiNL = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	var skip int
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if wikiSkipElements[tag] {
				skip++
			}
			if skip == 0 {
				switch tag {
				case "br", "p", "div", "li", "tr", "dd", "dt":
					b.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if wikiSkipElements[string(name)] && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
}

var wikiSkipElements = map[string]bool{
	"ref": true, "gallery": true, "nowiki": true,
	"score": true, "source": true, "syntaxhighlight": true,
}

func (w *Wiki) Links(title string) (WikiLinksResult, error) {
	page, err := w.fetchPage(title)
	if err != nil {
		return WikiLinksResult{}, err
	}
	page, err = w.followRedirect(page)
	if err != nil {
		return WikiLinksResult{}, err
	}
	if len(page.Revisions) == 0 {
		return WikiLinksResult{}, fmt.Errorf("no revisions for %q", title)
	}
	n := len(page.Revisions) - 1
	links := wikiparse.FindLinks(page.Revisions[n].Text)
	return WikiLinksResult{Title: page.Title, Links: links}, nil
}

type WikiLinksResult struct {
	Title string   `json:"title"`
	Links []string `json:"links"`
}

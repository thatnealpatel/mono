package main

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

type elementInfo struct {
	Element        string   `json:"element"`
	Secno          string   `json:"secno"`
	ContentModel   string   `json:"content_model"`
	TokenizerState string   `json:"tokenizer_state"`
	ContentAttrs   []string `json:"content_attributes"`
}

func whatwgElement(name string) error {
	doc, err := specDoc()
	if err != nil {
		return err
	}
	info, err := extractElementInfo(doc, name)
	if err != nil {
		return err
	}
	return printJSON(info)
}

func extractElementInfo(doc *html.Node, name string) (elementInfo, error) {
	h, dl := findElementDL(doc, name)
	if dl == nil {
		return elementInfo{}, fmt.Errorf("element %q not found", name)
	}
	info := elementInfo{
		Element:      name,
		Secno:        h.Secno,
		ContentAttrs: []string{},
	}
	parseElementDL(dl, &info)
	info.TokenizerState = resolveTokenizerState(doc, name)
	return info, nil
}

func findElementDL(doc *html.Node, name string) (heading, *html.Node) {
	for n := range doc.Descendants() {
		if !isElement(n, "dfn") || attr(n, "data-dfn-type") != "element" {
			continue
		}
		for c := range n.ChildNodes() {
			if !isElement(c, "code") || textOf(c) != name {
				continue
			}
			parent := n.Parent
			if _, ok := headingLevel(parent); !ok {
				continue
			}
			hd := extractHeading(parent)
			for sib := parent.NextSibling; sib != nil; sib = sib.NextSibling {
				if isElement(sib, "dl") && slices.Contains(strings.Fields(attr(sib, "class")), "element") {
					return hd, sib
				}
				if _, ok := headingLevel(sib); ok {
					break
				}
			}
		}
	}
	return heading{}, nil
}

func parseElementDL(dl *html.Node, info *elementInfo) {
	var label string
	for c := range dl.ChildNodes() {
		if isElement(c, "dt") {
			label = strings.ToLower(collapseSpace(textOf(c)))
			continue
		}
		if !isElement(c, "dd") {
			continue
		}
		switch {
		case strings.Contains(label, "content model"):
			text := collapseSpace(textOf(c))
			if info.ContentModel != "" {
				info.ContentModel += "; "
			}
			info.ContentModel += text
		case strings.Contains(label, "content attributes"):
			for code := range c.ChildNodes() {
				if isElement(code, "code") {
					info.ContentAttrs = append(info.ContentAttrs, collapseSpace(textOf(code)))
					break
				}
			}
		}
	}
}

func resolveTokenizerState(doc *html.Node, name string) string {
	for _, e := range elementNamesAtAnchor(doc, "raw-text-elements") {
		if e == name {
			if name == "script" {
				return "script-data-state"
			}
			return "rawtext-state"
		}
	}
	if slices.Contains(elementNamesAtAnchor(doc, "escapable-raw-text-elements"), name) {
		return "rcdata-state"
	}
	return "data-state"
}

func elementNamesAtAnchor(doc *html.Node, id string) []string {
	dfn := findAnchor(doc, id)
	if dfn == nil {
		return nil
	}
	dt := dfn.Parent
	if dt == nil || !isElement(dt, "dt") {
		return nil
	}
	var names []string
	for sib := dt.NextSibling; sib != nil; sib = sib.NextSibling {
		if isElement(sib, "dt") {
			break
		}
		if !isElement(sib, "dd") {
			continue
		}
		for code := range sib.Descendants() {
			if isElement(code, "code") {
				names = append(names, textOf(code))
			}
		}
	}
	return names
}

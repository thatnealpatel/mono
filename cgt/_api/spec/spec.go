// Package spec parses cgt/_api/go.yaml — the crystallized unified plan
// for the Go side of the mmgroup translation — into a queryable model.
//
// go.yaml is emitted by the _api generator and is a small, regular YAML
// subset: a top-level sequence whose elements are either a package block
// ({package, funcs}) or the trailing constants block ({constants}). The
// parser here is line-oriented and independent of the python.yaml /
// cython.yaml parsers in _api/main.go; it understands exactly the keys the
// go.yaml emitter produces (see the header comment in go.yaml) and fails
// closed on anything it does not recognize at a structural position.
package spec

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Spec is the parsed contents of go.yaml: the ordered package blocks plus
// the trailing constants block (if present).
type Spec struct {
	Packages  []Package
	Constants []Constant
}

// Package is one "- package: <name>" block with its function/method entries.
type Package struct {
	Name  string
	Funcs []Func
}

// Func is a single entry under a package's funcs:. A method is distinguished
// by a non-empty Recv. Params, Calls, and Dispatch may be empty. Return,
// Py, and C may be empty strings (a blank Return marks a deferred return
// type; a missing Py marks an entry with no Python provenance).
type Func struct {
	Pkg      string // owning package name, denormalized for convenience
	Name     string // exported Go name
	C        string // originating C symbol (may be empty)
	Py       string // Python entry point: "Class.method" or "module_func"
	Recv     string // receiver type for instance methods (empty for funcs)
	Return   string // Go return type ("" means deferred / blank)
	Note     string // free-form annotation
	Params   []Param
	Calls    []string   // non-authoritative C hints
	Dispatch []Dispatch // value/type dispatch ladder
}

// Param is one positional parameter. Type may be empty (deferred). Default
// carries the Python default literal verbatim when present.
type Param struct {
	Name    string
	Type    string
	Default string
}

// Dispatch is one rung of a dispatch: ladder ("on: <case>, returns: <type>").
type Dispatch struct {
	On      string
	Returns string
}

// Constant is one entry in the trailing constants block.
type Constant struct {
	Name  string
	Value string
	C     string
}

// Load reads and parses the go.yaml file at path.
func Load(path string) (*Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(raw))
}

// Parse parses the textual contents of a go.yaml manifest.
func Parse(text string) (*Spec, error) {
	p := &parser{lines: splitLines(text)}
	return p.parse()
}

// indentOf returns the count of leading spaces; comment-only and blank lines
// are reported with indent -1 so callers can skip them.
func indentOf(line string) int {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return -1
	}
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

func splitLines(text string) []string {
	// Strip a trailing newline's empty final element but keep interior blanks.
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type parser struct {
	lines []string
	i     int
}

func (p *parser) parse() (*Spec, error) {
	s := &Spec{}
	for p.i < len(p.lines) {
		line := p.lines[p.i]
		ind := indentOf(line)
		if ind < 0 {
			p.i++
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- package:"):
			pkg, err := p.parsePackage()
			if err != nil {
				return nil, err
			}
			s.Packages = append(s.Packages, pkg)
		case strings.HasPrefix(trimmed, "- constants:"):
			consts, err := p.parseConstants()
			if err != nil {
				return nil, err
			}
			s.Constants = consts
		default:
			return nil, fmt.Errorf("line %d: unexpected top-level line %q", p.i+1, trimmed)
		}
	}
	return s, nil
}

func (p *parser) parsePackage() (Package, error) {
	line := strings.TrimSpace(p.lines[p.i])
	name := strings.TrimSpace(strings.TrimPrefix(line, "- package:"))
	p.i++ // consume "- package:"
	pkg := Package{Name: name}
	// Expect the "funcs:" key at 2-space indent next (skipping blanks).
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind == 0 {
			return pkg, nil // next top-level block
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		if trimmed == "funcs:" {
			p.i++
			funcs, err := p.parseFuncs(name)
			if err != nil {
				return pkg, err
			}
			pkg.Funcs = funcs
			continue
		}
		return pkg, fmt.Errorf("line %d: unexpected key in package %q: %q", p.i+1, name, trimmed)
	}
	return pkg, nil
}

// parseFuncs reads the sequence of "- name:" entries under funcs:. Entries
// are at 4-space indent; their fields at 6-space.
func (p *parser) parseFuncs(pkg string) ([]Func, error) {
	var funcs []Func
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind < 4 {
			return funcs, nil // dedent out of funcs list
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		if ind == 4 && strings.HasPrefix(trimmed, "- name:") {
			fn, err := p.parseFunc(pkg)
			if err != nil {
				return funcs, err
			}
			funcs = append(funcs, fn)
			continue
		}
		return funcs, fmt.Errorf("line %d: unexpected line in funcs: %q", p.i+1, trimmed)
	}
	return funcs, nil
}

func (p *parser) parseFunc(pkg string) (Func, error) {
	fn := Func{Pkg: pkg}
	first := strings.TrimSpace(p.lines[p.i])
	fn.Name = unquote(strings.TrimSpace(strings.TrimPrefix(first, "- name:")))
	p.i++ // consume "- name:"
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind < 6 {
			return fn, nil // dedent: end of this func's fields
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		key, val := splitKey(trimmed)
		switch key {
		case "c":
			fn.C = unquote(val)
			p.i++
		case "py":
			fn.Py = unquote(val)
			p.i++
		case "recv":
			fn.Recv = unquote(val)
			p.i++
		case "return":
			fn.Return = unquote(val)
			p.i++
		case "note":
			fn.Note = unquote(val)
			p.i++
		case "calls":
			fn.Calls = parseFlowSeq(val)
			p.i++
		case "params":
			p.i++
			params, err := p.parseParams()
			if err != nil {
				return fn, err
			}
			fn.Params = params
		case "dispatch":
			p.i++
			disp, err := p.parseDispatch()
			if err != nil {
				return fn, err
			}
			fn.Dispatch = disp
		default:
			return fn, fmt.Errorf("line %d: unknown func field %q in %s.%s", p.i+1, key, pkg, fn.Name)
		}
	}
	return fn, nil
}

// parseParams reads the params: block. An inline "params: []" is handled by
// the caller (val == "[]"); here we only reach block form, where each entry
// is "- name:" at 8-space indent with type/default fields at 10-space.
func (p *parser) parseParams() ([]Param, error) {
	// Inline empty list: the previous line was "params: []" — handled by
	// detecting that parseFunc already advanced. If the next meaningful
	// line is not an 8-space "- name:", we are an empty params list.
	var params []Param
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind < 8 {
			return params, nil
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		if ind == 8 && strings.HasPrefix(trimmed, "- name:") {
			param := Param{Name: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")))}
			p.i++
			for p.i < len(p.lines) {
				ind2 := indentOf(p.lines[p.i])
				if ind2 < 0 {
					p.i++
					continue
				}
				if ind2 < 10 {
					break
				}
				k, v := splitKey(strings.TrimSpace(p.lines[p.i]))
				switch k {
				case "type":
					param.Type = unquote(v)
				case "default":
					param.Default = unquote(v)
				default:
					return params, fmt.Errorf("line %d: unknown param field %q", p.i+1, k)
				}
				p.i++
			}
			params = append(params, param)
			continue
		}
		return params, fmt.Errorf("line %d: unexpected line in params: %q", p.i+1, trimmed)
	}
	return params, nil
}

func (p *parser) parseDispatch() ([]Dispatch, error) {
	var disp []Dispatch
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind < 8 {
			return disp, nil
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		if ind == 8 && strings.HasPrefix(trimmed, "- on:") {
			d := Dispatch{On: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- on:")))}
			p.i++
			for p.i < len(p.lines) {
				ind2 := indentOf(p.lines[p.i])
				if ind2 < 0 {
					p.i++
					continue
				}
				if ind2 < 10 {
					break
				}
				k, v := splitKey(strings.TrimSpace(p.lines[p.i]))
				if k == "returns" {
					d.Returns = unquote(v)
				} else {
					return disp, fmt.Errorf("line %d: unknown dispatch field %q", p.i+1, k)
				}
				p.i++
			}
			disp = append(disp, d)
			continue
		}
		return disp, fmt.Errorf("line %d: unexpected line in dispatch: %q", p.i+1, trimmed)
	}
	return disp, nil
}

func (p *parser) parseConstants() ([]Constant, error) {
	// p.i points at "- constants:". The entries are at 4-space indent.
	p.i++
	var consts []Constant
	for p.i < len(p.lines) {
		ind := indentOf(p.lines[p.i])
		if ind < 0 {
			p.i++
			continue
		}
		if ind < 4 {
			return consts, nil
		}
		trimmed := strings.TrimSpace(p.lines[p.i])
		if ind == 4 && strings.HasPrefix(trimmed, "- name:") {
			c := Constant{Name: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")))}
			p.i++
			for p.i < len(p.lines) {
				ind2 := indentOf(p.lines[p.i])
				if ind2 < 0 {
					p.i++
					continue
				}
				if ind2 < 6 {
					break
				}
				k, v := splitKey(strings.TrimSpace(p.lines[p.i]))
				switch k {
				case "value":
					c.Value = unquote(v)
				case "c":
					c.C = unquote(v)
				default:
					return consts, fmt.Errorf("line %d: unknown constant field %q", p.i+1, k)
				}
				p.i++
			}
			consts = append(consts, c)
			continue
		}
		return consts, fmt.Errorf("line %d: unexpected line in constants: %q", p.i+1, trimmed)
	}
	return consts, nil
}

// splitKey splits "key: value" into ("key", "value"). A line with no value
// (e.g. "params:") returns an empty value. The split is on the first colon.
func splitKey(line string) (key, val string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
}

// unquote removes a single pair of surrounding double quotes, if present,
// and leaves bare scalars untouched.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// parseFlowSeq parses a YAML flow sequence "[a, b, c]" into its elements.
// An empty or "[]" value yields nil.
func parseFlowSeq(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, unquote(t))
		}
	}
	return out
}

// AllFuncs flattens every package's funcs into a single slice, preserving
// package and within-package order.
func (s *Spec) AllFuncs() []Func {
	var out []Func
	for _, pkg := range s.Packages {
		out = append(out, pkg.Funcs...)
	}
	return out
}

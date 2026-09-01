// Package main implements a program that
// codifies my idiosyncratic comment prose
// style that most people surely dislike.
//
// I like writing comments that minimize
// the line length difference in a given
// comment block while commonly choosing
// the line length based on the location
// of the `{` on the following line. An
// exception is made when a single-line
// comment can fit in 60 columns or fewer.
//
// All non-terminal comment lines must be
// at least 40 columns wide. The algorithm
// tries a few local orientations and
// picks the global block-optimal.
//
// `gocmt` is a rather unserious program
// that I unironically use.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	minCols = 40
	maxCols = 80
	jitter  = 2 // candidate widths straddle the anchor target by this much

	// A paragraph that fits on one line no
	// wider than this stays on one line,
	// even when the target is narrower.
	oneLineCols = 60
)

var (
	list   = flag.Bool("l", false, "list files whose formatting differs")
	write  = flag.Bool("w", false, "write result to (source) file instead of stdout")
	doDiff = flag.Bool("d", false, "display diffs instead of rewriting files")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("gocmt: ")
	flag.Parse()

	if flag.NArg() == 0 {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		out, err := process("<stdin>", src)
		if err != nil {
			log.Fatal(err)
		}
		os.Stdout.Write(out)
		return
	}

	status := 0
	for _, arg := range flag.Args() {
		if err := visit(arg); err != nil {
			log.Print(err)
			status = 2
		}
	}
	os.Exit(status)
}

func visit(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return handle(path)
	}
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != path && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".") {
			return handle(p)
		}
		return nil
	})
}

func handle(name string) error {
	src, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	out, err := process(name, src)
	if err != nil {
		return err
	}
	changed := !bytes.Equal(src, out)
	if *list && changed {
		fmt.Println(name)
	}
	if *doDiff && changed {
		diff(name, src, out)
	}
	if *write && changed {
		if err := os.WriteFile(name, out, 0o644); err != nil {
			return err
		}
	}
	if !*list && !*write && !*doDiff {
		os.Stdout.Write(out)
	}
	return nil
}

// diff shows old versus new for -d. It
// shells out to diff(1), so an internal
// diff would be needed to run where that
// command is missing.
func diff(name string, old, new []byte) {
	dir, err := os.MkdirTemp("", "gocmt")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	a := filepath.Join(dir, "old")
	b := filepath.Join(dir, "new")
	os.WriteFile(a, old, 0o600)
	os.WriteFile(b, new, 0o600)
	cmd := exec.Command("diff", "-u", "--label", name+".orig", "--label", name, a, b)
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// process returns src with every eligible comment group
// rewrapped; bytes outside comments are never modified.
func process(name string, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if ast.IsGenerated(f) {
		return src, nil
	}
	tf := fset.File(f.Package)
	var out []byte
	last := 0
	for _, g := range f.Comments {
		if g == f.Doc {
			continue // package doc comments are left as written
		}
		start, end, text, ok := rewrap(tf, src, g)
		if !ok {
			continue
		}
		out = append(out, src[last:start]...)
		out = append(out, text...)
		last = end
	}
	out = append(out, src[last:]...)
	return out, nil
}

// rewrap formats one comment group. It reports ok=false when the group is not
// eligible (block comment, trailing comment, mixed indentation, no prose) or
// already formatted. On success the replacement text spans src[start:end],
// from the first line's indent to the last comment's final byte.
func rewrap(tf *token.File, src []byte, g *ast.CommentGroup) (start, end int, text []byte, ok bool) {
	var indent string
	for i, c := range g.List {
		if !strings.HasPrefix(c.Text, "//") {
			return 0, 0, nil, false
		}
		off := tf.Offset(c.Pos())
		ls := tf.Offset(tf.LineStart(tf.Line(c.Pos())))
		ind := string(src[ls:off])
		if strings.Trim(ind, " \t") != "" {
			return 0, 0, nil, false // trailing comment: code precedes it
		}
		if i == 0 {
			indent, start = ind, ls
		} else if ind != indent {
			return 0, 0, nil, false
		}
	}
	end = tf.Offset(g.End())

	target := min(max(anchorWidth(src, end), minCols), maxCols)
	lo := max(target-jitter, minCols)
	hi := min(target+jitter, maxCols)
	pad := utf8.RuneCountInString(indent) + len("// ")

	// Split the group into prose paragraphs
	// and verbatim lines, preserving order.
	type chunk struct {
		verbatim string
		words    []string
	}
	var chunks []chunk
	var words []string
	prose := false
	cut := func() {
		if len(words) > 0 {
			chunks = append(chunks, chunk{words: words})
			words = nil
		}
	}
	for _, c := range g.List {
		content := c.Text[len("//"):]
		switch classify(content) {
		case blankLine:
			cut()
			chunks = append(chunks, chunk{verbatim: "//"})
		case verbatimLine:
			cut()
			chunks = append(chunks, chunk{verbatim: strings.TrimRight(c.Text, " \t")})
		default:
			prose = true
			words = append(words, strings.Fields(content)...)
		}
	}
	cut()
	if !prose {
		return 0, 0, nil, false
	}

	// Every paragraph in the block wraps
	// at the same candidate width so their
	// right edges agree, and the block
	// score is the summed wrap cost. Ties
	// go to the width nearest the anchor
	// target, then to the narrower one.
	var best []string
	bestCost, bestDist := 1<<60, 1<<60
	for w := lo; w <= hi; w++ {
		avail := max(w-pad, 1)
		var lines []string
		cost := 0
		for _, ch := range chunks {
			if ch.words == nil {
				lines = append(lines, ch.verbatim)
				continue
			}
			if joined := strings.Join(ch.words, " "); utf8.RuneCountInString(joined)+pad <= oneLineCols {
				lines = append(lines, "// "+joined)
				continue
			}
			ls, c := wrapWords(ch.words, avail)
			cost += c
			// Naive widow rule; tune the divisor if taste disagrees.
			if last := utf8.RuneCountInString(ls[len(ls)-1]); last < avail/3 {
				d := avail/3 - last
				cost += d * d
			}
			for _, l := range ls {
				lines = append(lines, "// "+l)
			}
		}
		dist := w - target
		if dist < 0 {
			dist = -dist
		}
		if cost < bestCost || (cost == bestCost && dist < bestDist) {
			best, bestCost, bestDist = lines, cost, dist
		}
	}

	text = []byte(indent + strings.Join(best, "\n"+indent))
	if bytes.Equal(text, src[start:end]) {
		return 0, 0, nil, false
	}
	return start, end, text, true
}

// anchorWidth returns the display width of
// the first non-blank line after byte offset
// from. A tab counts as one column, matching
// the corpus habit of a fixed text width
// regardless of indentation.
func anchorWidth(src []byte, from int) int {
	for _, line := range strings.Split(string(src[from:]), "\n")[1:] {
		line = strings.TrimRight(line, " \t")
		if line != "" {
			return utf8.RuneCountInString(line)
		}
	}
	return maxCols
}

const (
	proseLine = iota
	blankLine
	verbatimLine
)

// classify buckets one comment line's
// content (text after "//"): prose is
// rewrapped, blanks separate paragraphs,
// and everything godoc treats as
// structure (directives, indented code,
// lists, headings) passes verbatim.
func classify(content string) int {
	if strings.TrimRight(content, " \t") == "" {
		return blankLine
	}
	if isDirective(content) {
		return verbatimLine
	}
	rest := strings.TrimPrefix(content, " ")
	if strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
		return verbatimLine // indented: code block or nested list
	}
	if listOrHeading(rest) {
		return verbatimLine
	}
	return proseLine
}

// isDirective mirrors go/ast's unexported rule.
func isDirective(c string) bool {
	if strings.HasPrefix(c, "line ") {
		return true
	}
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return false
	}
	for i := range colon {
		b := c[i]
		if ('a' > b || b > 'z') && ('0' > b || b > '9') {
			return false
		}
	}
	return true
}

func listOrHeading(s string) bool {
	if strings.HasPrefix(s, "# ") {
		return true
	}
	if len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return true
	}
	i := 0
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	return i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')')
}

// wrapWords breaks words into lines of at most width
// columns, minimizing the summed squared deviation from
// width so the lines form an even block (Knuth-style
// minimum raggedness), and returns the lines with that
// cost. The last line pays no penalty for running short. A
// single word longer than the width gets a line to itself.
func wrapWords(words []string, width int) ([]string, int) {
	n := len(words)
	if n == 0 {
		return nil, 0
	}
	wl := make([]int, n)
	for i, w := range words {
		wl[i] = utf8.RuneCountInString(w)
	}
	const inf = 1 << 50
	best := make([]int, n+1)
	brk := make([]int, n+1)
	for i := 1; i <= n; i++ {
		best[i] = inf
		length := -1
		for j := i; j >= 1; j-- {
			length += 1 + wl[j-1]
			if length > width && j < i {
				break
			}
			d := width - length
			cost := d * d
			if i == n && d >= 0 {
				cost = 0
			}
			if best[j-1] != inf && best[j-1]+cost < best[i] {
				best[i], brk[i] = best[j-1]+cost, j-1
			}
		}
	}
	var lines []string
	for i := n; i > 0; i = brk[i] {
		lines = append(lines, strings.Join(words[brk[i]:i], " "))
	}
	slices.Reverse(lines)
	return lines, best[n]
}

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

var httpClient = http.DefaultClient
var arxivBaseURL = "https://arxiv.org/e-print/"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: fetch <url> -o <dir> (helper for .pdf and arXiv)

Fetch and postprocess a reference document.

Supported Cases:
  arXiv   Download .tex source from arXiv (accepts URLs or bare IDs)
  PDF     Download and convert to text via pdftotext
`

func run(args []string) error {
	if len(args) == 0 || slices.Contains(args, "-h") {
		fmt.Print(usage)
		return nil
	}

	url, outdir, err := parseArgs(args)
	if err != nil {
		return err
	}

	if id, ok := parseArXivID(url); ok {
		dir, status, err := fetchArXiv(id, outdir)
		if err != nil {
			return err
		}
		return printResult(url, dir, status)
	}
	if strings.HasSuffix(strings.ToLower(url), ".pdf") {
		dir, status, err := fetchPDF(url, outdir)
		if err != nil {
			return err
		}
		return printResult(url, dir, status)
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/pdf") {
		dir := filepath.Join(outdir, pdfDir(url))
		if hasFiles(dir) {
			return printResult(url, dir, "cached")
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if err := pdfToText(body, dir); err != nil {
			return err
		}
		return printResult(url, dir, "fetched")
	}
	return fmt.Errorf("unknown URL format: %s", url)
}

func parseArgs(args []string) (url, outdir string, err error) {
	if len(args) != 3 || args[1] != "-o" {
		return "", "", fmt.Errorf("usage: fetch <url> -o <dir>")
	}
	return args[0], args[2], nil
}

type result struct {
	URL    string   `json:"url"`
	Dir    string   `json:"dir"`
	Status string   `json:"status"`
	Files  []string `json:"files,omitempty"`
}

func printResult(url, dir, status string) error {
	r := result{URL: url, Dir: dir, Status: status}
	if status == "fetched" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() {
				r.Files = append(r.Files, e.Name())
			}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}

func parseArXivID(s string) (string, bool) {
	// Strip scheme and host.
	id := s
	for _, prefix := range []string{
		"https://arxiv.org/abs/",
		"https://arxiv.org/pdf/",
		"https://arxiv.org/e-print/",
		"http://arxiv.org/abs/",
		"http://arxiv.org/pdf/",
		"http://arxiv.org/e-print/",
	} {
		if after, ok := strings.CutPrefix(id, prefix); ok {
			id = normalizeArXivID(after)
			if id == "" {
				return "", false
			}
			return id, true
		}
	}
	if after, ok := strings.CutPrefix(id, "arXiv:"); ok {
		id = after
	}

	// New-style: YYMM.NNNNN
	if len(id) >= 9 && id[4] == '.' && isDigits(id[:4]) && isDigits(id[5:]) {
		return id, true
	}
	// Old-style: subject/NNNNNNN (alpha prefix, 7 digits)
	if i := strings.IndexByte(id, '/'); i > 0 && i < len(id)-1 && isAlpha(id[:i]) && len(id[i+1:]) == 7 && isDigits(id[i+1:]) {
		return id, true
	}
	return "", false
}

func isDigits(s string) bool {
	return len(s) > 0 && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
}

func isAlpha(s string) bool {
	return len(s) > 0 && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '-' })
}

func normalizeArXivID(id string) string {
	id = strings.TrimSuffix(id, ".pdf")
	// Strip version suffix (e.g. v2, v13)
	if i := strings.LastIndex(id, "v"); i > 0 && isDigits(id[i+1:]) {
		id = id[:i]
	}
	return id
}

func arxivDir(id string) string {
	return "arXiv-" + strings.NewReplacer(".", "-", "/", "-").Replace(id)
}

func fetchArXiv(id, outdir string) (dir, status string, err error) {
	outdir = filepath.Join(outdir, arxivDir(id))
	if hasFiles(outdir) {
		return outdir, "cached", nil
	}

	resp, err := httpClient.Get(arxivBaseURL + id)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("arXiv: HTTP %d for %s", resp.StatusCode, id)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		if err := handleGzip(body, outdir); err != nil {
			return "", "", err
		}
		return outdir, "fetched", nil
	}
	if bytes.HasPrefix(body, []byte("%PDF")) {
		if err := pdfToText(body, outdir); err != nil {
			return "", "", err
		}
		return outdir, "fetched", nil
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(outdir, "paper.tex"), body, 0o644); err != nil {
		return "", "", err
	}
	return outdir, "fetched", nil
}

func handleGzip(body []byte, outdir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	hdr, terr := tr.Next()
	if terr != nil {
		// Not a tar — gzipped single file.
		gr2, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer gr2.Close()
		if err := os.MkdirAll(outdir, 0o755); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(outdir, "paper.tex"))
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, gr2)
		return err
	}

	// tar.gz — extract .tex files.
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return err
	}
	for ; err == nil; hdr, err = tr.Next() {
		if !strings.HasSuffix(hdr.Name, ".tex") {
			continue
		}
		dst := filepath.Join(outdir, hdr.Name)
		if rel, err := filepath.Rel(outdir, dst); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("tar entry escapes output directory: %s", hdr.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	if err != io.EOF {
		return err
	}
	// Ensure idempotency even when no .tex files were extracted.
	entries, _ := os.ReadDir(outdir)
	if len(entries) == 0 {
		return os.WriteFile(filepath.Join(outdir, ".fetched"), nil, 0o644)
	}
	return nil
}

func pdfDir(rawURL string) string {
	s := rawURL
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".pdf"), ".PDF")
	if s == "" {
		s = "paper"
	}
	return s
}

func fetchPDF(url, outdir string) (dir, status string, err error) {
	outdir = filepath.Join(outdir, pdfDir(url))
	if hasFiles(outdir) {
		return outdir, "cached", nil
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("PDF: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if err := pdfToText(body, outdir); err != nil {
		return "", "", err
	}
	return outdir, "fetched", nil
}

func pdfToText(body []byte, outdir string) error {
	tmp, err := os.CreateTemp("", "fetch-*.pdf")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(outdir, "paper.txt")
	cmd := exec.Command("pdftotext", tmp.Name(), out)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	txt, err := os.ReadFile(out)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(bytes.ReplaceAll(txt, []byte{'\f'}, nil))) == 0 {
		fmt.Fprintf(os.Stderr, "fetch: warning: PDF has no text layer (scanned), copying raw PDF\n")
		os.Remove(out)
		return os.WriteFile(filepath.Join(outdir, "paper.pdf"), body, 0o644)
	}
	return nil
}

func hasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

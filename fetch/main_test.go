package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	httpClient = srv.Client()
	arxivBaseURL = srv.URL + "/e-print/"
	return t.TempDir()
}

func TestParseArgs_RequiresO(t *testing.T) {
	if _, _, err := parseArgs([]string{"http://x"}); err == nil {
		t.Fatal("expected error for missing -o")
	}
	if _, _, err := parseArgs([]string{"http://x", "-o"}); err == nil {
		t.Fatal("expected error for -o without dir")
	}
	url, dir, err := parseArgs([]string{"http://x", "-o", "/tmp/out"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://x" || dir != "/tmp/out" {
		t.Fatalf("got url=%q dir=%q", url, dir)
	}
}

func TestArxivDir(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"2402.01011", "arXiv-2402-01011"},
		{"hep-th/9905111", "arXiv-hep-th-9905111"},
	}
	for _, tt := range tests {
		if got := arxivDir(tt.id); got != tt.want {
			t.Errorf("arxivDir(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestIsPDFURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/foo/thesis.pdf", true},
		{"https://example.com/foo/thesis.PDF", true},
		{"https://dl.acm.org/doi/pdf/10.1145/3618260.3649656", true},
		{"https://dl.acm.org/doi/epdf/10.1145/3618260.3649656", true},
		{"https://example.com/foo/doc", false},
		{"https://arxiv.org/abs/2402.01011", false},
	}
	for _, tt := range tests {
		if got := isPDFURL(tt.url); got != tt.want {
			t.Errorf("isPDFURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestPdfDir(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/foo/thesis.pdf", "thesis"},
		{"https://example.com/foo/thesis.PDF", "thesis"},
		{"https://example.com/foo/thesis.pdf?v=2", "thesis"},
		{"https://example.com/foo/thesis.pdf#page=3", "thesis"},
		{"https://example.com/foo/.pdf", "paper"},
		{"https://example.com/foo/doc", "doc"},
		{"https://dl.acm.org/doi/pdf/10.1145/3618260.3649656", "doi-10-1145-3618260-3649656"},
		{"https://dl.acm.org/doi/epdf/10.1145/3618260.3649656", "doi-10-1145-3618260-3649656"},
		{"https://dl.acm.org/doi/pdf/10.1145/3618260.3649656?download=true", "doi-10-1145-3618260-3649656"},
	}
	for _, tt := range tests {
		if got := pdfDir(tt.url); got != tt.want {
			t.Errorf("pdfDir(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeArXivID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"2402.01011", "2402.01011"},
		{"2402.01011v2", "2402.01011"},
		{"2402.01011v13", "2402.01011"},
		{"2402.01011.pdf", "2402.01011"},
		{"2402.01011v2.pdf", "2402.01011"},
		{"solv-int/9905111v2", "solv-int/9905111"},
		{"solv-int/9905111", "solv-int/9905111"},
	}
	for _, tt := range tests {
		if got := normalizeArXivID(tt.in); got != tt.want {
			t.Errorf("normalizeArXivID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseArXivID(t *testing.T) {
	tests := []struct {
		in   string
		id   string
		ok   bool
	}{
		{"2402.01011", "2402.01011", true},
		{"arXiv:2402.01011", "2402.01011", true},
		{"https://arxiv.org/abs/2402.01011", "2402.01011", true},
		{"https://arxiv.org/abs/2402.01011v3", "2402.01011", true},
		{"https://arxiv.org/pdf/2402.01011.pdf", "2402.01011", true},
		{"hep-th/9905111", "hep-th/9905111", true},
		{"https://example.com/paper.pdf", "", false},
		{"not-an-id", "", false},
	}
	for _, tt := range tests {
		id, ok := parseArXivID(tt.in)
		if ok != tt.ok || id != tt.id {
			t.Errorf("parseArXivID(%q) = (%q, %v), want (%q, %v)", tt.in, id, ok, tt.id, tt.ok)
		}
	}
}

func makeGzipTex(content string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(content))
	gw.Close()
	return buf.Bytes()
}

func makeTarGz(files map[string]string) []byte {
	var tbuf bytes.Buffer
	tw := tar.NewWriter(&tbuf)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(content)),
		})
		tw.Write([]byte(content))
	}
	tw.Close()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(tbuf.Bytes())
	gw.Close()
	return buf.Bytes()
}

func TestFetchArXiv_PlainTex(t *testing.T) {
	body := []byte(`\documentclass{article}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	wantDir := filepath.Join(outdir, "arXiv-2402-01011")
	if dir != wantDir {
		t.Fatalf("dir = %q, want %q", dir, wantDir)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("content mismatch")
	}
}

func TestFetchArXiv_GzipSingleTex(t *testing.T) {
	content := `\documentclass{article}\begin{document}Hello\end{document}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(makeGzipTex(content))
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestFetchArXiv_TarGz(t *testing.T) {
	files := map[string]string{
		"main.tex":    `\input{appendix}`,
		"appendix.tex": `\section{Appendix}`,
		"figure.png":  "not-tex",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(makeTarGz(files))
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	for _, name := range []string{"main.tex", "appendix.tex"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if string(got) != files[name] {
			t.Fatalf("%s: got %q, want %q", name, got, files[name])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "figure.png")); err == nil {
		t.Fatal("non-.tex file figure.png should not be extracted")
	}
}

func TestFetchArXiv_Cached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit on cached fetch")
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	paperDir := filepath.Join(outdir, "arXiv-2402-01011")
	os.MkdirAll(paperDir, 0o755)
	os.WriteFile(filepath.Join(paperDir, "paper.tex"), []byte("cached"), 0o644)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "cached" {
		t.Fatalf("status = %q, want cached", status)
	}
	if dir != paperDir {
		t.Fatalf("dir = %q, want %q", dir, paperDir)
	}
}

// Regression: sibling paper in -o dir must not mask an unfetched paper.
func TestFetchArXiv_SiblingDoesNotMask(t *testing.T) {
	body := []byte(`\documentclass{article}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	// Pre-populate a sibling paper.
	siblingDir := filepath.Join(outdir, "arXiv-1406-5145")
	os.MkdirAll(siblingDir, 0o755)
	os.WriteFile(filepath.Join(siblingDir, "paper.tex"), []byte("sibling"), 0o644)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched (sibling should not mask)", status)
	}
	wantDir := filepath.Join(outdir, "arXiv-2402-01011")
	if dir != wantDir {
		t.Fatalf("dir = %q, want %q", dir, wantDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "paper.tex")); err != nil {
		t.Fatal("paper.tex not created")
	}
}

func loadTestPDF(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/minimal.pdf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestFetchPDF_EndToEnd(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	url := srv.URL + "/papers/thesis.pdf"
	dir, status, err := fetchPDF(url, outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	if filepath.Base(dir) != "thesis" {
		t.Fatalf("dir base = %q, want thesis", filepath.Base(dir))
	}
	txt, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(txt, []byte("Hello World")) {
		t.Fatalf("pdftotext output = %q, want it to contain 'Hello World'", txt)
	}
}

// Regression: two different PDFs to the same -o dir get distinct subdirs.
func TestFetchPDF_SubdirPerPaper(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir1, status1, err := fetchPDF(srv.URL+"/papers/thesis.pdf", outdir)
	if err != nil {
		t.Fatal(err)
	}
	dir2, status2, err := fetchPDF(srv.URL+"/papers/notes.pdf", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status1 != "fetched" || status2 != "fetched" {
		t.Fatalf("statuses = %q, %q; want fetched, fetched", status1, status2)
	}
	if dir1 == dir2 {
		t.Fatalf("two different PDFs resolved to same dir: %s", dir1)
	}
	if filepath.Base(dir1) != "thesis" || filepath.Base(dir2) != "notes" {
		t.Fatalf("dirs = %q, %q; want thesis, notes bases", dir1, dir2)
	}
}

// Regression: sibling PDF in -o dir must not mask a different PDF.
func TestFetchPDF_SiblingDoesNotMask(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	siblingDir := filepath.Join(outdir, "thesis")
	os.MkdirAll(siblingDir, 0o755)
	os.WriteFile(filepath.Join(siblingDir, "paper.txt"), []byte("sibling"), 0o644)

	dir, status, err := fetchPDF(srv.URL+"/other/notes.pdf", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched (sibling should not mask)", status)
	}
	if filepath.Base(dir) != "notes" {
		t.Fatalf("dir base = %q, want notes", filepath.Base(dir))
	}
	if _, err := os.Stat(filepath.Join(dir, "paper.txt")); err != nil {
		t.Fatal("paper.txt not created in notes subdir")
	}
}

func TestFetchPDF_DOI(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	url := srv.URL + "/doi/pdf/10.1145/3618260.3649656"
	dir, status, err := fetchPDF(url, outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	if filepath.Base(dir) != "doi-10-1145-3618260-3649656" {
		t.Fatalf("dir base = %q, want doi-10-1145-3618260-3649656", filepath.Base(dir))
	}
	txt, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(txt, []byte("Hello World")) {
		t.Fatalf("pdftotext output = %q, want it to contain 'Hello World'", txt)
	}
}

func TestFetchPDF_Cached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit on cached fetch")
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	paperDir := filepath.Join(outdir, "thesis")
	os.MkdirAll(paperDir, 0o755)
	os.WriteFile(filepath.Join(paperDir, "paper.txt"), []byte("cached"), 0o644)

	dir, status, err := fetchPDF(srv.URL+"/papers/thesis.pdf", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "cached" {
		t.Fatalf("status = %q, want cached", status)
	}
	if dir != paperDir {
		t.Fatalf("dir = %q, want %q", dir, paperDir)
	}
}

// arXiv sometimes returns PDF instead of TeX source.
func TestFetchArXiv_PDFResponse(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if status != "fetched" {
		t.Fatalf("status = %q, want fetched", status)
	}
	txt, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(txt, []byte("Hello World")) {
		t.Fatalf("pdftotext output = %q, want it to contain 'Hello World'", txt)
	}
}

func TestFetchArXiv_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	_, _, err := fetchArXiv("9999.99999", outdir)
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestHandleGzip_TarTraversalBlocked(t *testing.T) {
	files := map[string]string{
		"../../../etc/passwd.tex": "malicious",
	}
	dir := t.TempDir()
	err := handleGzip(makeTarGz(files), dir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestHandleGzip_NoTexFiles(t *testing.T) {
	files := map[string]string{
		"readme.md": "no tex here",
	}
	dir := t.TempDir()
	if err := handleGzip(makeTarGz(files), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fetched")); err != nil {
		t.Fatal("expected .fetched sentinel for tar with no .tex files")
	}
}

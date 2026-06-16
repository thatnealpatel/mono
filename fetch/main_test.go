package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	httpClient = srv.Client()
	arxivBaseURL = srv.URL + "/e-print/"
	unpaywallBaseURL = srv.URL + "/v2/"
	return t.TempDir()
}

func TestParseArgsRequiresO(t *testing.T) {
	if _, _, err := parseArgs([]string{"http://x"}); err == nil {
		t.Fatal("got nil error, want error for missing -o")
	}
	if _, _, err := parseArgs([]string{"http://x", "-o"}); err == nil {
		t.Fatal("got nil error, want error for -o without dir")
	}
	url, dir, err := parseArgs([]string{"http://x", "-o", "/tmp/out"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://x" || dir != "/tmp/out" {
		t.Fatalf("got url=%q dir=%q, want url=%q dir=%q", url, dir, "http://x", "/tmp/out")
	}
}

func TestArxivDir(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"2402.01011", "arXiv-2402-01011"},
		{"2402.01011v2", "arXiv-2402-01011-v2"},
		{"2402.01011v13", "arXiv-2402-01011-v13"},
		{"hep-th/9905111", "arXiv-hep-th-9905111"},
		{"hep-th/9905111v2", "arXiv-hep-th-9905111-v2"},
	}
	for _, tt := range tests {
		if got := arxivDir(tt.id); got != tt.want {
			t.Errorf("arxivDir(%q): got %q, want %q", tt.id, got, tt.want)
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
			t.Errorf("isPDFURL(%q): got %v, want %v", tt.url, got, tt.want)
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
			t.Errorf("pdfDir(%q): got %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestStripVersion(t *testing.T) {
	tests := []struct {
		in        string
		bare, ver string
	}{
		{"2402.01011", "2402.01011", ""},
		{"2402.01011v2", "2402.01011", "v2"},
		{"2402.01011v13", "2402.01011", "v13"},
		{"hep-th/9905111", "hep-th/9905111", ""},
		{"hep-th/9905111v2", "hep-th/9905111", "v2"},
	}
	for _, tt := range tests {
		bare, ver := stripVersion(tt.in)
		if bare != tt.bare || ver != tt.ver {
			t.Errorf("stripVersion(%q): got (%q, %q), want (%q, %q)", tt.in, bare, ver, tt.bare, tt.ver)
		}
	}
}

func TestNormalizeArXivID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"2402.01011", "2402.01011"},
		{"2402.01011v2", "2402.01011v2"},
		{"2402.01011v13", "2402.01011v13"},
		{"2402.01011.pdf", "2402.01011"},
		{"2402.01011v2.pdf", "2402.01011v2"},
		{"solv-int/9905111v2", "solv-int/9905111v2"},
		{"solv-int/9905111", "solv-int/9905111"},
	}
	for _, tt := range tests {
		if got := normalizeArXivID(tt.in); got != tt.want {
			t.Errorf("normalizeArXivID(%q): got %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseArXivID(t *testing.T) {
	tests := []struct {
		in string
		id string
		ok bool
	}{
		{"2402.01011", "2402.01011", true},
		{"2402.01011v2", "2402.01011v2", true},
		{"arXiv:2402.01011", "2402.01011", true},
		{"arXiv:2402.01011v3", "2402.01011v3", true},
		{"https://arxiv.org/abs/2402.01011", "2402.01011", true},
		{"https://arxiv.org/abs/2402.01011v3", "2402.01011v3", true},
		{"https://arxiv.org/pdf/2402.01011.pdf", "2402.01011", true},
		{"https://arxiv.org/pdf/2402.01011v2.pdf", "2402.01011v2", true},
		{"hep-th/9905111", "hep-th/9905111", true},
		{"hep-th/9905111v2", "hep-th/9905111v2", true},
		{"cs/0703145v4", "cs/0703145v4", true},
		{"https://example.com/paper.pdf", "", false},
		{"not-an-id", "", false},
	}
	for _, tt := range tests {
		id, ok := parseArXivID(tt.in)
		if ok != tt.ok || id != tt.id {
			t.Errorf("parseArXivID(%q): got (%q, %v), want (%q, %v)", tt.in, id, ok, tt.id, tt.ok)
		}
	}
}

func TestParseDOIURL(t *testing.T) {
	tests := []struct {
		in  string
		doi string
		ok  bool
	}{
		{"https://doi.org/10.1515/gcc-2012-0006", "10.1515/gcc-2012-0006", true},
		{"http://doi.org/10.1515/gcc-2012-0006", "10.1515/gcc-2012-0006", true},
		{"https://dx.doi.org/10.1515/gcc-2012-0006", "10.1515/gcc-2012-0006", true},
		{"10.1515/gcc-2012-0006", "10.1515/gcc-2012-0006", true},
		{"https://doi.org/not-a-doi", "", false},
		{"https://example.com/paper.pdf", "", false},
	}
	for _, tt := range tests {
		doi, ok := parseDOIURL(tt.in)
		if ok != tt.ok || doi != tt.doi {
			t.Errorf("parseDOIURL(%q): got (%q, %v), want (%q, %v)", tt.in, doi, ok, tt.doi, tt.ok)
		}
	}
}

func TestDoiDir(t *testing.T) {
	tests := []struct {
		doi  string
		want string
	}{
		{"10.1515/gcc-2012-0006", "doi-10-1515-gcc-2012-0006"},
		{"10.1145/3618260.3649656", "doi-10-1145-3618260-3649656"},
	}
	for _, tt := range tests {
		if got := doiDir(tt.doi); got != tt.want {
			t.Errorf("doiDir(%q): got %q, want %q", tt.doi, got, tt.want)
		}
	}
}

func TestFetchDOI(t *testing.T) {
	pdfData := loadTestPDF(t)
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/") {
			json.NewEncoder(w).Encode(map[string]any{
				"best_oa_location": map[string]any{
					"url_for_pdf": srvURL + "/paper.pdf",
				},
			})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	srvURL = srv.URL
	outdir := setup(t, srv)

	dir, status, err := fetchDOI("10.1515/gcc-2012-0006", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir), "doi-10-1515-gcc-2012-0006"; got != want {
		t.Fatalf("got dir base %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Hello World")) {
		t.Fatalf("got pdftotext output %q, want it to contain \"Hello World\"", got)
	}
}

func TestFetchDOINoOA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"best_oa_location": nil,
		})
	}))
	defer srv.Close()
	setup(t, srv)

	_, _, err := fetchDOI("10.1515/gcc-2012-0006", t.TempDir())
	if err == nil {
		t.Fatal("got nil error, want error for no OA PDF")
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

func TestFetchArXivPlainTex(t *testing.T) {
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
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := dir, filepath.Join(outdir, "arXiv-2402-01011"); got != want {
		t.Fatalf("got dir %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("got content %q, want %q", got, body)
	}
}

func TestFetchArXivGzipSingleTex(t *testing.T) {
	content := `\documentclass{article}\begin{document}Hello\end{document}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(makeGzipTex(content))
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	_, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(outdir, "arXiv-2402-01011", "paper.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(got), content; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFetchArXivTarGz(t *testing.T) {
	files := map[string]string{
		"main.tex":     `\input{appendix}`,
		"appendix.tex": `\section{Appendix}`,
		"figure.png":   "not-tex",
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
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	for _, name := range []string{"main.tex", "appendix.tex"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: got error %v, want file to exist", name, err)
		}
		if got, want := string(got), files[name]; got != want {
			t.Fatalf("%s: got %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "figure.png")); err == nil {
		t.Fatal("got figure.png extracted, want non-.tex files excluded")
	}
}

func TestFetchArXivOverwrite(t *testing.T) {
	body := []byte(`\documentclass{new}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	paperDir := filepath.Join(outdir, "arXiv-2402-01011")
	os.MkdirAll(paperDir, 0o755)
	os.WriteFile(filepath.Join(paperDir, "paper.tex"), []byte("old"), 0o644)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "overwritten"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := dir, paperDir; got != want {
		t.Fatalf("got dir %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("got content %q, want %q", got, body)
	}
}

func TestFetchArXivSiblingDoesNotMask(t *testing.T) {
	body := []byte(`\documentclass{article}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	siblingDir := filepath.Join(outdir, "arXiv-1406-5145")
	os.MkdirAll(siblingDir, 0o755)
	os.WriteFile(filepath.Join(siblingDir, "paper.tex"), []byte("sibling"), 0o644)

	dir, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := dir, filepath.Join(outdir, "arXiv-2402-01011"); got != want {
		t.Fatalf("got dir %q, want %q", got, want)
	}
}

func TestFetchArXivVersionsCoexist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`\documentclass{` + r.URL.Path + `}`))
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	dir1, _, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	dir2, _, err := fetchArXiv("2402.01011v2", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if dir1 == dir2 {
		t.Fatalf("got same dir for unversioned and v2: %s", dir1)
	}
	if got, want := filepath.Base(dir1), "arXiv-2402-01011"; got != want {
		t.Fatalf("unversioned: got dir %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir2), "arXiv-2402-01011-v2"; got != want {
		t.Fatalf("v2: got dir %q, want %q", got, want)
	}
}

func TestFetchArXivPDFResponse(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	_, status, err := fetchArXiv("2402.01011", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(outdir, "arXiv-2402-01011", "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Hello World")) {
		t.Fatalf("got pdftotext output %q, want it to contain \"Hello World\"", got)
	}
}

func TestFetchArXivHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	_, _, err := fetchArXiv("9999.99999", outdir)
	if err == nil {
		t.Fatal("got nil error, want error on 404")
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

func TestFetchPDFEndToEnd(t *testing.T) {
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
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir), "thesis"; got != want {
		t.Fatalf("got dir base %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Hello World")) {
		t.Fatalf("got pdftotext output %q, want it to contain \"Hello World\"", got)
	}
}

func TestFetchPDFSubdirPerPaper(t *testing.T) {
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
	if got, want := status1, "fetched"; got != want {
		t.Fatalf("thesis: got status %q, want %q", got, want)
	}
	if got, want := status2, "fetched"; got != want {
		t.Fatalf("notes: got status %q, want %q", got, want)
	}
	if dir1 == dir2 {
		t.Fatalf("got same dir for two different PDFs: %s", dir1)
	}
	if got, want := filepath.Base(dir1), "thesis"; got != want {
		t.Fatalf("got dir1 base %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir2), "notes"; got != want {
		t.Fatalf("got dir2 base %q, want %q", got, want)
	}
}

func TestFetchPDFSiblingDoesNotMask(t *testing.T) {
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
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir), "notes"; got != want {
		t.Fatalf("got dir base %q, want %q", got, want)
	}
}

func TestFetchPDFDOI(t *testing.T) {
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
	if got, want := status, "fetched"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := filepath.Base(dir), "doi-10-1145-3618260-3649656"; got != want {
		t.Fatalf("got dir base %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Hello World")) {
		t.Fatalf("got pdftotext output %q, want it to contain \"Hello World\"", got)
	}
}

func TestFetchPDFOverwrite(t *testing.T) {
	pdfData := loadTestPDF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()
	outdir := setup(t, srv)

	paperDir := filepath.Join(outdir, "thesis")
	os.MkdirAll(paperDir, 0o755)
	os.WriteFile(filepath.Join(paperDir, "paper.txt"), []byte("old"), 0o644)

	dir, status, err := fetchPDF(srv.URL+"/papers/thesis.pdf", outdir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := status, "overwritten"; got != want {
		t.Fatalf("got status %q, want %q", got, want)
	}
	if got, want := dir, paperDir; got != want {
		t.Fatalf("got dir %q, want %q", got, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "paper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Hello World")) {
		t.Fatalf("got pdftotext output %q, want it to contain \"Hello World\"", got)
	}
}

func TestHandleGzipTarTraversalBlocked(t *testing.T) {
	files := map[string]string{
		"../../../etc/passwd.tex": "malicious",
	}
	dir := t.TempDir()
	if err := handleGzip(makeTarGz(files), dir); err == nil {
		t.Fatal("got nil error, want error for path traversal")
	}
}

func TestHandleGzipNoTexFiles(t *testing.T) {
	files := map[string]string{
		"readme.md": "no tex here",
	}
	dir := t.TempDir()
	if err := handleGzip(makeTarGz(files), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fetched")); err != nil {
		t.Fatal("got no .fetched sentinel, want .fetched for tar with no .tex files")
	}
}

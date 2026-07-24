package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRemoteArticle(t *testing.T) {
	const want = "# Erdős number\n\nSome article text.\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		io.WriteString(w, want)
	}))
	defer srv.Close()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	if err := wikiRemote(srv.URL, "article", "Erdős number"); err != nil {
		os.Stdout = old
		t.Fatalf("wikiRemote: %v", err)
	}
	w.Close()
	os.Stdout = old
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoteSearch(t *testing.T) {
	const want = `{"query":"euler","results":1,"truncated":false,"matches":[{"title":"Euler","score":1.5}]}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, want)
	}))
	defer srv.Close()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	if err := wikiRemote(srv.URL, "search", "euler"); err != nil {
		os.Stdout = old
		t.Fatalf("wikiRemote: %v", err)
	}
	w.Close()
	os.Stdout = old
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoteLinks(t *testing.T) {
	const want = `{"title":"Euler","links":["Mathematician","Swiss"]}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, want)
	}))
	defer srv.Close()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	if err := wikiRemote(srv.URL, "links", "Euler"); err != nil {
		os.Stdout = old
		t.Fatalf("wikiRemote: %v", err)
	}
	w.Close()
	os.Stdout = old
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoteRequestShape(t *testing.T) {
	var gotPath, gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, "ok")
	}))
	defer srv.Close()
	old := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer devnull.Close()
	os.Stdout = devnull
	if err := wikiRemote(srv.URL, "article", "Erdős number"); err != nil {
		os.Stdout = old
		t.Fatalf("wikiRemote: %v", err)
	}
	os.Stdout = old
	if got, want := gotPath, "/wikipedia"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := gotContentType, "application/json"; got != want {
		t.Errorf("content-type = %q, want %q", got, want)
	}
	var req struct {
		Subcommand string `json:"subcommand"`
		Query      string `json:"query"`
	}
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if got, want := req.Subcommand, "article"; got != want {
		t.Errorf("subcommand = %q, want %q", got, want)
	}
	if got, want := req.Query, "Erdős number"; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestRemoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	err := wikiRemote(srv.URL, "search", "x")
	if err == nil {
		t.Fatal("want error on non-200 response")
	}
	if got := err.Error(); !strings.Contains(got, "503") {
		t.Errorf("error = %q, want status code in message", got)
	}
}

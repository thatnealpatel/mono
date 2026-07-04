package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdRepoFork(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/repos/owner/repo/forks"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var body struct{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"full_name":"notnealpatel/repo","html_url":"https://github.com/notnealpatel/repo","default_branch":"main","fork":true}`))
	}))

	if err := cmdRepoFork(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCmdRepoForkHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Forbidden"}`))
	}))

	err := cmdRepoFork(context.Background(), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to contain 403", err)
	}
}

func TestCmdRepoSync(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/repos/owner/repo/merge-upstream"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var body struct {
			Branch string `json:"branch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := body.Branch, "main"; got != want {
			t.Errorf("branch = %q, want %q", got, want)
		}
		w.Write([]byte(`{"message":"Successfully fetched and fast-forwarded from upstream main.","merge_type":"fast-forward","base_branch":"main"}`))
	}))

	if err := cmdRepoSync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCmdRepoSyncCustomBranch(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Branch string `json:"branch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := body.Branch, "develop"; got != want {
			t.Errorf("branch = %q, want %q", got, want)
		}
		w.Write([]byte(`{"message":"ok","merge_type":"fast-forward","base_branch":"develop"}`))
	}))

	if err := cmdRepoSync(context.Background(), []string{"-branch", "develop"}); err != nil {
		t.Fatal(err)
	}
}

func TestCmdRepoSyncHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"message":"merge conflict"}`))
	}))

	err := cmdRepoSync(context.Background(), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error = %q, want it to contain 409", err)
	}
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	// Add a top-level directory entry (GitHub convention).
	if err := tw.WriteHeader(&tar.Header{
		Name:     "owner-repo-abc123/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		t.Fatalf("write dir header: %v", err)
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     "owner-repo-abc123/" + name,
			Size:     int64(len(content)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestCmdRepoClone(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{
		"README.md": "# hello",
		"main.go":   "package main",
	})

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/repos/owner/repo/tarball/"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Write(tarball)
	}))

	dir := filepath.Join(t.TempDir(), "repo")
	if err := cmdRepoClone(context.Background(), []string{"-dir", dir}); err != nil {
		t.Fatal(err)
	}

	// Verify files were extracted.
	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if got, want := string(data), "# hello"; got != want {
		t.Errorf("README.md = %q, want %q", got, want)
	}

	// Verify git repo was initialized.
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf(".git directory missing: %v", err)
	}
}

func TestCmdRepoCloneDefaultDir(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{"x.txt": "x"})

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))

	// Run from a temp directory so the default "repo" dir lands there.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	if err := cmdRepoClone(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "repo", "x.txt")); err != nil {
		t.Fatalf("default dir file missing: %v", err)
	}
}

func TestCmdRepoCloneHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))

	err := cmdRepoClone(context.Background(), []string{"-dir", t.TempDir()})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err)
	}
}

func TestExtractTarGzStripsPrefix(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{
		"sub/deep.txt": "nested",
	})
	dir := t.TempDir()
	if err := extractTarGz(tarball, dir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sub", "deep.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, want := string(data), "nested"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestExtractTarGzPathTraversal(t *testing.T) {
	// Craft a tarball with a path-traversal entry.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "owner-repo-abc123/../../../etc/passwd",
		Size:     5,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("owned")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	dir := t.TempDir()
	err := extractTarGz(buf.Bytes(), dir)
	if err == nil {
		t.Fatal("want error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "escapes target") {
		t.Errorf("error = %q, want it to contain 'escapes target'", err)
	}
}

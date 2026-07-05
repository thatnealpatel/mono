package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdRepoFork(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/gh/repos/owner/repo/forks"; got != want {
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
		if got, want := r.URL.Path, "/gh/repos/owner/repo/merge-upstream"; got != want {
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

func TestGitURL(t *testing.T) {
	oldBase := proxyBase
	t.Cleanup(func() { proxyBase = oldBase })

	for _, tc := range []struct {
		name, base, repo, want string
	}{
		{"Simple", "http://host:9001", "owner/repo", "http://host:9001/git/owner/repo.git"},
		{"TrailingSlash", "http://host:9001", "org/lib", "http://host:9001/git/org/lib.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxyBase = tc.base
			got, err := gitURL(tc.repo)
			if err != nil {
				t.Fatalf("gitURL: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// initBareRepo creates a bare git repo with one commit, suitable for
// serving over smart HTTP via git-http-backend.
func initBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "test.git")

	work := filepath.Join(dir, "work")
	for _, argv := range [][]string{
		{"git", "init", work},
		{"git", "-C", work, "commit", "--allow-empty", "-m", "initial"},
		{"git", "clone", "--bare", work, bare},
		{"git", "-C", bare, "update-server-info"},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
	// Enable http.receivepack for smart HTTP.
	cmd := exec.Command("git", "-C", bare, "config", "http.receivepack", "true")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	return bare
}

// gitHTTPServer returns an httptest.Server serving smart HTTP for bare
// repos under root. Repos are accessed at /git/<name>/... mirroring the
// proxy path shape that Px1 will serve.
func gitHTTPServer(t *testing.T, root string) *httptest.Server {
	t.Helper()
	backend, err := exec.LookPath("git-http-backend")
	if err != nil {
		// Fall back to the git exec-path location.
		out, lookErr := exec.Command("git", "--exec-path").Output()
		if lookErr != nil {
			t.Skipf("git-http-backend not found: %v", err)
		}
		backend = filepath.Join(strings.TrimSpace(string(out)), "git-http-backend")
		if _, statErr := os.Stat(backend); statErr != nil {
			t.Skipf("git-http-backend not found at %s", backend)
		}
	}
	handler := &cgi.Handler{
		Path: backend,
		Env: []string{
			"GIT_PROJECT_ROOT=" + root,
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}
	// Strip /git/ prefix so git-http-backend sees repo-relative paths.
	mux := http.NewServeMux()
	mux.Handle("/git/", http.StripPrefix("/git", handler))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCmdRepoClone(t *testing.T) {
	bare := initBareRepo(t)
	root := filepath.Dir(bare)
	srv := gitHTTPServer(t, root)

	oldProxy := proxyBase
	t.Cleanup(func() { proxyBase = oldProxy })
	proxyBase = srv.URL

	// Repo name matches bare dir name (test.git -> test).
	// Clone URL: <srv>/git/owner/test.git
	// We name the bare dir to match the owner/repo.git pattern.
	ownerDir := filepath.Join(root, "owner")
	if err := os.MkdirAll(ownerDir, 0o755); err != nil {
		t.Fatalf("mkdir owner: %v", err)
	}
	if err := os.Rename(bare, filepath.Join(ownerDir, "repo.git")); err != nil {
		t.Fatalf("rename bare: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "cloned")
	if err := cmdRepoClone(context.Background(), []string{"owner/repo", dir}); err != nil {
		t.Fatalf("cmdRepoClone: %v", err)
	}
	// Verify .git exists (real clone).
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf(".git missing: %v", err)
	}
	// Verify origin remote points through the proxy.
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	if got := strings.TrimSpace(string(out)); !strings.HasPrefix(got, srv.URL) {
		t.Errorf("origin = %q, want prefix %q", got, srv.URL)
	}
}

func TestCmdRepoCloneDefaultDir(t *testing.T) {
	bare := initBareRepo(t)
	root := filepath.Dir(bare)
	srv := gitHTTPServer(t, root)

	oldProxy := proxyBase
	t.Cleanup(func() { proxyBase = oldProxy })
	proxyBase = srv.URL

	ownerDir := filepath.Join(root, "org")
	if err := os.MkdirAll(ownerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(bare, filepath.Join(ownerDir, "lib.git")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// Run from a temp directory so the default dir lands there.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	if err := cmdRepoClone(context.Background(), []string{"org/lib"}); err != nil {
		t.Fatalf("cmdRepoClone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "lib", ".git")); err != nil {
		t.Fatalf("default dir .git missing: %v", err)
	}
}

func TestCmdRepoCloneInvalidRepo(t *testing.T) {
	for _, repo := range []string{"noslash", "/leading", "trailing/", ""} {
		args := []string{repo}
		if repo == "" {
			args = nil // triggers len(args) < 1
		}
		err := cmdRepoClone(context.Background(), args)
		if err == nil {
			t.Errorf("cmdRepoClone(%q): want error, got nil", repo)
		}
	}
}

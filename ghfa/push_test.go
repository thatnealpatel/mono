package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a git repo with some commits for push testing.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	}
	for _, argv := range [][]string{
		{"git", "init"},
		{"git", "checkout", "-b", "main"},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
	// Create initial commit.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	for _, argv := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
	return dir
}

func addCommit(t *testing.T, dir, file, content, message string) {
	t.Helper()
	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	for _, argv := range [][]string{
		{"git", "add", file},
		{"git", "commit", "-m", message},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
}

// deleteCommit removes a file and commits with the given message.
func deleteCommit(t *testing.T, dir, file, message string) {
	t.Helper()
	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	}
	for _, argv := range [][]string{
		{"git", "rm", file},
		{"git", "commit", "-m", message},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestMarkerPath(t *testing.T) {
	got := markerPath("/repo/.git", "bot/machine/slug")
	if want := "/repo/.git/ghfa-push-bot_machine_slug"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastPushedSHANoMarker(t *testing.T) {
	dir := initTestRepo(t)
	gitDir := filepath.Join(dir, ".git")
	sha, err := lastPushedSHA(gitDir, "nonexistent")
	if err != nil {
		t.Fatalf("lastPushedSHA: %v", err)
	}
	// Should return root commit.
	root, err := rootCommit(gitDir)
	if err != nil {
		t.Fatalf("rootCommit: %v", err)
	}
	if sha != root {
		t.Errorf("got %q, want root %q", sha, root)
	}
}

func TestLastPushedSHAWithMarker(t *testing.T) {
	dir := initTestRepo(t)
	gitDir := filepath.Join(dir, ".git")
	if err := saveLastPushedSHA(gitDir, "mybranch", "deadbeef"); err != nil {
		t.Fatalf("save: %v", err)
	}
	sha, err := lastPushedSHA(gitDir, "mybranch")
	if err != nil {
		t.Fatalf("lastPushedSHA: %v", err)
	}
	if got, want := sha, "deadbeef"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCommitChain(t *testing.T) {
	dir := initTestRepo(t)
	rootSHA := headSHA(t, dir)
	addCommit(t, dir, "b.txt", "world", "add b")
	addCommit(t, dir, "c.txt", "!", "add c")

	gitDir := filepath.Join(dir, ".git")
	commits, err := buildCommitChain(context.Background(), gitDir, rootSHA)
	if err != nil {
		t.Fatalf("buildCommitChain: %v", err)
	}
	if got, want := len(commits), 2; got != want {
		t.Fatalf("len(commits) = %d, want %d", got, want)
	}
	if got, want := commits[0].Message, "add b"; got != want {
		t.Errorf("commits[0].Message = %q, want %q", got, want)
	}
	if got, want := commits[1].Message, "add c"; got != want {
		t.Errorf("commits[1].Message = %q, want %q", got, want)
	}
	// Check files are base64-encoded.
	if len(commits[0].Files) != 1 {
		t.Fatalf("commits[0].Files = %d, want 1", len(commits[0].Files))
	}
	if got, want := commits[0].Files[0].Path, "b.txt"; got != want {
		t.Errorf("commits[0].Files[0].Path = %q, want %q", got, want)
	}
	decoded, err := base64.StdEncoding.DecodeString(commits[0].Files[0].Content)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if got, want := string(decoded), "world"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestBuildCommitChainEmpty(t *testing.T) {
	dir := initTestRepo(t)
	sha := headSHA(t, dir)
	gitDir := filepath.Join(dir, ".git")
	commits, err := buildCommitChain(context.Background(), gitDir, sha)
	if err != nil {
		t.Fatalf("buildCommitChain: %v", err)
	}
	if commits != nil {
		t.Errorf("got %v, want nil", commits)
	}
}

func TestCmdPush(t *testing.T) {
	dir := initTestRepo(t)
	rootSHA := headSHA(t, dir)
	addCommit(t, dir, "proof.lean", "theorem p : True := trivial", "add proof")

	// Point findGitDir at our test repo.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	resultSHA := "abc123resultsha"
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/gh/push"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var req pushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := req.Repo, "owner/repo"; got != want {
			t.Errorf("repo = %q, want %q", got, want)
		}
		if got, want := req.Ref, "refs/heads/bot/test"; got != want {
			t.Errorf("ref = %q, want %q", got, want)
		}
		if got, want := req.Parent, rootSHA; got != want {
			t.Errorf("parent = %q, want %q", got, want)
		}
		if got, want := len(req.Commits), 1; got != want {
			t.Fatalf("len(commits) = %d, want %d", got, want)
		}
		if got, want := req.Commits[0].Message, "add proof"; got != want {
			t.Errorf("message = %q, want %q", got, want)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ref":"refs/heads/bot/test","sha":"` + resultSHA + `"}`))
	}))

	if err := cmdPush(context.Background(), []string{"-ref", "bot/test"}); err != nil {
		t.Fatal(err)
	}

	// Verify marker was saved.
	gitDir := filepath.Join(dir, ".git")
	saved, err := lastPushedSHA(gitDir, "bot/test")
	if err != nil {
		t.Fatalf("lastPushedSHA: %v", err)
	}
	if got, want := saved, resultSHA; got != want {
		t.Errorf("saved SHA = %q, want %q", got, want)
	}
}

func TestCmdPushMissingRef(t *testing.T) {
	err := cmdPush(context.Background(), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error = %q, want usage message", err)
	}
}

func TestCmdPushNoCommitsAhead(t *testing.T) {
	dir := initTestRepo(t)
	sha := headSHA(t, dir)

	// Save marker at HEAD so there's nothing to push.
	gitDir := filepath.Join(dir, ".git")
	if err := saveLastPushedSHA(gitDir, "test", sha); err != nil {
		t.Fatalf("save: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	setupTest(t, nil)
	err = cmdPush(context.Background(), []string{"-ref", "test"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "no commits ahead") {
		t.Errorf("error = %q, want 'no commits ahead'", err)
	}
}

func TestCmdPushHTTPError(t *testing.T) {
	dir := initTestRepo(t)
	addCommit(t, dir, "x.txt", "x", "add x")

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"push not allowed"}`))
	}))

	err = cmdPush(context.Background(), []string{"-ref", "test"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to contain 403", err)
	}
}

func TestBuildCommitChainMultiLineMessage(t *testing.T) {
	dir := initTestRepo(t)
	rootSHA := headSHA(t, dir)

	// Create a commit with a multi-line message.
	if err := os.WriteFile(filepath.Join(dir, "m.txt"), []byte("multi"), 0o644); err != nil {
		t.Fatalf("write m.txt: %v", err)
	}
	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	}
	for _, argv := range [][]string{
		{"git", "add", "m.txt"},
		{"git", "commit", "-m", "subject line\n\nbody paragraph"},
	} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}

	gitDir := filepath.Join(dir, ".git")
	commits, err := buildCommitChain(context.Background(), gitDir, rootSHA)
	if err != nil {
		t.Fatalf("buildCommitChain: %v", err)
	}
	if got, want := len(commits), 1; got != want {
		t.Fatalf("len(commits) = %d, want %d", got, want)
	}
	if !strings.Contains(commits[0].Message, "body paragraph") {
		t.Errorf("message = %q, want it to contain body", commits[0].Message)
	}
}

func TestBuildCommitChainDeletedFile(t *testing.T) {
	dir := initTestRepo(t)
	addCommit(t, dir, "doomed.txt", "bye", "add doomed")
	rootSHA := headSHA(t, dir)
	deleteCommit(t, dir, "doomed.txt", "remove doomed")

	gitDir := filepath.Join(dir, ".git")
	commits, err := buildCommitChain(context.Background(), gitDir, rootSHA)
	if err != nil {
		t.Fatalf("buildCommitChain: %v", err)
	}
	if got, want := len(commits), 1; got != want {
		t.Fatalf("len(commits) = %d, want %d", got, want)
	}
	if got, want := len(commits[0].Files), 1; got != want {
		t.Fatalf("len(files) = %d, want %d", got, want)
	}
	f := commits[0].Files[0]
	if got, want := f.Path, "doomed.txt"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if !f.Deleted {
		t.Errorf("deleted = false, want true")
	}
	if f.Content != "" {
		t.Errorf("content = %q, want empty", f.Content)
	}
}

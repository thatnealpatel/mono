package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pushRequest is the payload sent to the proxy's push endpoint.
type pushRequest struct {
	Repo    string        `json:"repo"`
	Ref     string        `json:"ref"`
	Parent  string        `json:"parent"`
	Commits []commitEntry `json:"commits"`
}

// commitEntry is a single commit in the push payload.
type commitEntry struct {
	Message string      `json:"message"`
	Files   []fileEntry `json:"files"`
}

// fileEntry is a single file change within a commit.
// Deleted is true when the file was removed; Content is empty in that case.
type fileEntry struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// pushResult is the response from the proxy push endpoint.
type pushResult struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// cmdPush reads local git state and sends commits to the proxy.
// Usage: ghfa <owner/repo> push -ref <branch>
func cmdPush(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	ref := fs.String("ref", "", "branch name to push (e.g. bot/machine/slug)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return fmt.Errorf("usage: ghfa <owner/repo> push -ref <branch>")
	}

	gitDir, err := findGitDir()
	if err != nil {
		return err
	}

	parent, err := lastPushedSHA(gitDir, *ref)
	if err != nil {
		return err
	}

	commits, err := buildCommitChain(ctx, gitDir, parent)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return fmt.Errorf("ghfa: no commits ahead of %s", parent)
	}

	req := pushRequest{
		Repo:    upstream,
		Ref:     "refs/heads/" + *ref,
		Parent:  parent,
		Commits: commits,
	}
	pushURL, err := url.JoinPath(apiBase, "gh", "push")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPost, pushURL, req)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return statusError(status, resp)
	}

	var result pushResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("ghfa: decode push result: %w", err)
	}

	// Update the local marker for next push.
	if err := saveLastPushedSHA(gitDir, *ref, result.SHA); err != nil {
		return fmt.Errorf("ghfa: save push marker: %w", err)
	}

	return printJSON(result)
}

// findGitDir locates the .git directory from the working directory.
func findGitDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".git")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("ghfa: not a git repository")
		}
		dir = parent
	}
}

// markerPath returns the path for the push SHA marker file.
func markerPath(gitDir, branch string) string {
	safe := strings.ReplaceAll(branch, "/", "_")
	return filepath.Join(gitDir, "ghfa-push-"+safe)
}

// lastPushedSHA reads the SHA of the last successful push for a branch.
// Returns the root commit if no marker exists.
func lastPushedSHA(gitDir, branch string) (string, error) {
	data, err := os.ReadFile(markerPath(gitDir, branch))
	if err != nil {
		if os.IsNotExist(err) {
			// No previous push; use the root commit.
			return rootCommit(gitDir)
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// rootCommit returns the SHA of the first commit in the repo.
func rootCommit(gitDir string) (string, error) {
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = filepath.Dir(gitDir)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-list: %w", err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return "", fmt.Errorf("ghfa: no root commit found")
	}
	return lines[0], nil
}

// saveLastPushedSHA writes the push marker for next invocation.
func saveLastPushedSHA(gitDir, branch, sha string) error {
	return os.WriteFile(markerPath(gitDir, branch), []byte(sha+"\n"), 0o644)
}

// buildCommitChain returns the commits between parent (exclusive) and HEAD (inclusive).
// Each commit includes only the files changed in that commit.
func buildCommitChain(ctx context.Context, gitDir, parent string) ([]commitEntry, error) {
	repoDir := filepath.Dir(gitDir)

	// List commit SHAs parent..HEAD, oldest first.
	cmd := exec.CommandContext(ctx, "git", "log", "--reverse", "--format=%H", parent+"..HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	shas := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(shas) == 1 && shas[0] == "" {
		return nil, nil
	}

	var commits []commitEntry
	for _, sha := range shas {
		msg, err := commitMessage(ctx, repoDir, sha)
		if err != nil {
			return nil, err
		}
		files, err := changedFiles(ctx, repoDir, sha)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commitEntry{
			Message: msg,
			Files:   files,
		})
	}
	return commits, nil
}

// commitMessage returns the full commit message for a SHA.
func commitMessage(ctx context.Context, repoDir, sha string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%B", sha)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log -1: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// changedFiles returns the files changed in a single commit
// with their content base64-encoded. Deleted files are marked
// with Deleted: true and no content.
func changedFiles(ctx context.Context, repoDir, sha string) ([]fileEntry, error) {
	cmd := exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "-r", "--diff-filter=ACDMRT", "--name-status", sha)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	var files []fileEntry
	for _, line := range lines {
		status, name, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("ghfa: malformed diff-tree line: %q", line)
		}
		if status == "D" {
			files = append(files, fileEntry{Path: name, Deleted: true})
			continue
		}
		// Read file content at this commit's version.
		show := exec.CommandContext(ctx, "git", "show", sha+":"+name)
		show.Dir = repoDir
		var buf bytes.Buffer
		show.Stdout = &buf
		if err := show.Run(); err != nil {
			return nil, fmt.Errorf("git show %s:%s: %w", sha, name, err)
		}
		files = append(files, fileEntry{
			Path:    name,
			Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
		})
	}
	return files, nil
}

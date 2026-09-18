package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

// repository is a GitHub repository object.
type repository struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Fork          bool   `json:"fork"`
}

// cmdRepoFork forks the repository. Usage: ghfa <owner/repo> repo fork
func cmdRepoFork(ctx context.Context, proxy, repo string, out io.Writer, args []string) error {
	rawURL, err := url.JoinPath(proxy, "gh", "repos", repo, "forks")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPost, rawURL, struct{}{})
	if err != nil {
		return err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return statusError(status, resp)
	}
	var fork repository
	if err := json.Unmarshal(resp, &fork); err != nil {
		return fmt.Errorf("ghfa: decode fork: %w", err)
	}
	return writeJSON(out, fork)
}

// cmdRepoClone clones a repo through the proxy's smart HTTP surface. Usage:
// ghfa <owner/repo> repo clone [<dir>]
func cmdRepoClone(ctx context.Context, proxy, repo string, out io.Writer, args []string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("ghfa: invalid repo %q, want owner/repo", repo)
	}

	target := parts[1]
	if len(args) >= 1 {
		target = args[0]
	}

	cloneURL, err := url.JoinPath(proxy, "git", repo+".git")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, target)
	commandOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, commandOutput)
	}
	return writeJSON(out, struct {
		Dir string `json:"dir"`
	}{Dir: target})
}

// mergeUpstreamResult is the response
// from POST merge-upstream.
type mergeUpstreamResult struct {
	Message    string `json:"message"`
	MergeType  string `json:"merge_type"`
	BaseBranch string `json:"base_branch"`
}

// cmdRepoSync syncs a fork from its upstream. Usage: ghfa <owner/repo> repo
// sync [-branch <name>]
func cmdRepoSync(ctx context.Context, proxy, repo string, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("repo sync", flag.ContinueOnError)
	branch := fs.String("branch", "main", "branch to sync")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rawURL, err := url.JoinPath(proxy, "gh", "repos", repo, "merge-upstream")
	if err != nil {
		return err
	}
	body := struct {
		Branch string `json:"branch"`
	}{Branch: *branch}
	resp, _, status, err := do(ctx, http.MethodPost, rawURL, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}
	var result mergeUpstreamResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("ghfa: decode merge-upstream: %w", err)
	}
	return writeJSON(out, result)
}

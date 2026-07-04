package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repository is a GitHub repository object.
type repository struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Fork          bool   `json:"fork"`
}

// cmdRepoFork forks the repository.
// Usage: ghfa <owner/repo> repo fork
func cmdRepoFork(ctx context.Context, args []string) error {
	rawURL, err := url.JoinPath(apiBase, "repos", upstream, "forks")
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
	var repo repository
	if err := json.Unmarshal(resp, &repo); err != nil {
		return fmt.Errorf("ghfa: decode fork: %w", err)
	}
	return printJSON(repo)
}

// cmdRepoClone downloads a tarball and initializes a local git repo.
// Usage: ghfa <owner/repo> repo clone [-dir <path>]
func cmdRepoClone(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo clone", flag.ContinueOnError)
	dir := fs.String("dir", "", "target directory (default: repo name)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Default dir to the repo name (second segment of owner/repo).
	target := *dir
	if target == "" {
		parts := strings.SplitN(upstream, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upstream %q", upstream)
		}
		target = parts[1]
	}

	// Trailing slash stands for the default branch ref segment.
	rawURL, err := url.JoinPath(apiBase, "repos", upstream, "tarball/")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(resp, target); err != nil {
		return fmt.Errorf("ghfa: extract tarball: %w", err)
	}
	if err := gitInit(ctx, target); err != nil {
		return fmt.Errorf("ghfa: git init: %w", err)
	}
	return printJSON(struct {
		Dir string `json:"dir"`
	}{Dir: target})
}

// extractTarGz extracts a gzipped tarball into dir,
// stripping the top-level directory GitHub includes.
func extractTarGz(data []byte, dir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	// GitHub tarballs have a top-level prefix like "owner-repo-sha/".
	// We strip the first path component.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Strip the first path component.
		name := hdr.Name
		idx := strings.IndexByte(name, '/')
		if idx < 0 {
			continue
		}
		rel := name[idx+1:]
		if rel == "" {
			continue
		}
		target := filepath.Join(dir, rel)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dir)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry escapes target: %s", name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755|0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

// gitInit runs git init, add, commit in the target directory.
func gitInit(ctx context.Context, dir string) error {
	for _, argv := range [][]string{
		{"git", "init"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=ghfa",
			"GIT_AUTHOR_EMAIL=ghfa@localhost",
			"GIT_COMMITTER_NAME=ghfa",
			"GIT_COMMITTER_EMAIL=ghfa@localhost",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(argv, " "), err, out)
		}
	}
	return nil
}

// mergeUpstreamResult is the response from POST merge-upstream.
type mergeUpstreamResult struct {
	Message    string `json:"message"`
	MergeType  string `json:"merge_type"`
	BaseBranch string `json:"base_branch"`
}

// cmdRepoSync syncs a fork from its upstream.
// Usage: ghfa <owner/repo> repo sync [-branch <name>]
func cmdRepoSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo sync", flag.ContinueOnError)
	branch := fs.String("branch", "main", "branch to sync")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rawURL, err := url.JoinPath(apiBase, "repos", upstream, "merge-upstream")
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
	return printJSON(result)
}

// Package main implements a read-only
// version of the 'gh' CLI that otherwise
// permits mutating operations.
//
// It is intended to minimize the exposed
// surface area when used by LLM agents.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ghCacheRoot string
	httpClient  = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	log.SetFlags(0)

	ghCacheRoot = os.Getenv("GH_CACHE_DIR")
	if ghCacheRoot == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			log.Fatal(err)
		}
		ghCacheRoot = filepath.Join(base, "gh")
	}

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	if args[0] == "-h" {
		fmt.Fprint(os.Stdout, usage)
		return
	}
	repo := args[0]
	args = args[1:]

	if len(args) == 0 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	if args[0] == "search" {
		if len(args) < 2 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		query := strings.Join(args[1:], " ")
		if err := ghSearch(repo, query); err != nil {
			fmt.Fprintf(os.Stderr, "goof-gh: %v\n", err)
			os.Exit(1)
		}
		return
	}

	num, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	all := len(args) > 1 && args[1] == "-all"
	if err := ghShow(repo, num, all); err != nil {
		fmt.Fprintf(os.Stderr, "goof-gh: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: gh <owner/repo> <command> [args]

  <owner/repo> <num>          show issue
  <owner/repo> <num> -all     show issue with comments
  <owner/repo> search <query> search issues
`

func ghSearch(repo string, query string) error {
	token := loadGitHubToken()
	q := "repo:" + repo + " " + query
	if !strings.Contains(q, "is:") {
		q = "is:issue " + q
	}
	base, err := url.JoinPath(githubAPI, "search", "issues")
	if err != nil {
		return err
	}
	u := base + "?q=" + url.QueryEscape(q) + "&per_page=30"
	body, _, err := githubGet(u, token)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}
	fmt.Print(string(body))
	return nil
}

func loadGitHubToken() string {
	return os.Getenv("READONLY_GITHUB_TOKEN")
}

const githubAPI = "https://api.github.com"

func githubGet(u, token string) (body []byte, next string, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "goof")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, "", rateLimitError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
	}
	return data, parseLinkNext(resp.Header.Get("Link")), nil
}

func rateLimitError(resp *http.Response) error {
	reset := resp.Header.Get("X-RateLimit-Reset")
	if secs, err := strconv.ParseInt(reset, 10, 64); err == nil {
		reset = time.Until(time.Unix(secs, 0)).Round(time.Second).String()
	}
	return fmt.Errorf("HTTP %d rate limited: %s/%s remaining, resets in %s",
		resp.StatusCode,
		resp.Header.Get("X-RateLimit-Remaining"),
		resp.Header.Get("X-RateLimit-Limit"),
		reset)
}

func parseLinkNext(header string) string {
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		if i := strings.Index(part, "<"); i >= 0 {
			if j := strings.Index(part, ">"); j > i {
				return part[i+1 : j]
			}
		}
	}
	return ""
}

func ghCacheDir(repo string) string {
	return filepath.Join(ghCacheRoot, repo)
}

func ghShow(repo string, num int, comments bool) error {
	dir := ghCacheDir(repo)
	token := loadGitHubToken()
	numStr := strconv.Itoa(num)
	baseURL, err := url.JoinPath(githubAPI, "repos", repo)
	if err != nil {
		return err
	}
	if err := ghEnsure(dir, numStr, token, baseURL); err != nil {
		return err
	}
	if comments {
		data, err := os.ReadFile(filepath.Join(dir, numStr, "comments.json"))
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, numStr, "issue.json"))
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func ghEnsure(dir, num, token, baseURL string) (retErr error) {
	ghDir := filepath.Join(dir, num)
	if info, err := os.Stat(filepath.Join(ghDir, "issue.json")); err == nil {
		if time.Since(info.ModTime()) < 30*time.Minute {
			return nil
		}
		os.RemoveAll(ghDir) // ignore error
	}
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(ghDir) // ignore error
		}
	}()
	issueURL, err := url.JoinPath(baseURL, "issues", num)
	if err != nil {
		return err
	}
	issue, _, err := githubGet(issueURL, token)
	if err != nil {
		return fmt.Errorf("fetching issue: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ghDir, "issue.json"), issue, 0o644); err != nil {
		return err
	}
	cmtsURL, err := url.JoinPath(baseURL, "issues", num, "comments")
	if err != nil {
		return err
	}
	cmts, err := githubGetAll(cmtsURL+"?per_page=100", token)
	if err != nil {
		return fmt.Errorf("fetching comments: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ghDir, "comments.json"), cmts, 0o644); err != nil {
		return err
	}
	return nil
}

func githubGetAll(u, token string) ([]byte, error) {
	var all []json.RawMessage
	for u != "" {
		page, next, err := githubGet(u, token)
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(page, &items); err != nil {
			return nil, fmt.Errorf("parsing page: %w", err)
		}
		all = append(all, items...)
		u = next
	}
	return json.Marshal(all)
}

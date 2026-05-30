// Package main implements a read-only
// version of the 'gh' CLI that otherwise
// permits mutating operations.
//
// It is intended to minimize the exposed
// surface area when used by LLM agents.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ghCacheRoot string

func main() {
	log.SetFlags(0)

	ghCacheRoot = os.Getenv("GH_CACHE_DIR")
	if ghCacheRoot == "" {
		log.Fatal("GH_CACHE_DIR is not set")
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
		if err := ghList(repo); err != nil {
			fmt.Fprintf(os.Stderr, "goof-gh: %v\n", err)
			os.Exit(1)
		}
		return
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

const usage = `usage: gh <owner/repo> [search <query> | <num> [-all]]

  <owner/repo>              list cached issues
  <owner/repo> <num>        show issue
  <owner/repo> <num> -all   show issue with comments
  <owner/repo> search <q>   search issues
`

func ghList(repo string) error {
	dir := ghCacheDir(repo)
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("no cached issues")
		return nil
	}
	var nums []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err == nil {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	if len(nums) == 0 {
		fmt.Println("no cached issues")
		return nil
	}
	fmt.Printf("%d cached issues:\n", len(nums))
	for i, n := range nums {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Printf("%d", n)
		if (i+1)%10 == 0 {
			fmt.Println()
		}
	}
	if len(nums)%10 != 0 {
		fmt.Println()
	}
	return nil
}

func ghCacheDir(repo string) string {
	return filepath.Join(ghCacheRoot, repo)
}

func ghSearch(repo string, query string) error {
	token := loadGitHubToken()
	q := "repo:" + repo + " " + query
	if !strings.Contains(q, "is:") {
		q = "is:issue " + q
	}
	u := githubAPI + "/search/issues?q=" + url.QueryEscape(q) + "&per_page=30"
	body, _, err := githubGet(u, token)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}
	os.Stdout.Write(body)
	return nil
}

func loadGitHubToken() string {
	return os.Getenv("READONLY_GITHUB_TOKEN")
}

const githubAPI = "https://api.github.com"

func githubGet(u, token string) (body []byte, next string, err error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "goof")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
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

func ghShow(repo string, num int, comments bool) error {
	dir := ghCacheDir(repo)
	token := loadGitHubToken()
	numStr := strconv.Itoa(num)
	baseURL := githubAPI + "/repos/" + repo
	if err := ghEnsure(dir, numStr, token, baseURL); err != nil {
		return err
	}
	if comments {
		data, err := os.ReadFile(filepath.Join(dir, numStr, "comments.json"))
		if err != nil {
			return err
		}
		os.Stdout.Write(data)
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, numStr, "issue.json"))
	if err != nil {
		return err
	}
	os.Stdout.Write(data)
	return nil
}

func ghEnsure(dir, num, token, baseURL string) (retErr error) {
	ghDir := filepath.Join(dir, num)
	// https://patel.codes/goof/issues/64
	// if _, err := os.Stat(filepath.Join(ghDir, "issue.json")); err == nil {
	// 	return nil
	// }
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(ghDir)
		}
	}()
	issue, _, err := githubGet(baseURL+"/issues/"+num, token)
	if err != nil {
		return fmt.Errorf("fetching issue: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ghDir, "issue.json"), issue, 0o644); err != nil {
		return err
	}
	cmts, err := githubGetAll(baseURL+"/issues/"+num+"/comments?per_page=100", token)
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

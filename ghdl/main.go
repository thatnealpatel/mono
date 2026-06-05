package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	var ghpat string
	flag.StringVar(&ghpat, "ghpat", "", "GitHub personal access token")
	flag.Parse()

	if ghpat == "" {
		ghpat = os.Getenv("GITHUB_TOKEN")
	}
	if ghpat == "" {
		log.Fatal("provide -ghpat or set GITHUB_TOKEN")
	}

	client = &http.Client{
		Transport: &authTransport{token: ghpat, rt: http.DefaultTransport},
	}

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("usage: ghdl <owner/repo> | rate")
	}

	var err error
	switch args[0] {
	case "rate":
		err = rateLimit()
	default:
		repo = args[0]
		cacheRoot := os.Getenv("GHDL_CACHE_DIR")
		if cacheRoot == "" {
			base, err := os.UserCacheDir()
			if err != nil {
				log.Fatal(err)
			}
			cacheRoot = filepath.Join(base, "ghdl")
		}
		cacheDir = filepath.Join(cacheRoot, repo)
		err = scrape()
	}
	if err != nil {
		log.Fatal(err)
	}
}

var client *http.Client

type authTransport struct {
	token string
	rt    http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	return t.rt.RoundTrip(req)
}

func scrape() error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	listURL := repoURL("issues", url.Values{
		"state":     {"all"},
		"per_page":  {"100"},
		"direction": {"asc"},
	})
	var fetched, skipped, updated int

	for listURL != "" {
		r, err := ghGet(listURL, "")
		if err != nil {
			return fmt.Errorf("listing page: %w", err)
		}
		var raw []struct {
			Number      int       `json:"number"`
			Comments    int       `json:"comments"`
			PullRequest *struct{} `json:"pull_request"`
		}
		if err := json.Unmarshal(r.Body, &raw); err != nil {
			return fmt.Errorf("parsing issues: %w", err)
		}

		for _, entry := range raw {
			if entry.PullRequest != nil {
				continue
			}

			numStr := strconv.Itoa(entry.Number)
			issDir := filepath.Join(cacheDir, numStr)
			issFile := filepath.Join(issDir, "issue.json")
			issETag := filepath.Join(issDir, "issue.etag")
			issURL := repoURL("issues/"+numStr, nil)

			cached := readETag(issETag)
			if cached != "" {
				cr, err := ghGet(issURL, cached)
				if err != nil {
					return fmt.Errorf("issue %d conditional: %w", entry.Number, err)
				}
				if cr.NotMod {
					skipped++
					progress(fetched, updated, skipped)
					continue
				}
				if err := os.WriteFile(issFile, cr.Body, 0o644); err != nil {
					return err
				}
				if err := writeETag(issETag, cr.ETag); err != nil {
					return err
				}
				if err := updateComments(issDir, numStr); err != nil {
					return err
				}
				updated++
				progress(fetched, updated, skipped)
				continue
			}

			if err := os.MkdirAll(issDir, 0o755); err != nil {
				return err
			}

			ir, err := ghGet(issURL, "")
			if err != nil {
				return fmt.Errorf("issue %d fetch: %w", entry.Number, err)
			}
			if err := os.WriteFile(issFile, ir.Body, 0o644); err != nil {
				return err
			}
			if err := writeETag(issETag, ir.ETag); err != nil {
				return err
			}

			if err := updateComments(issDir, numStr); err != nil {
				return err
			}

			fetched++
			progress(fetched, updated, skipped)
		}
		listURL = r.Next
	}
	fmt.Fprintf(os.Stderr, "\rdone: %d new, %d updated, %d unchanged\n", fetched, updated, skipped)
	return nil
}

var (
	repo       string
	cacheDir   string
	linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
)

func repoURL(path string, query url.Values) string {
	u := &url.URL{
		Scheme:   "https",
		Host:     "api.github.com",
		Path:     "/repos/" + repo + "/" + path,
		RawQuery: query.Encode(),
	}
	return u.String()
}

func ghGet(rawURL, ifNoneMatch string) (*ghResponse, error) {
	var retries int
	for {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return nil, err
		}
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotModified {
			return &ghResponse{NotMod: true}, nil
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			wait := rateLimitWait(resp)
			if wait == 0 {
				return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
			}
			fmt.Fprintf(os.Stderr, "\nrate limited, resuming in %s\n", wait.Round(time.Second))
			time.Sleep(wait + time.Minute)
			continue
		}
		if resp.StatusCode >= 500 {
			retries++
			if retries > 5 {
				return nil, fmt.Errorf("HTTP %d after %d retries: %s", resp.StatusCode, retries, body)
			}
			fmt.Fprintf(os.Stderr, "\nHTTP %d, retrying in 5s\n", resp.StatusCode)
			time.Sleep(5 * time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
		}
		r := &ghResponse{Body: body, ETag: resp.Header.Get("ETag")}
		if m := linkNextRe.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			r.Next = m[1]
		}
		return r, nil
	}
}

type ghResponse struct {
	Body   []byte
	Next   string
	ETag   string
	NotMod bool
}

func rateLimitWait(resp *http.Response) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && reset > 0 {
		wait := time.Until(time.Unix(reset, 0))
		if wait < time.Second {
			return time.Second
		}
		return wait
	}
	return 0
}

func readETag(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func progress(fetched, updated, skipped int) {
	fmt.Fprintf(os.Stderr, "\r%d new, %d updated, %d unchanged", fetched, updated, skipped)
}

func writeETag(path, etag string) error {
	if etag == "" {
		return nil
	}
	return os.WriteFile(path, []byte(etag+"\n"), 0o644)
}

func updateComments(issDir, numStr string) error {
	cmtETagFile := filepath.Join(issDir, "comments.etag")
	cmtURL := repoURL("issues/"+numStr+"/comments", url.Values{"per_page": {"100"}})
	cmts, etag, err := ghGetAll(cmtURL)
	if err != nil {
		return fmt.Errorf("issue %s comments: %w", numStr, err)
	}
	if err := os.WriteFile(filepath.Join(issDir, "comments.json"), cmts, 0o644); err != nil {
		return err
	}
	return writeETag(cmtETagFile, etag)
}

func ghGetAll(rawURL string) ([]byte, string, error) {
	var all []json.RawMessage
	var lastETag string
	for rawURL != "" {
		r, err := ghGet(rawURL, "")
		if err != nil {
			return nil, "", err
		}
		var page []json.RawMessage
		if err := json.Unmarshal(r.Body, &page); err != nil {
			return nil, "", err
		}
		all = append(all, page...)
		lastETag = r.ETag
		rawURL = r.Next
	}
	b, err := json.Marshal(all)
	return b, lastETag, err
}

func rateLimit() error {
	r, err := ghGet("https://api.github.com/rate_limit", "")
	if err != nil {
		return err
	}
	type bucket struct {
		Limit     int `json:"limit"`
		Remaining int `json:"remaining"`
		Reset     int `json:"reset"`
	}
	var result struct {
		Resources struct {
			Core   bucket `json:"core"`
			Search bucket `json:"search"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(r.Body, &result); err != nil {
		return err
	}
	for _, b := range []struct {
		name string
		bucket
	}{
		{"core", result.Resources.Core},
		{"search", result.Resources.Search},
	} {
		reset := time.Unix(int64(b.Reset), 0)
		fmt.Fprintf(os.Stderr, "%s: %d/%d remaining (resets %s)\n", b.name, b.Remaining, b.Limit, reset.Format(time.Kitchen))
	}
	return nil
}

// Package main implements a read-only
// wrapper around the Gerrit API; it
// does not implement or permit write
// operations by design.
//
// It is intended to minimize the exposed
// surface area when used by LLM agents.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	gerritDir  string
	gerritBase = "https://go-review.googlesource.com"
	httpClient = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	log.SetFlags(0)

	if instance := os.Getenv("GERRIT_REVIEW_INSTANCE"); instance != "" {
		gerritBase = "https://" + instance + ".googlesource.com"
	}

	gerritDir = os.Getenv("GERRIT_CACHE_DIR")
	if gerritDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			log.Fatal(err)
		}
		gerritDir = filepath.Join(base, "gerrit")
	}

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	switch args[0] {
	case "-h":
		fmt.Print(usage)
	case "search":
		if len(args) < 2 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		if err := gerritSearch(strings.Join(args[1:], "+")); err != nil {
			fmt.Fprintf(os.Stderr, "goof-cl: %v\n", err)
			os.Exit(1)
		}
	case "fetch":
		if len(args) < 2 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(1)
		}
		all := len(args) > 2 && args[2] == "-all"
		if err := gerritShow(args[1], all); err != nil {
			fmt.Fprintf(os.Stderr, "goof-gerrit: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
}

const usage = `usage: gerrit <command> [args]

  fetch <number> [-all]   show CL by Gerrit change number
  fetch <commit> [-all]   show CL by commit hash (min 8 hex chars)
  search <query>          search Gerrit

flags:
  -all   include patchset diff in output

env: GERRIT_REVIEW_INSTANCE (default: go-review)
`

func gerritSearch(query string) error {
	base, err := neturl.JoinPath(gerritBase, "changes")
	if err != nil {
		return err
	}
	url := base + "?q=" + neturl.QueryEscape(query) + "&n=50&o=DETAILED_ACCOUNTS"
	body, err := gerritJSON(url)
	if err != nil {
		return err
	}
	var results []struct {
		Number  int    `json:"_number"`
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Project string `json:"project"`
		Owner   struct {
			Name string `json:"name"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return fmt.Errorf("parsing search results: %w", err)
	}
	if len(results) == 0 {
		fmt.Println("no results")
		return nil
	}
	return printJSON(results)
}

func gerritJSON(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if i := bytes.IndexByte(body, '\n'); i >= 0 && body[0] == ')' {
		body = body[i+1:]
	}
	return body, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func gerritShow(arg string, all bool) error {
	num, err := gerritResolveArg(arg)
	if err != nil {
		return err
	}
	if err := gerritEnsure(gerritDir, num); err != nil {
		return err
	}
	detail, err := os.ReadFile(filepath.Join(gerritDir, num, "detail.json"))
	if err != nil {
		return err
	}
	comments, err := os.ReadFile(filepath.Join(gerritDir, num, "comments.json"))
	if err != nil {
		return err
	}

	cleanDetail := cleanupDetail(detail)
	cleanComments := cleanupComments(comments)

	var patch string
	if all {
		p, err := os.ReadFile(filepath.Join(gerritDir, num, "patch.diff"))
		if err != nil {
			return err
		}
		patch = string(p)
	}

	out := struct {
		Number   int    `json:"number"`
		URL      string `json:"url"`
		Detail   any    `json:"detail"`
		Comments any    `json:"comments"`
		Issues   []int  `json:"issues,omitempty"`
		Patch    string `json:"patch,omitempty"`
	}{
		Number:   mustAtoi(num),
		URL:      "https://go.dev/cl/" + num,
		Detail:   cleanDetail,
		Comments: cleanComments,
		Issues:   clExtractIssueNums(detail),
		Patch:    patch,
	}
	return printJSON(out)
}

type gerritAccount struct {
	AccountID   int      `json:"_account_id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email"`
	Avatars     any      `json:"avatars"`
	Tags        []string `json:"tags"`
}

type gerritDetail struct {
	Number          int                        `json:"_number"`
	Subject         string                     `json:"subject"`
	Status          string                     `json:"status"`
	Project         string                     `json:"project"`
	Branch          string                     `json:"branch"`
	Created         string                     `json:"created"`
	Updated         string                     `json:"updated"`
	Insertions      int                        `json:"insertions"`
	Deletions       int                        `json:"deletions"`
	CurrentRevision string                     `json:"current_revision"`
	Owner           gerritAccount              `json:"owner"`
	Reviewers       map[string][]gerritAccount `json:"reviewers"`
	Labels          map[string]json.RawMessage `json:"labels"`
	Revisions       map[string]gerritRevision  `json:"revisions"`
}

type gerritRevision struct {
	Number  int    `json:"_number"`
	Branch  string `json:"branch"`
	Created string `json:"created"`
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Commit  struct {
		Parents []struct {
			Commit  string `json:"commit"`
			Subject string `json:"subject"`
		} `json:"parents"`
		Author struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Subject string `json:"subject"`
		Message string `json:"message"`
	} `json:"commit"`
	Uploader gerritAccount `json:"uploader"`
}

type cleanAccount struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

func toCleanAccount(a gerritAccount) cleanAccount {
	name := a.DisplayName
	if name == "" {
		name = a.Name
	}
	return cleanAccount{Name: name, Email: a.Email}
}

func cleanupDetail(raw []byte) any {
	var d gerritDetail
	if err := json.Unmarshal(raw, &d); err != nil {
		return json.RawMessage(raw)
	}

	type cleanRevision struct {
		Number   int          `json:"number"`
		Kind     string       `json:"kind,omitempty"`
		Ref      string       `json:"ref"`
		Uploader cleanAccount `json:"uploader"`
		Commit   struct {
			Parents []struct {
				Commit  string `json:"commit"`
				Subject string `json:"subject"`
			} `json:"parents"`
			Author struct {
				Name  string `json:"name"`
				Email string `json:"email"`
				Date  string `json:"date"`
			} `json:"author"`
			Subject string `json:"subject"`
			Message string `json:"message"`
		} `json:"commit"`
	}
	revisions := make(map[string]cleanRevision, len(d.Revisions))
	for hash, rev := range d.Revisions {
		cr := cleanRevision{
			Number:   rev.Number,
			Kind:     rev.Kind,
			Ref:      rev.Ref,
			Uploader: toCleanAccount(rev.Uploader),
		}
		cr.Commit = rev.Commit
		revisions[hash] = cr
	}

	reviewers := make(map[string][]cleanAccount, len(d.Reviewers))
	for role, accts := range d.Reviewers {
		clean := make([]cleanAccount, 0, len(accts))
		for _, a := range accts {
			clean = append(clean, toCleanAccount(a))
		}
		reviewers[role] = clean
	}

	type cleanLabel struct {
		Approved    *cleanAccount `json:"approved,omitempty"`
		Rejected    *cleanAccount `json:"rejected,omitempty"`
		Value       *int          `json:"value,omitempty"`
		Description string        `json:"description,omitempty"`
	}
	labels := make(map[string]cleanLabel, len(d.Labels))
	for name, raw := range d.Labels {
		var full struct {
			Approved    *gerritAccount `json:"approved"`
			Rejected    *gerritAccount `json:"rejected"`
			Value       *int           `json:"value"`
			Description string         `json:"description"`
		}
		if err := json.Unmarshal(raw, &full); err != nil {
			continue
		}
		cl := cleanLabel{Value: full.Value, Description: full.Description}
		if full.Approved != nil {
			ca := toCleanAccount(*full.Approved)
			cl.Approved = &ca
		}
		if full.Rejected != nil {
			cr := toCleanAccount(*full.Rejected)
			cl.Rejected = &cr
		}
		labels[name] = cl
	}

	return struct {
		Subject         string                    `json:"subject"`
		Status          string                    `json:"status"`
		Project         string                    `json:"project"`
		Branch          string                    `json:"branch"`
		Created         string                    `json:"created"`
		Updated         string                    `json:"updated"`
		Insertions      int                       `json:"insertions"`
		Deletions       int                       `json:"deletions"`
		CurrentRevision string                    `json:"current_revision"`
		Owner           cleanAccount              `json:"owner"`
		Reviewers       map[string][]cleanAccount `json:"reviewers"`
		Labels          map[string]cleanLabel     `json:"labels"`
		Revisions       map[string]cleanRevision  `json:"revisions"`
	}{
		Subject:         d.Subject,
		Status:          d.Status,
		Project:         d.Project,
		Branch:          d.Branch,
		Created:         d.Created,
		Updated:         d.Updated,
		Insertions:      d.Insertions,
		Deletions:       d.Deletions,
		CurrentRevision: d.CurrentRevision,
		Owner:           toCleanAccount(d.Owner),
		Reviewers:       reviewers,
		Labels:          labels,
		Revisions:       revisions,
	}
}

type gerritComment struct {
	Author          gerritAccount `json:"author"`
	PatchSet        int           `json:"patch_set"`
	Message         string        `json:"message"`
	Updated         string        `json:"updated"`
	Unresolved      bool          `json:"unresolved"`
	ID              string        `json:"id"`
	CommitID        string        `json:"commit_id"`
	ChangeMessageID string        `json:"change_message_id"`
}

func cleanupComments(raw []byte) any {
	var byFile map[string][]gerritComment
	if err := json.Unmarshal(raw, &byFile); err != nil {
		return json.RawMessage(raw)
	}

	type cleanComment struct {
		Author     cleanAccount `json:"author"`
		PatchSet   int          `json:"patch_set"`
		Message    string       `json:"message"`
		Updated    string       `json:"updated"`
		Unresolved bool         `json:"unresolved"`
	}

	out := make(map[string][]cleanComment, len(byFile))
	for file, comments := range byFile {
		clean := make([]cleanComment, 0, len(comments))
		for _, c := range comments {
			clean = append(clean, cleanComment{
				Author:     toCleanAccount(c.Author),
				PatchSet:   c.PatchSet,
				Message:    c.Message,
				Updated:    c.Updated,
				Unresolved: c.Unresolved,
			})
		}
		out[file] = clean
	}
	return out
}

func gerritResolveArg(arg string) (string, error) {
	if _, err := strconv.Atoi(arg); err == nil {
		return arg, nil
	}
	if len(arg) >= 8 && isHex(arg) {
		return clResolveHash(arg)
	}
	return "", fmt.Errorf("invalid CL number or commit hash: %s", arg)
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func clResolveHash(hash string) (string, error) {
	u, err := neturl.JoinPath(gerritBase, "changes")
	if err != nil {
		return "", fmt.Errorf("resolving hash %s: %w", hash, err)
	}
	body, err := gerritJSON(u + "?q=commit:" + hash)
	if err != nil {
		return "", fmt.Errorf("resolving hash %s: %w", hash, err)
	}
	var results []struct {
		Number int `json:"_number"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return "", fmt.Errorf("parsing hash query: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no CL found for commit %s", hash)
	}
	return strconv.Itoa(results[0].Number), nil
}

func gerritEnsure(dir, num string) (retErr error) {
	numDir := filepath.Join(dir, num)
	if info, err := os.Stat(filepath.Join(numDir, "detail.json")); err == nil {
		if time.Since(info.ModTime()) < 30*time.Minute {
			return nil
		}
		os.RemoveAll(numDir) // ignore error
	}
	if err := os.MkdirAll(numDir, 0o755); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(numDir) // ignore error
		}
	}()
	detailURL, err := neturl.JoinPath(gerritBase, "changes", num, "detail")
	if err != nil {
		return err
	}
	detail, err := gerritJSON(detailURL + "?o=ALL_REVISIONS&o=ALL_COMMITS")
	if err != nil {
		return fmt.Errorf("fetching detail: %w", err)
	}
	if err := os.WriteFile(filepath.Join(numDir, "detail.json"), detail, 0o644); err != nil {
		return err
	}
	patchURL, err := neturl.JoinPath(gerritBase, "changes", num, "revisions", "current", "patch")
	if err != nil {
		return err
	}
	resp, err := httpClient.Get(patchURL)
	if err != nil {
		return fmt.Errorf("fetching patch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CL %s: HTTP %d", num, resp.StatusCode)
	}
	b64, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading patch: %w", err)
	}
	patch, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b64)))
	if err != nil {
		return fmt.Errorf("decoding patch: %w", err)
	}
	if err := os.WriteFile(filepath.Join(numDir, "patch.diff"), patch, 0o644); err != nil {
		return err
	}
	cmtURL, err := neturl.JoinPath(gerritBase, "changes", num, "comments")
	if err != nil {
		return err
	}
	cmt, err := gerritJSON(cmtURL)
	if err != nil {
		return fmt.Errorf("fetching comments: %w", err)
	}
	if err := os.WriteFile(filepath.Join(numDir, "comments.json"), cmt, 0o644); err != nil {
		return err
	}
	return nil
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func clExtractIssueNums(detail []byte) []int {
	var d struct {
		Revisions map[string]struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(detail, &d); err != nil {
		return nil
	}
	seen := map[int]bool{}
	var nums []int
	for _, rev := range d.Revisions {
		for line := range strings.SplitSeq(rev.Commit.Message, "\n") {
			line = strings.TrimSpace(line)
			if _, after, ok := strings.Cut(line, "go.dev/issue/"); ok {
				n := parseLeadingInt(after)
				if n > 0 && !seen[n] {
					seen[n] = true
					nums = append(nums, n)
				}
			}
			if _, after, ok := strings.Cut(line, "golang/go#"); ok {
				n := parseLeadingInt(after)
				if n > 0 && !seen[n] {
					seen[n] = true
					nums = append(nums, n)
				}
			} else if _, after, ok := strings.Cut(line, "#"); ok {
				n := parseLeadingInt(after)
				if n > 0 && !seen[n] {
					seen[n] = true
					nums = append(nums, n)
				}
			}
		}
	}
	return nums
}

func parseLeadingInt(s string) int {
	end := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if end == 0 {
		return 0
	}
	if end < 0 {
		end = len(s)
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

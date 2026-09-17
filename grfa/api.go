package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const magicPrefix = ")]}'\n"

func request(ctx context.Context, host, method string, parts []string, query url.Values, body any) ([]byte, error) {
	var u strings.Builder
	u.WriteString("http://" + net.JoinHostPort(host, "9001") + "/gerrit/a")
	for _, p := range parts {
		u.WriteString("/" + url.PathEscape(p))
	}
	if len(query) > 0 {
		u.WriteString("?" + query.Encode())
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "patel.codes/grfa")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return bytes.TrimPrefix(data, []byte(magicPrefix)), nil
}

func fetchChangeDetail(ctx context.Context, host, change string) (*changeDetail, error) {
	q := url.Values{}
	for _, o := range []string{"ALL_REVISIONS", "DETAILED_LABELS", "MESSAGES", "DETAILED_ACCOUNTS"} {
		q.Add("o", o)
	}
	data, err := request(ctx, host, http.MethodGet, []string{"changes", change, "detail"}, q, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch change detail: %w", err)
	}
	var d changeDetail
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("fetch change detail: decode: %w", err)
	}
	return &d, nil
}

func fetchChangeComments(ctx context.Context, host, change string) (map[string][]commentInfo, error) {
	data, err := request(ctx, host, http.MethodGet, []string{"changes", change, "comments"}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch comments: %w", err)
	}
	var m map[string][]commentInfo
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("fetch comments: decode: %w", err)
	}
	if m == nil {
		m = map[string][]commentInfo{}
	}
	return m, nil
}

func postReview(ctx context.Context, host, change, revision string, in *reviewInput) error {
	if _, err := request(ctx, host, http.MethodPost, []string{"changes", change, "revisions", revision, "review"}, nil, in); err != nil {
		return fmt.Errorf("post review: %w", err)
	}
	return nil
}

func queryCurrentRevisions(ctx context.Context, host, changeID string) ([]string, error) {
	q := url.Values{}
	q.Set("q", "change:"+changeID)
	q.Add("o", "CURRENT_REVISION")
	data, err := request(ctx, host, http.MethodGet, []string{"changes"}, q, nil)
	if err != nil {
		return nil, fmt.Errorf("query change: %w", err)
	}
	var list []changeDetail
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("query change: decode: %w", err)
	}
	revs := make([]string, 0, len(list))
	for _, d := range list {
		revs = append(revs, d.CurrentRevision)
	}
	return revs, nil
}

type accountInfo struct {
	ID       *int   `json:"_account_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

type commentRange struct {
	StartLine int `json:"start_line"`
	StartChar int `json:"start_character"`
	EndLine   int `json:"end_line"`
	EndChar   int `json:"end_character"`
}

type commentInfo struct {
	ID         string        `json:"id"`
	PatchSet   int           `json:"patch_set"`
	CommitID   string        `json:"commit_id"`
	InReplyTo  string        `json:"in_reply_to,omitempty"`
	Message    string        `json:"message"`
	Unresolved bool          `json:"unresolved"`
	Author     *accountInfo  `json:"author,omitempty"`
	Side       string        `json:"side,omitempty"`
	Parent     int           `json:"parent,omitempty"`
	Line       int           `json:"line,omitempty"`
	Range      *commentRange `json:"range,omitempty"`
	Updated    string        `json:"updated,omitempty"`
}

type commentInput struct {
	Message    string        `json:"message"`
	InReplyTo  string        `json:"in_reply_to,omitempty"`
	Unresolved bool          `json:"unresolved"`
	Side       string        `json:"side,omitempty"`
	Parent     int           `json:"parent,omitempty"`
	Line       int           `json:"line,omitempty"`
	Range      *commentRange `json:"range,omitempty"`
}

type reviewInput struct {
	Comments map[string][]commentInput `json:"comments"`
}

type revisionInfo struct {
	Number int `json:"_number"`
}

type labelInfo struct {
	Value        int               `json:"value"`
	DefaultValue int               `json:"default_value,omitempty"`
	Values       map[string]string `json:"values,omitempty"`
	All          []voteInfo        `json:"all,omitempty"`
}

type voteInfo struct {
	Value          int          `json:"value"`
	Date           string       `json:"date,omitempty"`
	PermittedRange *votingRange `json:"permitted_voting_range,omitempty"`
	accountInfo
}

type votingRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type changeMessage struct {
	ID       string       `json:"id"`
	Author   *accountInfo `json:"author,omitempty"`
	Date     string       `json:"date,omitempty"`
	Revision int          `json:"_revision_number,omitempty"`
	Message  string       `json:"message"`
}

type changeDetail struct {
	ID                     string                  `json:"id"`
	Project                string                  `json:"project,omitempty"`
	Branch                 string                  `json:"branch,omitempty"`
	ChangeID               string                  `json:"change_id,omitempty"`
	Number                 int                     `json:"_number"`
	Status                 string                  `json:"status"`
	Subject                string                  `json:"subject"`
	Created                string                  `json:"created,omitempty"`
	Updated                string                  `json:"updated,omitempty"`
	Insertions             int                     `json:"insertions,omitempty"`
	Deletions              int                     `json:"deletions,omitempty"`
	CurrentRevision        string                  `json:"current_revision"`
	Revisions              map[string]revisionInfo `json:"revisions"`
	Labels                 map[string]labelInfo    `json:"labels"`
	Messages               []changeMessage         `json:"messages"`
	UnresolvedCommentCount *int                    `json:"unresolved_comment_count"`
}

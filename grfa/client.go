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
	"os"
	"strings"
	"time"
)

// magicPrefix is the XSS-protection prefix Gerrit puts in front
// of every JSON response body.
const magicPrefix = ")]}'\n"

// selectEndpoint picks the Gerrit entrance from the environment
// hints. The port and expected identity follow the entrance hint:
// agent traffic enters on 9001 as Agent, everything else on 9999
// as Human. These are routing hints, not authorization
// boundaries; the server enforces authority, and a denied route
// is an error, never a fallback.
func selectEndpoint(host, yah string) (base string, expectedUser string) {
	if host == "" {
		host = "supermarket"
	}
	port, user := "9999", "Human"
	if yah == "1" {
		port, user = "9001", "Agent"
	}
	base = (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   "/gerrit/a",
	}).String()
	return base, user
}

// newClient builds the production client from the environment.
func newClient() *gerritClient {
	base, expectedUser := selectEndpoint(os.Getenv("GRFA_HOST"), os.Getenv("YAH"))
	return newGerritClient(base, expectedUser, nil)
}

// newGerritClient builds a client for one authenticated entrance.
// A nil transport uses the default. Redirects are always refused so
// a request can never silently change entrance, and no
// credential is ever attached.
func newGerritClient(base, expectedUser string, rt http.RoundTripper) *gerritClient {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &gerritClient{
		base:         strings.TrimRight(base, "/"),
		expectedUser: expectedUser,
		hc: &http.Client{
			Transport: rt,
			Timeout:   30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("refused redirect to %s", req.URL)
			},
		},
	}
}

// gerritClient talks to exactly one authenticated /gerrit/a
// entrance.
type gerritClient struct {
	base         string
	expectedUser string
	hc           *http.Client
}

// request performs one HTTP round trip and returns the raw body
// and status. Path components are escaped individually rather
// than concatenated from user input. Accepted statuses are
// checked explicitly by the callers.
func (c *gerritClient) request(ctx context.Context, method string, parts []string, query url.Values, body any) ([]byte, int, error) {
	u := c.base
	for _, p := range parts {
		u += "/" + url.PathEscape(p)
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "patel.codes/grfa")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return data, resp.StatusCode, nil
}

// checkIdentity verifies GET /accounts/self reports the identity
// selected for this entrance. It runs before any mutation so a
// wrong identity prevents the POST.
func (c *gerritClient) checkIdentity(ctx context.Context) error {
	data, status, err := c.request(ctx, http.MethodGet, []string{"accounts", "self"}, nil, nil)
	if err != nil {
		return fmt.Errorf("check identity: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("check identity: %w", statusError(status, data))
	}
	var acct struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := decodeGerritJSON(data, &acct); err != nil {
		return fmt.Errorf("check identity: decode: %w", err)
	}
	if acct.Username != c.expectedUser && acct.Name != c.expectedUser {
		return fmt.Errorf("check identity: entrance %s reports %q, expected %q",
			c.base, firstNonEmpty(acct.Username, acct.Name), c.expectedUser)
	}
	return nil
}

// fetchChangeDetail fetches the change with every revision,
// labels, votes, and published change messages.
func (c *gerritClient) fetchChangeDetail(ctx context.Context, change string) (*changeDetail, error) {
	q := url.Values{}
	for _, o := range []string{"ALL_REVISIONS", "DETAILED_LABELS", "MESSAGES", "DETAILED_ACCOUNTS"} {
		q.Add("o", o)
	}
	data, status, err := c.request(ctx, http.MethodGet, []string{"changes", change, "detail"}, q, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch change detail: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fetch change detail: %w", statusError(status, data))
	}
	var d changeDetail
	if err := decodeGerritJSON(data, &d); err != nil {
		return nil, fmt.Errorf("fetch change detail: decode: %w", err)
	}
	return &d, nil
}

// fetchChangeComments fetches the change-level comment list: the
// only list that includes patch_set and commit_id for every
// published comment, across all patch sets, and the same list
// -reply resolves against.
func (c *gerritClient) fetchChangeComments(ctx context.Context, change string) (map[string][]commentInfo, error) {
	data, status, err := c.request(ctx, http.MethodGet, []string{"changes", change, "comments"}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch comments: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fetch comments: %w", statusError(status, data))
	}
	var m map[string][]commentInfo
	if err := decodeGerritJSON(data, &m); err != nil {
		return nil, fmt.Errorf("fetch comments: decode: %w", err)
	}
	if m == nil {
		m = map[string][]commentInfo{}
	}
	return m, nil
}

// postReview posts one review to one specific revision. It is the
// only mutation grfa performs.
func (c *gerritClient) postReview(ctx context.Context, change, revision string, in *reviewInput) error {
	data, status, err := c.request(ctx, http.MethodPost, []string{"changes", change, "revisions", revision, "review"}, nil, in)
	if err != nil {
		return fmt.Errorf("post review: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("post review: %w", statusError(status, data))
	}
	return nil
}

// decodeGerritJSON strips Gerrit's magic prefix before decoding.
func decodeGerritJSON(data []byte, dst any) error {
	return json.Unmarshal(bytes.TrimPrefix(data, []byte(magicPrefix)), dst)
}

// statusError reports an unexpected HTTP status with a bounded
// slice of the body, without inventing a permission diagnosis.
func statusError(status int, body []byte) error {
	return fmt.Errorf("status %d: %s", status, truncate(string(bytes.TrimSpace(body)), 4<<10))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// accountInfo is Gerrit's AccountInfo.
type accountInfo struct {
	ID       *int   `json:"_account_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

// commentRange is Gerrit's Comment.Range.
type commentRange struct {
	StartLine int `json:"start_line"`
	StartChar int `json:"start_character"`
	EndLine   int `json:"end_line"`
	EndChar   int `json:"end_character"`
}

// commentInfo is a published comment as returned by the
// change-level /comments endpoint.
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

// commentInput is the wire form of one comment in a review.
// Unresolved is always sent explicitly: Gerrit inherits a reply's
// resolution from its parent comment when the field is omitted.
type commentInput struct {
	Message    string        `json:"message"`
	InReplyTo  string        `json:"in_reply_to,omitempty"`
	Unresolved bool          `json:"unresolved"`
	Side       string        `json:"side,omitempty"`
	Parent     int           `json:"parent,omitempty"`
	Line       int           `json:"line,omitempty"`
	Range      *commentRange `json:"range,omitempty"`
}

// reviewInput carries only comments: no change message and no
// votes are ever sent.
type reviewInput struct {
	Comments map[string][]commentInput `json:"comments"`
}

// revisionInfo is one revision entry in ChangeInfo.revisions.
type revisionInfo struct {
	Number int `json:"_number"`
}

// labelInfo is Gerrit's LabelInfo with its votes.
type labelInfo struct {
	Value int        `json:"value"`
	All   []voteInfo `json:"all,omitempty"`
}

// voteInfo is one vote on a label; the account rides in the
// _account_id key as an AccountInfo object.
type voteInfo struct {
	Value   int          `json:"value"`
	Account *accountInfo `json:"_account_id,omitempty"`
}

// changeMessage is one published change message.
type changeMessage struct {
	ID       string       `json:"id"`
	Author   *accountInfo `json:"author,omitempty"`
	Revision int          `json:"_revision_number,omitempty"`
	Message  string       `json:"message"`
}

// changeDetail is the ChangeInfo /detail response.
type changeDetail struct {
	ID                     string                  `json:"id"`
	Number                 int                     `json:"_number"`
	Status                 string                  `json:"status"`
	Subject                string                  `json:"subject"`
	CurrentRevision        string                  `json:"current_revision"`
	Revisions              map[string]revisionInfo `json:"revisions"`
	Labels                 map[string]labelInfo    `json:"labels"`
	Messages               []changeMessage         `json:"messages"`
	UnresolvedCommentCount *int                    `json:"unresolved_comment_count"`
}

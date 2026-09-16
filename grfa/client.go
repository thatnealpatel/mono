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

// magicPrefix is the XSS-protection
// prefix Gerrit puts in front of every
// JSON response body.
const magicPrefix = ")]}'\n"

// selectEndpoint builds the authenticated Agent entrance for a host.
// The port is fixed at 9001, the least-privileged entrance and the only
// one an agent host can reach, and the expected identity is Agent: no
// environment hint selects the entrance, so a bare environment is enough.
// GRFA_HOST is a routing hint for the host, not an authorization boundary;
// the server enforces authority, and a denied route is an error, never a
// fallback.
func selectEndpoint(host string) (base string, expectedUser string) {
	if host == "" {
		host = "supermarket"
	}
	base = (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, "9001"),
		Path:   "/gerrit/a",
	}).String()
	return base, "Agent"
}

// newClient builds the production client from the environment over the
// given transport; a nil transport uses the default.
func newClient(rt http.RoundTripper) *gerritClient {
	base, expectedUser := selectEndpoint(os.Getenv("GRFA_HOST"))
	return newGerritClient(base, expectedUser, rt)
}

// newGerritClient builds a client for one authenticated entrance. A nil
// transport uses the default. Redirects are always refused so a request can
// never silently change entrance, and no credential is ever attached.
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

// gerritClient talks to exactly one
// authenticated /gerrit/a entrance.
type gerritClient struct {
	base         string
	expectedUser string
	hc           *http.Client
}

// request performs one HTTP round trip and returns the raw body and status.
// Path components are escaped individually rather than concatenated from user
// input. Accepted statuses are checked explicitly by the callers.
func (c *gerritClient) request(ctx context.Context, method string, parts []string, query url.Values, body any) ([]byte, int, error) {
	var u strings.Builder
	u.WriteString(c.base)
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
			return nil, 0, fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
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
	var acct accountInfo
	if err := decodeGerritJSON(data, &acct); err != nil {
		return fmt.Errorf("check identity: decode: %w", err)
	}
	if acct.Username != c.expectedUser && acct.Name != c.expectedUser {
		return fmt.Errorf("check identity: entrance %s reports %q, expected %q",
			c.base, firstNonEmpty(acct.Username, acct.Name), c.expectedUser)
	}
	return nil
}

// fetchChangeDetail fetches the change with every revision, labels, votes, and
// published change messages.
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

// fetchChangeComments fetches the change-level comment list: the only list
// that includes patch_set and commit_id for every published comment, across
// all patch sets, and the same list -reply resolves against.
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

// postReview posts one review to one specific revision. It is the only
// mutation grfa performs.
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

// queryCurrentRevisions looks a change up by its Change-Id through the
// authenticated entrance and returns the current revisions of the matching
// changes, in the server's order. The query is read-only and list-form;
// an empty result means no such change exists. It is how an upload
// recognizes a push that was rejected because every revision was already
// on the server as its current patch set, without ever parsing the
// delegated push's output.
func (c *gerritClient) queryCurrentRevisions(ctx context.Context, changeID string) ([]string, error) {
	q := url.Values{}
	q.Set("q", "change:"+changeID)
	q.Add("o", "CURRENT_REVISION")
	data, status, err := c.request(ctx, http.MethodGet, []string{"changes"}, q, nil)
	if err != nil {
		return nil, fmt.Errorf("query change: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("query change: %w", statusError(status, data))
	}
	var list []changeDetail
	if err := decodeGerritJSON(data, &list); err != nil {
		return nil, fmt.Errorf("query change: decode: %w", err)
	}
	revs := make([]string, 0, len(list))
	for _, d := range list {
		revs = append(revs, d.CurrentRevision)
	}
	return revs, nil
}

// decodeGerritJSON strips Gerrit's magic prefix
// before decoding.
func decodeGerritJSON(data []byte, dst any) error {
	return json.Unmarshal(bytes.TrimPrefix(data, []byte(magicPrefix)), dst)
}

// statusError reports an unexpected HTTP status
// with a bounded slice of the body, without
// inventing a permission diagnosis.
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

// commentInfo is a published comment as
// returned by the change-level /comments
// endpoint.
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

// commentInput is the wire form of one
// comment in a review. Unresolved is
// always sent explicitly: Gerrit inherits
// a reply's resolution from its parent
// comment when the field is omitted.
type commentInput struct {
	Message    string        `json:"message"`
	InReplyTo  string        `json:"in_reply_to,omitempty"`
	Unresolved bool          `json:"unresolved"`
	Side       string        `json:"side,omitempty"`
	Parent     int           `json:"parent,omitempty"`
	Line       int           `json:"line,omitempty"`
	Range      *commentRange `json:"range,omitempty"`
}

// reviewInput carries only comments: no
// change message and no votes are ever
// sent.
type reviewInput struct {
	Comments map[string][]commentInput `json:"comments"`
}

// revisionInfo is one revision entry in
// ChangeInfo.revisions.
type revisionInfo struct {
	Kind     string       `json:"kind,omitempty"`
	Number   int          `json:"_number"`
	Created  string       `json:"created,omitempty"`
	Uploader *accountInfo `json:"uploader,omitempty"`
	Ref      string       `json:"ref,omitempty"`
}

// labelInfo is Gerrit's LabelInfo with its votes.
type labelInfo struct {
	Value        int               `json:"value"`
	DefaultValue int               `json:"default_value,omitempty"`
	Values       map[string]string `json:"values,omitempty"`
	All          []voteInfo        `json:"all,omitempty"`
}

// voteInfo is one entry of a label's all list. Gerrit flattens the voter's
// AccountInfo into the vote object — _account_id is a number, with name and
// username as siblings — alongside the vote's own fields: value, date, and
// the permitted_voting_range the querying account may cast.
type voteInfo struct {
	Value          int          `json:"value"`
	Date           string       `json:"date,omitempty"`
	PermittedRange *votingRange `json:"permitted_voting_range,omitempty"`
	accountInfo
}

// votingRange is Gerrit's VotingRangeInfo.
type votingRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// changeMessage is one published change message.
type changeMessage struct {
	ID       string       `json:"id"`
	Author   *accountInfo `json:"author,omitempty"`
	Date     string       `json:"date,omitempty"`
	Revision int          `json:"_revision_number,omitempty"`
	Message  string       `json:"message"`
}

// changeDetail is the ChangeInfo /detail response.
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

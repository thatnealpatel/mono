package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recordedPost is one review POST the
// fake Gerrit received, with both the
// decoded body and the raw wire bytes so
// tests can assert on exact JSON field
// presence and omission.
type recordedPost struct {
	Change   string
	Revision string
	Body     reviewInput
	Raw      []byte
}

// fakeGerrit is a minimal in-memory
// Gerrit behind httptest. It serves
// the routes grfa uses, records every
// request, and applies posted reviews
// to its own state so refreshed views
// see them.
type fakeGerrit struct {
	mu       sync.Mutex
	user     string
	details  map[string]*changeDetail
	comments map[string]map[string][]commentInfo
	// onServer records which changes the change-query endpoint can
	// see: a change that was never pushed is not on the server.
	onServer map[string]bool
	// raw serves verbatim payloads keyed by "METHOD path",
	// mirroring the exact bytes a real Gerrit produced.
	raw      map[string]string
	posts    []recordedPost
	requests []string
	statuses map[string]int
	nextID   int
	authSeen bool
}

func newFakeGerrit(t *testing.T, user string) (*fakeGerrit, *httptest.Server) {
	t.Helper()
	f := &fakeGerrit{
		user:     user,
		details:  map[string]*changeDetail{},
		comments: map[string]map[string][]commentInfo{},
		raw:      map[string]string{},
		statuses: map[string]int{},
		onServer: map[string]bool{},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeGerrit) writeJSON(w http.ResponseWriter, v any) {
	w.Write([]byte(magicPrefix))
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

func (f *fakeGerrit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		rec += "?" + r.URL.RawQuery
	}
	f.requests = append(f.requests, rec)
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		f.authSeen = true
	}
	w.Header().Set("Content-Type", "application/json")
	if status, ok := f.statuses[r.Method+" "+r.URL.Path]; ok {
		if status/100 == 3 {
			// A different entrance the client must never reach.
			w.Header().Set("Location", "/gerrit/other-entrance")
		}
		w.WriteHeader(status)
		return
	}
	if body, ok := f.raw[r.Method+" "+r.URL.Path]; ok {
		// A verbatim payload, exactly as the real server sent it.
		w.Write([]byte(body))
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/accounts/self":
		f.writeJSON(w, f.selfAccount())
	case r.Method == http.MethodGet && len(parts) == 1 && parts[0] == "changes" && r.URL.RawQuery != "":
		// The change-query endpoint, GET /changes/?q=<term>&o=<option>.
		f.serveChangeQuery(w, r)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "changes" && parts[2] == "detail":
		d, ok := f.details[parts[1]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			f.writeJSON(w, map[string]string{"error": "not found"})
			return
		}
		f.writeJSON(w, d)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "changes" && parts[2] == "comments":
		m := f.comments[parts[1]]
		if m == nil {
			m = map[string][]commentInfo{}
		}
		f.writeJSON(w, m)
	case r.Method == http.MethodPost && len(parts) == 5 && parts[0] == "changes" && parts[2] == "revisions" && parts[4] == "review":
		f.applyReview(w, parts[1], parts[3], r)
	default:
		w.WriteHeader(http.StatusNotFound)
		f.writeJSON(w, map[string]string{"error": "no such route"})
	}
}

// selfAccount is the fake's authenticated account, mirroring the real
// AccountInfo shape: a numeric _account_id with name and username.
func (f *fakeGerrit) selfAccount() *accountInfo {
	return &accountInfo{ID: accountID(1000000), Name: f.user, Username: f.user}
}

// serveChangeQuery mirrors the real change-query endpoint,
// GET /changes/?q=change:<Change-Id>&o=CURRENT_REVISION, whose response is
// a list of ChangeInfo entries. Only the change:<Change-Id> term is
// supported, and the CURRENT_REVISION option is required, because that is
// the only query grfa issues.
func (f *fakeGerrit) serveChangeQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, ok := strings.CutPrefix(q.Get("q"), "change:")
	if !ok || id == "" {
		w.WriteHeader(http.StatusNotFound)
		f.writeJSON(w, map[string]string{"error": "unsupported query term"})
		return
	}
	hasOption := false
	for _, o := range q["o"] {
		if o == "CURRENT_REVISION" {
			hasOption = true
		}
	}
	if !hasOption {
		w.WriteHeader(http.StatusBadRequest)
		f.writeJSON(w, map[string]string{"error": "query requires o=CURRENT_REVISION"})
		return
	}
	matches := []*changeDetail{}
	for key, d := range f.details {
		if !f.onServer[key] {
			// A change no push ever landed is not
			// on the server, so no query can see it.
			continue
		}
		if d.ChangeID == id || d.ID == id || strings.HasSuffix(d.ID, "~"+id) {
			matches = append(matches, d)
		}
	}
	f.writeJSON(w, matches)
}

// land puts the given revisions on the fake server as their changes'
// current patch sets: from then on the change-query endpoint reports
// them. It models the moment a push lands — or a concurrent upload
// lands the same content between grfa's lookup and its own push.
func (f *fakeGerrit) land(revs []revision) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, d := range f.details {
		for _, r := range revs {
			if d.ChangeID == r.ChangeID || d.ID == r.ChangeID {
				d.CurrentRevision = r.Commit
				f.onServer[key] = true
			}
		}
	}
}

// applyReview records the post and folds it into the fake's change state, the
// way a real Gerrit would publish it.
func (f *fakeGerrit) applyReview(w http.ResponseWriter, change, revision string, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var in reviewInput
	if err := json.Unmarshal(raw, &in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.posts = append(f.posts, recordedPost{Change: change, Revision: revision, Body: in, Raw: raw})
	detail := f.details[change]
	if f.comments[change] == nil {
		f.comments[change] = map[string][]commentInfo{}
	}
	for path, list := range in.Comments {
		for _, ci := range list {
			f.nextID++
			ps := 0
			if detail != nil {
				if rev, ok := detail.Revisions[revision]; ok {
					ps = rev.Number
				}
			}
			f.comments[change][path] = append(f.comments[change][path], commentInfo{
				ID:         fmt.Sprintf("fake-%d", f.nextID),
				PatchSet:   ps,
				CommitID:   revision,
				InReplyTo:  ci.InReplyTo,
				Message:    ci.Message,
				Unresolved: ci.Unresolved,
				Side:       ci.Side,
				Parent:     ci.Parent,
				Line:       ci.Line,
				Range:      ci.Range,
				Author:     f.selfAccount(),
			})
		}
	}
	f.writeJSON(w, map[string]any{"labels": map[string]any{}, "ready": true})
}

// snapshot returns copies of the recorded posts and requests.
func (f *fakeGerrit) snapshot() (posts []recordedPost, requests []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedPost{}, f.posts...), append([]string{}, f.requests...)
}

func (f *fakeGerrit) postCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.posts)
}

func (f *fakeGerrit) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// selfRequestCount reports how many GET /accounts/self requests the
// fake has served: the identity check is the only endpoint used by it.
func (f *fakeGerrit) selfRequestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, "GET /accounts/self") {
			n++
		}
	}
	return n
}

func (f *fakeGerrit) sawCredentials() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authSeen
}

// testClient wires a grfa client at the fake server with the given
// expected entrance identity.
func testClient(srv *httptest.Server, expectedUser string) *gerritClient {
	return newGerritClient(srv.URL, expectedUser, nil)
}

// fakeRunner is the injectable command
// runner: it records every command and
// answers from a programmed responder.
type fakeRunner struct {
	mu       sync.Mutex
	commands []command
	respond  func(cmd command) (string, error)
}

func (f *fakeRunner) record(cmd command) {
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
}

func (f *fakeRunner) output(_ context.Context, cmd command) (string, error) {
	f.record(cmd)
	return f.respond(cmd)
}

func (f *fakeRunner) run(_ context.Context, cmd command) error {
	f.record(cmd)
	_, err := f.respond(cmd)
	return err
}

func (f *fakeRunner) snapshot() []command {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]command{}, f.commands...)
}

// refusingRunner rejects any subprocess;
// change-scoped commands must never
// spawn one.
func refusingRunner() *fakeRunner {
	return &fakeRunner{respond: func(cmd command) (string, error) {
		return "", fmt.Errorf("unexpected subprocess %s %v", cmd.name, cmd.args)
	}}
}

// jjFake answers the read-only jj queries
// and the push around an upload, plus the
// optional pre-upload hook.
type jjFake struct {
	mu         sync.Mutex
	root       string
	defaultRev string // answer to the description probe: "@" or "@-"
	setRevs    []revision
	pushErr    error
	pushStdout string // stdout the push emits; grfa must never parse it
	hookErr    error
	// onPush, when set, runs when the push command is attempted;
	// tests use it to script what lands on the server at that
	// moment.
	onPush   func()
	commands []command
}

func (j *jjFake) runner() runner {
	return &fakeRunner{respond: j.respond}
}

// jjBody strips the read-only global
// flags from a recorded jj command and
// joins the remainder for matching.
func jjBody(cmd command) string {
	rest := cmd.args
	for len(rest) > 0 && (rest[0] == "--ignore-working-copy" || rest[0] == "--no-pager") {
		rest = rest[1:]
	}
	return strings.Join(rest, " ")
}

func (j *jjFake) respond(cmd command) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.commands = append(j.commands, cmd)
	if cmd.name != jjBinary {
		// The only non-jj subprocess grfa runs
		// is the pre-upload hook.
		return "", j.hookErr
	}
	joined := jjBody(cmd)
	switch {
	case strings.HasPrefix(joined, "root"):
		return j.root + "\n", nil
	case strings.Contains(joined, `if(description`):
		return j.defaultRev + "\n", nil
	case strings.HasPrefix(joined, "log --no-graph -r"):
		return uploadSetOutput(j.setRevs), nil
	case strings.HasPrefix(joined, "gerrit upload"):
		// The push is attempted: fire the scripted
		// landing before answering, so a test can put
		// content on the server between grfa's lookup
		// and this push.
		if j.onPush != nil {
			j.onPush()
		}
		// pushStdout is never parsed by grfa; it exists
		// so tests can prove exactly that.
		return j.pushStdout, j.pushErr
	}
	return "", fmt.Errorf("unexpected jj command: %s", joined)
}

func (j *jjFake) commandsSnapshot() []command {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]command{}, j.commands...)
}

// accountID is a helper for building the numeric
// _account_id of a real AccountInfo.
func accountID(n int) *int { return &n }

// uploadSetOutput renders the upload-set
// query answer in the same shape the real
// jj template produces.
func uploadSetOutput(revs []revision) string {
	var b strings.Builder
	for _, r := range revs {
		desc := r.Subject
		if desc != "" {
			desc += "\n\n"
		}
		if r.ChangeID != "" {
			desc += "Change-Id: " + r.ChangeID
		}
		b.WriteString(r.Commit + "\x1f" + desc + "\x1f")
	}
	return b.String()
}

// writeHook creates an executable pre-upload hook
// under a fake repository root.
func writeHook(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".grfa"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(root, ".grfa", "pre-upload")
	if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

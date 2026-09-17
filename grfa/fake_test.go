package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordedPost struct {
	Change   string
	Revision string
	Body     reviewInput
	Raw      []byte
}

type fakeGerrit struct {
	mu       sync.Mutex
	user     string
	details  map[string]*changeDetail
	comments map[string]map[string][]commentInfo

	onServer map[string]bool

	raw      map[string]string
	posts    []recordedPost
	requests []string
	statuses map[string]int
	nextID   int
	authSeen bool
}

func newFakeGerrit(t *testing.T, user string) (*fakeGerrit, string) {
	t.Helper()
	f := &fakeGerrit{
		user:     user,
		details:  map[string]*changeDetail{},
		comments: map[string]map[string][]commentInfo{},
		raw:      map[string]string{},
		statuses: map[string]int{},
		onServer: map[string]bool{},
	}
	srv := httptest.NewUnstartedServer(f)
	srv.Listener.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:9001")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return f, "127.0.0.1"
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

	path := strings.TrimPrefix(r.URL.Path, "/gerrit/a")
	rec := r.Method + " " + path
	if r.URL.RawQuery != "" {
		rec += "?" + r.URL.RawQuery
	}
	f.requests = append(f.requests, rec)
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		f.authSeen = true
	}
	w.Header().Set("Content-Type", "application/json")
	if status, ok := f.statuses[r.Method+" "+path]; ok {
		if status/100 == 3 {
			w.Header().Set("Location", "/gerrit/other-entrance")
		}
		w.WriteHeader(status)
		return
	}
	if body, ok := f.raw[r.Method+" "+path]; ok {
		w.Write([]byte(body))
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	switch {
	case r.Method == http.MethodGet && path == "/accounts/self":
		f.writeJSON(w, f.selfAccount())
	case r.Method == http.MethodGet && len(parts) == 1 && parts[0] == "changes" && r.URL.RawQuery != "":
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

func (f *fakeGerrit) selfAccount() *accountInfo {
	return &accountInfo{ID: accountID(1000000), Name: f.user, Username: f.user}
}

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
			continue
		}
		if d.ChangeID == id || d.ID == id || strings.HasSuffix(d.ID, "~"+id) {
			matches = append(matches, d)
		}
	}
	f.writeJSON(w, matches)
}

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

func (f *fakeGerrit) sawCredentials() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authSeen
}

func accountID(n int) *int { return &n }

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// unsetenv removes name from the environment for the duration of the
// test and restores it afterwards, so a case can run in a genuinely
// bare environment rather than a set-but-empty one.
func unsetenv(t *testing.T, name string) {
	t.Helper()
	v, ok := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !ok {
			return
		}
		if err := os.Setenv(name, v); err != nil {
			t.Fatal(err)
		}
	})
}

// Acceptance 1: endpoint selection is the
// Agent entrance for every host: the port
// is fixed at 9001 and the identity is
// Agent; no credentials or fallback route
// are used.
func TestSelectEndpoint(t *testing.T) {
	cases := []struct {
		host         string
		wantBase     string
		wantExpected string
	}{
		// A bare environment: no host override.
		{"", "http://supermarket:9001/gerrit/a", "Agent"},
		{"gerrit.corp", "http://gerrit.corp:9001/gerrit/a", "Agent"},
	}
	for _, tc := range cases {
		base, expected := selectEndpoint(tc.host)
		if base != tc.wantBase {
			t.Errorf("selectEndpoint(%q) base = %q, want %q", tc.host, base, tc.wantBase)
		}
		if expected != tc.wantExpected {
			t.Errorf("selectEndpoint(%q) user = %q, want %q", tc.host, expected, tc.wantExpected)
		}
	}
}

func TestNewClientFromEnv(t *testing.T) {
	cases := []struct {
		name     string
		host     string
		bareHost bool
		wantBase string
	}{
		{"bare environment", "", true, "http://supermarket:9001/gerrit/a"},
		{"explicit GRFA_HOST", "gerrit.internal", false, "http://gerrit.internal:9001/gerrit/a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.bareHost {
				unsetenv(t, "GRFA_HOST")
			} else {
				t.Setenv("GRFA_HOST", tc.host)
			}
			c := newClient(nil)
			if c.base != tc.wantBase {
				t.Errorf("base = %q, want %q", c.base, tc.wantBase)
			}
			if c.expectedUser != "Agent" {
				t.Errorf("expectedUser = %q, want Agent", c.expectedUser)
			}
			if c.hc.Timeout <= 0 {
				t.Errorf("client timeout must be bounded")
			}
			if c.hc.CheckRedirect == nil {
				t.Errorf("automatic redirects must be disabled")
			}
		})
	}
}

// recordingTransport records the full URL of every request it carries and
// serves it through an injected function, so a test can prove which
// entrance was actually used rather than compare a computed string.
type recordingTransport struct {
	mu    sync.Mutex
	urls  []string
	serve func(*http.Request) (*http.Response, error)
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.urls = append(rt.urls, req.URL.String())
	rt.mu.Unlock()
	return rt.serve(req)
}

func (rt *recordingTransport) snapshot() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string{}, rt.urls...)
}

// Acceptance 1: a bare environment reaches the Agent entrance. No hint,
// no session, and no host override is set, and a full mutation runs over
// the injected transport: every request actually travels to
// http://supermarket:9001/gerrit/a, and the identity asserted is Agent.
// The assertion is end to end, so a regression that consults a hint and
// lands on another entrance fails it.
func TestBareEnvironmentReachesAgentEntrance(t *testing.T) {
	for _, name := range []string{"YAH", "YAH_SESSION", "GRFA_HOST"} {
		unsetenv(t, name)
	}
	run := func(t *testing.T) {
		t.Helper()
		f, _ := newFakeGerrit(t, "Agent")
		d, comments := testFixture()
		f.details[changeKey] = d
		f.comments[changeKey] = comments
		rt := &recordingTransport{serve: func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			f.ServeHTTP(rec, req)
			resp := rec.Result()
			resp.Request = req
			return resp, nil
		}}
		c := &cli{
			out:    &bytes.Buffer{},
			api:    newClient(rt),
			runner: refusingRunner(),
			getenv: emptyGetenv,
		}
		// The identity check runs before the mutation, and the fake
		// reports Agent: any other expected identity aborts here, so
		// success proves the identity asserted is Agent.
		if err := c.cmdComment(context.Background(), changeKey, "hello", nil); err != nil {
			t.Fatalf("a bare environment must reach the Agent entrance: %v", err)
		}
		if f.postCount() != 1 {
			t.Errorf("posts = %d, want 1", f.postCount())
		}
		if got := f.selfRequestCount(); got != 1 {
			t.Errorf("accounts/self checks = %d, want 1 (identity asserted before the mutation)", got)
		}
		urls := rt.snapshot()
		if len(urls) == 0 {
			t.Fatal("no request was made over the injected transport")
		}
		for _, u := range urls {
			if !strings.HasPrefix(u, "http://supermarket:9001/gerrit/a/") {
				t.Errorf("request left the Agent entrance: %q", u)
			}
		}
	}
	t.Run("bare environment", run)
	t.Run("an entrance hint in the environment is ignored", func(t *testing.T) {
		// The old lookup keyed on this variable; a leftover value of it
		// must not select an entrance, so this pins its absence as a
		// routing input.
		t.Setenv("YAH", "yes")
		run(t)
	})
}

// Acceptance 1: a wrong accounts/self prevents the POST.
func TestWrongIdentityPreventsPost(t *testing.T) {
	f, c, _ := seededClient(t, "Imposter")
	// The client entered through the agent
	// entrance; the server reports some
	// other identity.
	c.api.expectedUser = "Agent"
	err := c.cmdComment(context.Background(), changeKey, "hello", nil)
	if err == nil {
		t.Fatal("wrong identity: want an error")
	}
	if !strings.Contains(err.Error(), "Agent") {
		t.Errorf("error should name the expected identity: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("posted %d reviews, want 0", f.postCount())
	}
	if f.requestCount() != 1 {
		t.Errorf("made %d requests, want 1 (accounts/self only)", f.requestCount())
	}
}

// Acceptance 1: no credentials are ever attached.
func TestNoCredentialsAttached(t *testing.T) {
	f, c, _ := seededClient(t, "Agent")
	if err := c.cmdComment(context.Background(), changeKey, "hello", nil); err != nil {
		t.Fatal(err)
	}
	if f.sawCredentials() {
		t.Errorf("client attached an Authorization header or cookie")
	}
}

// Acceptance 9: HTTP errors and redirect refusal do
// not trigger another entrance or a false success.
func TestHTTPErrorAndRedirectRefused(t *testing.T) {
	t.Run("server error on detail", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		f.mu.Lock()
		f.statuses["GET /changes/"+changeKey+"/detail"] = http.StatusInternalServerError
		f.mu.Unlock()
		err := c.cmdView(context.Background(), changeKey)
		if err == nil {
			t.Fatal("internal server error: want an error")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("error should surface the status: %v", err)
		}
		if f.postCount() != 0 {
			t.Errorf("no mutation may follow an error")
		}
	})
	t.Run("redirect on post is refused", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		f.mu.Lock()
		f.statuses["POST /changes/"+changeKey+"/revisions/"+ps2SHA+"/review"] = http.StatusFound
		f.mu.Unlock()
		err := c.cmdComment(context.Background(), changeKey, "hello", nil)
		if err == nil {
			t.Fatal("redirect: want an error, not a false success")
		}
		if !strings.Contains(err.Error(), "refused redirect") {
			t.Errorf("error should name the refusal: %v", err)
		}
		if f.postCount() != 0 {
			t.Errorf("posted %d reviews, want 0", f.postCount())
		}
		_, requests := f.snapshot()
		for _, r := range requests {
			if strings.Contains(r, "other-entrance") {
				t.Errorf("client followed the redirect to %q", r)
			}
		}
		// The identity check, the detail fetch,
		// and the refused post: no retries
		// through any other entrance.
		if len(requests) > 3 {
			t.Errorf("made %d requests (%v), want at most 3", len(requests), requests)
		}
	})
	t.Run("identity endpoint down prevents post", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		f.mu.Lock()
		f.statuses["GET /accounts/self"] = http.StatusForbidden
		f.mu.Unlock()
		err := c.cmdComment(context.Background(), changeKey, "hello", nil)
		if err == nil {
			t.Fatal("denied identity: want an error, never a fallback")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("error should surface the status: %v", err)
		}
		if f.postCount() != 0 {
			t.Errorf("a denied route must not fall back to posting")
		}
	})
}

func TestDecodeStripsMagicPrefix(t *testing.T) {
	var v map[string]string
	if err := decodeGerritJSON([]byte(")]}'\n{\"k\":\"v\"}"), &v); err != nil {
		t.Fatal(err)
	}
	if v["k"] != "v" {
		t.Errorf("decoded %v", v)
	}
}

func TestRedirectDisabledEvenForGET(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Location", "/gerrit/other-entrance")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()
	c := newGerritClient(srv.URL, "Agent", nil)
	_, _, err := c.request(context.Background(), http.MethodGet, []string{"accounts", "self"}, nil, nil)
	if err == nil {
		t.Fatal("a redirect must be refused, not followed")
	}
	if !strings.Contains(err.Error(), "refused redirect") {
		t.Errorf("error should name the refusal: %v", err)
	}
	if hits != 1 {
		t.Errorf("hits = %d, want 1 (no second entrance)", hits)
	}
}

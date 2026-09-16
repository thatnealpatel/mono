package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Acceptance 1: endpoint routing selects
// Agent for exactly the entrance hint,
// otherwise Human; no credentials or
// fallback route are used.
func TestSelectEndpoint(t *testing.T) {
	cases := []struct {
		host, yah    string
		wantBase     string
		wantExpected string
	}{
		{"", "1", "http://supermarket:9001/gerrit/a", "Agent"},
		{"", "", "http://supermarket:9999/gerrit/a", "Human"},
		{"", "0", "http://supermarket:9999/gerrit/a", "Human"},
		{"", "yes", "http://supermarket:9999/gerrit/a", "Human"},
		{"gerrit.corp", "1", "http://gerrit.corp:9001/gerrit/a", "Agent"},
		{"gerrit.corp", "", "http://gerrit.corp:9999/gerrit/a", "Human"},
	}
	for _, tc := range cases {
		base, expected := selectEndpoint(tc.host, tc.yah)
		if base != tc.wantBase {
			t.Errorf("selectEndpoint(%q, %q) base = %q, want %q", tc.host, tc.yah, base, tc.wantBase)
		}
		if expected != tc.wantExpected {
			t.Errorf("selectEndpoint(%q, %q) user = %q, want %q", tc.host, tc.yah, expected, tc.wantExpected)
		}
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("YAH", "1")
	t.Setenv("GRFA_HOST", "gerrit.internal")
	c := newClient()
	if c.base != "http://gerrit.internal:9001/gerrit/a" {
		t.Errorf("base = %q", c.base)
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
}

// Acceptance 1: a wrong accounts/self prevents the POST.
func TestWrongIdentityPreventsPost(t *testing.T) {
	f, c, _ := seededClient(t, "Human")
	// The client entered through the agent
	// door; the server reports the human
	// identity.
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

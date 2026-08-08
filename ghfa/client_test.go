package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupTest points the package globals at an httptest.Server so
// commands hit the fake instead of api.github.com.
func setupTest(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldBase, oldUpstream := proxyBase, upstream
	t.Cleanup(func() {
		proxyBase = oldBase
		upstream = oldUpstream
	})
	proxyBase = srv.URL
	upstream = "owner/repo"
	return srv
}

func TestDoSetsHeaders(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("auth = %q, want empty (proxy handles auth)", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("accept = %q, want application/vnd.github+json", got)
		}
		if got := r.Header.Get("User-Agent"); got != "patel.codes/ghfa" {
			t.Errorf("user-agent = %q, want patel.codes/ghfa", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	_, _, _, err := do(context.Background(), http.MethodGet, proxyBase+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoSetsContentType(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	_, _, _, err := do(context.Background(), http.MethodPost, proxyBase+"/test", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoNoContentTypeOnNilBody(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("content-type = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	_, _, _, err := do(context.Background(), http.MethodGet, proxyBase+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatusError(t *testing.T) {
	err := statusError(404, []byte("  Not Found\n"))
	msg := err.Error()
	if !strings.Contains(msg, "404") {
		t.Errorf("error = %q, want it to contain 404", msg)
	}
	if !strings.Contains(msg, "Not Found") {
		t.Errorf("error = %q, want it to contain trimmed body", msg)
	}
}

func TestStatusErrorTruncatesLargeBody(t *testing.T) {
	big := strings.Repeat("x", 8192)
	err := statusError(500, []byte(big))
	if len(err.Error()) > 4200 {
		t.Errorf("error too long: %d bytes, want <= ~4KiB", len(err.Error()))
	}
}

func TestNextLink(t *testing.T) {
	for _, tc := range []struct {
		name, header, want string
	}{
		{"NextFirst", `<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"`, "https://api.github.com/x?page=2"},
		{"NextNotFirst", `<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=2>; rel="next"`, "https://api.github.com/x?page=2"},
		{"NoNext", `<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=5>; rel="last"`, ""},
		{"Empty", "", ""},
		{"Malformed", `no angle brackets; rel="next"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextLink(tc.header); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintJSON(t *testing.T) {
	if err := printJSON(map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestPrintJSONToIgnoresWriteError(t *testing.T) {
	if err := printJSONTo(errorWriter{}, map[string]int{"n": 1}); err != nil {
		t.Errorf("printJSONTo error = %v, want nil", err)
	}
}

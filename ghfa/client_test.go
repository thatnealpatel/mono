package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	proxy    = ""
	testRepo = "owner/repo"
)

func setupTest(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func setupURL(t *testing.T, handler http.Handler) string {
	t.Helper()
	return setupTest(t, handler).URL
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestDoSetsHeaders(t *testing.T) {
	srv := setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	_, _, _, err := do(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoSetsContentType(t *testing.T) {
	srv := setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	_, _, _, err := do(context.Background(), http.MethodPost, srv.URL+"/test", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoNoContentTypeOnNilBody(t *testing.T) {
	srv := setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("content-type = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	_, _, _, err := do(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteJSONReportsEncodingError(t *testing.T) {
	err := writeJSON(io.Discard, make(chan int))
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error = %v, want unsupported type", err)
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

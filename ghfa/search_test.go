package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCmdSearchIssues(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "repo:owner/repo") {
			t.Errorf("q = %q, want no repo injection for search issues", q)
		}
		if want := "is:issue is:open"; q != want {
			t.Errorf("q = %q, want %q", q, want)
		}
		w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
	}))

	var out bytes.Buffer
	err := cmdSearchIssuesTo(t.Context(), &out, []string{"is:issue", "is:open"})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got, want := string(got.Items), "[]"; got != want {
		t.Errorf("items = %s, want %s", got, want)
	}
}

func TestSearchIssuesPreservesUnknownFieldsAcrossPages(t *testing.T) {
	srv := setupTest(t, nil)
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/gh/search/issues", func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Query().Get("q"), "repo:owner/repo is:issue"; got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(`{"total_count":99,"incomplete_results":true,"items":[{"number":2,"proxy_second":{"nested":true}}]}`))
		default:
			w.Header().Set("Link", `<`+proxyBase+`/gh/search/issues?q=repo%3Aowner%2Frepo+is%3Aissue&page=2>; rel="next"`)
			w.Write([]byte(`{"total_count":2,"incomplete_results":false,"items":[{"number":1,"proxy_first":"kept"}]}`))
		}
	})

	var out bytes.Buffer
	if err := searchIssues(t.Context(), &out, "repo:owner/repo is:issue"); err != nil {
		t.Fatalf("command: %v", err)
	}
	var got struct {
		TotalCount        int                          `json:"total_count"`
		IncompleteResults bool                         `json:"incomplete_results"`
		Items             []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got, want := got.TotalCount, 2; got != want {
		t.Errorf("total count = %d, want %d", got, want)
	}
	if got, want := got.IncompleteResults, false; got != want {
		t.Errorf("incomplete results = %t, want %t", got, want)
	}
	if got := got.Items; got == nil {
		t.Errorf("items = %#v, want non-nil slice", got)
	}
	if got, want := len(got.Items), 2; got != want {
		t.Fatalf("items length = %d, want %d", got, want)
	}
	if got, want := string(got.Items[0]["proxy_first"]), `"kept"`; got != want {
		t.Errorf("first unknown field = %s, want %s", got, want)
	}
	var second struct {
		Nested bool `json:"nested"`
	}
	if err := json.Unmarshal(got.Items[1]["proxy_second"], &second); err != nil {
		t.Fatalf("decode second unknown field: %v", err)
	}
	if !second.Nested {
		t.Errorf("second unknown field nested = false, want true")
	}
}

func TestCmdSearchIssuesBadArgs(t *testing.T) {
	if err := cmdSearchIssues(context.Background(), nil); err == nil {
		t.Fatal("want error for empty args")
	}
}

func TestSearchIssuesHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("rate limited"))
	}))

	err := searchIssues(t.Context(), io.Discard, "test")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to contain 403", err)
	}
}

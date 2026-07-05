package main

import (
	"context"
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

	err := cmdSearchIssues(context.Background(), []string{"is:issue", "is:open"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchIssuesPaginated(t *testing.T) {
	srv := setupTest(t, nil)
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/gh/search/issues", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(`{"total_count":99,"incomplete_results":true,"items":[{"number":2}]}`))
		default:
			w.Header().Set("Link", `<`+proxyBase+`/gh/search/issues?page=2>; rel="next"`)
			w.Write([]byte(`{"total_count":2,"incomplete_results":false,"items":[{"number":1}]}`))
		}
	})

	err := searchIssues(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
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

	err := searchIssues(context.Background(), "test")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to contain 403", err)
	}
}

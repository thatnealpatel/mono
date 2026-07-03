package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestInjectQuery(t *testing.T) {
	t.Run("AddsRepoAndType", func(t *testing.T) {
		got := injectQuery("owner/repo", "label:bug")
		if want := "repo:owner/repo is:issue label:bug"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("SkipsIsWhenPresent", func(t *testing.T) {
		got := injectQuery("owner/repo", "is:pr label:bug")
		if strings.Count(got, "is:") != 1 {
			t.Errorf("got %q, want exactly one is: qualifier", got)
		}
		if !strings.Contains(got, "is:pr") {
			t.Errorf("got %q, want it to preserve is:pr", got)
		}
	})
}

func TestCmdIssueSearch(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/search/issues" {
			t.Errorf("path = %q, want /search/issues", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if !strings.HasPrefix(q, "repo:owner/repo") {
			t.Errorf("q = %q, want repo:owner/repo prefix", q)
		}
		if !strings.Contains(q, "is:issue") {
			t.Errorf("q = %q, want is:issue injected", q)
		}
		if !strings.Contains(q, "label:bug") {
			t.Errorf("q = %q, want label:bug from args", q)
		}
		w.Write([]byte(`{"total_count":1,"incomplete_results":false,"items":[{"number":5}]}`))
	}))

	err := cmdIssueSearch(context.Background(), []string{"label:bug"})
	if err != nil {
		t.Fatal(err)
	}
}

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

	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(`{"total_count":99,"incomplete_results":true,"items":[{"number":2}]}`))
		default:
			w.Header().Set("Link", `<`+apiBase+`/search/issues?page=2>; rel="next"`)
			w.Write([]byte(`{"total_count":2,"incomplete_results":false,"items":[{"number":1}]}`))
		}
	})

	err := searchIssues(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdIssueSearchBadArgs(t *testing.T) {
	if err := cmdIssueSearch(context.Background(), nil); err == nil {
		t.Fatal("want error for empty args")
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

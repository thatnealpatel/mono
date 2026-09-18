package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-h"},
		{"owner/repo"},
	} {
		var out bytes.Buffer
		if err := run(context.Background(), "http://proxy", &out, args); err != nil {
			t.Errorf("run(%v) = %v, want nil", args, err)
		}
		if got := out.String(); got != usage {
			t.Errorf("run(%v) output = %q, want usage", args, got)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run(context.Background(), "http://proxy", &bytes.Buffer{}, []string{"owner/repo", "bogus", "cmd"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	for _, want := range []string{`unknown command "bogus cmd"`, "issue view", "repo clone", "pr create"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestRunSearchIssuesWithoutRepo(t *testing.T) {
	proxy := setupURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/gh/search/issues"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("q"), "is:issue is:open"; got != want {
			t.Errorf("query = %q, want %q", got, want)
		}
		w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
	}))
	var out bytes.Buffer
	if err := run(t.Context(), proxy, &out, []string{"search", "issues", "is:issue", "is:open"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"items": []`) {
		t.Errorf("output = %q, want empty items", out.String())
	}
}

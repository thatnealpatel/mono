package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type issueViewRecorder struct {
	t        *testing.T
	requests int
}

func (rec *issueViewRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.requests++
	if got, want := r.Method, http.MethodGet; got != want {
		rec.t.Errorf("method = %q, want %q", got, want)
	}
	if got, want := r.URL.Path, "/gh/repos/owner/repo/issues/7"; got != want {
		rec.t.Errorf("path = %q, want %q", got, want)
	}
	w.Write([]byte(`{"issue":{"number":7,"proxy_issue_field":"kept"},"timeline":[{"event":"commented","proxy_event_field":{"nested":true}}]}`))
}

func TestCmdViewPrintsRawEnvelopeWithOneRequest(t *testing.T) {
	recorder := &issueViewRecorder{t: t}
	setupTest(t, recorder)

	var out bytes.Buffer
	if err := cmdIssueViewTo(t.Context(), &out, []string{"7"}); err != nil {
		t.Fatalf("command: %v", err)
	}
	got := out.String()
	const want = "{\n  \"issue\": {\n    \"number\": 7,\n    \"proxy_issue_field\": \"kept\"\n  },\n  \"timeline\": [\n    {\n      \"event\": \"commented\",\n      \"proxy_event_field\": {\n        \"nested\": true\n      }\n    }\n  ]\n}\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if got, want := recorder.requests, 1; got != want {
		t.Errorf("requests = %d, want %d", got, want)
	}
}

func TestCmdViewNotFound(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))

	err := cmdIssueView(context.Background(), []string{"999"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err)
	}
}

func TestCmdViewBadArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}, {"abc"}} {
		if err := cmdIssueView(context.Background(), args); err == nil {
			t.Errorf("cmdIssueView(%v) = nil, want error", args)
		}
	}
}

func TestCmdCreate(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/gh/repos/owner/repo/issues" {
			t.Errorf("path = %q, want /gh/repos/owner/repo/issues", r.URL.Path)
		}
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Title != "the title" {
			t.Errorf("title = %q, want 'the title'", req.Title)
		}
		if req.Body != "the body" {
			t.Errorf("body = %q, want 'the body'", req.Body)
		}
		if len(req.Labels) != 2 || req.Labels[0] != "bug" || req.Labels[1] != "auto-filed" {
			t.Errorf("labels = %v, want [bug auto-filed]", req.Labels)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":42,"html_url":"https://github.com/owner/repo/issues/42","state":"open"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "the title", "-body", "the body", "-label", "bug,auto-filed"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCreateNoLabels(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Labels != nil {
			t.Errorf("labels = %v, want nil (omitted)", req.Labels)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1,"html_url":"u","state":"open"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "bare issue"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCreateLabelCSVTrimming(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(req.Labels) != 2 || req.Labels[0] != "bug" || req.Labels[1] != "feature request" {
			t.Errorf("labels = %v, want [bug, feature request]", req.Labels)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1,"html_url":"u","state":"open"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "t", "-label", " bug , feature request "})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCreateLabelCSVEmpty(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Labels != nil {
			t.Errorf("labels = %v, want nil for empty CSV segments", req.Labels)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1,"html_url":"u","state":"open"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "t", "-label", " , , "})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCreateFile(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "body.md")
	if err := os.WriteFile(md, []byte("file body"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Body != "file body" {
			t.Errorf("body = %q, want 'file body'", req.Body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1,"html_url":"u","state":"open"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "t", "-file", md})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCreateBodyFileExclusive(t *testing.T) {
	err := cmdIssueCreate(context.Background(), []string{"-title", "t", "-body", "x", "-file", "y"})
	if err == nil {
		t.Fatal("want error for -body and -file together")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want 'mutually exclusive'", err)
	}
}

func TestCmdCreateMissingTitle(t *testing.T) {
	err := cmdIssueCreate(context.Background(), []string{"-body", "only body"})
	if err == nil {
		t.Fatal("want error for missing title")
	}
}

func TestCmdCreateHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))

	err := cmdIssueCreate(context.Background(), []string{"-title", "t"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error = %q, want it to contain 422", err)
	}
}

func TestCmdEdit(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		if r.URL.Path != "/gh/repos/owner/repo/issues/7" {
			t.Errorf("path = %q, want /gh/repos/owner/repo/issues/7", r.URL.Path)
		}
		var m map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := m["title"]; !ok {
			t.Error("request missing title key")
		}
		if _, ok := m["body"]; !ok {
			t.Error("request missing body key")
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"open"}`))
	}))

	err := cmdIssueEdit(context.Background(), []string{"7", "-title", "new title", "-body", "new body"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdEditTitleOnly(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := m["title"]; !ok {
			t.Error("request missing title key")
		}
		if _, ok := m["body"]; ok {
			t.Error("request has body key, want it omitted")
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"open"}`))
	}))

	err := cmdIssueEdit(context.Background(), []string{"7", "-title", "updated"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdEditBodyOnly(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := m["title"]; ok {
			t.Error("request has title key, want it omitted")
		}
		if _, ok := m["body"]; !ok {
			t.Error("request missing body key")
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"open"}`))
	}))

	err := cmdIssueEdit(context.Background(), []string{"7", "-body", "updated body"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdEditNoFlags(t *testing.T) {
	err := cmdIssueEdit(context.Background(), []string{"7"})
	if err == nil {
		t.Fatal("want error for no flags")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("error = %q, want 'at least one' message", err)
	}
}

func TestCmdEditBadNumber(t *testing.T) {
	if err := cmdIssueEdit(context.Background(), []string{"abc"}); err == nil {
		t.Fatal("want error for non-numeric issue number")
	}
}

func TestCmdEditNoArgs(t *testing.T) {
	if err := cmdIssueEdit(context.Background(), nil); err == nil {
		t.Fatal("want error for empty args")
	}
}

func TestCmdEditHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))

	err := cmdIssueEdit(context.Background(), []string{"999", "-title", "t"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err)
	}
}

func TestCmdClose(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantState  string
		wantReason string
	}{
		{"Completed", []string{"7"}, "closed", "completed"},
		{"CompletedExplicit", []string{"7", "-r", "completed"}, "closed", "completed"},
		{"NotPlanned", []string{"7", "-r", "not planned"}, "closed", "not_planned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.Method, http.MethodPatch; got != want {
					t.Errorf("method = %q, want %q", got, want)
				}
				var m map[string]string
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got, want := m["state"], tc.wantState; got != want {
					t.Errorf("state = %q, want %q", got, want)
				}
				if got, want := m["state_reason"], tc.wantReason; got != want {
					t.Errorf("state_reason = %q, want %q", got, want)
				}
				w.Write([]byte(`{"number":7,"html_url":"h","state":"` + tc.wantState + `"}`))
			}))
			if err := cmdIssueClose(context.Background(), tc.args); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCmdCloseDupeof(t *testing.T) {
	var gotComment string
	srv := setupTest(t, nil)
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/gh/repos/owner/repo/issues/7", func(w http.ResponseWriter, r *http.Request) {
		var m map[string]string
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := m["state"], "closed"; got != want {
			t.Errorf("state = %q, want %q", got, want)
		}
		if got, want := m["state_reason"], "duplicate"; got != want {
			t.Errorf("state_reason = %q, want %q", got, want)
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"closed"}`))
	})
	mux.HandleFunc("/gh/repos/owner/repo/issues/7/comments", func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		var req commentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gotComment = req.Body
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"user":{"login":"bot","id":1},"created_at":"t","updated_at":"t","body":"ok"}`))
	})

	if err := cmdIssueClose(context.Background(), []string{"7", "-dupeof", "42"}); err != nil {
		t.Fatal(err)
	}
	if got, want := gotComment, "Duplicate of #42"; got != want {
		t.Errorf("comment body = %q, want %q", got, want)
	}
}

func TestCmdCloseBadArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"NoArgs", nil},
		{"BadNumber", []string{"abc"}},
		{"InvalidReason", []string{"7", "-r", "bogus"}},
		{"NegativeDupeof", []string{"7", "-dupeof", "-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := cmdIssueClose(context.Background(), tc.args); err == nil {
				t.Errorf("cmdIssueClose(%v) = nil, want error", tc.args)
			}
		})
	}
}

func TestCmdCloseConflictingFlags(t *testing.T) {
	err := cmdIssueClose(context.Background(), []string{"7", "-dupeof", "5", "-r", "not planned"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got, want := err.Error(), "mutually exclusive"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}
}

func TestCmdCloseHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	err := cmdIssueClose(context.Background(), []string{"999"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "404") {
		t.Errorf("error = %q, want it to contain 404", got)
	}
}

func TestCmdCloseOmitsUnsetFields(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, key := range []string{"body", "title", "labels"} {
			if _, ok := m[key]; ok {
				t.Errorf("request[%q] present, want omitted", key)
			}
		}
		for _, key := range []string{"state", "state_reason"} {
			if _, ok := m[key]; !ok {
				t.Errorf("request[%q] absent, want present", key)
			}
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"closed"}`))
	}))
	if err := cmdIssueClose(context.Background(), []string{"7"}); err != nil {
		t.Fatal(err)
	}
}

func TestCloseResultShape(t *testing.T) {
	out, err := json.Marshal(closeResult{Number: 7, HTMLURL: "h", State: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"number":7`, `"html_url":"h"`, `"state":"closed"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("got %s, want it to contain %s", out, want)
		}
	}
}

func TestCmdReopen(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPatch; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		var m map[string]string
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := m["state"], "open"; got != want {
			t.Errorf("state = %q, want %q", got, want)
		}
		if got, want := m["state_reason"], "reopened"; got != want {
			t.Errorf("state_reason = %q, want %q", got, want)
		}
		w.Write([]byte(`{"number":7,"html_url":"h","state":"open"}`))
	}))
	if err := cmdIssueReopen(context.Background(), []string{"7"}); err != nil {
		t.Fatal(err)
	}
}

func TestCmdReopenWithComment(t *testing.T) {
	var gotComment string
	srv := setupTest(t, nil)
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/gh/repos/owner/repo/issues/7", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"number":7,"html_url":"h","state":"open"}`))
	})
	mux.HandleFunc("/gh/repos/owner/repo/issues/7/comments", func(w http.ResponseWriter, r *http.Request) {
		var req commentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gotComment = req.Body
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"user":{"login":"neal","id":1},"created_at":"t","updated_at":"t","body":"ok"}`))
	})

	if err := cmdIssueReopen(context.Background(), []string{"7", "-c", "reopening this"}); err != nil {
		t.Fatal(err)
	}
	if got, want := gotComment, "reopening this"; got != want {
		t.Errorf("comment body = %q, want %q", got, want)
	}
}

func TestCmdReopenBadArgs(t *testing.T) {
	if err := cmdIssueReopen(context.Background(), nil); err == nil {
		t.Fatal("want error for no args")
	}
	if err := cmdIssueReopen(context.Background(), []string{"abc"}); err == nil {
		t.Fatal("want error for non-numeric issue number")
	}
}

func TestCmdReopenHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	err := cmdIssueReopen(context.Background(), []string{"999"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "404") {
		t.Errorf("error = %q, want it to contain 404", got)
	}
}

func TestCmdComment(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "comment.md")
	if err := os.WriteFile(md, []byte("hello from test"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/gh/repos/owner/repo/issues/7/comments" {
			t.Errorf("path = %q, want /gh/repos/owner/repo/issues/7/comments", r.URL.Path)
		}
		var req commentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Body != "hello from test" {
			t.Errorf("body = %q, want 'hello from test'", req.Body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"user":{"login":"neal","id":1},"created_at":"t","updated_at":"t","body":"hello from test"}`))
	}))

	err := cmdIssueComment(context.Background(), []string{"7", "-file", md})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCommentBody(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req commentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Body != "inline body" {
			t.Errorf("body = %q, want 'inline body'", req.Body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"user":{"login":"neal","id":1},"created_at":"t","updated_at":"t","body":"inline body"}`))
	}))

	err := cmdIssueComment(context.Background(), []string{"7", "-body", "inline body"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdCommentBadArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"7"}, {"7", "-body", "x", "-file", "y"}} {
		if err := cmdIssueComment(context.Background(), args); err == nil {
			t.Errorf("cmdIssueComment(%v) = nil, want error", args)
		}
	}
}

func TestCmdCommentBadNumber(t *testing.T) {
	if err := cmdIssueComment(context.Background(), []string{"abc", "-body", "x"}); err == nil {
		t.Fatal("want error for non-numeric issue number")
	}
}

func TestCmdCommentMissingFile(t *testing.T) {
	err := cmdIssueComment(context.Background(), []string{"7", "-file", "/nonexistent/file.md"})
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestCmdCommentHTTPError(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "c.md")
	if err := os.WriteFile(md, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("rate limited"))
	}))

	err := cmdIssueComment(context.Background(), []string{"7", "-file", md})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to contain 403", err)
	}
}

func TestCommentResultShape(t *testing.T) {
	out, err := json.Marshal(commentResult{Number: 73})
	if err != nil {
		t.Fatal(err)
	}
	if want := `"number":73`; !strings.Contains(string(out), want) {
		t.Errorf("got %s, want it to contain %s", out, want)
	}
	for _, absent := range []string{"html_url", "body", "user"} {
		if strings.Contains(string(out), absent) {
			t.Errorf("got %s, want it to omit %s", out, absent)
		}
	}
}

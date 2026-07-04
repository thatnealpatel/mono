package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdPRCreate(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/repos/owner/repo/pulls"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var body struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Head  string `json:"head"`
			Base  string `json:"base"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := body.Title, "add lemma"; got != want {
			t.Errorf("title = %q, want %q", got, want)
		}
		if got, want := body.Head, "notnealpatel:bot/machine/slug"; got != want {
			t.Errorf("head = %q, want %q", got, want)
		}
		if got, want := body.Base, "main"; got != want {
			t.Errorf("base = %q, want %q", got, want)
		}
		if got, want := body.Body, "proof body"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1,"html_url":"https://github.com/owner/repo/pull/1","state":"open"}`))
	}))

	err := cmdPRCreate(context.Background(), []string{
		"-title", "add lemma",
		"-head", "notnealpatel:bot/machine/slug",
		"-base", "main",
		"-body", "proof body",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdPRCreateFile(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "pr.md")
	if err := os.WriteFile(md, []byte("file body"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := body.Body, "file body"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":2,"html_url":"u","state":"open"}`))
	}))

	err := cmdPRCreate(context.Background(), []string{
		"-title", "t",
		"-head", "notnealpatel:bot/x",
		"-file", md,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdPRCreateDefaultBase(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Base string `json:"base"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, want := body.Base, "main"; got != want {
			t.Errorf("base = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":3,"html_url":"u","state":"open"}`))
	}))

	err := cmdPRCreate(context.Background(), []string{
		"-title", "t",
		"-head", "notnealpatel:bot/x",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdPRCreateBadArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"MissingTitle", []string{"-head", "x:y"}},
		{"MissingHead", []string{"-title", "t"}},
		{"HeadNoColon", []string{"-title", "t", "-head", "branch-only"}},
		{"BodyAndFile", []string{"-title", "t", "-head", "x:y", "-body", "b", "-file", "f"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := cmdPRCreate(context.Background(), tc.args); err == nil {
				t.Errorf("cmdPRCreate(%v) = nil, want error", tc.args)
			}
		})
	}
}

func TestCmdPRCreateHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))

	err := cmdPRCreate(context.Background(), []string{"-title", "t", "-head", "x:y"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error = %q, want it to contain 422", err)
	}
}

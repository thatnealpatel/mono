package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCmdLabelList(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/labels" {
			t.Errorf("path = %q, want /repos/owner/repo/labels", r.URL.Path)
		}
		w.Write([]byte(`[{"name":"bug","color":"d73a4a","description":"Something isn't working"},{"name":"feature","color":"a2eeef","description":"New feature"}]`))
	}))

	err := cmdLabelList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdLabelListPaginated(t *testing.T) {
	srv := setupTest(t, nil)
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/repos/owner/repo/labels", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(`[{"name":"feature","color":"a2eeef","description":""}]`))
		default:
			w.Header().Set("Link", `<`+apiBase+`/repos/owner/repo/labels?page=2>; rel="next"`)
			w.Write([]byte(`[{"name":"bug","color":"d73a4a","description":""}]`))
		}
	})

	err := cmdLabelList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdLabelListHTTPError(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))

	err := cmdLabelList(context.Background(), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err)
	}
}

func TestCmdLabelListEmpty(t *testing.T) {
	setupTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))

	err := cmdLabelList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

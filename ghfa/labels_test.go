package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCmdLabelList(t *testing.T) {
	proxy := setupURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/gh/repos/owner/repo/labels" {
			t.Errorf("path = %q, want /gh/repos/owner/repo/labels", r.URL.Path)
		}
		w.Write([]byte(`[{"name":"bug","color":"d73a4a","description":"Something isn't working"},{"name":"feature","color":"a2eeef","description":"New feature"}]`))
	}))

	err := cmdLabelList(context.Background(), proxy, testRepo, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmdLabelListPaginated(t *testing.T) {
	srv := setupTest(t, nil)
	proxy := srv.URL
	mux := http.NewServeMux()
	srv.Config.Handler = mux

	mux.HandleFunc("/gh/repos/owner/repo/labels", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "2":
			w.Write([]byte(`[{"name":"feature","color":"a2eeef","description":""}]`))
		default:
			w.Header().Set("Link", `<`+proxy+`/gh/repos/owner/repo/labels?page=2>; rel="next"`)
			w.Write([]byte(`[{"name":"bug","color":"d73a4a","description":""}]`))
		}
	})

	var out bytes.Buffer
	if err := cmdLabelList(context.Background(), proxy, testRepo, &out, nil); err != nil {
		t.Fatal(err)
	}
	want := "[\n  {\n    \"name\": \"bug\",\n    \"color\": \"d73a4a\",\n    \"description\": \"\"\n  },\n  {\n    \"name\": \"feature\",\n    \"color\": \"a2eeef\",\n    \"description\": \"\"\n  }\n]\n"
	if got := out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestCmdLabelListHTTPError(t *testing.T) {
	proxy := setupURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))

	err := cmdLabelList(context.Background(), proxy, testRepo, io.Discard, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err)
	}
}

func TestCmdLabelListEmpty(t *testing.T) {
	proxy := setupURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))

	err := cmdLabelList(context.Background(), proxy, testRepo, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
}

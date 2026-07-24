package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"patel.codes/retrieval"
)

func TestSearchOutputFormat(t *testing.T) {
	out := retrieval.OeisSearchResult{
		Query:   "groups",
		Results: 2,
		Matches: []retrieval.OeisSearchMatch{
			{ID: "A000001", Name: "Number of groups of order n", Score: 5.0},
			{ID: "A000040", Name: "The prime numbers", Score: 3.0},
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed retrieval.OeisSearchResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := parsed.Query, "groups"; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
	if got, want := len(parsed.Matches), 2; got != want {
		t.Fatalf("len(matches) = %d, want %d", got, want)
	}
	if got, want := parsed.Matches[0].ID, "A000001"; got != want {
		t.Errorf("matches[0].id = %q, want %q", got, want)
	}
}

func TestMatchOutputArray(t *testing.T) {
	matches := []retrieval.OeisMatch{
		{ID: "A000045", Name: "Fibonacci numbers", Terms: "0,1,1,2,3,5,8,13,21,34,"},
		{ID: "A000079", Name: "Powers of 2", Terms: "1,2,4,8,16,32,64,128,256,512,"},
	}
	data, err := json.Marshal(matches)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed []retrieval.OeisMatch
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := len(parsed), 2; got != want {
		t.Fatalf("len(matches) = %d, want %d", got, want)
	}
	if got, want := parsed[0].ID, "A000045"; got != want {
		t.Errorf("matches[0].id = %q, want %q", got, want)
	}
	if got, want := parsed[1].ID, "A000079"; got != want {
		t.Errorf("matches[1].id = %q, want %q", got, want)
	}
}

func TestRemoteMode(t *testing.T) {
	want := `{"id":"A000045","name":"Fibonacci numbers","terms":"0,1,1,2,3,"}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/oeis"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var req struct {
			Subcommand string `json:"subcommand"`
			Query      string `json:"query"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		if got, want := req.Subcommand, "show"; got != want {
			t.Errorf("subcommand = %q, want %q", got, want)
		}
		if got, want := req.Query, "A000045"; got != want {
			t.Errorf("query = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, want)
	}))
	defer srv.Close()

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = pw
	t.Cleanup(func() { os.Stdout = origStdout })

	if err := oeisRemote(srv.Client(), srv.URL, "show", "A000045"); err != nil {
		t.Fatalf("oeisRemote: %v", err)
	}
	pw.Close()
	got, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if string(got) != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestRemoteModeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "store not loaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := oeisRemote(srv.Client(), srv.URL, "show", "A000045")
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
	if !errors.Is(err, errRemoteStatus) {
		t.Errorf("error = %v, want %v", err, errRemoteStatus)
	}
}

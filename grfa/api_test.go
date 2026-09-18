package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNoCredentialsAttached(t *testing.T) {
	f, proxy := seededServer(t, "Agent")
	if err := cmdComment(t.Context(), proxy, changeKey, []string{"hello"}); err != nil {
		t.Fatal(err)
	}
	if f.sawCredentials() {
		t.Errorf("client attached an Authorization header or cookie")
	}
}

func TestHTTPErrorOnDetail(t *testing.T) {
	f, proxy := seededServer(t, "Agent")
	f.mu.Lock()
	f.statuses["GET /changes/"+changeKey+"/detail"] = http.StatusInternalServerError
	f.mu.Unlock()
	err := cmdView(t.Context(), proxy, changeKey)
	if err == nil {
		t.Fatal("internal server error: want an error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should surface the status: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("no mutation may follow an error")
	}
}

func TestRequestStripsMagicPrefix(t *testing.T) {
	f, proxy := newFakeGerrit(t, "Agent")
	f.mu.Lock()
	f.raw["GET /changes/x/detail"] = magicPrefix + `{"k":"v"}`
	f.mu.Unlock()
	data, err := request(t.Context(), proxy, http.MethodGet, []string{"changes", "x", "detail"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]string
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v["k"] != "v" {
		t.Errorf("decoded %v", v)
	}
}

func TestQueryCurrentRevisions(t *testing.T) {
	f, proxy := newFakeGerrit(t, "Agent")
	f.details[changeKey] = &changeDetail{
		ID: changeKey, ChangeID: changeKey, CurrentRevision: ps2SHA,
		Revisions: map[string]revisionInfo{ps2SHA: {Number: 2}},
	}
	f.land([]revision{{Commit: ps2SHA, ChangeID: changeKey}})
	revs, err := queryCurrentRevisions(t.Context(), proxy, changeKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0] != ps2SHA {
		t.Errorf("revs = %v, want [%s]", revs, ps2SHA)
	}
	_, requests := f.snapshot()
	var sawQuery bool
	for _, r := range requests {
		if strings.HasPrefix(r, "GET /changes?") &&
			strings.Contains(r, "q=change%3A") &&
			strings.Contains(r, "o=CURRENT_REVISION") {
			sawQuery = true
		}
	}
	if !sawQuery {
		t.Errorf("the query must use the change-query endpoint with CURRENT_REVISION: %v", requests)
	}
}

package main

import (
	"os"
	"strings"
	"testing"
)

const (
	session42 = "sess-42"
	marker42  = "Yah-Session: " + session42
)

const (
	revA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	revB      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	revC      = "cccccccccccccccccccccccccccccccccccccccc"
	changeID1 = "Iaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	changeID2 = "Ibbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func unsetenv(t *testing.T, name string) {
	t.Helper()
	v, ok := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !ok {
			return
		}
		if err := os.Setenv(name, v); err != nil {
			t.Fatal(err)
		}
	})
}

func seededChanges(t *testing.T, session string) (*fakeGerrit, string) {
	t.Helper()
	t.Setenv("YAH_SESSION", session)
	f, host := newFakeGerrit(t, "Agent")
	f.details[changeID1] = &changeDetail{
		ID: changeID1, ChangeID: changeID1, Number: 1, Status: "NEW", CurrentRevision: revA,
		Revisions: map[string]revisionInfo{revA: {Number: 1}},
	}
	f.details[changeID2] = &changeDetail{
		ID: changeID2, ChangeID: changeID2, Number: 2, Status: "NEW", CurrentRevision: revB,
		Revisions: map[string]revisionInfo{revB: {Number: 1}},
	}
	return f, host
}

func twoRevisions() []revision {
	return []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
		{Commit: revB, ChangeID: changeID2, Subject: "second change"},
	}
}

func TestStampUploadStampsEveryChange(t *testing.T) {
	f, host := seededChanges(t, session42)
	if err := stampUpload(t.Context(), host, twoRevisions(), session42); err != nil {
		t.Fatal(err)
	}
	posts, _ := f.snapshot()
	if len(posts) != 2 {
		t.Fatalf("stamps = %d, want 2 (one per change in the set)", len(posts))
	}
	byChange := map[string]recordedPost{}
	for _, p := range posts {
		byChange[p.Change] = p
	}
	for _, tc := range []struct {
		change, rev string
	}{
		{changeID1, revA},
		{changeID2, revB},
	} {
		p := byChange[tc.change]
		if p.Revision != tc.rev {
			t.Errorf("stamp for %s posted against %q, want the pushed revision %q", tc.change, p.Revision, tc.rev)
		}
		list := p.Body.Comments[patchSetLevel]
		if len(list) != 1 {
			t.Fatalf("stamp for %s: comments = %v, want one patch-set-level comment", tc.change, p.Body.Comments)
		}
		if list[0].Message != marker42 {
			t.Errorf("marker = %q, want %q", list[0].Message, marker42)
		}
		if list[0].Unresolved {
			t.Errorf("marker must be a resolved comment")
		}
		if list[0].InReplyTo != "" || list[0].Side != "" || list[0].Line != 0 {
			t.Errorf("marker must carry no location: %+v", list[0])
		}
	}
}

func TestStampUploadOneMarkerPerChange(t *testing.T) {
	f, host := seededChanges(t, session42)
	revs := []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
		{Commit: revB, ChangeID: changeID1, Subject: "same change"},
		{Commit: revB, ChangeID: changeID2, Subject: "second change"},
	}
	if err := stampUpload(t.Context(), host, revs, session42); err != nil {
		t.Fatal(err)
	}
	if got := f.postCount(); got != 2 {
		t.Errorf("stamps = %d, want 2 (a repeated Change-Id is stamped once)", got)
	}
}

func TestStampChangeOnce(t *testing.T) {
	f, host := seededChanges(t, session42)
	stamped, err := stampChange(t.Context(), host, changeID1, marker42)
	if err != nil {
		t.Fatal(err)
	}
	if !stamped {
		t.Fatal("an unmarked change must be stamped")
	}
	posts, _ := f.snapshot()
	if len(posts) != 1 || posts[0].Revision != revA {
		t.Fatalf("posts = %+v, want one against %s", posts, revA)
	}
	stamped, err = stampChange(t.Context(), host, changeID1, marker42)
	if err != nil {
		t.Fatal(err)
	}
	if stamped {
		t.Error("an already marked change must not be stamped again")
	}
	if got := f.postCount(); got != 1 {
		t.Errorf("posts = %d, want 1", got)
	}
}

func TestStampIdempotent(t *testing.T) {
	f, host := seededChanges(t, session42)
	one := []revision{{Commit: revA, ChangeID: changeID1, Subject: "first change"}}

	f.mu.Lock()
	f.comments[changeID1] = map[string][]commentInfo{
		patchSetLevel: {
			{ID: "m0", PatchSet: 1, CommitID: revA, Message: marker42, Unresolved: false,
				Author: &accountInfo{ID: accountID(1000000), Name: "Agent", Username: "Agent"}},
		},
	}
	f.mu.Unlock()
	for run := 1; run <= 2; run++ {
		if err := stampUpload(t.Context(), host, one, session42); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if got := f.postCount(); got != 0 {
			t.Errorf("run %d: posted %d markers, want 0 (already stamped)", run, got)
		}
	}

	f.mu.Lock()
	f.comments[changeID1] = map[string][]commentInfo{
		patchSetLevel: {
			{ID: "m1", PatchSet: 1, CommitID: revC, Message: marker42, Unresolved: false},
		},
	}
	f.mu.Unlock()
	if err := stampUpload(t.Context(), host, one, session42); err != nil {
		t.Fatal(err)
	}
	if got := f.postCount(); got != 0 {
		t.Errorf("posts = %d, want 0: a marker on an older patch set still marks the change", got)
	}

	f.mu.Lock()
	f.comments[changeID1] = map[string][]commentInfo{
		patchSetLevel: {
			{ID: "m2", PatchSet: 1, CommitID: revA, Message: "Yah-Session: another-session", Unresolved: false},
		},
	}
	f.mu.Unlock()
	if err := stampUpload(t.Context(), host, one, session42); err != nil {
		t.Fatal(err)
	}
	if got := f.postCount(); got != 0 {
		t.Errorf("posts = %d, want 0: a marker naming another session still marks the change", got)
	}
}

func TestFailedStampIsPartialResult(t *testing.T) {
	f, host := seededChanges(t, session42)
	f.mu.Lock()
	delete(f.details, changeID2)
	f.mu.Unlock()
	err := stampUpload(t.Context(), host, twoRevisions(), session42)
	if err == nil {
		t.Fatal("stamp failure: want an error")
	}
	if !strings.Contains(err.Error(), "push succeeded, but stamping failed") {
		t.Errorf("error should report the partial result: %v", err)
	}
	if !strings.Contains(err.Error(), changeID2) {
		t.Errorf("error should name the unstamped change: %v", err)
	}
	if f.postCount() != 1 {
		t.Errorf("posts = %d, want 1 (the healthy change was still stamped)", f.postCount())
	}
}

func TestSessionMarkerValidation(t *testing.T) {
	cases := []struct {
		name    string
		session string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"whitespace", "a b", "single line"},
		{"tab", "a\tb", "single line"},
		{"newline", "a\nb", "single line"},
		{"control", "a\x01b", "single line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("YAH_SESSION", tc.session)
			if _, err := sessionMarker(); err == nil {
				t.Fatal("invalid session: want an error")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestSessionMarkerAbsentAndPresent(t *testing.T) {
	unsetenv(t, "YAH_SESSION")
	v, err := sessionMarker()
	if err != nil {
		t.Fatalf("an unset YAH_SESSION must not be an error: %v", err)
	}
	if v != "" {
		t.Errorf("marker = %q, want empty for an unset YAH_SESSION", v)
	}
	t.Setenv("YAH_SESSION", session42)
	v, err = sessionMarker()
	if err != nil {
		t.Fatal(err)
	}
	if v != session42 {
		t.Errorf("marker = %q, want %q", v, session42)
	}
}

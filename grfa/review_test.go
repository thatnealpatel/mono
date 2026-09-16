package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const (
	changeKey = "I1234567890abcdef1234567890abcdef12345678"
	ps1SHA    = "1111111111111111111111111111111111111111"
	ps2SHA    = "2222222222222222222222222222222222222222"
)

// testFixture is a change with two patch sets: c1/c2 are a thread
// on file.go (c2 resolved the thread), c3 sits on the old patch
// set on a path missing from the current one, and c4 is a
// patch-set-level thread. Only c4 is unresolved at the leaves.
func testFixture() (*changeDetail, map[string][]commentInfo) {
	d := &changeDetail{
		ID:              "proj~main~" + changeKey,
		Number:          42,
		Status:          "NEW",
		Subject:         "add the widget",
		CurrentRevision: ps2SHA,
		Revisions: map[string]revisionInfo{
			ps1SHA: {Number: 1},
			ps2SHA: {Number: 2},
		},
		Labels: map[string]labelInfo{
			"Code-Review": {Value: 1, All: []voteInfo{
				{Value: 2, Account: &accountInfo{Username: "alice"}},
				{Value: -1, Account: &accountInfo{Username: "bob"}},
			}},
		},
		Messages: []changeMessage{
			{ID: "msg-1", Author: &accountInfo{Username: "carol"}, Message: "Patch Set 1: Code-Review+2"},
		},
	}
	comments := map[string][]commentInfo{
		"file.go": {
			{ID: "c1", PatchSet: 1, CommitID: ps1SHA, Message: "first", Unresolved: true,
				Author: &accountInfo{Username: "alice"}, Line: 3},
			{ID: "c2", PatchSet: 2, CommitID: ps2SHA, Message: "second", Unresolved: false,
				InReplyTo: "c1", Author: &accountInfo{Username: "dave"}},
		},
		"removed/file.go": {
			{ID: "c3", PatchSet: 1, CommitID: ps1SHA, Message: "on old patch set", Unresolved: true,
				Author: &accountInfo{Username: "alice"}, Side: "PARENT", Parent: 2,
				Range: &commentRange{StartLine: 1, StartChar: 0, EndLine: 2, EndChar: 5}},
		},
		patchSetLevel: {
			{ID: "c4", PatchSet: 2, CommitID: ps2SHA, Message: "patch-set-level", Unresolved: true,
				Author: &accountInfo{Username: "eve"}},
		},
	}
	return d, comments
}

func seededClient(t *testing.T, user string) (*fakeGerrit, *cli, *bytes.Buffer) {
	t.Helper()
	f, srv := newFakeGerrit(t, user)
	d, comments := testFixture()
	f.details[changeKey] = d
	f.comments[changeKey] = comments
	out := &bytes.Buffer{}
	c := &cli{
		out:    out,
		api:    testClient(srv, user),
		runner: refusingRunner(),
		getenv: func(string) (string, bool) { return "", false },
	}
	return f, c, out
}

// Acceptance 2: one view includes current and older patch-set
// comments with their exact revision identities, and prints IDs
// that -reply accepts verbatim.
func TestViewIncludesAllPatchSets(t *testing.T) {
	f, c, out := seededClient(t, "Agent")
	if err := c.cmdView(context.Background(), changeKey); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"change proj~main~" + changeKey + " (#42) NEW",
		"revision " + ps2SHA + " patch-set 2",
		"unresolved-threads 2",
		"Code-Review +1 (alice=+2, bob=-1)",
		"carol: Patch Set 1: Code-Review+2",
		// c1: old patch set, old revision identity
		"file.go patch-set 1 revision " + ps1SHA + " id c1 line 3 unresolved author alice",
		// c2: current patch set, resolved leaf
		"file.go patch-set 2 revision " + ps2SHA + " id c2 reply-to c1 resolved author dave",
		// c3: merge-parent number printed alongside side
		"removed/file.go patch-set 1 revision " + ps1SHA + " id c3 side PARENT parent 2 range 1:0-2:5 unresolved",
		// c4: patch-set-level thread
		patchSetLevel + " patch-set 2 revision " + ps2SHA + " id c4 unresolved author eve",
		"    first",
		"    second",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("view output missing %q\ngot:\n%s", want, got)
		}
	}
	if f.requestCount() != 2 {
		t.Errorf("view made %d requests, want 2 (detail + comments)", f.requestCount())
	}
}

// Acceptance 3: a reply to a file absent from the current patch
// set posts to the target comment's commit_id, against the
// original path, not against revisions/current.
func TestReplyToRemovedFileUsesTargetCommitAndPath(t *testing.T) {
	f, c, _ := seededClient(t, "Agent")
	if err := c.cmdComment(context.Background(), changeKey, "why was this removed?", []string{"-reply", "c3"}); err != nil {
		t.Fatal(err)
	}
	posts, _ := f.snapshot()
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	p := posts[0]
	if p.Revision != ps1SHA {
		t.Errorf("reply revision = %q, want the target's commit_id %q", p.Revision, ps1SHA)
	}
	if p.Change != changeKey {
		t.Errorf("reply change = %q, want %q", p.Change, changeKey)
	}
	list, ok := p.Body.Comments["removed/file.go"]
	if !ok || len(list) != 1 {
		t.Fatalf("reply comments = %v, want one on removed/file.go", p.Body.Comments)
	}
	in := list[0]
	if in.InReplyTo != "c3" {
		t.Errorf("in_reply_to = %q, want c3", in.InReplyTo)
	}
	if in.Message != "why was this removed?" {
		t.Errorf("message = %q", in.Message)
	}
	// Positional metadata copied from the target.
	if in.Side != "PARENT" || in.Parent != 2 {
		t.Errorf("side/parent = %q/%d, want PARENT/2", in.Side, in.Parent)
	}
	if in.Range == nil {
		t.Errorf("range not copied from target")
	}
}

// Acceptance 4: reply matching is exact. An id absent from the
// change-level comment list fails without posting, a change-message
// id fails, and an id that exists only on an older patch set
// succeeds.
func TestReplyMatchingIsExact(t *testing.T) {
	t.Run("unknown id fails without posting", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		err := c.cmdComment(context.Background(), changeKey, "hello", []string{"-reply", "does-not-exist"})
		if err == nil {
			t.Fatal("reply to unknown id: want error")
		}
		if f.postCount() != 0 {
			t.Errorf("posted %d reviews, want 0", f.postCount())
		}
	})
	t.Run("change message id is not a target", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		err := c.cmdComment(context.Background(), changeKey, "hello", []string{"-reply", "msg-1"})
		if err == nil {
			t.Fatal("reply to change-message id: want error")
		}
		if !strings.Contains(err.Error(), "msg-1") {
			t.Errorf("error should name the id: %v", err)
		}
		if f.postCount() != 0 {
			t.Errorf("posted %d reviews, want 0", f.postCount())
		}
	})
	t.Run("older patch set id succeeds", func(t *testing.T) {
		f, c, _ := seededClient(t, "Agent")
		if err := c.cmdComment(context.Background(), changeKey, "hello", []string{"-reply", "c3"}); err != nil {
			t.Fatal(err)
		}
		posts, _ := f.snapshot()
		if len(posts) != 1 || posts[0].Revision != ps1SHA {
			t.Errorf("posts = %+v, want one against %s", posts, ps1SHA)
		}
	})
}

// Acceptance 5: replies preserve line, range, merge-parent-2,
// file-level, and patch-set-level locations with correct
// omission of empty fields.
func TestReplyLocationCopy(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		target      commentInfo
		wantPresent []string
		wantAbsent  []string
		checkValue  func(t *testing.T, raw map[string]any)
	}{
		{
			name:        "line only",
			path:        "file.go",
			target:      commentInfo{Line: 7},
			wantPresent: []string{"line"},
			wantAbsent:  []string{"range", "side", "parent"},
		},
		{
			name:        "range only",
			path:        "file.go",
			target:      commentInfo{Range: &commentRange{StartLine: 2, StartChar: 0, EndLine: 4, EndChar: 9}},
			wantPresent: []string{"range"},
			wantAbsent:  []string{"line", "side", "parent"},
		},
		{
			name:        "merge parent two",
			path:        "file.go",
			target:      commentInfo{Side: "PARENT", Parent: 2},
			wantPresent: []string{"side", "parent"},
			wantAbsent:  []string{"range", "line"},
			checkValue: func(t *testing.T, raw map[string]any) {
				if raw["side"] != "PARENT" || raw["parent"] != float64(2) {
					t.Errorf("side/parent = %v/%v, want PARENT/2", raw["side"], raw["parent"])
				}
			},
		},
		{
			name:        "file level has no location",
			path:        "file.go",
			target:      commentInfo{},
			wantPresent: nil,
			wantAbsent:  []string{"line", "range", "side", "parent"},
		},
		{
			name:        "patch set level carries no location",
			path:        patchSetLevel,
			target:      commentInfo{},
			wantPresent: nil,
			wantAbsent:  []string{"line", "range", "side", "parent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFakeGerrit(t, "Agent")
			target := tc.target
			target.ID = "target-1"
			target.PatchSet = 2
			target.CommitID = ps2SHA
			target.Message = "root"
			target.Unresolved = true
			f.details[changeKey] = &changeDetail{
				ID: changeKey, Number: 7, Status: "NEW", CurrentRevision: ps2SHA,
				Revisions: map[string]revisionInfo{ps2SHA: {Number: 2}},
			}
			f.comments[changeKey] = map[string][]commentInfo{tc.path: {target}}
			c := &cli{
				out:    &bytes.Buffer{},
				api:    testClient(srv, "Agent"),
				runner: refusingRunner(),
				getenv: func(string) (string, bool) { return "", false },
			}
			if err := c.cmdComment(context.Background(), changeKey, "reply body", []string{"-reply", "target-1"}); err != nil {
				t.Fatal(err)
			}
			posts, _ := f.snapshot()
			if len(posts) != 1 {
				t.Fatalf("got %d posts, want 1", len(posts))
			}
			var body map[string]any
			if err := json.Unmarshal(posts[0].Raw, &body); err != nil {
				t.Fatal(err)
			}
			comments := body["comments"].(map[string]any)
			list := comments[tc.path].([]any)
			if len(list) != 1 {
				t.Fatalf("comments[%s] = %v, want one entry", tc.path, comments[tc.path])
			}
			one := list[0].(map[string]any)
			for _, k := range tc.wantPresent {
				if _, ok := one[k]; !ok {
					t.Errorf("posted comment missing %q: %v", k, one)
				}
			}
			for _, k := range tc.wantAbsent {
				if _, ok := one[k]; ok {
					t.Errorf("posted comment must omit %q: %v", k, one)
				}
			}
			if one["in_reply_to"] != "target-1" {
				t.Errorf("in_reply_to = %v, want target-1", one["in_reply_to"])
			}
			if _, ok := one["unresolved"]; !ok {
				t.Errorf("unresolved must always be sent explicitly: %v", one)
			}
			if tc.checkValue != nil {
				tc.checkValue(t, one)
			}
		})
	}
}

// Acceptance 6: default unresolved and explicit resolved work for
// both new comments and replies; a reply never inherits the
// target's state.
func TestUnresolvedDefaults(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// replyTarget is the id to reply to; empty means a new
		// comment.
		replyTarget string
		want        bool
	}{
		{name: "new comment defaults to unresolved", want: true},
		{name: "new comment with -resolved", args: []string{"-resolved"}, want: false},
		// c2 is itself resolved; the reply must not inherit that.
		{name: "reply defaults to unresolved", args: []string{"-reply", "c2"}, replyTarget: "c2", want: true},
		{name: "reply with -resolved", args: []string{"-reply", "c2", "-resolved"}, replyTarget: "c2", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c, _ := seededClient(t, "Agent")
			if err := c.cmdComment(context.Background(), changeKey, "body", tc.args); err != nil {
				t.Fatal(err)
			}
			posts, _ := f.snapshot()
			if len(posts) != 1 {
				t.Fatalf("got %d posts, want 1", len(posts))
			}
			path := patchSetLevel
			if tc.replyTarget != "" {
				path = "file.go"
			}
			list := posts[0].Body.Comments[path]
			if len(list) != 1 {
				t.Fatalf("comments on %s = %v", path, posts[0].Body.Comments)
			}
			if list[0].Unresolved != tc.want {
				t.Errorf("unresolved = %v, want %v", list[0].Unresolved, tc.want)
			}
			var raw map[string]any
			if err := json.Unmarshal(posts[0].Raw, &raw); err != nil {
				t.Fatal(err)
			}
			one := raw["comments"].(map[string]any)[path].([]any)[0].(map[string]any)
			if one["unresolved"] != tc.want {
				t.Errorf("wire unresolved = %v, want %v (must be explicit)", one["unresolved"], tc.want)
			}
		})
	}
}

// Acceptance 7: quoted option-like message text is preserved.
func TestOptionLikeMessagePreserved(t *testing.T) {
	f, c, _ := seededClient(t, "Agent")
	msg := "-r main --dry-run \"quoted text\" -resolved"
	if err := c.cmdComment(context.Background(), changeKey, msg, nil); err != nil {
		t.Fatal(err)
	}
	posts, _ := f.snapshot()
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	got := posts[0].Body.Comments[patchSetLevel][0].Message
	if got != msg {
		t.Errorf("message = %q, want %q verbatim", got, msg)
	}
	if posts[0].Body.Comments[patchSetLevel][0].Unresolved != true {
		t.Errorf("option-like message text must not be parsed as flags")
	}
}

// Acceptance 8: the refreshed response state is actually
// consumed after posting; the fake changes the relevant comment
// and resolution state, and the printed view must reflect it.
func TestRefreshAfterPostIsConsumed(t *testing.T) {
	t.Run("resolved reply updates the thread", func(t *testing.T) {
		f, c, out := seededClient(t, "Agent")
		if err := c.cmdComment(context.Background(), changeKey, "fixed now", []string{"-reply", "c4", "-resolved"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		for _, want := range []string{
			"fixed now",
			"resolved",
			"unresolved-threads 1",
			"id fake-1 reply-to c4",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("refreshed view missing %q\ngot:\n%s", want, got)
			}
		}
		if f.postCount() != 1 {
			t.Errorf("post count = %d, want 1", f.postCount())
		}
	})
	t.Run("unresolved new comment shows up", func(t *testing.T) {
		_, c, out := seededClient(t, "Agent")
		if err := c.cmdComment(context.Background(), changeKey, "please explain this branch", nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "please explain this branch") {
			t.Errorf("refreshed view missing the new comment:\n%s", got)
		}
		if !strings.Contains(got, "unresolved-threads 3") {
			t.Errorf("refreshed view should count the new unresolved thread:\n%s", got)
		}
	})
}

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	changeKey = "I1234567890abcdef1234567890abcdef12345678"
	ps1SHA    = "1111111111111111111111111111111111111111"
	ps2SHA    = "2222222222222222222222222222222222222222"
)

func testFixture() (*changeDetail, map[string][]commentInfo) {
	alice := &accountInfo{ID: accountID(1000002), Name: "Alice", Username: "alice"}
	d := &changeDetail{
		ID:              "proj~main~" + changeKey,
		Project:         "proj",
		Branch:          "main",
		ChangeID:        changeKey,
		Number:          42,
		Status:          "NEW",
		Subject:         "add the widget",
		Created:         "2026-09-14 10:00:00.000000000",
		Updated:         "2026-09-15 10:00:00.000000000",
		CurrentRevision: ps2SHA,
		Revisions: map[string]revisionInfo{
			ps1SHA: {Number: 1},
			ps2SHA: {Number: 2},
		},
		Labels: map[string]labelInfo{
			"Code-Review": {
				Value:        1,
				DefaultValue: 0,
				Values: map[string]string{
					" 0": "No score",
					"-1": "I would prefer this is not submitted as is",
					"-2": "This shall not be submitted",
					"+1": "Looks good to me, but someone else must approve",
					"+2": "Looks good to me, approved",
				},
				All: []voteInfo{
					{
						Value:          2,
						Date:           "2026-09-14 11:00:00.000000000",
						PermittedRange: &votingRange{Min: -2, Max: 2},
						accountInfo:    *alice,
					},
					{
						Value:       -1,
						Date:        "2026-09-15 09:00:00.000000000",
						accountInfo: accountInfo{ID: accountID(1000003), Name: "Bob", Username: "bob"},
					},
				},
			},
		},
		Messages: []changeMessage{
			{ID: "msg-1", Author: &accountInfo{ID: accountID(1000004), Name: "Carol", Username: "carol"},
				Date: "2026-09-14 12:00:00.000000000", Message: "Patch Set 1: Code-Review+2", Revision: 1},
		},
	}
	comments := map[string][]commentInfo{
		"file.go": {
			{ID: "c1", PatchSet: 1, CommitID: ps1SHA, Message: "first", Unresolved: true,
				Updated: "2026-09-14 10:01:00.000000000",
				Author:  &accountInfo{ID: accountID(1000002), Name: "Alice", Username: "alice"}, Line: 3},
			{ID: "c2", PatchSet: 2, CommitID: ps2SHA, Message: "second", Unresolved: false,
				InReplyTo: "c1", Updated: "2026-09-15 10:01:00.000000000",
				Author: &accountInfo{ID: accountID(1000005), Name: "Dave", Username: "dave"}},
		},
		"removed/file.go": {
			{ID: "c3", PatchSet: 1, CommitID: ps1SHA, Message: "on old patch set", Unresolved: true,
				Updated: "2026-09-14 10:02:00.000000000",
				Author:  &accountInfo{ID: accountID(1000002), Name: "Alice", Username: "alice"}, Side: "PARENT", Parent: 2,
				Range: &commentRange{StartLine: 1, StartChar: 0, EndLine: 2, EndChar: 5}},
		},
		patchSetLevel: {
			{ID: "c4", PatchSet: 2, CommitID: ps2SHA, Message: "patch-set-level", Unresolved: true,
				Updated: "2026-09-15 10:03:00.000000000",
				Author:  &accountInfo{ID: accountID(1000006), Name: "Eve", Username: "eve"}},
		},
	}
	return d, comments
}

func seededServer(t *testing.T, user string) (*fakeGerrit, string) {
	t.Helper()
	f, proxy := newFakeGerrit(t, user)
	d, comments := testFixture()
	f.details[changeKey] = d
	f.comments[changeKey] = comments
	return f, proxy
}

const realVotesFragment = `"all":[{"value":0,"permitted_voting_range":{"min":-2,"max":2},"_account_id":1000000,"name":"Human","username":"Human"}]`

const realDetailPayload = ")]}'\n" + `{
  "id": "grfa~main~I1a2b3c4d5e6f708192a3b4c5d6e7f80918273a",
  "project": "grfa",
  "branch": "main",
  "change_id": "I1a2b3c4d5e6f708192a3b4c5d6e7f80918273a",
  "subject": "add the widget",
  "status": "NEW",
  "created": "2026-09-15 21:07:05.000000000",
  "updated": "2026-09-16 01:39:12.000000000",
  "insertions": 12,
  "deletions": 4,
  "_number": 141,
  "current_revision": "4f0c9d0a1e2b3c4d5e6f708192a3b4c5d6e7f80",
  "revisions": {
    "3b9f8a7d6c5e4f30129384756a7b8c9d0e1f2a3b": {
      "kind": "REWORK",
      "_number": 1,
      "created": "2026-09-15 21:07:05.000000000",
      "uploader": {"_account_id": 1000000, "name": "Human", "username": "Human"},
      "ref": "refs/changes/41/141/1"
    },
    "4f0c9d0a1e2b3c4d5e6f708192a3b4c5d6e7f80": {
      "kind": "REWORK",
      "_number": 2,
      "created": "2026-09-16 01:38:44.000000000",
      "uploader": {"_account_id": 1000000, "name": "Human", "username": "Human"},
      "ref": "refs/changes/41/141/2"
    }
  },
  "labels": {
    "Code-Review": {
      "default_value": 0,
      "values": {
        " 0": "No score",
        "-1": "I would prefer this is not submitted as is",
        "-2": "This shall not be submitted",
        "+1": "Looks good to me, but someone else must approve",
        "+2": "Looks good to me, approved"
      },
      ` + realVotesFragment + `
    },
    "Verified": {
      "default_value": 0,
      "values": {" 0": "No score", "-1": "Fails", "+1": "Verified"},
      "all": [
        {
          "value": 1,
          "permitted_voting_range": {"min": -1, "max": 1},
          "date": "2026-09-16 01:39:01.000000000",
          "_account_id": 1000001,
          "name": "Alice",
          "username": "alice",
          "email": "alice@example.com"
        }
      ]
    },
    "Presubmit-Ready": {
      "default_value": 0,
      "values": {" 0": "No score", "+1": "Presubmit-Ready"}
    }
  },
  "messages": [
    {
      "id": "message-9f2e5b1c3d7a4801928374656a7b8c9d",
      "author": {"_account_id": 1000001, "name": "Alice", "username": "alice"},
      "date": "2026-09-16 01:39:01.000000000",
      "message": "Patch Set 2: Verified+1",
      "_revision_number": 2
    }
  ],
  "unresolved_comment_count": 0
}`

func TestFetchChangeDetailDecodesRealPayload(t *testing.T) {
	f, proxy := newFakeGerrit(t, "Agent")
	f.mu.Lock()
	f.raw["GET /changes/141/detail"] = realDetailPayload
	f.mu.Unlock()
	d, err := fetchChangeDetail(t.Context(), proxy, "141")
	if err != nil {
		t.Fatalf("the real payload must decode: %v", err)
	}
	if d.ID != "grfa~main~I1a2b3c4d5e6f708192a3b4c5d6e7f80918273a" || d.Number != 141 {
		t.Errorf("identity = %q/#%d, want the payload's id and number", d.ID, d.Number)
	}
	if d.Project != "grfa" || d.Branch != "main" || d.ChangeID != "I1a2b3c4d5e6f708192a3b4c5d6e7f80918273a" {
		t.Errorf("project/branch/change_id = %q/%q/%q", d.Project, d.Branch, d.ChangeID)
	}
	if d.Status != "NEW" || d.Subject != "add the widget" || d.Insertions != 12 || d.Deletions != 4 {
		t.Errorf("metadata = %+v", d)
	}
	if d.CurrentRevision != "4f0c9d0a1e2b3c4d5e6f708192a3b4c5d6e7f80" {
		t.Errorf("current_revision = %q", d.CurrentRevision)
	}
	if got := d.Revisions["3b9f8a7d6c5e4f30129384756a7b8c9d0e1f2a3b"].Number; got != 1 {
		t.Errorf("first patch set number = %d, want 1", got)
	}
	if got := d.Revisions["4f0c9d0a1e2b3c4d5e6f708192a3b4c5d6e7f80"].Number; got != 2 {
		t.Errorf("second patch set number = %d, want 2", got)
	}
	if len(d.Revisions) != 2 {
		t.Errorf("revisions = %v, want two patch sets", d.Revisions)
	}
	if len(d.Labels) != 3 {
		t.Errorf("labels = %v, want Code-Review, Verified and Presubmit-Ready", d.Labels)
	}
	review := d.Labels["Code-Review"].All
	if len(review) != 1 {
		t.Fatalf("Code-Review votes = %+v, want exactly one", review)
	}
	if review[0].Username != "Human" || review[0].Value != 0 {
		t.Errorf("Code-Review vote = %+v, want Human=+0", review[0])
	}
	if review[0].PermittedRange == nil || review[0].PermittedRange.Min != -2 || review[0].PermittedRange.Max != 2 {
		t.Errorf("Code-Review permitted range = %+v", review[0].PermittedRange)
	}
	verified := d.Labels["Verified"].All
	if len(verified) != 1 {
		t.Fatalf("Verified votes = %+v, want exactly one", verified)
	}
	if verified[0].Username != "alice" || verified[0].Value != 1 {
		t.Errorf("Verified vote = %+v, want alice=+1", verified[0])
	}
	if got := d.Labels["Presubmit-Ready"].All; len(got) != 0 {
		t.Errorf("Presubmit-Ready votes = %+v, want none", got)
	}
	if len(d.Messages) != 1 || d.Messages[0].ID != "message-9f2e5b1c3d7a4801928374656a7b8c9d" || d.Messages[0].Revision != 2 {
		t.Errorf("messages = %+v", d.Messages)
	}
	if d.Messages[0].Author == nil || d.Messages[0].Author.Username != "alice" || d.Messages[0].Message != "Patch Set 2: Verified+1" {
		t.Errorf("message author/body = %+v", d.Messages[0])
	}
	if d.UnresolvedCommentCount == nil || *d.UnresolvedCommentCount != 0 {
		t.Errorf("unresolved_comment_count = %v, want 0", d.UnresolvedCommentCount)
	}
	if f.requestCount() != 1 {
		t.Errorf("fetch made %d requests, want 1", f.requestCount())
	}
}

func TestReplyToRemovedFileUsesTargetCommitAndPath(t *testing.T) {
	f, proxy := seededServer(t, "Agent")
	if err := cmdComment(t.Context(), proxy, changeKey, []string{"-reply", "c3", "why was this removed?"}); err != nil {
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

	if in.Side != "PARENT" || in.Parent != 2 {
		t.Errorf("side/parent = %q/%d, want PARENT/2", in.Side, in.Parent)
	}
	if in.Range == nil {
		t.Errorf("range not copied from target")
	}
}

func TestReplyMatchingIsExact(t *testing.T) {
	t.Run("unknown id fails without posting", func(t *testing.T) {
		f, proxy := seededServer(t, "Agent")
		err := cmdComment(t.Context(), proxy, changeKey, []string{"-reply", "does-not-exist", "hello"})
		if err == nil {
			t.Fatal("reply to unknown id: want error")
		}
		if f.postCount() != 0 {
			t.Errorf("posted %d reviews, want 0", f.postCount())
		}
	})
	t.Run("change message id is not a target", func(t *testing.T) {
		f, proxy := seededServer(t, "Agent")
		err := cmdComment(t.Context(), proxy, changeKey, []string{"-reply", "msg-1", "hello"})
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
		f, proxy := seededServer(t, "Agent")
		if err := cmdComment(t.Context(), proxy, changeKey, []string{"-reply", "c3", "hello"}); err != nil {
			t.Fatal(err)
		}
		posts, _ := f.snapshot()
		if len(posts) != 1 || posts[0].Revision != ps1SHA {
			t.Errorf("posts = %+v, want one against %s", posts, ps1SHA)
		}
	})
}

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
			f, proxy := newFakeGerrit(t, "Agent")
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
			if err := cmdComment(t.Context(), proxy, changeKey, []string{"-reply", "target-1", "reply body"}); err != nil {
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

func TestUnresolvedDefaults(t *testing.T) {
	cases := []struct {
		name string
		args []string

		replyTarget string
		want        bool
	}{
		{name: "new comment defaults to unresolved", args: []string{"body"}, want: true},
		{name: "new comment with -resolved", args: []string{"-resolved", "body"}, want: false},

		{name: "reply defaults to unresolved", args: []string{"-reply", "c2", "body"}, replyTarget: "c2", want: true},
		{name: "reply with -resolved", args: []string{"-reply", "c2", "-resolved", "body"}, replyTarget: "c2", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, proxy := seededServer(t, "Agent")
			if err := cmdComment(t.Context(), proxy, changeKey, tc.args); err != nil {
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

func TestOptionLikeMessagePreserved(t *testing.T) {
	f, proxy := seededServer(t, "Agent")
	msg := `-r main --remote upstream "quoted text" -resolved`
	if err := cmdComment(t.Context(), proxy, changeKey, []string{"--", msg}); err != nil {
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

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// uploadFixture wires a client whose fake jj reports the given upload set,
// and seeds the fake Gerrit with both changes. The seeded changes are not
// on the server yet — no push has landed — so a fresh upload's pre-push
// lookup finds nothing.
func uploadFixture(t *testing.T, revs []revision, session string, sessionSet bool) (*fakeGerrit, *jjFake, *cli, *bytes.Buffer) {
	t.Helper()
	f, srv := newFakeGerrit(t, "Agent")
	return uploadFixtureAt(t, f, srv, "Agent", revs, session, sessionSet)
}

// uploadFixtureAt wires the cli at an already-built fake entrance, for
// tests that need a different server identity or reachability.
func uploadFixtureAt(t *testing.T, f *fakeGerrit, srv *httptest.Server, expectedUser string, revs []revision, session string, sessionSet bool) (*fakeGerrit, *jjFake, *cli, *bytes.Buffer) {
	t.Helper()
	f.details[changeID1] = &changeDetail{
		ID: changeID1, ChangeID: changeID1, Number: 1, Status: "NEW", CurrentRevision: revA,
		Revisions: map[string]revisionInfo{revA: {Kind: "REWORK", Number: 1, Ref: "refs/changes/01/1/1"}},
	}
	f.details[changeID2] = &changeDetail{
		ID: changeID2, ChangeID: changeID2, Number: 2, Status: "NEW", CurrentRevision: revB,
		Revisions: map[string]revisionInfo{revB: {Kind: "REWORK", Number: 1, Ref: "refs/changes/02/2/1"}},
	}
	root := t.TempDir()
	jj := &jjFake{root: root, defaultRev: "@", setRevs: revs}
	out := &bytes.Buffer{}
	c := &cli{
		out:    out,
		api:    testClient(srv, "Agent"),
		runner: jj.runner(),
		getenv: func(k string) (string, bool) {
			if k == "YAH_SESSION" {
				return session, sessionSet
			}
			return "", false
		},
	}
	return f, jj, c, out
}

func twoRevisions() []revision {
	return []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
		{Commit: revB, ChangeID: changeID2, Subject: "second change"},
	}
}

func countPushes(cmds []command) int {
	n := 0
	for _, cmd := range cmds {
		if cmd.name == jjBinary && strings.HasPrefix(jjBody(cmd), "gerrit upload") {
			n++
		}
	}
	return n
}

// assertOneIdentityCheck pins the invariant that a session-present
// upload which reaches stamping performs the entrance identity check
// exactly once, before any stamp POST: one check covers the push and
// the stamp, and the stamping path never re-checks. A removed pre-push
// check (zero requests) and a re-added stamp-path check (two or more)
// both fail here, as does a check that runs only after the stamps.
func assertOneIdentityCheck(t *testing.T, f *fakeGerrit) {
	t.Helper()
	if got := f.selfRequestCount(); got != 1 {
		t.Errorf("identity checks = %d, want exactly 1 GET /accounts/self before stamping", got)
	}
	_, requests := f.snapshot()
	self, stamp := -1, -1
	for i, r := range requests {
		if strings.HasPrefix(r, "GET /accounts/self") && self == -1 {
			self = i
		}
		if strings.HasPrefix(r, "POST /changes/") && stamp == -1 {
			stamp = i
		}
	}
	if self == -1 || stamp == -1 || self > stamp {
		t.Errorf("the identity check must precede the stamp POST: %v", requests)
	}
}

// Acceptance 12: upload stamps each change in
// the upload set with the environment session
// as a resolved patch-set-level comment.
func TestUploadStampsEveryChange(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	cmds := jj.commandsSnapshot()
	if got := countPushes(cmds); got != 1 {
		t.Fatalf("pushes = %d, want exactly 1", got)
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
	if !strings.Contains(out.String(), "stamped "+changeID1) {
		t.Errorf("output should report stamping:\n%s", out)
	}
	assertOneIdentityCheck(t, f)
}

// Acceptance 12: no marker is posted when the environment
// variable is absent.
func TestUploadWithoutSessionPostsNoMarker(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), "", false)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if f.postCount() != 0 {
		t.Errorf("posts = %d, want 0", f.postCount())
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want 1", got)
	}
	if !strings.Contains(out.String(), "no provenance marker") {
		t.Errorf("output should explain the missing marker:\n%s", out)
	}
}

// Acceptance 13: re-running upload
// against a revision that already carries
// the identical marker posts nothing
// new, and performs no duplicate push
// for unchanged content: once the first
// push has landed the changes, the second
// run's pre-push lookup confirms presence
// and skips the push entirely.
func TestStampIdempotent(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
	}, session42, true)
	// The push lands the change on the
	// server, so a re-run sees it as the
	// current patch set.
	jj.onPush = func() { f.land([]revision{{Commit: revA, ChangeID: changeID1}}) }
	// The revision already carries the identical marker.
	f.mu.Lock()
	f.comments[changeID1] = map[string][]commentInfo{
		patchSetLevel: {
			{ID: "m0", PatchSet: 1, CommitID: revA, Message: marker42, Unresolved: false,
				Author: &accountInfo{ID: accountID(1000000), Name: "Agent", Username: "Agent"}},
		},
	}
	f.mu.Unlock()
	for run := 1; run <= 2; run++ {
		if err := c.cmdUpload(context.Background(), nil); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if got := f.postCount(); got != 0 {
			t.Errorf("run %d: posted %d markers, want 0 (already stamped)", run, got)
		}
		cmds := jj.commandsSnapshot()
		if got := countPushes(cmds); got != 1 {
			t.Errorf("run %d: cumulative pushes = %d, want 1 (the re-run skips the redundant push)", run, got)
		}
	}
	// A marker on an older revision is not
	// identical: a new patch set gets a
	// fresh stamp.
	f.mu.Lock()
	f.comments[changeID1] = map[string][]commentInfo{
		patchSetLevel: {
			{ID: "m1", PatchSet: 1, CommitID: "cccccccccccccccccccccccccccccccccccccccc", Message: marker42, Unresolved: false},
		},
	}
	f.mu.Unlock()
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := f.postCount(); got != 1 {
		t.Errorf("posts = %d, want 1 fresh stamp for the new revision", got)
	}
	posts, _ := f.snapshot()
	if posts[0].Revision != revA {
		t.Errorf("fresh stamp revision = %q, want %q", posts[0].Revision, revA)
	}
}

// Acceptance 10: a failing pre-upload hook aborts
// before the push and stamps nothing; the recorded
// command log contains no push.
func TestFailingHookAbortsBeforePush(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	writeHook(t, jj.root, "#!/bin/sh\nexit 3\n")
	jj.mu.Lock()
	jj.hookErr = errors.New("exit status 3")
	jj.mu.Unlock()
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("failing hook: want an error")
	}
	if !strings.Contains(err.Error(), "pre-upload hook") {
		t.Errorf("error should name the hook stage: %v", err)
	}
	cmds := jj.commandsSnapshot()
	if got := countPushes(cmds); got != 0 {
		t.Errorf("recorded commands contain %d pushes, want 0", got)
	}
	var hookRan bool
	for _, cmd := range cmds {
		if strings.HasSuffix(cmd.name, "/.grfa/pre-upload") {
			hookRan = true
			if cmd.dir != jj.root {
				t.Errorf("hook ran in %q, want the repository root %q", cmd.dir, jj.root)
			}
		}
	}
	if !hookRan {
		t.Errorf("the hook was never executed")
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0", f.postCount())
	}
	if f.requestCount() != 0 {
		t.Errorf("made %d HTTP requests, want 0 (no stamping after an aborted push)", f.requestCount())
	}
	if strings.Contains(out.String(), "stamped") {
		t.Errorf("output must not report stamping:\n%s", out)
	}
}

// A successful hook runs and the upload proceeds.
func TestSuccessfulHookRuns(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	writeHook(t, jj.root, "#!/bin/sh\nexit 0\n")
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var hookRan bool
	for _, cmd := range jj.commandsSnapshot() {
		if strings.HasSuffix(cmd.name, "/.grfa/pre-upload") {
			hookRan = true
		}
	}
	if !hookRan {
		t.Errorf("hook never ran")
	}
	if f.postCount() != 2 {
		t.Errorf("posts = %d, want 2", f.postCount())
	}
}

// An absent hook runs no checks.
func TestAbsentHookIsNoop(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range jj.commandsSnapshot() {
		if strings.HasSuffix(cmd.name, ".grfa/pre-upload") {
			t.Errorf("absent hook was invoked: %v", cmd)
		}
	}
}

// Acceptance 11: a missing local Change-Id aborts
// before the push.
func TestMissingChangeIDAbortsBeforePush(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "good"},
		{Commit: revB, ChangeID: "", Subject: "no trailer"},
	}, session42, true)
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("missing Change-Id: want an error")
	}
	if !strings.Contains(err.Error(), "change-id:") {
		t.Errorf("error should name the Change-Id stage, not the hook stage: %v", err)
	}
	if !strings.Contains(err.Error(), "Change-Id") {
		t.Errorf("error should name the Change-Id stage: %v", err)
	}
	if !strings.Contains(err.Error(), "no trailer") {
		t.Errorf("error should identify the offending revision: %v", err)
	}
	cmds := jj.commandsSnapshot()
	if got := countPushes(cmds); got != 0 {
		t.Errorf("recorded commands contain %d pushes, want 0", got)
	}
	if f.postCount() != 0 || f.requestCount() != 0 {
		t.Errorf("no network traffic may precede the Change-Id check")
	}
	for _, cmd := range cmds {
		if strings.HasSuffix(cmd.name, ".grfa/pre-upload") {
			t.Errorf("hook ran before the Change-Id check")
		}
	}
}

// Acceptance 14: grfa mutates no repository state:
// the recorded commands are read-only queries plus at
// most one push, with no snapshot, rebase, or commit.
func TestUploadIsReadOnlyPlusOnePush(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	cmds := jj.commandsSnapshot()
	if got := countPushes(cmds); got != 1 {
		t.Fatalf("pushes = %d, want 1", got)
	}
	for _, cmd := range cmds {
		if cmd.name != jjBinary {
			t.Errorf("unexpected subprocess %q", cmd.name)
			continue
		}
		if len(cmd.args) == 0 || cmd.args[0] != "--ignore-working-copy" {
			t.Errorf("command is not in the read-only stance: %v", cmd.args)
		}
		// The subcommand is the first argument
		// that is not a global flag.
		sub := ""
		for _, a := range cmd.args {
			if !strings.HasPrefix(a, "-") {
				sub = a
				break
			}
		}
		switch sub {
		case "root", "log":
			// read-only query
		case "gerrit":
			if !strings.Contains(strings.Join(cmd.args, " "), "gerrit upload") {
				t.Errorf("unexpected gerrit subcommand: %v", cmd.args)
			}
		default:
			t.Errorf("mutating or unexpected subcommand %q in %v", sub, cmd.args)
		}
	}
}

// Pass-through hints reach jj gerrit upload
// verbatim, and the upload-set query mirrors
// the given revset.
func TestUploadHintsPassThrough(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	args := []string{"-r", "myrev", "-b", "feature", "-remote", "upstream", "-reviewer", "a@x", "-reviewer", "b@x"}
	if err := c.cmdUpload(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	var push, query string
	for _, cmd := range jj.commandsSnapshot() {
		joined := jjBody(cmd)
		if strings.HasPrefix(joined, "gerrit upload") {
			push = joined
		}
		if strings.HasPrefix(joined, "log --no-graph -r") {
			query = joined
		}
	}
	wantPush := "gerrit upload -r myrev -b feature --remote upstream --reviewer a@x --reviewer b@x"
	if push != wantPush {
		t.Errorf("push = %q, want %q", push, wantPush)
	}
	if !strings.Contains(query, "mutable()::(myrev)") {
		t.Errorf("upload-set query = %q, want it to mirror the given revset", query)
	}
	// With an explicit -r, the default-revision
	// probe never runs.
	for _, cmd := range jj.commandsSnapshot() {
		if strings.Contains(jjBody(cmd), `if(description`) {
			t.Errorf("default-revision probe ran despite -r")
		}
	}
}

// Without -r, the VCS default is probed and mirrored.
func TestUploadDefaultRevsetMirrored(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.defaultRev = "@-"
	jj.mu.Unlock()
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var probe, query, push string
	for _, cmd := range jj.commandsSnapshot() {
		joined := jjBody(cmd)
		switch {
		case strings.Contains(joined, `if(description`):
			probe = joined
		case strings.HasPrefix(joined, "log --no-graph -r"):
			query = joined
		case strings.HasPrefix(joined, "gerrit upload"):
			push = joined
		}
	}
	if !strings.Contains(probe, "-r @") || !strings.Contains(probe, `if(description, "@", "@-")`) {
		t.Errorf("probe = %q", probe)
	}
	if !strings.Contains(query, "mutable()::(@-)") {
		t.Errorf("query = %q, want it to mirror the probed default", query)
	}
	// The push itself passes no -r: the
	// choice stays with the VCS.
	if push != "gerrit upload" {
		t.Errorf("push = %q, want the bare delegated push", push)
	}
}

// -dry-run runs the checks and reports the upload
// set and the changes that would be stamped, without
// pushing or stamping.
func TestDryRunReportsWithoutPushOrStamp(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	writeHook(t, jj.root, "#!/bin/sh\nexit 0\n")
	if err := c.cmdUpload(context.Background(), []string{"-dry-run"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{changeID1, changeID2, "would stamp: " + marker42} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
	if got := countPushes(jj.commandsSnapshot()); got != 0 {
		t.Errorf("dry-run pushed %d times, want 0", got)
	}
	if f.postCount() != 0 || f.requestCount() != 0 {
		t.Errorf("dry-run made network requests or stamped: posts=%d requests=%d", f.postCount(), f.requestCount())
	}
	var hookRan bool
	for _, cmd := range jj.commandsSnapshot() {
		if strings.HasSuffix(cmd.name, ".grfa/pre-upload") {
			hookRan = true
		}
	}
	if !hookRan {
		t.Errorf("dry-run must run the pre-upload checks")
	}
}

func TestDryRunWithoutSession(t *testing.T) {
	_, jj, c, out := uploadFixture(t, twoRevisions(), "", false)
	if err := c.cmdUpload(context.Background(), []string{"-dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no YAH_SESSION") {
		t.Errorf("dry-run output should explain there is no session:\n%s", out)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 0 {
		t.Errorf("dry-run pushed %d times, want 0", got)
	}
}

// A failed push is reported as its own stage,
// and nothing is stamped. The failure is genuine:
// the server holds a different current revision for
// the first change, so the failure cannot be mistaken
// for an already-completed upload.
func TestFailedPushIsItsOwnStage(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	// The server already holds an older current
	// revision for the first change.
	f.land([]revision{{Commit: revC, ChangeID: changeID1}})
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.mu.Unlock()
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("failed push: want an error")
	}
	if !strings.Contains(err.Error(), "push:") {
		t.Errorf("error should name the push stage: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes after a failed push", f.postCount())
	}
}

// The delegated push's exit status is grfa's own exit
// status; it does not collapse to 1 through the error
// text. The failure is genuine: the server holds a
// different current revision.
func TestFailedPushExitStatusFlowsThrough(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	// The server already holds an older current
	// revision for the first change.
	f.land([]revision{{Commit: revC, ChangeID: changeID1}})
	jj.mu.Lock()
	jj.pushErr = &exitCodeError{err: fmt.Errorf("%s: exit status 3", jjBinary), code: 3}
	jj.mu.Unlock()
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("failed push: want an error")
	}
	if !strings.Contains(err.Error(), "push:") {
		t.Errorf("error should still name the push stage: %v", err)
	}
	if got, want := exitStatus(err), 3; got != want {
		t.Errorf("exit status = %d, want %d (the delegated push's status)", got, want)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes after a failed push", f.postCount())
	}
}

// Defect 3, case (d): post-push recovery, scripted honestly as the concurrent case.
// The delegated
// push fails because the change is already current on the server, and the
// read-only Change-Id lookup confirms every revision of the upload set is
// the change's current_revision: the push was a no-op, so upload continues
// to stamping, posts the marker, and exits successfully. The push's stdout
// claims a fresh upload, which must not matter: grfa never parses it.
// Concretely: the pre-push lookup reports both changes absent, the change
// appears before the push is attempted (modeled by the fake's onPush
// landing at push time), so the push is rejected and the recovery confirms it.
func TestUploadRecoversWhenPushAlreadyPresent(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "Uploaded new patch sets, fresh as can be"
	jj.mu.Unlock()
	// A concurrent upload lands the same
	// content just as the push is attempted,
	// so Gerrit rejects ours as redundant.
	jj.onPush = func() { f.land(twoRevisions()) }
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatalf("a push that was already present must be recovered, not an error: %v", err)
	}
	posts, _ := f.snapshot()
	if len(posts) != 2 {
		t.Fatalf("stamps = %d, want 2 (the missing marker is repaired)", len(posts))
	}
	byChange := map[string]recordedPost{}
	for _, p := range posts {
		byChange[p.Change] = p
	}
	if byChange[changeID1].Revision != revA || byChange[changeID2].Revision != revB {
		t.Errorf("stamps went against %q/%q, want the pushed revisions %q/%q",
			byChange[changeID1].Revision, byChange[changeID2].Revision, revA, revB)
	}
	if byChange[changeID1].Body.Comments[patchSetLevel][0].Message != marker42 {
		t.Errorf("marker = %v, want %q", byChange[changeID1].Body.Comments, marker42)
	}
	got := out.String()
	if !strings.Contains(got, "already present") {
		t.Errorf("output should report plainly that the push was already present:\n%s", got)
	}
	if !strings.Contains(got, "stamped "+changeID1) {
		t.Errorf("output should still report the stamp stage:\n%s", got)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want exactly 1 (no retry)", got)
	}
	// The recovery lookup went through the authenticated entrance as a
	// read-only query carrying the CURRENT_REVISION option.
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
		t.Errorf("the recovery lookup must use the change-query endpoint with CURRENT_REVISION: %v", requests)
	}
}

// The change exists but its current revision differs
// from the local commit, so the push genuinely failed: the original error
// surfaces unchanged and nothing is stamped. The push's stdout claims
// success, which must not matter.
func TestUploadSurfacesPushFailureWhenRevisionDiffers(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "push successful, nothing to worry about"
	jj.mu.Unlock()
	// The server holds an older current
	// revision for the first change.
	f.land([]revision{{Commit: revC, ChangeID: changeID1}})
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("differing server revision: want the original push error")
	}
	if !strings.Contains(err.Error(), "push:") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("the original push failure must surface unchanged: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0", f.postCount())
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want exactly 1 (no retry)", got)
	}
	if strings.Contains(out.String(), "already present") || strings.Contains(out.String(), "stamped") {
		t.Errorf("output must not report recovery or stamping:\n%s", out)
	}
}

// The change is absent from the server, so the push
// genuinely failed: the original error surfaces unchanged and nothing is
// stamped.
func TestUploadSurfacesPushFailureWhenChangeAbsent(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.mu.Unlock()
	f.mu.Lock()
	delete(f.details, changeID1)
	f.mu.Unlock()
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("absent change: want the original push error")
	}
	if !strings.Contains(err.Error(), "push:") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("the original push failure must surface unchanged: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0", f.postCount())
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want exactly 1 (no retry)", got)
	}
	if strings.Contains(out.String(), "already present") || strings.Contains(out.String(), "stamped") {
		t.Errorf("output must not report recovery or stamping:\n%s", out)
	}
}

// Post-push recovery, concurrent case: during recovery the push still
// runs exactly once (the pre-push lookup reported the change absent),
// its stdout is never captured or parsed (it runs in pass-through mode),
// and the recorded command log stays read-only queries plus at most one
// push.
func TestRecoveryRunsPushOnceAndNeverParsesStdout(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "Uploaded fix for review: this line is not a contract"
	jj.mu.Unlock()
	// The change appears before the push is
	// attempted, so the push is rejected and
	// the recovery confirms it.
	jj.onPush = func() { f.land(twoRevisions()) }
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	cmds := jj.commandsSnapshot()
	if got := countPushes(cmds); got != 1 {
		t.Fatalf("pushes = %d, want exactly 1: the failed push is never retried", got)
	}
	for _, cmd := range cmds {
		if cmd.name != jjBinary {
			t.Errorf("unexpected subprocess %q", cmd.name)
			continue
		}
		if strings.HasPrefix(jjBody(cmd), "gerrit upload") {
			// Pass-through stdio: the push's stdout goes to the
			// terminal untouched, so grfa cannot have parsed it.
			if !cmd.passthrough {
				t.Errorf("the delegated push must run in pass-through mode: %v", cmd)
			}
			continue
		}
		if len(cmd.args) == 0 || cmd.args[0] != "--ignore-working-copy" {
			t.Errorf("command is not in the read-only stance: %v", cmd.args)
		}
	}
}

// Defect 3, case (a): the pre-push lookup confirms every revision is
// already the server's current patch set, so the delegated push is
// skipped entirely, the report says the revision(s) were already
// present, and stamping still runs.
func TestPrePushLookupSkipsRedundantPush(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	// A previous upload already landed both
	// changes as their current patch sets.
	f.land(twoRevisions())
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 0 {
		t.Errorf("pushes = %d, want 0 (the redundant push must be skipped)", got)
	}
	if got := f.postCount(); got != 2 {
		t.Errorf("stamps = %d, want 2 (stamping continues without the push)", got)
	}
	got := out.String()
	if !strings.Contains(got, "already present") {
		t.Errorf("output should report the revision(s) as already present:\n%s", got)
	}
	if !strings.Contains(got, "stamped "+changeID1) {
		t.Errorf("output should still report the stamp stage:\n%s", got)
	}
	// The pre-push lookup went through the authenticated entrance as a
	// read-only query carrying the CURRENT_REVISION option.
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
		t.Errorf("the pre-push lookup must use the change-query endpoint with CURRENT_REVISION: %v", requests)
	}
	assertOneIdentityCheck(t, f)
}

// Defect 3, case (b): a pre-push lookup failure is never fatal and never
// a skip: it is reported and the push runs, with the rest of the flow
// unchanged.
func TestPrePushLookupErrorStillPushes(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	f.mu.Lock()
	f.statuses["GET /changes"] = http.StatusInternalServerError // the lookup endpoint errors
	f.mu.Unlock()
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatalf("a lookup failure must not abort the upload: %v", err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want 1 (an inconclusive lookup never skips the push)", got)
	}
	if got := f.postCount(); got != 2 {
		t.Errorf("stamps = %d, want 2 (the flow is unchanged)", got)
	}
	if !strings.Contains(out.String(), "pre-push lookup") {
		t.Errorf("output should report the failed lookup, not hide it:\n%s", out.String())
	}
}

// Defect 3, case (c): the pre-push lookup reports the revisions absent,
// so the push runs.
func TestPrePushLookupAbsentStillPushes(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want 1 (absent revisions still push)", got)
	}
	// The lookup actually consulted the server before the push.
	_, requests := f.snapshot()
	var sawQuery bool
	for _, r := range requests {
		if strings.HasPrefix(r, "GET /changes?") && strings.Contains(r, "q=change%3A") {
			sawQuery = true
		}
	}
	if !sawQuery {
		t.Errorf("the pre-push lookup never queried the server: %v", requests)
	}
}

// Defect 2: with a session present, the identity check runs before the
// push. The entrance reports the wrong identity, so the upload aborts
// before any push or stamp; the error names the entrance it tried.
func TestIdentityCheckAbortsBeforePushWrongIdentity(t *testing.T) {
	f, srv := newFakeGerrit(t, "Imposter") // the entrance reports the wrong identity
	_, jj, c, _ := uploadFixtureAt(t, f, srv, "Agent", twoRevisions(), session42, true)
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("wrong identity: want an error")
	}
	if !strings.Contains(err.Error(), "entrance") || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error should name the entrance it tried: %v", err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 0 {
		t.Errorf("recorded commands contain %d pushes, want 0", got)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0", f.postCount())
	}
	if got := f.requestCount(); got != 1 {
		t.Errorf("made %d requests, want 1 (accounts/self only, then abort)", got)
	}
}

// Defect 2: with a session present and the entrance unreachable, the
// identity check's transport failure aborts before any push or stamp;
// the error names the entrance it tried.
func TestIdentityCheckAbortsBeforePushUnreachableEntrance(t *testing.T) {
	f, srv := newFakeGerrit(t, "Agent")
	srv.Close() // the entrance is down
	_, jj, c, _ := uploadFixtureAt(t, f, srv, "Agent", twoRevisions(), session42, true)
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("unreachable entrance: want an error")
	}
	if !strings.Contains(err.Error(), "check identity") || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error should name the entrance it tried: %v", err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 0 {
		t.Errorf("recorded commands contain %d pushes, want 0", got)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0", f.postCount())
	}
	// The entrance is down, so no request can arrive to be recorded;
	// the abort is proven by the error naming it and the empty push log.
}

// Defect 2, the other half of the asymmetry: with YAH_SESSION absent no
// marker will be posted, so the upload must not require API reachability.
// Even a dead entrance cannot abort it; the pre-push lookup failure is
// reported but never fatal.
func TestNoSessionDoesNotRequireAPIReachability(t *testing.T) {
	f, srv := newFakeGerrit(t, "Agent")
	srv.Close() // the entrance is down
	_, jj, c, out := uploadFixtureAt(t, f, srv, "Agent", twoRevisions(), "", false)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatalf("an upload that will never stamp must not require API reachability: %v", err)
	}
	if f.postCount() != 0 {
		t.Errorf("stamped %d changes, want 0 (no session, no marker)", f.postCount())
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want 1 (the push is unaffected)", got)
	}
	if !strings.Contains(out.String(), "pre-push lookup") {
		t.Errorf("output should report the failed lookup:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "no provenance marker") {
		t.Errorf("output should explain the missing marker:\n%s", out.String())
	}
}

// A failed stamp after a successful push is a
// partial result, reported plainly.
func TestFailedStampIsPartialResult(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	f.mu.Lock()
	delete(f.details, changeID2) // the second change cannot be fetched
	f.mu.Unlock()
	err := c.cmdUpload(context.Background(), nil)
	if err == nil {
		t.Fatal("stamp failure: want an error")
	}
	if !strings.Contains(err.Error(), "push succeeded, but stamping failed") {
		t.Errorf("error should report the partial result: %v", err)
	}
	if !strings.Contains(err.Error(), changeID2) {
		t.Errorf("error should name the unstamped change: %v", err)
	}
	if got := countPushes(jj.commandsSnapshot()); got != 1 {
		t.Errorf("pushes = %d, want 1 (the push itself succeeded)", got)
	}
	if f.postCount() != 1 {
		t.Errorf("posts = %d, want 1 (the healthy change was still stamped)", f.postCount())
	}
}

// The session identifier is validated before anything runs.
func TestSessionValidation(t *testing.T) {
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
			f, srv := newFakeGerrit(t, "Agent")
			jj := &jjFake{root: t.TempDir(), defaultRev: "@"}
			c := &cli{
				out:    &bytes.Buffer{},
				api:    testClient(srv, "Agent"),
				runner: jj.runner(),
				getenv: func(k string) (string, bool) {
					if k == "YAH_SESSION" {
						return tc.session, true
					}
					return "", false
				},
			}
			err := c.cmdUpload(context.Background(), nil)
			if err == nil {
				t.Fatal("invalid session: want an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tc.wantErr)
			}
			if len(jj.commandsSnapshot()) != 0 {
				t.Errorf("invalid session must abort before any subprocess")
			}
			if f.requestCount() != 0 {
				t.Errorf("invalid session must abort before any network traffic")
			}
		})
	}
}

// The marker must never appear in process
// arguments: upload is invoked with only
// pass-through hints.
func TestSessionNeverInArgs(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	if err := c.cmdUpload(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range jj.commandsSnapshot() {
		for _, a := range cmd.args {
			if strings.Contains(a, session42) {
				t.Errorf("session id leaked into subprocess args: %v", cmd.args)
			}
		}
	}
}

// The upload-set query records commit
// ids, subjects, and Change-Ids exactly
// as jj reports them.
func TestUploadSetRecords(t *testing.T) {
	jj := &jjFake{root: t.TempDir(), defaultRev: "@"}
	c := &cli{out: &bytes.Buffer{}, api: nil, runner: jj.runner(),
		getenv: func(string) (string, bool) { return "", false }}
	jj.mu.Lock()
	jj.setRevs = []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
		{Commit: revB, ChangeID: "", Subject: ""},
	}
	jj.mu.Unlock()
	revs, _, err := c.resolveUploadSet(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 {
		t.Fatalf("revs = %d, want 2", len(revs))
	}
	if revs[0].ChangeID != changeID1 || revs[0].Subject != "first change" {
		t.Errorf("revs[0] = %+v", revs[0])
	}
	if revs[1].ChangeID != "" {
		t.Errorf("revs[1] = %+v, want no Change-Id", revs[1])
	}
}

// A malformed upload-set answer is an error, not a guess.
func TestUploadSetRejectsGarbage(t *testing.T) {
	jj := &jjFake{root: t.TempDir(), defaultRev: "@"}
	jj.mu.Lock()
	jj.setRevs = nil
	jj.mu.Unlock()
	c := &cli{out: &bytes.Buffer{}, api: nil, runner: &fakeRunner{respond: func(cmd command) (string, error) {
		joined := strings.Join(cmd.args, " ")
		switch {
		case strings.HasPrefix(joined, "root"):
			return "/repo\n", nil
		case strings.Contains(joined, `if(description`):
			return "@\n", nil
		default:
			return "garbage without terminator", nil
		}
	}}, getenv: func(string) (string, bool) { return "", false }}
	if _, _, err := c.resolveUploadSet(context.Background(), ""); err == nil {
		t.Fatal("garbage jj output: want an error")
	}
}

// The two cause clauses the empty-upload-set diagnosis may carry, pinned
// here as literal text rather than read from the production constants, so
// a change to the wording fails the tests that depend on it.
const (
	causeMatchedNothing = "the revset matched no revisions"
	causeLanded         = "the named revisions are already landed and immutable"
)

// mandatedEmptySetSentence is the part of the diagnosis that is owed
// unconditionally: the revset the caller asked for, no internal
// mutable():: query, and no push failure.
func mandatedEmptySetSentence(revset string) string {
	return fmt.Sprintf("no mutable revisions in the upload set (%s); nothing to upload", revset)
}

// emptySetDiagnosis is the exact report an empty upload set owes the
// operator: the mandated sentence, plus — when the cause is known — the
// cause clause naming why the set is empty. An empty cause means the
// mandated sentence stands alone, which is what a failed cause probe owes.
func emptySetDiagnosis(revset, cause string) string {
	s := mandatedEmptySetSentence(revset)
	if cause != "" {
		s += " (" + cause + ")"
	}
	return s
}

// causeProbed reports whether the cause diagnosis ran as a read-only
// probe of the plain revset the caller asked for — never of the internal
// mutable()::(...) query. The probe must also still carry the global
// read-only flags: jjBody strips them before matching, so the revset
// match alone cannot prove the probe was read-only.
func causeProbed(cmds []command, revset string) bool {
	for _, cmd := range cmds {
		if cmd.name == jjBinary && jjBody(cmd) == "log --no-graph -r "+revset+" -T commit_id" {
			return hasReadOnlyGlobalFlags(cmd)
		}
	}
	return false
}

// hookRan reports whether the pre-upload hook was invoked at all.
func hookRan(cmds []command) bool {
	for _, cmd := range cmds {
		if strings.HasSuffix(cmd.name, ".grfa/pre-upload") {
			return true
		}
	}
	return false
}

// assertEmptySetAborted pins the resolution-stage contract for an empty
// upload set: the exact diagnosis naming the requested revset and — when
// cause is non-empty — the cause clause that distinguishes a revset which
// matched nothing from one whose revisions are all immutable, a non-zero
// exit status of its own, no delegated push, no pre-upload hook, and no
// network traffic of any kind. The fake runner never executes the hook
// script — it returns its own hookErr, which these tests leave nil — so the
// detector for a hook that ran is the hookRan command-log assertion.
func assertEmptySetAborted(t *testing.T, f *fakeGerrit, jj *jjFake, err error, out *bytes.Buffer, revset, cause string) {
	t.Helper()
	if err == nil {
		t.Fatalf("empty upload set: want an error, got nil; output:\n%s", out)
	}
	want := emptySetDiagnosis(revset, cause)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	// The mandated sentence is present verbatim whether or not a cause
	// clause follows it.
	if !strings.HasPrefix(err.Error(), mandatedEmptySetSentence(revset)) {
		t.Errorf("error = %q, want it to begin with the mandated sentence %q", err, mandatedEmptySetSentence(revset))
	}
	if strings.Contains(err.Error(), "mutable()::") {
		t.Errorf("error leaks the internal upload-set query: %q", err)
	}
	if strings.Contains(err.Error(), "push:") {
		t.Errorf("the empty upload set must not be reported as a push failure: %q", err)
	}
	if got := exitStatus(err); got == 0 {
		t.Errorf("exitStatus = %d, want non-zero", got)
	}
	cmds := jj.commandsSnapshot()
	if !causeProbed(cmds, revset) {
		t.Errorf("the cause diagnosis did not probe the plain revset %q read-only", revset)
	}
	if got := countPushes(cmds); got != 0 {
		t.Errorf("delegated %d push(es), want 0", got)
	}
	if hookRan(cmds) {
		t.Errorf("the pre-upload hook ran before the empty upload set was reported")
	}
	if got := f.requestCount(); got != 0 {
		t.Errorf("made %d network request(s), want 0", got)
	}
}

// The defect, shape 1: an explicit -r whose set resolves empty (a
// post-merge re-run against an immutable revision) must be reported as
// its own stage naming that revset, before the pre-upload hook and before
// the delegated push — not as jj's raw immutable-commit fault or a bare
// push: jj: exit status 1. A revset that names nothing at all is the
// cause that must be named: the revset matched no revisions.
func TestEmptyUploadSetWithExplicitRevsetStopsBeforePush(t *testing.T) {
	f, jj, c, out := uploadFixture(t, nil, session42, true)
	// The fake runner never executes this script — it returns its own
	// hookErr, which this test leaves nil — so hookRan, not the script,
	// is what detects a hook that ran.
	writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
	err := c.cmdUpload(context.Background(), []string{"-r", "none()"})
	assertEmptySetAborted(t, f, jj, err, out, "none()", causeMatchedNothing)
}

// An explicit -r that does name revisions while the mutable upload set is
// empty is the post-merge re-run: everything named is immutable (already
// landed). The cause clause must say so — telling the operator (or an
// agent) to stop rather than to try to make the commit mutable — and must
// not read like a repository fault.
func TestEmptyUploadSetLandedRevisionReportsCause(t *testing.T) {
	f, jj, c, out := uploadFixture(t, nil, session42, true)
	jj.mu.Lock()
	jj.probeRevs = []string{revA}
	jj.mu.Unlock()
	writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
	err := c.cmdUpload(context.Background(), []string{"-r", revA})
	assertEmptySetAborted(t, f, jj, err, out, revA, causeLanded)
}

// The landed cause is diagnosed identically when YAH_SESSION is absent:
// the cause is a property of the repository, not of the session.
func TestEmptyUploadSetLandedRevisionWithoutSessionReportsCause(t *testing.T) {
	f, jj, c, out := uploadFixture(t, nil, "", false)
	jj.mu.Lock()
	jj.probeRevs = []string{revA}
	jj.mu.Unlock()
	writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
	err := c.cmdUpload(context.Background(), []string{"-r", revA})
	assertEmptySetAborted(t, f, jj, err, out, revA, causeLanded)
}

// A cause probe that fails must not turn the diagnosis into an error and
// must not guess: the mandated sentence stands alone, the exit status
// stays non-zero, and nothing is pushed, hooked, or requested. Both the
// session-present and session-absent paths are covered.
func TestEmptyUploadSetCauseProbeFailureKeepsMandatedSentence(t *testing.T) {
	for _, sessionSet := range []bool{false, true} {
		name := "session-absent"
		session := ""
		if sessionSet {
			name = "session-present"
			session = session42
		}
		t.Run(name, func(t *testing.T) {
			f, jj, c, out := uploadFixture(t, nil, session, sessionSet)
			jj.mu.Lock()
			jj.probeErr = errors.New("probe failed")
			jj.mu.Unlock()
			writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
			err := c.cmdUpload(context.Background(), []string{"-r", "none()"})
			assertEmptySetAborted(t, f, jj, err, out, "none()", "")
		})
	}
}

// An empty upload set is diagnosed identically when YAH_SESSION is absent:
// the abort is at resolution, so it precedes the pre-upload hook and the
// delegated push whether or not a provenance marker would be posted. A
// revset that names nothing is still the matched-nothing cause.
func TestEmptyUploadSetWithoutSessionStopsBeforePush(t *testing.T) {
	f, jj, c, out := uploadFixture(t, nil, "", false)
	writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
	err := c.cmdUpload(context.Background(), []string{"-r", "none()"})
	assertEmptySetAborted(t, f, jj, err, out, "none()", causeMatchedNothing)
}

// The default revset is no exception without YAH_SESSION: the diagnosis
// names the default the resolver chose, @ or @-, and the cause clause
// says the named revisions are already landed and immutable.
func TestEmptyUploadSetWithDefaultRevsetWithoutSession(t *testing.T) {
	for _, def := range []string{"@", "@-"} {
		t.Run(def, func(t *testing.T) {
			f, jj, c, out := uploadFixture(t, nil, "", false)
			jj.mu.Lock()
			jj.defaultRev = def
			jj.probeRevs = []string{revA}
			jj.mu.Unlock()
			writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
			err := c.cmdUpload(context.Background(), nil)
			assertEmptySetAborted(t, f, jj, err, out, def, causeLanded)
		})
	}
}

// The defect, shape 2: a revset that resolves to nothing must not be a
// silent success with jj's No revisions to upload. on stderr. The same
// diagnosis names the default the resolver actually chose, @ or @-, and
// the cause clause for a default that names an immutable parent: the
// named revisions are already landed and immutable.
func TestEmptyUploadSetWithDefaultRevsetStopsBeforePush(t *testing.T) {
	for _, def := range []string{"@", "@-"} {
		t.Run(def, func(t *testing.T) {
			f, jj, c, out := uploadFixture(t, nil, session42, true)
			jj.mu.Lock()
			jj.defaultRev = def
			jj.probeRevs = []string{revA}
			jj.mu.Unlock()
			writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
			err := c.cmdUpload(context.Background(), nil)
			assertEmptySetAborted(t, f, jj, err, out, def, causeLanded)
		})
	}
}

// -dry-run is no exception: an empty upload set is the same diagnosis, not
// dry-run: 0 revision(s) in the upload set. Nothing is pushed, nothing is
// stamped, and the report is non-zero because the requested push cannot
// happen.
func TestEmptyUploadSetDryRunReportsSameDiagnosis(t *testing.T) {
	f, jj, c, out := uploadFixture(t, nil, session42, true)
	writeHook(t, jj.root, "#!/bin/sh\nexit 7\n")
	err := c.cmdUpload(context.Background(), []string{"-r", "none()", "-dry-run"})
	assertEmptySetAborted(t, f, jj, err, out, "none()", causeMatchedNothing)
	if got := out.String(); strings.Contains(got, "dry-run:") {
		t.Errorf("dry-run reported a count for an empty upload set:\n%s", got)
	}
	if f.postCount() != 0 {
		t.Errorf("dry-run with an empty upload set stamped %d time(s), want 0", f.postCount())
	}
}

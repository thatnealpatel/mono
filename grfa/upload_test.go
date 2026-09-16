package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	changeID1 = "Iaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	changeID2 = "Ibbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// uploadFixture wires a client whose fake jj reports the given upload set, and
// seeds the fake Gerrit with both changes.
func uploadFixture(t *testing.T, revs []revision, session string, sessionSet bool) (*fakeGerrit, *jjFake, *cli, *bytes.Buffer) {
	t.Helper()
	f, srv := newFakeGerrit(t, "Agent")
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
// new, and no duplicate push happens for
// unchanged content.
func TestStampIdempotent(t *testing.T) {
	f, jj, c, _ := uploadFixture(t, []revision{
		{Commit: revA, ChangeID: changeID1, Subject: "first change"},
	}, session42, true)
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
		if got := countPushes(cmds); got != run {
			t.Errorf("run %d: cumulative pushes = %d, want %d (one delegated push per run)", run, got, run)
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
	f.mu.Lock()
	f.details[changeID1].CurrentRevision = "cccccccccccccccccccccccccccccccccccccccc"
	f.mu.Unlock()
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
	f.mu.Lock()
	f.details[changeID1].CurrentRevision = "cccccccccccccccccccccccccccccccccccccccc"
	f.mu.Unlock()
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

// Defect 3, case (a): re-running upload is the recovery path. The delegated
// push fails because the change is already current on the server, and the
// read-only Change-Id lookup confirms every revision of the upload set is
// the change's current_revision: the push was a no-op, so upload continues
// to stamping, posts the marker, and exits successfully. The push's stdout
// claims a fresh upload, which must not matter: grfa never parses it.
func TestUploadRecoversWhenPushAlreadyPresent(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "Uploaded new patch sets, fresh as can be"
	jj.mu.Unlock()
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

// Defect 3, case (b): the change exists but its current revision differs
// from the local commit, so the push genuinely failed: the original error
// surfaces unchanged and nothing is stamped. The push's stdout claims
// success, which must not matter.
func TestUploadSurfacesPushFailureWhenRevisionDiffers(t *testing.T) {
	f, jj, c, out := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "push successful, nothing to worry about"
	jj.mu.Unlock()
	f.mu.Lock()
	f.details[changeID1].CurrentRevision = "cccccccccccccccccccccccccccccccccccccccc"
	f.mu.Unlock()
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

// Defect 3, case (c): the change is absent from the server, so the push
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

// Defect 3, case (d): during recovery the push still runs exactly once,
// its stdout is never captured or parsed (it runs in pass-through mode),
// and the recorded command log stays read-only queries plus at most one
// push.
func TestRecoveryRunsPushOnceAndNeverParsesStdout(t *testing.T) {
	_, jj, c, _ := uploadFixture(t, twoRevisions(), session42, true)
	jj.mu.Lock()
	jj.pushErr = errors.New("exit status 1")
	jj.pushStdout = "Uploaded fix for review: this line is not a contract"
	jj.mu.Unlock()
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
	revs, err := c.uploadSet(context.Background(), "")
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
	if _, err := c.uploadSet(context.Background(), ""); err == nil {
		t.Fatal("garbage jj output: want an error")
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// jjBinary is the repository's VCS; grfa
// delegates every repository operation to
// it as a subprocess.
const jjBinary = "jj"

// jjGlobalArgs is the read-only stance every supervised jj
// invocation runs under: an upload or query never snapshots or
// rewrites the working copy.
var jjGlobalArgs = []string{"--ignore-working-copy", "--no-pager"}

// changeIDTrailers matches a Change-Id trailer line in a commit description.
var changeIDTrailers = regexp.MustCompile(`(?m)^Change-Id:[ \t]*(\S+)[ \t]*$`)

// revision is one commit in the resolved upload set.
type revision struct {
	Commit   string
	ChangeID string
	Subject  string
}

// cmdUpload supervises an upload: resolve the upload set, require
// local Change-Id trailers, run the pre-upload hook, check the entrance
// identity when a session will stamp, look the revisions up before the
// push, delegate the push to jj gerrit upload, and stamp the session
// that produced the change. A failed check, a missing Change-Id, a
// failed push, and a failed stamp are distinct stages and are reported
// as such; re-running upload is the recovery path.
func (c *cli) cmdUpload(ctx context.Context, args []string) error {
	opts, err := parseUploadArgs(args)
	if err != nil {
		return err
	}
	session, err := c.sessionMarker()
	if err != nil {
		return err
	}
	root, err := c.repoRoot(ctx)
	if err != nil {
		return err
	}
	revs, err := c.uploadSet(ctx, opts.revset)
	if err != nil {
		return err
	}

	// Require local Change-Id trailers
	// before any network traffic: a
	// Change-Id generated only on the
	// uploaded commit cannot be recovered
	// afterwards, and requiring it
	// locally is what makes attribution
	// deterministic without parsing upload
	// output or performing a Gerrit search.
	var missing []string
	for _, r := range revs {
		if r.ChangeID == "" {
			missing = append(missing, fmt.Sprintf("%s (%q)", shortCommit(r.Commit), r.Subject))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("change-id: %d revision(s) lack a local Change-Id trailer: %s; add the trailer locally before uploading", len(missing), strings.Join(missing, ", "))
	}

	if err := c.runPreUploadHook(ctx, root); err != nil {
		return err
	}

	if opts.dryRun {
		return c.reportDryRun(revs, session)
	}

	// The identity check runs before the push, and only when a provenance
	// marker will actually be posted. The asymmetry is deliberate: when
	// YAH_SESSION is absent no marker is posted, and such an upload must
	// not require API reachability, so no check runs and an unreachable
	// entrance cannot abort it; when a session is present the upload must
	// never reach Gerrit unable to be stamped, so a wrong identity or an
	// unreachable entrance aborts here, before the push, instead of after
	// it. One check precedes every mutation that follows — the push and the
	// stamp — so the stamping path never re-checks.
	if session != "" {
		if err := c.api.checkIdentity(ctx); err != nil {
			return err
		}
	}

	pushArgs := []string{"--ignore-working-copy", "gerrit", "upload"}
	if opts.revset != "" {
		pushArgs = append(pushArgs, "-r", opts.revset)
	}
	if opts.branch != "" {
		pushArgs = append(pushArgs, "-b", opts.branch)
	}
	if opts.remote != "" {
		pushArgs = append(pushArgs, "--remote", opts.remote)
	}
	for _, r := range opts.reviewers {
		pushArgs = append(pushArgs, "--reviewer", r)
	}
	if err := c.pushUpload(ctx, revs, pushArgs); err != nil {
		return err
	}

	if session == "" {
		fmt.Fprintf(c.out, "uploaded %d revision(s); YAH_SESSION not set, no provenance marker posted\n", len(revs))
		return nil
	}
	return c.stampUpload(ctx, revs, session)
}

// pushUpload delegates the push to jj gerrit upload, unless the pre-push
// lookup positively confirms every revision of the upload set is already on
// the server as its change's current patch set: a re-run of a completed
// upload must not delegate a push Gerrit would only reject. A lookup
// failure is never fatal and never a skip — it is reported and the push
// proceeds as usual — and the post-push recovery remains the second chance,
// because a change can appear between the lookup and the push, or the
// lookup itself can fail. The delegated command's output is never parsed.
func (c *cli) pushUpload(ctx context.Context, revs []revision, pushArgs []string) error {
	present, lookupErr := c.pushAlreadyPresent(ctx, revs)
	switch {
	case lookupErr != nil:
		// Only a positive confirmation may skip
		// the push; an inconclusive lookup is
		// reported and the push runs as usual.
		fmt.Fprintf(c.out, "pre-push lookup: %v; pushing as usual\n", lookupErr)
	case present:
		c.reportAlreadyPresent(len(revs))
		return nil
	}
	if err := c.runner.run(ctx, command{name: jjBinary, args: pushArgs, passthrough: true}); err != nil {
		present, lookupErr := c.pushAlreadyPresent(ctx, revs)
		if lookupErr != nil || !present {
			// A genuine push failure, or a lookup
			// that could not say either way: surface
			// the original failure unchanged, with
			// its exit status. Never retry the push,
			// and never invent a success.
			return fmt.Errorf("push: %w", err)
		}
		// Every revision of the upload set is already
		// on the server as its change's current
		// patch set, so the push was the no-op it
		// appears to be: re-running upload is the
		// recovery path, and the stamp is what
		// repairs the partial result.
		c.reportAlreadyPresent(len(revs))
	}
	return nil
}

// reportAlreadyPresent reports plainly that the revision(s) of the upload
// set are already on the server as the current patch set(s).
func (c *cli) reportAlreadyPresent(n int) {
	fmt.Fprintf(c.out, "push: %d revision(s) already present on the server as the current patch set(s); nothing new to push\n", n)
}

// pushAlreadyPresent reports whether every revision of the upload set is
// already on the server as its change's current revision: that is how a
// re-run of an already-completed upload looks, both to the lookup before
// the push and to the recovery after a rejected one. Each revision is
// looked up by its local Change-Id through a read-only query on the
// authenticated entrance, and the server's current_revision is compared
// with the locally known full commit id — the delegated command's output
// is never parsed. A lookup failure is reported as such: it never turns a
// push failure into a success.
func (c *cli) pushAlreadyPresent(ctx context.Context, revs []revision) (bool, error) {
	if len(revs) == 0 {
		return false, nil
	}
	for _, r := range revs {
		currents, err := c.api.queryCurrentRevisions(ctx, r.ChangeID)
		if err != nil {
			return false, err
		}
		present := false
		for _, cur := range currents {
			// A Change-Id can name more than one open
			// change (one per branch); any change whose
			// current revision is this commit makes the
			// push a no-op for this revision.
			if cur == r.Commit {
				present = true
				break
			}
		}
		if !present {
			return false, nil
		}
	}
	return true, nil
}

// sessionMarker returns the provenance
// session from YAH_SESSION. The identifier
// arrives through the environment, never an
// argument, and is treated as opaque; it must
// stay a single line. An absent variable
// means no marker and a successful upload,
// not a guessed one.
func (c *cli) sessionMarker() (string, error) {
	v, ok := c.getenv("YAH_SESSION")
	if !ok {
		return "", nil
	}
	if v == "" {
		return "", fmt.Errorf("YAH_SESSION is set but empty")
	}
	for _, r := range v {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("YAH_SESSION must be a single line: contains whitespace or control characters")
		}
	}
	return v, nil
}

// jj runs a read-only jj query and captures its stdout.
func (c *cli) jj(ctx context.Context, args ...string) (string, error) {
	full := append(append([]string{}, jjGlobalArgs...), args...)
	return c.runner.output(ctx, command{name: jjBinary, args: full})
}

// repoRoot asks the VCS for the repository root.
func (c *cli) repoRoot(ctx context.Context) (string, error) {
	out, err := c.jj(ctx, "root")
	if err != nil {
		return "", fmt.Errorf("find repository root: %w", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("find repository root: empty output")
	}
	return root, nil
}

// uploadSet resolves the revisions jj gerrit upload would push: the given
// revset plus its mutable ancestors, or the VCS default (@ when described, @-
// otherwise) when no revset was given. Resolution is delegated to jj; grfa
// only mirrors the documented default so the Change-Id requirement can be
// checked locally, read-only.
func (c *cli) uploadSet(ctx context.Context, revset string) ([]revision, error) {
	rev := revset
	if rev == "" {
		out, err := c.jj(ctx, "log", "--no-graph", "-r", "@", "-T", `if(description, "@", "@-")`)
		if err != nil {
			return nil, fmt.Errorf("resolve default upload revision: %w", err)
		}
		rev = strings.TrimSpace(out)
		if rev == "" {
			return nil, fmt.Errorf("resolve default upload revision: empty output")
		}
	}
	query := fmt.Sprintf("mutable()::(%s)", rev)
	out, err := c.jj(ctx, "log", "--no-graph", "-r", query, "-T", `commit_id ++ "\x1f" ++ description ++ "\x1f"`)
	if err != nil {
		return nil, fmt.Errorf("resolve upload set %s: %w", query, err)
	}
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil, nil
	}
	fields := strings.Split(out, "\x1f")
	if len(fields)%2 != 1 || fields[len(fields)-1] != "" {
		return nil, fmt.Errorf("resolve upload set %s: unexpected jj output", query)
	}
	var revs []revision
	for i := 0; i+1 < len(fields); i += 2 {
		commit, desc := fields[i], fields[i+1]
		if commit == "" {
			return nil, fmt.Errorf("resolve upload set %s: unexpected jj output", query)
		}
		r := revision{Commit: commit, Subject: firstLine(desc)}
		if m := changeIDTrailers.FindAllStringSubmatch(desc, -1); len(m) > 0 {
			// The Change-Id is a trailer in the
			// last paragraph; take the final
			// match.
			r.ChangeID = m[len(m)-1][1]
		}
		revs = append(revs, r)
	}
	return revs, nil
}

// runPreUploadHook runs <repo>/.grfa/pre-upload from the repository root
// with the inherited environment and passed-through stdio. An absent hook
// runs no checks; a nonzero exit aborts the upload before the push. Hooks
// are check-only: grfa never rewrites files on a hook's behalf, since the
// revision under review is immutable. Hook behavior is repository policy,
// not grfa configuration.
func (c *cli) runPreUploadHook(ctx context.Context, root string) error {
	hook := filepath.Join(root, ".grfa", "pre-upload")
	if _, err := os.Stat(hook); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("pre-upload hook: %w", err)
	}
	if err := c.runner.run(ctx, command{name: hook, dir: root, passthrough: true}); err != nil {
		return fmt.Errorf("pre-upload hook: %w", err)
	}
	return nil
}

// reportDryRun reports the upload set and the changes that would be
// stamped, without pushing or stamping.
func (c *cli) reportDryRun(revs []revision, session string) error {
	fmt.Fprintf(c.out, "dry-run: %d revision(s) in the upload set\n", len(revs))
	for _, r := range revs {
		fmt.Fprintf(c.out, "  %s %s %s\n", r.Commit, r.ChangeID, r.Subject)
	}
	if session == "" {
		fmt.Fprintln(c.out, "no YAH_SESSION: no provenance marker would be posted")
		return nil
	}
	fmt.Fprintf(c.out, "would stamp: Yah-Session: %s\n", session)
	return nil
}

// stampUpload posts the provenance marker for every change in the upload set.
// A failed stamp after a successful push is a partial result and is reported
// plainly.
func (c *cli) stampUpload(ctx context.Context, revs []revision, session string) error {
	// The entrance identity was checked before the push, so the
	// stamp needs no second check.
	marker := "Yah-Session: " + session
	seen := map[string]bool{}
	var problems []string
	for _, r := range revs {
		if seen[r.ChangeID] {
			continue
		}
		seen[r.ChangeID] = true
		stamped, err := c.stampChange(ctx, r.ChangeID, marker)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if stamped {
			fmt.Fprintf(c.out, "stamped %s with %q\n", r.ChangeID, marker)
		} else {
			fmt.Fprintf(c.out, "%s already carries %q\n", r.ChangeID, marker)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("push succeeded, but stamping failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// stampChange posts the marker against the change's current revision, the one
// the push just produced. Stamping is idempotent per revision: an identical
// marker already on that revision is skipped, so a re-run repairs a partial
// stamp without duplicating it.
func (c *cli) stampChange(ctx context.Context, changeID, marker string) (bool, error) {
	detail, err := c.api.fetchChangeDetail(ctx, changeID)
	if err != nil {
		return false, fmt.Errorf("stamp %s: %w", changeID, err)
	}
	comments, err := c.api.fetchChangeComments(ctx, changeID)
	if err != nil {
		return false, fmt.Errorf("stamp %s: %w", changeID, err)
	}
	for _, pc := range flattenComments(comments) {
		if pc.Path == patchSetLevel && pc.Info.Message == marker && pc.Info.CommitID == detail.CurrentRevision {
			return false, nil
		}
	}
	in := &reviewInput{
		Comments: map[string][]commentInput{
			patchSetLevel: {{Message: marker, Unresolved: false}},
		},
	}
	if err := c.api.postReview(ctx, changeID, detail.CurrentRevision, in); err != nil {
		return false, fmt.Errorf("stamp %s: %w", changeID, err)
	}
	return true, nil
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

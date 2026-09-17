package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	jjBinary                  = "jj"
	causeRevsetMatchedNothing = "the revset matched no revisions"
	causeAlreadyLanded        = "the named revisions are already landed and immutable"
)

var changeIDTrailers = regexp.MustCompile(`(?m)^Change-Id:[ \t]*(\S+)[ \t]*$`)

type revision struct {
	Commit   string
	ChangeID string
	Subject  string
}

func cmdUpload(ctx context.Context, host string, args []string) error {
	opts, err := parseUploadArgs(args)
	if err != nil {
		return err
	}
	session, err := sessionMarker()
	if err != nil {
		return err
	}
	root, err := repoRoot(ctx)
	if err != nil {
		return err
	}
	revs, rev, err := resolveUploadSet(ctx, opts.revset)
	if err != nil {
		return err
	}
	if len(revs) == 0 {
		return fmt.Errorf("no mutable revisions in the upload set (%s); nothing to upload%s", rev, emptySetCause(ctx, rev))
	}
	var missing []string
	for _, r := range revs {
		if r.ChangeID == "" {
			missing = append(missing, fmt.Sprintf("%s (%q)", shortCommit(r.Commit), r.Subject))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("change-id: %d revision(s) lack a local Change-Id trailer: %s; add the trailer locally before uploading", len(missing), strings.Join(missing, ", "))
	}

	if err := runPreUploadHook(ctx, root); err != nil {
		return err
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
	if err := pushUpload(ctx, host, revs, pushArgs); err != nil {
		return err
	}

	if session == "" {
		log.Printf("uploaded %d revision(s); YAH_SESSION not set, no provenance marker posted", len(revs))
		return nil
	}
	return stampUpload(ctx, host, revs, session)
}

func pushUpload(ctx context.Context, host string, revs []revision, pushArgs []string) error {
	present, lookupErr := pushAlreadyPresent(ctx, host, revs)
	switch {
	case lookupErr != nil:
		log.Printf("pre-push lookup: %v; pushing as usual", lookupErr)
	case present:
		log.Printf("push: %d revision(s) already present on the server as the current patch set(s); nothing new to push", len(revs))
		return nil
	}
	c := exec.CommandContext(ctx, jjBinary, pushArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		present, lookupErr := pushAlreadyPresent(ctx, host, revs)
		if lookupErr != nil || !present {
			return fmt.Errorf("push: %w", err)
		}
		log.Printf("push: %d revision(s) already present on the server as the current patch set(s); nothing new to push", len(revs))
	}
	return nil
}

func pushAlreadyPresent(ctx context.Context, host string, revs []revision) (bool, error) {
	if len(revs) == 0 {
		return false, nil
	}
	for _, r := range revs {
		currents, err := queryCurrentRevisions(ctx, host, r.ChangeID)
		if err != nil {
			return false, err
		}
		present := false
		for _, cur := range currents {
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

func sessionMarker() (string, error) {
	v, ok := os.LookupEnv("YAH_SESSION")
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

func exitError(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

func repoRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, jjBinary, "--ignore-working-copy", "--no-pager", "root").Output()
	if err != nil {
		return "", fmt.Errorf("find repository root: %w", exitError(err))
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("find repository root: empty output")
	}
	return root, nil
}

func resolveUploadSet(ctx context.Context, revset string) ([]revision, string, error) {
	rev := revset
	if rev == "" {
		out, err := exec.CommandContext(ctx, jjBinary, "--ignore-working-copy", "--no-pager", "log", "--no-graph", "-r", "@", "-T", `if(description, "@", "@-")`).Output()
		if err != nil {
			return nil, "", fmt.Errorf("resolve default upload revision: %w", exitError(err))
		}
		rev = strings.TrimSpace(string(out))
		if rev == "" {
			return nil, "", fmt.Errorf("resolve default upload revision: empty output")
		}
	}
	revs, err := uploadSetOf(ctx, rev)
	if err != nil {
		return nil, "", err
	}
	return revs, rev, nil
}

func emptySetCause(ctx context.Context, rev string) string {
	out, err := exec.CommandContext(ctx, jjBinary, "--ignore-working-copy", "--no-pager", "log", "--no-graph", "-r", rev, "-T", "commit_id").Output()
	if err != nil {
		return ""
	}
	if strings.TrimSpace(string(out)) == "" {
		return " (" + causeRevsetMatchedNothing + ")"
	}
	return " (" + causeAlreadyLanded + ")"
}

func uploadSetOf(ctx context.Context, rev string) ([]revision, error) {
	query := fmt.Sprintf("mutable()::(%s)", rev)
	out, err := exec.CommandContext(ctx, jjBinary, "--ignore-working-copy", "--no-pager", "log", "--no-graph", "-r", query, "-T", `commit_id ++ "\x1f" ++ description ++ "\x1f"`).Output()
	if err != nil {
		return nil, fmt.Errorf("resolve upload set %s: %w", query, exitError(err))
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil, nil
	}
	fields := strings.Split(text, "\x1f")
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
			r.ChangeID = m[len(m)-1][1]
		}
		revs = append(revs, r)
	}
	return revs, nil
}

func runPreUploadHook(ctx context.Context, root string) error {
	hook := filepath.Join(root, ".grfa", "pre-upload")
	if _, err := os.Stat(hook); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("pre-upload hook: %w", err)
	}
	c := exec.CommandContext(ctx, hook)
	c.Dir = root
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("pre-upload hook: %w", err)
	}
	return nil
}

const sessionMarkerPrefix = "Yah-Session: "

func stampUpload(ctx context.Context, host string, revs []revision, session string) error {
	marker := sessionMarkerPrefix + session
	seen := map[string]bool{}
	var problems []string
	for _, r := range revs {
		if seen[r.ChangeID] {
			continue
		}
		seen[r.ChangeID] = true
		stamped, err := stampChange(ctx, host, r.ChangeID, marker)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if stamped {
			log.Printf("stamped %s with %q", r.ChangeID, marker)
		} else {
			log.Printf("%s already carries a Yah-Session marker", r.ChangeID)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("push succeeded, but stamping failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func stampChange(ctx context.Context, host, changeID, marker string) (bool, error) {
	detail, err := fetchChangeDetail(ctx, host, changeID)
	if err != nil {
		return false, fmt.Errorf("stamp %s: %w", changeID, err)
	}
	comments, err := fetchChangeComments(ctx, host, changeID)
	if err != nil {
		return false, fmt.Errorf("stamp %s: %w", changeID, err)
	}
	for _, pc := range flattenComments(comments) {
		if pc.Path == patchSetLevel && strings.HasPrefix(pc.Info.Message, sessionMarkerPrefix) {
			return false, nil
		}
	}
	in := &reviewInput{
		Comments: map[string][]commentInput{
			patchSetLevel: {{Message: marker, Unresolved: false}},
		},
	}
	if err := postReview(ctx, host, changeID, detail.CurrentRevision, in); err != nil {
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

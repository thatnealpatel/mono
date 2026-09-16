package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// patchSetLevel is Gerrit's pseudo-path
// for patch-set-level comment threads:
// a real thread with no file location,
// which is what gives -resolved meaning.
const patchSetLevel = "/PATCHSET_LEVEL"

// cmdView prints the change's status, current revision and patch
// set, labels and votes, unresolved-thread count, published change
// messages, and every published comment across all patch sets.
func (c *cli) cmdView(ctx context.Context, change string) error {
	detail, err := c.api.fetchChangeDetail(ctx, change)
	if err != nil {
		return err
	}
	comments, err := c.api.fetchChangeComments(ctx, change)
	if err != nil {
		return err
	}
	return renderChange(c.out, detail, comments)
}

// cmdComment posts one patch-set-level comment thread, or a reply into an
// existing published thread, and then displays the refreshed change. Approvers
// are rechecked by the refetch; an old approval is never inferred.
func (c *cli) cmdComment(ctx context.Context, change, message string, args []string) error {
	opts, err := parseCommentArgs(args)
	if err != nil {
		return err
	}
	if err := c.api.checkIdentity(ctx); err != nil {
		return err
	}

	var revision, path string
	in := commentInput{
		Message:    message,
		Unresolved: !opts.resolved,
	}
	if opts.reply == "" {
		detail, err := c.api.fetchChangeDetail(ctx, change)
		if err != nil {
			return err
		}
		revision = detail.CurrentRevision
		path = patchSetLevel
	} else {
		comments, err := c.api.fetchChangeComments(ctx, change)
		if err != nil {
			return err
		}
		target, ok := findComment(comments, opts.reply)
		if !ok {
			// Fail without posting rather than searching elsewhere; an id missing from
			// the change-level list is not a target at all.
			return fmt.Errorf("reply: no published comment with id %q (change messages are not reply targets)", opts.reply)
		}
		// Reply against the target comment's
		// own patch set: a reply through
		// revisions/current would carry the
		// original location against a revision
		// where the commented region may no
		// longer exist.
		revision = target.Info.CommitID
		path = target.Path
		in.InReplyTo = target.Info.ID
		// Copy the target's positional
		// metadata; absent fields stay absent
		// instead of being invented. side
		// defaults to REVISION server-side,
		// and line is ignored when range is
		// present, so line and range travel
		// together with the target's own
		// values.
		in.Side = target.Info.Side
		in.Parent = target.Info.Parent
		in.Line = target.Info.Line
		in.Range = target.Info.Range
	}
	if err := c.api.postReview(ctx, change, revision, &reviewInput{
		Comments: map[string][]commentInput{path: {in}},
	}); err != nil {
		return err
	}
	detail, err := c.api.fetchChangeDetail(ctx, change)
	if err != nil {
		return err
	}
	comments, err := c.api.fetchChangeComments(ctx, change)
	if err != nil {
		return err
	}
	return renderChange(c.out, detail, comments)
}

// pathComment is a published comment
// paired with the map key it appeared
// under in the change-level comment
// response.
type pathComment struct {
	Path string
	Info commentInfo
}

// flattenComments returns every published comment, paths sorted for
// deterministic output, comments in server order within a path.
func flattenComments(comments map[string][]commentInfo) []pathComment {
	paths := make([]string, 0, len(comments))
	for p := range comments {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out []pathComment
	for _, p := range paths {
		for _, ci := range comments[p] {
			out = append(out, pathComment{Path: p, Info: ci})
		}
	}
	return out
}

// findComment matches a comment id verbatim, exactly as view printed it,
// against the decoded change-level comment list. IDs are opaque strings: never
// normalized, re-cased, or re-encoded.
func findComment(comments map[string][]commentInfo, id string) (pathComment, bool) {
	for _, pc := range flattenComments(comments) {
		if pc.Info.ID == id {
			return pc, true
		}
	}
	return pathComment{}, false
}

// unresolvedThreads reports the unresolved-thread count. The server's count
// is authoritative when present; otherwise it is derived from the published
// threads, whose resolution state lives in their chronologically last comment.
func unresolvedThreads(d *changeDetail, comments map[string][]commentInfo) int {
	if d.UnresolvedCommentCount != nil {
		return *d.UnresolvedCommentCount
	}
	replied := map[string]bool{}
	for _, list := range comments {
		for _, ci := range list {
			if ci.InReplyTo != "" {
				replied[ci.InReplyTo] = true
			}
		}
	}
	n := 0
	for _, list := range comments {
		for _, ci := range list {
			if !replied[ci.ID] && ci.Unresolved {
				n++
			}
		}
	}
	return n
}

// accountName renders an account
// for display, preferring the most
// human-readable field the server
// provided.
func accountName(a *accountInfo) string {
	if a == nil {
		return "unknown"
	}
	if v := firstNonEmpty(a.Username, a.Name, a.Email); v != "" {
		return v
	}
	if a.ID != nil {
		return fmt.Sprintf("account-%d", *a.ID)
	}
	return "unknown"
}

// renderChange prints the change in a compact, deterministic layout: comment
// ids are printed exactly as received, since they are the only handle -reply
// accepts.
func renderChange(w io.Writer, d *changeDetail, comments map[string][]commentInfo) error {
	var b strings.Builder
	fmt.Fprintf(&b, "change %s (#%d) %s %q\n", d.ID, d.Number, d.Status, d.Subject)
	patchSet := 0
	if rev, ok := d.Revisions[d.CurrentRevision]; ok {
		patchSet = rev.Number
	}
	fmt.Fprintf(&b, "revision %s patch-set %d\n", d.CurrentRevision, patchSet)
	fmt.Fprintf(&b, "unresolved-threads %d\n", unresolvedThreads(d, comments))
	b.WriteString("labels\n")
	for _, name := range sortedKeys(d.Labels) {
		l := d.Labels[name]
		// Every entry of a label's all is a voter, including an
		// account permitted to vote that has not yet: its identity
		// and vote value — 0 included — is the label's real state.
		votes := make([]string, 0, len(l.All))
		for _, v := range l.All {
			votes = append(votes, fmt.Sprintf("%s=%+d", accountName(&v.accountInfo), v.Value))
		}
		if len(votes) > 0 {
			fmt.Fprintf(&b, "  %s %+d (%s)\n", name, l.Value, strings.Join(votes, ", "))
		} else {
			fmt.Fprintf(&b, "  %s %+d\n", name, l.Value)
		}
	}
	b.WriteString("messages\n")
	for _, m := range d.Messages {
		fmt.Fprintf(&b, "  %s: %s\n", accountName(m.Author), strings.ReplaceAll(m.Message, "\n", " "))
	}
	b.WriteString("comments\n")
	for _, pc := range flattenComments(comments) {
		c := pc.Info
		fmt.Fprintf(&b, "  %s patch-set %d", pc.Path, c.PatchSet)
		if c.CommitID != "" {
			fmt.Fprintf(&b, " revision %s", c.CommitID)
		}
		fmt.Fprintf(&b, " id %s", c.ID)
		if c.InReplyTo != "" {
			fmt.Fprintf(&b, " reply-to %s", c.InReplyTo)
		}
		// side=PARENT alone does not
		// distinguish parent 1 from parent
		// 2, so the merge-parent number is
		// printed too.
		if c.Side != "" {
			fmt.Fprintf(&b, " side %s", c.Side)
		}
		if c.Parent != 0 {
			fmt.Fprintf(&b, " parent %d", c.Parent)
		}
		if c.Line != 0 {
			fmt.Fprintf(&b, " line %d", c.Line)
		}
		if c.Range != nil {
			fmt.Fprintf(&b, " range %d:%d-%d:%d",
				c.Range.StartLine, c.Range.StartChar, c.Range.EndLine, c.Range.EndChar)
		}
		if c.Unresolved {
			b.WriteString(" unresolved")
		} else {
			b.WriteString(" resolved")
		}
		fmt.Fprintf(&b, " author %s\n", accountName(c.Author))
		for line := range strings.SplitSeq(c.Message, "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

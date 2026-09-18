package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
)

const patchSetLevel = "/PATCHSET_LEVEL"

func cmdView(ctx context.Context, proxy, change string) error {
	detail, err := fetchChangeDetail(ctx, proxy, change)
	if err != nil {
		return err
	}
	comments, err := fetchChangeComments(ctx, proxy, change)
	if err != nil {
		return err
	}
	renderChange(detail, comments)
	return nil
}

func cmdComment(ctx context.Context, proxy, change string, args []string) error {
	opts, message, err := parseCommentArgs(args)
	if err != nil {
		return err
	}

	var revision, path string
	in := commentInput{
		Message:    message,
		Unresolved: !opts.resolved,
	}
	if opts.reply == "" {
		detail, err := fetchChangeDetail(ctx, proxy, change)
		if err != nil {
			return err
		}
		revision = detail.CurrentRevision
		path = patchSetLevel
	} else {
		comments, err := fetchChangeComments(ctx, proxy, change)
		if err != nil {
			return err
		}
		target, ok := findComment(comments, opts.reply)
		if !ok {
			return fmt.Errorf("reply: no published comment with id %q (change messages are not reply targets)", opts.reply)
		}
		revision = target.Info.CommitID
		path = target.Path
		in.InReplyTo = target.Info.ID
		in.Side = target.Info.Side
		in.Parent = target.Info.Parent
		in.Line = target.Info.Line
		in.Range = target.Info.Range
	}
	if err := postReview(ctx, proxy, change, revision, &reviewInput{
		Comments: map[string][]commentInput{path: {in}},
	}); err != nil {
		return err
	}
	detail, err := fetchChangeDetail(ctx, proxy, change)
	if err != nil {
		return err
	}
	comments, err := fetchChangeComments(ctx, proxy, change)
	if err != nil {
		return err
	}
	renderChange(detail, comments)
	return nil
}

type pathComment struct {
	Path string
	Info commentInfo
}

func flattenComments(comments map[string][]commentInfo) []pathComment {
	paths := make([]string, 0, len(comments))
	for p := range comments {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	var out []pathComment
	for _, p := range paths {
		for _, ci := range comments[p] {
			out = append(out, pathComment{Path: p, Info: ci})
		}
	}
	return out
}

func findComment(comments map[string][]commentInfo, id string) (pathComment, bool) {
	for _, pc := range flattenComments(comments) {
		if pc.Info.ID == id {
			return pc, true
		}
	}
	return pathComment{}, false
}

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

func accountName(a *accountInfo) string {
	if a == nil {
		return "unknown"
	}
	if v := cmp.Or(a.Username, a.Name, a.Email); v != "" {
		return v
	}
	if a.ID != nil {
		return fmt.Sprintf("account-%d", *a.ID)
	}
	return "unknown"
}

func renderChange(d *changeDetail, comments map[string][]commentInfo) {
	var b strings.Builder
	fmt.Fprintf(&b, "change %s (#%d) %s %q\n", d.ID, d.Number, d.Status, d.Subject)
	patchSet := 0
	if rev, ok := d.Revisions[d.CurrentRevision]; ok {
		patchSet = rev.Number
	}
	fmt.Fprintf(&b, "revision %s patch-set %d\n", d.CurrentRevision, patchSet)
	fmt.Fprintf(&b, "unresolved-threads %d\n", unresolvedThreads(d, comments))
	b.WriteString("labels\n")
	for _, name := range slices.Sorted(maps.Keys(d.Labels)) {
		l := d.Labels[name]
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
	log.Print(b.String())
}

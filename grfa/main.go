// Package main implements grfa, a small Gerrit CLI for agents.
//
// It is meant to be a thin replacement for .git/hooks/pre-commit
// and other hooks. It currently hardcodes a post-upload hook that
// writes the value of `YAH_SESSION` if it is non-empty into a new
// resolved comment on each new CL.
//
// patel.codes/grfa assumes the user is untrusted: Trust and safety
// are the caller's responsibility.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	host := os.Getenv("GRFA_HOST")
	if host == "" {
		log.Fatal("grfa: GRFA_HOST is not set")
	}
	if err := run(context.Background(), host, os.Args[1:]); err != nil {
		log.Printf("grfa: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, host string, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return nil
	}
	if args[0] == "upload" {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		return usageOnHelp(cmdUpload(ctx, host, args[1:]))
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("unknown flag %q\n\n%s", args[0], usage)
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: grfa <change> <view|comment> [args]\n\n%s", usage)
	}
	change, verb, rest := args[0], args[1], args[2:]
	switch verb {
	case "view":
		if len(rest) != 0 {
			return fmt.Errorf("usage: grfa <change> view (takes no arguments)")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cmdView(ctx, host, change)
	case "comment":
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return usageOnHelp(cmdComment(ctx, host, change, rest))
	default:
		return fmt.Errorf("unknown command %q\n\n%s", verb, usage)
	}
}

func usageOnHelp(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(usage)
		return nil
	}
	return err
}

const usage = `usage: grfa <command> [args]

upload (repository-scoped):
  grfa upload [-r <revset>] [-b <branch>] [-remote <name>] [-reviewer <name>]

change-scoped; <change> is a Change-Id, change number, or project~branch~Change-Id:
  grfa <change> view
  grfa <change> comment [-reply <comment-id>] [-resolved] <message>
`

type uploadOptions struct {
	revset    string
	branch    string
	remote    string
	reviewers []string
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseUploadArgs(args []string) (*uploadOptions, error) {
	opts := &uploadOptions{}
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.revset, "r", "", "revset to upload")
	fs.StringVar(&opts.branch, "b", "", "target branch")
	fs.StringVar(&opts.remote, "remote", "", "remote name")
	fs.Var((*stringList)(&opts.reviewers), "reviewer", "reviewer to add (repeatable)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, fmt.Errorf("upload takes no positional arguments, got %q", fs.Args())
	}
	return opts, nil
}

type commentOptions struct {
	reply    string
	resolved bool
}

func parseCommentArgs(args []string) (*commentOptions, string, error) {
	opts := &commentOptions{}
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.reply, "reply", "", "comment id to reply to")
	fs.BoolVar(&opts.resolved, "resolved", false, "mark the comment thread resolved")
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	if fs.NArg() != 1 {
		return nil, "", fmt.Errorf("comment requires exactly one message argument")
	}
	return opts, fs.Arg(0), nil
}

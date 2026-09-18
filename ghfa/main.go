// Package main implements a subset of the
// GitHub https://cli.github.com/manual/gh.
//
// patel.codes/ghfa should NOT be treated
// as a security boundary; it merely forces
// all gh-shaped traffic through a proxy
// that allows for more desirable control
// over what agents are permitted to do.
//
// A few choice edits were made to the gh
// syntax; however, it should largely mirror
// the upstream CLI.
//
// GHFA_PROXY supplies the proxy base URL (scheme://host:port) to
// which the GitHub routes are appended.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	proxy := os.Getenv("GHFA_PROXY")
	if proxy == "" {
		log.Fatal("ghfa: GHFA_PROXY is required; ghfa refuses to make requests without a proxy")
	}
	if err := run(context.Background(), proxy, os.Stdout, os.Args[1:]); err != nil {
		log.Fatalf("ghfa: %v", err)
	}
}

func run(ctx context.Context, proxy string, out io.Writer, args []string) error {
	if len(args) == 0 || args[0] == "-h" {
		_, err := fmt.Fprint(out, usage)
		return err
	}
	// Search takes no positional owner/repo prefix.
	if len(args) >= 2 && args[0] == "search" && args[1] == "issues" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cmdSearchIssues(ctx, proxy, out, args[2:])
	}
	if !strings.Contains(args[0], "/") {
		return fmt.Errorf("first argument must be <owner/repo>, got %q\n\n%s", args[0], usage)
	}
	repo := args[0]
	rest := args[1:]
	if len(rest) == 0 {
		_, err := fmt.Fprint(out, usage)
		return err
	}
	resource := rest[0]
	rest = rest[1:]
	if len(rest) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> %s <verb> [args]", resource)
	}
	verb := rest[0]
	rest = rest[1:]
	name := resource + " " + verb

	timeout := 30 * time.Second
	if name == "repo clone" {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch name {
	case "issue view":
		return cmdIssueView(ctx, proxy, repo, out, rest)
	case "issue create":
		return cmdIssueCreate(ctx, proxy, repo, out, rest)
	case "issue edit":
		return cmdIssueEdit(ctx, proxy, repo, out, rest)
	case "issue close":
		return cmdIssueClose(ctx, proxy, repo, out, rest)
	case "issue reopen":
		return cmdIssueReopen(ctx, proxy, repo, out, rest)
	case "issue comment":
		return cmdIssueComment(ctx, proxy, repo, out, rest)
	case "search issues":
		return cmdSearchIssues(ctx, proxy, out, rest)
	case "label list":
		return cmdLabelList(ctx, proxy, repo, out, rest)
	case "repo fork":
		return cmdRepoFork(ctx, proxy, repo, out, rest)
	case "repo clone":
		return cmdRepoClone(ctx, proxy, repo, out, rest)
	case "repo sync":
		return cmdRepoSync(ctx, proxy, repo, out, rest)
	case "pr create":
		return cmdPRCreate(ctx, proxy, repo, out, rest)
	default:
		return fmt.Errorf("unknown command %q\ncommands: issue view, issue create, issue edit, issue close, issue reopen, issue comment, search issues, label list, repo fork, repo clone, repo sync, pr create", name)
	}
}

const usage = `usage: ghfa <owner/repo> <resource> <verb> [args]

issue:
  issue view <num>                    show issue with timeline
  issue create -title [-body|-file] [-label]  create an issue
  issue edit <num> [-title] [-body]   edit an issue
  issue close <num> [-r completed|"not planned"] [-dupeof N]
  issue reopen <num> [-c <comment>]   reopen an issue
  issue comment <num> [-body|-file]   post a comment
label:
  label list                          list repository labels
repo:
  repo fork                           fork the repository
  repo clone [<dir>]                  clone via proxy smart HTTP
  repo sync [-branch <name>]          sync fork from upstream (default: main)
pr:
  pr create -title -head -base [-body|-file]  create a cross-repo PR
search:
  search issues <query>               search issues (raw query, no repo scope)
`

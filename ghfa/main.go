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
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	if err := initClient(); err != nil {
		log.Fatalf("ghfa: %v", err)
	}
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("ghfa: %v", err)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" {
		fmt.Fprint(os.Stdout, usage)
		return nil
	}
	// Commands that take positional <repo>, not the global owner/repo prefix.
	if len(args) >= 2 && args[0] == "search" && args[1] == "issues" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return cmdSearchIssues(ctx, args[2:])
	}
	if len(args) >= 2 && args[0] == "repo" && args[1] == "clone" {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		return cmdRepoClone(ctx, args[2:])
	}
	if !strings.Contains(args[0], "/") {
		return fmt.Errorf("first argument must be <owner/repo>, got %q\n\n%s", args[0], usage)
	}
	upstream = args[0]
	rest := args[1:]
	if len(rest) == 0 {
		fmt.Fprint(os.Stdout, usage)
		return nil
	}
	resource := rest[0]
	rest = rest[1:]
	if len(rest) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> %s <verb> [args]", resource)
	}
	verb := rest[0]
	rest = rest[1:]
	key := resource + " " + verb
	for _, cmd := range commands {
		if cmd.name == key {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return cmd.run(ctx, rest)
		}
	}
	return usageError(key)
}

func usageError(name string) error {
	names := make([]string, len(commands))
	for i, cmd := range commands {
		names[i] = cmd.name
	}
	return fmt.Errorf("unknown command %q\ncommands: %s", name, strings.Join(names, ", "))
}

const usage = `usage: ghfa <owner/repo> <resource> <verb> [args]

issue:
  issue view <num>                    show issue with comments
  issue create -title [-body|-file] [-label]  create an issue
  issue edit <num> [-title] [-body]   edit an issue
  issue close <num> [-r completed|"not planned"] [-dupeof N]
  issue reopen <num> [-c <comment>]   reopen an issue
  issue comment <num> [-body|-file]   post a comment
label:
  label list                          list repository labels
repo:
  repo fork                           fork the repository
  repo clone <owner/repo> [<dir>]     clone via proxy smart HTTP
  repo sync [-branch <name>]          sync fork from upstream (default: main)
pr:
  pr create -title -head -base [-body|-file]  create a cross-repo PR
search:
  search issues <query>               search issues (raw query, no repo scope)
`

var commands = []command{
	{"issue view", cmdIssueView},
	{"issue create", cmdIssueCreate},
	{"issue edit", cmdIssueEdit},
	{"issue close", cmdIssueClose},
	{"issue reopen", cmdIssueReopen},
	{"issue comment", cmdIssueComment},
	{"search issues", cmdSearchIssues},
	{"label list", cmdLabelList},
	{"repo fork", cmdRepoFork},
	{"repo sync", cmdRepoSync},
	{"pr create", cmdPRCreate},
}

type command struct {
	name string
	run  func(ctx context.Context, args []string) error
}

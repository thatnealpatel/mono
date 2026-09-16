// Package main implements grfa, a small Gerrit review CLI for
// harness agents. It is the single place an upload is checked and
// attributed: it runs the repository's pre-upload checks, delegates
// the push to the repository's jj gerrit upload, and stamps the
// session that produced the change. Everything else is a narrow,
// read-mostly view of one change through the authenticated
// /gerrit/a entrance.
//
// patel.codes/grfa is not a security boundary: authority is
// enforced by the Gerrit server behind the selected endpoint.
// Environment variables are routing hints, not authorization.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	c := &cli{
		out:    os.Stdout,
		api:    newClient(),
		runner: execRunner{},
		getenv: os.LookupEnv,
	}
	if err := c.run(context.Background(), os.Args[1:]); err != nil {
		log.Printf("grfa: %v", err)
		os.Exit(exitStatus(err))
	}
}

// cli bundles the process-wide collaborators so tests can inject
// the HTTP client, the VCS command runner, and the environment
// without touching the real ones.
type cli struct {
	out    io.Writer
	api    *gerritClient
	runner runner
	getenv func(string) (string, bool)
}

// run dispatches one grfa invocation. upload is the only
// repository-scoped command; every other command is scoped to the
// <change> named as the first argument.
func (c *cli) run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(c.out, usage)
		return nil
	}
	if args[0] == "upload" {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		return c.cmdUpload(ctx, args[1:])
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
		return c.cmdView(ctx, change)
	case "comment":
		if len(rest) == 0 {
			return fmt.Errorf("usage: grfa <change> comment <message> [-reply <comment-id>] [-resolved]")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		// The message is always the first operand, taken
		// verbatim, so option-like text stays unambiguous.
		return c.cmdComment(ctx, change, rest[0], rest[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", verb, usage)
	}
}

const usage = `usage: grfa <command> [args]

upload (repository-scoped):
  grfa upload [-r <revset>] [-b <branch>] [-remote <name>] [-reviewer <name>] [-dry-run]

change-scoped; <change> is a Change-Id, change number, or project~branch~Change-Id:
  grfa <change> view
  grfa <change> comment <message> [-reply <comment-id>] [-resolved]
`

// uploadOptions are the pass-through hints for jj gerrit upload.
// Absent flags leave the choice to the repository's VCS
// configuration.
type uploadOptions struct {
	revset    string
	branch    string
	remote    string
	reviewers []string
	dryRun    bool
}

// parseUploadArgs parses upload's flag surface. Unknown flags,
// missing operands, and duplicate options are usage errors.
func parseUploadArgs(args []string) (*uploadOptions, error) {
	opts := &uploadOptions{}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-r", "-b", "-remote":
			if seen[a] {
				return nil, fmt.Errorf("duplicate flag %s", a)
			}
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %s requires a value", a)
			}
			i++
			switch a {
			case "-r":
				opts.revset = args[i]
			case "-b":
				opts.branch = args[i]
			case "-remote":
				opts.remote = args[i]
			}
			seen[a] = true
		case "-reviewer":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag -reviewer requires a value")
			}
			i++
			opts.reviewers = append(opts.reviewers, args[i])
		case "-dry-run":
			if seen[a] {
				return nil, fmt.Errorf("duplicate flag -dry-run")
			}
			seen[a] = true
			opts.dryRun = true
		default:
			return nil, fmt.Errorf("unknown flag %q\n\n%s", a, usage)
		}
	}
	return opts, nil
}

// commentOptions are comment's optional flags.
type commentOptions struct {
	reply    string
	resolved bool
}

// parseCommentArgs parses the flags that may follow the message.
func parseCommentArgs(args []string) (*commentOptions, error) {
	opts := &commentOptions{}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-reply":
			if seen[a] {
				return nil, fmt.Errorf("duplicate flag -reply")
			}
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag -reply requires a value")
			}
			i++
			// An explicitly empty value is not an absent flag:
			// a blank id would fall through to a brand-new
			// comment instead of a reply.
			if args[i] == "" {
				return nil, fmt.Errorf("flag -reply requires a non-empty comment id")
			}
			opts.reply = args[i]
			seen[a] = true
		case "-resolved":
			if seen[a] {
				return nil, fmt.Errorf("duplicate flag -resolved")
			}
			seen[a] = true
			opts.resolved = true
		default:
			return nil, fmt.Errorf("unknown flag %q", a)
		}
	}
	return opts, nil
}

// command is one supervised subprocess invocation.
type command struct {
	name string
	args []string
	// dir is the working directory; empty inherits the parent's.
	dir string
	// passthrough wires the child's stdin, stdout, and stderr to
	// the parent's instead of capturing them.
	passthrough bool
}

// runner is the injectable process boundary to the repository's
// VCS and pre-upload hook. Tests replace it so the suite never
// needs a real jj.
type runner interface {
	output(ctx context.Context, cmd command) (string, error)
	run(ctx context.Context, cmd command) error
}

// execRunner executes real subprocesses.
type execRunner struct{}

func startCommand(ctx context.Context, cmd command) *exec.Cmd {
	c := exec.CommandContext(ctx, cmd.name, cmd.args...)
	if cmd.dir != "" {
		c.Dir = cmd.dir
	}
	if cmd.passthrough {
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}
	return c
}

// output captures stdout for read-only queries; a failing command
// reports its stderr.
func (execRunner) output(ctx context.Context, cmd command) (string, error) {
	c := startCommand(ctx, cmd)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %v: %s", cmd.name, err, truncate(msg, 4<<10))
	}
	return stdout.String(), nil
}

// exitCodeError carries a delegated command's exit status so it
// flows through grfa's own exit status instead of collapsing
// every failure to 1. The wrapped error keeps the original
// message and stage prefix intact.
type exitCodeError struct {
	err  error
	code int
}

func (e *exitCodeError) Error() string { return e.err.Error() }

func (e *exitCodeError) Unwrap() error { return e.err }

// exitStatus reports the status grfa should exit with for err:
// a delegated command's status when it carries one, otherwise 1.
func exitStatus(err error) int {
	var ec *exitCodeError
	if errors.As(err, &ec) && ec.code >= 0 {
		return ec.code
	}
	return 1
}

// run passes stdio through and returns the exit status unchanged.
func (execRunner) run(ctx context.Context, cmd command) error {
	c := startCommand(ctx, cmd)
	if err := c.Run(); err != nil {
		wrapped := fmt.Errorf("%s: %w", cmd.name, err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code >= 0 {
				return &exitCodeError{err: wrapped, code: code}
			}
		}
		return wrapped
	}
	return nil
}

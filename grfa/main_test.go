package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// emptyGetenv reports no environment at all.
func emptyGetenv(string) (string, bool) { return "", false }

// Acceptance 7: bad arguments fail without any network
// mutation. Every case must produce an error before a
// single HTTP request.
func TestBadArgumentsFailWithoutNetwork(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"view with extra operand", []string{changeKey, "view", "extra"}},
		{"comment without message", []string{changeKey, "comment"}},
		{"comment unknown flag", []string{changeKey, "comment", "hi", "-bogus"}},
		{"comment missing reply value", []string{changeKey, "comment", "hi", "-reply"}},
		{"comment empty reply value", []string{changeKey, "comment", "hi", "-reply", ""}},
		{"comment duplicate reply", []string{changeKey, "comment", "hi", "-reply", "a", "-reply", "b"}},
		{"comment duplicate resolved", []string{changeKey, "comment", "hi", "-resolved", "-resolved"}},
		{"unknown verb", []string{changeKey, "frobnicate"}},
		{"missing verb", []string{changeKey}},
		{"unknown top-level flag", []string{"-bogus", "view"}},
		{"upload unknown flag", []string{"upload", "-x"}},
		{"upload missing -r value", []string{"upload", "-r"}},
		{"upload duplicate -r", []string{"upload", "-r", "a", "-r", "b"}},
		{"upload duplicate -b", []string{"upload", "-b", "x", "-b", "y"}},
		{"upload duplicate -remote", []string{"upload", "-remote", "a", "-remote", "b"}},
		{"upload duplicate -dry-run", []string{"upload", "-dry-run", "-dry-run"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c, _ := seededClient(t, "Agent")
			err := c.run(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("args %v: want a usage error", tc.args)
			}
			if f.requestCount() != 0 {
				t.Errorf("args %v: made %d HTTP requests, want 0", tc.args, f.requestCount())
			}
		})
	}
}

// Argument errors must also abort before any
// subprocess runs.
func TestBadArgumentsRunNoSubprocess(t *testing.T) {
	runner := refusingRunner()
	c := &cli{out: &bytes.Buffer{}, api: nil, runner: runner, getenv: emptyGetenv}
	if err := c.run(context.Background(), []string{"upload", "-r", "a", "-r", "b"}); err == nil {
		t.Fatal("duplicate -r: want an error")
	}
	if len(runner.snapshot()) != 0 {
		t.Errorf("argument errors must abort before any subprocess")
	}
}

// A real delegated subprocess failure carries its
// own exit status, and a plain error still means
// exit status 1.
func TestExecRunnerRunCarriesExitStatus(t *testing.T) {
	r := execRunner{}
	err := r.run(context.Background(), command{name: "sh", args: []string{"-c", "exit 7"}})
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("run error = %v, want an exitCodeError", err)
	}
	if ec.code != 7 {
		t.Errorf("code = %d, want 7", ec.code)
	}
	if got, want := err.Error(), "sh: exit status 7"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if got := exitStatus(err); got != 7 {
		t.Errorf("exitStatus = %d, want 7", got)
	}
	if got := exitStatus(errors.New("boom")); got != 1 {
		t.Errorf("exitStatus(plain error) = %d, want 1", got)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"-h"}, {"--help"}} {
		c := &cli{out: &bytes.Buffer{}, api: nil, runner: refusingRunner(), getenv: emptyGetenv}
		if err := c.run(context.Background(), args); err != nil {
			t.Errorf("args %v: %v", args, err)
		}
	}
}

func TestUsagePrintsCommandSurface(t *testing.T) {
	out := &bytes.Buffer{}
	c := &cli{out: out, api: nil, runner: refusingRunner(), getenv: emptyGetenv}
	if err := c.run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"grfa upload", "grfa <change> view", "grfa <change> comment"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("usage missing %q:\n%s", want, out)
		}
	}
}

// A message that looks like a flag is still
// the message when it is the first operand:
// it is only the following arguments that
// are parsed as flags.
func TestMessageTakenVerbatim(t *testing.T) {
	f, c, _ := seededClient(t, "Agent")
	if err := c.run(context.Background(), []string{changeKey, "comment", "-resolved", "extra"}); err == nil {
		// "-resolved" is the message; "extra" is an unknown flag.
		t.Fatal("want an unknown-flag error for the trailing operand")
	}
	if f.postCount() != 0 {
		t.Errorf("no post may follow a usage error")
	}
}

func TestViewDispatch(t *testing.T) {
	f, c, out := seededClient(t, "Agent")
	if err := c.run(context.Background(), []string{changeKey, "view"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "id c1") {
		t.Errorf("view did not render:\n%s", out)
	}
	if f.requestCount() != 2 {
		t.Errorf("requests = %d, want 2", f.requestCount())
	}
}

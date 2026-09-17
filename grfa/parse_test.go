package main

import (
	"errors"
	"flag"
	"testing"
)

func TestParseUploadArgs(t *testing.T) {
	opts, err := parseUploadArgs([]string{"-r", "@", "-b", "main", "-remote", "gerrit", "-reviewer", "alice", "-reviewer", "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.revset != "@" || opts.branch != "main" || opts.remote != "gerrit" {
		t.Errorf("opts = %+v", opts)
	}
	if len(opts.reviewers) != 2 || opts.reviewers[0] != "alice" || opts.reviewers[1] != "bob" {
		t.Errorf("reviewers = %v, want [alice bob]", opts.reviewers)
	}
}

func TestParseUploadArgsRejectsPositional(t *testing.T) {
	if _, err := parseUploadArgs([]string{"stray"}); err == nil {
		t.Fatal("a positional argument must be rejected")
	}
}

func TestParseCommentArgs(t *testing.T) {
	opts, message, err := parseCommentArgs([]string{"-reply", "c1", "-resolved", "hello there"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.reply != "c1" || !opts.resolved || message != "hello there" {
		t.Errorf("opts = %+v message = %q", opts, message)
	}
}

func TestParseCommentArgsRequiresOneMessage(t *testing.T) {
	for _, args := range [][]string{nil, {"-resolved"}, {"one", "two"}} {
		if _, _, err := parseCommentArgs(args); err == nil {
			t.Errorf("parseCommentArgs(%q) must require exactly one message", args)
		}
	}
}

func TestParseFlagsHelp(t *testing.T) {
	if _, err := parseUploadArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("upload -h = %v, want flag.ErrHelp", err)
	}
	if _, _, err := parseCommentArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("comment -h = %v, want flag.ErrHelp", err)
	}
}

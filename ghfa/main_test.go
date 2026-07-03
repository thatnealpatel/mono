package main

import (
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-h"},
		{"owner/repo"},
	} {
		if err := run(args); err != nil {
			t.Errorf("run(%v) = %v, want nil", args, err)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"owner/repo", "bogus", "cmd"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %q, want it to contain 'unknown command'", err)
	}
}

func TestUsageError(t *testing.T) {
	err := usageError("bogus cmd")
	msg := err.Error()
	if !strings.Contains(msg, `"bogus cmd"`) {
		t.Errorf("error = %q, want it to contain the command name", msg)
	}
	for _, cmd := range commands {
		if !strings.Contains(msg, cmd.name) {
			t.Errorf("error = %q, want it to list %q", msg, cmd.name)
		}
	}
}

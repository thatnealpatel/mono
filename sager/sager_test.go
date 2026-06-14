package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestEvalExpr exercises the full -c passthrough path
// against a live container by invoking docker directly,
// mirroring what evalExpr does. It skips when the
// container is not running so the suite stays green in
// environments without the oracle.
func TestEvalExpr(t *testing.T) {
	if err := ensureRunning(); err != nil {
		t.Skipf("sage-oracle container not running: %v", err)
	}

	cmd := exec.Command("docker", "exec", "-i", container,
		"sage", "-c", "print(1+1)")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("sage -c failed (exit %d): %s",
				exitErr.ExitCode(), exitErr.Stderr)
		}
		t.Fatalf("running docker exec: %v", err)
	}

	if got := strings.TrimSpace(string(out)); got != "2" {
		t.Fatalf("print(1+1): got %q, want %q", got, "2")
	}
}

// TestTranslatePath checks bind-mount path translation
// without touching Docker.
func TestTranslatePath(t *testing.T) {
	t.Run("inside mount", func(t *testing.T) {
		got, err := translatePath(hostMount + "/sub/script.sage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := containerMount + "/sub/script.sage"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("mount root file", func(t *testing.T) {
		got, err := translatePath(hostMount + "/script.sage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := containerMount + "/script.sage"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("outside mount", func(t *testing.T) {
		if _, err := translatePath("/etc/passwd"); err == nil {
			t.Fatal("expected error for path outside bind mount, got nil")
		}
	})
}

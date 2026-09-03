package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMangleToolchain(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"Plain", "leanprover/lean4:v4.30.0", "leanprover--lean4---v4.30.0"},
		{"ReleaseCandidate", "leanprover/lean4:v4.30.0-rc2", "leanprover--lean4---v4.30.0-rc2"},
		{"Nightly", "leanprover/lean4-nightly:2024-01-01", "leanprover--lean4-nightly---2024-01-01"},
		{"NoColon", "leanprover/lean4", "leanprover--lean4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mangleToolchain(tt.raw); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectRootEnv(t *testing.T) {
	t.Setenv("LEANDOC_ROOT", "/explicit/root")
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != "/explicit/root" {
		t.Errorf("got %q", root)
	}
}

func TestProjectRootWalksUp(t *testing.T) {
	t.Setenv("LEANDOC_ROOT", "")
	dir := realTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "lean-toolchain"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(dir, "Proofs", "Sub")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(deep)
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("got %q, want %q", root, dir)
	}
}

func TestProjectRootMissing(t *testing.T) {
	t.Setenv("LEANDOC_ROOT", "")
	t.Chdir(realTempDir(t))
	_, err := projectRoot()
	if err == nil || !strings.Contains(err.Error(), "LEANDOC_ROOT") {
		t.Errorf("err = %v, want mention of LEANDOC_ROOT", err)
	}
}

func TestToolchainDir(t *testing.T) {
	root := fakeProject(t)
	dir, err := toolchainDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".elan", "toolchains", "leanprover--lean4---v4.30.0-rc2")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestToolchainDirMissing(t *testing.T) {
	root := fakeProject(t)
	os.WriteFile(filepath.Join(root, "lean-toolchain"), []byte("leanprover/lean4:v9.99.0\n"), 0o644)
	if _, err := toolchainDir(root); err == nil {
		t.Fatal("expected error for uninstalled toolchain")
	}
}

// realTempDir resolves symlinks so that
// paths compare equal with what os.Getwd
// reports after Chdir.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

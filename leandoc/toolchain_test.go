package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
			if got, want := mangleToolchain(tt.raw), tt.want; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func setupFakeToolchain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "lean-toolchain"), []byte("leanprover/lean4:v4.30.0-rc2\n"), 0o644); err != nil {
		t.Fatalf("write lean-toolchain: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, ".lake", "packages"), 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}

	mangled := "leanprover--lean4---v4.30.0-rc2"
	elanRoot := filepath.Join(root, ".elan", "toolchains", mangled)
	for _, sub := range []string{
		filepath.Join("src", "lean", "Init"),
		filepath.Join("src", "lean", "Std"),
		filepath.Join("lib", "lean"),
	} {
		if err := os.MkdirAll(filepath.Join(elanRoot, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	return root
}

func TestToolchainDir(t *testing.T) {
	root := setupFakeToolchain(t)
	t.Setenv("LEANDOC_DOT_LAKE", filepath.Join(root, ".lake"))
	t.Setenv("HOME", root)

	dir, err := toolchainDir()
	if err != nil {
		t.Fatalf("toolchainDir: %v", err)
	}
	want := filepath.Join(root, ".elan", "toolchains", "leanprover--lean4---v4.30.0-rc2")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestToolchainDirMissingFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LEANDOC_DOT_LAKE", filepath.Join(root, ".lake"))
	if err := os.MkdirAll(filepath.Join(root, ".lake"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := toolchainDir()
	if err == nil {
		t.Fatalf("expected error for missing lean-toolchain")
	}
}

func TestToolchainDirMissingDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LEANDOC_DOT_LAKE", filepath.Join(root, ".lake"))
	t.Setenv("HOME", root)
	if err := os.MkdirAll(filepath.Join(root, ".lake"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "lean-toolchain"), []byte("leanprover/lean4:v9.99.0\n"), 0o644); err != nil {
		t.Fatalf("write lean-toolchain: %v", err)
	}

	_, err := toolchainDir()
	if err == nil {
		t.Fatalf("expected error for missing toolchain dir")
	}
}

func TestStalenessToolchainChange(t *testing.T) {
	root := setupFakeToolchain(t)
	t.Setenv("LEANDOC_DOT_LAKE", filepath.Join(root, ".lake"))
	t.Setenv("HOME", root)

	cache := t.TempDir()
	t.Setenv("LEANDOC_CACHE_DIR", cache)

	// First build to establish baseline cache files.
	if err := ensureIndex(); err != nil {
		t.Fatalf("initial build: %v", err)
	}

	bm25Path := filepath.Join(cache, "bm25.gob")

	// Push bm25.gob mtime into the future so it's newer than all corpus dirs.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(bm25Path, future, future); err != nil {
		t.Fatalf("chtimes bm25: %v", err)
	}

	// With everything fresh, ensureIndex should skip the rebuild.
	// Stamp a sentinel into bm25.gob that would be overwritten on rebuild.
	sentinel := []byte("sentinel-will-be-overwritten")
	if err := os.WriteFile(bm25Path, sentinel, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := os.Chtimes(bm25Path, future, future); err != nil {
		t.Fatalf("chtimes bm25: %v", err)
	}

	if err := ensureIndex(); err != nil {
		t.Fatalf("ensureIndex (should skip): %v", err)
	}
	data, err := os.ReadFile(bm25Path)
	if err != nil {
		t.Fatalf("read bm25.gob: %v", err)
	}
	if got, want := string(data), string(sentinel); got != want {
		t.Errorf("bm25.gob after skip: got %q, want %q", got, want)
	}

	// Touch lean-toolchain to be even newer, triggering rebuild.
	evenFurther := future.Add(time.Hour)
	tcPath := filepath.Join(root, "lean-toolchain")
	if err := os.Chtimes(tcPath, evenFurther, evenFurther); err != nil {
		t.Fatalf("chtimes lean-toolchain: %v", err)
	}

	if err := ensureIndex(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	data, err = os.ReadFile(bm25Path)
	if err != nil {
		t.Fatalf("read bm25.gob after rebuild: %v", err)
	}
	if string(data) == string(sentinel) {
		t.Errorf("ensureIndex did not rebuild after lean-toolchain changed")
	}
}

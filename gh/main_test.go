package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGhEnsureCacheHit(t *testing.T) {
	dir := t.TempDir()
	num := "42"
	numDir := filepath.Join(dir, num)
	if err := os.MkdirAll(numDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(numDir, "issue.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ghEnsure(dir, num, "", ""); err != nil {
		t.Fatalf("cache hit should return nil, got %v", err)
	}
}

func TestGhEnsureCacheStale(t *testing.T) {
	dir := t.TempDir()
	num := "99"
	numDir := filepath.Join(dir, num)
	if err := os.MkdirAll(numDir, 0o755); err != nil {
		t.Fatal(err)
	}
	issuePath := filepath.Join(numDir, "issue.json")
	if err := os.WriteFile(issuePath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-31 * time.Minute)
	if err := os.Chtimes(issuePath, stale, stale); err != nil {
		t.Fatal(err)
	}
	err := ghEnsure(dir, num, "", "")
	if err == nil {
		t.Fatal("stale cache should attempt re-fetch and fail (no network), got nil")
	}
	if _, serr := os.Stat(numDir); !os.IsNotExist(serr) {
		t.Error("stale cache dir should have been removed before re-fetch")
	}
}

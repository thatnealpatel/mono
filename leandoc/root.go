package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// projectRoot returns the Lean project
// root: LEANDOC_ROOT if set, otherwise
// the nearest ancestor of the working
// directory containing a lean-toolchain
// file.
func projectRoot() (string, error) {
	if root := os.Getenv("LEANDOC_ROOT"); root != "" {
		return root, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, "lean-toolchain")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no lean-toolchain in %s or any parent; set LEANDOC_ROOT", start)
		}
		dir = parent
	}
}

func packagesDir(root string) string {
	return filepath.Join(root, ".lake", "packages")
}

func mangleToolchain(raw string) string {
	s := strings.ReplaceAll(raw, "/", "--")
	return strings.ReplaceAll(s, ":", "---")
}

// toolchainDir derives the elan toolchain
// directory from the project's lean-toolchain
// file.
func toolchainDir(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "lean-toolchain"))
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return "", fmt.Errorf("lean-toolchain is empty")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".elan", "toolchains", mangleToolchain(raw))
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func cacheDir() (string, error) {
	dir := os.Getenv("LEANDOC_CACHE_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "leandoc")
	}
	return dir, os.MkdirAll(dir, 0o755)
}

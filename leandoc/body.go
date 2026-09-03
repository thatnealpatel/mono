package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveFile maps an indexed relative path back to
// the packages directory or the toolchain sources.
func resolveFile(root, rel string) (string, error) {
	abs := filepath.Join(packagesDir(root), rel)
	if _, err := os.Stat(abs); err == nil {
		return abs, nil
	}
	tcDir, err := toolchainDir(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Join(tcDir, "src", "lean", rel)
	if _, err := os.Stat(abs); err == nil {
		return abs, nil
	}
	return "", fmt.Errorf("source file not found: %s", rel)
}

func isStructuralBoundary(trimmed string) bool {
	kind, _ := matchDecl(trimmed)
	if kind != "" {
		return true
	}
	switch {
	case strings.HasPrefix(trimmed, "/-"),
		strings.HasPrefix(trimmed, "--"),
		strings.HasPrefix(trimmed, "@["),
		strings.HasPrefix(trimmed, "namespace "),
		trimmed == "end",
		strings.HasPrefix(trimmed, "end "),
		strings.HasPrefix(trimmed, "section"),
		strings.HasPrefix(trimmed, "variable"),
		strings.HasPrefix(trimmed, "open "),
		strings.HasPrefix(trimmed, "set_option"),
		strings.HasPrefix(trimmed, "module"),
		strings.HasPrefix(trimmed, "import "),
		strings.HasPrefix(trimmed, "export "),
		strings.HasPrefix(trimmed, "public "),
		strings.HasPrefix(trimmed, "noncomputable section"):
		return true
	}
	return false
}

// extractBody returns the declaration starting at line, up
// to the next top-level structural boundary.
func extractBody(absPath string, line int) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return "", fmt.Errorf("line %d out of range", line)
	}
	start := line - 1
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if len(lines[i]) > 0 && lines[i][0] != ' ' && lines[i][0] != '\t' {
			if isStructuralBoundary(trimmed) {
				end = i
				break
			}
		}
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n"), nil
}

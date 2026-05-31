package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsHex(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"", true},
		{"0", true},
		{"deadbeef", true},
		{"DEADBEEF", true},
		{"0123456789abcdef", true},
		{"0123456789ABCDEF", true},
		{"abcdefg", false},
		{"xyz", false},
		{"0x1", false},
	} {
		if got := isHex(tt.in); got != tt.want {
			t.Errorf("isHex(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseLeadingInt(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 0},
		{"123", 123},
		{"123abc", 123},
		{"0", 0},
		{"007bond", 7},
		{"42 is the answer", 42},
		{"99999", 99999},
	} {
		if got := parseLeadingInt(tt.in); got != tt.want {
			t.Errorf("parseLeadingInt(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestClExtractIssueNums(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message string
		want    []int
	}{
		{
			name:    "go.dev/issue",
			message: "Fixes go.dev/issue/12345",
			want:    []int{12345},
		},
		{
			name:    "golang/go#",
			message: "For golang/go#6789",
			want:    []int{6789},
		},
		{
			name:    "bare hash",
			message: "Fixes #42",
			want:    []int{42},
		},
		{
			name:    "multiple formats",
			message: "go.dev/issue/100\ngolang/go#200\nFixes #300",
			want:    []int{100, 200, 300},
		},
		{
			name:    "dedup across formats",
			message: "go.dev/issue/555\ngolang/go#555",
			want:    []int{555},
		},
		{
			name:    "no issues",
			message: "refactor: clean up tests",
			want:    nil,
		},
		{
			name:    "bare hash skipped when golang/go# present on same line",
			message: "For golang/go#99 and #100",
			want:    []int{99},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			detail := buildDetailJSON(tt.message)
			got := clExtractIssueNums(detail)
			if !intsEqual(got, tt.want) {
				t.Errorf("clExtractIssueNums(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func buildDetailJSON(message string) []byte {
	d := struct {
		Revisions map[string]struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"revisions"`
	}{
		Revisions: map[string]struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		}{
			"abc123": {Commit: struct {
				Message string `json:"message"`
			}{Message: message}},
		},
	}
	b, _ := json.Marshal(d)
	return b
}

func TestGerritEnsureCacheHit(t *testing.T) {
	dir := t.TempDir()
	num := "12345"
	numDir := filepath.Join(dir, num)
	if err := os.MkdirAll(numDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(numDir, "detail.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gerritEnsure(dir, num); err != nil {
		t.Fatalf("cache hit should return nil, got %v", err)
	}
}

func TestGerritEnsureCacheStale(t *testing.T) {
	dir := t.TempDir()
	num := "99999"
	numDir := filepath.Join(dir, num)
	if err := os.MkdirAll(numDir, 0o755); err != nil {
		t.Fatal(err)
	}
	detailPath := filepath.Join(numDir, "detail.json")
	if err := os.WriteFile(detailPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-31 * time.Minute)
	if err := os.Chtimes(detailPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	err := gerritEnsure(dir, num)
	if err == nil {
		t.Fatal("stale cache should attempt re-fetch and fail (no network), got nil")
	}
	if _, serr := os.Stat(numDir); !os.IsNotExist(serr) {
		t.Error("stale cache dir should have been removed before re-fetch")
	}
}

func intsEqual(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

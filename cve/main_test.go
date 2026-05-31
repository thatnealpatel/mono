package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripHTML(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text",
			in:   "no html here",
			want: "no html here",
		},
		{
			name: "paragraph tags",
			in:   "<p>first</p><p>second</p>",
			want: " first second",
		},
		{
			name: "br tags",
			in:   "line one<br>line two",
			want: "line one line two",
		},
		{
			name: "nested tags",
			in:   "<div><p>inside <b>bold</b> text</p></div>",
			want: "  inside bold text",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "only tags",
			in:   "<div><br></div>",
			want: "  ",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.in)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	raw := `{
		"cveMetadata": {
			"cveId": "CVE-2024-1234",
			"state": "PUBLISHED",
			"assignerShortName": "acme"
		},
		"containers": {
			"cna": {
				"title": "Buffer Overflow in Widget",
				"descriptions": [
					{"lang": "en", "value": "A buffer overflow in widget.c allows RCE."},
					{"lang": "es", "value": "Un desbordamiento de búfer..."}
				]
			}
		}
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "CVE-2024-1234.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, err := parseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "CVE-2024-1234" {
		t.Errorf("ID = %q, want CVE-2024-1234", rec.ID)
	}
	if rec.State != "PUBLISHED" {
		t.Errorf("State = %q, want PUBLISHED", rec.State)
	}
	if rec.Year != "2024" {
		t.Errorf("Year = %q, want 2024", rec.Year)
	}
	if rec.CNA != "acme" {
		t.Errorf("CNA = %q, want acme", rec.CNA)
	}
	if rec.Title != "Buffer Overflow in Widget" {
		t.Errorf("Title = %q, want Buffer Overflow in Widget", rec.Title)
	}
	if rec.Desc != "A buffer overflow in widget.c allows RCE." {
		t.Errorf("Desc = %q, want A buffer overflow in widget.c allows RCE.", rec.Desc)
	}
}

func TestParseFileNoEnglishDesc(t *testing.T) {
	raw := `{
		"cveMetadata": {"cveId": "CVE-2024-0001", "state": "REJECTED", "assignerShortName": "x"},
		"containers": {"cna": {"title": "T", "descriptions": [{"lang": "fr", "value": "desc en francais"}]}}
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, err := parseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Desc != "" {
		t.Errorf("Desc = %q, want empty (no English description)", rec.Desc)
	}
}

func TestParseFileMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := parseFile(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

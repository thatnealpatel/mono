package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestContentFingerprintDeterministicCreationOrder(t *testing.T) {
	base := t.TempDir()
	executable := filepath.Join(base, "sagedoc")
	tfWriteFile(t, executable, "executable bytes")

	first := filepath.Join(base, "first", "sage")
	second := filepath.Join(base, "second", "sage")
	tfMkdir(t, first)
	tfMkdir(t, second)

	// Construct identical trees in deliberately different orders.
	tfWriteFile(t, filepath.Join(first, "z", "last.py"), "last")
	tfWriteFile(t, filepath.Join(first, "a.py"), "first")
	tfMkdir(t, filepath.Join(first, "empty"))

	tfMkdir(t, filepath.Join(second, "empty"))
	tfWriteFile(t, filepath.Join(second, "a.py"), "first")
	tfWriteFile(t, filepath.Join(second, "z", "last.py"), "last")

	distributions := []distributionRecord{
		{Name: "zeta", Version: "2", Location: "/env/zeta"},
		{Name: "alpha", Version: "1", Location: "/env/alpha"},
	}
	firstFingerprint := tfFingerprint(t, executable, first, distributions)
	secondFingerprint := tfFingerprint(t, executable, second, []distributionRecord{distributions[1], distributions[0]})
	if firstFingerprint != secondFingerprint {
		t.Fatalf("equivalent trees and inventories had different fingerprints:\nfirst:  %s\nsecond: %s", firstFingerprint, secondFingerprint)
	}
	if repeated := tfFingerprint(t, executable, first, distributions); repeated != firstFingerprint {
		t.Fatalf("repeated fingerprint changed: got %s, want %s", repeated, firstFingerprint)
	}
}

func TestContentFingerprintInventoryMutations(t *testing.T) {
	t.Run("file addition", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		before := tfFingerprint(t, executable, root, nil)
		tfWriteFile(t, filepath.Join(root, "new.py"), "new")
		tfRequireDifferentFingerprint(t, before, tfFingerprint(t, executable, root, nil))
	})

	t.Run("path", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		oldPath := filepath.Join(root, "module.py")
		tfWriteFile(t, oldPath, "same bytes")
		before := tfFingerprint(t, executable, root, nil)
		if err := os.Rename(oldPath, filepath.Join(root, "renamed.py")); err != nil {
			t.Fatalf("rename fixture: %v", err)
		}
		tfRequireDifferentFingerprint(t, before, tfFingerprint(t, executable, root, nil))
	})

	t.Run("entry type", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		path := filepath.Join(root, "node")
		tfWriteFile(t, path, "")
		before := tfFingerprint(t, executable, root, nil)
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove regular-file fixture: %v", err)
		}
		tfMkdir(t, path)
		tfRequireDifferentFingerprint(t, before, tfFingerprint(t, executable, root, nil))
	})

	t.Run("file content", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		path := filepath.Join(root, "module.py")
		tfWriteFile(t, path, "old content")
		before := tfFingerprint(t, executable, root, nil)
		tfWriteFile(t, path, "new content")
		tfRequireDifferentFingerprint(t, before, tfFingerprint(t, executable, root, nil))
	})

	t.Run("executable content", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		before := tfFingerprint(t, executable, root, nil)
		tfWriteFile(t, executable, "changed executable")
		tfRequireDifferentFingerprint(t, before, tfFingerprint(t, executable, root, nil))
	})
}

func TestContentFingerprintDistributionInventoryMutations(t *testing.T) {
	executable, root := tfFingerprintFixture(t)
	baseInventory := []distributionRecord{{Name: "alpha", Version: "1.0", Location: "/env/alpha"}}
	base := tfFingerprint(t, executable, root, baseInventory)

	tests := []struct {
		name          string
		distributions []distributionRecord
	}{
		{
			name: "addition",
			distributions: []distributionRecord{
				{Name: "alpha", Version: "1.0", Location: "/env/alpha"},
				{Name: "beta", Version: "2.0", Location: "/env/beta"},
			},
		},
		{name: "name", distributions: []distributionRecord{{Name: "renamed", Version: "1.0", Location: "/env/alpha"}}},
		{name: "version", distributions: []distributionRecord{{Name: "alpha", Version: "1.1", Location: "/env/alpha"}}},
		{name: "location", distributions: []distributionRecord{{Name: "alpha", Version: "1.0", Location: "/other/alpha"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tfRequireDifferentFingerprint(t, base, tfFingerprint(t, executable, root, test.distributions))
		})
	}
}

func TestContentFingerprintExclusions(t *testing.T) {
	t.Run("__pycache__ subtree", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		tfWriteFile(t, filepath.Join(root, "module.py"), "source")
		before := tfFingerprint(t, executable, root, nil)

		cacheFile := filepath.Join(root, "__pycache__", "nested", "module.cpython-313.pyc")
		tfWriteFile(t, cacheFile, "cached bytecode")
		if got := tfFingerprint(t, executable, root, nil); got != before {
			t.Fatalf("adding __pycache__ subtree changed fingerprint: got %s, want %s", got, before)
		}
		tfWriteFile(t, cacheFile, "mutated cached bytecode")
		tfWriteFile(t, filepath.Join(root, "__pycache__", "another"), "another cache file")
		if got := tfFingerprint(t, executable, root, nil); got != before {
			t.Fatalf("mutating __pycache__ subtree changed fingerprint: got %s, want %s", got, before)
		}
	})

	t.Run("derived sibling bytecode", func(t *testing.T) {
		executable, root := tfFingerprintFixture(t)
		tfWriteFile(t, filepath.Join(root, "module.py"), "source")
		before := tfFingerprint(t, executable, root, nil)

		for _, extension := range []string{".pyc", ".pyo"} {
			path := filepath.Join(root, "module"+extension)
			tfWriteFile(t, path, "derived bytecode")
			if got := tfFingerprint(t, executable, root, nil); got != before {
				t.Fatalf("adding derived %s changed fingerprint: got %s, want %s", extension, got, before)
			}
			tfWriteFile(t, path, "mutated derived bytecode")
			if got := tfFingerprint(t, executable, root, nil); got != before {
				t.Fatalf("mutating derived %s changed fingerprint: got %s, want %s", extension, got, before)
			}
		}
	})
}

func TestContentFingerprintIncludesSourceLessBytecode(t *testing.T) {
	for _, extension := range []string{".pyc", ".pyo"} {
		t.Run(extension, func(t *testing.T) {
			executable, root := tfFingerprintFixture(t)
			before := tfFingerprint(t, executable, root, nil)
			path := filepath.Join(root, "source_less"+extension)
			tfWriteFile(t, path, "first bytecode")
			withBytecode := tfFingerprint(t, executable, root, nil)
			tfRequireDifferentFingerprint(t, before, withBytecode)

			tfWriteFile(t, path, "second bytecode")
			tfRequireDifferentFingerprint(t, withBytecode, tfFingerprint(t, executable, root, nil))
		})
	}
}

func TestContentFingerprintRejectsSymlinks(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "valid file target",
			setup: func(t *testing.T, root string) {
				tfWriteFile(t, filepath.Join(root, "target"), "target")
				tfSymlink(t, "target", filepath.Join(root, "link"))
			},
		},
		{
			name: "valid directory target",
			setup: func(t *testing.T, root string) {
				tfMkdir(t, filepath.Join(root, "target"))
				tfSymlink(t, "target", filepath.Join(root, "link"))
			},
		},
		{
			name: "broken",
			setup: func(t *testing.T, root string) {
				tfSymlink(t, "missing", filepath.Join(root, "link"))
			},
		},
		{
			name: "cyclic",
			setup: func(t *testing.T, root string) {
				tfSymlink(t, "b", filepath.Join(root, "a"))
				tfSymlink(t, "a", filepath.Join(root, "b"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executable, root := tfFingerprintFixture(t)
			test.setup(t, root)
			_, err := contentFingerprint(executable, root, nil)
			if err == nil {
				t.Fatal("contentFingerprint accepted a Sage tree containing a symlink")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
				t.Fatalf("symlink rejection error %q does not identify the symlink", err)
			}
		})
	}
}

func TestEnvironmentFingerprintSeparation(t *testing.T) {
	baseEnvironment := sageEnvironment{
		Executable: "/env/bin/python",
		Prefix:     "/env",
		SageRoot:   "/env/lib/python/site-packages/sage",
		Distributions: []distributionRecord{
			{Name: "alpha", Version: "1", Location: "/env/alpha"},
		},
	}
	base := environmentFingerprint(baseEnvironment)
	if len(base) != 64 {
		t.Fatalf("environment fingerprint has length %d, want 64: %q", len(base), base)
	}

	tests := []struct {
		name string
		env  sageEnvironment
	}{
		{
			name: "interpreter",
			env:  sageEnvironment{Executable: "/other/bin/python", Prefix: baseEnvironment.Prefix},
		},
		{
			name: "prefix",
			env:  sageEnvironment{Executable: baseEnvironment.Executable, Prefix: "/other"},
		},
		{
			name: "length framing prevents path-boundary collision",
			env:  sageEnvironment{Executable: "/env/bin/pytho", Prefix: "n/env"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := environmentFingerprint(test.env); got == base {
				t.Fatalf("environment mutation did not change fingerprint %s", base)
			}
		})
	}

	// Sage-tree and distribution changes
	// select a content database within an
	// environment; they do not select a
	// different environment cache directory.
	sameEnvironment := baseEnvironment
	sameEnvironment.SageRoot = "/different/sage"
	sameEnvironment.Distributions = []distributionRecord{{Name: "beta", Version: "2", Location: "/other/beta"}}
	if got := environmentFingerprint(sameEnvironment); got != base {
		t.Fatalf("content-only fields changed environment fingerprint: got %s, want %s", got, base)
	}
}

func TestContentFingerprintDistributionMetadata(t *testing.T) {
	executable, root := tfFingerprintFixture(t)
	empty := ""
	recordA, recordB := "a.py,sha256=one,1\n", "a.py,sha256=two,1\n"
	directA, directB := `{"url":"file:///one"}`, `{"url":"file:///two"}`
	baseRecord := distributionRecord{
		Name: "alpha", Version: "1.0", Location: "/env/site", MetadataPath: "/env/site/alpha.dist-info",
	}
	base := tfFingerprint(t, executable, root, []distributionRecord{baseRecord})

	mutations := []struct {
		name   string
		mutate func(*distributionRecord)
	}{
		{name: "metadata path", mutate: func(record *distributionRecord) { record.MetadataPath += "-other" }},
		{name: "empty RECORD differs from absent", mutate: func(record *distributionRecord) { record.Record = &empty }},
		{name: "RECORD content", mutate: func(record *distributionRecord) { record.Record = &recordA }},
		{name: "empty direct URL differs from absent", mutate: func(record *distributionRecord) { record.DirectURL = &empty }},
		{name: "direct URL content", mutate: func(record *distributionRecord) { record.DirectURL = &directA }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			record := baseRecord
			test.mutate(&record)
			tfRequireDifferentFingerprint(t, base, tfFingerprint(t, executable, root, []distributionRecord{record}))
		})
	}

	first := baseRecord
	first.Record, first.DirectURL = &recordA, &directB
	second := baseRecord
	second.Record, second.DirectURL = &recordB, &directA
	ordered := tfFingerprint(t, executable, root, []distributionRecord{first, second})
	shuffled := tfFingerprint(t, executable, root, []distributionRecord{second, first})
	if ordered != shuffled {
		t.Fatalf("full-evidence sorting is nondeterministic: ordered=%s shuffled=%s", ordered, shuffled)
	}
}

func TestContentFingerprintCondaInventory(t *testing.T) {
	executable, root := tfFingerprintFixture(t)
	base := tfFingerprintWithConda(t, executable, root, nil, nil)
	first := []condaRecord{
		{Path: "zeta-2.0-0.json", Content: `{"name":"zeta","version":"2.0"}`},
		{Path: "alpha-1.0-0.json", Content: `{"name":"alpha","version":"1.0"}`},
	}
	withRecords := tfFingerprintWithConda(t, executable, root, nil, first)
	tfRequireDifferentFingerprint(t, base, withRecords)
	if shuffled := tfFingerprintWithConda(t, executable, root, nil, []condaRecord{first[1], first[0]}); shuffled != withRecords {
		t.Fatalf("shuffled conda inventory changed fingerprint: got %s, want %s", shuffled, withRecords)
	}
	mutated := append([]condaRecord(nil), first...)
	mutated[0].Content += "\n"
	tfRequireDifferentFingerprint(t, withRecords, tfFingerprintWithConda(t, executable, root, nil, mutated))
}

func TestContentFingerprintRejectsFIFO(t *testing.T) {
	executable, root := tfFingerprintFixture(t)
	fifo := filepath.Join(root, "stream")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("create FIFO fixture: %v", err)
	}

	_, err := contentFingerprint(executable, root, nil)
	if err == nil {
		t.Fatal("contentFingerprint accepted a FIFO")
	}
	if message := err.Error(); !strings.Contains(message, "stream") || !strings.Contains(message, "unsupported file mode") {
		t.Fatalf("FIFO rejection error = %q, want entry path and unsupported-mode diagnosis", message)
	}
}

func tfFingerprintFixture(t *testing.T) (executable, root string) {
	t.Helper()
	base := t.TempDir()
	executable = filepath.Join(base, "sagedoc")
	root = filepath.Join(base, "sage")
	tfWriteFile(t, executable, "executable")
	tfMkdir(t, root)
	return executable, root
}

func tfFingerprint(t *testing.T, executable, root string, distributions []distributionRecord) string {
	t.Helper()
	fingerprint, err := contentFingerprint(executable, root, distributions)
	if err != nil {
		t.Fatalf("contentFingerprint: %v", err)
	}
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint has length %d, want 64: %q", len(fingerprint), fingerprint)
	}
	return fingerprint
}

func tfFingerprintWithConda(t *testing.T, executable, root string, distributions []distributionRecord, conda []condaRecord) string {
	t.Helper()
	fingerprint, err := contentFingerprint(executable, root, distributions, conda)
	if err != nil {
		t.Fatalf("contentFingerprint: %v", err)
	}
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint has length %d, want 64: %q", len(fingerprint), fingerprint)
	}
	return fingerprint
}

func tfRequireDifferentFingerprint(t *testing.T, before, after string) {
	t.Helper()
	if after == before {
		t.Fatalf("mutation did not change fingerprint %s", before)
	}
}

func tfWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func tfMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create directory %q: %v", path, err)
	}
}

func tfSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink %q -> %q: %v", path, target, err)
	}
}

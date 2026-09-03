package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patel.codes/indexing"
)

const pkgSource = `/-- Addition of natural numbers is commutative. -/
theorem Nat.add_comm (n m : Nat) : n + m = m + n := by
  omega

@[simp]
def Nat.succ_pred (n : Nat) : n > 0 → n.pred.succ = n := by
  omega

/-- A homomorphism between two groups. -/
structure Group.Hom (G H : Type) where
  toFun : G → H
`

const toolchainSource = `/-- The core list map. -/
def List.map (f : α → β) : List α → List β := fun l => l
`

// fakeProject lays out a project root, an
// elan toolchain under HOME, and an empty
// cache directory.
func fakeProject(t *testing.T) string {
	t.Helper()
	root := realTempDir(t)
	t.Setenv("HOME", root)
	t.Setenv("LEANDOC_ROOT", root)
	t.Setenv("LEANDOC_CACHE_DIR", filepath.Join(root, "cache"))

	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lean-toolchain", "leanprover/lean4:v4.30.0-rc2\n")
	write("lake-manifest.json", `{"packages":[{"name":"foo","rev":"abc"}]}`)
	write(".lake/packages/foo/Foo/Nat.lean", pkgSource)

	ilean, err := os.ReadFile("testdata/generated_only.ilean")
	if err != nil {
		t.Fatal(err)
	}
	write(".lake/packages/foo/.lake/build/lib/Foo.ilean", string(ilean))

	tc := ".elan/toolchains/leanprover--lean4---v4.30.0-rc2/"
	write(tc+"src/lean/Init/Data/List.lean", toolchainSource)
	write(tc+"src/lean/Std/Empty.lean", "")
	if err := os.MkdirAll(filepath.Join(root, tc, "lib", "lean"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func query(t *testing.T, ix *indexing.Index, root, q string, verbose bool) indexing.Envelope {
	t.Helper()
	var buf bytes.Buffer
	if err := run(&buf, ix, root, q, verbose); err != nil {
		t.Fatalf("run(%q): %v", q, err)
	}
	var env indexing.Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), `"matches":[`) {
		t.Errorf("matches must always be an array: %s", buf.String())
	}
	return env
}

func TestEndToEnd(t *testing.T) {
	root := fakeProject(t)
	ix, err := openIndex(root)
	if err != nil {
		t.Fatalf("openIndex: %v", err)
	}
	defer ix.Close()

	t.Run("ExactWithBody", func(t *testing.T) {
		env := query(t, ix, root, "Nat.add_comm", false)
		if env.Mode != "exact" || len(env.Matches) != 1 {
			t.Fatalf("env = %+v", env)
		}
		m := env.Matches[0]
		if m.File != "foo/Foo/Nat.lean" || m.Line != 2 || m.Kind != "theorem" {
			t.Errorf("match = %+v", m)
		}
		want := "theorem Nat.add_comm (n m : Nat) : n + m = m + n := by\n  omega"
		if m.Body != want {
			t.Errorf("body = %q, want %q", m.Body, want)
		}
		if !strings.Contains(m.Docstring, "commutative") {
			t.Errorf("docstring = %q", m.Docstring)
		}
	})

	t.Run("ExactToolchainBody", func(t *testing.T) {
		env := query(t, ix, root, "List.map", false)
		if env.Mode != "exact" || len(env.Matches) != 1 {
			t.Fatalf("env = %+v", env)
		}
		if env.Matches[0].File != "Init/Data/List.lean" || !strings.HasPrefix(env.Matches[0].Body, "def List.map") {
			t.Errorf("match = %+v", env.Matches[0])
		}
	})

	t.Run("Generated", func(t *testing.T) {
		env := query(t, ix, root, "Fin.sum_univ_eq_sum_range", false)
		if env.Mode != "exact" || len(env.Matches) != 1 {
			t.Fatalf("env = %+v", env)
		}
		m := env.Matches[0]
		if m.Kind != "generated" || m.File != "Mathlib/Data/Fintype/BigOperators.lean" || m.Body != "" {
			t.Errorf("match = %+v", m)
		}
	})

	t.Run("Miss", func(t *testing.T) {
		env := query(t, ix, root, "Bogus.add_comm", false)
		if env.Mode != "miss" || len(env.Matches) != 0 {
			t.Fatalf("env = %+v", env)
		}
		if len(env.Candidates) != 1 || env.Candidates[0] != "Nat.add_comm" {
			t.Errorf("candidates = %v", env.Candidates)
		}
	})

	t.Run("Search", func(t *testing.T) {
		env := query(t, ix, root, "group homomorphism", false)
		if env.Mode != "search" || len(env.Matches) == 0 {
			t.Fatalf("env = %+v", env)
		}
		m := env.Matches[0]
		if m.Name != "Group.Hom" || m.Score != 0 || m.Body != "" {
			t.Errorf("match = %+v", m)
		}
	})

	t.Run("SearchVerbose", func(t *testing.T) {
		env := query(t, ix, root, "group homomorphism", true)
		if env.Matches[0].Score <= 0 {
			t.Errorf("score = %v, want positive", env.Matches[0].Score)
		}
	})

	t.Run("CacheReused", func(t *testing.T) {
		entries, _ := os.ReadDir(filepath.Join(root, "cache"))
		if len(entries) != 1 {
			t.Fatalf("cache entries = %d, want 1", len(entries))
		}
		again, err := openIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		again.Close()
		entries, _ = os.ReadDir(filepath.Join(root, "cache"))
		if len(entries) != 1 {
			t.Errorf("cache entries after reopen = %d, want 1", len(entries))
		}
	})

	t.Run("ManifestChangeRebuilds", func(t *testing.T) {
		os.WriteFile(filepath.Join(root, "lake-manifest.json"), []byte(`{"packages":[]}`), 0o644)
		again, err := openIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		again.Close()
		entries, _ := os.ReadDir(filepath.Join(root, "cache"))
		if len(entries) != 2 {
			t.Errorf("cache entries after manifest change = %d, want 2", len(entries))
		}
	})
}

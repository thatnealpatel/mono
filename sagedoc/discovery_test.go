package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestDiscoverRequiresConfiguredPython(t *testing.T) {

	env, err := discover(context.Background(), "")
	if err == nil {
		t.Fatal("discover succeeded without SAGEDOC_PYTHON")
	}
	if got, want := err.Error(), "SAGEDOC_PYTHON is required"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(env, sageEnvironment{}) {
		t.Fatalf("environment on error = %#v, want zero value", env)
	}
}

func TestDiscoverRejectsUnusableInterpreter(t *testing.T) {

	configuredPython := filepath.Join(t.TempDir(), "missing-python")
	env, err := discover(context.Background(), configuredPython)
	if err == nil {
		t.Fatal("discover succeeded with a nonexistent interpreter")
	}
	discRequireErrorContains(t, err, "run Sage discovery", configuredPython)
	if !reflect.DeepEqual(env, sageEnvironment{}) {
		t.Fatalf("environment on error = %#v, want zero value", env)
	}
}

func TestDiscoverSyntheticSageCanonicalPathsAndImports(t *testing.T) {
	python := discFindPython(t)
	temp := t.TempDir()
	realSite := filepath.Join(temp, "real-site")
	discMkdir(t, filepath.Join(realSite, "sage"))
	importLog := filepath.Join(temp, "sage-imports")
	allImportLog := filepath.Join(temp, "sage-all-imports")
	discWriteFile(t, filepath.Join(realSite, "sage", "__init__.py"), []byte(
		"with open("+strconv.Quote(importLog)+", 'a', encoding='utf-8') as stream:\n"+
			"    stream.write('sage\\n')\n"), 0o644)
	discWriteFile(t, filepath.Join(realSite, "sage", "all.py"), []byte(
		"with open("+strconv.Quote(allImportLog)+", 'a', encoding='utf-8') as stream:\n"+
			"    stream.write('sage.all\\n')\n"), 0o644)

	discWriteDistribution(t, realSite, "Zeta_Project-9.0.dist-info", "Zeta_Project", "9.0")
	discWriteDistribution(t, realSite, "Alpha.Project-2.5.dist-info", "Alpha.Project", "2.5")
	alphaRecordBytes := "alpha.py,sha256=abc,3\n"
	alphaDirectURLBytes := `{"url":"file:///src/alpha"}`
	alphaRecord := discSHA256(alphaRecordBytes)
	alphaDirectURL := discSHA256(alphaDirectURLBytes)
	discWriteFile(t, filepath.Join(realSite, "Alpha.Project-2.5.dist-info", "RECORD"), []byte(alphaRecordBytes), 0o644)
	discWriteFile(t, filepath.Join(realSite, "Alpha.Project-2.5.dist-info", "direct_url.json"), []byte(alphaDirectURLBytes), 0o644)

	aliasSite := filepath.Join(temp, "site-alias")
	if err := os.Symlink(realSite, aliasSite); err != nil {
		t.Fatalf("create synthetic site-package symlink: %v", err)
	}
	pythonAlias := filepath.Join(temp, "python-alias")
	if err := os.Symlink(python, pythonAlias); err != nil {
		t.Fatalf("create Python symlink: %v", err)
	}
	configuredPython := discWritePythonWrapper(t, pythonAlias, aliasSite)

	wantExecutable := discEvalSymlinks(t, python)
	wantPrefix := discPythonPrefix(t, python)
	wantSageRoot := discEvalSymlinks(t, filepath.Join(realSite, "sage"))
	wantDistributionLocation := discEvalSymlinks(t, realSite)

	for invocation := 1; invocation <= 2; invocation++ {
		env, err := discover(context.Background(), configuredPython)
		if err != nil {
			t.Fatalf("discover invocation %d: %v", invocation, err)
		}
		if env.Executable != wantExecutable {
			t.Errorf("invocation %d executable = %q, want canonical %q", invocation, env.Executable, wantExecutable)
		}
		if env.Prefix != wantPrefix {
			t.Errorf("invocation %d prefix = %q, want canonical %q", invocation, env.Prefix, wantPrefix)
		}
		if env.SageRoot != wantSageRoot {
			t.Errorf("invocation %d Sage root = %q, want canonical %q", invocation, env.SageRoot, wantSageRoot)
		}
		if !discDistributionsSorted(env.Distributions) {
			t.Errorf("invocation %d distribution inventory is not sorted: %#v", invocation, env.Distributions)
		}
		discRequireDistribution(t, env.Distributions, distributionRecord{
			Name: "alpha-project", Version: "2.5", Location: wantDistributionLocation,
			MetadataPath: discEvalSymlinks(t, filepath.Join(realSite, "Alpha.Project-2.5.dist-info")),
			Record:       &alphaRecord, DirectURL: &alphaDirectURL,
		})
		discRequireDistribution(t, env.Distributions, distributionRecord{
			Name: "zeta-project", Version: "9.0", Location: wantDistributionLocation,
			MetadataPath: discEvalSymlinks(t, filepath.Join(realSite, "Zeta_Project-9.0.dist-info")),
		})

		data, readErr := os.ReadFile(importLog)
		if readErr != nil {
			t.Fatalf("read sage import log after invocation %d: %v", invocation, readErr)
		}
		if got, want := string(data), strings.Repeat("sage\n", invocation); got != want {
			t.Errorf("sage import log after invocation %d = %q, want %q", invocation, got, want)
		}
		if data, readErr := os.ReadFile(allImportLog); readErr == nil {
			t.Errorf("sage.all was imported on invocation %d; log = %q", invocation, data)
		} else if !os.IsNotExist(readErr) {
			t.Fatalf("inspect sage.all import log after invocation %d: %v", invocation, readErr)
		}
	}
}

func TestDiscoverSortsDistributionInventory(t *testing.T) {

	temp := t.TempDir()
	sageRoot := filepath.Join(temp, "sage")
	discMkdir(t, sageRoot)
	locationA := filepath.Join(temp, "a")
	locationB := filepath.Join(temp, "b")
	input := []distributionRecord{
		{Name: "zeta", Version: "1", Location: locationA},
		{Name: "alpha", Version: "2", Location: locationB},
		{Name: "alpha", Version: "1", Location: locationB},
		{Name: "alpha", Version: "2", Location: locationA},
	}
	env := sageEnvironment{
		Executable:    filepath.Join(temp, "python"),
		Prefix:        temp,
		SageRoot:      sageRoot,
		Distributions: input,
		CondaRecords: []condaRecord{
			{Path: "zeta.json", Content: `{"name":"zeta"}`},
			{Path: "alpha.json", Content: `{"name":"alpha"}`},
		},
	}
	configuredPython := discWriteOutputWrapper(t, discMarshalEnvironment(t, env))

	got, err := discover(context.Background(), configuredPython)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []distributionRecord{
		{Name: "alpha", Version: "1", Location: locationB},
		{Name: "alpha", Version: "2", Location: locationA},
		{Name: "alpha", Version: "2", Location: locationB},
		{Name: "zeta", Version: "1", Location: locationA},
	}
	if !reflect.DeepEqual(got.Distributions, want) {
		t.Fatalf("distributions = %#v, want %#v", got.Distributions, want)
	}
	wantConda := []condaRecord{
		{Path: "alpha.json", Content: `{"name":"alpha"}`},
		{Path: "zeta.json", Content: `{"name":"zeta"}`},
	}
	if !reflect.DeepEqual(got.CondaRecords, wantConda) {
		t.Fatalf("conda records = %#v, want %#v", got.CondaRecords, wantConda)
	}
}

func TestSameSageEnvironmentDetectsRetargeting(t *testing.T) {
	record := "RECORD"
	directURL := "direct"
	base := sageEnvironment{
		Executable: "/env/bin/python",
		Prefix:     "/env",
		SageRoot:   "/env/site/sage",
		Distributions: []distributionRecord{{
			Name: "sage", Version: "1", Location: "/env/site", MetadataPath: "/env/site/sage.dist-info",
			Record: &record, DirectURL: &directURL,
		}},
		CondaRecords: []condaRecord{{Path: "sage.json", Content: `{"name":"sage"}`}},
	}
	if !sameSageEnvironment(base, base) {
		t.Fatal("identical environment did not compare equal")
	}
	tests := []struct {
		name   string
		mutate func(*sageEnvironment)
	}{
		{name: "interpreter", mutate: func(env *sageEnvironment) { env.Executable += "-other" }},
		{name: "prefix", mutate: func(env *sageEnvironment) { env.Prefix += "-other" }},
		{name: "Sage root", mutate: func(env *sageEnvironment) { env.SageRoot += "-other" }},
		{name: "distribution", mutate: func(env *sageEnvironment) { env.Distributions[0].Version = "2" }},
		{name: "conda", mutate: func(env *sageEnvironment) { env.CondaRecords[0].Content += " " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			changed.Distributions = append([]distributionRecord(nil), base.Distributions...)
			changed.CondaRecords = append([]condaRecord(nil), base.CondaRecords...)
			test.mutate(&changed)
			if sameSageEnvironment(base, changed) {
				t.Fatal("retargeted environment compared equal")
			}
		})
	}
}

func TestDiscoverRejectsSagePackageOverlay(t *testing.T) {
	python := discFindPython(t)

	for _, test := range []struct {
		name  string
		paths []string
	}{
		{name: "no roots", paths: []string{}},
		{name: "two roots", paths: []string{"first", "second"}},
	} {
		t.Run(test.name, func(t *testing.T) {

			temp := t.TempDir()
			site := filepath.Join(temp, "site")
			discMkdir(t, filepath.Join(site, "sage"))
			for _, path := range test.paths {
				discMkdir(t, filepath.Join(temp, path))
			}
			quotedPaths := make([]string, 0, len(test.paths))
			for _, path := range test.paths {
				quotedPaths = append(quotedPaths, strconv.Quote(filepath.Join(temp, path)))
			}
			discWriteFile(t, filepath.Join(site, "sage", "__init__.py"), []byte(
				"__path__ = ["+strings.Join(quotedPaths, ", ")+"]\n"), 0o644)
			configuredPython := discWritePythonWrapper(t, python, site)

			env, err := discover(context.Background(), configuredPython)
			if err == nil {
				t.Fatal("discover accepted a synthetic sage package overlay")
			}
			discRequireErrorContains(t, err,
				"run Sage discovery",
				configuredPython,
				"expected exactly one sage.__path__ entry",
				"got "+strconv.Itoa(len(test.paths)),
			)
			if !reflect.DeepEqual(env, sageEnvironment{}) {
				t.Fatalf("environment on error = %#v, want zero value", env)
			}
		})
	}
}

func TestDiscoverRejectsMalformedAndTrailingOutput(t *testing.T) {

	temp := t.TempDir()
	sageRoot := filepath.Join(temp, "sage")
	discMkdir(t, sageRoot)
	valid := discMarshalEnvironment(t, sageEnvironment{
		Executable: filepath.Join(temp, "python"),
		Prefix:     temp,
		SageRoot:   sageRoot,
	})
	unknownField := strings.TrimSuffix(valid, "}") + `,"unexpected":true}`

	for _, test := range []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty", output: "", want: "EOF"},
		{name: "truncated JSON", output: `{"executable":`, want: "unexpected EOF"},
		{name: "unknown field", output: unknownField, want: "unknown field"},
		{name: "trailing garbage", output: valid + "\nnot-json", want: "invalid character"},
		{name: "second JSON value", output: valid + "\n{}", want: "more than one JSON value"},
	} {
		t.Run(test.name, func(t *testing.T) {

			configuredPython := discWriteOutputWrapper(t, test.output)
			env, err := discover(context.Background(), configuredPython)
			if err == nil {
				t.Fatal("discover accepted invalid output")
			}
			discRequireErrorContains(t, err, "decode Sage discovery output", configuredPython, test.want)
			if !reflect.DeepEqual(env, sageEnvironment{}) {
				t.Fatalf("environment on error = %#v, want zero value", env)
			}
		})
	}
}

func TestDiscoverRejectsInvalidPaths(t *testing.T) {

	temp := t.TempDir()
	sageRoot := filepath.Join(temp, "sage")
	discMkdir(t, sageRoot)
	nonDirectory := filepath.Join(temp, "sage-file")
	discWriteFile(t, nonDirectory, []byte("not a directory\n"), 0o644)

	for _, test := range []struct {
		name      string
		mutate    func(*sageEnvironment)
		operation string
		want      string
	}{
		{
			name: "missing executable path",
			mutate: func(env *sageEnvironment) {
				env.Executable = ""
			},
			operation: "validate Sage discovery output",
			want:      `interpreter path "" is not absolute`,
		},
		{
			name: "relative executable path",
			mutate: func(env *sageEnvironment) {
				env.Executable = "python"
			},
			operation: "validate Sage discovery output",
			want:      `interpreter path "python" is not absolute`,
		},
		{
			name: "relative prefix path",
			mutate: func(env *sageEnvironment) {
				env.Prefix = "prefix"
			},
			operation: "validate Sage discovery output",
			want:      `prefix path "prefix" is not absolute`,
		},
		{
			name: "relative Sage root",
			mutate: func(env *sageEnvironment) {
				env.SageRoot = "sage"
			},
			operation: "validate Sage discovery output",
			want:      `Sage root path "sage" is not absolute`,
		},
		{
			name: "relative distribution location",
			mutate: func(env *sageEnvironment) {
				env.Distributions[0].Location = "packages"
			},
			operation: "validate distribution inventory",
			want:      `distribution "demo" has non-absolute location "packages"`,
		},
		{
			name: "Sage root is not a directory",
			mutate: func(env *sageEnvironment) {
				env.SageRoot = nonDirectory
			},
			operation: "validate Sage package root",
			want:      "is not a directory",
		},
	} {
		t.Run(test.name, func(t *testing.T) {

			env := sageEnvironment{
				Executable: filepath.Join(temp, "python"),
				Prefix:     temp,
				SageRoot:   sageRoot,
				Distributions: []distributionRecord{
					{Name: "demo", Version: "1", Location: temp},
				},
			}
			test.mutate(&env)
			configuredPython := discWriteOutputWrapper(t, discMarshalEnvironment(t, env))

			got, err := discover(context.Background(), configuredPython)
			if err == nil {
				t.Fatal("discover accepted invalid path output")
			}
			discRequireErrorContains(t, err, test.operation, configuredPython, test.want)
			if !reflect.DeepEqual(got, sageEnvironment{}) {
				t.Fatalf("environment on error = %#v, want zero value", got)
			}
		})
	}
}

func discFindPython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := exec.Command(path, "-c", "import importlib.metadata").Run(); err == nil {
			return path
		}
	}
	t.Skip("Python with importlib.metadata is required for synthetic discovery tests")
	return ""
}

func discPythonPrefix(t *testing.T, python string) string {
	t.Helper()
	output, err := exec.Command(python, "-c", "import os, sys; print(os.path.realpath(sys.prefix))").Output()
	if err != nil {
		t.Fatalf("query Python prefix: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func discWritePythonWrapper(t *testing.T, python, pythonPath string) string {
	t.Helper()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = '-I' ]; then shift; fi\n" +
		"PYTHONPATH=" + discShellQuote(pythonPath) + "\n" +
		"export PYTHONPATH\n" +
		"exec " + discShellQuote(python) + " \"$@\"\n"
	return discWriteExecutable(t, body)
}

func discWriteOutputWrapper(t *testing.T, output string) string {
	t.Helper()
	body := "#!/bin/sh\ncat <<'DISCOVERY_TEST_EOF'\n" + output + "\nDISCOVERY_TEST_EOF\n"
	return discWriteExecutable(t, body)
}

func discWriteExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "python-wrapper")
	discWriteFile(t, path, []byte(contents), 0o755)
	return path
}

func discWriteDistribution(t *testing.T, site, directory, name, version string) {
	t.Helper()
	path := filepath.Join(site, directory)
	discMkdir(t, path)
	metadata := "Metadata-Version: 2.1\nName: " + name + "\nVersion: " + version + "\n"
	discWriteFile(t, filepath.Join(path, "METADATA"), []byte(metadata), 0o644)
}

func discWriteFile(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func discMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", path, err)
	}
}

func discEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", path, err)
	}
	if !filepath.IsAbs(canonical) {
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			t.Fatalf("make canonical path absolute: %v", err)
		}
	}
	return canonical
}

func discMarshalEnvironment(t *testing.T, env sageEnvironment) string {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal discovery output: %v", err)
	}
	return string(data)
}

func discShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func discDistributionsSorted(distributions []distributionRecord) bool {
	for index := 1; index < len(distributions); index++ {
		if discDistributionLess(distributions[index], distributions[index-1]) {
			return false
		}
	}
	return true
}

func discDistributionLess(left, right distributionRecord) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Version != right.Version {
		return left.Version < right.Version
	}
	return left.Location < right.Location
}

func discRequireDistribution(t *testing.T, distributions []distributionRecord, want distributionRecord) {
	t.Helper()
	for _, distribution := range distributions {
		if !distributionLess(distribution, want) && !distributionLess(want, distribution) {
			return
		}
	}
	t.Errorf("distribution inventory does not contain %#v; inventory = %#v", want, distributions)
}

func discSHA256(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func discRequireErrorContains(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	message := err.Error()
	for _, fragment := range fragments {
		if !strings.Contains(message, fragment) {
			t.Errorf("error %q does not contain %q", message, fragment)
		}
	}
}

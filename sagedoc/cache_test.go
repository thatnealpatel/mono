package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"patel.codes/indexing"
)

type cachetestFixture struct {
	dir           string
	python        string
	cache         string
	executable    string
	sageRoot      string
	counter       string
	distribution  string
	record        string
	directURL     string
	conda         string
	rootSelection string
}

func cachetestNewFixture(t *testing.T) *cachetestFixture {
	t.Helper()
	realPython, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the synthetic Sage interpreter")
	}
	realPython, err = filepath.Abs(realPython)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fixture := &cachetestFixture{
		dir:           dir,
		python:        filepath.Join(dir, "python"),
		cache:         filepath.Join(dir, "cache"),
		executable:    filepath.Join(dir, "sagedoc-executable"),
		sageRoot:      filepath.Join(dir, "sage"),
		counter:       filepath.Join(dir, "extractions"),
		distribution:  filepath.Join(dir, "distribution-version"),
		record:        filepath.Join(dir, "distribution-RECORD"),
		directURL:     filepath.Join(dir, "distribution-direct-url"),
		conda:         filepath.Join(dir, "conda-record"),
		rootSelection: filepath.Join(dir, "sage-root-selection"),
	}
	if err := os.MkdirAll(fixture.sageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cachetestWriteFile(t, filepath.Join(fixture.sageRoot, "all.py"), "# synthetic sage.all v1\n")
	cachetestWriteFile(t, fixture.executable, "synthetic sagedoc executable v1\n")
	cachetestWriteFile(t, fixture.distribution, "1.0\n")
	cachetestWriteFile(t, fixture.record, "module.py,sha256=one,1\n")
	cachetestWriteFile(t, fixture.directURL, `{"url":"file:///one"}`)
	cachetestWriteFile(t, fixture.conda, `{"name":"synthetic-sage","version":"1.0"}`)
	cachetestWriteFile(t, fixture.rootSelection, "sage\n")

	// The fake executable consumes the
	// embedded script just like Python
	// would, but supplies deterministic
	// discovery and extraction results.
	// fcntl makes the extraction count
	// reliable across concurrently
	// executing processes.
	script := fmt.Sprintf(`#!%s
import fcntl
import json
import os
import struct
import sys

sys.stdin.read()
base = os.path.dirname(os.path.realpath(__file__))
if sys.argv[1:] in (["-"], ["-I", "-"]):
    with open(os.path.join(base, "distribution-version"), encoding="utf-8") as source:
        version = source.read().strip()
    with open(os.path.join(base, "distribution-RECORD"), encoding="utf-8") as source:
        record = source.read()
    with open(os.path.join(base, "distribution-direct-url"), encoding="utf-8") as source:
        direct_url = source.read()
    with open(os.path.join(base, "conda-record"), encoding="utf-8") as source:
        conda = source.read()
    with open(os.path.join(base, "sage-root-selection"), encoding="utf-8") as source:
        sage_root = source.read().strip()
    json.dump({
        "executable": os.path.realpath(__file__),
        "prefix": base,
        "sage_root": os.path.join(base, sage_root),
        "distributions": [{
            "name": "synthetic-sage",
            "version": version,
            "location": base,
            "metadata_path": base,
            "record": record,
            "direct_url": direct_url,
        }],
        "conda_records": [{"path": "synthetic-sage.json", "content": conda}],
    }, sys.stdout, separators=(",", ":"))
    sys.stdout.write("\n")
    raise SystemExit(0)

if sys.argv[1:] not in (["-", "--jsonl"], ["-I", "-", "--jsonl-fd", "3"]):
    raise SystemExit(64)
count_path = os.path.join(base, "extractions")
with open(count_path, "a+", encoding="utf-8") as count_file:
    fcntl.flock(count_file.fileno(), fcntl.LOCK_EX)
    count_file.seek(0)
    text = count_file.read().strip()
    count = int(text) if text else 0
    count_file.seek(0)
    count_file.truncate()
    count_file.write(str(count + 1))
    count_file.flush()
    fcntl.flock(count_file.fileno(), fcntl.LOCK_UN)

attempt = count + 1
barrier_path = os.path.join(base, "extraction-barriers")
if os.path.exists(barrier_path):
    with open(barrier_path, encoding="utf-8") as barriers:
        barrier_attempts = {int(line) for line in barriers if line.strip()}
    if attempt in barrier_attempts:
        token = struct.pack(">I", attempt)
        with open(os.path.join(base, "extraction-ready"), "wb", buffering=0) as ready:
            ready.write(token)
        with open(os.path.join(base, "extraction-release"), "rb", buffering=0) as release:
            released = release.read(len(token))
        if released != token:
            raise RuntimeError("invalid extraction release token")

records = [
    {
        "name": "GF",
        "qualname": "sage.rings.finite_rings.finite_field_constructor.FiniteFieldFactory",
        "kind": "object",
        "signature": "GF(order, name=None)",
        "docstring": "Return a finite field. Construct finite field elements and polynomial examples.",
        "examples": "",
        "file": "sage/rings/finite_rings/finite_field_constructor.py",
        "line": 100,
    },
    {
        "name": "factor",
        "qualname": "sage.arith.misc.factor",
        "kind": "function",
        "signature": "factor(n)",
        "docstring": "Return the factorization of an integer into prime factors.",
        "examples": "",
        "file": "sage/arith/misc.py",
        "line": 200,
    },
]
protocol = os.fdopen(3, "w", encoding="utf-8")
for record in records:
    json.dump(record, protocol, separators=(",", ":"))
    protocol.write("\n")
protocol.close()
`, realPython)
	if err := os.WriteFile(fixture.python, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func cachetestWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (fixture *cachetestFixture) cachetestOptions() indexOptions {
	return indexOptions{
		ConfiguredPython: fixture.python,
		CacheRoot:        fixture.cache,
		Executable:       fixture.executable,
	}
}

func (fixture *cachetestFixture) cachetestExtractionCount(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(fixture.counter)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("invalid extraction count %q: %v", data, err)
	}
	return count
}

func (fixture *cachetestFixture) cachetestDatabasePaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(fixture.cache, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".db") {
			paths = append(paths, path)
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func (fixture *cachetestFixture) cachetestAssertNoTemporaryFiles(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir(fixture.cache, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary build artifact remains: %s", path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func cachetestOpenAndCheckGF(t *testing.T, fixture *cachetestFixture) {
	t.Helper()
	index, err := openIndex(context.Background(), fixture.cachetestOptions())
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := index.Lookup("GF")
	if err != nil {
		_ = index.Close()
		t.Fatal(err)
	}
	if envelope.Mode != "exact" || len(envelope.Matches) != 1 || envelope.Matches[0].Name != "GF" {
		_ = index.Close()
		t.Fatalf("unexpected GF result: %+v", envelope)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
}

func cachetestRunCLI(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if status := cli(context.Background(), args, &stdout, &stderr); status != 0 {
		t.Fatalf("cli(%q) status = %d; stderr: %s", args, status, stderr.String())
	}
	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatalf("cli(%q) returned invalid JSON %q: %v", args, stdout.String(), err)
	}
	return value
}

func cachetestMatches(t *testing.T, envelope map[string]any) []any {
	t.Helper()
	matches, ok := envelope["matches"].([]any)
	if !ok {
		t.Fatalf("matches is absent or not an array: %#v", envelope)
	}
	return matches
}

func cachetestMatch(t *testing.T, value any) map[string]any {
	t.Helper()
	match, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("match is not an object: %#v", value)
	}
	return match
}

func TestCacheCLIColdBuildWarmReuseAndJSONEnvelopes(t *testing.T) {
	fixture := cachetestNewFixture(t)
	t.Setenv("SAGEDOC_PYTHON", fixture.python)
	t.Setenv("SAGEDOC_CACHE_DIR", fixture.cache)

	exact := cachetestRunCLI(t, "GF")
	if exact["mode"] != "exact" {
		t.Fatalf("exact mode = %#v", exact["mode"])
	}
	exactMatches := cachetestMatches(t, exact)
	if len(exactMatches) != 1 {
		t.Fatalf("exact matches = %#v", exactMatches)
	}
	exactMatch := cachetestMatch(t, exactMatches[0])
	if exactMatch["name"] != "GF" || exactMatch["kind"] != "object" {
		t.Fatalf("unexpected exact match: %#v", exactMatch)
	}
	if _, ok := exactMatch["docstring"]; !ok {
		t.Fatalf("exact match omits docstring: %#v", exactMatch)
	}
	for _, omitted := range []string{"snippet", "score"} {
		if _, ok := exactMatch[omitted]; ok {
			t.Errorf("exact match unexpectedly contains %q: %#v", omitted, exactMatch)
		}
	}
	if _, ok := exact["candidates"]; ok {
		t.Errorf("exact envelope unexpectedly contains candidates: %#v", exact)
	}

	miss := cachetestRunCLI(t, "gf")
	if miss["mode"] != "miss" || len(cachetestMatches(t, miss)) != 0 {
		t.Fatalf("unexpected miss envelope: %#v", miss)
	}
	candidates, ok := miss["candidates"].([]any)
	if !ok || len(candidates) != 1 || candidates[0] != "GF" {
		t.Fatalf("miss candidates = %#v", miss["candidates"])
	}

	nonverbose := cachetestRunCLI(t, "finite", "field")
	if nonverbose["mode"] != "search" {
		t.Fatalf("search mode = %#v", nonverbose["mode"])
	}
	searchMatches := cachetestMatches(t, nonverbose)
	if len(searchMatches) == 0 {
		t.Fatal("nonverbose search returned no matches")
	}
	searchMatch := cachetestMatch(t, searchMatches[0])
	if _, ok := searchMatch["snippet"]; !ok {
		t.Fatalf("search match omits snippet: %#v", searchMatch)
	}
	for _, omitted := range []string{"docstring", "score"} {
		if _, ok := searchMatch[omitted]; ok {
			t.Errorf("nonverbose search match unexpectedly contains %q: %#v", omitted, searchMatch)
		}
	}
	if _, ok := nonverbose["candidates"]; ok {
		t.Errorf("search envelope unexpectedly contains candidates: %#v", nonverbose)
	}

	verbose := cachetestRunCLI(t, "-v", "finite", "field")
	verboseMatches := cachetestMatches(t, verbose)
	if len(verboseMatches) == 0 {
		t.Fatal("verbose search returned no matches")
	}
	if score, ok := cachetestMatch(t, verboseMatches[0])["score"].(float64); !ok || score <= 0 {
		t.Fatalf("verbose score is absent or nonpositive: %#v", verboseMatches[0])
	}

	// -v changes search presentation only; exact
	// output still omits score.
	verboseExact := cachetestRunCLI(t, "-v", "GF")
	if _, ok := cachetestMatch(t, cachetestMatches(t, verboseExact)[0])["score"]; ok {
		t.Fatalf("verbose exact result unexpectedly contains score: %#v", verboseExact)
	}

	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("cold build followed by warm queries ran %d extractions, want 1", got)
	}
	if got := len(fixture.cachetestDatabasePaths(t)); got != 1 {
		t.Fatalf("database count = %d, want 1", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func TestCacheInputMutationsSelectRebuild(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, filepath.Join(fixture.sageRoot, "all.py"), "# synthetic sage.all v2\n")
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, fixture.executable, "synthetic sagedoc executable v2\n")
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, fixture.distribution, "2.0\n")
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, fixture.record, "module.py,sha256=two,1\n")
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, fixture.directURL, `{"url":"file:///two"}`)
	cachetestOpenAndCheckGF(t, fixture)

	cachetestWriteFile(t, fixture.conda, `{"name":"synthetic-sage","version":"2.0"}`)
	cachetestOpenAndCheckGF(t, fixture)

	if got := fixture.cachetestExtractionCount(t); got != 7 {
		t.Fatalf("extraction count after tree, executable, distribution, and metadata mutations = %d, want 7", got)
	}
	if got := len(fixture.cachetestDatabasePaths(t)); got != 7 {
		t.Fatalf("database count after mutations = %d, want 7", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func TestCacheCorruptDatabaseConcurrentRepair(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)
	databases := fixture.cachetestDatabasePaths(t)
	if len(databases) != 1 {
		t.Fatalf("database count = %d, want 1", len(databases))
	}
	cachetestCorruptUnprobedTable(t, databases[0])
	cachetestAssertLegacyProbesAccept(t, databases[0])

	const callers = 8
	errorsByCaller := make([]error, callers)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for caller := 0; caller < callers; caller++ {
		wait.Add(1)
		go func(caller int) {
			defer wait.Done()
			<-start
			index, err := openIndex(context.Background(), fixture.cachetestOptions())
			if err != nil {
				errorsByCaller[caller] = err
				return
			}
			envelope, queryErr := index.Lookup("GF")
			closeErr := index.Close()
			if queryErr != nil || closeErr != nil {
				errorsByCaller[caller] = errors.Join(queryErr, closeErr)
				return
			}
			if envelope.Mode != "exact" || len(envelope.Matches) != 1 {
				errorsByCaller[caller] = fmt.Errorf("invalid repaired result: %+v", envelope)
			}
		}(caller)
	}
	close(start)
	wait.Wait()
	for caller, err := range errorsByCaller {
		if err != nil {
			t.Errorf("caller %d: %v", caller, err)
		}
	}
	if got := fixture.cachetestExtractionCount(t); got != 2 {
		t.Fatalf("initial build plus concurrent repair ran %d extractions, want 2", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func TestCacheRepairsDisposableStructuralDamage(t *testing.T) {
	for _, test := range []struct {
		name   string
		damage func(t *testing.T, path string)
	}{
		{
			name: "zero length",
			damage: func(t *testing.T, path string) {
				if err := os.Truncate(path, 0); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong schema version",
			damage: func(t *testing.T, path string) {
				cachetestReplaceSQLite(t, path, fmt.Sprintf("pragma user_version = %d", indexing.SchemaVersion+1))
			},
		},
		{
			name: "right version missing required structures",
			damage: func(t *testing.T, path string) {
				cachetestReplaceSQLite(t, path, fmt.Sprintf("pragma user_version = %d", indexing.SchemaVersion))
			},
		},
		{
			name: "truncated unprobed pages",
			damage: func(t *testing.T, path string) {
				cachetestTruncateUnprobedTable(t, path)
				cachetestAssertLegacyProbesAccept(t, path)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := cachetestNewFixture(t)
			cachetestOpenAndCheckGF(t, fixture)
			path := fixture.cachetestDatabasePaths(t)[0]
			test.damage(t, path)
			cachetestOpenAndCheckGF(t, fixture)
			if got := fixture.cachetestExtractionCount(t); got != 2 {
				t.Fatalf("extraction count after repair = %d, want 2", got)
			}
			fixture.cachetestAssertNoTemporaryFiles(t)
		})
	}
}

func TestCacheRepairsDroppedRequiredSchemaObjects(t *testing.T) {
	objects := []struct {
		name, objectType string
	}{
		{name: "decls", objectType: "table"},
		{name: "names", objectType: "table"},
		{name: "fts", objectType: "table"},
		{name: "fts_data", objectType: "table"},
		{name: "fts_idx", objectType: "table"},
		{name: "fts_content", objectType: "table"},
		{name: "fts_docsize", objectType: "table"},
		{name: "fts_config", objectType: "table"},
		{name: "decls_name", objectType: "index"},
		{name: "decls_final", objectType: "index"},
		{name: "names_final", objectType: "index"},
	}
	for _, object := range objects {
		t.Run(object.objectType+" "+object.name, func(t *testing.T) {
			fixture := cachetestNewFixture(t)
			cachetestOpenAndCheckGF(t, fixture)
			path := fixture.cachetestDatabasePaths(t)[0]
			cachetestExecSQLite(t, path, "drop "+object.objectType+" "+object.name)

			cachetestOpenAndCheckGF(t, fixture)
			if got := fixture.cachetestExtractionCount(t); got != 2 {
				t.Fatalf("extraction count after repairing dropped %s = %d, want 2", object.name, got)
			}
			fixture.cachetestAssertNoTemporaryFiles(t)
		})
	}
}

func TestCacheRepairWaitsForEnvironmentLock(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)
	path := fixture.cachetestDatabasePaths(t)[0]
	lock, err := acquireEnvironmentLock(context.Background(), filepath.Join(filepath.Dir(path), "build.lock"))
	if err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			_ = releaseEnvironmentLock(lock)
		}
	}()

	cachetestExecSQLite(t, path, "drop index decls_name")
	done := make(chan error, 1)
	go func() {
		index, openErr := openIndex(context.Background(), fixture.cachetestOptions())
		if openErr != nil {
			done <- openErr
			return
		}
		done <- index.Close()
	}()

	select {
	case err := <-done:
		t.Fatalf("repair completed without acquiring the held environment lock: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("damaged cache was removed before lock acquisition: %v", err)
	}
	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("repair extracted while lock was held: extraction count = %d, want 1", got)
	}

	if err := releaseEnvironmentLock(lock); err != nil {
		t.Fatal(err)
	}
	locked = false
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := fixture.cachetestExtractionCount(t); got != 2 {
		t.Fatalf("serialized repair extraction count = %d, want 2", got)
	}
}

func TestCacheValidationEscapesSQLiteURIPath(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)
	original := fixture.cachetestDatabasePaths(t)[0]
	special := filepath.Join(filepath.Dir(original), "literal?mode=memory#fragment&immutable=0%25.db")
	if err := os.Rename(original, special); err != nil {
		t.Fatal(err)
	}
	if err := validateCacheDatabase(context.Background(), special); err != nil {
		t.Fatalf("validate cache at URI-metacharacter path: %v", err)
	}
}

func TestCacheDoesNotRemovePermissionFailure(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)
	path := fixture.cachetestDatabasePaths(t)[0]
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore cache permissions: %v", err)
		}
	}()

	index, err := openIndex(context.Background(), fixture.cachetestOptions())
	if err == nil {
		_ = index.Close()
		t.Skip("database remains readable without permission bits")
	}
	if isDisposableCacheError(err) {
		t.Fatalf("permission error was classified as disposable corruption: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("cache with permission failure was removed: %v", statErr)
	}
	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("permission error triggered extraction count %d, want 1", got)
	}
}

func TestCacheDoesNotRemoveUnrelatedOpenError(t *testing.T) {
	fixture := cachetestNewFixture(t)
	cachetestOpenAndCheckGF(t, fixture)
	path := fixture.cachetestDatabasePaths(t)[0]
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := openIndex(context.Background(), fixture.cachetestOptions())
	if err == nil {
		t.Fatal("openIndex accepted a directory at the database path")
	}
	if isDisposableCacheError(err) {
		t.Fatalf("directory open error was classified as disposable corruption: %v", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil || !info.IsDir() {
		t.Fatalf("unrelated database-path directory was removed: info=%v err=%v", info, statErr)
	}
	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("unrelated open error triggered extraction count %d, want 1", got)
	}
}

func cachetestReplaceSQLite(t *testing.T, path, statement string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	cachetestExecSQLite(t, path, statement)
}

func cachetestExecSQLite(t *testing.T, path, statement string) {
	t.Helper()
	dsn, err := sqliteFileDSN(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := database.Exec(statement)
	closeErr := database.Close()
	if execErr != nil || closeErr != nil {
		t.Fatal(errors.Join(execErr, closeErr))
	}
}

func cachetestAddUnprobedTable(t *testing.T, path string) (rootPage, pageSize int) {
	t.Helper()
	dsn, err := sqliteFileDSN(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`create table cachetest_unprobed(
		id integer primary key,
		payload text not null
	)`)
	if err == nil {
		_, err = database.Exec(`insert into cachetest_unprobed(payload)
			with recursive rows(n) as (
				values(1) union all select n + 1 from rows where n < 2000
			)
			select printf('%0800d', n) from rows`)
	}
	if err == nil {
		err = database.QueryRow("select rootpage from sqlite_schema where name = 'cachetest_unprobed'").Scan(&rootPage)
	}
	if err == nil {
		err = database.QueryRow("pragma page_size").Scan(&pageSize)
	}
	closeErr := database.Close()
	if err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	return rootPage, pageSize
}

func cachetestCorruptUnprobedTable(t *testing.T, path string) {
	t.Helper()
	rootPage, pageSize := cachetestAddUnprobedTable(t, path)
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt(bytes.Repeat([]byte{0xff}, 32), int64(rootPage-1)*int64(pageSize))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(errors.Join(writeErr, closeErr))
	}
}

func cachetestTruncateUnprobedTable(t *testing.T, path string) {
	t.Helper()
	_, pageSize := cachetestAddUnprobedTable(t, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	truncatedSize := info.Size() - int64(pageSize)
	if truncatedSize <= int64(pageSize) {
		t.Fatalf("database size = %d, cannot truncate one %d-byte page", info.Size(), pageSize)
	}
	if err := os.Truncate(path, truncatedSize); err != nil {
		t.Fatal(err)
	}

	// Keep the header's page count consistent with
	// the shorter file so opening and the normal
	// no-hit probes do not reject it immediately.
	// References in the unprobed table still reach
	// the removed page, which integrity_check must
	// discover by traversing the whole database.
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	var pageCount [4]byte
	binary.BigEndian.PutUint32(pageCount[:], uint32(truncatedSize/int64(pageSize)))
	_, writeErr := file.WriteAt(pageCount[:], 28)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(errors.Join(writeErr, closeErr))
	}
}

func cachetestAssertLegacyProbesAccept(t *testing.T, path string) {
	t.Helper()
	index, err := indexing.Open(path, SageTokenizer{})
	if err != nil {
		t.Fatalf("ordinary index open reached unprobed damage: %v", err)
	}
	probeErr := validateRequiredIndexStructure(index)
	closeErr := index.Close()
	if probeErr != nil || closeErr != nil {
		t.Fatalf("ordinary no-hit probes reached unprobed damage: %v", errors.Join(probeErr, closeErr))
	}
}

func TestCacheRealSageEndToEnd(t *testing.T) {
	python := os.Getenv("SAGEDOC_PYTHON")
	if python == "" {
		t.Skip("SAGEDOC_PYTHON is not set")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	var diagnostics bytes.Buffer
	options := indexOptions{
		ConfiguredPython: python,
		CacheRoot:        cacheRoot,
		Executable:       executable,
		Diagnostics:      &diagnostics,
	}

	coldStart := time.Now()
	index, err := openIndex(context.Background(), options)
	if err != nil {
		t.Fatalf("cold real-Sage open: %v\ndiagnostics:\n%s", err, diagnostics.String())
	}
	coldElapsed := time.Since(coldStart)

	gf, err := index.Lookup("GF")
	if err != nil {
		t.Fatal(err)
	}
	if gf.Mode != "exact" || len(gf.Matches) != 1 {
		t.Fatalf("GF lookup = %+v", gf)
	}
	gfMatch := gf.Matches[0]
	if gfMatch.Kind != "object" || !strings.Contains(gfMatch.Qualname, "FiniteFieldFactory") || !strings.HasPrefix(gfMatch.Signature, "GF(") || !strings.HasPrefix(gfMatch.Docstring, "Return") || !strings.Contains(gfMatch.Docstring, "EXAMPLES") {
		t.Errorf("unexpected GF record: %+v", gfMatch)
	}

	miss, err := index.Lookup("gf")
	if err != nil {
		t.Fatal(err)
	}
	if miss.Mode != "miss" || len(miss.Matches) != 0 || len(miss.Candidates) == 0 || miss.Candidates[0] != "GF" {
		t.Errorf("gf miss = %+v", miss)
	}

	factor, err := index.Lookup("factor")
	if err != nil {
		t.Fatal(err)
	}
	if factor.Mode != "exact" || len(factor.Matches) != 1 || factor.Matches[0].Kind != "function" || !strings.HasPrefix(factor.Matches[0].Signature, "factor(") || strings.TrimSpace(factor.Matches[0].Docstring) == "" {
		t.Errorf("factor lookup = %+v", factor)
	}

	affine, err := index.Lookup("AffineGroup")
	if err != nil {
		t.Fatal(err)
	}
	if affine.Mode != "exact" || len(affine.Matches) != 1 || affine.Matches[0].Kind != "class" || strings.Contains(affine.Matches[0].Qualname, "LazyImport") || !strings.Contains(affine.Matches[0].File, "sage/groups/affine_gps/") {
		t.Errorf("AffineGroup lookup = %+v", affine)
	}

	search, err := index.Search("finite field")
	if err != nil {
		t.Fatal(err)
	}
	if search.Mode != "search" || len(search.Matches) == 0 {
		t.Errorf("finite-field search = %+v", search)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics.String(), "summary public=") || !strings.Contains(diagnostics.String(), "indexed=") {
		t.Errorf("extractor diagnostics lack accounting summary:\n%s", diagnostics.String())
	}

	var databasePath string
	err = filepath.WalkDir(cacheRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".db") {
			if databasePath != "" {
				return fmt.Errorf("multiple databases: %q and %q", databasePath, path)
			}
			databasePath = path
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if databasePath == "" {
		t.Fatal("cold build published no database")
	}
	dsn, err := sqliteFileDSN(databasePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var declarations int
	queryErr := database.QueryRow("select count(*) from decls").Scan(&declarations)
	closeErr := database.Close()
	if queryErr != nil || closeErr != nil {
		t.Fatal(errors.Join(queryErr, closeErr))
	}
	if declarations < 1000 {
		t.Errorf("real-Sage corpus has %d declarations, want at least 1000", declarations)
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}

	diagnostics.Reset()
	warmStart := time.Now()
	warm, err := openIndex(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	warmElapsed := time.Since(warmStart)
	if _, err := warm.Lookup("GF"); err != nil {
		_ = warm.Close()
		t.Fatal(err)
	}
	if err := warm.Close(); err != nil {
		t.Fatal(err)
	}
	if diagnostics.Len() != 0 {
		t.Errorf("warm query reran extraction; diagnostics:\n%s", diagnostics.String())
	}
	t.Logf("real Sage: declarations=%d database=%d bytes cold=%v warm=%v", declarations, info.Size(), coldElapsed, warmElapsed)
}

func TestCacheConcurrentColdCallersShareExtraction(t *testing.T) {
	fixture := cachetestNewFixture(t)
	const callers = 12
	errorsByCaller := make([]error, callers)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for caller := 0; caller < callers; caller++ {
		wait.Add(1)
		go func(caller int) {
			defer wait.Done()
			<-start
			index, err := openIndex(context.Background(), fixture.cachetestOptions())
			if err != nil {
				errorsByCaller[caller] = err
				return
			}
			envelope, queryErr := index.Search("finite field")
			closeErr := index.Close()
			if queryErr != nil || closeErr != nil {
				errorsByCaller[caller] = errors.Join(queryErr, closeErr)
				return
			}
			if envelope.Mode != "search" || len(envelope.Matches) == 0 {
				errorsByCaller[caller] = fmt.Errorf("invalid search result: %+v", envelope)
			}
		}(caller)
	}
	close(start)
	wait.Wait()
	for caller, err := range errorsByCaller {
		if err != nil {
			t.Errorf("caller %d: %v", caller, err)
		}
	}
	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("%d concurrent cold callers ran %d extractions, want 1", callers, got)
	}
	if got := len(fixture.cachetestDatabasePaths(t)); got != 1 {
		t.Fatalf("database count = %d, want 1", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func TestCacheRediscoveryDetectsChangeAfterLockSelection(t *testing.T) {
	fixture := cachetestNewFixture(t)
	options := fixture.cachetestOptions()
	preliminary, fingerprint, environmentDir, _, err := discoverSelection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(fixture.dir, "sage-retargeted")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cachetestWriteFile(t, filepath.Join(newRoot, "all.py"), "# retargeted synthetic sage.all\n")
	cachetestWriteFile(t, fixture.rootSelection, "sage-retargeted\n")

	_, err = selectOrBuildIndex(context.Background(), options, preliminary, fingerprint, environmentDir, 1)
	if !errors.Is(err, errFingerprintChanged) {
		t.Fatalf("selectOrBuildIndex error = %v, want installation-change sentinel", err)
	}
	if got := fixture.cachetestExtractionCount(t); got != 0 {
		t.Fatalf("stale locked selection ran %d extractions, want 0", got)
	}
}

func TestCacheChangeDuringExtractionAbortsAndReselects(t *testing.T) {
	fixture := cachetestNewFixture(t)
	barrier := fixture.cachetestNewExtractionBarrier(t, 1)
	mutation := make(chan error, 1)
	go func() {
		if err := barrier.cachetestWaitReady(1); err != nil {
			mutation <- err
			return
		}
		newRoot := filepath.Join(fixture.dir, "sage-retargeted")
		mutationErr := os.MkdirAll(newRoot, 0o755)
		if mutationErr == nil {
			mutationErr = os.WriteFile(filepath.Join(newRoot, "all.py"), []byte("# synthetic sage.all v2\n"), 0o644)
		}
		if mutationErr == nil {
			mutationErr = os.WriteFile(fixture.rootSelection, []byte("sage-retargeted\n"), 0o644)
		}
		mutation <- errors.Join(mutationErr, barrier.cachetestRelease(1))
	}()

	cachetestOpenAndCheckGF(t, fixture)
	if err := <-mutation; err != nil {
		t.Fatal(err)
	}
	if got := fixture.cachetestExtractionCount(t); got != 2 {
		t.Fatalf("changed installation ran %d extractions, want one aborted and one published", got)
	}
	if got := len(fixture.cachetestDatabasePaths(t)); got != 1 {
		t.Fatalf("published database count = %d, want 1", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func TestCacheContinuouslyChangingInstallationStopsAfterBoundedRetries(t *testing.T) {
	fixture := cachetestNewFixture(t)
	attempts := make([]int, maxFingerprintBuildAttempts)
	for attempt := range attempts {
		attempts[attempt] = attempt + 1
	}
	barrier := fixture.cachetestNewExtractionBarrier(t, attempts...)
	mutation := make(chan error, 1)
	go func() {
		for count := 1; count <= maxFingerprintBuildAttempts; count++ {
			if err := barrier.cachetestWaitReady(count); err != nil {
				mutation <- err
				return
			}
			version := strconv.Itoa(count+1) + "\n"
			mutationErr := os.WriteFile(fixture.distribution, []byte(version), 0o644)
			releaseErr := barrier.cachetestRelease(count)
			if mutationErr != nil || releaseErr != nil {
				mutation <- errors.Join(mutationErr, releaseErr)
				return
			}
		}
		mutation <- nil
	}()

	_, err := openIndex(context.Background(), fixture.cachetestOptions())
	if !errors.Is(err, errFingerprintChanged) {
		t.Fatalf("openIndex error = %v, want installation-change sentinel", err)
	}
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("bounded retry error = %v", err)
	}
	if mutationErr := <-mutation; mutationErr != nil {
		t.Fatal(mutationErr)
	}
	if got := fixture.cachetestExtractionCount(t); got != maxFingerprintBuildAttempts {
		t.Fatalf("extraction count = %d, want bounded %d", got, maxFingerprintBuildAttempts)
	}
	if got := len(fixture.cachetestDatabasePaths(t)); got != 0 {
		t.Fatalf("stale database count = %d, want 0", got)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

const cachetestBarrierTimeout = 10 * time.Second

type cachetestExtractionBarrier struct {
	ready   *os.File
	release *os.File
}

func (fixture *cachetestFixture) cachetestNewExtractionBarrier(t *testing.T, attempts ...int) *cachetestExtractionBarrier {
	t.Helper()
	readyPath := filepath.Join(fixture.dir, "extraction-ready")
	releasePath := filepath.Join(fixture.dir, "extraction-release")
	for _, path := range []string{readyPath, releasePath} {
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatalf("create extraction barrier FIFO: %v", err)
		}
	}
	openFIFO := func(path string) *os.File {
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatalf("open extraction barrier FIFO: %v", err)
		}
		return os.NewFile(uintptr(fd), path)
	}
	barrier := &cachetestExtractionBarrier{
		ready:   openFIFO(readyPath),
		release: openFIFO(releasePath),
	}
	t.Cleanup(func() {
		if err := errors.Join(barrier.ready.Close(), barrier.release.Close()); err != nil {
			t.Errorf("close extraction barrier: %v", err)
		}
	})

	var configured strings.Builder
	for _, attempt := range attempts {
		if attempt < 1 {
			t.Fatalf("invalid extraction barrier attempt %d", attempt)
		}
		fmt.Fprintln(&configured, attempt)
	}
	cachetestWriteFile(t, filepath.Join(fixture.dir, "extraction-barriers"), configured.String())
	return barrier
}

func (barrier *cachetestExtractionBarrier) cachetestWaitReady(want int) error {
	if err := barrier.ready.SetReadDeadline(time.Now().Add(cachetestBarrierTimeout)); err != nil {
		return fmt.Errorf("set extraction-ready deadline: %w", err)
	}
	var token [4]byte
	if _, err := io.ReadFull(barrier.ready, token[:]); err != nil {
		return fmt.Errorf("wait for extraction %d ready: %w", want, err)
	}
	if got := int(binary.BigEndian.Uint32(token[:])); got != want {
		releaseErr := barrier.cachetestRelease(got)
		return errors.Join(fmt.Errorf("extraction ready attempt = %d, want %d", got, want), releaseErr)
	}
	return nil
}

func (barrier *cachetestExtractionBarrier) cachetestRelease(attempt int) error {
	if err := barrier.release.SetWriteDeadline(time.Now().Add(cachetestBarrierTimeout)); err != nil {
		return fmt.Errorf("set extraction-release deadline: %w", err)
	}
	var token [4]byte
	binary.BigEndian.PutUint32(token[:], uint32(attempt))
	if _, err := barrier.release.Write(token[:]); err != nil {
		return fmt.Errorf("release extraction %d: %w", attempt, err)
	}
	return nil
}

func TestCacheLockCancellationAndStableInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.lock")
	first, err := acquireEnvironmentLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	firstInfo, err := first.Stat()
	if err != nil {
		_ = releaseEnvironmentLock(first)
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	blocked, err := acquireEnvironmentLock(ctx, path)
	if blocked != nil {
		_ = releaseEnvironmentLock(blocked)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		_ = releaseEnvironmentLock(first)
		t.Fatalf("blocked acquisition error = %v, want deadline exceeded", err)
	}
	if err := releaseEnvironmentLock(first); err != nil {
		t.Fatal(err)
	}

	second, err := acquireEnvironmentLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := second.Stat()
	if err != nil {
		_ = releaseEnvironmentLock(second)
		t.Fatal(err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		_ = releaseEnvironmentLock(second)
		t.Fatalf("lock inode changed across cancellation/reacquisition: before=%v after=%v", firstInfo, secondInfo)
	}
	if err := releaseEnvironmentLock(second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("persistent lock file missing after release: %v", err)
	}
}

// Keep the indexing import anchored
// in this cache-focused test file:
// these tests intentionally validate
// sagedoc's public JSON behavior, not a
// parallel test-only representation of
// index records.
var _ *indexing.Envelope

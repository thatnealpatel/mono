package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildIndexFailuresAbortTemporaryDatabase(t *testing.T) {
	verificationErr := errors.New("synthetic verification failure")
	tests := []struct {
		name       string
		mode       string
		want       []string
		verifyFail bool
	}{
		{name: "malformed JSON", mode: "malformed-hang", want: []string{"decode Sage extractor output", "invalid character"}},
		{name: "truncated JSON", mode: "truncated", want: []string{"decode Sage extractor output", "unexpected EOF"}},
		{name: "nonzero child exit", mode: "valid-nonzero", want: []string{"wait for Sage extraction", "exit status 7"}},
		{name: "verification failure", mode: "stderr-success", want: []string{verificationErr.Error()}, verifyFail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "index.db")
			marker := filepath.Join(dir, "extractor.pid")
			python := procHelperWrapper(t, test.mode, marker)
			var verify func() error
			if test.verifyFail {
				verify = func() error {
					if _, err := os.Stat(path + ".tmp"); err != nil {
						return fmt.Errorf("temporary database was unavailable during verification: %w", err)
					}
					if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("final database existed before verification completed: %v", err)
					}
					return verificationErr
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := buildIndex(ctx, python, path, nil, verify)
			if err == nil {
				t.Fatal("buildIndex unexpectedly succeeded")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("buildIndex error = %v, want failure containing %q", err, want)
				}
			}
			if test.verifyFail && !errors.Is(err, verificationErr) {
				t.Fatalf("buildIndex error = %v, want wrapped verification failure", err)
			}
			lifecycleAssertNoBuildArtifacts(t, path)
			procAssertReaped(t, marker)
		})
	}
}

func TestBuildIndexCancellationAbortsAndReaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	marker := filepath.Join(dir, "extractor.pid")
	python := procHelperWrapper(t, "idle", marker)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- buildIndex(ctx, python, path, nil, nil)
	}()

	procWaitForMarker(t, marker)
	if _, err := os.Stat(path + ".tmp"); err != nil {
		cancel()
		<-result
		t.Fatalf("builder temporary database was not present before cancellation: %v", err)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("buildIndex error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("buildIndex did not return after cancellation")
	}
	lifecycleAssertNoBuildArtifacts(t, path)
	procAssertReaped(t, marker)
}

func lifecycleAssertNoBuildArtifacts(t *testing.T, path string) {
	t.Helper()
	for _, artifact := range []string{path, path + ".tmp"} {
		if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("failed build artifact %q remains: %v", artifact, err)
		}
	}
}

func TestOpenIndexFlockContentionReselectsRetargetedEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("deterministic observation of the flock contender requires /proc/self/fd")
	}
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skipf("cannot observe open lock descriptors: %v", err)
	}

	fixture := cachetestNewFixture(t)
	baseOptions := fixture.cachetestOptions()
	_, _, oldEnvironmentDir, oldDatabasePath, err := discoverSelection(context.Background(), baseOptions)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(oldEnvironmentDir, "build.lock")
	heldLock, err := acquireEnvironmentLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}

	// Signal only after the preliminary discovery subprocess has completely
	// emitted the old selection. This avoids timing guesses: observing a
	// second descriptor for lockPath below then proves openIndex reached the
	// real flock acquisition loop with that old selection.
	discoveryDone := filepath.Join(fixture.dir, "preliminary-discovery.done")
	signalingPython := filepath.Join(fixture.dir, "signaling-python")
	wrapper := "#!/bin/sh\n" +
		"if [ \"$#\" -eq 2 ] && [ \"$1\" = -I ] && [ \"$2\" = - ]; then\n" +
		"  " + procShellQuote(fixture.python) + " \"$@\"\n" +
		"  status=$?\n" +
		"  printf '%s\\n' \"$$\" > " + procShellQuote(discoveryDone) + "\n" +
		"  exit \"$status\"\n" +
		"fi\n" +
		"exec " + procShellQuote(fixture.python) + " \"$@\"\n"
	if err := os.WriteFile(signalingPython, []byte(wrapper), 0o755); err != nil {
		_ = releaseEnvironmentLock(heldLock)
		t.Fatal(err)
	}
	options := baseOptions
	options.ConfiguredPython = signalingPython

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	type openResult struct{ err error }
	result := make(chan openResult, 1)
	started := false
	consumed := false
	defer func() {
		if heldLock != nil {
			_ = releaseEnvironmentLock(heldLock)
		}
		cancel()
		if started && !consumed {
			select {
			case <-result:
			case <-time.After(5 * time.Second):
			}
		}
	}()

	started = true
	go func() {
		index, openErr := openIndex(ctx, options)
		if openErr != nil {
			result <- openResult{err: openErr}
			return
		}
		envelope, lookupErr := index.Lookup("GF")
		closeErr := index.Close()
		if lookupErr == nil && (envelope.Mode != "exact" || len(envelope.Matches) != 1) {
			lookupErr = fmt.Errorf("unexpected GF result: %+v", envelope)
		}
		result <- openResult{err: errors.Join(lookupErr, closeErr)}
	}()

	if err := lifecycleWaitForFile(discoveryDone, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := lifecycleWaitForOpenFileCount(lockPath, 2, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		consumed = true
		t.Fatalf("openIndex returned before held flock was released: %v", got.err)
	default:
	}

	newRoot := filepath.Join(fixture.dir, "sage-retargeted-under-lock")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cachetestWriteFile(t, filepath.Join(newRoot, "all.py"), "# fresh locked Sage environment\n")
	cachetestWriteFile(t, fixture.rootSelection, filepath.Base(newRoot)+"\n")
	_, _, freshEnvironmentDir, freshDatabasePath, err := discoverSelection(context.Background(), baseOptions)
	if err != nil {
		t.Fatal(err)
	}
	if freshEnvironmentDir != oldEnvironmentDir {
		t.Fatalf("retargeting unexpectedly changed lock directory: old=%q fresh=%q", oldEnvironmentDir, freshEnvironmentDir)
	}
	if freshDatabasePath == oldDatabasePath {
		t.Fatalf("retargeting did not change selected database path: %q", freshDatabasePath)
	}

	if err := releaseEnvironmentLock(heldLock); err != nil {
		heldLock = nil
		t.Fatal(err)
	}
	heldLock = nil
	select {
	case got := <-result:
		consumed = true
		if got.err != nil {
			t.Fatalf("openIndex after retargeting: %v", got.err)
		}
	case <-ctx.Done():
		t.Fatalf("openIndex did not finish after flock release: %v", ctx.Err())
	}

	if got := fixture.cachetestExtractionCount(t); got != 1 {
		t.Fatalf("extraction count = %d, want only one build for the fresh selection", got)
	}
	databases := fixture.cachetestDatabasePaths(t)
	if len(databases) != 1 || databases[0] != freshDatabasePath {
		t.Fatalf("published databases = %q, want only fresh database %q", databases, freshDatabasePath)
	}
	lifecycleAssertNoBuildArtifacts(t, oldDatabasePath)
	if _, err := os.Stat(freshDatabasePath + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh temporary database remains: %v", err)
	}
	fixture.cachetestAssertNoTemporaryFiles(t)
}

func lifecycleWaitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect synchronization file %q: %w", path, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for synchronization file %q", path)
}

func lifecycleWaitForOpenFileCount(path string, want int, timeout time.Duration) error {
	target, err := os.Stat(path)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, readErr := os.ReadDir("/proc/self/fd")
		if readErr != nil {
			return readErr
		}
		count := 0
		for _, entry := range entries {
			info, statErr := os.Stat(filepath.Join("/proc/self/fd", entry.Name()))
			if statErr == nil && os.SameFile(target, info) {
				count++
			}
		}
		if count >= want {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %d open descriptors for %q", want, path)
}

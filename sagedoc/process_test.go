package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"patel.codes/indexing"
)

type procRecordingAdder struct {
	mu      sync.Mutex
	records []indexing.Record
	err     error
}

func (a *procRecordingAdder) Add(record indexing.Record) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.records = append(a.records, record)
	return nil
}

func (a *procRecordingAdder) snapshot() []indexing.Record {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]indexing.Record(nil), a.records...)
}

type procErrorWriter struct {
	err error
}

func (writer procErrorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type procShortWriter struct {
	n int
}

type procBlockingErrorWriter struct {
	entered chan struct{}
	release chan struct{}
	err     error
	once    sync.Once
}

func (writer *procBlockingErrorWriter) Write([]byte) (int, error) {
	writer.once.Do(func() { close(writer.entered) })
	<-writer.release
	return 0, writer.err
}

// procNaturalExitErrorAdder holds
// runExtractor in Add until Linux
// reports the helper as a zombie.
// Since runExtractor cannot start its
// kill/Wait cleanup until Add returns,
// this observes exit without reaping
// away the status under test.
type procNaturalExitErrorAdder struct {
	pidMarker           string
	err                 error
	naturalExitObserved bool
}

func (adder *procNaturalExitErrorAdder) Add(indexing.Record) error {
	data, err := os.ReadFile(adder.pidMarker)
	if err != nil {
		return errors.Join(adder.err, fmt.Errorf("read helper PID marker: %w", err))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return errors.Join(adder.err, fmt.Errorf("parse helper PID marker %q: %w", data, err))
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		status, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if readErr != nil {
			return errors.Join(adder.err, fmt.Errorf("read helper process status: %w", readErr))
		}
		for _, line := range strings.Split(string(status), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "State:" && fields[1] == "Z" {
				adder.naturalExitObserved = true
				return adder.err
			}
		}
		if time.Now().After(deadline) {
			return errors.Join(adder.err, fmt.Errorf("helper PID %d did not become a zombie", pid))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (writer procShortWriter) Write(data []byte) (int, error) {
	if writer.n > len(data) {
		return len(data), nil
	}
	return writer.n, nil
}

type procSeekingAdder struct {
	found indexing.Record
}

var procTargetFound = errors.New("target record found")

func (a *procSeekingAdder) Add(record indexing.Record) error {
	if record.Name != "AffineGroup" {
		return nil
	}
	a.found = record
	return procTargetFound
}

func TestExtractorSyntheticSage(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the synthetic extractor test")
	}

	root := t.TempDir()
	procWriteFile(t, filepath.Join(root, "sage", "__init__.py"), "")
	procWriteFile(t, filepath.Join(root, "sage", "misc", "__init__.py"), "")
	procWriteFile(t, filepath.Join(root, "sage", "fixture_module.py"), `"""Public fixture module documentation."""
`)
	procWriteFile(t, filepath.Join(root, "sage", "misc", "lazy_import.py"), `import os

class LazyImport:
    def __init__(self, value=None, error=None, noisy=False):
        self._value = value
        self._error = error
        self._noisy = noisy

    def _get_object(self):
        if self._noisy:
            print("PYTHON_LAZY_NOISE")
            os.write(1, b"NATIVE_LAZY_NOISE\n")
        if self._error is not None:
            raise self._error
        return self._value
`)
	procWriteFile(t, filepath.Join(root, "sage", "misc", "sageinspect.py"), `def _flag(value, name, default=None):
    return getattr(value, name, default)


def sage_getdoc_original(value):
    mode = _flag(value, "_proc_doc_mode", "")
    if mode == "raise":
        raise RuntimeError("original documentation unavailable")
    if mode == "empty":
        return "   "
    if mode == "original":
        return "  Original wins.  "
    return getattr(value, "__doc__")


def sage_getdef(value, name):
    if _flag(value, "_proc_signature_fallback", False):
        return name + "(value, scale=2)"
    raise RuntimeError("definition unavailable")


def sage_getfile_relative(value):
    if _flag(value, "_proc_file_failure", False):
        raise OSError("file unavailable")
    return _flag(value, "_proc_file", "/installed/prefix/sage/default.py")


def sage_getsourcelines(value):
    if _flag(value, "_proc_line_failure", False):
        raise OSError("lines unavailable")
    return (["source"], _flag(value, "_proc_line", 17))
`)
	procWriteFile(t, filepath.Join(root, "sage", "all.py"), `import os as _os
from sage.misc.lazy_import import LazyImport as _LazyImport
import sage.fixture_module as _public_module

print("PYTHON_IMPORT_NOISE")
_os.write(1, b"NATIVE_IMPORT_NOISE\n")

class DemoClass:
    """Demo class documentation."""
    def __init__(self, value=1):
        self.value = value


def DemoFunction(left, right=2):
    """Demo function documentation."""
    return left + right
DemoFunction._proc_file = "/temporary/conda/build/sage/calculus/demo.pyx"
DemoFunction._proc_line = 123


class _CallableThing:
    """Callable object documentation."""
    _proc_signature_fallback = True

    @property
    def __signature__(self):
        raise ValueError("inspect.signature deliberately unavailable")

    def __call__(self, value, scale=2):
        return value * scale
CallableObject = _CallableThing()


class _SignatureFailureThing:
    """Signature failure remains documented."""
    @property
    def __signature__(self):
        raise ValueError("inspect.signature deliberately unavailable")

    def __call__(self, value):
        return value
SignatureFailure = _SignatureFailureThing()


class _BrokenNameMeta(type):
    def __getattribute__(cls, name):
        if name in ("__module__", "__qualname__"):
            raise RuntimeError("qualified name unavailable")
        return super().__getattribute__(name)


class _QualnameFailureThing(metaclass=_BrokenNameMeta):
    """Qualname failure remains documented."""
    def __call__(self, value=3):
        return value
QualnameFailure = _QualnameFailureThing()


def FileFailure(value=1):
    """File failure remains documented."""
    return value
FileFailure._proc_file_failure = True


def LineFailure(value=1):
    """Line failure remains documented."""
    return value
LineFailure._proc_file = "/other/tree/sage/rings/line.py"
LineFailure._proc_line_failure = True


def OriginalPreferred():
    """Fallback must not replace a nonempty original."""
OriginalPreferred._proc_doc_mode = "original"


def OriginalRaisesFallback():
    """Fallback after original raises."""
OriginalRaisesFallback._proc_doc_mode = "raise"


def OriginalEmptyFallback():
    """Fallback after empty original."""
OriginalEmptyFallback._proc_doc_mode = "empty"


def EmptyDoc():
    """   """
EmptyDoc._proc_doc_mode = "empty"


class _BrokenDoc:
    _proc_doc_mode = "raise"
    def __getattribute__(self, name):
        if name == "__doc__":
            raise AttributeError("fallback documentation unavailable")
        return super().__getattribute__(name)
DocBothFail = _BrokenDoc()


class _HugeThing:
    pass
_HugeThing.__doc__ = "H" * 70000
Huge = _HugeThing()


class _LazyTarget:
    """Resolved lazy class documentation."""
    pass
LazySuccess = _LazyImport(_LazyTarget, noisy=True)
LazyFailure = _LazyImport(error=ImportError("optional package absent"))

PublicModule = _public_module
NoneValue = None

_PUBLIC_NAMES = [
    "CallableObject", "DemoClass", "DemoFunction", "DocBothFail", "EmptyDoc",
    "FileFailure", "Huge", "LazyFailure", "LazySuccess", "LineFailure",
    "MissingAttribute", "NoneValue", "OriginalEmptyFallback", "OriginalPreferred",
    "OriginalRaisesFallback", "PublicModule", "QualnameFailure", "SignatureFailure",
]


def __dir__():
    return list(_PUBLIC_NAMES)


def __getattr__(name):
    if name == "MissingAttribute":
        raise LookupError("synthetic getattr failure")
    raise AttributeError(name)
`)

	wrapper := procPythonWrapper(t, python, root)
	var diagnostics bytes.Buffer
	adder := &procRecordingAdder{}
	count, err := runExtractor(context.Background(), wrapper, adder, &diagnostics)
	if err != nil {
		t.Fatalf("runExtractor: %v\ndiagnostics:\n%s", err, diagnostics.String())
	}
	if count != 13 {
		t.Fatalf("record count = %d, want 13; records: %#v", count, adder.snapshot())
	}

	byName := make(map[string]indexing.Record)
	for _, record := range adder.snapshot() {
		if _, exists := byName[record.Name]; exists {
			t.Fatalf("duplicate record %q", record.Name)
		}
		byName[record.Name] = record
	}
	for _, omitted := range []string{"DocBothFail", "EmptyDoc", "LazyFailure", "MissingAttribute", "NoneValue"} {
		if _, exists := byName[omitted]; exists {
			t.Errorf("omitted name %q was indexed", omitted)
		}
	}

	procCheckRecord(t, byName, "PublicModule", "module", "sage.fixture_module")
	procCheckRecord(t, byName, "DemoClass", "class", "sage.all.DemoClass")
	procCheckRecord(t, byName, "DemoFunction", "function", "sage.all.DemoFunction")
	procCheckRecord(t, byName, "CallableObject", "object", "sage.all._CallableThing")
	procCheckRecord(t, byName, "LazySuccess", "class", "sage.all._LazyTarget")

	if got := byName["CallableObject"].Signature; got != "CallableObject(value, scale=2)" {
		t.Errorf("sage_getdef fallback signature = %q", got)
	}
	if got := byName["DemoFunction"].Signature; got != "DemoFunction(left, right=2)" {
		t.Errorf("inspect.signature result = %q", got)
	}
	if got := byName["DemoFunction"].File; got != "sage/calculus/demo.pyx" {
		t.Errorf("file suffix = %q, want sage/calculus/demo.pyx", got)
	}
	if got := byName["DemoFunction"].Line; got != 123 {
		t.Errorf("source line = %d, want 123", got)
	}

	if got := byName["OriginalPreferred"].Docstring; got != "Original wins." {
		t.Errorf("preferred original docstring = %q", got)
	}
	if got := byName["OriginalRaisesFallback"].Docstring; got != "Fallback after original raises." {
		t.Errorf("doc fallback after error = %q", got)
	}
	if got := byName["OriginalEmptyFallback"].Docstring; got != "Fallback after empty original." {
		t.Errorf("doc fallback after empty original = %q", got)
	}
	if got := len(byName["Huge"].Docstring); got != 70000 {
		t.Errorf("large docstring length = %d, want 70000", got)
	}

	if got := byName["SignatureFailure"].Signature; got != "" {
		t.Errorf("signature failure metadata = %q, want empty", got)
	}
	if got := byName["SignatureFailure"].Qualname; got == "" {
		t.Error("signature failure unexpectedly erased independent qualname metadata")
	}
	if got := byName["QualnameFailure"].Qualname; got != "" {
		t.Errorf("qualname failure metadata = %q, want empty", got)
	}
	if got := byName["QualnameFailure"].Signature; got == "" {
		t.Error("qualname failure unexpectedly erased independent signature metadata")
	}
	if got := byName["FileFailure"].File; got != "" {
		t.Errorf("file failure metadata = %q, want empty", got)
	}
	if got := byName["FileFailure"].Line; got == 0 {
		t.Error("file failure unexpectedly erased independent line metadata")
	}
	if got := byName["LineFailure"].Line; got != 0 {
		t.Errorf("line failure metadata = %d, want zero", got)
	}
	if got := byName["LineFailure"].File; got != "sage/rings/line.py" {
		t.Errorf("line failure unexpectedly changed file metadata to %q", got)
	}

	diagnosticText := diagnostics.String()
	for _, want := range []string{
		"PYTHON_IMPORT_NOISE",
		"NATIVE_IMPORT_NOISE",
		"PYTHON_LAZY_NOISE",
		"NATIVE_LAZY_NOISE",
		"omit LazyFailure: lazy resolution ImportError",
		"omit MissingAttribute: attribute LookupError",
		"omit DocBothFail: docstring RuntimeError,AttributeError",
		"omit EmptyDoc: empty docstring",
		"summary public=18 indexed=13 none=1 empty_doc=1 attribute_failures=1 lazy_failures=1 doc_failures=1",
		"qualname_failures=1 file_failures=1 line_failures=1",
	} {
		if !strings.Contains(diagnosticText, want) {
			t.Errorf("diagnostics do not contain %q:\n%s", want, diagnosticText)
		}
	}
}

func TestExtractorRealSageAffineGroup(t *testing.T) {
	python := os.Getenv("SAGEDOC_PYTHON")
	if python == "" {
		t.Skip("SAGEDOC_PYTHON is not set")
	}

	adder := &procSeekingAdder{}
	var diagnostics bytes.Buffer
	_, err := runExtractor(context.Background(), python, adder, &diagnostics)
	if !errors.Is(err, procTargetFound) {
		t.Fatalf("extract until AffineGroup: %v\ndiagnostics:\n%s", err, diagnostics.String())
	}
	record := adder.found
	if record.Name != "AffineGroup" {
		t.Fatalf("found record name = %q", record.Name)
	}
	if record.Kind != "class" {
		t.Errorf("AffineGroup kind = %q, want class", record.Kind)
	}
	if record.Qualname == "" || strings.Contains(strings.ToLower(record.Qualname), "lazyimport") {
		t.Errorf("AffineGroup target qualname = %q", record.Qualname)
	}
	if record.File == "" || strings.Contains(strings.ToLower(record.File), "lazy_import") {
		t.Errorf("AffineGroup target file = %q", record.File)
	}
	if record.Signature == "" {
		t.Error("AffineGroup has no target signature")
	}
	if strings.TrimSpace(record.Docstring) == "" {
		t.Error("AffineGroup has no target documentation")
	}
}

func TestRunExtractorProtocolFailuresDoNotHang(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "malformed while child remains alive", mode: "malformed-hang", want: "invalid character"},
		{name: "truncated at EOF", mode: "truncated", want: "unexpected EOF"},
		{name: "unknown field", mode: "unknown-field-hang", want: "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "pid")
			wrapper := procHelperWrapper(t, test.mode, marker)
			started := time.Now()
			count, err := runExtractor(context.Background(), wrapper, &procRecordingAdder{}, nil)
			if err == nil || !strings.Contains(err.Error(), "decode Sage extractor output") || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runExtractor count=%d error=%v, want decode error containing %q", count, err, test.want)
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("protocol failure took %v; child likely was not terminated promptly", elapsed)
			}
			procAssertReaped(t, marker)
		})
	}
}

func TestRunExtractorValidRecordsThenNonzero(t *testing.T) {
	wrapper := procHelperWrapper(t, "valid-nonzero", "")
	adder := &procRecordingAdder{}
	var diagnostics bytes.Buffer
	count, err := runExtractor(context.Background(), wrapper, adder, &diagnostics)
	if err == nil {
		t.Fatal("runExtractor succeeded after nonzero child exit")
	}
	if count != 0 {
		t.Errorf("failed extraction count = %d, want 0", count)
	}
	records := adder.snapshot()
	if len(records) != 1 || records[0].Name != "ValidBeforeExit" {
		t.Errorf("records added before exit = %#v", records)
	}
	for _, want := range []string{"wait for Sage extraction", "exit status 7", "NONZERO_STDERR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if got := diagnostics.String(); !strings.Contains(got, "NONZERO_STDERR") {
		t.Errorf("forwarded diagnostics = %q", got)
	}
}

func TestRunExtractorNoisyFailureRetainsDiagnosticTail(t *testing.T) {
	wrapper := procHelperWrapper(t, "noisy-nonzero", "")
	count, err := runExtractor(context.Background(), wrapper, &procRecordingAdder{}, nil)
	if err == nil {
		t.Fatal("runExtractor succeeded after noisy nonzero child exit")
	}
	if count != 0 {
		t.Errorf("failed extraction count = %d, want 0", count)
	}
	text := err.Error()
	for _, want := range []string{"wait for Sage extraction", "exit status 9", "TAIL_DIAGNOSTIC", "[diagnostics truncated]"} {
		if !strings.Contains(text, want) {
			t.Errorf("error does not contain %q", want)
		}
	}
	if strings.Contains(text, "HEAD_DIAGNOSTIC") {
		t.Error("bounded diagnostic error retained the beginning instead of the tail")
	}
}

func TestRunExtractorForwardsSuccessfulDiagnostics(t *testing.T) {
	wrapper := procHelperWrapper(t, "stderr-success", "")
	adder := &procRecordingAdder{}
	var diagnostics bytes.Buffer
	count, err := runExtractor(context.Background(), wrapper, adder, &diagnostics)
	if err != nil {
		t.Fatalf("runExtractor: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got := diagnostics.String()
	for _, want := range []string{"SUCCESS_STDERR", "SUCCESS_STDOUT"} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics %q do not contain %q", got, want)
		}
	}
}

func TestRunExtractorDiagnosticFailureReapsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "diagnostic-hang", marker)
	writeErr := errors.New("synthetic diagnostic write failure")

	started := time.Now()
	count, err := runExtractor(context.Background(), wrapper, &procRecordingAdder{}, procErrorWriter{err: writeErr})
	if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "drain Sage extractor diagnostics") {
		t.Fatalf("runExtractor count=%d error=%v, want diagnostic write failure", count, err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("diagnostic failure took %v; child likely was not terminated promptly", elapsed)
	}
	if !strings.Contains(err.Error(), "DIAGNOSTIC_BEFORE_HANG") {
		t.Errorf("error %q does not retain the child diagnostic", err)
	}
	procAssertReaped(t, marker)
}

func TestRunExtractorCancellationReapsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "idle", marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := runExtractor(ctx, wrapper, &procRecordingAdder{}, nil)
		result <- err
	}()
	procWaitForMarker(t, marker)
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runExtractor error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runExtractor did not return after cancellation")
	}
	procAssertReaped(t, marker)
}

func TestRunExtractorCancellationPreservesDiagnosticDestinationFailure(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "diagnostic-hang", marker)
	ctx, cancel := context.WithCancel(context.Background())
	closedErr := fmt.Errorf("diagnostic destination: %w", os.ErrClosed)
	writer := &procBlockingErrorWriter{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		err:     closedErr,
	}
	result := make(chan error, 1)
	go func() {
		_, err := runExtractor(ctx, wrapper, &procRecordingAdder{}, writer)
		result <- err
	}()
	select {
	case <-writer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("extractor did not write diagnostics")
	}
	cancel()
	close(writer.release)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, os.ErrClosed) {
			t.Fatalf("runExtractor error = %v, want context cancellation and destination failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runExtractor did not return after cancellation")
	}
	procAssertReaped(t, marker)
}

func TestRunExtractorAddFailurePreservesNaturalNonzeroExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux /proc to observe an exited child without reaping it")
	}
	if _, err := os.ReadFile("/proc/self/status"); err != nil {
		t.Skipf("requires a mounted Linux /proc: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "valid-nonzero", marker)
	addErr := errors.New("synthetic add failure after natural exit")
	adder := &procNaturalExitErrorAdder{pidMarker: marker, err: addErr}
	count, err := runExtractor(context.Background(), wrapper, adder, nil)
	if !adder.naturalExitObserved {
		t.Fatalf("Add cleanup began without observing the child in a naturally exited state: %v", err)
	}
	if count != 0 || !errors.Is(err, addErr) || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("runExtractor count=%d error=%v, want Add failure joined with natural exit status 7", count, err)
	}
}

func TestRunExtractorAddFailureReapsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "valid-hang", marker)
	addErr := errors.New("synthetic add failure")
	adder := &procRecordingAdder{err: addErr}

	started := time.Now()
	count, err := runExtractor(context.Background(), wrapper, adder, nil)
	if !errors.Is(err, addErr) || !strings.Contains(err.Error(), "add Sage extractor record") {
		t.Fatalf("runExtractor count=%d error=%v, want wrapped Add failure", count, err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Add failure took %v; child likely was not terminated promptly", elapsed)
	}
	procAssertReaped(t, marker)
}

func TestRunExtractorRejectsInvalidRecordAndReapsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	wrapper := procHelperWrapper(t, "invalid-hang", marker)
	count, err := runExtractor(context.Background(), wrapper, &procRecordingAdder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "validate Sage extractor record") || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("runExtractor count=%d error=%v, want validation error", count, err)
	}
	procAssertReaped(t, marker)
}

func TestRunExtractorStartFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	count, err := runExtractor(context.Background(), missing, &procRecordingAdder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "start Sage extraction") || !strings.Contains(err.Error(), missing) {
		t.Fatalf("runExtractor count=%d error=%v, want configured-interpreter start error", count, err)
	}
}

func TestValidateRecord(t *testing.T) {
	valid := indexing.Record{Name: "GF", Kind: "object", Docstring: "documented", Line: 0}
	if err := validateRecord(valid); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	for _, kind := range []string{"class", "function", "module", "object"} {
		record := valid
		record.Kind = kind
		if err := validateRecord(record); err != nil {
			t.Errorf("kind %q rejected: %v", kind, err)
		}
	}

	tests := []struct {
		name   string
		mutate func(*indexing.Record)
		want   string
	}{
		{name: "empty name", mutate: func(r *indexing.Record) { r.Name = "" }, want: "empty name"},
		{name: "empty docstring", mutate: func(r *indexing.Record) { r.Docstring = " \n\t" }, want: "GF: empty docstring"},
		{name: "unknown kind", mutate: func(r *indexing.Record) { r.Kind = "method" }, want: `GF: unknown kind "method"`},
		{name: "negative line", mutate: func(r *indexing.Record) { r.Line = -1 }, want: "GF: negative source line -1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			test.mutate(&record)
			if err := validateRecord(record); err == nil || err.Error() != test.want {
				t.Errorf("validateRecord error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDiagnosticCaptureBoundsAndForwards(t *testing.T) {
	var destination bytes.Buffer
	capture := &diagnosticCapture{destination: &destination, limit: 5}
	if n, err := capture.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write = %d, %v; want 3, nil", n, err)
	}
	if n, err := capture.Write([]byte("def")); err != nil || n != 3 {
		t.Fatalf("second Write = %d, %v; want 3, nil", n, err)
	}
	if got := destination.String(); got != "abcdef" {
		t.Errorf("forwarded diagnostics = %q", got)
	}
	if got, want := capture.String(), "[diagnostics truncated]\nbcdef"; got != want {
		t.Errorf("bounded diagnostic tail = %q, want %q", got, want)
	}

	if n, err := capture.Write([]byte("ghijklmnop")); err != nil || n != 10 {
		t.Fatalf("oversized Write = %d, %v; want 10, nil", n, err)
	}
	if got, want := capture.String(), "[diagnostics truncated]\nlmnop"; got != want {
		t.Errorf("bounded diagnostic tail after oversized write = %q, want %q", got, want)
	}
}

func TestDiagnosticCapturePropagatesDestinationFailures(t *testing.T) {
	writeErr := errors.New("write failed")
	capture := &diagnosticCapture{destination: procErrorWriter{err: writeErr}, limit: 32}
	if n, err := capture.Write([]byte("diagnostic")); n != 0 || !errors.Is(err, writeErr) {
		t.Errorf("error Write = %d, %v; want 0, %v", n, err, writeErr)
	}
	if got := capture.String(); got != "diagnostic" {
		t.Errorf("captured diagnostic after forwarding error = %q", got)
	}

	capture = &diagnosticCapture{destination: procShortWriter{n: 2}, limit: 32}
	if n, err := capture.Write([]byte("short")); n != 2 || !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("short Write = %d, %v; want 2, %v", n, err, io.ErrShortWrite)
	}
}

// TestProcHelperProcess is launched
// through a generated interpreter
// wrapper. It deliberately ignores the
// extractor source on stdin and writes
// protocol records to the descriptor
// named by SAGEDOC_RECORD_FD.
func TestProcHelperProcess(t *testing.T) {
	if os.Getenv("PROC_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	if len(args) < 4 || args[len(args)-4] != "-I" || args[len(args)-3] != "-" || args[len(args)-2] != "--jsonl-fd" || args[len(args)-1] != "3" {
		fmt.Fprintf(os.Stderr, "unexpected extractor arguments: %q\n", args)
		os.Exit(22)
	}
	fd, err := strconv.Atoi(os.Getenv("SAGEDOC_RECORD_FD"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad SAGEDOC_RECORD_FD: %v\n", err)
		os.Exit(23)
	}
	records := os.NewFile(uintptr(fd), "sagedoc-records")
	if records == nil {
		fmt.Fprintln(os.Stderr, "record descriptor is unavailable")
		os.Exit(24)
	}
	defer records.Close()

	procWritePIDMarker(os.Getenv("PROC_MARKER"))
	valid := indexing.Record{Name: "ValidBeforeExit", Kind: "function", Docstring: "valid documentation"}
	switch os.Getenv("PROC_MODE") {
	case "malformed-hang":
		_, _ = records.WriteString("{not-json}\n")
		procSleepForever()
	case "truncated":
		_, _ = records.WriteString(`{"name":"Truncated","kind":"function"`)
		return
	case "unknown-field-hang":
		_, _ = records.WriteString(`{"name":"Unknown","kind":"function","docstring":"docs","extra":true}` + "\n")
		procSleepForever()
	case "valid-nonzero":
		_ = json.NewEncoder(records).Encode(valid)
		fmt.Fprintln(os.Stderr, "NONZERO_STDERR")
		os.Exit(7)
	case "noisy-nonzero":
		fmt.Fprintln(os.Stderr, "HEAD_DIAGNOSTIC")
		_, _ = io.WriteString(os.Stderr, strings.Repeat("x", diagnosticLimit+1024))
		fmt.Fprintln(os.Stderr, "TAIL_DIAGNOSTIC")
		os.Exit(9)
	case "stderr-success":
		_ = json.NewEncoder(records).Encode(valid)
		fmt.Fprintln(os.Stderr, "SUCCESS_STDERR")
		fmt.Fprintln(os.Stdout, "SUCCESS_STDOUT")
		return
	case "diagnostic-hang":
		fmt.Fprintln(os.Stderr, "DIAGNOSTIC_BEFORE_HANG")
		procSleepForever()
	case "idle":
		procSleepForever()
	case "valid-hang":
		_ = json.NewEncoder(records).Encode(valid)
		procSleepForever()
	case "invalid-hang":
		valid.Kind = "method"
		_ = json.NewEncoder(records).Encode(valid)
		procSleepForever()
	default:
		fmt.Fprintf(os.Stderr, "unknown PROC_MODE %q\n", os.Getenv("PROC_MODE"))
		os.Exit(25)
	}
}

func procCheckRecord(t *testing.T, records map[string]indexing.Record, name, kind, qualname string) {
	t.Helper()
	record, ok := records[name]
	if !ok {
		t.Errorf("missing record %q", name)
		return
	}
	if record.Kind != kind || record.Qualname != qualname {
		t.Errorf("record %q kind/qualname = %q/%q, want %q/%q", name, record.Kind, record.Qualname, kind, qualname)
	}
	if strings.TrimSpace(record.Docstring) == "" {
		t.Errorf("record %q has empty documentation", name)
	}
}

func procWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func procPythonWrapper(t *testing.T, python, fixtureRoot string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "python")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" = -I ]; then shift; fi\n" +
		"PYTHONPATH=" + procShellQuote(fixtureRoot) + " exec " + procShellQuote(python) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func procHelperWrapper(t *testing.T, mode, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "python")
	content := "#!/bin/sh\n" +
		"export PROC_HELPER_PROCESS=1\n" +
		"export PROC_MODE=" + procShellQuote(mode) + "\n" +
		"export PROC_MARKER=" + procShellQuote(marker) + "\n" +
		"export SAGEDOC_RECORD_FD=3\n" +
		"exec " + procShellQuote(os.Args[0]) + " -test.run='^TestProcHelperProcess$' -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func procShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func procWritePIDMarker(path string) {
	if path == "" {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

func procWaitForMarker(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if convErr != nil {
				t.Fatalf("invalid PID marker %q: %v", data, convErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read PID marker: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("helper process did not write its PID marker")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func procAssertReaped(t *testing.T, marker string) {
	t.Helper()
	pid := procWaitForMarker(t, marker)
	if runtime.GOOS == "windows" {
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("probe helper PID %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper PID %d still exists after runExtractor returned", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func procSleepForever() {
	for {
		time.Sleep(time.Hour)
	}
}

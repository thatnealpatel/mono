package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"patel.codes/indexing"
)

const diagnosticLimit = 32 << 10

//go:embed extract.py
var extractorScript []byte

type recordAdder interface {
	Add(indexing.Record) error
}

type extractorCleanup struct {
	killed bool
	err    error
}

type extractorDrain struct {
	diagnosticErr error
	cleanup       extractorCleanup
}

// runExtractor starts the configured interpreter, consumes every JSON value
// through EOF, and reaps the child before returning. Index records travel on
// a dedicated inherited descriptor so Python and native Sage stdout cannot
// corrupt the protocol.
func runExtractor(ctx context.Context, configuredPython string, adder recordAdder, diagnostics io.Writer) (int, error) {
	recordReader, recordWriter, err := os.Pipe()
	if err != nil {
		return 0, commandError(configuredPython, "create extractor record pipe", err, "")
	}
	diagnosticReader, diagnosticWriter, err := os.Pipe()
	if err != nil {
		cleanupErr := errors.Join(
			closeExtractorFile(recordReader, "close extractor record reader"),
			closeExtractorFile(recordWriter, "close extractor record writer"),
		)
		return 0, commandError(configuredPython, "create extractor diagnostic pipe", errors.Join(err, cleanupErr), "")
	}

	cmd := exec.CommandContext(ctx, configuredPython, "-I", "-", "--jsonl-fd", "3")
	// Start still rejects a context that is
	// already done. Cancellation after Start
	// is handled below so it can also close
	// inherited-pipe readers and cannot
	// leave Decode or the diagnostic drain
	// blocked by a descendant.
	cmd.Cancel = nil
	cmd.Stdin = bytes.NewReader(extractorScript)
	cmd.ExtraFiles = []*os.File{recordWriter} // descriptor 3 in the child
	cmd.Stdout = diagnosticWriter
	cmd.Stderr = diagnosticWriter
	captured := &diagnosticCapture{destination: diagnostics, limit: diagnosticLimit}

	closeBeforeStart := func() error {
		return errors.Join(
			closeExtractorFile(recordReader, "close extractor record reader"),
			closeExtractorFile(recordWriter, "close extractor record writer"),
			closeExtractorFile(diagnosticReader, "close extractor diagnostic reader"),
			closeExtractorFile(diagnosticWriter, "close extractor diagnostic writer"),
		)
	}
	if err := cmd.Start(); err != nil {
		return 0, commandError(configuredPython, "start Sage extraction", errors.Join(err, closeBeforeStart()), captured.String())
	}

	var diagnosticClosing atomic.Bool
	closeDiagnosticReader := func() error {
		diagnosticClosing.Store(true)
		return closeExtractorFile(diagnosticReader, "close extractor diagnostic reader")
	}
	terminate := func() extractorCleanup {
		killed, killErr := killExtractor(cmd.Process)
		return extractorCleanup{
			killed: killed,
			err: errors.Join(
				killErr,
				closeExtractorFile(recordReader, "close extractor record reader"),
				closeExtractorFile(recordWriter, "close extractor record writer"),
				closeDiagnosticReader(),
				closeExtractorFile(diagnosticWriter, "close extractor diagnostic writer"),
			),
		}
	}

	// stdout and stderr share one OS pipe.
	// This keeps native writes out of the
	// fd 3 protocol and gives the parent
	// an explicitly closable concurrent
	// drain, rather than opaque os/exec
	// copy goroutines.
	drainDone := make(chan extractorDrain, 1)
	go func() {
		_, copyErr := io.Copy(captured, diagnosticReader)
		var writeErr *diagnosticWriteError
		if copyErr != nil && errors.Is(copyErr, os.ErrClosed) && !errors.As(copyErr, &writeErr) && diagnosticClosing.Load() {
			copyErr = nil
		}
		closeErr := closeExtractorFile(diagnosticReader, "close extractor diagnostic reader")
		result := extractorDrain{}
		if copyErr != nil {
			result.diagnosticErr = fmt.Errorf("copy extractor diagnostics: %w", copyErr)
			result.cleanup = terminate()
		}
		result.cleanup.err = errors.Join(result.cleanup.err, closeErr)
		drainDone <- result
	}()

	cancelStop := make(chan struct{})
	cancelDone := make(chan extractorCleanup, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelDone <- terminate()
		case <-cancelStop:
			cancelDone <- extractorCleanup{}
		}
	}()

	// The parent copies of child-only write descriptors must be closed. In
	// particular, retaining recordWriter would prevent recordReader from ever
	// observing EOF.
	if err := closeExtractorFile(recordWriter, "close parent extractor record writer"); err != nil {
		return finishExtractor(configuredPython, "close extractor record pipe", err, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
	}
	if err := closeExtractorFile(diagnosticWriter, "close parent extractor diagnostic writer"); err != nil {
		return finishExtractor(configuredPython, "close extractor diagnostic pipe", err, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
	}

	decoder := json.NewDecoder(recordReader)
	decoder.DisallowUnknownFields()
	count := 0
	for {
		var record indexing.Record
		decodeErr := decoder.Decode(&record)
		if errors.Is(decodeErr, io.EOF) {
			break
		}
		if decodeErr != nil {
			operation, cause := "decode Sage extractor output", decodeErr
			if ctxErr := ctx.Err(); ctxErr != nil {
				operation, cause = "run Sage extraction", ctxErr
			}
			return finishExtractor(configuredPython, operation, cause, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
		}
		if err := validateRecord(record); err != nil {
			return finishExtractor(configuredPython, "validate Sage extractor record", err, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
		}
		if err := ctx.Err(); err != nil {
			return finishExtractor(configuredPython, "run Sage extraction", err, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
		}
		if err := adder.Add(record); err != nil {
			return finishExtractor(configuredPython, "add Sage extractor record", err, captured, cmd, terminate(), drainDone, cancelStop, cancelDone)
		}
		count++
	}

	recordCloseErr := closeExtractorFile(recordReader, "close extractor record reader")
	drain := <-drainDone
	waitErr := cmd.Wait()
	close(cancelStop)
	cancelCleanup := <-cancelDone

	if ctxErr := ctx.Err(); ctxErr != nil {
		cause := errors.Join(ctxErr, recordCloseErr, drain.diagnosticErr, drain.cleanup.err, cancelCleanup.err, extractorWaitCleanup(waitErr, drain.cleanup.killed || cancelCleanup.killed))
		return 0, commandError(configuredPython, "run Sage extraction", cause, captured.String())
	}
	if drain.diagnosticErr != nil {
		cause := errors.Join(drain.diagnosticErr, recordCloseErr, drain.cleanup.err, cancelCleanup.err, extractorWaitCleanup(waitErr, drain.cleanup.killed || cancelCleanup.killed))
		return 0, commandError(configuredPython, "drain Sage extractor diagnostics", cause, captured.String())
	}
	if waitErr != nil {
		cause := errors.Join(waitErr, recordCloseErr, drain.cleanup.err, cancelCleanup.err)
		return 0, commandError(configuredPython, "wait for Sage extraction", cause, captured.String())
	}
	if recordCloseErr != nil || drain.cleanup.err != nil || cancelCleanup.err != nil {
		cause := errors.Join(recordCloseErr, drain.cleanup.err, cancelCleanup.err)
		return 0, commandError(configuredPython, "clean up Sage extraction", cause, captured.String())
	}
	return count, nil
}

func finishExtractor(
	configuredPython, operation string,
	primary error,
	captured *diagnosticCapture,
	cmd *exec.Cmd,
	initial extractorCleanup,
	drainDone <-chan extractorDrain,
	cancelStop chan struct{},
	cancelDone <-chan extractorCleanup,
) (int, error) {
	drain := <-drainDone
	waitErr := cmd.Wait()
	close(cancelStop)
	cancelCleanup := <-cancelDone
	killed := initial.killed || drain.cleanup.killed || cancelCleanup.killed

	// If closing the record reader merely exposed an asynchronous diagnostic drain
	// failure, report that failure as the cause rather than the induced "file
	// already closed" decode error.
	if operation == "decode Sage extractor output" && errors.Is(primary, os.ErrClosed) && drain.diagnosticErr != nil {
		operation = "drain Sage extractor diagnostics"
		primary = drain.diagnosticErr
		drain.diagnosticErr = nil
	}

	cleanupErr := errors.Join(
		initial.err,
		drain.diagnosticErr,
		drain.cleanup.err,
		cancelCleanup.err,
		extractorWaitCleanup(waitErr, killed),
	)
	return 0, commandError(configuredPython, operation, errors.Join(primary, cleanupErr), captured.String())
}

func killExtractor(process *os.Process) (bool, error) {
	if process == nil {
		return false, nil
	}
	if err := process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return false, nil
		}
		return false, fmt.Errorf("kill Sage extractor: %w", err)
	}
	return true, nil
}

func closeExtractorFile(file *os.File, operation string) error {
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil {
		if errors.Is(err, os.ErrClosed) {
			return nil
		}
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func extractorWaitCleanup(err error, killed bool) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if killed && errors.As(err, &exitErr) {
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		if ok && status.Signaled() && status.Signal() == syscall.SIGKILL {
			return nil
		}
	}
	return fmt.Errorf("wait for Sage extractor cleanup: %w", err)
}

func validateRecord(record indexing.Record) error {
	if record.Name == "" {
		return errors.New("empty name")
	}
	if strings.TrimSpace(record.Docstring) == "" {
		return fmt.Errorf("%s: empty docstring", record.Name)
	}
	switch record.Kind {
	case "class", "function", "module", "object":
	default:
		return fmt.Errorf("%s: unknown kind %q", record.Name, record.Kind)
	}
	if record.Line < 0 {
		return fmt.Errorf("%s: negative source line %d", record.Name, record.Line)
	}
	return nil
}

type diagnosticCapture struct {
	mu          sync.Mutex
	destination io.Writer
	limit       int
	data        []byte
	truncated   bool
}

type diagnosticWriteError struct {
	err error
}

func (err *diagnosticWriteError) Error() string { return err.err.Error() }
func (err *diagnosticWriteError) Unwrap() error { return err.err }

func (capture *diagnosticCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	capture.appendTail(data)
	if capture.destination == nil {
		return len(data), nil
	}
	n, err := capture.destination.Write(data)
	if n != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = &diagnosticWriteError{err: err}
	}
	return n, err
}

func (capture *diagnosticCapture) appendTail(data []byte) {
	if capture.limit <= 0 {
		if len(data) != 0 {
			capture.truncated = true
		}
		capture.data = nil
		return
	}
	if len(data) >= capture.limit {
		dropped := len(capture.data)+len(data) > capture.limit
		capture.data = append(capture.data[:0], data[len(data)-capture.limit:]...)
		capture.truncated = capture.truncated || dropped
		return
	}
	overflow := len(capture.data) + len(data) - capture.limit
	if overflow > 0 {
		copy(capture.data, capture.data[overflow:])
		capture.data = capture.data[:len(capture.data)-overflow]
		capture.truncated = true
	}
	capture.data = append(capture.data, data...)
}

func (capture *diagnosticCapture) String() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	text := string(capture.data)
	if capture.truncated {
		text = "[diagnostics truncated]\n" + text
	}
	return text
}

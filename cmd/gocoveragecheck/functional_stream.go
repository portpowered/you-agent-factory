package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// functionalStreamReporter serializes the two child output streams so a
// decoded test event and its human-readable package result reach the same
// diagnostic sink as complete writes. Package completion is tracked across
// invocation batches because a functional selection can be split into more
// than one go test process on Windows.
type functionalStreamReporter struct {
	sink              io.Writer
	mu                sync.Mutex
	sinkMu            sync.Mutex
	completedPackages map[string]struct{}
	onEvent           func(goTestTimingEvent)
}

type functionalStreamWriter struct {
	reporter *functionalStreamReporter
	pending  []byte
}

type lockedFunctionalStreamWriter struct {
	reporter *functionalStreamReporter
	sink     io.Writer
}

func newFunctionalStreamReporter(sink io.Writer) *functionalStreamReporter {
	return &functionalStreamReporter{
		sink:              sink,
		completedPackages: make(map[string]struct{}),
	}
}

func newFunctionalStreamReporterWithObserver(sink io.Writer, observer func(goTestTimingEvent)) *functionalStreamReporter {
	reporter := newFunctionalStreamReporter(sink)
	reporter.onEvent = observer
	return reporter
}

func (reporter *functionalStreamReporter) stdoutWriter() *functionalStreamWriter {
	return &functionalStreamWriter{reporter: reporter}
}

func (reporter *functionalStreamReporter) stderrWriter(sink io.Writer) io.Writer {
	return &lockedFunctionalStreamWriter{reporter: reporter, sink: sink}
}

func (writer *functionalStreamWriter) Write(data []byte) (int, error) {
	writer.reporter.mu.Lock()
	defer writer.reporter.mu.Unlock()

	writer.pending = append(writer.pending, data...)
	for {
		lineEnd := bytes.IndexByte(writer.pending, '\n')
		if lineEnd < 0 {
			break
		}
		line := append([]byte(nil), writer.pending[:lineEnd+1]...)
		writer.pending = writer.pending[lineEnd+1:]
		if err := writer.writeLineLocked(line); err != nil {
			return len(data), err
		}
	}
	return len(data), nil
}

// Flush preserves a final child-output fragment when a subprocess exits
// without a trailing newline. Normal go test JSON events are newline-delimited,
// but timeout and signal paths should not lose the last diagnostic fragment.
func (writer *functionalStreamWriter) Flush() error {
	writer.reporter.mu.Lock()
	defer writer.reporter.mu.Unlock()

	if len(writer.pending) == 0 {
		return nil
	}
	line := append([]byte(nil), writer.pending...)
	writer.pending = nil
	return writer.writeLineLocked(line)
}

func (writer *functionalStreamWriter) writeLineLocked(line []byte) error {
	var event goTestTimingEvent
	if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil || event.Package == "" {
		return writer.reporter.writeSink(line)
	}
	if writer.reporter.onEvent != nil {
		writer.reporter.onEvent(event)
	}

	switch event.Action {
	case "output":
		if isRedundantCoverageOutputLine(event.Output) {
			// The per-package coverage percentage is reported authoritatively
			// by the end-of-run verdict block and the coverage-summary JSON.
			// Streaming it once per package buries the actionable verdict.
			return nil
		}
		// Keep the child failure text readable in the job log while the
		// command runner retains the original JSON bytes for timing parsing.
		return writer.reporter.writeSink([]byte(event.Output))
	case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		if event.Test != "" {
			return nil
		}
		if _, alreadyReported := writer.reporter.completedPackages[event.Package]; alreadyReported {
			return nil
		}
		writer.reporter.completedPackages[event.Package] = struct{}{}
		var err error
		writer.reporter.withSink(func(sink io.Writer) {
			_, err = fmt.Fprintf(
				sink,
				"Functional package result: package=%s outcome=%s elapsed=%.3fs\n",
				event.Package,
				event.Action,
				event.Elapsed,
			)
		})
		return err
	default:
		// Unknown JSON lines remain visible rather than being silently
		// discarded if go test adds an event action this reporter does not
		// understand yet.
		return writer.reporter.writeSink(line)
	}
}

// isRedundantCoverageOutputLine reports whether a go test output event is the
// standalone per-package coverage percentage line. Only a line that is
// entirely that report is suppressed: package result lines ("ok  pkg 1.2s
// coverage: ..."), test failures, and panics keep their exact text so a hang
// or crash stays diagnosable from the raw stream.
func isRedundantCoverageOutputLine(output string) bool {
	trimmed := strings.TrimSpace(output)
	return strings.HasPrefix(trimmed, "coverage: ") && strings.Contains(trimmed, "% of statements")
}

func (writer *lockedFunctionalStreamWriter) Write(data []byte) (int, error) {
	writer.reporter.mu.Lock()
	defer writer.reporter.mu.Unlock()
	return len(data), writer.reporter.writeSinkTo(writer.sink, data)
}

func (reporter *functionalStreamReporter) writeSink(data []byte) error {
	return reporter.writeSinkTo(reporter.sink, data)
}

func (reporter *functionalStreamReporter) writeSinkTo(sink io.Writer, data []byte) error {
	reporter.sinkMu.Lock()
	defer reporter.sinkMu.Unlock()
	return writeFunctionalStreamBytes(sink, data)
}

func (reporter *functionalStreamReporter) withSink(fn func(io.Writer)) {
	reporter.sinkMu.Lock()
	defer reporter.sinkMu.Unlock()
	fn(reporter.sink)
}

func writeFunctionalStreamBytes(sink io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	written, err := sink.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func flushFunctionalStreamWriter(writer io.Writer) error {
	flusher, ok := writer.(*functionalStreamWriter)
	if !ok {
		return nil
	}
	return flusher.Flush()
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	completedPackages map[string]struct{}
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
		return writeFunctionalStreamBytes(writer.reporter.sink, line)
	}

	switch event.Action {
	case "output":
		// Keep the child failure text readable in the job log while the
		// command runner retains the original JSON bytes for timing parsing.
		return writeFunctionalStreamBytes(writer.reporter.sink, []byte(event.Output))
	case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		if event.Test != "" {
			return nil
		}
		if _, alreadyReported := writer.reporter.completedPackages[event.Package]; alreadyReported {
			return nil
		}
		writer.reporter.completedPackages[event.Package] = struct{}{}
		_, err := fmt.Fprintf(
			writer.reporter.sink,
			"Functional package result: package=%s outcome=%s elapsed=%.3fs\n",
			event.Package,
			event.Action,
			event.Elapsed,
		)
		return err
	default:
		// Unknown JSON lines remain visible rather than being silently
		// discarded if go test adds an event action this reporter does not
		// understand yet.
		return writeFunctionalStreamBytes(writer.reporter.sink, line)
	}
}

func (writer *lockedFunctionalStreamWriter) Write(data []byte) (int, error) {
	writer.reporter.mu.Lock()
	defer writer.reporter.mu.Unlock()
	return len(data), writeFunctionalStreamBytes(writer.sink, data)
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

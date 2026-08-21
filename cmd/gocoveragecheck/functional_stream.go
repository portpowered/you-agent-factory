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
	reporter                   *functionalStreamReporter
	pending                    []byte
	coverageOutputMode         functionalCoverageOutputMode
	coverageContinuationBuffer []byte
}

type functionalCoverageOutputMode uint8

const (
	functionalCoverageOutputNormal functionalCoverageOutputMode = iota
	functionalCoverageOutputSuppress
	functionalCoverageOutputCompact
	functionalCoverageOutputSuppressPending
	functionalCoverageOutputCompactPending
)

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

	if len(writer.pending) > 0 {
		line := append([]byte(nil), writer.pending...)
		writer.pending = nil
		if err := writer.writeLineLocked(line); err != nil {
			return err
		}
	}
	return writer.flushCoverageContinuationLocked()
}

func (writer *functionalStreamWriter) writeLineLocked(line []byte) error {
	var event goTestTimingEvent
	if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil || event.Package == "" {
		return writer.reporter.writeSink(line)
	}
	if writer.reporter.onEvent != nil {
		writer.reporter.onEvent(event)
	}
	if event.Action != "output" {
		if err := writer.flushCoverageContinuationLocked(); err != nil {
			return err
		}
	}

	switch event.Action {
	case "output":
		output := writer.filterCoverageOutput(event.Output)
		if output == "" {
			// The per-package coverage percentage is reported authoritatively
			// by the end-of-run verdict block and the coverage-summary JSON.
			// Streaming it once per package, including wrapped fragments, buries
			// the actionable verdict.
			return nil
		}
		compactContinuation := writer.coverageOutputMode == functionalCoverageOutputCompact
		output = compactSuccessfulCoverageOutputLine(output)
		if compactContinuation && !strings.Contains(output, "\n") {
			output += "\n"
		}
		// Keep the child failure text readable in the job log while the
		// command runner retains the original JSON bytes for timing parsing.
		return writer.reporter.writeSink([]byte(output))
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

// filterCoverageOutput suppresses the standalone per-package coverage line,
// including a long line split across multiple test2json output events. It also
// records a split successful package result so its coverpkg continuation can
// be dropped after the compact prefix is rendered. Candidate continuation
// bytes are buffered until their line is complete, because a diagnostic can
// begin with the same module path as a coverpkg entry. The complete line is
// then parsed rather than classified by a broad substring match.
func (writer *functionalStreamWriter) filterCoverageOutput(output string) string {
	var filtered strings.Builder
	for {
		if writer.coverageOutputMode != functionalCoverageOutputNormal {
			newline := strings.IndexByte(output, '\n')
			if newline < 0 {
				writer.coverageContinuationBuffer = append(writer.coverageContinuationBuffer, output...)
				return filtered.String()
			}

			candidate := append(writer.coverageContinuationBuffer, output[:newline+1]...)
			writer.coverageContinuationBuffer = nil
			writer.coverageOutputMode = functionalCoverageOutputNormal
			if isCoveragePackageListLine(string(candidate)) {
				output = output[newline+1:]
				if output == "" {
					return filtered.String()
				}
				continue
			}
			output = string(candidate) + output[newline+1:]
			continue
		}

		newline := strings.IndexByte(output, '\n')
		if newline < 0 {
			if isRedundantCoverageOutputLine(output) {
				writer.coverageOutputMode = functionalCoverageOutputSuppress
				writer.coverageContinuationBuffer = append(writer.coverageContinuationBuffer, coveragePackageListFragment(output)...)
				return filtered.String()
			}
			if isSuccessfulCoverageOutputFragment(output) {
				writer.coverageOutputMode = functionalCoverageOutputCompact
				writer.coverageContinuationBuffer = append(writer.coverageContinuationBuffer, coveragePackageListFragment(output)...)
			}
			filtered.WriteString(output)
			return filtered.String()
		}

		lineEnd := newline + 1
		line := output[:lineEnd]
		if isRedundantCoverageOutputLine(line) {
			if hasCoveragePackageList(line) {
				writer.coverageOutputMode = functionalCoverageOutputNormal
			} else {
				writer.coverageOutputMode = functionalCoverageOutputSuppressPending
			}
		} else {
			filtered.WriteString(line)
			if isSuccessfulCoverageOutputLine(line) && !strings.Contains(line, "% of statements in ") {
				writer.coverageOutputMode = functionalCoverageOutputCompactPending
			}
		}
		output = output[lineEnd:]
		if output == "" {
			return filtered.String()
		}
	}
}

func isSuccessfulCoverageOutputLine(output string) bool {
	line, _, ok := singleFunctionalOutputLine(output)
	if !ok || (!strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "ok\t")) {
		return false
	}
	return strings.Contains(line, "coverage: ") && strings.Contains(line, "% of statements")
}

func isSuccessfulCoverageOutputFragment(output string) bool {
	line, _, ok := singleFunctionalOutputLine(output)
	if !ok || (!strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "ok\t")) {
		return false
	}
	return strings.Contains(line, "coverage: ") && strings.Contains(line, "% of statements in ")
}

func hasCoveragePackageList(output string) bool {
	return strings.Contains(output, "% of statements in ")
}

func coveragePackageListFragment(output string) string {
	const coveragePackageListSuffix = "% of statements in "
	start := strings.Index(output, coveragePackageListSuffix)
	if start < 0 {
		return ""
	}
	return output[start+len(coveragePackageListSuffix):]
}

func isCoveragePackageListLine(output string) bool {
	line, _, ok := singleFunctionalOutputLine(output)
	if !ok || line == "" {
		return false
	}
	for _, packagePath := range strings.Split(line, ", ") {
		if !isCoveragePackagePath(packagePath) {
			return false
		}
	}
	return true
}

func isCoveragePackagePath(packagePath string) bool {
	if packagePath == modulePath {
		return true
	}
	if !strings.HasPrefix(packagePath, modulePath+"/") {
		return false
	}
	suffix := strings.TrimPrefix(packagePath, modulePath+"/")
	if suffix == "" || strings.HasPrefix(suffix, "/") || strings.HasSuffix(suffix, "/") || strings.Contains(suffix, "//") {
		return false
	}
	for index := 0; index < len(suffix); index++ {
		character := suffix[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' || character == '~' || character == '/' {
			continue
		}
		return false
	}
	return true
}

// isRedundantCoverageOutputLine reports whether a go test output event is the
// standalone per-package coverage percentage line. Only a line that is
// entirely that report is suppressed: package result lines ("ok  pkg 1.2s
// coverage: ..."), test failures, and panics keep their exact text so a hang
// or crash stays diagnosable from the raw stream.
func isRedundantCoverageOutputLine(output string) bool {
	line, _, ok := singleFunctionalOutputLine(output)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "coverage: ") && strings.Contains(trimmed, "% of statements")
}

// compactSuccessfulCoverageOutputLine removes the repeated -coverpkg import
// list from one successful go test package-result line. It deliberately
// accepts only a single line beginning with "ok": a failing package, panic,
// or test-produced diagnostic keeps its original bytes even when its text
// mentions coverage.
func compactSuccessfulCoverageOutputLine(output string) string {
	line, lineEnding, ok := singleFunctionalOutputLine(output)
	if !ok || (!strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "ok\t")) {
		return output
	}

	const coveragePrefix = "coverage: "
	const coveragePackageListSuffix = "% of statements in "
	coverageStart := strings.Index(line, coveragePrefix)
	if coverageStart < 0 {
		return output
	}
	suffixStart := strings.Index(line[coverageStart+len(coveragePrefix):], coveragePackageListSuffix)
	if suffixStart < 0 {
		return output
	}
	suffixStart += coverageStart + len(coveragePrefix)
	return line[:suffixStart+len("% of statements")] + lineEnding
}

// singleFunctionalOutputLine separates a complete output line from its line
// ending. Multi-line events are left alone because their later bytes may be a
// failure or diagnostic that the stream must preserve exactly.
func singleFunctionalOutputLine(output string) (line, lineEnding string, ok bool) {
	newline := strings.IndexByte(output, '\n')
	if newline < 0 {
		return output, "", true
	}
	if newline != len(output)-1 {
		return "", "", false
	}
	line = output[:newline]
	lineEnding = output[newline:]
	if strings.HasSuffix(line, "\r") {
		line = strings.TrimSuffix(line, "\r")
		lineEnding = "\r" + lineEnding
	}
	return line, lineEnding, true
}

func (writer *functionalStreamWriter) flushCoverageContinuationLocked() error {
	if len(writer.coverageContinuationBuffer) == 0 {
		writer.coverageOutputMode = functionalCoverageOutputNormal
		return nil
	}
	candidate := writer.coverageContinuationBuffer
	writer.coverageContinuationBuffer = nil
	writer.coverageOutputMode = functionalCoverageOutputNormal
	if isCoveragePackageListLine(string(candidate)) {
		return nil
	}
	return writer.reporter.writeSink(candidate)
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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// The native Go process still owns execution and its exit status. This writer
// removes only coordinator wrappers, keeping every original package/test event
// for inventory checks, failure capture, streaming and coverage diagnostics.
type functionalMonolithEventWriter struct {
	sink                    io.Writer
	groups                  map[string]string
	pending                 []byte
	lines                   int
	started                 map[string]time.Time
	observedCustomerFailure bool
}

func (writer *functionalMonolithEventWriter) Write(data []byte) (int, error) {
	if writer.started == nil {
		writer.started = make(map[string]time.Time)
	}
	writer.pending = append(writer.pending, data...)
	for {
		index := bytes.IndexByte(writer.pending, '\n')
		if index < 0 {
			return len(data), nil
		}
		line, err := writer.normalizeEvent(writer.pending[:index])
		if err != nil {
			return 0, err
		}
		writer.lines++
		if len(line) > 0 {
			if _, err := writer.sink.Write(append(line, '\n')); err != nil {
				return 0, fmt.Errorf("functional coordinator event %s: %w", line, err)
			}
		}
		writer.pending = writer.pending[index+1:]
	}
}

// An ordinary child failure also fails both Go coordinator wrappers. Keep the
// original failure once, so diagnostics and focused retries select its package.
// Unattributed coordinator failures remain visible; the native exit status, raw
// panic diagnostics, and original-package completion checks are always retained.
func (writer *functionalMonolithEventWriter) normalizeEvent(raw []byte) ([]byte, error) {
	line, err := normalizeFunctionalMonolithEvent(raw, writer.groups, writer.started)
	if err != nil || len(line) == 0 {
		return line, err
	}
	var event goTestTimingEvent
	if json.Unmarshal(line, &event) == nil && event.Action == timingOutcomeFail {
		if event.Package == functionalMonolithPackage && writer.observedCustomerFailure {
			return nil, nil
		}
		if event.Test != "" {
			for _, original := range writer.groups {
				if event.Package == original {
					writer.observedCustomerFailure = true
				}
			}
		}
	}
	return line, nil
}

func (writer *functionalMonolithEventWriter) flush() error {
	if len(writer.pending) == 0 {
		return nil
	}
	line, err := writer.normalizeEvent(writer.pending)
	if err != nil {
		return err
	}
	writer.pending = nil
	if len(line) == 0 {
		return nil
	}
	_, err = writer.sink.Write(append(line, '\n'))
	return err
}

func runFunctionalMonolithCommand(invocation commandInvocation) (string, string, error) {
	var normalized bytes.Buffer
	sink := io.Writer(&normalized)
	if invocation.stdoutWriter != nil {
		sink = io.MultiWriter(sink, invocation.stdoutWriter)
	}
	writer := &functionalMonolithEventWriter{sink: sink, groups: invocation.monolithGroups}
	invocation.stdoutWriter = writer
	raw, stderr, err := commandRunner(invocation)
	// Command-runner doubles may return captured data without streaming it.
	if writer.lines == 0 && len(writer.pending) == 0 && raw != "" {
		_, writeErr := writer.Write([]byte(raw))
		err = errors.Join(err, writeErr)
	}
	err = errors.Join(err, writer.flush())
	if err != nil {
		// Retain coordinator panic/build diagnostics without attributing its
		// successful bookkeeping to an extra customer package.
		stderr += "\n" + raw
	}
	return normalized.String(), stderr, err
}

func normalizeFunctionalMonolithEvent(line []byte, groups map[string]string, started map[string]time.Time) ([]byte, error) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(line, &event); err != nil {
		return line, nil
	}
	var pkg, test, action, output string
	_ = json.Unmarshal(event["Package"], &pkg)
	if pkg != functionalMonolithPackage {
		return line, nil
	}
	_ = json.Unmarshal(event["Test"], &test)
	_ = json.Unmarshal(event["Action"], &action)
	wrapped, ok := strings.CutPrefix(test, "TestFunctionalPackages/")
	if !ok {
		// A coordinator panic/build failure must remain visible and fail the
		// native command; successful coordinator bookkeeping is redundant.
		if action == "fail" {
			return line, nil
		}
		if action == "output" && json.Unmarshal(event["Output"], &output) == nil && firstBinaryDeathLine(output) != "" {
			return line, nil
		}
		return nil, nil
	}
	group, original, nested := strings.Cut(wrapped, "/")
	originalPackage, ok := groups[group]
	if !ok {
		return nil, fmt.Errorf("unknown functional coordinator group %q", group)
	}
	event["Package"], _ = json.Marshal(originalPackage)
	if !nested {
		delete(event, "Test")
		annotateFunctionalPackageWallTime(event, action, group, started)
		switch action {
		case "run":
			event["Action"] = json.RawMessage(`"start"`)
		case "pause", "cont":
			return nil, nil
		}
	} else {
		event["Test"], _ = json.Marshal(original)
	}
	if json.Unmarshal(event["Output"], &output) == nil {
		event["Output"], _ = json.Marshal(strings.ReplaceAll(output, "TestFunctionalPackages/"+group+"/", ""))
	}
	return json.Marshal(event)
}

func annotateFunctionalPackageWallTime(event map[string]json.RawMessage, action, group string, started map[string]time.Time) {
	// Go excludes parallel children from the parent's Elapsed. Package
	// wall time must include those children and their scheduling waits.
	var timestamp string
	_ = json.Unmarshal(event["Time"], &timestamp)
	at, timeErr := time.Parse(time.RFC3339Nano, timestamp)
	if timeErr == nil && started != nil {
		if action == "run" {
			started[group] = at
		}
		if action == "pass" || action == "fail" || action == "skip" {
			if began, found := started[group]; found && !at.Before(began) {
				event["Elapsed"], _ = json.Marshal(at.Sub(began).Seconds())
			}
			delete(started, group)
		}
	}
}

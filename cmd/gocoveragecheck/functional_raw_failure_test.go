package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const rawFailureTestPackage = "github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailure"

func TestFunctionalRawFailureCapturePreservesOnlyFailedPackageEvents(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 1<<20)
	invocation := commandInvocation{
		name: "go",
		args: []string{"test", "-json", "-p=12", "-run=^TestRawFailureWitness$", "-coverprofile=/tmp/coverage.out", rawFailureTestPackage},
	}
	capture.beginInvocation(invocation)
	events := [][]byte{
		rawFailureTestEvent("run", rawFailureTestPackage, "TestRawFailureWitness", ""),
		rawFailureTestEvent("output", rawFailureTestPackage, "TestRawFailureWitness", "    raw_failure_test.go:15: assertion marker\n"),
		rawFailureTestEvent("output", rawFailureTestPackage, "TestRawFailureWitness", "Factory Event: work.accepted sequence=1\n"),
		rawFailureTestEvent("output", rawFailureTestPackage, "TestRawFailureWitness", "Factory Event: work.completed sequence=2\n"),
		rawFailureTestEvent(timingOutcomeFail, rawFailureTestPackage, "TestRawFailureWitness", ""),
		rawFailureTestEvent(timingOutcomeFail, rawFailureTestPackage, "", ""),
	}
	for _, event := range events {
		if err := capture.observeLine(event); err != nil {
			t.Fatalf("capture event: %v", err)
		}
	}
	capture.finishInvocation(errors.New("exit status 1"))
	if err := capture.publish(strings.Repeat("a", 40), true); err != nil {
		t.Fatalf("publish raw failure index: %v", err)
	}

	index := readRawFailureTestIndex(t, directory)
	if index.Head != strings.Repeat("a", 40) || index.CaptureStatus != "complete" || len(index.Failures) != 1 {
		t.Fatalf("raw index identity/status/failures = %q/%q/%d, want exact head/complete/1", index.Head, index.CaptureStatus, len(index.Failures))
	}
	failure := index.Failures[0]
	if failure.Package != rawFailureTestPackage || len(failure.Selectors) != 1 || failure.Selectors[0] != "TestRawFailureWitness" {
		t.Fatalf("failure attribution = package %q selectors %v, want exact package and selector", failure.Package, failure.Selectors)
	}
	if failure.ExitStatus != 1 || !strings.Contains(failure.Reproduce, "TestRawFailureWitness") || failure.Command[len(failure.Command)-1] != rawFailureTestPackage {
		t.Fatalf("failure command/status = %v/%d reproduce %q", failure.Command, failure.ExitStatus, failure.Reproduce)
	}
	if failure.CapturedBytes != failure.ObservedBytes || failure.CapHit || failure.CaptureStatus != "complete" {
		t.Fatalf("below-cap metadata = %+v, want complete capture without omission", failure)
	}
	raw, err := os.ReadFile(filepath.Join(directory, failure.File))
	if err != nil {
		t.Fatalf("read raw failure package file: %v", err)
	}
	if string(raw) != string(joinRawFailureEvents(events)) {
		t.Fatalf("raw package events differ\n got: %s\nwant: %s", raw, joinRawFailureEvents(events))
	}
	if len(index.ExecutedCommands) != 1 || len(index.ExecutedCommands[0]) == 0 || index.ExecutedCommands[0][0] != "go" {
		t.Fatalf("executed commands = %v, want exact go argv", index.ExecutedCommands)
	}
}

func TestFunctionalRawFailureCaptureDiscardsVerbosePassingPackage(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 128)
	invocation := commandInvocation{name: "go", args: []string{"test", "-json", rawFailureTestPackage}}
	capture.beginInvocation(invocation)
	for i := 0; i < 20; i++ {
		line := rawFailureTestEvent("output", rawFailureTestPackage, "TestVerbosePass", strings.Repeat("verbose-success-output-", 3)+"\n")
		if err := capture.observeLine(line); err != nil {
			t.Fatalf("capture verbose passing output: %v", err)
		}
	}
	if err := capture.observeLine(rawFailureTestEvent(timingOutcomePass, rawFailureTestPackage, "", "")); err != nil {
		t.Fatalf("capture passing terminal event: %v", err)
	}
	capture.finishInvocation(nil)
	if err := capture.publish(strings.Repeat("b", 40), false); err != nil {
		t.Fatalf("publish green raw index: %v", err)
	}
	index := readRawFailureTestIndex(t, directory)
	if index.CaptureStatus != "complete" || len(index.Failures) != 0 || index.SuccessfulPackagesDiscarded != 1 || index.SuccessfulObservedBytes == 0 {
		t.Fatalf("green raw index = status %q failures %d successful packages/bytes %d/%d", index.CaptureStatus, len(index.Failures), index.SuccessfulPackagesDiscarded, index.SuccessfulObservedBytes)
	}
	files, err := filepath.Glob(filepath.Join(directory, "functional-failure-*.jsonl"))
	if err != nil {
		t.Fatalf("list retained raw files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("green run retained raw package files: %v", files)
	}
}

func TestFunctionalRawFailureCaptureCleansOnlyOwnedStaleArtifacts(t *testing.T) {
	directory := t.TempDir()
	staleArtifact := filepath.Join(directory, "functional-failure-old.jsonl")
	ownedIndex := filepath.Join(directory, functionalRawFailureIndexName)
	userFile := filepath.Join(directory, "review-notes.jsonl")
	for _, file := range []string{staleArtifact, ownedIndex, userFile} {
		if err := os.WriteFile(file, []byte("stale"), 0o644); err != nil {
			t.Fatalf("write stale fixture %s: %v", file, err)
		}
	}
	newRawFailureTestCapture(t, directory, 1024)
	if _, err := os.Stat(staleArtifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale raw artifact stat error = %v, want removed", err)
	}
	if _, err := os.Stat(ownedIndex); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale index stat error = %v, want removed", err)
	}
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "stale" {
		t.Fatalf("unowned JSONL file = %q error %v, want preserved", data, err)
	}
}

func TestFunctionalRawFailureCaptureReportsCapOmission(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 256)
	capture.packageMaxBytes = 256
	invocation := commandInvocation{name: "go", args: []string{"test", "-json", rawFailureTestPackage}}
	capture.beginInvocation(invocation)
	first := rawFailureTestEvent("output", rawFailureTestPackage, "TestLargeFailure", "first diagnostic\n")
	second := rawFailureTestEvent("output", rawFailureTestPackage, "TestLargeFailure", strings.Repeat("x", 400)+"\n")
	failedTest := rawFailureTestEvent(timingOutcomeFail, rawFailureTestPackage, "TestLargeFailure", "")
	failedPackage := rawFailureTestEvent(timingOutcomeFail, rawFailureTestPackage, "", "")
	for _, line := range [][]byte{first, second, failedTest, failedPackage} {
		if err := capture.observeLine(line); err != nil {
			t.Fatalf("capture capped failure output: %v", err)
		}
	}
	capture.finishInvocation(errors.New("exit status 1"))
	if err := capture.publish(strings.Repeat("c", 40), true); err != nil {
		t.Fatalf("publish capped raw index: %v", err)
	}
	index := readRawFailureTestIndex(t, directory)
	if index.CaptureStatus != "capped" || len(index.Failures) != 1 {
		t.Fatalf("capped index = status %q failures %d, want capped and one failure", index.CaptureStatus, len(index.Failures))
	}
	failure := index.Failures[0]
	if !failure.CapHit || failure.OmittedEventRange == nil || failure.OmittedBytesRange == nil || failure.CapturedBytes >= failure.ObservedBytes {
		t.Fatalf("cap metadata = %+v, want omitted byte and event ranges", failure)
	}
	if *failure.OmittedEventRange != [2]int64{2, 4} || *failure.OmittedBytesRange != [2]int64{failure.CapturedBytes + 1, failure.ObservedBytes} || !strings.Contains(failure.Reproduce, "TestLargeFailure") {
		t.Fatalf("omitted event/byte ranges and selector = %v/%v %q", failure.OmittedEventRange, failure.OmittedBytesRange, failure.Reproduce)
	}
}

func TestFunctionalRawFailureCaptureMarksEarlyCrashIncomplete(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 1<<20)
	invocation := commandInvocation{name: "go", args: []string{"test", "-json", rawFailureTestPackage}}
	capture.beginInvocation(invocation)
	if err := capture.observeLine(rawFailureTestEvent("output", rawFailureTestPackage, "TestEarlyCrash", "partial before crash\n")); err != nil {
		t.Fatalf("capture pre-crash output: %v", err)
	}
	capture.finishInvocation(errors.New("exit status 2"))
	if err := capture.publish(strings.Repeat("d", 40), true); err == nil {
		t.Fatal("publish incomplete crash capture returned nil, want fail-closed error")
	}
	index := readRawFailureTestIndex(t, directory)
	if index.CaptureStatus != "incomplete" || len(index.Failures) != 1 || index.Failures[0].CaptureStatus != "incomplete" || index.Failures[0].ExitStatus != 2 {
		t.Fatalf("early crash metadata = %+v, want incomplete entry and nonzero status", index)
	}
}

func TestFunctionalRawFailureCaptureWriteFailureIsIncomplete(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 1<<20)
	capture.write = func(io.Writer, []byte) error { return errors.New("controlled spool write failure") }
	invocation := commandInvocation{name: "go", args: []string{"test", "-json", rawFailureTestPackage}}
	capture.beginInvocation(invocation)
	if err := capture.observeLine(rawFailureTestEvent("output", rawFailureTestPackage, "TestWriteFailure", "output before write failure\n")); err == nil {
		t.Fatal("capture write returned nil, want injected write failure")
	}
	capture.finishInvocation(errors.New("exit status 1"))
	if err := capture.publish(strings.Repeat("e", 40), true); err == nil {
		t.Fatal("publish failed capture returned nil, want fail-closed error")
	}
	index := readRawFailureTestIndex(t, directory)
	if index.CaptureStatus != "incomplete" || !strings.Contains(index.CaptureError, "controlled spool write failure") {
		t.Fatalf("write-failure index = %+v, want incomplete capture error", index)
	}
}

func TestFunctionalRawFailureCaptureRejectsUnattributedOutput(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 1<<20)
	capture.beginInvocation(commandInvocation{name: "go", args: []string{"test", "-json", rawFailureTestPackage}})
	if err := capture.observeLine([]byte("compiler output without a test2json package event\n")); err == nil {
		t.Fatal("capture unattributed output returned nil, want incomplete capture error")
	}
	capture.finishInvocation(nil)
	if err := capture.publish(strings.Repeat("a", 40), false); err == nil {
		t.Fatal("publish unattributed output returned nil, want fail-closed error")
	}
	index := readRawFailureTestIndex(t, directory)
	if index.CaptureStatus != "incomplete" || len(index.Failures) != 1 || index.Failures[0].CaptureStatus != "incomplete" {
		t.Fatalf("unattributed output index = %+v, want incomplete capture", index)
	}
}

func TestFunctionalRawFailureCaptureSerializesConcurrentPackageWrites(t *testing.T) {
	directory := t.TempDir()
	capture := newRawFailureTestCapture(t, directory, 1<<20)
	packages := []string{rawFailureTestPackage + "/alpha", rawFailureTestPackage + "/beta"}
	capture.beginInvocation(commandInvocation{name: "go", args: append([]string{"test", "-json"}, packages...)})
	var writers sync.WaitGroup
	for _, packageName := range packages {
		packageName := packageName
		writers.Add(1)
		go func() {
			defer writers.Done()
			for index := 0; index < 20; index++ {
				line := rawFailureTestEvent("output", packageName, "TestConcurrentFailure", fmt.Sprintf("event-%02d\n", index))
				if err := capture.observeLine(line); err != nil {
					t.Errorf("capture concurrent event: %v", err)
					return
				}
			}
			if err := capture.observeLine(rawFailureTestEvent(timingOutcomeFail, packageName, "TestConcurrentFailure", "")); err != nil {
				t.Errorf("capture concurrent failed test: %v", err)
				return
			}
			if err := capture.observeLine(rawFailureTestEvent(timingOutcomeFail, packageName, "", "")); err != nil {
				t.Errorf("capture concurrent failed package: %v", err)
			}
		}()
	}
	writers.Wait()
	capture.finishInvocation(errors.New("exit status 1"))
	if err := capture.publish(strings.Repeat("f", 40), true); err != nil {
		t.Fatalf("publish concurrent raw index: %v", err)
	}
	index := readRawFailureTestIndex(t, directory)
	if len(index.Failures) != 2 {
		t.Fatalf("concurrent failure count = %d, want 2", len(index.Failures))
	}
	for _, failure := range index.Failures {
		data, err := os.ReadFile(filepath.Join(directory, failure.File))
		if err != nil {
			t.Fatalf("read concurrent package %s: %v", failure.Package, err)
		}
		for event := 0; event < 20; event++ {
			if !strings.Contains(string(data), fmt.Sprintf("event-%02d", event)) {
				t.Errorf("package %s missing event-%02d", failure.Package, event)
			}
		}
	}
}

func newRawFailureTestCapture(t *testing.T, directory string, maxBytes int64) *functionalRawFailureCapture {
	t.Helper()
	capture, err := newFunctionalRawFailureCapture(directory, maxBytes)
	if err != nil {
		t.Fatalf("create raw failure capture: %v", err)
	}
	return capture
}

func rawFailureTestEvent(action, packageName, testName, output string) []byte {
	data, err := json.Marshal(goTestTimingEvent{Action: action, Package: packageName, Test: testName, Output: output})
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func joinRawFailureEvents(events [][]byte) []byte {
	joined := make([]byte, 0)
	for _, event := range events {
		joined = append(joined, event...)
	}
	return joined
}

func readRawFailureTestIndex(t *testing.T, directory string) functionalRawFailureIndex {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, functionalRawFailureIndexName))
	if err != nil {
		t.Fatalf("read raw failure index: %v", err)
	}
	var index functionalRawFailureIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("decode raw failure index: %v", err)
	}
	return index
}

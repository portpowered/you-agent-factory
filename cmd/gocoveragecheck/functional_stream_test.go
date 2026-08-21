package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	functionalStreamChildEnv    = "GOCOVERAGECHECK_FUNCTIONAL_STREAM_CHILD"
	functionalStreamChildMode   = "GOCOVERAGECHECK_FUNCTIONAL_STREAM_MODE"
	functionalStreamChildPkg    = modulePath + "/tests/functional/streaming"
	functionalStreamPassMode    = "pass"
	functionalStreamTimeoutMode = "timeout"
)

func TestFunctionalStreamReportsPackageBeforeChildCompletes(t *testing.T) {
	liveOutput, resultCh, releaseWriter, rawOutput := startFunctionalStreamChild(t, functionalStreamPassMode)
	waitForFunctionalStreamMarker(t, liveOutput, "Functional package result: package="+functionalStreamChildPkg)

	got := liveOutput.String()
	if !strings.Contains(got, "outcome=pass") || !strings.Contains(got, "elapsed=0.125s") {
		t.Fatalf("live functional output = %q, want package outcome and elapsed duration", got)
	}
	if strings.Contains(got, " of statements in ") {
		t.Fatalf("live functional output retained the repeated coverpkg suffix: %q", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if len(line) > 1024 {
			t.Fatalf("live functional output line length = %d, want <= 1024: %q", len(line), line)
		}
	}
	select {
	case result := <-resultCh:
		t.Fatalf("functional child completed before release: %+v", result)
	default:
	}

	releaseFunctionalStreamChild(t, releaseWriter)
	result := waitForStreamChild(t, resultCh)
	if result.err != nil {
		t.Fatalf("functional child error = %v, want nil", result.err)
	}
	if result.stdout != rawOutput {
		t.Fatalf("captured functional JSON = %q, want unchanged %q", result.stdout, rawOutput)
	}
}

func TestFunctionalStreamCompactsSuccessfulPackageCoverageList(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	input := "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements in a, b, c\n"
	event, err := json.Marshal(goTestTimingEvent{
		Action:  "output",
		Package: packageName,
		Output:  input,
	})
	if err != nil {
		t.Fatalf("marshal stream event: %v", err)
	}
	if _, err := writer.Write(append(event, '\n')); err != nil {
		t.Fatalf("stream reporter write error = %v", err)
	}

	want := "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n"
	if got := sink.String(); got != want {
		t.Fatalf("compacted successful coverage output = %q, want %q", got, want)
	}
}

func TestFunctionalStreamCompactsWrappedSuccessfulCoverageOutput(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	coverPackages := make([]string, 530)
	for index := range coverPackages {
		coverPackages[index] = fmt.Sprintf("%s/pkg/cover%03d", modulePath, index)
	}
	coverList := strings.Join(coverPackages, ", ")
	outputs := []string{
		"ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements in " + coverList[:len(coverList)/2],
		coverList[len(coverList)/2:] + "\n",
	}
	for _, output := range outputs {
		event, err := json.Marshal(goTestTimingEvent{
			Action:  "output",
			Package: packageName,
			Output:  output,
		})
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := writer.Write(append(event, '\n')); err != nil {
			t.Fatalf("stream reporter write error = %v", err)
		}
	}

	want := "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n"
	if got := sink.String(); got != want {
		t.Fatalf("wrapped successful coverage output = %q, want %q", got, want)
	}
}

func TestFunctionalStreamSuppressesWrappedStandaloneCoverageOutput(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	coverPackages := make([]string, 530)
	for index := range coverPackages {
		coverPackages[index] = fmt.Sprintf("%s/pkg/cover%03d", modulePath, index)
	}
	events := []string{
		"coverage: 71.4% of statements in ",
		strings.Join(coverPackages, ", ") + "\n",
		"--- FAIL: TestSomething (0.12s)\n",
	}
	for _, output := range events {
		event, err := json.Marshal(goTestTimingEvent{
			Action:  "output",
			Package: packageName,
			Output:  output,
		})
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := writer.Write(append(event, '\n')); err != nil {
			t.Fatalf("stream reporter write error = %v", err)
		}
	}

	if got := sink.String(); got != "--- FAIL: TestSomething (0.12s)\n" {
		t.Fatalf("wrapped standalone coverage output = %q, want only the failure line", got)
	}
}

func TestFunctionalStreamSuppressesBareCoverageContinuationAfterCompleteOutput(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	coverPackages := make([]string, 530)
	for index := range coverPackages {
		coverPackages[index] = fmt.Sprintf("%s/pkg/cover%03d", modulePath, index)
	}
	coverList := strings.Join(coverPackages, ", ")
	events := []string{
		"ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
		coverList[:len(coverList)/2],
		coverList[len(coverList)/2:] + "\n",
		"--- FAIL: TestSomething (0.12s)\n",
	}
	for _, output := range events {
		event, err := json.Marshal(goTestTimingEvent{
			Action:  "output",
			Package: packageName,
			Output:  output,
		})
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := writer.Write(append(event, '\n')); err != nil {
			t.Fatalf("stream reporter write error = %v", err)
		}
	}

	want := "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n--- FAIL: TestSomething (0.12s)\n"
	if got := sink.String(); got != want {
		t.Fatalf("bare coverage continuation = %q, want %q", got, want)
	}
}

func TestFunctionalStreamSuppressesBareStandaloneCoverageContinuationAfterCompleteOutput(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	coverPackages := make([]string, 530)
	for index := range coverPackages {
		coverPackages[index] = fmt.Sprintf("%s/pkg/cover%03d", modulePath, index)
	}
	coverList := strings.Join(coverPackages, ", ")
	events := []string{
		"coverage: 42.7% of statements\n",
		coverList[:len(coverList)/2],
		coverList[len(coverList)/2:] + "\n",
		"--- FAIL: TestSomething (0.12s)\n",
	}
	for _, output := range events {
		event, err := json.Marshal(goTestTimingEvent{
			Action:  "output",
			Package: packageName,
			Output:  output,
		})
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := writer.Write(append(event, '\n')); err != nil {
			t.Fatalf("stream reporter write error: %v", err)
		}
	}

	want := "--- FAIL: TestSomething (0.12s)\n"
	if got := sink.String(); got != want {
		t.Fatalf("bare standalone coverage continuation = %q, want %q", got, want)
	}
}

func TestFunctionalStreamPreservesTimeoutOutputBeforeChildCompletes(t *testing.T) {
	liveOutput, resultCh, releaseWriter, rawOutput := startFunctionalStreamChild(t, functionalStreamTimeoutMode)
	waitForFunctionalStreamMarker(t, liveOutput, "goroutine 123 [running]")

	got := liveOutput.String()
	for _, want := range []string{"panic: test timed out", "goroutine 123 [running]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("live timeout output = %q, want %q", got, want)
		}
	}
	select {
	case result := <-resultCh:
		t.Fatalf("functional timeout child completed before release: %+v", result)
	default:
	}

	releaseFunctionalStreamChild(t, releaseWriter)
	result := waitForStreamChild(t, resultCh)
	if result.err != nil {
		t.Fatalf("functional timeout child error = %v, want nil", result.err)
	}
	if result.stdout != rawOutput {
		t.Fatalf("captured timeout JSON = %q, want unchanged %q", result.stdout, rawOutput)
	}
}

func TestFunctionalStreamReporterSerializesUniquePackageResults(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packages := []string{
		modulePath + "/tests/functional/streaming/alpha",
		modulePath + "/tests/functional/streaming/beta",
		modulePath + "/tests/functional/streaming/gamma",
	}
	events := make([][]byte, 0, len(packages)+1)
	for index, packageName := range packages {
		events = append(events, marshalFunctionalStreamEvent(goTestTimingEvent{
			Action:  timingOutcomePass,
			Elapsed: float64(index+1) / 10,
			Package: packageName,
		}))
	}
	// A duplicate terminal event must not create a second completion line.
	events = append(events, events[0])

	var writes sync.WaitGroup
	errCh := make(chan error, len(events))
	for _, event := range events {
		event := event
		writes.Add(1)
		go func() {
			defer writes.Done()
			_, err := writer.Write(event)
			errCh <- err
		}()
	}
	writes.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("stream reporter write error = %v", err)
		}
	}

	output := sink.String()
	for _, packageName := range packages {
		if got := strings.Count(output, "package="+packageName+" "); got != 1 {
			t.Fatalf("package %q appears in %d result lines, want 1; output=%q", packageName, got, output)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if !strings.HasPrefix(line, "Functional package result: ") {
			t.Fatalf("stream output line = %q, want one complete package result line", line)
		}
	}
}

func startFunctionalStreamChild(t *testing.T, mode string) (*functionalStreamCapture, <-chan streamCommandResult, *io.PipeWriter, string) {
	t.Helper()
	originalCommandRunner := commandRunner
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	releaseReader, releaseWriter := io.Pipe()
	t.Cleanup(func() { _ = releaseWriter.Close() })
	execCommand = func(string, ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestGocoveragecheckFunctionalStreamChildProcess", "--")
		cmd.Stdin = releaseReader
		return cmd
	}
	commandRunner = originalCommandRunner

	rawOutput := functionalStreamChildOutput(t, mode)
	liveOutput := newFunctionalStreamCapture(functionalStreamMarker(mode))
	stdoutWriter = liveOutput
	stderrWriter = liveOutput
	plan := coverageInvocationPlan{invocations: []commandInvocation{{
		name: "go",
		env: append(os.Environ(),
			functionalStreamChildEnv+"=1",
			functionalStreamChildMode+"="+mode,
		),
	}}}
	configureCoverageInvocationStreaming(&plan, true)

	resultCh := make(chan streamCommandResult, 1)
	go func() {
		stdout, stderr, err := runCommand(plan.invocations[0])
		resultCh <- streamCommandResult{stdout: stdout, stderr: stderr, err: err}
	}()
	return liveOutput, resultCh, releaseWriter, rawOutput
}

func releaseFunctionalStreamChild(t *testing.T, releaseWriter *io.PipeWriter) {
	t.Helper()
	if _, err := releaseWriter.Write([]byte{1}); err != nil {
		t.Fatalf("release functional stream child: %v", err)
	}
	if err := releaseWriter.Close(); err != nil {
		t.Fatalf("close functional stream child release: %v", err)
	}
}

func waitForFunctionalStreamMarker(t *testing.T, output *functionalStreamCapture, marker string) {
	t.Helper()
	select {
	case <-output.seen:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for functional stream marker %q; output=%q", marker, output.String())
	}
}

func functionalStreamMarker(mode string) string {
	if mode == functionalStreamTimeoutMode {
		return "goroutine 123 [running]"
	}
	return "Functional package result: package=" + functionalStreamChildPkg
}

func functionalStreamChildOutput(t *testing.T, mode string) string {
	t.Helper()
	var output strings.Builder
	for _, event := range functionalStreamChildEvents(mode) {
		data := marshalFunctionalStreamEvent(event)
		output.Write(data)
	}
	return output.String()
}

func functionalStreamChildEvents(mode string) []goTestTimingEvent {
	if mode == functionalStreamTimeoutMode {
		return []goTestTimingEvent{{
			Action:  "output",
			Package: functionalStreamChildPkg,
			Output:  "panic: test timed out\ngoroutine 123 [running]:\n\tcreated by TestHung in goroutine 1\n",
		}}
	}
	return []goTestTimingEvent{
		{
			Action:  "output",
			Package: functionalStreamChildPkg,
			Output:  functionalStreamCoverageResultOutput(),
		},
		{
			Action:  timingOutcomePass,
			Elapsed: 0.125,
			Package: functionalStreamChildPkg,
		},
	}
}

func functionalStreamCoverageResultOutput() string {
	coverPackages := make([]string, 530)
	for index := range coverPackages {
		coverPackages[index] = fmt.Sprintf("%s/pkg/cover%03d", modulePath, index)
	}
	return "ok  " + functionalStreamChildPkg + "\t0.125s\tcoverage: 71.4% of statements in " + strings.Join(coverPackages, ", ") + "\n"
}

func marshalFunctionalStreamEvent(event goTestTimingEvent) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func TestGocoveragecheckFunctionalStreamChildProcess(t *testing.T) {
	if os.Getenv(functionalStreamChildEnv) != "1" {
		return
	}
	mode := os.Getenv(functionalStreamChildMode)
	for _, event := range functionalStreamChildEvents(mode) {
		_, _ = os.Stdout.Write(marshalFunctionalStreamEvent(event))
	}
	var release [1]byte
	if _, err := io.ReadFull(os.Stdin, release[:]); err != nil {
		os.Exit(8)
	}
	os.Exit(0)
}

type functionalStreamCapture struct {
	mu     sync.Mutex
	data   bytes.Buffer
	marker string
	seen   chan struct{}
	once   sync.Once
}

func newFunctionalStreamCapture(marker string) *functionalStreamCapture {
	return &functionalStreamCapture{
		marker: marker,
		seen:   make(chan struct{}),
	}
}

func (capture *functionalStreamCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	n, err := capture.data.Write(data)
	if strings.Contains(capture.data.String(), capture.marker) {
		capture.once.Do(func() { close(capture.seen) })
	}
	return n, err
}

func (capture *functionalStreamCapture) String() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.data.String()
}

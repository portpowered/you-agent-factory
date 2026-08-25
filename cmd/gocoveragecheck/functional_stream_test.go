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

func TestFunctionalStreamSuppressesKnownLifecycleActionsButNotObserverUpdates(t *testing.T) {
	var sink bytes.Buffer
	observed := make([]goTestTimingEvent, 0, 4)
	reporter := newFunctionalStreamReporterWithObserver(&sink, func(event goTestTimingEvent) {
		observed = append(observed, event)
	})
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	for _, action := range []string{"start", "run", "pause", "cont"} {
		if _, err := writer.Write(marshalFunctionalStreamEvent(goTestTimingEvent{
			Action:  action,
			Package: packageName,
		})); err != nil {
			t.Fatalf("write %s event: %v", action, err)
		}
	}

	if got := sink.String(); got != "" {
		t.Fatalf("known lifecycle events reached the human stream: %q", got)
	}
	if len(observed) != 4 {
		t.Fatalf("observer saw %d lifecycle events, want 4", len(observed))
	}
	for index, action := range []string{"start", "run", "pause", "cont"} {
		if observed[index].Action != action || observed[index].Package != packageName {
			t.Fatalf("observer event %d = %+v, want action=%s package=%s", index, observed[index], action, packageName)
		}
	}
}

func TestFunctionalStreamQuietModeSuppressesConcurrentSuccessfulOutput(t *testing.T) {
	var sink bytes.Buffer
	var observedMu sync.Mutex
	var observed []goTestTimingEvent
	reporter := newFunctionalStreamReporterWithObserverMode(&sink, func(event goTestTimingEvent) {
		observedMu.Lock()
		defer observedMu.Unlock()
		observed = append(observed, event)
	}, true)
	writer := reporter.stdoutWriter()
	packages := []string{
		modulePath + "/tests/functional/quiet/alpha",
		modulePath + "/tests/functional/quiet/beta",
	}
	if _, err := writer.Write([]byte("raw successful test debug output\n")); err != nil {
		t.Fatalf("write raw quiet functional output: %v", err)
	}

	var writes sync.WaitGroup
	for index, packageName := range packages {
		packageName := packageName
		elapsed := float64(index+1) / 10
		writes.Add(1)
		go func() {
			defer writes.Done()
			for _, event := range []goTestTimingEvent{
				{Action: "run", Package: packageName, Test: "TestNoisy"},
				{Action: "output", Package: packageName, Test: "TestNoisy", Output: "=== RUN   TestNoisy\nsuccessful test debug log\n--- PASS: TestNoisy (0.01s)\n"},
				{Action: timingOutcomePass, Package: packageName, Test: "TestNoisy", Elapsed: 0.01},
				{Action: "output", Package: packageName, Output: "ok  \t" + packageName + "\t" + fmt.Sprintf("%.3fs\n", elapsed)},
				{Action: timingOutcomePass, Package: packageName, Elapsed: elapsed},
			} {
				if _, err := writer.Write(marshalFunctionalStreamEvent(event)); err != nil {
					t.Errorf("write %s event: %v", packageName, err)
				}
			}
		}()
	}
	writes.Wait()
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush quiet functional stream: %v", err)
	}

	if got := sink.String(); got != "" {
		t.Fatalf("quiet functional stream emitted child output: %q", got)
	}
	observedMu.Lock()
	defer observedMu.Unlock()
	if len(observed) != len(packages)*5 {
		t.Fatalf("quiet functional stream observer saw %d events, want %d", len(observed), len(packages)*5)
	}
}

func TestFunctionalStreamQuietFlushObservesUnterminatedEvent(t *testing.T) {
	var sink bytes.Buffer
	var observed []goTestTimingEvent
	reporter := newFunctionalStreamReporterWithObserverMode(&sink, func(event goTestTimingEvent) {
		observed = append(observed, event)
	}, true)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/quiet/timeout"
	finalEvent := goTestTimingEvent{
		Action:  timingOutcomeFail,
		Package: packageName,
		Elapsed: 2.5,
	}
	data, err := json.Marshal(finalEvent)
	if err != nil {
		t.Fatalf("marshal final quiet event: %v", err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write unterminated quiet event: %v", err)
	}

	if err := writer.Flush(); err != nil {
		t.Fatalf("flush unterminated quiet event: %v", err)
	}
	if sink.Len() != 0 {
		t.Fatalf("quiet flush emitted child output: %q", sink.String())
	}
	if len(observed) != 1 || observed[0] != finalEvent {
		t.Fatalf("quiet flush observed = %+v, want final event %+v", observed, finalEvent)
	}
}

func TestFunctionalStreamPreservesUnknownActionAndNormalOutputAndResult(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()
	packageName := modulePath + "/tests/functional/streaming"
	unknown := []byte(`{"Time":"2026-08-20T12:00:00Z","Action":"future","Package":"` + packageName + `","Test":"TestFuture"}` + "\n")
	if _, err := writer.Write(unknown); err != nil {
		t.Fatalf("write unknown action: %v", err)
	}
	if _, err := writer.Write(marshalFunctionalStreamEvent(goTestTimingEvent{
		Action:  "output",
		Package: packageName,
		Output:  "--- FAIL: TestVisible (0.01s)\n",
	})); err != nil {
		t.Fatalf("write output event: %v", err)
	}
	if _, err := writer.Write(marshalFunctionalStreamEvent(goTestTimingEvent{
		Action:  timingOutcomeFail,
		Package: packageName,
		Elapsed: 0.01,
	})); err != nil {
		t.Fatalf("write package result event: %v", err)
	}

	want := string(unknown) + "--- FAIL: TestVisible (0.01s)\n" +
		"Functional package result: package=" + packageName + " outcome=fail elapsed=0.010s\n"
	if got := sink.String(); got != want {
		t.Fatalf("functional stream = %q, want unknown action, output, and result %q", got, want)
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

func TestFunctionalStreamPreservesModulePathDiagnosticsAfterCoverageRecords(t *testing.T) {
	packageName := modulePath + "/tests/functional/streaming"
	tests := []struct {
		name         string
		coverage     string
		wantCoverage string
		diagnostic   string
	}{
		{
			name:         "successful result build diagnostic",
			coverage:     "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
			wantCoverage: "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
			diagnostic:   "# " + modulePath + "/pkg/diagnostic: build failed\n",
		},
		{
			name:         "successful result stack frame",
			coverage:     "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
			wantCoverage: "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
			diagnostic:   modulePath + "/pkg/diagnostic.TestSomething(0x123)\n",
		},
		{
			name:       "standalone coverage build diagnostic",
			coverage:   "coverage: 42.7% of statements\n",
			diagnostic: "# " + modulePath + "/pkg/diagnostic: build failed\n",
		},
		{
			name:       "standalone coverage stack frame",
			coverage:   "coverage: 42.7% of statements\n",
			diagnostic: modulePath + "/pkg/diagnostic.TestSomething(0x123)\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sink bytes.Buffer
			reporter := newFunctionalStreamReporter(&sink)
			writer := reporter.stdoutWriter()
			for _, output := range []string{test.coverage, test.diagnostic} {
				if _, err := writer.Write(marshalFunctionalStreamEvent(goTestTimingEvent{
					Action:  "output",
					Package: packageName,
					Output:  output,
				})); err != nil {
					t.Fatalf("stream reporter write error: %v", err)
				}
			}

			if got, want := sink.String(), test.wantCoverage+test.diagnostic; got != want {
				t.Fatalf("coverage diagnostic output = %q, want %q", got, want)
			}
		})
	}
}

func TestFunctionalStreamSuppressesPackageListSplitBeforeNextModulePath(t *testing.T) {
	packageName := modulePath + "/tests/functional/streaming"
	tests := []struct {
		name         string
		coverage     string
		wantCoverage string
	}{
		{
			name:         "successful result",
			coverage:     "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
			wantCoverage: "ok  " + packageName + "\t1.234s\tcoverage: 42.7% of statements\n",
		},
		{
			name:     "standalone coverage",
			coverage: "coverage: 42.7% of statements\n",
		},
	}
	diagnostic := "# " + modulePath + "/pkg/diagnostic: build failed\n"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sink bytes.Buffer
			reporter := newFunctionalStreamReporter(&sink)
			writer := reporter.stdoutWriter()
			for _, output := range []string{
				test.coverage,
				modulePath + "/pkg/cover-first, ",
				modulePath + "/pkg/cover-second\n",
				diagnostic,
			} {
				if _, err := writer.Write(marshalFunctionalStreamEvent(goTestTimingEvent{
					Action:  "output",
					Package: packageName,
					Output:  output,
				})); err != nil {
					t.Fatalf("stream reporter write error: %v", err)
				}
			}

			if got, want := sink.String(), test.wantCoverage+diagnostic; got != want {
				t.Fatalf("package-list continuation output = %q, want %q", got, want)
			}
		})
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

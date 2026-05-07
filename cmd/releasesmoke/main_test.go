package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/releasesmoke"
)

type failingWriter struct {
	err error
}

func (writer failingWriter) Write(_ []byte) (int, error) {
	return 0, writer.err
}

func TestMainRoutesThroughCommandMain(t *testing.T) {
	originalCommandMain := commandMain
	originalExitFunc := exitFunc
	originalStdout := stdout
	originalStderr := stderr
	originalArgs := os.Args
	t.Cleanup(func() {
		commandMain = originalCommandMain
		exitFunc = originalExitFunc
		stdout = originalStdout
		stderr = originalStderr
		os.Args = originalArgs
	})

	var gotArgs []string
	var gotStdout io.Writer
	var gotStderr io.Writer
	var exitCode int
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	commandMain = func(args []string, stdout io.Writer, stderr io.Writer) int {
		gotArgs = append([]string(nil), args...)
		gotStdout = stdout
		gotStderr = stderr
		return 17
	}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"releasesmoke", "-binary", "dist/factory", "-fixture", "testdata/fixture", "-timeout", "45s"}

	main()

	if exitCode != 17 {
		t.Fatalf("main() exit code = %d, want 17", exitCode)
	}
	if len(gotArgs) != 6 {
		t.Fatalf("main() args len = %d, want 6 (%v)", len(gotArgs), gotArgs)
	}
	if gotArgs[0] != "-binary" || gotArgs[1] != "dist/factory" || gotArgs[2] != "-fixture" || gotArgs[3] != "testdata/fixture" || gotArgs[4] != "-timeout" || gotArgs[5] != "45s" {
		t.Fatalf("main() args = %v", gotArgs)
	}
	if gotStdout != out {
		t.Fatalf("main() stdout writer mismatch")
	}
	if gotStderr != errOut {
		t.Fatalf("main() stderr writer mismatch")
	}
}

func TestRunForwardsParsedConfigToHarness(t *testing.T) {
	originalRunReleaseSmoke := runReleaseSmoke
	t.Cleanup(func() {
		runReleaseSmoke = originalRunReleaseSmoke
	})

	var gotCfg releasesmoke.Config
	runReleaseSmoke = func(_ context.Context, cfg releasesmoke.Config) (releasesmoke.Result, error) {
		gotCfg = cfg
		return releasesmoke.Result{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-binary", "dist/factory", "-fixture", "testdata/fixture", "-timeout", "45s"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if gotCfg.BinaryPath != "dist/factory" {
		t.Fatalf("run() binary path = %q, want %q", gotCfg.BinaryPath, "dist/factory")
	}
	if gotCfg.FixturePath != "testdata/fixture" {
		t.Fatalf("run() fixture path = %q, want %q", gotCfg.FixturePath, "testdata/fixture")
	}
	if gotCfg.Timeout != 45*time.Second {
		t.Fatalf("run() timeout = %s, want %s", gotCfg.Timeout, 45*time.Second)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunInvalidArgsWritesFlagParseErrorToStderrAndReturnsNonZero(t *testing.T) {
	originalRunReleaseSmoke := runReleaseSmoke
	t.Cleanup(func() {
		runReleaseSmoke = originalRunReleaseSmoke
	})

	runReleaseSmoke = func(_ context.Context, cfg releasesmoke.Config) (releasesmoke.Result, error) {
		t.Fatalf("runReleaseSmoke() should not be called for invalid args: %+v", cfg)
		return releasesmoke.Result{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-bogus"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("run() exit code = %d, want non-zero", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := strings.TrimSpace(stderr.String()); !strings.Contains(got, "flag provided but not defined: -bogus") {
		t.Fatalf("run() stderr = %q, want raw flag parse error", got)
	}
}

func TestRunSuccessWritesStructuredJSONToStdout(t *testing.T) {
	originalRunReleaseSmoke := runReleaseSmoke
	t.Cleanup(func() {
		runReleaseSmoke = originalRunReleaseSmoke
	})

	wantResult := releasesmoke.Result{
		BaseURL:            "http://127.0.0.1:7777",
		DashboardURL:       "http://127.0.0.1:7777/dashboard",
		WorkspacePath:      "C:/tmp/releasesmoke-workspace",
		ObservedEventTypes: []string{"factory.started", "work.completed"},
	}
	runReleaseSmoke = func(_ context.Context, cfg releasesmoke.Config) (releasesmoke.Result, error) {
		return wantResult, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-binary", "dist/factory", "-fixture", "testdata/fixture", "-timeout", "45s"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}

	var got releasesmoke.Result
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("run() stdout JSON decode error = %v\noutput:\n%s", err, stdout.String())
	}
	if got.BaseURL != wantResult.BaseURL {
		t.Fatalf("run() stdout baseUrl = %q, want %q", got.BaseURL, wantResult.BaseURL)
	}
	if got.DashboardURL != wantResult.DashboardURL {
		t.Fatalf("run() stdout dashboardUrl = %q, want %q", got.DashboardURL, wantResult.DashboardURL)
	}
	if got.WorkspacePath != wantResult.WorkspacePath {
		t.Fatalf("run() stdout workspacePath = %q, want %q", got.WorkspacePath, wantResult.WorkspacePath)
	}
	if len(got.ObservedEventTypes) != len(wantResult.ObservedEventTypes) {
		t.Fatalf("run() stdout observedEventTypes len = %d, want %d", len(got.ObservedEventTypes), len(wantResult.ObservedEventTypes))
	}
	for index, wantType := range wantResult.ObservedEventTypes {
		if got.ObservedEventTypes[index] != wantType {
			t.Fatalf("run() stdout observedEventTypes[%d] = %q, want %q", index, got.ObservedEventTypes[index], wantType)
		}
	}
	if stdout.String() == "" || stdout.String()[0] != '{' {
		t.Fatalf("run() stdout = %q, want JSON object output", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\n  \"baseUrl\": ")) {
		t.Fatalf("run() stdout = %q, want indented JSON field", stdout.String())
	}
}

func TestRunFailureWritesStructuredJSONToStderrAndReturnsNonZero(t *testing.T) {
	originalRunReleaseSmoke := runReleaseSmoke
	t.Cleanup(func() {
		runReleaseSmoke = originalRunReleaseSmoke
	})

	wantFailure := &releasesmoke.Failure{
		Phase:              "verify_events",
		Message:            "timed out waiting for work.completed",
		BinaryPath:         "dist/factory",
		FixturePath:        "testdata/fixture",
		BaseURL:            "http://127.0.0.1:7777",
		DashboardURL:       "http://127.0.0.1:7777/dashboard",
		WorkspacePath:      "C:/tmp/releasesmoke-workspace",
		ObservedEventTypes: []string{"factory.started"},
	}
	runReleaseSmoke = func(_ context.Context, cfg releasesmoke.Config) (releasesmoke.Result, error) {
		return releasesmoke.Result{}, wantFailure
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-binary", "dist/factory", "-fixture", "testdata/fixture", "-timeout", "45s"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("run() exit code = %d, want non-zero", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	var got releasesmoke.Failure
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("run() stderr JSON decode error = %v\noutput:\n%s", err, stderr.String())
	}
	if got.Phase != wantFailure.Phase {
		t.Fatalf("run() stderr phase = %q, want %q", got.Phase, wantFailure.Phase)
	}
	if got.Message != wantFailure.Message {
		t.Fatalf("run() stderr message = %q, want %q", got.Message, wantFailure.Message)
	}
	if got.BinaryPath != wantFailure.BinaryPath {
		t.Fatalf("run() stderr binaryPath = %q, want %q", got.BinaryPath, wantFailure.BinaryPath)
	}
	if got.FixturePath != wantFailure.FixturePath {
		t.Fatalf("run() stderr fixturePath = %q, want %q", got.FixturePath, wantFailure.FixturePath)
	}
	if got.BaseURL != wantFailure.BaseURL {
		t.Fatalf("run() stderr baseUrl = %q, want %q", got.BaseURL, wantFailure.BaseURL)
	}
	if got.DashboardURL != wantFailure.DashboardURL {
		t.Fatalf("run() stderr dashboardUrl = %q, want %q", got.DashboardURL, wantFailure.DashboardURL)
	}
	if got.WorkspacePath != wantFailure.WorkspacePath {
		t.Fatalf("run() stderr workspacePath = %q, want %q", got.WorkspacePath, wantFailure.WorkspacePath)
	}
	if len(got.ObservedEventTypes) != 1 || got.ObservedEventTypes[0] != "factory.started" {
		t.Fatalf("run() stderr observedEventTypes = %v, want [factory.started]", got.ObservedEventTypes)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("\n  \"phase\": ")) {
		t.Fatalf("run() stderr = %q, want indented JSON field", stderr.String())
	}
}

func TestWriteJSONReportsEncodeFailuresToStderr(t *testing.T) {
	originalStderr := stderr
	t.Cleanup(func() {
		stderr = originalStderr
	})

	var errOut bytes.Buffer
	stderr = &errOut

	writeJSON(failingWriter{err: errors.New("writer failed")}, map[string]string{"status": "ok"})

	if got := strings.TrimSpace(errOut.String()); !strings.Contains(got, "encode JSON output: writer failed") {
		t.Fatalf("writeJSON() stderr = %q, want encode failure diagnostic", got)
	}
}

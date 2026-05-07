package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/releaseprep"
)

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
	os.Args = []string{"releaseprep", "-version", "v1.2.3"}

	main()

	if exitCode != 17 {
		t.Fatalf("main() exit code = %d, want 17", exitCode)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-version" || gotArgs[1] != "v1.2.3" {
		t.Fatalf("main() args = %v, want [-version v1.2.3]", gotArgs)
	}
	if gotStdout != out {
		t.Fatalf("main() stdout writer mismatch")
	}
	if gotStderr != errOut {
		t.Fatalf("main() stderr writer mismatch")
	}
}

func TestRunForwardsVersionAndProgressWriter(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
	})

	var gotOptions releaseprep.Options
	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		gotOptions = options
		return nil
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitCode := run([]string{"-version", "v1.2.3"}, out, errOut)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if gotOptions.Version != "v1.2.3" {
		t.Fatalf("run() version = %q, want %q", gotOptions.Version, "v1.2.3")
	}
	if gotOptions.ProgressWriter != out {
		t.Fatalf("run() progress writer mismatch")
	}
}

func TestRunReturnsZeroOnFlagHelpWithoutExecutingReleasePrep(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
	})

	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		t.Fatal("runReleasePrep should not be called when flag parsing returns help")
		return nil
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitCode := run([]string{"-h"}, out, errOut)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := out.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := errOut.String(); got == "" {
		t.Fatal("run() stderr was empty, want help output")
	}
	if !bytes.Contains(errOut.Bytes(), []byte("Usage of releaseprep:\n")) {
		t.Fatalf("run() stderr = %q, want usage header", errOut.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("-version string")) {
		t.Fatalf("run() stderr = %q, want version flag usage", errOut.String())
	}
}

func TestRunReturnsTwoOnFlagParseFailureWithoutExecutingReleasePrep(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
	})

	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		t.Fatal("runReleasePrep should not be called when flag parsing fails")
		return nil
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitCode := run([]string{"-bogus"}, out, errOut)

	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
	if got := out.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	gotErr := errOut.String()
	if gotErr == "" {
		t.Fatal("run() stderr was empty, want parse error and usage")
	}
	if !bytes.Contains(errOut.Bytes(), []byte("flag provided but not defined: -bogus")) {
		t.Fatalf("run() stderr = %q, want unknown flag error", gotErr)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("Usage of releaseprep:\n")) {
		t.Fatalf("run() stderr = %q, want usage header", gotErr)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("-version string")) {
		t.Fatalf("run() stderr = %q, want version flag usage", gotErr)
	}
}

func TestMainSuccessWritesProgressToStdoutAndExitsZero(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	originalExitFunc := exitFunc
	originalStdout := stdout
	originalStderr := stderr
	originalArgs := os.Args
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
		exitFunc = originalExitFunc
		stdout = originalStdout
		stderr = originalStderr
		os.Args = originalArgs
	})

	var gotOptions releaseprep.Options
	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		gotOptions = options
		_, err := io.WriteString(options.ProgressWriter, "checking release branch\nready to tag\n")
		return err
	}

	var exitCode int
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"releaseprep", "-version", "v1.2.3"}

	main()

	if exitCode != 0 {
		t.Fatalf("main() exit code = %d, want 0", exitCode)
	}
	if gotOptions.Version != "v1.2.3" {
		t.Fatalf("main() version = %q, want %q", gotOptions.Version, "v1.2.3")
	}
	if gotOptions.ProgressWriter != out {
		t.Fatalf("main() progress writer mismatch")
	}
	if got := out.String(); got != "checking release branch\nready to tag\n" {
		t.Fatalf("main() stdout = %q", got)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("main() stderr = %q, want empty", got)
	}
}

func TestMainFailureWritesStderrAndExitsNonZero(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	originalExitFunc := exitFunc
	originalStdout := stdout
	originalStderr := stderr
	originalArgs := os.Args
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
		exitFunc = originalExitFunc
		stdout = originalStdout
		stderr = originalStderr
		os.Args = originalArgs
	})

	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		return errors.New("release readiness check failed")
	}

	var exitCode int
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"releaseprep", "-version", "v1.2.3"}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := out.String(); got != "" {
		t.Fatalf("main() stdout = %q, want empty", got)
	}
	if got := errOut.String(); got != "release readiness check failed\n" {
		t.Fatalf("main() stderr = %q", got)
	}
}

func TestMainInvalidFlagWritesUsageToStderrAndExitsTwo(t *testing.T) {
	originalRunReleasePrep := runReleasePrep
	originalExitFunc := exitFunc
	originalStdout := stdout
	originalStderr := stderr
	originalArgs := os.Args
	t.Cleanup(func() {
		runReleasePrep = originalRunReleasePrep
		exitFunc = originalExitFunc
		stdout = originalStdout
		stderr = originalStderr
		os.Args = originalArgs
	})

	runReleasePrep = func(_ context.Context, options releaseprep.Options) error {
		t.Fatalf("runReleasePrep should not be called on flag parse failure")
		return nil
	}

	var exitCode int
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"releaseprep", "-bogus"}

	main()

	if exitCode != 2 {
		t.Fatalf("main() exit code = %d, want 2", exitCode)
	}
	if got := out.String(); got != "" {
		t.Fatalf("main() stdout = %q, want empty", got)
	}
	gotErr := errOut.String()
	if gotErr == "" {
		t.Fatal("main() stderr was empty, want parse error and usage")
	}
	if !bytes.Contains(errOut.Bytes(), []byte("flag provided but not defined: -bogus")) {
		t.Fatalf("main() stderr = %q, want unknown flag error", gotErr)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("Usage of releaseprep:\n")) {
		t.Fatalf("main() stderr = %q, want usage header", gotErr)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("-version string")) {
		t.Fatalf("main() stderr = %q, want version flag usage", gotErr)
	}
}

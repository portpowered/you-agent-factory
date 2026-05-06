package main

import (
	"bytes"
	"context"
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

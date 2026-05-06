package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/releasesmoke"
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

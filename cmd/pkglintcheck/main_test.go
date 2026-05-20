package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
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
		return 23
	}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"pkglintcheck", "-example"}

	main()

	if exitCode != 23 {
		t.Fatalf("main() exit code = %d, want 23", exitCode)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "-example" {
		t.Fatalf("main() args = %v, want [-example]", gotArgs)
	}
	if gotStdout != out {
		t.Fatal("main() stdout writer mismatch")
	}
	if gotStderr != errOut {
		t.Fatal("main() stderr writer mismatch")
	}
}

func TestRunPkgLintUsesExpectedCommand(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_PKGLINTCHECK_HELPER", "1")

	var captured *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		captured = fakePkgLintCheckCommand(name, args...)
		return captured
	}

	if err := runPkgLint(); err != nil {
		t.Fatalf("runPkgLint() error = %v, want nil", err)
	}
	if captured == nil {
		t.Fatal("runPkgLint() did not create a subprocess command")
	}
	if got := captured.Args; len(got) < 8 || got[len(got)-7] != "go" || got[len(got)-6] != "run" || got[len(got)-5] != golangciTool || got[len(got)-4] != "run" || got[len(got)-3] != "--config" || got[len(got)-2] != configPath {
		t.Fatalf("runPkgLint() args = %v, want go run %s run --config %s %s", captured.Args, golangciTool, configPath, packageScope)
	}
	if got := captured.Args[len(captured.Args)-1]; got != packageScope {
		t.Fatalf("runPkgLint() package scope = %q, want %q", got, packageScope)
	}
}

func TestRunReportsSuccessMessage(t *testing.T) {
	restore := stubPkgLintCommand(t, nil)
	defer restore()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); got != "[agent-factory:pkg-lint] pkg lint passed\n" {
		t.Fatalf("run() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunFailurePreservesToolOutput(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_PKGLINTCHECK_HELPER", "1")
	t.Setenv("PKGLINTCHECK_HELPER_STDOUT", "pkg/foo.go:12: lint failure\n")
	t.Setenv("PKGLINTCHECK_HELPER_STDERR", "helper stderr\n")
	t.Setenv("PKGLINTCHECK_HELPER_FAIL", "1")
	execCommand = fakePkgLintCheckCommand

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "run pkg lint:") {
		t.Fatalf("run() stderr = %q, want run pkg lint context", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/foo.go:12: lint failure") {
		t.Fatalf("run() stderr = %q, want tool stdout details", errOutput)
	}
	if !strings.Contains(errOutput, "helper stderr") {
		t.Fatalf("run() stderr = %q, want tool stderr details", errOutput)
	}
}

func stubPkgLintCommand(t *testing.T, err error) func() {
	t.Helper()
	originalRunPkgLintCommand := runPkgLintCommand
	runPkgLintCommand = func() error {
		return err
	}
	return func() {
		runPkgLintCommand = originalRunPkgLintCommand
	}
}

func restoreExecCommand(t *testing.T) {
	t.Helper()
	originalExecCommand := execCommand
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})
}

func fakePkgLintCheckCommand(name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{"-test.run=TestHelperProcessPkgLintCheck", "--", name}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_PKGLINTCHECK_HELPER=1")
	return cmd
}

func TestHelperProcessPkgLintCheck(t *testing.T) {
	if os.Getenv("GO_WANT_PKGLINTCHECK_HELPER") != "1" {
		return
	}

	if value := os.Getenv("PKGLINTCHECK_HELPER_STDOUT"); value != "" {
		_, _ = io.WriteString(os.Stdout, value)
	}
	if value := os.Getenv("PKGLINTCHECK_HELPER_STDERR"); value != "" {
		_, _ = io.WriteString(os.Stderr, value)
	}
	if os.Getenv("PKGLINTCHECK_HELPER_FAIL") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

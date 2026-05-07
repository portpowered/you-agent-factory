package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		return 17
	}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"deadcodecheck", "-example"}

	main()

	if exitCode != 17 {
		t.Fatalf("main() exit code = %d, want 17", exitCode)
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

func TestRunDeadcodeUsesExpectedCommandAndAmbientEnvironment(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_STDOUT", "pkg/foo.go: Example\n")

	var captured *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		captured = fakeDeadcodecheckCommand(name, args...)
		return captured
	}

	report, err := runDeadcode()
	if err != nil {
		t.Fatalf("runDeadcode() error = %v, want nil", err)
	}
	if report != "pkg/foo.go: Example\n" {
		t.Fatalf("runDeadcode() report = %q, want helper stdout", report)
	}
	if captured == nil {
		t.Fatal("runDeadcode() did not create a subprocess command")
	}
	if got := captured.Args; len(got) < 7 || got[len(got)-5] != "go" || got[len(got)-4] != "run" || got[len(got)-3] != deadcodeTool || got[len(got)-2] != "-test" || got[len(got)-1] != "./..." {
		t.Fatalf("runDeadcode() args = %v, want go run %s -test ./...", captured.Args, deadcodeTool)
	}
	if captured.Env != nil {
		t.Fatalf("runDeadcode() env override = %v, want ambient environment inheritance", captured.Env)
	}
}

func TestRunBaselineMatchWritesCurrentReport(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "pkg\\foo.go: Example\n", nil)
	defer restore()

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Example\r\n")
	chdirForTest(t, tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0 with stderr %q", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "[agent-factory:deadcode] baseline matches\n" {
		t.Fatalf("run() stdout = %q, want baseline match message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}

	currentReport, err := os.ReadFile(filepath.Join(tempDir, currentPath))
	if err != nil {
		t.Fatalf("read current deadcode report: %v", err)
	}
	if got := string(currentReport); got != "pkg/foo.go: Example\n" {
		t.Fatalf("current deadcode report = %q, want normalized report", got)
	}
}

func TestRunBaselineDriftReportsCurrentAndBaselinePaths(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "pkg/foo.go: Current\n", nil)
	defer restore()

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Baseline\n")
	chdirForTest(t, tempDir)

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
	if !strings.Contains(errOutput, "deadcode baseline drift detected; review "+currentPath+" and update "+baselinePath+" when intentional") {
		t.Fatalf("run() stderr = %q, want drift guidance", errOutput)
	}
	if !strings.Contains(errOutput, "baseline findings: 1, current findings: 1") {
		t.Fatalf("run() stderr = %q, want finding counts", errOutput)
	}

	currentReport, err := os.ReadFile(filepath.Join(tempDir, currentPath))
	if err != nil {
		t.Fatalf("read current deadcode report: %v", err)
	}
	if got := string(currentReport); got != "pkg/foo.go: Current\n" {
		t.Fatalf("current deadcode report = %q, want current findings", got)
	}
}

func TestRunDeadcodeFailurePreservesContextAndToolStderr(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_STDERR", "fake deadcode stderr\n")
	t.Setenv("DEADCODECHECK_HELPER_FAIL", "1")
	execCommand = fakeDeadcodecheckCommand

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
	if !strings.Contains(errOutput, "run deadcode:") {
		t.Fatalf("run() stderr = %q, want run deadcode context", errOutput)
	}
	if !strings.Contains(errOutput, "fake deadcode stderr") {
		t.Fatalf("run() stderr = %q, want tool stderr details", errOutput)
	}
}

func TestRunFailsWhenCurrentOutputDirectorySetupFails(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "pkg/foo.go: Example\n", nil)
	defer restore()

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Example\n")
	blockingPath := filepath.Join(tempDir, "bin")
	if err := os.WriteFile(blockingPath, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write blocking bin path: %v", err)
	}
	chdirForTest(t, tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "create deadcode output directory:") {
		t.Fatalf("run() stderr = %q, want output directory failure", got)
	}
}

func TestRunFailsWhenCurrentReportWriteFails(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "pkg\\foo.go: Example", nil)
	defer restore()

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Example\n")
	blockingPath := filepath.Join(tempDir, currentPath)
	if err := os.MkdirAll(blockingPath, 0o755); err != nil {
		t.Fatalf("create blocking current report directory: %v", err)
	}
	chdirForTest(t, tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "write current deadcode report:") {
		t.Fatalf("run() stderr = %q, want current report write failure", got)
	}

	currentReportInfo, err := os.Stat(blockingPath)
	if err != nil {
		t.Fatalf("stat blocking current report path: %v", err)
	}
	if !currentReportInfo.IsDir() {
		t.Fatalf("current report path mode = %v, want directory to preserve write failure", currentReportInfo.Mode())
	}
}

func TestRunFailsWhenBaselineReadFails(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "pkg\\foo.go: Example", nil)
	defer restore()

	tempDir := t.TempDir()
	baselineDir := filepath.Join(tempDir, baselinePath)
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		t.Fatalf("create blocking baseline directory: %v", err)
	}
	chdirForTest(t, tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(nil, stdout, stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "read deadcode baseline:") {
		t.Fatalf("run() stderr = %q, want baseline read failure", got)
	}

	currentReport, err := os.ReadFile(filepath.Join(tempDir, currentPath))
	if err != nil {
		t.Fatalf("read current deadcode report: %v", err)
	}
	if got := string(currentReport); got != "pkg/foo.go: Example\n" {
		t.Fatalf("current deadcode report = %q, want normalized report before baseline read failure", got)
	}
}

func TestDeadcodecheckFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" || os.Getenv("GO_WANT_DEADCODECHECK_HELPER") != "1" {
		return
	}

	if os.Getenv("DEADCODECHECK_HELPER_STDOUT") != "" {
		fmt.Fprint(os.Stdout, os.Getenv("DEADCODECHECK_HELPER_STDOUT"))
	}
	if os.Getenv("DEADCODECHECK_HELPER_STDERR") != "" {
		fmt.Fprint(os.Stderr, os.Getenv("DEADCODECHECK_HELPER_STDERR"))
	}
	if os.Getenv("DEADCODECHECK_HELPER_FAIL") == "1" {
		os.Exit(2)
	}
	os.Exit(0)
}

func stubDeadcodecheckCommand(t *testing.T, output string, err error) func() {
	t.Helper()

	original := runDeadcodeCommand
	runDeadcodeCommand = func() (string, error) {
		return output, err
	}
	return func() {
		runDeadcodeCommand = original
	}
}

func writeDeadcodeBaseline(t *testing.T, root string, contents string) {
	t.Helper()

	baselineFile := filepath.Join(root, baselinePath)
	if err := os.MkdirAll(filepath.Dir(baselineFile), 0o755); err != nil {
		t.Fatalf("create baseline directory: %v", err)
	}
	if err := os.WriteFile(baselineFile, []byte(contents), 0o644); err != nil {
		t.Fatalf("write baseline file: %v", err)
	}
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func fakeDeadcodecheckCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestDeadcodecheckFakeGoProcess", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func helperCommandArgs(argv []string) ([]string, bool) {
	for index, arg := range argv {
		if arg == "--" {
			return argv[index+1:], true
		}
	}
	return nil, false
}

func restoreExecCommand(t *testing.T) {
	t.Helper()

	original := execCommand
	t.Cleanup(func() {
		execCommand = original
	})
}

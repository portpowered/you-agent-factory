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

func TestRunDeadcodeUsesExpectedCommandAndGoTypesAliasEnvironment(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_STDOUT", "pkg/foo.go: Example\n")
	t.Setenv("GODEBUG", "gocachehash=1,gotypesalias=0")

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
	if !envContains(captured.Env, "GODEBUG=gocachehash=1,gotypesalias=1") {
		t.Fatalf("runDeadcode() env = %v, want gotypesalias enabled", captured.Env)
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

func TestRunSuccessfulDeadcodePassesThroughToolStderr(t *testing.T) {
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_STDOUT", "pkg/foo.go: Example\n")
	t.Setenv("DEADCODECHECK_HELPER_STDERR", "fake deadcode stderr\n")
	execCommand = fakeDeadcodecheckCommand

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Example\n")
	chdirForTest(t, tempDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	originalStderr := os.Stderr
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = originalStderr
	}()

	exitCode := run(nil, stdout, stderr)

	if err := writeStderr.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	passthroughStderr, err := io.ReadAll(readStderr)
	if err != nil {
		t.Fatalf("read passthrough stderr: %v", err)
	}
	if err := readStderr.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); got != "[agent-factory:deadcode] baseline matches\n" {
		t.Fatalf("run() stdout = %q, want baseline match message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty command stderr on successful passthrough", got)
	}
	if got := string(passthroughStderr); got != "fake deadcode stderr\n" {
		t.Fatalf("passthrough stderr = %q, want helper stderr", got)
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

func TestEnsureGoTypesAliasEnabled(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "gotypesalias=1"},
		{name: "preserves other flags", in: "gocachehash=1", want: "gocachehash=1,gotypesalias=1"},
		{name: "replaces disabled flag", in: "gotypesalias=0", want: "gotypesalias=1"},
		{name: "preserves flag order", in: "gocachehash=1,gotypesalias=0,inittrace=1", want: "gocachehash=1,gotypesalias=1,inittrace=1"},
		{name: "leaves enabled flag", in: "gotypesalias=1", want: "gotypesalias=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureGoTypesAliasEnabled(tt.in); got != tt.want {
				t.Fatalf("ensureGoTypesAliasEnabled(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeReport(t *testing.T) {
	if got := normalizeReport("pkg\\foo.go: Example\r\npkg\\bar.go: Other"); got != "pkg/foo.go: Example\npkg/bar.go: Other\n" {
		t.Fatalf("normalizeReport() = %q", got)
	}
}

func TestCountFindings(t *testing.T) {
	if got := countFindings(""); got != 0 {
		t.Fatalf("countFindings(empty) = %d, want 0", got)
	}
	if got := countFindings("one\n\ntwo\n"); got != 3 {
		t.Fatalf("countFindings() = %d, want 3", got)
	}
}

func TestDeadcodecheckHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DEADCODECHECK_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("DEADCODECHECK_HELPER_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("DEADCODECHECK_HELPER_STDERR"))
	if os.Getenv("DEADCODECHECK_HELPER_FAIL") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func stubDeadcodecheckCommand(t *testing.T, report string, err error) func() {
	t.Helper()

	original := runDeadcodeCommand
	runDeadcodeCommand = func() (string, error) {
		return report, err
	}
	return func() {
		runDeadcodeCommand = original
	}
}

func restoreExecCommand(t *testing.T) {
	t.Helper()

	original := execCommand
	t.Cleanup(func() {
		execCommand = original
	})
}

func fakeDeadcodecheckCommand(name string, args ...string) *exec.Cmd {
	helperArgs := append([]string{"-test.run=TestDeadcodecheckHelperProcess", "--", name}, args...)
	cmd := exec.Command(os.Args[0], helperArgs...)
	cmd.Env = deadcodeEnv()
	return cmd
}

func writeDeadcodeBaseline(t *testing.T, root string, content string) {
	t.Helper()

	path := filepath.Join(root, baselinePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create baseline directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

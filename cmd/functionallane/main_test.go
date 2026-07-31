package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestMainExecutesFunctionalLane(t *testing.T) {
	original := executeFunctionalLane
	t.Cleanup(func() { executeFunctionalLane = original })
	called := false
	executeFunctionalLane = func() error { called = true; return nil }
	main()
	if !called {
		t.Fatal("main() did not execute the functional lane entrypoint")
	}
}

func TestMainRoutesCommandFailureThroughFailf(t *testing.T) {
	originalExecute, originalStderr, originalExit := executeFunctionalLane, stderrWriter, exitFunc
	t.Cleanup(func() { executeFunctionalLane, stderrWriter, exitFunc = originalExecute, originalStderr, originalExit })
	var stderr bytes.Buffer
	exitCode := 0
	executeFunctionalLane = func() error { return fmt.Errorf("functional lane failed") }
	stderrWriter = &stderr
	exitFunc = func(code int) { exitCode = code }
	main()
	if exitCode != 1 || stderr.String() != "functional lane failed\n" {
		t.Fatalf("main failure = exit %d stderr %q", exitCode, stderr.String())
	}
}

func TestParseConfigHonorsOverridesAndNormalizesJobs(t *testing.T) {
	restoreArgsAndFlags(t)
	os.Args = []string{"functionallane", "-count=3", "-jobs=0", "-root=./tests/functional/runtime_api/...", "-short=false", "-timeout=2m30s"}
	flag.CommandLine = flag.NewFlagSet("functionallane", flag.ContinueOnError)
	got := parseConfig()
	want := config{count: 3, jobs: 1, root: "./tests/functional/runtime_api/...", short: false, timeout: 150 * time.Second}
	if got != want {
		t.Fatalf("parseConfig() = %+v, want %+v", got, want)
	}
}

func TestParseConfigDefaults(t *testing.T) {
	restoreArgsAndFlags(t)
	os.Args = []string{"functionallane"}
	flag.CommandLine = flag.NewFlagSet("functionallane", flag.ContinueOnError)
	got := parseConfig()
	if got.count != 1 || got.jobs != 8 || got.root != "./tests/functional/..." || !got.short || got.timeout != 5*time.Minute {
		t.Fatalf("parseConfig() defaults = %+v", got)
	}
}

func TestRunFunctionalTestsUsesPackagePatternDirectly(t *testing.T) {
	restoreExecCommand(t)
	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName, gotArgs = name, append([]string(nil), args...)
		return fakeFunctionalLaneCommand(name, args...)
	}
	t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")
	cfg := config{count: 3, jobs: 8, root: "./tests/functional/...", short: true, timeout: 2 * time.Minute}
	if err := runFunctionalTests(cfg); err != nil {
		t.Fatalf("runFunctionalTests() error = %v", err)
	}
	want := []string{"test", "-p=8", "-short", "./tests/functional/...", "-count=3", "-timeout=2m0s"}
	if gotName != "go" || !slices.Equal(gotArgs, want) {
		t.Fatalf("command = %s %v, want go %v", gotName, gotArgs, want)
	}
}

func TestRunWrapsFunctionalTestExecutionFailures(t *testing.T) {
	restoreExecCommand(t)
	restoreArgsAndFlags(t)
	execCommand = fakeFunctionalLaneCommand
	os.Args = []string{"functionallane"}
	flag.CommandLine = flag.NewFlagSet("functionallane", flag.ContinueOnError)
	t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")
	t.Setenv("FUNCTIONALLANE_HELPER_TEST_FAIL", "1")
	err := run()
	if err == nil || err.Error() != "run functional lane: exit status 2" {
		t.Fatalf("run() error = %v", err)
	}
}

func TestFunctionallaneFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || os.Getenv("GO_WANT_FUNCTIONALLANE_HELPER") != "1" {
		return
	}
	if args[1] != "test" {
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
	if os.Getenv("FUNCTIONALLANE_HELPER_TEST_FAIL") == "1" {
		os.Exit(2)
	}
}

func fakeFunctionalLaneCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(err)
	}
	cmdArgs := append([]string{"-test.run=TestFunctionallaneFakeGoProcess", "--", name}, args...)
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

func restoreArgsAndFlags(t *testing.T) {
	t.Helper()
	originalArgs, originalFlagSet := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = originalArgs, originalFlagSet })
}

func restoreExecCommand(t *testing.T) {
	t.Helper()
	original := execCommand
	t.Cleanup(func() { execCommand = original })
}

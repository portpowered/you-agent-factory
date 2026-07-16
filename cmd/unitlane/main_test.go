package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDiscoverPackagesExcludesIndependentSuiteRoots(t *testing.T) {
	restoreExecCommand(t)
	execCommand = fakeUnitLaneCommand
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_LIST_STDOUT", strings.Join([]string{
		modulePath + "/pkg/factory",
		modulePath + "/tests/release",
		modulePath + "/tests/functional/runtime_api",
		modulePath + "/cmd/factory",
		modulePath + "/tests/stress",
		modulePath + "/tests/factory/scripts",
		modulePath + "/pkg/factory",
	}, "\r\n"))

	packages, err := discoverPackages("./...")
	if err != nil {
		t.Fatalf("discoverPackages() error = %v", err)
	}
	want := []string{
		modulePath + "/cmd/factory",
		modulePath + "/pkg/factory",
		modulePath + "/tests/factory/scripts",
	}
	if !slices.Equal(packages, want) {
		t.Fatalf("discoverPackages() = %v, want %v", packages, want)
	}
}

func TestDiscoverPackagesReportsListFailure(t *testing.T) {
	restoreExecCommand(t)
	execCommand = fakeUnitLaneCommand
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_LIST_FAIL", "1")
	t.Setenv("UNITLANE_HELPER_LIST_STDERR", "list failed")

	_, err := discoverPackages("./...")
	if err == nil || err.Error() != "exit status 2\nlist failed" {
		t.Fatalf("discoverPackages() error = %v, want list failure with stderr", err)
	}
}

func TestRunExecutesOnlyDiscoveredUnitPackages(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	execCommand = fakeUnitLaneCommand
	os.Args = []string{"unitlane", "-count=2", "-jobs=3", "-timeout=2m"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

	argsFile := t.TempDir() + "/go-test-args.txt"
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_LIST_STDOUT", strings.Join([]string{
		modulePath + "/pkg/factory",
		modulePath + "/tests/functional/runtime_api",
		modulePath + "/tests/stress",
		modulePath + "/tests/release",
	}, "\n"))
	t.Setenv("UNITLANE_HELPER_TEST_ARGS_FILE", argsFile)

	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	gotBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(gotBytes)), "\n")
	want := []string{"test", "-p=3", "-short", modulePath + "/pkg/factory", "-count=2", "-timeout=2m0s"}
	if !slices.Equal(got, want) {
		t.Fatalf("run() go test args = %v, want %v", got, want)
	}
}

func TestRunReportsTestFailure(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	execCommand = fakeUnitLaneCommand
	os.Args = []string{"unitlane"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_LIST_STDOUT", modulePath+"/pkg/factory")
	t.Setenv("UNITLANE_HELPER_TEST_FAIL", "1")

	err := run()
	if err == nil || err.Error() != "run unit lane: exit status 2" {
		t.Fatalf("run() error = %v, want wrapped test failure", err)
	}
}

func TestMainReportsFailureAndExits(t *testing.T) {
	originalExecute := executeUnitLane
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		executeUnitLane = originalExecute
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stderr bytes.Buffer
	var exitCode int
	executeUnitLane = func() error { return fmt.Errorf("unit lane failed") }
	stderrWriter = &stderr
	exitFunc = func(code int) { exitCode = code }

	main()
	if exitCode != 1 || stderr.String() != "unit lane failed\n" {
		t.Fatalf("main() exit = %d stderr = %q", exitCode, stderr.String())
	}
}

func TestUnitlaneFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || os.Getenv("GO_WANT_UNITLANE_HELPER") != "1" {
		return
	}

	switch args[1] {
	case "list":
		fmt.Fprint(os.Stdout, os.Getenv("UNITLANE_HELPER_LIST_STDOUT"))
		fmt.Fprint(os.Stderr, os.Getenv("UNITLANE_HELPER_LIST_STDERR"))
		if os.Getenv("UNITLANE_HELPER_LIST_FAIL") == "1" {
			os.Exit(2)
		}
		os.Exit(0)
	case "test":
		if path := os.Getenv("UNITLANE_HELPER_TEST_ARGS_FILE"); path != "" {
			if err := os.WriteFile(path, []byte(strings.Join(args[1:], "\n")), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		if os.Getenv("UNITLANE_HELPER_TEST_FAIL") == "1" {
			os.Exit(2)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func fakeUnitLaneCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(err)
	}
	cmdArgs := append([]string{"-test.run=TestUnitlaneFakeGoProcess", "--", name}, args...)
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
	t.Cleanup(func() { execCommand = original })
}

func restoreArgsFlagsAndCommand(t *testing.T) {
	t.Helper()
	originalArgs := os.Args
	originalFlags := flag.CommandLine
	restoreExecCommand(t)
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
	})
}

func TestParseConfigDefaultsToShortMode(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	os.Args = []string{"unitlane"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

	got := parseConfig()
	if got != (config{count: 1, jobs: 2, root: "./...", short: true, timeout: 5 * time.Minute}) {
		t.Fatalf("parseConfig() = %+v", got)
	}
}

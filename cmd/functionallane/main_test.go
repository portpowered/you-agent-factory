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

func TestMainExecutesFunctionalLane(t *testing.T) {
	original := executeFunctionalLane
	t.Cleanup(func() {
		executeFunctionalLane = original
	})

	called := false
	executeFunctionalLane = func() error {
		called = true
		return nil
	}

	main()

	if !called {
		t.Fatal("main() did not execute the functional lane entrypoint")
	}
}

func TestMainRoutesCommandFailureThroughFailf(t *testing.T) {
	originalExecute := executeFunctionalLane
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		executeFunctionalLane = originalExecute
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stderr bytes.Buffer
	var exitCode int

	executeFunctionalLane = func() error {
		return fmt.Errorf("functional lane failed")
	}
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stderr.String(); got != "functional lane failed\n" {
		t.Fatalf("main() stderr = %q, want %q", got, "functional lane failed\n")
	}
}

func TestFailfWritesFormattedErrorAndExits(t *testing.T) {
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stderr bytes.Buffer
	var exitCode int

	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	failf("failed package %s (%d)\n", "runtime_api", 2)

	if exitCode != 1 {
		t.Fatalf("failf() exit code = %d, want 1", exitCode)
	}
	if got := stderr.String(); got != "failed package runtime_api (2)\n" {
		t.Fatalf("failf() stderr = %q, want %q", got, "failed package runtime_api (2)\n")
	}
}

func TestDiscoverPackagesKeepsRunnablePackagesAndExcludesSupport(t *testing.T) {
	restoreExecCommand(t)

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return fakeFunctionalLaneCommand(name, args...)
	}

	t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")
	t.Setenv("FUNCTIONALLANE_HELPER_LIST_STDOUT", strings.Join([]string{
		"github.com/portpowered/infinite-you/tests/functional/runtime_api",
		"github.com/portpowered/infinite-you/tests/functional/internal/support",
		"",
		"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
	}, "\n"))

	pkgs, err := discoverPackages("./tests/functional/...")
	if err != nil {
		t.Fatalf("discoverPackages() error = %v", err)
	}

	if gotName != "go" {
		t.Fatalf("discoverPackages() command name = %q, want go", gotName)
	}
	if !slices.Equal(gotArgs, []string{"list", "./tests/functional/..."}) {
		t.Fatalf("discoverPackages() args = %v, want %v", gotArgs, []string{"list", "./tests/functional/..."})
	}

	want := []string{
		"github.com/portpowered/infinite-you/tests/functional/runtime_api",
		"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
	}
	if !slices.Equal(pkgs, want) {
		t.Fatalf("discoverPackages() packages = %v, want %v", pkgs, want)
	}
}

func TestRunFailsWhenNoRunnablePackagesRemain(t *testing.T) {
	restoreExecCommand(t)
	restoreArgsAndFlags(t)

	execCommand = fakeFunctionalLaneCommand
	os.Args = []string{"functionallane"}
	flag.CommandLine = flag.NewFlagSet("functionallane", flag.ContinueOnError)

	t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")
	t.Setenv("FUNCTIONALLANE_HELPER_LIST_STDOUT", strings.Join([]string{
		"github.com/portpowered/infinite-you/tests/functional/internal/support",
		"",
	}, "\n"))

	err := run()
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	want := "discover functional packages: no test packages found under ./tests/functional/..."
	if err.Error() != want {
		t.Fatalf("run() error = %q, want %q", err.Error(), want)
	}
}

func TestRunExecutesDiscoveredFunctionalPackagesWithParsedConfig(t *testing.T) {
	restoreExecCommand(t)
	restoreArgsAndFlags(t)

	execCommand = fakeFunctionalLaneCommand
	os.Args = []string{
		"functionallane",
		"-count=3",
		"-jobs=4",
		"-timeout=2m0s",
	}
	flag.CommandLine = flag.NewFlagSet("functionallane", flag.ContinueOnError)

	testArgsFile := t.TempDir() + "/go-test-args.txt"
	t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")
	t.Setenv("FUNCTIONALLANE_HELPER_LIST_STDOUT", strings.Join([]string{
		"github.com/portpowered/infinite-you/tests/functional/runtime_api",
		"github.com/portpowered/infinite-you/tests/functional/internal/support",
		"",
		"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
	}, "\n"))
	t.Setenv("FUNCTIONALLANE_HELPER_TEST_ARGS_FILE", testArgsFile)

	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	gotBytes, err := os.ReadFile(testArgsFile)
	if err != nil {
		t.Fatalf("read captured go test args: %v", err)
	}

	gotArgs := strings.Split(strings.TrimSpace(string(gotBytes)), "\n")
	wantArgs := []string{
		"test",
		"-p=4",
		"-short",
		"github.com/portpowered/infinite-you/tests/functional/runtime_api",
		"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
		"-count=3",
		"-timeout=2m0s",
	}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("run() go test args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestRunFunctionalTestsBuildsGoTestInvocation(t *testing.T) {
	cases := []struct {
		name string
		cfg  config
		want []string
	}{
		{
			name: "short enabled",
			cfg: config{
				count:   3,
				jobs:    4,
				short:   true,
				timeout: 2 * time.Minute,
			},
			want: []string{
				"test",
				"-p=4",
				"-short",
				"github.com/portpowered/infinite-you/tests/functional/runtime_api",
				"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
				"-count=3",
				"-timeout=2m0s",
			},
		},
		{
			name: "short disabled",
			cfg: config{
				count:   1,
				jobs:    2,
				short:   false,
				timeout: 5 * time.Minute,
			},
			want: []string{
				"test",
				"-p=2",
				"github.com/portpowered/infinite-you/tests/functional/runtime_api",
				"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
				"-count=1",
				"-timeout=5m0s",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restoreExecCommand(t)

			var gotName string
			var gotArgs []string
			execCommand = func(name string, args ...string) *exec.Cmd {
				gotName = name
				gotArgs = append([]string(nil), args...)
				return fakeFunctionalLaneCommand(name, args...)
			}

			t.Setenv("GO_WANT_FUNCTIONALLANE_HELPER", "1")

			pkgs := []string{
				"github.com/portpowered/infinite-you/tests/functional/runtime_api",
				"github.com/portpowered/infinite-you/tests/functional/bootstrap_portability",
			}
			if err := runFunctionalTests(tc.cfg, pkgs); err != nil {
				t.Fatalf("runFunctionalTests() error = %v", err)
			}

			if gotName != "go" {
				t.Fatalf("runFunctionalTests() command name = %q, want go", gotName)
			}
			if !slices.Equal(gotArgs, tc.want) {
				t.Fatalf("runFunctionalTests() args = %v, want %v", gotArgs, tc.want)
			}
		})
	}
}

func TestFunctionallaneFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" || os.Getenv("GO_WANT_FUNCTIONALLANE_HELPER") != "1" {
		return
	}

	switch {
	case len(args) == 3 && args[1] == "list":
		fmt.Fprint(os.Stdout, os.Getenv("FUNCTIONALLANE_HELPER_LIST_STDOUT"))
		fmt.Fprint(os.Stderr, os.Getenv("FUNCTIONALLANE_HELPER_LIST_STDERR"))
		exitCode := 0
		if os.Getenv("FUNCTIONALLANE_HELPER_LIST_FAIL") == "1" {
			exitCode = 2
		}
		os.Exit(exitCode)
	case len(args) >= 2 && args[1] == "test":
		if path := os.Getenv("FUNCTIONALLANE_HELPER_TEST_ARGS_FILE"); path != "" {
			if err := os.WriteFile(path, []byte(strings.Join(args[1:], "\n")), 0o600); err != nil {
				fmt.Fprintf(os.Stderr, "write fake go test args: %v", err)
				os.Exit(2)
			}
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func fakeFunctionalLaneCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
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

	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
	})
}

func restoreExecCommand(t *testing.T) {
	t.Helper()

	original := execCommand
	t.Cleanup(func() {
		execCommand = original
	})
}

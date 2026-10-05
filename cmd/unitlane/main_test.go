package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

func TestDiscoverPackagesExcludesIndependentSuiteRoots(t *testing.T) {
	root := t.TempDir()
	writeTestPackageFile(t, root, "factory")
	writeTestPackageFile(t, root, "wire")
	writeTestPackageFile(t, root, "wire/internal/managedbackend")
	writeTestPackageFile(t, root, "services/models/wire")
	writeTestPackageFile(t, root, "services/factory_definitions/internal/services/compilation/runtimetests")
	writeTestPackageFile(t, root, "transports/http/contracttests")
	writeTestPackageFile(t, root, "transports/http/servertests/factorysessionsse")
	writeTestPackageFile(t, root, "services/factory_runtime/internal/exhaustiontests")
	writeTestPackageFile(t, root, "services/workers/provider/functionaltests")
	writeTestPackageFile(t, root, "ignored/testdata/nested")

	packages, err := discoverPackagesUnderMatching(root, modulePath+"/pkg", testlanes.IsUnitPackage)
	if err != nil {
		t.Fatalf("discoverPackagesUnder() error = %v", err)
	}
	want := []string{
		modulePath + "/pkg/factory",
	}
	if !slices.Equal(packages, want) {
		t.Fatalf("discoverPackages() = %v, want %v", packages, want)
	}
}

func TestWiringIntegrationDiscoveryRetainsRootAndServiceWiring(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, pkg := range []string{"wire", "wire/internal/backend", "services/models/wire", "services/models/wire/nested", "services/work", "services/models/integrationtests", "wire/testdata/hidden"} {
		writeTestPackageFile(t, root, pkg)
	}
	packages, err := discoverPackagesUnderMatching(root, modulePath+"/pkg", isWiringIntegrationPackage)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{modulePath + "/pkg/services/models/wire", modulePath + "/pkg/services/models/wire/nested", modulePath + "/pkg/wire", modulePath + "/pkg/wire/internal/backend"}
	if !slices.Equal(packages, want) {
		t.Fatalf("wiring integrations = %v, want %v", packages, want)
	}
}

func TestDiscoverPackagesExcludesBuildConstrainedTestFiles(t *testing.T) {
	root := t.TempDir()
	writeTestPackageFile(t, root, "portable")
	constrained := filepath.Join(root, "constrained", "build_constrained_test.go")
	if err := os.MkdirAll(filepath.Dir(constrained), 0o755); err != nil {
		t.Fatalf("create constrained package: %v", err)
	}
	if err := os.WriteFile(constrained, []byte("//go:build unitlane_never_enabled\n\npackage constrained\n"), 0o600); err != nil {
		t.Fatalf("write constrained test: %v", err)
	}

	packages, err := discoverPackagesUnderMatching(root, modulePath+"/pkg", testlanes.IsUnitPackage)
	if err != nil {
		t.Fatalf("discoverPackagesUnder() error = %v", err)
	}
	want := []string{modulePath + "/pkg/portable"}
	if !slices.Equal(packages, want) {
		t.Fatalf("discoverPackages() = %v, want %v", packages, want)
	}
}

func TestDiscoverPackagesReportsListFailure(t *testing.T) {
	_, err := discoverPackagesUnderMatching(filepath.Join(t.TempDir(), "missing"), modulePath+"/pkg", testlanes.IsUnitPackage)
	if err == nil {
		t.Fatal("discoverPackagesUnder() error = nil, want missing-root failure")
	}
}

func TestRunExecutesOnlyDiscoveredUnitPackages(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	execCommand = fakeUnitLaneCommand
	discoverUnitPackages = func(string) ([]string, error) {
		return []string{modulePath + "/pkg/factory"}, nil
	}
	os.Args = []string{"unitlane", "-count=2", "-jobs=3", "-timeout=2m"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

	argsFile := t.TempDir() + "/go-test-args.txt"
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_TEST_ARGS_FILE", argsFile)

	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	gotBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(gotBytes)), "\n")
	want := []string{"test", "-p=3", "-json", "-vet=off", "-short", "./pkg/factory", "-count=2", "-timeout=2m0s"}
	if !slices.Equal(got, want) {
		t.Fatalf("run() go test args = %v, want %v", got, want)
	}
}

func TestLocalPackageArgumentsShortensRepositoryImports(t *testing.T) {
	t.Parallel()

	got := localPackageArguments([]string{
		modulePath + "/pkg/services/work",
		"example.com/external/package",
	})
	want := []string{
		"./pkg/services/work",
		"example.com/external/package",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("localPackageArguments() = %v, want %v", got, want)
	}
}

func TestRunUnitTestsBatchesByExecutedArguments(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	stdoutWriter = &bytes.Buffer{}
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_TIMING_JSON", "1")
	cfg := config{jobs: 2, short: true, count: 1, timeout: time.Minute}
	for _, test := range []struct {
		name     string
		packages int
		batches  int
	}{
		{name: "shortened imports fit together", packages: 450, batches: 1},
		{name: "oversized arguments still split", packages: 800, batches: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			packages := make([]string, test.packages)
			for index := range packages {
				packages[index] = fmt.Sprintf("%s/pkg/services/owner%03d/internal/service", modulePath, index)
			}
			var commands [][]string
			execCommand = func(name string, args ...string) *exec.Cmd {
				commands = append(commands, slices.Clone(args))
				return fakeUnitLaneCommand(name, args...)
			}
			if err := runUnitTests(cfg, packages); err != nil {
				t.Fatalf("runUnitTests() error = %v", err)
			}
			if len(commands) != test.batches {
				t.Fatalf("go test commands = %d, want %d", len(commands), test.batches)
			}
			var executed []string
			for _, args := range commands {
				if commandArgLen(args) > maxGoTestCommandLen {
					t.Fatalf("go test argument length = %d, exceeds %d", commandArgLen(args), maxGoTestCommandLen)
				}
				executed = append(executed, helperUnitLanePackages(args)...)
			}
			if !slices.Equal(executed, packages) {
				t.Fatal("go test batches must execute every package exactly once in order")
			}
		})
	}
}

func TestRunReportsTestFailure(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	execCommand = fakeUnitLaneCommand
	discoverUnitPackages = func(string) ([]string, error) {
		return []string{modulePath + "/pkg/factory"}, nil
	}
	os.Args = []string{"unitlane"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
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
	case "test":
		if path := os.Getenv("UNITLANE_HELPER_TEST_ARGS_FILE"); path != "" {
			if err := os.WriteFile(path, []byte(strings.Join(args[1:], "\n")), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		if os.Getenv("UNITLANE_HELPER_TIMING_JSON") == "1" {
			for _, packageName := range helperUnitLanePackages(args[2:]) {
				if os.Getenv("UNITLANE_HELPER_TEST_FAIL") == "1" {
					writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "output", Package: packageName, Output: "--- FAIL: TestFailure\n"})
					writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "fail", Package: packageName, Elapsed: 0.4})
					continue
				}
				writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "pass", Package: packageName, Test: "TestHelper", Elapsed: 0.1})
				if os.Getenv("UNITLANE_HELPER_TIMING_CACHED") == "1" {
					writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "output", Package: packageName, Output: "ok  \t" + packageName + "\t(cached)\n"})
				} else {
					writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "output", Package: packageName, Output: "ok  \t" + packageName + "\t0.500s\n"})
				}
				writeUnitLaneHelperEvent(goTestUnitTimingEvent{Action: "pass", Package: packageName, Elapsed: 0.5})
			}
			if os.Getenv("UNITLANE_HELPER_TEST_FAIL") == "1" {
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

func helperUnitLanePackages(args []string) []string {
	var packages []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "./") {
			continue
		}
		packages = append(packages, modulePath+strings.TrimPrefix(arg, "."))
	}
	return packages
}

func writeUnitLaneHelperEvent(event goTestUnitTimingEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, string(data))
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
	originalDiscover := discoverUnitPackages
	originalStdout := stdoutWriter
	restoreExecCommand(t)
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
		discoverUnitPackages = originalDiscover
		stdoutWriter = originalStdout
	})
}

func writeTestPackageFile(t *testing.T, root, relativeDir string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relativeDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create test package directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package_test.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatalf("write test package file: %v", err)
	}
}

func TestParseConfigDefaultsToShortMode(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	t.Setenv(logicalCPUsEnv, "24")
	t.Setenv(expectedConcurrentLanesEnv, "4")
	os.Args = []string{"unitlane"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

	got := parseConfig()
	if got != (config{count: 0, jobs: 6, root: "./pkg/...", short: true, timeout: 5 * time.Minute, vet: false, timingOutput: ""}) {
		t.Fatalf("parseConfig() = %+v, expected count: %v, jobs: %v", got, 0, 6)
	}
}

func TestBoundedUnitLaneJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		logicalCPUs   int
		expectedLanes string
		want          int
	}{
		{name: "24 CPUs divided across four lanes", logicalCPUs: 24, expectedLanes: "4", want: 6},
		{name: "four CPUs keep the floor", logicalCPUs: 4, expectedLanes: "4", want: 2},
		{name: "CPU detection failure uses the floor", logicalCPUs: 0, expectedLanes: "4", want: 2},
		{name: "invalid divisor uses the floor", logicalCPUs: 24, expectedLanes: "invalid", want: 2},
		{name: "zero divisor uses the floor", logicalCPUs: 24, expectedLanes: "0", want: 2},
		{name: "valid divisor override wins", logicalCPUs: 24, expectedLanes: "6", want: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := boundedUnitLaneJobs(test.logicalCPUs, test.expectedLanes); got != test.want {
				t.Fatalf("boundedUnitLaneJobs(%d, %q) = %d, want %d", test.logicalCPUs, test.expectedLanes, got, test.want)
			}
		})
	}
}

func TestUnitLaneDefaultReadsControlledHostInputs(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	t.Setenv(logicalCPUsEnv, "24")
	t.Setenv(expectedConcurrentLanesEnv, "4")

	if got := defaultUnitLaneJobs(); got != 6 {
		t.Fatalf("defaultUnitLaneJobs() = %d, want 6", got)
	}
}

func TestUnitLaneDefaultUsesFourLaneDivisorWhenUnset(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	t.Setenv(logicalCPUsEnv, "24")
	original, wasSet := os.LookupEnv(expectedConcurrentLanesEnv)
	if err := os.Unsetenv(expectedConcurrentLanesEnv); err != nil {
		t.Fatalf("unset %s: %v", expectedConcurrentLanesEnv, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(expectedConcurrentLanesEnv, original)
			return
		}
		_ = os.Unsetenv(expectedConcurrentLanesEnv)
	})

	if got := defaultUnitLaneJobs(); got != 6 {
		t.Fatalf("defaultUnitLaneJobs() with unset divisor = %d, want 6", got)
	}
}

func TestParseConfigExplicitJobsRemainAuthoritative(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "positive override", args: []string{"unitlane", "-jobs=9"}, want: 9},
		{name: "existing minimum clamp", args: []string{"unitlane", "-jobs=0"}, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restoreArgsFlagsAndCommand(t)
			t.Setenv(logicalCPUsEnv, "24")
			t.Setenv(expectedConcurrentLanesEnv, "4")
			os.Args = test.args
			flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

			if got := parseConfig().jobs; got != test.want {
				t.Fatalf("parseConfig().jobs = %d, want %d", got, test.want)
			}
		})
	}
}

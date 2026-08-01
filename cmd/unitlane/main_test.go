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
)

func TestDiscoverPackagesExcludesIndependentSuiteRoots(t *testing.T) {
	root := t.TempDir()
	writeTestPackageFile(t, root, "factory")
	writeTestPackageFile(t, root, "services/factory_definitions/internal/services/compilation/runtimetests")
	writeTestPackageFile(t, root, "transports/http/contracttests")
	writeTestPackageFile(t, root, "transports/http/servertests/factorysessionsse")
	writeTestPackageFile(t, root, "services/factory_runtime/internal/exhaustiontests")
	writeTestPackageFile(t, root, "services/workers/provider/functionaltests")
	writeTestPackageFile(t, root, "ignored/testdata/nested")

	packages, err := discoverPackagesUnder(root, modulePath+"/pkg")
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

func TestDiscoverPackagesReportsListFailure(t *testing.T) {
	_, err := discoverPackagesUnder(filepath.Join(t.TempDir(), "missing"), modulePath+"/pkg")
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
	want := []string{"test", "-p=3", "-vet=off", "-short", "./pkg/factory", "-count=2", "-timeout=2m0s"}
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

func TestDiagnosticRunWritesReportAndPreservesFailure(t *testing.T) {
	restoreArgsFlagsAndCommand(t)
	execCommand = fakeUnitLaneCommand

	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutWriter = &stdout
	stderrWriter = &stderr
	t.Cleanup(func() {
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_STDOUT", strings.Join([]string{
		`{"Action":"start","Package":"pkg/slow"}`,
		`{"Action":"run","Package":"pkg/slow","Test":"TestSlow"}`,
		`{"Action":"pass","Package":"pkg/slow","Test":"TestSlow","Elapsed":0.25}`,
		`{"Action":"output","Package":"pkg/slow","Output":"ok slow\n"}`,
		`{"Action":"pass","Package":"pkg/slow","Elapsed":2.5}`,
		`{"Action":"pass","Package":"pkg/cached","Elapsed":0,"Cached":true}`,
	}, "\n")+"\n")
	t.Setenv("UNITLANE_HELPER_TEST_FAIL", "1")

	reportPath := filepath.Join(t.TempDir(), "reports", "unit-lane.json")
	err := runUnitTests(config{
		count:      1,
		diagnostic: true,
		jobs:       3,
		reportPath: reportPath,
		root:       "./pkg/...",
		short:      true,
		timeout:    2 * time.Minute,
	}, []string{modulePath + "/pkg/slow"})
	if err == nil || !strings.Contains(err.Error(), "exit status 2") {
		t.Fatalf("runUnitTests() error = %v, want preserved child failure", err)
	}
	if stdout.String() != "ok slow\n" {
		t.Fatalf("diagnostic stdout = %q, want readable replayed output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unit lane diagnostics: outcome=fail") || !strings.Contains(stderr.String(), "pkg/slow") {
		t.Fatalf("diagnostic stderr = %q, want failure summary", stderr.String())
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read diagnostic report: %v", err)
	}
	var report unitLaneReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode diagnostic report: %v", err)
	}
	if report.Outcome != "fail" || report.CacheMode != "fresh" || report.EffectiveJobs != 3 {
		t.Fatalf("report metadata = %+v, want failed fresh run with three jobs", report)
	}
	if len(report.Packages) != 2 || report.Packages[0].Package != "pkg/cached" || report.Packages[1].TestCount != 1 {
		t.Fatalf("report packages = %+v, want sorted package timings and one test", report.Packages)
	}
	if !report.Packages[0].Cached || !report.Packages[0].CacheObserved {
		t.Fatalf("cached package = %+v, want observed cached result", report.Packages[0])
	}
}

func TestDiagnosticBaseArgumentsRequestJSONEvents(t *testing.T) {
	got := baseGoTestArgs(config{diagnostic: true, jobs: 4, short: true, vet: false})
	want := []string{"test", "-p=4", "-json", "-vet=off", "-short"}
	if !slices.Equal(got, want) {
		t.Fatalf("baseGoTestArgs() = %v, want %v", got, want)
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
		if output := os.Getenv("UNITLANE_HELPER_STDOUT"); output != "" {
			fmt.Fprint(os.Stdout, output)
		}
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
	originalDiscover := discoverUnitPackages
	restoreExecCommand(t)
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
		discoverUnitPackages = originalDiscover
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
	os.Args = []string{"unitlane"}
	flag.CommandLine = flag.NewFlagSet("unitlane", flag.ContinueOnError)

	got := parseConfig()
	if got != (config{count: 0, diagnostic: false, jobs: 32, reportPath: "", root: "./pkg/...", short: true, timeout: 5 * time.Minute, vet: false}) {
		t.Fatalf("parseConfig() = %+v, expected count: %v, jobs: %v", got, 0, 32)
	}
}

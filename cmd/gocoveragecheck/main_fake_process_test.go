package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoCoverageCheckFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
				modulePath+"/pkg/generatedclient\t\tcoverage: 0.0% of statements\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessWithOKSummary(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout,
			"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements\n"+
				"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessPassing(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config/config.go:1.1,2.1\t75.0%\n"+
				modulePath+"/pkg/service/factory.go:1.1,2.1\t100.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessWithCoverpkgOKSummary(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout,
			"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/config, "+modulePath+"/pkg/service, "+modulePath+"/pkg/generatedclient\n"+
				"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements in "+modulePath+"/pkg/generatedclient, "+modulePath+"/pkg/service\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessWithTempProfileReport(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			"",
		}, "\n"))
		fmt.Fprintf(os.Stdout, "TEMP_PROFILE=%s\n", profilePath)
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config/config.go:1.1,2.1\t75.0%\n"+
				modulePath+"/pkg/service/factory.go:1.1,2.1\t100.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithStderr(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stderr, "stderr detail from cover tool")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithStdout(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout, "stdout detail from cover tool")
		os.Exit(4)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessTestFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	if len(args) >= 2 && args[1] == "test" {
		os.Exit(7)
	}

	fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
	os.Exit(2)
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := helperCoverProfilePath(args[2:])
		writeFakeCoverageProfileOrExit(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n"))
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		os.Exit(8)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoListCommandFailsWithStderr(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprint(os.Stderr, "stderr detail from go list")
	os.Exit(5)
}

func TestGoListCommandFailsWithStdout(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprint(os.Stdout, "stdout detail from go list")
	os.Exit(6)
}

func TestGoListCommandFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	os.Exit(9)
}

func TestGoListCommandWithExcludedPackagesOnly(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/generatedclient")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/testutil/runtimefixtures")
	os.Exit(0)
}

func TestGoListCommandWithCoverageButNoTestPackages(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	if slices.Contains(args, "./tests/functional/...") {
		fmt.Fprintln(os.Stdout, modulePath+"/tests/functional/internal/support")
		os.Exit(0)
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config")
	os.Exit(0)
}

func TestGoListCommandWithDuplicatesAndExcludedPackages(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config\t2")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config\t2")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config/exhaustiontests\t0")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/generatedclient\t4")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/testutil/runtimefixtures\t1")
	fmt.Fprintln(os.Stdout, modulePath+"/cmd/factory\t1")
	os.Exit(0)
}

func writeCoverageProfile(t *testing.T, contents string) string {
	t.Helper()

	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}
	return profilePath
}

func helperCommandArgs(argv []string) ([]string, bool) {
	for index, arg := range argv {
		if arg == "--" {
			return argv[index+1:], true
		}
	}
	return nil, false
}

func parseTempProfilePath(t *testing.T, output string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "TEMP_PROFILE=") {
			return strings.TrimPrefix(line, "TEMP_PROFILE=")
		}
	}
	t.Fatalf("TEMP_PROFILE marker missing from output %q", output)
	return ""
}

func helperCoverProfilePath(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-coverprofile=") {
			return strings.TrimPrefix(arg, "-coverprofile=")
		}
	}
	fmt.Fprint(os.Stderr, "missing coverprofile argument")
	os.Exit(2)
	return ""
}

func writeFakeCoverageProfileOrExit(profilePath string, profile string) {
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
		os.Exit(2)
	}
}

func fakeGoCoverageCommand(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcess", name, args...)
}

func fakeGoCoverageCommandPassing(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessPassing", name, args...)
}

func fakeGoCoverageCommandWithOKSummary(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessWithOKSummary", name, args...)
}

func fakeGoCoverageCommandWithCoverpkgOKSummary(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessWithCoverpkgOKSummary", name, args...)
}

func fakeGoCoverageCommandWithTempProfileReport(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessWithTempProfileReport", name, args...)
}

func fakeGoCoverageCommandCoverFailsWithStderr(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessCoverFailsWithStderr", name, args...)
}

func fakeGoCoverageCommandCoverFailsWithStdout(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessCoverFailsWithStdout", name, args...)
}

func fakeGoCoverageCommandTestFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessTestFailsWithoutDetail", name, args...)
}

func fakeGoCoverageCommandCoverFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoCoverageCheckFakeGoProcessCoverFailsWithoutDetail", name, args...)
}

func fakeGoListCommandFailsWithStderr(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandFailsWithStderr", name, args...)
}

func fakeGoListCommandFailsWithStdout(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandFailsWithStdout", name, args...)
}

func fakeGoListCommandFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandFailsWithoutDetail", name, args...)
}

func fakeGoListCommandWithExcludedPackagesOnly(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandWithExcludedPackagesOnly", name, args...)
}

func fakeGoListCommandWithCoverageButNoTestPackages(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandWithCoverageButNoTestPackages", name, args...)
}

func fakeGoListCommandWithDuplicatesAndExcludedPackages(name string, args ...string) *exec.Cmd {
	return helperFakeCommand("TestGoListCommandWithDuplicatesAndExcludedPackages", name, args...)
}

func helperFakeCommand(testName string, name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=" + testName, "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

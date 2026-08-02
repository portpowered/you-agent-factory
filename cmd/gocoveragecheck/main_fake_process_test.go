package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tempProfileMarkerFilename = "gocoveragecheck-last-temp-profile.txt"

func fakeGoCoverageCommand(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-default", invocation.name, invocation.args...)
}

func fakeGoCoverageCommandPassing(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-passing", invocation.name, invocation.args...)
}

func fakeGoCoverageCommandWithOKSummary(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-with-ok-summary", invocation.name, invocation.args...)
}

func fakeGoCoverageCommandWithCoverpkgOKSummary(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-with-coverpkg-ok-summary", invocation.name, invocation.args...)
}

func fakeGoCoverageCommandWithTempProfileReport(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-temp-profile", invocation.name, invocation.args...)
}

func fakeGoCoverageCommandTestFailsWithoutDetail(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("coverage-test-fails-without-detail", invocation.name, invocation.args...)
}

func fakeGoListCommandFailsWithStderr(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-fails-with-stderr", invocation.name, invocation.args...)
}

func fakeGoListCommandFailsWithStdout(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-fails-with-stdout", invocation.name, invocation.args...)
}

func fakeGoListCommandFailsWithoutDetail(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-fails-without-detail", invocation.name, invocation.args...)
}

func fakeGoListCommandWithExcludedPackagesOnly(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-excluded-packages-only", invocation.name, invocation.args...)
}

func fakeGoListCommandWithCoverageButNoTestPackages(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-coverage-no-test-packages", invocation.name, invocation.args...)
}

func fakeGoListCommandWithDuplicatesAndExcludedPackages(invocation commandInvocation) (string, string, error) {
	return fakeGoCommandByScenario("go-list-duplicates", invocation.name, invocation.args...)
}

func fakeGoCommandByScenario(scenario string, name string, args ...string) (string, string, error) {
	if name != "go" {
		return "", "", fmt.Errorf("unexpected command %q", name)
	}
	if len(args) == 0 {
		return "", "", fmt.Errorf("missing go command args")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "test":
		return fakeGoTestScenario(scenario, subArgs)
	case "tool":
		return fakeGoToolScenario(scenario, subArgs)
	case "list":
		return fakeGoListScenario(scenario, subArgs)
	default:
		return "", "", fmt.Errorf("unexpected go subcommand %q", subcommand)
	}
}

func fakeGoTestScenario(scenario string, args []string) (string, string, error) {
	if scenario == "go-list-fails-with-stderr" || scenario == "go-list-fails-with-stdout" || scenario == "go-list-fails-without-detail" ||
		scenario == "go-list-excluded-packages-only" || scenario == "go-list-coverage-no-test-packages" || scenario == "go-list-duplicates" {
		return "", "", fmt.Errorf("scenario %q cannot run go test", scenario)
	}

	if helperHasArgPrefix(args, "-coverpkg=") {
		profilePath := helperCoverProfilePath(args)
		if profilePath == "" {
			return "", "", fmt.Errorf("missing -coverprofile")
		}
		switch scenario {
		case "coverage-passing", "coverage-cover-fails-with-stderr", "coverage-cover-fails-with-stdout", "coverage-cover-fails-without-detail", "coverage-temp-profile", "coverage-test-fails-without-detail":
			if err := writeFakeCoverageProfile(profilePath, strings.Join([]string{
				"mode: count",
				modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
				modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
				"",
			}, "\n")); err != nil {
				return "", "", err
			}
		case "coverage-with-coverpkg-ok-summary", "coverage-with-ok-summary", "coverage-default":
			if err := writeFakeCoverageProfile(profilePath, strings.Join([]string{
				"mode: count",
				modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
				modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
				"",
			}, "\n")); err != nil {
				return "", "", err
			}
		default:
			return "", "", fmt.Errorf("unexpected cover scenario: %s", scenario)
		}

		switch scenario {
		case "coverage-passing":
			return modulePath + "/pkg/config\t\tcoverage: 75.0% of statements\n", "", nil
		case "coverage-temp-profile":
			if err := writeTempProfileMarkerOrErr(profilePath); err != nil {
				return "", "", err
			}
			return modulePath + "/pkg/config\t\tcoverage: 75.0% of statements\n", "", nil
		case "coverage-with-coverpkg-ok-summary":
			return modulePath + "/pkg/config\tok  \t0.0% of statements in " + modulePath + "/pkg/config, " + modulePath + "/pkg/service, " + modulePath + "/pkg/transports/http/client\n" +
				modulePath + "/pkg/transports/http/client\tok  \t0.0% of statements in " + modulePath + "/pkg/transports/http/client, " + modulePath + "/pkg/service\n", "", nil
		case "coverage-with-ok-summary":
			return "ok  " + modulePath + "/pkg/config\t0.123s\tcoverage: 0.0% of statements in " + modulePath + "/pkg/config, " + modulePath + "/pkg/service, " + modulePath + "/pkg/transports/http/client\n" +
				"ok  " + modulePath + "/pkg/transports/http/client\t(cached)\tcoverage: 0.0% of statements in " + modulePath + "/pkg/transports/http/client, " + modulePath + "/pkg/service\n", "", nil
		case "coverage-default", "coverage-cover-fails-with-stderr", "coverage-cover-fails-with-stdout", "coverage-cover-fails-without-detail":
			return modulePath + "/pkg/config\t\tcoverage: 0.0% of statements\n" +
				modulePath + "/pkg/transports/http/client\t\tcoverage: 0.0% of statements\n", "", nil
		case "coverage-test-fails-without-detail":
			return "", "raw failure output from go test", fmt.Errorf("exit status 7")
		default:
			return "", "", fmt.Errorf("unexpected cover scenario: %s", scenario)
		}
	}

	switch scenario {
	case "coverage-default", "coverage-with-coverpkg-ok-summary", "coverage-with-ok-summary":
		return modulePath + "/pkg/config\t\tcoverage: 0.0% of statements\n" +
			modulePath + "/pkg/transports/http/client\t\tcoverage: 0.0% of statements\n", "", nil
	case "coverage-cover-fails-with-stderr", "coverage-cover-fails-with-stdout", "coverage-cover-fails-without-detail":
		return modulePath + "/pkg/config\t\tcoverage: 0.0% of statements\n" +
			modulePath + "/pkg/service\t\tcoverage: 100.0% of statements\n", "", nil
	case "coverage-test-fails-without-detail":
		return "", "raw failure output from go test", fmt.Errorf("exit status 7")
	}

	switch scenario {
	case "coverage-passing", "coverage-cover-fails-with-stderr", "coverage-cover-fails-with-stdout", "coverage-cover-fails-without-detail", "coverage-temp-profile":
		return modulePath + "/pkg/config\t\tcoverage: 100.0% of statements\n" +
			modulePath + "/pkg/service\t\tcoverage: 100.0% of statements\n", "", nil
	case "coverage-test-fails-without-detail":
		return "", "raw failure output from go test", fmt.Errorf("exit status 7")
	default:
		return "", "", fmt.Errorf("unexpected go test scenario: %s", scenario)
	}
}

func fakeGoToolScenario(scenario string, args []string) (string, string, error) {
	if len(args) < 2 || args[0] != "cover" || args[1] != "-func" {
		return "", "", fmt.Errorf("unexpected go tool call: %v", args)
	}

	switch scenario {
	case "coverage-cover-fails-with-stderr":
		return "", "stderr detail from cover tool", fmt.Errorf("exit status 3")
	case "coverage-cover-fails-with-stdout":
		return "stdout detail from cover tool", "", fmt.Errorf("exit status 4")
	case "coverage-cover-fails-without-detail":
		return "", "", fmt.Errorf("exit status 8")
	case "coverage-default", "coverage-passing", "coverage-with-ok-summary", "coverage-with-coverpkg-ok-summary", "coverage-temp-profile":
		return modulePath + "/pkg/config/config.go:1.1,2.1\t75.0%\n" +
			modulePath + "/pkg/service/factory.go:1.1,2.1\t100.0%\n" +
			"total: (statements) 82.5%\n", "", nil
	default:
		return "", "", fmt.Errorf("unexpected go tool scenario: %s", scenario)
	}
}

func fakeGoListScenario(scenario string, args []string) (string, string, error) {
	if scenario == "go-list-fails-with-stderr" {
		return "", "stderr detail from go list", fmt.Errorf("exit status 5")
	}
	if scenario == "go-list-fails-with-stdout" {
		return "stdout detail from go list", "", fmt.Errorf("exit status 6")
	}
	if scenario == "go-list-fails-without-detail" {
		return "", "", fmt.Errorf("exit status 9")
	}
	if scenario == "go-list-excluded-packages-only" {
		return modulePath + "/pkg/transports/http/client\n" + modulePath + "/internal/testutil/runtimefixtures\n", "", nil
	}
	if scenario == "go-list-coverage-no-test-packages" {
		if slicesContains(args, "./tests/functional/...") {
			return modulePath + "/tests/functional/internal/support\n", "", nil
		}
		return modulePath + "/pkg/config\n", "", nil
	}
	if scenario == "go-list-duplicates" {
		return modulePath + "/pkg/config\t2\n" +
			modulePath + "/pkg/config\t2\n" +
			modulePath + "/pkg/config/exhaustiontests\t0\n" +
			modulePath + "/pkg/transports/http/client\t4\n" +
			modulePath + "/internal/testutil/runtimefixtures\t1\n" +
			modulePath + "/cmd/factory\t1\n", "", nil
	}

	return "", "", fmt.Errorf("unexpected go list scenario: %s", scenario)
}

func writeCoverageProfile(t *testing.T, contents string) string {
	t.Helper()

	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}
	return profilePath
}

func helperCoverProfilePath(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-coverprofile=") {
			return strings.TrimPrefix(arg, "-coverprofile=")
		}
	}
	return ""
}

func helperHasArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func writeTempProfileMarkerOrErr(profilePath string) error {
	markerPath := filepath.Join(os.TempDir(), tempProfileMarkerFilename)
	if err := os.WriteFile(markerPath, []byte(profilePath), 0o600); err != nil {
		return fmt.Errorf("write temp profile marker: %w", err)
	}
	return nil
}

func writeFakeCoverageProfile(profilePath string, profile string) error {
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		return fmt.Errorf("write fake profile: %w", err)
	}
	return nil
}

func slicesContains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

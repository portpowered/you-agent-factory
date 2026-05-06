package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIsBackendCoveragePackage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{name: "factory command", importPath: modulePath + "/cmd/factory", want: true},
		{name: "backend package", importPath: modulePath + "/pkg/config", want: true},
		{name: "generated api package", importPath: modulePath + "/pkg/api/generated", want: false},
		{name: "generated client package", importPath: modulePath + "/pkg/generatedclient", want: false},
		{name: "test helper package", importPath: modulePath + "/pkg/testutil/runtimefixtures", want: false},
		{name: "functional test package", importPath: modulePath + "/tests/functional/runtime_api", want: false},
		{name: "ui package", importPath: modulePath + "/ui", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isBackendCoveragePackage(tc.importPath); got != tc.want {
				t.Fatalf("isBackendCoveragePackage(%q) = %t, want %t", tc.importPath, got, tc.want)
			}
		})
	}
}

func TestResolveCoverageLaneDefaults(t *testing.T) {
	coverPackages, testPackages, err := resolveCoverageLane(config{})
	if err != nil {
		t.Fatalf("resolveCoverageLane() error = %v", err)
	}

	if !slices.Contains(coverPackages, modulePath+"/pkg/config") {
		t.Fatalf("cover packages missing backend package: %v", coverPackages)
	}
	if slices.Contains(coverPackages, modulePath+"/pkg/generatedclient") {
		t.Fatalf("cover packages unexpectedly include generated client: %v", coverPackages)
	}
	if slices.Contains(coverPackages, modulePath+"/pkg/testutil") {
		t.Fatalf("cover packages unexpectedly include test helper package: %v", coverPackages)
	}
	if !slices.Contains(testPackages, modulePath+"/tests/functional/runtime_api") {
		t.Fatalf("test packages missing backend functional package: %v", testPackages)
	}
	if slices.Contains(testPackages, modulePath+"/tests/functional/internal/support") {
		t.Fatalf("test packages unexpectedly include functional support helpers: %v", testPackages)
	}
}

func TestResolveCoverageLaneOverrides(t *testing.T) {
	t.Parallel()

	cfg := config{
		coverpkg: "example.com/backend, example.com/shared",
		packages: "./pkg/config ./tests/functional/runtime_api",
	}

	coverPackages, testPackages, err := resolveCoverageLane(cfg)
	if err != nil {
		t.Fatalf("resolveCoverageLane() error = %v", err)
	}

	wantCover := []string{"example.com/backend", "example.com/shared"}
	if !slices.Equal(coverPackages, wantCover) {
		t.Fatalf("cover packages = %v, want %v", coverPackages, wantCover)
	}

	wantTests := []string{"./pkg/config", "./tests/functional/runtime_api"}
	if !slices.Equal(testPackages, wantTests) {
		t.Fatalf("test packages = %v, want %v", testPackages, wantTests)
	}
}

func TestEvaluateCoverageFlagsZeroCoverageBackendPackages(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"github.com/portpowered/infinite-you/pkg/config\t\tcoverage: 0.0% of statements\n"+
			"total: (statements) 82.5%\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", result.actual)
	}
	if totalLine != "total: (statements) 82.5%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 82.5%")
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageSkipsExcludedZeroCoveragePackages(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		modulePath + "/pkg/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 81.0 {
		t.Fatalf("actual coverage = %v, want 81.0", result.actual)
	}
	if totalLine != "total: (statements) 81.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 81.0%")
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
}

func TestEvaluateCoverageSupportsRepositoryRelativeProfilePaths(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		"pkg\\config\\config.go:1.1,2.1 2 0",
		"pkg\\service\\factory.go:1.1,2.1 4 1",
		"",
	}, "\n"))

	result, _, err := evaluateCoverage(
		"total: (statements) 80.0%\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestFormatZeroCoverageFailure(t *testing.T) {
	t.Parallel()

	got := formatZeroCoverageFailure([]string{
		modulePath + "/pkg/config",
		modulePath + "/pkg/service",
	})
	want := "go coverage found backend-owned packages with 0% statement coverage: " +
		modulePath + "/pkg/config, " + modulePath + "/pkg/service"
	if got != want {
		t.Fatalf("formatZeroCoverageFailure() = %q, want %q", got, want)
	}
}

func TestMainFailsWithZeroCoveragePackageSummary(t *testing.T) {
	if os.Getenv("GO_WANT_GOCOVERAGECHECK_MAIN_HELPER") == "1" {
		originalArgs := os.Args
		originalFlagSet := flag.CommandLine
		originalExecCommand := execCommand
		originalStdout := stdoutWriter
		originalStderr := stderrWriter
		originalExit := exitFunc
		defer func() {
			os.Args = originalArgs
			flag.CommandLine = originalFlagSet
			execCommand = originalExecCommand
			stdoutWriter = originalStdout
			stderrWriter = originalStderr
			exitFunc = originalExit
		}()

		flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
		os.Args = []string{
			"gocoveragecheck",
			"-min=80",
			"-coverpkg=" + strings.Join([]string{
				modulePath + "/pkg/config",
				modulePath + "/pkg/service",
				modulePath + "/pkg/generatedclient",
			}, ","),
			"-packages=./pkg/config",
		}
		execCommand = fakeGoCoverageCommand
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainFailsWithZeroCoveragePackageSummary")
	cmd.Env = append(os.Environ(), "GO_WANT_GOCOVERAGECHECK_MAIN_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("main() unexpectedly succeeded")
	}

	got := string(output)
	if !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() output = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config"
	if !strings.Contains(got, wantFailure) {
		t.Fatalf("main() output = %q, want zero-coverage failure %q", got, wantFailure)
	}
	if strings.Contains(got, modulePath+"/pkg/generatedclient") {
		t.Fatalf("main() output = %q, did not expect excluded package in zero-coverage failure", got)
	}
}

func TestGoCoverageCheckFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func writeCoverageProfile(t *testing.T, contents string) string {
	t.Helper()

	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}
	return profilePath
}

func fakeGoCoverageCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcess", "--", name}, args...)
	cmd := exec.Command(testBinary, cmdArgs...)
	return cmd
}

func helperCommandArgs(argv []string) ([]string, bool) {
	for index, arg := range argv {
		if arg == "--" {
			return argv[index+1:], true
		}
	}
	return nil, false
}

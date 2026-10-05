package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func TestRunDeadcodeExcludesTestExecutablesAndUsesGoTypesAliasEnvironment(t *testing.T) {
	prepareGeneratedHost(t)
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_STDOUT", "pkg/foo.go: Example\n")
	t.Setenv("GODEBUG", "gocachehash=1,gotypesalias=0")

	var captured []*exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmd := fakeDeadcodecheckCommand(name, args...)
		captured = append(captured, cmd)
		return cmd
	}
	report, err := runDeadcode(hostPathFile)
	if err != nil || report != "pkg/foo.go: Example\n" {
		t.Fatalf("runDeadcode() report=%q error=%v", report, err)
	}
	if len(captured) != 3 {
		t.Fatalf("commands = %d, want repository analysis, host analysis and compiler ownership", len(captured))
	}
	for i, cmd := range captured {
		if slices.Contains(cmd.Args, "-test") {
			t.Fatalf("tests must not be reachability roots: %v", cmd.Args)
		}
		if !envContains(cmd.Env, "GODEBUG=gocachehash=1,gotypesalias=1") || !envContains(cmd.Env, "GOWORK=off") {
			t.Fatalf("command %d environment missing compiler policy: %v", i, cmd.Env)
		}
		if cmd.Dir != mustWorkingDirectory(t) {
			t.Fatalf("command directory = %q, want fixture module", cmd.Dir)
		}
	}
	for i, pattern := range []string{"./...", "./cmd/golangci-lint"} {
		args := captured[i].Args
		if args[len(args)-1] != pattern || !slices.Contains(args, deadcodeTool) || !slices.Contains(args, "-filter=^github\\.com/portpowered/infinite-you(/|$)") {
			t.Fatalf("production analysis args = %v", args)
		}
	}
	if !slices.Contains(captured[2].Args, "list") || !slices.Contains(captured[2].Args, "-deps") {
		t.Fatalf("ownership must come from compiler dependencies: %v", captured[2].Args)
	}
}

func TestRunDeadcodeFailsWithoutGeneratedHost(t *testing.T) {
	chdirForTest(t, t.TempDir())
	if _, err := runDeadcode(hostPathFile); err == nil || !strings.Contains(err.Error(), "read generated golangci host") {
		t.Fatalf("missing host error = %v", err)
	}
	if err := os.WriteFile("host.txt", []byte("relative/host"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runDeadcode("host.txt"); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("relative host error = %v", err)
	}
}

func TestValidateHostModuleRejectsMissingMalformedAndWrongCheckout(t *testing.T) {
	root := t.TempDir()
	host := t.TempDir()
	for _, data := range []string{
		"not a go module", "module example.test/other\n",
		"module github.com/golangci/golangci-lint/v2\n",
		"module github.com/golangci/golangci-lint/v2\nreplace " + repositoryModule + " => " + filepath.ToSlash(host) + "\n",
	} {
		if err := os.WriteFile(filepath.Join(host, "go.mod"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := validateHostModule(host, root); err == nil {
			t.Fatalf("accepted invalid host module %q", data)
		}
	}
	if err := validateHostModule(t.TempDir(), root); err == nil {
		t.Fatal("accepted absent host module")
	}
}

func TestRepositoryPositionsPreservesDeadAnalyzerAndPluginFunctions(t *testing.T) {
	root := t.TempDir()
	host := t.TempDir()
	for _, source := range []string{"internal/lint/analyzers/rule.go", "tools/golangcilintplugin/plugin.go", "pkg/product.go"} {
		relative, err := filepath.Rel(host, filepath.Join(root, source))
		if err != nil {
			t.Fatal(err)
		}
		report, err := repositoryPositions(relative+":12:4: unreachable func: Abandoned\n", host, root)
		if err != nil || normalizeReport(report) != source+": unreachable func: Abandoned\n" {
			t.Fatalf("finding %q: report=%q error=%v", source, report, err)
		}
	}
	for _, report := range []string{"malformed", filepath.Join(host, "outside.go") + ":1:2: unreachable func: Outside"} {
		if _, err := repositoryPositions(report, host, root); err == nil {
			t.Fatalf("accepted invalid report %q", report)
		}
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

func TestRunBaselineMatchIgnoresVendoredSDKFindings(t *testing.T) {
	restore := stubDeadcodecheckCommand(t, "third_party/acp-go-sdk/helpers.go: Upstream\npkg/foo.go: Current\n", nil)
	defer restore()

	tempDir := t.TempDir()
	writeDeadcodeBaseline(t, tempDir, "pkg/foo.go: Current\n")
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
	currentReport, err := os.ReadFile(filepath.Join(tempDir, currentPath))
	if err != nil {
		t.Fatalf("read current deadcode report: %v", err)
	}
	if got := string(currentReport); got != "pkg/foo.go: Current\n" {
		t.Fatalf("current deadcode report = %q, want SDK findings excluded from the authored report", got)
	}
}

func TestNormalizeReportOmitsPlatformSpecificFindings(t *testing.T) {
	report := "pkg\\runner_windows.go:1:2: unreachable func: windowsOnly\n" +
		"pkg/runner_unix.go: unreachable func: unixOnly\n" +
		"pkg/runner_arm64.go: unreachable func: armOnly\n" +
		"pkg/runner.go:3:4: unreachable func: portable\n"
	if got, want := normalizeReport(report), "pkg/runner.go: unreachable func: portable\n"; got != want {
		t.Fatalf("normalizeReport() = %q, want %q", got, want)
	}
}

func TestNormalizeReportOmitsVendoredSDKSources(t *testing.T) {
	tests := []struct {
		name   string
		report string
		want   string
	}{
		{
			name: "excludes only the preserved SDK directory",
			report: strings.Join([]string{
				"third_party/acp-go-sdk/helpers.go:12:4: unreachable func: upstreamHelper",
				"third_party/acp-go-sdk/internal/session/session_windows.go: unreachable func: upstreamWindowsOnly",
				"third_party\\acp-go-sdk\\nested\\agent_gen.go: unreachable func: nestedUpstreamHelper",
				"third_party/acp-go-sdk-extra/helper.go: unreachable func: siblingVendored",
				"pkg/transports/acp/third_party/acp-go-sdk/helper.go: unreachable func: nestedMisleadingPrefix",
				"third_party/other-vendor/helper.go: unreachable func: otherVendored",
				"pkg/services/acp/negotiation.go: unreachable func: authoredService",
				"cmd/deadcodecheck/main.go: unreachable func: authoredCommand",
			}, "\n"),
			want: "cmd/deadcodecheck/main.go: unreachable func: authoredCommand\n" +
				"pkg/services/acp/negotiation.go: unreachable func: authoredService\n" +
				"pkg/transports/acp/third_party/acp-go-sdk/helper.go: unreachable func: nestedMisleadingPrefix\n" +
				"third_party/acp-go-sdk-extra/helper.go: unreachable func: siblingVendored\n" +
				"third_party/other-vendor/helper.go: unreachable func: otherVendored\n",
		},
		{
			name: "drops reports with only SDK findings",
			report: strings.Join([]string{
				"third_party/acp-go-sdk/client.go: unreachable func: upstreamClientHelper",
				"third_party/acp-go-sdk/example/agent/main.go: unreachable func: upstreamExampleHelper",
			}, "\n"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReport(tt.report); got != tt.want {
				t.Fatalf("normalizeReport() = %q, want %q", got, tt.want)
			}
		})
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
	prepareGeneratedHost(t)
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
	if !strings.Contains(errOutput, "run deadcode in ") {
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

	prepareGeneratedHost(t)
	writeDeadcodeBaseline(t, mustWorkingDirectory(t), "pkg/foo.go: Example\n")

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
	if got := string(passthroughStderr); got != "fake deadcode stderr\nfake deadcode stderr\n" {
		t.Fatalf("passthrough stderr = %q, want stderr from both production analyses", got)
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
	if got := normalizeReport("pkg\\foo.go:12:4: Example\r\npkg\\bar.go:9:2: Other"); got != "pkg/bar.go: Other\npkg/foo.go: Example\n" {
		t.Fatalf("normalizeReport() = %q", got)
	}
}

func TestNormalizeReportSortsAndTrimsFindings(t *testing.T) {
	if got := normalizeReport("\npkg/zeta.go: Later  \n pkg/alpha.go: First\n"); got != "pkg/alpha.go: First\npkg/zeta.go: Later\n" {
		t.Fatalf("normalizeReport() sorted output = %q", got)
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
	if slices.Contains(os.Args, "list") {
		fmt.Fprint(os.Stdout, os.Getenv("DEADCODECHECK_HELPER_PACKAGES"))
	} else {
		fmt.Fprint(os.Stdout, os.Getenv("DEADCODECHECK_HELPER_STDOUT"))
	}
	fmt.Fprint(os.Stderr, os.Getenv("DEADCODECHECK_HELPER_STDERR"))
	if os.Getenv("DEADCODECHECK_HELPER_FAIL") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func stubDeadcodecheckCommand(t *testing.T, report string, err error) func() {
	t.Helper()

	original := runDeadcodeCommand
	runDeadcodeCommand = func(_ string) (string, error) {
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

func prepareGeneratedHost(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	chdirForTest(t, root)
	if err := os.MkdirAll(filepath.Dir(hostPathFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostPathFile, []byte(root+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	module := "module github.com/golangci/golangci-lint/v2\nreplace " + repositoryModule + " => " + filepath.ToSlash(root) + "\n"
	if err := os.WriteFile("go.mod", []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DEADCODECHECK_HELPER_PACKAGES", filepath.Join(root, "internal/lint/analyzers")+"\n"+filepath.Join(root, "tools/golangcilintplugin")+"\n"+filepath.Join(root, "pkg"))

}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestReconcileProductionReportsKeepsUnreachableFunctionsInEveryPackage(t *testing.T) {
	repository := "internal/lint/analyzers/rule.go:1:2: unreachable func: LiveInHost\n" +
		"internal/lint/analyzers/rule.go:3:4: unreachable func: Abandoned\n" +
		"tools/golangcilintplugin/plugin.go:5:6: unreachable func: New\n" +
		"tools/golangcilintplugin/plugin.go:7:8: unreachable func: Abandoned\n" +
		"pkg/product.go:9:10: unreachable func: TestsOnly\n"
	host := "internal/lint/analyzers/rule.go:3:4: unreachable func: Abandoned\n" +
		"tools/golangcilintplugin/plugin.go:7:8: unreachable func: Abandoned\n"
	packages := map[string]bool{"internal/lint/analyzers": true, "tools/golangcilintplugin": true}
	want := normalizeReport(host + "pkg/product.go:9:10: unreachable func: TestsOnly\n")
	if got := reconcileProductionReports(repository, host, packages); got != want {
		t.Fatalf("reconciled report = %q, want %q", got, want)
	}
}

func TestHostRepositoryPackagesRejectsIncompleteCompilerGraph(t *testing.T) {
	prepareGeneratedHost(t)
	restoreExecCommand(t)
	t.Setenv("GO_WANT_DEADCODECHECK_HELPER", "1")
	t.Setenv("DEADCODECHECK_HELPER_PACKAGES", filepath.Join(mustWorkingDirectory(t), "tools/golangcilintplugin"))
	execCommand = fakeDeadcodecheckCommand
	root := mustWorkingDirectory(t)
	if _, err := hostRepositoryPackages(root, root); err == nil || !strings.Contains(err.Error(), "does not compile") {
		t.Fatalf("incomplete graph error = %v", err)
	}
}

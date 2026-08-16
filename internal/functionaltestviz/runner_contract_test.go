package functionaltestviz_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFunctionalTestVizRunnerClearsStaleArtifactsBeforeTimeout proves a later
// timeout cannot advertise complete diagnostics from an earlier run.
func TestFunctionalTestVizRunnerClearsStaleArtifactsBeforeTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("functional test viz runner contract requires a POSIX shell")
	}
	repoRoot := testutil.MustRepoPath(t, ".")
	artifactRoot := filepath.Join(t.TempDir(), "functional-test-viz-stale-artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create stale artifact directory: %v", err)
	}
	staleArtifacts := map[string]string{
		"functional-timing-summary.json": `{"complete":true,"captureReason":"stale timing"}`,
		"coverage-summary.json":          `{"complete":true,"coverage":99,"marker":"stale coverage"}`,
		"coverage.out":                   "mode: set\nstale coverage profile\n",
		"functional-tests.md":            "# stale Markdown\n",
		"diagnostic-status.txt":          "stale status\n",
	}
	for name, contents := range staleArtifacts {
		if err := os.WriteFile(filepath.Join(artifactRoot, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("seed stale artifact %s: %v", name, err)
		}
	}

	makePath := writeShellExecutable(t, "fake-make-functional-viz-no-profile", "#!/bin/sh\nprintf '%s\\n' 'fake-make:functional-test-viz'\n")
	timeoutPath := writeShellExecutable(t, "timeout", "#!/bin/sh\n[ \"$1\" = \"--signal=TERM\" ] && shift\n[ \"$1\" = \"--kill-after=30s\" ] && shift\nshift\n\"$@\" >/dev/null 2>&1\nexit 124\n")
	pathEnv, ok := os.LookupEnv("PATH")
	if !ok {
		t.Skip("functional test viz runner requires PATH")
	}

	output, err := runShellScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-functional-test-viz.sh"),
		fmt.Sprintf("FUNCTIONAL_TEST_VIZ_DIR=%s", artifactRoot),
		fmt.Sprintf("FUNCTIONAL_TEST_BUDGET=%s", "0.1s"),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
		fmt.Sprintf("PATH=%s%c%s", filepath.Dir(timeoutPath), os.PathListSeparator, pathEnv),
	)
	if err == nil {
		t.Fatalf("functional runner unexpectedly succeeded after simulated budget expiry:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 124 {
		t.Fatalf("functional runner exit = %v, want timeout exit code 124\n%s", err, output)
	}

	statusPath := filepath.Join(artifactRoot, "diagnostic-status.txt")
	statusBody, readErr := os.ReadFile(statusPath)
	if readErr != nil {
		t.Fatalf("read current timeout diagnostic status: %v", readErr)
	}
	status := string(statusBody)
	for _, expected := range []string{
		"missing: name=timing",
		"missing: name=coverage-summary",
		"missing: name=coverage-profile",
		"missing: name=markdown",
		"reason=no trustworthy partial coverage summary was available before interruption",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("timeout diagnostic status missing %q:\n%s", expected, status)
		}
	}
	for _, staleMarker := range []string{"stale timing", "stale coverage", "stale coverage profile", "stale Markdown", "stale status", "complete=true"} {
		if strings.Contains(status, staleMarker) || strings.Contains(output, staleMarker) {
			t.Fatalf("stale marker %q leaked into current timeout diagnostics:\nstatus=%s\noutput=%s", staleMarker, status, output)
		}
	}

	for name := range staleArtifacts {
		if name == "diagnostic-status.txt" {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(artifactRoot, name)); !os.IsNotExist(statErr) {
			t.Fatalf("stale run-owned artifact %s survived timeout cleanup; stat error = %v", name, statErr)
		}
	}
}

func writeShellExecutable(t *testing.T, name string, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write shell executable %s: %v", name, err)
	}
	return path
}

func runShellScript(repoRoot string, scriptPath string, env ...string) (string, error) {
	cmd := exec.Command(scriptPath)
	cmd.Dir = repoRoot
	pathEnv, _ := os.LookupEnv("PATH")
	cmd.Env = append([]string{"PATH=" + pathEnv}, env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

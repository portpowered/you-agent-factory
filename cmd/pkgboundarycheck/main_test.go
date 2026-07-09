package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSucceedsWithApprovedRootPackageFamilies(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/service")
	makeDir(t, repoRoot, "pkg/orchestrators")
	makeDir(t, repoRoot, "pkg/generatedclient")
	makeDir(t, repoRoot, "pkg/workflowpreview")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed (approved root package families and documented exceptions only)") {
		t.Fatalf("run() stdout = %q, want package-boundary success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunFailsForUnapprovedRootPackageFamily(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/service")
	makeDir(t, repoRoot, "pkg/experimental")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want package-boundary violation")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	wantOutput := strings.Join([]string{
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"  reason: pkg/experimental is outside the approved package-family allowlist.",
		"  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.",
		"",
	}, "\n")
	if got := stderr.String(); got != wantOutput {
		t.Fatalf("run() stderr = %q, want diagnostic %q", got, wantOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want violation count", got)
	}
}

func TestRunReportsMultipleUnapprovedRootPackagesDeterministically(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/zeta")
	makeDir(t, repoRoot, "pkg/experimental")
	makeDir(t, repoRoot, "pkg/alpha")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want package-boundary violations")
	}

	errOutput := stderr.String()
	alphaIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/alpha")
	experimentalIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental")
	zetaIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/zeta")
	if alphaIndex < 0 || experimentalIndex < 0 || zetaIndex < 0 {
		t.Fatalf("run() stderr = %q, want all unapproved roots reported", errOutput)
	}
	if !(alphaIndex < experimentalIndex && experimentalIndex < zetaIndex) {
		t.Fatalf("run() stderr = %q, want package roots reported in path order", errOutput)
	}
	if got := strings.Count(errOutput, "outside the approved package-family allowlist"); got != 3 {
		t.Fatalf("run() stderr = %q, want remediation details for each violation", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 3 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want three violation count", got)
	}
}

func TestRunRejectsEmptyPackageRoot(t *testing.T) {
	t.Parallel()

	err := run(config{root: t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "package root must not be empty" {
		t.Fatalf("run() error = %v, want package root validation", err)
	}
}

func TestMakePkgBoundaryTargetFailsForUnapprovedRootPackageFamily(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	makeDir(t, fixtureRoot, "pkg/experimental")

	cmd := exec.Command("make", "pkg-boundary", "PACKAGE_BOUNDARY_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make pkg-boundary succeeded, want unapproved root failure; output:\n%s", output)
	}

	got := string(output)
	for _, want := range []string{
		"go run ./cmd/pkgboundarycheck -root " + fixtureRoot,
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"outside the approved package-family allowlist",
		"move the code under an approved owner or deliberately update the allowlist with ownership rationale",
		"[agent-factory:pkg-boundary] found 1 package-boundary violation(s)",
		"*** [pkg-boundary]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("make pkg-boundary output = %q, want substring %q", got, want)
		}
	}
}

func makeDir(t *testing.T, repoRoot string, relativePath string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(absolutePath, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", relativePath, err)
	}
}

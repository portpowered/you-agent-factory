package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunQuietlyHidesRecordedBoundaryDebtButReportsNewFinding(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const recordedImport = repositoryImportPrefix + "pkg/transports/http"
	writeGoImportFile(t, repoRoot, "pkg/services/work/recorded.go", "work", recordedImport)
	commitRecordedBoundaryFixture(t, repoRoot)
	writeGoImportFile(t, repoRoot, "pkg/services/work/new.go", "work", recordedImport)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD"}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want unrecorded boundary finding")
	}
	if got := stderr.String(); !strings.Contains(got, recordedImport+" (pkg/services/work/new.go)") {
		t.Fatalf("run() stderr = %q, want new boundary diagnostic", got)
	}
	if strings.Contains(stderr.String(), "recorded.go") {
		t.Fatalf("run() stderr = %q, recorded baseline finding must be quiet by default", stderr.String())
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want one blocking finding", got)
	}

	stdout.Reset()
	stderr.Reset()
	err = run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD", all: true}, stdout, stderr)
	if err == nil {
		t.Fatal("run(--all) error = nil, want the same unrecorded boundary finding")
	}
	if got := stderr.String(); !strings.Contains(got, "recorded.go") || !strings.Contains(got, "new.go") {
		t.Fatalf("run(--all) stderr = %q, want recorded and new diagnostics", got)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run(--all) error = %q, want unchanged blocking result", got)
	}
}

func TestRunQuietlyPassesRecordedBoundaryDebtAndAllShowsIt(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/work/recorded.go", "work", repositoryImportPrefix+"pkg/transports/http")
	commitRecordedBoundaryFixture(t, repoRoot)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD"}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, want recorded debt to pass; stderr=%q", err, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want quiet default diagnostics", got)
	}
	if strings.Contains(stdout.String(), "recorded.go") || strings.Contains(stdout.String(), "active peer-service") || strings.Contains(stdout.String(), "active test service") || strings.Contains(stdout.String(), "active product-service") {
		t.Fatalf("run() stdout = %q, want no recorded finding or debt summary", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD", all: true}, stdout, stderr); err != nil {
		t.Fatalf("run(--all) error = %v, want recorded debt to remain non-blocking; stderr=%q", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "recorded.go") {
		t.Fatalf("run(--all) stdout = %q, want recorded diagnostic", got)
	}
}

func TestRunReportsNewFindingAddedToRecordedFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/services/work/recorded.go"
	writeGoImportFile(t, repoRoot, filePath, "work", repositoryImportPrefix+"pkg/transports/http")
	commitRecordedBoundaryFixture(t, repoRoot)
	writeGoSourceFile(t, repoRoot, filePath, `package work

import (
	_ "github.com/portpowered/infinite-you/pkg/transports/http"
	_ "github.com/portpowered/infinite-you/pkg/wire"
)
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD"}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want the added same-file boundary finding")
	}
	if got := stderr.String(); !strings.Contains(got, "prohibited application composition import") || !strings.Contains(got, filePath) {
		t.Fatalf("run() stderr = %q, want the new same-file diagnostic", got)
	}
	if strings.Contains(stderr.String(), "prohibited domain transport import") {
		t.Fatalf("run() stderr = %q, recorded finding in the changed file must remain quiet", stderr.String())
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want one blocking finding", got)
	}
}

func TestMakeLintPathKeepsQuietBaselineAndAllDiagnosticsOptIn(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	writeGoImportFile(t, fixtureRoot, "pkg/services/work/recorded.go", "work", repositoryImportPrefix+"pkg/transports/http")
	commitRecordedBoundaryFixture(t, fixtureRoot)
	writeGoImportFile(t, fixtureRoot, "pkg/services/work/new.go", "work", repositoryImportPrefix+"pkg/transports/http")

	quietOutput := runMakeLintBoundaryFixture(t, repoRoot, fixtureRoot, "PACKAGE_BOUNDARY_ALL=0")
	if !strings.Contains(quietOutput, "pkg-boundary") || !strings.Contains(quietOutput, "new.go") {
		t.Fatalf("quiet make lint output = %q, want attributed new finding", quietOutput)
	}
	if strings.Contains(quietOutput, "recorded.go") {
		t.Fatalf("quiet make lint output = %q, recorded baseline finding must be hidden", quietOutput)
	}

	allOutput := runMakeLintBoundaryFixture(t, repoRoot, fixtureRoot, "PACKAGE_BOUNDARY_ALL=1")
	if !strings.Contains(allOutput, "recorded.go") || !strings.Contains(allOutput, "new.go") {
		t.Fatalf("all make lint output = %q, want both diagnostic sets", allOutput)
	}
}

func commitRecordedBoundaryFixture(t *testing.T, repoRoot string) {
	t.Helper()
	runGitFixtureCommand(t, repoRoot, "init", "-q")
	runGitFixtureCommand(t, repoRoot, "config", "user.email", "pkg-boundary-test@example.invalid")
	runGitFixtureCommand(t, repoRoot, "config", "user.name", "pkg-boundary test")
	runGitFixtureCommand(t, repoRoot, "add", ".")
	runGitFixtureCommand(t, repoRoot, "commit", "-qm", "record package-boundary fixture")
}

func runGitFixtureCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func runMakeLintBoundaryFixture(t *testing.T, repoRoot, fixtureRoot, allSetting string) string {
	t.Helper()
	command := exec.Command(
		"make",
		"lint",
		"LINT_TARGETS=pkg-boundary",
		"LINT_JOBS=1",
		"PACKAGE_BOUNDARY_ROOT="+fixtureRoot,
		"PACKAGE_BOUNDARY_BASE_REF=HEAD",
		allSetting,
	)
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("make lint %s succeeded, want new boundary failure; output:\n%s", allSetting, output)
	}
	return string(output)
}

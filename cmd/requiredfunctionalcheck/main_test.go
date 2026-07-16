package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunChecksRepositoryRequiredScenarioManifest(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	var stdout bytes.Buffer
	if err := run(config{root: root, manifestPath: defaultManifestPath}, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stdout.String(); got != "[agent-factory:required-functional] 2 required short customer-boundary scenario(s) are current; 2 reviewed non-required SSE disposition(s) are explicit; the full functional tree is boundary-enforced; 19 unchanged legacy file(s) remain quarantined by the reviewed migration baseline\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunReturnsStableGuardDiagnostic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, defaultManifestPath)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	manifest := `{"formatVersion":"required-functional-scenarios/v1","scenarios":[{"stableId":"cli/missing","test":"tests/functional/missing_test.go::TestMissing","interface":"cli","lane":"short","executionClass":"deterministic","customerBoundary":true}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	err := run(config{root: root, manifestPath: defaultManifestPath}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `required functional scenario "cli/missing" [missing-test]`) {
		t.Fatalf("run() error = %v, want stable missing-test diagnostic", err)
	}
}

func TestRunReturnsStableBoundaryDiagnostic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCommandCanonicalManifest(t, root)
	writeCommandFixture(t, root, defaultManifestPath, `{"formatVersion":"required-functional-scenarios/v1","scenarios":[{"stableId":"cli/direct","test":"tests/functional/direct_test.go::TestDirect","interface":"cli","lane":"short","executionClass":"deterministic","customerBoundary":true}]}`)
	writeCommandFixture(t, root, "tests/functional/direct_test.go", `package functional
import "testing"
import service "github.com/portpowered/infinite-you/pkg/service"
func TestDirect(t *testing.T) { service.BuildFactoryService() }
`)

	err := run(config{root: root, manifestPath: defaultManifestPath}, &bytes.Buffer{})
	want := `functional test boundary [direct-product-boundary]: tests/functional/direct_test.go:4 directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.BuildFactoryService"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("run() error = %v, want %q", err, want)
	}
}

func TestCommandProcessExitsNonZeroWithStableBoundaryDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeCommandCanonicalManifest(t, root)
	writeCommandFixture(t, root, defaultManifestPath, `{"formatVersion":"required-functional-scenarios/v1","scenarios":[{"stableId":"cli/direct","test":"tests/functional/direct_test.go::TestDirect","interface":"cli","lane":"short","executionClass":"deterministic","customerBoundary":true}]}`)
	writeCommandFixture(t, root, "tests/functional/direct_test.go", `package functional
import "testing"
import service "github.com/portpowered/infinite-you/pkg/service"
func TestDirect(t *testing.T) { service.BuildFactoryService() }
`)

	binaryName := "requiredfunctionalcheck"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build required-functional-check: %v\n%s", err, output)
	}

	command := exec.Command(binaryPath, "-root", root)
	output, err := command.CombinedOutput()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 1 {
		t.Fatalf("required-functional-check error = %v, output = %q; want exit code 1", err, output)
	}
	want := `functional test boundary [direct-product-boundary]: tests/functional/direct_test.go:4 directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.BuildFactoryService"`
	if !strings.Contains(string(output), want) {
		t.Fatalf("required-functional-check output = %q, want %q", output, want)
	}
}

func TestRunReturnsStableBoundaryDiagnosticOutsideManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCommandCanonicalManifest(t, root)
	writeCommandFixture(t, root, defaultManifestPath, `{"formatVersion":"required-functional-scenarios/v1","scenarios":[{"stableId":"cli/allowed","test":"tests/functional/allowed_test.go::TestAllowed","interface":"cli","lane":"short","executionClass":"deterministic","customerBoundary":true}]}`)
	writeCommandFixture(t, root, "tests/functional/allowed_test.go", `package functional
import "testing"
func TestAllowed(t *testing.T) {}
`)
	writeCommandFixture(t, root, "tests/functional/non_manifest_test.go", `package functional
import service "github.com/portpowered/infinite-you/pkg/service"
func direct() { service.BuildFactoryService() }
`)

	err := run(config{root: root, manifestPath: defaultManifestPath}, &bytes.Buffer{})
	want := `functional test boundary [direct-product-boundary]: tests/functional/non_manifest_test.go:3 directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.BuildFactoryService"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("run() error = %v, want %q", err, want)
	}
}

func writeCommandCanonicalManifest(t *testing.T, root string) {
	t.Helper()
	writeCommandFixture(t, root, "contracts/functional-scenarios.json", `{"formatVersion":"functional-scenario-manifest/v1","scenarios":[]}`)
}

func writeCommandFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

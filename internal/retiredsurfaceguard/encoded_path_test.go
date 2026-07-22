package retiredsurfaceguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
)

func TestScanEncodedPathProductionSourceViolations_PassesOnCanonicalTree(t *testing.T) {
	repoRoot := repositoryRoot(t)
	violations, err := retiredsurfaceguard.ScanEncodedPathProductionSourceViolations(repoRoot)
	if err != nil {
		t.Fatalf("ScanEncodedPathProductionSourceViolations: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none on canonical tree", violations)
	}
}

func TestScanEncodedPathProductionSourceViolations_FailsOnDeliberateReintroduction(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg", "config")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`package config

import namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

func deliberateEncodedReintroduction(name string) (string, error) {
	return namedfactorypath.LegacyLayoutSegment(name)
}
`)
	path := filepath.Join(pkgDir, "deliberate_reintroduction.go")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	violations, err := retiredsurfaceguard.ScanEncodedPathProductionSourceViolations(root)
	if err != nil {
		t.Fatalf("ScanEncodedPathProductionSourceViolations: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected deliberate encoded-path source reintroduction violation")
	}
	output := formatViolations(violations)
	if !strings.Contains(output, "LegacyLayoutSegment") {
		t.Fatalf("violations = %q, want LegacyLayoutSegment finding", output)
	}
	if !strings.Contains(output, "deliberate_reintroduction.go") {
		t.Fatalf("violations = %q, want deliberate fixture file path", output)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

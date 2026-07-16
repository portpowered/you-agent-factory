package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_ReportsFunctionalBoundaryDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeCommandFixture(t, root, "required.json", `{"formatVersion":"required-functional-scenarios/v1","scenarios":[]}`)
	writeCommandFixture(t, root, "tests/functional/direct_service_test.go", `package scenario
import "github.com/portpowered/infinite-you/pkg/service"
func run() { build := service.NewFactoryService; build() }
`)

	var stdout bytes.Buffer
	err := run(config{root: root, manifestPath: "required.json"}, &stdout)
	if err == nil {
		t.Fatal("run accepted a direct service-construction alias")
	}
	if !strings.Contains(err.Error(), "functional test boundary [direct-product-boundary]") ||
		!strings.Contains(err.Error(), "pkg/service.NewFactoryService") {
		t.Fatalf("run error = %q, want stable direct-service diagnostic", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no success payload on guard failure", stdout.String())
	}
}

func writeCommandFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourceAcceptsCustomerBoundaryRequestBatchScenario(t *testing.T) {
	path := writeFunctionalSource(t, `package runtime_api
import (
  "github.com/portpowered/infinite-you/pkg/transports/http/generated"
  "github.com/portpowered/infinite-you/tests/functional/internal/support"
)
`)

	if err := checkSource(path); err != nil {
		t.Fatalf("checkSource() error = %v", err)
	}
}

func TestCheckSourceRejectsDirectRequestBatchProjectionInternal(t *testing.T) {
	path := writeFunctionalSource(t, `package guards_batch
import "github.com/portpowered/infinite-you/pkg/factory/projections"
`)

	err := checkSource(path)
	if err == nil {
		t.Fatal("checkSource() error = nil, want direct-internal boundary failure")
	}
	if !strings.Contains(err.Error(), diagnosticPrefix+" prohibited direct request-batch internal import: github.com/portpowered/infinite-you/pkg/factory/projections") {
		t.Fatalf("checkSource() error = %q, want stable actionable diagnostic", err)
	}
	if !strings.Contains(err.Error(), "use generated REST/SSE customers or tests/functional/internal/support instead") {
		t.Fatalf("checkSource() error = %q, want customer-boundary remediation", err)
	}
}

func TestCheckSourceRejectsDirectRequestBatchRuntimeInternal(t *testing.T) {
	path := writeFunctionalSource(t, `package guards_batch
import "github.com/portpowered/infinite-you/pkg/factory/runtime"
`)

	if err := checkSource(path); err == nil || !strings.Contains(err.Error(), "pkg/factory/runtime") {
		t.Fatalf("checkSource() error = %v, want runtime boundary failure", err)
	}
}

func TestCheckSourceChecksMigratedScenarioWithoutLegacyQuarantine(t *testing.T) {
	path := filepath.Join("..", "..", filepath.FromSlash(defaultScenarioPath))
	if err := checkSource(path); err != nil {
		t.Fatalf("checkSource(%q) error = %v", defaultScenarioPath, err)
	}
}

func writeFunctionalSource(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tests", "functional", "guards_batch", "request_batch_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create functional fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write functional fixture: %v", err)
	}
	return path
}

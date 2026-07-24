package runtimeopening

import (
	"os"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestOperatorConfigPathRequiresExplicitProcessHome(t *testing.T) {
	t.Parallel()

	_, err := operatorConfigPath(factorysessions.SessionRuntimeOpeningRequest{})
	if err == nil || !strings.Contains(err.Error(), "operator config home is required") {
		t.Fatalf("operatorConfigPath() error = %v, want required process home", err)
	}
}

func TestRuntimeOpeningDoesNotDependOnInitializer(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read runtimeopening package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(source), "pkg/initializer") {
			t.Errorf("%s imports initializer-owned construction", entry.Name())
		}
	}
}

func TestRuntimeOpeningDoesNotImportProcessEdgesBag(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read runtimeopening package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(source), "pkg/services/edges") {
			t.Errorf("%s imports process-edge bag; consume ExternalEffects projected at Wire instead", entry.Name())
		}
	}
}

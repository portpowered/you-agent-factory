package ownershipinventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyProviderSessionsSuccessorHasImplementation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	successor := "pkg/services/provider_sessions/internal/services/codex_reader"
	successorDir := filepath.Join(root, filepath.FromSlash(successor))
	if err := os.MkdirAll(successorDir, 0o755); err != nil {
		t.Fatalf("mkdir successor: %v", err)
	}

	err := verifyProviderSessionsSuccessorHasImplementation(root, successor)
	if err == nil {
		t.Fatal("verifyProviderSessionsSuccessorHasImplementation() error = nil, want missing implementation failure")
	}
	if !strings.Contains(err.Error(), "no non-test Go implementation files") {
		t.Fatalf("verifyProviderSessionsSuccessorHasImplementation() error = %v, want missing implementation failure", err)
	}

	if err := os.WriteFile(filepath.Join(successorDir, "service.go"), []byte("package codex_reader\n"), 0o644); err != nil {
		t.Fatalf("write successor implementation: %v", err)
	}
	if err := verifyProviderSessionsSuccessorHasImplementation(root, successor); err != nil {
		t.Fatalf("verifyProviderSessionsSuccessorHasImplementation() with implementation error = %v", err)
	}
}

package functionalevidence

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionalscenarios"
)

// Covers binds a passing customer-boundary test to the stable component IDs it
// exercised and observed. Call it only after the behavior assertions succeed.
func Covers(t *testing.T, stableIDs ...string) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("resolve functional evidence caller")
	}
	repositoryRoot, err := findRepositoryRoot(filename)
	if err != nil {
		t.Fatal(err)
	}
	relativePath, err := filepath.Rel(repositoryRoot, filename)
	if err != nil {
		t.Fatalf("resolve functional evidence path: %v", err)
	}
	reference := filepath.ToSlash(relativePath) + "::" + t.Name()
	if err := functionalscenarios.CheckEvidenceDeclaration(repositoryRoot, reference, stableIDs); err != nil {
		t.Fatalf("verify functional evidence declaration: %v", err)
	}
}

func findRepositoryRoot(filename string) (string, error) {
	directory := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("find repository root from %s", filename)
		}
		directory = parent
	}
}

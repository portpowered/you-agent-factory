package functionalscenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReplayContractBoundariesAcceptsMigratedScenarios(t *testing.T) {
	root := replayBoundaryRepositoryRoot(t)
	if err := CheckReplayContractBoundaries(root); err != nil {
		t.Fatalf("CheckReplayContractBoundaries() error = %v", err)
	}
}

func TestCheckReplayContractBoundariesRejectsDirectReplayHarness(t *testing.T) {
	root := t.TempDir()
	for _, path := range ReplayBoundaryFiles {
		writeReplayBoundaryFixture(t, root, path, "package replay_contracts\n")
	}
	path := ReplayBoundaryFiles[0]
	writeReplayBoundaryFixture(t, root, path, `package replay_contracts
import "github.com/portpowered/infinite-you/pkg/testutil"
func direct() { testutil.NewReplayHarness() }
`)

	err := CheckReplayContractBoundaries(root)
	want := "replay functional boundary: " + path + ":3 directly invokes github.com/portpowered/infinite-you/pkg/testutil.NewReplayHarness"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckReplayContractBoundaries() error = %v, want %q", err, want)
	}
}

func replayBoundaryRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func writeReplayBoundaryFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

package bootstrap_portability

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func assertTokenPayload(t *testing.T, snap *petri.MarkingSnapshot, placeID, want string) {
	t.Helper()

	for _, tok := range snap.Tokens {
		if tok.PlaceID == placeID {
			if got := string(tok.Color.Payload); got != want {
				t.Fatalf("expected payload %q, got %q", want, got)
			}
			return
		}
	}

	t.Fatalf("no token found in %s", placeID)
}

type fakeCommandRunner struct {
	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte(f.stdout), Stderr: []byte(f.stderr), ExitCode: f.exitCode}, nil
}

func successRunner(stdout string) workers.CommandRunner {
	return &fakeCommandRunner{stdout: stdout, exitCode: 0}
}

func writeFatFactoryJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write fat factory.json: %v", err)
	}
}

func writeFactoryTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

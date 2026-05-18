//go:build functionallong

package guards_batch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()
	return support.ProviderErrorCorpusEntry(t, name)
}

type panickingExecutor struct{}

func (e *panickingExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	panic("intentional executor panic for testing")
}

var _ workers.WorkerExecutor = (*panickingExecutor)(nil)

type failOnNthPageExecutor struct {
	mu     sync.Mutex
	calls  int
	failOn int
}

func (e *failOnNthPageExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()

	outcome := interfaces.OutcomeAccepted
	if call == e.failOn {
		outcome = interfaces.OutcomeFailed
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      outcome,
	}, nil
}

func tokenPlaces(snap petri.MarkingSnapshot) map[string]int {
	places := make(map[string]int)
	for _, tok := range snap.Tokens {
		places[tok.PlaceID]++
	}
	return places
}

func writeAgentConfig(t *testing.T, dir, workerName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workers", workerName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create worker config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertArgsContainSequence(t *testing.T, args, want []string) {
	t.Helper()

	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return
		}
	}

	t.Fatalf("expected args %v to contain sequence %v", args, want)
}

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()
	return support.ProviderErrorCorpusEntry(t, name)
}

type panickingExecutor struct{}

func (e *panickingExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	panic("intentional executor panic for testing")
}

var _ workers.WorkerExecutor = (*panickingExecutor)(nil)

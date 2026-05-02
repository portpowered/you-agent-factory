package guards_batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

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

type fanoutParserExecutor struct {
	mu         sync.Mutex
	calls      int
	childCount int
}

func (e *fanoutParserExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		parentWorkID = support.FirstInputToken(dispatch.InputTokens).Color.WorkID
	}

	spawned := make([]interfaces.TokenColor, e.childCount)
	for i := range spawned {
		spawned[i] = interfaces.TokenColor{
			WorkTypeID: "page",
			WorkID:     fmt.Sprintf("page-%d", i+1),
			ParentID:   parentWorkID,
		}
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork:  spawned,
	}, nil
}

func (e *fanoutParserExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

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

type multiChapterParserExecutor struct {
	mu          sync.Mutex
	calls       int
	childCounts []int
}

func (e *multiChapterParserExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	call := e.calls
	e.calls++
	e.mu.Unlock()

	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		parentWorkID = support.FirstInputToken(dispatch.InputTokens).Color.WorkID
	}

	childCount := 0
	if call < len(e.childCounts) {
		childCount = e.childCounts[call]
	}

	spawned := make([]interfaces.TokenColor, childCount)
	for i := range spawned {
		spawned[i] = interfaces.TokenColor{
			WorkTypeID: "page",
			WorkID:     fmt.Sprintf("%s-page-%d", parentWorkID, i+1),
			ParentID:   parentWorkID,
		}
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork:  spawned,
	}, nil
}

type panickingExecutor struct{}

func (e *panickingExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	panic("intentional executor panic for testing")
}

func tokenPlaces(snap petri.MarkingSnapshot) map[string]int {
	places := make(map[string]int)
	for _, tok := range snap.Tokens {
		places[tok.PlaceID]++
	}
	return places
}

var _ workers.WorkerExecutor = (*panickingExecutor)(nil)

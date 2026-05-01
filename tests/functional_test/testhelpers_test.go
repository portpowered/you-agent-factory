package functional_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// fanoutParserExecutor dynamically spawns N page tokens with ParentID set
// from the chapter's WorkID in the dispatch input tokens.
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
		parentWorkID = firstInputToken(dispatch.InputTokens).Color.WorkID
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

type capturingExecutor struct {
	result       interfaces.WorkResult
	lastDispatch interfaces.WorkDispatch
	callCount    int
}

func (e *capturingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.lastDispatch = dispatch
	e.callCount++
	result := e.result
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, nil
}

type blockingExecutor struct {
	releaseCh <-chan struct{}
	mu        *sync.Mutex
	calls     *int
}

func (e *blockingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	*e.calls++
	e.mu.Unlock()

	<-e.releaseCh

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// tokenPlaces returns a map of place ID → token count for debugging.
func tokenPlaces(snap petri.MarkingSnapshot) map[string]int {
	places := make(map[string]int)
	for _, tok := range snap.Tokens {
		places[tok.PlaceID]++
	}
	return places
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

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}

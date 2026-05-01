package functional_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
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

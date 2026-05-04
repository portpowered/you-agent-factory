package runtime_api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func normalizeSubmitRequestsForFunctionalTest(requests []interfaces.SubmitRequest) []interfaces.SubmitRequest {
	if len(requests) == 0 {
		return nil
	}
	normalized := make([]interfaces.SubmitRequest, len(requests))
	copy(normalized, requests)
	traceID := ""
	for _, request := range normalized {
		if request.TraceID != "" {
			traceID = request.TraceID
			break
		}
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-functional-%d", time.Now().UnixNano())
	}
	for i := range normalized {
		if normalized[i].TraceID == "" {
			normalized[i].TraceID = traceID
		}
	}
	return normalized
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
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}

func tokenPlaces(snap petri.MarkingSnapshot) map[string]int {
	places := make(map[string]int)
	for _, tok := range snap.Tokens {
		places[tok.PlaceID]++
	}
	return places
}

func functionalEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	out := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

var retiredFunctionalFactoryEventTypes = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
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

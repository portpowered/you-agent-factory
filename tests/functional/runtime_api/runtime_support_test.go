package runtime_api

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
)

func workRequestFromSubmitRequests(requests []interfaces.SubmitRequest) interfaces.WorkRequest {
	if len(requests) == 0 {
		return interfaces.WorkRequest{Type: interfaces.WorkRequestTypeFactoryRequestBatch}
	}

	requestID := sharedSubmitRequestIDForFunctionalTest(requests)
	usedNames := make(map[string]int, len(requests))
	works := make([]interfaces.Work, 0, len(requests))
	for i, req := range requests {
		itemRequestID := req.RequestID
		if itemRequestID == "" {
			itemRequestID = requestID
		}
		works = append(works, interfaces.Work{
			Name:             uniqueSubmitWorkNameForFunctionalTest(req, i, usedNames),
			WorkID:           req.WorkID,
			RequestID:        itemRequestID,
			WorkTypeID:       req.WorkTypeID,
			State:            req.TargetState,
			TraceID:          req.TraceID,
			Payload:          append([]byte(nil), req.Payload...),
			Tags:             maps.Clone(req.Tags),
			ExecutionID:      req.ExecutionID,
			RuntimeRelations: cloneRelationsForFunctionalTest(req.Relations),
		})
	}

	return interfaces.WorkRequest{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     works,
	}
}

func sharedSubmitRequestIDForFunctionalTest(requests []interfaces.SubmitRequest) string {
	var shared string
	for _, req := range requests {
		if req.RequestID == "" {
			continue
		}
		if shared == "" {
			shared = req.RequestID
			continue
		}
		if shared != req.RequestID {
			return ""
		}
	}
	return shared
}

func uniqueSubmitWorkNameForFunctionalTest(req interfaces.SubmitRequest, index int, used map[string]int) string {
	base := req.Name
	if req.Tags != nil && req.Tags["_work_name"] != "" {
		base = req.Tags["_work_name"]
	}
	if base == "" {
		base = req.WorkID
	}
	if base == "" {
		base = "work-" + strconv.Itoa(index+1)
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func cloneRelationsForFunctionalTest(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.Relation, len(relations))
	copy(out, relations)
	return out
}

type sleepyExecutor struct{ sleep time.Duration }

func (e *sleepyExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	time.Sleep(e.sleep)
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
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

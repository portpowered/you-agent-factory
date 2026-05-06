package runtime_api

import (
	"sort"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSubmitRuntimeWork_EmitsCanonicalTraceAwareBatchEvent(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	const requestID = "request-functional-runtime-trace-batch"
	server.SubmitRuntimeWork(
		t,
		interfaces.SubmitRequest{
			RequestID:              requestID,
			Name:                   "explicit-current",
			WorkID:                 "work-runtime-explicit-current",
			WorkTypeID:             "task",
			CurrentChainingTraceID: "chain-request-current",
			TraceID:                "trace-request-legacy",
			Payload:                []byte(`{"title":"explicit current"}`),
		},
		interfaces.SubmitRequest{
			Name:       "legacy-fallback",
			WorkID:     "work-runtime-legacy-fallback",
			WorkTypeID: "task",
			TraceID:    "trace-work-legacy",
			Payload:    []byte(`{"title":"legacy fallback"}`),
		},
	)

	event := waitForRuntimeAPIWorkRequestEvent(t, server, requestID, 5*time.Second)
	if got := support.StringPointerValue(event.Context.RequestId); got != requestID {
		t.Fatalf("WORK_REQUEST context request ID = %q, want %q", got, requestID)
	}

	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("WORK_REQUEST payload type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}

	works := append([]factoryapi.Work(nil), support.FactoryWorksValue(payload.Works)...)
	if len(works) != 2 {
		t.Fatalf("WORK_REQUEST payload work count = %d, want 2", len(works))
	}
	sort.Slice(works, func(i, j int) bool { return works[i].Name < works[j].Name })

	explicit := works[0]
	if explicit.Name != "explicit-current" {
		t.Fatalf("first work name = %q, want explicit-current", explicit.Name)
	}
	if got := support.StringPointerValue(explicit.CurrentChainingTraceId); got != "chain-request-current" {
		t.Fatalf("explicit work current chaining trace ID = %q, want chain-request-current", got)
	}
	if got := support.StringPointerValue(explicit.TraceId); got != "trace-request-legacy" {
		t.Fatalf("explicit work trace ID = %q, want trace-request-legacy", got)
	}

	legacyFallback := works[1]
	if legacyFallback.Name != "legacy-fallback" {
		t.Fatalf("second work name = %q, want legacy-fallback", legacyFallback.Name)
	}
	if got := support.StringPointerValue(legacyFallback.CurrentChainingTraceId); got != "trace-work-legacy" {
		t.Fatalf("legacy-fallback current chaining trace ID = %q, want trace-work-legacy", got)
	}
	if got := support.StringPointerValue(legacyFallback.TraceId); got != "trace-work-legacy" {
		t.Fatalf("legacy-fallback trace ID = %q, want trace-work-legacy", got)
	}

	waitForGeneratedWorkIDsComplete(
		t,
		server.URL(),
		[]string{"work-runtime-explicit-current", "work-runtime-legacy-fallback"},
		10*time.Second,
	)

	snapshot := server.GetEngineStateSnapshot(t)
	explicitToken := requireRuntimeAPITokenByWorkID(t, snapshot.Marking.Tokens, "work-runtime-explicit-current")
	if explicitToken.Color.RequestID != requestID {
		t.Fatalf("explicit token request ID = %q, want %q", explicitToken.Color.RequestID, requestID)
	}
	if explicitToken.Color.CurrentChainingTraceID != "chain-request-current" {
		t.Fatalf("explicit token current chaining trace ID = %q, want chain-request-current", explicitToken.Color.CurrentChainingTraceID)
	}

	legacyToken := requireRuntimeAPITokenByWorkID(t, snapshot.Marking.Tokens, "work-runtime-legacy-fallback")
	if legacyToken.Color.RequestID != requestID {
		t.Fatalf("legacy-fallback token request ID = %q, want inherited %q", legacyToken.Color.RequestID, requestID)
	}
	if legacyToken.Color.CurrentChainingTraceID != "trace-work-legacy" {
		t.Fatalf("legacy-fallback token current chaining trace ID = %q, want trace-work-legacy", legacyToken.Color.CurrentChainingTraceID)
	}
}

func waitForRuntimeAPIWorkRequestEvent(
	t *testing.T,
	server *functionalAPIServer,
	requestID string,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := server.GetFactoryEvents(t)
		for _, event := range events {
			if event.Type == factoryapi.FactoryEventTypeWorkRequest &&
				support.StringPointerValue(event.Context.RequestId) == requestID {
				return event
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for WORK_REQUEST event for %q", requestID)
	return factoryapi.FactoryEvent{}
}

func requireRuntimeAPITokenByWorkID(
	t *testing.T,
	tokens map[string]*interfaces.Token,
	workID string,
) *interfaces.Token {
	t.Helper()

	for _, token := range tokens {
		if token != nil && token.Color.WorkID == workID {
			return token
		}
	}
	t.Fatalf("missing runtime token for work %q", workID)
	return nil
}

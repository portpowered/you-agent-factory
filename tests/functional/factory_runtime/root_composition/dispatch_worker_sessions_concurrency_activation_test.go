package root_composition_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryRuntimeConcurrentDispatchesGetDistinctWorkerSessionsThroughRootBuildProcess
// proves the ACP-L4-W4 Worker Sessions dispatch cutover holds under real
// concurrency on a process constructed only through root.BuildProcess with
// provider process effects replaced via edges.Edges (a command-runner provider
// mock returning sanitized, provider-shaped fixtures). Two concurrently
// admitted Work items share the same "plan" workstation and worker; an
// order-reversing command-runner wrapper forces the second-arriving dispatch
// to complete before the first-arriving one, proving both dispatches were
// genuinely in flight at once and that forced out-of-order completion still
// resolves to pairwise-distinct Worker Session associations and the correct
// terminal Work/trace identity for every dispatch. The association fact is
// not part of the public HTTP/OpenAPI surface (W4 adds no public API), so it
// is observed from the canonical `--record` replay artifact written to disk
// rather than through HTTP.
func TestFactoryRuntimeConcurrentDispatchesGetDistinctWorkerSessionsThroughRootBuildProcess(
	t *testing.T,
) {
	t.Parallel()

	const (
		alphaTraceID = "acp-w4-concurrency-alpha"
		betaTraceID  = "acp-w4-concurrency-beta"
		ideaWorkType = "idea"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
	artifactPath := filepath.Join(t.TempDir(), "acp-w4-concurrency.replay.json")

	runner := newOrderReversingCommandRunner(
		testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...),
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", artifactPath},
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	support.UpsertDefaultSessionWorkRequest(t, baseURL, factoryapi.WorkRequest{
		RequestId: "acp-w4-concurrency-seed",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "concurrency-idea-alpha",
				WorkTypeName: stringPointer(ideaWorkType),
				TraceId:      stringPointer(alphaTraceID),
				Payload:      map[string]string{"title": "concurrency idea alpha"},
			},
			{
				Name:         "concurrency-idea-beta",
				WorkTypeName: stringPointer(ideaWorkType),
				TraceId:      stringPointer(betaTraceID),
				Payload:      map[string]string{"title": "concurrency idea beta"},
			},
		},
	})

	support.WaitForRuntimeIdle(t, baseURL, 20*time.Second)

	terminal := support.WorkCustomerLocation("prd", "complete")
	listed := support.ListDefaultSessionWork(t, baseURL)
	assertConcurrencyActivationReachedTerminal(t, listed, terminal, []string{alphaTraceID, betaTraceID})
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("prd", "failed")); got != 0 {
		t.Fatalf("prd:failed Work count = %d, want 0", got)
	}

	if got := runner.arrivals(); got < 2 {
		t.Fatalf("order-reversing command runner observed %d arrivals, want at least 2 concurrent calls", got)
	}

	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	associations := concurrencyActivationAssociationEvents(t, artifact)
	if len(associations) != 6 {
		t.Fatalf(
			"DISPATCH_WORKER_SESSION_ASSOCIATION events = %d, want 6 (2 concurrently seeded ideas x 3 workstations)",
			len(associations),
		)
	}

	seenSessions := map[string]bool{}
	seenDispatches := map[string]bool{}
	for _, association := range associations {
		if seenSessions[association.sessionID] {
			t.Fatalf("duplicate Worker Session ID %q across concurrent dispatches", association.sessionID)
		}
		seenSessions[association.sessionID] = true
		if seenDispatches[association.dispatchID] {
			t.Fatalf("duplicate canonical association for dispatch %q", association.dispatchID)
		}
		seenDispatches[association.dispatchID] = true

		responseSequence, ok := concurrencyActivationDispatchResponseSequence(artifact, association.dispatchID)
		if !ok {
			t.Fatalf("dispatch %q missing a canonical DISPATCH_RESPONSE event", association.dispatchID)
		}
		if association.sequence >= responseSequence {
			t.Fatalf(
				"dispatch %q association sequence %d did not precede its response sequence %d",
				association.dispatchID,
				association.sequence,
				responseSequence,
			)
		}
	}
}

func assertConcurrencyActivationReachedTerminal(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()

	found := map[string]string{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || item.WorkId == nil || *item.WorkId == "" {
			t.Fatalf("terminal Work at %s missing trace/work identity: %#v", terminalLocation, item)
		}
		if existing, ok := found[*item.TraceId]; ok {
			t.Fatalf(
				"duplicate terminal Work for trace %q at %s: %q and %q",
				*item.TraceId,
				terminalLocation,
				existing,
				*item.WorkId,
			)
		}
		found[*item.TraceId] = *item.WorkId
	}
	for _, traceID := range traceIDs {
		if _, ok := found[traceID]; !ok {
			t.Fatalf("Work list missing %s trace %q: %#v", terminalLocation, traceID, listed.Results)
		}
	}
	if len(found) != len(traceIDs) {
		t.Fatalf("%s Work count = %d, want %d: %#v", terminalLocation, len(found), len(traceIDs), listed.Results)
	}
}

type concurrencyActivationAssociation struct {
	dispatchID string
	sessionID  string
	sequence   int
}

func concurrencyActivationAssociationEvents(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
) []concurrencyActivationAssociation {
	t.Helper()

	var associations []concurrencyActivationAssociation
	for _, event := range artifact.Events {
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc {
			continue
		}
		if event.Context.DispatchID == nil {
			t.Fatalf("DISPATCH_WORKER_SESSION_ASSOCIATION event %q missing dispatchId context", event.Id)
		}
		var payload interfaces.DispatchWorkerSessionAssociationEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode DISPATCH_WORKER_SESSION_ASSOCIATION payload for event %q: %v", event.Id, err)
		}
		if payload.WorkerSessionID == "" {
			t.Fatalf("DISPATCH_WORKER_SESSION_ASSOCIATION event %q missing workerSessionId", event.Id)
		}
		associations = append(associations, concurrencyActivationAssociation{
			dispatchID: *event.Context.DispatchID,
			sessionID:  payload.WorkerSessionID,
			sequence:   event.Context.Sequence,
		})
	}
	return associations
}

func concurrencyActivationDispatchResponseSequence(
	artifact *interfaces.ReplayArtifact,
	dispatchID string,
) (int, bool) {
	for _, event := range artifact.Events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		if event.Context.DispatchID == nil || *event.Context.DispatchID != dispatchID {
			continue
		}
		return event.Context.Sequence, true
	}
	return 0, false
}

// orderReversingCommandRunner forces the second concurrently-arriving command
// invocation to complete before the first, proving genuine concurrent
// dispatch (both calls are in flight before either returns) and that
// out-of-order Worker completion still resolves to the correct dispatch,
// session, and Work identity. Calls beyond the first two pass through
// unmodified.
type orderReversingCommandRunner struct {
	inner platformprocess.CommandRunner

	mu       sync.Mutex
	arrival  int
	firstIn  chan struct{}
	secondOK chan struct{}
}

func newOrderReversingCommandRunner(inner platformprocess.CommandRunner) *orderReversingCommandRunner {
	return &orderReversingCommandRunner{
		inner:    inner,
		firstIn:  make(chan struct{}),
		secondOK: make(chan struct{}),
	}
}

func (r *orderReversingCommandRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.arrival++
	arrival := r.arrival
	r.mu.Unlock()

	switch arrival {
	case 1:
		close(r.firstIn)
		select {
		case <-r.secondOK:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case 2:
		select {
		case <-r.firstIn:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}

	result, err := r.inner.Run(ctx, req)

	if arrival == 2 {
		close(r.secondOK)
	}
	return result, err
}

func (r *orderReversingCommandRunner) arrivals() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.arrival
}

var _ platformprocess.CommandRunner = (*orderReversingCommandRunner)(nil)

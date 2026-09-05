package execution_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var fscp01ReplayTransientFields = map[string]string{
	"confirmationState": "read-boundary confirmation is a process-local durability watermark, not a dispatch fact",
}

// TestFSCP01DispatchReplayParityAndSourceWitness closes the dispatch gap with
// a two-process root-built record/replay handoff. Public dispatch/session
// reads and canonical event/association observations are compared before and
// after the handoff. Fields that the current JavaScript fixture does not emit
// as distinguishable canonical dispatch facts remain INCONCLUSIVE with their
// stable blockers in the matrix.
func TestFSCP01DispatchReplayParityAndSourceWitness(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	locations := newFSCP01RunLocations(t)
	dir := support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-replay"})
	recordPath := filepath.Join(t.TempDir(), "fscp01-dispatch-replay.json")
	logFSCP01RunDeclaration(t, locations, dir, recordPath, "first root finalize; second root replay-only")

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("fscp01 replay provider output"),
	})
	first := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       locations.Env,
		Args:                      []string{"--record", recordPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	logFSCP01BoundPort(t, first.URL())
	firstClosed := false
	t.Cleanup(func() {
		if !firstClosed {
			first.Stop(t)
			first.Close(t)
		}
	})

	started := startFSCP01DispatchWorkflowSync(t, first.URL(), fscp01LiveDispatchCorrelationWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("recorded session status = %q, want SUCCEEDED", started.Status)
	}
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("recorded session id is empty")
	}
	listed := listFactorySessionDispatches(t, first.URL(), started.SessionId)
	summary := requireFSCP01DispatchSummaryByLabel(t, listed, dispatchCorrelationChildLabel)
	detail := getFactorySessionDispatch(t, first.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
	canonicalBefore := support.GetFactoryEventsForSessionAt(t, first.URL(), started.SessionId)
	if len(canonicalBefore) == 0 {
		t.Fatal("recorded session returned no canonical events")
	}
	publicFacts := observeFSCP01CanonicalDispatch(t, first.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, publicFacts)
	recordFSCP01DispatchFieldSources(t, "terminal", summary, detail)
	sessionBefore := readDurableSession(t, first.URL(), started.SessionId)
	assertFSCP01SessionReadMatchesStart(t, started, sessionBefore)

	// Closing the first root is the recording finalization boundary. The replay
	// root is not started until this process has been stopped and joined.
	first.Stop(t)
	first.Close(t)
	firstClosed = true
	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertFSCP01ReplayArtifactAssociation(t, artifact.Events, started.SessionId, summary.Id)

	second := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       locations.Env,
		Args:                      []string{"--replay", recordPath, "--no-record"},
	})
	logFSCP01BoundPort(t, second.URL())
	t.Cleanup(func() {
		second.Stop(t)
		second.Close(t)
	})

	replayed := listFactorySessionDispatches(t, second.URL(), started.SessionId)
	replayedSummary := requireFSCP01DispatchSummary(t, replayed, summary.Id)
	replayedDetail := getFactorySessionDispatch(t, second.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchListDetail(t, started.SessionId, replayedSummary, replayedDetail)
	sessionAfter := readDurableSession(t, second.URL(), started.SessionId)
	assertFSCP01ReplaySessionParity(t, sessionBefore, sessionAfter)
	assertFSCP01ReplayFieldParity(t, "summary", summary, replayedSummary)
	assertFSCP01ReplayFieldParity(t, "detail", detail, replayedDetail)

	canonicalAfter := support.GetFactoryEventsForSessionAt(t, second.URL(), started.SessionId)
	assertFSCP01CanonicalReplayParity(t, canonicalBefore, canonicalAfter)
	t.Logf("FSCP-01 replay evidence: session=%s dispatch=%s canonicalEvents=%d artifactEvents=%d workerSession=%s status=%s", started.SessionId, summary.Id, len(canonicalBefore), len(artifact.Events), publicFacts.WorkerSessionID, detail.Status)
}

func assertFSCP01ReplayArtifactAssociation(
	t *testing.T,
	events []recordings.FactoryEvent,
	sessionID, dispatchID string,
) {
	t.Helper()
	associationCount := 0
	terminalCount := 0
	for _, event := range events {
		if event.Type == recordings.FactoryEventTypeSessionCompleted {
			terminalCount++
		}
		if event.Type != recordings.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil ||
			!fscp01CanonicalDispatchIDMatches(*event.Context.DispatchID, sessionID, dispatchID) {
			continue
		}
		associationCount++
		if event.Context.SessionID == nil ||
			(*event.Context.SessionID != sessionID && *event.Context.SessionID != "~default") {
			t.Fatalf("replay artifact association sessionId = %#v, want %q or ~default", event.Context.SessionID, sessionID)
		}
	}
	if associationCount != 1 {
		t.Fatalf("replay artifact association count = %d, want exactly one", associationCount)
	}
	if terminalCount != 1 {
		t.Fatalf("replay artifact SESSION_COMPLETED count = %d, want exactly one", terminalCount)
	}
}

type fscp01ReplaySessionFields struct {
	SessionID        string                                          `json:"sessionId"`
	Status           factoryapi.FactorySessionDurableLifecycleStatus `json:"status"`
	OrchestratorKind factoryapi.FactoryOrchestratorKind              `json:"orchestratorKind"`
	Dialect          *string                                         `json:"dialect,omitempty"`
	ResolvedSource   factoryapi.FactorySessionResolvedSourceIdentity `json:"resolvedSource"`
	SourceHash       *string                                         `json:"sourceHash,omitempty"`
}

func projectFSCP01ReplaySessionFields(
	session factoryapi.FactorySessionDurableReadModel,
) fscp01ReplaySessionFields {
	return fscp01ReplaySessionFields{
		SessionID:        session.SessionId,
		Status:           session.Status,
		OrchestratorKind: session.OrchestratorKind,
		Dialect:          session.Dialect,
		ResolvedSource:   session.ResolvedSource,
		SourceHash:       session.SourceHash,
	}
}

func assertFSCP01SessionReadMatchesStart(
	t *testing.T,
	started factoryapi.FactorySessionSyncExecutionResponse,
	session factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()
	if started.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("sync start outcome = %q, want COMPLETED", started.SyncOutcome)
	}
	if session.SessionId != started.SessionId || session.Status != started.Status {
		t.Fatalf("public session read identity/status = %q/%q, want %q/%q from sync start", session.SessionId, session.Status, started.SessionId, started.Status)
	}
	if session.OrchestratorKind != started.OrchestratorKind {
		t.Fatalf("public session read orchestratorKind = %q, want %q from sync start", session.OrchestratorKind, started.OrchestratorKind)
	}
}

func assertFSCP01ReplaySessionParity(
	t *testing.T,
	before, after factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()
	beforePayload, err := json.Marshal(projectFSCP01ReplaySessionFields(before))
	if err != nil {
		t.Fatalf("marshal public session fields before replay: %v", err)
	}
	afterPayload, err := json.Marshal(projectFSCP01ReplaySessionFields(after))
	if err != nil {
		t.Fatalf("marshal public session fields after replay: %v", err)
	}
	if !bytes.Equal(beforePayload, afterPayload) {
		t.Fatalf("public session fields changed across replay: before=%s after=%s", beforePayload, afterPayload)
	}
	t.Logf("FSCP-01 replay session parity: mode=sync sessionId=%s status=%s orchestratorKind=%s source=%s", before.SessionId, before.Status, before.OrchestratorKind, before.ResolvedSource.Kind)
}

func assertFSCP01ReplayFieldParity(
	t *testing.T,
	shape string,
	before, after any,
) {
	t.Helper()
	beforeFields := marshalFSCP01JSONFields(t, before)
	afterFields := marshalFSCP01JSONFields(t, after)
	if len(beforeFields) == 0 || len(afterFields) == 0 {
		t.Fatalf("replay %s field maps = %d/%d, want non-empty", shape, len(beforeFields), len(afterFields))
	}
	for field, beforeValue := range beforeFields {
		if reason, transient := fscp01ReplayTransientFields[field]; transient {
			t.Logf("FSCP-01 replay parity shape=%s field=%s disposition=INCONCLUSIVE exclusion=%s before=%s after=%s", shape, field, reason, beforeValue, afterFields[field])
			continue
		}
		afterValue, ok := afterFields[field]
		if !ok {
			t.Fatalf("replay %s canonical field %q missing after handoff", shape, field)
		}
		if !bytes.Equal(beforeValue, afterValue) {
			t.Fatalf("replay %s canonical field %q changed: before=%s after=%s", shape, field, beforeValue, afterValue)
		}
	}
	for field := range afterFields {
		if _, transient := fscp01ReplayTransientFields[field]; transient {
			continue
		}
		if _, ok := beforeFields[field]; !ok {
			t.Fatalf("replay %s canonical field %q appeared only after handoff", shape, field)
		}
	}
	t.Logf("FSCP-01 replay parity shape=%s canonicalFields=%d transientExclusions=%d", shape, len(beforeFields)-len(fscp01ReplayTransientFields), len(fscp01ReplayTransientFields))
}

func assertFSCP01CanonicalReplayParity(
	t *testing.T,
	before, after []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("canonical replay event count = %d, want %d", len(after), len(before))
	}
	if len(before) == 0 {
		t.Fatal("canonical replay parity has no events")
	}
	for index := range before {
		beforePayload, err := json.Marshal(before[index])
		if err != nil {
			t.Fatalf("marshal canonical event before replay at index %d: %v", index, err)
		}
		afterPayload, err := json.Marshal(after[index])
		if err != nil {
			t.Fatalf("marshal canonical event after replay at index %d: %v", index, err)
		}
		if !bytes.Equal(beforePayload, afterPayload) {
			t.Fatalf("canonical replay event at index %d changed: before=%s after=%s", index, beforePayload, afterPayload)
		}
	}
	t.Logf("FSCP-01 canonical replay parity: events=%d ordering=exact association=retained", len(before))
}

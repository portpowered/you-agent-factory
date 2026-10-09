package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Only the registry and reservation component is composed. Detached source
// facts and cancellers are scenario-owned; no application or provider runs.
func scopedInterruptSources(t *testing.T) (*registry, workersessions.InterruptRequest) {
	t.Helper()
	req := workersessions.InterruptRequest{RequestID: "interrupt-scope", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: "replacement", ResumeMode: "provider"}
	r := newContinuationSource(t, workersessions.ContinueRequest{SourceWorkerSessionID: req.SourceWorkerSessionID})
	source := r.sessions[req.SourceWorkerSessionID]
	delete(r.sessions, req.SourceWorkerSessionID)
	for _, owner := range []string{"factory-a", "factory-b"} {
		address := scopedWorkerAddress(source.ID, owner)
		session := source.Clone()
		session.State = workersessions.StateRunning
		session.ProviderSessionAssociation.Reference.ID = "provider-" + owner
		r.sessions[address] = session
		execution := continuationValidExecution("dispatch-1")
		execution.Execution.FactorySessionID = owner
		s := newSupervision("dispatch-1", "turn-1", execution)
		s.accepted = true
		s.installCancel(func() { t.Error("validation canceled a source") })
		r.supervisions[address] = s
		r.observations[address] = &observation{factorySessionID: owner}
	}
	return r, req
}

func TestInterruptScopedReservationPreservesOwnerAndReplay(t *testing.T) {
	t.Parallel()
	r, req := scopedInterruptSources(t)
	for _, mode := range []string{"provider", "recorded"} {
		req.ResumeMode = mode
		result, err := r.Interrupt(t.Context(), req)
		var ambiguous *workersessions.AmbiguousAddressError
		if !errors.As(err, &ambiguous) || result.Phase != workersessions.InterruptPhaseValidation || result.Source.ID != "" || result.Successor.ID != "" {
			t.Fatalf("unscoped %s result=%+v error=%v", mode, result, err)
		}
		expected := []workersessions.AddressCandidate{
			{FactorySessionID: "factory-a", WorkerSessionID: "source", State: workersessions.StateRunning},
			{FactorySessionID: "factory-b", WorkerSessionID: "source", State: workersessions.StateRunning},
		}
		if !reflect.DeepEqual(ambiguous.Candidates, expected) {
			t.Fatalf("candidates=%+v", ambiguous.Candidates)
		}
	}
	req.FactorySessionID = "factory-missing"
	if _, _, err := r.reserveInterrupt(req); !errors.Is(err, workersessions.ErrInterruptSourceNotFound) {
		t.Fatalf("foreign scope=%v", err)
	}
	req.FactorySessionID, req.ResumeMode = "factory-a", "provider"
	replay, owner, err := r.reserveInterrupt(req)
	if err != nil || !owner {
		t.Fatalf("selected reservation owner=%v error=%v", owner, err)
	}
	defer r.finishStart()
	defer finishInterruptExecution(replay.plan.supervision, false)
	defer finishInterruptOperation(replay.plan.supervision)
	if replay.plan.sourceAddressOrID() != scopedWorkerAddress("source", "factory-a") || replay.plan.execution.Execution.FactorySessionID != "factory-a" || replay.plan.reference.ID != "provider-factory-a" {
		t.Fatalf("selected plan=%+v", replay.plan)
	}
	again, owner, err := r.reserveInterrupt(req)
	if err != nil || owner || again != replay {
		t.Fatalf("identical replay owner=%v error=%v", owner, err)
	}
	peer := req
	peer.FactorySessionID = "factory-b"
	if _, err := r.Interrupt(t.Context(), peer); !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
		t.Fatalf("cross-owner replay=%v", err)
	}
	peerSession := r.sessions[scopedWorkerAddress("source", "factory-b")]
	peerSupervision := r.supervisions[scopedWorkerAddress("source", "factory-b")]
	if peerSession.State != workersessions.StateRunning || peerSession.SuccessorWorkerSessionID != "" || peerSupervision.interrupting || peerSupervision.controlActive || len(r.sessions) != 2 {
		t.Fatal("peer or successor changed during validation")
	}
}

func TestInterruptScopedFactorySourceRemainsUnsupported(t *testing.T) {
	t.Parallel()
	r, req := scopedInterruptSources(t)
	req.FactorySessionID = "factory-a"
	r.runtimeAttempts[scopedWorkerAddress("source", "factory-a")] = struct{}{}
	result, err := r.Interrupt(t.Context(), req)
	if !errors.Is(err, workersessions.ErrInterruptFactoryUnsupported) || result.Phase != workersessions.InterruptPhaseValidation || result.Source.ProviderSessionAssociation.Reference.ID != "provider-factory-a" || len(r.interruptReplays) != 0 {
		t.Fatalf("Factory source result=%+v error=%v", result, err)
	}
}

func TestInterruptFrozenOwnerSurvivesPeerAppearingBeforeIntent(t *testing.T) {
	t.Parallel()
	r, plan, _ := newDurableInterruptFixture(t)
	address := scopedWorkerAddress("worker", "factory-a")
	r.sessions[address] = r.sessions["worker"]
	r.supervisions[address] = r.supervisions["worker"]
	r.publications[address] = r.publications["worker"]
	delete(r.sessions, "worker")
	delete(r.supervisions, "worker")
	delete(r.publications, "worker")
	r.publications[address].capture.FactorySessionID = "factory-a"
	plan.sourceAddress = address
	plan.execution.Execution.FactorySessionID = "factory-a"
	plan.supervision.execution = plan.execution
	// The admitted source address and attempt are immutable even though a
	// second owner appears before the durable intent is synchronized.
	peerAddress := scopedWorkerAddress("worker", "factory-b")
	r.sessions[peerAddress] = r.sessions[address].Clone()
	r.supervisions[peerAddress] = newSupervision("peer-attempt", "")
	operation, err := r.beginInterruptIntent(t.Context(), plan)
	if err != nil || operation.Target.FactorySessionID != "factory-a" || operation.Target.ExpectedAttemptID != plan.dispatchID {
		t.Fatalf("frozen intent=%+v error=%v", operation, err)
	}
	result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false, address)
	if result.Source.ID != "worker" || result.Source.State != workersessions.StateRunning {
		t.Fatalf("public frozen snapshot=%+v", result)
	}
	if err := r.commitInterruptResult(t.Context(), operation, result, workersessions.ErrInterruptSourceConflict); err != nil {
		t.Fatal(err)
	}
	req := plan.request
	req.FactorySessionID = "factory-a"
	replayed, found, err := r.replayDurableInterrupt(t.Context(), req)
	if !found || !errors.Is(err, workersessions.ErrInterruptSourceConflict) || !reflect.DeepEqual(result, replayed) {
		t.Fatalf("selected replay=%+v found=%v error=%v", replayed, found, err)
	}
	if r.supervisions[peerAddress].interrupting || r.sessions[peerAddress].SuccessorWorkerSessionID != "" {
		t.Fatal("late peer changed")
	}
}

func TestInterruptInputScopeMatchesCapturedExecution(t *testing.T) {
	t.Parallel()
	_, plan, _ := newDurableInterruptFixture(t)
	plan.execution.Execution.FactorySessionID = "factory-a"
	payload, err := encodeInterruptInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "factory-a", "factory-b"} {
		req := plan.request
		req.FactorySessionID = scope
		err := validateCapturedInterruptInput(payload, nil, req, plan.dispatchID)
		if scope == "factory-b" {
			if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
				t.Fatalf("foreign recipe accepted: %v", err)
			}
		} else if err != nil {
			t.Fatalf("matching recipe scope %q: %v", scope, err)
		}
	}
}

func TestInterruptScopedProviderRefusalDoesNotClaimPeer(t *testing.T) {
	t.Parallel()
	r, req := scopedInterruptSources(t)
	req.FactorySessionID = "factory-b"
	support := &interruptContinuationSupportFake{}
	r.continuationSupport = support
	result, err := r.Interrupt(t.Context(), req)
	if !errors.Is(err, workersessions.ErrInterruptContinuationUnsupported) || result.Phase != workersessions.InterruptPhaseValidation || support.reference.ID != "provider-factory-b" || result.Source.State != workersessions.StateRunning {
		t.Fatalf("selected refusal=%+v reference=%+v error=%v", result, support.reference, err)
	}
	for _, s := range r.supervisions {
		if s.interrupting || s.controlActive {
			t.Fatal("unsupported source claimed an attempt")
		}
	}
	if len(r.interruptReplays) != 0 || len(r.sessions) != 2 {
		t.Fatal("unsupported source admitted successor")
	}
}

func TestInterruptScopedArchivedReplayCannotFallBackToPeer(t *testing.T) {
	t.Parallel()
	r, plan, _ := newDurableInterruptFixture(t)
	r.logs = &LogReader{reader: &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
		RecordingID: "peer-recording", WorkerSessionID: "worker", FactorySessionID: "factory-b",
		RecordingGenerationID: "peer-generation", OwnerEpoch: "peer-epoch",
	}}}
	delete(r.sessions, "worker")
	delete(r.supervisions, "worker")
	delete(r.publications, "worker")
	plan.request.FactorySessionID = "factory-a"
	result, err := r.Interrupt(t.Context(), plan.request)
	if !errors.Is(err, workersessions.ErrInterruptSourceNotFound) || result.Phase != workersessions.InterruptPhaseValidation || result.Source.ID != "" || result.Successor.ID != "" {
		t.Fatalf("foreign capture fallback result=%+v error=%v", result, err)
	}
	if len(r.interruptReplays) != 0 || len(r.sessions) != 0 {
		t.Fatal("foreign capture changed registry")
	}
}

func TestInterruptScopedSelectionPreservesLegacyRequestReplay(t *testing.T) {
	t.Parallel()
	_, plan, _ := newDurableInterruptFixture(t)
	payload, err := json.Marshal(plan.request)
	if err != nil {
		t.Fatal(err)
	}
	plan.request.FactorySessionID = "factory-a"
	if err := validateCapturedInterruptInput(payload, nil, plan.request, plan.dispatchID); err != nil {
		t.Fatalf("selected legacy tuple=%v", err)
	}
	plan.request.ReplacementMessage = "changed"
	if err := validateCapturedInterruptInput(payload, nil, plan.request, plan.dispatchID); !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
		t.Fatalf("changed legacy tuple=%v", err)
	}
}

func TestInterruptPendingOpeningMatchesSelectedOwner(t *testing.T) {
	t.Parallel()
	r, plan, _ := newDurableInterruptFixture(t)
	target := r.publications["worker"].capture
	target.FactorySessionID = "factory-a"
	target.ExpectedAttemptID = plan.dispatchID
	reader, _ := pendingInterruptCaptureFixture("matching", plan, target)
	reader.entry.FactorySessionID = target.FactorySessionID
	reader.page.Catalog.FactorySessionID = target.FactorySessionID
	var draft workers.Draft
	var opening workers.SessionPayload
	if err := json.Unmarshal(reader.page.Opening.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(draft.Payload, &opening); err != nil {
		t.Fatal(err)
	}
	opening.FactorySessionID = target.FactorySessionID
	draft.Payload, _ = json.Marshal(opening)
	reader.page.Opening.Payload, _ = json.Marshal(draft)
	r.logs = &LogReader{reader: reader}
	if err := r.inspectPendingInterruptSuccessor(t.Context(), plan.request, target); err != nil {
		t.Fatalf("selected pending opening rejected: %v", err)
	}
	reader.entry.FactorySessionID = "factory-b"
	if err := r.inspectPendingInterruptSuccessor(t.Context(), plan.request, target); !errors.Is(err, workersessions.ErrInterruptSourceConflict) {
		t.Fatalf("peer opening accepted: %v", err)
	}
}

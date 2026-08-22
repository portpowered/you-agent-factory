package root_composition_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	w4AlphaWorkID = "acp-w4-direct-alpha"
	w4BetaWorkID  = "acp-w4-direct-beta"
	w4AlphaTrace  = "acp-w4-direct-alpha-trace"
	w4BetaTrace   = "acp-w4-direct-beta-trace"
)

// TestFactoryRuntimeMixedDirectAndFactoryChildDispatchesStayIsolatedThroughCLI
// proves the W4 cutover through the customer CLI rather than the HTTP service
// mode. The two directly admitted idea Work items race at the sole controlled
// provider edge and complete out of order. Each plan materializes a PRD child,
// which then executes and completes on its own Worker Session topic.
// Canonical recording is the public durable observation for this W4-only
// association, because Worker Session identity has no new HTTP surface.
func TestFactoryRuntimeMixedDirectAndFactoryChildDispatchesStayIsolatedThroughCLI(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
	writeMixedDispatchSeeds(t, dir)
	artifactPath := filepath.Join(t.TempDir(), "acp-w4-mixed-dispatch.replay.json")
	runner := newInterleavingW4ProviderCommandRunner()
	args := []string{
		"you", "run", "--dir", dir, "--record", artifactPath, "--quiet",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = dir

	if err := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
		t.Fatalf("successful CLI stderr = %q, want empty", stderr)
	}
	if !runner.firstTwoCompletedOutOfOrder() {
		t.Fatalf("controlled provider edge did not complete its first two direct dispatches out of order: %#v", runner.completedCalls())
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	observed := observeW4Dispatches(t, artifact)
	assertW4DirectAndChildIsolation(t, observed, w4AlphaWorkID, w4AlphaTrace)
	assertW4DirectAndChildIsolation(t, observed, w4BetaWorkID, w4BetaTrace)
	assertW4TerminalMaterialization(t, observed, []string{w4AlphaWorkID, w4BetaWorkID})
	assertW4DistinctAssociations(t, observed, []string{w4AlphaWorkID, w4BetaWorkID})
	assertW4PublishedAssociationsRoundTrip(t, observed, artifact.Events)
	if !runner.completedExpectedCalls() {
		t.Fatalf("provider stage calls = %#v, want two plan, execute, and review calls", runner.completedCalls())
	}
}

func writeMixedDispatchSeeds(t *testing.T, dir string) {
	t.Helper()
	for _, seed := range []struct{ workID, traceID, title string }{
		{w4AlphaWorkID, w4AlphaTrace, "w4 alpha direct input"},
		{w4BetaWorkID, w4BetaTrace, "w4 beta direct input"},
	} {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkID: seed.workID, WorkTypeID: "idea", TraceID: seed.traceID,
			Payload: []byte(`{"title":"` + seed.title + `"}`),
		})
	}
}

type w4DispatchObservation struct {
	dispatchID, transition, traceID, correlationID, sessionID, modelOutput string
	workIDs                                                                []string
	associationEventID                                                     string
	associationSequence, modelSequence, responseSequence                   int
	result                                                                 *workers.DispatchResponseEventPayload
}

type w4Dispatches struct {
	byID map[string]*w4DispatchObservation
}

func observeW4Dispatches(t *testing.T, artifact *interfaces.ReplayArtifact) *w4Dispatches {
	t.Helper()
	observed := &w4Dispatches{
		byID: make(map[string]*w4DispatchObservation),
	}
	for _, event := range artifact.Events {
		dispatchID := w4DispatchID(event)
		if dispatchID == "" {
			continue
		}
		entry := w4DispatchEntry(observed.byID, dispatchID)
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest:
			var payload interfaces.DispatchRequestEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode dispatch request %q: %v", event.Id, err)
			}
			entry.transition = payload.TransitionID
			entry.traceID = w4First(event.Context.TraceIDs)
			entry.correlationID = w4String(event.Context.CurrentChainingTraceID)
			entry.workIDs = append([]string(nil), w4Strings(event.Context.WorkIDs)...)
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			var payload interfaces.DispatchWorkerSessionAssociationEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode dispatch association %q: %v", event.Id, err)
			}
			if entry.sessionID != "" {
				t.Fatalf("dispatch %q has duplicate Worker Session associations", dispatchID)
			}
			entry.sessionID, entry.associationEventID, entry.associationSequence = payload.WorkerSessionID, event.Id, event.Context.Sequence
		case interfaces.FactoryEventTypeModelResponse:
			var payload workers.ModelResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode model response %q: %v", event.Id, err)
			}
			entry.modelOutput, entry.modelSequence = w4ContentText(payload.OutputContent), event.Context.Sequence
		case interfaces.FactoryEventTypeDispatchResponse:
			var payload workers.DispatchResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode dispatch response %q: %v", event.Id, err)
			}
			if entry.result != nil {
				t.Fatalf("dispatch %q has duplicate responses", dispatchID)
			}
			entry.result, entry.responseSequence = &payload, event.Context.Sequence
		}
	}
	return observed
}

func w4DispatchID(event interfaces.FactoryEvent) string {
	if event.Context.DispatchID == nil {
		return ""
	}
	return *event.Context.DispatchID
}

func w4DispatchEntry(observed map[string]*w4DispatchObservation, dispatchID string) *w4DispatchObservation {
	if entry := observed[dispatchID]; entry != nil {
		return entry
	}
	entry := &w4DispatchObservation{dispatchID: dispatchID}
	observed[dispatchID] = entry
	return entry
}

func w4Strings(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func w4First(value *[]string) string {
	for _, item := range w4Strings(value) {
		if item != "" {
			return item
		}
	}
	return ""
}

func w4String(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func w4ContentText(content *[]work.WorkContentPart) string {
	if content == nil {
		return ""
	}
	var parts []string
	for _, part := range *content {
		parts = append(parts, part.Text)
	}
	return strings.Join(parts, "")
}

func assertW4DirectAndChildIsolation(t *testing.T, observed *w4Dispatches, workID, traceID string) {
	t.Helper()
	direct := w4DispatchForWork(t, observed, "plan", workID)
	w4AssertDispatch(t, direct, traceID, "w4 planner COMPLETE")
	child := w4ChildWorkID(t, observed, workID)
	childDispatch := w4DispatchForWork(t, observed, "execute", child)
	w4AssertDispatch(t, childDispatch, traceID, "w4 executor COMPLETE")
	if direct.sessionID == childDispatch.sessionID || direct.dispatchID == childDispatch.dispatchID {
		t.Fatalf("direct and child identities collapsed: direct=%#v child=%#v", direct, childDispatch)
	}
	if workersessions.Topic(direct.sessionID) == workersessions.Topic(childDispatch.sessionID) {
		t.Fatalf("direct and child W3 topics collided: %q", workersessions.Topic(direct.sessionID))
	}
}

func w4DispatchForWork(t *testing.T, observed *w4Dispatches, transition, workID string) *w4DispatchObservation {
	t.Helper()
	var match *w4DispatchObservation
	for _, entry := range observed.byID {
		if entry.transition == transition && w4Contains(entry.workIDs, workID) {
			if match != nil {
				t.Fatalf("multiple %s dispatches for Work %q: %q and %q", transition, workID, match.dispatchID, entry.dispatchID)
			}
			match = entry
		}
	}
	if match == nil {
		t.Fatalf("missing %s dispatch for Work %q: %#v", transition, workID, observed.byID)
	}
	return match
}

func w4ChildWorkID(t *testing.T, observed *w4Dispatches, parentWorkID string) string {
	t.Helper()
	direct := w4DispatchForWork(t, observed, "plan", parentWorkID)
	if direct.result == nil || direct.result.OutputWork == nil {
		t.Fatalf("direct plan dispatch %q did not materialize a child Work", direct.dispatchID)
	}
	childWorkID := ""
	for _, outputWork := range *direct.result.OutputWork {
		if outputWork.WorkID == "" || outputWork.WorkID == parentWorkID {
			continue
		}
		if childWorkID != "" && childWorkID != outputWork.WorkID {
			t.Fatalf("direct Work %q materialized multiple children: %#v", parentWorkID, direct.result.OutputWork)
		}
		childWorkID = outputWork.WorkID
	}
	if childWorkID != "" {
		return childWorkID
	}
	t.Fatalf("direct Work %q did not materialize a child Work: %#v", parentWorkID, direct.result.OutputWork)
	return ""
}

func w4AssertDispatch(t *testing.T, entry *w4DispatchObservation, traceID, output string) {
	t.Helper()
	if entry.traceID != traceID || entry.correlationID != traceID || entry.sessionID == "" || entry.result == nil {
		t.Fatalf("dispatch observation = %#v, want trace/correlation/session/result for %q", entry, traceID)
	}
	if entry.modelOutput != output || entry.result.Output == nil || *entry.result.Output != output {
		t.Fatalf("dispatch %q outputs = model:%q result:%#v, want %q", entry.dispatchID, entry.modelOutput, entry.result.Output, output)
	}
	if entry.result.Outcome != workers.OutcomeAccepted {
		t.Fatalf("dispatch %q outcome = %q, want ACCEPTED", entry.dispatchID, entry.result.Outcome)
	}
	if entry.associationSequence >= entry.modelSequence || entry.associationSequence >= entry.responseSequence {
		t.Fatalf("dispatch %q association sequence %d must precede model %d and response %d", entry.dispatchID, entry.associationSequence, entry.modelSequence, entry.responseSequence)
	}
}

func assertW4TerminalMaterialization(t *testing.T, observed *w4Dispatches, directWorkIDs []string) {
	t.Helper()
	for _, workID := range directWorkIDs {
		child := w4ChildWorkID(t, observed, workID)
		review := w4DispatchForWork(t, observed, "review", child)
		w4AssertDispatch(t, review, review.traceID, "w4 reviewer COMPLETE")
		if review.result.OutputWork == nil || w4OutputWorkCount(*review.result.OutputWork, child, "complete") != 1 {
			t.Fatalf("review dispatch %q did not materialize child %q at prd:complete: %#v", review.dispatchID, child, review.result.OutputWork)
		}
	}
}

func assertW4DistinctAssociations(t *testing.T, observed *w4Dispatches, directWorkIDs []string) {
	t.Helper()
	seenDispatches, seenSessions, seenTopics := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, directWorkID := range directWorkIDs {
		childWorkID := w4ChildWorkID(t, observed, directWorkID)
		for _, entry := range []*w4DispatchObservation{
			w4DispatchForWork(t, observed, "plan", directWorkID),
			w4DispatchForWork(t, observed, "execute", childWorkID),
			w4DispatchForWork(t, observed, "review", childWorkID),
		} {
			topic := string(workersessions.Topic(entry.sessionID))
			if seenDispatches[entry.dispatchID] || seenSessions[entry.sessionID] || seenTopics[topic] {
				t.Fatalf("dispatch/session/topic association is not unique: dispatch=%q session=%q topic=%q", entry.dispatchID, entry.sessionID, topic)
			}
			seenDispatches[entry.dispatchID], seenSessions[entry.sessionID], seenTopics[topic] = true, true, true
		}
	}
}

// assertW4PublishedAssociationsRoundTrip proves the public representation of
// the real root-built recording preserves the exact dispatch-to-Worker-Session
// pairing. It reads the customer replay artifact after Process.Execute, maps
// each association through the generated Factory Event union, then normalizes
// the generated JSON back into a canonical event. This keeps the proof at the
// public replay/read boundary without reaching into Worker Sessions state.
func assertW4PublishedAssociationsRoundTrip(
	t *testing.T,
	observed *w4Dispatches,
	events []interfaces.FactoryEvent,
) {
	t.Helper()

	seen := make(map[string]bool)
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc {
			continue
		}
		if event.Context.DispatchID == nil || *event.Context.DispatchID == "" {
			t.Fatalf("published association %q context.dispatchId = %#v, want non-empty", event.Id, event.Context.DispatchID)
		}
		dispatchID := *event.Context.DispatchID
		observation := observed.byID[dispatchID]
		if observation == nil || observation.sessionID == "" {
			t.Fatalf("published association %q has no observed Worker Session for dispatch %q", event.Id, dispatchID)
		}
		if seen[dispatchID] {
			t.Fatalf("published replay contains duplicate association for dispatch %q", dispatchID)
		}
		assertW4GeneratedAssociationRoundTrip(t, event, observation)
		seen[dispatchID] = true
	}

	for dispatchID, observation := range observed.byID {
		if observation.sessionID != "" && !seen[dispatchID] {
			t.Fatalf("persisted replay omitted published association for dispatch %q", dispatchID)
		}
	}
}

func assertW4GeneratedAssociationRoundTrip(
	t *testing.T,
	event interfaces.FactoryEvent,
	observation *w4DispatchObservation,
) {
	t.Helper()

	dispatchID := observation.dispatchID

	public, err := apisurface.FactoryEventToAPI(event)
	if err != nil {
		t.Fatalf("map persisted association %q to generated Factory Event: %v", event.Id, err)
	}
	if public.Type != factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation ||
		public.Context.DispatchId == nil || *public.Context.DispatchId != dispatchID ||
		public.Context.Sequence != observation.associationSequence {
		t.Fatalf("generated association = %#v, want dispatch %q and sequence %d", public, dispatchID, observation.associationSequence)
	}
	publicPayload, err := public.Payload.AsDispatchWorkerSessionAssociationEventPayload()
	if err != nil {
		t.Fatalf("decode generated association payload %q: %v", public.Id, err)
	}
	if publicPayload.WorkerSessionId != observation.sessionID {
		t.Fatalf(
			"generated association dispatch %q workerSessionId = %q, want actual Worker Session %q",
			dispatchID,
			publicPayload.WorkerSessionId,
			observation.sessionID,
		)
	}
	assertW4NormalizedAssociation(t, public, observation)
}

func assertW4NormalizedAssociation(
	t *testing.T,
	public factoryapi.FactoryEvent,
	observation *w4DispatchObservation,
) {
	t.Helper()

	dispatchID := observation.dispatchID
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal generated association %q: %v", public.Id, err)
	}
	var roundTripped factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal generated association %q: %v", public.Id, err)
	}
	normalized, err := interfaces.NewFactoryEvent(roundTripped)
	if err != nil {
		t.Fatalf("normalize generated association %q: %v", public.Id, err)
	}
	if normalized.Id != observation.associationEventID ||
		normalized.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc ||
		normalized.Context.Sequence != observation.associationSequence ||
		normalized.Context.DispatchID == nil || *normalized.Context.DispatchID != dispatchID {
		t.Fatalf("normalized association = %#v, want event %q and dispatch %q", normalized, observation.associationEventID, dispatchID)
	}
	var normalizedPayload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := normalized.DecodePayload(&normalizedPayload); err != nil {
		t.Fatalf("decode normalized association payload %q: %v", normalized.Id, err)
	}
	if normalizedPayload.WorkerSessionID != observation.sessionID {
		t.Fatalf(
			"normalized association dispatch %q workerSessionId = %q, want actual Worker Session %q",
			dispatchID,
			normalizedPayload.WorkerSessionID,
			observation.sessionID,
		)
	}
}

func w4Contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func w4OutputWorkCount(values []work.WorkRequestEventWork, workID, state string) int {
	count := 0
	for _, value := range values {
		if value.WorkID == workID && value.State != nil && value.State.Name == state {
			count++
		}
	}
	return count
}

type interleavingW4ProviderCommandRunner struct {
	mu              sync.Mutex
	planArrivals    int
	planCompletions []int
	completed       map[string]int
	firstArrived    chan struct{}
	release         chan struct{}
}

func newInterleavingW4ProviderCommandRunner() *interleavingW4ProviderCommandRunner {
	return &interleavingW4ProviderCommandRunner{
		completed:    map[string]int{},
		firstArrived: make(chan struct{}),
		release:      make(chan struct{}),
	}
}

func (r *interleavingW4ProviderCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	stage := w4ProviderStage(req)
	output := "w4 " + stage + " COMPLETE"
	r.mu.Lock()
	if stage == "planner" {
		r.planArrivals++
	}
	planArrival := r.planArrivals
	r.mu.Unlock()
	if stage == "planner" && planArrival == 1 {
		close(r.firstArrived)
		select {
		case <-r.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	if stage == "planner" && planArrival == 2 {
		select {
		case <-r.firstArrived:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	r.mu.Lock()
	r.completed[stage]++
	if stage == "planner" {
		r.planCompletions = append(r.planCompletions, planArrival)
	}
	r.mu.Unlock()
	if stage == "planner" && planArrival == 2 {
		close(r.release)
	}
	return platformprocess.CommandResult{Stdout: w4ProviderStdout(req, output)}, nil
}

func w4ProviderStage(req platformprocess.CommandRequest) string {
	text := string(req.Stdin) + "\n" + strings.Join(req.Args, "\n")
	switch {
	case strings.Contains(text, "Plan workstation"):
		return "planner"
	case strings.Contains(text, "Execute workstation"):
		return "executor"
	case strings.Contains(text, "Review workstation"):
		return "reviewer"
	default:
		return "unexpected provider prompt"
	}
}

func w4ProviderStdout(req platformprocess.CommandRequest, output string) []byte {
	if strings.EqualFold(req.Command, "claude") {
		return support.ClaudeSuccessStdout(output)
	}
	return support.CodexSuccessStdout(output)
}

func (r *interleavingW4ProviderCommandRunner) completedCalls() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	completed := make(map[string]int, len(r.completed))
	for stage, count := range r.completed {
		completed[stage] = count
	}
	return completed
}

func (r *interleavingW4ProviderCommandRunner) firstTwoCompletedOutOfOrder() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.planCompletions) >= 2 && r.planCompletions[0] == 2 && r.planCompletions[1] == 1
}

func (r *interleavingW4ProviderCommandRunner) completedExpectedCalls() bool {
	completed := r.completedCalls()
	return len(completed) == 3 && completed["planner"] == 2 &&
		completed["executor"] == 2 && completed["reviewer"] == 2
}

var _ platformprocess.CommandRunner = (*interleavingW4ProviderCommandRunner)(nil)

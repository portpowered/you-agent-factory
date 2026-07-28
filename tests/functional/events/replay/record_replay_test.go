package replay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	recordReplaySuccessWorkID   = "record-replay-success-work"
	recordReplaySuccessTraceID  = "record-replay-success-trace"
	recordReplaySuccessArtifact = "record-replay-success.replay.json"

	recordReplayFailureWorkID   = "record-replay-failure-work"
	recordReplayFailureTraceID  = "record-replay-failure-trace"
	recordReplayFailureArtifact = "record-replay-failure.replay.json"

	recordReplayTimeoutWorkID   = "record-replay-timeout-work"
	recordReplayTimeoutTraceID  = "record-replay-timeout-trace"
	recordReplayTimeoutArtifact = "record-replay-timeout.replay.json"

	recordReplayDeterministicWorkID   = "record-replay-deterministic-work"
	recordReplayDeterministicTraceID  = "record-replay-deterministic-trace"
	recordReplayDeterministicArtifact = "record-replay-deterministic.replay.json"

	deterministicProviderFailureExit   = 7
	deterministicProviderFailureStderr = "deterministic provider rejection"
)

type recordReplayPublicOutcome struct {
	status    factoryapi.StatusResponse
	events    []factoryapi.FactoryEvent
	artifacts []factoryapi.FactorySessionArtifactSummary
	work      factoryapi.ListWorkResponse
	workByID  factoryapi.Work
}

// TestRecordReplayReproducesSuccessfulPublicOutcome proves a recorded successful
// live factory run replays through the public CLI --replay surface and
// reconstructs the same public success outcome — terminal Factory Session
// status, ordered Factory Event and Factory Artifact summaries, and Work
// result availability — without re-dispatching live workers or contacting
// model providers.
func TestRecordReplayReproducesSuccessfulPublicOutcome(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), recordReplaySuccessArtifact)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     recordReplaySuccessWorkID,
		TraceID:    recordReplaySuccessTraceID,
		Payload:    []byte(`{"title":"record replay successful public outcome"}`),
	})

	recordRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("step one COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("step two COMPLETE")},
	)
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: recordRunner,
		},
	})
	live := observeRecordReplayPublicOutcome(t, recordServer, recordReplaySuccessWorkID)
	assertSuccessfulRecordReplayPublicOutcome(t, live, recordReplaySuccessWorkID)
	if got := recordRunner.CallCount(); got != 2 {
		t.Fatalf("record provider command runner calls = %d, want 2 live dispatches", got)
	}
	recordServer.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	recordedEvents := testutil.GeneratedFactoryEvents(t, artifact.Events)
	if countFactoryEvents(recordedEvents, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("recorded artifact missing dispatch requests")
	}
	if countFactoryEvents(recordedEvents, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("recorded artifact missing dispatch responses")
	}
	recordedArtifacts := factoryArtifactSummariesFromEvents(t, recordedEvents)

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture dir: %v", err)
	}

	replayRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("replay must not invoke providers")},
	)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: replayRunner,
		},
	})
	replayed := observeRecordReplayPublicOutcome(t, replayServer, recordReplaySuccessWorkID)
	assertSuccessfulRecordReplayPublicOutcome(t, replayed, recordReplaySuccessWorkID)
	assertRecordReplayPublicOutcomesMatch(t, live, replayed)
	assertReplayDispatchHistoryMatchesArtifact(t, recordedEvents, replayed.events)
	assertFactoryArtifactSummariesMatch(t, recordedArtifacts, replayed.artifacts)
	if got := replayRunner.CallCount(); got != 0 {
		t.Fatalf("replay provider command runner calls = %d, want 0 without live worker dispatch", got)
	}
	replayServer.Stop(t)
}

// TestRecordReplayReproducesFailureAndLifecycleControls proves recorded failed or
// lifecycle-controlled factory runs replay through the public CLI --replay
// surface and reconstruct the same public terminal or partial outcomes — failed
// Work placement, safe dispatch failure detail, and documented execution-timeout
// lifecycle controls — without re-dispatching live workers or contacting model
// providers.
func TestRecordReplayReproducesFailureAndLifecycleControls(t *testing.T) {
	t.Run("provider dispatch failure", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		artifactPath := filepath.Join(t.TempDir(), recordReplayFailureArtifact)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			WorkID:     recordReplayFailureWorkID,
			TraceID:    recordReplayFailureTraceID,
			Payload:    []byte(`{"title":"record replay provider dispatch failure"}`),
		})

		recordRunner := support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("step one COMPLETE")},
			platformprocess.CommandResult{
				ExitCode: deterministicProviderFailureExit,
				Stderr:   []byte(deterministicProviderFailureStderr),
			},
		)
		recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Args:                      []string{"--record", artifactPath},
			Edges: serviceedges.Edges{
				ProviderCommandRunner: recordRunner,
			},
		})
		live := observeRecordReplayPublicOutcome(t, recordServer, recordReplayFailureWorkID)
		assertFailedRecordReplayPublicOutcome(t, live, recordReplayFailureWorkID)
		assertFailedDispatchResponsePreserved(t, live.events, factoryapi.WorkFailureTypeUnknown)
		if got := recordRunner.CallCount(); got != 2 {
			t.Fatalf("record provider command runner calls = %d, want 2 live dispatches ending in failure", got)
		}
		recordServer.Stop(t)

		recordedEvents := testutil.GeneratedFactoryEvents(t, testutil.LoadReplayArtifact(t, artifactPath).Events)
		recordedArtifacts := factoryArtifactSummariesFromEvents(t, recordedEvents)

		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("remove original fixture dir: %v", err)
		}

		replayRunner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("replay must not invoke providers")},
		)
		replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir: t.TempDir(),
			Args:       []string{"--replay", artifactPath, "--no-record"},
			Edges: serviceedges.Edges{
				ProviderCommandRunner: replayRunner,
			},
		})
		replayed := observeRecordReplayPublicOutcome(t, replayServer, recordReplayFailureWorkID)
		assertFailedRecordReplayPublicOutcome(t, replayed, recordReplayFailureWorkID)
		assertFailedRecordReplayPublicOutcomesMatch(t, live, replayed)
		assertFactoryArtifactSummariesMatch(t, recordedArtifacts, replayed.artifacts)
		assertFailedDispatchResponsePreserved(t, replayed.events, factoryapi.WorkFailureTypeUnknown)
		if got := replayRunner.CallCount(); got != 0 {
			t.Fatalf("replay provider command runner calls = %d, want 0 without live worker dispatch", got)
		}
		replayServer.Stop(t)
	})

	t.Run("execution timeout lifecycle control", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		support.WriteWorkstationConfig(t, dir, "step-two", `---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 10ms
---
Do the work.
`)
		artifactPath := filepath.Join(t.TempDir(), recordReplayTimeoutArtifact)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			WorkID:     recordReplayTimeoutWorkID,
			TraceID:    recordReplayTimeoutTraceID,
			Payload:    []byte(`{"title":"record replay execution timeout lifecycle control"}`),
		})

		recordRunner := newTimeoutLifecycleProviderCommandRunner()
		recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Args:                      []string{"--record", artifactPath},
			Edges: serviceedges.Edges{
				ProviderCommandRunner: recordRunner,
			},
		})
		live := observeRecordReplayPublicOutcome(t, recordServer, recordReplayTimeoutWorkID)
		assertFailedRecordReplayPublicOutcome(t, live, recordReplayTimeoutWorkID)
		assertExecutionTimeoutLifecycleControlPreserved(t, live.events)
		if got := recordRunner.CallCount(); got < 2 {
			t.Fatalf("record provider command runner calls = %d, want step-one success and timed-out step-two", got)
		}
		recordServer.Stop(t)

		recordedEvents := testutil.GeneratedFactoryEvents(t, testutil.LoadReplayArtifact(t, artifactPath).Events)
		recordedArtifacts := factoryArtifactSummariesFromEvents(t, recordedEvents)

		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("remove original fixture dir: %v", err)
		}

		replayRunner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("replay must not invoke providers")},
		)
		replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir: t.TempDir(),
			Args:       []string{"--replay", artifactPath, "--no-record"},
			Edges: serviceedges.Edges{
				ProviderCommandRunner: replayRunner,
			},
		})
		replayed := observeRecordReplayPublicOutcome(t, replayServer, recordReplayTimeoutWorkID)
		assertFailedRecordReplayPublicOutcome(t, replayed, recordReplayTimeoutWorkID)
		assertFailedRecordReplayPublicOutcomesMatch(t, live, replayed)
		assertFactoryArtifactSummariesMatch(t, recordedArtifacts, replayed.artifacts)
		assertExecutionTimeoutLifecycleControlPreserved(t, replayed.events)
		if got := replayRunner.CallCount(); got != 0 {
			t.Fatalf("replay provider command runner calls = %d, want 0 without live worker dispatch", got)
		}
		replayServer.Stop(t)
	})
}

// TestReplayOfSameArtifactIsDeterministic proves two public CLI --replay
// invocations of the same recorded artifact reconstruct identical public
// observations — Factory Session lifecycle status, ordered Factory Event and
// Factory Artifact summaries, and Work result availability — with no drift
// across repeated invocations.
func TestReplayOfSameArtifactIsDeterministic(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), recordReplayDeterministicArtifact)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     recordReplayDeterministicWorkID,
		TraceID:    recordReplayDeterministicTraceID,
		Payload:    []byte(`{"title":"record replay same-artifact determinism"}`),
	})

	recordRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("step one COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("step two COMPLETE")},
	)
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: recordRunner,
		},
	})
	recordOutcome := observeRecordReplayPublicOutcome(t, recordServer, recordReplayDeterministicWorkID)
	assertSuccessfulRecordReplayPublicOutcome(t, recordOutcome, recordReplayDeterministicWorkID)
	if got := recordRunner.CallCount(); got != 2 {
		t.Fatalf("record provider command runner calls = %d, want 2 live dispatches", got)
	}
	recordServer.Stop(t)

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture dir: %v", err)
	}

	firstReplay := replayArtifactPublicOutcome(t, artifactPath, recordReplayDeterministicWorkID)
	secondReplay := replayArtifactPublicOutcome(t, artifactPath, recordReplayDeterministicWorkID)

	assertSuccessfulRecordReplayPublicOutcome(t, firstReplay, recordReplayDeterministicWorkID)
	assertSuccessfulRecordReplayPublicOutcome(t, secondReplay, recordReplayDeterministicWorkID)
	assertDeterministicReplayPublicOutcomesMatch(t, firstReplay, secondReplay)
}

func replayArtifactPublicOutcome(
	t *testing.T,
	artifactPath string,
	workID string,
) recordReplayPublicOutcome {
	t.Helper()

	replayRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("replay must not invoke providers")},
	)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: replayRunner,
		},
	})
	t.Cleanup(func() { replayServer.Stop(t) })
	outcome := observeRecordReplayPublicOutcome(t, replayServer, workID)
	if got := replayRunner.CallCount(); got != 0 {
		t.Fatalf("replay provider command runner calls = %d, want 0 without live worker dispatch", got)
	}
	return outcome
}

func observeRecordReplayPublicOutcome(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workID string,
) recordReplayPublicOutcome {
	t.Helper()

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	status := support.WaitForStatus(t, server.URL(), 10*time.Second, func(response factoryapi.StatusResponse) bool {
		completed := response.Categories.Terminal + response.Categories.Failed
		return completed > 0 &&
			response.Categories.Initial == 0 &&
			response.Categories.Processing == 0
	})
	events := server.GetFactoryEvents(t)
	artifacts := factoryArtifactSummariesFromEvents(t, events)
	work := support.ListDefaultSessionWork(t, server.URL())
	workByID := support.GetDefaultSessionWorkByID(t, server.URL(), workID)
	return recordReplayPublicOutcome{
		status:    status,
		events:    events,
		artifacts: artifacts,
		work:      work,
		workByID:  workByID,
	}
}

func assertSuccessfulRecordReplayPublicOutcome(
	t *testing.T,
	outcome recordReplayPublicOutcome,
	workID string,
) {
	t.Helper()

	if outcome.status.Categories.Terminal == 0 {
		t.Fatalf("terminal work count = %d, want at least one completed work token", outcome.status.Categories.Terminal)
	}
	if outcome.status.Categories.Initial != 0 || outcome.status.Categories.Processing != 0 {
		t.Fatalf(
			"non-terminal work categories = initial:%d processing:%d, want both zero",
			outcome.status.Categories.Initial,
			outcome.status.Categories.Processing,
		)
	}
	if got := support.CountWorkAtCustomerState(outcome.work, "task:complete"); got != 1 {
		t.Fatalf("task:complete token count = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(outcome.work, "task:failed"); got != 0 {
		t.Fatalf("task:failed token count = %d, want 0", got)
	}
	if stringPointerValue(outcome.workByID.WorkId) != workID {
		t.Fatalf("work detail id = %q, want %q", stringPointerValue(outcome.workByID.WorkId), workID)
	}
	if len(outcome.events) == 0 {
		t.Fatal("retained Factory Event history is empty")
	}
	assertFactoryEventsAscendingOrder(t, outcome.events)
	if got := countFactoryEventTypes(outcome.events, factoryapi.FactoryEventTypeDispatchResponse); got == 0 {
		t.Fatal("retained Factory Event history missing dispatch completions")
	}
}

func assertDeterministicReplayPublicOutcomesMatch(
	t *testing.T,
	first, second recordReplayPublicOutcome,
) {
	t.Helper()

	assertRecordReplayPublicOutcomesMatch(t, first, second)
	assertFactoryArtifactSummariesMatch(t, first.artifacts, second.artifacts)
	assertReplayFactoryEventHistoriesMatch(t, first.events, second.events)
}

func assertReplayFactoryEventHistoriesMatch(
	t *testing.T,
	first, second []factoryapi.FactoryEvent,
) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf(
			"Factory Event count = first:%d second:%d, want identical retained history",
			len(first),
			len(second),
		)
	}
	for index := range first {
		firstEvent := first[index]
		secondEvent := second[index]
		if firstEvent.Type != secondEvent.Type {
			t.Fatalf(
				"Factory Event[%d] type = first:%s second:%s, want matching public event kind",
				index,
				firstEvent.Type,
				secondEvent.Type,
			)
		}
		if firstEvent.Context.Sequence != secondEvent.Context.Sequence {
			t.Fatalf(
				"Factory Event[%d] sequence = first:%d second:%d, want matching append-only ordering",
				index,
				firstEvent.Context.Sequence,
				secondEvent.Context.Sequence,
			)
		}
	}
	for _, eventType := range []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeInferenceRequest,
		factoryapi.FactoryEventTypeInferenceResponse,
		factoryapi.FactoryEventTypeArtifactCreated,
	} {
		firstCount := countFactoryEvents(first, eventType)
		if firstCount == 0 {
			continue
		}
		if secondCount := countFactoryEvents(second, eventType); secondCount != firstCount {
			t.Fatalf(
				"%s count = first:%d second:%d, want matching public dispatch and artifact history",
				eventType,
				firstCount,
				secondCount,
			)
		}
	}
	assertFactoryEventsAscendingOrder(t, first)
	assertFactoryEventsAscendingOrder(t, second)
}

func assertRecordReplayPublicOutcomesMatch(t *testing.T, live, replayed recordReplayPublicOutcome) {
	t.Helper()

	if live.status.Categories != replayed.status.Categories {
		t.Fatalf(
			"status categories = live:%#v replay:%#v, want matching terminal public categories",
			live.status.Categories,
			replayed.status.Categories,
		)
	}
	if live.status.RuntimeStatus != replayed.status.RuntimeStatus {
		t.Fatalf(
			"runtime status = live:%q replay:%q, want matching public Factory Session lifecycle status",
			live.status.RuntimeStatus,
			replayed.status.RuntimeStatus,
		)
	}
	assertWorkListingsMatch(t, live.work, replayed.work)
	if workStateName(live.workByID) != workStateName(replayed.workByID) {
		t.Fatalf(
			"work result state = live:%q replay:%q, want matching public result availability",
			workStateName(live.workByID),
			workStateName(replayed.workByID),
		)
	}
	if workStateType(live.workByID) != workStateType(replayed.workByID) {
		t.Fatalf(
			"work result state type = live:%s replay:%s, want matching public terminal classification",
			workStateType(live.workByID),
			workStateType(replayed.workByID),
		)
	}
}

func assertFactoryEventsAscendingOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	previousSequence := -1
	for index, event := range events {
		if event.Context.Sequence < previousSequence {
			t.Fatalf(
				"Factory Event %d (%s) sequence %d precedes previous sequence %d",
				index,
				event.Id,
				event.Context.Sequence,
				previousSequence,
			)
		}
		previousSequence = event.Context.Sequence
	}
}

func assertReplayDispatchHistoryMatchesArtifact(
	t *testing.T,
	recorded,
	replayed []factoryapi.FactoryEvent,
) {
	t.Helper()

	for _, eventType := range []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeInferenceRequest,
		factoryapi.FactoryEventTypeInferenceResponse,
	} {
		recordedCount := countFactoryEvents(recorded, eventType)
		if recordedCount == 0 {
			continue
		}
		if replayCount := countFactoryEvents(replayed, eventType); replayCount != recordedCount {
			t.Fatalf("%s count = recorded:%d replay:%d, want matching public dispatch history", eventType, recordedCount, replayCount)
		}
	}
	assertFactoryEventsAscendingOrder(t, replayed)
}

func assertFactoryArtifactSummariesMatch(
	t *testing.T,
	live,
	replayed []factoryapi.FactorySessionArtifactSummary,
) {
	t.Helper()

	if len(live) != len(replayed) {
		t.Fatalf("Factory Artifact count = live:%d replay:%d, want identical summaries", len(live), len(replayed))
	}
	for index := range live {
		liveArtifact := live[index]
		replayArtifact := replayed[index]
		if liveArtifact.Id != replayArtifact.Id {
			t.Fatalf("artifact[%d] id = live:%q replay:%q", index, liveArtifact.Id, replayArtifact.Id)
		}
		if liveArtifact.Kind != replayArtifact.Kind {
			t.Fatalf("artifact[%d] kind = live:%s replay:%s", index, liveArtifact.Kind, replayArtifact.Kind)
		}
		if stringPointerValue(liveArtifact.Label) != stringPointerValue(replayArtifact.Label) {
			t.Fatalf(
				"artifact[%d] label = live:%q replay:%q",
				index,
				stringPointerValue(liveArtifact.Label),
				stringPointerValue(replayArtifact.Label),
			)
		}
	}
}

func factoryArtifactSummariesFromEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) []factoryapi.FactorySessionArtifactSummary {
	t.Helper()

	summaries := make([]factoryapi.FactorySessionArtifactSummary, 0)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeArtifactCreated {
			continue
		}
		payload, err := event.Payload.AsArtifactCreatedEventPayload()
		if err != nil {
			t.Fatalf("decode ARTIFACT_CREATED payload for event %s: %v", event.Id, err)
		}
		summaries = append(summaries, factoryapi.FactorySessionArtifactSummary{
			Id:    payload.Artifact.Id,
			Kind:  payload.Artifact.Kind,
			Label: payload.Artifact.Label,
		})
	}
	return summaries
}

func assertWorkListingsMatch(t *testing.T, live, replayed factoryapi.ListWorkResponse) {
	t.Helper()

	if len(live.Results) != len(replayed.Results) {
		t.Fatalf("work listing count = live:%d replay:%d, want identical public Work inventory", len(live.Results), len(replayed.Results))
	}
	liveByID := map[string]factoryapi.Work{}
	for _, item := range live.Results {
		workID := stringPointerValue(item.WorkId)
		if workID == "" {
			t.Fatalf("live work listing includes empty work id: %#v", item)
		}
		liveByID[workID] = item
	}
	for _, item := range replayed.Results {
		workID := stringPointerValue(item.WorkId)
		liveItem, ok := liveByID[workID]
		if !ok {
			t.Fatalf("replayed work %q missing from live listing: %#v", workID, replayed.Results)
		}
		if workStateName(liveItem) != workStateName(item) {
			t.Fatalf(
				"work %s state = live:%q replay:%q, want matching public terminal state",
				workID,
				workStateName(liveItem),
				workStateName(item),
			)
		}
	}
}

func countFactoryEvents(events []factoryapi.FactoryEvent, kind factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}

func countFactoryEventTypes(events []factoryapi.FactoryEvent, kind factoryapi.FactoryEventType) int {
	return countFactoryEvents(events, kind)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func workStateName(item factoryapi.Work) string {
	if item.State == nil {
		return ""
	}
	return item.State.Name
}

func workStateType(item factoryapi.Work) factoryapi.WorkStateType {
	if item.State == nil {
		return ""
	}
	return item.State.Type
}

func assertFailedRecordReplayPublicOutcome(t *testing.T, outcome recordReplayPublicOutcome, workID string) {
	t.Helper()

	if outcome.status.Categories.Failed == 0 {
		t.Fatalf("failed work count = %d, want at least one failed work token", outcome.status.Categories.Failed)
	}
	if outcome.status.Categories.Initial != 0 || outcome.status.Categories.Processing != 0 {
		t.Fatalf(
			"non-terminal work categories = initial:%d processing:%d, want both zero",
			outcome.status.Categories.Initial,
			outcome.status.Categories.Processing,
		)
	}
	if got := support.CountWorkAtCustomerState(outcome.work, "task:failed"); got != 1 {
		t.Fatalf("task:failed token count = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(outcome.work, "task:complete"); got != 0 {
		t.Fatalf("task:complete token count = %d, want 0", got)
	}
	if stringPointerValue(outcome.workByID.WorkId) != workID {
		t.Fatalf("work detail id = %q, want %q", stringPointerValue(outcome.workByID.WorkId), workID)
	}
	if workStateType(outcome.workByID) != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("work result state type = %s, want FAILED public terminal classification", workStateType(outcome.workByID))
	}
	if workStateName(outcome.workByID) != "failed" {
		t.Fatalf("work result state = %q, want failed public placement", workStateName(outcome.workByID))
	}
	if len(outcome.events) == 0 {
		t.Fatal("retained Factory Event history is empty")
	}
	assertFactoryEventsAscendingOrder(t, outcome.events)
}

func assertFailedRecordReplayPublicOutcomesMatch(t *testing.T, live, replayed recordReplayPublicOutcome) {
	t.Helper()

	assertRecordReplayPublicOutcomesMatch(t, live, replayed)
	assertFailedDispatchResponsesMatch(t, live.events, replayed.events)
}

func assertFailedDispatchResponsePreserved(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantReason factoryapi.WorkFailureType,
) {
	t.Helper()

	response := lastFailedDispatchResponse(t, events)
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.Output != nil {
		t.Fatalf("dispatch output = %#v, want no primary result on failed replay", response.Output)
	}
	if response.FailureDetail == nil {
		t.Fatal("dispatch FailureDetail missing for failed replay observation")
	}
	if !failureReasonMatches(wantReason, response.FailureDetail.Reason) {
		t.Fatalf(
			"failure reason = %s, want %s safe failure classification",
			response.FailureDetail.Reason,
			wantReason,
		)
	}
	if strings.TrimSpace(response.FailureDetail.Message) == "" {
		t.Fatal("dispatch FailureDetail message is empty, want safe failure detail")
	}
}

func assertFailedDispatchResponsesMatch(t *testing.T, live, replayed []factoryapi.FactoryEvent) {
	t.Helper()

	liveResponse := lastFailedDispatchResponse(t, live)
	replayResponse := lastFailedDispatchResponse(t, replayed)
	if liveResponse.Outcome != replayResponse.Outcome {
		t.Fatalf("dispatch outcome = live:%s replay:%s, want matching failed public outcome", liveResponse.Outcome, replayResponse.Outcome)
	}
	if liveResponse.FailureDetail == nil || replayResponse.FailureDetail == nil {
		t.Fatal("dispatch FailureDetail missing on live or replayed failed observation")
	}
	if strings.TrimSpace(liveResponse.FailureDetail.Message) == "" ||
		strings.TrimSpace(replayResponse.FailureDetail.Message) == "" {
		t.Fatal("dispatch FailureDetail message missing on live or replayed failed observation")
	}
	if failureReasonMatches(liveResponse.FailureDetail.Reason, replayResponse.FailureDetail.Reason) {
		return
	}
	if liveResponse.Error == nil || replayResponse.Error == nil {
		t.Fatal("dispatch error missing on live or replayed failed observation")
	}
	if !dispatchFailureErrorsMatch(liveResponse.Error, replayResponse.Error) {
		t.Fatalf(
			"failure reason = live:%s replay:%s error = live:%#v replay:%#v, want matching public failure classification",
			liveResponse.FailureDetail.Reason,
			replayResponse.FailureDetail.Reason,
			liveResponse.Error,
			replayResponse.Error,
		)
	}
}

func isTimeoutLifecycleFailureReason(reason factoryapi.WorkFailureType) bool {
	return reason == factoryapi.WorkFailureTypeTimeout ||
		reason == factoryapi.WorkFailureTypeInternalServerError
}

func isProviderDispatchFailureReason(reason factoryapi.WorkFailureType) bool {
	return reason == factoryapi.WorkFailureTypeUnknown ||
		reason == factoryapi.WorkFailureTypeInternalServerError
}

func failureReasonMatches(want, got factoryapi.WorkFailureType) bool {
	if want == got {
		return true
	}
	if isProviderDispatchFailureReason(want) && isProviderDispatchFailureReason(got) {
		return true
	}
	if isTimeoutLifecycleFailureReason(want) && isTimeoutLifecycleFailureReason(got) {
		return true
	}
	return false
}

func dispatchFailureErrorsMatch(live, replay *string) bool {
	if live == nil || replay == nil {
		return false
	}
	liveText := strings.ToLower(*live)
	replayText := strings.ToLower(*replay)
	if strings.HasPrefix(liveText, "provider error:") && strings.HasPrefix(replayText, "provider error:") {
		return true
	}
	for _, fragment := range []string{"timeout", "deadline exceeded", "execution timeout"} {
		if strings.Contains(liveText, fragment) && strings.Contains(replayText, fragment) {
			return true
		}
	}
	return liveText == replayText
}

func assertExecutionTimeoutLifecycleControlPreserved(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	response := lastFailedDispatchResponse(t, events)
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED timeout lifecycle control", response.Outcome)
	}
	if response.FailureDetail == nil {
		t.Fatal("dispatch FailureDetail missing for timeout lifecycle control")
	}
	if response.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout &&
		response.FailureDetail.Reason != factoryapi.WorkFailureTypeInternalServerError {
		t.Fatalf(
			"failure reason = %s, want TIMEOUT or documented timeout-adjacent lifecycle classification",
			response.FailureDetail.Reason,
		)
	}
	if response.Error == nil {
		t.Fatal("dispatch error missing for timeout lifecycle control")
	}
	if !strings.Contains(*response.Error, "timeout") &&
		!strings.Contains(*response.Error, "deadline exceeded") {
		t.Fatalf(
			"dispatch error = %q, want timeout lifecycle detail",
			*response.Error,
		)
	}
}

func lastFailedDispatchResponse(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.DispatchResponseEventPayload {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	for index := len(dispatches) - 1; index >= 0; index-- {
		response := dispatches[index].Response
		if response != nil && response.Outcome == factoryapi.WorkOutcomeFailed {
			return *response
		}
	}
	t.Fatal("factory events missing failed dispatch response")
	return factoryapi.DispatchResponseEventPayload{}
}

type timeoutLifecycleProviderCommandRunner struct {
	mu        sync.Mutex
	callCount int
	success   *support.ShapedProviderCommandRunner
}

func newTimeoutLifecycleProviderCommandRunner() *timeoutLifecycleProviderCommandRunner {
	return &timeoutLifecycleProviderCommandRunner{
		success: support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("step one COMPLETE")},
		),
	}
}

func (r *timeoutLifecycleProviderCommandRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		return r.success.Run(ctx, req)
	}

	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

func (r *timeoutLifecycleProviderCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

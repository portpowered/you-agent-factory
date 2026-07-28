package replay

import (
	"os"
	"path/filepath"
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
	assertSuccessfulRecordReplayPublicOutcome(t, live)
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
	assertSuccessfulRecordReplayPublicOutcome(t, replayed)
	assertRecordReplayPublicOutcomesMatch(t, live, replayed)
	assertReplayDispatchHistoryMatchesArtifact(t, recordedEvents, replayed.events)
	assertFactoryArtifactSummariesMatch(t, recordedArtifacts, replayed.artifacts)
	if got := replayRunner.CallCount(); got != 0 {
		t.Fatalf("replay provider command runner calls = %d, want 0 without live worker dispatch", got)
	}
	replayServer.Stop(t)
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

func assertSuccessfulRecordReplayPublicOutcome(t *testing.T, outcome recordReplayPublicOutcome) {
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
	if stringPointerValue(outcome.workByID.WorkId) != recordReplaySuccessWorkID {
		t.Fatalf("work detail id = %q, want %q", stringPointerValue(outcome.workByID.WorkId), recordReplaySuccessWorkID)
	}
	if len(outcome.events) == 0 {
		t.Fatal("retained Factory Event history is empty")
	}
	assertFactoryEventsAscendingOrder(t, outcome.events)
	if got := countFactoryEventTypes(outcome.events, factoryapi.FactoryEventTypeDispatchResponse); got == 0 {
		t.Fatal("retained Factory Event history missing dispatch completions")
	}
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

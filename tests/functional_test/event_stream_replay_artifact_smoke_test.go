package functional_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/internal/testpath"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"go.uber.org/zap"
)

func TestEventStreamReplayArtifactSmoke_ConvertsAgentFailsLogAndReplays(t *testing.T) {
	eventStreamPath := testpath.MustRepoPathFromCaller(t, 0, "factory", "logs", "agent-fails.json")
	if _, err := os.Stat(eventStreamPath); err != nil {
		t.Skipf("root event-stream fixture not present in this checkout: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")

	result, err := replay.SaveArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("SaveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents < 1000 {
		t.Fatalf("ParsedEvents = %d, want recovered event stream", result.ParsedEvents)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, t.TempDir(), "", artifact)

	h := testutil.AssertReplaySucceeds(t, artifactPath, 30*time.Second)
	if h.Artifact == nil {
		t.Fatal("replay harness artifact = nil, want loaded replay artifact")
	}
}

func TestEventStreamReplayArtifactSmoke_ReplaysWithCopiedRootFactoryDefinition(t *testing.T) {
	rootFactoryDir := testpath.MustRepoPathFromCaller(t, 0, "factory")
	eventStreamPath := filepath.Join(rootFactoryDir, "logs", "agent-fails.json")
	if _, err := os.Stat(eventStreamPath); err != nil {
		t.Skipf("root event-stream fixture not present in this checkout: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")
	copiedFactoryDir := testutil.CopyFixtureDir(t, rootFactoryDir)

	result, err := replay.SaveArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("SaveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents < 1000 {
		t.Fatalf("ParsedEvents = %d, want recovered event stream", result.ParsedEvents)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, copiedFactoryDir, copiedFactoryDir, artifact)

	h := testutil.AssertReplaySucceeds(
		t,
		artifactPath,
		30*time.Second,
		testutil.WithReplayHarnessDir(copiedFactoryDir),
		testutil.WithReplayHarnessServiceOptions(
			testutil.WithExecutionBaseDir(copiedFactoryDir),
		),
	)
	if h.Artifact == nil {
		t.Fatal("replay harness artifact = nil, want loaded replay artifact")
	}
}

func TestEventStreamReplayArtifactSmoke_ReplaysCheckedInSampleArtifactWithCopiedRootFactoryDefinition(t *testing.T) {
	rootFactoryDir := testpath.MustRepoPathFromCaller(t, 0, "factory")
	copiedFactoryDir := testutil.CopyFixtureDir(t, rootFactoryDir)
	artifactPath := filepath.Join(copiedFactoryDir, "logs", "agent-fails.replay.json")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Skipf("root replay artifact not present in this checkout: %v", err)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, copiedFactoryDir, copiedFactoryDir, artifact)

	h := testutil.AssertReplaySucceeds(
		t,
		artifactPath,
		30*time.Second,
		testutil.WithReplayHarnessDir(copiedFactoryDir),
		testutil.WithReplayHarnessServiceOptions(
			testutil.WithExecutionBaseDir(copiedFactoryDir),
		),
	)
	if h.Artifact == nil {
		t.Fatal("replay harness artifact = nil, want loaded replay artifact")
	}
}

func assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(
	t *testing.T,
	factoryDir string,
	executionBaseDir string,
	artifact *interfaces.ReplayArtifact,
) {
	t.Helper()

	artifactPath := filepath.Join(t.TempDir(), "streamed-agent-fails.replay.json")
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact for SSE mirror: %v", err)
	}

	server := StartFunctionalServerWithConfig(t, factoryDir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.ReplayPath = artifactPath
		cfg.ExecutionBaseDir = executionBaseDir
		cfg.Logger = zap.NewNop()
	})

	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)
	streamedEvents := collectUnifiedSmokeEventsUntilRunResponse(
		t,
		stream,
		[]factoryapi.FactoryEvent{runStarted, first},
		30*time.Second,
	)
	stream.close()

	select {
	case <-server.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replay server run to finish")
	}

	runtimeEvents := assertReplayEventTimelineMatchesRuntime(t, server, streamedEvents)
	assertReplayWorldStateMatchesRuntime(t, server, streamedEvents, runtimeEvents)
	assertReplayRecordedWorkGraphMatchesArtifact(t, streamedEvents, artifact.Events)
}

func assertReplayEventTimelineMatchesRuntime(
	t *testing.T,
	server *FunctionalServer,
	streamedEvents []factoryapi.FactoryEvent,
) []factoryapi.FactoryEvent {
	t.Helper()

	runtimeEvents, err := server.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("get runtime factory events: %v", err)
	}
	runResponseIndex := lastIndexOfFunctionalEventType(runtimeEvents, factoryapi.FactoryEventTypeRunResponse)
	if runResponseIndex < 0 {
		t.Fatalf("runtime event history missing RUN_RESPONSE: %#v", unifiedSmokeEventSummaries(runtimeEvents))
	}
	runtimePrefix := runtimeEvents[:runResponseIndex+1]

	if len(streamedEvents) != len(runtimePrefix) {
		t.Fatalf(
			"streamed event count = %d, want runtime /events mirror count %d through RUN_RESPONSE",
			len(streamedEvents),
			len(runtimePrefix),
		)
	}

	for i := range runtimePrefix {
		streamed := streamedEvents[i]
		runtimeEvent := runtimePrefix[i]
		if streamed.Id != runtimeEvent.Id ||
			streamed.Type != runtimeEvent.Type ||
			streamed.Context.Tick != runtimeEvent.Context.Tick ||
			streamed.Context.Sequence != runtimeEvent.Context.Sequence ||
			stringPointerValue(streamed.Context.DispatchId) != stringPointerValue(runtimeEvent.Context.DispatchId) ||
			stringPointerValue(streamed.Context.RequestId) != stringPointerValue(runtimeEvent.Context.RequestId) ||
			!reflect.DeepEqual(sliceValue(streamed.Context.TraceIds), sliceValue(runtimeEvent.Context.TraceIds)) ||
			!reflect.DeepEqual(sliceValue(streamed.Context.WorkIds), sliceValue(runtimeEvent.Context.WorkIds)) {
			t.Fatalf(
				"streamed event[%d] = id=%q type=%s tick=%d seq=%d dispatch=%q request=%q trace_ids=%#v work_ids=%#v; want runtime event id=%q type=%s tick=%d seq=%d dispatch=%q request=%q trace_ids=%#v work_ids=%#v",
				i,
				streamed.Id,
				streamed.Type,
				streamed.Context.Tick,
				streamed.Context.Sequence,
				stringPointerValue(streamed.Context.DispatchId),
				stringPointerValue(streamed.Context.RequestId),
				sliceValue(streamed.Context.TraceIds),
				sliceValue(streamed.Context.WorkIds),
				runtimeEvent.Id,
				runtimeEvent.Type,
				runtimeEvent.Context.Tick,
				runtimeEvent.Context.Sequence,
				stringPointerValue(runtimeEvent.Context.DispatchId),
				stringPointerValue(runtimeEvent.Context.RequestId),
				sliceValue(runtimeEvent.Context.TraceIds),
				sliceValue(runtimeEvent.Context.WorkIds),
			)
		}
	}

	return append([]factoryapi.FactoryEvent(nil), runtimePrefix...)
}

func assertReplayWorldStateMatchesRuntime(
	t *testing.T,
	server *FunctionalServer,
	streamedEvents []factoryapi.FactoryEvent,
	runtimeEvents []factoryapi.FactoryEvent,
) {
	t.Helper()

	streamedTick := maxUnifiedSmokeTick(streamedEvents)
	streamedState, err := projections.ReconstructFactoryWorldState(streamedEvents, streamedTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState streamed events: %v", err)
	}

	runtimeTick := maxUnifiedSmokeTick(runtimeEvents)
	runtimeState, err := projections.ReconstructFactoryWorldState(runtimeEvents, runtimeTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState runtime events: %v", err)
	}

	streamedCanonical, err := canonicalizeFunctionalFactoryWorldState(streamedState)
	if err != nil {
		t.Fatalf("canonicalize streamed world state: %v", err)
	}
	runtimeCanonical, err := canonicalizeFunctionalFactoryWorldState(runtimeState)
	if err != nil {
		t.Fatalf("canonicalize runtime world state: %v", err)
	}

	if !reflect.DeepEqual(streamedCanonical, runtimeCanonical) {
		t.Fatalf("streamed reconstructed world state did not match runtime event history world state")
	}

	snapshot := server.GetEngineStateSnapshot(t)
	if snapshot.TickCount != streamedTick {
		t.Fatalf("engine snapshot tick = %d, want %d from streamed /events", snapshot.TickCount, streamedTick)
	}
	if snapshot.FactoryState != streamedState.FactoryState {
		t.Fatalf("engine snapshot factory state = %q, want %q from streamed /events", snapshot.FactoryState, streamedState.FactoryState)
	}

	streamedView := projections.BuildFactoryWorldView(streamedState)
	dashboard := server.GetDashboard(t)
	if dashboard.FactoryState != streamedState.FactoryState {
		t.Fatalf("dashboard factory state = %q, want %q from streamed world state", dashboard.FactoryState, streamedState.FactoryState)
	}
	if dashboard.TickCount != streamedTick {
		t.Fatalf("dashboard tick = %d, want %d from streamed /events", dashboard.TickCount, streamedTick)
	}
	if dashboard.Runtime.InFlightDispatchCount != streamedView.Runtime.InFlightDispatchCount {
		t.Fatalf(
			"dashboard in-flight dispatch count = %d, want %d from streamed world view",
			dashboard.Runtime.InFlightDispatchCount,
			streamedView.Runtime.InFlightDispatchCount,
		)
	}
	if dashboard.Runtime.Session.CompletedCount != streamedView.Runtime.Session.CompletedCount {
		t.Fatalf(
			"dashboard completed count = %d, want %d from streamed world view",
			dashboard.Runtime.Session.CompletedCount,
			streamedView.Runtime.Session.CompletedCount,
		)
	}
	if dashboard.Runtime.Session.DispatchedCount != streamedView.Runtime.Session.DispatchedCount {
		t.Fatalf(
			"dashboard dispatched count = %d, want %d from streamed world view",
			dashboard.Runtime.Session.DispatchedCount,
			streamedView.Runtime.Session.DispatchedCount,
		)
	}
	if dashboard.Runtime.Session.FailedCount != streamedView.Runtime.Session.FailedCount {
		t.Fatalf(
			"dashboard failed count = %d, want %d from streamed world view",
			dashboard.Runtime.Session.FailedCount,
			streamedView.Runtime.Session.FailedCount,
		)
	}
}

func assertReplayRecordedWorkGraphMatchesArtifact(
	t *testing.T,
	streamedEvents []factoryapi.FactoryEvent,
	recordedEvents []factoryapi.FactoryEvent,
) {
	t.Helper()

	streamedTick := maxUnifiedSmokeTick(streamedEvents)
	streamedState, err := projections.ReconstructFactoryWorldState(streamedEvents, streamedTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState streamed events: %v", err)
	}

	recordedTick := maxUnifiedSmokeTick(recordedEvents)
	recordedState, err := projections.ReconstructFactoryWorldState(recordedEvents, recordedTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState recorded events: %v", err)
	}

	if !reflect.DeepEqual(streamedState.WorkRequestsByID, recordedState.WorkRequestsByID) {
		t.Fatalf("replayed work-request graph did not match recorded artifact work requests")
	}
	if !reflect.DeepEqual(streamedState.RelationsByWorkID, recordedState.RelationsByWorkID) {
		t.Fatalf("replayed work-request relations did not match recorded artifact relations")
	}
	if !reflect.DeepEqual(sortedFactoryWorldTraceIDs(streamedState), sortedFactoryWorldTraceIDs(recordedState)) {
		t.Fatalf("replayed trace ID set did not match recorded artifact trace IDs")
	}
}

func sortedFactoryWorldTraceIDs(state interfaces.FactoryWorldState) []string {
	traceIDs := make([]string, 0, len(state.TracesByID))
	for traceID := range state.TracesByID {
		traceIDs = append(traceIDs, traceID)
	}
	sort.Strings(traceIDs)
	return traceIDs
}

func canonicalizeFunctionalFactoryWorldState(state interfaces.FactoryWorldState) (interfaces.FactoryWorldState, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	var canonical interfaces.FactoryWorldState
	if err := json.Unmarshal(data, &canonical); err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	return canonical, nil
}

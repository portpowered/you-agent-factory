package replay_contracts

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestReplayEventStreamArtifactSmoke_ConvertsAgentFailsLogAndReplays(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay artifact conversion smoke")

	eventStreamPath := testutil.MustClassifiedArtifactPath(t, "factory/logs/agent-fails.json", testutil.ArtifactCheckedIn)
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")

	result, err := replay.SaveArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("SaveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents < 100 {
		t.Fatalf("ParsedEvents = %d, want recovered event stream", result.ParsedEvents)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, t.TempDir(), "", artifact)

	h := testutil.AssertReplaySucceeds(t, artifactPath, 30*time.Second)
	if h.Artifact == nil {
		t.Fatal("replay harness artifact = nil, want loaded replay artifact")
	}
}

func TestReplayEventStreamArtifactSmoke_ReplaysWithCopiedRootFactoryDefinition(t *testing.T) {
	adhocFactoryDir := testutil.MustRepoPath(t, "tests/adhoc/factory")
	eventStreamPath := testutil.MustClassifiedArtifactPath(t, "factory/logs/agent-fails.json", testutil.ArtifactCheckedIn)
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")
	copiedFactoryDir := testutil.CopyFixtureDir(t, adhocFactoryDir)

	result, err := replay.SaveArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("SaveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents < 100 {
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

func TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifactWithCopiedRootFactoryDefinition(t *testing.T) {
	copiedFactoryDir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	artifactPath := testutil.MustClassifiedArtifactPath(t, "factory/logs/agent-fails.replay.json", testutil.ArtifactCheckedIn)

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

	server := startReplayFunctionalServerWithConfig(t, factoryDir, false, func(cfg *service.FactoryServiceConfig) {
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
}

func assertReplayEventTimelineMatchesRuntime(
	t *testing.T,
	server *replayFunctionalServer,
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
	server *replayFunctionalServer,
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

	streamedCanonical, err := canonicalizeReplayFactoryWorldState(streamedState)
	if err != nil {
		t.Fatalf("canonicalize streamed world state: %v", err)
	}
	runtimeCanonical, err := canonicalizeReplayFactoryWorldState(runtimeState)
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

func canonicalizeReplayFactoryWorldState(state interfaces.FactoryWorldState) (interfaces.FactoryWorldState, error) {
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

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

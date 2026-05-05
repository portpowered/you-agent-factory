//go:build functionallong

package replay_contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestReplayEventStreamArtifactSmoke_ConvertsAgentFailsLogAndReplays(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay artifact conversion smoke")

	eventStreamPath := testutil.MustClassifiedArtifactPath(t, "factory/logs/agent-fails.json", testutil.ArtifactCheckedIn)
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")

	result, err := saveReplayArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("saveReplayArtifactFromEventStreamFile: %v", err)
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
	support.SkipLongFunctional(t, "slow replay artifact root-factory mirroring sweep")

	adhocFactoryDir := testutil.MustRepoPath(t, "tests/adhoc/factory")
	eventStreamPath := testutil.MustClassifiedArtifactPath(t, "factory/logs/agent-fails.json", testutil.ArtifactCheckedIn)
	artifactPath := filepath.Join(t.TempDir(), "agent-fails.replay.json")
	copiedFactoryDir := testutil.CopyFixtureDir(t, adhocFactoryDir)

	result, err := saveReplayArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("saveReplayArtifactFromEventStreamFile: %v", err)
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

func saveReplayArtifactFromEventStreamFile(
	eventStreamPath string,
	artifactPath string,
) (*replay.EventStreamArtifactResult, error) {
	result, err := replayArtifactFromEventStreamFile(eventStreamPath)
	if err != nil {
		return nil, err
	}
	if err := replay.Save(artifactPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("save replay artifact from event stream %q: %w", eventStreamPath, err)
	}
	return result, nil
}

func replayArtifactFromEventStreamFile(eventStreamPath string) (*replay.EventStreamArtifactResult, error) {
	file, err := os.Open(eventStreamPath)
	if err != nil {
		return nil, fmt.Errorf("open event stream %q: %w", eventStreamPath, err)
	}
	defer file.Close()

	result, err := replay.ArtifactFromEventStream(file)
	if err != nil {
		return nil, fmt.Errorf("parse event stream %q: %w", eventStreamPath, err)
	}
	if err := hydrateReplayArtifactFromAdjacentFactory(eventStreamPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("hydrate replay artifact from adjacent factory for %q: %w", eventStreamPath, err)
	}
	return result, nil
}

func hydrateReplayArtifactFromAdjacentFactory(eventStreamPath string, artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return nil
	}
	factoryDir, ok := adjacentFactoryDir(eventStreamPath)
	if !ok {
		return nil
	}
	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		return nil
	}
	generated, err := replay.GeneratedFactoryFromRuntimeConfig(
		loaded.FactoryDir(),
		loaded.FactoryConfig(),
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		return nil
	}
	merged := mergeGeneratedFactoryMissingRuntimeFields(artifact.Factory, generated)
	if err := rewriteArtifactFactoryEvents(artifact, merged); err != nil {
		return err
	}
	artifact.Factory = merged
	return nil
}

func adjacentFactoryDir(eventStreamPath string) (string, bool) {
	candidates := []string{
		filepath.Dir(eventStreamPath),
		filepath.Dir(filepath.Dir(eventStreamPath)),
	}
	for _, dir := range candidates {
		if dir == "" || dir == "." {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, interfaces.FactoryConfigFile)); err == nil {
			return dir, true
		}
	}
	return "", false
}

func mergeGeneratedFactoryMissingRuntimeFields(recorded factoryapi.Factory, authored factoryapi.Factory) factoryapi.Factory {
	merged := recorded
	if merged.FactoryDirectory == nil {
		merged.FactoryDirectory = authored.FactoryDirectory
	}
	if merged.SourceDirectory == nil {
		merged.SourceDirectory = authored.SourceDirectory
	}
	if merged.Id == nil {
		merged.Id = authored.Id
	}
	if merged.Metadata == nil || len(*merged.Metadata) == 0 {
		merged.Metadata = authored.Metadata
	}
	if merged.InputTypes == nil || len(*merged.InputTypes) == 0 {
		merged.InputTypes = authored.InputTypes
	}
	if merged.Workers != nil && authored.Workers != nil {
		authoredByName := make(map[string]factoryapi.Worker, len(*authored.Workers))
		for _, worker := range *authored.Workers {
			authoredByName[worker.Name] = worker
		}
		for i := range *merged.Workers {
			worker := &(*merged.Workers)[i]
			authoredWorker, ok := authoredByName[worker.Name]
			if !ok {
				continue
			}
			if worker.Type == nil {
				worker.Type = authoredWorker.Type
			}
			if worker.Command == nil {
				worker.Command = authoredWorker.Command
			}
			if worker.Args == nil || len(*worker.Args) == 0 {
				worker.Args = authoredWorker.Args
			}
			if worker.ModelProvider == nil {
				worker.ModelProvider = authoredWorker.ModelProvider
			}
			if worker.ExecutorProvider == nil {
				worker.ExecutorProvider = authoredWorker.ExecutorProvider
			}
			if worker.Timeout == nil {
				worker.Timeout = authoredWorker.Timeout
			}
			if worker.StopToken == nil {
				worker.StopToken = authoredWorker.StopToken
			}
			if worker.SkipPermissions == nil {
				worker.SkipPermissions = authoredWorker.SkipPermissions
			}
			if worker.Body == nil {
				worker.Body = authoredWorker.Body
			}
			if worker.Resources == nil || len(*worker.Resources) == 0 {
				worker.Resources = authoredWorker.Resources
			}
		}
	}
	if merged.Workstations != nil && authored.Workstations != nil {
		authoredByName := make(map[string]factoryapi.Workstation, len(*authored.Workstations))
		for _, workstation := range *authored.Workstations {
			authoredByName[workstation.Name] = workstation
		}
		for i := range *merged.Workstations {
			workstation := &(*merged.Workstations)[i]
			authoredWorkstation, ok := authoredByName[workstation.Name]
			if !ok {
				continue
			}
			if workstation.Id == nil {
				workstation.Id = authoredWorkstation.Id
			}
			if workstation.Behavior == nil {
				workstation.Behavior = authoredWorkstation.Behavior
			}
			if workstation.Type == nil {
				workstation.Type = authoredWorkstation.Type
			}
			if workstation.Worker == "" {
				workstation.Worker = authoredWorkstation.Worker
			}
			if len(workstation.Inputs) == 0 {
				workstation.Inputs = authoredWorkstation.Inputs
			}
			if len(workstation.Outputs) == 0 {
				workstation.Outputs = authoredWorkstation.Outputs
			}
			if workstation.OnFailure == nil {
				workstation.OnFailure = authoredWorkstation.OnFailure
			}
			if workstation.OnContinue == nil {
				workstation.OnContinue = authoredWorkstation.OnContinue
			}
			if workstation.OnRejection == nil {
				workstation.OnRejection = authoredWorkstation.OnRejection
			}
			if workstation.Resources == nil || len(*workstation.Resources) == 0 {
				workstation.Resources = authoredWorkstation.Resources
			}
			if workstation.Cron == nil {
				workstation.Cron = authoredWorkstation.Cron
			}
			if workstation.Guards == nil || len(*workstation.Guards) == 0 {
				workstation.Guards = authoredWorkstation.Guards
			}
			if workstation.Limits == nil {
				workstation.Limits = authoredWorkstation.Limits
			}
			if workstation.Worktree == nil {
				workstation.Worktree = authoredWorkstation.Worktree
			}
			if workstation.WorkingDirectory == nil {
				workstation.WorkingDirectory = authoredWorkstation.WorkingDirectory
			}
			if workstation.PromptFile == nil {
				workstation.PromptFile = authoredWorkstation.PromptFile
			}
			if workstation.Body == nil {
				workstation.Body = authoredWorkstation.Body
			}
			if workstation.StopWords == nil || len(*workstation.StopWords) == 0 {
				workstation.StopWords = authoredWorkstation.StopWords
			}
		}
	}
	return merged
}

func rewriteArtifactFactoryEvents(artifact *interfaces.ReplayArtifact, factory factoryapi.Factory) error {
	if artifact == nil {
		return nil
	}
	for index := range artifact.Events {
		event := &artifact.Events[index]
		switch event.Type {
		case factoryapi.FactoryEventTypeRunRequest:
			payload, err := event.Payload.AsRunRequestEventPayload()
			if err != nil {
				return fmt.Errorf("decode run request event %q: %w", event.Id, err)
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromRunRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite run request factory payload: %w", err)
			}
			event.Payload = union
		case factoryapi.FactoryEventTypeInitialStructureRequest:
			payload, err := event.Payload.AsInitialStructureRequestEventPayload()
			if err != nil {
				return fmt.Errorf("decode initial structure event %q: %w", event.Id, err)
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromInitialStructureRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite initial structure factory payload: %w", err)
			}
			event.Payload = union
		}
	}
	return nil
}

func TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifactWithCopiedRootFactoryDefinition(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay checked-in sample artifact sweep")

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

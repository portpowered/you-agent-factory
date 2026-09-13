package runtimeopening

import (
	"encoding/json"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

type canonicalProjectionReplayInputLoader struct {
	artifact         *recordings.ReplayArtifact
	state            recordings.FactoryWorldState
	loadCalls        int
	reconstructTicks []int
}

func (loader *canonicalProjectionReplayInputLoader) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	loader.loadCalls++
	return recordings.LoadReplayInputResult{Legacy: loader.artifact}, nil
}

func (loader *canonicalProjectionReplayInputLoader) ReconstructCanonicalFactoryWorldState(
	_ []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	loader.reconstructTicks = append(loader.reconstructTicks, selectedTick)
	return loader.state, nil
}

func TestLoadRuntimeRoutesStructurallyValidLegacyReplayToDetachedProjection(t *testing.T) {
	t.Parallel()

	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "legacy-history"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	eventTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	artifact := &recordings.ReplayArtifact{
		SchemaVersion: "legacy", Factory: factorySnapshot,
		Events: []recordings.FactoryEvent{{
			SchemaVersion: recordings.FactoryEventSchemaVersionV1,
			Id:            "legacy-event-1", Type: recordings.FactoryEventTypeFactoryStateResponse,
			Context: recordings.FactoryEventContext{Tick: 7, Sequence: 0, EventTime: eventTime},
			Payload: json.RawMessage(`{"state":"RUNNING"}`),
		}},
	}
	loader := &canonicalProjectionReplayInputLoader{
		artifact: artifact,
		state:    recordings.FactoryWorldState{FactoryState: "RUNNING"},
	}

	loaded, err := LoadRuntime(
		t.TempDir(), "", "legacy-replay.jsonl", operatorconfig.ResolvedDefaults{}, nil,
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()},
		nil, nil, nil, loader, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if loader.loadCalls != 1 || len(loader.reconstructTicks) != 1 || loader.reconstructTicks[0] != 7 {
		t.Fatalf("legacy loader calls/ticks = %d/%v, want one load and selected tick 7", loader.loadCalls, loader.reconstructTicks)
	}
	if loaded.ReplayArtifact != artifact || loaded.PortableRecording != nil || loaded.LoadedFactoryCfg != nil {
		t.Fatalf("legacy runtime load = %#v, want artifact plus detached replay only", loaded)
	}
	if loaded.HistoricalReplay == nil ||
		loaded.HistoricalReplay.FactoryProjection == nil ||
		loaded.HistoricalReplay.FactoryProjection.FactoryState != "RUNNING" {
		t.Fatalf("legacy historical replay = %#v, want canonical projection", loaded.HistoricalReplay)
	}
	inspection := factorysessions.HistoricalReplayFactoryProjection{
		Availability: factorysessions.HistoricalReplayFactoryProjectionAvailable,
		State:        loaded.HistoricalReplay.FactoryProjection,
	}
	if inspection.Availability != factorysessions.HistoricalReplayFactoryProjectionAvailable || inspection.State == nil {
		t.Fatalf("legacy projection availability = %#v, want AVAILABLE", inspection)
	}
}

type legacyProjectionRecordingsRoot struct {
	*recordingsRootConstructionStub
	loader *canonicalProjectionReplayInputLoader
}

func (root *legacyProjectionRecordingsRoot) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return root.loader.LoadReplayInput(request)
}

func (root *legacyProjectionRecordingsRoot) ReconstructCanonicalFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	return root.loader.ReconstructCanonicalFactoryWorldState(events, selectedTick)
}

func TestNewFactorySelectsLegacyHistoricalReplayBeforeLiveRuntimeAssembly(t *testing.T) {
	t.Parallel()

	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "legacy-history"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	artifact := &recordings.ReplayArtifact{
		SchemaVersion: "legacy", Factory: factorySnapshot,
		Events: []recordings.FactoryEvent{{
			SchemaVersion: recordings.FactoryEventSchemaVersionV1,
			Id:            "legacy-event-1", Type: recordings.FactoryEventTypeFactoryStateResponse,
			Context: recordings.FactoryEventContext{Tick: 9, Sequence: 0, EventTime: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)},
			Payload: json.RawMessage(`{"state":"RUNNING"}`),
		}},
	}
	loader := &canonicalProjectionReplayInputLoader{
		artifact: artifact,
		state:    recordings.FactoryWorldState{FactoryState: "RUNNING"},
	}
	root := &legacyProjectionRecordingsRoot{
		recordingsRootConstructionStub: &recordingsRootConstructionStub{}, loader: loader,
	}
	calls := 0
	dependencies := validRuntimeOpeningOwnerPorts(&calls)
	dependencies.Recordings.Service = root
	dependencies.Recordings.Runtime = root
	// Metadata drift inspection may consult the current authored Factory, but
	// this selection test keeps the live-runtime fail-on-call counter focused
	// on activation collaborators.
	dependencies.FactoryDefinitions.LoadFactory = func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, nil
	}
	dependencies.FactorySessions.GenerateRuntimeInstanceID = func() string { return "legacy-runtime" }
	dependencies.FactoryRuntime.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		return zap.NewNop()
	}
	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	opened, err := factory.OpenApplicationRuntime(
		t.Context(),
		&factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
			Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "legacy-replay.jsonl"},
		},
	)
	if err != nil {
		t.Fatalf("OpenApplicationRuntime: %v", err)
	}
	if opened.HistoricalReplay == nil || opened.HistoricalReplay.FactoryProjection.Availability != factorysessions.HistoricalReplayFactoryProjectionAvailable {
		t.Fatalf("opened legacy replay = %#v, want AVAILABLE historical projection", opened.HistoricalReplay)
	}
	if opened.Process == nil {
		t.Fatalf("opened historical roles = %#v, want inert process role", opened)
	}
	if loader.loadCalls != 1 || len(loader.reconstructTicks) != 1 || loader.reconstructTicks[0] != 9 {
		t.Fatalf("legacy selection calls/ticks = %d/%v, want one load and selected tick 9", loader.loadCalls, loader.reconstructTicks)
	}
	if calls != 0 {
		t.Fatalf("legacy historical opening invoked %d live collaborators, want zero", calls)
	}
}

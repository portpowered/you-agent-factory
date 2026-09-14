package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestReplayRequestsHistoricalInspectionUsesEffectiveListenerPort(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		host factorysessions.RuntimeHostRequest
		want bool
	}{
		{
			name: "offline clears port but retains parsed auto port default",
			host: factorysessions.RuntimeHostRequest{AutoPort: true},
			want: true,
		},
		{
			name: "hosted replay has a resolved listener port",
			host: factorysessions.RuntimeHostRequest{Port: 7437, AutoPort: true},
			want: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := &factorysessions.RuntimeOpeningRequest{
				FactorySession: factorysessions.SessionRuntimeOpeningRequest{Host: testCase.host},
			}
			if got := replayRequestsHistoricalInspection(request); got != testCase.want {
				t.Fatalf("replayRequestsHistoricalInspection() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestResolveRuntimeRootNormalizesSharedProcessInputs(t *testing.T) {
	dir := t.TempDir()
	root, err := ResolveRuntimeRoot(filepath.Join(dir, "."), nil, "", func() string { return "runtime-id" }, os.UserHomeDir)
	if err != nil {
		t.Fatalf("resolve runtime root: %v", err)
	}
	if root.FactoryRootDir != filepath.Clean(dir) {
		t.Fatalf("root = %q, want %q", root.FactoryRootDir, filepath.Clean(dir))
	}
	if root.BaseLogger == nil {
		t.Fatal("resolve runtime root did not normalize the base logger")
	}
	if root.RuntimeInstanceID != "runtime-id" {
		t.Fatalf("runtime instance ID = %q, want runtime-id", root.RuntimeInstanceID)
	}
}

func TestResolveRuntimeRootPreservesExplicitIdentityWithoutGenerator(t *testing.T) {
	root, err := ResolveRuntimeRoot(t.TempDir(), nil, "explicit-runtime", nil, os.UserHomeDir)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot: %v", err)
	}
	if root.RuntimeInstanceID != "explicit-runtime" {
		t.Fatalf("runtime instance ID = %q", root.RuntimeInstanceID)
	}
}

func TestActivationRequestDefersCanonicalIdentityUntilRuntimeActivation(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	var canonicalCalls atomic.Int32
	factory := &Factory{
		generateSessionID: func() string {
			canonicalCalls.Add(1)
			return canonicalID
		},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.FactorySessionID != factorysessions.DefaultSessionID || activation.RuntimeID != "runtime-1" {
		t.Fatalf("activation identity = %#v, want default/runtime-1", activation)
	}
	if activation.Inputs.Session.CanonicalSessionID != "" {
		t.Fatalf("canonical session identity = %q, want deferred allocation", activation.Inputs.Session.CanonicalSessionID)
	}
	if got := canonicalCalls.Load(); got != 0 {
		t.Fatalf("canonical session ID generator calls = %d, want 0 before Runtime activation", got)
	}
}

func TestActivationOpeningDefersCanonicalIdentityUntilDefinitionAdmission(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	var canonicalCalls atomic.Int32
	factory := &Factory{
		generateSessionID: func() string {
			canonicalCalls.Add(1)
			return canonicalID
		},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
	}
	opening, runtimeID, err := factory.activationOpening(&factorysessions.RuntimeOpeningRequest{
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID: factorysessions.DefaultSessionID,
		},
		Recordings: recordings.RuntimeOpeningRequest{
			ResumePath: "source.recording.json",
			ResumeInput: recordings.LoadResumeInputResult{
				SourceCanonicalSessionID: "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba",
			},
		},
	})
	if err != nil {
		t.Fatalf("activationOpening(resume) error = %v", err)
	}
	if runtimeID != "runtime-1" {
		t.Fatalf("runtime ID = %q, want runtime-1", runtimeID)
	}
	if opening.FactorySession.CanonicalSessionID != "" {
		t.Fatalf("successor canonical session ID = %q, want empty before definition admission", opening.FactorySession.CanonicalSessionID)
	}
	if got := canonicalCalls.Load(); got != 0 {
		t.Fatalf("canonical session ID generator calls = %d, want 0 before definition admission", got)
	}
}

func TestActivationOpeningDefersCanonicalIdentityForAliasOnlyResume(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return canonicalID },
	}
	opening, _, err := factory.activationOpening(&factorysessions.RuntimeOpeningRequest{
		Recordings: recordings.RuntimeOpeningRequest{ResumePath: "alias-only.recording.json"},
	})
	if err != nil {
		t.Fatalf("activationOpening(alias-only resume) error = %v", err)
	}
	if opening.FactorySession.CanonicalSessionID != "" {
		t.Fatalf("alias-only successor canonical session ID = %q, want empty before definition admission", opening.FactorySession.CanonicalSessionID)
	}
}

func TestResolveRuntimeRootFailsClosedWithoutRequiredIdentityGenerator(t *testing.T) {
	_, err := ResolveRuntimeRoot(t.TempDir(), nil, "", nil, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "ID generator is required") {
		t.Fatalf("error = %v, want missing ID generator failure", err)
	}
	_, err = ResolveRuntimeRoot(t.TempDir(), nil, "", func() string { return "  " }, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "empty identity") {
		t.Fatalf("error = %v, want empty generated identity failure", err)
	}
}

func TestResolveDefinitionPathPreservesReplayAndExplicitSourceSelection(t *testing.T) {
	t.Parallel()

	replay := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	got, err := resolveDefinitionPath(&replay, "recording.json", nil, nil)
	if err != nil || got != "factory-root" {
		t.Fatalf("replay path = (%q, %v), want (factory-root, nil)", got, err)
	}

	sourcePath := filepath.Join(t.TempDir(), "factory.yaml")
	explicit := factorydefinitions.RuntimeOpeningRequest{
		Directory:  "factory-root",
		SourcePath: sourcePath,
	}
	got, err = resolveDefinitionPath(&explicit, "", nil, func() (string, error) {
		return t.TempDir(), nil
	})
	if err != nil || got != filepath.Clean(sourcePath) {
		t.Fatalf("explicit source path = (%q, %v), want (%q, nil)", got, err, sourcePath)
	}
	if explicit.Directory != "factory-root" {
		t.Fatalf("explicit source changed runtime root to %q", explicit.Directory)
	}
}

func TestResolveDefinitionPathResolvesCurrentFactoryAndErrors(t *testing.T) {
	t.Parallel()

	definition := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	currentDir := filepath.Join(t.TempDir(), "current")
	got, err := resolveDefinitionPath(
		&definition,
		"",
		func(root string) (string, error) {
			if root != "factory-root" {
				t.Fatalf("current root = %q, want factory-root", root)
			}
			return currentDir, nil
		},
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil || got != filepath.Clean(currentDir) || definition.Directory != got {
		t.Fatalf("current Factory path = (%q, %v, directory %q)", got, err, definition.Directory)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"},
		"",
		nil,
		nil,
	); err == nil || !strings.Contains(err.Error(), "named Factory path resolver is required") {
		t.Fatalf("missing current resolver error = %v", err)
	}

	want := errors.New("current unavailable")
	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"},
		"",
		func(string) (string, error) { return "", want },
		nil,
	); !errors.Is(err, want) {
		t.Fatalf("current resolver error = %v, want %v", err, want)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{SourcePath: "~\\factory.yaml"},
		"",
		nil,
		func() (string, error) { return "", want },
	); !errors.Is(err, want) {
		t.Fatalf("source home resolver error = %v, want %v", err, want)
	}
}

func TestOpenActivatedRuntimeRoutesRoleCleanupThroughRuntimeDeactivation(t *testing.T) {
	t.Parallel()

	root := &cleanupRoutingRoot{}
	factory := &Factory{
		runtimeRoot:               root,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}

	products, err := factory.openActivatedRuntime(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
	})
	if err != nil {
		t.Fatalf("openActivatedRuntime() error = %v", err)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want exactly one", root.activations)
	}

	roleCleanups := []struct {
		name  string
		close func() error
	}{
		{name: "application", close: products.application.Resources.Close},
		{name: "invocation", close: products.invocation.CloseArtifacts},
		{name: "execution", close: products.execution.Resources.Close},
	}
	for _, role := range roleCleanups {
		if role.close == nil {
			t.Fatalf("%s cleanup edge = nil, want the Runtime deactivation operation", role.name)
		}
	}
	if root.deactivations != 0 {
		t.Fatalf("Runtime deactivations before cleanup = %d, want zero", root.deactivations)
	}

	for _, role := range roleCleanups {
		if err := role.close(); err != nil {
			t.Fatalf("%s cleanup error = %v", role.name, err)
		}
	}
	if root.deactivations != 1 {
		t.Fatalf(
			"Runtime deactivations after draining every role cleanup = %d, want exactly one Runtime-routed deactivation",
			root.deactivations,
		)
	}

	// Opening publishes the Runtime root itself; it does not hand callers a
	// Sessions-retained runtime handle recovered from the opening products.
	if products.application.FactoryRuntime != factoryruntime.Service(root) {
		t.Fatalf(
			"opened application FactoryRuntime = %T, want the Runtime root %T",
			products.application.FactoryRuntime,
			root,
		)
	}
}

type cleanupRoutingRoot struct {
	factoryruntime.Service
	activations   int
	deactivations int
}

func (root *cleanupRoutingRoot) Activate(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activations++
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   &activatedRuntimeService{products: runtimeProducts{}},
		},
	}, nil
}

func (root *cleanupRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	root.deactivations++
	return factoryruntime.RuntimeDeactivationResult{}, nil
}

func TestWarnReplayMetadataMismatchesResolvesCurrentOperatorDefaults(t *testing.T) {
	t.Parallel()

	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "replay-model",
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker",
			Type: factorydefinitions.WorkerTypeModel,
		}},
	}
	factoryDir := t.TempDir()
	recorded, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	if err := applyOperatorDefaults(recorded, defaults); err != nil {
		t.Fatalf("apply recorded operator defaults: %v", err)
	}
	capture := runtimeLoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}

	current, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	warnReplayMetadataMismatches(
		factoryDir,
		"recording.replay.json",
		nil,
		&factorydefinitions.ReplayArtifact{Factory: artifactFactory},
		zap.New(core),
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return current, nil
		},
		capture,
		defaults,
	)
	if logs.Len() != 0 {
		t.Fatalf("equivalent effective config emitted replay metadata warnings: %v", logs.All())
	}
}

func TestWarnReplayMetadataMismatchesReturnsStructuredComponentWarnings(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	recordedConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "recorded prompt",
		}},
	}
	currentConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "changed prompt",
		}},
	}
	recorded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, recordedConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	current, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, currentConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	capture := runtimeLoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	warnings := warnReplayMetadataMismatches(
		factoryDir,
		"recording.replay.json",
		nil,
		&factorydefinitions.ReplayArtifact{Factory: artifactFactory},
		zap.New(core),
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return current, nil
		},
		capture,
		operatorconfig.ResolvedDefaults{},
	)
	gotKeys := make(map[string]bool, len(warnings))
	for _, warning := range warnings {
		gotKeys[warning.Key] = true
	}
	for _, key := range []string{"factory_hash", "workers_hash", "runtime_config_hash"} {
		if !gotKeys[key] {
			t.Fatalf("metadata warnings = %#v, missing %q", warnings, key)
		}
	}
	if logs.Len() != len(warnings) {
		t.Fatalf("structured warning log count = %d, want %d", logs.Len(), len(warnings))
	}
	for _, entry := range logs.All() {
		if entry.ContextMap()["metadata_key"] == nil {
			t.Fatalf("structured warning omitted metadata_key: %#v", entry)
		}
	}
}

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

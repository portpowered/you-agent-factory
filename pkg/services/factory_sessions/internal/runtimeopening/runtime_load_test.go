package runtimeopening

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testpath"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoadRuntimePreservesValidatedPortableRecording(t *testing.T) {
	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	rootDir := t.TempDir()
	var loggerSessionID string
	var loggerFolderPath string
	var loggerFactoryDir string
	replayInputs := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil, zap.NewNop())
	loaded, err := LoadRuntime(
		t.TempDir(),
		"",
		path,
		operatorconfig.ResolvedDefaults{},
		nil,
		RuntimeRoot{FactoryRootDir: rootDir, BaseLogger: zap.NewNop()},
		nil,
		nil,
		nil,
		replayInputs,
		nil,
		func(base *zap.Logger, sessionID, folderPath, factoryDir string) *zap.Logger {
			loggerSessionID = sessionID
			loggerFolderPath = folderPath
			loggerFactoryDir = factoryDir
			return base
		},
	)
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if loaded.PortableRecording == nil {
		t.Fatal("portable recording = nil")
	}
	if got := loaded.PortableRecording.Session.ID; got != "session-js-001" {
		t.Fatalf("session id = %q, want session-js-001", got)
	}
	if got := loaded.PortableRecording.Source; got.Ref != "workflow/example.js" || got.Hash == "" {
		t.Fatalf("portable source = %#v, want preserved source reference and hash", got)
	}
	if got := loaded.PortableRecording.Events; len(got) != 2 || got[0].ID != "event-1" || got[1].ID != "event-2" {
		t.Fatalf("portable events = %#v, want canonical event summaries in order", got)
	}
	if got := loaded.PortableRecording.Result; got == nil || got.Status != "FINAL" || len(got.ArtifactIDs) != 1 || got.ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("portable result = %#v, want preserved result availability", got)
	}
	if got := loaded.PortableRecording.Redaction; !got.RuntimeStateOmitted || !got.CheckpointBodiesOmitted || got.SecretsRedacted != 2 {
		t.Fatalf("portable redaction = %#v, want preserved redaction metadata", got)
	}
	if loaded.ReplayArtifact != nil || loaded.LoadedFactoryCfg != nil {
		t.Fatal("portable recording mixed with Factory-event replay state")
	}
	if loggerSessionID != "~default" || loggerFolderPath != rootDir || loggerFactoryDir == "" {
		t.Fatalf(
			"logger identity = (%q, %q, %q), want injected session and directories",
			loggerSessionID, loggerFolderPath, loggerFactoryDir,
		)
	}
}

// TestLoadRuntimePreservesLegacyReplayInputsAndMetadataWarning proves the
// migrated runtime-opening lane retains the established legacy runtime
// configuration, event, replay-clock, and checkout-divergence behavior after
// Recordings returns its directly owned detached replay input.
func TestLoadRuntimePreservesLegacyReplayInputsAndMetadataWarning(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, time.August, 4, 14, 0, 0, 0, time.UTC)
	recordedSnapshot := runtimeLoadSnapshot(t, "sha256:recorded")
	currentSnapshot := runtimeLoadSnapshot(t, "sha256:current")
	legacy := &recordings.ReplayArtifact{
		SchemaVersion: "agent-factory.replay.v1",
		RecordedAt:    recordedAt,
		Factory:       recordedSnapshot,
		Events: []recordings.FactoryEvent{{
			Id:      "legacy-event-1",
			Payload: json.RawMessage(`{"requestId":"request-1"}`),
		}},
		Diagnostics: recordings.ReplayDiagnostics{Notes: []string{"recorded diagnostics"}},
		WallClock: &recordings.ReplayWallClockMetadata{
			StartedAt:  recordedAt,
			FinishedAt: recordedAt.Add(time.Minute),
		},
	}
	replayArtifacts := recordingswire.NewRecordingReplayArtifactsFactory(
		func(string) ([]byte, error) { return []byte(`{"schemaVersion":"legacy"}`), nil },
		func(path string) (*recordings.ReplayArtifact, error) {
			if path != "legacy-replay.json" {
				t.Fatalf("legacy artifact path = %q, want legacy-replay.json", path)
			}
			return legacy, nil
		},
		zap.NewNop(),
		nil,
	)()
	core, observed := observer.New(zap.InfoLevel)
	current := &runtimeLoadFactorySource{factoryDir: "current-factory", config: &factorydefinitions.FactoryConfig{}}
	var decodedSnapshot *factorydefinitions.FactorySnapshot
	var decodedFactoryDir string
	loaded, err := LoadRuntime(
		"current-factory",
		"runtime-base",
		"legacy-replay.json",
		operatorconfig.ResolvedDefaults{},
		nil,
		RuntimeRoot{FactoryRootDir: "factory-root", BaseLogger: zap.New(core)},
		func(dir string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			if dir != "current-factory" {
				t.Fatalf("current Factory directory = %q, want current-factory", dir)
			}
			return current, nil
		},
		func(
			factoryDir string,
			factoryConfig *factorydefinitions.FactoryConfig,
			runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
			_ []factorydefinitions.PortableBundledFileReplacement,
		) (factorydefinitions.MutableLoadedFactorySource, error) {
			decodedFactoryDir = factoryDir
			if factoryConfig == nil || runtimeDefinitions == nil {
				t.Fatal("embedded replay configuration was not passed to the loaded-source factory")
			}
			return &runtimeLoadFactorySource{factoryDir: factoryDir, config: factoryConfig}, nil
		},
		func(snapshot *factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			decodedSnapshot = snapshot
			return &runtimeLoadFactorySource{factoryDir: "recorded-factory", config: &factorydefinitions.FactoryConfig{}}, nil
		},
		replayArtifacts,
		func(
			source factorydefinitions.FactorySnapshotSource,
			factoryDir string,
			_ map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			if source != current || factoryDir != "current-factory" {
				t.Fatalf("current snapshot source = (%#v, %q), want current Factory", source, factoryDir)
			}
			return currentSnapshot, nil
		},
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	assertRuntimeLoadLegacyOutput(t, loaded, legacy, recordedSnapshot, decodedSnapshot, decodedFactoryDir, recordedAt)
	assertRuntimeLoadMetadataWarning(t, observed)
}

func assertRuntimeLoadLegacyOutput(
	t testing.TB,
	loaded RuntimeLoad,
	legacy *recordings.ReplayArtifact,
	recordedSnapshot, decodedSnapshot *factorydefinitions.FactorySnapshot,
	decodedFactoryDir string,
	recordedAt time.Time,
) {
	t.Helper()
	if decodedSnapshot == nil || string(*decodedSnapshot) != string(*recordedSnapshot) {
		t.Fatalf("embedded replay snapshot = %v, want recorded snapshot", decodedSnapshot)
	}
	if decodedFactoryDir != "recorded-factory" || loaded.LoadedFactoryCfg == nil || loaded.LoadedFactoryCfg.RuntimeBaseDir() != "runtime-base" {
		t.Fatalf("loaded legacy Factory config = (%q, %#v), want recorded config and runtime base", decodedFactoryDir, loaded.LoadedFactoryCfg)
	}
	if loaded.PortableRecording != nil || loaded.ReplayArtifact == nil {
		t.Fatalf("runtime load = %#v, want only legacy replay state", loaded)
	}
	if got := loaded.ReplayArtifact; got.SchemaVersion != legacy.SchemaVersion || !got.RecordedAt.Equal(recordedAt) || len(got.Events) != 1 || got.Events[0].Id != "legacy-event-1" {
		t.Fatalf("legacy replay facts = %#v, want preserved schema, time, and events", got)
	}
	if got := loaded.ReplayArtifact.Diagnostics.Notes; len(got) != 1 || got[0] != "recorded diagnostics" {
		t.Fatalf("legacy diagnostics = %#v, want preserved diagnostics", got)
	}
	if got := loaded.ReplayArtifact.WallClock; got == nil || !got.StartedAt.Equal(recordedAt) || !got.FinishedAt.Equal(recordedAt.Add(time.Minute)) {
		t.Fatalf("legacy replay clock input = %#v, want preserved wall clock", got)
	}
}

func assertRuntimeLoadMetadataWarning(t testing.TB, observed *observer.ObservedLogs) {
	t.Helper()
	metadataWarnings := observed.FilterMessage("replay artifact metadata differs from current checkout").All()
	if len(metadataWarnings) != 1 {
		t.Fatalf("metadata warning count = %d, want 1", len(metadataWarnings))
	}
	fields := metadataWarnings[0].ContextMap()
	if fields["category"] != recordings.DivergenceCategoryConfigMismatch || fields["metadata_key"] != "factory_hash" || fields["artifact"] != "sha256:recorded" || fields["current"] != "sha256:current" {
		t.Fatalf("metadata warning fields = %#v, want recorded/current Factory hash mismatch", fields)
	}
}

// TestLoadRuntimeRejectsReplayInputFailuresBeforeConfigConstruction proves
// missing and invalid replay input never reaches Factory configuration,
// provider, runtime, or active-session construction. It also preserves the
// direct, structured diagnostics supplied by the Recordings capability.
func TestLoadRuntimeRejectsReplayInputFailuresBeforeConfigConstruction(t *testing.T) {
	t.Parallel()

	valid := runtimeLoadPortableFixture(t)
	cases := []runtimeLoadFailureCase{
		{name: "missing", readErr: os.ErrNotExist, kind: recordings.ReplayInputErrorRead},
		{
			name: "malformed", payload: []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","unknown":true}`),
			kind: recordings.ReplayInputErrorPortable, diagnosticCode: recordings.ReplayInputDiagnosticMalformed, area: "document",
		},
		{
			name: "unsupported compatibility", payload: runtimeLoadMutatePortableFixture(t, valid, func(document map[string]any) {
				document["replayCompatibilityVersion"] = "99"
			}),
			kind: recordings.ReplayInputErrorPortable, diagnosticCode: recordings.ReplayInputDiagnosticUnsupportedVersion,
			area: "compatibility", path: "replayCompatibilityVersion",
		},
		{
			name: "invalid identity", payload: runtimeLoadMutatePortableFixture(t, valid, func(document map[string]any) {
				document["session"].(map[string]any)["id"] = ""
			}),
			kind: recordings.ReplayInputErrorPortable, diagnosticCode: recordings.ReplayInputDiagnosticInvalidIdentity,
			area: "session", path: "session.id",
		},
		{
			name: "invalid summary", payload: runtimeLoadMutatePortableFixture(t, valid, func(document map[string]any) {
				document["result"].(map[string]any)["status"] = "UNKNOWN"
			}),
			kind: recordings.ReplayInputErrorPortable, diagnosticCode: recordings.ReplayInputDiagnosticInvalidSummary,
			area: "result", path: "result.status",
		},
		{
			name: "invalid digest", payload: runtimeLoadMutatePortableFixture(t, valid, func(document map[string]any) {
				document["source"].(map[string]any)["hash"] = "sha256:not-a-digest"
			}),
			kind: recordings.ReplayInputErrorPortable, diagnosticCode: recordings.ReplayInputDiagnosticInvalidDigest,
			area: "digest", path: "source.hash",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertRuntimeLoadRejectsReplayInput(t, testCase)
		})
	}
}

type runtimeLoadFailureCase struct {
	name           string
	payload        []byte
	readErr        error
	kind           recordings.ReplayInputErrorKind
	diagnosticCode recordings.ReplayInputDiagnosticCode
	area           string
	path           string
}

func assertRuntimeLoadRejectsReplayInput(t testing.TB, testCase runtimeLoadFailureCase) {
	t.Helper()
	constructed := false
	replayArtifacts := recordingswire.NewRecordingReplayArtifactsFactory(
		func(path string) ([]byte, error) {
			if path != "replay.json" {
				t.Fatalf("replay path = %q, want replay.json", path)
			}
			return testCase.payload, testCase.readErr
		},
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for missing or portable replay input")
			return nil, nil
		},
		zap.NewNop(),
		nil,
	)()
	_, err := LoadRuntime(
		"factory", "", "replay.json", operatorconfig.ResolvedDefaults{}, nil,
		RuntimeRoot{FactoryRootDir: "factory-root", BaseLogger: zap.NewNop()},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			constructed = true
			return nil, errors.New("Factory configuration must not load")
		},
		func(string, *factorydefinitions.FactoryConfig, factorydefinitions.RuntimeDefinitionLookup, []factorydefinitions.PortableBundledFileReplacement) (factorydefinitions.MutableLoadedFactorySource, error) {
			constructed = true
			return nil, errors.New("replay Factory configuration must not construct")
		},
		func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			constructed = true
			return nil, errors.New("replay Factory configuration must not decode")
		},
		replayArtifacts,
		nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	assertRuntimeLoadRejectedInput(t, err, constructed, testCase)
}

func assertRuntimeLoadRejectedInput(
	t testing.TB,
	err error,
	constructed bool,
	testCase runtimeLoadFailureCase,
) {
	t.Helper()
	if err == nil {
		t.Fatal("LoadRuntime() error = nil, want replay input failure")
	}
	if constructed {
		t.Fatal("invalid replay input constructed Factory runtime configuration")
	}
	if !strings.HasPrefix(err.Error(), "load portable replay: ") {
		t.Fatalf("LoadRuntime() error = %q, want portable replay context", err)
	}
	var typed *recordings.ReplayInputError
	if !errors.As(err, &typed) || typed.Kind != testCase.kind {
		t.Fatalf("LoadRuntime() error = %v, want replay input kind %q", err, testCase.kind)
	}
	if testCase.diagnosticCode == "" {
		if typed.Diagnostic != nil {
			t.Fatalf("read failure diagnostic = %#v, want nil", typed.Diagnostic)
		}
		return
	}
	if typed.Diagnostic == nil || typed.Diagnostic.Code != testCase.diagnosticCode || typed.Diagnostic.Area != testCase.area || typed.Diagnostic.Path != testCase.path {
		t.Fatalf("replay diagnostic = %#v, want (%q, %q, %q)", typed.Diagnostic, testCase.diagnosticCode, testCase.area, testCase.path)
	}
	if testCase.diagnosticCode == recordings.ReplayInputDiagnosticUnsupportedVersion && len(typed.Diagnostic.SupportedVersions) == 0 {
		t.Fatal("unsupported-version diagnostic omitted supported versions")
	}
}

func TestLoadRuntimeRejectsMissingReplayInputCapability(t *testing.T) {
	t.Parallel()

	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	_, err := LoadRuntime(
		t.TempDir(), "", "recording.json", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, nil, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if err == nil {
		t.Fatal("missing replay input capability error = nil")
	}
}

func TestLoadRuntimePropagatesReplayInputFailure(t *testing.T) {
	t.Parallel()

	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	want := errors.New("recording read unavailable")
	replayInputs := recordingswire.NewReplayInputLoader(func(path string) ([]byte, error) {
		if path != "recording.json" {
			t.Fatalf("path = %q, want recording.json", path)
		}
		return nil, want
	}, nil, zap.NewNop())
	_, err := LoadRuntime(
		t.TempDir(), "", "recording.json", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, replayInputs, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if !errors.Is(err, want) {
		t.Fatalf("LoadRuntime() error = %v, want %v", err, want)
	}
	if !strings.HasPrefix(err.Error(), "load portable replay: ") {
		t.Fatalf("LoadRuntime() error = %q, want portable replay context", err)
	}
}

func TestLoadRuntimePreservesLegacyReplayFailureContext(t *testing.T) {
	t.Parallel()

	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	legacyCause := errors.New("legacy artifact cannot be decoded")
	replayInputs := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) { return []byte(`{"schemaVersion":"legacy"}`), nil },
		func(string) (*recordings.ReplayArtifact, error) { return nil, legacyCause },
		zap.NewNop(),
	)
	_, err := LoadRuntime(
		t.TempDir(), "", "recording.json", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, replayInputs, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if !errors.Is(err, legacyCause) {
		t.Fatalf("LoadRuntime() error = %v, want legacy cause %v", err, legacyCause)
	}
	if !strings.HasPrefix(err.Error(), "load factory config: load replay artifact: ") {
		t.Fatalf("LoadRuntime() error = %q, want established legacy factory-config context", err)
	}
}

func TestLegacyReplayArtifactFromInputPreservesCompatibilityFacts(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	input := &recordings.ReplayInputLegacyArtifact{
		SchemaVersion:       "legacy.v1",
		RecordedAt:          recordedAt,
		FactorySnapshotJSON: []byte(`{"id":"factory-legacy"}`),
		DiagnosticsJSON:     []byte(`{"notes":["captured"]}`),
		Events: []recordings.ReplayInputLegacyEvent{
			{EventJSON: []byte(`{"id":"event-legacy","context":{},"payload":{},"schemaVersion":"agent-factory.event.v1","type":"RUN_REQUEST"}`)},
		},
		WallClock: &recordings.ReplayInputWallClockMetadata{StartedAt: recordedAt},
	}

	artifact, err := legacyReplayArtifactFromInput(input)
	if err != nil {
		t.Fatalf("legacyReplayArtifactFromInput() error = %v", err)
	}
	if artifact.SchemaVersion != input.SchemaVersion || !artifact.RecordedAt.Equal(recordedAt) {
		t.Fatalf("legacy artifact identity = (%q, %v), want (%q, %v)", artifact.SchemaVersion, artifact.RecordedAt, input.SchemaVersion, recordedAt)
	}
	if artifact.Factory == nil || string(*artifact.Factory) != string(input.FactorySnapshotJSON) {
		t.Fatalf("legacy artifact Factory = %v, want detached decoded Factory snapshot", artifact.Factory)
	}
	if len(artifact.Events) != 1 || artifact.Events[0].Id != "event-legacy" {
		t.Fatalf("legacy artifact Events = %#v, want decoded legacy event", artifact.Events)
	}
	if len(artifact.Diagnostics.Notes) != 1 || artifact.Diagnostics.Notes[0] != "captured" {
		t.Fatalf("legacy artifact Diagnostics = %#v, want preserved diagnostics", artifact.Diagnostics)
	}
	if artifact.WallClock == nil || !artifact.WallClock.StartedAt.Equal(recordedAt) {
		t.Fatalf("legacy artifact WallClock = %#v, want preserved replay clock metadata", artifact.WallClock)
	}
	input.FactorySnapshotJSON[0] = 'x'
	input.Events[0].EventJSON[0] = 'x'
	if string(*artifact.Factory) != `{"id":"factory-legacy"}` || artifact.Events[0].Id != "event-legacy" {
		t.Fatalf("legacy artifact observed mutation from replay-input value: %#v", artifact)
	}
}

func TestClockForReplayPreservesOverridesAndInjectedDefaults(t *testing.T) {
	explicit := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 1, 0, 0, 0, time.UTC))
	replay := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC))
	fallback := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC))
	artifact := &factorydefinitions.ReplayArtifact{}

	selected, err := clockForReplay(
		explicit,
		artifact,
		func(*factorydefinitions.ReplayArtifact) recordings.Clock { return replay },
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != explicit.Now() {
		t.Fatalf("explicit clock = (%v, %v), want preserved override", selected, err)
	}

	selected, err = clockForReplay(
		nil,
		artifact,
		func(*factorydefinitions.ReplayArtifact) recordings.Clock { return replay },
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != replay.Now() {
		t.Fatalf("replay clock = (%v, %v), want replay-selected clock", selected, err)
	}

	selected, err = clockForReplay(
		nil,
		nil,
		nil,
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != fallback.Now() {
		t.Fatalf("default clock = (%v, %v), want injected fallback", selected, err)
	}
}

func TestClockForReplayRejectsMissingOrNilResolver(t *testing.T) {
	if _, err := clockForReplay(nil, nil, nil, nil); err == nil {
		t.Fatal("missing clock resolver error = nil")
	}
	if _, err := clockForReplay(
		nil, nil, nil,
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
	); err == nil {
		t.Fatal("nil resolved clock error = nil")
	}
}

func TestLoadRuntimeRejectsMissingOrNilSessionLoggerFactory(t *testing.T) {
	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	if _, err := LoadRuntime("", "", "", operatorconfig.ResolvedDefaults{}, nil, root, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("missing session logger factory error = nil")
	}
	if _, err := LoadRuntime(
		"", "", "", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, nil, nil,
		func(*zap.Logger, string, string, string) *zap.Logger { return nil },
	); err == nil {
		t.Fatal("nil session logger error = nil")
	}
}

func TestNewDurableExecutionCanonicalizesOperatorDefaultsAndPresets(t *testing.T) {
	var got factoryruntime.JavaScriptWorkerSettings
	executionFactory := func(
		_ string,
		_ factorysessions.PersistencePolicy,
		_ workers.Provider,
		_ factoryruntime.Clock,
		_ map[string]struct{},
		settings factoryruntime.JavaScriptWorkerSettings,
		_ *workers.MockWorkersConfig,
		_ []operatorconfig.ACPIntegration,
	) (durableexecution.Service, error) {
		got = settings
		return nil, nil
	}
	_, err := NewDurableExecution(
		func(string) (operatorconfig.Config, error) {
			return operatorconfig.Config{
				Defaults: operatorconfig.Defaults{WorkerModelProvider: "customer"},
				WorkerPresets: []operatorconfig.WorkerPreset{{
					ID: "review", ModelProvider: "agent",
				}},
			}, nil
		},
		factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
		factorysessions.SessionRuntimeOpeningRequest{SystemConfigHome: t.TempDir()},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModel: "operator-model"},
		RuntimeRoot{FactoryRootDir: t.TempDir()},
		nil,
		nil,
		nil,
		executionFactory,
		factorysessions.ProviderIdentityResolver(func(identity string) (string, error) {
			switch identity {
			case "CODEX":
				return "codex", nil
			case "customer":
				return "customer.provider", nil
			case "agent":
				return "cursor", nil
			default:
				return "", errors.New("unexpected provider")
			}
		}),
	)
	if err != nil {
		t.Fatalf("NewDurableExecution: %v", err)
	}
	if got.DefaultModelProvider != "codex" || got.DefaultModel != "operator-model" {
		t.Fatalf("resolved defaults = %#v, want codex/operator-model", got)
	}
	if preset := got.Presets["review"]; preset.ModelProvider != "cursor" {
		t.Fatalf("review preset = %#v, want canonical cursor identity", preset)
	}
}

func runtimeLoadSnapshot(t testing.TB, factoryHash string) *factorydefinitions.FactorySnapshot {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"metadata": map[string]string{"factory_hash": factoryHash},
	})
	if err != nil {
		t.Fatalf("new Factory snapshot: %v", err)
	}
	return snapshot
}

func runtimeLoadPortableFixture(t testing.TB) []byte {
	t.Helper()
	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read portable replay fixture: %v", err)
	}
	return payload
}

func runtimeLoadMutatePortableFixture(
	t testing.TB,
	payload []byte,
	mutate func(map[string]any),
) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode portable replay fixture: %v", err)
	}
	mutate(document)
	updated, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode portable replay fixture: %v", err)
	}
	return updated
}

// runtimeLoadFactorySource is the minimal Factory Definitions runtime source
// needed to observe LoadRuntime's legacy replay adaptation without activating
// a Factory Runtime or touching authored Factory storage.
type runtimeLoadFactorySource struct {
	factoryDir     string
	runtimeBaseDir string
	config         *factorydefinitions.FactoryConfig
}

func (source *runtimeLoadFactorySource) FactoryDir() string {
	return source.factoryDir
}

func (source *runtimeLoadFactorySource) RuntimeBaseDir() string {
	if source.runtimeBaseDir != "" {
		return source.runtimeBaseDir
	}
	return source.factoryDir
}

func (source *runtimeLoadFactorySource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}

func (source *runtimeLoadFactorySource) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (source *runtimeLoadFactorySource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (source *runtimeLoadFactorySource) WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (source *runtimeLoadFactorySource) SetRuntimeBaseDir(path string) {
	source.runtimeBaseDir = path
}

func (source *runtimeLoadFactorySource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}

func (source *runtimeLoadFactorySource) MutateWorkers(func(*factorydefinitions.FactoryWorkerConfig) error) error {
	return nil
}

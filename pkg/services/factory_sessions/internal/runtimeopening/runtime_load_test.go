package runtimeopening

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"go.uber.org/zap"
)

type runtimeLoadPortableFailureCase struct {
	name     string
	payload  []byte
	readErr  error
	wantCode recordings.ReplayArtifactDiagnosticCode
}

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
	replayInputs := recordingswire.NewReplayArtifactCapability(
		recordings.RecordingReadFile(os.ReadFile), nil, logging.NoopLogger{},
	)
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
	if loaded.PortableRecording.SchemaVersion != "2" ||
		loaded.PortableRecording.ReplayCompatibilityVersion != "1" ||
		loaded.PortableRecording.Source.Ref != "workflow/example.js" ||
		len(loaded.PortableRecording.Artifacts) != 1 ||
		len(loaded.PortableRecording.Events) != 2 ||
		loaded.PortableRecording.Events[0].Sequence != 0 ||
		loaded.PortableRecording.Events[1].Sequence != 1 ||
		loaded.PortableRecording.Result == nil ||
		loaded.PortableRecording.Result.Status != "FINAL" ||
		!loaded.PortableRecording.Redaction.RuntimeStateOmitted ||
		!loaded.PortableRecording.Redaction.CheckpointBodiesOmitted ||
		!loaded.PortableRecording.Redaction.ProviderTranscriptsOmitted ||
		!loaded.PortableRecording.Redaction.ChildDispatchesOmitted ||
		loaded.PortableRecording.Redaction.SecretsRedacted != 2 {
		t.Fatalf("portable recording = %#v, want validated compatibility, summaries, order, and redaction facts", loaded.PortableRecording)
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
	replayInputs := recordingswire.NewReplayArtifactCapability(func(path string) ([]byte, error) {
		if path != "recording.json" {
			t.Fatalf("path = %q, want recording.json", path)
		}
		return nil, want
	}, nil, logging.NoopLogger{})
	_, err := LoadRuntime(
		t.TempDir(), "", "recording.json", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, replayInputs, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if !errors.Is(err, want) {
		t.Fatalf("LoadRuntime() error = %v, want %v", err, want)
	}
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) ||
		inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticDependencyFailure {
		t.Fatalf("LoadRuntime() error = %v, want Recordings-owned safe dependency diagnostic", err)
	}
}

func TestLoadRuntimePreservesLegacyReplayFailureContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "legacy-replay.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"legacy"}`), 0o600); err != nil {
		t.Fatalf("write legacy replay fixture: %v", err)
	}
	want := errors.New("legacy replay unavailable")
	replayInputs := recordingswire.NewReplayArtifactCapability(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) { return nil, want },
		logging.NoopLogger{},
	)
	loaded, err := LoadRuntime(
		t.TempDir(), "", path, operatorconfig.ResolvedDefaults{}, nil,
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()},
		nil, nil, nil, replayInputs, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if !errors.Is(err, want) {
		t.Fatalf("LoadRuntime() error = %v, want wrapped %v", err, want)
	}
	if got, wantText := err.Error(), "load factory config: load replay artifact: legacy replay unavailable"; got != wantText {
		t.Fatalf("LoadRuntime() error = %q, want %q", got, wantText)
	}
	if loaded.PortableRecording != nil || loaded.ReplayArtifact != nil || loaded.LoadedFactoryCfg != nil || loaded.SessionLogger != nil {
		t.Fatalf("LoadRuntime() result = %#v, want zero result on legacy load failure", loaded)
	}
}

func TestLoadRuntimePreservesLegacyReplayInputs(t *testing.T) {
	t.Parallel()

	artifact := &recordings.ReplayArtifact{
		SchemaVersion: "legacy",
		Events:        []factorydefinitions.FactoryEvent{{}},
		Factory:       runtimeLoadFactorySnapshot(t),
		WallClock: &factorydefinitions.ReplayWallClockMetadata{
			StartedAt:  time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC),
			FinishedAt: time.Date(2026, time.July, 20, 2, 5, 0, 0, time.UTC),
		},
	}
	capability := recordingswire.NewReplayArtifactCapability(
		func(path string) ([]byte, error) {
			if path != "legacy-replay.json" {
				t.Fatalf("read path = %q, want legacy-replay.json", path)
			}
			return []byte(`{"schemaVersion":"legacy"}`), nil
		},
		func(path string) (*recordings.ReplayArtifact, error) {
			if path != "legacy-replay.json" {
				t.Fatalf("legacy path = %q, want legacy-replay.json", path)
			}
			return artifact, nil
		},
		logging.NoopLogger{},
	)

	loaded, err := LoadRuntime(
		t.TempDir(),
		"runtime-base",
		"legacy-replay.json",
		operatorconfig.ResolvedDefaults{},
		nil,
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()},
		nil,
		factorydefinitionswire.LoadedFactorySourceFactory(),
		factorydefinitionswire.ReplayRuntimeConfigDecoder(),
		capability,
		nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if loaded.PortableRecording != nil || loaded.LoadedFactoryCfg == nil || loaded.ReplayArtifact != artifact {
		t.Fatalf("LoadRuntime() = %#v, want only reconstructed legacy runtime inputs", loaded)
	}
	if got := loaded.LoadedFactoryCfg.RuntimeBaseDir(); got != "runtime-base" {
		t.Fatalf("runtime base dir = %q, want runtime-base", got)
	}
	if len(loaded.ReplayArtifact.Events) != 1 ||
		loaded.ReplayArtifact.WallClock == nil ||
		!loaded.ReplayArtifact.WallClock.StartedAt.Equal(artifact.WallClock.StartedAt) {
		t.Fatalf("legacy replay facts = %#v, want events and replay-clock metadata", loaded.ReplayArtifact)
	}
}

func TestLoadRuntimeRejectsPortableFailureMatrixBeforeFactoryConstruction(t *testing.T) {
	t.Parallel()

	validPayload := runtimeLoadPortablePayload(t, nil)
	readFailure := errors.New("portable reader unavailable")
	testCases := []runtimeLoadPortableFailureCase{
		{
			name:     "missing input",
			readErr:  os.ErrNotExist,
			wantCode: recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name:     "portable read failure",
			readErr:  readFailure,
			wantCode: recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name:     "malformed JSON",
			payload:  []byte(`{"recordingKind":"you.factory-session.javascript.recording","schemaVersion":`),
			wantCode: recordings.ReplayArtifactDiagnosticMalformed,
		},
		{
			name:     "trailing document",
			payload:  append(validPayload, []byte("\n{}")...),
			wantCode: recordings.ReplayArtifactDiagnosticMalformed,
		},
		{
			name: "unsupported compatibility version",
			payload: runtimeLoadPortablePayload(t, func(recording *recordings.PortableRecording) {
				recording.ReplayCompatibilityVersion = "99"
			}),
			wantCode: recordings.ReplayArtifactDiagnosticUnsupportedVersion,
		},
		{
			name: "invalid identity",
			payload: runtimeLoadPortablePayload(t, func(recording *recordings.PortableRecording) {
				recording.Session.ID = ""
			}),
			wantCode: recordings.ReplayArtifactDiagnosticInvalidIdentity,
		},
		{
			name: "invalid summary",
			payload: runtimeLoadPortablePayload(t, func(recording *recordings.PortableRecording) {
				recording.Session.Status = "UNSUPPORTED"
			}),
			wantCode: recordings.ReplayArtifactDiagnosticInvalidSummary,
		},
		{
			name: "invalid integrity",
			payload: runtimeLoadPortablePayload(t, func(recording *recordings.PortableRecording) {
				recording.Source.Hash = "invalid"
			}),
			wantCode: recordings.ReplayArtifactDiagnosticInvalidIntegrity,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertPortableRuntimeFailureBeforeFactoryConstruction(t, testCase)
		})
	}
}

func assertPortableRuntimeFailureBeforeFactoryConstruction(
	t *testing.T,
	testCase runtimeLoadPortableFailureCase,
) {
	t.Helper()

	loadFactoryCalls := 0
	decodeReplayConfigCalls := 0
	newLoadedFactoryCalls := 0
	capability := recordingswire.NewReplayArtifactCapability(
		func(path string) ([]byte, error) {
			if path != "portable-replay.json" {
				t.Fatalf("read path = %q, want portable-replay.json", path)
			}
			return testCase.payload, testCase.readErr
		},
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for portable replay failures")
			return nil, nil
		},
		logging.NoopLogger{},
	)

	loaded, err := LoadRuntime(
		t.TempDir(), "", "portable-replay.json", operatorconfig.ResolvedDefaults{}, nil,
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			loadFactoryCalls++
			return nil, errors.New("Factory loading must not start")
		},
		func(string, *factorydefinitions.FactoryConfig, factorydefinitions.RuntimeDefinitionLookup, []factorydefinitions.PortableBundledFileReplacement) (factorydefinitions.MutableLoadedFactorySource, error) {
			newLoadedFactoryCalls++
			return nil, errors.New("legacy Factory reconstruction must not start")
		},
		func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			decodeReplayConfigCalls++
			return nil, errors.New("legacy Factory decoding must not start")
		},
		capability,
		nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if err == nil {
		t.Fatal("LoadRuntime() error = nil")
	}
	if !strings.HasPrefix(err.Error(), "load portable replay: ") {
		t.Fatalf("LoadRuntime() error = %q, want portable replay context", err)
	}
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) ||
		inputErr.Family != recordings.ReplayInputFamilyPortable ||
		inputErr.Diagnostic.Code != testCase.wantCode {
		t.Fatalf("LoadRuntime() error = %v, want portable diagnostic %q", err, testCase.wantCode)
	}
	if loaded.PortableRecording != nil || loaded.ReplayArtifact != nil || loaded.LoadedFactoryCfg != nil || loaded.SessionLogger != nil {
		t.Fatalf("LoadRuntime() result = %#v, want zero result", loaded)
	}
	if loadFactoryCalls != 0 || decodeReplayConfigCalls != 0 || newLoadedFactoryCalls != 0 {
		t.Fatalf(
			"Factory replay construction calls = load:%d decode:%d new:%d, want zero before input validation",
			loadFactoryCalls,
			decodeReplayConfigCalls,
			newLoadedFactoryCalls,
		)
	}
}

func TestLoadRuntimePreservesLegacyTypedDiagnostic(t *testing.T) {
	t.Parallel()

	failure := &recordings.ReplayArtifactError{
		Kind: recordings.ReplayArtifactErrorForeign,
		Diagnostic: recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticForeignReference,
			Area:    "artifact",
			Path:    "reference",
			Message: "artifact reference does not belong to the selected recording",
		},
		Cause: recordings.ErrForeignPortableArtifact,
	}
	capability := recordingswire.NewReplayArtifactCapability(
		func(string) ([]byte, error) { return []byte(`{"schemaVersion":"legacy"}`), nil },
		func(string) (*recordings.ReplayArtifact, error) { return nil, failure },
		logging.NoopLogger{},
	)

	loaded, err := LoadRuntime(
		t.TempDir(), "", "legacy-replay.json", operatorconfig.ResolvedDefaults{}, nil,
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()},
		nil, nil, nil, capability, nil,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
	)
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("LoadRuntime() error = %v, want ErrForeignPortableArtifact", err)
	}
	if got, want := err.Error(), "load factory config: load replay artifact: reference: artifact reference does not belong to the selected recording"; got != want {
		t.Fatalf("LoadRuntime() error = %q, want %q", got, want)
	}
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) ||
		inputErr.Family != recordings.ReplayInputFamilyLegacy ||
		inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticForeignReference {
		t.Fatalf("LoadRuntime() error = %v, want legacy foreign-reference diagnostic", err)
	}
	if loaded.PortableRecording != nil || loaded.ReplayArtifact != nil || loaded.LoadedFactoryCfg != nil || loaded.SessionLogger != nil {
		t.Fatalf("LoadRuntime() result = %#v, want zero result", loaded)
	}
}

func runtimeLoadPortablePayload(
	t *testing.T,
	mutate func(*recordings.PortableRecording),
) []byte {
	t.Helper()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read valid portable recording: %v", err)
	}
	if mutate == nil {
		return payload
	}
	recording, err := recordings.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode valid portable recording: %v", err)
	}
	mutate(&recording)
	payload, err = json.Marshal(recording)
	if err != nil {
		t.Fatalf("marshal portable recording: %v", err)
	}
	return payload
}

func runtimeLoadFactorySnapshot(t *testing.T) *factorydefinitions.FactorySnapshot {
	t.Helper()

	config, err := factorymapping.FactoryConfigFromOpenAPIJSON([]byte(factoryfixtures.CrossPathValidAlphaFactoryJSON))
	if err != nil {
		t.Fatalf("decode Factory fixture: %v", err)
	}
	snapshot, err := factorydefinitionswire.FactorySnapshotCapturer()(t.TempDir(), config, nil, "", nil)
	if err != nil {
		t.Fatalf("capture Factory snapshot: %v", err)
	}
	return snapshot
}

func TestClockForReplayPreservesOverridesAndInjectedDefaults(t *testing.T) {
	explicit := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 1, 0, 0, 0, time.UTC))
	replay := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC))
	fallback := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC))
	artifact := &factorydefinitions.ReplayArtifact{
		WallClock: &factorydefinitions.ReplayWallClockMetadata{
			StartedAt: time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC),
		},
	}

	selected, err := clockForReplay(
		explicit,
		artifact,
		func(got *factorydefinitions.ReplayArtifact) recordings.Clock {
			if got != artifact || got.WallClock == nil || !got.WallClock.StartedAt.Equal(replay.Now()) {
				t.Fatalf("replay clock input = %#v, want legacy artifact wall-clock metadata", got)
			}
			return replay
		},
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
		func(got *factorydefinitions.ReplayArtifact) recordings.Clock {
			if got != artifact || got.WallClock == nil || !got.WallClock.StartedAt.Equal(replay.Now()) {
				t.Fatalf("replay clock input = %#v, want legacy artifact wall-clock metadata", got)
			}
			return replay
		},
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

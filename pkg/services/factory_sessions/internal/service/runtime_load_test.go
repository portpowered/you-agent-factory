package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type runtimeLoadPortableFailureCase struct {
	name     string
	readErr  error
	wantCode recordings.ReplayArtifactDiagnosticCode
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLoadRuntimePreservesValidatedPortableRecording(t *testing.T) {
	path := "selected-portable.json"
	rootDir := t.TempDir()
	var loggerSessionID string
	var loggerFolderPath string
	var loggerFactoryDir string
	portable := runtimeInputPortableFixture()
	replayInputs := runtimeInputReplayFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.Path != path {
			t.Fatalf("replay path = %q", request.Path)
		}
		return recordings.LoadReplayInputResult{Portable: &portable}, nil
	})
	loaded, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		replayInputs,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, sessionID, folderPath, factoryDir string) *zap.Logger {
			loggerSessionID = sessionID
			loggerFolderPath = folderPath
			loggerFactoryDir = factoryDir
			return base
		},
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           path,
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       rootDir,
		HistoricalInspection: true})
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if loaded.PortableRecording == nil {
		t.Fatal("portable recording = nil")
	}
	if loaded.HistoricalReplay == nil {
		t.Fatal("historical replay = nil")
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
	if loaded.HistoricalReplay.Session.SessionID != "session-js-001" ||
		loaded.HistoricalReplay.Session.ResolvedSource.SourceRef != "workflow/example.js" ||
		loaded.HistoricalReplay.Result.ResultStatus != factorysessions.ResultStatusFinal ||
		len(loaded.HistoricalReplay.Artifacts.Artifacts) != 1 ||
		loaded.HistoricalReplay.Artifacts.Artifacts[0].ID != "artifact-1" ||
		len(loaded.HistoricalReplay.Events.Events) != 2 ||
		!loaded.HistoricalReplay.Redaction.RuntimeStateOmitted ||
		!loaded.HistoricalReplay.Redaction.CheckpointBodiesOmitted ||
		!loaded.HistoricalReplay.Redaction.ProviderTranscriptsOmitted ||
		!loaded.HistoricalReplay.Redaction.ChildDispatchesOmitted ||
		loaded.HistoricalReplay.Redaction.SecretsRedacted != 2 {
		t.Fatalf("historical replay = %#v, want public session, result, artifact, and ordered event facts", loaded.HistoricalReplay)
	}
	if loggerSessionID != "~default" || loggerFolderPath != rootDir || loggerFactoryDir == "" {
		t.Fatalf(
			"logger identity = (%q, %q, %q), want injected session and directories",
			loggerSessionID, loggerFolderPath, loggerFactoryDir,
		)
	}
}

func TestLoadRuntimeLogsIgnoredFactoryFieldPathsWithoutValues(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	config := &factorydefinitions.FactoryConfig{Name: "future-fields"}
	config.SetIgnoredJSONPaths([]string{
		"$.workers[0].futurePolicy",
		"$.logicalRoundTrip",
	})
	loaded, err := factorydefinitionfixtures.NewLoadedSource(directory, config, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedSource: %v", err)
	}
	logger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	_, err = NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return loaded, nil
	},
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			panic("unexpected replay read")
		}),
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		logger).Load(RuntimeInputLoadRequest{Dir: directory,
		ExecutionBaseDir:     "",
		ReplayPath:           "",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       directory,
		HistoricalInspection: true})
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}

	records := observed.FilterMessage("ignored unknown Factory Definition fields").All()
	if len(records) != 1 {
		t.Fatalf("warning records = %d, want 1", len(records))
	}
	context := records[0].ContextMap()
	if context["code"] != "FACTORY_CONFIG_UNKNOWN_FIELDS_IGNORED" {
		t.Fatalf("warning code = %#v", context["code"])
	}
	paths, ok := context["ignored_json_paths"].([]interface{})
	if !ok || len(paths) != 2 || paths[0] != "$.logicalRoundTrip" || paths[1] != "$.workers[0].futurePolicy" {
		t.Fatalf("warning paths = %#v, want sorted paths", context["ignored_json_paths"])
	}
	if strings.Contains(records[0].Message, "secret") || strings.Contains(fmt.Sprint(context), "secret") {
		t.Fatalf("warning leaked an ignored value: message=%q context=%#v", records[0].Message, context)
	}
}

func TestLoadRuntimePropagatesReplayInputFailure(t *testing.T) {
	t.Parallel()

	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	want := errors.New("recording read unavailable")
	replayInputs := runtimeInputReplayFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.Path != "recording.json" {
			t.Fatalf("path = %q", request.Path)
		}
		return recordings.LoadReplayInputResult{}, &recordings.ReplayInputError{Family: recordings.ReplayInputFamilyPortable,
			Diagnostic: recordings.ReplayArtifactDiagnostic{Code: recordings.ReplayArtifactDiagnosticDependencyFailure}, Cause: want}
	})
	_, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		replayInputs,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		root.BaseLogger).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           "recording.json",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       root.FactoryRootDir,
		HistoricalInspection: true})
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

	path := "legacy-replay.json"
	want := errors.New("legacy replay unavailable")
	replayInputs := runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		return recordings.LoadReplayInputResult{}, &recordings.ReplayInputError{Family: recordings.ReplayInputFamilyLegacy, Cause: fmt.Errorf("load replay artifact: %w", want)}
	})
	loaded, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		replayInputs,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           path,
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       t.TempDir(),
		HistoricalInspection: true})
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
	capability := runtimeInputReplayFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.Path != "legacy-replay.json" {
			t.Fatalf("replay path = %q", request.Path)
		}
		return recordings.LoadReplayInputResult{Legacy: artifact}, nil
	})
	loaded, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		func(snapshot *factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			var config factorydefinitions.FactoryConfig
			if err := json.Unmarshal(*snapshot, &config); err != nil {
				return nil, err
			}
			return runtimefixtures.ReplayRuntimeConfigValue(&config, t.TempDir()), nil
		},
		capability,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "runtime-base",
		ReplayPath:           "legacy-replay.json",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       t.TempDir(),
		HistoricalInspection: false})
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

func TestLoadRuntimeUsesDetachedSnapshotWithoutReloadingAuthoredSource(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir:                      factoryDir,
		RuntimeBaseDir:                  filepath.Join(factoryDir, "runtime"),
		DefinitionVersion:               &factorydefinitions.FactoryVersion{Logical: 4},
		InvocationSensitiveJSONPointers: []string{"/workers/0/body"},
		InvocationSensitiveJSONSpans:    []factorydefinitions.InvocationSensitiveJSONSpan{{JSONPointer: "/workers/0/body", Start: 2, End: 7}},
		PromptProvenance:                []factorydefinitions.RuntimePromptProvenance{{Name: "worker", Body: "${input}"}},
		EffectiveFactory: factorydefinitions.FactoryConfig{
			Name:    "snapshot-factory",
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				Name: "station", WorkerTypeName: "worker",
			}},
		},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "station", WorkerTypeName: "worker",
		}},
		PromptSources: []factorydefinitions.RuntimePromptSource{
			{Role: "worker", Name: "worker", Path: "workers/worker.md"},
			{Role: "workstation", Name: "station", Path: "workstations/station.md", IsTemplate: true},
		},
	}
	loadCalls := 0
	loggerSessionID := ""
	var passedConfig *factorydefinitions.FactoryConfig
	newLoadedFactory := func(
		dir string,
		config *factorydefinitions.FactoryConfig,
		lookup factorydefinitions.RuntimeDefinitionLookup,
		replacements []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		passedConfig, _ = factorydefinitions.CloneFactoryConfig(config)
		return factorydefinitionfixtures.NewLoadedSource(dir, config, lookup, replacements)
	}
	loaded, err := NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		loadCalls++
		return nil, errors.New("authored loader must not run")
	},
		newLoadedFactory,
		runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			panic("unexpected replay read")
		}),
		runtimeInputUnusedCapture,
		func(base *zap.Logger, sessionID, _, _ string) *zap.Logger {
			loggerSessionID = sessionID
			return base
		},
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: factoryDir,
		ExecutionBaseDir:     "",
		ReplayPath:           "",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       factoryDir,
		ResolvedSnapshot:     &snapshot,
		PreloadedReplayInput: nil,
		SessionID:            "session-1",
		HistoricalInspection: true})
	if err != nil {
		t.Fatalf("loadRuntime(snapshot) error = %v", err)
	}
	spans := loaded.LoadedFactoryCfg.(interface {
		InvocationSensitiveJSONSpans() []factorydefinitions.InvocationSensitiveJSONSpan
	}).InvocationSensitiveJSONSpans()
	if len(spans) != 1 || spans[0] != snapshot.InvocationSensitiveJSONSpans[0] {
		t.Fatalf("detached spans = %v", spans)
	}
	spans[0].Start = 99
	provenance, ok := loaded.LoadedFactoryCfg.(factorydefinitions.RuntimePromptProvenanceLookup).WorkerPromptProvenance("worker")
	if !ok || provenance.Body != "${input}" || snapshot.InvocationSensitiveJSONSpans[0].Start != 2 {
		t.Fatal("detached provenance changed or shared mutable spans")
	}
	if loadCalls != 0 {
		t.Fatalf("authored source loader calls = %d, want zero", loadCalls)
	}
	if loggerSessionID != "session-1" {
		t.Fatalf("snapshot runtime logger session ID = %q, want session-1", loggerSessionID)
	}
	if loaded.LoadedFactoryCfg == nil {
		t.Fatal("loaded Factory source = nil")
	}
	if got := loaded.LoadedFactoryCfg.FactoryDir(); got != factoryDir {
		t.Fatalf("loaded Factory directory = %q, want %q", got, factoryDir)
	}
	if got := loaded.LoadedFactoryCfg.RuntimeBaseDir(); got != snapshot.RuntimeBaseDir {
		t.Fatalf("loaded runtime base directory = %q, want %q", got, snapshot.RuntimeBaseDir)
	}
	if passedConfig == nil || passedConfig.Workers[0].PromptSourcePath != "workers/worker.md" ||
		passedConfig.Workstations[0].PromptSourcePath != "workstations/station.md" ||
		!passedConfig.Workstations[0].PromptSourceIsTemplate {
		t.Fatalf("snapshot prompt metadata passed to source factory = %#v", passedConfig)
	}
}

func TestLoadRuntimeRejectsPortableFailureMatrixBeforeFactoryConstruction(t *testing.T) {
	t.Parallel()

	testCases := []runtimeLoadPortableFailureCase{
		{"missing input", os.ErrNotExist, recordings.ReplayArtifactDiagnosticDependencyFailure},
		{"portable read failure", errors.New("reader unavailable"), recordings.ReplayArtifactDiagnosticDependencyFailure},
		{"malformed JSON", errors.New("malformed"), recordings.ReplayArtifactDiagnosticMalformed},
		{"trailing document", errors.New("trailing"), recordings.ReplayArtifactDiagnosticMalformed},
		{"unsupported compatibility version", errors.New("version"), recordings.ReplayArtifactDiagnosticUnsupportedVersion},
		{"invalid identity", errors.New("identity"), recordings.ReplayArtifactDiagnosticInvalidIdentity},
		{"invalid summary", errors.New("summary"), recordings.ReplayArtifactDiagnosticInvalidSummary},
		{"invalid event order", errors.New("order"), recordings.ReplayArtifactDiagnosticInvalidOrder},
		{"invalid integrity", errors.New("integrity"), recordings.ReplayArtifactDiagnosticInvalidIntegrity},
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
	capability := runtimeInputReplayFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.Path != "portable-replay.json" {
			t.Fatalf("read path = %q", request.Path)
		}
		return recordings.LoadReplayInputResult{}, &recordings.ReplayInputError{Family: recordings.ReplayInputFamilyPortable,
			Diagnostic: recordings.ReplayArtifactDiagnostic{Code: testCase.wantCode}, Cause: testCase.readErr}
	})
	loaded, err := NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
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
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           "portable-replay.json",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       t.TempDir(),
		HistoricalInspection: true})
	if !errors.Is(err, testCase.readErr) {
		t.Fatalf("lost selected input cause: %v", err)
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
	capability := runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		return recordings.LoadReplayInputResult{}, &recordings.ReplayInputError{Family: recordings.ReplayInputFamilyLegacy,
			Diagnostic: failure.Diagnostic, Cause: fmt.Errorf("load replay artifact: %w", failure)}
	})
	loaded, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		capability,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           "legacy-replay.json",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       t.TempDir(),
		HistoricalInspection: true})
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

func runtimeInputPortableFixture() recordings.PortableRecording {
	instant := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	return recordings.PortableRecording{
		RecordingKind: recordings.KindJavaScriptFactorySession, SchemaVersion: "2", ReplayCompatibilityVersion: "1",
		Session:         recordings.PortableRecordingSessionSummary{ID: "session-js-001", Status: "COMPLETED", OrchestratorKind: "JAVASCRIPT"},
		Source:          recordings.PortableRecordingSourceSummary{Ref: "workflow/example.js", Hash: "sha256:" + strings.Repeat("1", 64)},
		ArgumentsDigest: "sha256:" + strings.Repeat("2", 64), PolicyHash: "sha256:" + strings.Repeat("3", 64),
		Artifacts: []recordings.PortableRecordingArtifactSummary{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("4", 64), CreatedAt: instant}},
		Events:    []recordings.PortableRecordingEventSummary{{ID: "event-1", Type: "SESSION_STARTED", Timestamp: instant}, {ID: "event-2", Type: "SESSION_COMPLETED", Sequence: 1, Timestamp: instant.Add(time.Second), ArtifactIDs: []string{"artifact-1"}}},
		Result:    &recordings.PortableRecordingResult{Status: "FINAL", Mode: "final", ArtifactIDs: []string{"artifact-1"}},
		Redaction: recordings.PortableRecordingRedactionMetadata{RuntimeStateOmitted: true, CheckpointBodiesOmitted: true, ProviderTranscriptsOmitted: true, ChildDispatchesOmitted: true, SecretsRedacted: 2},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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

func TestClockForReplayRetainsSelectedIdentityWithoutFallback(t *testing.T) {
	t.Parallel()
	selected := clockwork.NewFakeClock()
	artifact := &factorydefinitions.ReplayArtifact{}
	for _, explicit := range []bool{true, false} {
		t.Run(fmt.Sprintf("explicit=%t", explicit), func(t *testing.T) {
			t.Parallel()
			var input factoryruntime.Clock
			if explicit {
				input = selected
			}
			got, err := clockForReplay(input, artifact,
				func(received *factorydefinitions.ReplayArtifact) recordings.Clock {
					if explicit || received != artifact {
						t.Fatal("replay clock selection bypassed explicit precedence or changed the artifact")
					}
					return selected
				},
				func(factoryruntime.Clock) factoryruntime.Clock {
					t.Fatal("selected clock reached the compatibility fallback")
					return nil
				})
			if err != nil || got != selected {
				t.Fatalf("clock = (%v, %v), want selected identity %v", got, err, selected)
			}
		})
	}
}

func TestLiveOpeningRetainsInjectedClockWithoutReplayCollaborators(t *testing.T) {
	t.Parallel()
	selected := clockwork.NewFakeClock()
	preparation := &RuntimePreparation{
		resolveClock: func(factoryruntime.Clock) factoryruntime.Clock {
			t.Fatal("live opening attempted fallback clock selection")
			return nil
		},
	}
	got, err := preparation.clockForOpening(selected, nil)
	if err != nil || got != selected {
		t.Fatalf("live clock = (%v, %v), want injected identity %v", got, err, selected)
	}
}

func TestOpenForRequestResumeUsesCapturedFactoryDefinition(t *testing.T) {
	t.Parallel()

	factorySnapshot := factorydefinitions.FactorySnapshot(`{"factoryDirectory":"/recorded","name":"recorded"}`)
	resumeInput := recordings.LoadResumeInputResult{
		Input: recordings.LoadReplayInputResult{
			Legacy: &factorydefinitions.ReplayArtifact{
				Factory: &factorySnapshot,
				Events: []factorydefinitions.FactoryEvent{{
					Id:      "resume-event",
					Context: factorydefinitions.FactoryEventContext{Tick: 4},
				}},
			},
		},
	}
	definitions := &resumeDefinitionStub{
		snapshot: factorydefinitions.RuntimeSnapshot{
			FactoryDir:     "/recorded",
			RuntimeBaseDir: "/recorded",
			EffectiveFactory: factorydefinitions.FactoryConfig{
				Name: "recorded",
			},
		},
	}
	root := &resumeRoutingRoot{}
	factory := &Root{
		runtimeRoot:               root,
		recordingsRuntime:         &resumeInputRuntime{result: resumeInput},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		snapshotSelection: NewRuntimeSnapshotSelection((definitions).ResolveRuntimeSnapshot, func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{factoryDir: "/recorded"}, nil
		}, nil, nil, nil),
	}

	_, err := factory.openForRequest(context.Background(), (runtimeOwnerFixture{
		FactoryDefinition: factorydefinitions.RuntimeSelection{
			Directory:        "/authored-b",
			SourcePath:       "/authored-b/factory.json",
			ExecutionBaseDir: "/authored-b",
		},
		Recordings: recordings.RuntimeSelection{
			RecordPath: "successor.recording.json",
			ResumePath: "source.recording.json",
		},
	}).startRequest())
	if err != nil {
		t.Fatalf("openForRequest(bare resume) error = %v", err)
	}
	if string(definitions.request.Canonical) != string(factorySnapshot) {
		t.Fatalf("resume Factory Definition canonical = %q, want captured recording definition", definitions.request.Canonical)
	}
	if definitions.request.Canonical != nil && strings.Contains(string(definitions.request.Canonical), "authored-b") {
		t.Fatalf("resume Factory Definition canonical selected authored input B: %q", definitions.request.Canonical)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want exactly one", root.activations)
	}
	if root.activation.Snapshot.EffectiveFactory.Name != "recorded" {
		t.Fatalf("activation Factory name = %q, want captured recording definition", root.activation.Snapshot.EffectiveFactory.Name)
	}
	if root.activation.Snapshot.FactoryDir != "/recorded" {
		t.Fatalf("activation Factory directory = %q, want captured recording directory", root.activation.Snapshot.FactoryDir)
	}
	if root.activation.Inputs.Recordings.RecordPath != "successor.recording.json" {
		t.Fatalf("activation successor path = %q, want successor.recording.json", root.activation.Inputs.Recordings.RecordPath)
	}
	if root.activation.Inputs.Recordings.ReplayPath != "" {
		t.Fatalf("activation replay path = %q, want empty for resume", root.activation.Inputs.Recordings.ReplayPath)
	}
}

type runtimeInputReplayFunc func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error)

func (f runtimeInputReplayFunc) LoadReplayInput(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return f(request)
}
func runtimeInputNoCurrentSource(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return nil, nil
}
func runtimeInputUnusedDecoder(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
	panic("unexpected replay decode")
}
func runtimeInputUnusedCapture(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
	panic("unexpected metadata capture")
}
func runtimeInputSessionLogger(base *zap.Logger, _, _, _ string) *zap.Logger { return base }

func TestRuntimeInputLoadingFreshFactsAndRetry(t *testing.T) {
	t.Parallel()
	cause := errors.New("selected source unavailable")
	base, logs := testdeps.CapturingZapLogger(zap.InfoLevel)
	calls := []string{}
	loader := NewRuntimeInputLoading(
		func(dir string, workstation factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			if workstation != nil {
				t.Fatal("opening supplied a workstation loader")
			}
			calls = append(calls, dir)
			if dir == "/failed" {
				return nil, cause
			}
			return factorydefinitionfixtures.NewLoadedSource(dir, &factorydefinitions.FactoryConfig{Name: dir,
				Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Type: factorydefinitions.WorkerTypeModel}}}, nil, nil)
		}, factorydefinitionfixtures.NewLoadedSource, runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			panic("unexpected replay read")
		}),
		runtimeInputUnusedCapture,
		func(logger *zap.Logger, id, root, dir string) *zap.Logger {
			if logger != base {
				t.Fatal("logger origin changed")
			}
			return logger.With(zap.String("session_id", id), zap.String("root", root), zap.String("source", dir))
		}, base)
	first, err := loader.Load(RuntimeInputLoadRequest{Dir: "/peer", ExecutionBaseDir: "/peer-base", FactoryRootDir: "/peer-root", SessionID: "peer"})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := loader.Load(RuntimeInputLoadRequest{Dir: "/failed", SessionID: "candidate"})
	if !errors.Is(err, cause) || !strings.HasPrefix(err.Error(), "load factory config: ") {
		t.Fatalf("failure = %v", err)
	}
	assertRuntimeInputFailureIsEmpty(t, failed)
	corrected, err := loader.Load(RuntimeInputLoadRequest{Dir: "/corrected", ExecutionBaseDir: "/corrected-base", FactoryRootDir: "/corrected-root", SessionID: "candidate",
		OperatorDefaults: operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModel: "selected-model"}})
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeInputRetryFacts(t, first, corrected, calls)
	entries := logs.FilterMessage("loading factory config").All()
	if len(entries) != 3 {
		t.Fatalf("accepted load logs = %v", entries)
	}
	for i, want := range []string{"peer", "candidate", "candidate"} {
		if entries[i].ContextMap()["session_id"] != want || entries[i].ContextMap()["source"] != calls[i] {
			t.Fatalf("load attribution = %v", entries[i].ContextMap())
		}
	}
}

func assertRuntimeInputFailureIsEmpty(t *testing.T, failed RuntimeLoad) {
	t.Helper()
	if failed.LoadedFactoryCfg != nil || failed.SessionLogger != nil || failed.ReplayArtifact != nil || failed.HistoricalReplay != nil {
		t.Fatalf("failure published partial inputs: %#v", failed)
	}
}

func assertRuntimeInputRetryFacts(t *testing.T, first, corrected RuntimeLoad, calls []string) {
	t.Helper()
	if first.LoadedFactoryCfg.FactoryDir() != "/peer" || first.LoadedFactoryCfg.RuntimeBaseDir() != "/peer-base" || corrected.LoadedFactoryCfg.RuntimeBaseDir() != "/corrected-base" {
		t.Fatal("a later request changed selected paths")
	}
	worker, ok := corrected.LoadedFactoryCfg.Worker("worker")
	if !ok || worker.Model != "selected-model" || worker.ModelProvider != "codex" {
		t.Fatalf("effective defaults = %#v", worker)
	}
	if len(calls) != 3 || calls[0] != "/peer" || calls[1] != "/failed" || calls[2] != "/corrected" {
		t.Fatalf("selected reads = %v", calls)
	}
}

func TestRuntimeInputLoadingDetachedMetadataSurvivesPeerSelection(t *testing.T) {
	t.Parallel()
	for _, withSpans := range []bool{false, true} {
		t.Run(fmt.Sprintf("spans=%v", withSpans), func(t *testing.T) {
			t.Parallel()
			snapshot := factorydefinitions.RuntimeSnapshot{
				FactoryDir: t.TempDir(), RuntimeBaseDir: t.TempDir(),
				EffectiveFactory:                factorydefinitions.FactoryConfig{Name: "selected"},
				InvocationSensitiveJSONPointers: []string{"/workers/0/body"},
				PromptProvenance: []factorydefinitions.RuntimePromptProvenance{
					{Name: "worker", Body: "selected ${input}"}, {Name: "station", Body: "selected ${task}"},
				},
			}
			if withSpans {
				snapshot.InvocationSensitiveJSONSpans = []factorydefinitions.InvocationSensitiveJSONSpan{{JSONPointer: "/workers/0/body", Start: 2, End: 7}}
			}
			loader := NewRuntimeInputLoading(runtimeInputNoCurrentSource, factorydefinitionfixtures.NewLoadedSource,
				runtimeInputUnusedDecoder, runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
					panic("unexpected replay read")
				}), runtimeInputUnusedCapture, runtimeInputSessionLogger, zap.NewNop())
			peer := factorydefinitions.RuntimeSnapshot{FactoryDir: t.TempDir(), RuntimeBaseDir: t.TempDir(),
				EffectiveFactory: factorydefinitions.FactoryConfig{Name: "peer"},
				PromptProvenance: []factorydefinitions.RuntimePromptProvenance{{Name: "worker", Body: "peer prompt"}}}
			if _, err := loader.Load(RuntimeInputLoadRequest{ResolvedSnapshot: &peer, SessionID: "peer"}); err != nil {
				t.Fatal(err)
			}
			selected, err := loader.Load(RuntimeInputLoadRequest{ResolvedSnapshot: &snapshot, SessionID: "selected"})
			if err != nil {
				t.Fatal(err)
			}
			// Change the caller-owned artifact and select a peer through the same
			// fixed loader. Neither operation may rewrite the detached result.
			snapshot.PromptProvenance[0].Body = "changed authored prompt"
			snapshot.InvocationSensitiveJSONPointers[0] = "/peer"
			if withSpans {
				snapshot.InvocationSensitiveJSONSpans[0].Start = 99
			}
			other, err := loader.Load(RuntimeInputLoadRequest{ResolvedSnapshot: &peer, SessionID: "peer"})
			if err != nil || other.LoadedFactoryCfg.FactoryDir() != peer.FactoryDir {
				t.Fatalf("peer selection = %#v, %v", other, err)
			}
			assertDetachedRuntimeMetadata(t, selected, snapshot, withSpans)
		})
	}
}

func assertDetachedRuntimeMetadata(t *testing.T, selected RuntimeLoad, snapshot factorydefinitions.RuntimeSnapshot, withSpans bool) {
	t.Helper()
	source := selected.LoadedFactoryCfg
	assertDetachedRuntimePathsAndProvenance(t, source, snapshot)
	if withSpans {
		assertDetachedRuntimeSpans(t, source)
		return
	}
	assertDetachedRuntimePointers(t, source)
}

func assertDetachedRuntimePathsAndProvenance(t *testing.T, source factorydefinitions.MutableLoadedFactorySource, snapshot factorydefinitions.RuntimeSnapshot) {
	t.Helper()
	if source.FactoryDir() != snapshot.FactoryDir || source.RuntimeBaseDir() != snapshot.RuntimeBaseDir {
		t.Fatal("peer changed selected source/base paths")
	}
	lookup := source.(factorydefinitions.RuntimePromptProvenanceLookup)
	worker, workerOK := lookup.WorkerPromptProvenance("worker")
	station, stationOK := lookup.WorkstationPromptProvenance("station")
	if !workerOK || !stationOK || worker.Body != "selected ${input}" || station.Body != "selected ${task}" {
		t.Fatalf("detached provenance = %#v, %#v", worker, station)
	}
}

func assertDetachedRuntimeSpans(t *testing.T, source factorydefinitions.MutableLoadedFactorySource) {
	t.Helper()
	lookup := source.(interface {
		InvocationSensitiveJSONSpans() []factorydefinitions.InvocationSensitiveJSONSpan
	})
	spans := lookup.InvocationSensitiveJSONSpans()
	if len(spans) != 1 || spans[0].JSONPointer != "/workers/0/body" || spans[0].Start != 2 || spans[0].End != 7 {
		t.Fatalf("selected spans = %v", spans)
	}
	spans[0].Start = 99
	if lookup.InvocationSensitiveJSONSpans()[0].Start != 2 {
		t.Fatal("span accessor exposed mutable metadata")
	}
}

func assertDetachedRuntimePointers(t *testing.T, source factorydefinitions.MutableLoadedFactorySource) {
	t.Helper()
	lookupPointers := source.(interface{ InvocationSensitiveJSONPointers() []string })
	pointers := lookupPointers.InvocationSensitiveJSONPointers()
	if len(pointers) != 1 || pointers[0] != "/workers/0/body" {
		t.Fatalf("selected pointers = %v", pointers)
	}
	pointers[0] = "/changed"
	if lookupPointers.InvocationSensitiveJSONPointers()[0] != "/workers/0/body" {
		t.Fatal("pointer accessor exposed mutable metadata")
	}
}

func TestRuntimeInputLoadingPreloadedReplayIsNotReadAgain(t *testing.T) {
	t.Parallel()
	snapshot := factorydefinitions.FactorySnapshot(`{"name":"recorded"}`)
	artifact := &recordings.ReplayArtifact{Factory: &snapshot}
	input := recordings.LoadReplayInputResult{Legacy: artifact}
	reads := 0
	loader := NewRuntimeInputLoading(runtimeInputNoCurrentSource, factorydefinitionfixtures.NewLoadedSource,
		func(got *factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			if got != &snapshot {
				t.Fatal("selected replay definition changed")
			}
			return runtimefixtures.ReplayRuntimeConfigValue(&factorydefinitions.FactoryConfig{Name: "recorded"}, "/recorded"), nil
		}, runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			reads++
			return input, nil
		}),
		runtimeInputUnusedCapture, runtimeInputSessionLogger, zap.NewNop())
	for _, preloaded := range []*recordings.LoadReplayInputResult{nil, &input} {
		loaded, err := loader.Load(RuntimeInputLoadRequest{ReplayPath: "selected.jsonl", ExecutionBaseDir: "/selected-base", PreloadedReplayInput: preloaded})
		if err != nil || loaded.ReplayArtifact != artifact || loaded.LoadedFactoryCfg.RuntimeBaseDir() != "/selected-base" {
			t.Fatalf("selected replay = %#v, %v", loaded, err)
		}
	}
	if reads != 1 {
		t.Fatalf("replay reads = %d, want one acquisition and zero rereads", reads)
	}
}

func TestRuntimeInputLoadingRejectsNilLoggerResultBeforeReading(t *testing.T) {
	t.Parallel()
	loader := NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("source read before logger success")
		return nil, nil
	},
		factorydefinitionfixtures.NewLoadedSource, runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			t.Fatal("replay read before logger success")
			return recordings.LoadReplayInputResult{}, nil
		}),
		runtimeInputUnusedCapture, func(*zap.Logger, string, string, string) *zap.Logger { return nil }, zap.NewNop())
	loaded, err := loader.Load(RuntimeInputLoadRequest{ReplayPath: "selected.jsonl"})
	if err == nil || err.Error() != "Factory Runtime session logger factory returned nil" || loaded.SessionLogger != nil {
		t.Fatalf("nil logger result = %#v, %v", loaded, err)
	}
}

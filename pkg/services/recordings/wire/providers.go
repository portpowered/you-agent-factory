package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// NewPortableRecordingWriter constructs the portable recording writer selected
// by process-graph composition without importing the transitional artifacts/
// shim.
func NewPortableRecordingWriter(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
) (recordings.PortableRecordingWriter, error) {
	return artifactsimpl.NewAtomicWriter(makeDirectories, createTemporaryFile, removePath, renamePath)
}

// NewReplayArtifactLoader constructs the replay artifact loader selected by
// process-graph composition without importing the transitional replay/ shim.
func NewReplayArtifactLoader(
	storage platformreplay.Storage,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) recordings.ReplayArtifactLoader {
	return func(path string) (*recordings.ReplayArtifact, error) {
		return replayimpl.Load(storage, path, decodeFactorySnapshot)
	}
}

// NewReplayInputLoader constructs the ReplayInputCapability selected by
// process-graph composition, composing the existing portable-recording
// decoder/validator with the existing legacy replay artifact loader behind
// one Recordings-owned capability so callers no longer combine a raw file
// reader, the aliased portable-recording decoder/validator, and the legacy
// loader themselves. logger is the repository's injected structured logging
// abstraction; LoadReplayInput never logs the replay input's decoded
// payload, only stable identifiers and outcome classification.
func NewReplayInputLoader(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger *zap.Logger,
) recordings.ReplayInputCapability {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &replayInputLoader{readFile: readFile, loadLegacy: loadLegacy, logger: logger}
}

type replayInputLoader struct {
	readFile   recordings.RecordingReadFile
	loadLegacy recordings.ReplayArtifactLoader
	logger     *zap.Logger
}

var _ recordings.ReplayInputCapability = (*replayInputLoader)(nil)

func (loader *replayInputLoader) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	loader.logger.Info("loading replay input")
	if loader.readFile == nil {
		err := &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorRead,
			Message: "Factory Session replay recording reader is required",
		}
		loader.logger.Error("replay input reader is not configured")
		return recordings.LoadReplayInputResult{}, err
	}
	data, readErr := loader.readFile(request.Path)
	if readErr != nil {
		err := &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorRead,
			Message: fmt.Sprintf("read replay recording: %s", readErr.Error()),
			Cause:   readErr,
		}
		loader.logger.Error("failed to read replay input")
		return recordings.LoadReplayInputResult{}, err
	}
	var header struct {
		RecordingKind string `json:"recordingKind"`
	}
	if err := json.Unmarshal(data, &header); err == nil &&
		header.RecordingKind == recordings.KindJavaScriptFactorySession {
		value, decodeErr := recordings.DecodePortableRecording(bytes.NewReader(data))
		if decodeErr != nil {
			typed := &recordings.ReplayInputError{
				Kind:       recordings.ReplayInputErrorPortable,
				Diagnostic: toReplayInputDiagnostic(decodeErr),
				Message:    decodeErr.Error(),
				Cause:      decodeErr,
			}
			loader.logger.Warn(
				"replay input failed portable recording validation",
				zap.String("diagnosticCode", diagnosticCodeOf(typed.Diagnostic)),
			)
			return recordings.LoadReplayInputResult{}, typed
		}
		loader.logger.Info(
			"loaded portable replay input",
			zap.String("sessionID", value.Session.ID),
		)
		return recordings.LoadReplayInputResult{Portable: &value}, nil
	}
	if loader.loadLegacy == nil {
		err := &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorLegacy,
			Message: "replay artifact loader is required",
		}
		loader.logger.Error("legacy replay artifact loader is not configured")
		return recordings.LoadReplayInputResult{}, err
	}
	artifact, legacyErr := loader.loadLegacy(request.Path)
	if legacyErr != nil {
		err := &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorLegacy,
			Message: fmt.Sprintf("load replay artifact: %s", legacyErr.Error()),
			Cause:   legacyErr,
		}
		loader.logger.Warn("failed to load legacy replay artifact")
		return recordings.LoadReplayInputResult{}, err
	}
	loader.logger.Info("loaded legacy replay artifact")
	return recordings.LoadReplayInputResult{Legacy: artifact}, nil
}

// toReplayInputDiagnostic maps the existing portable-recording diagnostic
// into the directly owned ReplayInputDiagnostic vocabulary so callers do not
// depend on recordings/internal/contracts to observe structured facts.
func toReplayInputDiagnostic(err error) *recordings.ReplayInputDiagnostic {
	var diagnostic *recordings.PortableRecordingDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic == nil {
		return nil
	}
	var versions []string
	if len(diagnostic.SupportedVersions) > 0 {
		versions = append([]string(nil), diagnostic.SupportedVersions...)
	}
	return &recordings.ReplayInputDiagnostic{
		Code:              recordings.ReplayInputDiagnosticCode(diagnostic.Code),
		Area:              diagnostic.Area,
		Path:              diagnostic.Path,
		Message:           diagnostic.Message,
		SupportedVersions: versions,
	}
}

func diagnosticCodeOf(diagnostic *recordings.ReplayInputDiagnostic) string {
	if diagnostic == nil {
		return ""
	}
	return string(diagnostic.Code)
}

// NewProjectionService constructs the Recordings projection capability for
// process-graph composition.
func NewProjectionService() recordings.ProjectionService {
	return recordingsinternal.NewProjectionService()
}

// NewRuntimeLedger constructs a runtime event ledger for process-graph
// composition.
func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingsinternal.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}

// NewLifecycleRuntimeRecorder constructs the runtime recorder used while
// opening Factory Session runtime state.
func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (recordings.RuntimeRecorder, error) {
	return recordingsinternal.NewLifecycleRuntimeRecorder(
		flushInterval,
		loaded,
		now,
		recordPath,
		captureLoadedFactorySnapshot,
	)
}

// NewReplayClock constructs the replay clock selected from a replay artifact.
func NewReplayClock(artifact *recordings.ReplayArtifact) recordings.Clock {
	return recordingsinternal.NewReplayClock(artifact)
}

// NewReplayExecution constructs replay execution collaborators from a replay
// artifact and the canonical definition decoders selected by the process graph.
func NewReplayExecution(
	artifact *recordings.ReplayArtifact,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
) (
	workers.Provider,
	workers.CommandRunner,
	[]recordings.ReplayHook,
	recordings.CompletionDeliveryPlanner,
	error,
) {
	return recordingsinternal.NewReplayExecution(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
}

// NewServiceWithProjection constructs the Recordings root from a caller-owned
// ledger and projection. Process composition uses this when runtime opening
// shares one projection instance across reconnect validation and root assembly.
func NewServiceWithProjection(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	if writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	return NewServiceWithProjectionAndEffects(
		ledger,
		projection,
		targets,
		writeFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		clocks...,
	)
}

// NewServiceWithProjectionAndEffects constructs the Recordings root with the
// exact portable-artifact filesystem effects selected by the application
// graph. This owner wire adapts those effects into the private artifact
// publication capability and selects no host defaults.
func NewServiceWithProjectionAndEffects(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	publication, err := recordingsinternal.NewPortableArtifactPublication(
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Recordings publication: %w", err)
	}
	return newServiceWithProjection(
		ledger,
		projection,
		targets,
		writeFile,
		publication,
		false,
		clocks...,
	)
}

type portableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

func newServiceWithProjection(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	publication portableArtifactPublication,
	requireWriter bool,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	if requireWriter && writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	var writer recordings.RecordingSnapshotWriter
	var tickers recordings.RecordingFlushTickerFactory
	if writeFile != nil {
		writer = recordingsinternal.NewReplayRecordingSnapshotWriter(writeFile)
		tickers = recordingsinternal.NewRecordingFlushTickerFactory()
	}
	service := recordingsinternal.NewServiceWithLifecycleEffects(
		ledger,
		projection,
		targets,
		writer,
		tickers,
		publication,
		clocks...,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Recordings: implementation rejected its dependencies")
	}
	return service, nil
}

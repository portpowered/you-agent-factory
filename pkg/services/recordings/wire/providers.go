package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

// NewReplayArtifactCapability constructs the path-based, pre-ledger
// recordings.RecordingReplayArtifacts implementation selected by
// process-graph composition, composing the existing portable-recording
// decoder/validator with the existing legacy replay artifact loader behind
// the one Recordings-owned replay/artifact capability so callers no longer
// combine a raw file reader, the aliased portable-recording decoder/
// validator, and the legacy loader themselves.
//
// This implementation only supports LoadReplayInput: it is constructed and
// injected before a Factory Session ledger exists (while Factory Sessions
// opens runtime state from historical replay input), so its ledger-scoped
// operations (LoadReplay, BuildArtifact, ValidateArtifact, EncodeArtifact,
// DecodeArtifact, SummarizeArtifact, ExportArtifact, ReadArtifact) return a
// ReplayArtifactErrorUnsupportedContext failure rather than fabricating
// ledger access they do not have.
func NewReplayArtifactCapability(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
) recordings.RecordingReplayArtifacts {
	return &replayArtifactCapability{readFile: readFile, loadLegacy: loadLegacy}
}

type replayArtifactCapability struct {
	readFile   recordings.RecordingReadFile
	loadLegacy recordings.ReplayArtifactLoader
}

var _ recordings.RecordingReplayArtifacts = (*replayArtifactCapability)(nil)

func (loader *replayArtifactCapability) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if loader.readFile == nil {
		return recordings.LoadReplayInputResult{}, fmt.Errorf("Factory Session replay recording reader is required")
	}
	data, err := loader.readFile(request.Path)
	if err != nil {
		return recordings.LoadReplayInputResult{}, fmt.Errorf("read replay recording: %w", err)
	}
	var header struct {
		RecordingKind string `json:"recordingKind"`
	}
	if err := json.Unmarshal(data, &header); err == nil &&
		header.RecordingKind == recordings.KindJavaScriptFactorySession {
		value, err := recordings.DecodePortableRecording(bytes.NewReader(data))
		if err != nil {
			return recordings.LoadReplayInputResult{}, err
		}
		return recordings.LoadReplayInputResult{Portable: &value}, nil
	}
	if loader.loadLegacy == nil {
		return recordings.LoadReplayInputResult{}, fmt.Errorf("replay artifact loader is required")
	}
	artifact, err := loader.loadLegacy(request.Path)
	if err != nil {
		return recordings.LoadReplayInputResult{}, fmt.Errorf("load replay artifact: %w", err)
	}
	return recordings.LoadReplayInputResult{Legacy: artifact}, nil
}

func (loader *replayArtifactCapability) unsupportedContext(operation string) error {
	return &recordings.ReplayArtifactError{
		Kind:    recordings.ReplayArtifactErrorUnsupportedContext,
		Message: operation + " requires an already-open Factory Session recording ledger",
		Cause:   recordings.ErrReplayArtifactUnsupportedContext,
	}
}

func (loader *replayArtifactCapability) LoadReplay(
	recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	return recordings.LoadReplayResult{}, loader.unsupportedContext("LoadReplay")
}

func (loader *replayArtifactCapability) BuildArtifact(
	recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	return recordings.BuildArtifactResult{}, loader.unsupportedContext("BuildArtifact")
}

func (loader *replayArtifactCapability) ValidateArtifact(
	recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	return recordings.ValidateArtifactResult{}, loader.unsupportedContext("ValidateArtifact")
}

func (loader *replayArtifactCapability) EncodeArtifact(
	recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	return recordings.EncodeArtifactResult{}, loader.unsupportedContext("EncodeArtifact")
}

func (loader *replayArtifactCapability) DecodeArtifact(
	recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	return recordings.DecodeArtifactResult{}, loader.unsupportedContext("DecodeArtifact")
}

func (loader *replayArtifactCapability) SummarizeArtifact(
	recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	return recordings.SummarizeArtifactResult{}, loader.unsupportedContext("SummarizeArtifact")
}

func (loader *replayArtifactCapability) ExportArtifact(
	context.Context, recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	return recordings.ExportArtifactResult{}, loader.unsupportedContext("ExportArtifact")
}

func (loader *replayArtifactCapability) ReadArtifact(
	context.Context, recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	return recordings.ReadArtifactResult{}, loader.unsupportedContext("ReadArtifact")
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

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

type replayInputLoader struct {
	readFile   recordings.RecordingReadFile
	loadLegacy recordings.ReplayArtifactLoader
	logger     *zap.Logger
}

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
		return recordings.LoadReplayInputResult{Portable: toReplayInputPortableRecording(value)}, nil
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
	legacy, conversionErr := toReplayInputLegacyArtifact(artifact)
	if conversionErr != nil {
		err := &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorLegacy,
			Message: fmt.Sprintf("prepare replay artifact: %s", conversionErr.Error()),
			Cause:   conversionErr,
		}
		loader.logger.Warn("failed to prepare legacy replay artifact")
		return recordings.LoadReplayInputResult{}, err
	}
	loader.logger.Info("loaded legacy replay artifact")
	return recordings.LoadReplayInputResult{Legacy: legacy}, nil
}

func newReplayInputLoader(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger *zap.Logger,
) *replayInputLoader {
	return &replayInputLoader{readFile: readFile, loadLegacy: loadLegacy, logger: logger}
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

// toReplayInputPortableRecording copies the legacy portable-recording
// compatibility value into the narrow capability's directly owned contract.
// The conversion deliberately clones every nested slice and pointer so a
// caller cannot mutate a later replay-input result through this value.
func toReplayInputPortableRecording(
	value recordings.PortableRecording,
) *recordings.ReplayInputPortableRecording {
	converted := &recordings.ReplayInputPortableRecording{
		RecordingKind:              value.RecordingKind,
		SchemaVersion:              value.SchemaVersion,
		ReplayCompatibilityVersion: value.ReplayCompatibilityVersion,
		Session: recordings.ReplayInputSessionSummary{
			ID:               value.Session.ID,
			Status:           value.Session.Status,
			OrchestratorKind: value.Session.OrchestratorKind,
		},
		Source: recordings.ReplayInputSourceSummary{
			Ref:  value.Source.Ref,
			Hash: value.Source.Hash,
		},
		ArgumentsDigest: value.ArgumentsDigest,
		PolicyHash:      value.PolicyHash,
		Artifacts:       toReplayInputArtifactSummaries(value.Artifacts),
		Events:          toReplayInputEventSummaries(value.Events),
		Checkpoint:      toReplayInputCheckpointSummary(value.Checkpoint),
		Result:          toReplayInputResultSummary(value.Result),
		Redaction: recordings.ReplayInputRedactionMetadata{
			RuntimeStateOmitted:        value.Redaction.RuntimeStateOmitted,
			CheckpointBodiesOmitted:    value.Redaction.CheckpointBodiesOmitted,
			ProviderTranscriptsOmitted: value.Redaction.ProviderTranscriptsOmitted,
			ChildDispatchesOmitted:     value.Redaction.ChildDispatchesOmitted,
			SecretsRedacted:            value.Redaction.SecretsRedacted,
		},
	}
	return converted
}

func toReplayInputArtifactSummaries(
	values []recordings.PortableRecordingArtifactSummary,
) []recordings.ReplayInputArtifactSummary {
	converted := make([]recordings.ReplayInputArtifactSummary, len(values))
	for index, artifact := range values {
		converted[index] = recordings.ReplayInputArtifactSummary{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			Label:       artifact.Label,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
			CreatedAt:   artifact.CreatedAt,
		}
	}
	return converted
}

func toReplayInputEventSummaries(
	values []recordings.PortableRecordingEventSummary,
) []recordings.ReplayInputEventSummary {
	converted := make([]recordings.ReplayInputEventSummary, len(values))
	for index, event := range values {
		converted[index] = recordings.ReplayInputEventSummary{
			ID:           event.ID,
			Type:         event.Type,
			Sequence:     event.Sequence,
			Timestamp:    event.Timestamp,
			ArtifactIDs:  append([]string(nil), event.ArtifactIDs...),
			CheckpointID: event.CheckpointID,
		}
	}
	return converted
}

func toReplayInputCheckpointSummary(
	value *recordings.PortableRecordingCheckpointSummary,
) *recordings.ReplayInputCheckpointSummary {
	if value == nil {
		return nil
	}
	return &recordings.ReplayInputCheckpointSummary{
		ID:         value.ID,
		Label:      value.Label,
		Summary:    value.Summary,
		Timestamp:  value.Timestamp,
		ArtifactID: value.ArtifactID,
	}
}

func toReplayInputResultSummary(
	value *recordings.PortableRecordingResult,
) *recordings.ReplayInputResultSummary {
	if value == nil {
		return nil
	}
	converted := &recordings.ReplayInputResultSummary{
		Status:        value.Status,
		Mode:          value.Mode,
		PrimaryResult: append([]byte(nil), value.PrimaryResult...),
		ContentHash:   value.ContentHash,
		ArtifactIDs:   append([]string(nil), value.ArtifactIDs...),
	}
	if value.Failure != nil {
		converted.Failure = &recordings.ReplayInputFailureSummary{
			Reason:                 value.Failure.Reason,
			Message:                value.Failure.Message,
			PartialResultAvailable: value.Failure.PartialResultAvailable,
		}
	}
	if value.Availability != nil {
		converted.Availability = &recordings.ReplayInputAvailability{
			Reason:    value.Availability.Reason,
			Message:   value.Availability.Message,
			Retryable: value.Availability.Retryable,
		}
	}
	return converted
}

func toReplayInputLegacyArtifact(
	value *recordings.ReplayArtifact,
) (*recordings.ReplayInputLegacyArtifact, error) {
	if value == nil {
		return nil, nil
	}
	converted := &recordings.ReplayInputLegacyArtifact{
		SchemaVersion: value.SchemaVersion,
		RecordedAt:    value.RecordedAt,
	}
	var err error
	if converted.FactorySnapshotJSON, err = json.Marshal(value.Factory); err != nil {
		return nil, fmt.Errorf("encode legacy Factory snapshot: %w", err)
	}
	if value.Factory == nil {
		converted.FactorySnapshotJSON = nil
	}
	if converted.DiagnosticsJSON, err = json.Marshal(value.Diagnostics); err != nil {
		return nil, fmt.Errorf("encode legacy replay diagnostics: %w", err)
	}
	converted.Events = make([]recordings.ReplayInputLegacyEvent, len(value.Events))
	for index, event := range value.Events {
		payload, encodeErr := json.Marshal(event)
		if encodeErr != nil {
			return nil, fmt.Errorf("encode legacy replay event %d: %w", index, encodeErr)
		}
		converted.Events[index] = recordings.ReplayInputLegacyEvent{EventJSON: payload}
	}
	if value.WallClock != nil {
		converted.WallClock = &recordings.ReplayInputWallClockMetadata{
			StartedAt:  value.WallClock.StartedAt,
			FinishedAt: value.WallClock.FinishedAt,
		}
	}
	return converted, nil
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

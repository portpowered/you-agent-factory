package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

var _ recordings.RecordingReplayArtifacts = (*combinedService)(nil)

// LoadReplay implements recordings.RecordingReplayArtifacts by adapting the
// existing LoadReplayRecording operation to the root-owned replay/artifact
// vocabulary.
func (service *combinedService) LoadReplay(
	request recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	result, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.LoadReplayResult{}, translateReplayArtifactError(err)
	}
	return recordings.LoadReplayResult{Replay: toReplayFacts(result.Recording)}, nil
}

// BuildArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing BuildPortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) BuildArtifact(
	request recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	result, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.BuildArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.BuildArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

// ValidateArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing ValidatePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) ValidateArtifact(
	request recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	result, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.ValidateArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ValidateArtifactResult{Summary: toArtifactSummary(result.Summary)}, nil
}

// EncodeArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing EncodePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) EncodeArtifact(
	request recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	result, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.EncodeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.EncodeArtifactResult{Payload: result.Payload}, nil
}

// DecodeArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing DecodePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) DecodeArtifact(
	request recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	result, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: request.Payload,
	})
	if err != nil {
		return recordings.DecodeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.DecodeArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

// SummarizeArtifact implements recordings.RecordingReplayArtifacts by
// adapting the existing SummarizePortableArtifact operation to the
// root-owned replay/artifact vocabulary.
func (service *combinedService) SummarizeArtifact(
	request recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	result, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.SummarizeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.SummarizeArtifactResult{Summary: toArtifactSummary(result.Summary)}, nil
}

// ExportArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing ExportPortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) ExportArtifact(
	ctx context.Context,
	request recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	result, err := service.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.ExportArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ExportArtifactResult{
		Reference: recordings.ArtifactReference(result.Reference),
		Artifact:  toArtifactEnvelope(result.Artifact),
	}, nil
}

// ReadArtifact implements recordings.RecordingReplayArtifacts by adapting the
// existing ReadPortableArtifact operation to the root-owned replay/artifact
// vocabulary.
func (service *combinedService) ReadArtifact(
	ctx context.Context,
	request recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	result, err := service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Reference:   recordings.RecordingArtifactReference(request.Reference),
	})
	if err != nil {
		return recordings.ReadArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ReadArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

// LoadReplayInput implements recordings.RecordingReplayArtifacts for the
// ledger-backed Recordings root. This construction path always has an
// already-recorded ledger and projection, never a bare filesystem path to
// classify, so it does not support LoadReplayInput; that operation is
// implemented by the path-based bootstrap capability Factory Sessions
// injects while opening runtime state, before any ledger exists (see
// pkg/services/recordings/wire.NewReplayArtifactCapability).
func (service *combinedService) LoadReplayInput(
	recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, unsupportedReplayArtifactContext()
}

// replayInputArtifactCapability is the path-based Recordings implementation
// Factory Sessions receives before a recording ledger exists. It owns the
// portable-versus-legacy classification and its safe operation observability;
// callers receive only the narrow RecordingReplayArtifacts contract.
type replayInputArtifactCapability struct {
	readFile   recordings.RecordingReadFile
	loadLegacy recordings.ReplayArtifactLoader
	logger     logging.Logger
}

var _ recordings.RecordingReplayArtifacts = (*replayInputArtifactCapability)(nil)

// NewReplayArtifactCapability constructs the path-based replay/artifact
// capability from the exact reader, legacy loader, and process logger selected
// by Recordings Wire. It is inert: it performs no I/O until LoadReplayInput.
func NewReplayArtifactCapability(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger logging.Logger,
) recordings.RecordingReplayArtifacts {
	return &replayInputArtifactCapability{
		readFile:   readFile,
		loadLegacy: loadLegacy,
		logger:     logging.EnsureLogger(logger),
	}
}

func (loader *replayInputArtifactCapability) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	loader.logReplayInputIntent()
	if loader.readFile == nil {
		return loader.replayInputDependencyFailure("reader_unavailable", fmt.Errorf("Factory Session replay recording reader is required"))
	}
	data, err := loader.readFile(request.Path)
	if err != nil {
		return loader.replayInputDependencyFailure("read_failure", fmt.Errorf("read replay recording: %w", err))
	}
	if isPortableReplayInput(data) {
		return loader.loadPortableReplayInput(data)
	}
	return loader.loadLegacyReplayInput(request.Path)
}

func isPortableReplayInput(data []byte) bool {
	var header struct {
		RecordingKind string `json:"recordingKind"`
	}
	return json.Unmarshal(data, &header) == nil &&
		header.RecordingKind == recordings.KindJavaScriptFactorySession
}

func (loader *replayInputArtifactCapability) loadPortableReplayInput(
	data []byte,
) (recordings.LoadReplayInputResult, error) {
	value, err := recordings.DecodePortableRecording(bytes.NewReader(data))
	if err != nil {
		failure := newReplayInputError(recordings.ReplayInputFamilyPortable, err)
		loader.logReplayInputOutcome("validation_failure", string(failure.Diagnostic.Code), "")
		return recordings.LoadReplayInputResult{}, failure
	}
	loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyPortable))
	return recordings.LoadReplayInputResult{Portable: &value}, nil
}

func (loader *replayInputArtifactCapability) loadLegacyReplayInput(
	path string,
) (recordings.LoadReplayInputResult, error) {
	if loader.loadLegacy == nil {
		return loader.replayInputDependencyFailure("legacy_loader_unavailable", fmt.Errorf("replay artifact loader is required"))
	}
	artifact, err := loader.loadLegacy(path)
	if err != nil {
		failure := newReplayInputError(
			recordings.ReplayInputFamilyLegacy,
			fmt.Errorf("load replay artifact: %w", err),
		)
		loader.logReplayInputOutcome("dependency_failure", string(failure.Diagnostic.Code), "")
		return recordings.LoadReplayInputResult{}, failure
	}
	loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyLegacy))
	return recordings.LoadReplayInputResult{Legacy: artifact}, nil
}

func (loader *replayInputArtifactCapability) replayInputDependencyFailure(
	classification string,
	cause error,
) (recordings.LoadReplayInputResult, error) {
	failure := newReplayInputError(recordings.ReplayInputFamilyPortable, cause)
	outcome := "dependency_failure"
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		outcome = "canceled"
	}
	loader.logReplayInputOutcome(outcome, classification, "")
	return recordings.LoadReplayInputResult{}, failure
}

func (loader *replayInputArtifactCapability) logReplayInputIntent() {
	loader.logger.Info(
		"recordings replay input accepted",
		"operation", "load_replay_input",
		"input_source", "filesystem_path",
	)
}

func (loader *replayInputArtifactCapability) logReplayInputOutcome(
	outcome string,
	classification string,
	family string,
) {
	fields := []any{"operation", "load_replay_input", "outcome", outcome}
	if classification != "" {
		fields = append(fields, "error_class", classification)
	}
	if family != "" {
		fields = append(fields, "replay_family", family)
	}
	loader.logger.Info("recordings replay input outcome", fields...)
}

func newReplayInputError(family recordings.ReplayInputFamily, cause error) *recordings.ReplayInputError {
	return &recordings.ReplayInputError{
		Family:     family,
		Diagnostic: replayInputDiagnostic(cause),
		Cause:      cause,
	}
}

func (loader *replayInputArtifactCapability) LoadReplay(
	recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	return recordings.LoadReplayResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) BuildArtifact(
	recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	return recordings.BuildArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) ValidateArtifact(
	recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	return recordings.ValidateArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) EncodeArtifact(
	recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	return recordings.EncodeArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) DecodeArtifact(
	recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	return recordings.DecodeArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) SummarizeArtifact(
	recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	return recordings.SummarizeArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) ExportArtifact(
	context.Context,
	recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	return recordings.ExportArtifactResult{}, unsupportedReplayArtifactContext()
}

func (loader *replayInputArtifactCapability) ReadArtifact(
	context.Context,
	recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	return recordings.ReadArtifactResult{}, unsupportedReplayArtifactContext()
}

func unsupportedReplayArtifactContext() error {
	return &recordings.ReplayArtifactError{
		Kind: recordings.ReplayArtifactErrorUnsupportedContext,
		Diagnostic: recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticUnsupportedContext,
			Area:    "capability",
			Path:    "operation",
			Message: "operation is unavailable in this Recordings capability context",
		},
		Cause: recordings.ErrReplayArtifactUnsupportedContext,
	}
}

func replayInputDiagnostic(err error) recordings.ReplayArtifactDiagnostic {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticCancelled,
			Area:    "input",
			Path:    "replayInput",
			Message: "replay input loading was canceled",
		}
	}
	var diagnostic *recordings.PortableRecordingDiagnostic
	if errors.As(err, &diagnostic) {
		return replayArtifactDiagnosticFromPortable(diagnostic)
	}
	return replayInputDependencyDiagnostic()
}

func replayInputDependencyDiagnostic() recordings.ReplayArtifactDiagnostic {
	return recordings.ReplayArtifactDiagnostic{
		Code:    recordings.ReplayArtifactDiagnosticDependencyFailure,
		Area:    "input",
		Path:    "replayInput",
		Message: "replay input could not be loaded",
	}
}

func replayArtifactDiagnosticFromPortable(
	diagnostic *recordings.PortableRecordingDiagnostic,
) recordings.ReplayArtifactDiagnostic {
	if diagnostic == nil {
		return replayInputDependencyDiagnostic()
	}
	result := recordings.ReplayArtifactDiagnostic{
		Area: diagnostic.Area,
		Path: safeReplayArtifactDiagnosticPath(diagnostic.Path),
	}
	switch diagnostic.Code {
	case recordings.PortableRecordingCodeMalformedContract:
		result.Code = recordings.ReplayArtifactDiagnosticMalformed
		result.Message = "recording document is malformed"
	case recordings.PortableRecordingCodeUnsupportedVersion:
		result.Code = recordings.ReplayArtifactDiagnosticUnsupportedVersion
		result.Message = "recording uses an unsupported replay compatibility version"
		result.SupportedVersions = slices.Clone(diagnostic.SupportedVersions)
	case recordings.PortableRecordingCodeInvalidIdentity:
		result.Code = recordings.ReplayArtifactDiagnosticInvalidIdentity
		result.Message = "recording identity is invalid"
	case recordings.PortableRecordingCodeInvalidDigest:
		result.Code = recordings.ReplayArtifactDiagnosticInvalidIntegrity
		result.Message = "recording integrity is invalid"
	case recordings.PortableRecordingCodeInvalidSummary:
		result.Code = recordings.ReplayArtifactDiagnosticInvalidSummary
		result.Message = "recording summary is invalid"
	default:
		return replayInputDependencyDiagnostic()
	}
	return result
}

func safeReplayArtifactDiagnosticPath(path string) string {
	for _, character := range path {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '.' && character != '[' && character != ']' {
			return ""
		}
	}
	return path
}

func toReplayScope(scope recordings.CanonicalEventScope) recordings.ReplayScope {
	return recordings.ReplayScope{FactorySessionID: scope.FactorySessionID}
}

func fromReplayScope(scope recordings.ReplayScope) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: scope.FactorySessionID}
}

func toReplayEventCursor(cursor *recordings.CanonicalEventCursor) *recordings.ReplayEventCursor {
	if cursor == nil {
		return nil
	}
	detached := recordings.ReplayEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
	return &detached
}

func fromReplayEventCursor(cursor recordings.ReplayEventCursor) recordings.CanonicalEventCursor {
	return recordings.CanonicalEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           recordings.CanonicalEventSequence(cursor.Sequence),
	}
}

func toReplayEventCursorValue(cursor recordings.CanonicalEventCursor) recordings.ReplayEventCursor {
	return recordings.ReplayEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
}

func toReplayEvents(events []recordings.CanonicalEvent) []recordings.ReplayEvent {
	if len(events) == 0 {
		return nil
	}
	detached := make([]recordings.ReplayEvent, len(events))
	for index, event := range events {
		detached[index] = recordings.ReplayEvent{
			ID:            string(event.ID),
			Sequence:      int64(event.Sequence),
			FactoryTick:   event.FactoryTick,
			Scope:         toReplayScope(event.Scope),
			Cursor:        toReplayEventCursorValue(event.Cursor),
			RecordedAt:    event.RecordedAt,
			Kind:          string(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		}
	}
	return detached
}

func fromReplayEvents(events []recordings.ReplayEvent) []recordings.CanonicalEvent {
	if len(events) == 0 {
		return nil
	}
	detached := make([]recordings.CanonicalEvent, len(events))
	for index, event := range events {
		detached[index] = recordings.CanonicalEvent{
			ID:            recordings.CanonicalEventID(event.ID),
			Sequence:      recordings.CanonicalEventSequence(event.Sequence),
			FactoryTick:   event.FactoryTick,
			Scope:         fromReplayScope(event.Scope),
			Cursor:        fromReplayEventCursor(event.Cursor),
			RecordedAt:    event.RecordedAt,
			Kind:          recordings.CanonicalEventKind(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		}
	}
	return detached
}

func toReplayFacts(facts recordings.ReplayRecordingFacts) recordings.ReplayFacts {
	return recordings.ReplayFacts{
		RecordingID: recordings.ReplayRecordingID(facts.RecordingID),
		Scope:       toReplayScope(facts.Scope),
		Events:      toReplayEvents(facts.Events),
	}
}

func toArtifactFailures(failures []recordings.RecordingFailure) []recordings.ArtifactFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.ArtifactFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.ArtifactFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func fromArtifactFailures(failures []recordings.ArtifactFailure) []recordings.RecordingFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.RecordingFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.RecordingFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func toArtifactSummary(summary recordings.PortableArtifactSummary) recordings.ArtifactSummary {
	return recordings.ArtifactSummary{
		RecordingID: recordings.ReplayRecordingID(summary.RecordingID),
		Reference:   recordings.ArtifactReference(summary.Reference),
		Scope:       toReplayScope(summary.Scope),
		State:       recordings.ArtifactState(summary.State),
		EventCount:  summary.EventCount,
		FirstCursor: toReplayEventCursor(summary.FirstCursor),
		LastCursor:  toReplayEventCursor(summary.LastCursor),
		Failures:    toArtifactFailures(summary.Failures),
		Available:   summary.Available,
	}
}

func fromArtifactSummary(summary recordings.ArtifactSummary) recordings.PortableArtifactSummary {
	var firstCursor, lastCursor *recordings.CanonicalEventCursor
	if summary.FirstCursor != nil {
		cursor := fromReplayEventCursor(*summary.FirstCursor)
		firstCursor = &cursor
	}
	if summary.LastCursor != nil {
		cursor := fromReplayEventCursor(*summary.LastCursor)
		lastCursor = &cursor
	}
	return recordings.PortableArtifactSummary{
		RecordingID: recordings.RecordingID(summary.RecordingID),
		Reference:   recordings.RecordingArtifactReference(summary.Reference),
		Scope:       fromReplayScope(summary.Scope),
		State:       recordings.RecordingLifecycleState(summary.State),
		EventCount:  summary.EventCount,
		FirstCursor: firstCursor,
		LastCursor:  lastCursor,
		Failures:    fromArtifactFailures(summary.Failures),
		Available:   summary.Available,
	}
}

func toArtifactEnvelope(artifact recordings.PortableArtifact) recordings.ArtifactEnvelope {
	return recordings.ArtifactEnvelope{
		SchemaVersion: recordings.ArtifactSchemaVersion(artifact.SchemaVersion),
		Summary:       toArtifactSummary(artifact.Summary),
		Events:        toReplayEvents(artifact.Events),
		Integrity: recordings.ArtifactIntegrity{
			Algorithm: artifact.Integrity.Algorithm,
			Digest:    artifact.Integrity.Digest,
		},
	}
}

func fromArtifactEnvelope(envelope recordings.ArtifactEnvelope) recordings.PortableArtifact {
	return recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaVersion(envelope.SchemaVersion),
		Summary:       fromArtifactSummary(envelope.Summary),
		Events:        fromReplayEvents(envelope.Events),
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: envelope.Integrity.Algorithm,
			Digest:    envelope.Integrity.Digest,
		},
	}
}

func translateReplayArtifactError(err error) error {
	if err == nil {
		return nil
	}
	kind := recordings.ReplayArtifactErrorInvalid
	switch {
	case errors.Is(err, recordings.ErrReplayRecordingNotFound):
		kind = recordings.ReplayArtifactErrorNotFound
	case errors.Is(err, recordings.ErrReplayRecordingNotFinalized):
		kind = recordings.ReplayArtifactErrorNotFinalized
	case errors.Is(err, recordings.ErrCorruptReplayInput):
		kind = recordings.ReplayArtifactErrorCorruptInput
	case errors.Is(err, recordings.ErrPortableArtifactUnavailable):
		kind = recordings.ReplayArtifactErrorUnavailable
	case errors.Is(err, recordings.ErrUnsupportedPortableArtifactSchema):
		kind = recordings.ReplayArtifactErrorUnsupportedSchema
	case errors.Is(err, recordings.ErrInvalidPortableArtifactIntegrity):
		kind = recordings.ReplayArtifactErrorInvalidIntegrity
	case errors.Is(err, recordings.ErrInvalidPortableArtifactOrder):
		kind = recordings.ReplayArtifactErrorInvalidOrder
	case errors.Is(err, recordings.ErrPortableArtifactExportFailed):
		kind = recordings.ReplayArtifactErrorExportFailed
	case errors.Is(err, recordings.ErrForeignPortableArtifact):
		kind = recordings.ReplayArtifactErrorForeign
	case errors.Is(err, recordings.ErrPortableArtifactCancelled):
		kind = recordings.ReplayArtifactErrorCancelled
	case errors.Is(err, recordings.ErrInvalidPortableArtifact):
		kind = recordings.ReplayArtifactErrorInvalid
	}
	return &recordings.ReplayArtifactError{
		Kind:       kind,
		Diagnostic: replayArtifactDiagnostic(kind),
		Cause:      err,
	}
}

func replayArtifactDiagnostic(kind recordings.ReplayArtifactErrorKind) recordings.ReplayArtifactDiagnostic {
	switch kind {
	case recordings.ReplayArtifactErrorNotFound:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticRecordingNotFound,
			Area:    "recording",
			Path:    "recordingId",
			Message: "recording was not found",
		}
	case recordings.ReplayArtifactErrorNotFinalized:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticRecordingNotFinalized,
			Area:    "recording",
			Path:    "recordingId",
			Message: "recording is not finalized",
		}
	case recordings.ReplayArtifactErrorCorruptInput:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticInvalidSummary,
			Area:    "replay",
			Path:    "recording",
			Message: "replay recording is invalid",
		}
	case recordings.ReplayArtifactErrorUnavailable:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticMissingReference,
			Area:    "artifact",
			Path:    "reference",
			Message: "published replay artifact is unavailable",
		}
	case recordings.ReplayArtifactErrorUnsupportedSchema:
		return recordings.ReplayArtifactDiagnostic{
			Code:              recordings.ReplayArtifactDiagnosticUnsupportedVersion,
			Area:              "compatibility",
			Path:              "schemaVersion",
			Message:           "artifact uses an unsupported schema version",
			SupportedVersions: []string{string(recordings.ArtifactSchemaV1)},
		}
	case recordings.ReplayArtifactErrorInvalidIntegrity:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticInvalidIntegrity,
			Area:    "integrity",
			Path:    "integrity.digest",
			Message: "artifact integrity is invalid",
		}
	case recordings.ReplayArtifactErrorInvalidOrder:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticInvalidOrder,
			Area:    "events",
			Path:    "events",
			Message: "artifact event order is invalid",
		}
	case recordings.ReplayArtifactErrorForeign:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticForeignReference,
			Area:    "artifact",
			Path:    "reference",
			Message: "artifact reference does not belong to the selected recording",
		}
	case recordings.ReplayArtifactErrorCancelled:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticCancelled,
			Area:    "operation",
			Path:    "context",
			Message: "replay artifact operation was canceled",
		}
	case recordings.ReplayArtifactErrorExportFailed:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticDependencyFailure,
			Area:    "publication",
			Path:    "artifact",
			Message: "replay artifact could not be published",
		}
	case recordings.ReplayArtifactErrorUnsupportedContext:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticUnsupportedContext,
			Area:    "capability",
			Path:    "operation",
			Message: "operation is unavailable in this Recordings capability context",
		}
	default:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticMalformed,
			Area:    "artifact",
			Path:    "artifact",
			Message: "artifact is malformed or invalid",
		}
	}
}

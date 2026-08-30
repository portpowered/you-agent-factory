package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
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
	return recordings.DecodeArtifactResult{
		Artifact:         toArtifactEnvelope(result.Artifact),
		IgnoredJSONPaths: append([]string(nil), result.IgnoredJSONPaths...),
	}, nil
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
	return recordings.ReadArtifactResult{
		Artifact:         toArtifactEnvelope(result.Artifact),
		IgnoredJSONPaths: append([]string(nil), result.IgnoredJSONPaths...),
	}, nil
}

// replayInputLoader is the path-based Recordings implementation Factory
// Sessions receives before a recording ledger exists. It owns the
// portable-versus-legacy classification and its safe operation observability.
// Its public ReplayInputLoader contract contains only that lifecycle-safe
// operation.
type replayInputLoader struct {
	readFile           recordings.RecordingReadFile
	openFile           recordings.RecordingOpenFile
	loadLegacy         recordings.ReplayArtifactLoader
	loadLegacyMetadata recordings.ReplayArtifactMetadataLoader
	logger             logging.Logger
}

var _ recordings.ReplayInputLoader = (*replayInputLoader)(nil)

// NewReplayInputLoader constructs the path-based replay-input capability from
// the exact reader, legacy loader, and process logger selected by Recordings
// Wire. It is inert: it performs no I/O until LoadReplayInput.
func NewReplayInputLoader(
	readFile recordings.RecordingReadFile,
	openFile recordings.RecordingOpenFile,
	loadLegacy recordings.ReplayArtifactLoader,
	loadLegacyMetadata recordings.ReplayArtifactMetadataLoader,
	logger logging.Logger,
) recordings.ReplayInputLoader {
	return &replayInputLoader{
		readFile:           readFile,
		openFile:           openFile,
		loadLegacy:         loadLegacy,
		loadLegacyMetadata: loadLegacyMetadata,
		logger:             logging.EnsureLogger(logger),
	}
}

func (loader *replayInputLoader) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if request.MetadataOnly {
		return loader.loadReplayInputMetadata(request.Path)
	}
	loader.logReplayInputIntent(false)
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
	return loader.loadLegacyReplayInput(request.Path, data)
}

func isPortableReplayInput(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return false
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		keyText, ok := key.(string)
		if !ok {
			return false
		}
		if keyText == "recordingKind" {
			var kind string
			if err := decoder.Decode(&kind); err != nil {
				return false
			}
			return kind == recordings.KindJavaScriptFactorySession
		}
		var ignored json.RawMessage
		if err := decoder.Decode(&ignored); err != nil {
			return false
		}
	}
	return false
}

func (loader *replayInputLoader) loadPortableReplayInput(
	data []byte,
) (recordings.LoadReplayInputResult, error) {
	value, diagnostics, err := recordings.DecodePortableRecordingWithDiagnostics(bytes.NewReader(data))
	if err != nil {
		failure := newPortableReplayInputError(err)
		loader.logReplayInputOutcome("validation_failure", string(failure.Diagnostic.Code), "")
		return recordings.LoadReplayInputResult{}, failure
	}
	loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyPortable))
	return recordings.LoadReplayInputResult{
		Portable: &value,
		Diagnostics: &recordings.ReplayInputDecodeDiagnostics{
			IgnoredJSONPaths: diagnostics.Paths(),
		},
	}, nil
}

func (loader *replayInputLoader) loadLegacyReplayInput(
	path string,
	data []byte,
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
	metadata, metadataErr := replayV2InputMetadata(data)
	if metadataErr != nil {
		failure := newReplayInputError(
			recordings.ReplayInputFamilyLegacy,
			fmt.Errorf("parse replay artifact framing: %w", metadataErr),
		)
		loader.logReplayInputOutcome("validation_failure", string(failure.Diagnostic.Code), string(recordings.ReplayInputFamilyLegacy))
		return recordings.LoadReplayInputResult{}, failure
	}
	loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyLegacy))
	return recordings.LoadReplayInputResult{Legacy: artifact, Metadata: metadata}, nil
}

// replayV2InputMetadata preserves framing facts that are intentionally absent
// from the normalized legacy artifact. The full-input path already has the
// durable bytes, so it can retain the terminal marker without widening the
// replay artifact model or making metadata-only reads materialize history.
func replayV2InputMetadata(data []byte) (*recordings.ReplayInputMetadata, error) {
	if !replayimpl.IsReplayV2Artifact(data) {
		return nil, nil
	}
	stream, err := replayimpl.ParseReplayV2(data)
	if err != nil {
		return nil, err
	}
	return &recordings.ReplayInputMetadata{
		FactorySessionID: strings.TrimSpace(stream.Header.SessionID),
		Completed:        stream.Terminal != nil,
	}, nil
}

func (loader *replayInputLoader) loadReplayInputMetadata(
	path string,
) (recordings.LoadReplayInputResult, error) {
	loader.logReplayInputIntent(true)
	if loader.openFile == nil {
		return loader.replayInputDependencyFailure(
			"metadata_reader_unavailable",
			fmt.Errorf("Factory Session replay recording streaming reader is required"),
		)
	}
	if loader.loadLegacyMetadata == nil {
		return loader.replayInputDependencyFailure(
			"legacy_metadata_loader_unavailable",
			fmt.Errorf("replay artifact metadata loader is required"),
		)
	}
	isPortable, err := loader.classifyPortableReplayInputMetadata(path)
	if err != nil {
		return loader.replayInputDependencyFailure("metadata_classification_failure", err)
	}
	if isPortable {
		file, err := loader.openFile(path)
		if err != nil {
			return loader.replayInputDependencyFailure("metadata_open_failure", fmt.Errorf("open replay recording metadata: %w", err))
		}
		id, decodeErr := recordings.DecodePortableRecordingMetadata(file)
		closeErr := file.Close()
		if decodeErr != nil {
			failure := newPortableReplayInputError(decodeErr)
			loader.logReplayInputOutcome("validation_failure", string(failure.Diagnostic.Code), string(recordings.ReplayInputFamilyPortable), true)
			return recordings.LoadReplayInputResult{}, failure
		}
		if closeErr != nil {
			return loader.replayInputDependencyFailure("metadata_close_failure", fmt.Errorf("close replay recording metadata: %w", closeErr))
		}
		loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyPortable), true)
		return recordings.LoadReplayInputResult{
			Metadata: &recordings.ReplayInputMetadata{FactorySessionID: id},
		}, nil
	}
	metadata, err := loader.loadLegacyMetadata(path)
	if err != nil {
		failure := newReplayInputError(recordings.ReplayInputFamilyLegacy, fmt.Errorf("load replay artifact metadata: %w", err))
		loader.logReplayInputOutcome("dependency_failure", string(failure.Diagnostic.Code), string(recordings.ReplayInputFamilyLegacy), true)
		return recordings.LoadReplayInputResult{}, failure
	}
	loader.logReplayInputOutcome("success", "", string(recordings.ReplayInputFamilyLegacy), true)
	return recordings.LoadReplayInputResult{Metadata: &metadata}, nil
}

func (loader *replayInputLoader) classifyPortableReplayInputMetadata(path string) (bool, error) {
	file, err := loader.openFile(path)
	if err != nil {
		return false, fmt.Errorf("open replay recording for classification: %w", err)
	}
	defer file.Close()
	return isPortableReplayInputReader(file), nil
}

func isPortableReplayInputReader(reader io.Reader) bool {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return false
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		keyText, ok := key.(string)
		if !ok {
			return false
		}
		if keyText == "recordingKind" {
			var kind string
			if err := decoder.Decode(&kind); err != nil {
				return false
			}
			return kind == recordings.KindJavaScriptFactorySession
		}
		if err := skipReplayInputJSONValue(decoder); err != nil {
			return false
		}
	}
	return false
}

func skipReplayInputJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := skipReplayInputJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := skipReplayInputJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	_, err = decoder.Token()
	return err
}

func (loader *replayInputLoader) replayInputDependencyFailure(
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

func (loader *replayInputLoader) logReplayInputIntent(metadataOnly bool) {
	loader.logger.Info(
		"recordings replay input accepted",
		"operation", "load_replay_input",
		"input_source", "filesystem_path",
		"metadata_only", metadataOnly,
	)
}

func (loader *replayInputLoader) logReplayInputOutcome(
	outcome string,
	classification string,
	family string,
	metadataOnly ...bool,
) {
	fields := []any{"operation", "load_replay_input", "outcome", outcome}
	if classification != "" {
		fields = append(fields, "error_class", classification)
	}
	if family != "" {
		fields = append(fields, "replay_family", family)
	}
	if len(metadataOnly) > 0 {
		fields = append(fields, "metadata_only", metadataOnly[0])
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

func newPortableReplayInputError(cause error) *recordings.ReplayInputError {
	diagnostic := replayInputDiagnostic(cause)
	if diagnostic.Code == recordings.ReplayArtifactDiagnosticDependencyFailure {
		diagnostic = recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticMalformed,
			Area:    "recording",
			Path:    "recording",
			Message: "recording document is malformed",
		}
	}
	return &recordings.ReplayInputError{
		Family:     recordings.ReplayInputFamilyPortable,
		Diagnostic: diagnostic,
		Cause:      cause,
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
	var replayArtifactErr *recordings.ReplayArtifactError
	if errors.As(err, &replayArtifactErr) {
		return cloneReplayArtifactDiagnostic(replayArtifactErr.Diagnostic)
	}
	var diagnostic *recordings.PortableRecordingDiagnostic
	if errors.As(err, &diagnostic) {
		return replayArtifactDiagnosticFromPortable(diagnostic)
	}
	return replayInputDependencyDiagnostic()
}

func cloneReplayArtifactDiagnostic(
	diagnostic recordings.ReplayArtifactDiagnostic,
) recordings.ReplayArtifactDiagnostic {
	diagnostic.SupportedVersions = slices.Clone(diagnostic.SupportedVersions)
	return diagnostic
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
		Area:               diagnostic.Area,
		Path:               safeReplayArtifactDiagnosticPath(diagnostic.Path),
		EncounteredVersion: diagnostic.EncounteredVersion,
		Action:             diagnostic.Action,
	}
	switch diagnostic.Code {
	case recordings.PortableRecordingCodeMalformedContract:
		result.Code = recordings.ReplayArtifactDiagnosticMalformed
		result.Message = "recording document is malformed"
	case recordings.PortableRecordingCodeUnsupportedVersion:
		result.Code = recordings.ReplayArtifactDiagnosticUnsupportedVersion
		result.Message = "recording uses an unsupported replay compatibility version"
		result.SupportedVersions = slices.Clone(diagnostic.SupportedVersions)
	case recordings.PortableRecordingCodeUnsupportedSchema:
		result.Code = recordings.ReplayArtifactDiagnosticUnsupportedSchema
		result.Message = "recording uses an unsupported schema version"
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
	case recordings.PortableRecordingCodeInvalidOrder:
		result.Code = recordings.ReplayArtifactDiagnosticInvalidOrder
		result.Message = "recording event order is invalid"
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
	default:
		return recordings.ReplayArtifactDiagnostic{
			Code:    recordings.ReplayArtifactDiagnosticMalformed,
			Area:    "artifact",
			Path:    "artifact",
			Message: "artifact is malformed or invalid",
		}
	}
}

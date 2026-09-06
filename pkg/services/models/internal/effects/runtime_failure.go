package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// RuntimeStage identifies the private boundary at which a managed model
// invocation stopped. These values are intentionally narrower than the
// public Models failure vocabulary and never contain paths, endpoints, or
// provider-native text.
type RuntimeStage string

const (
	RuntimeStageArtifactResolve  RuntimeStage = "ARTIFACT_RESOLVE"
	RuntimeStageArtifactDownload RuntimeStage = "ARTIFACT_DOWNLOAD"
	RuntimeStageArtifactDigest   RuntimeStage = "ARTIFACT_DIGEST"
	RuntimeStageBackendExtract   RuntimeStage = "BACKEND_EXTRACT"
	RuntimeStageBackendStart     RuntimeStage = "BACKEND_START"
	RuntimeStageProtocolLoad     RuntimeStage = "PROTOCOL_LOAD"
	RuntimeStageInvoke           RuntimeStage = "INVOKE"
)

// RuntimeFailureClass is a bounded private cause category. It is suitable for
// logs and evidence, while the wrapped cause remains available to local code
// through errors.Is/errors.As.
type RuntimeFailureClass string

const (
	RuntimeFailureUnavailable          RuntimeFailureClass = "UNAVAILABLE"
	RuntimeFailureInvalidArtifact      RuntimeFailureClass = "INVALID_ARTIFACT"
	RuntimeFailureIntegrityMismatch    RuntimeFailureClass = "INTEGRITY_MISMATCH"
	RuntimeFailureExtractionFailed     RuntimeFailureClass = "EXTRACTION_FAILED"
	RuntimeFailureProcessStartFailed   RuntimeFailureClass = "PROCESS_START_FAILED"
	RuntimeFailureProcessExited        RuntimeFailureClass = "PROCESS_EXITED"
	RuntimeFailureProtocolIncompatible RuntimeFailureClass = "PROTOCOL_INCOMPATIBLE"
	RuntimeFailureInvocationFailed     RuntimeFailureClass = "INVOCATION_FAILED"
	RuntimeFailureMalformedResponse    RuntimeFailureClass = "MALFORMED_RESPONSE"
	RuntimeFailureCancelled            RuntimeFailureClass = "CANCELLED"
	RuntimeFailureTimedOut             RuntimeFailureClass = "TIMED_OUT"
)

// RuntimeStageError carries a bounded stage and class around an existing
// typed error. Error deliberately excludes Cause.Error() so a caller cannot
// accidentally publish a token, URL, endpoint, path, prompt, or media body.
type RuntimeStageError struct {
	Stage RuntimeStage
	Class RuntimeFailureClass
	Cause error
}

func (failure *RuntimeStageError) Error() string {
	if failure == nil {
		return ""
	}
	return fmt.Sprintf("model runtime stage failed: %s (%s)",
		safeRuntimeStage(failure.Stage), safeRuntimeFailureClass(failure.Class))
}

func (failure *RuntimeStageError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func (failure *RuntimeStageError) ModelRuntimeStage() string {
	if failure == nil {
		return ""
	}
	return string(failure.Stage)
}

func (failure *RuntimeStageError) ModelRuntimeFailureClass() string {
	if failure == nil {
		return ""
	}
	return string(failure.Class)
}

// RuntimeFailureClassifier is implemented by private runtime errors across
// the Models and composition packages. The string boundary avoids an import
// cycle between the private Models effects package and the process launcher.
type RuntimeFailureClassifier interface {
	ModelRuntimeStage() string
	ModelRuntimeFailureClass() string
}

// NewRuntimeStageError constructs one safe private wrapper. A nil cause is
// retained as a bounded failure, which is useful for controlled stage tests.
func NewRuntimeStageError(
	stage RuntimeStage,
	class RuntimeFailureClass,
	cause error,
) *RuntimeStageError {
	return &RuntimeStageError{
		Stage: normalizeRuntimeStage(stage),
		Class: normalizeRuntimeFailureClass(class),
		Cause: cause,
	}
}

// WrapRuntimeFailure adds a stage/class only when the error chain does not
// already carry one. This preserves a more precise lower-level classification
// such as BACKEND_EXTRACT or PROTOCOL_LOAD while keeping errors.Is/errors.As
// traversal intact.
func WrapRuntimeFailure(stage RuntimeStage, err error) error {
	if err == nil {
		return nil
	}
	if _, _, ok := ClassifyRuntimeFailure(err); ok {
		return err
	}
	class := RuntimeFailureClassForError(err)
	if stage == RuntimeStageInvoke && class == RuntimeFailureUnavailable {
		class = RuntimeFailureInvocationFailed
	}
	return NewRuntimeStageError(stage, class, err)
}

// ClassifyRuntimeFailure returns exactly one valid bounded classification from
// an error chain. Unknown implementations and malformed values fail closed.
func ClassifyRuntimeFailure(err error) (RuntimeStage, RuntimeFailureClass, bool) {
	if err == nil {
		return "", "", false
	}
	var classifier RuntimeFailureClassifier
	if !errors.As(err, &classifier) || classifier == nil {
		return "", "", false
	}
	stage := RuntimeStage(classifier.ModelRuntimeStage())
	class := RuntimeFailureClass(classifier.ModelRuntimeFailureClass())
	if !isRuntimeStage(stage) || !isRuntimeFailureClass(class) {
		return "", "", false
	}
	return stage, class, true
}

// RuntimeFailureClassForError maps known provider-neutral errors to the
// bounded private class used when a lower layer has not supplied one.
func RuntimeFailureClassForError(err error) RuntimeFailureClass {
	if err == nil {
		return RuntimeFailureUnavailable
	}
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, models.ErrInferenceCancelled),
		errors.Is(err, models.ErrAssetCancelled),
		errors.Is(err, models.ErrHostCancelled):
		return RuntimeFailureCancelled
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, models.ErrInferenceTimeout),
		errors.Is(err, models.ErrHostLoadingTimeout):
		return RuntimeFailureTimedOut
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return RuntimeFailureIntegrityMismatch
	case errors.Is(err, models.ErrInferenceArtifactInvalid):
		return RuntimeFailureMalformedResponse
	case errors.Is(err, models.ErrHostProtocolIncompatible):
		return RuntimeFailureProtocolIncompatible
	case errors.Is(err, models.ErrHostProcessCrash):
		return RuntimeFailureProcessExited
	case errors.Is(err, models.ErrInferenceFailed):
		return RuntimeFailureInvocationFailed
	}

	var invocationFailure *models.InvocationFailure
	if errors.As(err, &invocationFailure) && invocationFailure != nil {
		switch invocationFailure.Class {
		case models.InvocationFailureClassMalformedResponse:
			return RuntimeFailureMalformedResponse
		case models.InvocationFailureClassBackendProtocol:
			return RuntimeFailureProtocolIncompatible
		case models.InvocationFailureClassCancellation:
			return RuntimeFailureCancelled
		case models.InvocationFailureClassTimeout:
			return RuntimeFailureTimedOut
		case models.InvocationFailureClassArtifact:
			return RuntimeFailureInvalidArtifact
		case models.InvocationFailureClassBackendReadiness:
			return RuntimeFailureUnavailable
		}
	}
	return RuntimeFailureUnavailable
}

// RuntimeFailureDiagnostic is the safe structured projection used by private
// logs and evidence. It contains no raw cause or caller payload.
type RuntimeFailureDiagnostic struct {
	Message        string              `json:"message"`
	Stage          RuntimeStage        `json:"runtime_stage"`
	Class          RuntimeFailureClass `json:"failure_class"`
	Outcome        string              `json:"outcome"`
	DurationMillis int64               `json:"duration_millis"`
	CauseSHA256    string              `json:"cause_sha256,omitempty"`
}

const (
	// RuntimeEvidenceKindStage identifies an observation of one bounded
	// runtime stage. A failed stage is followed by one terminal record by the
	// Models invocation owner.
	RuntimeEvidenceKindStage = "STAGE"
	// RuntimeEvidenceKindTerminal identifies the one terminal decision for a
	// diagnostic invocation.
	RuntimeEvidenceKindTerminal     = "TERMINAL"
	RuntimeEvidenceOutcomeCompleted = "COMPLETED"
	RuntimeEvidenceOutcomeFailed    = "FAILED"
)

// RuntimeEvidenceRecord is the private, ordered representation shared by the
// Models runtime and its integration witness. It deliberately contains only
// bounded enums, elapsed time, and a cause digest; callers never publish a
// raw error, endpoint, path, prompt, token, or media payload through it.
type RuntimeEvidenceRecord struct {
	Sequence       uint64              `json:"sequence"`
	Kind           string              `json:"kind"`
	Stage          RuntimeStage        `json:"stage,omitempty"`
	Outcome        string              `json:"outcome"`
	Class          RuntimeFailureClass `json:"failure_class,omitempty"`
	DurationMillis int64               `json:"duration_millis"`
	CauseSHA256    string              `json:"cause_sha256,omitempty"`
}

// RuntimeEvidenceRecorder accepts one private runtime observation. The
// recorder is optional: a nil recorder preserves normal product behavior.
type RuntimeEvidenceRecorder interface {
	RecordRuntimeEvidence(RuntimeEvidenceRecord)
}

// orderedRuntimeEvidenceRecorder assigns a process-local order to otherwise
// detached records before forwarding them to the owner sink. Sequence is
// deliberately assigned at this boundary so concurrent runtime components do
// not race on evidence ordering.
type orderedRuntimeEvidenceRecorder struct {
	mu               sync.Mutex
	next             uint64
	terminalRecorded bool
	recorder         RuntimeEvidenceRecorder
}

// NewOrderedRuntimeEvidenceRecorder wraps an optional sink with validation and
// sequence assignment. Passing nil returns nil so normal runtime construction
// remains unchanged when integration evidence is not requested.
func NewOrderedRuntimeEvidenceRecorder(
	recorder RuntimeEvidenceRecorder,
) RuntimeEvidenceRecorder {
	if isNilRuntimeEvidenceRecorder(recorder) {
		return nil
	}
	if ordered, ok := recorder.(*orderedRuntimeEvidenceRecorder); ok {
		return ordered
	}
	return &orderedRuntimeEvidenceRecorder{recorder: recorder}
}

func (recorder *orderedRuntimeEvidenceRecorder) RecordRuntimeEvidence(
	record RuntimeEvidenceRecord,
) {
	if recorder == nil || isNilRuntimeEvidenceRecorder(recorder.recorder) {
		return
	}
	normalized, ok := normalizeRuntimeEvidenceRecord(record)
	if !ok {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if normalized.Kind == RuntimeEvidenceKindTerminal {
		if recorder.terminalRecorded {
			return
		}
		recorder.terminalRecorded = true
	}
	recorder.next++
	normalized.Sequence = recorder.next
	recorder.recorder.RecordRuntimeEvidence(normalized)
}

// RecordRuntimeEvidenceStage emits one bounded stage observation. For a
// failure, an existing lower-level classification is retained through the
// normal error chain; otherwise the supplied stage is used as the fallback.
func RecordRuntimeEvidenceStage(
	recorder RuntimeEvidenceRecorder,
	stage RuntimeStage,
	err error,
	elapsed time.Duration,
) {
	if isNilRuntimeEvidenceRecorder(recorder) {
		return
	}
	if err == nil {
		recorder.RecordRuntimeEvidence(RuntimeEvidenceRecord{
			Kind:           RuntimeEvidenceKindStage,
			Stage:          normalizeRuntimeStage(stage),
			Outcome:        RuntimeEvidenceOutcomeCompleted,
			DurationMillis: durationMillis(elapsed),
		})
		return
	}
	diagnostic := ProjectRuntimeFailure(WrapRuntimeFailure(stage, err), elapsed)
	recorder.RecordRuntimeEvidence(runtimeEvidenceRecordFromDiagnostic(
		RuntimeEvidenceKindStage,
		diagnostic,
	))
}

// RecordRuntimeEvidenceTerminal emits the single bounded terminal decision
// owned by one Models invocation. The invocation owner calls this exactly once
// after it has classified the final result.
func RecordRuntimeEvidenceTerminal(
	recorder RuntimeEvidenceRecorder,
	stage RuntimeStage,
	err error,
	elapsed time.Duration,
) {
	if isNilRuntimeEvidenceRecorder(recorder) {
		return
	}
	if err == nil {
		recorder.RecordRuntimeEvidence(RuntimeEvidenceRecord{
			Kind:           RuntimeEvidenceKindTerminal,
			Stage:          normalizeRuntimeStage(stage),
			Outcome:        RuntimeEvidenceOutcomeCompleted,
			DurationMillis: durationMillis(elapsed),
		})
		return
	}
	diagnostic := ProjectRuntimeFailure(WrapRuntimeFailure(stage, err), elapsed)
	recorder.RecordRuntimeEvidence(runtimeEvidenceRecordFromDiagnostic(
		RuntimeEvidenceKindTerminal,
		diagnostic,
	))
}

func runtimeEvidenceRecordFromDiagnostic(
	kind string,
	diagnostic RuntimeFailureDiagnostic,
) RuntimeEvidenceRecord {
	return RuntimeEvidenceRecord{
		Kind:           kind,
		Stage:          diagnostic.Stage,
		Outcome:        diagnostic.Outcome,
		Class:          diagnostic.Class,
		DurationMillis: diagnostic.DurationMillis,
		CauseSHA256:    diagnostic.CauseSHA256,
	}
}

func normalizeRuntimeEvidenceRecord(
	record RuntimeEvidenceRecord,
) (RuntimeEvidenceRecord, bool) {
	if record.Kind != RuntimeEvidenceKindStage && record.Kind != RuntimeEvidenceKindTerminal {
		return RuntimeEvidenceRecord{}, false
	}
	if !isRuntimeStage(record.Stage) {
		return RuntimeEvidenceRecord{}, false
	}
	if record.Outcome != RuntimeEvidenceOutcomeCompleted && record.Outcome != RuntimeEvidenceOutcomeFailed {
		return RuntimeEvidenceRecord{}, false
	}
	if record.DurationMillis < 0 {
		record.DurationMillis = 0
	}
	if record.Outcome == RuntimeEvidenceOutcomeCompleted {
		record.Class = ""
		record.CauseSHA256 = ""
		return record, true
	}
	if !isRuntimeFailureClass(record.Class) || !validRuntimeCauseSHA256(record.CauseSHA256) {
		return RuntimeEvidenceRecord{}, false
	}
	record.CauseSHA256 = strings.ToLower(record.CauseSHA256)
	return record, true
}

func validRuntimeCauseSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func isNilRuntimeEvidenceRecorder(recorder RuntimeEvidenceRecorder) bool {
	if recorder == nil {
		return true
	}
	value := reflect.ValueOf(recorder)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// ProjectRuntimeFailure creates a bounded projection. An unclassified error
// is represented as ARTIFACT_RESOLVE/UNAVAILABLE rather than copying any
// implementation text into the diagnostic.
func ProjectRuntimeFailure(err error, elapsed time.Duration) RuntimeFailureDiagnostic {
	diagnostic := RuntimeFailureDiagnostic{
		Message:        "model runtime completed",
		Outcome:        "COMPLETED",
		DurationMillis: durationMillis(elapsed),
	}
	if err == nil {
		return diagnostic
	}
	stage, class, ok := ClassifyRuntimeFailure(err)
	if !ok {
		stage = RuntimeStageArtifactResolve
		class = RuntimeFailureClassForError(err)
	}
	diagnostic.Message = "model runtime stage failed"
	diagnostic.Stage = stage
	diagnostic.Class = class
	diagnostic.Outcome = "FAILED"
	diagnostic.CauseSHA256 = RuntimeCauseSHA256(err)
	return diagnostic
}

// RuntimeCauseSHA256 returns a stable digest of the private cause without
// returning the cause text. The digest is diagnostic evidence, not a secret
// redaction substitute.
func RuntimeCauseSHA256(err error) string {
	cause := runtimeDiagnosticCause(err)
	if cause == nil {
		return ""
	}
	digest := sha256.Sum256([]byte(cause.Error()))
	return hex.EncodeToString(digest[:])
}

// DiagnosticFields returns only bounded string fields suitable for the
// repository's injected diagnostic logger.
func (diagnostic RuntimeFailureDiagnostic) DiagnosticFields() map[string]string {
	fields := map[string]string{
		"message":         diagnostic.Message,
		"runtime_stage":   string(diagnostic.Stage),
		"failure_class":   string(diagnostic.Class),
		"outcome":         diagnostic.Outcome,
		"duration_millis": fmt.Sprintf("%d", diagnostic.DurationMillis),
	}
	if diagnostic.CauseSHA256 != "" {
		fields["cause_sha256"] = diagnostic.CauseSHA256
	}
	return fields
}

func runtimeDiagnosticCause(err error) error {
	current := err
	for depth := 0; current != nil && depth < 32; depth++ {
		switch unwrapped := current.(type) {
		case interface{ Unwrap() error }:
			next := unwrapped.Unwrap()
			if next == nil {
				return current
			}
			current = next
		case interface{ Unwrap() []error }:
			causes := unwrapped.Unwrap()
			if len(causes) == 0 || causes[0] == nil {
				return current
			}
			current = causes[0]
		default:
			return current
		}
	}
	return current
}

func durationMillis(elapsed time.Duration) int64 {
	if elapsed <= 0 {
		return 0
	}
	return elapsed.Milliseconds()
}

func isRuntimeStage(stage RuntimeStage) bool {
	switch stage {
	case RuntimeStageArtifactResolve,
		RuntimeStageArtifactDownload,
		RuntimeStageArtifactDigest,
		RuntimeStageBackendExtract,
		RuntimeStageBackendStart,
		RuntimeStageProtocolLoad,
		RuntimeStageInvoke:
		return true
	default:
		return false
	}
}

func isRuntimeFailureClass(class RuntimeFailureClass) bool {
	switch class {
	case RuntimeFailureUnavailable,
		RuntimeFailureInvalidArtifact,
		RuntimeFailureIntegrityMismatch,
		RuntimeFailureExtractionFailed,
		RuntimeFailureProcessStartFailed,
		RuntimeFailureProcessExited,
		RuntimeFailureProtocolIncompatible,
		RuntimeFailureInvocationFailed,
		RuntimeFailureMalformedResponse,
		RuntimeFailureCancelled,
		RuntimeFailureTimedOut:
		return true
	default:
		return false
	}
}

func normalizeRuntimeStage(stage RuntimeStage) RuntimeStage {
	if isRuntimeStage(stage) {
		return stage
	}
	return RuntimeStageArtifactResolve
}

func normalizeRuntimeFailureClass(class RuntimeFailureClass) RuntimeFailureClass {
	if isRuntimeFailureClass(class) {
		return class
	}
	return RuntimeFailureUnavailable
}

func safeRuntimeStage(stage RuntimeStage) RuntimeStage {
	return normalizeRuntimeStage(stage)
}

func safeRuntimeFailureClass(class RuntimeFailureClass) RuntimeFailureClass {
	return normalizeRuntimeFailureClass(class)
}

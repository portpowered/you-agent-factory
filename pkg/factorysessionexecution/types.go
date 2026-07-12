package factorysessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// DurableRecordKind identifies one record in the persisted Factory Session
// history without erasing orchestrator-specific replay semantics.
type DurableRecordKind string

const (
	DurableRecordKindCanonicalFactoryEvent DurableRecordKind = "canonical_factory_event"
	DurableRecordKindJavaScriptRuntime     DurableRecordKind = "javascript_runtime"
	DurableRecordKindPetriTokenMutation    DurableRecordKind = "petri_token_mutation"
)

// DurableSessionRecord is the tagged persistence union for canonical events
// and explicitly orchestration-owned records. Exactly one payload must match Kind.
type DurableSessionRecord struct {
	Kind             DurableRecordKind               `json:"kind"`
	CanonicalEvent   json.RawMessage                 `json:"canonicalEvent,omitempty"`
	JavaScriptRecord *workflowruntime.RuntimeRecord  `json:"javascriptRecord,omitempty"`
	PetriMutation    *interfaces.TokenMutationRecord `json:"petriMutation,omitempty"`
}

// UnmarshalJSON rejects unknown and mismatched records so older runtimes never
// silently discard newly introduced replay facts.
func (r *DurableSessionRecord) UnmarshalJSON(data []byte) error {
	type durableSessionRecordJSON DurableSessionRecord
	var decoded durableSessionRecordJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("decode durable session record: %w", err)
	}
	record := DurableSessionRecord(decoded)
	if err := validateDurableSessionRecord(record); err != nil {
		return err
	}
	*r = record
	return nil
}

func validateDurableSessionRecord(record DurableSessionRecord) error {
	payloads := map[DurableRecordKind]bool{
		DurableRecordKindCanonicalFactoryEvent: len(record.CanonicalEvent) > 0,
		DurableRecordKindJavaScriptRuntime:     record.JavaScriptRecord != nil,
		DurableRecordKindPetriTokenMutation:    record.PetriMutation != nil,
	}
	present, known := payloads[record.Kind]
	if !known {
		return fmt.Errorf("decode durable session record: unsupported kind %q", record.Kind)
	}
	if !present {
		return fmt.Errorf("decode durable session record kind %q: matching payload is required", record.Kind)
	}
	for kind, hasPayload := range payloads {
		if hasPayload && kind != record.Kind {
			return fmt.Errorf("decode durable session record kind %q: unexpected %s payload", record.Kind, kind)
		}
	}
	if record.Kind == DurableRecordKindCanonicalFactoryEvent {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(record.CanonicalEvent, &envelope); err != nil {
			return fmt.Errorf("decode durable canonical Factory Event: %w", err)
		}
		if strings.TrimSpace(envelope.Type) == "" {
			return errors.New("decode durable canonical Factory Event: type is required")
		}
	}
	return nil
}

func durableRecordsFromRuntimeState(state runtimeSessionState) []DurableSessionRecord {
	records := make([]DurableSessionRecord, 0, len(state.events)+len(state.runtimeRecords))
	for _, event := range state.events {
		records = append(records, DurableSessionRecord{
			Kind: DurableRecordKindCanonicalFactoryEvent, CanonicalEvent: append(json.RawMessage(nil), event...),
		})
	}
	for _, runtimeRecord := range state.runtimeRecords {
		cloned := cloneRuntimeRecord(runtimeRecord)
		records = append(records, DurableSessionRecord{
			Kind: DurableRecordKindJavaScriptRuntime, JavaScriptRecord: &cloned,
		})
	}
	return records
}

// SyncOutcome reports how a sync start wait ended.
type SyncOutcome string

const (
	SyncOutcomeCompleted    SyncOutcome = "COMPLETED"
	SyncOutcomeTimedOut     SyncOutcome = "TIMED_OUT"
	SyncOutcomeStillRunning SyncOutcome = "STILL_RUNNING"
)

// InlineWorkflowSource carries inline workflow source from a durable execution request.
type InlineWorkflowSource struct {
	Dialect      string
	InlineSource string
	Entrypoint   string
	Metadata     map[string]string
}

// Source is the normalized durable execution source selector.
type Source struct {
	Kind           workflowsource.Kind
	FactoryID      string
	FactoryInline  json.RawMessage
	WorkflowFile   string
	WorkflowName   string
	InlineWorkflow *InlineWorkflowSource
}

// OrchestratorOverride is an optional orchestrator override on a start request.
type OrchestratorOverride struct {
	Kind string
	Raw  json.RawMessage
}

// WaitOptions bounds sync execution waits.
type WaitOptions struct {
	TimeoutMillis   *int64
	CancelOnTimeout bool
}

const (
	// ChildExecutorModeFake selects deterministic in-process child execution.
	ChildExecutorModeFake = workflowruntime.ChildExecutionModeFake
	// ChildExecutorModeLive selects provider-backed child execution.
	ChildExecutorModeLive = workflowruntime.ChildExecutionModeLive
)

// RuntimeOptions selects durable JavaScript runtime execution behavior without
// changing workflow source syntax.
type RuntimeOptions struct {
	ChildExecutorMode string
}

// ResumeSessionRequest resumes one interrupted durable session from persisted
// checkpoint summaries and shared session state.
type ResumeSessionRequest struct {
	RequestID string
}

// StartRequest is the normalized durable session execution request shared by async
// and sync start across API, CLI, MCP, and UI.
type StartRequest struct {
	RequestID       string
	Source          Source
	Args            map[string]any
	Orchestrator    *OrchestratorOverride
	RequestedPolicy map[string]any
	Runtime         *RuntimeOptions
	Wait            *WaitOptions
}

// ResolvedSource is the customer-visible resolved source identity for one session.
type ResolvedSource struct {
	Kind            workflowsource.Kind
	SourceRef       string
	SourceHash      string
	Dialect         string
	ResolutionOrder []string
	Metadata        map[string]string
}

// InspectionLinks are API-relative links for polling and inspecting one session.
type InspectionLinks struct {
	Session    string
	Status     string
	Events     string
	Results    string
	Dispatches string
	Artifacts  string
}

// PolicyProjection carries requested and effective policy hashes for start responses.
type PolicyProjection struct {
	Requested     map[string]any
	Effective     map[string]any
	EffectiveHash string
}

// AsyncStartResult is the shared async durable execution start outcome.
type AsyncStartResult struct {
	SessionID        string
	Status           string
	OrchestratorKind string
	Dialect          string
	ResolvedSource   ResolvedSource
	SourceHash       string
	Policy           PolicyProjection
	Links            InspectionLinks
}

// SyncStartResult is the shared sync durable execution start outcome.
type SyncStartResult struct {
	AsyncStartResult
	SyncOutcome              SyncOutcome
	Result                   json.RawMessage
	TimedOut                 bool
	SessionCanceledByTimeout bool
}

// ErrExecutionRequestIDConflict reports that requestId was reused with a different
// normalized execution tuple or a different start operation (async vs sync).
var ErrExecutionRequestIDConflict = errors.New("execution request id conflict")

// ErrControlRequestIDConflict reports that requestId was reused with a different
// normalized lifecycle-control tuple.
var ErrControlRequestIDConflict = errors.New("control request id conflict")

// ErrSessionNotFound reports that no durable session matched the requested id.
var ErrSessionNotFound = errors.New("factory session not found")

// ErrDispatchNotFound reports that no dispatch matched the requested id within
// the targeted durable session.
var ErrDispatchNotFound = errors.New("dispatch not found")

// ErrArtifactNotFound reports that no artifact matched the requested id within
// the targeted durable session.
var ErrArtifactNotFound = errors.New("artifact not found")

// ErrReconnectCursorNotFound reports that the reconnect cursor did not match any
// recorded durable session event.
var ErrReconnectCursorNotFound = errors.New("reconnect cursor not found in event history")

// ErrUnsupportedControl reports that the requested control is not supported by
// the active durable session runtime.
var ErrUnsupportedControl = errors.New("unsupported lifecycle control")

// ControlError carries a typed lifecycle-control outcome for invalid transitions
// and other actionable control failures surfaced by service implementations.
type ControlError struct {
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Status    LifecycleStatus
	Message   string
}

func (e *ControlError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

// ValidationError reports a stable client-side normalization failure.
type ValidationError struct {
	Message string
	Field   string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewValidationError constructs one field-scoped validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// ResumeOutcome identifies one typed restart-resume failure class.
type ResumeOutcome string

const (
	// ResumeOutcomeMissingCheckpoint reports that no persisted checkpoint summary
	// exists for the interrupted session.
	ResumeOutcomeMissingCheckpoint ResumeOutcome = "MISSING_CHECKPOINT"
	// ResumeOutcomeInvalidState reports corrupted resume metadata or a session that
	// is not eligible for restart-resume reconstruction.
	ResumeOutcomeInvalidState ResumeOutcome = "INVALID_RESUME_STATE"
	// ResumeOutcomeCorruptedPersistence reports unreadable or invalid persisted
	// session snapshots required for restart-resume.
	ResumeOutcomeCorruptedPersistence ResumeOutcome = "CORRUPTED_PERSISTENCE"
)

// ResumeError carries a typed restart-resume failure surfaced by durable session
// execution when checkpoint summaries or persisted resume state are missing or invalid.
type ResumeError struct {
	Outcome   ResumeOutcome
	Status    LifecycleStatus
	Field     string
	Message   string
	SessionID string
}

func (e *ResumeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

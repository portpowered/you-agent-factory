package factorysessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/work"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"
)

func workContentJSONFromParts(parts []work.WorkContentPart) json.RawMessage {
	content := contentcontract.GeneratedPtrFromParts(parts)
	if content == nil {
		return nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return encoded
}

func resultSummaryTextFromParts(parts []work.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

// PetriSessionCompletion is the terminal invocation outcome recorded by the
// canonical Factory Session owner after a Petri runtime finishes.
type PetriSessionCompletion struct {
	Status        LifecycleStatus
	PrimaryResult []work.WorkContentPart
	Failure       *FailureSummary
}

// RecordPetriSessionCompletion advances a Petri-backed durable session to its
// terminal projection. Persistence succeeds before the result becomes visible
// to live readers or the invocation caller.
func (s *JavaScriptRuntimeService) RecordPetriSessionCompletion(
	sessionID string,
	completion PetriSessionCompletion,
) error {
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	if completion.Status != LifecycleStatusSucceeded && completion.Status != LifecycleStatusFailed {
		return NewValidationError("status", "Petri session completion status must be SUCCEEDED or FAILED")
	}
	_, err = s.snapshotSessionState(id)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[id]
	if !ok {
		initial := projectPetriRunningSessionState(id, s.now())
		state = &initial
	}
	if state.session.OrchestratorKind != interfaces.OrchestratorKindPetri {
		return fmt.Errorf("record Petri session completion: session %q uses orchestrator %q", id, state.session.OrchestratorKind)
	}
	if IsTerminalLifecycleStatus(state.session.Status) {
		if state.session.Status == completion.Status {
			return nil
		}
		return fmt.Errorf("record Petri session completion: session %q is already %s", id, state.session.Status)
	}

	candidate := projectPetriTerminalSessionState(*state, completion, s.now())
	if err := s.persistSessionSnapshot(candidate); err != nil {
		return err
	}
	if ok {
		*state = candidate
	} else {
		s.sessions[id] = &candidate
	}
	return nil
}

func projectPetriTerminalSessionState(
	state runtimeSessionState,
	completion PetriSessionCompletion,
	completedAt time.Time,
) runtimeSessionState {
	candidate := cloneRuntimeSessionState(&state)
	candidate.session.Status = completion.Status
	if candidate.session.Lifecycle == nil {
		candidate.session.Lifecycle = &LifecycleTimestamps{}
	}
	candidate.session.Lifecycle.FinishedAt = &completedAt
	candidate.session.Lifecycle.UpdatedAt = &completedAt
	candidate.session.Failure = cloneFailureSummary(completion.Failure)
	resultStatus := ResultStatusFinal
	if completion.Status == LifecycleStatusFailed {
		resultStatus = ResultStatusUnavailable
		if len(completion.PrimaryResult) > 0 {
			resultStatus = ResultStatusFailedWithPartial
		}
	}
	candidate.session.ResultSummary = &ResultSummary{
		ResultStatus: string(resultStatus),
		Summary:      resultSummaryTextFromParts(completion.PrimaryResult),
	}
	candidate.result = ResultReadResult{
		SessionID:     candidate.session.SessionID,
		ResultStatus:  resultStatus,
		SessionStatus: completion.Status,
		Mode:          ResultModeFinal,
		PrimaryResult: workContentJSONFromParts(completion.PrimaryResult),
		Failure:       cloneFailureSummary(completion.Failure),
	}
	if resultStatus == ResultStatusUnavailable {
		candidate.result.Availability = defaultUnavailableAvailability()
	}
	candidate.events = BuildCanonicalRuntimeSessionEvents(candidate.session, candidate.result)
	return candidate
}

func artifactStatesFromSummaries(artifacts []ArtifactSummary) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	states := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		states = append(states, interfaces.FactorySessionArtifactState{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, AuditMode: artifact.AuditMode,
		})
	}
	return states
}

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
	records := make([]DurableSessionRecord, 0, len(state.events)+len(state.runtimeRecords)+len(state.petriMutations))
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
	for _, mutation := range state.petriMutations {
		cloned := mutation
		records = append(records, DurableSessionRecord{
			Kind: DurableRecordKindPetriTokenMutation, PetriMutation: &cloned,
		})
	}
	return records
}

func runtimeHistoryFromDurableRecords(records []DurableSessionRecord) (
	[]json.RawMessage,
	[]workflowruntime.RuntimeRecord,
	[]interfaces.TokenMutationRecord,
) {
	events := make([]json.RawMessage, 0, len(records))
	runtimeRecords := make([]workflowruntime.RuntimeRecord, 0, len(records))
	petriMutations := make([]interfaces.TokenMutationRecord, 0, len(records))
	for _, record := range records {
		switch record.Kind {
		case DurableRecordKindCanonicalFactoryEvent:
			events = append(events, append(json.RawMessage(nil), record.CanonicalEvent...))
		case DurableRecordKindJavaScriptRuntime:
			runtimeRecords = append(runtimeRecords, cloneRuntimeRecord(*record.JavaScriptRecord))
		case DurableRecordKindPetriTokenMutation:
			petriMutations = append(petriMutations, *record.PetriMutation)
		}
	}
	return events, runtimeRecords, petriMutations
}

func runtimeHistoryFromPersistedSnapshot(snapshot PersistedRuntimeSessionState) (
	[]json.RawMessage,
	[]workflowruntime.RuntimeRecord,
	[]interfaces.TokenMutationRecord,
) {
	if len(snapshot.Records) > 0 {
		return runtimeHistoryFromDurableRecords(snapshot.Records)
	}
	events := make([]json.RawMessage, len(snapshot.Events))
	for i, event := range snapshot.Events {
		events[i] = append(json.RawMessage(nil), event...)
	}
	return events, cloneRuntimeRecords(snapshot.RuntimeRecords), nil
}

func clonePetriMutations(mutations []interfaces.TokenMutationRecord) []interfaces.TokenMutationRecord {
	if len(mutations) == 0 {
		return nil
	}
	cloned := make([]interfaces.TokenMutationRecord, len(mutations))
	copy(cloned, mutations)
	return cloned
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
	Agents          map[string]interfaces.FactoryOrchestratorJavaScriptAgent
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

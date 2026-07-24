package factorysessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"strings"
	"time"
)

func workContentJSONFromParts(parts []work.WorkContentPart) json.RawMessage {
	parts = work.SupportedContentParts(parts)
	if len(parts) == 0 {
		return nil
	}
	encoded, err := json.Marshal(parts)
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
	Kind             DurableRecordKind                       `json:"kind"`
	CanonicalEvent   json.RawMessage                         `json:"canonicalEvent,omitempty"`
	JavaScriptRecord *workflowsource.JavaScriptRuntimeRecord `json:"javascriptRecord,omitempty"`
	PetriMutation    *interfaces.TokenMutationRecord         `json:"petriMutation,omitempty"`
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
	[]workflowsource.JavaScriptRuntimeRecord,
	[]interfaces.TokenMutationRecord,
) {
	events := make([]json.RawMessage, 0, len(records))
	runtimeRecords := make([]workflowsource.JavaScriptRuntimeRecord, 0, len(records))
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
	[]workflowsource.JavaScriptRuntimeRecord,
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

const (
	SyncOutcomeCompleted    SyncOutcome = "COMPLETED"
	SyncOutcomeTimedOut     SyncOutcome = "TIMED_OUT"
	SyncOutcomeStillRunning SyncOutcome = "STILL_RUNNING"
)

// ErrControlRequestIDConflict reports that requestId was reused with a different
// normalized lifecycle-control tuple.
var ErrControlRequestIDConflict = errors.New("control request id conflict")

// ErrUnsupportedControl reports that the requested control is not supported by
// the active durable session runtime.
var ErrUnsupportedControl = errors.New("unsupported lifecycle control")

func NewValidationError(field, message string) *ValidationError {
	return factorysessions.NewValidationError(field, message)
}

package worker_capture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	// WorkerPortableRecordingKind identifies the source-native Worker Session
	// recording envelope. It is deliberately separate from the generic Factory
	// Session recording contract: a Worker recording contains provider-neutral
	// Worker records, not a Factory projection or provider transcript.
	WorkerPortableRecordingKind = "you.worker-session.recording"

	WorkerPortableRecordingSchemaV1        = "1"
	WorkerPortableRecordingReplayCompatV1  = "1"
	WorkerPortableRecordingIntegritySHA256 = "sha256"
)

// WorkerPortableRecording is the detached, self-validating Worker Session
// recording contract. Records retain the exact source-native payload while the
// surrounding facts make lifecycle, correlation, provider, provenance, and
// fidelity claims inspectable without a Workers or Providers dependency.
type WorkerPortableRecording struct {
	RecordingKind              string                             `json:"recordingKind"`
	SchemaVersion              string                             `json:"schemaVersion"`
	ReplayCompatibilityVersion string                             `json:"replayCompatibilityVersion"`
	Identity                   WorkerPortableRecordingIdentity    `json:"identity"`
	Lifecycle                  WorkerPortableRecordingLifecycle   `json:"lifecycle"`
	Correlation                WorkerPortableRecordingCorrelation `json:"correlation"`
	Provider                   WorkerPortableProviderAttribution  `json:"provider"`
	Records                    []WorkerPortableRecord             `json:"records"`
	Integrity                  WorkerPortableRecordingIntegrity   `json:"integrity"`
}

// WorkerPortableRecordingIdentity is the detached source identity required to
// replay one Worker Session without consulting the live Events service.
type WorkerPortableRecordingIdentity struct {
	RecordingID     string       `json:"recordingId"`
	WorkerSessionID string       `json:"workerSessionId"`
	Topic           events.Topic `json:"topic"`
}

// WorkerPortableRecordingLifecycle contains the authoritative opening and
// terminal facts. OpeningTimestamp is optional because older source-native
// opening records did not carry StartedAt; when present it is preserved
// exactly through export and replay.
type WorkerPortableRecordingLifecycle struct {
	Status           WorkerRecordingStatus   `json:"status"`
	OpeningTimestamp *time.Time              `json:"openingTimestamp,omitempty"`
	Terminal         *WorkerPortableTerminal `json:"terminal"`
}

// WorkerPortableRecordingCorrelation contains the opening SessionPayload's
// detached correlation facts. It intentionally carries no provider client or
// transcript handle.
type WorkerPortableRecordingCorrelation struct {
	WorkerType        string                            `json:"workerType,omitempty"`
	FactorySessionID  string                            `json:"factorySessionId,omitempty"`
	RecordingID       string                            `json:"recordingId,omitempty"`
	DispatchID        string                            `json:"dispatchId,omitempty"`
	TransitionID      string                            `json:"transitionId,omitempty"`
	WorkstationName   string                            `json:"workstationName,omitempty"`
	TurnID            string                            `json:"turnId,omitempty"`
	TraceID           string                            `json:"traceId,omitempty"`
	ReplayKey         string                            `json:"replayKey,omitempty"`
	WorkIDs           []string                          `json:"workIds,omitempty"`
	AttemptID         string                            `json:"attemptId,omitempty"`
	Attempt           int                               `json:"attempt,omitempty"`
	AttemptReason     workers.AttemptReason             `json:"attemptReason,omitempty"`
	Continuation      *workers.SessionContinuation      `json:"continuation,omitempty"`
	Lineage           *workers.SessionLineage           `json:"lineage,omitempty"`
	ProviderSelection *workers.SessionProviderSelection `json:"providerSelection,omitempty"`
	Model             string                            `json:"model,omitempty"`
	ReasoningEffort   string                            `json:"reasoningEffort,omitempty"`
}

// WorkerPortableProviderAttribution records provider identity and the
// optional opaque Provider Session reference actually observed in Worker
// records. An absent ProviderSessionRef is meaningful and valid.
type WorkerPortableProviderAttribution struct {
	Provider           string `json:"provider,omitempty"`
	ProviderSessionRef string `json:"providerSessionRef,omitempty"`
}

// WorkerPortableTerminal is the detached terminal lifecycle fact required for
// a completed portable recording.
type WorkerPortableTerminal struct {
	Position events.AggregateSequence `json:"position"`
	Phase    workers.Phase            `json:"phase"`
	Status   string                   `json:"status"`
}

// WorkerPortableRecord contains one canonical Worker record in an explicit
// portable shape. Payload is the exact source-native workers.Draft JSON; the
// adjacent fields are detached facts validated against that payload.
type WorkerPortableRecord struct {
	Position           events.AggregateSequence `json:"position"`
	SourceType         events.SourceType        `json:"sourceType"`
	SourceID           events.SourceID          `json:"sourceId"`
	SourceSequence     events.SourceSequence    `json:"sourceSequence"`
	SourceEventID      events.SourceEventID     `json:"sourceEventId"`
	SchemaID           events.SchemaID          `json:"schemaId"`
	Kind               workers.Kind             `json:"kind"`
	Phase              workers.Phase            `json:"phase"`
	Provenance         workers.Provenance       `json:"provenance"`
	RunID              string                   `json:"runId,omitempty"`
	DispatchID         string                   `json:"dispatchId,omitempty"`
	TurnID             string                   `json:"turnId,omitempty"`
	ItemID             string                   `json:"itemId,omitempty"`
	ParentItemID       string                   `json:"parentItemId,omitempty"`
	ProviderSessionRef string                   `json:"providerSessionRef,omitempty"`
	Payload            json.RawMessage          `json:"payload"`
}

// WorkerPortableRecordingIntegrity protects the complete detached envelope,
// including record payloads and all fidelity facts. Digest is computed with
// the Digest field empty.
type WorkerPortableRecordingIntegrity struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

// WorkerPortableRecordingDiagnosticCode identifies a stable validation area.
type WorkerPortableRecordingDiagnosticCode string

const (
	WorkerPortableCodeMalformedContract  WorkerPortableRecordingDiagnosticCode = "MALFORMED_CONTRACT"
	WorkerPortableCodeUnsupportedVersion WorkerPortableRecordingDiagnosticCode = "UNSUPPORTED_COMPATIBILITY"
	WorkerPortableCodeInvalidIdentity    WorkerPortableRecordingDiagnosticCode = "INVALID_IDENTITY"
	WorkerPortableCodeInvalidLifecycle   WorkerPortableRecordingDiagnosticCode = "INVALID_LIFECYCLE"
	WorkerPortableCodeInvalidCorrelation WorkerPortableRecordingDiagnosticCode = "INVALID_CORRELATION"
	WorkerPortableCodeInvalidProvenance  WorkerPortableRecordingDiagnosticCode = "INVALID_PROVENANCE"
	WorkerPortableCodeInvalidFidelity    WorkerPortableRecordingDiagnosticCode = "INVALID_FIDELITY"
	WorkerPortableCodeInvalidOrder       WorkerPortableRecordingDiagnosticCode = "INVALID_ORDER"
	WorkerPortableCodeInvalidTerminal    WorkerPortableRecordingDiagnosticCode = "INVALID_TERMINAL"
	WorkerPortableCodeInvalidIntegrity   WorkerPortableRecordingDiagnosticCode = "INVALID_INTEGRITY"
)

// WorkerPortableRecordingDiagnostic is a stable actionable portable
// validation error. It never copies provider output or payload values into
// the diagnostic message.
type WorkerPortableRecordingDiagnostic struct {
	Code    WorkerPortableRecordingDiagnosticCode
	Path    string
	Message string
	cause   error
}

func (diagnostic *WorkerPortableRecordingDiagnostic) Error() string {
	if diagnostic == nil {
		return ""
	}
	if diagnostic.Path == "" {
		return string(diagnostic.Code) + ": " + diagnostic.Message
	}
	return string(diagnostic.Code) + " at " + diagnostic.Path + ": " + diagnostic.Message
}

func (diagnostic *WorkerPortableRecordingDiagnostic) Unwrap() error {
	if diagnostic == nil {
		return nil
	}
	return ErrWorkerPortableRecording
}

func (diagnostic *WorkerPortableRecordingDiagnostic) Is(target error) bool {
	return diagnostic != nil && (target == ErrWorkerPortableRecording || target == diagnostic.cause)
}

var (
	ErrWorkerPortableRecording              = errors.New("recordings: invalid portable Worker recording")
	ErrWorkerPortableRecordingCompatibility = errors.New("recordings: unsupported portable Worker recording compatibility")
	ErrWorkerPortableRecordingIdentity      = errors.New("recordings: invalid portable Worker recording identity")
	ErrWorkerPortableRecordingLifecycle     = errors.New("recordings: invalid portable Worker recording lifecycle")
	ErrWorkerPortableRecordingCorrelation   = errors.New("recordings: invalid portable Worker recording correlation")
	ErrWorkerPortableRecordingProvenance    = errors.New("recordings: invalid portable Worker recording provenance")
	ErrWorkerPortableRecordingFidelity      = errors.New("recordings: invalid portable Worker recording fidelity")
	ErrWorkerPortableRecordingOrder         = errors.New("recordings: invalid portable Worker recording order")
	ErrWorkerPortableRecordingTerminal      = errors.New("recordings: invalid portable Worker recording terminal")
	ErrWorkerPortableRecordingIntegrity     = errors.New("recordings: invalid portable Worker recording integrity")
)

// BuildWorkerPortableRecording exports one completed Worker snapshot. A
// snapshot with multiple Worker Sessions requires the selected Worker Session
// ID so the portable identity and reducer input remain unambiguous. The
// optional argument preserves the single-session convenience form.
func (codec WorkerRecordingCodec) BuildWorkerPortableRecording(snapshot WorkerRecordingSnapshot, workerSessionIDs ...string) (WorkerPortableRecording, error) {
	if strings.TrimSpace(snapshot.RecordingID) == "" {
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeInvalidIdentity, "identity.recordingId", "recording identity is required", ErrWorkerPortableRecordingIdentity,
		)
	}
	session, err := selectedWorkerSession(snapshot, workerSessionIDs...)
	if err != nil {
		return WorkerPortableRecording{}, err
	}
	replayed, err := codec.ReplayWorkerRecording(WorkerRecordingReplayRequest{
		Snapshot:        snapshot,
		WorkerSessionID: session.WorkerSessionID,
	})
	if err != nil {
		return WorkerPortableRecording{}, err
	}
	projection := replayed.Projection
	if !projection.Complete {
		message := "only a terminal-complete capture can be exported"
		if projection.Status == WorkerRecordingStatusDegraded {
			message = "degraded capture cannot be exported as lossless portable replay"
		}
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeInvalidLifecycle, "lifecycle.status", message, ErrWorkerPortableRecordingLifecycle,
		)
	}

	records := make([]WorkerPortableRecord, 0, len(projection.Records))
	for index, record := range projection.Records {
		portable, err := portableRecordFromCanonical(record)
		if err != nil {
			return WorkerPortableRecording{}, portableDiagnostic(
				WorkerPortableCodeMalformedContract, fmt.Sprintf("records[%d]", index), "Worker record cannot be detached", ErrWorkerPortableRecording,
			)
		}
		records = append(records, portable)
	}
	opening, err := decodeWorkerDraft(projection.Opening)
	if err != nil {
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeMalformedContract, "records[0].payload", "opening Worker record cannot be decoded", ErrWorkerPortableRecording,
		)
	}
	openingPayload := workers.SessionPayload{}
	if err := json.Unmarshal(opening.Payload, &openingPayload); err != nil {
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeInvalidCorrelation, "correlation", "opening SessionPayload cannot be decoded", ErrWorkerPortableRecordingCorrelation,
		)
	}
	terminal := projection.Terminal
	if terminal == nil {
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeInvalidTerminal, "lifecycle.terminal", "terminal lifecycle fact is required", ErrWorkerPortableRecordingTerminal,
		)
	}
	value := WorkerPortableRecording{
		RecordingKind:              WorkerPortableRecordingKind,
		SchemaVersion:              WorkerPortableRecordingSchemaV1,
		ReplayCompatibilityVersion: WorkerPortableRecordingReplayCompatV1,
		Identity: WorkerPortableRecordingIdentity{
			RecordingID:     snapshot.RecordingID,
			WorkerSessionID: projection.WorkerSessionID,
			Topic:           projection.Topic,
		},
		Lifecycle: WorkerPortableRecordingLifecycle{
			Status:           WorkerRecordingStatusComplete,
			OpeningTimestamp: cloneTime(openingPayload.StartedAt),
			Terminal: &WorkerPortableTerminal{
				Position: terminal.Position,
				Phase:    terminal.Phase,
				Status:   terminal.Status,
			},
		},
		Correlation: correlationFromSessionPayload(openingPayload),
		Provider:    providerAttributionFromRecords(records),
		Records:     records,
		Integrity: WorkerPortableRecordingIntegrity{
			Algorithm: WorkerPortableRecordingIntegritySHA256,
		},
	}
	if err := validateWorkerPortableRecording(value, false); err != nil {
		return WorkerPortableRecording{}, err
	}
	digest, err := workerPortableRecordingDigest(value)
	if err != nil {
		return WorkerPortableRecording{}, portableDiagnostic(
			WorkerPortableCodeInvalidIntegrity, "integrity.digest", "portable recording cannot be digested", ErrWorkerPortableRecordingIntegrity,
		)
	}
	value.Integrity.Digest = digest
	if err := codec.ValidateWorkerPortableRecording(value); err != nil {
		return WorkerPortableRecording{}, err
	}
	return cloneWorkerPortableRecording(value), nil
}

// ExportWorkerPortableRecording is the descriptive alias used by callers
// that treat a completed snapshot as an export source.
func (codec WorkerRecordingCodec) ExportWorkerPortableRecording(snapshot WorkerRecordingSnapshot, workerSessionIDs ...string) (WorkerPortableRecording, error) {
	return codec.BuildWorkerPortableRecording(snapshot, workerSessionIDs...)
}

func selectedWorkerSession(snapshot WorkerRecordingSnapshot, workerSessionIDs ...string) (WorkerSessionRecordingSnapshot, error) {
	if len(workerSessionIDs) > 1 {
		return WorkerSessionRecordingSnapshot{}, portableDiagnostic(
			WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "portable export accepts at most one Worker Session selector", ErrWorkerPortableRecordingIdentity,
		)
	}
	selectedID := ""
	if len(workerSessionIDs) == 1 {
		selectedID = strings.TrimSpace(workerSessionIDs[0])
		if selectedID == "" {
			return WorkerSessionRecordingSnapshot{}, portableDiagnostic(
				WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "Worker Session selector is required when supplied", ErrWorkerPortableRecordingIdentity,
			)
		}
	}
	if selectedID == "" && len(snapshot.Sessions) != 1 {
		return WorkerSessionRecordingSnapshot{}, portableDiagnostic(
			WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "portable export requires a Worker Session selector for a multi-session snapshot", ErrWorkerPortableRecordingIdentity,
		)
	}
	if selectedID == "" {
		selectedID = snapshot.Sessions[0].WorkerSessionID
	}
	seen := make(map[string]struct{}, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		if _, exists := seen[session.WorkerSessionID]; exists {
			return WorkerSessionRecordingSnapshot{}, portableDiagnostic(
				WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "snapshot contains duplicate Worker Session identities", ErrWorkerPortableRecordingIdentity,
			)
		}
		seen[session.WorkerSessionID] = struct{}{}
		if session.WorkerSessionID == selectedID {
			return session, nil
		}
	}
	return WorkerSessionRecordingSnapshot{}, portableDiagnostic(
		WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "selected Worker Session is not present in the snapshot", ErrWorkerPortableRecordingIdentity,
	)
}

// ValidateWorkerPortableRecording validates compatibility, detached identity,
// exact source order, reducer parity, provenance, fidelity, and integrity.
func (WorkerRecordingCodec) ValidateWorkerPortableRecording(recording WorkerPortableRecording) error {
	return validateWorkerPortableRecording(recording, true)
}

// EncodeWorkerPortableRecording validates and encodes exactly one portable
// Worker recording document.
func (codec WorkerRecordingCodec) EncodeWorkerPortableRecording(recording WorkerPortableRecording) ([]byte, error) {
	if err := codec.ValidateWorkerPortableRecording(recording); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(recording)
	if err != nil {
		return nil, portableDiagnostic(
			WorkerPortableCodeMalformedContract, "document", "portable recording could not be encoded", ErrWorkerPortableRecording,
		)
	}
	return payload, nil
}

// ReplayWorkerPortableRecording validates and reduces portable records using
// the same reducer used during live capture. No provider, Worker, clock,
// transcript, process, or network capability is accepted by this API.
func (codec WorkerRecordingCodec) ReplayWorkerPortableRecording(recording WorkerPortableRecording) (WorkerRecordingReplayResult, error) {
	if err := codec.ValidateWorkerPortableRecording(recording); err != nil {
		return WorkerRecordingReplayResult{}, err
	}
	records, err := canonicalRecords(recording)
	if err != nil {
		return WorkerRecordingReplayResult{}, err
	}
	return codec.ReplayWorkerRecording(WorkerRecordingReplayRequest{
		Snapshot: WorkerRecordingSnapshot{
			RecordingID: recording.Identity.RecordingID,
			Sessions: []WorkerSessionRecordingSnapshot{{
				WorkerSessionID: recording.Identity.WorkerSessionID,
				Topic:           recording.Identity.Topic,
				Status:          WorkerRecordingStatusComplete,
				LastPosition:    records[len(records)-1].ID.Position,
				Records:         records,
			}},
		},
	})
}

func validateWorkerPortableRecording(recording WorkerPortableRecording, requireIntegrity bool) error {
	if err := validatePortableHeader(recording); err != nil {
		return err
	}
	drafts, projection, err := reducePortableHistory(recording)
	if err != nil {
		return err
	}
	if !projection.Complete || projection.Terminal == nil {
		return portableDiagnostic(
			WorkerPortableCodeInvalidTerminal, "lifecycle.terminal", "portable Worker recording has no legal terminal", ErrWorkerPortableRecordingTerminal,
		)
	}
	if err := validatePortableFacts(recording, drafts, *projection.Terminal); err != nil {
		return err
	}
	if requireIntegrity {
		return validatePortableIntegrity(recording)
	}
	return nil
}

func validatePortableHeader(recording WorkerPortableRecording) error {
	if recording.RecordingKind != WorkerPortableRecordingKind {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "recordingKind", "recording kind is unsupported", ErrWorkerPortableRecordingIdentity)
	}
	if recording.SchemaVersion != WorkerPortableRecordingSchemaV1 || recording.ReplayCompatibilityVersion != WorkerPortableRecordingReplayCompatV1 {
		return portableDiagnostic(WorkerPortableCodeUnsupportedVersion, "replayCompatibilityVersion", "recording compatibility version is unsupported", ErrWorkerPortableRecordingCompatibility)
	}
	if err := validatePortableIdentity(recording.Identity); err != nil {
		return err
	}
	if !portableRecordingLifecycleIsComplete(recording.Lifecycle.Status) || recording.Lifecycle.Terminal == nil {
		return portableDiagnostic(WorkerPortableCodeInvalidLifecycle, "lifecycle", "portable Worker recording must be completed and contain a terminal", ErrWorkerPortableRecordingLifecycle)
	}
	if len(recording.Records) == 0 {
		return portableDiagnostic(WorkerPortableCodeInvalidOrder, "records", "at least one opening record is required", ErrWorkerPortableRecordingOrder)
	}
	return nil
}

func portableRecordingLifecycleIsComplete(status WorkerRecordingStatus) bool {
	// Schema v1 was emitted with the legacy capture-state spelling
	// COMPLETED. Keep that pinned portable vocabulary readable while all new
	// exports use the recording-health spelling COMPLETE.
	return status == WorkerRecordingStatusComplete || status == WorkerRecordingStatusCompleted
}

func reducePortableHistory(recording WorkerPortableRecording) ([]workers.Draft, WorkerRecordingProjection, error) {
	records, drafts, err := canonicalRecordsAndDrafts(recording)
	if err != nil {
		return nil, WorkerRecordingProjection{}, err
	}
	projection, err := (WorkerRecordingCodec{}).ReduceWorkerRecording(WorkerRecordingHistory{
		RecordingID: recording.Identity.RecordingID, WorkerSessionID: recording.Identity.WorkerSessionID,
		Topic: recording.Identity.Topic, Records: records,
	})
	if err != nil {
		return nil, WorkerRecordingProjection{}, portableReducerDiagnostic(err)
	}
	return drafts, projection, nil
}

func validatePortableFacts(recording WorkerPortableRecording, drafts []workers.Draft, terminal WorkerRecordingTerminal) error {
	if err := validatePortableTerminal(*recording.Lifecycle.Terminal, terminal); err != nil {
		return err
	}
	if err := validatePortableOpening(recording, drafts[0]); err != nil {
		return err
	}
	if err := validatePortableCorrelation(recording, drafts[0]); err != nil {
		return err
	}
	if err := validatePortableProvider(recording, drafts); err != nil {
		return err
	}
	return validatePortableFidelity(recording, drafts)
}

func validatePortableIntegrity(recording WorkerPortableRecording) error {
	if recording.Integrity.Algorithm != WorkerPortableRecordingIntegritySHA256 {
		return portableDiagnostic(WorkerPortableCodeInvalidIntegrity, "integrity.algorithm", "integrity algorithm is unsupported", ErrWorkerPortableRecordingIntegrity)
	}
	expected, err := workerPortableRecordingDigest(recording)
	if err != nil || recording.Integrity.Digest != expected {
		return portableDiagnostic(WorkerPortableCodeInvalidIntegrity, "integrity.digest", "integrity digest does not match the detached recording", ErrWorkerPortableRecordingIntegrity)
	}
	return nil
}

func validatePortableIdentity(identity WorkerPortableRecordingIdentity) error {
	if err := validateWorkerOpaqueIdentity(identity.RecordingID); err != nil {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "identity.recordingId", "recording identity is malformed", ErrWorkerPortableRecordingIdentity)
	}
	if err := validateWorkerOpaqueIdentity(identity.WorkerSessionID); err != nil {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "Worker Session identity is malformed", ErrWorkerPortableRecordingIdentity)
	}
	if err := identity.Topic.Validate(); err != nil || identity.Topic != canonicalWorkerTopic(identity.WorkerSessionID) {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "identity.topic", "topic is not the canonical Worker Session topic", ErrWorkerPortableRecordingIdentity)
	}
	return nil
}

func validatePortableOpening(recording WorkerPortableRecording, draft workers.Draft) error {
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted {
		return portableDiagnostic(WorkerPortableCodeInvalidLifecycle, "records[0]", "position 1 must be SESSION/STARTED", ErrWorkerPortableRecordingLifecycle)
	}
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return portableDiagnostic(WorkerPortableCodeInvalidLifecycle, "lifecycle.openingTimestamp", "opening timestamp cannot be read from the opening record", ErrWorkerPortableRecordingLifecycle)
	}
	if payload.WorkerSessionID != "" && payload.WorkerSessionID != recording.Identity.WorkerSessionID {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "identity.workerSessionId", "opening Worker Session identity disagrees with the portable identity", ErrWorkerPortableRecordingIdentity)
	}
	if payload.RecordingID != "" && payload.RecordingID != recording.Identity.RecordingID {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "identity.recordingId", "opening recording identity disagrees with the portable identity", ErrWorkerPortableRecordingIdentity)
	}
	if (payload.StartedAt == nil) != (recording.Lifecycle.OpeningTimestamp == nil) ||
		(payload.StartedAt != nil && !payload.StartedAt.Equal(*recording.Lifecycle.OpeningTimestamp)) {
		return portableDiagnostic(WorkerPortableCodeInvalidLifecycle, "lifecycle.openingTimestamp", "opening timestamp disagrees with the opening record", ErrWorkerPortableRecordingLifecycle)
	}
	return nil
}

func validatePortableTerminal(portable WorkerPortableTerminal, reduced WorkerRecordingTerminal) error {
	if portable.Position != reduced.Position || portable.Phase != reduced.Phase || portable.Status != reduced.Status {
		return portableDiagnostic(WorkerPortableCodeInvalidTerminal, "lifecycle.terminal", "terminal facts disagree with the canonical Worker record", ErrWorkerPortableRecordingTerminal)
	}
	return nil
}

func validatePortableCorrelation(recording WorkerPortableRecording, opening workers.Draft) error {
	var payload workers.SessionPayload
	if err := json.Unmarshal(opening.Payload, &payload); err != nil {
		return portableDiagnostic(WorkerPortableCodeInvalidCorrelation, "correlation", "opening correlation payload is malformed", ErrWorkerPortableRecordingCorrelation)
	}
	want := correlationFromSessionPayload(payload)
	if !reflect.DeepEqual(recording.Correlation, want) {
		return portableDiagnostic(WorkerPortableCodeInvalidCorrelation, "correlation", "correlation facts disagree with the opening record", ErrWorkerPortableRecordingCorrelation)
	}
	return nil
}

func validatePortableProvider(recording WorkerPortableRecording, drafts []workers.Draft) error {
	state, err := buildPortableProviderState(drafts)
	if err != nil {
		return err
	}
	if err := validatePortableProviderEnvelope(recording, state); err != nil {
		return err
	}
	for index, draft := range drafts {
		if err := validatePortableProviderDraft(index, draft, state.boundProvider); err != nil {
			return err
		}
	}
	return nil
}

type portableProviderState struct {
	boundProvider      string
	providerSessionRef string
}

func buildPortableProviderState(drafts []workers.Draft) (portableProviderState, error) {
	state := portableProviderState{}
	for index, draft := range drafts {
		if err := validatePortableProviderRecord(&state, index, draft); err != nil {
			return portableProviderState{}, err
		}
	}
	return state, nil
}

func validatePortableProviderRecord(state *portableProviderState, index int, draft workers.Draft) error {
	ref := strings.TrimSpace(draft.ProviderSessionRef)
	if ref != "" {
		if ref != draft.ProviderSessionRef || strings.IndexFunc(ref, func(r rune) bool { return r < 0x20 }) >= 0 {
			return portableDiagnostic(WorkerPortableCodeInvalidIdentity, fmt.Sprintf("records[%d].providerSessionRef", index), "Provider Session reference is malformed", ErrWorkerPortableRecordingIdentity)
		}
		if state.providerSessionRef != "" && state.providerSessionRef != ref {
			return portableDiagnostic(WorkerPortableCodeInvalidIdentity, fmt.Sprintf("records[%d].providerSessionRef", index), "Provider Session reference changed within one recording", ErrWorkerPortableRecordingIdentity)
		}
		state.providerSessionRef = ref
	}
	provider := canonicalProvider(draft.Provenance.Provider)
	if provider == "" {
		return nil
	}
	if state.boundProvider == "" {
		if index != 0 && !isProviderBindingRecord(draft) {
			return portableDiagnostic(WorkerPortableCodeInvalidProvenance, fmt.Sprintf("records[%d].provenance.provider", index), "provider output appears before provider binding", ErrWorkerPortableRecordingProvenance)
		}
		state.boundProvider = provider
		return nil
	}
	if !sameProvider(state.boundProvider, provider) {
		return portableDiagnostic(WorkerPortableCodeInvalidProvenance, fmt.Sprintf("records[%d].provenance.provider", index), "provider attribution changed within one recording", ErrWorkerPortableRecordingProvenance)
	}
	return nil
}

func validatePortableProviderEnvelope(recording WorkerPortableRecording, state portableProviderState) error {
	if state.boundProvider != "" && recording.Provider.Provider == "" {
		return portableDiagnostic(WorkerPortableCodeInvalidProvenance, "provider.provider", "provider attribution is missing", ErrWorkerPortableRecordingProvenance)
	}
	if recording.Provider.Provider != "" && !sameProvider(recording.Provider.Provider, state.boundProvider) {
		return portableDiagnostic(WorkerPortableCodeInvalidProvenance, "provider.provider", "provider attribution disagrees with Worker records", ErrWorkerPortableRecordingProvenance)
	}
	if recording.Provider.ProviderSessionRef != state.providerSessionRef {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "provider.providerSessionRef", "Provider Session reference disagrees with Worker records", ErrWorkerPortableRecordingIdentity)
	}
	if state.providerSessionRef != "" && state.boundProvider == "" {
		return portableDiagnostic(WorkerPortableCodeInvalidIdentity, "provider.providerSessionRef", "Provider Session reference has no provider binding", ErrWorkerPortableRecordingIdentity)
	}
	return nil
}

func validatePortableProviderDraft(index int, draft workers.Draft, boundProvider string) error {
	if isLifecycleProvenance(draft) {
		return nil
	}
	if draft.Provenance.Provider == "" {
		return portableDiagnostic(WorkerPortableCodeInvalidProvenance, fmt.Sprintf("records[%d].provenance.provider", index), "provider output is missing provider attribution", ErrWorkerPortableRecordingProvenance)
	}
	if !isProviderBindingRecord(draft) && boundProvider == "" {
		return portableDiagnostic(WorkerPortableCodeInvalidProvenance, fmt.Sprintf("records[%d]", index), "provider output has no authoritative binding", ErrWorkerPortableRecordingProvenance)
	}
	return nil
}

func validatePortableFidelity(recording WorkerPortableRecording, drafts []workers.Draft) error {
	facts := portableFidelityFacts{}
	for index, draft := range drafts {
		if err := validateDraftProvenance(draft); err != nil {
			code, cause := WorkerPortableCodeInvalidProvenance, ErrWorkerPortableRecordingProvenance
			if errors.Is(err, ErrWorkerPortableRecordingFidelity) {
				code, cause = WorkerPortableCodeInvalidFidelity, ErrWorkerPortableRecordingFidelity
			}
			return portableDiagnostic(code, fmt.Sprintf("records[%d].provenance", index), "provenance facts are unsupported or contradictory", cause)
		}
		if isLifecycleProvenance(draft) || (draft.Kind == workers.KindError && draft.Provenance.Delivery == workers.DeliverySynthesized) {
			continue
		}
		nextFacts, err := addPortableFidelityFacts(facts, draft)
		if err != nil {
			return portableDiagnostic(WorkerPortableCodeInvalidFidelity, fmt.Sprintf("records[%d].provenance.delivery", index), err.Error(), ErrWorkerPortableRecordingFidelity)
		}
		facts = nextFacts
	}
	return validatePortableFidelityFacts(recording, facts)
}

type portableFidelityFacts struct {
	hasDelta     bool
	hasSnapshot  bool
	hasFinalOnly bool
}

func addPortableFidelityFacts(facts portableFidelityFacts, draft workers.Draft) (portableFidelityFacts, error) {
	switch draft.Provenance.Delivery {
	case workers.DeliveryNativeFinal:
		facts.hasFinalOnly = true
	case workers.DeliveryNativeStream:
		if draft.Provenance.Representation == workers.RepresentationDelta {
			facts.hasDelta = true
		} else {
			facts.hasSnapshot = true
		}
	case workers.DeliverySynthesized:
		return portableFidelityFacts{}, errors.New("synthesized provider output cannot claim portable fidelity")
	default:
		return portableFidelityFacts{}, errors.New("provider delivery is unsupported")
	}
	return facts, nil
}

func validatePortableFidelityFacts(recording WorkerPortableRecording, facts portableFidelityFacts) error {
	if facts.hasFinalOnly && (facts.hasDelta || facts.hasSnapshot) {
		return portableDiagnostic(WorkerPortableCodeInvalidFidelity, "records", "final-only output cannot be mixed with streaming or snapshot delivery", ErrWorkerPortableRecordingFidelity)
	}
	if facts.hasFinalOnly && recording.Provider.Provider == "" {
		return portableDiagnostic(WorkerPortableCodeInvalidFidelity, "provider.provider", "final-only output requires provider attribution", ErrWorkerPortableRecordingFidelity)
	}
	return nil
}

func validateDraftProvenance(draft workers.Draft) error {
	if err := validateProvenanceVocabulary(draft.Provenance); err != nil {
		return err
	}
	return validateProvenanceRelationships(draft)
}

func validateProvenanceVocabulary(provenance workers.Provenance) error {
	if strings.TrimSpace(provenance.NativeEventType) == "" || provenance.NativeEventType != strings.TrimSpace(provenance.NativeEventType) {
		return ErrWorkerPortableRecordingProvenance
	}
	if err := validateProvenanceDelivery(provenance.Delivery); err != nil {
		return err
	}
	if err := validateProvenanceRepresentation(provenance.Representation); err != nil {
		return err
	}
	return validateProvenanceFidelity(provenance.Fidelity)
}

func validateProvenanceDelivery(delivery workers.Delivery) error {
	switch delivery {
	case workers.DeliveryNativeStream, workers.DeliveryNativeFinal, workers.DeliverySynthesized:
		return nil
	default:
		return ErrWorkerPortableRecordingProvenance
	}
}

func validateProvenanceRepresentation(representation workers.Representation) error {
	switch representation {
	case workers.RepresentationDelta, workers.RepresentationSnapshot, workers.RepresentationNotification:
		return nil
	default:
		return ErrWorkerPortableRecordingProvenance
	}
}

func validateProvenanceFidelity(fidelity workers.Fidelity) error {
	switch fidelity {
	case workers.FidelityLossless, workers.FidelityNormalized, workers.FidelityLossy, workers.FidelityFinalOnly, workers.FidelityLifecycleOnly:
		return nil
	default:
		return ErrWorkerPortableRecordingProvenance
	}
}

func validateProvenanceRelationships(draft workers.Draft) error {
	if draft.Provenance.Fidelity == workers.FidelityFinalOnly &&
		(draft.Provenance.Delivery != workers.DeliveryNativeFinal || draft.Provenance.Representation != workers.RepresentationSnapshot) {
		return ErrWorkerPortableRecordingFidelity
	}
	if draft.Provenance.Fidelity == workers.FidelityLifecycleOnly && draft.Provenance.Representation != workers.RepresentationNotification {
		return ErrWorkerPortableRecordingFidelity
	}
	if draft.Provenance.Delivery == workers.DeliveryNativeFinal && draft.Provenance.Fidelity != workers.FidelityFinalOnly {
		return ErrWorkerPortableRecordingFidelity
	}
	if draft.Provenance.Delivery == workers.DeliveryNativeStream && draft.Provenance.Fidelity == workers.FidelityFinalOnly {
		return ErrWorkerPortableRecordingFidelity
	}
	if draft.Provenance.Representation == workers.RepresentationDelta && draft.Phase != workers.PhaseDelta {
		return ErrWorkerPortableRecordingFidelity
	}
	if draft.Provenance.Delivery == workers.DeliverySynthesized &&
		draft.Provenance.Fidelity != workers.FidelityLifecycleOnly && draft.Kind != workers.KindError {
		return ErrWorkerPortableRecordingFidelity
	}
	return nil
}

func canonicalRecordsAndDrafts(recording WorkerPortableRecording) ([]events.Record, []workers.Draft, error) {
	records := make([]events.Record, 0, len(recording.Records))
	drafts := make([]workers.Draft, 0, len(recording.Records))
	seen := make(map[events.AppendIdentity]struct{}, len(recording.Records))
	lastSourceSequences := make(map[string]events.SourceSequence)
	for index, portable := range recording.Records {
		record, draft, err := canonicalRecordAndDraft(recording, index, portable, seen, lastSourceSequences)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
		drafts = append(drafts, draft)
	}
	return records, drafts, nil
}

func canonicalRecordAndDraft(recording WorkerPortableRecording, index int, portable WorkerPortableRecord, seen map[events.AppendIdentity]struct{}, lastSourceSequences map[string]events.SourceSequence) (events.Record, workers.Draft, error) {
	if portable.Position != events.AggregateSequence(index+1) {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeInvalidOrder, fmt.Sprintf("records[%d].position", index), "aggregate positions must be contiguous from one", ErrWorkerPortableRecordingOrder)
	}
	record := events.Record{
		ID:         events.RecordID{Topic: recording.Identity.Topic, Position: portable.Position},
		SourceType: portable.SourceType, SourceID: portable.SourceID,
		SourceSequence: portable.SourceSequence, SourceEventID: portable.SourceEventID,
		SchemaID: portable.SchemaID, Payload: append(json.RawMessage(nil), portable.Payload...),
	}
	if err := record.Validate(); err != nil {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeMalformedContract, fmt.Sprintf("records[%d]", index), "canonical Worker record is malformed", ErrWorkerPortableRecording)
	}
	identity := record.Identity()
	if _, exists := seen[identity]; exists {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeInvalidIdentity, fmt.Sprintf("records[%d]", index), "source idempotency identity is duplicated", ErrWorkerPortableRecordingIdentity)
	}
	seen[identity] = struct{}{}
	sourceKey := string(record.SourceType) + "\x00" + string(record.SourceID)
	if previous, exists := lastSourceSequences[sourceKey]; exists && record.SourceSequence <= previous {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeInvalidOrder, fmt.Sprintf("records[%d].sourceSequence", index), "source sequence must increase for one source", ErrWorkerPortableRecordingOrder)
	}
	lastSourceSequences[sourceKey] = record.SourceSequence
	draft, err := decodeWorkerDraftStrict(record.Payload)
	if err != nil {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeMalformedContract, fmt.Sprintf("records[%d].payload", index), "Worker draft payload is malformed", ErrWorkerPortableRecording)
	}
	if !portableFactsMatchDraft(portable, draft) {
		return events.Record{}, workers.Draft{}, portableDiagnostic(WorkerPortableCodeInvalidProvenance, fmt.Sprintf("records[%d]", index), "detached Worker facts disagree with the canonical payload", ErrWorkerPortableRecordingProvenance)
	}
	return record, draft, nil
}

func portableFactsMatchDraft(portable WorkerPortableRecord, draft workers.Draft) bool {
	return portable.Kind == draft.Kind && portable.Phase == draft.Phase &&
		reflect.DeepEqual(portable.Provenance, draft.Provenance) &&
		portable.RunID == draft.RunID && portable.DispatchID == draft.DispatchID &&
		portable.TurnID == draft.TurnID && portable.ItemID == draft.ItemID &&
		portable.ParentItemID == draft.ParentItemID && portable.ProviderSessionRef == draft.ProviderSessionRef
}

func canonicalRecords(recording WorkerPortableRecording) ([]events.Record, error) {
	records, _, err := canonicalRecordsAndDrafts(recording)
	return records, err
}

func portableRecordFromCanonical(record events.Record) (WorkerPortableRecord, error) {
	draft, err := decodeWorkerDraft(record)
	if err != nil {
		return WorkerPortableRecord{}, err
	}
	return WorkerPortableRecord{
		Position: record.ID.Position, SourceType: record.SourceType, SourceID: record.SourceID,
		SourceSequence: record.SourceSequence, SourceEventID: record.SourceEventID,
		SchemaID: record.SchemaID, Kind: draft.Kind, Phase: draft.Phase, Provenance: draft.Provenance,
		RunID: draft.RunID, DispatchID: draft.DispatchID, TurnID: draft.TurnID,
		ItemID: draft.ItemID, ParentItemID: draft.ParentItemID,
		ProviderSessionRef: draft.ProviderSessionRef, Payload: append(json.RawMessage(nil), record.Payload...),
	}, nil
}

func providerAttributionFromRecords(records []WorkerPortableRecord) WorkerPortableProviderAttribution {
	value := WorkerPortableProviderAttribution{}
	for _, record := range records {
		if value.Provider == "" && strings.TrimSpace(record.Provenance.Provider) != "" {
			value.Provider = canonicalProvider(record.Provenance.Provider)
		}
		if value.ProviderSessionRef == "" && strings.TrimSpace(record.ProviderSessionRef) != "" {
			value.ProviderSessionRef = strings.TrimSpace(record.ProviderSessionRef)
		}
	}
	return value
}

func correlationFromSessionPayload(payload workers.SessionPayload) WorkerPortableRecordingCorrelation {
	return WorkerPortableRecordingCorrelation{
		WorkerType: payload.WorkerType, FactorySessionID: payload.FactorySessionID,
		RecordingID: payload.RecordingID, DispatchID: payload.DispatchID,
		TransitionID: payload.TransitionID, WorkstationName: payload.WorkstationName,
		TurnID: payload.TurnID, TraceID: payload.TraceID, ReplayKey: payload.ReplayKey,
		WorkIDs: append([]string(nil), payload.WorkIDs...), AttemptID: payload.AttemptID,
		Attempt: payload.Attempt, AttemptReason: payload.AttemptReason,
		Continuation:      cloneSessionContinuation(payload.Continuation),
		Lineage:           cloneSessionLineage(payload.Lineage),
		ProviderSelection: cloneProviderSelection(payload.ProviderSelection),
		Model:             payload.Model, ReasoningEffort: payload.ReasoningEffort,
	}
}

func workerPortableRecordingDigest(recording WorkerPortableRecording) (string, error) {
	clone := cloneWorkerPortableRecording(recording)
	clone.Integrity.Digest = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return WorkerPortableRecordingIntegritySHA256 + ":" + hex.EncodeToString(digest[:]), nil
}

func cloneWorkerPortableRecording(recording WorkerPortableRecording) WorkerPortableRecording {
	clone := recording
	clone.Identity = recording.Identity
	clone.Lifecycle = recording.Lifecycle
	clone.Lifecycle.OpeningTimestamp = cloneTime(recording.Lifecycle.OpeningTimestamp)
	if recording.Lifecycle.Terminal != nil {
		terminal := *recording.Lifecycle.Terminal
		clone.Lifecycle.Terminal = &terminal
	}
	clone.Correlation.WorkIDs = append([]string(nil), recording.Correlation.WorkIDs...)
	clone.Correlation.Continuation = cloneSessionContinuation(recording.Correlation.Continuation)
	clone.Correlation.Lineage = cloneSessionLineage(recording.Correlation.Lineage)
	clone.Correlation.ProviderSelection = cloneProviderSelection(recording.Correlation.ProviderSelection)
	clone.Records = make([]WorkerPortableRecord, len(recording.Records))
	for index, record := range recording.Records {
		clone.Records[index] = record
		clone.Records[index].Payload = append(json.RawMessage(nil), record.Payload...)
	}
	return clone
}

func cloneSessionContinuation(value *workers.SessionContinuation) *workers.SessionContinuation {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneProviderSelection(value *workers.SessionProviderSelection) *workers.SessionProviderSelection {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
}

func validateWorkerOpaqueIdentity(value string) error {
	if err := (events.SourceID(value)).Validate(); err != nil {
		return err
	}
	return nil
}

func isProviderBindingRecord(draft workers.Draft) bool {
	return draft.Kind == workers.KindSession && draft.Phase == workers.PhaseUpdated &&
		draft.Provenance.Fidelity == workers.FidelityLifecycleOnly && strings.TrimSpace(draft.Provenance.Provider) != ""
}

func isLifecycleProvenance(draft workers.Draft) bool {
	return draft.Provenance.Fidelity == workers.FidelityLifecycleOnly &&
		draft.Provenance.Representation == workers.RepresentationNotification &&
		(draft.Provenance.Delivery == workers.DeliveryNativeStream || draft.Provenance.Delivery == workers.DeliverySynthesized)
}

func canonicalProvider(value string) string {
	return providers.ID(strings.TrimSpace(value)).CanonicalSessionProvider()
}

func sameProvider(left, right string) bool {
	left, right = canonicalProvider(left), canonicalProvider(right)
	return left != "" && right != "" && left == right
}

func portableReducerDiagnostic(err error) error {
	code := WorkerPortableCodeMalformedContract
	cause := ErrWorkerPortableRecording
	switch {
	case errors.Is(err, ErrWorkerRecordingOrder):
		code, cause = WorkerPortableCodeInvalidOrder, ErrWorkerPortableRecordingOrder
	case errors.Is(err, ErrWorkerRecordingDuplicate):
		code, cause = WorkerPortableCodeInvalidIdentity, ErrWorkerPortableRecordingIdentity
	case errors.Is(err, ErrWorkerRecordingOpening):
		code, cause = WorkerPortableCodeInvalidLifecycle, ErrWorkerPortableRecordingLifecycle
	case errors.Is(err, ErrWorkerRecordingTerminal):
		code, cause = WorkerPortableCodeInvalidTerminal, ErrWorkerPortableRecordingTerminal
	case errors.Is(err, ErrWorkerRecordingIncomplete):
		code, cause = WorkerPortableCodeInvalidLifecycle, ErrWorkerPortableRecordingLifecycle
	}
	return portableDiagnostic(code, "records", "canonical Worker history failed deterministic validation", cause)
}

func portableDiagnostic(code WorkerPortableRecordingDiagnosticCode, path, message string, cause error) error {
	return &WorkerPortableRecordingDiagnostic{Code: code, Path: path, Message: message, cause: cause}
}

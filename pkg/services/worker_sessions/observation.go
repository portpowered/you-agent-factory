package workersessions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ObservationService is retained as the public name for the Worker Sessions
// observation capability. The capability is part of Service, so callers do
// not need a second service locator or a type assertion to inspect a session.
type ObservationService = Service

// ListObservationsRequest narrows observations to one Work identity.
type ListObservationsRequest struct {
	WorkID string
}

// Validate reports whether the request names a non-empty Work identity.
func (r ListObservationsRequest) Validate() error {
	if strings.TrimSpace(r.WorkID) == "" {
		return ErrInvalidObservationWorkID
	}
	return nil
}

// ListObservationsResult is a detached deterministic collection of
// observations.
type ListObservationsResult struct {
	Observations []Observation
}

// ObservationScope selects which Worker Session origin a top-level list may
// return. The zero value is the fleet-wide view; Direct and Factory remain
// explicit origin filters for callers that need one origin only.
type ObservationScope string

const (
	ObservationScopeDirect  ObservationScope = "direct"
	ObservationScopeFactory ObservationScope = "factory"
	ObservationScopeAll     ObservationScope = "all"
)

func (s ObservationScope) Valid() bool {
	switch s {
	case "", ObservationScopeDirect, ObservationScopeFactory, ObservationScopeAll:
		return true
	default:
		return false
	}
}

func (s ObservationScope) Normalized() ObservationScope {
	if s == "" {
		return ObservationScopeAll
	}
	return s
}

// ConfirmationState is the durability outcome for one Worker Session
// observation. It reports only whether the event that produced the current
// state or terminal outcome is covered by completed recording storage; it does
// not describe Worker execution progress or recording health.
type ConfirmationState string

const (
	ConfirmationStateConfirmed   ConfirmationState = "CONFIRMED"
	ConfirmationStateUnconfirmed ConfirmationState = "UNCONFIRMED"
)

const DefaultWorkerSessionObservationListMaxResults = 50

// ListWorkerSessionObservationsRequest is the bounded top-level observation
// query. NextToken is an opaque base64 cursor returned by the previous page.
type ListWorkerSessionObservationsRequest struct {
	Scope      ObservationScope
	States     []State
	MaxResults int
	NextToken  string
}

func (r ListWorkerSessionObservationsRequest) Validate() error {
	if !r.Scope.Valid() {
		return ErrInvalidObservationScope
	}
	for _, state := range r.States {
		if !state.Valid() {
			return ErrInvalidState
		}
	}
	if r.MaxResults < 0 {
		return ErrInvalidObservationPagination
	}
	if strings.TrimSpace(r.NextToken) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(r.NextToken))
		if err != nil || strings.TrimSpace(string(decoded)) == "" {
			return ErrInvalidObservationPagination
		}
	}
	return nil
}

type ListWorkerSessionObservationsResult struct {
	Observations []Observation
	MaxResults   int
	NextToken    string
}

// GetObservationRequest names one exact Provider Session identity.
type GetObservationRequest struct {
	ProviderSession providers.SessionRef
}

// Validate reports whether the request carries a complete typed identity.
func (r GetObservationRequest) Validate() error {
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	return nil
}

// GetObservationByWorkerSessionIDRequest names one canonical Worker Session
// without requiring a provider-native session reference.
type GetObservationByWorkerSessionIDRequest struct {
	WorkerSessionID string
}

// Validate reports whether the request carries a complete Worker Session
// identity.
func (r GetObservationByWorkerSessionIDRequest) Validate() error {
	if !validSessionID(r.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	return nil
}

// ReadTranscriptByWorkerSessionIDRequest names a Worker Session whose exact
// recorded Provider Session association should be projected.
type ReadTranscriptByWorkerSessionIDRequest struct {
	WorkerSessionID string
}

func (r ReadTranscriptByWorkerSessionIDRequest) Validate() error {
	if !validSessionID(r.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	return nil
}

// StreamObservationsRequest names one exact Provider Session identity and the
// bounded live-delivery capacity requested from Events.
type StreamObservationsRequest struct {
	ProviderSession providers.SessionRef
	// Limit bounds the retained batch and live buffer. Zero uses the stable
	// service default.
	Limit int
	// ReplayOnly drains the retained Events range captured when the stream is
	// opened and then returns one completeness summary without registering a
	// live follower.
	ReplayOnly bool
	// Cursor resumes strictly after the last acknowledged Worker Session event.
	// The cursor is scoped by the selected Provider Session and, when present,
	// the durable Factory event-stream generation.
	Cursor *ObservationCursor
}

const DefaultObservationStreamLimit = 64

// Validate reports whether the request carries a complete identity and a
// positive effective delivery limit.
func (r StreamObservationsRequest) Validate() error {
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	if r.Limit < 0 {
		return ErrInvalidObservationStreamLimit
	}
	if r.Cursor != nil {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// StreamObservationsByWorkerSessionIDRequest names one canonical Worker
// Session and the bounded delivery policy for its retained/live stream.
type StreamObservationsByWorkerSessionIDRequest struct {
	WorkerSessionID string
	Limit           int
	ReplayOnly      bool
	// Cursor resumes strictly after the last acknowledged Worker Session event.
	Cursor *ObservationCursor
}

// Validate reports whether the request carries a complete Worker Session
// identity and a non-negative effective delivery limit.
func (r StreamObservationsByWorkerSessionIDRequest) Validate() error {
	if !validSessionID(r.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	if r.Limit < 0 {
		return ErrInvalidObservationStreamLimit
	}
	if r.Cursor != nil {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ObservationCursor identifies one acknowledged Worker Session event. The
// position is the aggregate position exposed in ObservationEvent.Position;
// WorkerSessionID makes a cursor portable without allowing it to be replayed
// against another Worker Session; StreamGenerationID is populated for durable
// Factory history and is empty for the process-local Events fallback.
type ObservationCursor struct {
	WorkerSessionID    string
	StreamGenerationID string
	Position           uint64
}

// Validate rejects a cursor that cannot identify an acknowledged record.
// Position zero remains the unqualified start-of-stream cursor used only by
// the internal Events service, never by this reconnect contract.
func (c ObservationCursor) Validate() error {
	if c.Position == 0 {
		return ErrInvalidObservationCursor
	}
	if strings.TrimSpace(c.WorkerSessionID) != c.WorkerSessionID {
		return ErrInvalidObservationCursor
	}
	if strings.TrimSpace(c.StreamGenerationID) != c.StreamGenerationID {
		return ErrInvalidObservationCursor
	}
	return nil
}

// Clone returns a detached cursor value.
func (c ObservationCursor) Clone() ObservationCursor { return c }

// Observation is the detached authoritative projection shared by list and
// show. Optional values stay nil when the owning source cannot provide them;
// callers must not infer zero usage or zero duration from absence.
type Observation struct {
	WorkerSessionID            string
	PredecessorWorkerSessionID string
	SuccessorWorkerSessionID   string
	// Model and ReasoningEffort are the optional resolved execution facts
	// retained by Worker Sessions. Nil means the source record did not carry
	// that fact; callers must not infer it from current Factory configuration.
	Model           *string
	ReasoningEffort *string
	Direct          bool
	// FactorySessionID is the Factory Session that admitted this observation,
	// when the runtime supplied one. Worker Sessions records the correlation but
	// does not authorize or resolve a caller-selected scope here.
	FactorySessionID         string
	ProviderSession          providers.SessionRef
	ProviderSessionAvailable bool
	WorkIDs                  []string
	TurnID                   string
	AttemptID                string
	State                    State
	// ConfirmationState reports whether the canonical event responsible for
	// State or the terminal outcome is covered by the completed Recordings
	// flush watermark. Process-local observations and reads without a known
	// cursor remain UNCONFIRMED.
	ConfirmationState ConfirmationState
	// StreamGenerationID, StateSequence, and StateSequenceKnown are internal
	// read-boundary cursor facts. They are intentionally excluded from public
	// transport mapping; the runtime compares them with one sampled completed
	// flush watermark before returning the observation.
	StreamGenerationID string `json:"-"`
	StateSequence      int64  `json:"-"`
	StateSequenceKnown bool   `json:"-"`
	StartedAt          *time.Time
	EndedAt            *time.Time
	Duration           *time.Duration
	DurationBasis      DurationBasis
	TokenUsage         *TokenUsage
	TurnUsage          *TurnUsage
	Transcript         TranscriptAvailability
	// RecordingHealth is optional when the runtime has no durable Worker
	// recording configured. When present it is the Recordings-owned health
	// projection and never describes Worker execution outcome.
	RecordingHealth       recordings.WorkerRecordingStatus
	RecordingHealthReason string
	Failure               *FailureCause
	Parse                 ParseDiagnostics
}

// Validate reports whether an observation has a coherent detached identity,
// lifecycle, timing, and failure projection.
func (o Observation) Validate() error {
	if err := o.validateIdentity(); err != nil {
		return err
	}
	if err := o.validateLifecycleBasis(); err != nil {
		return err
	}
	if err := o.validateRecordingHealth(); err != nil {
		return err
	}
	if err := o.validateDuration(); err != nil {
		return err
	}
	return o.validateFailure()
}

// validateIdentity checks the detached Worker Session identity, its optional
// exact Provider Session reference, lifecycle state, and attempt identity.
func (o Observation) validateIdentity() error {
	if strings.TrimSpace(o.WorkerSessionID) == "" {
		return ErrInvalidObservationIdentity
	}
	if err := validateLineage(o.WorkerSessionID, o.PredecessorWorkerSessionID, o.SuccessorWorkerSessionID); err != nil {
		return err
	}
	if o.ProviderSessionAvailable {
		if err := o.ProviderSession.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
		}
	}
	if !o.State.Valid() {
		return ErrInvalidState
	}
	if strings.TrimSpace(o.AttemptID) == "" {
		return ErrInvalidObservationAttempt
	}
	return nil
}

// validateLifecycleBasis checks that the reported duration basis and
// transcript availability are coherent with the observation's terminal state.
func (o Observation) validateLifecycleBasis() error {
	if !o.DurationBasis.Valid() {
		return ErrInvalidObservationDuration
	}
	if !o.Transcript.Valid() {
		return ErrObservationProjectionUnavailable
	}
	if o.State.Terminal() && o.DurationBasis == DurationBasisActiveClock {
		return ErrInvalidObservationDuration
	}
	if !o.State.Terminal() && o.DurationBasis == DurationBasisRecordedTimestamps {
		return ErrInvalidObservationDuration
	}
	return nil
}

func (o Observation) validateRecordingHealth() error {
	if o.RecordingHealth == "" {
		if strings.TrimSpace(o.RecordingHealthReason) != "" {
			return ErrInvalidObservationRecordingHealth
		}
		return nil
	}
	switch o.RecordingHealth {
	case recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete:
		return nil
	default:
		return ErrInvalidObservationRecordingHealth
	}
}

// validateDuration checks that Duration is present only when its basis
// allows it, is never negative, and never precedes StartedAt.
func (o Observation) validateDuration() error {
	if o.DurationBasis == DurationBasisUnavailable && o.Duration != nil {
		return ErrInvalidObservationDuration
	}
	if o.Duration != nil && *o.Duration < 0 {
		return ErrInvalidObservationDuration
	}
	if o.StartedAt != nil && o.EndedAt != nil && o.EndedAt.Before(*o.StartedAt) {
		return ErrInvalidObservationDuration
	}
	return nil
}

// validateFailure checks that a present Failure is itself valid and only
// attached to a terminal, non-completed state.
func (o Observation) validateFailure() error {
	if o.Failure == nil {
		return nil
	}
	if err := o.Failure.Validate(); err != nil {
		return err
	}
	if !o.State.Terminal() || o.State == StateCompleted {
		return ErrInvalidObservationFailure
	}
	return nil
}

// Clone returns a detached observation snapshot.
func (o Observation) Clone() Observation {
	o.ProviderSession = o.ProviderSession.Clone()
	o.Model = cloneString(o.Model)
	o.ReasoningEffort = cloneString(o.ReasoningEffort)
	o.WorkIDs = append([]string(nil), o.WorkIDs...)
	if o.StartedAt != nil {
		started := *o.StartedAt
		o.StartedAt = &started
	}
	if o.EndedAt != nil {
		ended := *o.EndedAt
		o.EndedAt = &ended
	}
	if o.Duration != nil {
		duration := *o.Duration
		o.Duration = &duration
	}
	if o.TokenUsage != nil {
		tokens := o.TokenUsage.Clone()
		o.TokenUsage = &tokens
	}
	if o.TurnUsage != nil {
		turnUsage := o.TurnUsage.Clone()
		o.TurnUsage = &turnUsage
	}
	if o.Failure != nil {
		failure := *o.Failure
		o.Failure = &failure
	}
	o.Parse = o.Parse.Clone()
	return o
}

// DurationBasis explains which authoritative time facts produced Duration.
type DurationBasis string

const (
	DurationBasisUnavailable        DurationBasis = "UNAVAILABLE"
	DurationBasisActiveClock        DurationBasis = "ACTIVE_CLOCK"
	DurationBasisRecordedTimestamps DurationBasis = "RECORDED_TIMESTAMPS"
)

func (b DurationBasis) Valid() bool {
	switch b {
	case DurationBasisUnavailable, DurationBasisActiveClock, DurationBasisRecordedTimestamps:
		return true
	default:
		return false
	}
}

// TranscriptAvailability reports whether a normalized Provider Sessions
// transcript projection is available for this observation.
type TranscriptAvailability string

const (
	TranscriptAvailabilityUnavailable TranscriptAvailability = "UNAVAILABLE"
	TranscriptAvailabilityAvailable   TranscriptAvailability = "AVAILABLE"
)

func (a TranscriptAvailability) Valid() bool {
	return a == TranscriptAvailabilityUnavailable || a == TranscriptAvailabilityAvailable
}

// TokenUsage is the provider-neutral token projection used by Worker Session
// observations. A nil field means the source did not report that component.
type TokenUsage struct {
	CacheWriteTokens      *int
	CachedInputTokens     *int
	InputTokens           *int
	OutputTokens          *int
	ReasoningOutputTokens *int
	TotalTokens           *int
}

func (u TokenUsage) Clone() TokenUsage {
	clone := u
	clone.CacheWriteTokens = cloneInt(u.CacheWriteTokens)
	clone.CachedInputTokens = cloneInt(u.CachedInputTokens)
	clone.InputTokens = cloneInt(u.InputTokens)
	clone.OutputTokens = cloneInt(u.OutputTokens)
	clone.ReasoningOutputTokens = cloneInt(u.ReasoningOutputTokens)
	clone.TotalTokens = cloneInt(u.TotalTokens)
	return clone
}

// TurnUsage is the provider-neutral per-turn context projection used by
// Worker Session observations. The values are derived by differencing
// consecutive cumulative provider input counters; nil means the transcript
// did not provide supported evidence for the calculation.
type TurnUsage struct {
	TurnCount          int
	FinalContextTokens int
	PeakContextTokens  int
}

func (u TurnUsage) Clone() TurnUsage { return u }

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// ParseDiagnostics contains normalized parse facts from the Provider Sessions
// projection. It intentionally omits source paths and raw provider payloads.
type ParseDiagnostics struct {
	EventCount         int
	MalformedLineCount int
	UnknownEventCount  int
	Errors             []ParseDiagnostic
}

func (p ParseDiagnostics) Clone() ParseDiagnostics {
	clone := p
	clone.Errors = append([]ParseDiagnostic(nil), p.Errors...)
	return clone
}

// ParseDiagnostic is one safe, normalized parse issue.
type ParseDiagnostic struct {
	Code       string
	LineNumber int
	Message    string
}

// ObservationEvent is one detached canonical Events record projected without
// exposing the Events store or its implementation.
type ObservationEvent struct {
	Position       uint64
	Cursor         ObservationCursor
	SourceType     string
	SourceID       string
	SourceSequence uint64
	SourceEventID  string
	SchemaID       string
	Payload        json.RawMessage
}

func (e ObservationEvent) Clone() ObservationEvent {
	clone := e
	clone.Cursor = e.Cursor.Clone()
	clone.Payload = append(json.RawMessage(nil), e.Payload...)
	return clone
}

// ObservationDeliveryKind distinguishes an event, terminal completion,
// already-terminal replay, cancellation, and source failure without requiring
// callers to parse an error string or guess from an empty event.
type ObservationDeliveryKind string

const (
	ObservationDeliveryRecord         ObservationDeliveryKind = "RECORD"
	ObservationDeliveryTerminal       ObservationDeliveryKind = "TERMINAL"
	ObservationDeliveryTerminalReplay ObservationDeliveryKind = "TERMINAL_REPLAY"
	ObservationDeliveryReplaySummary  ObservationDeliveryKind = "REPLAY_SUMMARY"
	ObservationDeliveryClosed         ObservationDeliveryKind = "CLOSED"
	ObservationDeliveryCanceled       ObservationDeliveryKind = "CANCELED"
	ObservationDeliverySourceFailure  ObservationDeliveryKind = "SOURCE_FAILURE"
)

// ReplaySummary describes the completeness of one finite retained-history
// drain. EventsEmitted counts event records delivered before the summary.
type ReplaySummary struct {
	Complete      bool
	Reason        string
	EventsEmitted int
}

// ObservationDelivery is one subscription outcome. Event is present for
// RECORD, TERMINAL, and TERMINAL_REPLAY; Summary is present for
// REPLAY_SUMMARY and may accompany a terminal durable event; Err is present
// only for CANCELED or SOURCE_FAILURE.
type ObservationDelivery struct {
	Kind    ObservationDeliveryKind
	Event   ObservationEvent
	Summary *ReplaySummary
	Err     error
}

// ObservationSubscription is a cancellable retained/live canonical event
// stream. Close is idempotent and releases the underlying Events subscription.
// The function fields let the root service return a stable concrete handle
// while implementations keep their subscription state private.
type ObservationSubscription struct {
	NextFunc  func(context.Context) ObservationDelivery
	CloseFunc func()
}

func (s ObservationSubscription) Next(ctx context.Context) ObservationDelivery {
	if s.NextFunc == nil {
		return ObservationDelivery{Kind: ObservationDeliveryClosed, Err: ErrObservationSourceClosed}
	}
	return s.NextFunc(ctx)
}

func (s ObservationSubscription) Close() {
	if s.CloseFunc != nil {
		s.CloseFunc()
	}
}

var (
	ErrInvalidObservationWorkID          = errors.New("worker session observation: invalid work id")
	ErrInvalidObservationIdentity        = errors.New("worker session observation: invalid provider session identity")
	ErrInvalidObservationScope           = errors.New("worker session observation: invalid scope")
	ErrInvalidObservationPagination      = errors.New("worker session observation: invalid pagination")
	ErrInvalidObservationAttempt         = errors.New("worker session observation: invalid attempt")
	ErrInvalidObservationDuration        = errors.New("worker session observation: invalid duration projection")
	ErrInvalidObservationFailure         = errors.New("worker session observation: invalid failure projection")
	ErrInvalidObservationRecordingHealth = errors.New("worker session observation: invalid recording health")
	ErrInvalidObservationStreamLimit     = errors.New("worker session observation: stream limit must not be negative")
	ErrInvalidObservationCursor          = errors.New("worker session observation: invalid cursor")
	ErrObservationCursorForeign          = errors.New("worker session observation: cursor belongs to another Worker Session")
	ErrObservationCursorFuture           = errors.New("worker session observation: cursor is ahead of the available history")
	ErrObservationCursorStale            = errors.New("worker session observation: cursor is no longer retained")
	ErrObservationCursorUnavailable      = errors.New("worker session observation: cursor stream generation is unavailable")
	ErrObservationWorkNotFound           = errors.New("worker session observation: work not found")
	ErrObservationSessionNotFound        = errors.New("worker session observation: provider session not found")
	ErrObservationNotDirect              = errors.New("worker session observation: session is not direct")
	ErrObservationProjectionUnavailable  = errors.New("worker session observation: projection unavailable")
	ErrObservationRecordingCorrupt       = errors.New("worker session observation: recording history is corrupt")
	ErrObservationRecordingUnavailable   = errors.New("worker session observation: recording history unavailable")
	ErrObservationSourceUnavailable      = errors.New("worker session observation: event source unavailable")
	ErrObservationSourceGap              = errors.New("worker session observation: retained event gap")
	ErrObservationSourceClosed           = errors.New("worker session observation: event source closed before terminal")
	ErrObservationCanceled               = fmt.Errorf("worker session observation: canceled: %w", context.Canceled)
)

// ReadTranscriptRequest identifies one Worker Session whose normalized
// transcript should be returned. ProviderSession is retained as the legacy
// compatibility identity; new callers should use WorkerSessionID so the read
// remains provider-neutral and can resolve durable Factory Session history.
type ReadTranscriptRequest struct {
	WorkerSessionID string
	ProviderSession providers.SessionRef
}

// Validate reports whether the request carries exactly one complete typed
// identity. Accepting both identities would make a disagreement ambiguous and
// could let a legacy provider reference escape its Worker Session scope.
func (r ReadTranscriptRequest) Validate() error {
	workerSessionID := strings.TrimSpace(r.WorkerSessionID)
	providerIdentityPresent := strings.TrimSpace(string(r.ProviderSession.Provider)) != "" ||
		strings.TrimSpace(r.ProviderSession.Kind) != "" || strings.TrimSpace(r.ProviderSession.ID) != ""
	switch {
	case workerSessionID != "" && providerIdentityPresent:
		return fmt.Errorf("%w: Worker Session ID and Provider Session identity are mutually exclusive", ErrInvalidObservationIdentity)
	case workerSessionID != "":
		return nil
	default:
		if err := r.ProviderSession.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
		}
		return nil
	}
}

// ReadTranscriptResult is the detached Worker Session envelope and ordered
// normalized transcript returned for a finished Worker Session with readable
// provider-native transcript detail. A Worker without that detail receives
// ErrObservationTranscriptUnavailable while canonical history remains
// available through the Worker-ID event read.
type ReadTranscriptResult struct {
	WorkerSessionID string
	ProviderSession providers.SessionRef
	// RecordingHealth is required for provider-neutral durable transcripts and
	// remains empty for the legacy provider-backed projection.
	RecordingHealth       recordings.WorkerRecordingStatus
	RecordingHealthReason string
	WorkIDs               []string
	TurnID                string
	AttemptID             string
	State                 State
	Entries               []TranscriptEntry
}

// Clone returns a detached transcript result.
func (r ReadTranscriptResult) Clone() ReadTranscriptResult {
	clone := r
	clone.ProviderSession = r.ProviderSession.Clone()
	clone.WorkIDs = append([]string(nil), r.WorkIDs...)
	clone.Entries = make([]TranscriptEntry, len(r.Entries))
	for index, entry := range r.Entries {
		clone.Entries[index] = entry.Clone()
	}
	return clone
}

// Validate reports whether the transcript envelope has a coherent identity,
// provider transcript identity, terminal lifecycle state, and ordered entries.
func (r ReadTranscriptResult) Validate() error {
	if strings.TrimSpace(r.WorkerSessionID) == "" {
		return ErrInvalidObservationIdentity
	}
	providerIdentityPresent := strings.TrimSpace(string(r.ProviderSession.Provider)) != "" ||
		strings.TrimSpace(r.ProviderSession.Kind) != "" || strings.TrimSpace(r.ProviderSession.ID) != ""
	if providerIdentityPresent {
		if err := r.ProviderSession.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
		}
	} else if !validTranscriptRecordingHealth(r.RecordingHealth, r.RecordingHealthReason) {
		return ErrInvalidObservationRecordingHealth
	}
	if !r.State.Valid() {
		return ErrInvalidState
	}
	if !r.State.Terminal() && (providerIdentityPresent || r.RecordingHealth != recordings.WorkerRecordingStatusIncomplete) {
		return ErrObservationTranscriptActive
	}
	if strings.TrimSpace(r.AttemptID) == "" {
		return ErrInvalidObservationAttempt
	}
	for index, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("transcript entry %d: %w", index+1, err)
		}
	}
	return nil
}

func validTranscriptRecordingHealth(status recordings.WorkerRecordingStatus, reason string) bool {
	switch status {
	case recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete:
		return status == recordings.WorkerRecordingStatusComplete || strings.TrimSpace(reason) != ""
	default:
		return false
	}
}

// TranscriptEntryType is the normalized provider-neutral role or activity
// represented by one transcript entry.
type TranscriptEntryType string

const (
	TranscriptAssistantMessage TranscriptEntryType = "assistant_message"
	TranscriptReasoning        TranscriptEntryType = "reasoning"
	TranscriptSystemEvent      TranscriptEntryType = "system_event"
	TranscriptToolCall         TranscriptEntryType = "tool_call"
	TranscriptToolOutput       TranscriptEntryType = "tool_output"
	TranscriptUserMessage      TranscriptEntryType = "user_message"
)

// TranscriptEntry is one normalized, bounded transcript item. Optional
// fields remain nil when the Provider Sessions projection cannot supply them.
type TranscriptEntry struct {
	Arguments        *string
	CallID           *string
	Encrypted        *bool
	EncryptedContent *string
	LineNumber       *int
	Name             *string
	Order            int
	Output           *string
	SourceType       *string
	Status           *string
	Summary          *string
	Text             *string
	Timestamp        *time.Time
	TurnIndex        *int
	Type             TranscriptEntryType
}

// Validate reports whether the entry has a stable order and supported
// normalized type. Provider-specific payloads are intentionally not accepted.
func (e TranscriptEntry) Validate() error {
	if e.Order < 0 {
		return fmt.Errorf("transcript entry order must not be negative")
	}
	if strings.TrimSpace(string(e.Type)) == "" {
		return fmt.Errorf("transcript entry type is required")
	}
	return nil
}

// Clone returns a detached transcript entry.
func (e TranscriptEntry) Clone() TranscriptEntry {
	clone := e
	clone.Arguments = cloneString(e.Arguments)
	clone.CallID = cloneString(e.CallID)
	clone.Encrypted = cloneBool(e.Encrypted)
	clone.EncryptedContent = cloneString(e.EncryptedContent)
	clone.LineNumber = cloneInt(e.LineNumber)
	clone.Name = cloneString(e.Name)
	clone.Output = cloneString(e.Output)
	clone.SourceType = cloneString(e.SourceType)
	clone.Status = cloneString(e.Status)
	clone.Summary = cloneString(e.Summary)
	clone.Text = cloneString(e.Text)
	clone.Timestamp = cloneTime(e.Timestamp)
	clone.TurnIndex = cloneInt(e.TurnIndex)
	return clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneString(value *string) *string {
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
	clone := *value
	return &clone
}

var (
	// ErrObservationTranscriptActive means the requested session has not
	// reached an absorbing lifecycle state, so its transcript is not final.
	ErrObservationTranscriptActive = errors.New("worker session transcript: session is active")
	// ErrObservationTranscriptUnavailable means no normalized transcript is
	// available for an otherwise terminal Worker Session.
	ErrObservationTranscriptUnavailable = errors.New("worker session transcript: transcript unavailable")
	// ErrObservationTranscriptProjectionUnavailable means Provider Sessions
	// could not project the normalized transcript source.
	ErrObservationTranscriptProjectionUnavailable = errors.New("worker session transcript: projection unavailable")
)

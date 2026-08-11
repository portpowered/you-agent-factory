package workersessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
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
	return nil
}

// StreamObservationsByWorkerSessionIDRequest names one canonical Worker
// Session and the bounded delivery policy for its retained/live stream.
type StreamObservationsByWorkerSessionIDRequest struct {
	WorkerSessionID string
	Limit           int
	ReplayOnly      bool
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
	return nil
}

// Observation is the detached authoritative projection shared by list and
// show. Optional values stay nil when the owning source cannot provide them;
// callers must not infer zero usage or zero duration from absence.
type Observation struct {
	WorkerSessionID          string
	ProviderSession          providers.SessionRef
	ProviderSessionAvailable bool
	WorkIDs                  []string
	TurnID                   string
	AttemptID                string
	State                    State
	StartedAt                *time.Time
	EndedAt                  *time.Time
	Duration                 *time.Duration
	DurationBasis            DurationBasis
	TokenUsage               *TokenUsage
	Transcript               TranscriptAvailability
	Failure                  *FailureCause
	Parse                    ParseDiagnostics
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
	SourceType     string
	SourceID       string
	SourceSequence uint64
	SourceEventID  string
	SchemaID       string
	Payload        json.RawMessage
}

func (e ObservationEvent) Clone() ObservationEvent {
	clone := e
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
// REPLAY_SUMMARY; Err is present only for CANCELED or SOURCE_FAILURE.
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
	ErrInvalidObservationWorkID         = errors.New("worker session observation: invalid work id")
	ErrInvalidObservationIdentity       = errors.New("worker session observation: invalid provider session identity")
	ErrInvalidObservationAttempt        = errors.New("worker session observation: invalid attempt")
	ErrInvalidObservationDuration       = errors.New("worker session observation: invalid duration projection")
	ErrInvalidObservationFailure        = errors.New("worker session observation: invalid failure projection")
	ErrInvalidObservationStreamLimit    = errors.New("worker session observation: stream limit must not be negative")
	ErrObservationWorkNotFound          = errors.New("worker session observation: work not found")
	ErrObservationSessionNotFound       = errors.New("worker session observation: provider session not found")
	ErrObservationProjectionUnavailable = errors.New("worker session observation: projection unavailable")
	ErrObservationSourceUnavailable     = errors.New("worker session observation: event source unavailable")
	ErrObservationSourceGap             = errors.New("worker session observation: retained event gap")
	ErrObservationSourceClosed          = errors.New("worker session observation: event source closed before terminal")
	ErrObservationCanceled              = fmt.Errorf("worker session observation: canceled: %w", context.Canceled)
)

// ReadTranscriptRequest identifies one exact Provider Session whose normalized
// transcript should be returned.
type ReadTranscriptRequest struct {
	ProviderSession providers.SessionRef
}

// Validate reports whether the request carries a complete typed identity.
func (r ReadTranscriptRequest) Validate() error {
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	return nil
}

// ReadTranscriptResult is the detached Worker Session envelope and ordered
// normalized transcript returned for a finished Provider Session.
type ReadTranscriptResult struct {
	WorkerSessionID string
	ProviderSession providers.SessionRef
	WorkIDs         []string
	TurnID          string
	AttemptID       string
	State           State
	Entries         []TranscriptEntry
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
// terminal lifecycle state, and ordered entries.
func (r ReadTranscriptResult) Validate() error {
	if strings.TrimSpace(r.WorkerSessionID) == "" {
		return ErrInvalidObservationIdentity
	}
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	if !r.State.Valid() {
		return ErrInvalidState
	}
	if !r.State.Terminal() {
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

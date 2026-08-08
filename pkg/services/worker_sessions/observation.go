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

// ObservationService is the read/stream portion of the Worker Sessions root.
// It exposes only detached, provider-neutral facts owned by Worker Sessions,
// Provider Sessions, and Events. It does not expose provider readers,
// recording stores, filesystem paths, or storage handles.
//
// Service includes this contract so callers do not need a second service
// locator or a type assertion to inspect a Worker Session.
type ObservationService interface {
	// ListObservations returns every observed attempt correlated with req.WorkID
	// in deterministic Worker Session identity order.
	ListObservations(context.Context, ListObservationsRequest) (ListObservationsResult, error)

	// GetObservation returns the one observed attempt identified by its exact
	// Providers-owned provider/kind/id reference.
	GetObservation(context.Context, GetObservationRequest) (Observation, error)

	// ReadTranscript returns the normalized transcript for one terminal Worker
	// Session. Active sessions, missing sessions, unavailable transcripts, and
	// projection failures are distinct typed outcomes.
	ReadTranscript(context.Context, ReadTranscriptRequest) (ReadTranscriptResult, error)

	// StreamObservations subscribes to the canonical Worker Session event topic
	// for the exact provider session identity. The subscription first replays
	// retained Events records and then follows newly committed records.
	StreamObservations(context.Context, StreamObservationsRequest) (ObservationSubscription, error)
}

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

// StreamObservationsRequest names one exact Provider Session identity and the
// bounded live-delivery capacity requested from Events.
type StreamObservationsRequest struct {
	ProviderSession providers.SessionRef
	// Limit bounds the retained batch and live buffer. Zero uses the stable
	// service default.
	Limit int
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
	if !o.DurationBasis.Valid() {
		return ErrInvalidObservationDuration
	}
	if !o.Transcript.Valid() {
		return ErrObservationProjectionUnavailable
	}
	if o.DurationBasis == DurationBasisUnavailable && o.Duration != nil {
		return ErrInvalidObservationDuration
	}
	if o.Duration != nil && *o.Duration < 0 {
		return ErrInvalidObservationDuration
	}
	if o.StartedAt != nil && o.EndedAt != nil && o.EndedAt.Before(*o.StartedAt) {
		return ErrInvalidObservationDuration
	}
	if o.State.Terminal() && o.DurationBasis == DurationBasisActiveClock {
		return ErrInvalidObservationDuration
	}
	if !o.State.Terminal() && o.DurationBasis == DurationBasisRecordedTimestamps {
		return ErrInvalidObservationDuration
	}
	if o.Failure != nil {
		if err := o.Failure.Validate(); err != nil {
			return err
		}
		if !o.State.Terminal() || o.State == StateCompleted {
			return ErrInvalidObservationFailure
		}
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
	ObservationDeliveryClosed         ObservationDeliveryKind = "CLOSED"
	ObservationDeliveryCanceled       ObservationDeliveryKind = "CANCELED"
	ObservationDeliverySourceFailure  ObservationDeliveryKind = "SOURCE_FAILURE"
)

// ObservationDelivery is one subscription outcome. Event is present for
// RECORD, TERMINAL, and TERMINAL_REPLAY; Err is present only for CANCELED or
// SOURCE_FAILURE.
type ObservationDelivery struct {
	Kind  ObservationDeliveryKind
	Event ObservationEvent
	Err   error
}

// ObservationSubscription is a cancellable retained/live canonical event
// stream. Close is idempotent and releases the underlying Events subscription.
type ObservationSubscription interface {
	Next(context.Context) ObservationDelivery
	Close()
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

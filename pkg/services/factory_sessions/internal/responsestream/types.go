package responsestream

import (
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// EventKind identifies internal session response-stream record semantics.
// These kinds are intentionally separate from factoryapi.FactoryEventType and
// must not be projected into canonical factory event history.
type EventKind string

const (
	// EventKindProgressFragment carries transient provider progress that is
	// useful during execution but not part of canonical replay.
	EventKindProgressFragment EventKind = "PROGRESS_FRAGMENT"
	// EventKindResponseFragment carries partial provider response text before
	// terminal workstation completion.
	EventKindResponseFragment EventKind = "RESPONSE_FRAGMENT"
	// EventKindStreamCompleted marks provider-stream terminal success for one
	// dispatch without changing final WorkResult routing.
	EventKindStreamCompleted EventKind = "STREAM_COMPLETED"
	// EventKindStreamFailed marks provider-stream terminal failure for one
	// dispatch without changing final WorkResult routing.
	EventKindStreamFailed EventKind = "STREAM_FAILED"
	// EventKindCompactionSignal records bounded-fidelity degradation such as
	// truncation, coalescing, or age eviction for downstream consumers. At most
	// one compaction signal is retained; later compactions replace it at the tail.
	EventKindCompactionSignal EventKind = "STREAM_COMPACTION_SIGNAL"
)

// EventType identifies the provider-neutral semantic shape carried by one
// internal response-stream event.
type EventType string

const (
	EventTypeUnknown   EventType = "UNKNOWN"
	EventTypeStarted   EventType = "STARTED"
	EventTypeProgress  EventType = "PROGRESS"
	EventTypeTextDelta EventType = "TEXT_DELTA"
	EventTypeFinalText EventType = "FINAL_TEXT"
	EventTypeFailed    EventType = "FAILED"
	EventTypeCanceled  EventType = "CANCELED"
)

// CompactionReason classifies why retained stream fidelity was reduced.
type CompactionReason string

const (
	CompactionReasonTruncated  CompactionReason = "TRUNCATED"
	CompactionReasonCoalesced  CompactionReason = "COALESCED"
	CompactionReasonAgeEvicted CompactionReason = "AGE_EVICTED"
)

// RetentionLimits documents the independent bounded-retention controls applied
// to one internal session response stream. Enforcement is applied by the stream
// implementation when limits are exceeded.
type RetentionLimits struct {
	// MaxBytes bounds total PayloadBytes across retained events.
	MaxBytes int
	// MaxEvents bounds the number of retained events.
	MaxEvents int
	// MaxAge bounds how long retained events may remain buffered.
	MaxAge time.Duration
}

// DefaultRetentionLimits returns the documented default retention controls for
// one live Factory Session response stream.
func DefaultRetentionLimits() RetentionLimits {
	return RetentionLimits{
		MaxBytes:  1 << 20, // 1 MiB
		MaxEvents: 1024,
		MaxAge:    15 * time.Minute,
	}
}

// DefaultCompletedDispatchRetention returns how long a completed dispatch stream
// remains discoverable for late subscribers before the stream set evicts it.
func DefaultCompletedDispatchRetention() time.Duration {
	return DefaultRetentionLimits().MaxAge
}

// RetentionAccounting summarizes the current retained window for byte-size,
// event-count, and age-based retention decisions.
type RetentionAccounting struct {
	EventCount        int
	TotalPayloadBytes int
	OldestRecordedAt  time.Time
}

// ReadResult is the bounded catch-up view for one consumer resume point.
type ReadResult struct {
	Events                []Event
	BehindRetainedWindow  bool
	Compaction            *CompactionSummary
	FirstRetainedSequence int64
}

// CompactionSummary records fidelity loss for consumers that resume after
// truncation, coalescing, or age eviction.
type CompactionSummary struct {
	Reason                CompactionReason
	DroppedSequenceCount  int
	FirstRetainedSequence int64
	LastDroppedSequence   int64
}

// Event is the internal envelope for provider progress and response fragments
// within one Factory Session runtime. Unlike canonical factory events, these
// records are ephemeral, session-runtime-local, and excluded from durable
// replay contracts.
type Event struct {
	// Sequence is the monotonically increasing per-session-stream ordering key.
	// Retained events preserve ascending Sequence values after compaction.
	Sequence int64

	// RecordedAt supplies age-based retention accounting. Eviction compares
	// RecordedAt against the stream clock when MaxAge limits are enforced.
	RecordedAt time.Time

	// PayloadBytes is the byte size of Payload used for retention budgeting.
	// Downstream truncation uses the sum of PayloadBytes across retained events.
	PayloadBytes int

	// Kind distinguishes progress fragments, response fragments, and
	// compaction signals.
	Kind EventKind

	// Type records the provider-neutral semantic meaning of the event.
	Type EventType

	// DispatchID correlates one stream record with a workstation dispatch when set.
	DispatchID string

	// ProviderSessionRef correlates one stream record with a Provider Session
	// identity without promoting the record into canonical factory history.
	ProviderSessionRef *workerexecution.ProviderSessionMetadata

	// Payload carries the transient progress or response fragment body.
	Payload string

	// ExternalEventType preserves the original provider event name for
	// maintainer diagnostics without promoting provider-native schemas into
	// public contracts.
	ExternalEventType string

	// Metadata carries provider-boundary diagnostic identifiers such as runner,
	// workstation, or work ids that stay local to the internal stream.
	Metadata map[string]string

	// Compaction summarizes fidelity loss when Kind is EventKindCompactionSignal.
	Compaction *CompactionSummary
}

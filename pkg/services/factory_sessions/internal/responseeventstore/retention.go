package responseeventstore

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
)

const retentionGapReason = "retention_window"

// CompletedStreamRetentionWindow is how long a completed Factory Session's
// retained response events remain available to new subscribers.
const CompletedStreamRetentionWindow = 15 * time.Minute

const (
	defaultMaxRetainedEvents = 10_000
	defaultMaxRetainedBytes  = 16 * 1024 * 1024
)

// RetentionLimits are hard session-wide bounds for retained response events.
// Both values must be positive. MaxBytes counts the JSON serialization of each
// complete retained FactoryResponseEvent envelope, including identity,
// provenance, correlation fields, and payload.
type RetentionLimits struct {
	MaxEvents int
	MaxBytes  int
}

// DefaultRetentionLimits returns the bounded controls used by compatibility
// constructors that do not accept explicit limits.
func DefaultRetentionLimits() RetentionLimits {
	return RetentionLimits{
		MaxEvents: defaultMaxRetainedEvents,
		MaxBytes:  defaultMaxRetainedBytes,
	}
}

// RetentionAccounting reports the exact current retained count and serialized
// envelope bytes.
type RetentionAccounting struct {
	EventCount int
	TotalBytes int
}

// sequenceSpan is an inclusive range of unavailable published sequences.
// Spans stay disjoint and ordered. Their count is bounded by the retained
// records that separate them, so exact stale-cursor reporting does not create
// another unbounded per-session event history.
type sequenceSpan struct {
	from int64
	to   int64
}

func addDroppedSequence(spans []sequenceSpan, sequence int64) []sequenceSpan {
	index := sort.Search(len(spans), func(index int) bool {
		return spans[index].to >= sequence-1
	})
	if index == len(spans) {
		return append(spans, sequenceSpan{from: sequence, to: sequence})
	}
	if sequence < spans[index].from-1 {
		spans = append(spans, sequenceSpan{})
		copy(spans[index+1:], spans[index:])
		spans[index] = sequenceSpan{from: sequence, to: sequence}
		return spans
	}
	if sequence < spans[index].from {
		spans[index].from = sequence
	}
	if sequence > spans[index].to {
		spans[index].to = sequence
	}
	if index+1 < len(spans) && spans[index].to+1 >= spans[index+1].from {
		spans[index].to = spans[index+1].to
		copy(spans[index+1:], spans[index+2:])
		spans = spans[:len(spans)-1]
	}
	return spans
}

func droppedBoundsAfter(spans []sequenceSpan, afterSequence int64) (int64, int64, bool) {
	index := sort.Search(len(spans), func(index int) bool {
		return spans[index].to > afterSequence
	})
	if index == len(spans) {
		return 0, 0, false
	}
	from := spans[index].from
	if from <= afterSequence {
		from = afterSequence + 1
	}
	return from, spans[len(spans)-1].to, true
}

// SerializedEventSize returns the production byte-accounting size for an
// immutable retained event envelope.
func SerializedEventSize(event responseevents.FactoryResponseEvent) (int, error) {
	serialized, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("serialize response event for retention accounting: %w", err)
	}
	return len(serialized), nil
}

func validateRetentionLimits(limits RetentionLimits) error {
	if limits.MaxEvents <= 0 {
		return fmt.Errorf("%w: max retained events must be positive", ErrInvalidRetentionLimits)
	}
	if limits.MaxBytes <= 0 {
		return fmt.Errorf("%w: max retained bytes must be positive", ErrInvalidRetentionLimits)
	}
	return nil
}

type retentionTier uint8

const (
	retentionTierTransient retentionTier = iota
	retentionTierSnapshot
	retentionTierFinalSemantic
)

func eventRetentionTier(event responseevents.FactoryResponseEvent) retentionTier {
	if event.Phase == responseevents.PhaseDelta || event.Kind == responseevents.KindProgress {
		return retentionTierTransient
	}
	if isFinalSemanticEvent(event) {
		return retentionTierFinalSemantic
	}
	return retentionTierSnapshot
}

func isFinalSemanticEvent(event responseevents.FactoryResponseEvent) bool {
	switch event.Kind {
	case responseevents.KindMessage:
		return event.Phase == responseevents.PhaseCompleted
	case responseevents.KindTool:
		return event.Phase == responseevents.PhaseFailed
	case responseevents.KindRun:
		return event.Phase == responseevents.PhaseCompleted ||
			event.Phase == responseevents.PhaseFailed ||
			event.Phase == responseevents.PhaseCanceled
	default:
		return false
	}
}

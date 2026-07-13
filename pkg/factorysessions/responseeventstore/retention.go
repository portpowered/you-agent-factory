package responseeventstore

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
)

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

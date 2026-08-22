package process

import (
	"context"
	"errors"
	"fmt"
)

// CancellationReason identifies why a command context was deliberately
// canceled. It is carried by context cancellation so process cleanup can
// preserve intent without importing a higher-level Worker or Runtime type.
type CancellationReason string

const (
	CancellationReasonCanceled    CancellationReason = "CANCELED"
	CancellationReasonSuperseded  CancellationReason = "SUPERSEDED"
	CancellationReasonProcessGone CancellationReason = "PROCESS_GONE"
)

// CancellationCause is the safe context cause used for deliberate command
// cancellation. It still unwraps to context.Canceled so existing cancellation
// handling remains compatible while cleanup and logs retain the reason.
type CancellationCause struct {
	Reason CancellationReason
}

func (cause CancellationCause) Error() string {
	reason := cause.Reason
	if reason == "" {
		reason = CancellationReasonCanceled
	}
	return fmt.Sprintf("command execution canceled: %s", reason)
}

func (cause CancellationCause) Unwrap() error { return context.Canceled }

// NewCancellationCause creates a context cancellation cause with a bounded
// reason. Unknown or empty reasons are normalized to ordinary cancellation.
func NewCancellationCause(reason CancellationReason) error {
	if reason == "" {
		reason = CancellationReasonCanceled
	}
	switch reason {
	case CancellationReasonCanceled, CancellationReasonSuperseded, CancellationReasonProcessGone:
		return CancellationCause{Reason: reason}
	default:
		return CancellationCause{Reason: CancellationReasonCanceled}
	}
}

// CancellationReasonFromError extracts an explicit cancellation reason from
// an error chain. It returns empty for deadlines and ordinary failures.
func CancellationReasonFromError(err error) CancellationReason {
	if err == nil {
		return ""
	}
	var cause CancellationCause
	if errors.As(err, &cause) {
		return cause.Reason
	}
	var causePointer *CancellationCause
	if errors.As(err, &causePointer) && causePointer != nil {
		return causePointer.Reason
	}
	if errors.Is(err, context.Canceled) {
		return CancellationReasonCanceled
	}
	return ""
}

// CancellationReasonFromContext returns the explicit reason for a canceled
// context, or ordinary CANCELED when a caller used context.WithCancel.
func CancellationReasonFromContext(ctx context.Context) CancellationReason {
	if ctx == nil || ctx.Err() != context.Canceled {
		return ""
	}
	if reason := CancellationReasonFromError(context.Cause(ctx)); reason != "" {
		return reason
	}
	return CancellationReasonCanceled
}

func firstCancellationReason(reasons ...CancellationReason) CancellationReason {
	for _, reason := range reasons {
		if reason != "" {
			return reason
		}
	}
	return ""
}

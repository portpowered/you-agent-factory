// Package canonicalledger defines the Recordings-owned canonical ledger
// capability. Consumers outside Recordings use the Recordings root service
// instead of this parent-private subservice contract.
package canonicalledger

import (
	"context"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Service owns validate, sequence, idempotent append, retain, and reconnect-
// aware subscribe for canonical Factory events behind the Recordings root.
type Service interface {
	Append(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error)
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

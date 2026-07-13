package responseeventstore

import "errors"

var errNilStore = errors.New("response event store is nil")

// ErrStoreClosed indicates the store no longer accepts subscriptions or publishes.
var ErrStoreClosed = errors.New("session response event store is closed")

// ErrStoreCompleted indicates publication has finished and further publishes are rejected.
var ErrStoreCompleted = errors.New("session response event store publication is complete")

// ErrSubscriptionClosed indicates the subscription was detached or the store closed.
var ErrSubscriptionClosed = errors.New("session response event store subscription is closed")

// ErrInvalidDispatchFilter indicates a dispatch filter was requested without a dispatch identity.
var ErrInvalidDispatchFilter = errors.New("session response event store dispatch filter requires a dispatch identity")

var errInvalidDispatchFilter = ErrInvalidDispatchFilter

package responseeventstore

import "errors"

var errNilStore = errors.New("response event store is nil")

// ErrStoreClosed indicates the store no longer accepts subscriptions or publishes.
var ErrStoreClosed = errors.New("session response event store is closed")

// ErrStoreCompleted indicates publication has finished and further publishes are rejected.
var ErrStoreCompleted = errors.New("session response event store publication is complete")

// ErrStoreExpired indicates the completed stream's late-subscription retention
// window has elapsed. Existing subscribers are already closed by completion.
var ErrStoreExpired = errors.New("session response event store retention window has expired")

// ErrFactorySessionMismatch indicates an event names a different session than
// the session-scoped store receiving it.
var ErrFactorySessionMismatch = errors.New("response event factory session does not match store")

// ErrSubscriptionClosed indicates the subscription was detached or the store closed.
var ErrSubscriptionClosed = errors.New("session response event store subscription is closed")

// ErrInvalidDispatchFilter indicates a dispatch filter was requested without a dispatch identity.
var ErrInvalidDispatchFilter = errors.New("session response event store dispatch filter requires a dispatch identity")

// ErrInvalidRetentionLimits indicates that one or both hard limits are not positive.
var ErrInvalidRetentionLimits = errors.New("invalid session response event retention limits")

var errInvalidDispatchFilter = ErrInvalidDispatchFilter

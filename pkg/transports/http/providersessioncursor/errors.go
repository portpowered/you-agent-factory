package providersessioncursor

import "errors"

var (
	// ErrInvalidProviderSessionIdentifier marks unsafe or malformed session ids.
	ErrInvalidProviderSessionIdentifier = errors.New("invalid provider session identifier")
	// ErrProviderSessionNotFound marks a missing cursor session store.
	ErrProviderSessionNotFound = errors.New("provider session not found")
	// ErrAmbiguousProviderSessionFile marks multiple matching store.db files.
	ErrAmbiguousProviderSessionFile = errors.New("ambiguous provider session file")
)

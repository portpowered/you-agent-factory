package filesystem_watchers

import "errors"

var (
	// ErrInvalidResumeFacts reports malformed committed watcher facts supplied on resume.
	ErrInvalidResumeFacts = errors.New("filesystem watcher: invalid resume facts")
	// ErrStaleResumeFacts reports resume facts that are foreign, stale, or contradict authoritative state.
	ErrStaleResumeFacts = errors.New("filesystem watcher: stale resume facts")
	// ErrCursorPersistFailed reports that durable cursor facts could not be committed.
	ErrCursorPersistFailed = errors.New("filesystem watcher: cursor persist failed")
)

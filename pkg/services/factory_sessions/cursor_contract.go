package factorysessions

import (
	"io"
	"io/fs"

	cursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
)

// Reconnect cursor recovery contracts are exposed from the Factory Sessions
// root. Cursor tracking and persistence remain owner-internal.
type (
	CursorIdentityScope   = cursors.IdentityScope
	CursorPreflightReason = cursors.PreflightReason
	CursorPreflightResult = cursors.PreflightResult
	CursorStore           = cursors.Store
)

// CursorPersistenceFileSystem is the exact filesystem capability used by the
// reconnect-cursor store after Factory Sessions has selected its paths and
// permissions. Wire selects the policy-free host adapter.
type CursorPersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	Remove(string) error
	Rename(string, string) error
}

// CursorPersistenceTemporaryFile is the complete atomic-write handle needed
// by the reconnect-cursor store.
type CursorPersistenceTemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

// CursorPersistenceCreateTemporaryFile reserves an atomic-write file beneath
// the already-selected persistence directory.
type CursorPersistenceCreateTemporaryFile func(
	string,
	string,
) (CursorPersistenceTemporaryFile, error)

// CursorStoreFactory opens one explicitly rooted reconnect-cursor store from
// the external effects selected once by Wire.
type CursorStoreFactory func(string) (cursors.Store, error)

const (
	CursorPreflightStale           = cursors.PreflightCursorStale
	CursorPreflightSessionNotFound = cursors.PreflightSessionNotFound
	CursorPreflightSessionRemapped = cursors.PreflightSessionRemapped
)

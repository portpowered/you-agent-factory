package internal

import (
	"database/sql"
	"io"
	"io/fs"
)

// Construction/process-edge ports below are Wire and owner-constructor inputs.
// They are intentionally kept out of the peer-facing Service method signatures
// and are not the published source of truth for Inspect/Project/Details slices.

// FileSystem is a construction/process-edge storage port used when Wire or
// owner-local constructors assemble a production Service. It is not part of the
// peer-facing Provider Sessions root contract; cross-service callers invoke
// Service methods without supplying filesystem effect ports.
type FileSystem interface {
	Open(string) (io.ReadCloser, error)
	Stat(string) (fs.FileInfo, error)
}

// ResolveHomeDirectory is a construction/process-edge port that supplies the
// process home used to derive provider-owned default storage roots. Peers do
// not pass this through Service method signatures.
type ResolveHomeDirectory func() (string, error)

// CodexWalkDirectory is a construction/process-edge port that traverses the
// configured Codex session tree. It is not part of the peer-facing root
// contract surface.
type CodexWalkDirectory func(string, fs.WalkDirFunc) error

// CodexResolveSymlinks is a construction/process-edge port that resolves Codex
// session paths before containment checks. It is not part of the peer-facing
// root contract surface.
type CodexResolveSymlinks func(string) (string, error)

// CursorWalkDirectory is a construction/process-edge port that traverses the
// configured Cursor session storage tree. It is not part of the peer-facing
// root contract surface.
type CursorWalkDirectory func(string, fs.WalkDirFunc) error

// CursorResolveSymlinks is a construction/process-edge port that resolves
// Cursor storage paths before containment checks. It is not part of the
// peer-facing root contract surface.
type CursorResolveSymlinks func(string) (string, error)

// CursorOpenSQLDatabase is a construction/process-edge port that opens a Cursor
// database driver connection. It is not part of the peer-facing root contract
// surface.
type CursorOpenSQLDatabase func(driverName, dataSourceName string) (*sql.DB, error)

// OperatingSystem is a construction/process-edge value identifying the platform
// whose provider storage convention should be selected. It is not part of the
// peer-facing root contract surface.
type OperatingSystem string

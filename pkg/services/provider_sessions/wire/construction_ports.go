package wire

import (
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
)

// Construction/process-edge port aliases for owner wire and process-edge bags.
// Canonical definitions live in provider_sessions/internal; peers depend on
// Service rather than these ports.
type (
	FileSystem            = providersessionsinternal.FileSystem
	ResolveHomeDirectory  = providersessionsinternal.ResolveHomeDirectory
	CodexWalkDirectory    = providersessionsinternal.CodexWalkDirectory
	CodexResolveSymlinks  = providersessionsinternal.CodexResolveSymlinks
	CursorWalkDirectory   = providersessionsinternal.CursorWalkDirectory
	CursorResolveSymlinks = providersessionsinternal.CursorResolveSymlinks
	CursorOpenSQLDatabase = providersessionsinternal.CursorOpenSQLDatabase
	OperatingSystem       = providersessionsinternal.OperatingSystem
)

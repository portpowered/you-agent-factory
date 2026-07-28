// Package service is a transitional compile shim for DEL-PSES. Production
// construction should use provider_sessions/wire; the composed root lives in
// provider_sessions/internal/service.
package service

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
)

// New delegates to the owner-private composed root in internal/service.
func New(
	files providersessionsinternal.FileSystem,
	resolveHome providersessionsinternal.ResolveHomeDirectory,
	codexWalkDirectory providersessionsinternal.CodexWalkDirectory,
	codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	cursorOperatingSystem providersessionsinternal.OperatingSystem,
) (providersessions.Service, error) {
	return internalservice.New(
		files,
		resolveHome,
		codexWalkDirectory,
		codexResolveSymlinks,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		cursorOperatingSystem,
	)
}

// NewForRoots delegates to the owner-private composed root in internal/service.
func NewForRoots(
	files providersessionsinternal.FileSystem,
	codexWalkDirectory providersessionsinternal.CodexWalkDirectory,
	codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	codexRoot, cursorRoot string,
) (providersessions.Service, error) {
	return internalservice.NewForRoots(
		files,
		codexWalkDirectory,
		codexResolveSymlinks,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		codexRoot,
		cursorRoot,
	)
}

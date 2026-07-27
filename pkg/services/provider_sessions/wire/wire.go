// Package wire is the Provider Sessions service composition boundary.
//
// Wire performs construction only, returns the singular providersessions.Service
// root interface, and starts no lifecycle components. Parent-private Codex and
// Cursor reader wiring stays inside the owner service assembly path; peers
// depend on Service rather than reader internals or construction ports.
package wire

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
)

// NewService constructs an inert Provider Sessions root from construction and
// process-edge ports. It composes the accepted root through parent-private
// Codex/Cursor reader construction without publishing reader types on the
// returned peer surface.
func NewService(
	files providersessions.FileSystem,
	resolveHome providersessions.ResolveHomeDirectory,
	codexWalkDirectory providersessions.CodexWalkDirectory,
	codexResolveSymlinks providersessions.CodexResolveSymlinks,
	cursorWalkDirectory providersessions.CursorWalkDirectory,
	cursorResolveSymlinks providersessions.CursorResolveSymlinks,
	cursorOpenDatabase providersessions.CursorOpenSQLDatabase,
	cursorOperatingSystem providersessions.OperatingSystem,
) (providersessions.Service, error) {
	return providersessionsservice.New(
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

// NewForRoots constructs Provider Sessions with explicit Codex and Cursor
// storage roots. Missing required construction ports fail with a deterministic
// construction error and a nil service.
func NewForRoots(
	files providersessions.FileSystem,
	codexWalkDirectory providersessions.CodexWalkDirectory,
	codexResolveSymlinks providersessions.CodexResolveSymlinks,
	cursorWalkDirectory providersessions.CursorWalkDirectory,
	cursorResolveSymlinks providersessions.CursorResolveSymlinks,
	cursorOpenDatabase providersessions.CursorOpenSQLDatabase,
	codexRoot, cursorRoot string,
) (providersessions.Service, error) {
	return providersessionsservice.NewForRoots(
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

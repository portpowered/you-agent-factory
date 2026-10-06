// Package wire is the Provider Sessions service composition boundary.
//
// Wire performs construction only, returns the singular providersessions.Service
// root interface, and starts no lifecycle components. Captured Codex and parent-private
// Cursor reader wiring stays inside the owner service assembly path; peers
// depend on Service rather than reader internals or construction ports.
package wire

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService constructs an inert Provider Sessions root from construction and
// process-edge ports. It composes the accepted root through parent-private
// captured Codex projection and Cursor reader construction without publishing reader types on the
// returned peer surface.
func NewService(
	files FileSystem,
	resolveHome ResolveHomeDirectory,
	cursorWalkDirectory CursorWalkDirectory,
	cursorResolveSymlinks CursorResolveSymlinks,
	cursorOpenDatabase CursorOpenSQLDatabase,
	cursorOperatingSystem OperatingSystem,
	captured recordings.WorkerCapturedActivityReader,
) (providersessions.Service, error) {
	return internalservice.New(
		files,
		resolveHome,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		cursorOperatingSystem,
		captured,
	)
}

// NewForRoots constructs Provider Sessions with an explicit Cursor
// storage root and required captured activity reader. Missing required construction ports fail with a deterministic
// construction error and a nil service.
func NewForRoots(
	files FileSystem,
	cursorWalkDirectory CursorWalkDirectory,
	cursorResolveSymlinks CursorResolveSymlinks,
	cursorOpenDatabase CursorOpenSQLDatabase,
	cursorRoot string,
	captured recordings.WorkerCapturedActivityReader,
) (providersessions.Service, error) {
	return internalservice.NewForRoots(
		files,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		cursorRoot,
		captured,
	)
}

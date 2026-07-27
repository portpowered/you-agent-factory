// Package wire constructs the parent-private Cursor Provider Session reader.
package wire

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	cursorreaderservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/internal/service"
)

// NewService constructs an inert Cursor reader from its exact storage effects.
func NewService(
	files providersessions.FileSystem,
	walkDirectory providersessions.CursorWalkDirectory,
	resolveSymlinks providersessions.CursorResolveSymlinks,
	openDatabase providersessions.CursorOpenSQLDatabase,
	root string,
) (cursorreader.Service, error) {
	return cursorreaderservice.New(files, walkDirectory, resolveSymlinks, openDatabase, root)
}

// DefaultStorageRoot returns the configured platform's Cursor storage root.
func DefaultStorageRoot(
	resolveHome providersessions.ResolveHomeDirectory,
	files providersessions.FileSystem,
	operatingSystem providersessions.OperatingSystem,
) (string, error) {
	return cursorreaderservice.DefaultStorageRoot(resolveHome, files, operatingSystem)
}

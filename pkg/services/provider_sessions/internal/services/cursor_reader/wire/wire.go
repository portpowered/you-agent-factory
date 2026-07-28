// Package wire constructs the parent-private Cursor Provider Session reader.
package wire

import (
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	cursorreaderservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/internal/service"
)

// NewService constructs an inert Cursor reader from its exact storage effects.
func NewService(
	files providersessionsinternal.FileSystem,
	walkDirectory providersessionsinternal.CursorWalkDirectory,
	resolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	openDatabase providersessionsinternal.CursorOpenSQLDatabase,
	root string,
) (cursorreader.Service, error) {
	return cursorreaderservice.New(files, walkDirectory, resolveSymlinks, openDatabase, root)
}

// DefaultStorageRoot returns the configured platform's Cursor storage root.
func DefaultStorageRoot(
	resolveHome providersessionsinternal.ResolveHomeDirectory,
	files providersessionsinternal.FileSystem,
	operatingSystem providersessionsinternal.OperatingSystem,
) (string, error) {
	return cursorreaderservice.DefaultStorageRoot(resolveHome, files, operatingSystem)
}

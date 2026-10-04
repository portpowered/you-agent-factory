// Package wire constructs the scoped cursor recovery owner.
package wire

import (
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	cursorscopesservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/internal/service"
)

type CursorPersistenceFileSystem = cursorscopesservice.CursorPersistenceFileSystem

// NewService stores the filesystem effect without performing IO.
func NewService(files CursorPersistenceFileSystem) cursorscopes.CursorScopes {
	return cursorscopesservice.New(files)
}

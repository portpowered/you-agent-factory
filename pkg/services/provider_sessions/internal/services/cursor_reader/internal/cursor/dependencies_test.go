package cursor

import (
	"database/sql"
	"path/filepath"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
)

var (
	testFiles           platformfilesystem.Local
	testWalkDirectory   = providersessionsinternal.CursorWalkDirectory(filepath.WalkDir)
	testResolveSymlinks = providersessionsinternal.CursorResolveSymlinks(filepath.EvalSymlinks)
	testOpenSQLDatabase = providersessionsinternal.CursorOpenSQLDatabase(sql.Open)
)

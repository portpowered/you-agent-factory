package cursor

import (
	"database/sql"
	"path/filepath"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

var (
	testFiles           platformfilesystem.Local
	testWalkDirectory   = providersessions.CursorWalkDirectory(filepath.WalkDir)
	testResolveSymlinks = providersessions.CursorResolveSymlinks(filepath.EvalSymlinks)
	testOpenSQLDatabase = providersessions.CursorOpenSQLDatabase(sql.Open)
)
